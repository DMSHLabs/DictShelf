// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package htmlref

import "testing"

func TestCanonRef(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		changed  bool
	}{
		{"bword://run", "entry://run", true},
		{"bword:run", "entry://run", true},
		{"BWord://Run#s2", "entry://Run#s2", true},
		{"bword://New York", "entry://New York", true},
		{"bword://@sub", "entry:@sub", true},
		{"bword:@sub#x", "entry:@sub#x", true},
		{"bword://#frag", "entry://#frag", true},
		{"bword:", "entry://", true},
		{"entry://@sub", "entry:@sub", true},
		{"ENTRY://@sub", "entry:@sub", true},
		{"entry://run", "entry://run", false},
		{"entry:@sub", "entry:@sub", false},
		{"entry:run", "entry:run", false},
		{"d:run", "d:run", false},
		{"http://bword.example/", "http://bword.example/", false},
		{"bwordy://x", "bwordy://x", false},
		{"", "", false},
	} {
		got, changed := CanonRef(tc.in)
		if got != tc.want || changed != tc.changed {
			t.Errorf("CanonRef(%q) = %q, %v; want %q, %v", tc.in, got, changed, tc.want, tc.changed)
		}
		if again, ch := CanonRef(got); again != got || ch {
			t.Errorf("CanonRef(%q) not idempotent: %q", got, again)
		}
	}
}

func TestCanonLinks(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"no link", `<p>plain</p>`, `<p>plain</p>`},
		{"prose is not a link", `<p>bword://x</p>`, `<p>bword://x</p>`},
		{"script string untouched", `<script>var u="bword://x"</script>`, `<script>var u="bword://x"</script>`},
		{"anchor", `<a href="bword://run">run</a>`, `<a href="entry://run">run</a>`},
		{"unquoted", `<a href=BWORD:run>run</a>`, `<a href="entry://run">run</a>`},
		{"sub-entry", `<a href="entry://@ex">e</a>`, `<a href="entry:@ex">e</a>`},
		{"others byte-exact", `<a class=x href='entry://a'>a</a><a href="bword:b">b</a>`,
			`<a class=x href='entry://a'>a</a><a href="entry://b">b</a>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanonLinks(tc.in); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}
