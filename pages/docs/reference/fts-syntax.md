---
title: Full-text query syntax
description: How a full-text query is read - quotes, AND, OR, NOT, NEAR, prefixes, precedence and limits.
---

# Full-text query syntax

## How a query is read

A query is read in one of two ways:

-   **Unquoted**: no quotes and no operators. It is matched in stages: phrase,
    proximity, anywhere ([Unquoted queries](../start/search.md#unquoted-queries)).
    Each word also matches as a prefix (`pun` finds *puns*); in the phrase
    stage, only the last word.
-   **Explicit**: the query contains a quote, or `AND`, `OR`, `NOT` or `NEAR(`
    in capitals. It is run once, as written, without further stages.

Operators are recognized only in capitals: `wage NOT minimum` excludes
*minimum*; `wage not minimum` searches for three words. Parentheses alone, as
in `(coll.)`, are ordinary text.

## Quotes

A quoted phrase matches its words adjacent and in order; its last word is not
a prefix. Four pairs of quotes are accepted: `"…"`, `'…'`, `“…”`, `‘…’`. A
single quote opens a phrase only at the start of a word and closes it only at
the end of a word, so in `don't cry`, `l'année` and `dogs' lives` it is an
apostrophe.

A `*` after the closing quote makes the last word a prefix:
`"no pun intended"*` also finds *no pun intendedly*.

## Operators

| Query | Matches |
| --- | --- |
| `pun` | words that start with *pun* |
| `"pun"` | the word *pun* only |
| `pun intended` | both words, anywhere in the article (in an explicit query) |
| `pun AND intended` | the same |
| `pun OR intended` | either word |
| `pun NOT intended` | *pun*, in articles that do not contain *intended* |
| `NEAR("pun" "intended", 5)` | both words, at most 5 words between them, in any order |
| `NEAR("pun" "intended")` | the same, at most 10 words between them |
| `(pun OR joke) AND intended` | grouping; parentheses nest |

## Precedence

Highest first:

| Operator | Example | Read as |
| --- | --- | --- |
| `NOT` | `a OR b NOT c` | `a OR (b NOT c)` |
| `AND`, or two terms side by side | `a OR b c` | `a OR (b AND c)` |
| `OR` | | |

Parentheses override precedence.

## Examples

| Goal | Query |
| --- | --- |
| The phrase only | `"no pun intended"` |
| The phrase; else the words near each other; else anywhere | `no pun intended` |
| Both words, at most 10 words between them | `NEAR("pun" "intended")` |
| Both words, anywhere in the article | `"pun" AND "intended"` |
| *pun* or *joke*, with *intended* | `(pun OR joke) intended` |
| A fixed expression, except in articles mentioning linguistics | `"faux ami" NOT linguistics` |
| Two spellings | `"colour blind" OR "color blind"` |
| A word close to another | `NEAR("bank" "river", 4)` |
| An abbreviation and its expansion in one article | `NEAR("i.e." "that is", 6)` |
| Every word that starts with a stem | `"lexicograph"*` |
| A remembered fragment of a definition | `sudden fear of` |
| Exclude a sense | `crane NOT bird` |
| One fixed condition and one of two alternatives | `"phrasal verb" AND (idiom OR colloquial)` |

## Case, accents and punctuation

Case and accents are ignored, as in the other modes: `"corazon"` finds
*corazón*. Punctuation in a query is split on the same word boundaries as the
article text: `"i.e."` matches *i.e.*, and `"no-pun-intended"` matches like
`"no pun intended"`.

## Not supported

-   Field filters such as `headword:` or `definition:`. `title:foo` is read as
    the words *title* and *foo*.
-   `-word`, and `NOT` without a left operand: write `wage NOT minimum`.
-   `*` inside or at the start of a word: `*age` and `w*ge` are literal text.
-   Spelling correction. Inflected forms are handled by
    [lemmatization](../start/search.md#inflected-words).
-   Regular expressions.

## Invalid queries and limits

A query that is not a valid explicit query (an unclosed quote, unbalanced
parentheses, an operator without an operand) is read as an unquoted query. No
query returns a syntax error; `OR` alone therefore searches for the word *or*.

An unquoted query uses its first 128 words. An explicit query of more than 128
tokens, or with parentheses nested more than 24 levels deep, is read as an
unquoted query.
