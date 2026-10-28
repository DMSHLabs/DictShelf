## What’s New

### Lingvo LSD support

DictShelf can now open Lingvo LSD dictionaries.

### Dictzip v2 (`.dz2`)

The original Dictzip format stores the uncompressed file size in a 32-bit field, limiting dictionary size to about 2 GB. Dictzip v2 uses a 64-bit size field, allowing larger dictionaries. The block size remains the same as in v1.

### Global hotkeys

Use hotkeys to show or hide the DictShelf window, or look up selected text from another app.

Shortcuts can use a single key combination, such as `Ctrl+Shift+B`, or a two-press sequence, such as `Ctrl+Shift+B, Ctrl+Shift+B`.

### Update checks

DictShelf checks for new versions and can notify you when an update is available. Open **☰ → Update** to view update options.

### Expanded search modes

#### Search each word of the headword

These modes match individual words within a headword. For a multiword headword such as *work in progress*, a match in any one of its words is enough.

| Pattern | Matches | Example |
|---|---|---|
| `ab→` | Words that start with the query | `work` → *work*, *worked*, *workshop* |
| `→ab` | Words that end with the query | `work` → *work*, *network*, *homework* |
| `ab` | Words that contain the query anywhere | `work` → *work*, *network*, *homework*, *working* |
| `·ab·` | An exact word match | `worked` → *worked* |
| `·ab·+` | An exact word or lemma match | `worked` → *worked*, *work* |

#### Search the whole headword

These modes match the entire headword rather than individual words within it.

| Pattern | Matches | Example |
|---|---|---|
| `=` | An exact headword match | `work in` → *work in* |
| `=+` | An exact headword or lemma match; both the query and headword must be one word | `worked` → *worked*, *work* |
| `*?` | A headword matching a pattern: `*` matches any number of characters; `?` matches exactly one | `*wo?k*` → *work*, *works*, *network*, *homework*, *work in progress* |

#### Search article text

**Search the text** looks inside dictionary articles, not just their headwords. For a multiword query, DictShelf tries these match types in order, moving to the next only if the previous one finds no results:

1. The complete phrase, with all words in order and next to each other.
2. All query words, in any order, with no more than 10 words between them.
3. Each query word anywhere in the article.

### Faster results, loaded in batches

Search displays the first five matching articles from each dictionary, then continues finding all matches in the background. Additional articles are loaded in batches set by **Settings → Results → Results per portion**.

Use **Browse** in a dictionary’s header to open the list of all its headwords.

### Optional article sections

Choose how optional sections in dictionary articles are displayed.

### Browse and Wildcards improvements

Search and display performance has been improved in **Browse** and **Wildcards** modes.

### Interface refinements

Includes various interface improvements and fixes.
