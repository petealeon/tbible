package tui

import (
	"strings"
	"testing"

	"github.com/petealeon/tbible/v2/internal/embedded"
	"github.com/petealeon/tbible/v2/internal/library"
	"github.com/petealeon/tbible/v2/internal/ref"
)

func TestPassageSelectionStaysStickyForActions(t *testing.T) {
	m := testModel(t)
	m.openReference(ref.Reference{BookID: 43, Chapter: 3, Verse: 16})

	m.handleReaderKey(keyMsg("V"))
	if !m.selecting {
		t.Fatal("V did not start a selection")
	}
	m.handleReaderKey(keyMsg("l")) // John 4:16
	m.handleReaderKey(keyMsg("V"))
	if m.selecting {
		t.Fatal("second V did not finish the selection")
	}

	r, ok := m.readerSelection()
	if !ok {
		t.Fatal("finished selection was not retained")
	}
	if r.BookID != 43 || r.Chapter != 3 || r.Verse != 16 || r.EndChapter != 4 || r.EndVerse != 1 {
		t.Fatalf("cross-chapter selection = %+v", r)
	}
	_, verses, ok := m.focusedRange()
	if !ok || len(verses) < 2 {
		t.Fatalf("focused range = %d verses, %v", len(verses), ok)
	}

	// A copy consumes the range but must still use it before clearing.
	m.handleReaderKey(keyMsg("y"))
	if m.selAnchor.BookID != 0 || m.selHead.BookID != 0 {
		t.Fatal("copy did not clear the selection")
	}
}

func TestReaderMoveDiscardsStaleSelection(t *testing.T) {
	m := testModel(t)
	m.openReference(ref.Reference{BookID: 43, Chapter: 3, Verse: 16})
	m.handleReaderKey(keyMsg("V"))
	m.handleReaderKey(keyMsg("V"))
	if m.selAnchor.BookID == 0 {
		t.Fatal("selection was cleared too early")
	}
	m.handleReaderKey(keyMsg("j"))
	if m.selAnchor.BookID != 0 || m.selecting {
		t.Fatal("moving after a finished selection kept a stale range")
	}
	if _, ok := m.readerSelection(); ok {
		t.Fatal("readerSelection returned a stale range")
	}
}

func TestSelectionCannotCrossBooks(t *testing.T) {
	m := testModel(t)
	m.openReference(ref.Reference{BookID: 43, Chapter: 3, Verse: 16})
	m.handleReaderKey(keyMsg("V"))
	m.handleReaderKey(keyMsg("}")) // next book: Romans
	if m.selAnchor.BookID != 0 || m.selecting {
		t.Fatal("selection survived a book change")
	}
	if _, ok := m.readerSelection(); ok {
		t.Fatal("readerSelection returned a cross-book range")
	}
}

func TestNoteUsesSelectionAsContext(t *testing.T) {
	m := testModel(t)
	m.openReference(ref.Reference{BookID: 43, Chapter: 3, Verse: 16})
	m.handleReaderKey(keyMsg("V"))
	m.handleReaderKey(keyMsg("l"))
	m.handleReaderKey(keyMsg("V"))
	m.handleReaderKey(keyMsg("n"))
	if !m.noteOpen {
		t.Fatal("n did not open the note editor")
	}
	if !strings.Contains(m.noteRef, "John 3:16") || !strings.Contains(m.noteRef, "4:1") {
		t.Fatalf("note context = %q", m.noteRef)
	}
	m.noteInput.SetValue("range note")
	m.handleNoteKey(keyMsg("ctrl+s"))
	if m.noteOpen || m.selAnchor.BookID != 0 {
		t.Fatal("saving a note did not close the editor and clear the selection")
	}
}

func TestCleanParallelIDs(t *testing.T) {
	got := cleanParallelIDs([]string{"KJV", "web", "WEB", "asv", "ylt", "wmb"}, "kjv")
	want := []string{"web", "asv", "ylt"}
	if len(got) != len(want) {
		t.Fatalf("cleanParallelIDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cleanParallelIDs = %v, want %v", got, want)
		}
	}
}

func TestParallelRenderingAndCopyLabels(t *testing.T) {
	m := testModel(t)
	other, err := embedded.Bible()
	if err != nil {
		t.Fatal(err)
	}
	m.translationID = library.KJVId
	m.parallel = []parallelView{{id: "web", name: "World English Bible", bible: other}}

	r := ref.Reference{BookID: 43, Chapter: 3, Verse: 16}
	verses := r.Verses(m.bible)
	if len(verses) != 1 {
		t.Fatalf("test reference yielded %d verses", len(verses))
	}
	lines := m.parallelVerseLines(verses[0], 100)
	if len(lines) == 0 || !strings.Contains(lines[0], "WEB") {
		t.Fatalf("parallel lines = %q", lines)
	}
	text := m.copyRangeText(r, verses)
	if !strings.Contains(text, "KJV:") || !strings.Contains(text, "WEB:") {
		t.Fatalf("parallel copy text = %q", text)
	}
}
