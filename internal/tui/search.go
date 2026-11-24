package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/petealeon/tbible/v2/internal/activity"
	"github.com/petealeon/tbible/v2/internal/ref"
	"github.com/petealeon/tbible/v2/internal/search"
	"github.com/petealeon/tbible/v2/internal/store"
)

type rowKind int

const (
	rowRef rowKind = iota
	rowBook
	rowChapter
	rowVerse
	rowBookmark
	rowNote
	rowHistory
	rowSaved
	rowCommand
)

type row struct {
	kind      rowKind
	ref       ref.Reference
	bookID    uint16
	chapter   uint16
	verse     int
	label     string
	text      string
	key       string
	command   string
	scopeName string
	score     float64
	at        time.Time
	eventKind activity.Kind
}

type searchSection struct {
	title string
	start int
	end   int
}

type commandDef struct {
	id   string
	name string
	desc string
	keys string
}

var commandDefs = []commandDef{
	{"search", "Search", "Jump to the omnibox", "/"},
	{"library", "Library", "Bookmarks, notes, history, saved searches, calendar", "L"},
	{"calendar", "Calendar", "Browse notes, bookmarks, and history by date", "Library tab"},
	{"help", "Help", "Keyboard map and query syntax", "?"},
	{"palette", "Command palette", "Run any action by name", "Ctrl-P"},
	{"numbers", "Toggle verse numbers", "Show or hide verse numbers", "v"},
	{"footnotes", "Toggle notes and footnotes", "Show or hide your notes and translation footnotes", "i"},
	{"strongs", "Toggle Strong's numbers", "Show or hide corpus-supplied inline Strong's tags", "s"},
	{"strongs-next", "Next Strong's word", "Highlight the next tagged word in the focused verse", "w"},
	{"strongs-prev", "Previous Strong's word", "Highlight the previous tagged word in the focused verse", "W"},
	{"strongs-entry", "Strong's dictionary", "Open the focused Strong's entry", "K"},
	{"sidebar", "Toggle reader sidebar", "Chapter and bookmark details", "Ctrl-B"},
	{"translations", "Translations", "Choose primary and parallel translations; manage packs", "T"},
	{"refresh-catalog", "Refresh translation catalog", "Check for packs (explicit network action)", "Translations · U"},
	{"scope", "Cycle search scope", "All, Old Testament, New Testament, current book", "f"},
	{"heatmap", "Heatmap", "Visualise result concentration across books and chapters", "H / Ctrl-H / F2"},
	{"save-search", "Save search", "Keep the current query in the library", "S"},
	{"copy", "Copy verse", "Copy the focused verse or range", "y"},
	{"bookmark", "Bookmark verse", "Add or remove a bookmark", "b"},
	{"note", "Edit note", "Write a note on the focused verse", "n"},
	{"home", "Home", "Return to search and recent items", "Esc"},
	{"quit", "Quit", "Exit tbible", "Ctrl-C"},
}

func (m *model) handleSearchKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	if key == "ctrl+p" {
		m.openPalette()
		return nil
	}
	if key == "ctrl+n" {
		m.moveSel(1)
		return nil
	}

	if m.resultsFocused {
		switch key {
		case "up", "k":
			m.moveSel(-1)
			return nil
		case "down", "j":
			m.moveSel(1)
			return nil
		case "g", "home":
			m.sel = 0
			m.resultsFocused = true
			return nil
		case "G", "end":
			m.sel = max(0, len(m.flat)-1)
			m.resultsFocused = true
			return nil
		case "ctrl+d", "pgdown":
			m.moveSel(max(1, (m.height-6)/2))
			return nil
		case "ctrl+u", "pgup":
			m.moveSel(-max(1, (m.height-6)/2))
			return nil
		case "tab":
			m.jumpSection(1)
			return nil
		case "shift+tab":
			m.jumpSection(-1)
			return nil
		case "enter":
			return m.activate()
		case "esc":
			m.resultsFocused = false
			m.input.Focus()
			return textinput.Blink
		case "y":
			return m.copyFocused()
		case "b":
			return m.toggleBookmark()
		case "n":
			return m.beginNote()
		case "f":
			m.cycleScope()
			return nil
		case "s":
			return m.runCommand("strongs")
		case "K":
			return m.runCommand("strongs-entry")
		case "w":
			return m.moveStrongsFocus(1)
		case "W":
			return m.moveStrongsFocus(-1)
		case "H":
			return m.openHeatmap()
		case "S":
			m.saveSearch()
			return nil
		case "L":
			return m.runCommand("library")
		case "T":
			return m.runCommand("translations")
		case "?":
			m.help = true
			m.helpScroll = 0
			return nil
		case "/", "ctrl+g":
			m.resultsFocused = false
			m.input.Focus()
			return textinput.Blink
		}
		if isPrintable(msg) {
			m.resultsFocused = false
			m.input.Focus()
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.rebuild()
			return cmd
		}
		return nil
	}

	// The omnibox has focus. Help and the palette remain reachable while the
	// box is empty; once a query is in progress, those characters are text.
	if strings.TrimSpace(m.input.Value()) == "" {
		switch key {
		case "?":
			m.help = true
			m.helpScroll = 0
			return nil
		case ":":
			m.openPalette()
			return nil
		case "T":
			return m.runCommand("translations")
		}
	}

	switch key {
	case "esc":
		if m.input.Value() != "" {
			m.input.SetValue("")
			m.rebuild()
		} else {
			m.resultsFocused = true
			m.input.Blur()
		}
		return nil
	case "up":
		m.moveSel(-1)
		return nil
	case "down":
		m.moveSel(1)
		return nil
	case "tab":
		m.jumpSection(1)
		return nil
	case "shift+tab":
		m.jumpSection(-1)
		return nil
	case "enter":
		return m.activate()
	case "ctrl+g":
		m.input.SetValue("")
		m.rebuild()
		return nil
	case "ctrl+s":
		m.saveSearch()
		return nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != m.query {
		m.rebuild()
	}
	return cmd
}

func isPrintable(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace
}

func (m *model) moveSel(delta int) {
	if len(m.flat) == 0 {
		return
	}
	m.resultsFocused = true
	m.input.Blur()
	m.sel += delta
	if m.sel < 0 {
		m.sel = 0
	}
	if m.sel >= len(m.flat) {
		m.sel = len(m.flat) - 1
	}
}

func (m *model) jumpSection(delta int) {
	if len(m.flat) == 0 || len(m.sections) == 0 {
		return
	}
	cur := 0
	for i, s := range m.sections {
		if m.sel >= s.start && m.sel < s.end {
			cur = i
			break
		}
	}
	next := cur + delta
	if next < 0 {
		next = len(m.sections) - 1
	}
	if next >= len(m.sections) {
		next = 0
	}
	m.sel = m.sections[next].start
	m.resultsFocused = true
	m.input.Blur()
}

func (m *model) activate() tea.Cmd {
	r, ok := m.focusedRow()
	if !ok {
		return nil
	}
	switch r.kind {
	case rowRef, rowChapter, rowVerse:
		return m.openReference(r.ref)
	case rowBook:
		return m.openBook(r.bookID)
	case rowBookmark, rowHistory:
		if rr, ok := ref.ParseKey(r.key); ok {
			return m.openReference(rr)
		}
	case rowNote:
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
	case rowCommand:
		return m.runCommand(r.command)
	}
	return nil
}

func (m *model) rebuild() {
	prev := ""
	if m.sel >= 0 && m.sel < len(m.flat) {
		prev = rowIdentity(m.flat[m.sel])
	}
	m.query = m.input.Value()
	m.sections = nil
	m.flat = nil
	m.matchQuery = search.Query{}
	m.lastResult = search.Result{}
	if strings.TrimSpace(m.query) == "" {
		m.buildHome()
	} else {
		m.buildQuery(strings.TrimSpace(m.query))
	}
	m.layoutRows()
	if prev != "" {
		found := false
		for i := range m.flat {
			if rowIdentity(m.flat[i]) == prev {
				m.sel = i
				found = true
				break
			}
		}
		if !found {
			m.sel = min(m.sel, max(0, len(m.flat)-1))
		}
	}
	if m.sel >= len(m.flat) {
		m.sel = max(0, len(m.flat)-1)
	}
	if m.sel < 0 {
		m.sel = 0
	}
}

func rowIdentity(r row) string {
	switch r.kind {
	case rowCommand:
		return "cmd:" + r.command
	case rowSaved:
		return fmt.Sprintf("saved:%s:%s:%d", r.label, r.scopeName, r.bookID)
	default:
		return fmt.Sprintf("%d:%s:%s", r.kind, r.key, r.label)
	}
}

func (m *model) addSection(title string, rows []row) {
	if len(rows) == 0 {
		return
	}
	s := searchSection{title: title, start: len(m.flat), end: len(m.flat) + len(rows)}
	m.flat = append(m.flat, rows...)
	m.sections = append(m.sections, s)
}

func (m *model) layoutRows() {
	m.rowLine = make([]int, len(m.flat))
	line := 0
	for i, sec := range m.sections {
		if i > 0 {
			line++
		}
		line++ // section header
		for j := sec.start; j < sec.end; j++ {
			m.rowLine[j] = line
			line++
		}
	}
	m.totalLines = line
}

func (m *model) buildHome() {
	snap := m.store.Snapshot()
	m.interpretation = "Home · type a reference, a word, or a phrase"

	if snap.LastRead.Key != "" {
		if r, ok := m.rowFromItem(rowHistory, snap.LastRead); ok {
			m.addSection("Continue reading", []row{r})
		}
	}
	var recent []row
	for _, it := range snap.History {
		if it.Key == snap.LastRead.Key {
			continue
		}
		if r, ok := m.rowFromItem(rowHistory, it); ok {
			recent = append(recent, r)
		}
		if len(recent) >= 6 {
			break
		}
	}
	m.addSection("Recent", recent)

	var marks []row
	for _, it := range snap.Bookmarks {
		if r, ok := m.rowFromItem(rowBookmark, it); ok {
			marks = append(marks, r)
		}
		if len(marks) >= 6 {
			break
		}
	}
	m.addSection("Bookmarks", marks)

	var notes []row
	for _, n := range snap.Notes {
		notes = append(notes, row{kind: rowNote, ref: mustRef(n.Key), label: n.Ref, text: n.Body, key: n.Key})
		if len(notes) >= 6 {
			break
		}
	}
	m.addSection("Notes", notes)

	var saved []row
	for _, s := range snap.SavedSearches {
		saved = append(saved, m.savedRow(s))
		if len(saved) >= 6 {
			break
		}
	}
	m.addSection("Saved searches", saved)

	if len(m.flat) == 0 {
		m.addSection("Start here", []row{
			{kind: rowRef, ref: ref.Reference{BookID: 43, Chapter: 3, Verse: 16}, label: "John 3:16", text: m.bible.VerseAt(43, 3, 16).Text, key: "43:3:16"},
			{kind: rowRef, ref: ref.Reference{BookID: 19, Chapter: 23, Verse: 1}, label: "Psalms 23", text: m.bible.VerseAt(19, 23, 1).Text, key: "19:23:1"},
		})
	}
	var cmds []row
	for _, id := range []string{"search", "library", "help", "quit"} {
		for _, c := range commandDefs {
			if c.id == id {
				cmds = append(cmds, row{kind: rowCommand, label: c.name, text: c.desc + "  " + c.keys, command: c.id})
			}
		}
	}
	m.addSection("Commands", cmds)
}

func mustRef(key string) ref.Reference {
	r, _ := ref.ParseKey(key)
	return r
}

func (m *model) rowFromItem(kind rowKind, it store.VerseItem) (row, bool) {
	rr, ok := ref.ParseKey(it.Key)
	if !ok {
		return row{}, false
	}
	label := it.Ref
	if label == "" {
		label = rr.Label(m.bible)
	}
	text := it.Text
	if text == "" {
		if v := m.bible.VerseAt(rr.BookID, max(1, rr.Chapter), max(1, rr.Verse)); v != nil {
			text = v.Text
		}
	}
	return row{kind: kind, ref: rr, label: label, text: text, key: it.Key}, true
}

func (m *model) buildQuery(q string) {
	refs, refErr := ref.Parse(m.bible, q, m.currentBook)
	if refErr == nil {
		var rows []row
		for _, r := range refs {
			if r.Verse == 0 {
				v := m.bible.VerseAt(r.BookID, max(1, r.Chapter), 1)
				text := ""
				if v != nil {
					text = v.Text
				}
				rows = append(rows, row{kind: rowChapter, ref: r, label: r.Label(m.bible), text: text, key: r.Key()})
			} else {
				v := m.bible.VerseAt(r.BookID, r.Chapter, r.Verse)
				text := ""
				if v != nil {
					text = v.Text
				}
				rows = append(rows, row{kind: rowRef, ref: r, label: r.Label(m.bible), text: text, key: r.Key()})
			}
		}
		m.addSection("References", rows)
	}

	hasDigits := strings.ContainsAny(q, "0123456789")
	books := ref.FindBooks(q, 6)
	if len(books) > 0 && !hasDigits {
		var rows []row
		for _, bk := range books {
			rows = append(rows, row{
				kind:   rowBook,
				bookID: bk.ID,
				label:  bk.Name,
				text:   fmt.Sprintf("%d chapters · %d verses", bk.Chapters, bk.Verses),
			})
		}
		m.addSection("Books", rows)
	}

	pureRef := refErr == nil && hasDigits
	if !pureRef {
		parsed := search.ParseQuery(q)
		parsed.Scope = m.scope
		parsed.BookID = m.scopeBook
		res := m.index.Search(parsed, func(i int) string { return m.bible.Verses[i].Text }, search.Options{
			Scope:  m.scope,
			BookID: m.scopeBook,
			Limit:  50,
		})
		m.lastResult = res
		m.matchQuery = res.Query
		var rows []row
		for _, h := range res.Hits {
			v := &m.bible.Verses[h.Verse]
			bk := m.bible.BookByID(v.BookID)
			name := "?"
			if bk != nil {
				name = bk.Name
			}
			rr := ref.Reference{BookID: v.BookID, Chapter: v.Chapter, Verse: v.Number}
			rows = append(rows, row{
				kind:  rowVerse,
				ref:   rr,
				verse: h.Verse,
				label: fmt.Sprintf("%s %d:%d", name, v.Chapter, v.Number),
				text:  v.Text,
				key:   rr.Key(),
				score: h.Score,
			})
		}
		m.addSection("Verses", rows)
	}

	m.addSection("Library", m.libraryMatches(q))
	m.addSection("Commands", m.matchingCommands(q, 6))

	switch {
	case len(refs) > 0:
		labels := make([]string, 0, 2)
		for i, r := range refs {
			if i == 2 {
				labels = append(labels, fmt.Sprintf("+%d more", len(refs)-2))
				break
			}
			labels = append(labels, r.Label(m.bible))
		}
		m.interpretation = "Reference · " + strings.Join(labels, " · ")
	case len(books) > 0 && len(m.lastResult.Hits) == 0:
		m.interpretation = "Book · " + books[0].Name
	default:
		parsed := search.ParseQuery(q)
		var parts []string
		for _, t := range parsed.Terms {
			parts = append(parts, t)
		}
		for _, p := range parsed.Phrases {
			parts = append(parts, `"`+strings.Join(p, " ")+`"`)
		}
		for _, e := range parsed.Exclude {
			parts = append(parts, "-"+e)
		}
		desc := strings.Join(parts, " ")
		if desc == "" {
			desc = q
		}
		summary := fmt.Sprintf("Keywords: %s · %s · %s", desc, m.scopeLabel(), verseCount(m.lastResult.Total))
		if m.lastResult.Relaxed {
			summary += " · relaxed (not all terms matched)"
		}
		m.interpretation = summary
	}
}

func verseCount(n int) string {
	if n == 1 {
		return "1 verse"
	}
	return fmt.Sprintf("%d verses", n)
}

func (m *model) libraryMatches(q string) []row {
	lq := strings.ToLower(q)
	var out []row
	seen := make(map[string]bool)
	add := func(r row) {
		if seen[r.key] && r.key != "" {
			return
		}
		if r.key != "" {
			seen[r.key] = true
		}
		out = append(out, r)
	}
	snap := m.store.Snapshot()
	for _, it := range snap.Bookmarks {
		if strings.Contains(strings.ToLower(it.Ref+" "+it.Text), lq) {
			if r, ok := m.rowFromItem(rowBookmark, it); ok {
				add(r)
			}
		}
		if len(out) >= 6 {
			return out
		}
	}
	for _, it := range snap.History {
		if strings.Contains(strings.ToLower(it.Ref+" "+it.Text), lq) {
			if r, ok := m.rowFromItem(rowHistory, it); ok {
				add(r)
			}
		}
		if len(out) >= 6 {
			return out
		}
	}
	for _, n := range snap.Notes {
		if strings.Contains(strings.ToLower(n.Ref+" "+n.Body), lq) {
			add(row{kind: rowNote, ref: mustRef(n.Key), label: n.Ref, text: n.Body, key: n.Key})
		}
		if len(out) >= 6 {
			return out
		}
	}
	for _, s := range snap.SavedSearches {
		if strings.Contains(strings.ToLower(s.Query), lq) {
			add(m.savedRow(s))
		}
		if len(out) >= 6 {
			return out
		}
	}
	return out
}

func (m *model) matchingCommands(q string, limit int) []row {
	lq := strings.ToLower(q)
	if strings.HasPrefix(q, ":") {
		lq = strings.TrimPrefix(lq, ":")
		lq = strings.TrimSpace(lq)
	}
	var out []row
	for _, c := range commandDefs {
		hay := strings.ToLower(c.name + " " + c.desc + " " + c.keys)
		if lq != "" && !strings.Contains(hay, lq) {
			continue
		}
		out = append(out, row{kind: rowCommand, label: c.name, text: c.desc + "  " + c.keys, command: c.id})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (m *model) ensureVisible(avail int) {
	if avail < 1 {
		avail = 1
	}
	if len(m.flat) == 0 || m.sel >= len(m.rowLine) {
		m.scroll = 0
		return
	}
	line := m.rowLine[m.sel]
	if line < m.scroll+1 {
		m.scroll = line - 1
	}
	if line >= m.scroll+avail-1 {
		m.scroll = line - avail + 2
	}
	maxScroll := max(0, m.totalLines-avail)
	if m.scroll < 0 {
		m.scroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
}

func (m *model) rowLabelWidth(width int) int {
	return min(24, max(11, width/4))
}

func (m *model) renderRow(r row, width int, selected bool) string {
	marker := "  "
	if selected {
		marker = cursorMarker.Render("▌ ")
	}
	labelW := m.rowLabelWidth(width)
	label := runewidthTruncate(r.label, labelW)
	label = padTo(label, labelW)
	if selected {
		label = selStyle.Render(label)
	} else {
		label = labelStyle.Render(label)
	}
	avail := max(8, width-labelW-3)
	var body string
	switch r.kind {
	case rowCommand, rowSaved:
		body = mutedStyle.Render(runewidthTruncate(r.text, avail))
	default:
		snip := makeSnippet(r.text, m.matchQuery, avail)
		body = highlightWords(snip, m.matchQuery)
	}
	return marker + label + " " + body
}

func runewidthTruncate(s string, width int) string {
	return truncateANSI(s, width)
}

func (m *model) resultLines(width int) []string {
	var lines []string
	for i, sec := range m.sections {
		if i > 0 {
			lines = append(lines, "")
		}
		title := sectionStyle.Render(strings.ToUpper(sec.title))
		count := mutedStyle.Render(fmt.Sprintf("  %d", sec.end-sec.start))
		lines = append(lines, title+count)
		for j := sec.start; j < sec.end; j++ {
			lines = append(lines, m.renderRow(m.flat[j], width, j == m.sel))
		}
	}
	if len(lines) == 0 {
		lines = append(lines,
			mutedStyle.Render("No matches."),
			"",
			mutedStyle.Render("Try a reference like John 3:16, a book name like Romans,"),
			mutedStyle.Render("a word like shepherd, or a phrase in \"double quotes\"."),
		)
	}
	return lines
}

func (m *model) previewLines(width, height int) []string {
	var lines []string
	r, v, ok := m.focusedVerse()
	if !ok {
		return []string{
			mutedStyle.Render("PREVIEW"),
			"",
			mutedStyle.Render("Select a verse to read it here."),
			"",
			mutedStyle.Render("Enter opens the reader."),
			mutedStyle.Render("y copies · b bookmarks · n adds a note."),
		}
	}
	label := r.Label(m.bible)
	rangeVerses := 0
	if rr, verses, ranged := m.focusedRange(); ranged {
		label = rr.Label(m.bible)
		rangeVerses = len(verses)
	}
	lines = append(lines, sectionStyle.Render("PREVIEW"), "", labelStyle.Render(label), "")
	for _, h := range v.Headings() {
		style := sectionHead
		if h.Kind == "title" {
			style = noteStyle
		}
		for _, rg := range wrapRanges(h.Text, width) {
			lines = append(lines, style.Render(h.Text[rg.Start:rg.End]))
		}
	}
	strongs := v.Strongs()
	active := -1
	if m.showStrongs {
		if _, idx, _, ok := m.focusedStrongs(); ok {
			active = idx
		}
	}
	ranges := wrapVerseRanges(v.Text, width, strongs, m.showStrongs)
	for _, rg := range ranges {
		lines = append(lines, renderVerseRange(v.Text, rg.Start, rg.End, v.Italics(), v.RedLetter(), strongs, active, m.showStrongs))
	}
	if len(m.parallel) > 0 {
		lines = append(lines, "")
		lines = append(lines, m.parallelVerseLines(v, width)...)
	}
	if rangeVerses > 1 {
		lines = append(lines, "", mutedStyle.Render(fmt.Sprintf("… %d verses selected · y copy · b bookmark", rangeVerses)))
	}
	if len(v.Notes()) > 0 {
		lines = append(lines, "")
		for _, n := range v.Notes() {
			for _, rg := range wrapRanges("↳ "+n.Text, width) {
				lines = append(lines, noteStyle.Render(("↳ " + n.Text)[rg.Start:rg.End]))
			}
		}
	}
	if m.showStrongs {
		summary := m.strongsSummary(v, width)
		if len(summary) > 0 {
			lines = append(lines, "")
			lines = append(lines, summary...)
		}
	}
	if note, ok := m.store.Note(r.Key()); ok && strings.TrimSpace(note.Body) != "" {
		body := strings.TrimSpace(note.Body)
		lines = append(lines, "")
		for i, text := range wrapTextLines(body, max(8, width-14)) {
			if i == 0 {
				text = "✎ Your note: " + text
			} else {
				text = "              " + text
			}
			lines = append(lines, userNoteStyle.Render(text))
		}
	}
	if len(lines) > height {
		lines = lines[:max(0, height-1)]
		lines = append(lines, mutedStyle.Render("…"))
	}
	return lines
}

// stackedSearchLayout reports whether there is enough room to show the
// heatmap below the preview in the right column.
func (m *model) stackedSearchLayout() bool {
	return m.width >= 120 && m.height >= 30
}

func (m *model) searchView() string {
	header := m.headerLine(m.headerLeft(), m.headerRight())

	inputLine := m.input.View()
	chip := chipStyle.Render("[" + m.scopeLabel() + "]")
	gap := m.width - lipgloss.Width(inputLine) - lipgloss.Width(chip)
	if gap < 1 {
		inputLine = truncateANSI(inputLine, max(1, m.width-lipgloss.Width(chip)-1))
		gap = max(1, m.width-lipgloss.Width(inputLine)-lipgloss.Width(chip))
	}
	line2 := inputLine + strings.Repeat(" ", gap) + chip
	interp := mutedStyle.Render(truncateANSI(m.interpretation, m.width))

	avail := max(1, m.height-6)
	m.ensureVisible(avail)
	all := m.resultLines(m.width)
	start := min(m.scroll, max(0, len(all)-1))
	end := min(len(all), start+avail)

	hasPreview := m.width >= 110
	resW, prevW := m.width, 0
	if hasPreview {
		resW = (m.width*3)/5 - 1
		prevW = m.width - resW - 3
		if prevW <= 20 {
			resW, prevW, hasPreview = m.width, 0, false
		}
	}
	lines := fitLineSlice(all[start:end], resW, avail)

	if hasPreview {
		stacked := m.stackedSearchLayout()
		if stacked {
			m.syncHeatmap()
		}
		previewH := avail
		var heat []string
		if stacked {
			previewH = max(10, (avail*60)/100)
			heatH := avail - previewH - 1
			if heatH >= 8 {
				heat = m.inlineHeatmapLines(prevW, heatH)
			} else {
				previewH, heat = avail, nil
			}
		}
		right := fitLineSlice(m.previewLines(prevW, previewH), prevW, previewH)
		if heat != nil {
			right = append(right, readerRule.Render(strings.Repeat("─", prevW)))
			right = append(right, heat...)
		}
		joined := make([]string, 0, avail)
		rule := readerRule.Render("│")
		for i := range lines {
			joined = append(joined, padTo(lines[i], resW)+" "+rule+" "+right[i])
		}
		lines = joined
	}
	block := strings.Join(lines, "\n")

	hints := actionHints(m.width) + " · ↑↓ · Enter open · H map · f scope · ? help"
	if m.width >= 110 {
		hints = "Enter open · ↑↓ select · Tab section · " + actionHints(m.width) + " · H full heatmap · f scope · / edit · L library · ? help"
	}
	if !m.resultsFocused {
		hints = "Type · Enter open · ↓ results · Ctrl-S save · Esc clear · ? help"
		if m.width >= 100 {
			hints = "Type to search · Enter open · ↓ results · Tab section · Ctrl-S save · Esc clear · Ctrl-P commands"
		}
	}
	if m.showStrongs && m.resultsFocused {
		switch {
		case m.width >= 120:
			hints = "Enter open · ↑↓ select · Tab section · y copy · b bookmark · n note · w/W word · K entry · H map · f scope · ? help"
		case m.width >= 100:
			hints = "Enter open · ↑↓ select · y copy · b bookmark · n note · w/W word · K entry · H map · ? help"
		default:
			hints = "y copy · b mark · n note · w/W word · K entry · ↑↓ · Enter open · ? help"
		}
	}
	return header + "\n" + truncateANSI(line2, m.width) + "\n" + interp + "\n\n" + block + "\n" + m.footer(hints)
}
