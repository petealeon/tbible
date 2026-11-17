// Command tbgen builds the binary corpus, search index, and the legacy JSON
// source from the verified eBible USFM KJV.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/search"
	"github.com/petealeon/tbible/v2/internal/usfm"
)

func main() {
	var (
		src        = flag.String("src", "data/eng-kjv2006_usfm.zip", "USFM source zip")
		corpusOut  = flag.String("corpus", "internal/embedded/data/kjv.tbc", "binary corpus output")
		indexOut   = flag.String("index", "internal/embedded/data/kjv.tbi", "search index output")
		jsonOut    = flag.String("json", "legacy/v1/data/kjv.json", "legacy KJV JSON output")
		verifyOnly = flag.Bool("verify", false, "validate only; do not write files")
		quiet      = flag.Bool("quiet", false, "suppress detail")
	)
	flag.Parse()

	start := time.Now()
	zipBytes, err := os.ReadFile(*src)
	if err != nil {
		fatal(err)
	}
	sum := sha256.Sum256(zipBytes)
	if !*quiet {
		fmt.Printf("source: %s (%d bytes, sha256 %s)\n", *src, len(zipBytes), hex.EncodeToString(sum[:]))
	}

	b, err := usfm.ParseZip(*src)
	if err != nil {
		fatal(err)
	}
	if err := validate(b); err != nil {
		fatal(err)
	}
	if !*quiet {
		fmt.Printf("corpus: %d books, %d chapters, %d verses\n", len(b.Books), len(b.Chapters), len(b.Verses))
	}

	if *verifyOnly {
		fmt.Println("verified: canonical KJV")
		return
	}

	corpusTmp := *corpusOut + ".tmp"
	if err := writeAtomic(corpusTmp, *corpusOut, func(f *os.File) error {
		n, err := corpus.Write(f, b)
		if err == nil && !*quiet {
			fmt.Printf("corpus written: %s (%d bytes)\n", *corpusOut, n)
		}
		return err
	}); err != nil {
		fatal(err)
	}
	// Reopen to prove the reader agrees with the writer.
	blob, err := os.ReadFile(*corpusOut)
	if err != nil {
		fatal(err)
	}
	check, err := corpus.Open(blob)
	if err != nil {
		fatal(fmt.Errorf("corpus round-trip: %w", err))
	}
	if check.Verses[0].Text != b.Verses[0].Text || check.Verses[len(check.Verses)-1].Text != b.Verses[len(b.Verses)-1].Text {
		fatal(fmt.Errorf("corpus round-trip mismatch"))
	}

	ix := search.Build(b)
	if !*quiet {
		fmt.Printf("index: %d terms, %d verses\n", ix.Tokens(), ix.Verses())
	}
	indexTmp := *indexOut + ".tmp"
	if err := writeAtomic(indexTmp, *indexOut, func(f *os.File) error {
		n, err := ix.Write(f)
		if err == nil && !*quiet {
			fmt.Printf("index written: %s (%d bytes)\n", *indexOut, n)
		}
		return err
	}); err != nil {
		fatal(err)
	}
	indexBlob, err := os.ReadFile(*indexOut)
	if err != nil {
		fatal(err)
	}
	if _, err := search.Open(indexBlob); err != nil {
		fatal(fmt.Errorf("index round-trip: %w", err))
	}

	if err := writeJSON(*jsonOut, b); err != nil {
		fatal(err)
	}
	if !*quiet {
		fmt.Printf("legacy JSON written: %s\n", *jsonOut)
		fmt.Printf("done in %s\n", time.Since(start).Round(time.Millisecond))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "tbgen:", err)
	os.Exit(1)
}

func writeAtomic(tmp, final string, fn func(*os.File) error) error {
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return err
	}
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := fn(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func validate(b *corpus.Bible) error {
	if err := b.ValidateKJV(); err != nil {
		return err
	}
	for i := range b.Verses {
		v := &b.Verses[i]
		if strings.TrimSpace(v.Text) == "" {
			return fmt.Errorf("empty verse %d", i)
		}
		if strings.ContainsAny(v.Text, `\{}|`) {
			return fmt.Errorf("%s %d:%d contains markup: %q", b.Books[v.BookID-1].Name, v.Chapter, v.Number, v.Text)
		}
		for _, n := range v.Notes() {
			if strings.ContainsAny(n.Text, `\{}`) {
				return fmt.Errorf("%s %d:%d note contains markup: %q", b.Books[v.BookID-1].Name, v.Chapter, v.Number, n.Text)
			}
		}
	}

	known := []struct {
		book, text string
		ch, v      uint16
	}{
		{"Genesis", "In the beginning God created the heaven and the earth.", 1, 1},
		{"Matthew", "Then Herod, when he saw that he was mocked of the wise men", 2, 16},
		{"Matthew", "And Jesus answered and spake unto them again by parables", 22, 1},
		{"Matthew", "My soul is exceeding sorrowful, even unto death", 26, 38},
		{"Mark", "Why are ye so fearful? how is it that ye have no faith?", 4, 40},
		{"Mark", "It is Corban, that is to say, a gift", 7, 11},
		{"Mark", "they took up of the broken meat that was left seven baskets", 8, 8},
		{"Mark", "If any man have ears to hear, let him hear.", 7, 16},
		{"Matthew", "Howbeit this kind goeth not out but by prayer and fasting.", 17, 21},
		{"Matthew", "For the Son of man is come to save that which was lost.", 18, 11},
		{"Mark", "Where their worm dieth not, and the fire is not quenched.", 9, 44},
		{"Luke", "Two men shall be in the field", 17, 36},
		{"John", "For God so loved the world, that he gave his only begotten Son", 3, 16},
		{"Acts", "And Philip said, If thou believest with all thine heart, thou mayest.", 8, 37},
		{"Romans", "The grace of our Lord Jesus Christ be with you all. Amen.", 16, 24},
		{"1 John", "For there are three that bear record in heaven, the Father, the Word, and the Holy Ghost", 5, 7},
		{"Revelation", "The grace of our Lord Jesus Christ be with you all. Amen.", 22, 21},
	}
	byName := make(map[string]uint16, len(b.Books))
	for _, bk := range b.Books {
		byName[bk.Name] = bk.ID
	}
	for _, c := range known {
		id, ok := byName[c.book]
		if !ok {
			return fmt.Errorf("unknown book %q", c.book)
		}
		v := b.VerseAt(id, c.ch, c.v)
		if v == nil {
			return fmt.Errorf("missing %s %d:%d", c.book, c.ch, c.v)
		}
		if !strings.Contains(v.Text, c.text) {
			return fmt.Errorf("%s %d:%d = %q, want %q", c.book, c.ch, c.v, v.Text, c.text)
		}
	}
	return nil
}

type jsonBook struct {
	Name     string     `json:"name"`
	Abbrev   string     `json:"abbrev,omitempty"`
	Chapters [][]string `json:"chapters"`
}

func writeJSON(path string, b *corpus.Bible) error {
	books := make([]jsonBook, 0, len(b.Books))
	for _, bk := range b.Books {
		chapters := make([][]string, 0, bk.ChapterCount)
		for c := uint16(1); c <= bk.ChapterCount; c++ {
			vs := b.ChapterVerses(bk.ID, c)
			texts := make([]string, len(vs))
			for i := range vs {
				texts[i] = vs[i].Text
			}
			chapters = append(chapters, texts)
		}
		books = append(books, jsonBook{Name: bk.Name, Abbrev: bk.Abbrev, Chapters: chapters})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(books); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
