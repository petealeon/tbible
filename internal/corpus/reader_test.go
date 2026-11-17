package corpus_test

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"strings"
	"testing"

	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/embedded"
)

func TestCorpusRoundTrip(t *testing.T) {
	want := syntheticBible()
	want.Prepare()
	var buf bytes.Buffer
	if _, err := corpus.Write(&buf, want); err != nil {
		t.Fatal(err)
	}
	got, err := corpus.Open(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(got.Books) != len(want.Books) || len(got.Chapters) != len(want.Chapters) || len(got.Verses) != len(want.Verses) {
		t.Fatalf("round trip counts differ: %+v", got)
	}
	for i := range want.Verses {
		if got.Verses[i].Text != want.Verses[i].Text {
			t.Fatalf("verse %d text = %q, want %q", i, got.Verses[i].Text, want.Verses[i].Text)
		}
	}
}

func TestOpenBorrowedMatchesOpen(t *testing.T) {
	want := syntheticBible()
	want.SetVerseDecorations(0, corpus.VerseDecorations{
		Italics:   []corpus.Span{{Start: 2, End: 5}},
		RedLetter: []corpus.Span{{Start: 6, End: 9}},
		Notes:     []corpus.Note{{Anchor: 2, Text: "A marginal note"}},
		Headings:  []corpus.Heading{{Text: "A heading", Kind: "title"}},
	})
	want.Prepare()

	var buf bytes.Buffer
	if _, err := corpus.Write(&buf, want); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	copied, err := corpus.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	borrowed, err := corpus.OpenBorrowed(data)
	if err != nil {
		t.Fatal(err)
	}
	for i := range want.Verses {
		a, b := &copied.Verses[i], &borrowed.Verses[i]
		if a.Text != b.Text || a.Flags != b.Flags || a.BookID != b.BookID || a.Chapter != b.Chapter || a.Number != b.Number {
			t.Fatalf("verse %d metadata differs: %+v vs %+v", i, a, b)
		}
		if !reflect.DeepEqual(a.Italics(), b.Italics()) || !reflect.DeepEqual(a.RedLetter(), b.RedLetter()) ||
			!reflect.DeepEqual(a.Notes(), b.Notes()) || !reflect.DeepEqual(a.Headings(), b.Headings()) {
			t.Fatalf("verse %d decorations differ: %+v vs %+v", i, a, b)
		}
	}
	if !reflect.DeepEqual(copied.Books, borrowed.Books) || !reflect.DeepEqual(copied.Chapters, borrowed.Chapters) {
		t.Fatal("borrowed corpus metadata differs from copied corpus")
	}
}

func TestOpenEmbeddedLookups(t *testing.T) {
	b, err := corpus.Open(embedded.CorpusBytes())
	if err != nil {
		t.Fatal(err)
	}
	v := b.VerseAt(43, 3, 16)
	if v == nil || !strings.Contains(v.Text, "For God so loved the world") {
		t.Fatalf("John 3:16 = %+v", v)
	}
	if b.VerseAt(43, 3, 0) != nil || b.VerseAt(43, 3, 999) != nil {
		t.Fatal("out-of-range verse resolved")
	}
	if got := len(b.ChapterVerses(43, 3)); got != 36 {
		t.Fatalf("John 3 chapter has %d verses, want 36", got)
	}
	if got := b.ChapterVerses(43, 999); got != nil {
		t.Fatalf("missing chapter returned %d verses", len(got))
	}
	if got := b.VerseIndex(43, 3, 16); got < 0 || got >= len(b.Verses) {
		t.Fatalf("John 3:16 index = %d", got)
	}
}

func TestOpenRejectsCorruptData(t *testing.T) {
	valid := embedded.CorpusBytes()
	corrupt := func(mutate func([]byte)) []byte {
		out := append([]byte(nil), valid...)
		mutate(out)
		return out
	}
	put16 := func(off int, v uint16) func([]byte) {
		return func(b []byte) { binary.LittleEndian.PutUint16(b[off:off+2], v) }
	}
	put32 := func(off int, v uint32) func([]byte) {
		return func(b []byte) { binary.LittleEndian.PutUint32(b[off:off+4], v) }
	}

	cases := []struct {
		name string
		data []byte
	}{
		{"nil", nil},
		{"short header", valid[:10]},
		{"bad magic", corrupt(func(b []byte) { copy(b[0:4], "NOPE") })},
		{"bad version", corrupt(put16(4, corpus.FormatVersion+1))},
		{"bad header size", corrupt(put16(6, 71))},
		{"no books", corrupt(put16(8, 0))},
		{"no chapters", corrupt(put16(10, 0))},
		{"no verses", corrupt(put32(12, 0))},
		{"huge verse count", corrupt(put32(12, ^uint32(0)))},
		{"string blob too long", corrupt(put32(16, ^uint32(0)))},
		{"book offset unaligned", corrupt(put32(20, 9))},
		{"book section wrong size", corrupt(put32(24, 1))},
		{"book count mismatch", corrupt(put16(8, 2))},
		{"chapter count mismatch", corrupt(put16(10, 2))},
		{"verse section wrong size", corrupt(put32(40, 1))},
		{"chapter offset past end", corrupt(put32(28, uint32(len(valid))+8))},
		{"verse offset past end", corrupt(put32(36, uint32(len(valid))+8))},
		{"span offset past end", corrupt(put32(44, uint32(len(valid))+8))},
		{"note offset past end", corrupt(put32(52, uint32(len(valid))+8))},
		{"heading offset past end", corrupt(put32(60, uint32(len(valid))+8))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := corpus.Open(c.data)
			if err == nil {
				t.Fatalf("Open accepted corrupt corpus: %+v", b)
			}
		})
	}
}

func FuzzOpen(f *testing.F) {
	f.Add(embedded.CorpusBytes())
	f.Add([]byte{})
	f.Add([]byte("TBC2"))
	f.Fuzz(func(t *testing.T, data []byte) {
		b, err := corpus.Open(data)
		if err != nil {
			return
		}
		if err := b.Validate(); err != nil {
			t.Fatalf("Open returned an invalid corpus: %v", err)
		}
		if len(b.Books) > 0 {
			b.VerseAt(b.Books[0].ID, 1, 1)
		}
	})
}
