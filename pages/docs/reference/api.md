---
title: HTTP API
description: The wudict HTTP API - who may call it, streaming search results, the dictionary list, resources, errors, and the OpenAPI document.
---

# HTTP API

The web page, the browser extension and the Android app use this API; so can
your own programs. The server answers on `http://127.0.0.1:6888` by default.

[API reference (OpenAPI)](../api/index.html){ .md-button }

## Access

| Client | Can call |
| --- | --- |
| a program on the same machine (curl, Python, Node, an application) | every endpoint |
| the wudict page | every endpoint |
| a browser extension | `/api/dicts`, `/api/search`, `/res/`; [`BROWSER_EXTENSIONS`](configuration.md#browser_extensions) can restrict which extensions |
| a web page | the same three endpoints, if [`WEB_ORIGINS`](configuration.md#web_origins) lists its origin |
| a client on another host | every endpoint with the access key; the three endpoints without it |

When the server listens on the network, every endpoint except the three needs
the access key ([`AUTH`](configuration.md#auth)): a program sends
`Authorization: Bearer <key>`; a browser receives it as a cookie from the link
`wudict token` prints. Deleting dictionaries from another host also needs
[`ALLOW_REMOTE_DELETE`](configuration.md#allow_remote_delete).

CORS (`WEB_ORIGINS`, `BROWSER_EXTENSIONS`) applies only to pages and
extensions in a browser; other programs send no `Origin` header and are not
affected by it.

## Calling the API from a web page

List the page's origin and restart wudict:

``` toml title="~/.wudict/wudict.toml"
WEB_ORIGINS = ["http://localhost:3000"]
```

-   The origin must match exactly: scheme, host and port.
-   Send no credentials and no custom headers. The server never sends
    `Access-Control-Allow-Credentials`, so `credentials: 'include'` fails. A
    plain `GET` needs no preflight.
-   A `file://` page sends `Origin: null`, which is never allowed. Serve the
    page over `http://`, also during development.
-   Chrome preflights requests from a non-local page to `127.0.0.1` (local
    network access). wudict answers the preflight for listed origins; a Chrome
    policy or another extension can still block the request.

A refused request appears in the browser console as a CORS error, not as an
HTTP status.

## OpenAPI document

Every endpoint, parameter, field and status code is defined in one OpenAPI 3.1
document. A test checks it against the server's routes in both directions.

| Where | |
| --- | --- |
| `http://127.0.0.1:6888/api/openapi.yaml` | served by the running server |
| [`internal/server/web/openapi.yaml`](https://github.com/wuweidict/wudict/blob/master/internal/server/web/openapi.yaml) | in the repository |
| [API reference](../api/index.html) | rendered, with a `curl` example per endpoint |
| `make api-ui` | rendered to `dist/api-explorer.html`, for offline use |

Endpoints tagged `internal` serve the wudict page and the Android app and may
change between releases.

## Streaming

`/api/dicts`, `/api/search` and `/api/rescan` answer in **NDJSON**: one JSON
object per line, sent as soon as it is ready. Every stream starts with a
`begin` line and ends with an `end` line. Read it line by line: the first
dictionary's results arrive while others are still searching.

`/api/ingest` sends Server-Sent Events: progress, not results.

## Search

``` text
GET /api/search?q=flight&mode=prefix&format=clean&n=5
```

``` json title="lines in arrival order"
{"t":"begin","i":0,"slots":[{"dict":"oxford","name":"oxford"},{"dict":"webster","name":"webster"}]}
{"t":"hit","i":1,"dict":"webster","name":"Webster's Revised Unabridged","results":[{"Headword":"flight","Body":"<div>…</div>"}]}
{"t":"hit","i":0,"dict":"oxford","name":"Oxford Advanced Learner's","results":[]}
{"t":"end","i":0}
```

`begin` lists the dictionaries that will answer, in the user's order. Each
`hit` carries `i`, its position in `slots`; hits arrive in completion order.

A `hit` may carry one of these instead of results:

| Field | Meaning |
| --- | --- |
| `skipped` | the dictionary does not support the mode; `caps` in `/api/dicts` lists the supported modes |
| `deferred` | not opened: the search reached [`SEARCH_MEMORY`](configuration.md#search_memory). A search of that dictionary alone answers it and moves it to the front of the indexing queue |
| `indexing` | with `deferred`: the dictionary is being indexed |
| `error` | this dictionary failed; the others still answer |

A search ends after 30 seconds. `mode=fuzzy` is read as `prefix`.

### Article formats

| `format` | Content | Size |
| --- | --- | --- |
| `raw` | the dictionary's HTML | 1 |
| `clean` | structure, emphasis and media; no scripts, stylesheets or presentation | about 0.5 |
| `text` | text only | about 0.4 |

`clean` also rewrites root-relative links such as `/res/…` to absolute URLs,
so the article works in a page served from elsewhere. Use it unless you render
articles with the dictionaries' own stylesheets.

## Dictionary list

``` json title="GET /api/dicts"
{"t":"begin","total":2}
{"t":"dict","dict":{"id":"oxford","name":"Oxford Advanced Learner's","format":"mdx","path":"/Users/me/Dictionaries/oald.mdx","entries":184000,"caps":{"Exact":true,"Prefix":true,"Contains":false,"FTS":false}}}
{"t":"end"}
```

`total` is sent first, before any dictionary is opened. `id` is the value for
`dict` in `/api/search`. `caps` lists the supported modes: `Contains` and `FTS`
are true once the dictionary has the contains or full-text index. The other
fields (files, library folder, sizes) are listed in the OpenAPI document.

``` sh
curl -s http://127.0.0.1:6888/api/dicts | jq
```

## Resources

``` text
GET /res/oxford/audio/flight__gb.mp3
```

The path after the dictionary id is the path the article requests. `404`: the
dictionary has no such file. Speex audio is converted to WAV and served as
`audio/wav`; a `.spx` that cannot be converted, or one in a
[resource override](../dictionaries/override.md), is served as `audio/ogg`.
Resource overrides are served with `Cache-Control: no-cache`.

## Errors

A failed request answers `{"error":"…"}` with an HTTP status. A failure in one
dictionary during a search is reported in that dictionary's `hit`, and the
request succeeds.

## Example

Search, then print each headword and article as it arrives.

=== "Shell"

    ``` sh title="curl and jq"
    curl -sN 'http://127.0.0.1:6888/api/search?q=phubbing&mode=exact&format=text&n=3' |
      jq -r --unbuffered 'select(.t == "hit") | .name as $d | (.results // [])[]
             | "\($d) | \(.Headword)", ("=" * 70), (.Body // "")'

    # one dictionary, by its id
    curl -sN 'http://127.0.0.1:6888/api/search?q=flight&mode=exact&format=text&n=3&dict=4112242f8cf1' |
          jq -r --unbuffered 'select(.t == "hit") | .name as $d | (.results // [])[]
                 | "\($d) | \(.Headword)", ("=" * 70), (.Body // "")'
    ```

    `curl -N` and `jq --unbuffered` print each dictionary's results as they
    arrive. `(.results // [])` skips dictionaries without results.

=== "Python"

    ``` py title="Python standard library"
    import json, urllib.request

    url = "http://127.0.0.1:6888/api/search?q=flight&mode=exact&format=text&n=3"

    with urllib.request.urlopen(url) as f:
        for line in f:
            m = json.loads(line)
            if m.get("t") == "hit":
                for r in m.get("results") or []:
                    print(m["name"], "|", r["Headword"])
                    print("=" * 70)
                    print(r.get("Body", ""))
    ```
    `&dict=<id>` limits the search to one dictionary.

=== "JavaScript"

    ``` js title="from a page in a browser; needs WEB_ORIGINS"
    const url = "http://127.0.0.1:6888/api/search?q=flight&mode=exact&format=text&n=3";
    const res = await fetch(url);           // no credentials, no custom headers
    const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();

    let buf = "";
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += value;
      const lines = buf.split("\n");
      buf = lines.pop();
      for (const line of lines) {
        if (!line) continue;
        const m = JSON.parse(line);
        if (m.t !== "hit") continue;
        for (const r of m.results || []) {
          console.log(m.name, "|", r.Headword);
          console.log("=".repeat(70));
          console.log(r.Body || "");
        }
      }
    }
    ```
