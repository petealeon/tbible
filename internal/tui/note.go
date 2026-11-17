package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) noteView() string {
	contentW := max(30, min(72, m.width-8))
	m.noteInput.SetWidth(contentW - 2)

	title := sectionStyle.Render("NOTE") + mutedStyle.Render("  "+m.noteRef)
	body := m.noteInput.View()
	lines := []string{
		title,
		"",
	}
	for _, line := range strings.Split(body, "\n") {
		lines = append(lines, truncateANSI(line, contentW))
	}
	lines = append(lines, "", footerStyle.Render("Ctrl-S save · Esc cancel · empty note deletes"))
	for i := range lines {
		lines[i] = truncateANSI(lines[i], contentW)
	}

	panel := panelStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(max(1, m.width), max(1, m.height), lipgloss.Center, lipgloss.Center, panel)
}
