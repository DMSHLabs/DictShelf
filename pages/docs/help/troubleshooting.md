---
title: Troubleshooting
description: Symptoms, causes and fixes - missing dictionaries, port in use, no sound, broken articles, slow first searches, disk use, Android.
---

# Troubleshooting

## No dictionaries appear { #no-dictionaries }

| Cause | Fix |
| --- | --- |
| The files are not in a dictionary folder | Compare the folders printed at startup, or shown in the folder summary, with where the files are |
| A file is missing | A StarDict `.dict.dz` needs its `.ifo` and `.idx`; see [Formats](../dictionaries/formats.md) |
| The files were added while wudict was running | <kbd>☰</kbd> → folder summary → <kbd>Rescan folders</kbd> |
| Wrong separator in `DICT_DIR` (environment) | `:` on macOS and Linux, `;` on Windows |

``` sh title="the dictionaries wudict finds in a folder"
wudict list ~/Dictionaries
```

## wudict does not start: DICT_DIR and DB_DIR are the same folder

The library ([`DB_DIR`](../reference/configuration.md#db_dir)) must be a
different folder from every dictionary folder. Usually only `DICT_DIR` needs
setting; the library defaults to `~/.wudict/db`.

## Port 6888 is in use { #port-taken }

Another program uses the port; startup reports it. Choose another port:

``` sh title="for one run"
wudict --port 9090
```

``` toml title="~/.wudict/wudict.toml"
SERVER_PORT = "9090"
```

Then open `localhost:9090`.

## No browser tab opens { #no-browser }

Open [localhost:6888](http://localhost:6888). A tab is not opened when
[`NO_BROWSER`](../reference/configuration.md#no_browser) is set.

Started without a console, wudict shows an icon instead of output: on Windows
in the notification area (log: `%LOCALAPPDATA%\wudict\wudict.log`), on macOS in
the menu bar (log: `~/Library/Logs/wudict.log`).

## No sound { #audio }

`.mp3`, `.ogg` and `.wav` play in the browser. Speex (`.spx`) is converted to
WAV first: by the built-in decoder in `-cgo` builds, by the external
`speexdec` program in `-purego` builds.

``` sh title="install speexdec"
brew install speex     # macOS
apt install speex      # Debian, Ubuntu
```

[`SPEEXDEC`](../reference/configuration.md#speexdec) (`--speexdec`) sets its
path. Startup prints the decoder in use, or how to install one.

A `.spx` file in a `res/` folder is not converted: use `.mp3` or `.wav` there.

## An article is broken, or its buttons do nothing

A dictionary's stylesheet or script is damaged or missing. wudict logs a
warning when it serves a text resource that contains NUL bytes, with the
`res/` path that would override it. A ⚠ in the dictionary's result header
marks a script error.

[Resource overrides](../dictionaries/override.md){ .md-button }

## An image or sound is missing

In the browser's network panel, find the failing request to
`/res/<dictionary id>/<path>`. A `404` means the dictionary lacks the file;
`<path>` is where a [resource override](../dictionaries/override.md) goes.

## The first searches are slow, and the CPU is busy

wudict is indexing the dictionaries, once each, one at a time, in the
background. Indexing one dictionary uses one CPU core.
[`INDEX_WORKERS`](../reference/configuration.md#index_workers) indexes several
at once; [`AUTO_INDEX = "off"`](../reference/configuration.md#auto_index)
turns indexing off.

## Some dictionaries are reported as not searched

The search reached [`SEARCH_MEMORY`](../reference/configuration.md#search_memory)
and did not open the remaining dictionaries that are not indexed. Choose one of
them in the picker to search it alone. The limit is off by default on the
desktop and on by default on Android.

## Contains search misses a headword

- Contains matches the query as a literal substring, ignoring case and
  accents; it does not correct spelling.
- The contains index is outdated: its switch in the dictionary panel says so.
  Click it to rebuild the index.

## The library uses too much disk space

The dictionary panel shows each index's size; click a switch to remove that
index. <kbd>Rescan folders</kbd> lists [orphans](../dictionaries/library.md#orphans),
library folders whose dictionary files are gone, and deletes the ones you
tick.

``` sh title="from a terminal"
wudict clean              # list broken library folders and orphans
wudict clean -f           # delete the broken ones
wudict clean -f -orphans  # and the orphans
```

## The Android app shows no dictionaries { #android-no-dictionaries }

FOSS build:

1. **No storage access.** *Settings → Apps → wuDict → Permissions*, or
   Android's *All files access* list: allow it, then reopen the app.
2. **Wrong folder.** The folder is *Internal storage → Dictionaries*, at the
   top level, not inside *Documents* or *Download*.
3. **Files added while the app was open.** <kbd>☰</kbd> → folder summary →
   <kbd>Rescan folders</kbd>.

Google Play build: dictionaries must be imported; see
[Add dictionaries](../apps/android.md#add-dictionaries).

## The Android app while it is not on screen

When the app is not visible, the server starts no new indexing and closes the
dictionaries it can reopen. Indexed dictionaries stay open. Indexing,
downloads and rebuilds already running continue in a foreground service, with
a progress notification. See [Android app](../apps/android.md#battery-and-memory).

## A change to wudict.toml has no effect

A command-line flag or an environment variable sets the same key and takes
precedence. The setup page and `/api/config` show where each value comes from.

[Priority](../reference/configuration.md#priority){ .md-button }

## Anything else

Start wudict with `--verbose`: it logs every request, dictionary open, indexing
step and audio conversion.

``` sh
wudict --verbose
```

[Open an issue](https://github.com/wuweidict/wudict/issues) with that output
and the output of `wudict --version`.
