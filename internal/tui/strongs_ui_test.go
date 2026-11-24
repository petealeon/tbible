package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/ref"
)

func TestStrongTagsAttachToTheirWords(t *testing.T) {
	text := "alpha beta gamma"
	spans := []corpus.StrongSpan{
		{Start: 0, End: 5, Number: 1},
		{Start: 6, End: 10, Number: 2},
	}
	// One italic span covers the whole verse: tags must still land after each
	// word instead of being collected at the end of the decorated segment.
	italics := []corpus.Span{{Start: 0, End: len(text)}}
	got := ansi.Strip(renderVerseRange(text, 0, len(text), italics, nil, spans, -1, true))
	want := "alpha·H0001 beta·H0002 gamma"
	if got != want {
		t.Fatalf("render = %q, want %q", got, want)
	}
}

func TestMultiwordStrongTagAppearsAfterTheWholePhrase(t *testing.T) {
	text := "the world began"
	spans := []corpus.StrongSpan{{Start: 0, End: 9, Number: 1}}
	got := ansi.Strip(renderVerseRange(text, 0, len(text), nil, nil, spans, -1, true))
	want := "the world·H0001 began"
	if got != want {
		t.Fatalf("render = %q, want %q", got, want)
	}

	// Splitting the phrase across lines must not duplicate its tag.
	first := ansi.Strip(renderVerseRange(text, 0, 4, nil, nil, spans, -1, true))
	second := ansi.Strip(renderVerseRange(text, 4, len(text), nil, nil, spans, -1, true))
	if strings.Contains(first, "H0001") {
		t.Fatalf("tag appeared before the phrase ended: %q", first)
	}
	if strings.Count(second, "H0001") != 1 {
		t.Fatalf("tag count after split = %d in %q", strings.Count(second, "H0001"), second)
	}
}

func TestActiveStrongTagIsStyled(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	text := "alpha beta"
	spans := []corpus.StrongSpan{
		{Start: 0, End: 5, Number: 1},
		{Start: 6, End: 10, Number: 2},
	}
	inactive := renderVerseRange(text, 0, len(text), nil, nil, spans, -1, true)
	active := renderVerseRange(text, 0, len(text), nil, nil, spans, 1, true)
	if inactive == active {
		t.Fatal("active Strong's tag was not highlighted")
	}
	if !strings.Contains(active, "beta") {
		t.Fatal("active render lost the highlighted word")
	}
}

func TestVerseWrapCountsInlineTagWidth(t *testing.T) {
	text := "one two three four five six seven eight"
	start := strings.Index(text, "three")
	spans := []corpus.StrongSpan{{Start: start, End: start + 5, Number: 5}}
	const width = 12

	var joined []string
	for _, rg := range wrapVerseRanges(text, width, spans, true) {
		line := ansi.Strip(renderVerseRange(text, rg.Start, rg.End, nil, nil, spans, -1, true))
		if w := displayWidth(line); w > width {
			t.Errorf("line %q is %d cells, want <= %d", line, w, width)
		}
		joined = append(joined, line)
	}
	if got := strings.Join(joined, " "); !strings.Contains(got, "three·H0005") {
		t.Fatalf("tagged word was lost: %q", got)
	}
}

func TestStrongWordCursorAndDictionary(t *testing.T) {
	m := testModel(t)
	m.openReference(ref.Reference{BookID: 43, Chapter: 3, Verse: 16})

	m.handleReaderKey(keyMsg("s"))
	if !m.showStrongs {
		t.Fatal("s did not enable Strong's tags")
	}
	spans, idx, v, ok := m.focusedStrongs()
	if !ok || idx != 0 {
		t.Fatalf("focusedStrongs = %v, %d, %v", spans, idx, ok)
	}

	m.handleReaderKey(keyMsg("w"))
	spans, idx, v, ok = m.focusedStrongs()
	if !ok || idx != 1 {
		t.Fatalf("w did not move to the next word: %d, %v", idx, ok)
	}
	if summary := strings.Join(m.strongsSummary(v, 80), "\n"); !strings.Contains(summary, strongWord(v, spans[1])) {
		t.Fatalf("summary did not follow the focused word: %q", summary)
	}

	m.handleReaderKey(keyMsg("W"))
	if _, idx, _, _ = m.focusedStrongs(); idx != 0 {
		t.Fatalf("W did not move to the previous word: %d", idx)
	}

	m.handleReaderKey(keyMsg("K"))
	if !m.strongsOpen || m.strongsIndex != 0 {
		t.Fatalf("K did not open the focused entry: open=%v index=%d", m.strongsOpen, m.strongsIndex)
	}
	m.handleStrongsKey(keyMsg("j"))
	if m.strongsIndex != 1 || m.strongsFocus != 1 {
		t.Fatalf("overlay navigation did not update inline focus: index=%d focus=%d", m.strongsIndex, m.strongsFocus)
	}
	m.handleStrongsKey(keyMsg("esc"))
	if m.strongsOpen {
		t.Fatal("Esc did not close the dictionary overlay")
	}
}

func TestStrongFocusNeedsTaggedVerse(t *testing.T) {
	// Mark 9:43 is one of the few KJV verses with no Strong's spans, which
	// exercises the same guard as an installed translation without tags.
	m := testModel(t)
	m.openReference(ref.Reference{BookID: 41, Chapter: 9, Verse: 43})
	m.handleReaderKey(keyMsg("s"))

	m.handleReaderKey(keyMsg("w"))
	if !strings.Contains(m.status, "No Strong's numbers") {
		t.Fatalf("w status = %q", m.status)
	}
	m.handleReaderKey(keyMsg("K"))
	if !m.statusErr || !strings.Contains(m.status, "No Strong's numbers") {
		t.Fatalf("K status = %q, err = %v", m.status, m.statusErr)
	}
}

func TestStrongFocusResetsWhenVerseChanges(t *testing.T) {
	m := testModel(t)
	m.openReference(ref.Reference{BookID: 43, Chapter: 3, Verse: 16})
	m.handleReaderKey(keyMsg("s"))
	m.handleReaderKey(keyMsg("w"))
	if _, idx, _, _ := m.focusedStrongs(); idx != 1 {
		t.Fatalf("setup focus = %d", idx)
	}

	m.handleReaderKey(keyMsg("j"))
	if _, idx, _, ok := m.focusedStrongs(); !ok || idx != 0 {
		t.Fatalf("focus did not reset on verse change: %d, %v", idx, ok)
	}
}
