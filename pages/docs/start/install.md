---
title: Install
description: Download and run wudict on macOS, Linux, Windows or Android - one executable, no dependencies.
---

# Install

`wudict` is one executable per platform, with no runtime or database server to
install. macOS also has an app bundle, Windows an installer, Android an APK.

[Releases](https://github.com/wuweidict/wudict/releases/latest){ .md-button .md-button--primary }

## Download and run

=== "macOS"

    Apple silicon Macs (M-series) use `arm64`, Intel Macs `amd64`.

    ``` sh title="Apple silicon (arm64), downloaded to ~/Downloads"
    chmod +x ~/Downloads/wudict-darwin-arm64-cgo
    mv ~/Downloads/wudict-darwin-arm64-cgo /usr/local/bin/wudict
    wudict
    ```

    ``` sh title="Intel (amd64)"
    chmod +x ~/Downloads/wudict-darwin-amd64-cgo
    mv ~/Downloads/wudict-darwin-amd64-cgo /usr/local/bin/wudict
    wudict
    ```

    If macOS blocks the first run ("Apple could not verify…"), open *System
    Settings → Privacy & Security* and click **Open Anyway**, or remove the
    quarantine attribute: `xattr -d com.apple.quarantine /usr/local/bin/wudict`.

    The app bundle `wudict-macos-universal-app-<version>.zip` contains
    **wuDict.app**: the same program, universal, with a menu-bar icon instead
    of a terminal. See [macOS app](../apps/macos.md).

=== "Linux"

    ``` sh title="amd64"
    chmod +x wudict-linux-amd64-cgo
    sudo mv wudict-linux-amd64-cgo /usr/local/bin/wudict
    wudict
    ```

    ``` sh title="Raspberry Pi"
    # Pi 3, 4, 5, Zero 2 W, 64-bit OS  -> wudict-linux-arm64-cgo
    # Pi 2; Pi 3, 4, 5 on a 32-bit OS   -> wudict-linux-arm-v7-purego
    # Pi 1, Zero, Zero W                -> wudict-linux-arm-v6-purego
    chmod +x wudict-linux-arm64-cgo
    sudo mv wudict-linux-arm64-cgo /usr/local/bin/wudict
    wudict
    ```

=== "Windows"

    ``` cmd title="cmd"
    wuDict2.exe
    ```

    ``` pwsh title="PowerShell"
    .\wuDict2.exe
    ```

    On Arm64 Windows use `wudict-windows-arm64-purego.exe` where available.

    The file is not code-signed, so Windows Defender SmartScreen stops its
    first run: choose **More info**, then **Run anyway**.

    `wuDict2.exe` is a Windows app and opens without a console window. For
    terminal commands, use `wuDict2-cli.exe`; it writes to the terminal and
    returns an exit code. Desktop launches show a tray icon and log to
    `%LOCALAPPDATA%\wudict\wudict.log`.

    The installer `wudict2-windows-x64-setup-<version>.exe` adds a Start menu
    entry, an uninstaller, `PATH` and *Open with*. See
    [Windows installer](../apps/windows.md).

=== "Android"

    Build **`wudict2-android-arm64-v8a-foss.apk`** with
    `build-android.cmd release` on Windows, then install the signed APK with
    your file manager; Android asks once to allow *Install unknown apps*. The
    Google Play build is on Google Play. See [Android app](../apps/android.md).

## Check

``` sh
wudict --version
wudict
```

`wudict` prints the address it listens on and opens the browser at
[localhost:6888](http://localhost:6888). `wudict --help` lists every command
and setting.

## -cgo or -purego

Most platforms have a **`-cgo`** and a **`-purego`** build. They differ in two
components:

| | `-cgo` | `-purego` |
| --- | --- | --- |
| SQLite driver | mattn/go-sqlite3 (C) | modernc.org/sqlite (Go); slightly slower |
| Speex `.spx` audio | built-in decoder | needs `speexdec` (`brew install speex`, `apt install speex`) |

Use `-cgo` where it exists: macOS, Linux on amd64 and arm64, Windows on x64.
Use `-purego` for 32-bit Raspberry Pi and Arm64 Windows. Building `-cgo`
yourself needs a C compiler; see [Building](../reference/building.md).

## Update

Replace the executable with the new one; on Windows, stop the server first.
The config file, the library and the dictionary files are kept, and the new
version reads them. Indexes built by an older version are reported as
[outdated](../dictionaries/library.md#outdated-indexes).

The installer, `wuDict.app` and the APK are updated by installing the new
version over the old one.

## Uninstall

Delete the executable. `~/.wudict` holds the settings and the library; delete
it as well to remove everything.

## Next

[Set the dictionary folders](first-run.md){ .md-button .md-button--primary }
