// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package inflate

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

// The result is the caller's: the next decode, into the same reused buffer,
// leaves it as it was.
func TestIntoReturnsACopy(t *testing.T) {
	first, err := Into(func(dst []byte) ([]byte, error) { return append(dst, "first"...), nil })
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		Into(func(dst []byte) ([]byte, error) { return append(dst, "XXXXXXXX"...), nil })
	}
	if string(first) != "first" {
		t.Fatalf("result changed under a later decode: %q", first)
	}
	empty, err := Into(func(dst []byte) ([]byte, error) { return dst, nil })
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("empty decode = %#v, %v; want a non-nil empty slice", empty, err)
	}
}

func TestIntoConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for g := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			want := bytes.Repeat([]byte{byte('a' + g)}, 1000+g)
			for range 200 {
				got, err := Into(func(dst []byte) ([]byte, error) { return append(dst, want...), nil })
				if err != nil || !bytes.Equal(got, want) {
					t.Errorf("goroutine %d: %d bytes, %v", g, len(got), err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestAppendAllBound(t *testing.T) {
	out, err := AppendAll([]byte("ab"), strings.NewReader("cdef"), 6)
	if err != nil || string(out) != "abcdef" {
		t.Errorf("at the bound = %q, %v", out, err)
	}
	if _, err := AppendAll(nil, strings.NewReader("1234567"), 6); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("past the bound: %v", err)
	}
}
