// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/limits"
)

// The MAX_* keys layer like every other key. A value that is not a positive
// size leaves the default; one outside the key's range is moved to its nearest
// end. Either way the guard stays on and the problem is reported.
func TestLimitKeys(t *testing.T) {
	toml := filepath.Join(t.TempDir(), Name)
	os.WriteFile(toml, []byte(`
MAX_SLOB_BIN = "32MB"
MAX_BGL_BLOCK = "8MB"
MAX_ZIM_CLUSTER = "lots"
MAX_IMPORT_RATIO = "0"
MAX_LEMMA_SIZE = "1e30GB"
MAX_MDICT_BLOCK = "nan"
MAX_STARDICT_INDEX = "1KB"
`), 0o644)
	t.Setenv("MAX_BGL_BLOCK", "2MB") // env beats toml, and is below the floor
	cfg, err := Load(toml, map[string]string{"MAX_UPLOAD_SIZE": "1GB", "MAX_IMPORT_SIZE": "3.5GB"})
	if err != nil {
		t.Fatal(err)
	}
	want := limits.Default
	want.SlobBin, want.BGLBlock, want.UploadSize, want.ImportSize = 32<<20, 8<<20, 1<<30, 7<<29
	want.LemmaSize, want.StarDictIndex = 1<<30, 16<<20
	if cfg.Limits != want {
		t.Errorf("Limits = %+v\nwant     %+v", cfg.Limits, want)
	}
	if cfg.Origin("MAX_BGL_BLOCK") != OriginEnv || cfg.Origin("MAX_UPLOAD_SIZE") != OriginFlag {
		t.Errorf("origins: %q %q", cfg.Origin("MAX_BGL_BLOCK"), cfg.Origin("MAX_UPLOAD_SIZE"))
	}
	var bad []string
	for _, p := range cfg.Problems {
		if strings.HasPrefix(p, "MAX_") {
			bad = append(bad, p)
		}
	}
	all := strings.Join(bad, "\n")
	for _, k := range []string{"MAX_ZIM_CLUSTER", "MAX_IMPORT_RATIO", "MAX_LEMMA_SIZE", "MAX_MDICT_BLOCK", "MAX_STARDICT_INDEX", "MAX_BGL_BLOCK"} {
		if !strings.Contains(all, k+" = ") {
			t.Errorf("no problem reported for %s in %q", k, bad)
		}
	}
	if len(bad) != 6 {
		t.Errorf("%d problems, want 6: %q", len(bad), bad)
	}
}

// Every default sits inside its key's range, and every range is one a value
// can be in: the floor at or below the ceiling.
func TestLimitRanges(t *testing.T) {
	d := limits.Default
	for _, k := range LimitKeys {
		if v := *k.Field(&d); v < k.Min || v > k.Max {
			t.Errorf("%s: default %d outside %d..%d", k.Key, v, k.Min, k.Max)
		}
	}
}

func TestParseSizeSaturates(t *testing.T) {
	for in, want := range map[string]int64{
		"1e30GB": math.MaxInt64, "inf": math.MaxInt64, "nan": 0, "-5MB": 0, "1.5KB": 1536, "64TB": 64 << 40,
	} {
		if got := ParseSize(in); got != want {
			t.Errorf("ParseSize(%q) = %d, want %d", in, got, want)
		}
	}
}

// The template documents every key, each with the default wudict ships, so
// the commented-out line is what leaving it out means.
func TestLimitKeysInTemplate(t *testing.T) {
	d := limits.Default
	for _, k := range LimitKeys {
		def := FormatSize(*k.Field(&d))
		if k.Ratio {
			def = strconv.FormatInt(*k.Field(&d), 10)
		}
		if !knownKeys[k.Key] {
			t.Errorf("%s is not in the template", k.Key)
			continue
		}
		found := false
		for _, line := range strings.Split(configTemplate, "\n") {
			if strings.HasPrefix(strings.TrimLeft(line, "# "), k.Key+" ") {
				found = strings.Contains(line, `"`+def+`"`)
				break
			}
		}
		if !found {
			t.Errorf("the template's %s line does not show the default %q", k.Key, def)
		}
	}
}
