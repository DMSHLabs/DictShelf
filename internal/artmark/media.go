// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package artmark

import (
	"path"
	"strings"

	"github.com/wuweidict/wudict/internal/htmlref"
)

// MediaKind is what a browser can do with one media file an article names.
type MediaKind int

const (
	MediaFile MediaKind = iota // the default: hand it over as a link
	MediaAudio
	MediaImage
	MediaVideo
)

// mediaExt classifies a media file by extension. Lingvo's [s] and StarDict's
// resource lists name images, sound and video, and an author may put anything
// else beside them - a PDF plate, a document. Only what a browser can
// actually render or decode is named here; everything else is deliberately
// absent and becomes a file link.
//
// The formats dictionaries name but a browser cannot handle are the reason
// this is an allowlist rather than a "video/ prefix" test: .avi, .wmv, .flv,
// .mkv and .mpg are Lingvo's and GoldenDict's own video formats, and an
// inline <video> pointed at one is a permanently broken player. As a link
// they reach the system player instead, which is exactly what Lingvo does
// with them. Same for its .pcx, .dcx, .wmf and .emf images, which no browser
// draws.
var mediaExt = map[string]MediaKind{
	// Sound. Lingvo documents .wav alone; the rest arrive from GoldenDict-era
	// dictionaries, and .spx is transcoded to WAV when served (D18).
	"wav": MediaAudio, "mp3": MediaAudio, "ogg": MediaAudio,
	"spx": MediaAudio, "m4a": MediaAudio,
	// Images.
	"bmp": MediaImage, "gif": MediaImage, "ico": MediaImage,
	"jpeg": MediaImage, "jpg": MediaImage, "png": MediaImage,
	"svg": MediaImage, "tif": MediaImage, "tiff": MediaImage,
	"webp": MediaImage, "avif": MediaImage,
	// Video.
	"mp4": MediaVideo, "webm": MediaVideo, "ogv": MediaVideo,
	"mov": MediaVideo, "m4v": MediaVideo, "3gp": MediaVideo,
}

// KindOf classifies a media file name by its extension, case-insensitively.
func KindOf(name string) MediaKind {
	return mediaExt[strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")]
}

// Media is the element for one media file of kind k. Every kind renders
// something: emitting nothing for an unrecognised extension would lose the
// file silently, and nothing downstream can recover a reference that was
// never written. The audio and file kinds are links (<a>), which a caller
// writing inside a link must close around.
func Media(name string, k MediaKind) string {
	switch k {
	case MediaAudio:
		// A link, not GoldenDict's `<object type="audio/x-wav">`. That
		// spelling is a plugin-era embedding vector: `clean` has to drop it as
		// unsafe and rescue the URL back out (server/articleformat.go
		// audioObject), and both renderers need a handler that exists for
		// this one element. An anchor needs none of it: the server's rewriter
		// already recognises a media href on <a> and points it at /res/, both
		// renderers already play such a link, `clean` keeps it as an ordinary
		// link, and no inline handler is emitted (the sanitiser strips every
		// on* attribute, by design).
		//
		// A media reference carries no link text of its own, so the glyph is
		// the affordance.
		return `<a class="wu-audio" href=` + quoteAttr(name) + `>&#128266;</a>`
	case MediaImage:
		return `<img align="top" src=` + quoteAttr(name) + ` alt=` + quoteAttr(name) + ` />`
	case MediaVideo:
		// preload="none" is what makes this affordable: a card may carry
		// several videos of tens of megabytes each, and nothing is fetched
		// until the reader presses play. src is a fetch site, so the article
		// rewriter points it at /res/{dict}/ with no special case, and `clean`
		// already keeps <video src|controls> (server/articleformat.go).
		return `<video class="wu-video" controls preload="none" src=` + quoteAttr(name) + `></video>`
	default:
		// Anything else - a PDF, a document, a format no browser handles. The
		// `file://` pseudo-scheme is the author saying "this names MY file":
		// the rewriter honours it (server/rewrite.go isResourceRef) and
		// rewrites the link to /res/ whatever the extension, so a
		// dictionary's own container is served in full without widening
		// dict.IsAssetName - which is also the allowlist for files lying
		// LOOSE beside an .mdx, a boundary this must not touch.
		//
		// The file name is the link text because it is all there is, and a
		// bare glyph would not say what it opens.
		return `<a class="wu-file" href=` + quoteAttr("file://"+name) + `>&#128196; ` + htmlref.EscapeText(name) + `</a>`
	}
}

// quoteAttr renders a value as a complete double-quoted attribute.
func quoteAttr(s string) string { return `"` + htmlref.EscapeAttr(s) + `"` }
