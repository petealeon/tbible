package tui

import (
	"fmt"
	"strings"
)

// noteView docks the note editor at the bottom of the current screen. The
// passage being annotated stays visible both above the editor and inside it,
// so editing a note never means losing the verses it belongs to.
func (m *model) noteView() string {
	contentW := max(36, min(78, m.width-4))
	textW := contentW - 4

	title := sectionStyle.Render("NOTE") + mutedStyle.Render("  "+m.noteRef)
	context := m.noteContextLines(textW)
	body := m.noteInput.View()

	lines := []string{title}
	lines = append(lines, context...)
	lines = append(lines, "")
	for _, line := range strings.Split(body, "\n") {
		lines = append(lines, truncateANSI(line, textW))
	}
	lines = append(lines, footerStyle.Render("Ctrl-S save · Esc cancel · empty note deletes"))

	panel := panelStyle.Render(strings.Join(lines, "\n"))
	panelLines := strings.Split(panel, "\n")
	panelH := len(panelLines)

	// Render the underlying screen with enough room left for the panel.
	baseH := max(6, m.height-panelH)
	saved := m.height
	m.height = baseH
	base := m.screenView()
	m.height = saved

	baseLines := strings.Split(base, "\n")
	baseLines = fitLineSlice(baseLines, m.width, baseH)

	out := append(baseLines, panelLines...)
	for len(out) < m.height {
		out = append(out, "")
	}
	if len(out) > m.height {
		out = out[:m.height]
	}
	return strings.Join(out, "\n")
}

// noteContextLines renders the verse or passage attached to the note.
func (m *model) noteContextLines(width int) []string {
	if width < 8 {
		width = 8
	}
	var label string
	var verses []string
	if r, list, ok := m.focusedRange(); ok {
		label = r.Label(m.bible)
		for _, v := range list {
			verses = append(verses, fmt.Sprintf("%d:%d  %s", v.Chapter, v.Number, v.Text))
		}
	} else if r, v, ok := m.focusedVerse(); ok {
		label = r.Label(m.bible)
		verses = append(verses, fmt.Sprintf("%d:%d  %s", r.Chapter, r.Verse, v.Text))
	} else if m.noteRef != "" {
		label = m.noteRef
	}
	var lines []string
	if label != "" {
		lines = append(lines, mutedStyle.Render(truncateANSI("on "+label, width)))
	}
	// Keep the editor usable on short terminals: at most four wrapped lines.
	cap := 4
	for _, text := range verses {
		wrapped := wrapPlainLines(text, width)
		if len(wrapped) > cap {
			wrapped = append(wrapped[:cap-1], "…")
		}
		for _, line := range wrapped {
			lines = append(lines, textStyle.Render(line))
		}
		cap -= len(wrapped)
		if cap <= 0 {
			break
		}
	}
	return lines
}

// wrapPlainLines wraps plain text without ANSI escapes.
func wrapPlainLines(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for _, line := range wrapTextLines(s, width) {
		out = append(out, line)
	}
	return out
}
