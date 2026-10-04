---
title: Picker groups
description: Groups in the dictionary picker - Language, Language pair and groups.ini - and how to edit them.
---

# Picker groups

The dictionary picker offers groups of dictionaries besides *All dictionaries*
and single dictionaries. Choosing a group searches its enabled dictionaries.

| Section | Groups |
| --- | --- |
| **Language** | dictionaries by headword language |
| **Language pair** | dictionaries by source and target language |
| sections in `groups.ini` | defined by you; the default file has **Content** (encyclopedias, thesauri, idioms, …) and **Publisher** (Oxford, Cambridge, Collins, …) |

Language and language pair come from the same
[language detection](../reference/configuration.md#lemmatization) as
lemmatization. A group that contains every dictionary is not offered.
<kbd>☰</kbd> → **Group by** chooses which sections the picker shows.

## groups.ini

<kbd>☰</kbd> → folder summary → <kbd>Edit groups…</kbd> opens the editor.
<kbd>Save</kbd> writes `groups.ini` beside the `wudict.toml` in effect;
<kbd>Reset to default</kbd> deletes it, and the built-in default applies again.

``` ini title="groups.ini"
; a comment
[Content]
Thesauri = thesaurus, synonyms
Law = legal, `\blaw\b`

[Mine]
Learner's = oald, ldoce, `^cobuild`
```

-   `[Section]` starts a section: a heading in the picker and in **Group by**.
-   `Name = item, item` defines a group; `:` may replace `=`.
-   A dictionary belongs to a group when an item matches its title or its file
    name (with extension).
-   A plain item matches anywhere, ignoring case. It needs at least 2
    characters.
-   An item in backticks is a regular expression (Go RE2), ignoring case,
    tested against the title and the file name separately. `,`, `;` and `#`
    inside the backticks belong to the pattern.
-   `;` and `#` start a comment, on a line of its own or after the items.

Sections you add are listed first in the picker, above Language; the default
sections follow Language pair. Up to 32 sections, with names of up to 64
bytes; *Language* and *Language pair* are reserved.

The editor lists lines it cannot use (an invalid regular expression, an item
of one character) and the groups that match no dictionary. The rest of the file
applies. While there are problems, the folder summary shows ⚠ and
<kbd>Edit groups… (N)</kbd>.

## Link to a group

A group can be selected in a link: `dict=g:<section>:<group>`, e.g.
`/?q=bank&dict=g:lang:en`. See [Link to a search](../start/search.md#link-to-a-search).
