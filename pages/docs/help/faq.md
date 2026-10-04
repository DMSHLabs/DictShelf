---
title: FAQ
description: Privacy, files on disk, moving the library, updating, disk space, the terminal, Android, network access, lemmatization.
---

# FAQ

For problems, see [Troubleshooting](troubleshooting.md).

??? question "Is any of my data sent anywhere?"

    No. wudict listens on the loopback address `127.0.0.1` and has no account,
    telemetry, analytics or crash reporting. It connects to the internet only to
    download what you ask for: lemma data, or a dictionary from a link you
    provide. See the [Privacy Policy](../privacy_policy.md).

??? question "Why two names, WuWeiDict and wudict?"

    *Wú wéi* (無為) is the classical Chinese term for effortless action, or
    non-action. `wudict` is the short form, used for the program.

??? question "Where are wudict's files?"

    ``` text title="macOS and Linux"
    ~/.wudict/
      wudict.toml     settings
      state.json      dictionary order, enabled dictionaries, display preferences
      groups.ini      picker groups, once edited
      style/          custom styles
      lemmas/         installed lemma data
      db/             the library; db/spxcache/ holds converted Speex audio
    ~/Dictionaries    the default dictionary folder
    ```

    Windows and Android: [Configuration → Files](../reference/configuration.md#files).

??? question "Can I move a dictionary to another machine?"

    Yes. Copy its library folder, e.g. `~/.wudict/db/Oxford`, into the other
    machine's library or one of its dictionary folders. The dictionary files are
    not needed; images and audio come from `media.db` if the media pack was
    added. See [The library](../dictionaries/library.md#move-a-dictionary-to-another-machine).

??? question "How do I update wudict?"

    Replace the executable, or run the new installer, `wuDict.app` or APK over
    the old one. Settings and the library are kept. Indexes built by an older
    version are reported as [outdated](../dictionaries/library.md#outdated-indexes)
    and rebuilt with <kbd>Rebuild</kbd> or `wudict reindex`.

??? question "How do I free the disk space an index uses?"

    Click the index's switch in the dictionary panel; the switch shows its size.
    The switches are locked when the dictionary files are gone, because the
    library folder is then the only copy; <kbd>🗑 Remove…</kbd> still deletes
    it. <kbd>Rescan folders</kbd> lists [orphans](../dictionaries/library.md#orphans)
    and deletes the ones you tick.

??? question "Can I use wudict without a browser?"

    Yes. The commands work without a running server:

    ``` sh
    wudict lookup ~/Dictionaries/Oxford.mdx flight   # the article as HTML
    wudict keys   ~/Dictionaries/Oxford.mdx          # every headword
    wudict ingest                                    # index the dictionary folders
    wudict clean                                     # list removable library folders
    ```

    [Command line](../reference/cli.md)

??? question "Is there a phone app?"

    On Android: the app runs the same server and works offline, and looks up a
    word from the text selection menu of other apps. There is no iOS app.

    [Android app](../apps/android.md)

??? question "Can other computers on my network use it?"

    Set [`SERVER_IP`](../reference/configuration.md#server_ip-and-server_port)
    to `0.0.0.0`. Requests from the network then need the access key
    ([`AUTH`](../reference/configuration.md#auth)): `wudict token` prints a link
    that carries it; open it once in the browser on the other device. Deleting
    dictionaries from another device also needs
    [`ALLOW_REMOTE_DELETE`](../reference/configuration.md#allow_remote_delete).

??? question "Does wudict change my dictionary files?"

    No. Indexes go into the library. A replacement for a broken resource goes
    into the dictionary's library folder as a
    [resource override](../dictionaries/override.md). Only
    <kbd>🗑 Remove…</kbd> and `wudict rm` delete dictionary files.

??? question "Why does *estuviera* find nothing?"

    Spanish lemma data is not installed, or wudict cannot tell that the
    dictionary is Spanish.

    Install it: <kbd>☰</kbd> → folder summary → <kbd>Lemmatization…</kbd> →
    tick Spanish. It applies from the next search. In a terminal:
    `wudict lemmas download es`.

    Most `.mdx` and `.slob` dictionaries do not declare their language. Name the
    file with a language code first (`es-en-collins.mdx`), or put it in a folder
    named by the code (`es/`). English is assumed when nothing else is found.
    See [language detection](../reference/configuration.md#lemmatization).

??? question "-cgo or -purego?"

    `-cgo` where it exists: faster SQLite, and a built-in Speex decoder.
    `-purego` elsewhere; it needs `speexdec` for Speex audio.
    See [-cgo or -purego](../start/install.md#-cgo-or-purego).

??? question "How do I stop the browser tab opening at startup?"

    `wudict --no-browser`, or `NO_BROWSER = "1"` in `~/.wudict/wudict.toml`.

??? question "How do I build this documentation site?"

    With [Zensical](https://zensical.org/), which needs Python 3:

    ``` sh
    cd pages
    pip install zensical
    zensical serve      # localhost:8000, rebuilt on change
    zensical build      # static HTML in pages/site
    ```
