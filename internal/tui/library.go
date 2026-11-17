package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/petealeon/tbible/v2/internal/ref"
	"github.com/petealeon/tbible/v2/internal/store"
)

type libraryFilter int

const (
	libAll libraryFilter = iota
	libBookmarks
	libNotes
	libHistory
	libSaved
	libCalendar
)

var libraryFilterNames = []string{"All", "Bookmarks", "Notes", "History", "Saved searches", "Calendar"}

type libraryModel struct {
	filter   libraryFilter
	rows     []row
	sections []searchSection
	sel      int
	scroll   int
	rowLine  []int
	total    int
}

func (m *model) addLibrarySection(title string, rows []row) {
	if len(rows) == 0 {
		return
	}
	s := searchSection{title: title, start: len(m.library.rows), end: len(m.library.rows) + len(rows)}
	m.library.rows = append(m.library.rows, rows...)
	m.library.sections = append(m.library.sections, s)
}

// rebuildLibrary repopulates the library from the persisted store. Selection
// is preserved by row identity when possible.
func (m *model) rebuildLibrary() {
	l := &m.library
	if l.filter == libCalendar {
		m.rebuildCalendar()
		return
	}
	prev := ""
	if l.sel >= 0 && l.sel < len(l.rows) {
		prev = rowIdentity(l.rows[l.sel])
	}
	l.rows = nil
	l.sections = nil
	snap := m.store.Snapshot()

	addBookmarks := func() {
		var rows []row
		for _, it := range snap.Bookmarks {
			if r, ok := m.rowFromItem(rowBookmark, it); ok {
				rows = append(rows, r)
			}
		}
		m.addLibrarySection("Bookmarks", rows)
	}
	addNotes := func() {
		var rows []row
		for _, n := range snap.Notes {
			rows = append(rows, row{kind: rowNote, ref: mustRef(n.Key), label: n.Ref, text: n.Body, key: n.Key})
		}
		m.addLibrarySection("Notes", rows)
	}
	addHistory := func() {
		var rows []row
		for _, it := range snap.History {
			if r, ok := m.rowFromItem(rowHistory, it); ok {
				rows = append(rows, r)
			}
		}
		m.addLibrarySection("History", rows)
	}
	addSaved := func() {
		var rows []row
		for _, s := range snap.SavedSearches {
			rows = append(rows, m.savedRow(s))
		}
		m.addLibrarySection("Saved searches", rows)
	}

	switch l.filter {
	case libBookmarks:
		addBookmarks()
	case libNotes:
		addNotes()
	case libHistory:
		addHistory()
	case libSaved:
		addSaved()
	default:
		addBookmarks()
		addNotes()
		addHistory()
		addSaved()
	}

	m.finishLibraryRows(prev)
}

// finishLibraryRows rebuilds the section/line index and restores selection by
// row identity. Both the regular library and the calendar's day list use it.
func (m *model) finishLibraryRows(prev string) {
	l := &m.library
	l.rowLine = make([]int, len(l.rows))
	line := 0
	for i, sec := range l.sections {
		if i > 0 {
			line++
		}
		line++ // heading
		for j := sec.start; j < sec.end; j++ {
			l.rowLine[j] = line
			line++
		}
	}
	l.total = line

	if prev != "" {
		for i := range l.rows {
			if rowIdentity(l.rows[i]) == prev {
				l.sel = i
				return
			}
		}
	}
	if l.sel >= len(l.rows) {
		l.sel = max(0, len(l.rows)-1)
	}
	if l.sel < 0 {
		l.sel = 0
	}
}

func (m *model) libraryBodyHeight() int {
	return max(1, m.height-5)
}

func (m *model) libraryEnsureVisible(avail int) {
	l := &m.library
	if len(l.rows) == 0 || l.sel >= len(l.rowLine) {
		l.scroll = 0
		return
	}
	line := l.rowLine[l.sel]
	avail = max(1, avail)
	if line < l.scroll+1 {
		l.scroll = line - 1
	}
	if line >= l.scroll+avail-2 {
		l.scroll = line - avail + 2
	}
	maxScroll := max(0, l.total-avail)
	if l.scroll < 0 {
		l.scroll = 0
	}
	if l.scroll > maxScroll {
		l.scroll = maxScroll
	}
}

func (m *model) setLibraryFilter(filter libraryFilter) {
	if filter == libCalendar && m.library.filter != libCalendar {
		m.calendar.prevFilter = m.library.filter
	}
	m.library.filter = filter
	m.library.sel = 0
	m.library.scroll = 0
	m.library.rows = nil
	m.library.sections = nil
	m.rebuildLibrary()
}

func (m *model) libraryCycleFilter(delta int) {
	n := len(libraryFilterNames)
	next := libraryFilter((int(m.library.filter) + delta + n) % n)
	m.setLibraryFilter(next)
}

func (m *model) handleLibraryKey(msg tea.KeyMsg) tea.Cmd {
	if m.library.filter == libCalendar {
		return m.handleCalendarKey(msg)
	}
	key := msg.String()
	if key == "ctrl+p" || key == ":" {
		m.openPalette()
		return nil
	}
	switch key {
	case "esc", "/":
		return m.focusSearch()
	case "tab", "right", "]":
		m.libraryCycleFilter(1)
		return nil
	case "shift+tab", "left", "[":
		m.libraryCycleFilter(-1)
		return nil
	case "up", "k":
		if m.library.sel > 0 {
			m.library.sel--
		}
		return nil
	case "down", "j":
		if m.library.sel < len(m.library.rows)-1 {
			m.library.sel++
		}
		return nil
	case "g", "home":
		m.library.sel = 0
		m.library.scroll = 0
		return nil
	case "G", "end":
		m.library.sel = max(0, len(m.library.rows)-1)
		return nil
	case "pgdown", "ctrl+d":
		m.library.sel = min(max(0, len(m.library.rows)-1), m.library.sel+max(1, (m.libraryBodyHeight()-2)/2))
		return nil
	case "pgup", "ctrl+u":
		m.library.sel = max(0, m.library.sel-max(1, (m.libraryBodyHeight()-2)/2))
		return nil
	case "enter":
		return m.activateLibrary()
	case "y":
		return m.copyFocused()
	case "b":
		return m.toggleBookmark()
	case "n":
		return m.beginNote()
	case "x", "delete":
		return m.deleteLibraryItem()
	case "L":
		m.libraryCycleFilter(1)
		return nil
	case "?":
		m.help = true
		m.helpScroll = 0
		return nil
	}
	return nil
}

func (m *model) activateLibrary() tea.Cmd {
	l := &m.library
	if l.sel < 0 || l.sel >= len(l.rows) {
		return nil
	}
	r := l.rows[l.sel]
	switch r.kind {
	case rowBookmark, rowHistory, rowNote:
		if rr, ok := ref.ParseKey(r.key); ok {
			return m.openReference(rr)
		}
	case rowSaved:
		m.scr = screenSearch
		m.resultsFocused = false
		m.scope, m.scopeBook = scopeFromSaved(r.scopeName, r.bookID)
		m.input.SetValue(r.label)
		m.input.Focus()
		m.rebuild()
		return textinput.Blink
	}
	return nil
}

// deleteLibraryItem removes the focused saved search, bookmark, or note.
func (m *model) deleteLibraryItem() tea.Cmd {
	l := &m.library
	if l.sel < 0 || l.sel >= len(l.rows) {
		return nil
	}
	r := l.rows[l.sel]
	switch r.kind {
	case rowSaved:
		if err := m.store.RemoveSavedSearchScoped(r.label, r.scopeName, r.bookID); err != nil {
			m.setStatus(err.Error(), true)
			return nil
		}
		m.setStatus("Deleted saved search", false)
	case rowBookmark:
		rr, ok := ref.ParseKey(r.key)
		if !ok {
			return nil
		}
		v := m.bible.VerseAt(rr.BookID, max(1, rr.Chapter), max(1, rr.Verse))
		if v == nil {
			return nil
		}
		if _, err := m.store.ToggleBookmark(store.VerseItem{Key: rr.Key(), Ref: rr.Label(m.bible), Text: v.Text}); err != nil {
			m.setStatus(err.Error(), true)
			return nil
		}
		m.setStatus("Removed bookmark", false)
	case rowNote:
		if err := m.store.SetNote(r.key, r.label, ""); err != nil {
			m.setStatus(err.Error(), true)
			return nil
		}
		m.setStatus("Deleted note", false)
	default:
		m.setStatus("Nothing to delete", true)
		return nil
	}
	m.rebuildLibrary()
	return nil
}

func (m *model) libraryTabs(width int) string {
	var parts []string
	for i, name := range libraryFilterNames {
		if libraryFilter(i) == m.library.filter {
			parts = append(parts, chipStyle.Render("["+name+"]"))
		} else {
			parts = append(parts, chipDim.Render(" "+name+" "))
		}
	}
	return truncateANSI(strings.Join(parts, " "), width)
}

func (m *model) libraryRows(width int) []string {
	l := &m.library
	var lines []string
	for i, sec := range l.sections {
		if i > 0 {
			lines = append(lines, "")
		}
		title := sectionStyle.Render(strings.ToUpper(sec.title))
		count := mutedStyle.Render(fmt.Sprintf("  %d", sec.end-sec.start))
		lines = append(lines, title+count)
		for j := sec.start; j < sec.end; j++ {
			r := l.rows[j]
			lines = append(lines, m.renderLibraryRow(r, width, j == l.sel))
		}
	}
	return lines
}

func (m *model) renderLibraryRow(r row, width int, selected bool) string {
	marker := "  "
	if selected {
		marker = cursorMarker.Render("▌ ")
	}
	labelW := min(30, max(14, width/3))
	label := padTo(truncateANSI(r.label, labelW), labelW)
	if selected {
		label = selStyle.Render(label)
	} else {
		label = labelStyle.Render(label)
	}
	avail := max(8, width-labelW-3)
	body := ""
	switch r.kind {
	case rowSaved:
		body = mutedStyle.Render(truncateANSI(r.text, avail))
	case rowNote:
		body = noteStyle.Render(truncateANSI(r.text, avail))
	default:
		body = textStyle.Render(truncateANSI(r.text, avail))
	}
	return marker + label + " " + body
}

func (m *model) libraryView() string {
	if m.library.filter == libCalendar {
		return m.calendarView()
	}
	header := m.headerLine(m.headerLeft(), m.headerRight())
	tabs := m.libraryTabs(m.width)
	avail := m.libraryBodyHeight()
	m.libraryEnsureVisible(avail)
	all := m.libraryRows(m.width)
	start := min(m.library.scroll, max(0, len(all)-1))
	end := min(len(all), start+avail)
	block := fitLines(all[start:end], m.width, avail)

	if len(m.library.rows) == 0 {
		block = fitLines([]string{
			sectionStyle.Render(strings.ToUpper(libraryFilterNames[m.library.filter])),
			"",
			mutedStyle.Render("Nothing here yet."),
			"",
			mutedStyle.Render("Search for a verse, open it, then press b to bookmark"),
			mutedStyle.Render("it or n to write a note."),
		}, m.width, avail)
	}

	hints := actionHints(m.width) + " · ↑↓ · Enter open · Tab filter · x delete · ? help"
	if m.width >= 110 {
		hints = "↑↓ select · Enter open · Tab filter · x delete · " + actionHints(m.width) + " · / search · Esc back · ? help"
	}
	return header + "\n" + tabs + "\n\n" + block + "\n" + m.footer(hints)
}
