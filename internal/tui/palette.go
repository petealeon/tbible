package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) openPalette() {
	m.palette = true
	m.paletteSel = 0
	m.paletteInput.SetValue("")
	m.paletteInput.Width = max(10, min(60, m.width-14))
	m.paletteInput.Focus()
}

func (m *model) closePalette() {
	m.palette = false
	m.paletteInput.Blur()
}

// commandMatches filters the command list by the palette query. Matches on a
// command's name or id rank above matches found only in its description, so
// typing "note" puts Edit note first even though other descriptions mention
// notes.
func (m *model) commandMatches() []commandDef {
	q := strings.ToLower(strings.TrimSpace(m.paletteInput.Value()))
	if q == "" {
		return commandDefs
	}
	type scored struct {
		def   commandDef
		score int
		order int
	}
	var hits []scored
	for i, c := range commandDefs {
		name := strings.ToLower(c.name)
		id := strings.ToLower(c.id)
		desc := strings.ToLower(c.desc)
		score := -1
		switch {
		case name == q || id == q:
			score = 0
		case strings.HasPrefix(name, q) || strings.HasPrefix(id, q):
			score = 1
		case strings.Contains(name, q) || strings.Contains(id, q):
			score = 2
		case strings.Contains(desc, q):
			score = 3
		}
		if score >= 0 {
			hits = append(hits, scored{def: c, score: score, order: i})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score < hits[j].score
		}
		return hits[i].order < hits[j].order
	})
	out := make([]commandDef, len(hits))
	for i, h := range hits {
		out[i] = h.def
	}
	return out
}

func (m *model) handlePaletteKey(msg tea.KeyMsg) tea.Cmd {
	matches := m.commandMatches()
	switch msg.String() {
	case "esc":
		m.closePalette()
		return nil
	case "enter":
		if len(matches) == 0 {
			return nil
		}
		idx := min(max(0, m.paletteSel), len(matches)-1)
		id := matches[idx].id
		m.closePalette()
		return m.runCommand(id)
	case "up", "ctrl+p":
		if m.paletteSel > 0 {
			m.paletteSel--
		}
		return nil
	case "down", "ctrl+n":
		if m.paletteSel < len(matches)-1 {
			m.paletteSel++
		}
		return nil
	case "pgup":
		m.paletteSel = max(0, m.paletteSel-6)
		return nil
	case "pgdown":
		m.paletteSel = min(max(0, len(matches)-1), m.paletteSel+6)
		return nil
	}
	prev := m.paletteInput.Value()
	var cmd tea.Cmd
	m.paletteInput, cmd = m.paletteInput.Update(msg)
	if m.paletteInput.Value() != prev {
		m.paletteSel = 0
	}
	return cmd
}

func (m *model) paletteView() string {
	contentW := max(36, min(72, m.width-8))
	matches := m.commandMatches()

	var lines []string
	lines = append(lines,
		sectionStyle.Render("COMMANDS"),
		"",
		truncateANSI(m.paletteInput.View(), contentW),
		"",
	)
	if len(matches) == 0 {
		lines = append(lines, mutedStyle.Render("No matching commands."))
	}
	visible := min(12, max(1, m.height-12))
	start := 0
	if m.paletteSel >= visible {
		start = m.paletteSel - visible + 1
	}
	end := min(len(matches), start+visible)
	nameW := 24
	for i := start; i < end; i++ {
		c := matches[i]
		marker := "  "
		if i == m.paletteSel {
			marker = cursorMarker.Render("▌ ")
		}
		name := padTo(c.name, nameW)
		if i == m.paletteSel {
			name = selStyle.Render(name)
		} else {
			name = labelStyle.Render(name)
		}
		descW := max(6, contentW-nameW-3)
		desc := mutedStyle.Render(truncateANSI(c.desc, descW))
		lines = append(lines, marker+name+" "+desc)
	}
	if len(matches) > visible {
		lines = append(lines, "", mutedStyle.Render(fmt.Sprintf("%d commands · ↑↓ to scroll", len(matches))))
	}
	lines = append(lines, "", footerStyle.Render("Enter run · Esc close"))
	for i := range lines {
		lines[i] = truncateANSI(lines[i], contentW)
	}

	panel := panelStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(max(1, m.width), max(1, m.height), lipgloss.Center, lipgloss.Center, panel)
}
