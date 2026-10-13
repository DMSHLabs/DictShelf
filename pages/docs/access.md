---
title: Access from other devices
description: Use a wudict server from other devices on your network - listen address, access key, host names, deletion and import limits.
---

# Access from other devices

By default wudict listens on `127.0.0.1` and only the machine it runs on can
reach it.

## Listen on the network

``` toml title="~/.wudict/wudict.toml"
SERVER_IP = "0.0.0.0"
```

or `wudict --ip 0.0.0.0`. Restart wudict.

## Access key

On a network address, every request needs the access key
([`AUTH`](reference/configuration.md#auth) `auto`). Exceptions: the three
read-only endpoints for browser extensions (`/api/dicts`, `/api/search`,
`/res/`).

1.  On the server machine, run `wudict token`. It prints a link such as
    `http://192.168.1.20:6888/?k=<key>`.
2.  Open the link once in the browser on the other device. The browser keeps
    the key as a cookie.

A program sends `Authorization: Bearer <key>`. `wudict token -rotate` replaces
the key; every device then needs the new link. The key is stored in
`~/.wudict/token`; [`AUTH_TOKEN`](reference/configuration.md#auth_token)
supplies it from elsewhere.

!!! warning "AUTH = off"

    With `AUTH = "off"` on a network address, anyone who can reach the port can
    read your library, your settings and your folder names, and import
    dictionaries.

## Host names

Requests to an IP address and to `localhost` are accepted. To reach the server
through a host name, e.g. behind a reverse proxy, list the name in
[`TRUSTED_HOSTS`](reference/configuration.md#trusted_hosts):

``` toml
TRUSTED_HOSTS = ["wudict.lan"]
```

## Deleting and importing

-   Deleting dictionaries from another device needs
    [`ALLOW_REMOTE_DELETE = "1"`](reference/configuration.md#allow_remote_delete).
-   [`IMPORT_URL_HOSTS`](reference/configuration.md#import_url_hosts) limits the
    hosts the server downloads dictionaries from at a client's request.

## Android

The FOSS build has *Settings → Reachable from other devices (testing only)*. It applies while the app is open on screen and on the network
it started on, and does not require the access key.

## Browser extension on another device

[wudict Hover](extension.md) uses only the three read-only endpoints and needs
no access key: set the server's address in its options.
