// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestStripComments(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
		kind           assetKind
	}{
		{"js line comment", "a();\n  // gone\nb();\n", "a();\nb();\n", jsAsset},
		{"js block on its own lines", "a();\n/* one\n   two */\nb();\n", "a();\nb();\n", jsAsset},
		{"js trailing comment stays", "a(); // kept\n", "a(); // kept\n", jsAsset},
		{"js block followed by code stays", "/* x */ a();\n", "/* x */ a();\n", jsAsset},
		{"js template literal content stays", "const p=`a\n/* css comment */\n// text\n`;\n// gone\n",
			"const p=`a\n/* css comment */\n// text\n`;\n", jsAsset},
		{"js nested template", "const p=`${x?`\n// in\n`:1}\n// in too\n`;\n// out\n",
			"const p=`${x?`\n// in\n`:1}\n// in too\n`;\n", jsAsset},
		{"js string holding //", "const u='//x';\n// gone\n", "const u='//x';\n", jsAsset},
		{"js regex holding a backtick", "const r=/`/;\n// gone\n", "const r=/`/;\n", jsAsset},
		{"js regex after return", "function f(){return /`/}\n// gone\n", "function f(){return /`/}\n", jsAsset},
		{"js division is not a regex", "x=a/2;\n// gone\nconst t=`\n// kept\n`;\n",
			"x=a/2;\nconst t=`\n// kept\n`;\n", jsAsset},
		{"css block", "a{b:c}\n  /* gone */\nd{e:f}\n", "a{b:c}\nd{e:f}\n", cssAsset},
		{"css // is not a comment", "// kept\n", "// kept\n", cssAsset},
		{"html comment on its own lines", "<p>\n<!-- one\n two -->\n</p>\n", "<p>\n</p>\n", htmlAsset},
		{"html comment inline", "<p>a<!-- x -->b</p>\n", "<p>ab</p>\n", htmlAsset},
		{"html leaves script strings", "<script>\nconst s='<!-- x -->';\n// gone\n</script>\n",
			"<script>\nconst s='<!-- x -->';\n</script>\n", htmlAsset},
		{"html style block", "<style>\n/* gone */\na{}\n</style>\n", "<style>\na{}\n</style>\n", htmlAsset},
		{"licence notices stay", "<!--\n SPDX-License-Identifier: X\n-->\n<style>\n/* SPDX-License-Identifier: X */\n</style>\n",
			"<!--\n SPDX-License-Identifier: X\n-->\n<style>\n/* SPDX-License-Identifier: X */\n</style>\n", htmlAsset},
		{"js licence notice stays", "/**\n * SPDX-License-Identifier: X\n */\n// gone\n", "/**\n * SPDX-License-Identifier: X\n */\n", jsAsset},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripComments(tc.in, tc.kind); got != tc.want {
				t.Errorf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

var placeholderRE = regexp.MustCompile(`\{\{[A-Z]+\}\}`)

// What is served is what the sources say, less their comments: every
// placeholder still there, every script still a script, the Custom styles
// presets - comments included - untouched, and stripping twice a no-op.
func TestServedAssets(t *testing.T) {
	node, _ := exec.LookPath("node")
	for _, a := range []struct {
		file   string
		served []byte
		kind   assetKind
	}{
		{"index.html", indexHTML, htmlAsset},
		{"setup.html", []byte(setupHTML), htmlAsset},
		{"lemmas.html", lemmasHTML, htmlAsset},
		{"groups.html", groupsHTML, htmlAsset},
		{"browse.html", browseHTML, htmlAsset},
		{"setup.css", setupCSS, cssAsset},
		{"frame.js", frameJS, jsAsset},
		{"pick.js", pickJS, jsAsset},
		{"speak.js", speakJS, jsAsset},
	} {
		t.Run(a.file, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("web", a.file))
			if err != nil {
				t.Fatal(err)
			}
			served := string(a.served)
			if len(served) >= len(src) && strings.Contains(string(src), "/*") {
				t.Errorf("nothing was stripped: %d of %d bytes", len(served), len(src))
			}
			if got := stripComments(served, a.kind); got != served {
				t.Error("stripping twice changed the output")
			}
			want := map[string]bool{}
			for _, p := range placeholderRE.FindAllString(string(src), -1) {
				want[p] = true
			}
			for _, p := range placeholderRE.FindAllString(served, -1) {
				delete(want, p)
			}
			for p := range want {
				t.Errorf("placeholder %s lost", p)
			}
			if node == "" {
				return
			}
			var scripts []string
			switch a.kind {
			case jsAsset:
				scripts = []string{served}
			case htmlAsset:
				for _, m := range scriptRE.FindAllStringSubmatch(served, -1) {
					scripts = append(scripts, m[2])
				}
			}
			for i, js := range scripts {
				f := filepath.Join(t.TempDir(), "s.js")
				if err := os.WriteFile(f, []byte(placeholderRE.ReplaceAllString(js, "0")), 0o644); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(node, "--check", f).CombinedOutput(); err != nil {
					t.Errorf("script %d no longer parses: %v\n%s", i, err, out)
				}
			}
		})
	}
	// The presets are text the user's stylesheet receives, comments and all.
	for _, keep := range []string{"/* The fit half.", "/* The dictionary author's own colour", "/* The app's own font"} {
		if !strings.Contains(string(indexHTML), keep) {
			t.Errorf("a Custom styles preset lost its comment %q", keep)
		}
	}
}
