// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// parseRef exists twice, in index.html (main page, Shadow-DOM articles) and in
// frame.js (sandboxed iframes), and the two must agree. This runs both copies,
// cut out of the shipped files, against one table under node. Without node on
// PATH it skips: the table is the contract, node is only the way to run it.

type refResult struct {
	Kind string `json:"kind"`
	Word string `json:"word"`
	Frag string `json:"frag"`
}

// cutJS returns src from the first occurrence of from through the first end
// after it (inclusive).
func cutJS(t *testing.T, src, from, end string) string {
	t.Helper()
	i := strings.Index(src, from)
	if i < 0 {
		t.Fatalf("%q not found", from)
	}
	j := strings.Index(src[i:], end)
	if j < 0 {
		t.Fatalf("end of %q not found", from)
	}
	return src[i : i+j+len(end)]
}

func TestParseRefBothCopies(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH")
	}
	tests := []struct {
		href string
		want *refResult
	}{
		{"entry://run", &refResult{"lookup", "run", ""}},
		{"entry:run", &refResult{"lookup", "run", ""}},
		{"bword://run", &refResult{"lookup", "run", ""}},
		{"BWORD:run", &refResult{"lookup", "run", ""}},
		{"d:run", &refResult{"lookup", "run", ""}},
		{"entry://long run#s2", &refResult{"lookup", "long run", "s2"}},
		{"entry://C%23#s%202", &refResult{"lookup", "C#", "s 2"}},
		{"entry://100%", &refResult{"lookup", "100%", ""}},
		{"entry://%25x", &refResult{"lookup", "%x", ""}},
		// R5.16: "@" is decided on the undecoded target
		{"entry:@examples", &refResult{"sub", "@examples", ""}},
		{"entry://@examples", &refResult{"sub", "@examples", ""}},
		{"entry://%40home", &refResult{"lookup", "@home", ""}},
		{"entry:@", &refResult{"lookup", "@", ""}},
		// trim spaces and tabs only
		{"entry:// \trun\t ", &refResult{"lookup", "run", ""}},
		{"entry://%E3%80%80x%C2%A0", &refResult{"lookup", "　x ", ""}},
		{"entry://#toc", &refResult{"anchor", "", "toc"}},
		{"entry://", &refResult{"anchor", "", ""}},
		{"https://example.com/a", nil},
		{"run.mp3", nil},
	}
	hrefs := make([]string, len(tests))
	for i, tt := range tests {
		hrefs[i] = tt.href
	}
	in, _ := json.Marshal(hrefs)

	index, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	frame, err := os.ReadFile("web/frame.js")
	if err != nil {
		t.Fatal(err)
	}
	index = []byte(strings.ReplaceAll(string(index), "\r\n", "\n"))
	frame = []byte(strings.ReplaceAll(string(frame), "\r\n", "\n"))
	copies := map[string]string{
		"index.html": cutJS(t, string(index), "const REF_SCHEME=", "\n  return{kind:word?\"lookup\":\"anchor\",word,frag};\n}\n"),
		"frame.js":   cutJS(t, string(frame), "var REF_SCHEME =", "\n\t\treturn { kind: word ? \"lookup\" : \"anchor\", word: word, frag: frag };\n\t}\n"),
	}
	for name, js := range copies {
		t.Run(name, func(t *testing.T) {
			prog := js + "\nconsole.log(JSON.stringify(" + string(in) + ".map(parseRef)));\n"
			out, err := exec.Command(node, "-e", prog).CombinedOutput()
			if err != nil {
				t.Fatalf("node: %v\n%s", err, out)
			}
			var got []*refResult
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("output %q: %v", out, err)
			}
			for i, tt := range tests {
				if !reflect.DeepEqual(got[i], tt.want) {
					t.Errorf("parseRef(%q) = %+v, want %+v", tt.href, got[i], tt.want)
				}
			}
		})
	}
}
