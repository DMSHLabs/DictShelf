// SPDX-License-Identifier: GPL-3.0-or-later
package dsl

import (
	"encoding/json"
	"os"
	"testing"
)

// TestGDCompareDump is an opt-in adapter, not a compatibility assertion.
// Keeping it in-package avoids exposing parser internals in the application API.
func TestGDCompareDump(t *testing.T) {
	input, output := os.Getenv("WUDICT_DSL_COMPARE_INPUT"), os.Getenv("WUDICT_DSL_COMPARE_OUTPUT")
	if input == "" || output == "" {
		t.Skip("comparison adapter not requested")
	}
	var cases []struct {
		ID       string   `json:"id"`
		Key      string   `json:"key"`
		Headings []string `json:"headings"`
		Body     string   `json:"body"`
	}
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	type node struct {
		Tag      string            `json:"tag,omitempty"`
		Attrs    map[string]string `json:"attrs,omitempty"`
		Text     string            `json:"text,omitempty"`
		Children []node            `json:"children,omitempty"`
	}
	var convert func(*gdNode, bool) node
	convert = func(n *gdNode, ipa bool) node {
		if n.tag == "media" {
			return node{Tag: n.mediaTag, Children: []node{{Text: n.mediaPayload}}}
		}
		v := node{Tag: n.tag, Attrs: n.attrs, Text: n.text}
		if ipa && n.tag == "" {
			v.Text = gdTranscription(v.Text)
		}
		for _, child := range n.children {
			v.Children = append(v.Children, convert(child, ipa || n.tag == "t"))
		}
		return v
	}
	var result []map[string]any
	for _, c := range cases {
		p := gdParser{key: c.Key}
		if err := p.parse(c.Body); err != nil {
			t.Fatalf("%s: %v", c.ID, err)
		}
		var keys []string
		for _, heading := range c.Headings {
			keys = append(keys, gdTitle(expandGDTitleTilde(heading, c.Key)).Keys...)
		}
		rendered, _, err := transformGDBody(c.Body, c.Key, nil)
		if err != nil {
			t.Fatal(err)
		}
		root := map[string]any{"tag": "", "children": convert(&p.root, false).Children}
		result = append(result, map[string]any{"id": c.ID, "keys": keys, "tree": root, "html": rendered})
	}
	data, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, data, 0600); err != nil {
		t.Fatal(err)
	}
}
