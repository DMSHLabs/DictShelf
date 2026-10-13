---
title: First run
description: Start wudict, set the dictionary folders, and check that the dictionaries are found.
---

# First run

Requires `wudict` installed ([Install](install.md)) and a folder of
dictionary files.

## 1. Start the server

``` sh
wudict
```

wudict prints the address it listens on, the dictionary folders it reads and
the config file in effect, then opens the browser at
[localhost:6888](http://localhost:6888). A binary in the current folder is
started with `./wudict`.

The **wudict howto**, a guide built in as a dictionary, is always available:
search for `wudict` to list its topics.

## 2. Set the dictionary folders

The default dictionary folder is `~/Dictionaries`, subfolders included. If it
is missing or empty, the main page shows the howto with a link to the setup
page, [localhost:6888/setup](http://localhost:6888/setup).

On the setup page, enter the path of a folder with dictionary files (`~` is
allowed). The page checks the path as you type and shows how many dictionaries
it contains. <kbd>Use this folder</kbd> saves it to `~/.wudict/wudict.toml`;
no restart is needed.

To open the setup page later: <kbd>☰</kbd> → folder summary →
<kbd>Edit folders…</kbd>.

The folders can also be set without the setup page:

=== "Config file"

    ``` toml title="~/.wudict/wudict.toml"
    DICT_DIR = ["~/Dictionaries", "/Volumes/Data/Dicts"]
    ```

=== "Command line"

    ``` sh title="repeat the flag for more folders"
    wudict --dict-dir ~/Dictionaries --dict-dir /Volumes/Data/Dicts
    ```

=== "Environment variable"

    ``` sh title="separate folders with : (; on Windows)"
    DICT_DIR="~/Dictionaries:/Volumes/Data/Dicts" wudict
    ```

A command-line flag overrides the environment variable, which overrides
`wudict.toml`. The setup page shows when a folder list is overridden.

[All settings](../reference/configuration.md){ .md-button }

## 3. Check

The main page lists one section per dictionary that has a result. Search for a
word you know is in one of them.

From a terminal:

``` sh title="the dictionaries wudict finds in a folder"
wudict list ~/Dictionaries
```

Each line is one dictionary wudict can read.

## Indexing

Exact and prefix search work at once, through each dictionary's own format.
On the first search, wudict indexes each dictionary's headwords in the
background, one dictionary at a time, into the library (`~/.wudict/db`).
Indexed dictionaries are searched faster and use less memory. Search works
during indexing.

[The library](../dictionaries/library.md){ .md-button }

## Problems

| Symptom | See |
| --- | --- |
| No dictionaries listed | [No dictionaries appear](../help/troubleshooting.md#no-dictionaries) |
| `address already in use` | [Port 6888 is in use](../help/troubleshooting.md#port-taken) |
| No browser tab opens | [No browser tab opens](../help/troubleshooting.md#no-browser) |
| No sound | [No sound](../help/troubleshooting.md#audio) |

## Next

[Search](search.md){ .md-button .md-button--primary }
