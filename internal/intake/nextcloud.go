// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package intake

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// Nextcloud public shares: https://host/index.php/s/<token>?dir=/some/folder
//
// The share page is an application shell drawn by JavaScript - its HTML holds
// no link to any file, so the folder-page reader finds nothing in it. The same
// share is published over WebDAV without credentials (Nextcloud 29 and later):
//
//	PROPFIND https://host/public.php/dav/files/<token>/some/folder/  (Depth: 1)
//
// answers with every entry of that folder, and each file's href there is a
// plain URL that answers HEAD, GET and Range like any other download - so the
// listing is the only part that needs to know about Nextcloud. Subfolders are
// not descended into, as a folder page's subfolder links are not followed.
//
// Recognised by the URL's shape alone, and only after the link was refused as
// not a dictionary. A site that merely has "/s/<word>" in a path costs one
// PROPFIND that fails, and then reads as the page it is.

// shareRE matches a share path: an optional install prefix ("/nextcloud"),
// "/index.php" or not, then "/s/<token>".
var shareRE = regexp.MustCompile(`^(/[^?#]*?)?/(?:index\.php/)?s/([A-Za-z0-9_-]{1,64})/?$`)

// shareDAV returns the WebDAV address of the folder a Nextcloud share link
// shows, and that folder's name; ok is false when u is not shaped like one.
func shareDAV(u *url.URL) (dav *url.URL, title string, ok bool) {
	m := shareRE.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, "", false
	}
	// The optional group is tried first, so "/index.php" can land in it
	// rather than in the alternative meant for it.
	prefix, token := strings.TrimSuffix(m[1], "/index.php"), m[2]
	q := u.Query()
	dir := q.Get("dir")
	if dir == "" {
		dir = q.Get("path") // the parameter's older name
	}
	dir = path.Clean("/" + dir) // ".." cannot climb above the share's root
	p := prefix + "/public.php/dav/files/" + token + strings.TrimSuffix(dir, "/") + "/"
	dav = &url.URL{Scheme: u.Scheme, Host: u.Host, Path: p} // String escapes the path
	if dir != "/" {
		title = path.Base(dir)
	}
	return dav, title, true
}

// davMultistatus is the part of a PROPFIND answer that is read: each entry's
// href, and whether it is a folder.
type davMultistatus struct {
	Responses []struct {
		Href     string `xml:"DAV: href"`
		Propstat []struct {
			Prop struct {
				ResourceType struct {
					Collection *struct{} `xml:"DAV: collection"`
				} `xml:"DAV: resourcetype"`
			} `xml:"DAV: prop"`
		} `xml:"DAV: propstat"`
	} `xml:"DAV: response"`
}

// propfindBody asks for the one property the listing needs; a server that
// ignores the body answers with its defaults, which include it.
const propfindBody = `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`

// shareListing lists a Nextcloud share folder. ok is false when u is not a
// share link or the server did not answer as Nextcloud does; the caller then
// reads the link as an ordinary page.
func shareListing(ctx context.Context, f Fetcher, u *url.URL) (l listing, ok bool, err error) {
	dav, title, isShare := shareDAV(u)
	if !isShare {
		return listing{}, false, nil
	}
	if _, err := f.Check(dav.String()); err != nil {
		return listing{}, false, nil
	}
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", dav.String(), strings.NewReader(propfindBody))
	if err != nil {
		return listing{}, false, nil
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	resp, err := f.Client().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return listing{}, true, ctx.Err()
		}
		return listing{}, false, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return listing{}, false, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxListBytes+1))
	if err != nil {
		return listing{}, true, err
	}
	if len(body) > maxListBytes {
		return listing{}, true, ErrNotArchive
	}
	var ms davMultistatus
	if xml.Unmarshal(body, &ms) != nil {
		return listing{}, false, nil
	}
	base := resp.Request.URL
	for _, r := range ms.Responses {
		folder := false
		for _, ps := range r.Propstat {
			if ps.Prop.ResourceType.Collection != nil {
				folder = true
			}
		}
		if folder || r.Href == "" {
			continue
		}
		if v, err := base.Parse(r.Href); err == nil {
			l.links = append(l.links, v)
		}
	}
	l.title = title
	if l.title == "" {
		l.title = u.Hostname()
	}
	return l, true, nil
}
