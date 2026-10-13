---
title: macOS app
description: wuDict.app - the wudict server as a macOS app bundle, with a menu-bar icon and no Dock icon or terminal.
---

# macOS app

`wuDict.app` is the command-line `wudict` packaged as a macOS app bundle. It runs
without a terminal and shows a **menu-bar icon**.

## Install

1.  Download **`wudict-macos-universal-app-<version>.zip`** from
    [the releases page](https://github.com/wuweidict/wudict/releases). It is a
    universal binary, for Apple silicon and Intel.
2.  Unzip it and move **wuDict.app** to *Applications*.
3.  Open the app. macOS blocks the first launch ("Apple could not verify…"):
    click **Done**, open *System Settings → Privacy & Security*, and click
    **Open Anyway**. On macOS 14 and earlier, Control-click the app and choose
    **Open** instead.

The app is signed with an ad-hoc certificate, not with an Apple Developer ID, so Gatekeeper blocks
it on first launch. Removing the quarantine attribute before the first launch
has the same effect as step 3:

``` sh
/usr/bin/xattr -cr /Applications/wuDict.app
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
| **Quit wuDict** | stop the server |

The app has no Dock icon and no window. Closing the browser tab does not stop
the server; **Quit wuDict** does. The log is `~/Library/Logs/wudict.log`.

Launching the app while the wudict server is already running just opens the page in the
browser.

## macOS app bundle vs LaunchAgent

| | `wuDict.app` | [LaunchAgent](../running.md) |
| --- | --- | --- |
| Starts | when you open it | at login |
| Visible | menu-bar icon | nothing |
| Stops | **Quit wuDict** | `make mac-agent-stop`, or logout |

Use one or the other.

## The binary inside the bundle

``` sh
/Applications/wuDict.app/Contents/MacOS/wudict --version
/Applications/wuDict.app/Contents/MacOS/wudict lookup ~/Dictionaries/Oxford.mdx serendipity
```

Started from a terminal, it behaves as the command-line `wudict`: output goes
to the terminal and no menu-bar icon appears.

## Uninstall

1.  **Quit wuDict** from the menu bar.
2.  Move `wuDict.app` to the Bin.
3.  Optionally delete `~/.wudict` (settings and library, unless `DB_DIR` points
    elsewhere) and `~/Library/Logs/wudict.log`.

## Next

[Set the dictionary folders](../start/first-run.md){ .md-button }
