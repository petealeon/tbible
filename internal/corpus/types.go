// Package corpus holds the in-memory Bible model and the compact on-disk
// format used by tbible v2. The model is produced by cmd/tbgen from USFM and
// embedded into the final executable.
package corpus

import (
	"fmt"
	"math"
)

// Span is a byte range within a verse's Text.
type Span struct {
	Start int
	End   int
}

// Note is a KJV marginal note attached to a verse. Anchor is a byte offset
// used to place a marker in the text; it is approximate after whitespace
// normalisation and may be zero for notes without a useful anchor.
type Note struct {
	Text   string
	Anchor int
}

// Heading is a section heading or psalm title attached to a verse.
type Heading struct {
	Text string
	Kind string
}

// Verse flags.
const (
	FlagParagraph uint8 = 1 << 0
	FlagPoetry    uint8 = 1 << 1
)

// Book is one of the 66 canonical books.
type Book struct {
	ID           uint16
	Name         string
	Abbrev       string
	Testament    uint8 // 0 = Old, 1 = New
	ChapterCount uint16
	FirstChapter uint16
}

// Chapter is one chapter of one book.
type Chapter struct {
	BookID     uint16
	Number     uint16
	FirstVerse uint32
	VerseCount uint16
}

// Verse is a single verse with display text and optional styling/notes.
//
// Decoration slices are deliberately not stored per verse: four slice headers
// on all 31,102 verses cost more than the decorations themselves. Instead the
// Bible keeps flat, borrowed decoration arrays and each verse stores only 16-bit
// offsets and counts into them. Use the Italics, RedLetter, Notes and Headings
// accessors to read a verse's decorations.
type Verse struct {
	BookID  uint16
	Chapter uint16
	Number  uint16
	Flags   uint8
	Text    string

	decor   *decorations
	italOff uint16
	italLen uint16
	redOff  uint16
	redLen  uint16
	noteOff uint16
	noteLen uint16
	headOff uint16
	headLen uint16
}

// decorations holds the flat decoration arrays shared by every verse in a
// Bible. Strings inside Note and Heading may be borrowed from the corpus blob.
type decorations struct {
	italics   []Span
	redLetter []Span
	notes     []Note
	headings  []Heading
}

// VerseDecorations is the builder-side representation of a verse's optional
// styling and notes. It is consumed by Bible.Prepare and not retained.
type VerseDecorations struct {
	Italics   []Span
	RedLetter []Span
	Notes     []Note
	Headings  []Heading
}

// Bible is the complete corpus.
type Bible struct {
	Books    []Book
	Chapters []Chapter
	Verses   []Verse

	sourceDecor []VerseDecorations
	decor       decorations
	decorBuilt  bool

	bookByID    map[uint16]int
	chapterByID map[uint32]int
}

// SetVerseDecorations supplies decorations for verse i while a Bible is being
// built. It must be called before Prepare/Write/Validate.
func (b *Bible) SetVerseDecorations(i int, d VerseDecorations) {
	if i < 0 || i >= len(b.Verses) {
		panic("corpus: verse decoration index out of range")
	}
	if i >= len(b.sourceDecor) {
		b.sourceDecor = append(b.sourceDecor, make([]VerseDecorations, i+1-len(b.sourceDecor))...)
	}
	b.sourceDecor[i] = d
}

// Italics returns the italic spans for v.
func (v *Verse) Italics() []Span {
	if v == nil || v.decor == nil || v.italLen == 0 {
		return nil
	}
	return v.decor.italics[v.italOff : v.italOff+v.italLen]
}

// RedLetter returns the red-letter spans for v.
func (v *Verse) RedLetter() []Span {
	if v == nil || v.decor == nil || v.redLen == 0 {
		return nil
	}
	return v.decor.redLetter[v.redOff : v.redOff+v.redLen]
}

// Notes returns the KJV marginal notes for v.
func (v *Verse) Notes() []Note {
	if v == nil || v.decor == nil || v.noteLen == 0 {
		return nil
	}
	return v.decor.notes[v.noteOff : v.noteOff+v.noteLen]
}

// Headings returns the section headings for v.
func (v *Verse) Headings() []Heading {
	if v == nil || v.decor == nil || v.headLen == 0 {
		return nil
	}
	return v.decor.headings[v.headOff : v.headOff+v.headLen]
}

// Prepare builds lookup maps and flattens builder-supplied decorations into
// shared arrays. It is idempotent.
func (b *Bible) Prepare() {
	b.bookByID = make(map[uint16]int, len(b.Books))
	for i := range b.Books {
		b.bookByID[b.Books[i].ID] = i
	}
	b.chapterByID = make(map[uint32]int, len(b.Chapters))
	for i := range b.Chapters {
		c := &b.Chapters[i]
		b.chapterByID[chapterKey(c.BookID, c.Number)] = i
	}
	if !b.decorBuilt && b.sourceDecor != nil {
		b.buildDecorations()
	}
}

func (b *Bible) buildDecorations() {
	var italics, red, notes, headings int
	for i := range b.sourceDecor {
		d := &b.sourceDecor[i]
		italics += len(d.Italics)
		red += len(d.RedLetter)
		notes += len(d.Notes)
		headings += len(d.Headings)
	}
	check := func(n int, what string) uint16 {
		if n > math.MaxUint16 {
			panic(fmt.Sprintf("corpus: too many %s (%d)", what, n))
		}
		return uint16(n)
	}
	b.decor = decorations{
		italics:   make([]Span, italics),
		redLetter: make([]Span, red),
		notes:     make([]Note, notes),
		headings:  make([]Heading, headings),
	}
	var oi, or, on, oh int
	for i := range b.Verses {
		var d VerseDecorations
		if i < len(b.sourceDecor) {
			d = b.sourceDecor[i]
		}
		v := &b.Verses[i]
		v.decor = &b.decor
		v.italOff, v.italLen = check(oi, "italic spans"), check(len(d.Italics), "italic spans")
		v.redOff, v.redLen = check(or, "red-letter spans"), check(len(d.RedLetter), "red-letter spans")
		v.noteOff, v.noteLen = check(on, "notes"), check(len(d.Notes), "notes")
		v.headOff, v.headLen = check(oh, "headings"), check(len(d.Headings), "headings")
		copy(b.decor.italics[oi:], d.Italics)
		copy(b.decor.redLetter[or:], d.RedLetter)
		copy(b.decor.notes[on:], d.Notes)
		copy(b.decor.headings[oh:], d.Headings)
		oi += len(d.Italics)
		or += len(d.RedLetter)
		on += len(d.Notes)
		oh += len(d.Headings)
	}
	b.decorBuilt = true
	b.sourceDecor = nil
}

func chapterKey(bookID, chapter uint16) uint32 {
	return uint32(bookID)<<16 | uint32(chapter)
}

// BookByID returns the book with the given canonical ID.
func (b *Bible) BookByID(id uint16) *Book {
	if b.bookByID == nil {
		b.Prepare()
	}
	i, ok := b.bookByID[id]
	if !ok {
		return nil
	}
	return &b.Books[i]
}

// ChapterAt returns the chapter record for book/chapter.
func (b *Bible) ChapterAt(bookID, chapter uint16) *Chapter {
	if b.chapterByID == nil {
		b.Prepare()
	}
	i, ok := b.chapterByID[chapterKey(bookID, chapter)]
	if !ok {
		return nil
	}
	return &b.Chapters[i]
}

// ChapterVerses returns the verses of a chapter as a slice into b.Verses.
func (b *Bible) ChapterVerses(bookID, chapter uint16) []Verse {
	c := b.ChapterAt(bookID, chapter)
	if c == nil {
		return nil
	}
	start := int(c.FirstVerse)
	if start < 0 || start > len(b.Verses) {
		return nil
	}
	end := start + int(c.VerseCount)
	if end < start || end > len(b.Verses) {
		return nil
	}
	return b.Verses[start:end]
}

// VerseAt returns the verse for book/chapter/verse.
func (b *Bible) VerseAt(bookID, chapter, verse uint16) *Verse {
	i := b.VerseIndex(bookID, chapter, verse)
	if i < 0 || i >= len(b.Verses) {
		return nil
	}
	v := &b.Verses[i]
	if v.BookID != bookID || v.Chapter != chapter || v.Number != verse {
		return nil
	}
	return v
}

// VerseIndex returns the index into b.Verses for a reference, or -1.
func (b *Bible) VerseIndex(bookID, chapter, verse uint16) int {
	c := b.ChapterAt(bookID, chapter)
	if c == nil || verse == 0 || verse > c.VerseCount {
		return -1
	}
	i := int(c.FirstVerse) + int(verse) - 1
	if i < 0 || i >= len(b.Verses) {
		return -1
	}
	return i
}

// Validate checks structural invariants shared by every corpus. It does not
// assume the 66-book Protestant canon, so optional packs can reuse it.
func (b *Bible) Validate() error {
	b.Prepare()
	if len(b.Books) == 0 {
		return fmt.Errorf("corpus: no books")
	}
	if len(b.Chapters) == 0 {
		return fmt.Errorf("corpus: no chapters")
	}
	if len(b.Verses) == 0 {
		return fmt.Errorf("corpus: no verses")
	}
	if len(b.Books) > math.MaxUint16 {
		return fmt.Errorf("corpus: %d books exceed the format limit", len(b.Books))
	}
	if len(b.Chapters) > math.MaxUint16 {
		return fmt.Errorf("corpus: %d chapters exceed the format limit", len(b.Chapters))
	}
	if uint64(len(b.Verses)) > math.MaxUint32 {
		return fmt.Errorf("corpus: %d verses exceed the format limit", len(b.Verses))
	}

	bookIDs := make(map[uint16]struct{}, len(b.Books))
	chapterIndex := make(map[uint32]int, len(b.Chapters))
	expectedChapter, expectedVerse := 0, 0
	for i := range b.Books {
		bk := &b.Books[i]
		if bk.ID == 0 {
			return fmt.Errorf("corpus: book %d has no id", i)
		}
		if _, dup := bookIDs[bk.ID]; dup {
			return fmt.Errorf("corpus: duplicate book id %d", bk.ID)
		}
		bookIDs[bk.ID] = struct{}{}
		if bk.Name == "" {
			return fmt.Errorf("corpus: book %d has no name", bk.ID)
		}
		if bk.ChapterCount == 0 {
			return fmt.Errorf("corpus: %s has no chapters", bk.Name)
		}
		if int(bk.FirstChapter) != expectedChapter {
			return fmt.Errorf("corpus: %s starts at chapter %d, want %d", bk.Name, bk.FirstChapter, expectedChapter)
		}
		if int(bk.FirstChapter)+int(bk.ChapterCount) > len(b.Chapters) {
			return fmt.Errorf("corpus: %s chapters out of range", bk.Name)
		}
		for j := 0; j < int(bk.ChapterCount); j++ {
			ci := expectedChapter + j
			c := &b.Chapters[ci]
			if c.BookID != bk.ID {
				return fmt.Errorf("corpus: chapter %d belongs to book %d, want %d", ci, c.BookID, bk.ID)
			}
			if c.Number != uint16(j+1) {
				return fmt.Errorf("corpus: %s chapter %d numbered %d", bk.Name, j+1, c.Number)
			}
			if c.VerseCount == 0 {
				return fmt.Errorf("corpus: %s %d has no verses", bk.Name, c.Number)
			}
			if int(c.FirstVerse) != expectedVerse {
				return fmt.Errorf("corpus: %s %d starts at verse %d, want %d", bk.Name, c.Number, c.FirstVerse, expectedVerse)
			}
			end := int(c.FirstVerse) + int(c.VerseCount)
			if end < int(c.FirstVerse) || end > len(b.Verses) {
				return fmt.Errorf("corpus: %s %d verses out of range", bk.Name, c.Number)
			}
			key := chapterKey(c.BookID, c.Number)
			if _, dup := chapterIndex[key]; dup {
				return fmt.Errorf("corpus: duplicate chapter %s %d", bk.Name, c.Number)
			}
			chapterIndex[key] = ci
			expectedVerse = end
		}
		expectedChapter += int(bk.ChapterCount)
	}
	if expectedChapter != len(b.Chapters) {
		return fmt.Errorf("corpus: %d chapters, expected %d", len(b.Chapters), expectedChapter)
	}
	if expectedVerse != len(b.Verses) {
		return fmt.Errorf("corpus: %d verses, expected %d", len(b.Verses), expectedVerse)
	}

	for i := range b.Verses {
		v := &b.Verses[i]
		if v.BookID == 0 || v.Chapter == 0 || v.Number == 0 {
			return fmt.Errorf("corpus: verse %d has an invalid reference", i)
		}
		ci, ok := chapterIndex[chapterKey(v.BookID, v.Chapter)]
		if !ok {
			return fmt.Errorf("corpus: verse %d references missing chapter %d:%d", i, v.BookID, v.Chapter)
		}
		c := &b.Chapters[ci]
		if v.Number > c.VerseCount {
			return fmt.Errorf("corpus: verse %d:%d:%d exceeds chapter length", v.BookID, v.Chapter, v.Number)
		}
		if want := int(c.FirstVerse) + int(v.Number) - 1; want != i {
			return fmt.Errorf("corpus: verse %d:%d:%d is stored at %d, want %d", v.BookID, v.Chapter, v.Number, i, want)
		}
		if v.Flags&^uint8(FlagParagraph|FlagPoetry) != 0 {
			return fmt.Errorf("corpus: verse %d has unknown flags %#x", i, v.Flags)
		}
		if err := validateSpans(v.Italics(), len(v.Text)); err != nil {
			return fmt.Errorf("corpus: verse %d italics: %w", i, err)
		}
		if err := validateSpans(v.RedLetter(), len(v.Text)); err != nil {
			return fmt.Errorf("corpus: verse %d red letter: %w", i, err)
		}
		for _, n := range v.Notes() {
			if n.Anchor < 0 || n.Anchor > len(v.Text) {
				return fmt.Errorf("corpus: verse %d note anchor out of range", i)
			}
		}
	}
	return nil
}

// ValidateKJV additionally enforces the exact 66-book King James canon.
func (b *Bible) ValidateKJV() error {
	if err := b.Validate(); err != nil {
		return err
	}
	if len(b.Books) != 66 || len(b.Chapters) != TotalChapters || len(b.Verses) != TotalVerses {
		return fmt.Errorf("corpus: canonical totals are %d books, %d chapters, %d verses", len(b.Books), len(b.Chapters), len(b.Verses))
	}
	total := 0
	for i, info := range Canonical {
		bk := &b.Books[i]
		if bk.ID != info.ID || bk.Name != info.Name || bk.Testament != info.Testament {
			return fmt.Errorf("corpus: book %d is %q (id %d), want %q (id %d)", i, bk.Name, bk.ID, info.Name, info.ID)
		}
		if bk.ChapterCount != info.Chapters {
			return fmt.Errorf("corpus: %s has %d chapters, want %d", info.Name, bk.ChapterCount, info.Chapters)
		}
		verses := 0
		for j := 0; j < int(bk.ChapterCount); j++ {
			verses += int(b.Chapters[int(bk.FirstChapter)+j].VerseCount)
		}
		if uint32(verses) != info.Verses {
			return fmt.Errorf("corpus: %s has %d verses, want %d", info.Name, verses, info.Verses)
		}
		total += verses
	}
	if total != TotalVerses {
		return fmt.Errorf("corpus: %d verses, want %d", total, TotalVerses)
	}
	return nil
}

func validateSpans(spans []Span, textLen int) error {
	for i, s := range spans {
		if s.Start < 0 || s.End < s.Start || s.End > textLen {
			return fmt.Errorf("span %d [%d:%d] outside text length %d", i, s.Start, s.End, textLen)
		}
	}
	return nil
}

// Reference is a convenient human-readable location.
func (v *Verse) Reference(book *Book) string {
	if book == nil {
		return fmt.Sprintf("%d:%d", v.Chapter, v.Number)
	}
	return fmt.Sprintf("%s %d:%d", book.Name, v.Chapter, v.Number)
}
