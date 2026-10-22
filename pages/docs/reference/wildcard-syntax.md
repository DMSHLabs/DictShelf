---
title: Wildcard syntax
description: How a wildcard query is read - ? and *, the whole headword, case and accents, what is searched, result order, and limits.
---

# Wildcard syntax

A wildcard query is a pattern for the whole headword. Choose **wildcard** in
the drop-down next to the search box.

## Wildcards

| Character | Matches | Pattern | Finds |
| --- | --- | --- | --- |
| `?` | exactly one character, a space included | `c?t` | *cat*, *cot*, *cut* |
| `*` | any number of characters, none included | `bio*` | *bio*, *biology*, *biome* |

`**` is the same as `*`. Every other character matches only itself, `[`, `]`
and `\` included.

## The whole headword

A pattern matches from the first character of the headword to the last.

| Pattern | Finds | Does not find |
| --- | --- | --- |
| `apple` | *apple*, *Apple* | *apples*, *pineapple* |
| `apple*` | *apple*, *apples*, *apple pie* | *pineapple* |
| `*apple` | *apple*, *pineapple* | *apples* |
| `*apple*` | *apple*, *apples*, *pineapple*, *apple pie* | |

A pattern without `*` or `?` finds only the headword it spells.

## Case and accents

Case and accents do not exclude a headword. `cafe`, `Cafe` and `café` all
find *cafe*, *café*, *Café* and *CAFÉ*. `koln` finds *Köln*, `елка` finds
*ёлка*.

A capital letter or an accent in the pattern sets the order of the results
([Result order](#result-order)):

-   A capital letter puts the headwords with that case first: `March` lists
    *March* before *march*.
-   An accent puts the headwords with that accent first: `café*` lists
    *café* and *Café* before *cafe*, and `äh*` lists *Ähnlichkeit* before
    *ah*.
-   With both, case comes first: `Café*` lists *Café*, then *café*, then
    *cafe*.

A capital letter is an uppercase or titlecase letter in any script: `M`, `É`,
`Ж`, `ǅ`. An accent is a mark that a letter loses when accents are ignored:
`é`, `ü`, `ñ`, `ё`, `й`.

`ß`, `ł`, `ø` and `đ` are letters of their own, not accented letters:
`strasse` does not find *Straße*.

## Examples

| To find | Pattern |
| --- | --- |
| five-letter words that start with *b* and end with *d* | `b???d` |
| three-letter words with *a* in the middle | `?a?` |
| words that end in *-ology* | `*ology` |
| words that start with *un* and end with *able* | `un*able` |
| a word with one letter you are not sure of | `s?parate` |
| headwords that start with the phrase *by the* | `by the *` |
| headwords of two or more words | `* *` |

## What is searched

-   Headwords of indexed dictionaries. A dictionary gets its headword index in
    the background on its first search. Until then, wildcard search does not
    search it, and its result section offers to add the headword index.
-   The headword an entry is filed under. Other spellings that the dictionary
    lists for the same entry are not matched; exact and prefix search find
    them.
-   Not the article text: use [full-text search](fts-syntax.md).

A query that contains `*` or `?` is not lemmatized. A query without them is
lemmatized as in the other modes
([Inflected words](../start/search.md#inflected-words)).

A followed link, a double-clicked word and a word from another app are looked
up in prefix mode, not as a pattern.

## Result order

1.  The headword the pattern spells, exactly as typed. A pattern spells a
    headword when it has no `?`: the pattern with every `*` matching
    nothing. `nacion*` spells *nacion*, `un*able` spells *unable*.
2.  The other results, by case and accents
    ([Case and accents](#case-and-accents)).
3.  Within the same case and accents: the headword the pattern spells, then
    the others in alphabetical order, as in a printed dictionary:
    -   Letters count first, without accents and case: *kohlen*, *Kohler*,
        *Köhler*.
    -   Between headwords that are otherwise the same, the one without an
        accent comes first: *Kohler* before *Köhler*.
    -   Then lowercase before capital: *über* before *Über*.

This is the order of DIN 5007-1 and of the Duden, and of the Unicode
Collation Algorithm. `ß` sorts as a letter after `z`, not as `ss`.

| Pattern | Results, in order |
| --- | --- |
| `nacion*` | *nación*, *nacional*… |
| `march` | *march*, *March* |
| `March` | *March*, *march* |
| `über*` | *über*, *über-*, *Über-*, *Über-Ich*, *überaktiv*, *Überaktivität*, *überall*… |
| `äh*` | *äh*, *Ähnlichkeit*, *Ähre*…, then *ah*, *Ah*… |

<kbd>Browse A–Z…</kbd> uses a different order: case is ignored, and a
headword that starts with an accented or non-Latin letter comes after *Z*.

The number of results per dictionary is the same as in the other modes
([Result order](../start/search.md#result-order)).

## Limits

-   A pattern has at most 64 characters.
-   There are no character sets (`[aeiou]`), alternatives or repeat counts.
    Write `???` for exactly three characters.
-   `*` and `?` cannot be searched as themselves. To find a headword with
    one of them, use exact or prefix.
-   A pattern that starts with `*` or `?` reads every headword of every
    searched dictionary, so it is slower on a large library.

## In a link, the API and the terminal

In a link and in the API, write `?` as `%3F`:

```
http://localhost:6888/?q=b%3F%3F%3Fd&mode=wildcard&dict=all
```

In a terminal, put the pattern in single quotes, so that the shell does not
expand `*` and `?`:

``` sh
wudict wildcard ~/.wudict/db/Oxford 'b???d'
wudict searchall -mode wildcard '*ology'
```
