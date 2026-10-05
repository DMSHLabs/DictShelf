// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

import "testing"

func TestParserUpgradeKeepsIndependentTranscriptionRules(t *testing.T) {
	input := `[t]ma3[/t]`
	original, _, err := transformBody(input, "key")
	if err != nil || original != `<span class="wu-ipa">ma3</span>` {
		t.Fatalf("Original: %q (%v)", original, err)
	}
	gd, _, err := transformGDBody(input, "key", nil)
	if err != nil || gd != `<span class="wu-ipa">maĩ</span>` {
		t.Fatalf("GD: %q (%v)", gd, err)
	}
}

func TestParserUpgradeRepairsTitleAcrossUnsortedParts(t *testing.T) {
	input := `удар{[']}е{[/']}ние`
	want := `удар<span class="wu-acc">е</span>ние`
	for name, title := range map[string]titleResult{"Original": transformTitle(input), "GD": gdTitle(input)} {
		if title.Display != want || title.first() != "ударение" {
			t.Errorf("%s: %+v", name, title)
		}
	}
}
