// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package zim implements the openZIM container (direct backend + ingest
// reader) - the format behind offline Wiktionary, Wikipedia and devdocs
// dumps. Little-endian throughout:
//
//	header (80 B): magic u32 = 0x044D495A, major u16, minor u16, uuid[16],
//	               entryCount u32, clusterCount u32, pathPtrPos u64,
//	               titlePtrPos u64, clusterPtrPos u64, mimeListPos u64,
//	               mainPage u32, layoutPage u32, checksumPos u64
//	mime list:     NUL-terminated strings; an EMPTY string terminates the list
//	path ptrs:     u64 file offset x entryCount, sorted by (namespace, path)
//	cluster ptrs:  u64 file offset x clusterCount
//	dirent:        mimetype u16, parameterLen u8, namespace u8, revision u32,
//	               then u32 redirectIndex           (mimetype == 0xFFFF)
//	               or   u32 cluster, u32 blob       (otherwise),
//	               then NUL path, NUL title, then parameterLen extra bytes
//	cluster:       info u8 (codec = b&0x0F, extended = b&0x10), then a
//	               compressed stream whose first bytes are the blob offset
//	               table: W-byte offsets (W = 8 when extended, else 4),
//	               count = offset[0]/W - 1, blobs following contiguously
//
// # Why this is written from scratch
//
// Of the four reference implementations available, pyglossary's `zimfile`,
// `zim-cgo` and GoldenDict-ng's `zim.cc` all delegate to libzim and carry no
// binary knowledge; cgo is disqualified here regardless (D4: a tag-less build
// and `-tags purego` must both work). unidict's czim is the only from-scratch
// reader, and its layout - cross-checked against real files - is what this
// follows. Two of its choices are deliberately NOT reproduced: a fixed 256-byte
// buffer per MIME string, which silently truncates and desynchronises the list
// walk, and `lzma_alone_decoder` for codec 4, where real ZIMs write an xz
// stream (magic FD 37 7A 58 5A 00) - hence ulikunitz/xz here, not xz/lzma.
//
// # The property that shapes the design
//
// The path pointer list is sorted in plain BYTE order over (namespace, path) -
// verified across all 181,966 entries of a real wiktionary. So exact and prefix
// lookup are an on-disk binary search of ~21 preads holding NO resident
// headword index, which makes ZIM the only wudict format whose direct
// ("preview", D15) backend is the right permanent answer rather than a
// compromise. See docs/FORMATS.md.
package zim

import (
	"bytes"
	"compress/bzip2"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/inflate"
	"github.com/wuweidict/wudict/internal/limits"
	"github.com/wuweidict/wudict/internal/resource"
)

const (
	headerSize = 80
	zimMagic   = 0x044D495A

	// mimeRedirect marks a dirent that points at another entry instead of at
	// a blob. 0xFFFE/0xFFFD are the deprecated linktarget/deleted markers -
	// they carry no content and are skipped everywhere.
	mimeRedirect   = 0xFFFF
	mimeLinkTarget = 0xFFFE
	mimeDeleted    = 0xFFFD

	// maxDirentSpan bounds one dirent read. Paths are normally short, but a
	// zimit web capture keys entries by full captured URL and reaches a few
	// kilobytes, so the window grows rather than truncating (czim's bug, in
	// the other list).
	maxDirentSpan = 1 << 16

	// maxMimeListSpan bounds the MIME string region.
	maxMimeListSpan = 1 << 20

	// maxTableBytes bounds a stored cluster's blob offset table, which is read
	// whole to be checked: 4 million 4-byte offsets, far beyond any writer.
	maxTableBytes = 16 << 20

	// xzDictProp is the LZMA2 dictionary an xz cluster is decoded with:
	// property 20, 2 << (20/2 + 11) = 4 MiB (see xzWithDict).
	xzDictProp = 20
	xzDictCap  = 4 << 20

	// clusterCacheBytes is the decompressed-cluster budget. A cluster larger
	// than the whole budget is still cached (alone), because evicting the
	// entry just inserted would make every blob read decompress again.
	clusterCacheBytes = 16 << 20

	// ptrPageLen is how many path pointers a walk reads at a time (4 KiB).
	// The list is 8 bytes per entry - 160 MB for a 20M-entry Wikipedia - so it
	// is read on demand rather than held resident: a binary search probe reads
	// its one pointer (ptrAt), a walk in list order one page per 512 entries
	// (ptrWalk).
	ptrPageLen = 512
)

// dirent is one directory entry: an article, a resource, or a redirect.
// maxCluster bounds one DECOMPRESSED cluster (MAX_ZIM_CLUSTER). Every codec
// here can be made to expand a bounded input without limit, and a cluster's
// compressed extent is attacker-controlled in a downloaded file.
func maxCluster() int64 { return limits.Get().ZIMCluster }

type dirent struct {
	mimetype  uint16
	namespace byte
	revision  uint32
	redirect  uint32 // valid when isRedirect
	cluster   uint32
	blob      uint32
	path      string
	title     string
}

func (d dirent) isRedirect() bool { return d.mimetype == mimeRedirect }

// headword is the title when the file records one, else the path. An empty
// title means "same as the path" and is how ZIM avoids storing it twice - so
// the path is used VERBATIM, never with '_' turned back into a space, which
// would invent a headword the file does not contain.
func (d dirent) headword() string {
	if d.title != "" {
		return d.title
	}
	return d.path
}

// key is the sort key of the path pointer list: namespace byte then path
// bytes, compared as raw bytes.
func (d dirent) key() string { return string([]byte{d.namespace}) + d.path }

func searchKey(ns byte, path string) string { return string([]byte{ns}) + path }

type clusterData struct {
	data     []byte // the decompressed body; nil when direct
	extended bool
	blobs    uint64 // offset-table entries minus one; set and checked at load when not direct

	// An uncompressed cluster is never loaded: its blobs are read in place,
	// so serving one image or video out of a multi-megabyte media cluster
	// costs that blob, and a large blob can be streamed (contentReader).
	direct bool
	body   int64  // file offset of the body, just past the info byte
	n      uint64 // body length
}

func (cd *clusterData) size() uint64 {
	if cd.direct {
		return cd.n
	}
	return uint64(len(cd.data))
}

type container struct {
	f    source
	path string
	size int64

	major, minor uint16
	entryCount   uint32
	clusterCount uint32
	mainPage     uint32

	mimes    []string
	htmlMIME map[uint16]bool

	pathPtrPos  int64    // entryCount u64 file offsets, sorted by (namespace, path)
	titlePtrPos int64    // entryCount u32 entry indices, sorted by (namespace, title); 0 when absent (see titles)
	clusterPtr  []uint64 // clusterCount+1: the tail is synthesised (see parse)

	zr *zstd.Decoder

	mu         sync.Mutex
	cache      map[uint32]*clusterData
	order      []uint32
	cacheBytes int64
}

// source is the archive's bytes: one file, or the parts of a split archive
// read as one (joined).
type source interface {
	io.ReaderAt
	io.Closer
}

func openContainer(path string) (*container, error) {
	paths := dict.ZIMParts(path)
	if paths == nil {
		paths = []string{path}
	}
	f, size, err := openJoined(paths)
	if err != nil {
		return nil, err
	}
	c := &container{f: f, path: path, size: size, cache: map[uint32]*clusterData{}}
	if err := c.parse(); err != nil {
		f.Close()
		return nil, fmt.Errorf("zim %s: %w", path, err)
	}
	return c, nil
}

func (c *container) Close() error {
	// zstd() builds and hands out c.zr under c.mu, so Close takes it too: the
	// registry closes superseded backends after a grace period while a decode
	// may still be starting one. A decode that grabbed the decoder just before
	// this line degrades to an error from that one cluster; what is gone is
	// the unlocked mutation of the field itself.
	c.mu.Lock()
	zr := c.zr
	c.zr = nil
	c.mu.Unlock()
	if zr != nil {
		zr.Close()
	}
	return c.f.Close()
}

func (c *container) parse() error {
	if c.size < headerSize {
		return fmt.Errorf("file shorter than a ZIM header")
	}
	var h [headerSize]byte
	if _, err := c.f.ReadAt(h[:], 0); err != nil {
		return err
	}
	if binary.LittleEndian.Uint32(h[0:]) != zimMagic {
		return fmt.Errorf("bad magic (not a ZIM file)")
	}
	c.major = binary.LittleEndian.Uint16(h[4:])
	c.minor = binary.LittleEndian.Uint16(h[6:])
	if c.major < 5 || c.major > 6 {
		return fmt.Errorf("unsupported ZIM version %d.%d", c.major, c.minor)
	}
	c.entryCount = binary.LittleEndian.Uint32(h[24:])
	c.clusterCount = binary.LittleEndian.Uint32(h[28:])
	pathPtrPos := binary.LittleEndian.Uint64(h[32:])
	titlePtrPos := binary.LittleEndian.Uint64(h[40:])
	clusterPtrPos := binary.LittleEndian.Uint64(h[48:])
	mimeListPos := binary.LittleEndian.Uint64(h[56:])
	c.mainPage = binary.LittleEndian.Uint32(h[64:])
	checksumPos := binary.LittleEndian.Uint64(h[72:])

	if c.entryCount == 0 {
		return fmt.Errorf("empty ZIM (no entries)")
	}
	if err := c.checkSpan(pathPtrPos, uint64(c.entryCount)*8, "path pointer list"); err != nil {
		return err
	}
	if err := c.checkSpan(clusterPtrPos, uint64(c.clusterCount)*8, "cluster pointer list"); err != nil {
		return err
	}
	if err := c.checkSpan(mimeListPos, 1, "MIME list"); err != nil {
		return err
	}

	if err := c.parseMIMEList(mimeListPos, pathPtrPos); err != nil {
		return err
	}

	c.pathPtrPos = int64(pathPtrPos)
	// ZIM 6.3 writes no title pointer list (all ones). One that does not fit
	// in the file is left unused rather than refusing the file: lookup by
	// path does not need it.
	if titlePtrPos != ^uint64(0) && c.checkSpan(titlePtrPos, uint64(c.entryCount)*4, "title pointer list") == nil {
		c.titlePtrPos = int64(titlePtrPos)
	}
	// The list itself is read on demand (ptrAt); its first and last pointers
	// are checked here, so a file cut short inside the entry table is refused
	// at open as before.
	for _, i := range []int{0, int(c.entryCount) - 1} {
		if _, err := c.ptrAt(i); err != nil {
			return fmt.Errorf("reading path pointer list: %w", err)
		}
	}

	// The format records no cluster SIZE, only a start offset; a cluster ends
	// where the next one begins. The last one therefore needs a synthetic
	// bound, and the checksum block is what follows it. A file whose header
	// checksum offset is missing or nonsense falls back to the file end,
	// which over-reads by 16 bytes at worst and never truncates a blob.
	end := checksumPos
	if end == 0 || end > uint64(c.size) {
		end = uint64(c.size)
	}
	c.clusterPtr = make([]uint64, c.clusterCount+1)
	if c.clusterCount > 0 {
		if err := c.readU64s(c.clusterPtr[:c.clusterCount], int64(clusterPtrPos)); err != nil {
			return fmt.Errorf("reading cluster pointer list: %w", err)
		}
	}
	c.clusterPtr[c.clusterCount] = end
	for i := 0; i < int(c.clusterCount); i++ {
		if c.clusterPtr[i] < headerSize || c.clusterPtr[i] >= uint64(c.size) {
			return fmt.Errorf("cluster pointer %d out of range (%d)", i, c.clusterPtr[i])
		}
	}
	return nil
}

func (c *container) checkSpan(off, n uint64, what string) error {
	if off < headerSize || off > uint64(c.size) || n > uint64(c.size)-off {
		return fmt.Errorf("%s out of range (offset %d, %d bytes, file %d)", what, off, n, c.size)
	}
	return nil
}

func (c *container) readU64s(dst []uint64, off int64) error {
	buf := make([]byte, len(dst)*8)
	if _, err := c.f.ReadAt(buf, off); err != nil {
		return err
	}
	for i := range dst {
		dst[i] = binary.LittleEndian.Uint64(buf[i*8:])
	}
	return nil
}

// entries is the length of the path pointer list.
func (c *container) entries() int { return int(c.entryCount) }

// ptrAt is the i-th path pointer, read on its own: the probes of a binary
// search land a page apart or more, so a page read for one would serve no
// other. ReadAt is safe for concurrent use, so concurrent lookups never wait
// on one another.
func (c *container) ptrAt(i int) (uint64, error) {
	if i < 0 || i >= c.entries() {
		return 0, fmt.Errorf("entry %d out of range (%d entries)", i, c.entries())
	}
	var b [8]byte
	if _, err := c.f.ReadAt(b[:], c.pathPtrPos+int64(i)*8); err != nil {
		return 0, err
	}
	return c.checkPtr(i, binary.LittleEndian.Uint64(b[:]))
}

func (c *container) checkPtr(i int, off uint64) (uint64, error) {
	if off < headerSize || off >= uint64(c.size) {
		return 0, fmt.Errorf("path pointer %d out of range (%d)", i, off)
	}
	return off, nil
}

// ptrWalk reads the path pointer list a page at a time, for a walk in list
// order: one read per ptrPageLen entries. Each walk owns its own.
type ptrWalk struct {
	c    *container
	base int // index of page[0]; -1 before the first read
	page [ptrPageLen]uint64
}

func (c *container) walk() *ptrWalk { return &ptrWalk{c: c, base: -1} }

// entry decodes the i-th entry, its pointer from the walk's page.
func (w *ptrWalk) entry(i int) (dirent, error) {
	c := w.c
	if i < 0 || i >= c.entries() {
		return dirent{}, fmt.Errorf("entry %d out of range (%d entries)", i, c.entries())
	}
	if base := i - i%ptrPageLen; base != w.base {
		var buf [ptrPageLen * 8]byte
		n := min(ptrPageLen, c.entries()-base)
		if _, err := c.f.ReadAt(buf[:n*8], c.pathPtrPos+int64(base)*8); err != nil {
			w.base = -1
			return dirent{}, err
		}
		for k := 0; k < n; k++ {
			w.page[k] = binary.LittleEndian.Uint64(buf[k*8:])
		}
		w.base = base
	}
	off, err := c.checkPtr(i, w.page[i-w.base])
	if err != nil {
		return dirent{}, err
	}
	return c.direntAt(int64(off))
}

// parseMIMEList reads the whole string region at once and splits it, rather
// than walking it one fixed-size buffer at a time: a string longer than the
// buffer would truncate, and every following offset would then be wrong.
//
// The index of "text/html" is per-file (6, 7, 12 and 15 across one real
// corpus) and is resolved by STRING, never hardcoded. A parameterised type
// ("text/html; charset=iso-8859-1", which real wiktionaries contain) is
// matched on its type/subtype alone.
func (c *container) parseMIMEList(start, next uint64) error {
	end := next
	if end <= start || end > uint64(c.size) {
		end = uint64(c.size)
	}
	if end-start > maxMimeListSpan {
		end = start + maxMimeListSpan
	}
	buf := make([]byte, end-start)
	n, err := c.f.ReadAt(buf, int64(start))
	if n == 0 && err != nil {
		return fmt.Errorf("reading MIME list: %w", err)
	}
	buf = buf[:n]

	c.htmlMIME = map[uint16]bool{}
	for pos := 0; pos < len(buf); {
		i := bytes.IndexByte(buf[pos:], 0)
		if i < 0 {
			return fmt.Errorf("unterminated MIME list")
		}
		if i == 0 {
			break // the empty string terminates the list
		}
		s := string(buf[pos : pos+i])
		if !plausibleMIME(s) {
			return fmt.Errorf("MIME list entry %d is not a MIME type: %.40q", len(c.mimes), s)
		}
		if isHTMLMIME(s) {
			c.htmlMIME[uint16(len(c.mimes))] = true
		}
		c.mimes = append(c.mimes, s)
		pos += i + 1
	}
	if len(c.mimes) == 0 {
		return fmt.Errorf("empty MIME list")
	}
	return nil
}

// plausibleMIME accepts a type/subtype in printable ASCII, parameters
// included. Without it, a MIME list overwritten by other bytes still splits at
// its NULs and opens as a dictionary with no articles; with it, the file is
// refused at open (zim-testing-suite invalid.bad_mimetype_list).
func plausibleMIME(s string) bool {
	if strings.IndexByte(s, '/') <= 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7E {
			return false
		}
	}
	return true
}

func mimeBase(s string) string {
	if i := strings.IndexByte(s, ';'); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func isHTMLMIME(s string) bool { return mimeBase(s) == "text/html" }

func (c *container) mimeOf(d dirent) string {
	if d.isRedirect() || int(d.mimetype) >= len(c.mimes) {
		return ""
	}
	return c.mimes[d.mimetype]
}

// contentNS is the namespace articles live in. ZIM 6.1 replaced the
// per-kind namespaces (A articles, I images, - layout) with a single content
// namespace C; the metadata namespace M is unchanged across both. This one
// function is the whole compatibility story, and every content lookup routes
// through it.
func (c *container) contentNS() byte {
	if c.major > 6 || (c.major == 6 && c.minor >= 1) {
		return 'C'
	}
	return 'A'
}

func (c *container) newNamespaces() bool { return c.contentNS() == 'C' }

// direntAt decodes the entry at a file offset, growing its read window until
// both NUL-terminated strings are inside it.
func (c *container) direntAt(off int64) (dirent, error) {
	if off < headerSize || off >= c.size {
		return dirent{}, fmt.Errorf("dirent offset %d out of range", off)
	}
	var stack [512]byte
	buf := stack[:]
	for {
		if int64(len(buf)) > c.size-off {
			buf = buf[:c.size-off]
		}
		n, err := c.f.ReadAt(buf, off)
		if n < 12 {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return dirent{}, err
		}
		d, ok := parseDirent(buf[:n])
		if ok {
			return d, nil
		}
		if int64(n) >= c.size-off {
			return dirent{}, fmt.Errorf("truncated dirent at %d", off)
		}
		if len(buf) >= maxDirentSpan {
			return dirent{}, fmt.Errorf("dirent at %d exceeds %d bytes", off, maxDirentSpan)
		}
		buf = make([]byte, min(len(buf)*8, maxDirentSpan))
	}
}

// parseDirent decodes one entry from a window. ok=false means the window ends
// before both strings do and must be grown - it is never an error by itself.
func parseDirent(b []byte) (dirent, bool) {
	if len(b) < 12 {
		return dirent{}, false
	}
	var d dirent
	d.mimetype = binary.LittleEndian.Uint16(b[0:])
	paramLen := int(b[2])
	d.namespace = b[3]
	d.revision = binary.LittleEndian.Uint32(b[4:])
	off := 16
	if d.isRedirect() {
		d.redirect = binary.LittleEndian.Uint32(b[8:])
		off = 12
	} else {
		if len(b) < 16 {
			return dirent{}, false
		}
		d.cluster = binary.LittleEndian.Uint32(b[8:])
		d.blob = binary.LittleEndian.Uint32(b[12:])
	}
	if off > len(b) {
		return dirent{}, false
	}
	i := bytes.IndexByte(b[off:], 0)
	if i < 0 {
		return dirent{}, false
	}
	d.path = string(b[off : off+i])
	rest := b[off+i+1:]
	j := bytes.IndexByte(rest, 0)
	if j < 0 {
		return dirent{}, false
	}
	d.title = string(rest[:j])
	// The extra parameter bytes are unused by every writer in the wild, but
	// they must be inside the window for the entry to be complete.
	if len(rest)-(j+1) < paramLen {
		return dirent{}, false
	}
	return d, true
}

// entry decodes the i-th entry of the path pointer list.
func (c *container) entry(i int) (dirent, error) {
	off, err := c.ptrAt(i)
	if err != nil {
		return dirent{}, err
	}
	return c.direntAt(int64(off))
}

// resolve follows a redirect chain to the entry that actually holds content.
// Real files contain cycles, so the walk is depth-bounded rather than trusting
// termination.
func (c *container) resolve(i int) (int, dirent, error) {
	for hop := 0; hop < 8; hop++ {
		d, err := c.entry(i)
		if err != nil {
			return 0, dirent{}, err
		}
		if !d.isRedirect() {
			return i, d, nil
		}
		next := int(d.redirect)
		if next == i || next < 0 || next >= c.entries() {
			return 0, dirent{}, fmt.Errorf("entry %d: bad redirect to %d", i, next)
		}
		i = next
	}
	return 0, dirent{}, fmt.Errorf("redirect chain too long")
}

// cluster returns one decompressed cluster body (everything after the info
// byte), through a byte-bounded LRU. An uncompressed cluster comes back
// direct: located, not read, and not cached.
func (c *container) cluster(i uint32) (*clusterData, error) {
	c.mu.Lock()
	if cd, ok := c.cache[i]; ok {
		c.touch(i)
		c.mu.Unlock()
		return cd, nil
	}
	c.mu.Unlock()

	if i >= c.clusterCount {
		return nil, fmt.Errorf("cluster %d out of range (%d clusters)", i, c.clusterCount)
	}
	start, end := c.clusterPtr[i], c.clusterPtr[i+1]
	if end <= start || end > uint64(c.size) {
		return nil, fmt.Errorf("cluster %d: bad extent %d..%d", i, start, end)
	}
	var info [1]byte
	if _, err := c.f.ReadAt(info[:], int64(start)); err != nil {
		return nil, fmt.Errorf("cluster %d: %w", i, err)
	}
	extended, codec := info[0]&0x10 != 0, info[0]&0x0F
	if codec <= 1 { // 0 and 1 both mean "stored"
		return &clusterData{extended: extended, direct: true, body: int64(start) + 1, n: end - start - 1}, nil
	}
	// No codec here inflates a stream past its input by more than a sliver,
	// so a compressed extent beyond twice the decompressed bound is a bad
	// pointer - refused before it becomes an allocation of that size.
	if end-start-1 > 2*uint64(maxCluster()) {
		return nil, fmt.Errorf("cluster %d: %d compressed bytes, more than twice the MAX_ZIM_CLUSTER limit of %s", i, end-start-1, limits.Format(maxCluster()))
	}
	raw := make([]byte, end-start-1)
	if _, err := c.f.ReadAt(raw, int64(start)+1); err != nil {
		return nil, fmt.Errorf("cluster %d: %w", i, err)
	}
	data, err := c.decompress(codec, raw)
	if errors.Is(err, inflate.ErrTooLarge) || errors.Is(err, zstd.ErrDecoderSizeExceeded) || errors.Is(err, zstd.ErrWindowSizeExceeded) {
		err = limits.Exceeded("the unpacked cluster", "MAX_ZIM_CLUSTER", maxCluster())
	}
	if err != nil {
		return nil, fmt.Errorf("cluster %d: %w", i, err)
	}
	cd := &clusterData{data: data, extended: extended}
	if cd.blobs, err = checkTable(data, offsetWidth(extended), uint64(len(data))); err != nil {
		return nil, fmt.Errorf("cluster %d: %w", i, err)
	}

	c.mu.Lock()
	if _, ok := c.cache[i]; !ok {
		c.cache[i] = cd
		c.order = append(c.order, i)
		c.cacheBytes += int64(len(cd.data))
		// Evict oldest first, but never the entry just inserted: one cluster
		// can exceed the whole budget, and dropping it would make every blob
		// read of that cluster decompress it again.
		for c.cacheBytes > clusterCacheBytes && len(c.order) > 1 {
			old := c.order[0]
			c.order = c.order[1:]
			if prev, ok := c.cache[old]; ok {
				c.cacheBytes -= int64(len(prev.data))
				delete(c.cache, old)
			}
		}
	}
	c.mu.Unlock()
	return cd, nil
}

// touch moves i to the back of the LRU order. Called under c.mu.
func (c *container) touch(i uint32) {
	for k, v := range c.order {
		if v == i {
			c.order = append(c.order[:k], c.order[k+1:]...)
			c.order = append(c.order, i)
			return
		}
	}
}

// decompress decodes one cluster body into a reused buffer (internal/inflate:
// libzim's zstd frames carry no content size, and DecodeAll without a
// destination allocates 4.6x the 2 MB it produces).
func (c *container) decompress(codec byte, data []byte) ([]byte, error) {
	return inflate.Into(func(dst []byte) ([]byte, error) { return c.decode(codec, data, dst) })
}

// decode appends one decompressed cluster body to dst.
func (c *container) decode(codec byte, data, dst []byte) ([]byte, error) {
	switch codec {
	case 2:
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return dst, err
		}
		defer r.Close()
		return inflate.AppendAll(dst, r, maxCluster())
	case 3:
		return inflate.AppendAll(dst, bzip2.NewReader(bytes.NewReader(data)), maxCluster())
	case 4:
		return decodeXZ(data, dst)
	case 5:
		zr, err := c.zstd()
		if err != nil {
			return dst, err
		}
		return zr.DecodeAll(data, dst)
	default:
		return dst, fmt.Errorf("unsupported compression codec %d", codec)
	}
}

// decodeXZ decodes an xz cluster - xz, NOT LZMA_Alone: real clusters carry the
// xz stream magic FD 37 7A 58 5A 00, where czim uses lzma_alone_decoder.
//
// The decoder allocates the larger of its own dictionary capacity and the one
// the stream declares, and pre-2020 ZIMs declare xz -9's 64 MiB: 64 MiB
// allocated and zeroed per cluster of 1-2 MB. A match reaches back at most
// over the data already decoded, so a 4 MiB dictionary decodes any cluster up
// to 4 MiB exactly. A stream that reaches further fails - the dictionary
// rejects the distance, and the stream's check catches anything else - and is
// decoded again with the dictionary it declares.
func decodeXZ(data, dst []byte) ([]byte, error) {
	if small, ok := xzWithDict(data); ok {
		r, err := xz.ReaderConfig{DictCap: xzDictCap}.NewReader(bytes.NewReader(small))
		if err == nil {
			out, err := inflate.AppendAll(dst, r, maxCluster())
			if err == nil {
				return out, nil
			}
			dst = out[:0]
		}
	}
	r, err := xz.NewReader(bytes.NewReader(data))
	if err != nil {
		return dst, err
	}
	return inflate.AppendAll(dst, r, maxCluster())
}

// xzWithDict returns a copy of an xz stream whose first block declares the
// xzDictProp dictionary, or ok=false when it declares no more than that or is
// not the block libzim writes: a header of one LZMA2 filter (ID 0x21, one
// property byte, the dictionary size) and no size fields. The block header's
// CRC32 is checked before and recomputed after the change.
func xzWithDict(data []byte) ([]byte, bool) {
	const streamHeader = 12
	if len(data) <= streamHeader || data[streamHeader] == 0 {
		return nil, false
	}
	hlen := (int(data[streamHeader]) + 1) * 4
	if len(data) < streamHeader+hlen {
		return nil, false
	}
	h := data[streamHeader : streamHeader+hlen]
	if h[1] != 0 || h[2] != 0x21 || h[3] != 1 || h[4] > 40 || h[4] <= xzDictProp {
		return nil, false
	}
	if crc32.ChecksumIEEE(h[:hlen-4]) != binary.LittleEndian.Uint32(h[hlen-4:]) {
		return nil, false
	}
	out := bytes.Clone(data)
	h = out[streamHeader : streamHeader+hlen]
	h[4] = xzDictProp
	binary.LittleEndian.PutUint32(h[hlen-4:], crc32.ChecksumIEEE(h[:hlen-4]))
	return out, true
}

// zstd builds the shared decoder on first use. DecodeAll is safe for
// concurrent callers, so one decoder serves the whole container; it is built
// lazily because a file with no zstd cluster should not pay for it.
func (c *container) zstd() (*zstd.Decoder, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.zr != nil {
		return c.zr, nil
	}
	zr, err := zstd.NewReader(nil,
		zstd.WithDecoderConcurrency(1),
		zstd.WithDecoderMaxMemory(uint64(maxCluster())))
	if err != nil {
		return nil, err
	}
	c.zr = zr
	return zr, nil
}

// offsetWidth is the size of one blob offset: 8 bytes in an extended cluster.
func offsetWidth(extended bool) uint64 {
	if extended {
		return 8
	}
	return 4
}

// checkTable checks a cluster's whole blob offset table and returns its blob
// count. The table lives INSIDE the (compressed) body: w-byte offsets, and
// offset[0] is both the first blob's position and the table's own length, so
// it is what says how many blobs there are. Every offset must lie past the
// table, inside the body, and no lower than the one before it.
//
// Checking offset[0] alone is not enough: a header that overstates it still
// names a table inside the body, and the first blob is then served from the
// middle of itself, short and without an error (zim-testing-suite
// invalid.too_large_offset_of_first_blob_in_cluster, 60 bytes into a PNG).
// Read as a table, the overstated length takes in blob bytes as offsets,
// which fail the bounds.
func checkTable(b []byte, w, size uint64) (uint64, error) {
	at := func(k uint64) uint64 {
		if w == 8 {
			return binary.LittleEndian.Uint64(b[k*8:])
		}
		return uint64(binary.LittleEndian.Uint32(b[k*4:]))
	}
	if uint64(len(b)) < w {
		return 0, fmt.Errorf("no offset table")
	}
	first := at(0)
	if first < w || first%w != 0 || first > size || first > uint64(len(b)) {
		return 0, fmt.Errorf("bad offset table header %d", first)
	}
	n := first / w
	prev := first
	for k := uint64(1); k < n; k++ {
		off := at(k)
		if off < prev || off > size {
			return 0, fmt.Errorf("bad offset table entry %d: %d after %d, body %d", k, off, prev, size)
		}
		prev = off
	}
	return n - 1, nil
}

// blobSpan locates one blob inside a cluster: [lo, hi) within the body. A
// stored cluster is never loaded, so its table is read and checked here, in
// one read; a compressed one was checked when it was decompressed.
func (c *container) blobSpan(cl, bl uint32) (cd *clusterData, lo, hi uint64, err error) {
	if cd, err = c.cluster(cl); err != nil {
		return nil, 0, 0, err
	}
	w := offsetWidth(cd.extended)
	table, count := cd.data, cd.blobs
	if cd.direct {
		var head [8]byte
		if cd.n < w {
			return nil, 0, 0, fmt.Errorf("cluster %d: no offset table", cl)
		}
		if _, err := c.f.ReadAt(head[:w], cd.body); err != nil {
			return nil, 0, 0, fmt.Errorf("cluster %d: %w", cl, err)
		}
		first := uint64(binary.LittleEndian.Uint32(head[:4]))
		if w == 8 {
			first = binary.LittleEndian.Uint64(head[:8])
		}
		if first > cd.n || first > maxTableBytes {
			return nil, 0, 0, fmt.Errorf("cluster %d: bad offset table header %d", cl, first)
		}
		table = make([]byte, max(first, w))
		if _, err := c.f.ReadAt(table, cd.body); err != nil {
			return nil, 0, 0, fmt.Errorf("cluster %d: %w", cl, err)
		}
		if count, err = checkTable(table, w, cd.n); err != nil {
			return nil, 0, 0, fmt.Errorf("cluster %d: %w", cl, err)
		}
	}
	if uint64(bl) >= count {
		return nil, 0, 0, fmt.Errorf("cluster %d: blob %d out of range (%d blobs)", cl, bl, count)
	}
	if w == 8 {
		lo, hi = binary.LittleEndian.Uint64(table[uint64(bl)*8:]), binary.LittleEndian.Uint64(table[uint64(bl+1)*8:])
	} else {
		lo, hi = uint64(binary.LittleEndian.Uint32(table[bl*4:])), uint64(binary.LittleEndian.Uint32(table[(bl+1)*4:]))
	}
	return cd, lo, hi, nil
}

// blob returns the bytes of one blob inside a cluster.
func (c *container) blob(cl, bl uint32) ([]byte, error) {
	cd, lo, hi, err := c.blobSpan(cl, bl)
	if err != nil {
		return nil, err
	}
	if !cd.direct {
		return cd.data[lo:hi], nil
	}
	if hi-lo > uint64(maxCluster()) {
		return nil, fmt.Errorf("cluster %d blob %d: %w", cl, bl, limits.Exceeded(fmt.Sprintf("%d bytes", hi-lo), "MAX_ZIM_CLUSTER", maxCluster()))
	}
	out := make([]byte, hi-lo)
	if _, err := c.f.ReadAt(out, cd.body+int64(lo)); err != nil {
		return nil, fmt.Errorf("cluster %d blob %d: %w", cl, bl, err)
	}
	return out, nil
}

// content returns the bytes of a non-redirect entry.
func (c *container) content(d dirent) ([]byte, error) {
	if d.isRedirect() {
		return nil, fmt.Errorf("entry is a redirect")
	}
	return c.blob(d.cluster, d.blob)
}

// contentReader is content as a seekable stream: a blob of a stored cluster
// is a window onto the file, so a video is served by range and never read
// whole into memory.
func (c *container) contentReader(d dirent) (io.ReadCloser, error) {
	if d.isRedirect() {
		return nil, fmt.Errorf("entry is a redirect")
	}
	cd, lo, hi, err := c.blobSpan(d.cluster, d.blob)
	if err != nil {
		return nil, err
	}
	if cd.direct {
		return sectionCloser{io.NewSectionReader(c.f, cd.body+int64(lo), int64(hi-lo))}, nil
	}
	return resource.Bytes(cd.data[lo:hi]), nil
}

type sectionCloser struct{ *io.SectionReader }

func (sectionCloser) Close() error { return nil }
