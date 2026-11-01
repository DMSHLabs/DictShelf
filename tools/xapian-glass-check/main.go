// Temporary diagnostic: read Kiwix's own full-text index (X/fulltext/xapian)
// from a ZIM archive with internal/format/zim/glass, and check what it says
// against the archive itself. The byte-range part is local to this tool until
// the container grows an API for it.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/format/zim"
	"github.com/DMSHLabs/DictShelf/internal/format/zim/glass"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: xapian-glass-check <archive.zim> [term ...]")
		os.Exit(2)
	}
	path := os.Args[1]
	blob, err := entryBlob(path, "fulltext/xapian")
	if err != nil {
		fmt.Println("blob:", err)
		os.Exit(1)
	}
	fmt.Println(path)
	fmt.Println("  fulltext/xapian, байт:", len(blob))

	db, err := glass.Open(blob)
	if err != nil {
		fmt.Println("glass.Open:", err)
		os.Exit(1)
	}
	fmt.Printf("  формат=%d ревизия=%d блок=%d документов=%d\n",
		db.FormatDate, db.Revision, db.BlockSize(), db.Stats.DocCount)

	article, err := zim.Open(path)
	if err != nil {
		fmt.Println("zim.Open:", err)
		os.Exit(1)
	}
	defer article.Close()

	terms := os.Args[2:]
	if len(terms) == 0 {
		terms = []string{"кларнетист", "джаз"}
	}
	dumpDoc(db, 250122)
	dumpDoc(db, 2472)
	for _, term := range terms {
		if raw, size, flags, kl, err := db.RawItem(glass.TablePostlist, []byte(term)); err == nil && raw != nil {
			fmt.Printf("  [dbg] %q элемент: % x размер=%d флаги=%02x ключДлина=%d\n", term, raw, size, flags, kl)
		}
		t, err := db.Lookup(term)
		if err != nil {
			fmt.Printf("  %q: %v\n", term, err)
			continue
		}
		if t.DocFreq == 0 {
			fmt.Printf("  %q: нет такого терма\n", term)
			continue
		}
		note := ""
		if !t.Complete {
			note = " (первый кусок из нескольких)"
		}
		fmt.Printf("  %q: документов %d, вхождений %d, прочитано %d%s\n",
			term, t.DocFreq, t.CollFreq, len(t.Postings), note)
		for i, p := range t.Postings {
			if i >= 3 {
				break
			}
			apath, err := db.DocPath(p.DocID)
			if err != nil {
				fmt.Printf("      doc %d: %v\n", p.DocID, err)
				continue
			}
			fmt.Printf("      doc %-8d wdf=%-4d путь=%q", p.DocID, p.WDF, apath)
			if body, err := articleText(article, apath); err != nil {
				fmt.Printf("  (статью не открыть: %v)\n", err)
			} else if strings.Contains(strings.ToLower(body), strings.ToLower(term)) {
				fmt.Printf("  ← слово есть в статье (%d байт)\n", len(body))
			} else {
				fmt.Printf("  ← СЛОВА В СТАТЬЕ НЕТ (%d байт)\n", len(body))
			}
		}
	}
}

// articleText fetches an article by the path the index gave us.
func articleText(d *zim.Dict, path string) (string, error) {
	name := strings.TrimPrefix(path, "C/")
	res, err := d.Exact(name, 1)
	if err != nil || len(res) == 0 {
		return "", fmt.Errorf("нет статьи по пути %q", path)
	}
	return string(res[0].Body), nil
}

// entryBlob returns the bytes of one X/ entry, parsing the ZIM header, the
// directory and the cluster pointer list.
func entryBlob(path, want string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := uint64(fi.Size())
	h := make([]byte, 80)
	if _, err := f.ReadAt(h, 0); err != nil {
		return nil, err
	}
	entryCount := int(binary.LittleEndian.Uint32(h[24:]))
	clusterCount := int(binary.LittleEndian.Uint32(h[28:]))
	pathPtrPos := binary.LittleEndian.Uint64(h[32:])
	clusterPtrPos := binary.LittleEndian.Uint64(h[48:])
	read := func(off uint64, n int) []byte {
		b := make([]byte, n)
		if _, err := f.ReadAt(b, int64(off)); err != nil {
			return nil
		}
		return b
	}
	cp := read(clusterPtrPos, clusterCount*8)
	starts := make([]uint64, clusterCount)
	for i := range starts {
		starts[i] = binary.LittleEndian.Uint64(cp[i*8:])
	}
	sorted := append([]uint64(nil), starts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	buf := read(pathPtrPos, entryCount*8)
	for i := 0; i < entryCount; i++ {
		off := binary.LittleEndian.Uint64(buf[i*8:])
		d := read(off, 512)
		if len(d) < 16 {
			continue
		}
		cluster := binary.LittleEndian.Uint32(d[8:])
		blobIdx := binary.LittleEndian.Uint32(d[12:])
		rest := d[16:]
		p := bytes.IndexByte(rest, 0)
		if p < 0 || string(rest[:p]) != want {
			continue
		}
		return readBlob(f, size, starts[cluster], blobIdx)
	}
	return nil, fmt.Errorf("записи %q в архиве нет", want)
}

func readBlob(f *os.File, size uint64, start uint64, want uint32) ([]byte, error) {
	info := make([]byte, 1)
	if _, err := f.ReadAt(info, int64(start)); err != nil {
		return nil, err
	}
	codec := info[0] & 0x0f
	if codec != 0 && codec != 1 {
		return nil, fmt.Errorf("кластер сжат (кодек %d)", codec)
	}
	w := 4
	if info[0]&0x10 != 0 {
		w = 8
	}
	dataStart := start + 1
	head := make([]byte, w*(int(want)+2))
	if _, err := f.ReadAt(head, int64(dataStart)); err != nil {
		return nil, err
	}
	at := func(i int) uint64 {
		if w == 8 {
			return binary.LittleEndian.Uint64(head[i*8:])
		}
		return uint64(binary.LittleEndian.Uint32(head[i*4:]))
	}
	count := int(at(0))/w - 1
	if int(want) >= count {
		return nil, fmt.Errorf("блоба %d нет (их %d)", want, count)
	}
	lo, hi := at(int(want)), at(int(want)+1)
	if hi <= lo || dataStart+hi > size {
		return nil, fmt.Errorf("диапазон блоба выходит за файл")
	}
	out := make([]byte, hi-lo)
	if _, err := f.ReadAt(out, int64(dataStart+lo)); err != nil {
		return nil, err
	}
	return out, nil
}

// dumpDoc prints the raw item the docdata table holds for a document id.
func dumpDoc(db *glass.DB, docid uint32) {
	raw, size, flags, kl, err := db.RawItem(glass.TableDocdata, glass.DocKey(docid))
	if err != nil {
		fmt.Printf("  [dbg] doc %d: %v\n", docid, err)
		return
	}
	fmt.Printf("  [dbg] doc %d элемент: % x\n        размер=%d флаги=%02x ключДлина=%d ключ=% x\n",
		docid, raw, size, flags, kl, raw[3:3+kl])
}
