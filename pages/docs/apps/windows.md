---
title: Windows installer
description: The wuDict2 installer for Windows - install modes, optional tasks, the tray icon, upgrades, silent install and uninstall.
---

# Windows installer

The installer is optional. It installs `wuDict2.exe` for the desktop and
`wuDict2-cli.exe` for terminal commands, alongside a Start menu entry, an
uninstaller and four optional actions.
It runs on 64-bit Windows on x64, and on Arm64 Windows 11 (x64 emulation). On
other Arm64 systems use `wudict-windows-arm64-purego.exe`.

## Install

1.  Download **`wudict2-windows-x64-setup-<version>.exe`** from
    [the releases page](https://github.com/wuweidict/wudict/releases).
2.  Run it. The file is not code-signed, so Windows shows *Windows protected
    your PC*: choose **More info**, then **Run anyway**.
3.  Choose an install mode:

    | Mode | Needs | Installs to |
    | --- | --- | --- |
    | **For all users** (default) | administrator rights | `C:\Program Files\wuDict2` |
    | **For me only** | | `%LOCALAPPDATA%\Programs\wuDict2` |

4.  Select the options.
5.  Finish. **Start wuDict2 now** starts the server.

The browser opens [localhost:6888](http://localhost:6888), and the **wuDict2**
icon appears in the notification area.

## Options

| Task                        | Default | Effect                                                        |
|-----------------------------| --- |---------------------------------------------------------------|
| Create a desktop shortcut   | off | a desktop shortcut                                            |
| Start wuDict2 at sign-in    | off | a Startup folder shortcut that runs `wuDict2.exe --no-browser` |
| Add wuDict2 to `%PATH%`     | on | makes `wuDict2-cli` command available in PowerShell and `cmd` |
| Offer wuDict2 in *Open with*| on | for `.mdx`, `.dsl`, `.slob`, `.bgl` and `.zim` files |

The uninstaller removes all four. *Start wuDict2 at sign-in* is the same as the
manual Startup folder shortcut in [Run at startup](../running.md); use one of
the two.

## Console or tray

The desktop `wuDict2.exe` runs without a console window and shows a **tray
icon**. Use `wuDict2-cli.exe` from PowerShell or `cmd` for commands and output.
The log is written to
`%LOCALAPPDATA%\wudict\wudict.log`.

The desktop and CLI builds share settings and library files. If one is active, a second operational launch displays a warning and exits.

| Tray menu | Action |
| --- | --- |
| **Open wuDict2** | open the page in the browser |
| **Rescan dictionaries** | re-read the dictionary folders |
| **Open dictionary folder** | show the dictionary folder in Explorer |
| **Quit wuDict2** | stop the server |

## Upgrade

[Download](https://github.com/wuweidict/wudict/releases/latest) and run the latest installer.
It keeps the install mode, closes a running
`wuDict2.exe`, and does not restart unless asked. Existing dictionary files, `wudict.toml` and the
library are not changed.

## Silent install

``` pwsh title="all users"
.\wudict2-windows-x64-setup-<version>.exe /ALLUSERS /VERYSILENT /NORESTART
```

``` pwsh title="current user"
.\wudict2-windows-x64-setup-<version>.exe /CURRENTUSER /VERYSILENT /NORESTART
```

`/ALLUSERS` and `/CURRENTUSER` skip the install mode page. A silent install
does not start the server.

## Uninstall

<kbd>Settings</kbd> → <kbd>Apps</kbd> → <kbd>Installed apps</kbd> → <kbd>wuDict2</kbd> → <kbd>Uninstall</kbd> removes the program,
the shortcuts, the `PATH` entry and the *Open with* entries. Dictionary files,
`%USERPROFILE%\.wudict` (settings and library) and the log remain; delete them
by hand if you no longer need them.

## Next

[Set the dictionary folders](../start/first-run.md){ .md-button }
