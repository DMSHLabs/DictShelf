# Browse and Wildcards headword control

Both views use the `wudictHeadwordControl` surface for the shared search/mask dock and ribbon helpers. The ribbon keeps one geometry and virtual-painting implementation: binary-search window bounds, 12 em chip caps with ellipsis, proportional scrollbar thumb sizing, and exact width measurement for the first `indexconst.BrowseStripExactWidthCount` entries followed by the measured mean.

There are two source adapters. `fromBrowseIndex` converts the prepared dictionary index's alphabet/page anchors into chips that navigate by headword offset. `fromWildcardSQL` converts the ordered per-dictionary results backed by the per-mask SQLite cache into chips, separators and provisional positions that navigate by dictionary and local page. The adapters do not change either source's query/build lifecycle: Browse reads its prepared index; Wildcards builds its SQL cache and serves subsequent result pages from that cache.

In Wildcards, each dictionary's visible ribbon span gets one of four low-saturation theme-mixed background tints, cycling in dictionary order. Virtual painting clips the tinted spans to the visible chip window; the chips, active state and separators stay interactive and above the tint.

The shared ribbon scroller also owns thumb geometry, drag and track clicks, keyboard, wheel, touch, and arrow-button movement. Browse adapts its native `scrollLeft` position and commits the nearest visible anchor; Wildcards adapts its logical offset and previews/commits a dictionary plus local page. Data loading and selection callbacks remain at each page edge. Build validation does not establish visual/runtime behavior; check Browse and multi-dictionary plus all-words Wildcards in Windows and Android builds.
