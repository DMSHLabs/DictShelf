---
title: Share a dictionary
description: Share a dictionary as a link - any address with the dictionary link after "#", which opens DictShelf on Android.
---

# Share a dictionary

A **share link** is any address with the link to a dictionary after `#`:

``` text
https://dictshelf.com/share/#https://example.org/dicts/oxford.zip
```

Nothing has to be uploaded first: the part after `#` is the dictionary, and the
page in front of it is only where a browser lands when DictShelf does not open
the link. A browser never sends the fragment to a server, so *any* page can carry
one — this one exists so that a link says where it came from.

## What happens when it is opened

| Where | What |
| --- | --- |
| Android with DictShelf installed | the import starts in the app: `/share/` on this site is a verified App Link |
| Anywhere else | this page opens, and the link is pasted into *Add dictionaries* → **Import a link** |

## What can be behind the `#`

| After `#` | Result |
| --- | --- |
| a dictionary file or an archive | that dictionary |
| a web folder page that lists dictionary files | everything on it, as a list to choose from |
| a text file listing links or file names | the files it names |
| several links, separated by a comma, a semicolon, a bar or a space | all of them |
| `/dict/…`, one leading slash | a path on the site that shared the link |

[Add dictionaries → Import](dictionaries/add.md#import-a-file-or-a-link){ .md-button }

## Without a page of your own

You do not need this page to share a dictionary: any address with the link after
`#` works, so wrapping the link in a page you already have is enough. The rules
are in [Add dictionaries → Share links](dictionaries/add.md#share-links).
