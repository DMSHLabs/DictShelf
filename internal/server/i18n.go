// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"embed"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/text/language"
)

// Catalogs are embedded and decoded once. The saved choice is System, English
// or Russian; System resolves from the requesting browser/app's preferred
// languages, independently of dictionary and article languages.
//
//go:embed web/i18n/*.json
var uiCatalogFiles embed.FS

//go:embed web/i18n.js
var i18nJS []byte

//go:embed web/i18n.css
var i18nCSS []byte

//go:embed web/work-progress.js
var workProgressJS []byte

var uiCatalogs = func() map[string]map[string]any {
	out := make(map[string]map[string]any)
	for _, code := range []string{"en", "ru"} {
		data, err := uiCatalogFiles.ReadFile("web/i18n/" + code + ".json")
		if err != nil {
			panic(err)
		}
		var catalog map[string]any
		if err := json.Unmarshal(data, &catalog); err != nil {
			panic(err)
		}
		out[code] = catalog
	}
	return out
}()

func uiLanguage(code string) string {
	if code == "ru" {
		return code
	}
	return "en"
}

func uiLanguageChoice(code string) string {
	switch code {
	case "en", "ru", "system":
		return code
	default:
		return "system"
	}
}

var uiLanguageMatcher = language.NewMatcher([]language.Tag{language.English, language.Russian})

func resolveUILanguage(choice, acceptLanguage string) string {
	choice = uiLanguageChoice(choice)
	if choice != "system" {
		return choice
	}
	if len(acceptLanguage) > 1024 {
		acceptLanguage = acceptLanguage[:1024]
	}
	preferred, _, _ := language.ParseAcceptLanguage(acceptLanguage)
	if len(preferred) != 0 {
		matched, _, _ := uiLanguageMatcher.Match(preferred...)
		if base, _ := matched.Base(); base.String() == "ru" {
			return "ru"
		}
	}
	return "en"
}

func (p *Prefs) Language() string {
	f, _ := p.data()
	return uiLanguageChoice(f.Language)
}

func (p *Prefs) ResolvedLanguage(acceptLanguage string) string {
	return resolveUILanguage(p.Language(), acceptLanguage)
}

// A dedicated update avoids an old tab's full UI preferences overwriting a
// newly chosen language. It also leaves dictionary order/groups untouched.
func (p *Prefs) setLanguage(code string) error {
	if code != "system" && code != "en" && code != "ru" {
		return errors.New("unsupported language")
	}
	p.editMu.Lock()
	defer p.editMu.Unlock()
	return p.mutate(func(f *prefsFile) { f.Language = code })
}

func (s *Server) handleLanguage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Vary", "Accept-Language")
	if r.Method == http.MethodPut {
		var req struct {
			Language string `json:"language"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := dec.Decode(new(any)); err != io.EOF || (req.Language != "system" && req.Language != "en" && req.Language != "ru") {
			http.Error(w, "unsupported language", http.StatusBadRequest)
			return
		}
		if err := s.reg.prefs.setLanguage(req.Language); err != nil {
			http.Error(w, "could not save language", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]string{"language": s.reg.prefs.Language(), "resolved": s.reg.prefs.ResolvedLanguage(r.Header.Get("Accept-Language"))})
}

var uiTextSlot = regexp.MustCompile(`\{\{T:([a-zA-Z0-9_.]+)\}\}`)
var uiParameter = regexp.MustCompile(`\{[a-zA-Z0-9_]+\}`)

func uiText(code, key string) string {
	if value, ok := uiCatalogs[uiLanguage(code)][key].(string); ok {
		return value
	}
	if value, ok := uiCatalogs["en"][key].(string); ok {
		return value
	}
	return key
}

// Only explicit UI slots are translated; dictionary text never passes here.
// Static text is escaped for HTML/attributes; JSON uses encoding/json's HTML
// escaping, so a translated </script> cannot terminate the bootstrap element.
func renderUI(page, code string) string {
	return renderUIChoice(page, code, code)
}

func renderUIChoice(page, code, choice string) string {
	code = uiLanguage(code)
	choice = uiLanguageChoice(choice)
	page = strings.ReplaceAll(page, "{{UI_SCALE_DEFAULTS}}", uiScaleDefaultsStyle)
	page = uiTextSlot.ReplaceAllStringFunc(page, func(slot string) string {
		key := uiTextSlot.FindStringSubmatch(slot)[1]
		return html.EscapeString(uiText(code, key))
	})
	boot, _ := json.Marshal(map[string]any{"language": code, "selection": choice, "messages": uiCatalogs[code], "fallback": uiCatalogs["en"]})
	assets := `<script id="wudict-i18n" type="application/json">` + string(boot) + `</script>` +
		`<script>window.__wudictI18nMarkupAtMs=performance.now()</script>` +
		`<script src="/assets/i18n.js?v=` + assetTag(i18nJS) + `"></script>` +
		`<link rel="stylesheet" href="/assets/i18n.css?v=` + assetTag(i18nCSS) + `">`
	assets += `<script defer src="/assets/work-progress.js?v=` + assetTag(workProgressJS) + `"></script>`
	return strings.ReplaceAll(page, "{{I18N}}", assets)
}
