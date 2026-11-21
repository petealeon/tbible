package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/embedded"
	"github.com/petealeon/tbible/v2/internal/ref"
)

// toggleStrongs shows or hides the compact Strong's numbers above tagged words.
func (m *model) toggleStrongs() tea.Cmd {
	m.showStrongs = !m.showStrongs
	m.reader.showStrongs = m.showStrongs
	if m.showStrongs {
		m.setStatus("Strong's numbers on · K opens the dictionary", false)
	} else {
		m.setStatus("Strong's numbers off", false)
	}
	return m.SaveSettings()
}

// openStrongs opens the full dictionary entry for the first tagged word in v.
func (m *model) openStrongs(r ref.Reference, v *corpus.Verse) tea.Cmd {
	if v == nil || len(v.Strongs()) == 0 {
		m.setStatus("No Strong's numbers in this verse", true)
		return nil
	}
	dict, err := embedded.Strongs()
	if err != nil {
		m.setStatus("Dictionary unavailable: "+err.Error(), true)
		return nil
	}
	m.strongsDict = dict
	m.strongsSpans = v.Strongs()
	m.strongsText = v.Text
	m.strongsVerse = r.Label(m.bible)
	m.strongsIndex = 0
	m.strongsScroll = 0
	m.strongsOpen = true
	return nil
}

func (m *model) closeStrongs() {
	m.strongsOpen = false
	m.strongsSpans = nil
}

func (m *model) strongsEntry() (string, corpus.StrongSpan, bool) {
	if !m.strongsOpen || len(m.strongsSpans) == 0 {
		return "", corpus.StrongSpan{}, false
	}
	m.strongsIndex = min(max(m.strongsIndex, 0), len(m.strongsSpans)-1)
	s := m.strongsSpans[m.strongsIndex]
	word := ""
	if s.Start >= 0 && s.End <= len(m.strongsText) {
		word = m.strongsText[s.Start:s.End]
	}
	if m.strongsDict == nil {
		return word, s, false
	}
	_, ok := m.strongsDict.Lookup(s.Greek, s.Number)
	return word, s, ok
}

func (m *model) handleStrongsKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "q":
		m.closeStrongs()
		return nil
	case "j", "down":
		if m.strongsIndex < len(m.strongsSpans)-1 {
			m.strongsIndex++
			m.strongsScroll = 0
		}
		return nil
	case "k", "up":
		if m.strongsIndex > 0 {
			m.strongsIndex--
			m.strongsScroll = 0
		}
		return nil
	case "ctrl+d", "pgdown":
		m.strongsScroll += 6
		return nil
	case "ctrl+u", "pgup":
		m.strongsScroll = max(0, m.strongsScroll-6)
		return nil
	case "g", "home":
		m.strongsIndex, m.strongsScroll = 0, 0
		return nil
	case "G", "end":
		m.strongsIndex = max(0, len(m.strongsSpans)-1)
		m.strongsScroll = 0
		return nil
	case "?", "ctrl+p":
		m.closeStrongs()
		if msg.String() == "?" {
			m.help, m.helpScroll = true, 0
		} else {
			m.openPalette()
		}
		return nil
	}
	return nil
}

func (m *model) strongsView() string {
	contentW := max(36, min(86, m.width-6))
	word, span, ok := m.strongsEntry()

	var lines []string
	if !ok || m.strongsDict == nil {
		lines = append(lines,
			mutedStyle.Render("STRONG'S DICTIONARY"),
			"",
			mutedStyle.Render("No dictionary entry is available."),
			"",
			footerStyle.Render("Esc close"),
		)
	} else {
		e, _ := m.strongsDict.Lookup(span.Greek, span.Number)
		title := sectionStyle.Render("STRONG'S") + mutedStyle.Render("  "+m.strongsVerse)
		count := mutedStyle.Render(fmt.Sprintf("  %d/%d", m.strongsIndex+1, len(m.strongsSpans)))
		lines = append(lines, title+count, "")
		head := labelStyle.Render(e.Tag())
		if e.Lemma != "" {
			head += "  " + titleStyle.Render(e.Lemma)
		}
		if e.Translit != "" {
			head += "  " + mutedStyle.Render("/"+e.Translit+"/")
		}
		lines = append(lines, truncateANSI(head, contentW))
		if word != "" {
			lines = append(lines, mutedStyle.Render("Verse word: "+word))
		}
		if e.Gloss != "" {
			lines = append(lines, "", strongTagStyle.Render(truncateANSI(e.Gloss, contentW)))
		}
		if e.Definition != "" {
			lines = append(lines, "")
			for _, line := range wrapTextLines(e.Definition, contentW) {
				lines = append(lines, textStyle.Render(line))
			}
		}
		lines = append(lines, "", footerStyle.Render("j/k next word · PgDn scroll · Esc close"))
	}

	maxBody := max(4, m.height-6)
	body := fitLineSlice(lines, contentW, maxBody)
	panel := panelStyle.Render(strings.Join(body, "\n"))
	return lipgloss.Place(max(1, m.width), max(1, m.height), lipgloss.Center, lipgloss.Center, panel)
}

// strongsSummary renders a short dictionary note for the first tagged word in
// v, used by the reader sidebar and the search preview.
func (m *model) strongsSummary(v *corpus.Verse, width int) []string {
	if v == nil || len(v.Strongs()) == 0 || m.strongsDict == nil {
		return nil
	}
	span := v.Strongs()[0]
	e, ok := m.strongsDict.Lookup(span.Greek, span.Number)
	if !ok {
		return nil
	}
	var lines []string
	lines = append(lines, sectionStyle.Render("STRONG'S"))
	head := strongTagStyle.Render(e.Tag())
	if e.Lemma != "" {
		head += "  " + labelStyle.Render(e.Lemma)
	}
	lines = append(lines, truncateANSI(head, width))
	if e.Gloss != "" {
		lines = append(lines, truncateANSI(e.Gloss, width))
	}
	if len(v.Strongs()) > 1 {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("%d tagged words · K for all", len(v.Strongs()))))
	}
	return lines
}

// shortStrongsSummary is a one-line variant for constrained panels.
func (m *model) shortStrongsSummary(v *corpus.Verse, width int) string {
	if v == nil || len(v.Strongs()) == 0 || m.strongsDict == nil {
		return ""
	}
	span := v.Strongs()[0]
	e, ok := m.strongsDict.Lookup(span.Greek, span.Number)
	if !ok {
		return ""
	}
	s := strongTagStyle.Render(e.Tag())
	if e.Gloss != "" {
		s += "  " + mutedStyle.Render(truncateANSI(e.Gloss, max(1, width-12)))
	}
	return truncateANSI(s, width)
}

// ensureDictionary loads the dictionary when Strong's display is enabled.
func (m *model) ensureDictionary() {
	if !m.showStrongs || m.strongsDict != nil {
		return
	}
	dict, err := embedded.Strongs()
	if err != nil {
		return
	}
	m.strongsDict = dict
}
