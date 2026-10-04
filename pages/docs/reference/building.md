---
title: Build from source
description: Build wudict yourself with Go, with or without a C compiler, including cross-compiled releases and the Android app.
---

# Build from source

Requires [Go](https://go.dev/doc/install). The `-cgo` build also needs a C
compiler: Xcode Command Line
Tools on macOS, `build-essential` on Debian and Ubuntu, MSYS2 or MinGW on
Windows.

## go install

=== "With a C compiler"

    ``` sh title="cgo SQLite plus the built-in Speex decoder"
    go install -tags sqlite_fts5 github.com/wuweidict/wudict@latest
    ```

=== "Without a C compiler"

    ``` sh title="pure-Go SQLite"
    go install github.com/wuweidict/wudict@latest
    ```

The tag selects the SQLite driver and the Speex decoder. Without a C compiler,
`-tags sqlite_fts5` builds the pure-Go variant instead of failing.

## From a clone

Every action has a `make` target.

``` sh
make build     # native build, cgo flavour, into ./wudict
make install   # into GOBIN
make check     # tidy, vet and all tests
make help      # every target, with a description
```

Run `make check` before submitting a change.

## The API document

The HTTP API is defined in `internal/server/web/openapi.yaml`, embedded in the
binary and served at `/api/openapi.yaml`.

``` sh
make api-test   # assert it matches the server's routes (also part of make test)
make api-lint   # validate it (needs Node, through npx)
make api-open   # render it to one offline HTML file in dist/ and open it
```

`make api-test` checks the document against the route table in both
directions. `api-lint` and `api-ui` need the network on first run; `make check`
does not.

## The documentation site

This site is built with [Zensical](https://zensical.org/) from `pages/`, and
published to GitHub Pages by `.github/workflows/docs.yml` on each push to
`master` that changes it.

``` sh
pip install -r pages/requirements.txt
make docs         # build into pages/site; warnings fail the build
make docs-serve   # preview at localhost:8000
make docs-clean   # remove the output, the cache and the copied explorer
```

The Zensical version is pinned in `pages/requirements.txt`.

`make docs` first renders the [API reference](../api/index.html) into
`pages/docs/api/`. The build runs with `--strict`: a broken link or anchor
fails it.

## Release builds

``` sh
make cross
```

Builds the `-purego` binaries for macOS, Linux and Windows into `dist/`.

``` sh
make mac-app          # wuDict.app into dist/
make mac-app-install  # and into ~/Applications
make win-installer    # the Windows installer (needs Inno Setup 6.3 or newer)
```

## Android

``` sh
make android-go     # build the server for the app (needs the Android NDK)
make apk-foss-debug # build the FOSS debug APK (needs the Android SDK)
```

Without the NDK, `make android-go-purego` builds the pure-Go variant, which
has no Speex audio. The app is a WebView shell around the same server binary.

### Builds

The two APKs differ only in how dictionaries reach the device.

| Build | Storage | Make targets |
| --- | --- | --- |
| `foss` | *All files access*; reads *Internal storage → Dictionaries* | `make apk-foss-debug`, `make apk-foss-release` |
| `play` | no storage permission; dictionaries are imported into the app's folder | `make apk-play-debug`, `make apk-play-release` |

The GitHub release `wudict2-android-arm64-v8a-foss.apk` is the `foss`
build. `make apk-verify` checks that the `play` build declares no storage
permission. See [Android app](../apps/android.md).

## Check

``` sh
./wudict --version
```
