---
title: Windows installer
description: The DictShelf installer for Windows - install modes, optional tasks, the tray icon, upgrades, silent install and uninstall.
---

# Windows installer

The installer is optional. It installs `DictShelf.exe` for the desktop and
terminal commands, alongside a Start menu entry, an uninstaller and four
optional actions.
It runs on 64-bit Windows on x64, and on Arm64 Windows 11 (x64 emulation). On
other Arm64 systems use `wudict-windows-arm64-purego.exe`.

## Install

1.  Download **`dictshelf-windows-x64-setup-<version>.exe`** from
    [the releases page](https://github.com/DMSHLabs/DictShelf/releases).
2.  Run it. The file is not code-signed, so Windows shows *Windows protected
    your PC*: choose **More info**, then **Run anyway**.
3.  Choose an install mode:

    | Mode | Needs | Installs to |
    | --- | --- | --- |
    | **For all users** (default) | administrator rights | `C:\Program Files\wuDict2` |
    | **For me only** | | `%LOCALAPPDATA%\Programs\wuDict2` |

4.  Select the options.
5.  Finish. **Start DictShelf now** starts the server.

The browser opens [localhost:6888](http://localhost:6888), and the **DictShelf**
icon appears in the notification area.

## Options

| Task                        | Default | Effect                                                        |
|-----------------------------| --- |---------------------------------------------------------------|
| Create a desktop shortcut   | off | a desktop shortcut                                            |
| Start DictShelf at sign-in    | off | a Startup folder shortcut that runs `DictShelf.exe --no-browser` |
| Add DictShelf to `%PATH%`     | on | makes the `wuDict2` command available in PowerShell and `cmd` |
| Offer DictShelf in *Open with*| on | for `.mdx`, `.dsl`, `.slob`, `.bgl` and `.zim` files |

The uninstaller removes all four. *Start DictShelf at sign-in* is the same as the
manual Startup folder shortcut in [Run at startup](../running.md); use one of
the two.

## Console or tray

`DictShelf.exe` opens the app without a console window and shows a **tray
icon**. The same executable accepts commands such as `DictShelf.exe --help` from
PowerShell or `cmd`. The log is written to
`%USERPROFILE%\.dmshlabs\dictshelf\db.logs\dictshelf.log`.

If the app is already active, a second operational launch displays a warning and exits.

DictShelf keeps its files in `%USERPROFILE%\.dmshlabs\dictshelf`. On first launch, if
`%USERPROFILE%\.wudict` exists, DictShelf checks its `state.json` for DictShelf-only
settings. If it finds them, it asks whether to copy or move DictShelf settings,
styles and lemma files into the new folder. Otherwise, it offers to import only
the dictionary folders and lemmas, or start with **Edit Folders**. Dictionary
indexes are not copied; DictShelf rebuilds them in its own format. The old wuDict
server is stopped before files are copied when its settings identify a running
instance.

| Tray menu | Action |
| --- | --- |
| **Open DictShelf** | open the page in the browser |
| **Rescan dictionaries** | re-read the dictionary folders |
| **Open dictionary folder** | show the dictionary folder in Explorer |
| **Quit DictShelf** | stop the server |

## Upgrade

[Download](https://github.com/DMSHLabs/DictShelf/releases/latest) and run the latest installer.
It keeps the install mode, closes a running
`DictShelf.exe`, and does not restart unless asked. Existing dictionary files, `dictshelf.toml` and the
library are not changed.

## Silent install

``` pwsh title="all users"
.\dictshelf-windows-x64-setup-<version>.exe /ALLUSERS /VERYSILENT /NORESTART
```

``` pwsh title="current user"
.\dictshelf-windows-x64-setup-<version>.exe /CURRENTUSER /VERYSILENT /NORESTART
```

`/ALLUSERS` and `/CURRENTUSER` skip the install mode page. A silent install
does not start the server.

## Uninstall

<kbd>Settings</kbd> → <kbd>Apps</kbd> → <kbd>Installed apps</kbd> → <kbd>DictShelf</kbd> → <kbd>Uninstall</kbd> removes the program,
the shortcuts, the `PATH` entry and the *Open with* entries. Dictionary files,
`%USERPROFILE%\.dmshlabs\dictshelf` (settings and library) and the log remain; delete them
by hand if you no longer need them.

## Next

[Set the dictionary folders](../start/first-run.md){ .md-button }
