---
title: Share a dictionary
description: Share a dictionary as a link - any address with the dictionary link after "#", which opens DictShelf on Android.
---

<style>
/* This page is an address to hand out, not an article to read through: the
   manual's sidebar, its "On this page" list, the breadcrumb and the prev/next
   links would all be answering questions nobody arrives here with. The search
   box is hidden site-wide (stylesheets/extra.css). */
body:has(.share-page) .md-sidebar--primary,
body:has(.share-page) .md-sidebar--secondary,
body:has(.share-page) .md-path,
body:has(.share-page) .md-footer__inner{display:none}
</style>

<div class="share-page" markdown>

# Share a dictionary

A share link is any address with the link to a dictionary, an archive or a
collection after `#`:

``` text
https://dictshelf.com/share/#https://example.org/dicts/oxford.zip
```

The part after `#` is the dictionary; the page in front of it is only where a
browser goes when DictShelf does not open the link, and a browser never sends
the fragment to a server — so any page can carry one, and the page needs to know
nothing about dictionaries. A single leading slash is short for a path on the
sharing site itself (`#/dict/x/`), and several links may follow the `#`,
separated by a comma, a semicolon, a bar or a space.

On Android with DictShelf installed, opening such a link starts the import in the
app; elsewhere, paste it on the setup page.

## What happens when it is opened

| Where | What |
| --- | --- |
| Android with DictShelf installed | the import starts in the app: `/share/` on this site is a verified App Link |
| Everywhere else | this page opens, and the link is pasted into *Add dictionaries* → **Import a link** |

## What can be behind the `#`

| After `#` | Result |
| --- | --- |
| a dictionary file or an archive | that dictionary |
| a web folder page that lists dictionary files | everything on it, as a list to choose from |
| a text file listing links or file names | the files it names |
| several links, separated by a comma, a semicolon, a bar or a space | all of them |
| `/dict/…`, one leading slash | a path on the site that shared the link |

## Without a page of your own

You do not need this page to share a dictionary: any address with the link after
`#` works, so wrapping the link in a page you already have is enough.

[Add dictionaries → Import](dictionaries/add.md#import-a-file-or-a-link){ .md-button }

</div>
