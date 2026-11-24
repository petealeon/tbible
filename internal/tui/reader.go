package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/ref"
)

// readerModel holds the reading position and per-screen display options.
type readerModel struct {
	bookID  uint16
	chapter uint16
	verse   int
	scroll  int
	center  bool

	showNumbers bool
	showNotes   bool
	showStrongs bool
}

// readerLine is one rendered line of the chapter body. Verse is the verse the
// line belongs to, used to keep the cursor visible while scrolling.
type readerLine struct {
	text  string
	verse int
}

// openAt moves the reader to a reference.
func (r *readerModel) openAt(b *corpus.Bible, rr ref.Reference) {
	if b == nil || rr.BookID == 0 {
		return
	}
	r.bookID = rr.BookID
	r.chapter = rr.Chapter
	if r.chapter == 0 {
		r.chapter = 1
	}
	r.verse = int(rr.Verse)
	if r.verse == 0 {
		r.verse = 1
	}
	if c := b.ChapterAt(r.bookID, r.chapter); c != nil && r.verse > int(c.VerseCount) {
		r.verse = int(c.VerseCount)
	}
	r.scroll = 0
	r.center = true
}

func (m *model) readerBodyHeight() int {
	return max(1, m.height-5)
}

func (m *model) readerContentWidth() int {
	w := m.width
	if m.sidebar && m.width >= 100 {
		w -= 31
	}
	return max(20, w)
}

func (m *model) readerChapter() *corpus.Chapter {
	return m.bible.ChapterAt(m.reader.bookID, m.reader.chapter)
}

func (m *model) readerMaxVerse() int {
	if c := m.readerChapter(); c != nil {
		return int(c.VerseCount)
	}
	return 1
}

// readerLayout renders the chapter into styled lines and records the first
// line of every verse for cursor tracking.
func (m *model) readerLayout(width int) ([]readerLine, map[int]int) {
	width = max(10, width)
	verses := m.bible.ChapterVerses(m.reader.bookID, m.reader.chapter)
	notes := m.userNotes()
	selFirst, selLast, inSelection := m.selectionInChapter(m.reader.chapter)
	lines := make([]readerLine, 0, len(verses)*3)
	verseLine := make(map[int]int, len(verses))

	for i := range verses {
		v := &verses[i]
		vn := int(v.Number)
		verseLine[vn] = len(lines)

		if len(v.Headings()) > 0 {
			if len(lines) > 0 {
				lines = append(lines, readerLine{verse: vn})
			}
			for _, h := range v.Headings() {
				style := sectionHead
				if h.Kind == "title" {
					style = noteStyle
				}
				for _, rg := range wrapRanges(h.Text, width-2) {
					lines = append(lines, readerLine{text: "  " + style.Render(h.Text[rg.Start:rg.End]), verse: vn})
				}
			}
			lines = append(lines, readerLine{verse: vn})
		} else if i > 0 && v.Flags&corpus.FlagParagraph != 0 {
			lines = append(lines, readerLine{verse: vn})
		}

		selected := m.reader.verse == vn
		poetry := v.Flags&corpus.FlagPoetry != 0

		marker := "  "
		num := ""
		if inSelection && vn >= int(selFirst) && vn <= int(selLast) {
			marker = cursorMarker.Render("┃ ")
		}
		if selected {
			marker = cursorMarker.Render("▌ ")
		}
		if m.reader.showNumbers {
			if selected {
				num = cursorNum.Render(padLeft(fmt.Sprintf("%d", vn), 3)) + " "
			} else {
				num = verseNumStyle.Render(padLeft(fmt.Sprintf("%d", vn), 3)) + " "
			}
		} else if selected {
			num = cursorMarker.Render("› ")
		}
		indent := ""
		if poetry {
			indent = "    "
		}
		prefix := marker + indent + num
		prefixWidth := lipgloss.Width(prefix)
		textWidth := max(8, width-prefixWidth)
		pad := strings.Repeat(" ", prefixWidth)
		strongs := v.Strongs()
		active := -1
		if m.reader.showStrongs && vn == m.reader.verse {
			if _, idx, _, ok := m.focusedStrongs(); ok {
				active = idx
			}
		}
		ranges := wrapVerseRanges(v.Text, textWidth, strongs, m.reader.showStrongs)
		for j, rg := range ranges {
			text := renderVerseRange(v.Text, rg.Start, rg.End, v.Italics(), v.RedLetter(), strongs, active, m.reader.showStrongs)
			if j == 0 {
				lines = append(lines, readerLine{text: prefix + text, verse: vn})
			} else {
				lines = append(lines, readerLine{text: pad + text, verse: vn})
			}
		}
		for _, line := range m.parallelVerseLines(v, width-max(2, prefixWidth)) {
			lines = append(lines, readerLine{text: pad + line, verse: vn})
		}

		if m.reader.showNotes {
			noteWidth := max(8, width-5)
			if note, ok := notes[ref.Reference{BookID: v.BookID, Chapter: v.Chapter, Verse: v.Number}.Key()]; ok {
				body := strings.TrimSpace(note.Body)
				if body != "" {
					for j, text := range wrapTextLines(body, noteWidth) {
						prefix := "          "
						if j == 0 {
							prefix = "   ✎ Your note: "
						}
						lines = append(lines, readerLine{
							text:  userNoteStyle.Render(prefix + text),
							verse: vn,
						})
					}
				}
			}
			for _, n := range v.Notes() {
				for j, rg := range wrapRanges(n.Text, noteWidth) {
					prefix := "     "
					if j == 0 {
						prefix = "   ↳ "
					}
					lines = append(lines, readerLine{
						text:  mutedStyle.Render(prefix) + noteStyle.Render(n.Text[rg.Start:rg.End]),
						verse: vn,
					})
				}
			}
		}
	}
	if len(lines) == 0 {
		lines = append(lines, readerLine{text: mutedStyle.Render("This chapter has no verses."), verse: 1})
	}
	return lines, verseLine
}

// ensureReaderCursor scrolls the body so the cursor verse stays visible.
func (m *model) ensureReaderCursor(verseLine map[int]int, avail int) {
	line, ok := verseLine[m.reader.verse]
	if !ok {
		m.reader.scroll = 0
		return
	}
	avail = max(1, avail)
	if m.reader.center {
		m.reader.scroll = line - avail/2
		if m.reader.scroll < 0 {
			m.reader.scroll = 0
		}
		m.reader.center = false
		return
	}
	if line < m.reader.scroll {
		m.reader.scroll = line
	}
	if line >= m.reader.scroll+avail {
		m.reader.scroll = line - avail + 1
	}
	if m.reader.scroll < 0 {
		m.reader.scroll = 0
	}
	return
}

func (m *model) readerScrollClamp(total, avail int) {
	maxScroll := max(0, total-avail)
	if m.reader.scroll < 0 {
		m.reader.scroll = 0
	}
	if m.reader.scroll > maxScroll {
		m.reader.scroll = maxScroll
	}
}

func padLeft(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

func (m *model) handleReaderKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	// A completed selection is discarded as soon as the reader moves on, so a
	// stale range cannot silently capture a later copy or bookmark.
	if !m.selecting && m.selAnchor.BookID != 0 && isReaderMove(key) {
		m.clearSelection()
	}
	cmd := m.readerKey(msg)
	if m.selecting && isReaderMove(key) {
		m.extendSelection()
	}
	return cmd
}

// isReaderMove reports whether a key changes the reading position.
func isReaderMove(key string) bool {
	switch key {
	case "j", "k", "down", "up", "g", "G", "home", "end", " ", "pgdown", "pgup", "ctrl+d", "ctrl+u",
		"h", "l", "left", "right", "[", "]", "{", "}":
		return true
	}
	return false
}

func (m *model) readerKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	maxVerse := m.readerMaxVerse()
	contentW := m.readerContentWidth()
	avail := m.readerBodyHeight()

	switch key {
	case "j", "down":
		if m.reader.verse < maxVerse {
			m.reader.verse++
		}
		return nil
	case "k", "up":
		if m.reader.verse > 1 {
			m.reader.verse--
		}
		return nil
	case "g", "home":
		m.reader.verse = 1
		m.reader.scroll = 0
		return nil
	case "G", "end":
		m.reader.verse = maxVerse
		return nil
	case " ", "pgdown", "ctrl+d":
		return m.readerPage(contentW, avail, 1)
	case "pgup", "ctrl+u":
		return m.readerPage(contentW, avail, -1)
	case "h", "left", "[":
		return m.chapterStep(-1)
	case "l", "right", "]":
		return m.chapterStep(1)
	case "{":
		return m.bookStep(-1)
	case "}":
		return m.bookStep(1)
	case "y":
		return m.copyFocused()
	case "b":
		return m.toggleBookmark()
	case "n":
		return m.beginNote()
	case "V":
		switch {
		case m.selecting:
			m.endSelection()
		case m.selAnchor.BookID != 0:
			m.clearSelection()
			m.setStatus("Selection cleared", false)
		default:
			m.beginSelection()
		}
		return nil
	case "i":
		return m.runCommand("footnotes")
	case "s":
		return m.runCommand("strongs")
	case "K":
		return m.runCommand("strongs-entry")
	case "w":
		return m.moveStrongsFocus(1)
	case "W":
		return m.moveStrongsFocus(-1)
	case "v":
		return m.runCommand("numbers")
	case "ctrl+b":
		m.sidebar = !m.sidebar
		return nil
	case "/":
		return m.focusSearch()
	case "L":
		return m.runCommand("library")
	case "T":
		return m.runCommand("translations")
	case "?":
		m.help = true
		m.helpScroll = 0
		return nil
	case ":", "ctrl+p":
		m.openPalette()
		return nil
	case "esc":
		if m.selecting || m.selAnchor.BookID != 0 {
			m.clearSelection()
			m.setStatus("Selection cancelled", false)
			return nil
		}
		return m.runCommand("home")
	}
	return nil
}

// readerPage pages the body and moves the cursor to the first visible verse.
func (m *model) readerPage(width, avail, dir int) tea.Cmd {
	lines, verseLine := m.readerLayout(width)
	if dir > 0 {
		m.reader.scroll += max(1, avail-1)
	} else {
		m.reader.scroll -= max(1, avail-1)
	}
	m.readerScrollClamp(len(lines), avail)
	best := 0
	for v := 1; v <= m.readerMaxVerse(); v++ {
		ln, ok := verseLine[v]
		if !ok {
			continue
		}
		if ln >= m.reader.scroll {
			best = v
			break
		}
	}
	if best == 0 {
		best = m.readerMaxVerse()
	}
	m.reader.verse = best
	return nil
}

func (m *model) readerNav() string {
	bk := m.bible.BookByID(m.reader.bookID)
	name := "?"
	if bk != nil {
		name = bk.Name
	}
	left := titleStyle.Render(fmt.Sprintf("%s %d", name, m.reader.chapter))
	c := m.readerChapter()
	right := ""
	if c != nil {
		right = mutedStyle.Render(fmt.Sprintf("verse %d/%d", m.reader.verse, c.VerseCount))
	}
	return m.headerLine(left, right)
}

func (m *model) readerSidebar(width, height int) []string {
	bk := m.bible.BookByID(m.reader.bookID)
	if bk == nil {
		return []string{mutedStyle.Render("No chapter")}
	}
	c := m.readerChapter()
	if c == nil {
		return []string{mutedStyle.Render("No chapter")}
	}
	lines := []string{
		sectionStyle.Render("CHAPTER"),
		"",
		labelStyle.Render(bk.Name),
		textStyle.Render(fmt.Sprintf("Chapter %d of %d", m.reader.chapter, bk.ChapterCount)),
		mutedStyle.Render(fmt.Sprintf("%d verses", c.VerseCount)),
		"",
		mutedStyle.Render("Testament"),
		textStyle.Render(testamentName(bk.Testament)),
	}
	if bk.ID > 1 || m.reader.chapter > 1 {
		lines = append(lines, "", mutedStyle.Render("Previous"))
		if m.reader.chapter > 1 {
			lines = append(lines, textStyle.Render(fmt.Sprintf("%s %d", bk.Name, m.reader.chapter-1)))
		} else if prev := m.bible.BookByID(bk.ID - 1); prev != nil {
			lines = append(lines, textStyle.Render(fmt.Sprintf("%s %d", prev.Name, prev.ChapterCount)))
		}
	}
	if int(m.reader.chapter) < int(bk.ChapterCount) || bk.ID < 66 {
		lines = append(lines, "", mutedStyle.Render("Next"))
		if int(m.reader.chapter) < int(bk.ChapterCount) {
			lines = append(lines, textStyle.Render(fmt.Sprintf("%s %d", bk.Name, m.reader.chapter+1)))
		} else if next := m.bible.BookByID(bk.ID + 1); next != nil {
			lines = append(lines, textStyle.Render(fmt.Sprintf("%s 1", next.Name)))
		}
	}

	if m.showStrongs {
		if v := m.bible.VerseAt(m.reader.bookID, m.reader.chapter, uint16(max(1, m.reader.verse))); v != nil {
			lines = append(lines, "")
			lines = append(lines, m.strongsSummary(v, width)...)
		}
	}

	snap := m.store.Snapshot()
	var marks []string
	for _, it := range snap.Bookmarks {
		rr, ok := ref.ParseKey(it.Key)
		if !ok || rr.BookID != m.reader.bookID || rr.Chapter != m.reader.chapter {
			continue
		}
		label := it.Ref
		if label == "" {
			label = rr.Label(m.bible)
		}
		marks = append(marks, textStyle.Render("• "+label))
		if len(marks) >= max(1, (height-18)/1) {
			break
		}
	}
	if len(marks) > 0 {
		lines = append(lines, "", sectionStyle.Render("BOOKMARKS"), "")
		lines = append(lines, marks...)
	}
	return lines
}

func testamentName(t uint8) string {
	if t == 1 {
		return "New Testament"
	}
	return "Old Testament"
}

func (m *model) readerView() string {
	header := m.headerLine(m.headerLeft(), m.headerRight())
	nav := m.readerNav()
	avail := m.readerBodyHeight()
	contentW := m.readerContentWidth()

	lines, verseLine := m.readerLayout(contentW)
	m.ensureReaderCursor(verseLine, avail)
	m.readerScrollClamp(len(lines), avail)

	texts := make([]string, len(lines))
	for i := range lines {
		texts[i] = lines[i].text
	}
	start := min(m.reader.scroll, max(0, len(texts)-1))
	end := min(len(texts), start+avail)
	block := fitLines(texts[start:end], contentW, avail)

	if m.sidebar && m.width >= 100 {
		sideW := 30
		side := fitLines(m.readerSidebar(sideW-2, avail), sideW-2, avail)
		rl := strings.Split(block, "\n")
		sl := strings.Split(side, "\n")
		var joined []string
		rule := readerRule.Render("│")
		for i := range rl {
			joined = append(joined, padTo(rl[i], contentW)+" "+rule+" "+sl[i])
		}
		block = strings.Join(joined, "\n")
	}

	hints := actionHints(m.width) + " · j/k verse · V select · [/] chapter · i notes · / search · ? help"
	if m.selecting {
		hints = "PASSAGE SELECTED · j/k extend · [/] next chapter · y copy · V finish · Esc cancel"
	} else if m.width >= 150 {
		hints = "j/k verse · V select · [/] chapter · {/} book · Space page · " + actionHints(m.width) + " · i notes & footnotes · v numbers · / search · L library · ? help · Esc home"
	} else if m.width < 100 {
		hints = actionHints(m.width) + " · j/k verse · V select · i notes · ? help"
	}
	if m.showStrongs {
		switch {
		case m.selecting:
			hints = "PASSAGE SELECTED · j/k extend · w/W word · y copy · V finish · Esc cancel"
		case m.width >= 120:
			hints = "y copy · b bookmark · n note · w/W word · K entry · j/k verse · V select · [/] chapter · i notes · / search · ? help"
		case m.width >= 100:
			hints = "y copy · b bookmark · n note · w/W word · K entry · j/k verse · V select · [/] chapter · ? help"
		default:
			hints = "y copy · b mark · n note · w/W word · K entry · j/k verse · V select · ? help"
		}
	}
	return header + "\n" + nav + "\n\n" + block + "\n" + m.footer(hints)
}
