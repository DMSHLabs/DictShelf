---
title: Share a dictionary
description: Share a dictionary as a link - any address with the dictionary link after "#", which opens DictShelf on Android.
---

<style>
/* This page is an address to hand out, not an article to read through: its "On
   this page" list, the breadcrumb and the prev/next links would all be answering
   questions nobody arrives here with. The SIDEBAR STAYS - it is the menu the
   header's "Navigation" button opens, and hiding it left that button opening an
   empty panel, which is exactly what the reader hit. The page sections go with
   it: the theme renders them as a nested md-nav--secondary INSIDE the
   primary nav, so hiding the sidebar alone left them in the menu. The
   search box is hidden site-wide (stylesheets/extra.css). */
body:has(.share-page) .md-sidebar--secondary,
body:has(.share-page) .md-nav--primary .md-nav--secondary,
body:has(.share-page) .md-path,
body:has(.share-page) .md-footer__inner{display:none}
/* The one control this page has: the way into the app when the browser got the
   link instead (see the script below). Hidden until that script shows it. */
.md-typeset .open-in-app{text-align:center;margin:0 0 1.6rem}
/* the attribute hides it; this makes it survive a theme rule that sets
   a display of its own on paragraphs */
.md-typeset .open-in-app[hidden]{display:none}
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

<p class="open-in-app" id="open-in-app-wrap" hidden>
  <a id="open-in-app" class="md-button md-button--primary">Open in DictShelf</a>
</p>

<script>
/* A browser tab is where a share link lands when Android did not hand it to the
   app: Android 11 and below have no App Links at all, Android 12+ may not have
   verified this domain on this device yet, and a messenger's own browser or
   Custom Tab never hands an App Link over. The extra below is what the app
   reads (Intake.java, EXTRA_URL): an intent: URL cannot carry the payload as
   data, because a share link's payload IS its fragment - the very part intent:
   syntax consumes.
   Shown on Android only, and only when there is something after "#": a bare
   /share/ has nothing to import, and the app would answer that it is a page of
   dictionary links. */
(function () {
  var wrap = document.getElementById('open-in-app-wrap');
  var link = document.getElementById('open-in-app');
  if (!wrap || !link) return;
  if (!/Android/i.test(navigator.userAgent)) return;
  if (location.hash.length < 2) return;
  var pkg = 'com.dmshlabs.dictshelf';
  link.href = 'intent://dictshelf.com/share/#Intent;scheme=https;package=' + pkg +
    ';S.' + pkg + '.extra.INTAKE_URL=' + encodeURIComponent(location.href) + ';end';
  wrap.hidden = false;
})();
</script>

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


</div>
