// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package slob implements the Aard2 .slob container (direct backend +
// ingest reader). Binary layout ported from pyglossary/pyglossary/slob
// (big-endian throughout):
//
//	header:  magic "!-1SLOB\x1f", uuid[16], enc(u8-str), compression(tiny),
//	         tags u8×{tiny,tiny}, content-types u8×{u16-str},
//	         blob_count u32, store_offset u64, file_size u64
//	refs:    count u32, pos u64×count, items {key u16-str, bin u32,
//	         item u16, fragment tiny}
//	store:   count u32, pos u64×count, items {n u32, ctype_id u8×n,
//	         zlen u32, compressed bin}
//	bin:     pos u32×n, items {len u32, bytes}   (after decompression)
//
// "tiny" strings written as editable (length byte 255) are NUL-truncated.
package slob

import (
	"bytes"
	"cmp"
	"compress/bzip2"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"sync"

	"github.com/ulikunitz/xz/lzma"
)

var magic = []byte("!-1SLOB\x1f")

type ref struct {
	key      string
	bin      uint32
	item     uint16
	fragment string
}

type container struct {
	f            *os.File
	size         int64 // the real file length; every span is checked against it
	uuid         [16]byte
	encoding     string
	compression  string
	tags         map[string]string
	tagOrder     []string // tag keys in file order, first occurrence only
	contentTypes []string
	blobCount    uint32

	// The ref list stays in the file: refCount records, their u64 positions
	// at refPosOff, each relative to refDataOff, the region ending at refEnd
	// (the store offset). locate and walk read them on demand; allRefs
	// decodes them all, once, for the preview lookups that need every key in
	// memory.
	refCount   int
	refPosOff  int64
	refDataOff int64
	refEnd     int64

	refsOnce sync.Once
	refs     []ref
	refsErr  error

	storePos     []uint64 // store item positions
	storeDataOff int64

	mu        sync.Mutex
	binCache  map[uint32][]byte
	binOrder  []uint32
	ctidCache map[uint32][]byte // bin -> content-type ids (headers only)
}

const binCacheSize = 4

func openContainer(path string) (*container, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	c := &container{f: f, binCache: map[uint32][]byte{}}
	if err := c.parse(); err != nil {
		f.Close()
		return nil, fmt.Errorf("slob %s: %w", path, err)
	}
	return c, nil
}

func (c *container) Close() error { return c.f.Close() }

// bufReader tracks absolute position over the file for header parsing.
type bufReader struct {
	data []byte
	pos  int
}

func (b *bufReader) read(n int) ([]byte, error) {
	// pos is derived from file-declared positions (refWindow), so it can
	// arrive negative from a value that overflows int - and a negative pos
	// passes an upper-bound-only check and panics on the slice.
	if b.pos < 0 || n < 0 || b.pos+n > len(b.data) {
		return nil, io.ErrUnexpectedEOF
	}
	out := b.data[b.pos : b.pos+n]
	b.pos += n
	return out, nil
}

func (b *bufReader) u8() (byte, error) {
	d, err := b.read(1)
	if err != nil {
		return 0, err
	}
	return d[0], nil
}

func (b *bufReader) u16() (uint16, error) {
	d, err := b.read(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(d), nil
}

func (b *bufReader) u32() (uint32, error) {
	d, err := b.read(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(d), nil
}

func (b *bufReader) u64() (uint64, error) {
	d, err := b.read(8)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(d), nil
}

// tinyText: u8 length; a length of 255 marks an editable (padded) string
// that is NUL-truncated.
func (b *bufReader) tinyText() (string, error) {
	d, err := b.tinyTextBytes()
	return string(d), err
}

// tinyTextBytes is tinyText without the copy: the bytes alias the buffer.
func (b *bufReader) tinyTextBytes() ([]byte, error) {
	n, err := b.u8()
	if err != nil {
		return nil, err
	}
	d, err := b.read(int(n))
	if err != nil {
		return nil, err
	}
	if n == 255 {
		if i := bytes.IndexByte(d, 0); i >= 0 {
			d = d[:i]
		}
	}
	return d, nil
}

// text: u16 length, NUL-truncated at max length.
func (b *bufReader) text() (string, error) {
	d, err := b.textBytes()
	return string(d), err
}

// textBytes is text without the copy: the bytes alias the buffer.
func (b *bufReader) textBytes() ([]byte, error) {
	n, err := b.u16()
	if err != nil {
		return nil, err
	}
	d, err := b.read(int(n))
	if err != nil {
		return nil, err
	}
	if n == 0xFFFF {
		if i := bytes.IndexByte(d, 0); i >= 0 {
			d = d[:i]
		}
	}
	return d, nil
}

func (c *container) parse() error {
	st, err := c.f.Stat()
	if err != nil {
		return err
	}
	fileSize := st.Size()

	// Header is small; refs region follows it and ends at store_offset.
	// Read a generous fixed prefix first to locate the offsets.
	head := make([]byte, min64(int64(1<<16), fileSize))
	if _, err := io.ReadFull(c.f, head); err != nil {
		return err
	}
	br := &bufReader{data: head}

	m, err := br.read(len(magic))
	if err != nil || !bytes.Equal(m, magic) {
		return fmt.Errorf("bad magic (not a slob file)")
	}
	uuid, err := br.read(16)
	if err != nil {
		return err
	}
	copy(c.uuid[:], uuid)
	if c.encoding, err = br.tinyText(); err != nil {
		return err
	}
	if c.encoding != "utf-8" && c.encoding != "utf8" && c.encoding != "UTF-8" {
		return fmt.Errorf("unsupported encoding %q", c.encoding)
	}
	if c.compression, err = br.tinyText(); err != nil {
		return err
	}
	tagCount, err := br.u8()
	if err != nil {
		return err
	}
	c.tags = make(map[string]string, tagCount)
	for i := 0; i < int(tagCount); i++ {
		k, err := br.tinyText()
		if err != nil {
			return err
		}
		v, err := br.tinyText()
		if err != nil {
			return err
		}
		if _, dup := c.tags[k]; !dup {
			c.tagOrder = append(c.tagOrder, k)
		}
		c.tags[k] = v
	}
	ctCount, err := br.u8()
	if err != nil {
		return err
	}
	for i := 0; i < int(ctCount); i++ {
		ct, err := br.text()
		if err != nil {
			return err
		}
		c.contentTypes = append(c.contentTypes, ct)
	}
	if c.blobCount, err = br.u32(); err != nil {
		return err
	}
	storeOffset, err := br.u64()
	if err != nil {
		return err
	}
	size, err := br.u64()
	if err != nil {
		return err
	}
	if int64(size) != fileSize {
		return fmt.Errorf("file size mismatch: header says %d, file is %d", size, fileSize)
	}
	c.size = fileSize
	refsOffset := int64(br.pos)

	if err := c.parseRefList(refsOffset, int64(storeOffset)); err != nil {
		return err
	}
	return c.parseStoreDir(int64(storeOffset))
}

// checkSpan rejects a span that does not lie inside the file, BEFORE it is
// used as an allocation size. Every length in this format is a u32 or u64 read
// out of the file being parsed, so an allocation sized from one is an
// allocation sized by whoever wrote the file: a store offset of 1<<31 asks for
// 2 GiB and a store offset of 1<<62 asks for more than makeslice will accept.
// Neither failure is a parse error the caller can survive - the first is a
// runtime out-of-memory abort, which no recover converts, and on Android it
// kills the server inside a few-hundred-MB ceiling.
//
// The header's own file_size field is NOT this check: it is compared against
// the real length once (above), and that comparison says nothing about where
// any later offset points. This is the same guard internal/format/zim applies
// to its pointer lists, written the same way on purpose.
func (c *container) checkSpan(off, n int64, what string) error {
	if off < 0 || n < 0 || off > c.size || n > c.size-off {
		return fmt.Errorf("%s out of range (offset %d, %d bytes, file %d)", what, off, n, c.size)
	}
	return nil
}

// parseRefList locates the ref list - count, position table, records - and
// checks that it lies inside the file. The records stay there (locate, walk):
// only the first and the last are decoded now, so a table pointing outside
// its region is refused at open rather than at the first lookup.
func (c *container) parseRefList(refsOffset, storeOffset int64) error {
	if storeOffset <= refsOffset {
		return fmt.Errorf("invalid store offset")
	}
	if err := c.checkSpan(refsOffset, storeOffset-refsOffset, "refs region"); err != nil {
		return err
	}
	region := storeOffset - refsOffset
	if region < 4 {
		return io.ErrUnexpectedEOF
	}
	var cnt [4]byte
	if _, err := c.f.ReadAt(cnt[:], refsOffset); err != nil {
		return fmt.Errorf("reading refs: %w", err)
	}
	count := int64(binary.BigEndian.Uint32(cnt[:]))
	// count is bounded by the region it indexes into, not by the file: each
	// position is 8 bytes of that region.
	if count*8 > region-4 {
		return fmt.Errorf("ref count %d exceeds refs region (%d bytes)", count, region)
	}
	if count > int64(math.MaxInt) {
		return fmt.Errorf("ref count %d exceeds this platform", count)
	}
	c.refCount = int(count)
	c.refPosOff = refsOffset + 4
	c.refDataOff = c.refPosOff + count*8
	c.refEnd = storeOffset
	if c.refCount == 0 {
		return nil
	}
	w := refWindow{c: c, size: pointWindowBytes}
	for _, i := range [2]int{0, c.refCount - 1} {
		var b [8]byte
		if _, err := c.f.ReadAt(b[:], c.refPosOff+int64(i)*8); err != nil {
			return fmt.Errorf("reading refs: %w", err)
		}
		if _, err := w.ref(binary.BigEndian.Uint64(b[:]), false); err != nil {
			return fmt.Errorf("ref %d: %w", i, err)
		}
	}
	return nil
}

// decodeRef reads one ref record: key, bin, item, fragment. Without keys the
// two strings are bounds-checked and skipped, not copied - all a scan for
// locations needs.
func decodeRef(br *bufReader, keys bool) (ref, error) {
	var r ref
	key, err := br.textBytes()
	if err != nil {
		return ref{}, err
	}
	if r.bin, err = br.u32(); err != nil {
		return ref{}, err
	}
	if r.item, err = br.u16(); err != nil {
		return ref{}, err
	}
	frag, err := br.tinyTextBytes()
	if err != nil {
		return ref{}, err
	}
	if keys {
		r.key, r.fragment = string(key), string(frag)
	}
	return r, nil
}

// maxRefBytes is the largest record the format can encode: a u16-length key,
// a u32 bin, a u16 item and a u8-length fragment.
const maxRefBytes = 2 + 0xFFFF + 4 + 2 + 1 + 0xFF

const (
	// scanWindowBytes is what a walk over dense records reads at a time: the
	// whole list in position order is contiguous, so one read covers
	// thousands of records.
	scanWindowBytes = 256 << 10
	// keyWindowBytes is the Reader's window for a blob's keys: their records
	// mostly follow one another blob by blob, as both were written in the
	// same order, and a miss costs one read of this size.
	keyWindowBytes = 16 << 10
	// pointWindowBytes is what a read of one record at a time reads first;
	// a longer record grows the window to maxRefBytes.
	pointWindowBytes = 512
	// posPageLen is how many ref positions are read at a time (4 KiB).
	posPageLen = 512
)

// refWindow reads ref records through one buffered span of the refs region.
// Not safe for concurrent use: each scan or reader owns its own.
type refWindow struct {
	c    *container
	size int64
	buf  []byte
	off  int64 // file offset of buf[0]
}

// ref decodes the record at pos, relative to the record data.
func (w *refWindow) ref(pos uint64, keys bool) (ref, error) {
	c := w.c
	if pos >= uint64(c.refEnd-c.refDataOff) {
		return ref{}, io.ErrUnexpectedEOF
	}
	off := c.refDataOff + int64(pos)
	if r, ok, err := w.decode(off, keys); ok {
		return r, err
	}
	// The window's size first, then the largest record there can be: a
	// window that long, starting at the record, always holds it whole or
	// reaches the region's end.
	for _, n := range [2]int64{w.size, maxRefBytes} {
		if err := w.fill(off, n); err != nil {
			return ref{}, err
		}
		if r, ok, err := w.decode(off, keys); ok {
			return r, err
		}
	}
	return ref{}, io.ErrUnexpectedEOF
}

// decode reads the record at off out of the window. ok is false when the
// window cannot answer - off lies outside it, or the record runs past its end
// while the region goes on - and a refill can.
func (w *refWindow) decode(off int64, keys bool) (ref, bool, error) {
	end := w.off + int64(len(w.buf))
	if off < w.off || off >= end {
		return ref{}, false, nil
	}
	r, err := decodeRef(&bufReader{data: w.buf, pos: int(off - w.off)}, keys)
	if err != nil && end < w.c.refEnd {
		return ref{}, false, nil
	}
	return r, true, err
}

// fill reads up to n bytes of the region from off into the window.
func (w *refWindow) fill(off, n int64) error {
	n = min(n, w.c.refEnd-off)
	if int64(cap(w.buf)) < n {
		w.buf = make([]byte, n)
	}
	w.buf = w.buf[:n]
	if _, err := w.c.f.ReadAt(w.buf, off); err != nil {
		w.buf = w.buf[:0]
		return err
	}
	w.off = off
	return nil
}

// refLoc is one ref by its place in the list (idx) and where its record is
// (pos). blob is filled by the walks that need it: bin<<16 | item, so that
// numeric order is (bin, item) order.
type refLoc struct {
	pos  uint64
	blob uint64
	idx  uint32
}

// locate reads the positions of refs lo..hi-1 and returns them ordered by
// position - the order their records lie in the file. The list is sorted by
// key and its records are not: writers (slob.py, pyglossary) sort the
// position table and leave the records in the order they were added, so a
// walk in list order reads the file at random and one in position order reads
// it front to back. The table is read a page at a time.
func (c *container) locate(lo, hi int) ([]refLoc, error) {
	if lo < 0 || hi > c.refCount || lo > hi {
		return nil, fmt.Errorf("refs %d..%d out of range (%d refs)", lo, hi, c.refCount)
	}
	locs := make([]refLoc, hi-lo)
	raw := make([]byte, posPageLen*8)
	for page := lo; page < hi; page += posPageLen {
		n := min(posPageLen, hi-page)
		if _, err := c.f.ReadAt(raw[:n*8], c.refPosOff+int64(page)*8); err != nil {
			return nil, fmt.Errorf("reading refs: %w", err)
		}
		for k := 0; k < n; k++ {
			locs[page-lo+k] = refLoc{pos: binary.BigEndian.Uint64(raw[k*8:]), idx: uint32(page + k)}
		}
	}
	slices.SortFunc(locs, func(a, b refLoc) int {
		if c := cmp.Compare(a.pos, b.pos); c != 0 {
			return c
		}
		return cmp.Compare(a.idx, b.idx)
	})
	return locs, nil
}

// walk decodes the record of every loc, in the order given, through one
// window: a large one when the locs are a sizeable share of the list - their
// records then sit close together - and a small one for a few scattered
// refs, such as a browse page's.
func (c *container) walk(locs []refLoc, keys bool, fn func(l *refLoc, r ref) error) error {
	w := refWindow{c: c, size: pointWindowBytes}
	if len(locs)*1024 >= c.refCount {
		w.size = scanWindowBytes
	}
	for k := range locs {
		l := &locs[k]
		r, err := w.ref(l.pos, keys)
		if err != nil {
			return fmt.Errorf("ref %d: %w", l.idx, err)
		}
		if err := fn(l, r); err != nil {
			return err
		}
	}
	return nil
}

// allRefs decodes the whole ref list, once, for the preview lookups that need
// every key in memory: the exact and folded maps and the prefix scan. Ingest,
// browsing and media listing never call it - they read refs from the file as
// they go - so opening a slob reads nothing proportional to its size.
//
// The list is wanted whole, so it is read whole: one sequential read of the
// position table and the records, decoded where they lie. The buffer is the
// size of the region and lives for this call; the keys copied out of it are
// what stays.
func (c *container) allRefs() ([]ref, error) {
	c.refsOnce.Do(func() {
		n := c.refEnd - c.refPosOff
		if n > int64(math.MaxInt) {
			c.refsErr = fmt.Errorf("refs region of %d bytes exceeds this platform", n)
			return
		}
		region := make([]byte, n)
		if _, err := c.f.ReadAt(region, c.refPosOff); err != nil {
			c.refsErr = fmt.Errorf("reading refs: %w", err)
			return
		}
		table, data := region[:c.refCount*8], &bufReader{data: region[c.refCount*8:]}
		refs := make([]ref, c.refCount)
		for i := range refs {
			pos := binary.BigEndian.Uint64(table[i*8:])
			if pos >= uint64(len(data.data)) {
				c.refsErr = fmt.Errorf("ref %d: %w", i, io.ErrUnexpectedEOF)
				return
			}
			data.pos = int(pos)
			r, err := decodeRef(data, true)
			if err != nil {
				c.refsErr = fmt.Errorf("ref %d: %w", i, err)
				return
			}
			refs[i] = r
		}
		c.refs = refs
	})
	return c.refs, c.refsErr
}

func (c *container) parseStoreDir(storeOffset int64) error {
	var cnt [4]byte
	if _, err := c.f.ReadAt(cnt[:], storeOffset); err != nil {
		return fmt.Errorf("reading store dir: %w", err)
	}
	count := binary.BigEndian.Uint32(cnt[:])
	if err := c.checkSpan(storeOffset+4, int64(count)*8, "store position table"); err != nil {
		return err
	}
	posBytes := make([]byte, int64(count)*8)
	if _, err := c.f.ReadAt(posBytes, storeOffset+4); err != nil {
		return fmt.Errorf("reading store positions: %w", err)
	}
	c.storePos = make([]uint64, count)
	for i := range c.storePos {
		c.storePos[i] = binary.BigEndian.Uint64(posBytes[i*8:])
	}
	c.storeDataOff = storeOffset + 4 + int64(count)*8
	return nil
}

// getItem returns the content type and bytes of one blob.
func (c *container) getItem(bin uint32, item uint16) (string, []byte, error) {
	if int(bin) >= len(c.storePos) {
		return "", nil, fmt.Errorf("bin %d out of range", bin)
	}
	base := c.storeDataOff + int64(c.storePos[bin])

	var hdr [4]byte
	if _, err := c.f.ReadAt(hdr[:], base); err != nil {
		return "", nil, err
	}
	itemCount := binary.BigEndian.Uint32(hdr[:])
	if uint32(item) >= itemCount {
		return "", nil, fmt.Errorf("item %d out of range (bin has %d)", item, itemCount)
	}
	if err := c.checkSpan(base+4, int64(itemCount), "bin content-type ids"); err != nil {
		return "", nil, err
	}
	ctids := make([]byte, itemCount)
	if _, err := c.f.ReadAt(ctids, base+4); err != nil {
		return "", nil, err
	}
	ctype := ""
	if int(ctids[item]) < len(c.contentTypes) {
		ctype = c.contentTypes[ctids[item]]
	}

	content, err := c.binContent(bin, base+4+int64(itemCount))
	if err != nil {
		return "", nil, err
	}
	// bin layout: u32 positions ×itemCount, then {u32 len, bytes} items
	posOff := int64(item) * 4
	if posOff+4 > int64(len(content)) {
		return "", nil, io.ErrUnexpectedEOF
	}
	itemPos := binary.BigEndian.Uint32(content[posOff:])
	dataOff := int64(itemCount)*4 + int64(itemPos)
	if dataOff+4 > int64(len(content)) {
		return "", nil, io.ErrUnexpectedEOF
	}
	n := binary.BigEndian.Uint32(content[dataOff:])
	if dataOff+4+int64(n) > int64(len(content)) {
		return "", nil, io.ErrUnexpectedEOF
	}
	return ctype, content[dataOff+4 : dataOff+4+int64(n)], nil
}

// itemContentType returns just the content type of one blob, reading
// only the bin header (no decompression); headers are cached per bin.
func (c *container) itemContentType(bin uint32, item uint16) (string, error) {
	c.mu.Lock()
	ctids, ok := c.ctidCache[bin]
	c.mu.Unlock()
	if !ok {
		if int(bin) >= len(c.storePos) {
			return "", fmt.Errorf("bin %d out of range", bin)
		}
		base := c.storeDataOff + int64(c.storePos[bin])
		var hdr [4]byte
		if _, err := c.f.ReadAt(hdr[:], base); err != nil {
			return "", err
		}
		n := binary.BigEndian.Uint32(hdr[:])
		// The same span check the lookup path does (getItem): a bin header
		// naming 4 GiB of content-type ids is a 4 GiB allocation in a media
		// listing, and it cannot wait for an Open-time validation that never
		// ran on this path.
		if err := c.checkSpan(base+4, int64(n), "bin content-type ids"); err != nil {
			return "", err
		}
		ctids = make([]byte, n)
		if _, err := c.f.ReadAt(ctids, base+4); err != nil {
			return "", err
		}
		c.mu.Lock()
		if c.ctidCache == nil {
			c.ctidCache = map[uint32][]byte{}
		}
		c.ctidCache[bin] = ctids
		c.mu.Unlock()
	}
	if int(item) >= len(ctids) || int(ctids[item]) >= len(c.contentTypes) {
		return "", nil
	}
	return c.contentTypes[ctids[item]], nil
}

// binContent returns the decompressed bin, via a small LRU cache.
func (c *container) binContent(bin uint32, zlenOff int64) ([]byte, error) {
	c.mu.Lock()
	if data, ok := c.binCache[bin]; ok {
		c.mu.Unlock()
		return data, nil
	}
	c.mu.Unlock()

	var zl [4]byte
	if _, err := c.f.ReadAt(zl[:], zlenOff); err != nil {
		return nil, err
	}
	zlen := binary.BigEndian.Uint32(zl[:])
	// On the LOOKUP path, unlike the five sites above: a bin that declares a
	// 4 GiB compressed length is a 4 GiB allocation per query, so this one
	// cannot wait for Open-time validation that never ran.
	if err := c.checkSpan(zlenOff+4, int64(zlen), "bin data"); err != nil {
		return nil, err
	}
	compressed := make([]byte, zlen)
	if _, err := c.f.ReadAt(compressed, zlenOff+4); err != nil {
		return nil, err
	}
	data, err := decompress(c.compression, compressed)
	if err != nil {
		return nil, fmt.Errorf("bin %d: %w", bin, err)
	}

	c.mu.Lock()
	c.binCache[bin] = data
	c.binOrder = append(c.binOrder, bin)
	if len(c.binOrder) > binCacheSize {
		delete(c.binCache, c.binOrder[0])
		c.binOrder = c.binOrder[1:]
	}
	c.mu.Unlock()
	return data, nil
}

// maxItemBytes caps one decompressed bin. Slob items are articles and media
// clips - kilobytes to a few megabytes - and every codec below would otherwise
// inflate whatever a corrupt container declares. The same ceiling zim's
// clusters get.
const maxItemBytes = 256 << 20

func readAllBounded(r io.Reader, max int64) ([]byte, error) {
	out, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(out)) > max {
		return nil, fmt.Errorf("decompressed bin exceeds %d bytes", max)
	}
	return out, nil
}

func decompress(name string, data []byte) ([]byte, error) {
	switch name {
	case "":
		return data, nil
	case "zlib":
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return readAllBounded(r, maxItemBytes)
	case "bz2":
		return readAllBounded(bzip2.NewReader(bytes.NewReader(data)), maxItemBytes)
	case "lzma2":
		r, err := lzma.Reader2Config{DictCap: 1 << 25}.NewReader2(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return readAllBounded(r, maxItemBytes)
	default:
		return nil, fmt.Errorf("unsupported compression %q", name)
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
