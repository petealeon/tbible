package corpus_test

import (
	"testing"

	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/embedded"
)

// syntheticBible is the smallest corpus that satisfies Validate: two chapters
// and four contiguous verses.
func syntheticBible() *corpus.Bible {
	return &corpus.Bible{
		Books: []corpus.Book{
			{ID: 1, Name: "Test", Abbrev: "Tst", Testament: 0, ChapterCount: 2, FirstChapter: 0},
		},
		Chapters: []corpus.Chapter{
			{BookID: 1, Number: 1, FirstVerse: 0, VerseCount: 2},
			{BookID: 1, Number: 2, FirstVerse: 2, VerseCount: 2},
		},
		Verses: []corpus.Verse{
			{BookID: 1, Chapter: 1, Number: 1, Text: "In the beginning"},
			{BookID: 1, Chapter: 1, Number: 2, Text: "and the word"},
			{BookID: 1, Chapter: 2, Number: 1, Text: "alpha"},
			{BookID: 1, Chapter: 2, Number: 2, Text: "omega"},
		},
	}
}

func cloneBible(b *corpus.Bible) *corpus.Bible {
	out := &corpus.Bible{
		Books:    append([]corpus.Book(nil), b.Books...),
		Chapters: append([]corpus.Chapter(nil), b.Chapters...),
		Verses:   make([]corpus.Verse, len(b.Verses)),
	}
	copy(out.Verses, b.Verses)
	return out
}

func TestValidateAcceptsSyntheticCorpus(t *testing.T) {
	b := syntheticBible()
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := b.ValidateKJV(); err == nil {
		t.Fatal("synthetic corpus passed KJV validation")
	}
}

func TestValidateStructuralErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*corpus.Bible)
	}{
		{"no books", func(b *corpus.Bible) { b.Books = nil }},
		{"no chapters", func(b *corpus.Bible) { b.Chapters = nil }},
		{"no verses", func(b *corpus.Bible) { b.Verses = nil }},
		{"duplicate book id", func(b *corpus.Bible) {
			b.Books = append(b.Books, corpus.Book{ID: 1, Name: "Other", ChapterCount: 1})
		}},
		{"book without chapters", func(b *corpus.Bible) { b.Books[0].ChapterCount = 0 }},
		{"book start out of order", func(b *corpus.Bible) { b.Books[0].FirstChapter = 1 }},
		{"chapter numbered wrong", func(b *corpus.Bible) { b.Chapters[0].Number = 7 }},
		{"chapter belongs to another book", func(b *corpus.Bible) { b.Chapters[1].BookID = 2 }},
		{"chapter without verses", func(b *corpus.Bible) { b.Chapters[0].VerseCount = 0 }},
		{"chapter start gap", func(b *corpus.Bible) { b.Chapters[0].FirstVerse = 1 }},
		{"chapter runs past verses", func(b *corpus.Bible) { b.Chapters[1].VerseCount = 99 }},
		{"verse reference invalid", func(b *corpus.Bible) { b.Verses[0].Number = 0 }},
		{"verse stored out of order", func(b *corpus.Bible) {
			b.Verses[0], b.Verses[1] = b.Verses[1], b.Verses[0]
		}},
		{"verse exceeds chapter", func(b *corpus.Bible) { b.Verses[0].Number = 9 }},
		{"unknown verse flags", func(b *corpus.Bible) { b.Verses[0].Flags = 0x80 }},
		{"italics span past text", func(b *corpus.Bible) {
			b.SetVerseDecorations(0, corpus.VerseDecorations{Italics: []corpus.Span{{Start: 0, End: 999}}})
		}},
		{"red letter span negative", func(b *corpus.Bible) {
			b.SetVerseDecorations(0, corpus.VerseDecorations{RedLetter: []corpus.Span{{Start: -1, End: 1}}})
		}},
		{"note anchor past text", func(b *corpus.Bible) {
			b.SetVerseDecorations(0, corpus.VerseDecorations{Notes: []corpus.Note{{Anchor: 999, Text: "note"}}})
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := cloneBible(syntheticBible())
			c.mutate(b)
			if err := b.Validate(); err == nil {
				t.Fatal("Validate accepted invalid corpus")
			}
		})
	}
}

func TestValidateKJVEmbedded(t *testing.T) {
	b, err := corpus.Open(embedded.CorpusBytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := b.ValidateKJV(); err != nil {
		t.Fatal(err)
	}
	if len(b.Books) != 66 || len(b.Chapters) != corpus.TotalChapters || len(b.Verses) != corpus.TotalVerses {
		t.Fatalf("totals = %d books, %d chapters, %d verses", len(b.Books), len(b.Chapters), len(b.Verses))
	}
}
