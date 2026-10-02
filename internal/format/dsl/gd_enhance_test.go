// SPDX-License-Identifier: GPL-3.0-or-later
package dsl

import (
	"strings"
	"testing"
)

func TestGDEnhancerCleanup(t *testing.T) {
	for _, c := range []struct{ input, want string }{
		{`<p></p><div>one</div><p> </p>`, `<div>one</div>`},
		{`<span><p></p></span><div>one</div>`, `<div>one</div>`},
		{`<p></p>inline`, `<p></p>inline`},
		{`a<span class="gde">b</span>c<span class="gde">d</span>`, `abcd`},
		{`<span class="gde" title="keep">b</span>`, `<span class="gde" title="keep">b</span>`},
		{`<div><span class="wu-ex">one</span></div>`, `<div class="wu-ex">one</div>`},
		{`<p><b>bold</b></p>`, `<p><b>bold</b></p>`},
		{`<p><span lang="ru">one</span></p>`, `<p lang="ru">one</p>`},
		{`<p lang="en"><span lang="ru">one</span></p>`, `<p lang="en"><span lang="ru">one</span></p>`},
		{`<p><a href="entry://word" title="noun">one</a></p>`, `<p><a href="entry://word" title="noun">one</a></p>`},
		{`<p><span class="wu-c" style="--wd-c:red"><span class="wu-c" style="--wd-c:blue">one</span></span></p>`, `<p class="wu-c" style="--wd-c:red"><span class="wu-c" style="--wd-c:blue">one</span></p>`},
		{`<span><img src="image.png"><p></p></span>`, `<span><img src="image.png"/></span>`},
	} {
		got, err := prepareGDHTML(c.input, GDOptions{Enhance: true})
		if err != nil || got != c.want {
			t.Errorf("%s: got %q (%v); want %q", c.input, got, err, c.want)
		}
	}
}

func TestGDExamplesGenerated(t *testing.T) {
	for _, c := range []struct {
		body string
		mark bool
	}{
		{`[m1][ex]Example.[/ex][/m]`, true},
		{`[m1]▪ [s]one.wav[/s] [ex]Example.[/ex][/m]`, true},
		{`[m1][ref]word[/ref] [ex]Example.[/ex][/m]`, true},
		{`[m1][ex]Example.[/ex] translation[/m]`, false},
		{`[m1]definition [ex]Example.[/ex][/m]`, false},
		{`[m1]definition[/m]`, false},
		{`[m1][*][ex][lang name="English"]Example.[/lang][/ex][/*][/m]`, true},
	} {
		for _, enhance := range []bool{false, true} {
			got, _, err := transformGDWithOptions(c.body, "word", nil, GDOptions{Enhance: enhance, Styles: true})
			if err != nil || strings.Contains(got, "wu-xonly") != c.mark {
				t.Errorf("enhance=%v %s: %s (%v)", enhance, c.body, got, err)
			}
			if strings.Contains(got, "gde-hwbtn") || strings.Contains(got, "<button") || strings.Contains(got, "<script") {
				t.Fatal("article controls added", got)
			}
		}
	}
}

func TestGDEnhancerDisabled(t *testing.T) {
	body := `[m1][ex]Example.[/ex][/m]`
	want, _, _ := transformGDBody(body, "word", nil)
	got, _, err := transformGDWithOptions(body, "word", nil, GDOptions{})
	if err != nil || got != want {
		t.Fatalf("disabled mode: %q %v; want %q", got, err, want)
	}
	path := writeDSL(t, "options.dsl", []byte("#NAME \"Test\"\nword\n\t"+body+"\n"))
	for _, c := range []struct {
		options GDOptions
		styled  bool
	}{{GDOptions{}, false}, {GDOptions{Enhance: true, Styles: true}, true}} {
		r, err := NewGDReaderWithOptions(path, c.options)
		if err != nil {
			t.Fatal(err)
		}
		entry, err := r.Next()
		r.Close()
		if err != nil || strings.Contains(entry.Body, "wu-gd") != c.styled {
			t.Fatal(entry.Body, err)
		}
	}
}
