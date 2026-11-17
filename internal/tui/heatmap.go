package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/petealeon/tbible/v2/internal/heatmap"
	"github.com/petealeon/tbible/v2/internal/ref"
	"github.com/petealeon/tbible/v2/internal/search"
)

var heatGlyphs = []string{"·", "░", "▒", "▓", "█"}

// heatmapCacheKey identifies the search result behind a cached report.
type heatmapCacheKey struct {
	query     string
	scope     search.Scope
	scopeBook uint16
	mode      heatmap.Mode
	total     int
	hits      int
}

// heatmapModel holds the shared heatmap report, its cache key, and the
// full-screen view's navigation state. The inline overview only reads the
// report so it cannot disturb this navigation state.
type heatmapModel struct {
	open   bool
	report *heatmap.Report
	mode   heatmap.Mode
	book   int
	chptr  int
	detail bool
	scroll int

	cacheKey   heatmapCacheKey
	cacheValid bool
}

func heatGlyph(level int) string {
	level = min(max(level, 0), len(heatGlyphs)-1)
	return heatGlyphs[level]
}

// openHeatmap shows the full-screen heatmap. The report itself is shared with
// the inline overview so it is built at most once per result set.
func (m *model) openHeatmap() tea.Cmd {
	hits := m.lastResult.AllHits
	if len(hits) == 0 {
		hits = m.lastResult.Hits
	}
	if len(hits) == 0 {
		m.setStatus("Run a search first", true)
		return nil
	}

	h := &m.heat
	h.mode = heatmap.ModeDensity
	h.detail = false
	h.book = 0
	h.chptr = 0
	h.scroll = 0
	if m.scope == search.ScopeBook {
		for i := range m.bible.Books {
			if m.bible.Books[i].ID == m.scopeBook {
				h.book = i
				break
			}
		}
	}
	m.syncHeatmap()
	h.open = true
	return nil
}

func (m *model) rebuildHeatmap() {
	m.syncHeatmap()
}

// syncHeatmap builds the report when the query, scope, metric, or result set
// changed. View calls this for the inline overview; openHeatmap calls it for
// the full-screen view. Both paths therefore share one cached report.
func (m *model) syncHeatmap() {
	h := &m.heat
	hits := m.lastResult.AllHits
	if len(hits) == 0 {
		hits = m.lastResult.Hits
	}
	if len(hits) == 0 {
		h.report = nil
		h.cacheValid = false
		return
	}

	key := heatmapCacheKey{
		query:     m.query,
		scope:     m.scope,
		scopeBook: m.scopeBook,
		mode:      h.mode,
		total:     m.lastResult.Total,
		hits:      len(hits),
	}
	if h.cacheValid && h.cacheKey == key && h.report != nil {
		return
	}
	h.report = heatmap.Build(m.bible, hits, heatmap.Options{
		Mode:   h.mode,
		Scope:  m.scope,
		BookID: m.scopeBook,
	})
	h.cacheKey = key
	h.cacheValid = true
	clampHeatmapSelection(h)
}

func clampHeatmapSelection(h *heatmapModel) {
	if h.report == nil || len(h.report.Books) == 0 {
		h.book, h.chptr, h.scroll = 0, 0, 0
		return
	}
	h.book = min(max(h.book, 0), len(h.report.Books)-1)
	chapters := len(h.report.Books[h.book].Chapters)
	h.chptr = min(max(h.chptr, 0), max(0, chapters-1))
	if h.scroll < 0 {
		h.scroll = 0
	}
}

// inlineHeatmapBook returns the report index for the focused result's book.
// It never mutates the full-screen heatmap's own selection.
func (m *model) inlineHeatmapBook() int {
	h := &m.heat
	if h.report == nil || len(h.report.Books) == 0 {
		return -1
	}
	if r, ok := m.focusedReference(); ok && r.BookID != 0 {
		for i := range h.report.Books {
			if h.report.Books[i].ID == r.BookID {
				return i
			}
		}
	}
	for i := range h.report.Books {
		if h.report.Books[i].Matches > 0 {
			return i
		}
	}
	return 0
}

// inlineHeatmapLines renders the compact, read-only overview used below the
// preview in wide terminals. The selected result's book is highlighted and
// kept in view as results are navigated.
func (m *model) inlineHeatmapLines(width, height int) []string {
	width, height = max(1, width), max(0, height)
	if height == 0 {
		return nil
	}
	h := &m.heat
	if h.report == nil || len(h.report.Books) == 0 {
		return fitLineSlice([]string{
			sectionStyle.Render("HEATMAP"),
			mutedStyle.Render("Run a search to see match concentration."),
		}, width, height)
	}

	selected := m.inlineHeatmapBook()
	showLegend := height >= 10
	rowCap := max(1, height-2)
	if showLegend {
		rowCap = max(1, rowCap-1)
	}
	start := 0
	if selected > 0 {
		start = selected - rowCap/2
	}
	start = max(start, 0)
	start = min(start, max(0, len(h.report.Books)-rowCap))
	end := min(len(h.report.Books), start+rowCap)

	summary := fmt.Sprintf("%s · %d matches", h.mode.Label(), h.report.Total)
	if end < len(h.report.Books) {
		summary += fmt.Sprintf(" · %d-%d/%d", start+1, end, len(h.report.Books))
	}
	lines := []string{
		sectionStyle.Render("HEATMAP"),
		mutedStyle.Render(truncateANSI(summary, width)),
	}
	for i := start; i < end; i++ {
		lines = append(lines, m.renderHeatmapBookRow(h.report.Books[i], width, i == selected))
	}
	if showLegend {
		lines = append(lines, m.compactHeatmapLegend(width))
	}
	return fitLineSlice(lines, width, height)
}

func (m *model) compactHeatmapLegend(width int) string {
	h := &m.heat
	parts := []string{mutedStyle.Render("scale")}
	for i, glyph := range heatGlyphs {
		parts = append(parts, heatStyle(i, false).Render(glyph))
	}
	parts = append(parts, mutedStyle.Render(h.mode.Label()))
	return truncateANSI(strings.Join(parts, "  "), max(1, width))
}

func (m *model) handleHeatmapKey(msg tea.KeyMsg) tea.Cmd {
	h := &m.heat
	switch msg.String() {
	case "esc":
		if h.detail {
			h.detail = false
			h.chptr = 0
			h.scroll = 0
			return nil
		}
		h.open = false
		return nil
	case "H":
		h.open = false
		return nil
	case "d":
		if h.mode == heatmap.ModeDensity {
			h.mode = heatmap.ModeCount
		} else {
			h.mode = heatmap.ModeDensity
		}
		m.rebuildHeatmap()
		return nil
	case "j", "down":
		m.moveHeatmap(1)
		return nil
	case "k", "up":
		m.moveHeatmap(-1)
		return nil
	case "g", "home":
		if h.detail {
			h.chptr = 0
		} else {
			h.book = 0
		}
		h.scroll = 0
		return nil
	case "G", "end":
		if h.report != nil && len(h.report.Books) > 0 {
			if h.detail {
				h.chptr = len(h.report.Books[h.book].Chapters) - 1
			} else {
				h.book = len(h.report.Books) - 1
			}
			clampHeatmapSelection(h)
		}
		return nil
	case "enter", "right", "l":
		if !h.detail {
			h.detail = true
			h.chptr = 0
			h.scroll = 0
			return nil
		}
		return m.openHeatmapVerse()
	case "left", "h":
		if h.detail {
			h.detail = false
			h.chptr = 0
			h.scroll = 0
		} else {
			h.open = false
		}
		return nil
	case "?":
		m.help = true
		m.helpScroll = 0
		return nil
	case ":", "ctrl+p":
		h.open = false
		m.openPalette()
		return nil
	}
	return nil
}

func (m *model) moveHeatmap(delta int) {
	h := &m.heat
	if h.report == nil || len(h.report.Books) == 0 {
		return
	}
	if h.detail {
		last := max(0, len(h.report.Books[h.book].Chapters)-1)
		h.chptr = min(max(h.chptr+delta, 0), last)
		return
	}
	h.book = min(max(h.book+delta, 0), len(h.report.Books)-1)
}

func (m *model) openHeatmapVerse() tea.Cmd {
	h := &m.heat
	if h.report == nil || h.book < 0 || h.book >= len(h.report.Books) {
		return nil
	}
	bk := h.report.Books[h.book]
	if h.chptr < 0 || h.chptr >= len(bk.Chapters) {
		return nil
	}
	ch := bk.Chapters[h.chptr]
	verse := uint16(1)
	if ch.TopVerse > 0 {
		verse = ch.TopVerse
	}
	h.open = false
	return m.openReference(ref.Reference{BookID: bk.ID, Chapter: ch.Number, Verse: verse})
}

func (m *model) heatmapOverviewLines(width int) ([]string, []int) {
	h := &m.heat
	if h.report == nil {
		return nil, nil
	}
	books := h.report.Books
	lines := []string{sectionStyle.Render("OLD TESTAMENT")}
	lineOfBook := make([]int, len(books))
	for i := range books {
		if i > 0 && books[i].Testament != books[i-1].Testament {
			lines = append(lines, "", sectionStyle.Render("NEW TESTAMENT"))
		}
		lineOfBook[i] = len(lines)
		lines = append(lines, m.renderHeatmapBookRow(books[i], width, i == h.book))
	}
	return lines, lineOfBook
}

func (m *model) heatmapDetailLines(width int) []string {
	h := &m.heat
	if h.report == nil || h.book < 0 || h.book >= len(h.report.Books) {
		return nil
	}
	bk := h.report.Books[h.book]
	lines := []string{sectionStyle.Render(strings.ToUpper(bk.Name) + " · CHAPTERS")}
	for i := range bk.Chapters {
		lines = append(lines, m.renderHeatmapChapterRow(bk.Chapters[i], width, i == h.chptr))
	}
	return lines
}

func (m *model) renderHeatmapBookRow(b heatmap.Book, width int, selected bool) string {
	marker := "  "
	label := b.Name
	if label == "" || (width < 80 && b.Abbrev != "") {
		label = b.Abbrev
	}
	if label == "" {
		label = fmt.Sprintf("Book %d", b.ID)
	}
	labelW := min(12, max(5, width/9))
	label = padTo(truncateANSI(label, labelW), labelW)
	if selected {
		marker = cursorMarker.Render("▌ ")
		label = selStyle.Render(label)
	} else if !b.InScope {
		label = mutedStyle.Render(label)
	} else {
		label = labelStyle.Render(label)
	}

	barW := max(6, min(60, width-labelW-26))
	var bar strings.Builder
	for _, score := range heatmap.Bucket(b.Chapters, barW) {
		level := heatLevel(score)
		bar.WriteString(heatStyle(level, !b.InScope).Render(heatGlyph(level)))
	}
	nums := fmt.Sprintf("%4d %5.1f%%", b.Matches, 100*b.Density)
	if !b.InScope {
		nums = mutedStyle.Render(nums)
	}
	return marker + label + " " + bar.String() + " " + nums
}

func (m *model) renderHeatmapChapterRow(ch heatmap.Chapter, width int, selected bool) string {
	marker := "  "
	if selected {
		marker = cursorMarker.Render("▌ ")
	}
	num := padLeft(fmt.Sprintf("%d", ch.Number), 3)
	if selected {
		num = cursorNum.Render(num)
	} else {
		num = verseNumStyle.Render(num)
	}

	barW := max(6, min(60, width-32))
	level := heatLevel(ch.Score)
	filled := min(barW, int(ch.Score*float64(barW)+0.5))
	var bar strings.Builder
	for i := 0; i < barW; i++ {
		if i < filled {
			bar.WriteString(heatStyles[level].Render(heatGlyph(level)))
		} else {
			bar.WriteString(heatStyle(0, false).Render(heatGlyph(0)))
		}
	}
	top := ""
	if ch.TopVerse > 0 {
		top = fmt.Sprintf("top v%d", ch.TopVerse)
	}
	nums := fmt.Sprintf("%4d %5.1f%% %-8s", ch.Matches, 100*ch.Density, top)
	return marker + num + " " + bar.String() + " " + nums
}

func (m *model) heatmapLegend(width int) string {
	h := &m.heat
	if h.report == nil {
		return ""
	}
	parts := []string{
		mutedStyle.Render("scale"),
	}
	for i, glyph := range heatGlyphs {
		parts = append(parts, heatStyle(i, false).Render(glyph))
	}
	metric := h.mode.Label()
	if h.mode == heatmap.ModeDensity {
		metric += " = matches / verses"
	} else {
		metric += " = raw matches"
	}
	parts = append(parts,
		mutedStyle.Render("low → high"),
		mutedStyle.Render(metric),
		mutedStyle.Render("dimmed = outside scope"),
	)
	return truncateANSI(strings.Join(parts, "  ·  "), max(1, width))
}

func (m *model) heatmapEnsureVisible(total, selected, avail int) {
	h := &m.heat
	if total <= 0 || avail <= 0 {
		h.scroll = 0
		return
	}
	if selected < h.scroll {
		h.scroll = selected
	}
	if selected >= h.scroll+avail {
		h.scroll = selected - avail + 1
	}
	maxScroll := max(0, total-avail)
	h.scroll = min(max(h.scroll, 0), maxScroll)
}

func (m *model) heatmapView() string {
	header := m.headerLine(appTitle.Render("Result heatmap"), mutedStyle.Render("Esc close · H close"))
	h := &m.heat
	if h.report == nil {
		return header + "\n\n" + mutedStyle.Render("No heatmap data.") + "\n" + m.footer("Esc close")
	}

	summary := fmt.Sprintf("%q · %s · %d matches · %s",
		m.query, m.scopeLabel(), h.report.Total, h.mode.Label())
	avail := max(1, m.height-6)

	var lines []string
	selected := 0
	if h.detail {
		lines = m.heatmapDetailLines(m.width)
		selected = min(h.chptr+1, max(0, len(lines)-1))
	} else {
		var lineOfBook []int
		lines, lineOfBook = m.heatmapOverviewLines(m.width)
		if h.book >= 0 && h.book < len(lineOfBook) {
			selected = lineOfBook[h.book]
		}
	}
	clampHeatmapSelection(h)
	m.heatmapEnsureVisible(len(lines), selected, avail)

	start := min(h.scroll, max(0, len(lines)-1))
	end := min(len(lines), start+avail)
	block := fitLines(lines[start:end], m.width, avail)

	hints := "j/k select · Enter drill/open · ← back · d count/density · Esc close · ? help"
	if m.width < 110 {
		hints = "j/k select · Enter open · ← back · d metric · Esc close"
	}
	return header + "\n" + mutedStyle.Render(truncateANSI(summary, m.width)) + "\n" +
		m.heatmapLegend(m.width) + "\n\n" + block + "\n" + m.footer(hints)
}
