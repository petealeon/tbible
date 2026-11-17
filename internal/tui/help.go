package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type helpEntry struct {
	keys string
	desc string
}

type helpGroup struct {
	title   string
	entries []helpEntry
}

var helpGroups = []helpGroup{
	{
		title: "Global",
		entries: []helpEntry{
			{"Ctrl-C", "Quit tbible"},
			{"?", "Open or close this help"},
			{"Ctrl-P", "Command palette (or : outside the omnibox)"},
			{"/", "Jump to the search omnibox"},
			{"L", "Library: bookmarks, notes, history, saved searches, calendar"},
			{"Esc", "Clear the query, leave the reader, or go home"},
		},
	},
	{
		title: "Search",
		entries: []helpEntry{
			{"type", "One omnibox handles references, words, and phrases"},
			{"Enter", "Open the selected reference or verse"},
			{"↑ ↓", "Move through results (works from the omnibox too)"},
			{"Tab / Shift-Tab", "Jump to the next or previous result section"},
			{"f", "Cycle scope: all, Old Testament, New Testament, current book"},
			{"H", "Open the full-screen heatmap (inline map follows the selected result in wide view)"},
			{"S or Ctrl-S", "Save the current query to the library"},
			{"y", "Copy the selected verse or range"},
			{"b", "Bookmark the selected verse or range"},
			{"n", "Add or edit a note on the selected verse"},
		},
	},
	{
		title: "Reader",
		entries: []helpEntry{
			{"j / k", "Move the verse cursor down or up"},
			{"g / G", "First or last verse in the chapter"},
			{"[ / ]", "Previous or next chapter"},
			{"{ / }", "Previous or next book"},
			{"Space / PgDn / PgUp", "Page through the chapter"},
			{"y", "Copy the current verse"},
			{"b", "Bookmark the current verse"},
			{"n", "Add or edit a note on the current verse"},
			{"i", "Toggle your notes and KJV marginal footnotes"},
			{"v", "Toggle verse numbers"},
			{"Ctrl-B", "Toggle the chapter sidebar"},
		},
	},
	{
		title: "Library",
		entries: []helpEntry{
			{"↑ ↓", "Move through saved items"},
			{"Enter", "Open the selected verse or run a saved search"},
			{"Tab / Shift-Tab", "Filter: all, bookmarks, notes, history, saved searches, calendar"},
			{"x", "Delete the selected saved search, bookmark, or note"},
			{"y", "Copy the selected verse"},
			{"b", "Bookmark the selected verse"},
			{"n", "Add or edit a note on the selected verse"},
		},
	},
	{
		title: "Calendar",
		entries: []helpEntry{
			{"L then Tab", "Open Calendar from the Library tab cycle"},
			{"h / l or ← →", "Previous or next day"},
			{"j / k or ↑ ↓", "Previous/next week, or entries when focused"},
			{"[ / ]", "Previous or next month"},
			{"1 - 4 or s", "All, Notes, Bookmarks, History"},
			{"Tab", "Switch between calendar and entries"},
			{"f", "Filter entries by text or reference"},
			{"Enter", "Open the selected entry"},
			{"Esc", "Clear filter, then return to the previous Library tab"},
		},
	},
	{
		title: "Heatmap",
		entries: []helpEntry{
			{"wide", "Inline overview under the preview follows the selected result"},
			{"H", "Open a heatmap for the current search results"},
			{"j / k", "Move between books, or chapters in detail view"},
			{"Enter", "Drill into a book, then open its strongest verse"},
			{"d", "Toggle density and raw count"},
			{"← / Esc", "Go back to books, then close the heatmap"},
		},
	},
	{
		title: "Query syntax",
		entries: []helpEntry{
			{"shepherd", "All words must appear (BM25-ranked)"},
			{"shep*", "Prefix match"},
			{`"still waters"`, "Exact phrase in double quotes"},
			{"love -world", "Exclude verses containing a word"},
			{"John 3:16", "References: full names and common abbreviations"},
			{"jn3.16", "Compact references work too"},
			{"1 Cor 13:4-7", "Chapter and verse ranges"},
			{"john 3:16; rom 5:8", "Several references separated by semicolons"},
			{"3:16", "Relative reference in the current book"},
		},
	},
	{
		title: "About",
		entries: []helpEntry{
			{"Text", "King James Version, 1769 text, from eBible.org eng-kjv2006"},
			{"Licence", "Public domain outside the United Kingdom"},
			{"Crown", "The KJV is Crown copyright in the UK; see the README notice"},
		},
	},
}

func (m *model) helpLines() []string {
	width := max(30, min(96, m.width-4))
	var lines []string
	for gi, g := range helpGroups {
		if gi > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, sectionStyle.Render(strings.ToUpper(g.title)))
		for _, e := range g.entries {
			keyW := min(22, max(12, width/4))
			key := padTo(labelStyle.Render(e.keys), keyW)
			descW := max(10, width-keyW-2)
			ranges := wrapRanges(e.desc, descW)
			for i, rg := range ranges {
				if i == 0 {
					lines = append(lines, "  "+key+"  "+textStyle.Render(e.desc[rg.Start:rg.End]))
				} else {
					lines = append(lines, strings.Repeat(" ", keyW+4)+textStyle.Render(e.desc[rg.Start:rg.End]))
				}
			}
		}
	}
	return lines
}

func (m *model) helpView() string {
	header := m.headerLine(appTitle.Render("Help"), mutedStyle.Render("Esc to close"))
	avail := max(1, m.height-4)
	lines := m.helpLines()
	maxScroll := max(0, len(lines)-avail)
	if m.helpScroll < 0 {
		m.helpScroll = 0
	}
	if m.helpScroll > maxScroll {
		m.helpScroll = maxScroll
	}
	start := m.helpScroll
	end := min(len(lines), start+avail)
	block := fitLines(lines[start:end], m.width, avail)
	hints := "↑↓ scroll · g/G top/bottom · Esc close"
	return header + "\n\n" + block + "\n" + m.footer(hints)
}

func (m *model) handleHelpKey(msg tea.KeyMsg) tea.Cmd {
	page := max(1, m.height-8)
	switch msg.String() {
	case "esc", "?", "q":
		m.help = false
		return nil
	case "up", "k":
		m.helpScroll--
		return nil
	case "down", "j":
		m.helpScroll++
		return nil
	case "pgup", "ctrl+u":
		m.helpScroll -= page
		return nil
	case "pgdown", "ctrl+d", " ":
		m.helpScroll += page
		return nil
	case "g", "home":
		m.helpScroll = 0
		return nil
	case "G", "end":
		m.helpScroll = max(0, len(m.helpLines())-max(1, m.height-4))
		return nil
	case ":", "ctrl+p":
		m.help = false
		m.openPalette()
		return nil
	}
	return nil
}
