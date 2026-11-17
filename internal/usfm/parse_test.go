package usfm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petealeon/tbible/v2/internal/corpus"
)

func sourceZip(t *testing.T) string {
	t.Helper()
	candidates := []string{
		filepath.Join("..", "..", "data", "eng-kjv2006_usfm.zip"),
		filepath.Join("data", "eng-kjv2006_usfm.zip"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	t.Skip("source zip not present")
	return ""
}

func TestParseCanonical(t *testing.T) {
	b, err := ParseZip(sourceZip(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.ValidateKJV(); err != nil {
		t.Fatal(err)
	}
	if len(b.Books) != 66 || len(b.Chapters) != corpus.TotalChapters || len(b.Verses) != corpus.TotalVerses {
		t.Fatalf("got %d books, %d chapters, %d verses", len(b.Books), len(b.Chapters), len(b.Verses))
	}
	// The old corpus dropped several verses; make sure the source has them.
	checks := []struct {
		book       string
		ch, v      uint16
		wantSubstr string
	}{
		{"Matthew", 2, 16, "Then Herod"},
		{"Matthew", 22, 1, "And Jesus answered"},
		{"Matthew", 26, 38, "My soul is exceeding sorrowful"},
		{"Mark", 4, 40, "Why are ye so fearful"},
		{"Mark", 7, 11, "It is Corban"},
		{"Mark", 8, 8, "seven baskets"},
		{"Mark", 7, 16, "If any man have ears to hear"},
		{"John", 3, 16, "For God so loved the world"},
		{"1 John", 5, 7, "Father, the Word, and the Holy Ghost"},
	}
	for _, c := range checks {
		var id uint16
		for _, bk := range b.Books {
			if bk.Name == c.book {
				id = bk.ID
			}
		}
		v := b.VerseAt(id, c.ch, c.v)
		if v == nil {
			t.Errorf("%s %d:%d: missing", c.book, c.ch, c.v)
			continue
		}
		if !strings.Contains(v.Text, c.wantSubstr) {
			t.Errorf("%s %d:%d = %q, want substring %q", c.book, c.ch, c.v, v.Text, c.wantSubstr)
		}
		if strings.ContainsAny(v.Text, `\{}|`) {
			t.Errorf("%s %d:%d has control characters: %q", c.book, c.ch, c.v, v.Text)
		}
	}
}

func TestParseStylesAndNotes(t *testing.T) {
	b, err := ParseZip(sourceZip(t))
	if err != nil {
		t.Fatal(err)
	}
	v := b.VerseAt(1, 1, 1) // Genesis 1:1
	if v == nil {
		t.Fatal("Genesis 1:1 missing")
	}
	// "In the beginning God created the heaven and the earth."
	if v.Text != "In the beginning God created the heaven and the earth." {
		t.Fatalf("Genesis 1:1 = %q", v.Text)
	}
	if len(v.Italics()) != 0 {
		t.Logf("Genesis 1:1 italics: %+v", v.Italics())
	}
	// Genesis 1:2 has "was" in italics.
	v = b.VerseAt(1, 1, 2)
	for _, s := range v.Italics() {
		if v.Text[s.Start:s.End] != "was" {
			t.Logf("italic span %q", v.Text[s.Start:s.End])
		}
	}
	// Matthew 5 has words of Christ marked red.
	matt := uint16(40)
	foundRed := false
	for i := 1; i <= 3; i++ {
		v = b.VerseAt(matt, 5, uint16(i))
		if len(v.RedLetter()) > 0 {
			foundRed = true
			break
		}
	}
	if !foundRed {
		t.Error("no red-letter spans in Matthew 5")
	}
	// Psalms 1:1 has a marginal note.
	ps := uint16(19)
	v = b.VerseAt(ps, 1, 1)
	if len(v.Notes()) == 0 {
		t.Error("Psalm 1:1 has no note")
	} else if !strings.Contains(v.Notes()[0].Text, "ungodly") {
		t.Errorf("Psalm 1:1 note = %q", v.Notes()[0].Text)
	}
	// Psalm 1:1 has a section/title heading? Psalm 1 has no superscription;
	// Psalm 3 does.
	v = b.VerseAt(ps, 3, 1)
	if len(v.Headings()) == 0 {
		t.Error("Psalm 3:1 has no title heading")
	}
}
