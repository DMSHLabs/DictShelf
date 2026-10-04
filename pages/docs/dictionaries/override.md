---
title: Resource overrides
description: Serve your own copy of a dictionary's stylesheet, script, image or audio file from its library folder.
---

# Resource overrides

A **resource override** is a file in a library folder's `res/` subfolder.
wudict serves it in place of the dictionary's own resource at the same path, or
in addition, if the dictionary lacks it. Use it to replace a broken stylesheet
or script, or to supply a missing image.

The dictionary files are not modified. Delete the override to restore the
dictionary's own resource.

## Create an override

1. Find the library folder: <kbd>☰</kbd> → the dictionary's file row lists its
   path.
2. Create `res/` inside it.
3. Put the file at the path the article requests.

``` text title="~/.wudict/db/Cambridge English Dictionary Online/"
text.db
res/
  jquery.js      replaces the dictionary's copy
  js/entry.js    supplies a file the dictionary lacks
  css/style.css  replaces the dictionary's stylesheet
```

The path below `res/` is the path the article requests: an article that loads
`js/entry.js` needs `res/js/entry.js`.

## Find the requested path

In the browser's network panel, a resource request reads
`/res/<dictionary id>/<path>`. `<path>` is the path to use under `res/`; a `404`
marks a missing resource.

wudict logs a warning when it serves a `.js`, `.css`, `.html`, `.json`, `.xml`,
`.svg` or `.txt` resource that contains NUL bytes, which these formats cannot
contain, and names the `res/` path that would override it.

## Serving

Overrides are served with `Cache-Control: no-cache`: a reload shows a changed
file.

A `.spx` file in `res/` is served as is, not converted to WAV. Browsers cannot
play Speex: use `.mp3` or `.wav`.

## Dictionary scripts

An article that contains scripts is rendered in a sandboxed frame with its own
document, styles and scripts. Its scripts cannot reach wudict's page; `parent`
exposes only a small fixed interface.

Scripts that test for a host application, as many MDict dictionaries do, find
none. Host functions they call return empty values, and the rest of the article
works.

If a dictionary's script raises an error, the dictionary's result header shows
⚠ (the tooltip holds the message) and the browser console logs one line. The
article is displayed without what the script would have done. The usual cause
is a broken or missing `.js` resource, which an override fixes.
