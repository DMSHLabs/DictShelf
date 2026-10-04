---
title: Add dictionaries
description: Add dictionaries by copying files into a dictionary folder, or import a file, an archive, a web folder, a list or a share link.
---

# Add dictionaries

## Copy into a dictionary folder

Copy every file of the dictionary into one of the dictionary folders
([`DICT_DIR`](../reference/configuration.md#dict_dir)), e.g. `Oxford.mdx` and
`Oxford.mdd`; [Formats](formats.md) lists the files of each format. Subfolders
are read. Then <kbd>☰</kbd> → folder summary → <kbd>Rescan folders</kbd>.

## Import a file or a link

On the setup page (<kbd>☰</kbd> → folder summary → <kbd>Edit folders…</kbd>),
under **Add dictionaries**:

-   <kbd>Choose a file…</kbd>, or drop a file: a dictionary file, or a `.zip`
    or `.7z` archive.
-   Paste or drop a link, then <kbd>Download</kbd>. A link may point to:
    -   a dictionary file or an archive;
    -   a web folder page that lists dictionary files;
    -   a text file listing links or file names, one per line;
    -   a Nextcloud public share folder (Nextcloud 29 or later);
    -   a [share link](#share-links).

    Several links can be pasted at once.

wudict lists the dictionaries found: the files of one dictionary (`.mdx` and
`.mdd`, `.ifo` and its companions) form one entry, and an entry shows whether
it is already installed or needs a file that is missing. Tick the ones you want
and click <kbd>Install</kbd>. They are installed into the first dictionary
folder.

After an import from an archive, the archive is kept or deleted according to
[`IMPORT_KEEP`](../reference/configuration.md#import_keep); with the default,
`ask`, the **Keep afterwards** checkbox decides. A failed import keeps the
archive.

Links must be `https://` and point to public hosts, unless
[`IMPORT_INSECURE`](../reference/configuration.md#import_insecure) is set.
[`IMPORT_URL_HOSTS`](../reference/configuration.md#import_url_hosts) restricts
the hosts. A folder page or list may name up to 500 files.

## Share links

A share link carries a link to a dictionary, an archive or a collection after
`#`:

``` text
https://legbehindneck.com/wudict#https://example.org/dicts/oxford.zip
```

On Android with wuDict installed, opening a share link starts the import in the
app. Elsewhere, paste it on the setup page.

## Android

See [Android app → Add dictionaries](../apps/android.md#add-dictionaries).

## The wudict howto as a file

**Put the wudict howto in this folder**, on the setup page, writes the howto
guide as a [wudict markdown](formats.md#wudict-markdown) file into the
dictionary folder. Edit it in a text editor, then <kbd>Rescan folders</kbd>.
