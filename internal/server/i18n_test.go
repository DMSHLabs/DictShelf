// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestI18nPersistenceAndIsolation(t *testing.T) {
	s, path := newPrefsServer(t)
	call := func(body string, status int) {
		t.Helper()
		r := serve(s, newRequest("PUT", "/api/language", strings.NewReader(body)))
		if r.Code != status {
			t.Fatalf("language: %d %s", r.Code, r.Body.String())
		}
	}
	before := serve(s, newRequest("GET", "/api/dicts", nil)).Body.String()
	call(`{"language":"ru"}`, 200)
	if got := LoadPrefs(path).Language(); got != "ru" {
		t.Fatalf("after reload: %s", got)
	}
	putPrefs(t, s, `{"ui":{"fontSize":24},"dicts":[]}`)
	if got := LoadPrefs(path); got.Language() != "ru" || got.UI().FontSize != 24 {
		t.Fatal("ordinary preference save lost language or font size")
	}
	call(`{"language":"de"}`, 400)
	call(`{"language":"ru-RU"}`, 400)
	call(`{}`, 400)
	call(`{"language":"en"} {}`, 400)
	if s.reg.prefs.Language() != "ru" {
		t.Fatal("invalid request changed language")
	}
	call(`{"language":"en"}`, 200)
	if got := LoadPrefs(path); got.Language() != "en" || got.UI().FontSize != 24 {
		t.Fatal("language switch changed reading preferences")
	}
	after := serve(s, newRequest("GET", "/api/dicts", nil)).Body.String()
	// NDJSON dictionaries are streamed concurrently; compare records, not order.
	a, b := strings.Split(strings.TrimSpace(before), "\n"), strings.Split(strings.TrimSpace(after), "\n")
	slices.Sort(a)
	slices.Sort(b)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("interface language changed dictionary data")
	}
}

func TestI18nDefaultAndWriteFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	for _, data := range []string{`{"version":1}`, `{"language":"zz"}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if LoadPrefs(path).Language() != "en" {
			t.Fatal("missing/unsupported language must default to English")
		}
	}
	p := LoadPrefs("")
	if err := p.setLanguage("ru"); err != nil {
		t.Fatal(err)
	}
	// The parent is a regular file: saving must fail and leave memory unchanged.
	p.path = filepath.Join(path, "state.json")
	if err := p.setLanguage("en"); err == nil {
		t.Fatal("expected save failure")
	}
	if p.Language() != "ru" {
		t.Fatal("failed save changed in-memory language")
	}
}

func TestI18nPagesAndCache(t *testing.T) {
	s, _ := newPrefsServer(t)
	req := newRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "ru")
	en := serve(s, req)
	if !strings.Contains(en.Body.String(), `"language":"en"`) {
		t.Fatal("system/browser language affected default")
	}
	if err := s.reg.prefs.setLanguage("ru"); err != nil {
		t.Fatal(err)
	}
	req = newRequest("GET", "/", nil)
	req.Header.Set("If-None-Match", en.Header().Get("ETag"))
	ru := serve(s, req)
	if ru.Code != 200 || ru.Header().Get("ETag") == en.Header().Get("ETag") {
		t.Fatal("language did not invalidate the cached page")
	}
	for _, path := range []string{"/", "/setup", "/browse", "/lemmas"} {
		r := serve(s, newRequest("GET", path, nil))
		body := r.Body.String()
		if r.Code != 200 || strings.Contains(body, "{{") || !strings.Contains(body, `"language":"ru"`) {
			t.Errorf("untranslated/bootstrap missing: %s", path)
		}
		if path == "/browse" && !strings.Contains(body, `<title>Просмотр словаря</title>`) {
			t.Fatal("static UI was not translated")
		}
		if path != "/" && strings.Contains(body, "data-ui-language") {
			t.Errorf("temporary language button remains on %s", path)
		}
		for route, title := range map[string]string{"/setup": "Папки словарей", "/lemmas": "Словоформы"} {
			if path == route && !strings.Contains(body, "<title>"+title+"</title>") {
				t.Errorf("page title not translated: %s", path)
			}
		}
		if path == "/" && !strings.Contains(body, `<html lang="en">`) {
			t.Fatal("partial UI translation changed article segmentation fallback")
		}
		if path == "/" {
			for _, text := range []string{`placeholder="Поиск…"`, `<option value="prefix">начинается с</option>`, `<option value="exact">точно</option>`} {
				if !strings.Contains(body, text) {
					t.Errorf("search UI missing %q", text)
				}
			}
			for _, text := range []string{`id="panel" lang="ru"`, `>Настройки`, `>Фон окна…</button>`, `>Слои оформления…</button>`, `>Сохранить оформление</h2>`} {
				if !strings.Contains(body, text) {
					t.Errorf("main settings/appearance missing %q", text)
				}
			}
		}
	}
	if err := s.reg.prefs.setLanguage("en"); err != nil {
		t.Fatal(err)
	}
	back := serve(s, newRequest("GET", "/", nil))
	if back.Header().Get("ETag") != en.Header().Get("ETag") {
		t.Fatal("switching back did not restore English page")
	}
	// No loaded dictionaries is the separate first-run path through setupPage.
	empty, err := NewRegistry(nil, false, WithPrefs(LoadPrefs("")))
	if err != nil {
		t.Fatal(err)
	}
	if err := empty.prefs.setLanguage("ru"); err != nil {
		t.Fatal(err)
	}
	first := serve(New(empty), newRequest("GET", "/", nil)).Body.String()
	if strings.Contains(first, "{{") || !strings.Contains(first, "Папка словарей ещё не задана.") {
		t.Fatal("first run is not translated")
	}
}

func TestI18nFolderIntro(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		dirs  []string
		count int
		want  string
	}{
		{nil, 0, "Папка словарей ещё не задана."},
		{[]string{dir}, 0, "пока нет словарей."},
		{[]string{dir, dir}, 0, "ни в одной из папок (2)."},
		{[]string{dir}, 22, "Используется словарей: 22. Папок: 1."},
		{[]string{`missing/<b>{count}</b>`}, 0, `<code>missing/&lt;b&gt;{count}&lt;/b&gt;</code> не существует.`},
	} {
		if page := setupPage(tc.dirs, tc.count, "", "ru"); !strings.Contains(page, tc.want) {
			t.Errorf("intro missing %q", tc.want)
		}
	}
	if page := setupPage([]string{dir}, 2, "", "en"); !strings.Contains(page, "Serving 2 dictionaries from 1 folder.") {
		t.Fatal("English intro changed")
	}
}

func TestI18nCatalogs(t *testing.T) {
	params := regexp.MustCompile(`\{[a-zA-Z0-9_]+\}`)
	placeholders := func(s string) []string { v := params.FindAllString(s, -1); slices.Sort(v); return slices.Compact(v) }
	for code, catalog := range uiCatalogs {
		if len(catalog) != len(uiCatalogs["en"]) {
			t.Errorf("%s: keys differ", code)
		}
		for key, base := range uiCatalogs["en"] {
			translation, ok := catalog[key]
			if !ok {
				t.Errorf("%s: missing %s", code, key)
				continue
			}
			switch base := base.(type) {
			case string:
				value, ok := translation.(string)
				if !ok || value == "" || !reflect.DeepEqual(placeholders(base), placeholders(value)) {
					t.Errorf("%s: invalid text/parameters for %s", code, key)
				}
			case map[string]any:
				forms, ok := translation.(map[string]any)
				if !ok {
					t.Errorf("%s: plural missing for %s", code, key)
					continue
				}
				required := []string{"one", "other"}
				if code == "ru" {
					required = append(required, "few", "many")
				}
				for _, form := range required {
					if _, ok := forms[form]; !ok {
						t.Errorf("%s: %s lacks %s", code, key, form)
					}
				}
				for _, value := range forms {
					text, ok := value.(string)
					if !ok || text == "" || !reflect.DeepEqual(placeholders(base["other"].(string)), placeholders(text)) {
						t.Errorf("%s: invalid plural parameters for %s", code, key)
					}
				}
			default:
				t.Errorf("unexpected catalog value %s", key)
			}
		}
	}
	// This checks explicit translation references, not arbitrary new English
	// upstream literals. Those still need review when merging upstream changes.
	refs := regexp.MustCompile(`\b(?:t|tx)\("([a-zA-Z0-9_.]+)"`)
	for _, file := range []string{"web/index.html", "web/setup.html", "web/lemmas.html", "web/browse.html", "web/i18n.js", "web/looks.js", "web/group-editor.js", "web/speak.js"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, re := range []*regexp.Regexp{refs, uiTextSlot} {
			for _, match := range re.FindAllStringSubmatch(string(data), -1) {
				if _, ok := uiCatalogs["en"][match[1]]; !ok {
					t.Errorf("%s: unknown key %s", file, match[1])
				}
			}
		}
	}
}

func TestI18nEscapingAndFallback(t *testing.T) {
	const key = "test.fallback"
	uiCatalogs["en"][key] = `</script><b>"&`
	defer delete(uiCatalogs["en"], key)
	page := renderUI(`{{T:test.fallback}} {{I18N}}`, "ru")
	if !strings.HasPrefix(page, "&lt;/script&gt;&lt;b&gt;&#34;&amp;") {
		t.Fatal("static fallback was not escaped")
	}
	if strings.Contains(page, `</script><b>`) {
		t.Fatal("catalog can break out of bootstrap")
	}
	start := strings.Index(page, `type="application/json">`) + len(`type="application/json">`)
	end := strings.Index(page[start:], "</script>") + start
	if !json.Valid([]byte(page[start:end])) {
		t.Fatal("invalid bootstrap JSON")
	}
}

func TestI18nAuth(t *testing.T) {
	s := authServer(t)
	for _, method := range []string{"GET", "PUT"} {
		r := httptest.NewRecorder()
		s.ServeHTTP(r, newRequest(method, "/api/language", strings.NewReader(`{"language":"ru"}`)))
		if r.Code != 401 {
			t.Errorf("%s: unauthenticated language access = %d", method, r.Code)
		}
	}
	for _, path := range []string{"/assets/i18n.js", "/assets/i18n.css"} {
		if r := serve(s, newRequest("GET", path, nil)); r.Code != 200 {
			t.Errorf("bootstrap asset unavailable: %s", path)
		}
	}
}
