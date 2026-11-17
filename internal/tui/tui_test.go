package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/petealeon/tbible/v2/internal/activity"
	"github.com/petealeon/tbible/v2/internal/embedded"
	"github.com/petealeon/tbible/v2/internal/heatmap"
	"github.com/petealeon/tbible/v2/internal/ref"
	"github.com/petealeon/tbible/v2/internal/search"
	"github.com/petealeon/tbible/v2/internal/store"
)

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func testRowBookID(r row) uint16 {
	if r.ref.BookID != 0 {
		return r.ref.BookID
	}
	if parsed, ok := ref.ParseKey(r.key); ok {
		return parsed.BookID
	}
	return 0
}

func testModel(t *testing.T) *model {
	t.Helper()
	b, err := embedded.Bible()
	if err != nil {
		t.Fatal(err)
	}
	ix, err := embedded.Index()
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.LoadPath(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(b, ix, st)
	m.width, m.height = 120, 36
	return m
}

func TestReferenceSearch(t *testing.T) {
	m := testModel(t)
	m.input.SetValue("John 3:16")
	m.rebuild()

	if m.interpretation == "" || !strings.Contains(m.interpretation, "John 3:16") {
		t.Fatalf("interpretation = %q", m.interpretation)
	}
	r, ok := m.focusedReference()
	if !ok || r.BookID != 43 || r.Chapter != 3 || r.Verse != 16 {
		t.Fatalf("focused reference = %+v, %v", r, ok)
	}
	if len(m.resultLines(m.width)) == 0 {
		t.Fatal("reference search rendered no lines")
	}
}

func TestKeywordSearch(t *testing.T) {
	m := testModel(t)
	m.input.SetValue("shepherd")
	m.rebuild()

	verseRows := 0
	for _, r := range m.flat {
		if r.kind == rowVerse {
			verseRows++
		}
	}
	if verseRows == 0 {
		t.Fatal("keyword search returned no verses")
	}
	if m.lastResult.Total == 0 {
		t.Fatal("search total is zero")
	}
	// The selected verse must resolve to a real location.
	if _, _, ok := m.focusedVerse(); !ok {
		t.Fatal("focused verse did not resolve")
	}
	if got := m.searchView(); got == "" {
		t.Fatal("search view is empty")
	}
}

func TestReaderLayoutAndNavigation(t *testing.T) {
	m := testModel(t)
	m.openReference(ref.Reference{BookID: 43, Chapter: 3, Verse: 16})
	if m.scr != screenReader {
		t.Fatal("did not switch to reader")
	}
	if m.reader.verse != 16 {
		t.Fatalf("reader verse = %d", m.reader.verse)
	}
	width := m.readerContentWidth()
	lines, verseLine := m.readerLayout(width)
	if len(lines) == 0 {
		t.Fatal("reader layout is empty")
	}
	if _, ok := verseLine[16]; !ok {
		t.Fatal("cursor verse missing from layout")
	}
	if m.readerView() == "" {
		t.Fatal("reader view is empty")
	}

	// j/k moves the cursor and chapter stepping crosses chapters.
	m.handleReaderKey(keyMsg("j"))
	if m.reader.verse != 17 {
		t.Fatalf("after j: verse = %d", m.reader.verse)
	}
	m.handleReaderKey(keyMsg("k"))
	if m.reader.verse != 16 {
		t.Fatalf("after k: verse = %d", m.reader.verse)
	}
	m.chapterStep(1)
	if m.reader.bookID != 43 || m.reader.chapter != 4 || m.reader.verse != 1 {
		t.Fatalf("next chapter = %d %d:%d", m.reader.bookID, m.reader.chapter, m.reader.verse)
	}
	m.reader.openAt(m.bible, ref.Reference{BookID: 43, Chapter: 21, Verse: 1})
	m.chapterStep(1)
	if m.reader.bookID != 44 || m.reader.chapter != 1 {
		t.Fatalf("cross-book step = %d:%d", m.reader.bookID, m.reader.chapter)
	}
}

func TestBookmarksNotesAndLibrary(t *testing.T) {
	m := testModel(t)
	m.openReference(ref.Reference{BookID: 43, Chapter: 3, Verse: 16})
	m.toggleBookmark()
	if !m.store.IsBookmarked("43:3:16") {
		t.Fatal("bookmark was not stored")
	}
	m.beginNote()
	m.noteInput.SetValue("A test note")
	m.handleNoteKey(keyMsg("ctrl+s"))
	if n, ok := m.store.Note("43:3:16"); !ok || n.Body != "A test note" {
		t.Fatalf("note = %+v, %v", n, ok)
	}

	m.runCommand("library")
	if m.scr != screenLibrary || len(m.library.rows) == 0 {
		t.Fatalf("library rows = %d", len(m.library.rows))
	}
	if m.libraryView() == "" {
		t.Fatal("library view is empty")
	}

	m.library.filter = libNotes
	m.rebuildLibrary()
	if len(m.library.rows) != 1 || m.library.rows[0].kind != rowNote {
		t.Fatalf("notes filter = %+v", m.library.rows)
	}
	m.library.filter = libSaved
	m.rebuildLibrary()
	if len(m.library.rows) != 0 {
		t.Fatalf("saved filter should be empty, got %+v", m.library.rows)
	}
}

func TestPaletteAndHelp(t *testing.T) {
	m := testModel(t)
	m.openPalette()
	m.paletteInput.SetValue("note")
	matches := m.commandMatches()
	if len(matches) == 0 || matches[0].id != "note" {
		t.Fatalf("palette matches = %+v", matches)
	}
	if m.paletteView() == "" {
		t.Fatal("palette view is empty")
	}
	m.closePalette()

	m.help = true
	if m.helpView() == "" {
		t.Fatal("help view is empty")
	}
	m.handleHelpKey(keyMsg("G"))
	if m.helpScroll == 0 {
		t.Fatal("help did not scroll to the bottom")
	}
	m.handleHelpKey(keyMsg("esc"))
	if m.help {
		t.Fatal("help did not close")
	}
}

func TestShortcutsFromEmptyOmnibox(t *testing.T) {
	m := testModel(t)
	m.input.SetValue("")
	m.input.Focus()

	m.handleSearchKey(keyMsg("?"))
	if !m.help {
		t.Fatal("? did not open help from an empty omnibox")
	}
	m.help = false

	m.handleSearchKey(keyMsg(":"))
	if !m.palette {
		t.Fatal(": did not open the palette from an empty omnibox")
	}
	m.closePalette()

	// With a query in progress these characters belong to the query.
	m.input.SetValue("faith")
	m.handleSearchKey(keyMsg("?"))
	if m.help {
		t.Fatal("? opened help while typing a query")
	}
	if !strings.Contains(m.input.Value(), "?") {
		t.Fatalf("? was not inserted into the query: %q", m.input.Value())
	}
}

func TestCtrlSSavesQueryWhileTyping(t *testing.T) {
	m := testModel(t)
	m.input.SetValue("faith")
	m.input.Focus()
	m.handleSearchKey(keyMsg("ctrl+s"))
	if got := m.store.SavedSearches(); len(got) != 1 || got[0].Query != "faith" {
		t.Fatalf("saved searches = %+v", got)
	}
}

func TestSavedSearchRoundTrip(t *testing.T) {
	m := testModel(t)
	m.input.SetValue(`"still waters"`)
	m.rebuild()
	m.saveSearch()

	m.library.filter = libSaved
	m.rebuildLibrary()
	if len(m.library.rows) != 1 {
		t.Fatalf("saved search rows = %+v", m.library.rows)
	}
	m.library.sel = 0
	m.activateLibrary()
	if m.input.Value() != `"still waters"` {
		t.Fatalf("restored query = %q", m.input.Value())
	}
}

func TestFocusedRange(t *testing.T) {
	m := testModel(t)
	m.scr = screenSearch
	m.flat = []row{
		{kind: rowChapter, ref: ref.Reference{BookID: 43, Chapter: 3}},
		{kind: rowRef, ref: ref.Reference{BookID: 43, Chapter: 3, Verse: 16, EndVerse: 18}},
		{kind: rowRef, ref: ref.Reference{BookID: 43, Chapter: 3, Verse: 16}},
		{kind: rowVerse, ref: ref.Reference{BookID: 43, Chapter: 3, Verse: 16, EndVerse: 18}},
	}

	m.sel = 0
	r, verses, ok := m.focusedRange()
	if !ok || r.Label(m.bible) != "John 3" || len(verses) != 36 {
		t.Fatalf("chapter range = %v, %d verses, %v", r.Label(m.bible), len(verses), ok)
	}

	m.sel = 1
	r, verses, ok = m.focusedRange()
	if !ok || r.Label(m.bible) != "John 3:16-18" || len(verses) != 3 {
		t.Fatalf("verse range = %v, %d verses, %v", r.Label(m.bible), len(verses), ok)
	}

	m.sel = 2
	if _, _, ok := m.focusedRange(); ok {
		t.Fatal("a single verse was treated as a range")
	}
	m.sel = 3
	if _, _, ok := m.focusedRange(); ok {
		t.Fatal("a verse row was treated as a range")
	}

	// Range bookmarks add every verse and remove them as a set.
	m.sel = 1
	m.toggleBookmark()
	for n := 16; n <= 18; n++ {
		if !m.store.IsBookmarked(ref.Reference{BookID: 43, Chapter: 3, Verse: uint16(n)}.Key()) {
			t.Fatalf("John 3:%d was not bookmarked", n)
		}
	}
	if got := len(m.store.Bookmarks()); got != 3 {
		t.Fatalf("bookmarks = %d, want 3", got)
	}
	m.toggleBookmark()
	if got := len(m.store.Bookmarks()); got != 0 {
		t.Fatalf("bookmarks after removal = %d", got)
	}
}

func TestSavedSearchRestoresScope(t *testing.T) {
	m := testModel(t)
	m.scope = search.ScopeNT
	m.input.SetValue("faith")
	m.rebuild()
	m.saveSearch()

	if got := m.store.SavedSearches(); len(got) != 1 || got[0].Scope != store.ScopeNT {
		t.Fatalf("saved = %+v", got)
	}

	m.scope = search.ScopeAll
	m.library.filter = libSaved
	m.rebuildLibrary()
	if len(m.library.rows) != 1 {
		t.Fatalf("saved rows = %+v", m.library.rows)
	}
	if got := m.library.rows[0].text; !strings.Contains(got, "New Testament") {
		t.Fatalf("saved row scope = %q", got)
	}
	m.library.sel = 0
	m.activateLibrary()
	if m.scope != search.ScopeNT {
		t.Fatalf("restored scope = %v, want NT", m.scope)
	}
	if m.input.Value() != "faith" {
		t.Fatalf("restored query = %q", m.input.Value())
	}
}

func TestRelaxedSearchSummary(t *testing.T) {
	m := testModel(t)
	m.input.SetValue("shepherd zerubbabel")
	m.rebuild()

	if !m.lastResult.Relaxed || m.lastResult.Total == 0 {
		t.Fatalf("relaxed=%v total=%d", m.lastResult.Relaxed, m.lastResult.Total)
	}
	if !strings.Contains(m.interpretation, "relaxed") {
		t.Fatalf("interpretation = %q", m.interpretation)
	}
	if !strings.Contains(m.interpretation, "verses") {
		t.Fatalf("interpretation has no result count: %q", m.interpretation)
	}
	if m.searchView() == "" {
		t.Fatal("search view is empty")
	}
}

func TestReaderShowsUserNotesDistinctly(t *testing.T) {
	m := testModel(t)
	if err := m.store.SetNote("43:3:16", "John 3:16", "Remember this verse"); err != nil {
		t.Fatal(err)
	}
	m.input.SetValue("John 3:16")
	m.rebuild()
	if preview := strings.Join(m.previewLines(80, 20), "\n"); !strings.Contains(preview, "Remember this verse") {
		t.Fatalf("preview note = %q", preview)
	}

	m.openReference(ref.Reference{BookID: 43, Chapter: 3, Verse: 16})
	m.showFootnotes = true
	m.reader.showNotes = true

	lines, _ := m.readerLayout(m.readerContentWidth())
	joined := strings.Join(linesOfReader(lines), "\n")
	if !strings.Contains(joined, "Your note") || !strings.Contains(joined, "Remember this verse") {
		t.Fatalf("reader notes = %q", joined)
	}

	m.reader.showNotes = false
	lines, _ = m.readerLayout(m.readerContentWidth())
	if joined := strings.Join(linesOfReader(lines), "\n"); strings.Contains(joined, "Your note") {
		t.Fatalf("notes remained visible when toggled off: %q", joined)
	}
}

func linesOfReader(lines []readerLine) []string {
	out := make([]string, len(lines))
	for i := range lines {
		out[i] = lines[i].text
	}
	return out
}

func TestActionHintsAreExplicit(t *testing.T) {
	for _, width := range []int{80, 120} {
		hint := actionHints(width)
		if strings.Contains(hint, "y/b/n") {
			t.Fatalf("width %d hint = %q", width, hint)
		}
		if !strings.Contains(hint, "y copy") || !strings.Contains(hint, "n note") {
			t.Fatalf("width %d hint = %q", width, hint)
		}
	}

	m := testModel(t)
	m.openReference(ref.Reference{BookID: 43, Chapter: 3, Verse: 16})
	m.input.SetValue("shepherd")
	m.rebuild()
	for name, view := range map[string]string{
		"reader":  m.readerView(),
		"search":  m.searchView(),
		"library": m.libraryView(),
		"help":    m.helpView(),
	} {
		if strings.Contains(view, "y/b/n") {
			t.Fatalf("%s view still uses y/b/n: %q", name, view)
		}
	}
	if !strings.Contains(m.helpView(), "Copy the selected verse") {
		t.Fatal("help does not spell out the copy action")
	}
}

func TestHeatmapViewAndNavigation(t *testing.T) {
	m := testModel(t)
	m.input.SetValue("shepherd")
	m.rebuild()
	if len(m.lastResult.AllHits) == 0 {
		t.Fatal("search returned no all hits")
	}

	m.resultsFocused = true
	m.handleSearchKey(keyMsg("H"))
	if !m.heat.open || m.heat.report == nil {
		t.Fatal("heatmap did not open")
	}
	if m.heat.report.Total != m.lastResult.Total {
		t.Fatalf("heatmap total = %d, search total = %d", m.heat.report.Total, m.lastResult.Total)
	}
	if m.heat.mode != heatmap.ModeDensity {
		t.Fatalf("initial mode = %v, want density", m.heat.mode)
	}
	view := m.heatmapView()
	if !strings.Contains(view, "Result heatmap") || !strings.Contains(view, "Density") {
		t.Fatalf("heatmap view = %q", view)
	}

	m.handleHeatmapKey(keyMsg("d"))
	if m.heat.mode != heatmap.ModeCount || !strings.Contains(m.heatmapView(), "Count") {
		t.Fatalf("mode toggle failed: %v", m.heat.mode)
	}

	m.handleHeatmapKey(keyMsg("j"))
	if m.heat.book != 1 {
		t.Fatalf("book selection = %d, want 1", m.heat.book)
	}
	m.handleHeatmapKey(keyMsg("enter"))
	if !m.heat.detail {
		t.Fatal("Enter did not open chapter detail")
	}
	m.handleHeatmapKey(keyMsg("enter"))
	if m.heat.open || m.scr != screenReader || m.reader.bookID != 2 {
		t.Fatalf("chapter open: open=%v screen=%v book=%d", m.heat.open, m.scr, m.reader.bookID)
	}
}

func TestHeatmapRequiresSearch(t *testing.T) {
	m := testModel(t)
	m.openHeatmap()
	if m.heat.open {
		t.Fatal("heatmap opened without a search")
	}
	if !strings.Contains(m.status, "Run a search first") {
		t.Fatalf("status = %q", m.status)
	}
}

func TestSearchWideStackedLayout(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 160, 40
	m.input.SetValue("shepherd")
	m.rebuild()

	view := m.searchView()
	previewAt := strings.Index(view, "PREVIEW")
	heatAt := strings.Index(view, "HEATMAP")
	if previewAt < 0 || heatAt < 0 || heatAt < previewAt {
		t.Fatalf("wide layout preview=%d heatmap=%d view=%q", previewAt, heatAt, view)
	}
	if !strings.Contains(view, "Density") || !strings.Contains(view, "matches") {
		t.Fatalf("inline heatmap missing summary: %q", view)
	}
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		t.Fatalf("view has %d lines for %d-row terminal", len(lines), m.height)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > m.width {
			t.Fatalf("line %d width = %d, want <= %d: %q", i, w, m.width, line)
		}
	}
}

func TestSearchWideButShortKeepsPreviewOnly(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 160, 24
	m.input.SetValue("shepherd")
	m.rebuild()

	view := m.searchView()
	if !strings.Contains(view, "PREVIEW") {
		t.Fatal("short wide layout lost the preview")
	}
	if strings.Contains(view, "HEATMAP") {
		t.Fatal("short wide layout showed the inline heatmap")
	}
	if m.heat.report != nil {
		t.Fatal("short wide layout built a heatmap report")
	}
}

func TestInlineHeatmapFollowsSelection(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 160, 40
	m.input.SetValue("shepherd")
	m.rebuild()
	m.syncHeatmap()
	if m.heat.report == nil {
		t.Fatal("no heatmap report")
	}

	firstBook := uint16(0)
	for _, r := range m.flat {
		if id := testRowBookID(r); id != 0 {
			firstBook = id
			break
		}
	}
	moveTo := -1
	var otherBook uint16
	for i, r := range m.flat {
		if id := testRowBookID(r); id != 0 && id != firstBook {
			moveTo, otherBook = i, id
			break
		}
	}
	if moveTo < 0 {
		t.Fatal("search did not span two books")
	}
	m.sel = moveTo
	m.resultsFocused = true

	want := -1
	for i := range m.heat.report.Books {
		if m.heat.report.Books[i].ID == otherBook {
			want = i
			break
		}
	}
	if got := m.inlineHeatmapBook(); got != want {
		t.Fatalf("inline book = %d, want %d", got, want)
	}
	joined := strings.Join(m.inlineHeatmapLines(60, 10), "\n")
	if !strings.Contains(joined, m.heat.report.Books[want].Abbrev) {
		t.Fatalf("inline heatmap did not show book %d: %q", otherBook, joined)
	}
}

func TestHeatmapReportIsCached(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 160, 40
	m.input.SetValue("shepherd")
	m.rebuild()
	m.syncHeatmap()
	first := m.heat.report
	if first == nil {
		t.Fatal("no heatmap report")
	}
	m.syncHeatmap()
	if m.heat.report != first {
		t.Fatal("unchanged result set rebuilt the heatmap")
	}

	m.input.SetValue("love")
	m.rebuild()
	m.syncHeatmap()
	if m.heat.report == first {
		t.Fatal("new result set reused the old heatmap")
	}
}

func seedCalendar(t *testing.T) *model {
	t.Helper()
	m := testModel(t)
	now := time.Now()
	if err := m.store.SetNote("43:3:16", "John 3:16", "calendar note"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.store.ToggleBookmark(store.VerseItem{
		Key: "23:1:1", Ref: "Psalms 23:1", Text: "The LORD is my shepherd", At: now,
	}); err != nil {
		t.Fatal(err)
	}
	m.store.TouchHistory(store.VerseItem{
		Key: "1:1:1", Ref: "Genesis 1:1", Text: "In the beginning", At: now,
	})
	m.calendar.selected = localDay(now)
	m.setLibraryFilter(libCalendar)
	return m
}

func TestCalendarViewAndNavigation(t *testing.T) {
	m := seedCalendar(t)
	if m.library.filter != libCalendar {
		t.Fatalf("filter = %v, want Calendar", m.library.filter)
	}
	if len(m.library.rows) != 3 {
		t.Fatalf("calendar day rows = %d, want 3", len(m.library.rows))
	}
	view := m.calendarView()
	for _, want := range []string{"CALENDAR", "ENTRIES", "Note", "Bookmark", "History", "calendar note"} {
		if !strings.Contains(view, want) {
			t.Fatalf("calendar view missing %q: %q", want, view)
		}
	}

	m.handleCalendarKey(keyMsg("2"))
	if m.calendar.source != activity.Note {
		t.Fatalf("source = %v, want Note", m.calendar.source)
	}
	if len(m.library.rows) != 1 || m.library.rows[0].kind != rowNote {
		t.Fatalf("notes-only rows = %+v", m.library.rows)
	}

	m.handleCalendarKey(keyMsg("1"))
	monthYear := time.Date(m.calendar.year, m.calendar.month, 1, 0, 0, 0, 0, time.Local)
	wantNext := monthYear.AddDate(0, 1, 0)
	m.handleCalendarKey(keyMsg("]"))
	if m.calendar.year != wantNext.Year() || m.calendar.month != wantNext.Month() {
		t.Fatalf("after ] = %d-%s, want %d-%s", m.calendar.year, m.calendar.month, wantNext.Year(), wantNext.Month())
	}
	m.handleCalendarKey(keyMsg("["))
	if m.calendar.year != monthYear.Year() || m.calendar.month != monthYear.Month() {
		t.Fatalf("after [ = %d-%s", m.calendar.year, m.calendar.month)
	}

	before := m.calendar.selected
	m.handleCalendarKey(keyMsg("right"))
	if sameDay(before, m.calendar.selected) {
		t.Fatal("right did not move to the next day")
	}
	m.handleCalendarKey(keyMsg("left"))
	if !sameDay(before, m.calendar.selected) {
		t.Fatal("left did not return to the original day")
	}

	m.handleCalendarKey(keyMsg("esc"))
	if m.library.filter != libAll {
		t.Fatalf("Esc returned to %v, want All", m.library.filter)
	}
}

func TestCalendarInlineFilter(t *testing.T) {
	m := seedCalendar(t)
	m.handleCalendarKey(keyMsg("f"))
	if !m.calendar.filterEditing {
		t.Fatal("f did not open the calendar filter")
	}
	for _, r := range "calendar note" {
		m.handleCalendarKey(keyMsg(string(r)))
	}
	m.handleCalendarKey(keyMsg("enter"))
	if m.calendar.filterEditing {
		t.Fatal("Enter did not close the calendar filter")
	}
	if m.calendar.filter != "calendar note" {
		t.Fatalf("filter = %q", m.calendar.filter)
	}
	if len(m.library.rows) != 1 || !strings.Contains(m.library.rows[0].text, "calendar note") {
		t.Fatalf("filtered rows = %+v", m.library.rows)
	}
	if !strings.Contains(m.calendarView(), "Filter: calendar note") {
		t.Fatal("filter was not shown in the calendar view")
	}
	m.handleCalendarKey(keyMsg("esc"))
	if m.calendar.filter != "" {
		t.Fatalf("Esc did not clear filter: %q", m.calendar.filter)
	}
}

func TestCalendarOpensEntry(t *testing.T) {
	m := seedCalendar(t)
	m.calendar.focus = calFocusEntries
	for i, r := range m.library.rows {
		if r.kind == rowNote {
			m.library.sel = i
			break
		}
	}
	if cmd := m.handleCalendarKey(keyMsg("enter")); cmd == nil && m.scr == screenLibrary {
		t.Fatal("Enter did not open the selected calendar entry")
	}
	if m.scr != screenReader || m.reader.bookID != 43 || m.reader.verse != 16 {
		t.Fatalf("reader = screen %v, %d:%d", m.scr, m.reader.bookID, m.reader.verse)
	}
}

func TestCalendarWideLayout(t *testing.T) {
	m := seedCalendar(t)
	m.width, m.height = 160, 40
	view := m.calendarView()
	if !strings.Contains(view, "CALENDAR") || !strings.Contains(view, "ENTRIES") {
		t.Fatalf("wide calendar view = %q", view)
	}
	if !strings.Contains(view, "│") {
		t.Fatal("wide calendar did not use the two-column layout")
	}
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		t.Fatalf("view has %d lines for %d-row terminal", len(lines), m.height)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > m.width {
			t.Fatalf("line %d width = %d, want <= %d: %q", i, w, m.width, line)
		}
	}
}

func TestCalendarCommandPalette(t *testing.T) {
	m := testModel(t)
	m.runCommand("calendar")
	if m.scr != screenLibrary || m.library.filter != libCalendar {
		t.Fatalf("calendar command: screen=%v filter=%v", m.scr, m.library.filter)
	}
	if !strings.Contains(m.calendarView(), "CALENDAR") {
		t.Fatal("calendar command produced an empty view")
	}
}

func TestStoreWarningShown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := embedded.Bible()
	if err != nil {
		t.Fatal(err)
	}
	ix, err := embedded.Index()
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(b, ix, st)
	if m.status == "" || !m.statusErr || !strings.Contains(m.status, "empty library") {
		t.Fatalf("status = %q (err=%v)", m.status, m.statusErr)
	}
}

func TestRecordHistoryIsDebounced(t *testing.T) {
	m := testModel(t)
	cmd := m.recordHistory(ref.Reference{BookID: 43, Chapter: 3, Verse: 16})
	if cmd == nil || !m.persistPending {
		t.Fatalf("first record: cmd=%v pending=%v", cmd, m.persistPending)
	}
	if _, err := os.Stat(m.store.Path()); !os.IsNotExist(err) {
		t.Fatalf("recordHistory wrote immediately: %v", err)
	}
	if got := m.store.History(); len(got) != 1 || got[0].Key != "43:3:16" {
		t.Fatalf("history = %+v", got)
	}

	if cmd := m.recordHistory(ref.Reference{BookID: 43, Chapter: 3, Verse: 17}); cmd != nil {
		t.Fatal("second record started another timer")
	}
	if got := m.store.History(); len(got) != 2 || got[0].Key != "43:3:17" {
		t.Fatalf("history after second record = %+v", got)
	}

	if err := m.store.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.LoadPath(m.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.History(); len(got) != 2 {
		t.Fatalf("persisted history = %+v", got)
	}
}
