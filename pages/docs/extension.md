---
title: Browser extensions
description: What a browser extension needs from DictShelf - the three read-only endpoints it uses, and BROWSER_EXTENSIONS.
status: new
---

# Browser extensions

An extension can look words up in DictShelf while you read: it calls the same
HTTP API the page itself uses and shows the answer in a popup, without leaving
the page you are on.

**DictShelf does not publish an extension of its own yet.** This page is the
contract, not a download: what an extension needs, and what the server allows.
Watch [the repository](https://github.com/DMSHLabs/DictShelf) — when ours is
published, it will be named here and linked from there.

## What an extension uses

Three read-only endpoints, and nothing else:

| Endpoint | For |
| --- | --- |
| `/api/dicts` | the dictionary list, with each dictionary's supported search modes |
| `/api/search` | the lookup itself; `format=clean` answers with about half the bytes and no stylesheets or scripts |
| `/res/` | an article's images and audio |

They need no [access key](access.md#access-key), so an extension also works
against a server on another computer in your network.
[`BROWSER_EXTENSIONS`](reference/configuration.md#browser_extensions) decides
which extensions may call them.

Two things worth knowing before writing one:

- An ordinary web page cannot call a server on your own machine; an extension's
  service worker can, through its host permissions.
- The server answers the CORS preflight on `/api/search`, so a Manifest V3
  extension needs no workaround — and its other requests are CORS-simple and
  never preflight at all.

[HTTP API](reference/api.md){ .md-button }
[Access from other devices](access.md){ .md-button }
