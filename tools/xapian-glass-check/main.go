// Temporary diagnostic: read Kiwix's own full-text index (X/fulltext/xapian)
// from a ZIM archive with internal/format/zim/glass, and check what it says
// against the archive itself. The index is read through the container's own
// seekable window, so nothing is copied; nothing here writes.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/format/zim"
	"github.com/DMSHLabs/DictShelf/internal/format/zim/glass"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: xapian-glass-check <archive.zim> [query | -chunks term] ...")
		os.Exit(2)
	}
	path := os.Args[1]
	article, err := zim.Open(path)
	if err != nil {
		fmt.Println("zim.Open:", err)
		os.Exit(1)
	}
	defer article.Close()
	window, size, err := article.XapianIndex()
	if err != nil {
		fmt.Println("XapianIndex:", err)
		os.Exit(1)
	}
	fmt.Println(path)
	fmt.Println("  fulltext/xapian, байт:", size)

	db, err := glass.OpenSource(window, size)
	if err != nil {
		fmt.Println("glass.OpenSource:", err)
		os.Exit(1)
	}
	fmt.Printf("  формат=%d ревизия=%d блок=%d документов=%d\n",
		db.FormatDate, db.Revision, db.BlockSize(), db.Stats.DocCount)

	printMetadata(db)
	if len(os.Args) > 2 && os.Args[2] == "-chunks" {
		dumpChunks(db, os.Args[3])
		return
	}

	terms := os.Args[2:]
	if len(terms) == 0 {
		terms = []string{"джазовый кларнетист", "джаз", "кларнетист"}
	}
	runQueries(db, article, terms)
	runFullText(article, terms)
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

// dumpChunks prints the raw items that continue a term's posting list.
func dumpChunks(db *glass.DB, term string) {
	raws, keys, err := db.RawChunks(term, 3)
	if err != nil {
		fmt.Println("  куски:", err)
	}
	for i := range raws {
		fmt.Printf("  кусок %d ключ=% x\n        % x\n", i+1, keys[i], raws[i])
	}
}

// printMetadata shows what the index says about itself: the language its terms
// were stemmed for, the kind of index, and whether stopwords were dropped.
func printMetadata(db *glass.DB) {
	for _, key := range []string{"language", "kind", "data", "valuesmap"} {
		v, found, err := db.Metadata(key)
		switch {
		case err != nil:
			fmt.Printf("  метаданные %-10s ошибка: %v\n", key, err)
		case !found:
			fmt.Printf("  метаданные %-10s нет\n", key)
		default:
			fmt.Printf("  метаданные %-10s %q\n", key, v)
		}
	}
	words, err := db.Stopwords()
	if err != nil {
		fmt.Println("  стоп-слова: ошибка:", err)
	} else if len(words) == 0 {
		fmt.Println("  стоп-слова: нет")
	} else {
		head := words
		if len(head) > 8 {
			head = head[:8]
		}
		fmt.Printf("  стоп-слова: %d, начало %q\n", len(words), head)
	}
	switch lang, err := db.IndexLanguage(); {
	case err != nil:
		fmt.Println("  язык индекса:", err)
	case lang == "":
		fmt.Println("  язык индекса: не указан, термы не стеммированы")
	default:
		fmt.Printf("  язык индекса: %q, стеммер по нему есть\n", lang)
	}
}

// runQueries exercises the query side: every argument is one query, and every
// article it returns is opened and checked to hold, for each term the index was
// asked for, a word whose stem is that term - which is exactly the index's own
// criterion, so the two agreeing is the whole point of the check.
func runQueries(db *glass.DB, article *zim.Dict, queries []string) {
	stem, _ := glass.StemmerFor(indexLanguage(db))
	for _, query := range queries {
		res, err := db.Search(query)
		if err != nil {
			fmt.Println("  запрос", query, ": ", err)
			continue
		}
		note := ""
		if !res.Complete {
			note = " (не все списки прочитаны до конца)"
		}
		fmt.Printf("  запрос %q: термов %d %v, статей %d%s\n",
			query, len(res.Terms), res.Terms, len(res.Hits), note)
		if len(res.Terms) == 1 {
			freq, _ := db.TermFreq(res.Terms[0])
			t, _ := db.Lookup(res.Terms[0])
			fmt.Printf("      сверка: терм %q частота=%d записей=%d попаданий=%d\n",
				res.Terms[0], freq, len(t.Postings), len(res.Hits))
		}
		if len(res.Skipped) > 0 {
			fmt.Printf("      стоп-слова пропущены: %v\n", res.Skipped)
		}
		if len(res.Missing) > 0 {
			fmt.Printf("      нет в индексе: %v\n", res.Missing)
		}
		checked, bad := 0, 0
		for _, h := range res.Hits {
			if checked >= 3 {
				break
			}
			apath, err := db.DocPath(h.DocID)
			if err != nil {
				fmt.Println("      doc", h.DocID, ":", err)
				continue
			}
			body, err := articleText(article, apath)
			if err != nil {
				fmt.Printf("      doc %-8d путь=%q  (статью не открыть: %v)\n", h.DocID, apath, err)
				continue
			}
			missing := termsNotPresent(stem, res.Terms, body)
			checked++
			if len(missing) > 0 {
				bad++
				fmt.Printf("      doc %-8d путь=%q  НЕТ СЛОВ: %v\n", h.DocID, apath, missing)
			} else {
				fmt.Printf("      doc %-8d путь=%q  все термы есть (%d байт)\n", h.DocID, apath, len(body))
			}
		}
		if checked > 0 && bad == 0 {
			fmt.Println("      → все проверенные статьи содержат все термы запроса")
		}
	}
}

// runFullText exercises the bridge the product would use: the archive answers a
// query from its own index, and the answer comes back as ordinary results -
// headword and article - which is the shape Exact returns.
func runFullText(article *zim.Dict, queries []string) {
	for _, query := range queries {
		found, err := article.FullText(query, 5)
		if err != nil {
			fmt.Printf("  FullText %q: %v\n", query, err)
			continue
		}
		fmt.Printf("  FullText %q: статей %d\n", query, len(found))
		for _, r := range found {
			fmt.Printf("      %-40s статья %d байт\n", r.Headword, len(r.Body))
		}
	}
}

// indexLanguage is the language the index declares, "" when it declares none.
func indexLanguage(db *glass.DB) string {
	lang, err := db.IndexLanguage()
	if err != nil {
		return ""
	}
	return lang
}

// termsNotPresent returns the query terms no word of the article stems to. The
// article's words are folded and stemmed the way a query is, so a term is
// "present" when the index would have indexed the article under it.
func termsNotPresent(stem glass.Stemmer, terms []string, body string) []string {
	have := map[string]bool{}
	for _, word := range glass.FoldTerms(body) {
		have[word] = true
		if stem != nil {
			if s := stem(word); s != word {
				have[s] = true
			}
		}
	}
	var missing []string
	for _, t := range terms {
		if !have[t] {
			missing = append(missing, t)
		}
	}
	return missing
}

// stemmedForms is the stem of a folded word. The language is asked of the
// package so the check uses the same stemmer a search does.
func stemmedForms(word string) []string {
	stem, ok := glass.StemmerFor("rus")
	if !ok {
		return nil
	}
	if s := stem(word); s != word {
		return []string{s}
	}
	return nil
}
