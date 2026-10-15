---
title: Run at startup
description: Start wudict at login - a macOS LaunchAgent, a Linux systemd user unit, or the Windows Startup folder.
---

# Run at startup

The macOS and Linux setups use `make` targets in a clone of the
[source repository](https://github.com/wuweidict/wudict).

=== "macOS"

    ## LaunchAgent

    | Command | Action |
    | --- | --- |
    | `make mac-agent-install` | build `wudict`, generate the LaunchAgent plist, and register it; runs `wudict serve --no-browser` (`AGENT_BIN=<path>` for another binary) |
    | `make mac-agent-start` | load and start the agent |
    | `make mac-agent-stop` | stop and unload the agent |
    | `make mac-agent-restart` | rebuild, then restart the agent |
    | `make mac-agent-status` | show the agent's state and process id |
    | `make mac-agent-uninstall` | stop the agent and delete the plist |

    Check with `make mac-agent-status`, then open
    [localhost:6888](http://localhost:6888).

    [wuDict.app](apps/macos.md) is the alternative that starts when you open
    it and shows a menu-bar icon. Use one of the two.

=== "Linux"

    ## systemd user unit

    The unit runs as your user. Only installing the binary to
    `/usr/local/bin` needs `sudo`.

    | Command | Action |
    | --- | --- |
    | `make linux-service-install` | install the binary (sudo), then create the user unit |
    | `make linux-service-start` | enable and start the unit |
    | `make linux-service-stop` | stop the unit |
    | `make linux-service-restart` | rebuild, reinstall the binary, restart |
    | `make linux-service-status` | show the unit's state |
    | `make linux-service-uninstall` | disable and remove the unit; the binary stays |

    The unit runs `/usr/local/bin/wudict`. `make linux-install` installs only
    the binary; `PREFIX=/opt/wudict` changes the location.

    A user unit is managed with `systemctl --user`, e.g.
    `systemctl --user status wudict`. To keep it running after logout:

    ``` sh
    sudo loginctl enable-linger "$(id -un)"
    ```

    Logs: `journalctl --user -u wudict -f`.

=== "Windows"

    ## Installer

    The installer's *Start wuDict2 at sign-in* task puts a shortcut to
    `wuDict2.exe --no-browser` in the Startup folder. See
    [Windows installer](apps/windows.md).

    ## By hand

    1. Put `wuDict2.exe` in a folder, e.g. `C:\tools\wudict\`.
    2. <kbd>Win</kbd>+<kbd>R</kbd>, enter `shell:startup`.
    3. Create a shortcut to `wuDict2.exe` in that folder, and add
       `--no-browser` to its target.

    Started this way, `wuDict2.exe` shows a tray icon and logs to
    `%LOCALAPPDATA%\wudict\wudict.log`. Sign out and in, then open
    [localhost:6888](http://localhost:6888).

## Settings for a background server

| Setting | Effect |
| --- | --- |
| [`NO_BROWSER`](reference/configuration.md#no_browser) | no browser tab at startup |
| [`TRAY`](reference/configuration.md#tray) | tray or menu-bar icon on or off |
| [`VERBOSE`](reference/configuration.md#verbose) | log requests, dictionary opens, indexing and audio conversion |
