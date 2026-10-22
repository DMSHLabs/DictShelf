---
title: macOS app
description: DictShelf.app - the wudict server as a macOS app bundle, with a menu-bar icon and no Dock icon or terminal.
---

# macOS app

`DictShelf.app` is the command-line `wudict` packaged as a macOS app bundle. It runs
without a terminal and shows a **menu-bar icon**.

## Install

1.  Download **`wudict-macos-universal-app-<version>.zip`** from
    [the releases page](https://github.com/wuweidict/wudict/releases). It is a
    universal binary, for Apple silicon and Intel.
2.  Unzip it and move **DictShelf.app** to *Applications*.
3.  Open the app. macOS blocks the first launch ("Apple could not verify…"):
    click **Done**, open *System Settings → Privacy & Security*, and click
    **Open Anyway**. On macOS 14 and earlier, Control-click the app and choose
    **Open** instead.

The app is signed with an ad-hoc certificate, not with an Apple Developer ID, so Gatekeeper blocks
it on first launch. Removing the quarantine attribute before the first launch
has the same effect as step 3:

``` sh
/usr/bin/xattr -cr /Applications/DictShelf.app
```

The menu bar then shows the wuDict icon, and the browser opens
[localhost:6888](http://localhost:6888).

## Menu-bar icon

| Entry | Action |
| --- | --- |
| *wuDict `<version>`* | none; shows the running version |
| **Open wuDict** | open the page in the browser |
| **Rescan dictionaries** | re-read the dictionary folders |
| **Open dictionary folder** | show the dictionary folder in Finder |
| **Quit DictShelf** | stop the server |

The app has no Dock icon and no window. Closing the browser tab does not stop
the server; **Quit DictShelf** does. The log is `~/Library/Logs/DMSH Labs/DictShelf/dictshelf.log`.

Launching the app while the wudict server is already running just opens the page in the
browser.

## macOS app bundle vs LaunchAgent

| | `DictShelf.app` | [LaunchAgent](../running.md) |
| --- | --- | --- |
| Starts | when you open it | at login |
| Visible | menu-bar icon | nothing |
| Stops | **Quit DictShelf** | `make mac-agent-stop`, or logout |

Use one or the other.

## The binary inside the bundle

``` sh
/Applications/DictShelf.app/Contents/MacOS/wudict --version
/Applications/DictShelf.app/Contents/MacOS/wudict lookup ~/Dictionaries/Oxford.mdx serendipity
```

Started from a terminal, it behaves as the command-line `wudict`: output goes
to the terminal and no menu-bar icon appears.

## Uninstall

1.  **Quit DictShelf** from the menu bar.
2.  Move `DictShelf.app` to the Bin.
3.  Optionally delete `~/.dmshlabs/dictshelf` (settings and library, unless `DB_DIR` points
    elsewhere) and `~/Library/Logs/DMSH Labs/DictShelf/dictshelf.log`.

## Next

[Set the dictionary folders](../start/first-run.md){ .md-button }
