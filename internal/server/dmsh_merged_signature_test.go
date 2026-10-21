// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"
	"testing"
)

func TestMergedSignatureMatchesIDsWithIndexSettings(t *testing.T) {
	for _, fts := range []bool{false, true} {
		for _, trigram := range []bool{false, true} {
			signature := fmt.Sprintf("%s\nfts=%t\ntrigram=%t\na\x00source-a\x001\nb\x00source-b\x002\n", mergedBrowseSignatureVersion, fts, trigram)
			if !mergedSignatureMatchesIDs(signature, "b,a") {
				t.Fatalf("valid selection rejected: fts=%t trigram=%t", fts, trigram)
			}
			for _, ids := range []string{"", "a", "a,c", "a,a", ",b"} {
				if mergedSignatureMatchesIDs(signature, ids) {
					t.Fatalf("invalid selection accepted: %q", ids)
				}
			}
		}
	}
	for _, metadata := range []string{"", "fts=false\n", "fts=invalid\ntrigram=false\n", "fts=false\ntrigram=invalid\n"} {
		if mergedSignatureMatchesIDs(mergedBrowseSignatureVersion+"\n"+metadata+"a\x00source\x001\n", "a") {
			t.Fatalf("invalid metadata accepted: %q", metadata)
		}
	}
}
