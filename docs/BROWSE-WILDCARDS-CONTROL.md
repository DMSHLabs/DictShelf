# Browse and Wildcards ribbon: code review

Status: code review only, 2026-10-19. No implementation decision has been made.

## Current findings

- Browse and Wildcards do **not** use one shared ribbon component. The small headword-search control at the bottom is shared; each page's section ribbon, width calculations, virtual rendering, scroll model and click handling are separate.
- Both ribbons show navigation labels for headword pages and virtualize DOM buttons: only visible entries plus a buffer are created.
- Browse operates on one dictionary. Its normal page and alphabet come from that backend's `dict.Browser` implementation: generally the prepared SQLite headword index, with supported self-indexed formats able to provide Browse data from their native index. The ribbon labels represent the dictionary's Browse sections/pages. Browse's filtered list has a separate per-result-page ribbon.
- Wildcards operates over dictionaries in group order. A selective mask is matched against indexed headwords for each supported dictionary; matching rows stream into the result view. An optional per-pattern/group SQLite cache stores ordered result blocks for paging and reuse. Opening a result then fetches the article from that dictionary.
- A pattern consisting only of `*` takes a special path: it pages the complete headword list through `dict.Browser` and stores visited pages in the Wildcards cache. It does not scan article text. Its per-page labels are derived while walking the headword list.
- Wildcards' ribbon has its own logical scroll coordinate, custom thumb, dictionary separators and dictionary/page targets. Browse uses a horizontally scrollable element with its own scrollbar and offsets. The click target is one section in Browse, but a dictionary plus local page in Wildcards.
- Both create only visible ribbon elements, but their geometry work differs. Browse measures the first `BrowseStripExactWidthCount` labels (currently 500) and estimates remaining widths from their mean. Wildcards computes widths and offsets for the complete label list when rebuilding the ribbon. All-words (`*`) uses fixed-width chips; selective masks measure each full label without a maximum width.
- Browse currently caps visible chip labels at 12 em, applies an ellipsis and retains the full text in `title`. Wildcards currently applies ellipsis only to its fixed-width all-words chips; selective-mask labels have no matching cap. Its chips already receive a `title` containing dictionary/count context and the full label.

## Questions to resolve, in order

1. **What should be shared?** Likely candidate: common chip sizing/rendering and virtual-window geometry, with separate data adapters and navigation callbacks. Decide whether the scroll/thumb behavior should also be common.
2. **What is a section label?** Keep Browse's dictionary section labels distinct from Wildcards' result-page labels; confirm both use the same truncation and tooltip rules.
3. **How should widths scale?** Apply Browse's 12 em cap to Wildcards selective masks? Should Wildcards also use the first-N exact / remaining-average geometry for long ribbons, or do result counts justify a different strategy?
4. **How should selection work?** Browse selects a dictionary offset; Wildcards selects a dictionary and page across a concatenated multi-dictionary ribbon. Preserve the existing group order and separators.
5. **How should progressive updates behave?** Wildcards rows and their section labels arrive asynchronously and may be provisional. Keep the current per-dictionary synchronization and make sure a shared renderer does not expose stale labels, lose selection, or rebuild the full geometry too often.

This note records observations and decision points. It is not a commitment to rewrite either ribbon or to change the current wildcard cache/index design.
