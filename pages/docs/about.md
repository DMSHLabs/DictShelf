---
title: About
description: DictShelf - what it is, its HTTP API, resource overrides, the name and the licence.
---

# About DictShelf

DictShelf (`dictshelf`) is a dictionary program that runs as a local web server
and is used in the browser. It searches all your dictionaries at once, in
several dictionary formats, and keeps an indexed copy of each in its library.

## HTTP API

The web page, the browser extension and the Android app all use the same HTTP
API, and so can your own programs.

[HTTP API](reference/api.md){ .md-button }

## Resource overrides

A dictionary with a broken stylesheet, script or image can be repaired without
changing the dictionary file: a file in the dictionary's library folder
replaces the dictionary's own.

[Resource overrides](dictionaries/override.md){ .md-button }

## The name

DictShelf is a shelf of dictionaries: one program, one library, every dictionary
you own in one search box. The command is `dictshelf`.

It is a fork of **wudict** (*WuWeiDict*, from *wú wéi* 無為, the classical
Chinese term for effortless action): DictShelf keeps that program's dictionary
formats, its library layout and its HTTP API, and is maintained under its own
name by DmShAl.

## Licence

DictShelf is free software under the GNU General Public License, version 3 or
later.

[Licence and attribution](help/license.md){ .md-button }
