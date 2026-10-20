// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"strings"
)

type desktopAssetResponse struct {
	body         []byte
	contentType  string
	cacheControl string
}

// desktopAsset resolves embedded resources without entering an HTTP handler.
// These assets are immutable for the lifetime of the executable; their URLs
// carry a content version except for the browser's conventional favicon path.
func desktopAsset(name string) (desktopAssetResponse, bool) {
	const immutable = "public, max-age=604800, immutable"
	const faviconCache = "public, max-age=604800"
	switch name {
	case "/assets/i18n.js":
		return desktopAssetResponse{i18nJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/i18n.css":
		return desktopAssetResponse{i18nCSS, "text/css; charset=utf-8", immutable}, true
	case "/assets/frame.js":
		return desktopAssetResponse{frameJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/app.css":
		return desktopAssetResponse{appCSS, "text/css; charset=utf-8", immutable}, true
	case "/assets/fonts/Quivira.otf":
		body, err := articleFontFiles.ReadFile("web/fonts/Quivira.otf")
		if err == nil {
			return desktopAssetResponse{body, "font/otf", immutable}, true
		}
	case "/assets/fonts/Charis-Regular.woff2", "/assets/fonts/Charis-Italic.woff2", "/assets/fonts/Charis-Medium.woff2", "/assets/fonts/Charis-MediumItalic.woff2", "/assets/fonts/Charis-Bold.woff2", "/assets/fonts/Charis-BoldItalic.woff2",
		"/assets/fonts/Gentium-Regular.woff2", "/assets/fonts/Gentium-Italic.woff2", "/assets/fonts/Gentium-Medium.woff2", "/assets/fonts/Gentium-MediumItalic.woff2", "/assets/fonts/Gentium-Bold.woff2", "/assets/fonts/Gentium-BoldItalic.woff2":
		body, err := articleFontFiles.ReadFile("web/fonts/" + name[strings.LastIndex(name, "/")+1:])
		if err == nil {
			return desktopAssetResponse{body, "font/woff2", immutable}, true
		}
	case "/assets/history.css":
		return desktopAssetResponse{historyCSS, "text/css; charset=utf-8", immutable}, true
	case "/assets/history.js":
		return desktopAssetResponse{historyJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/group-editor.css":
		return desktopAssetResponse{groupEditorCSS, "text/css; charset=utf-8", immutable}, true
	case "/assets/group-editor.js":
		return desktopAssetResponse{groupEditorJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/filters-dialog.css":
		return desktopAssetResponse{filtersDialogCSS, "text/css; charset=utf-8", immutable}, true
	case "/assets/filters-dialog.js":
		return desktopAssetResponse{filtersDialogJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/folders-dialog.css":
		return desktopAssetResponse{foldersDialogCSS, "text/css; charset=utf-8", immutable}, true
	case "/assets/setup.js":
		return desktopAssetResponse{setupJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/lemma-dialog.css":
		return desktopAssetResponse{lemmaDialogCSS, "text/css; charset=utf-8", immutable}, true
	case "/assets/lemmas.js":
		return desktopAssetResponse{lemmasJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/article-find.js":
		return desktopAssetResponse{articleFindJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/list-search.js":
		return desktopAssetResponse{listSearchJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/headword-control.js":
		return desktopAssetResponse{headwordControlJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/headword-control.css":
		return desktopAssetResponse{headwordControlCSS, "text/css; charset=utf-8", immutable}, true
	case "/assets/headword-ribbon.js":
		return desktopAssetResponse{headwordRibbonJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/work-progress.js":
		return desktopAssetResponse{workProgressJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/examples.js":
		return desktopAssetResponse{examplesJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/looks.js":
		return desktopAssetResponse{looksJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/pick.js":
		return desktopAssetResponse{pickJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/speak.js":
		return desktopAssetResponse{speakJS, "application/javascript; charset=utf-8", immutable}, true
	case "/assets/setup.css":
		return desktopAssetResponse{setupCSS, "text/css; charset=utf-8", immutable}, true
	case "/assets/workspace.css":
		return desktopAssetResponse{workspaceCSS, "text/css; charset=utf-8", immutable}, true
	case "/assets/favicon.svg", "/favicon.ico":
		return desktopAssetResponse{faviconSVG, "image/svg+xml", faviconCache}, true
	}
	if strings.HasPrefix(name, "/assets/presets/") {
		body, ok := presetAsset(strings.TrimPrefix(name, "/assets/presets/"))
		if ok {
			return desktopAssetResponse{body, "text/css; charset=utf-8", immutable}, true
		}
		return desktopAssetResponse{}, false
	}
	if strings.HasPrefix(name, "/assets/gd/fonts/") {
		body, ok := gdFontAsset(strings.TrimPrefix(name, "/assets/gd/fonts/"))
		if ok {
			return desktopAssetResponse{body, "font/otf", immutable}, true
		}
	}
	return desktopAssetResponse{}, false
}
