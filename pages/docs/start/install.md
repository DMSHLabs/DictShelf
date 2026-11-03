---
title: Install
description: Download and run DictShelf on macOS, Linux, Windows or Android - one executable, no dependencies.
---

# Install

`dictshelf` is one executable per platform, with no runtime or database server to
install. macOS also has an app bundle, Windows an installer, Android an APK.

[Releases](https://github.com/DMSHLabs/DictShelf/releases/latest){ .md-button .md-button--primary }

## Download and run

=== "macOS"

    Apple silicon Macs (M-series) use `arm64`, Intel Macs `amd64`.

    ``` sh title="Apple silicon (arm64), downloaded to ~/Downloads"
    chmod +x ~/Downloads/dictshelf-darwin-arm64-cgo
    mv ~/Downloads/dictshelf-darwin-arm64-cgo /usr/local/bin/dictshelf
    dictshelf
    ```

    ``` sh title="Intel (amd64)"
    chmod +x ~/Downloads/dictshelf-darwin-amd64-cgo
    mv ~/Downloads/dictshelf-darwin-amd64-cgo /usr/local/bin/dictshelf
    dictshelf
    ```

    If macOS blocks the first run ("Apple could not verify…"), open *System
    Settings → Privacy & Security* and click **Open Anyway**, or remove the
    quarantine attribute: `xattr -d com.apple.quarantine /usr/local/bin/dictshelf`.

    The app bundle `dictshelf-macos-universal-app-<version>.zip` contains
    **DictShelf.app**: the same program, universal, with a menu-bar icon instead
    of a terminal. See [macOS app](../apps/macos.md).

=== "Linux"

    ``` sh title="amd64"
    chmod +x dictshelf-linux-amd64-cgo
    sudo mv dictshelf-linux-amd64-cgo /usr/local/bin/dictshelf
    dictshelf
    ```

    ``` sh title="Raspberry Pi (64-bit OS)"
    # Pi 3, 4, 5, Zero 2 W -> dictshelf-linux-arm64-cgo
    chmod +x dictshelf-linux-arm64-cgo
    sudo mv dictshelf-linux-arm64-cgo /usr/local/bin/dictshelf
    dictshelf
    ```

    Releases are built for 64-bit Linux only: 32-bit Raspberry Pi OS is not
    supported.

=== "Windows"

    ``` cmd title="cmd"
    DictShelf.exe
    ```

    ``` pwsh title="PowerShell"
    .\DictShelf.exe
    ```

    The file is not code-signed, so Windows Defender SmartScreen stops its
    first run: choose **More info**, then **Run anyway**.

    `DictShelf.exe` is a Windows app and opens without a console window. The same
    executable handles terminal commands; run `DictShelf.exe --help` from
    PowerShell or `cmd`. Desktop launches show a tray icon and log to
    `%USERPROFILE%\.dmshlabs\dictshelf\db.logs\dictshelf.log`.

    The installer `dictshelf-windows-x64-setup-<version>.exe` adds a Start menu
    entry, an uninstaller, `PATH` and *Open with*. See
    [Windows installer](../apps/windows.md).

=== "Android"

    Build **`dictshelf-android-arm64-v8a-foss.apk`** with
    `build-android.cmd release` on Windows, then install the signed APK with
    your file manager; Android asks once to allow *Install unknown apps*. The
    Google Play build is on Google Play. See [Android app](../apps/android.md).

## Check

``` sh
dictshelf --version
dictshelf
```

`dictshelf` prints the address it listens on and opens the browser at
[localhost:6888](http://localhost:6888). `dictshelf --help` lists every command
and setting.

## One build { #one-build }

There is one kind of build, and nothing to choose between: SQLite through
`mattn/go-sqlite3`, compiled with cgo, with the Speex decoder beside it. A
build without a C compiler fails on purpose, by name
(`internal/store/driver_missing.go`), instead of quietly linking a different
driver with different index sizes. See [Building](../reference/building.md).

Speex `.spx` audio plays through the decoder compiled into the binary.
Setting `SPEEX_BACKEND = "external"` makes it use the `speexdec` program
instead (`brew install speex`, `apt install speex`).

## Update

Replace the executable with the new one; on Windows, stop the server first.
The config file, the library and the dictionary files are kept, and the new
version reads them. Indexes built by an older version are reported as
[outdated](../dictionaries/library.md#outdated-indexes).

The installer, `DictShelf.app` and the APK are updated by installing the new
version over the old one.

## Uninstall

Delete the executable. `~/.dmshlabs/dictshelf` holds the settings and the library; delete
it as well to remove everything.

## Next

[Set the dictionary folders](first-run.md){ .md-button .md-button--primary }
