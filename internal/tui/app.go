// Package tui implements the keyboard-driven terminal interface.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/petealeon/tbible/v2/internal/catalog"
	"github.com/petealeon/tbible/v2/internal/clipboard"
	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/embedded"
	"github.com/petealeon/tbible/v2/internal/library"
	"github.com/petealeon/tbible/v2/internal/ref"
	"github.com/petealeon/tbible/v2/internal/search"
	"github.com/petealeon/tbible/v2/internal/store"
	"github.com/petealeon/tbible/v2/internal/strongs"
	"github.com/petealeon/tbible/v2/internal/version"
)

type screen int

const (
	screenSearch screen = iota
	screenReader
	screenLibrary
)

type model struct {
	bible *corpus.Bible
	index *search.Index
	store *store.Store

	// packLib resolves the embedded KJV plus installed data-only packs.
	packLib       *library.Library
	cat           *catalog.Catalog
	translationID string
	parallelIDs   []string
	parallel      []parallelView

	transOpen bool
	transSel  int
	transRows []transRow
	transBusy bool

	width, height int
	scr           screen

	input          textinput.Model
	resultsFocused bool
	query          string
	sections       []searchSection
	flat           []row
	sel            int
	scroll         int
	rowLine        []int
	totalLines     int

	scope          search.Scope
	scopeBook      uint16
	currentBook    uint16
	interpretation string
	matchQuery     search.Query
	lastResult     search.Result
	persistPending bool

	reader   readerModel
	library  libraryModel
	calendar calendarModel
	heat     heatmapModel

	help       bool
	helpScroll int

	palette      bool
	paletteInput textinput.Model
	paletteSel   int

	noteOpen  bool
	noteInput textarea.Model
	noteKey   string
	noteRef   string

	status    string
	statusErr bool

	showVerseNumbers bool
	showFootnotes    bool
	showStrongs      bool
	sidebar          bool

	// hidden blanks the screen instantly for readers who need to conceal it.
	// Any key restores the previous view without being interpreted.
	hidden bool

	// selecting highlights a passage chosen with V in the reader. The anchor
	// stays after V finishes so y/b/n act on the same range; Esc clears it.
	selecting       bool
	selAnchor       ref.Reference
	selHead         ref.Reference
	heatReturnFocus bool

	// Strong's state. strongsFocus is the tagged word highlighted inline in
	// the focused verse; strongsIndex is the position inside the full-screen
	// overlay, which starts where the inline focus was.
	strongsFocus      int
	strongsFocusVerse string
	strongsOpen       bool
	strongsDict       *strongs.Dictionary
	strongsSpans      []corpus.StrongSpan
	strongsText       string
	strongsVerse      string
	strongsIndex      int
	strongsScroll     int
}

type copiedMsg struct {
	label string
	err   error
}

type savedMsg struct{ err error }

// persistStateMsg is delivered by the debounce timer after history changes.
type persistStateMsg struct{}

// Options control how the TUI starts.
type options struct {
	ephemeral bool
}

// Option customises TUI startup.
type Option func(*options)

// WithEphemeral starts the TUI without reading or writing user state.
func WithEphemeral(v bool) Option {
	return func(o *options) { o.ephemeral = v }
}

// New starts the TUI.
func New(opts ...Option) error {
	var o options
	for _, apply := range opts {
		apply(&o)
	}
	b, err := embedded.Bible()
	if err != nil {
		return err
	}
	ix, err := embedded.Index()
	if err != nil {
		return err
	}
	var st *store.Store
	if o.ephemeral {
		st, err = store.LoadEphemeral()
	} else {
		st, err = store.Load()
	}
	if err != nil {
		return err
	}

	m := newModel(b, ix, st)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func newModel(b *corpus.Bible, ix *search.Index, st *store.Store) *model {
	m := &model{
		bible:         b,
		index:         ix,
		store:         st,
		packLib:       defaultLibrary(),
		translationID: library.KJVId,
	}
	m.cat, _ = catalog.Embedded()

	ti := textinput.New()
	ti.Prompt = "/ "
	ti.PromptStyle = promptStyle
	ti.Placeholder = "Search or type a reference — John 3:16, shepherd, \"still waters\""
	ti.CharLimit = 200
	ti.Focus()
	m.input = ti

	pi := textinput.New()
	pi.Prompt = "> "
	pi.PromptStyle = promptStyle
	pi.Placeholder = "Type a command"
	pi.CharLimit = 80
	m.paletteInput = pi

	ta := textarea.New()
	ta.Placeholder = "Write a note…"
	ta.ShowLineNumbers = false
	ta.CharLimit = 4000
	ta.Blur()
	m.noteInput = ta
	m.initCalendar()

	s := st.Settings()
	if s.Translation != "" && m.packLib.Has(s.Translation) {
		if nb, nx, err := m.packLib.Get(s.Translation); err == nil {
			m.bible, m.index, m.translationID = nb, nx, s.Translation
		}
	}
	m.parallelIDs = cleanParallelIDs(s.Parallel, m.translationID)
	m.loadParallel()
	m.showVerseNumbers = s.VerseNumbers
	m.showFootnotes = s.Footnotes
	m.showStrongs = s.Strongs
	m.reader.showNumbers = s.VerseNumbers
	m.reader.showNotes = s.Footnotes
	m.reader.showStrongs = s.Strongs
	m.ensureDictionary()
	if warning := st.Warning(); warning != "" {
		m.setStatus(warning, true)
	}
	m.rebuild()
	return m
}

func (m *model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.Width = max(10, m.width-14)
		m.paletteInput.Width = max(10, min(60, m.width-8))
		m.noteInput.SetWidth(max(20, min(76, m.width-10)))
		m.noteInput.SetHeight(max(4, min(14, m.height-10)))
		m.calendar.filterInput.Width = max(10, min(50, m.width-12))
		return m, nil
	case copiedMsg:
		if msg.err != nil {
			m.setStatus("Copy failed: "+msg.err.Error(), true)
		} else {
			m.setStatus("Copied "+msg.label, false)
		}
		return m, nil
	case savedMsg:
		if msg.err != nil {
			m.setStatus("Save failed: "+msg.err.Error(), true)
		}
		return m, nil
	case persistStateMsg:
		if !m.persistPending {
			return m, nil
		}
		m.persistPending = false
		st := m.store
		return m, func() tea.Msg { return savedMsg{st.Save()} }
	case catalogMsg:
		m.finishCatalogRefresh(msg)
		return m, nil
	case packMsg:
		m.finishPackAction(msg)
		return m, nil
	case tea.KeyMsg:
		m.status = ""
		if msg.String() == "ctrl+c" {
			// History and last-read writes are debounced; flush them on quit.
			_ = m.store.Save()
			return m, tea.Quit
		}
		if msg.String() == "ctrl+x" {
			m.hidden = !m.hidden
			return m, nil
		}
		if m.hidden {
			// Swallow the key that restores the screen.
			m.hidden = false
			return m, nil
		}
		// The heatmap is reachable while typing in the omnibox, where H is
		// ordinary text. Ctrl-H works regardless of focus; F2 is an alias.
		if key := msg.String(); key == "ctrl+h" || key == "f2" {
			if m.heat.open {
				return m, m.closeHeatmap()
			}
			if m.scr == screenSearch && !m.resultsFocused && strings.TrimSpace(m.input.Value()) != "" {
				m.rebuild()
			}
			m.heatReturnFocus = m.scr == screenSearch && !m.resultsFocused
			return m, m.openHeatmap()
		}
		if m.strongsOpen {
			return m, m.handleStrongsKey(msg)
		}
		if m.transOpen {
			return m, m.handleTransKey(msg)
		}
		if m.help {
			return m, m.handleHelpKey(msg)
		}
		if m.palette {
			return m, m.handlePaletteKey(msg)
		}
		if m.noteOpen {
			return m, m.handleNoteKey(msg)
		}
		if m.heat.open {
			return m, m.handleHeatmapKey(msg)
		}
		switch m.scr {
		case screenReader:
			return m, m.handleReaderKey(msg)
		case screenLibrary:
			return m, m.handleLibraryKey(msg)
		default:
			return m, m.handleSearchKey(msg)
		}
	}
	// Forward blink and cursor messages to the active editor.
	var cmd tea.Cmd
	if m.noteOpen {
		m.noteInput, cmd = m.noteInput.Update(msg)
	} else if m.palette {
		m.paletteInput, cmd = m.paletteInput.Update(msg)
	} else if m.scr == screenLibrary && m.library.filter == libCalendar && m.calendar.filterEditing {
		m.calendar.filterInput, cmd = m.calendar.filterInput.Update(msg)
	} else if m.scr == screenSearch && !m.resultsFocused {
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}

func (m *model) View() string {
	if m.width <= 0 || m.height <= 0 || m.hidden {
		return ""
	}
	if m.help {
		return m.helpView()
	}
	if m.strongsOpen {
		return m.strongsView()
	}
	if m.transOpen {
		return m.transView()
	}
	if m.palette {
		return m.paletteView()
	}
	if m.noteOpen {
		return m.noteView()
	}
	if m.heat.open {
		return m.heatmapView()
	}
	return m.screenView()
}

// screenView renders whichever normal screen is active. The note editor uses
// it to keep the underlying screen visible above the docked panel.
func (m *model) screenView() string {
	switch m.scr {
	case screenReader:
		return m.readerView()
	case screenLibrary:
		return m.libraryView()
	default:
		return m.searchView()
	}
}

func (m *model) setStatus(s string, isErr bool) {
	m.status = s
	m.statusErr = isErr
}

// userNotes returns the saved notes keyed by verse so readers and previews can
// render them inline without repeated store lookups.
func (m *model) userNotes() map[string]store.Note {
	items := m.store.Notes()
	out := make(map[string]store.Note, len(items))
	for _, n := range items {
		out[n.Key] = n
	}
	return out
}

func (m *model) currentSettings() store.Settings {
	return store.Settings{
		VerseNumbers: m.showVerseNumbers,
		Footnotes:    m.showFootnotes,
		Strongs:      m.showStrongs,
		Translation:  m.translationID,
		Parallel:     append([]string(nil), m.parallelIDs...),
		CopyFormat:   m.store.Settings().CopyFormat,
	}
}

func (m *model) SaveSettings() tea.Cmd {
	// Capture values now: tea.Cmd runs on another goroutine, and the model may
	// change again before the save executes.
	settings := m.currentSettings()
	st := m.store
	return func() tea.Msg { return savedMsg{st.SetSettings(settings)} }
}

// openReference switches to the reader at r and records history.
func (m *model) openReference(r ref.Reference) tea.Cmd {
	if r.BookID == 0 {
		return nil
	}
	if r.Chapter == 0 {
		r.Chapter = 1
	}
	if r.Verse == 0 {
		r.Verse = 1
	}
	m.reader.openAt(m.bible, r)
	m.currentBook = r.BookID
	m.scr = screenReader
	m.resultsFocused = true
	m.input.Blur()
	return m.recordHistory(r)
}

// recordHistory updates history in memory and schedules a debounced save so
// flipping through chapters does not write the state file on every step.
func (m *model) recordHistory(r ref.Reference) tea.Cmd {
	bk := m.bible.BookByID(r.BookID)
	if bk == nil {
		return nil
	}
	v := m.bible.VerseAt(r.BookID, r.Chapter, max(1, r.Verse))
	text := ""
	if v != nil {
		text = v.Text
	}
	m.store.TouchHistory(store.VerseItem{
		Key: r.Key(), Ref: r.Label(m.bible), Text: text, At: time.Now(),
	})
	if m.persistPending {
		return nil
	}
	m.persistPending = true
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return persistStateMsg{}
	})
}

func (m *model) focusedRow() (row, bool) {
	if m.scr == screenLibrary {
		if m.library.sel < 0 || m.library.sel >= len(m.library.rows) {
			return row{}, false
		}
		return m.library.rows[m.library.sel], true
	}
	if m.sel < 0 || m.sel >= len(m.flat) {
		return row{}, false
	}
	return m.flat[m.sel], true
}

// focusedReference resolves the selected row to a verse location.
func (m *model) focusedReference() (ref.Reference, bool) {
	if m.scr == screenReader && m.reader.bookID != 0 {
		return ref.Reference{
			BookID:  m.reader.bookID,
			Chapter: m.reader.chapter,
			Verse:   uint16(max(1, m.reader.verse)),
		}, true
	}
	r, ok := m.focusedRow()
	if !ok {
		return ref.Reference{}, false
	}
	switch r.kind {
	case rowRef, rowChapter, rowVerse, rowBookmark, rowNote, rowHistory:
		rr := r.ref
		if rr.BookID == 0 && r.key != "" {
			if parsed, ok := ref.ParseKey(r.key); ok {
				rr = parsed
			}
		}
		if rr.BookID != 0 && rr.Chapter != 0 {
			return rr, true
		}
	}
	return ref.Reference{}, false
}

func (m *model) focusedVerse() (ref.Reference, *corpus.Verse, bool) {
	r, ok := m.focusedReference()
	if !ok {
		return ref.Reference{}, nil, false
	}
	ch := r.Chapter
	if ch == 0 {
		ch = 1
	}
	vnum := r.Verse
	if vnum == 0 {
		vnum = 1
	}
	v := m.bible.VerseAt(r.BookID, ch, vnum)
	if v == nil {
		return r, nil, false
	}
	r.Verse, r.Chapter = vnum, ch
	return r, v, true
}

// readerSelection returns the ordered range marked with V. It remains
// available after V finishes so the next copy/bookmark/note uses the passage.
func (m *model) readerSelection() (ref.Reference, bool) {
	if m.scr != screenReader || m.selAnchor.BookID == 0 || m.selHead.BookID != m.selAnchor.BookID {
		return ref.Reference{}, false
	}
	first, last := m.selAnchor, m.selHead
	if first.Chapter > last.Chapter || (first.Chapter == last.Chapter && first.Verse > last.Verse) {
		first, last = last, first
	}
	r := ref.Reference{
		BookID:  first.BookID,
		Chapter: first.Chapter,
		Verse:   first.Verse,
	}
	if last.Chapter > first.Chapter {
		r.EndChapter = last.Chapter
		r.EndVerse = last.Verse
	} else if last.Verse > first.Verse {
		r.EndVerse = last.Verse
	}
	return r, true
}

// selectionInChapter returns the verse bounds of the active selection in one
// chapter, for highlighting while reading.
func (m *model) selectionInChapter(chapter uint16) (uint16, uint16, bool) {
	r, ok := m.readerSelection()
	if !ok || r.BookID != m.reader.bookID {
		return 0, 0, false
	}
	lastCh := r.LastChapter()
	if chapter < r.Chapter || chapter > lastCh {
		return 0, 0, false
	}
	first := uint16(1)
	last := uint16(m.readerMaxVerse())
	if chapter == r.Chapter {
		first = r.Verse
	}
	if chapter == lastCh {
		if r.EndVerse != 0 {
			last = r.EndVerse
		} else if lastCh == r.Chapter {
			last = r.Verse
		}
	}
	return first, last, true
}

// beginSelection starts a passage selection at the cursor.
func (m *model) beginSelection() {
	r := ref.Reference{BookID: m.reader.bookID, Chapter: m.reader.chapter, Verse: uint16(max(1, m.reader.verse))}
	m.selecting, m.selAnchor, m.selHead = true, r, r
	m.setStatus("Selecting passage · j/k move · y copy · V finish · Esc cancel", false)
}

// extendSelection updates the far end of the passage after navigation.
func (m *model) extendSelection() {
	if !m.selecting {
		return
	}
	m.selHead = ref.Reference{BookID: m.reader.bookID, Chapter: m.reader.chapter, Verse: uint16(max(1, m.reader.verse))}
	if m.selHead.BookID != m.selAnchor.BookID {
		// Passages in one selection stay inside one book; chapter ranges are
		// supported, but silently mixing testaments would be surprising.
		m.clearSelection()
		m.setStatus("Selection stays within one book", true)
		return
	}
	if r, ok := m.readerSelection(); ok {
		m.setStatus("Selected "+r.Label(m.bible)+" · V done · Esc cancel", false)
	}
}

// endSelection stops highlighting but leaves the range ready for one action.
func (m *model) endSelection() {
	m.selecting = false
	if r, ok := m.readerSelection(); ok {
		m.setStatus("Selected "+r.Label(m.bible)+" · y copy · Esc clear", false)
	}
}

// clearSelection discards the passage range completely.
func (m *model) clearSelection() {
	m.selecting = false
	m.selAnchor, m.selHead = ref.Reference{}, ref.Reference{}
}

// focusedRange returns the multi-verse selection represented by the focused
// row: a chapter row (including chapter ranges) or a reference row spanning
// several verses. In the reader it is the passage marked with V.
func (m *model) focusedRange() (ref.Reference, []*corpus.Verse, bool) {
	if m.scr == screenReader {
		r, ok := m.readerSelection()
		if !ok {
			return ref.Reference{}, nil, false
		}
		verses := r.Verses(m.bible)
		if len(verses) == 0 {
			return ref.Reference{}, nil, false
		}
		return r, verses, true
	}
	row, ok := m.focusedRow()
	if !ok {
		return ref.Reference{}, nil, false
	}
	switch row.kind {
	case rowChapter, rowRef:
	default:
		return ref.Reference{}, nil, false
	}
	r, ok := m.focusedReference()
	if !ok {
		return ref.Reference{}, nil, false
	}
	ranged := row.kind == rowChapter || r.EndChapter > r.Chapter || r.EndVerse > r.Verse
	if !ranged {
		return ref.Reference{}, nil, false
	}
	verses := r.Verses(m.bible)
	if len(verses) == 0 {
		return ref.Reference{}, nil, false
	}
	return r, verses, true
}

func (m *model) copyFocused() tea.Cmd {
	if r, verses, ok := m.focusedRange(); ok {
		label := r.Label(m.bible)
		text := m.copyRangeText(r, verses)
		m.clearSelection()
		return func() tea.Msg { return copiedMsg{label: label, err: clipboard.Copy(text)} }
	}
	r, v, ok := m.focusedVerse()
	if !ok {
		if row, ok := m.focusedRow(); ok && row.kind == rowSaved {
			label := row.label
			return func() tea.Msg { return copiedMsg{label: label, err: clipboard.Copy(label)} }
		}
		m.setStatus("Nothing to copy", true)
		return nil
	}
	text := m.copyText(r, v)
	label := r.Label(m.bible)
	return func() tea.Msg { return copiedMsg{label: label, err: clipboard.Copy(text)} }
}

func (m *model) copyRangeText(r ref.Reference, verses []*corpus.Verse) string {
	parts := make([]string, len(verses))
	for i, v := range verses {
		parts[i] = v.Text
	}
	body := strings.Join(parts, " ")
	if len(m.parallel) > 0 {
		switch m.store.Settings().CopyFormat {
		case "ref":
			return r.Label(m.bible)
		case "text":
			return m.parallelCopyText(verses)
		default:
			return r.Label(m.bible) + "\n\n" + m.parallelCopyText(verses)
		}
	}
	switch m.store.Settings().CopyFormat {
	case "text":
		return body
	case "ref":
		return r.Label(m.bible)
	default:
		return fmt.Sprintf("%s (%s)", body, r.Label(m.bible))
	}
}

func (m *model) copyText(r ref.Reference, v *corpus.Verse) string {
	if len(m.parallel) > 0 {
		switch m.store.Settings().CopyFormat {
		case "ref":
			return r.Label(m.bible)
		case "text":
			return m.parallelCopyText([]*corpus.Verse{v})
		default:
			return r.Label(m.bible) + "\n\n" + m.parallelCopyText([]*corpus.Verse{v})
		}
	}
	switch m.store.Settings().CopyFormat {
	case "text":
		return v.Text
	case "ref":
		return r.Label(m.bible)
	default:
		return fmt.Sprintf("%s (%s)", v.Text, r.Label(m.bible))
	}
}

func (m *model) toggleBookmark() tea.Cmd {
	if r, verses, ok := m.focusedRange(); ok {
		items := make([]store.VerseItem, 0, len(verses))
		for _, v := range verses {
			rr := ref.Reference{BookID: v.BookID, Chapter: v.Chapter, Verse: v.Number}
			items = append(items, store.VerseItem{
				Key: rr.Key(), Ref: rr.Label(m.bible), Text: v.Text, At: time.Now(),
			})
		}
		added, err := m.store.ToggleBookmarks(items)
		if err != nil {
			m.setStatus(err.Error(), true)
			return nil
		}
		label := r.Label(m.bible)
		if added {
			m.setStatus(fmt.Sprintf("Bookmarked %s (%d verses)", label, len(items)), false)
		} else {
			m.setStatus(fmt.Sprintf("Removed bookmark %s (%d verses)", label, len(items)), false)
		}
		if m.scr == screenLibrary {
			m.rebuildLibrary()
		}
		m.clearSelection()
		return nil
	}

	r, v, ok := m.focusedVerse()
	if !ok {
		m.setStatus("Select a verse first", true)
		return nil
	}
	item := store.VerseItem{Key: r.Key(), Ref: r.Label(m.bible), Text: v.Text, At: time.Now()}
	added, err := m.store.ToggleBookmark(item)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	if added {
		m.setStatus("Bookmarked "+r.Label(m.bible), false)
	} else {
		m.setStatus("Removed bookmark "+r.Label(m.bible), false)
	}
	if m.scr == screenLibrary {
		m.rebuildLibrary()
	}
	return nil
}

func (m *model) beginNote() tea.Cmd {
	r, v, ok := m.focusedVerse()
	if !ok {
		m.setStatus("Select a verse first", true)
		return nil
	}
	noteRef := r.Label(m.bible)
	if rr, verses, ranged := m.focusedRange(); ranged {
		// Notes live on a verse key, so a range note attaches to its first
		// verse but is labelled with the whole range.
		first := verses[0]
		r = ref.Reference{BookID: first.BookID, Chapter: first.Chapter, Verse: first.Number}
		noteRef = rr.Label(m.bible)
	}
	m.noteKey = r.Key()
	m.noteRef = noteRef
	body := ""
	if n, ok := m.store.Note(m.noteKey); ok {
		body = n.Body
	} else {
		_ = v
	}
	m.noteInput.SetValue(body)
	m.noteInput.Focus()
	m.noteOpen = true
	return textarea.Blink
}

func (m *model) handleNoteKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.noteOpen = false
		m.noteInput.Blur()
		return nil
	case "ctrl+s":
		body := strings.TrimSpace(m.noteInput.Value())
		if err := m.store.SetNote(m.noteKey, m.noteRef, body); err != nil {
			m.setStatus(err.Error(), true)
		} else if body == "" {
			m.setStatus("Note removed", false)
		} else {
			m.setStatus("Note saved for "+m.noteRef, false)
		}
		m.noteOpen = false
		m.noteInput.Blur()
		m.clearSelection()
		if m.scr == screenLibrary {
			m.rebuildLibrary()
		}
		return nil
	}
	var cmd tea.Cmd
	m.noteInput, cmd = m.noteInput.Update(msg)
	return cmd
}

func (m *model) cycleScope() {
	switch m.scope {
	case search.ScopeAll:
		m.scope = search.ScopeOT
	case search.ScopeOT:
		m.scope = search.ScopeNT
	case search.ScopeNT:
		m.scope = search.ScopeBook
		if m.scopeBook == 0 {
			m.scopeBook = m.currentBook
		}
		if m.scopeBook == 0 {
			m.scopeBook = 1
		}
	case search.ScopeBook:
		m.scope = search.ScopeAll
	}
	if m.scr == screenSearch {
		m.rebuild()
	}
}

func (m *model) scopeLabel() string {
	switch m.scope {
	case search.ScopeOT:
		return "Old Testament"
	case search.ScopeNT:
		return "New Testament"
	case search.ScopeBook:
		if b := m.bible.BookByID(m.scopeBook); b != nil {
			return b.Name
		}
		return "Current book"
	default:
		return "All books"
	}
}

func savedScopeName(s search.Scope) string {
	switch s {
	case search.ScopeOT:
		return store.ScopeOT
	case search.ScopeNT:
		return store.ScopeNT
	case search.ScopeBook:
		return store.ScopeBook
	default:
		return store.ScopeAll
	}
}

func scopeFromSaved(name string, bookID uint16) (search.Scope, uint16) {
	switch name {
	case store.ScopeOT:
		return search.ScopeOT, 0
	case store.ScopeNT:
		return search.ScopeNT, 0
	case store.ScopeBook:
		if bookID != 0 {
			return search.ScopeBook, bookID
		}
	}
	return search.ScopeAll, 0
}

// savedScopeLabel describes a saved search's scope for display.
func (m *model) savedScopeLabel(s store.SavedSearch) string {
	scope, bookID := scopeFromSaved(s.Scope, s.BookID)
	switch scope {
	case search.ScopeOT:
		return "saved · Old Testament"
	case search.ScopeNT:
		return "saved · New Testament"
	case search.ScopeBook:
		if b := m.bible.BookByID(bookID); b != nil {
			return "saved · " + b.Name
		}
		return "saved · current book"
	default:
		return "saved · all books"
	}
}

func (m *model) savedRow(s store.SavedSearch) row {
	return row{
		kind:      rowSaved,
		label:     s.Query,
		text:      m.savedScopeLabel(s),
		scopeName: s.Scope,
		bookID:    s.BookID,
	}
}

func (m *model) saveSearch() {
	q := strings.TrimSpace(m.input.Value())
	if q == "" {
		q = strings.TrimSpace(m.query)
	}
	if q == "" {
		m.setStatus("Nothing to save", true)
		return
	}
	scope := savedScopeName(m.scope)
	bookID := m.scopeBook
	if scope != store.ScopeBook {
		bookID = 0
	}
	if err := m.store.AddSavedSearchScoped(q, scope, bookID); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.setStatus("Saved search: "+q+" · "+m.scopeLabel(), false)
}

func (m *model) focusSearch() tea.Cmd {
	m.scr = screenSearch
	m.resultsFocused = false
	m.input.Focus()
	return textinput.Blink
}

func (m *model) openBook(bookID uint16) tea.Cmd {
	bk := m.bible.BookByID(bookID)
	if bk == nil {
		return nil
	}
	return m.openReference(ref.Reference{BookID: bookID, Chapter: 1, Verse: 1})
}

func (m *model) openChapter(bookID, chapter uint16) tea.Cmd {
	if chapter == 0 {
		chapter = 1
	}
	return m.openReference(ref.Reference{BookID: bookID, Chapter: chapter, Verse: 1})
}

func (m *model) chapterStep(delta int) tea.Cmd {
	bookID, ch := m.reader.bookID, m.reader.chapter
	bk := m.bible.BookByID(bookID)
	if bk == nil {
		return nil
	}
	if delta > 0 {
		if ch < bk.ChapterCount {
			ch++
		} else if bookID < 66 {
			bookID++
			ch = 1
		}
	} else {
		if ch > 1 {
			ch--
		} else if bookID > 1 {
			bookID--
			if b := m.bible.BookByID(bookID); b != nil {
				ch = b.ChapterCount
			}
		}
	}
	return m.openChapter(bookID, ch)
}

func (m *model) bookStep(delta int) tea.Cmd {
	if m.selAnchor.BookID != 0 {
		m.clearSelection()
	}
	bookID := m.reader.bookID
	if delta > 0 && bookID < 66 {
		bookID++
	} else if delta < 0 && bookID > 1 {
		bookID--
	}
	bk := m.bible.BookByID(bookID)
	if bk == nil {
		return nil
	}
	ch := m.reader.chapter
	if ch > bk.ChapterCount {
		ch = bk.ChapterCount
	}
	return m.openChapter(bookID, ch)
}

func (m *model) runCommand(id string) tea.Cmd {
	switch id {
	case "search":
		return m.focusSearch()
	case "library", "bookmarks", "notes", "history", "saved", "calendar":
		filter := libAll
		switch id {
		case "bookmarks":
			filter = libBookmarks
		case "notes":
			filter = libNotes
		case "history":
			filter = libHistory
		case "saved":
			filter = libSaved
		case "calendar":
			filter = libCalendar
		}
		m.scr = screenLibrary
		m.setLibraryFilter(filter)
		return nil
	case "help":
		m.help = true
		m.helpScroll = 0
		return nil
	case "palette":
		m.openPalette()
		return nil
	case "numbers":
		m.showVerseNumbers = !m.showVerseNumbers
		m.reader.showNumbers = m.showVerseNumbers
		if m.showVerseNumbers {
			m.setStatus("Verse numbers on", false)
		} else {
			m.setStatus("Verse numbers off", false)
		}
		return m.SaveSettings()
	case "footnotes":
		m.showFootnotes = !m.showFootnotes
		m.reader.showNotes = m.showFootnotes
		if m.showFootnotes {
			m.setStatus("Notes & footnotes on", false)
		} else {
			m.setStatus("Notes & footnotes off", false)
		}
		return m.SaveSettings()
	case "strongs":
		return m.toggleStrongs()
	case "strongs-entry":
		r, v, ok := m.focusedVerse()
		if !ok {
			m.setStatus("Select a verse first", true)
			return nil
		}
		return m.openStrongs(r, v)
	case "strongs-next":
		return m.moveStrongsFocus(1)
	case "strongs-prev":
		return m.moveStrongsFocus(-1)
	case "sidebar":
		m.sidebar = !m.sidebar
		return nil
	case "translations":
		return m.openTrans()
	case "refresh-catalog":
		if !m.transOpen {
			m.openTrans()
		}
		return m.refreshCatalog()
	case "scope":
		m.cycleScope()
		return nil
	case "heatmap":
		return m.openHeatmap()
	case "save-search":
		m.saveSearch()
		return nil
	case "copy":
		return m.copyFocused()
	case "bookmark":
		return m.toggleBookmark()
	case "note":
		return m.beginNote()
	case "home":
		m.scr = screenSearch
		m.resultsFocused = true
		if m.query == "" {
			m.rebuild()
		}
		return nil
	case "quit":
		return tea.Quit
	}
	return nil
}

// headerLine renders a left/right header that spans the terminal width.
func (m *model) headerLine(left, right string) string {
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return truncateANSI(left, m.width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *model) headerLeft() string {
	return appTitle.Render("tbible") + "  " + appSubtitle.Render(m.translationLabel())
}

func (m *model) headerRight() string {
	return mutedStyle.Render("? help  ·  Ctrl-P commands")
}

func fitLineSlice(lines []string, width, height int) []string {
	if height <= 0 {
		return nil
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	out := make([]string, 0, height)
	for _, l := range lines {
		out = append(out, truncateANSI(l, width))
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

func fitLines(lines []string, width, height int) string {
	return strings.Join(fitLineSlice(lines, width, height), "\n")
}

func padTo(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return truncateANSI(s, width)
	}
	return s + strings.Repeat(" ", width-w)
}

func (m *model) footer(hints string) string {
	h := footerStyle.Render(truncateANSI(hints, m.width))
	st := ""
	if m.status != "" {
		if m.statusErr {
			st = statusErr.Render(truncateANSI(m.status, m.width))
		} else {
			st = statusOK.Render(truncateANSI(m.status, m.width))
		}
	}
	return h + "\n" + padTo(st, m.width)
}

func (m *model) versionLabel() string {
	return "tbible " + version.Version
}
