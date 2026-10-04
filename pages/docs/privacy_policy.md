---
title: Privacy Policy
description: wuDict collects no data about you and has no account. It connects to the internet only to download what you ask for.
---

# Privacy Policy

**Effective 3 October 2026.**

This policy covers the **wuDict** Android app (`com.legbehindneck.wudict`) and
the `wudict` program it is built from, on every platform.

## Summary

**wuDict collects no data about you.**

There is no account, no sign-in, no cloud service, no analytics, no advertising,
no crash reporting and no third-party SDK. Your dictionaries, your searches and
your settings stay on your device.

wuDict runs a web server inside the app, bound to `127.0.0.1`, and shows its
pages in a WebView. The app connects to the internet only to download something
you asked for: lemma data, or a dictionary from a link you provide. See
[Network requests](#network-requests).

## What the app stores, and where

| What | Where |
| --- | --- |
| Dictionary files | FOSS build: *Internal storage → Dictionaries*, a folder you manage. Google Play build: the app's own external files directory |
| The search indexes built from them | The app's own external files directory |
| Downloaded lemma data | The same place |
| Your settings | The same place |

Nothing is uploaded.

The app declares `allowBackup="false"`, so none of it is copied into Google's
Auto Backup. **Uninstalling the app deletes everything in its own directory.**
You can also clear it at any time from Android's *Settings → Apps → wuDict →
Storage → Clear storage*. In the FOSS build, *Internal storage → Dictionaries*
is yours and is not deleted.

## Permissions

Both builds declare:

- **`INTERNET`**: to connect to the wuDict server inside the app on
  `127.0.0.1`, and for the downloads listed under
  [Network requests](#network-requests).
- **`FOREGROUND_SERVICE`** and **`FOREGROUND_SERVICE_DATA_SYNC`**: granted at
  install time, with no prompt. They keep indexing or a download running
  while you use another app.
- **`POST_NOTIFICATIONS`**: the only permission you are asked for, and only the
  first time a dictionary is indexed. It shows the progress notification for
  that work. If you decline, everything still works without the notification.

The FOSS build (GitHub, Obtainium) also declares:

- **`MANAGE_EXTERNAL_STORAGE`** (*All files access*): to read dictionaries from
  *Internal storage → Dictionaries*. `READ_EXTERNAL_STORAGE` (Android 12 and
  older) and `WRITE_EXTERNAL_STORAGE` (Android 10 and older) are the older
  forms of the same access.

The Google Play build declares no storage permission. Dictionaries are imported
through Android's system file picker (the Storage Access Framework), which
grants access to the one file you choose. The app cannot read, list or scan
your storage.

Neither build requests location, contacts, camera, microphone, phone state or
device identifiers. The app does not read the clipboard and does not use the
Advertising ID.

## Network requests { #network-requests }

wuDict makes a network request only in these cases:

- **Lemma data.** The Lemmatization page reads the language catalogue from
  `raw.githubusercontent.com/wuweidict/lemmas`, and installing a language
  downloads its file from there.
- **Dictionaries from a link.** A link you paste, drop or share into the app,
  pointing to a dictionary, an archive, a web folder or a Nextcloud share, is
  downloaded from the server it names.
- **Dictionary content.** Dictionaries are authored by third parties. If an
  article references an image, font or script by an `http://` or `https://`
  address instead of bundling it, the WebView requests that address while the
  article is displayed. This is uncommon: most dictionary files bundle their
  media.

A tapped external link opens in your browser; wuDict does not follow it.

The server at the other end of each request sees your IP address, as with any
web request. wuDict's own requests identify themselves as `wudict` and carry
nothing else about you or your device.

With the network turned off, everything except these downloads works.

## Data collected by Google Play

Distributing through Google Play means Google collects information about
installs, and receives crash and ANR reports from Android itself, under
[Google's own privacy policy](https://policies.google.com/privacy). We can see
that only as aggregate statistics in the Play Console: install counts, device
and country breakdowns, stack traces. It is not collected by the app, we cannot
connect it to you, and it does not include anything you looked up.

None of this applies to the FOSS build from GitHub.

## Children

wuDict is a dictionary reader. It collects no data from anyone, of any age,
and contains no advertising and no in-app purchases.

## Your rights

Regulations such as the GDPR and the CCPA give you rights to access, correct,
export and delete the personal data a service holds about you. **We hold none.**
There is no account to close, no profile to export and no record to delete.
Everything the app produces is on your device.

## Open source

wuDict is free software under the GPL-3.0-or-later. The source is public, so
every statement on this page can be checked.

- Source: [github.com/wuweidict/wudict](https://github.com/wuweidict/wudict)

## Changes to this policy

If this policy changes, the effective date at the top changes with it, and the
previous text stays in the repository's history. A change to what the app
collects would arrive with an app update.

## Contact

Questions about this policy:

- [Open an issue](https://github.com/wuweidict/wudict/issues)
