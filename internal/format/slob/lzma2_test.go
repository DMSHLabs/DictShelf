// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package slob

import (
	"bytes"
	"math/rand/v2"
	"runtime"
	"strings"
	"testing"

	"github.com/ulikunitz/xz/lzma"
	"github.com/wuweidict/wudict/internal/limits"
)

func lzma2Bin(t *testing.T, in []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := lzma.Writer2Config{DictCap: 8 << 20}.NewWriter2(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(in); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A bin is decoded with a dictionary its size, not the 32 MiB the decoder
// used to allocate for every bin; one whose matches reach further than that
// is decoded again, exactly.
func TestLZMA2Dictionary(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	text := func(n int) []byte {
		words := []string{"casa ", "perro ", "árbol ", "<b>", "</b>", "definición ", "\n"}
		var b bytes.Buffer
		for b.Len() < n {
			b.WriteString(words[rng.IntN(len(words))])
		}
		return b.Bytes()[:n]
	}

	bin := text(530 << 10)
	packed := lzma2Bin(t, bin)
	decompress("lzma2", packed) // warm the reused buffer
	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	out, err := decompress("lzma2", packed)
	runtime.ReadMemStats(&b)
	if err != nil || !bytes.Equal(out, bin) {
		t.Fatalf("530 kB bin: %d bytes, %v", len(out), err)
	}
	if got := b.TotalAlloc - a.TotalAlloc; got > 2*lzma2DictCap+uint64(len(bin)) {
		t.Errorf("530 kB bin allocated %d MB", got>>20)
	}

	// 2.5 MiB of noise, then its first 512 kB again: the repeat is a match
	// 2.5 MiB back, past the first dictionary.
	noise := make([]byte, 5<<19)
	for i := range noise {
		noise[i] = byte(rng.Uint32())
	}
	far := append(append([]byte{}, noise...), noise[:512<<10]...)
	packed = lzma2Bin(t, far)
	if _, err := decodeLZMA2(packed, lzma2DictCap, nil); err == nil {
		t.Fatal("the far match decoded within the first dictionary: the case is not exercised")
	}
	if out, err := decompress("lzma2", packed); err != nil || !bytes.Equal(out, far) {
		t.Fatalf("far match: %d bytes, %v", len(out), err)
	}
}

// The bin bound is MAX_SLOB_BIN as configured, not a constant.
func TestBinLimitIsConfigurable(t *testing.T) {
	t.Cleanup(func() { limits.Set(limits.Default) })
	bin := bytes.Repeat([]byte("casa "), 20_000) // 100 kB
	packed := lzma2Bin(t, bin)
	limits.Set(limits.Limits{SlobBin: 64 << 10})
	if _, err := decompress("lzma2", packed); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("a 100 kB bin under a 64 kB MAX_SLOB_BIN: %v", err)
	}
	limits.Set(limits.Default)
	if out, err := decompress("lzma2", packed); err != nil || !bytes.Equal(out, bin) {
		t.Errorf("under the default: %d bytes, %v", len(out), err)
	}
}
