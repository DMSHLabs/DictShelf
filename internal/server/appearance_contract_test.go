// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// The appearance contract: what EVERY document the server serves must do with
// colour and with the host's background, so that a new page, pane or dialog
// cannot arrive without them.
//
// It is a static contract on purpose. Whether a page LOOKS right with a paper
// or a wallpaper is a rendered question — the browser pass and the reader's
// phone answer that — but everything that makes it possible is in the bytes
// this server sends: the hook the host calls, the parameters it is handed, the
// rules that react to them, and the tokens a colour has to be written in for a
// theme, a paper or a preset to reach it. Each check below has caught, or would
// have caught, a real defect of exactly that shape; the one it did is named
// where it happened (the System pane's greys, 2026-09-29).
//
// What is deliberately NOT covered: the articles' own presentation
// (internal/artmark's DefaultCSS) and the preset files under web/presets/.
// Both are allowed colours of their own — a dictionary role or a paper is a
// colour somebody chose, not chrome — and the reader said as much about the
// presets in so many words.
func TestAppearanceContract(t *testing.T) {
	// Each document with the stylesheets it actually loads and the marker that
	// says its palette follows the paper. The pages have no user stylesheet and
	// no layer machinery; setup.css is shared by two of them, and Browse
	// carries its own palette inline (it is a full-bleed list, not a centred
	// card). The app page's palette is driven by data-shell-sepia — the colour
	// it pins --bg with — where the pages derive a tone from it instead.
	type doc struct {
		name string
		path string
		// The sheets it links, if any, plus its own inline styles.
		sheets []string
		// The attribute its stylesheet keys the paper's palette on.
		toneAttr string
	}
	docs := []doc{
		{"the app page", "/", []string{string(appCSS), string(groupEditorCSS), string(historyCSS), string(i18nCSS)}, "data-shell-sepia"},
		{"Edit Folders", "/setup", []string{string(setupCSS), string(i18nCSS)}, "data-shell-tone"},
		{"Lemmatization", "/lemmas", []string{string(setupCSS)}, "data-shell-tone"},
		{"Browse", "/browse", []string{string(i18nCSS)}, "data-shell-tone"},
	}
	// Everything that can paint a colour without being one of the sheets above:
	// the scripts that build styles in memory (the voice menu, the article
	// bases, the frame bridge) and the app page's own script, which carries the
	// shadow root's base stylesheet.
	scripts := map[string]string{
		"speak.js": string(speakJS), "looks.js": string(looksJS),
		"history.js": string(historyJS), "group-editor.js": string(groupEditorJS),
		"examples.js": string(examplesJS), "pick.js": string(pickJS),
		"frame.js": string(frameJS), "index.html": string(indexHTML),
		"i18n.js": string(i18nJS),
	}

	s, _ := newStyleServer(t)
	served := map[string]string{}
	inline := map[string]string{}
	for _, d := range docs {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, newRequest("GET", d.path, nil))
		if rec.Code != 200 {
			t.Fatalf("GET %s: got %d", d.path, rec.Code)
		}
		served[d.name] = rec.Body.String()
		inline[d.name] = inlineStyles(served[d.name])
	}

	// 1. Nothing may reach the browser with a placeholder still in it. A
	//    handler that forgets one of its ReplaceAll calls ships a literal
	//    `{{PRESETS}}` into the page — invisible in every other test, and the
	//    reason this check comes first.
	t.Run("no placeholder survives into a served document", func(t *testing.T) {
		for name, body := range served {
			if at := strings.Index(body, "{{"); at >= 0 {
				t.Errorf("%s still carries %q", name, snippetAt(body, at))
			}
		}
	})

	// 2. The host's background: every document defines the hook it is called
	//    through, and reads what it is told. The FOUR arguments are the
	//    contract with Shell.applyBackground — the reader's colour, whether a
	//    wallpaper is active, its name, and the paper an enabled preset paints
	//    — and a document that forgets one silently stops wearing it. That Java
	//    call is the one half of this contract no Go test can see; what is
	//    checked here is that the page is ready to be called with all four.
	//    What is NOT checked, deliberately: that the host actually computes the
	//    right colour for the theme - TestPresetPaperFollowsTheEnabledSet and
	//    the shell's own side carry that.
	t.Run("every document takes the host's background", func(t *testing.T) {
		const hook = "window.wudictShellBackground = function (color, image, imageName, paper)"
		for name, body := range served {
			if !strings.Contains(body, hook) {
				t.Errorf("%s does not define the shell background hook with its four arguments", name)
			}
			for _, param := range []string{"shell_bg", "shell_image", "shell_paper"} {
				if !strings.Contains(body, param) {
					t.Errorf("%s never reads %s, so that half of the host's answer is lost", name, param)
				}
			}
		}
	})

	// 3. What the document does with it. The wallpaper makes the page
	//    transparent, because the host paints the image BEHIND the WebView; the
	//    paper has to be able to reach the palette, or a warm page sits under a
	//    cold card; and --label is here for the same reason, as the register a
	//    label preset flips — a document that does not read it stays ink while
	//    the rest of the app goes quiet.
	t.Run("every document reacts to the wallpaper and the paper", func(t *testing.T) {
		for _, d := range docs {
			styles := strings.Join(append(append([]string{}, d.sheets...), inline[d.name]), "\n")
			for _, want := range []struct{ needle, why string }{
				{"html[data-shell-image]", "the wallpaper attribute is never styled: the page would paint over the host's image"},
				{"background:transparent", "nothing goes transparent under a wallpaper"},
				{d.toneAttr, "the palette never follows the paper's colour"},
				{"--label", "the label register is not read: the Quiet labels choice cannot reach this document"},
			} {
				if !strings.Contains(styles, want.needle) {
					t.Errorf("%s: %s (%s)", d.name, want.why, want.needle)
				}
			}
		}
		// The app page's own paper variables, which its dialogs, its menus and
		// the history window are painted with.
		for _, needle := range []string{"--paper-bg", "--paper-bk-image"} {
			if !strings.Contains(string(appCSS), needle) {
				t.Errorf("app.css never uses %s, so the app's own windows cannot wear the host's paper", needle)
			}
		}
	})

	// 4. Text is a TOKEN. A literal colour in a `color:` is a colour no theme,
	//    paper or preset can reach — and it is how the new System pane arrived
	//    on 2026-09-29 with the two greys on its headings, hints, units and
	//    notes: grey in BOTH looks, which is the one thing the register exists
	//    to prevent. Two exceptions, both white on a filled surface.
	whiteOnFill := map[string]string{
		".pd .feat.locked:not([data-feat]):hover": "the hover of a filled pill: the fill comes from .pd .feat.on",
		".pd .feat.on.locked:hover":               "the hover of a filled pill, dimmed to say it cannot be pressed",
	}
	t.Run("text colours are tokens", func(t *testing.T) {
		seen := map[string]bool{}
		for _, d := range docs {
			for _, sheet := range append(append([]string{}, d.sheets...), inline[d.name]) {
				for _, rule := range cssRules(sheet) {
					if !textLiteralRE.MatchString(rule.body) {
						continue
					}
					if _, ok := whiteOnFill[rule.selector]; ok {
						seen[rule.selector] = true
						continue
					}
					// White on a fill the same rule states is legitimate in
					// every theme: the fill is what carries the contrast.
					if strings.Contains(rule.body, "color:#fff") &&
						(strings.Contains(rule.body, "background:var(--accent)") ||
							strings.Contains(rule.body, "background:var(--danger)") ||
							strings.Contains(rule.body, "background:var(--safe)")) {
						continue
					}
					t.Errorf("%s: %s paints text with a literal colour — write it as a token (var(--fg), var(--label), var(--link), the state colours), or add its selector to the list in this test with a reason",
						d.name, rule.selector)
				}
			}
		}
		for name, script := range scripts {
			if at := textLiteralRE.FindStringIndex(script); at != nil {
				t.Errorf("%s paints text with a literal colour: %q — a literal is a colour no theme or preset can reach",
					name, snippetAt(script, at[0]))
			}
		}
		for selector := range whiteOnFill {
			if !seen[selector] {
				t.Errorf("the white-on-fill list keeps %s, which no stylesheet paints white any more", selector)
			}
		}
	})

	// 5. Every token used WITHOUT a fallback has to be declared somewhere the
	//    app ships. A typo (`var(--labl)`) is otherwise invisible: the browser
	//    drops the declaration and the property falls back to whatever it
	//    inherits, which is the kind of defect a reader reports as "this looks
	//    wrong" and nobody can grep for. `var(--x, fallback)` needs no
	//    declaration — that is what a fallback is for — which is also how the
	//    host's runtime tokens (--wd-inset-top, --paper-bg, --barh) are read.
	t.Run("every token used without a fallback is declared", func(t *testing.T) {
		var all []string
		for _, d := range docs {
			all = append(all, d.sheets...)
			all = append(all, inline[d.name])
		}
		for _, script := range scripts {
			all = append(all, script)
		}
		declared := map[string]bool{}
		for _, sheet := range all {
			for _, name := range declaredTokens(sheet) {
				declared[name] = true
			}
		}
		reported := map[string]bool{}
		for _, sheet := range all {
			for _, name := range bareTokens(sheet) {
				if !declared[name] && !reported[name] {
					reported[name] = true
					t.Errorf("var(%s) is read without a fallback and declared nowhere: a typo here renders as whatever was inherited", name)
				}
			}
		}
	})
}

// textLiteralRE matches a `color:` whose value is a literal colour rather than a
// token. The leading delimiter is what keeps `border-color` and
// `background-color` out: those get their literals from the token system too,
// but a border is not text, and one hover rule sets a grey one on purpose.
// `color:var(--x, #fff)` does not match — a fallback is not a literal choice.
var textLiteralRE = regexp.MustCompile(`(?:^|[;{\s])color:\s*(#[0-9a-fA-F]{3,8}|rgba?\(|hsla?\()`)

// inlineStyles returns the text of every real <style> block of a document.
// Scripts are cut out FIRST: index.html builds a shadow root's stylesheet as a
// template literal that itself reads `<style>`, and a scan that took that for a
// block would report the article base's `a{color:#5b7a99}` as if the page had
// written it (which is exactly what it did before this comment existed).
func inlineStyles(html string) string {
	html = regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`).ReplaceAllString(html, "")
	var b strings.Builder
	for _, m := range regexp.MustCompile(`(?s)<style[^>]*>(.*?)</style>`).FindAllStringSubmatch(html, -1) {
		b.WriteString(m[1])
		b.WriteString("\n")
	}
	return b.String()
}

// declaredTokens names the custom properties a stylesheet or script declares —
// including the ones a script writes at runtime, which is how --paper-bg and
// --paper-bk-image exist at all: the host's hook sets them with setProperty,
// and a declaration in a stylesheet is not where they come from.
func declaredTokens(src string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`(--[A-Za-z0-9-]+)\s*:`).FindAllStringSubmatch(src, -1) {
		out = append(out, m[1])
	}
	for _, m := range regexp.MustCompile(`setProperty\(\s*['"](--[A-Za-z0-9-]+)`).FindAllStringSubmatch(src, -1) {
		out = append(out, m[1])
	}
	return out
}

// bareTokens names the custom properties read WITHOUT a fallback.
func bareTokens(src string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`var\(\s*(--[A-Za-z0-9-]+)\s*\)`).FindAllStringSubmatch(src, -1) {
		out = append(out, m[1])
	}
	return out
}

// snippetAt is a few characters of context around an offset, for a failure
// message that can be found in the file.
func snippetAt(src string, at int) string {
	end := at + 60
	if end > len(src) {
		end = len(src)
	}
	return strings.TrimSpace(src[at:end])
}
