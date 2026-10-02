// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGDAssets(t *testing.T) {
	s := newTestServer(t)
	for _, name := range []string{"Quivira.otf"} {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, newRequest("GET", "/assets/gd/fonts/"+name+"?v=4", nil))
		if rec.Code != 200 || rec.Body.Len() < 1000 || rec.Header().Get("Content-Type") != "font/otf" {
			t.Fatalf("%s: status %d, mime %s, size %d", name, rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
		}
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, newRequest("GET", "/assets/presets/gd/article-style.css?v=4", nil))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "file://") || !strings.Contains(rec.Body.String(), ".wu-gd .wu-ipa") {
		t.Fatal("GD CSS not adapted", rec.Code, rec.Body.String())
	}
	page, _ := s.pageFor("", "", "")
	for _, face := range []string{"Regular", "Bold", "Italic", "BoldItalic"} {
		for _, ext := range []string{"ttf", "otf"} {
			if !strings.Contains(rec.Body.String(), "/files/wugd_"+face+"."+ext) {
				t.Fatal("missing user font mapping", face, ext)
			}
		}
	}
	underline := strings.SplitN(strings.SplitN(rec.Body.String(), ".wu-gd .dsl_u,", 2)[1], "}", 2)[0]
	if !strings.Contains(underline, "text-decoration: underline;") || strings.Contains(underline, "background-color:") || strings.Contains(underline, "border:") {
		t.Fatal("GD underline must not become a highlighted box", underline)
	}
	if !strings.Contains(string(page), "/assets/presets/gd/article-style.css?v=7") {
		t.Fatal("global font faces missing for shadow articles")
	}
	for _, name := range []string{"../Quivira.otf", `..\Quivira.otf`, "missing.ttf", "ArialPlus.ttf", "ArialPlusBold.ttf", "ArialItalic.ttf", "ArialBoldItalic.ttf", "QuiviraPhonetic.ttf"} {
		rec := httptest.NewRecorder()
		s.handleGDFont(rec, newRequest("GET", "/assets/gd/fonts/"+name, nil))
		if rec.Code != 404 {
			t.Fatal("unsafe or unknown font path allowed", name)
		}
	}
}

func TestGDStyleSurvivesResourceRewrite(t *testing.T) {
	input := `<style>@import url("/assets/presets/gd/article-style.css?v=4");</style><div class="wu-gd"><img src="picture.png"></div>`
	got := RewriteEntryHTML(input, "dictionary")
	if !strings.Contains(got, `/assets/presets/gd/article-style.css?v=7`) || !strings.Contains(got, `src="/res/dictionary/picture.png"`) {
		t.Fatal(got)
	}
}
