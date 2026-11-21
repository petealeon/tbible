package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/petealeon/tbible/v2/internal/catalog"
	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/library"
	"github.com/petealeon/tbible/v2/internal/packs"
)

// parallelView is a loaded secondary translation. The corpus is shared and
// read-only; up to four translations are kept resident by the library cache.
type parallelView struct {
	id    string
	name  string
	bible *corpus.Bible
}

type transRowKind int

const (
	transInstalled transRowKind = iota
	transCatalog
)

type transRow struct {
	kind      transRowKind
	id        string
	name      string
	detail    string
	status    string
	installed bool
	entry     catalog.Entry
}

type catalogMsg struct {
	cat *catalog.Catalog
	err error
}

type packMsg struct {
	id     string
	action string // install or remove
	err    error
}

func defaultLibrary() *library.Library {
	lib, err := library.Default()
	if err == nil {
		return lib
	}
	base, err := os.UserHomeDir()
	if err != nil {
		base = os.TempDir()
	}
	return library.New(filepath.Join(base, ".tbible", "packs"))
}

// cleanParallelIDs drops unavailable, duplicate, and primary translations.
func cleanParallelIDs(ids []string, primary string) []string {
	var out []string
	seen := map[string]bool{primary: true}
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" || seen[id] || id == library.KJVId && primary == library.KJVId {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) == 3 {
			break
		}
	}
	return out
}

func (m *model) loadParallel() {
	m.parallelIDs = cleanParallelIDs(m.parallelIDs, m.translationID)
	m.parallel = nil
	for _, id := range m.parallelIDs {
		b, _, err := m.packLib.Get(id)
		if err != nil {
			continue
		}
		m.parallel = append(m.parallel, parallelView{id: id, name: m.packLib.Name(id), bible: b})
	}
}

func (m *model) translationLabel() string {
	if m.translationID == library.KJVId || m.translationID == "" {
		return "KJV"
	}
	return strings.ToUpper(m.translationID)
}

func (m *model) translationName() string {
	return m.packLib.Name(m.translationID)
}

// openTrans opens the translation picker.
func (m *model) openTrans() tea.Cmd {
	m.transOpen = true
	m.rebuildTransRows()
	return nil
}

func (m *model) closeTrans() {
	m.transOpen = false
}

func (m *model) rebuildTransRows() {
	available, _ := m.packLib.Available()
	var rows []transRow
	for _, t := range available {
		row := transRow{
			kind:      transInstalled,
			id:        t.ID,
			name:      t.Name,
			detail:    "installed locally",
			installed: true,
		}
		switch {
		case t.ID == m.translationID:
			row.status = "primary"
		case containsString(m.parallelIDs, t.ID):
			row.status = "parallel"
		default:
			row.status = "available"
		}
		if t.ID == library.KJVId {
			row.detail = "built in"
		}
		rows = append(rows, row)
	}
	if m.cat != nil {
		for _, e := range m.cat.Translations {
			if e.ID == library.KJVId || m.packLib.Has(e.ID) {
				continue
			}
			detail := e.Description
			if detail == "" {
				detail = e.License
			}
			if e.Size > 0 {
				detail = fmt.Sprintf("%s · %.1f MB", detail, float64(e.Size)/(1<<20))
			}
			rows = append(rows, transRow{
				kind:   transCatalog,
				id:     e.ID,
				name:   e.Name,
				detail: strings.TrimPrefix(detail, " · "),
				status: "download",
				entry:  e,
			})
		}
	}
	m.transRows = rows
	if m.transSel >= len(rows) {
		m.transSel = max(0, len(rows)-1)
	}
	if m.transSel < 0 {
		m.transSel = 0
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (m *model) handleTransKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "q", "T":
		m.closeTrans()
		return nil
	case "up", "k":
		if m.transSel > 0 {
			m.transSel--
		}
		return nil
	case "down", "j":
		if m.transSel < len(m.transRows)-1 {
			m.transSel++
		}
		return nil
	case "g", "home":
		m.transSel = 0
		return nil
	case "G", "end":
		m.transSel = max(0, len(m.transRows)-1)
		return nil
	case "pgdown", "ctrl+d":
		m.transSel = min(max(0, len(m.transRows)-1), m.transSel+6)
		return nil
	case "pgup", "ctrl+u":
		m.transSel = max(0, m.transSel-6)
		return nil
	case "enter":
		if m.transBusy || len(m.transRows) == 0 {
			return nil
		}
		row := m.transRows[m.transSel]
		if row.kind == transCatalog {
			return m.installCatalog(row.entry)
		}
		return m.setPrimary(row.id)
	case " ":
		if m.transBusy || len(m.transRows) == 0 {
			return nil
		}
		row := m.transRows[m.transSel]
		if row.kind == transInstalled {
			return m.toggleParallel(row.id)
		}
		return nil
	case "r", "R":
		if m.transBusy || len(m.transRows) == 0 {
			return nil
		}
		row := m.transRows[m.transSel]
		if row.kind == transInstalled {
			return m.removePack(row.id)
		}
		return nil
	case "u", "U":
		return m.refreshCatalog()
	}
	return nil
}

func (m *model) setPrimary(id string) tea.Cmd {
	if id == m.translationID {
		m.setStatus(m.packLib.Name(id)+" is already primary", false)
		return nil
	}
	b, ix, err := m.packLib.Get(id)
	if err != nil {
		m.setStatus("Cannot open translation: "+err.Error(), true)
		return nil
	}
	m.bible, m.index, m.translationID = b, ix, id
	m.loadParallel()
	m.rebuild()
	m.rebuildTransRows()
	m.setStatus("Reading "+m.packLib.Name(id), false)
	return m.SaveSettings()
}

func (m *model) toggleParallel(id string) tea.Cmd {
	if id == m.translationID {
		m.setStatus("Choose a different primary translation first", true)
		return nil
	}
	if containsString(m.parallelIDs, id) {
		out := m.parallelIDs[:0]
		for _, v := range m.parallelIDs {
			if v != id {
				out = append(out, v)
			}
		}
		m.parallelIDs = out
	} else {
		if len(m.parallelIDs) >= 3 {
			m.setStatus("Up to three parallel translations are shown", true)
			return nil
		}
		m.parallelIDs = append(m.parallelIDs, id)
	}
	m.loadParallel()
	m.rebuild()
	m.rebuildTransRows()
	m.setStatus("Parallel translations updated", false)
	return m.SaveSettings()
}

func (m *model) removePack(id string) tea.Cmd {
	if id == library.KJVId {
		m.setStatus("The embedded KJV cannot be removed", true)
		return nil
	}
	if m.store.Ephemeral() {
		m.setStatus("Packs cannot be changed in ephemeral mode", true)
		return nil
	}
	if err := packs.Remove(m.packLib.Dir(), id); err != nil {
		m.setStatus("Remove failed: "+err.Error(), true)
		return nil
	}
	m.packLib.Invalidate(id)
	if m.translationID == id {
		b, ix, err := m.packLib.Get(library.KJVId)
		if err == nil {
			m.bible, m.index, m.translationID = b, ix, library.KJVId
		}
	}
	out := m.parallelIDs[:0]
	for _, v := range m.parallelIDs {
		if v != id {
			out = append(out, v)
		}
	}
	m.parallelIDs = out
	m.loadParallel()
	m.rebuild()
	m.rebuildTransRows()
	m.setStatus("Removed "+id, false)
	return m.SaveSettings()
}

func (m *model) refreshCatalog() tea.Cmd {
	if m.store.Ephemeral() {
		m.setStatus("Catalog refresh is unavailable in ephemeral mode", true)
		return nil
	}
	source := ""
	if m.cat != nil {
		source = m.cat.Source
	}
	if source == "" {
		m.setStatus("No catalog source is configured", true)
		return nil
	}
	m.transBusy = true
	m.setStatus("Refreshing translation catalog…", false)
	return func() tea.Msg {
		c, err := catalog.Refresh(context.Background(), source)
		return catalogMsg{cat: c, err: err}
	}
}

func (m *model) finishCatalogRefresh(msg catalogMsg) {
	m.transBusy = false
	if msg.err != nil {
		m.setStatus("Catalog refresh failed: "+msg.err.Error(), true)
		return
	}
	m.cat = msg.cat
	m.rebuildTransRows()
	m.setStatus("Catalog updated · "+fmt.Sprintf("%d translations", len(msg.cat.Translations)), false)
}

func (m *model) installCatalog(e catalog.Entry) tea.Cmd {
	if m.store.Ephemeral() {
		m.setStatus("Downloads are unavailable in ephemeral mode", true)
		return nil
	}
	if e.URL == "" || e.SHA256 == "" {
		m.setStatus("Catalog entry has no download information", true)
		return nil
	}
	m.transBusy = true
	m.setStatus("Downloading "+e.Name+"…", false)
	dir := m.packLib.Dir()
	return func() tea.Msg {
		data, err := catalog.Download(context.Background(), e.URL)
		if err != nil {
			return packMsg{id: e.ID, action: "install", err: err}
		}
		_, err = packs.VerifyAndInstall(dir, data, e.SHA256)
		return packMsg{id: e.ID, action: "install", err: err}
	}
}

func (m *model) finishPackAction(msg packMsg) {
	m.transBusy = false
	if msg.err != nil {
		m.setStatus(strings.Title(msg.action)+" failed: "+msg.err.Error(), true)
		return
	}
	m.packLib.Invalidate(msg.id)
	m.rebuildTransRows()
	name := m.packLib.Name(msg.id)
	if msg.action == "install" {
		m.setStatus("Installed "+name+" · Enter sets it primary", false)
	} else {
		m.setStatus("Removed "+name, false)
	}
}

func (m *model) transView() string {
	contentW := max(40, min(86, m.width-6))
	var lines []string
	primary := m.packLib.Name(m.translationID)
	if len(m.parallel) > 0 {
		var names []string
		for _, p := range m.parallel {
			names = append(names, p.name)
		}
		primary += " + " + strings.Join(names, ", ")
	}
	lines = append(lines,
		sectionStyle.Render("TRANSLATIONS"),
		mutedStyle.Render(truncateANSI("Reading: "+primary, contentW)),
		"",
	)
	if m.transBusy {
		lines = append(lines, mutedStyle.Render("Working…"), "")
	}
	if len(m.transRows) == 0 {
		lines = append(lines, mutedStyle.Render("No translations are available."))
	}
	visible := min(14, max(4, m.height-11))
	start := 0
	if m.transSel >= visible {
		start = m.transSel - visible + 1
	}
	end := min(len(m.transRows), start+visible)
	nameW := min(28, max(16, contentW/3))
	for i := start; i < end; i++ {
		row := m.transRows[i]
		marker := "  "
		if i == m.transSel {
			marker = cursorMarker.Render("▌ ")
		}
		name := padTo(row.name, nameW)
		if i == m.transSel {
			name = selStyle.Render(name)
		} else {
			name = labelStyle.Render(name)
		}
		status := mutedStyle.Render(row.status)
		if row.status == "primary" || row.status == "parallel" {
			status = strongTagStyle.Render(row.status)
		}
		detailW := max(8, contentW-nameW-lipgloss.Width(row.status)-5)
		line := marker + name + " " + mutedStyle.Render(truncateANSI(row.detail, detailW)) + " " + status
		lines = append(lines, truncateANSI(line, contentW))
	}
	lines = append(lines,
		"",
		footerStyle.Render("Enter set primary / install · Space parallel · R remove · U refresh · Esc close"),
	)
	for i := range lines {
		lines[i] = truncateANSI(lines[i], contentW)
	}
	panel := panelStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(max(1, m.width), max(1, m.height), lipgloss.Center, lipgloss.Center, panel)
}

// parallelCopyText returns the passage from every active translation, each
// line labelled so a pasted comparison stays unambiguous.
func (m *model) parallelCopyText(verses []*corpus.Verse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s", m.translationLabel(), joinVerseText(verses))
	for _, p := range m.parallel {
		items := make([]*corpus.Verse, 0, len(verses))
		for _, v := range verses {
			if ov := p.bible.VerseAt(v.BookID, v.Chapter, v.Number); ov != nil {
				items = append(items, ov)
			}
		}
		if len(items) > 0 {
			fmt.Fprintf(&b, "\n%s: %s", strings.ToUpper(p.id), joinVerseText(items))
		}
	}
	return b.String()
}

func joinVerseText(verses []*corpus.Verse) string {
	parts := make([]string, len(verses))
	for i, v := range verses {
		parts[i] = v.Text
	}
	return strings.Join(parts, " ")
}

// parallelVerseLines renders the same verse from every selected secondary
// translation, with a short label so readers can tell the columns apart.
func (m *model) parallelVerseLines(v *corpus.Verse, width int) []string {
	var lines []string
	for _, p := range m.parallel {
		ov := p.bible.VerseAt(v.BookID, v.Chapter, v.Number)
		if ov == nil {
			continue
		}
		tag := strings.ToUpper(p.id)
		prefix := parallelLabelStyle.Render("  " + tag + " │ ")
		bodyW := max(8, width-lipgloss.Width(prefix))
		for i, rg := range wrapRanges(ov.Text, bodyW) {
			if i == 0 {
				lines = append(lines, prefix+textStyle.Render(ov.Text[rg.Start:rg.End]))
			} else {
				lines = append(lines, strings.Repeat(" ", lipgloss.Width(prefix))+textStyle.Render(ov.Text[rg.Start:rg.End]))
			}
		}
	}
	return lines
}
