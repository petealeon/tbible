package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/petealeon/tbible/v2/internal/activity"
	"github.com/petealeon/tbible/v2/internal/store"
)

type calendarFocus int

const (
	calFocusCalendar calendarFocus = iota
	calFocusEntries
)

type calendarModel struct {
	year          int
	month         time.Month
	selected      time.Time
	source        activity.Kind
	focus         calendarFocus
	filter        string
	filterEditing bool
	filterInput   textinput.Model
	report        activity.Month
	prevFilter    libraryFilter
}

var calendarSourceKinds = []activity.Kind{
	activity.SourceAll,
	activity.Note,
	activity.Bookmark,
	activity.History,
}

var calendarSourceNames = []string{"All", "Notes", "Bookmarks", "History"}

func localDay(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func sameDay(a, b time.Time) bool {
	a, b = a.Local(), b.Local()
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func (m *model) initCalendar() {
	now := time.Now().Local()
	filter := textinput.New()
	filter.Prompt = "Filter: "
	filter.PromptStyle = promptStyle
	filter.Placeholder = "text or reference"
	filter.CharLimit = 120
	filter.Blur()
	m.calendar = calendarModel{
		year:        now.Year(),
		month:       now.Month(),
		selected:    localDay(now),
		source:      activity.SourceAll,
		filterInput: filter,
		prevFilter:  libAll,
	}
}

func (m *model) rebuildCalendar() {
	c := &m.calendar
	if c.year == 0 || c.month == 0 {
		now := time.Now().Local()
		c.year, c.month, c.selected = now.Year(), now.Month(), localDay(now)
	}
	day := c.selected.Local()
	dayInMonth := min(day.Day(), activity.DaysInMonth(c.year, c.month))
	c.selected = time.Date(c.year, c.month, dayInMonth, 0, 0, 0, 0, time.Local)

	events := m.calendarEvents(m.store.Snapshot())
	if query := strings.ToLower(strings.TrimSpace(c.filter)); query != "" {
		events = filterCalendarEvents(events, query)
	}
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].At.Equal(events[j].At) {
			return events[i].At.After(events[j].At)
		}
		if events[i].Kind != events[j].Kind {
			return events[i].Kind < events[j].Kind
		}
		if events[i].Ref != events[j].Ref {
			return events[i].Ref < events[j].Ref
		}
		return events[i].Text < events[j].Text
	})
	c.report = activity.Build(events, activity.Options{
		Year:   c.year,
		Month:  c.month,
		Source: c.source,
	})

	l := &m.library
	prev := ""
	if l.sel >= 0 && l.sel < len(l.rows) {
		prev = rowIdentity(l.rows[l.sel])
	}
	l.rows = nil
	l.sections = nil
	dayReport := c.report.Day(c.selected)
	var rows []row
	for _, event := range dayReport.Events {
		rows = append(rows, row{
			kind:      calendarRowKind(event.Kind),
			ref:       mustRef(event.Key),
			label:     event.Ref,
			text:      event.Text,
			key:       event.Key,
			at:        event.At,
			eventKind: event.Kind,
		})
	}
	m.addLibrarySection("Entries", rows)
	m.finishLibraryRows(prev)
}

func (m *model) calendarEvents(snap store.State) []activity.Event {
	events := make([]activity.Event, 0, len(snap.Notes)+len(snap.Bookmarks)+len(snap.History))
	for _, n := range snap.Notes {
		events = append(events, activity.Event{
			Kind: activity.Note,
			At:   n.At,
			Key:  n.Key,
			Ref:  n.Ref,
			Text: n.Body,
		})
	}
	for _, item := range snap.Bookmarks {
		events = append(events, activity.Event{
			Kind: activity.Bookmark,
			At:   item.At,
			Key:  item.Key,
			Ref:  item.Ref,
			Text: item.Text,
		})
	}
	for _, item := range snap.History {
		events = append(events, activity.Event{
			Kind: activity.History,
			At:   item.At,
			Key:  item.Key,
			Ref:  item.Ref,
			Text: item.Text,
		})
	}
	return events
}

func filterCalendarEvents(events []activity.Event, query string) []activity.Event {
	out := make([]activity.Event, 0, len(events))
	for _, event := range events {
		if strings.Contains(strings.ToLower(event.Ref), query) ||
			strings.Contains(strings.ToLower(event.Text), query) ||
			strings.Contains(strings.ToLower(event.Key), query) ||
			strings.Contains(strings.ToLower(event.Kind.Label()), query) ||
			strings.Contains(strings.ToLower(event.At.Local().Format("Jan 2 15:04")), query) {
			out = append(out, event)
		}
	}
	return out
}

func calendarRowKind(kind activity.Kind) rowKind {
	switch kind {
	case activity.Note:
		return rowNote
	case activity.Bookmark:
		return rowBookmark
	case activity.History:
		return rowHistory
	default:
		return rowHistory
	}
}

func (m *model) calendarStep(days int) {
	c := &m.calendar
	next := localDay(c.selected).AddDate(0, 0, days)
	c.year, c.month = next.Year(), next.Month()
	c.selected = next
	m.library.sel = 0
	m.rebuildCalendar()
}

func (m *model) calendarStepMonth(delta int) {
	c := &m.calendar
	first := time.Date(c.year, c.month+time.Month(delta), 1, 0, 0, 0, 0, time.Local)
	day := min(c.selected.Day(), activity.DaysInMonth(first.Year(), first.Month()))
	c.year, c.month = first.Year(), first.Month()
	c.selected = time.Date(c.year, c.month, day, 0, 0, 0, 0, time.Local)
	m.library.sel = 0
	m.rebuildCalendar()
}

func (m *model) calendarSetDay(day int) {
	c := &m.calendar
	day = min(max(day, 1), activity.DaysInMonth(c.year, c.month))
	c.selected = time.Date(c.year, c.month, day, 0, 0, 0, 0, time.Local)
	m.library.sel = 0
	m.rebuildCalendar()
}

func (m *model) calendarSetSourceIndex(index int) {
	index = ((index % len(calendarSourceKinds)) + len(calendarSourceKinds)) % len(calendarSourceKinds)
	m.calendar.source = calendarSourceKinds[index]
	m.library.sel = 0
	m.rebuildCalendar()
}

func (m *model) calendarSourceIndex() int {
	for i, kind := range calendarSourceKinds {
		if m.calendar.source == kind {
			return i
		}
	}
	return 0
}

func (m *model) calendarCycleSource(delta int) {
	m.calendarSetSourceIndex(m.calendarSourceIndex() + delta)
}

func (m *model) calendarSetFilter(value string) {
	m.calendar.filter = strings.TrimSpace(value)
	m.library.sel = 0
	m.rebuildCalendar()
}

func (m *model) handleCalendarKey(msg tea.KeyMsg) tea.Cmd {
	c := &m.calendar
	key := msg.String()
	if c.filterEditing {
		switch key {
		case "enter":
			c.filterEditing = false
			c.filterInput.Blur()
			m.calendarSetFilter(c.filterInput.Value())
			c.focus = calFocusEntries
			return nil
		case "esc":
			c.filterEditing = false
			c.filterInput.Blur()
			c.filterInput.SetValue("")
			m.calendarSetFilter("")
			return nil
		}
		prev := c.filterInput.Value()
		var cmd tea.Cmd
		c.filterInput, cmd = c.filterInput.Update(msg)
		if c.filterInput.Value() != prev {
			m.calendarSetFilter(c.filterInput.Value())
		}
		return cmd
	}

	switch key {
	case "ctrl+p", ":":
		m.openPalette()
		return nil
	case "esc":
		if c.filter != "" {
			c.filterInput.SetValue("")
			m.calendarSetFilter("")
			return nil
		}
		m.setLibraryFilter(c.prevFilter)
		return nil
	case "/":
		return m.focusSearch()
	case "?":
		m.help = true
		m.helpScroll = 0
		return nil
	case "f":
		c.filterEditing = true
		c.filterInput.SetValue(c.filter)
		c.filterInput.Focus()
		return textinput.Blink
	case "1", "2", "3", "4":
		index := int(key[0] - '1')
		m.calendarSetSourceIndex(index)
		return nil
	case "s":
		m.calendarCycleSource(1)
		return nil
	case "tab":
		if c.focus == calFocusCalendar {
			c.focus = calFocusEntries
		} else {
			c.focus = calFocusCalendar
		}
		return nil
	case "h", "left":
		m.calendarStep(-1)
		return nil
	case "l", "right":
		m.calendarStep(1)
		return nil
	case "j", "down":
		if c.focus == calFocusEntries {
			m.library.sel = min(len(m.library.rows)-1, m.library.sel+1)
		} else {
			m.calendarStep(7)
		}
		return nil
	case "k", "up":
		if c.focus == calFocusEntries {
			m.library.sel = max(0, m.library.sel-1)
		} else {
			m.calendarStep(-7)
		}
		return nil
	case "[":
		m.calendarStepMonth(-1)
		return nil
	case "]":
		m.calendarStepMonth(1)
		return nil
	case "home":
		m.calendarSetDay(1)
		return nil
	case "end":
		m.calendarSetDay(activity.DaysInMonth(c.year, c.month))
		return nil
	case "enter":
		if c.focus == calFocusEntries {
			return m.activateLibrary()
		}
		if len(m.library.rows) > 0 {
			c.focus = calFocusEntries
			m.library.sel = 0
		}
		return nil
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
	}
	return nil
}

func (m *model) calendarLayout(width, avail int) (calW, calH, detailW, detailH int, side bool) {
	if width >= 120 && m.height >= 30 {
		calW = 64
		detailW = width - calW - 3
		if detailW >= 36 {
			return calW, avail, detailW, avail, true
		}
	}
	calH = min(11, max(6, avail-5))
	detailH = avail - calH - 1
	if detailH < 4 {
		detailH = max(1, avail/2)
		calH = max(1, avail-detailH-1)
	}
	return width, calH, width, detailH, false
}

func (m *model) calendarDetailHeaderCount() int {
	n := 2
	if m.calendar.filter != "" {
		n++
	}
	if m.calendar.filterEditing {
		n++
	}
	return n
}

func (m *model) calendarEnsureVisible(avail int) {
	l := &m.library
	if len(l.rows) == 0 {
		l.scroll = 0
		return
	}
	visible := max(1, avail-m.calendarDetailHeaderCount())
	if l.sel < l.scroll {
		l.scroll = l.sel
	}
	if l.sel >= l.scroll+visible {
		l.scroll = l.sel - visible + 1
	}
	maxScroll := max(0, len(l.rows)-visible)
	l.scroll = min(max(l.scroll, 0), maxScroll)
}

func (m *model) calendarView() string {
	header := m.headerLine(m.headerLeft(), m.headerRight())
	tabs := m.libraryTabs(m.width)
	avail := m.libraryBodyHeight()
	_, _, _, detailH, _ := m.calendarLayout(m.width, avail)
	m.calendarEnsureVisible(detailH)
	block := m.calendarBlock(m.width, avail)

	hints := "←/→ day · ↑/↓ week · [ ] month · Tab focus · 1-4 source · f filter · Enter open · Esc back · ? help"
	if m.width < 100 {
		hints = "h/l day · j/k week · [ ] month · Tab focus · f filter · Enter open · Esc back"
	}
	if m.calendar.focus == calFocusEntries {
		hints = actionHints(m.width) + " · " + hints
	}
	return header + "\n" + tabs + "\n\n" + block + "\n" + m.footer(hints)
}

func (m *model) calendarBlock(width, avail int) string {
	calW, calH, detailW, detailH, side := m.calendarLayout(width, avail)
	calLines := fitLineSlice(m.calendarGridLines(calW, calH), calW, calH)
	detailLines := fitLineSlice(m.calendarDetailLines(detailW, detailH), detailW, detailH)
	if side {
		joined := make([]string, 0, avail)
		rule := readerRule.Render("│")
		for i := 0; i < avail; i++ {
			joined = append(joined, padTo(calLines[i], calW)+" "+rule+" "+detailLines[i])
		}
		return strings.Join(joined, "\n")
	}
	out := append(calLines, readerRule.Render(strings.Repeat("─", width)))
	out = append(out, detailLines...)
	return strings.Join(out, "\n")
}

func calendarCellWidth(width int) int {
	switch {
	case width >= 62:
		return 8
	case width >= 48:
		return 6
	default:
		return max(4, (width-6)/7)
	}
}

func (m *model) calendarGridLines(width, height int) []string {
	c := &m.calendar
	cellW := calendarCellWidth(width)
	weekdays := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	if cellW < 8 {
		weekdays = []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
	}
	weekParts := make([]string, len(weekdays))
	for i, name := range weekdays {
		weekParts[i] = padTo(name, cellW)
	}

	notes, bookmarks, history := c.report.Totals()
	lines := []string{
		sectionStyle.Render("CALENDAR"),
		mutedStyle.Render(truncateANSI(fmt.Sprintf("%s %d · %d notes · %d bookmarks · %d history",
			c.month.String(), c.year, notes, bookmarks, history), width)),
		m.calendarSourceLine(width),
		strings.Join(weekParts, " "),
	}
	for _, week := range c.report.Weeks {
		cells := make([]string, len(week))
		for i, day := range week {
			cells[i] = m.calendarCell(day, cellW)
		}
		lines = append(lines, strings.Join(cells, " "))
	}
	lines = append(lines, m.calendarLegendLine(width))
	return fitLineSlice(lines, width, height)
}

func (m *model) calendarSourceLine(width int) string {
	parts := []string{"source"}
	for i, name := range calendarSourceNames {
		if m.calendar.source == calendarSourceKinds[i] {
			parts = append(parts, chipStyle.Render("["+name+"]"))
		} else {
			parts = append(parts, chipDim.Render(" "+name+" "))
		}
	}
	if m.calendar.filter != "" {
		parts = append(parts, mutedStyle.Render("filter: "+m.calendar.filter))
	}
	return truncateANSI(strings.Join(parts, " "), width)
}

func (m *model) calendarCell(day activity.Day, cellW int) string {
	if !day.InMonth {
		return padTo(mutedStyle.Render("  ·"), cellW)
	}
	number := fmt.Sprintf("%2d", day.Date.Day())
	glyph := calendarHeatGlyph(day)
	body := number + glyph
	if cellW >= 6 {
		body = number + " " + glyph
	}
	if sameDay(day.Date, m.calendar.selected) {
		body = cursorMarker.Render("▌") + body
	}
	return padTo(truncateANSI(body, cellW), cellW)
}

func calendarHeatGlyph(day activity.Day) string {
	if day.Total == 0 {
		return mutedStyle.Render(heatGlyphs[0])
	}
	style := calendarKindStyle(day.Dominant).Bold(day.Level >= 3)
	level := min(day.Level, len(heatGlyphs)-1)
	return style.Render(heatGlyphs[level])
}

func calendarKindStyle(kind activity.Kind) lipgloss.Style {
	switch kind {
	case activity.Note:
		return calNoteStyle
	case activity.Bookmark:
		return calBookmarkStyle
	case activity.History:
		return calHistoryStyle
	default:
		return mutedStyle
	}
}

func (m *model) calendarLegendLine(width int) string {
	parts := []string{mutedStyle.Render("low")}
	for i, glyph := range heatGlyphs {
		parts = append(parts, heatStyle(i, false).Render(glyph))
	}
	parts = append(parts,
		mutedStyle.Render("high"),
		calNoteStyle.Render("N note"),
		calBookmarkStyle.Render("B bookmark"),
		calHistoryStyle.Render("H history"),
	)
	return truncateANSI(strings.Join(parts, " "), width)
}

func (m *model) calendarDetailLines(width, height int) []string {
	c := &m.calendar
	day := c.report.Day(c.selected)
	lines := []string{
		sectionStyle.Render("ENTRIES"),
		mutedStyle.Render(truncateANSI(fmt.Sprintf("%s · %d entries",
			c.selected.Local().Format("Mon, Jan 2"), day.Total), width)),
	}
	if c.filter != "" {
		lines = append(lines, mutedStyle.Render(truncateANSI("Filter: "+c.filter, width)))
	}
	if c.filterEditing {
		lines = append(lines, c.filterInput.View())
	}
	if len(m.library.rows) == 0 {
		lines = append(lines,
			"",
			mutedStyle.Render("Nothing on this day."),
			mutedStyle.Render("Move to a busier day, or change the source filter."),
		)
		return fitLineSlice(lines, width, height)
	}
	visible := max(1, height-m.calendarDetailHeaderCount())
	start := min(m.library.scroll, max(0, len(m.library.rows)-1))
	end := min(len(m.library.rows), start+visible)
	for i := start; i < end; i++ {
		selected := c.focus == calFocusEntries && i == m.library.sel
		lines = append(lines, m.renderCalendarEntry(m.library.rows[i], width, selected))
	}
	return fitLineSlice(lines, width, height)
}

func (m *model) renderCalendarEntry(r row, width int, selected bool) string {
	marker := "  "
	if selected {
		marker = cursorMarker.Render("▌ ")
	}
	when := "--:--"
	if !r.at.IsZero() {
		when = r.at.Local().Format("15:04")
	}
	kind := padTo(calendarKindStyle(r.eventKind).Render(r.eventKind.Label()), 9)
	refW := min(16, max(10, width/4))
	ref := padTo(truncateANSI(r.label, refW), refW)
	if selected {
		ref = selStyle.Render(ref)
	} else {
		ref = labelStyle.Render(ref)
	}
	bodyW := max(4, width-2-5-1-9-1-refW-1)
	bodyStyle := textStyle
	if r.eventKind == activity.Note {
		bodyStyle = noteStyle
	}
	body := bodyStyle.Render(truncateANSI(r.text, bodyW))
	return marker + mutedStyle.Render(when) + " " + kind + " " + ref + " " + body
}
