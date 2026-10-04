---
title: Windows installer
description: The wudict installer for Windows - install modes, optional tasks, the tray icon, upgrades, silent install and uninstall.
---

# Windows installer

The installer is optional. It installs the same `wudict.exe` that is offered as
a separate download, and adds a Start menu entry, an uninstaller and four
optional actions.

It runs on 64-bit Windows on x64, and on Arm64 Windows 11 (x64 emulation). On
other Arm64 systems use `wudict-windows-arm64-purego.exe`.

## Install

1.  Download **`wudict-windows-x64-setup-<version>.exe`** from
    [the releases page](https://github.com/wuweidict/wudict/releases).
2.  Run it. The file is not code-signed, so Windows shows *Windows protected
    your PC*: choose **More info**, then **Run anyway**.
3.  Choose an install mode:

    | Mode | Needs | Installs to |
    | --- | --- | --- |
    | **For all users** (default) | administrator rights | `C:\Program Files\wuDict` |
    | **For me only** | | `%LOCALAPPDATA%\Programs\wuDict` |

4.  Select the options.
5.  Finish. **Start wuDict now** starts the server.

The browser opens [localhost:6888](http://localhost:6888), and the **wuDict**
icon appears in the notification area.

## Options

| Task                        | Default | Effect                                                        |
|-----------------------------| --- |---------------------------------------------------------------|
| Create a desktop shortcut   | off | a desktop shortcut                                            |
| Start wuDict at sign-in     | off | a Startup folder shortcut that runs `wudict.exe --no-browser` |
| Add wuDict to `%PATH%`      | on | makes `wudict` command available in PowerShell and `cmd`   |
| Offer wuDict in *Open with* | on | for `.mdx`, `.dsl`, `.slob`, `.bgl` and `.zim` files          |

The uninstaller removes all four. *Start wuDict at sign-in* is the same as the
manual Startup folder shortcut in [Run at startup](../running.md); use one of
the two.

## Console or tray

Started from PowerShell or `cmd`, `wudict.exe` is a console program: it outputs
to the console and returns an exit code.

Started from the Start menu, a shortcut, or by opening a dictionary file, wudict
runs without a console and shows a **tray icon**. The log is written to
`%LOCALAPPDATA%\wudict\wudict.log`.

| Tray menu | Action |
| --- | --- |
| **Open wuDict** | open the page in the browser |
| **Rescan dictionaries** | re-read the dictionary folders |
| **Open dictionary folder** | show the dictionary folder in Explorer |
| **Quit wuDict** | stop the server |

## Upgrade

[Download](https://github.com/wuweidict/wudict/releases/latest) and run the latest installer.
It keeps the install mode, closes a running
`wudict.exe`, and does not restart unless asked. Existing dictionary files, `wudict.toml` and the
library are not changed.

## Silent install

``` pwsh title="all users"
.\wudict-windows-x64-setup-<version>.exe /ALLUSERS /VERYSILENT /NORESTART
```

``` pwsh title="current user"
.\wudict-windows-x64-setup-<version>.exe /CURRENTUSER /VERYSILENT /NORESTART
```

`/ALLUSERS` and `/CURRENTUSER` skip the install mode page. A silent install
does not start the server.

## Uninstall

<kbd>Settings</kbd> → <kbd>Apps</kbd> → <kbd>Installed apps</kbd> → <kbd>wuDict</kbd> → <kbd>Uninstall</kbd> removes the program,
the shortcuts, the `PATH` entry and the *Open with* entries. Dictionary files,
`%USERPROFILE%\.wudict` (settings and library) and the log remain; delete them
by hand if you no longer need them.

## Next

[Set the dictionary folders](../start/first-run.md){ .md-button }
