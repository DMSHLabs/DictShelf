// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package mdictwrite writes unencrypted MDict 2.0 MDX and MDD containers.
package mdictwrite

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"hash"
	"hash/adler32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
)

// Entry supplies one record. Size includes the MDX trailing NUL where applicable.
// Open must return exactly Size bytes, and may be called only once.
type Entry struct {
	Key  string
	Size int64
	Open func() (io.ReadCloser, error)
}

type keyBlock struct {
	first, last string
	count       uint64
	data        []byte
	raw         uint64
}
type recordBlock struct{ compressed, raw uint64 }

func be64(w io.Writer, n uint64) error { return binary.Write(w, binary.BigEndian, n) }
func be16(w io.Writer, n uint16) error { return binary.Write(w, binary.BigEndian, n) }
func encodedKey(s string, media bool) []byte {
	if !media {
		return []byte(s)
	}
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2)
	for i, r := range u {
		binary.LittleEndian.PutUint16(b[i*2:], r)
	}
	return b
}

func compressed(raw []byte) ([]byte, error) {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.LittleEndian, uint32(2))
	_ = binary.Write(&b, binary.BigEndian, adler32.Checksum(raw))
	z := zlib.NewWriter(&b)
	if _, err := z.Write(raw); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func header(w io.Writer, title, about string, media bool) error {
	attr := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return strings.ReplaceAll(b.String(), `"`, "&quot;")
	}
	name := "Dictionary"
	extra := ` Encoding="UTF-8" Format="Html" StripKey="Yes"`
	if media {
		name = "Library_Data"
		extra = ""
	}
	s := fmt.Sprintf(`<%s GeneratedByEngineVersion="2.0" RequiredEngineVersion="2.0" Encrypted="No"%s Title="%s" Description="%s"/>`+"\r\n\x00", name, extra, attr(title), attr(about))
	u := utf16.Encode([]rune(s))
	b := make([]byte, len(u)*2)
	for i, r := range u {
		binary.LittleEndian.PutUint16(b[i*2:], r)
	}
	if err := binary.Write(w, binary.BigEndian, uint32(len(b))); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	return binary.Write(w, binary.LittleEndian, adler32.Checksum(b))
}

// Write streams records to an MDict container. It keeps only the key index and
// a bounded record block in memory; large resources span multiple blocks.
func Write(w io.Writer, title, about string, media bool, entries []Entry) error {
	if len(entries) == 0 {
		return fmt.Errorf("cannot export an empty dictionary")
	}
	entries = append([]Entry(nil), entries...)
	sort.SliceStable(entries, func(i, j int) bool { return strings.ToLower(entries[i].Key) < strings.ToLower(entries[j].Key) })
	keys := make([]keyBlock, 0)
	var keyRaw bytes.Buffer
	var keyFirst, keyLast string
	var keyCount uint64
	var total uint64
	for _, e := range entries {
		if e.Key == "" || strings.ContainsRune(e.Key, 0) || e.Size < 0 || e.Open == nil {
			return fmt.Errorf("invalid MDict entry %q", e.Key)
		}
		key := encodedKey(e.Key, media)
		if keyRaw.Len() > 0 && keyRaw.Len()+len(key)+10 > 32<<10 {
			data, err := compressed(keyRaw.Bytes())
			if err != nil {
				return err
			}
			keys = append(keys, keyBlock{keyFirst, keyLast, keyCount, data, uint64(keyRaw.Len())})
			keyRaw.Reset()
			keyCount = 0
		}
		if keyCount == 0 {
			keyFirst = e.Key
		}
		keyLast = e.Key
		keyCount++
		if err := be64(&keyRaw, total); err != nil {
			return err
		}
		keyRaw.Write(key)
		if media {
			keyRaw.Write([]byte{0, 0})
		} else {
			keyRaw.WriteByte(0)
		}
		total += uint64(e.Size)
	}
	data, err := compressed(keyRaw.Bytes())
	if err != nil {
		return err
	}
	keys = append(keys, keyBlock{keyFirst, keyLast, keyCount, data, uint64(keyRaw.Len())})
	var idx bytes.Buffer
	var keyBytes uint64
	for _, b := range keys {
		first, last := encodedKey(b.first, media), encodedKey(b.last, media)
		width := 1
		if media {
			width = 2
		}
		if len(first)/width > 65535 || len(last)/width > 65535 {
			return fmt.Errorf("MDict headword too long")
		}
		_ = be64(&idx, b.count)
		_ = be16(&idx, uint16(len(first)/width))
		idx.Write(first)
		idx.Write(make([]byte, width))
		_ = be16(&idx, uint16(len(last)/width))
		idx.Write(last)
		idx.Write(make([]byte, width))
		_ = be64(&idx, uint64(len(b.data)))
		_ = be64(&idx, b.raw)
		keyBytes += uint64(len(b.data))
	}
	keyIndex, err := compressed(idx.Bytes())
	if err != nil {
		return err
	}
	if err := header(w, title, about, media); err != nil {
		return err
	}
	var pre bytes.Buffer
	for _, n := range []uint64{uint64(len(keys)), uint64(len(entries)), uint64(idx.Len()), uint64(len(keyIndex)), keyBytes} {
		_ = be64(&pre, n)
	}
	if _, err := w.Write(pre.Bytes()); err != nil {
		return err
	}
	if err := binary.Write(w, binary.BigEndian, adler32.Checksum(pre.Bytes())); err != nil {
		return err
	}
	if _, err := w.Write(keyIndex); err != nil {
		return err
	}
	for _, b := range keys {
		if _, err := w.Write(b.data); err != nil {
			return err
		}
	}
	// Record section needs its block index before the data. Spool compressed
	// blocks to a temporary file rather than retaining the dictionary in RAM.
	tempDir := ""
	if file, ok := w.(*os.File); ok {
		tempDir = filepath.Dir(file.Name())
	}
	tmp, err := os.CreateTemp(tempDir, "dictshelf-mdict-*")
	if err != nil {
		return err
	}
	defer func() { tmp.Close(); os.Remove(tmp.Name()) }()
	blocks := make([]recordBlock, 0)
	buf := make([]byte, 1<<20)
	for _, e := range entries {
		r, err := e.Open()
		if err != nil {
			return err
		}
		for remaining, first := e.Size, true; remaining > 0 || first; first = false {
			chunk := min(remaining, int64(256<<20))
			start, err := tmp.Seek(0, io.SeekCurrent)
			if err != nil {
				r.Close()
				return err
			}
			if _, err := tmp.Write(make([]byte, 8)); err != nil {
				r.Close()
				return err
			}
			checksum := adler32.New()
			z := zlib.NewWriter(tmp)
			copied, err := io.CopyBuffer(z, io.TeeReader(io.LimitReader(r, chunk), checksum), buf)
			if err != nil {
				r.Close()
				return fmt.Errorf("read %q: %w", e.Key, err)
			}
			if copied != chunk {
				r.Close()
				return fmt.Errorf("short record %q", e.Key)
			}
			if err := z.Close(); err != nil {
				r.Close()
				return err
			}
			end, err := tmp.Seek(0, io.SeekCurrent)
			if err != nil {
				r.Close()
				return err
			}
			if _, err := tmp.Seek(start, io.SeekStart); err != nil {
				r.Close()
				return err
			}
			if err := writeBlockHeader(tmp, checksum); err != nil {
				r.Close()
				return err
			}
			if _, err := tmp.Seek(end, io.SeekStart); err != nil {
				r.Close()
				return err
			}
			blocks = append(blocks, recordBlock{uint64(end - start), uint64(chunk)})
			remaining -= chunk
		}
		var one [1]byte
		n, readErr := r.Read(one[:])
		closeErr := r.Close()
		if n != 0 || (readErr != nil && readErr != io.EOF) || closeErr != nil {
			return fmt.Errorf("record size mismatch: %q", e.Key)
		}
	}
	var recordBytes uint64
	for _, b := range blocks {
		recordBytes += b.compressed
	}
	for _, n := range []uint64{uint64(len(blocks)), uint64(len(entries)), uint64(len(blocks) * 16), recordBytes} {
		if err := be64(w, n); err != nil {
			return err
		}
	}
	for _, b := range blocks {
		if err := be64(w, b.compressed); err != nil {
			return err
		}
		if err := be64(w, b.raw); err != nil {
			return err
		}
	}
	if _, err := tmp.Seek(0, 0); err != nil {
		return err
	}
	_, err = io.Copy(w, tmp)
	return err
}

func writeBlockHeader(w io.Writer, checksum hash.Hash32) error {
	if err := binary.Write(w, binary.LittleEndian, uint32(2)); err != nil {
		return err
	}
	return binary.Write(w, binary.BigEndian, checksum.Sum32())
}
