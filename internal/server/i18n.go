package server

import (
	"embed"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// Catalogs are embedded and decoded once. No OS/browser locale detection: the
// reader explicitly chooses a language for this installation, default English.
//
//go:embed web/i18n/*.json
var uiCatalogFiles embed.FS

//go:embed web/i18n.js
var i18nJS []byte

//go:embed web/i18n.css
var i18nCSS []byte

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

func (p *Prefs) Language() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return uiLanguage(p.language)
}

// A dedicated update avoids an old tab's full UI preferences overwriting a
// newly chosen language. It also leaves dictionary order/groups untouched.
func (p *Prefs) setLanguage(code string) error {
	p.editMu.Lock()
	defer p.editMu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	old := p.language
	p.language = code
	if err := p.saveLocked(); err != nil {
		p.language = old
		return err
	}
	return nil
}

func (s *Server) handleLanguage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodPut {
		var req struct {
			Language string `json:"language"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := dec.Decode(new(any)); err != io.EOF || (req.Language != "en" && req.Language != "ru") {
			http.Error(w, "unsupported language", http.StatusBadRequest)
			return
		}
		if err := s.reg.prefs.setLanguage(req.Language); err != nil {
			http.Error(w, "could not save language", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]string{"language": s.reg.prefs.Language()})
}

var uiTextSlot = regexp.MustCompile(`\{\{T:([a-zA-Z0-9_.]+)\}\}`)

// Only explicit UI slots are translated; dictionary text never passes here.
// Static text is escaped for HTML/attributes; JSON uses encoding/json's HTML
// escaping, so a translated </script> cannot terminate the bootstrap element.
func renderUI(page, code string) string {
	code = uiLanguage(code)
	page = uiTextSlot.ReplaceAllStringFunc(page, func(slot string) string {
		key := uiTextSlot.FindStringSubmatch(slot)[1]
		value, ok := uiCatalogs[code][key].(string)
		if !ok {
			value, ok = uiCatalogs["en"][key].(string)
		}
		if !ok {
			value = key
		}
		return html.EscapeString(value)
	})
	boot, _ := json.Marshal(map[string]any{"language": code, "messages": uiCatalogs[code], "fallback": uiCatalogs["en"]})
	assets := `<script id="wudict-i18n" type="application/json">` + string(boot) + `</script>` +
		`<script src="/assets/i18n.js?v=` + assetTag(i18nJS) + `"></script>` +
		`<link rel="stylesheet" href="/assets/i18n.css?v=` + assetTag(i18nCSS) + `">`
	return strings.ReplaceAll(page, "{{I18N}}", assets)
}
