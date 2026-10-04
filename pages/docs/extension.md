---
title: wudict Hover browser extension
description: wudict Hover for Chrome and Firefox - definitions from your wudict server in a popup, on hover or from the context menu.
status: new
---
<style>
.md-typeset .badges{display:flex;flex-wrap:wrap;gap:.6rem;align-items:center;justify-content:center}
.md-typeset .badges img{height:80px;width:auto;max-width:none}
</style>

# wudict Hover browser extension

wudict Hover shows definitions from your wudict server in a popup on any web
page: when you hover over a word while holding a key, or from the context menu.

<div class="badges">
  <a href="https://chromewebstore.google.com/detail/bknaaoffefipfnpefmkbipcdemljbhjh">
    <img alt="Available in the Chrome Web Store" height="80" src="assets/chrome-web-store-badge.png" />
  </a>
  <a href="https://addons.mozilla.org/firefox/addon/wudict-hover/">
    <img alt="Get the Add-on for Firefox" height="80" src="assets/firefox-get-the-addon.svg" />
  </a>
</div>

Requires a running wudict server (by default `http://127.0.0.1:6888`, or on
another computer in your network) and Chrome 116 or Firefox 128 or later.

[Run at startup](running.md){ .md-button }

## Use

| Action | Result |
| --- | --- |
| hover over a word, holding <kbd>Alt</kbd> | the popup, with exact matches for the word |
| select text, right-click → **Look up "…" in wudict** | a search for the selection |
| <kbd>Alt</kbd>+<kbd>Shift</kbd>+<kbd>W</kbd> | a search for the selection |
| <kbd>Alt</kbd>+<kbd>W</kbd>, or the toolbar icon | the extension's panel, with a search box |

A word with no exact match is retried as variants: apostrophe, possessive,
hyphen and suffix forms. Audio plays in the popup. A link in the popup opens
its target in the wudict page.

A search from the panel, the context menu or the shortcut opens the wudict
page, with all dictionaries; **Search opens** in the options can choose the
popup instead. Hover always uses the popup.

Hover lookup can be paused everywhere or on one site: from the panel, the
icon's context menu, or a keyboard shortcut assigned in the browser's
extension shortcut settings.

## Options

| Option | Default |
| --- | --- |
| Base URL | `http://127.0.0.1:6888` |
| Hold key | <kbd>Alt</kbd>; also <kbd>Ctrl</kbd>, <kbd>Shift</kbd>, <kbd>Meta</kbd>, or none |
| Hover delay | 200 ms |
| Search opens | the full wudict page |
| Right-click menu | on |
| Dictionaries per lookup | 3, chosen automatically; up to 8, or a fixed selection |
| Entries per dictionary | 1 (up to 10) |
| Fallback attempts | 4 (up to 8) |

**Test connection** checks the base URL.

## How it works

The popup requests articles in the `clean` format: about half the size of the
original markup, without stylesheets or scripts. The extension's service worker
sends the requests, since browsers do not let ordinary web pages call a server
on your machine, and caches up to 400 answers (8 MB).

The server gives extensions three read-only endpoints: `/api/dicts`,
`/api/search` and `/res/`. They need no access key, so the extension also works
with a server on another computer.
[`BROWSER_EXTENSIONS`](reference/configuration.md#browser_extensions) limits
which extensions may call them.

If the popup reports that the server does not answer extensions, the server is
older than the extension: [update wudict](start/install.md#update).

Source: [github.com/wuweidict/wudict-browser-extension](https://github.com/wuweidict/wudict-browser-extension)

## Install an unpublished build

For development, or for features not yet in the stores.

=== "Chrome"

    1. Unzip the extension.
    2. Open `chrome://extensions` and turn on **Developer mode**.
    3. **Load unpacked**, and choose the folder.

=== "Firefox"

    1. Open `about:addons`.
    2. Gear menu → **Install Add-on From File**, and choose the `.xpi` file.

    Alternatively, `about:debugging#/runtime/this-firefox` →
    **Load Temporary Add-on** → `manifest.json` in the unpacked folder. A
    temporary add-on is removed when Firefox restarts.
