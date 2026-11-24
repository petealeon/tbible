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

// toggleStrongs shows or hides inline Strong's numbers. When enabled it also
// focuses the first tagged word in the current verse.
func (m *model) toggleStrongs() tea.Cmd {
	m.showStrongs = !m.showStrongs
	m.reader.showStrongs = m.showStrongs
	if m.showStrongs {
		m.ensureDictionary()
		if spans, _, _, ok := m.focusedStrongs(); ok {
			m.setStatus(fmt.Sprintf("Strong's on · %d tagged words · w/W move · K dictionary", len(spans)), false)
		} else {
			m.setStatus("Strong's on · w/W move · K dictionary", false)
		}
	} else {
		m.setStatus("Strong's numbers off", false)
	}
	return m.SaveSettings()
}

func verseFocusKey(id string, v *corpus.Verse) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%s|%d:%d:%d", id, v.BookID, v.Chapter, v.Number)
}

// activeStrongsIndex returns the inline focus for v, resetting it when the
// focused verse or translation changes.
func (m *model) activeStrongsIndex(v *corpus.Verse) int {
	if v == nil {
		return -1
	}
	spans := v.Strongs()
	if len(spans) == 0 {
		return -1
	}
	key := verseFocusKey(m.translationID, v)
	if m.strongsFocusVerse != key {
		m.strongsFocusVerse = key
		m.strongsFocus = 0
	}
	m.strongsFocus = min(max(m.strongsFocus, 0), len(spans)-1)
	return m.strongsFocus
}

// focusedStrongs resolves the tagged words for the currently focused verse.
func (m *model) focusedStrongs() ([]corpus.StrongSpan, int, *corpus.Verse, bool) {
	_, v, ok := m.focusedVerse()
	if !ok || len(v.Strongs()) == 0 {
		return nil, -1, nil, false
	}
	return v.Strongs(), m.activeStrongsIndex(v), v, true
}

func strongWord(v *corpus.Verse, s corpus.StrongSpan) string {
	if v == nil || s.Start < 0 || s.End <= s.Start || s.End > len(v.Text) {
		return ""
	}
	return v.Text[s.Start:s.End]
}

// moveStrongsFocus cycles the highlighted tagged word without moving the verse
// cursor. j/k still move by verse, so word navigation gets its own keys.
func (m *model) moveStrongsFocus(delta int) tea.Cmd {
	if !m.showStrongs {
		m.setStatus("Strong's numbers are off · press s", false)
		return nil
	}
	spans, idx, v, ok := m.focusedStrongs()
	if !ok {
		m.setStatus("No Strong's numbers in this verse or translation", false)
		return nil
	}
	if len(spans) == 1 {
		m.setStatus("Only one tagged word in this verse", false)
		return nil
	}
	next := (idx + delta + len(spans)) % len(spans)
	m.strongsFocus = next
	m.strongsFocusVerse = verseFocusKey(m.translationID, v)
	m.setStatus(fmt.Sprintf("Strong's word %d/%d · K for dictionary", next+1, len(spans)), false)
	return nil
}

// openStrongs opens the full dictionary entry for the inline-focused word.
func (m *model) openStrongs(r ref.Reference, v *corpus.Verse) tea.Cmd {
	spans, idx, focused, ok := m.focusedStrongs()
	if ok && focused != nil {
		v = focused
	} else {
		if v == nil || len(v.Strongs()) == 0 {
			m.setStatus("No Strong's numbers in this verse", true)
			return nil
		}
		spans, idx = v.Strongs(), 0
	}
	dict, err := embedded.Strongs()
	if err != nil {
		m.setStatus("Dictionary unavailable: "+err.Error(), true)
		return nil
	}
	m.strongsDict = dict
	m.strongsSpans = spans
	m.strongsText = v.Text
	r = ref.Reference{BookID: v.BookID, Chapter: v.Chapter, Verse: v.Number}
	m.strongsVerse = r.Label(m.bible)
	m.strongsIndex = idx
	m.strongsScroll = 0
	m.strongsOpen = true
	// Keep the inline highlight in step with the overlay selection.
	m.strongsFocus = idx
	m.strongsFocusVerse = verseFocusKey(m.translationID, v)
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
	word := strongWord(&corpus.Verse{Text: m.strongsText}, s)
	if m.strongsDict == nil {
		return word, s, false
	}
	_, ok := m.strongsDict.Lookup(s.Greek, s.Number)
	return word, s, ok
}

func (m *model) setOverlayStrongsIndex(i int) {
	m.strongsIndex = i
	m.strongsScroll = 0
	m.strongsFocus = i
}

func (m *model) handleStrongsKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "q":
		m.closeStrongs()
		return nil
	case "j", "down":
		if m.strongsIndex < len(m.strongsSpans)-1 {
			m.setOverlayStrongsIndex(m.strongsIndex + 1)
		}
		return nil
	case "k", "up":
		if m.strongsIndex > 0 {
			m.setOverlayStrongsIndex(m.strongsIndex - 1)
		}
		return nil
	case "ctrl+d", "pgdown":
		m.strongsScroll += 6
		return nil
	case "ctrl+u", "pgup":
		m.strongsScroll = max(0, m.strongsScroll-6)
		return nil
	case "g", "home":
		m.setOverlayStrongsIndex(0)
		return nil
	case "G", "end":
		m.setOverlayStrongsIndex(max(0, len(m.strongsSpans)-1))
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
		head := strongTagStyle.Render(e.Tag())
		if e.Lemma != "" {
			head += "  " + titleStyle.Render(e.Lemma)
		}
		if e.Translit != "" {
			head += "  " + mutedStyle.Render("/"+e.Translit+"/")
		}
		lines = append(lines, truncateANSI(head, contentW))
		if word != "" {
			lines = append(lines, labelStyle.Render("Verse word: "+word))
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

// strongsSummary renders a short dictionary note for the focused tagged word
// in v, used by the reader sidebar and the search preview.
func (m *model) strongsSummary(v *corpus.Verse, width int) []string {
	if v == nil || len(v.Strongs()) == 0 || m.strongsDict == nil {
		return nil
	}
	spans := v.Strongs()
	idx := m.activeStrongsIndex(v)
	span := spans[idx]
	e, ok := m.strongsDict.Lookup(span.Greek, span.Number)
	if !ok {
		return nil
	}
	lines := []string{sectionStyle.Render("STRONG'S")}
	head := strongTagStyle.Render(e.Tag())
	if word := strongWord(v, span); word != "" {
		head = labelStyle.Render(word+" ") + head
	}
	if e.Lemma != "" {
		head += "  " + mutedStyle.Render(e.Lemma)
	}
	lines = append(lines, truncateANSI(head, width))
	if e.Gloss != "" {
		lines = append(lines, truncateANSI(e.Gloss, width))
	}
	lines = append(lines, mutedStyle.Render(fmt.Sprintf("%d/%d tagged · w/W move · K entry", idx+1, len(spans))))
	return lines
}

// shortStrongsSummary is a one-line variant for constrained panels.
func (m *model) shortStrongsSummary(v *corpus.Verse, width int) string {
	if v == nil || len(v.Strongs()) == 0 || m.strongsDict == nil {
		return ""
	}
	spans := v.Strongs()
	span := spans[m.activeStrongsIndex(v)]
	e, ok := m.strongsDict.Lookup(span.Greek, span.Number)
	if !ok {
		return ""
	}
	s := strongTagStyle.Render(e.Tag())
	if word := strongWord(v, span); word != "" {
		s = labelStyle.Render(word+" ") + s
	}
	if e.Gloss != "" {
		s += "  " + mutedStyle.Render(truncateANSI(e.Gloss, max(1, width-14)))
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
