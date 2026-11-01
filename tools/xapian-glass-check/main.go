// Temporary diagnostic: read Kiwix's own full-text index (X/fulltext/xapian)
// from a ZIM archive with internal/format/zim/glass - the byte range part is
// local to this tool until the container grows an API for it.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sort"

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
	fmt.Printf("%s\n  fulltext/xapian: %d байт\n", path, len(blob))

	db, err := glass.Open(blob)
	if err != nil {
		fmt.Println("glass.Open:", err)
		os.Exit(1)
	}
	fmt.Printf("  формат=%d ревизия=%d uuid=%x блок=%d\n",
		db.FormatDate, db.Revision, db.UUID[:4], db.BlockSize())
	fmt.Printf("  документов=%d последний id=%d суммарная длина=%d\n",
		db.Stats.DocCount, db.Stats.LastDocID, db.Stats.TotalDocLen)
	for i, r := range db.Roots {
		fmt.Printf("  таблица %d: корень=%d уровень=%d записей=%d\n", i, r.Block, r.Level, r.Entries)
	}

	terms := os.Args[2:]
	if len(terms) == 0 {
		terms = []string{"Zджаз", "джаз", "Zкларнетист", "кларнетист"}
	}
	for _, term := range terms {
		tag, found, err := db.Find(glass.TablePostlist, []byte(term))
		switch {
		case err != nil:
			fmt.Printf("  %-14q ошибка: %v\n", term, err)
		case !found:
			fmt.Printf("  %-14q нет такого терма\n", term)
		case len(tag) == 0:
			fmt.Printf("  %-14q найден, тег пустой\n", term)
		default:
			n := len(tag)
			if n > 12 {
				n = 12
			}
			fmt.Printf("  %-14q найден, тег %d байт, начало % x\n", term, len(tag), tag[:n])
		}
	}
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
		return readBlob(f, size, sorted, starts[cluster], blobIdx)
	}
	return nil, fmt.Errorf("записи %q в архиве нет", want)
}

func readBlob(f *os.File, size uint64, sorted []uint64, start uint64, want uint32) ([]byte, error) {
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
