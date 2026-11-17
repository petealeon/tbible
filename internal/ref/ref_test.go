package ref_test

import (
	"testing"

	"github.com/petealeon/tbible/v2/internal/embedded"
	"github.com/petealeon/tbible/v2/internal/ref"
)

func TestParse(t *testing.T) {
	b, err := embedded.Bible()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in   string
		ctx  uint16
		want string
	}{
		{"John 3:16", 0, "John 3:16"},
		{"john 3 16", 0, "John 3:16"},
		{"jn3.16", 0, "John 3:16"},
		{"JN 3:16", 0, "John 3:16"},
		{"jhn 3:16", 0, "John 3:16"},
		{"john316", 0, "John 3:16"},
		{"1 Cor 13", 0, "1 Corinthians 13"},
		{"I Corinthians 13", 0, "1 Corinthians 13"},
		{"First Corinthians 13", 0, "1 Corinthians 13"},
		{"1cor13:4", 0, "1 Corinthians 13:4"},
		{"Ps 23:1-6", 0, "Psalms 23:1-6"},
		{"psalm 23", 0, "Psalms 23"},
		{"Song of Solomon 2:1", 0, "Song of Solomon 2:1"},
		{"Song of Songs 2:1", 0, "Song of Solomon 2:1"},
		{"Canticles 2:1", 0, "Song of Solomon 2:1"},
		{"Phil 4:13", 0, "Philippians 4:13"},
		{"Philemon 1:6", 0, "Philemon 1:6"},
		{"Jude 1:3", 0, "Jude 1:3"},
		{"Judges 5:1", 0, "Judges 5:1"},
		{"Rev 22:21", 0, "Revelation 22:21"},
		{"Revelations 22", 0, "Revelation 22"},
		{"3 John 1:2", 0, "3 John 1:2"},
		{"III John 2", 0, "3 John 1:2"},
		{"Second Timothy 3:16", 0, "2 Timothy 3:16"},
		{"Matt 5", 0, "Matthew 5"},
		{"Matthew", 0, "Matthew 1"},
		{"3:16", 43, "John 3:16"},
		{"john 3:16; rom 5:8", 0, "John 3:16"},
		{"genesis 1:1", 0, "Genesis 1:1"},
	}
	for _, c := range cases {
		refs, err := ref.Parse(b, c.in, c.ctx)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		got := refs[0].Label(b)
		if got != c.want {
			t.Errorf("%q => %q, want %q", c.in, got, c.want)
		}
	}
	// Multi-reference.
	refs, err := ref.Parse(b, "john 3:16; rom 5:8", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || refs[1].Label(b) != "Romans 5:8" {
		t.Fatalf("multi refs = %+v", refs)
	}
}

func TestRejects(t *testing.T) {
	b, _ := embedded.Bible()
	for _, in := range []string{"", "not a book", "John 99:1", "John 3:999", "3:16"} {
		if refs, err := ref.Parse(b, in, 0); err == nil {
			t.Errorf("%q unexpectedly parsed: %+v", in, refs)
		}
	}
}

func TestFindBooks(t *testing.T) {
	for query, want := range map[string]string{
		"joh":  "John",
		"1 co": "1 Corinthians",
		"psa":  "Psalms",
		"rev":  "Revelation",
		"son":  "Song of Solomon",
	} {
		books := ref.FindBooks(query, 5)
		if len(books) == 0 || books[0].Name != want {
			t.Errorf("FindBooks(%q) = %v, want %s", query, books, want)
		}
	}
}

func TestKeyRoundTrip(t *testing.T) {
	r := ref.Reference{BookID: 43, Chapter: 3, Verse: 16}
	got, ok := ref.ParseKey(r.Key())
	if !ok || got != r {
		t.Fatalf("round trip: %+v %v", got, ok)
	}
}

func TestChapterAndCrossChapterRanges(t *testing.T) {
	b, err := embedded.Bible()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in        string
		label     string
		last      uint16
		verses    int
		firstRef  string
		lastRef   string
		wantError bool
	}{
		{in: "John 3-5", label: "John 3-5", last: 5, verses: 36 + 54 + 47, firstRef: "John 3:1", lastRef: "John 5:47"},
		{in: "John 3:16-5:2", label: "John 3:16-5:2", last: 5, verses: 21 + 54 + 2, firstRef: "John 3:16", lastRef: "John 5:2"},
		{in: "Ps 23:1-6", label: "Psalms 23:1-6", last: 23, verses: 6, firstRef: "Psalms 23:1", lastRef: "Psalms 23:6"},
		{in: "Jude 3", label: "Jude 1:3", last: 1, verses: 1, firstRef: "Jude 1:3", lastRef: "Jude 1:3"},
		{in: "John 5-3", wantError: true},
		{in: "John 3:16-5:99", label: "John 3:16-5:47", last: 5, verses: 21 + 54 + 47, firstRef: "John 3:16", lastRef: "John 5:47"},
	}
	for _, c := range cases {
		refs, err := ref.Parse(b, c.in, 0)
		if c.wantError {
			if err == nil {
				t.Errorf("%q unexpectedly parsed: %+v", c.in, refs)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if len(refs) != 1 {
			t.Errorf("%q: got %d references", c.in, len(refs))
			continue
		}
		r := refs[0]
		if got := r.Label(b); got != c.label {
			t.Errorf("%q label = %q, want %q", c.in, got, c.label)
		}
		if got := r.LastChapter(); got != c.last {
			t.Errorf("%q last chapter = %d, want %d", c.in, got, c.last)
		}
		verses := r.Verses(b)
		if len(verses) != c.verses {
			t.Errorf("%q has %d verses, want %d", c.in, len(verses), c.verses)
			continue
		}
		if got := verses[0].Reference(b.BookByID(verses[0].BookID)); got != c.firstRef {
			t.Errorf("%q first verse = %q, want %q", c.in, got, c.firstRef)
		}
		if got := verses[len(verses)-1].Reference(b.BookByID(verses[len(verses)-1].BookID)); got != c.lastRef {
			t.Errorf("%q last verse = %q, want %q", c.in, got, c.lastRef)
		}
	}
}

func TestVersesBounds(t *testing.T) {
	b, err := embedded.Bible()
	if err != nil {
		t.Fatal(err)
	}
	// A whole chapter.
	if got := len((ref.Reference{BookID: 43, Chapter: 3}).Verses(b)); got != 36 {
		t.Fatalf("John 3 has %d verses, want 36", got)
	}
	// An end verse beyond the chapter is clamped instead of failing.
	verses := ref.Reference{BookID: 43, Chapter: 3, Verse: 16, EndVerse: 999}.Verses(b)
	if len(verses) != 21 {
		t.Fatalf("clamped range has %d verses, want 21", len(verses))
	}
	if last := verses[len(verses)-1]; last.Chapter != 3 || last.Number != 36 {
		t.Fatalf("clamped range ends at %d:%d, want 3:36", last.Chapter, last.Number)
	}
	if got := (ref.Reference{BookID: 999, Chapter: 1}).Verses(b); got != nil {
		t.Fatalf("unknown book returned %d verses", len(got))
	}
}

func TestLabelForChapter(t *testing.T) {
	b, err := embedded.Bible()
	if err != nil {
		t.Fatal(err)
	}
	base := ref.Reference{BookID: 43, Chapter: 3}
	if got := base.LabelForChapter(b, 3, 16, 36, false); got != "John 3:16-36" {
		t.Errorf("partial = %q", got)
	}
	if got := base.LabelForChapter(b, 4, 1, 54, true); got != "John 4" {
		t.Errorf("whole = %q", got)
	}
	if got := base.LabelForChapter(b, 3, 16, 16, false); got != "John 3:16" {
		t.Errorf("single = %q", got)
	}
}
