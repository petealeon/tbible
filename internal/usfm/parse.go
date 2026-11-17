// Package usfm parses the eBible.org KJV USFM source into the corpus model.
//
// The eBible edition is one verse per line and uses a small marker set:
//
//	\w word|strong="H1234"\w*   original-language word
//	\add supplied\add*          italic supplied word
//	\wj words\wj*               words of Christ
//	\nd LORD\nd*                divine name
//	\f + \fr 1.1 \ft note\f*    marginal note
//	\s1 heading                 section heading
//	\d psalm title              psalm superscription
//
// Unknown markers are removed while their text content is kept; unknown
// note-style markers are dropped along with their content.
package usfm

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/petealeon/tbible/v2/internal/corpus"
)

// ParseZip reads a USFM source zip and returns the assembled Bible.
func ParseZip(filename string) (*corpus.Bible, error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	books := make(map[uint16]*parsedBook, 66)
	for _, f := range zr.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".usfm") {
			continue
		}
		pb, err := parseBookFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path.Base(f.Name), err)
		}
		if _, dup := books[pb.Info.ID]; dup {
			return nil, fmt.Errorf("%s: duplicate book %s", path.Base(f.Name), pb.Info.Name)
		}
		books[pb.Info.ID] = pb
	}
	if len(books) != len(corpus.Canonical) {
		return nil, fmt.Errorf("expected %d books, found %d", len(corpus.Canonical), len(books))
	}

	b := &corpus.Bible{
		Books:    make([]corpus.Book, 0, len(corpus.Canonical)),
		Chapters: make([]corpus.Chapter, 0, corpus.TotalChapters),
		Verses:   make([]corpus.Verse, 0, corpus.TotalVerses),
	}
	for i := range corpus.Canonical {
		info := corpus.Canonical[i]
		pb := books[info.ID]
		if pb == nil {
			return nil, fmt.Errorf("missing book %s", info.Name)
		}
		if got, want := len(pb.Chapters), int(info.Chapters); got != want {
			return nil, fmt.Errorf("%s: %d chapters, want %d", info.Name, got, want)
		}
		if got, want := len(pb.Verses), int(info.Verses); got != want {
			return nil, fmt.Errorf("%s: %d verses, want %d", info.Name, got, want)
		}
		b.Books = append(b.Books, corpus.Book{
			ID:           info.ID,
			Name:         info.Name,
			Abbrev:       info.Abbrev,
			Testament:    info.Testament,
			ChapterCount: info.Chapters,
			FirstChapter: uint16(len(b.Chapters)),
		})
		for _, ch := range pb.Chapters {
			ch.FirstVerse = uint32(len(b.Verses)) + ch.FirstVerse
			b.Chapters = append(b.Chapters, ch)
		}
		firstVerse := len(b.Verses)
		for _, v := range pb.Verses {
			v.BookID = info.ID
			b.Verses = append(b.Verses, v)
		}
		for i, d := range pb.Decorations {
			b.SetVerseDecorations(firstVerse+i, d)
		}
	}
	b.Prepare()
	return b, nil
}

type parsedBook struct {
	Info        corpus.BookInfo
	Chapters    []corpus.Chapter
	Verses      []corpus.Verse
	Decorations []corpus.VerseDecorations
}

type bookParser struct {
	info            corpus.BookInfo
	chapter         uint16
	pendingFlags    uint8
	pendingHeadings []corpus.Heading
	verses          []corpus.Verse
	decorations     []corpus.VerseDecorations
}

func parseBookFile(f *zip.File) (*parsedBook, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return parseBook(rc)
}

func parseBook(r io.Reader) (*parsedBook, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var bp *bookParser
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimRight(sc.Text(), " \t\r")
		if line == "" {
			continue
		}
		if line[0] != '\\' {
			// The eBible text is one verse per line; anything else is
			// ignorable front/back matter.
			continue
		}
		marker, rest := splitMarker(line)
		switch marker {
		case "id":
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				return nil, fmt.Errorf("line %d: empty \\id", lineNo)
			}
			info := corpus.BookByUSFM(fields[0])
			if info == nil {
				return nil, fmt.Errorf("line %d: unknown book code %q", lineNo, fields[0])
			}
			bp = &bookParser{info: *info}
		case "c":
			if bp == nil {
				return nil, fmt.Errorf("line %d: \\c before \\id", lineNo)
			}
			n, err := strconv.Atoi(strings.TrimSpace(rest))
			if err != nil || n < 1 || n > 150 {
				return nil, fmt.Errorf("line %d: bad chapter %q", lineNo, rest)
			}
			bp.chapter = uint16(n)
		case "v":
			if bp == nil {
				return nil, fmt.Errorf("line %d: \\v before \\id", lineNo)
			}
			if bp.chapter == 0 {
				return nil, fmt.Errorf("line %d: \\v before \\c", lineNo)
			}
			num, text, err := splitVerse(rest)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo, err)
			}
			ip := newInlineParser()
			ip.parseSegment(text, false, false)
			text, italics, red, notes := ip.result()
			if ip.paragraph {
				bp.pendingFlags |= corpus.FlagParagraph
			}
			bp.verses = append(bp.verses, corpus.Verse{
				Chapter: bp.chapter,
				Number:  uint16(num),
				Flags:   bp.pendingFlags,
				Text:    text,
			})
			bp.decorations = append(bp.decorations, corpus.VerseDecorations{
				Italics:   italics,
				RedLetter: red,
				Notes:     notes,
				Headings:  bp.pendingHeadings,
			})
			bp.pendingFlags = 0
			bp.pendingHeadings = nil
		case "p", "b", "m":
			if bp != nil {
				bp.pendingFlags |= corpus.FlagParagraph
			}
		case "q1", "q2", "q3":
			if bp != nil {
				bp.pendingFlags |= corpus.FlagPoetry
			}
		case "s1", "s2", "s3":
			if bp == nil {
				continue
			}
			h := strings.TrimSpace(plainInline(rest))
			if h != "" {
				bp.pendingHeadings = append(bp.pendingHeadings, corpus.Heading{Text: h, Kind: "section"})
			}
		case "d":
			if bp == nil {
				continue
			}
			h := strings.TrimSpace(plainInline(rest))
			if h != "" {
				bp.pendingHeadings = append(bp.pendingHeadings, corpus.Heading{Text: h, Kind: "title"})
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if bp == nil {
		return nil, fmt.Errorf("no \\id marker")
	}
	if len(bp.verses) == 0 {
		return nil, fmt.Errorf("no verses")
	}
	pb := &parsedBook{Info: bp.info, Decorations: bp.decorations}
	first := 0
	for i := 1; i < len(bp.verses); i++ {
		if bp.verses[i].Chapter != bp.verses[first].Chapter {
			ch := bp.verses[first].Chapter
			pb.Chapters = append(pb.Chapters, corpus.Chapter{
				BookID:     bp.info.ID,
				Number:     ch,
				FirstVerse: uint32(first),
				VerseCount: uint16(i - first),
			})
			first = i
		}
	}
	ch := bp.verses[first].Chapter
	pb.Chapters = append(pb.Chapters, corpus.Chapter{
		BookID:     bp.info.ID,
		Number:     ch,
		FirstVerse: uint32(first),
		VerseCount: uint16(len(bp.verses) - first),
	})
	for i, c := range pb.Chapters {
		if c.Number != uint16(i+1) {
			return nil, fmt.Errorf("chapter sequence breaks at %d (found %d)", i+1, c.Number)
		}
	}
	pb.Verses = bp.verses
	// Verse numbering must be contiguous within each chapter.
	for i := 0; i < len(pb.Chapters); i++ {
		c := pb.Chapters[i]
		vs := pb.Verses[c.FirstVerse : c.FirstVerse+uint32(c.VerseCount)]
		for j := range vs {
			if vs[j].Number != uint16(j+1) {
				return nil, fmt.Errorf("chapter %d: verse sequence breaks at %d (found %d)", c.Number, j+1, vs[j].Number)
			}
		}
	}
	return pb, nil
}

func splitMarker(line string) (string, string) {
	s := line[1:]
	i := 0
	for i < len(s) && (isASCIILetter(s[i]) || s[i] >= '0' && s[i] <= '9') {
		i++
	}
	return s[:i], strings.TrimLeft(s[i:], " \t")
}

func splitVerse(rest string) (int, string, error) {
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, "", fmt.Errorf("bad verse number %q", rest)
	}
	n, err := strconv.Atoi(rest[:i])
	if err != nil || n < 1 || n > 200 {
		return 0, "", fmt.Errorf("bad verse number %q", rest)
	}
	return n, strings.TrimLeft(rest[i:], " \t"), nil
}

func isASCIILetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// inlineParser emits normalised verse text and records style spans and notes.
type inlineParser struct {
	out          []byte
	italics      []corpus.Span
	red          []corpus.Span
	notes        []corpus.Note
	pendingSpace bool
	paragraph    bool
}

func newInlineParser() *inlineParser { return &inlineParser{} }

func (p *inlineParser) result() (string, []corpus.Span, []corpus.Span, []corpus.Note) {
	text := string(p.out)
	return text, trimSpans(p.italics, text), trimSpans(p.red, text), p.notes
}

func trimSpans(spans []corpus.Span, text string) []corpus.Span {
	out := spans[:0]
	for _, s := range spans {
		if s.Start < 0 {
			s.Start = 0
		}
		if s.End > len(text) {
			s.End = len(text)
		}
		for s.Start < s.End && text[s.Start] == ' ' {
			s.Start++
		}
		for s.End > s.Start && text[s.End-1] == ' ' {
			s.End--
		}
		if s.End > s.Start {
			out = append(out, s)
		}
	}
	return out
}

func (p *inlineParser) writeRune(r rune) {
	if r == '¶' {
		p.paragraph = true
		return
	}
	if unicode.IsSpace(r) {
		p.pendingSpace = true
		return
	}
	if p.pendingSpace && len(p.out) > 0 {
		p.out = append(p.out, ' ')
	}
	p.pendingSpace = false
	p.out = utf8.AppendRune(p.out, r)
}

func (p *inlineParser) parseSegment(s string, italic, red bool) {
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				i++
				continue
			}
			p.writeRune(r)
			i += size
			continue
		}

		marker, next, closing := readInlineMarker(s, i)
		if marker == "" {
			i++
			continue
		}
		i = next

		switch marker {
		case "w", "+w":
			content, after := markerContent(s, i, marker)
			if k := strings.IndexByte(content, '|'); k >= 0 {
				content = content[:k]
			}
			p.parseSegment(content, italic, red)
			i = after
		case "add", "+add":
			content, after := markerContent(s, i, marker)
			start := len(p.out)
			p.parseSegment(content, true, red)
			p.addSpan(&p.italics, start, len(p.out))
			i = after
		case "wj", "+wj":
			content, after := markerContent(s, i, marker)
			start := len(p.out)
			p.parseSegment(content, italic, true)
			p.addSpan(&p.red, start, len(p.out))
			i = after
		case "nd", "+nd", "tl", "+tl", "bk", "+bk", "em", "+em", "bd", "+bd", "it", "+it", "sc", "+sc", "no", "+no", "sup", "+sup":
			content, after := markerContent(s, i, marker)
			p.parseSegment(content, italic, red)
			i = after
		case "f", "+f", "x", "+x":
			content, after := markerContent(s, i, marker)
			if note := extractNote(content); note != "" {
				p.notes = append(p.notes, corpus.Note{Text: note, Anchor: len(p.out)})
			}
			i = after
		case "fig", "+fig", "cat", "+cat":
			_, after := markerContent(s, i, marker)
			i = after
		default:
			if closing {
				// Stray close marker; nothing to emit.
				continue
			}
			// Unknown inline marker: remove the marker, keep following text.
		}
	}
}

func (p *inlineParser) addSpan(dst *[]corpus.Span, start, end int) {
	if end <= start {
		return
	}
	*dst = append(*dst, corpus.Span{Start: start, End: end})
}

// readInlineMarker reads \name or \+name, optionally followed by *.
func readInlineMarker(s string, at int) (name string, next int, closing bool) {
	j := at + 1
	if j >= len(s) {
		return "", at + 1, false
	}
	plus := false
	if s[j] == '+' {
		plus = true
		j++
	}
	start := j
	for j < len(s) && (isASCIILetter(s[j]) || s[j] >= '0' && s[j] <= '9') {
		j++
	}
	if j == start {
		return "", at + 1, false
	}
	name = s[start:j]
	if plus {
		name = "+" + name
	}
	if j < len(s) && s[j] == '*' {
		return name, j + 1, true
	}
	// USFM opening markers are followed by exactly one separator space.
	if j < len(s) && s[j] == ' ' {
		j++
	}
	return name, j, false
}

// markerContent finds the matching close marker and returns its content and
// the offset just past the close.
func markerContent(s string, from int, marker string) (string, int) {
	delim := "\\" + marker + "*"
	if k := strings.Index(s[from:], delim); k >= 0 {
		return s[from : from+k], from + k + len(delim)
	}
	return s[from:], len(s)
}

func extractNote(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "+")
	content = strings.TrimSpace(content)
	if k := strings.Index(content, `\ft`); k >= 0 {
		content = content[k+3:]
	}
	content = stripMarkers(content)
	return strings.Join(strings.Fields(content), " ")
}

// stripMarkers removes USFM control words while preserving their text. It is
// used for notes and headings, where styling is not represented.
func stripMarkers(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			r, size := utf8.DecodeRuneInString(s[i:])
			out.WriteRune(r)
			i += size
			continue
		}
		j := i + 1
		if j < len(s) && s[j] == '+' {
			j++
		}
		for j < len(s) && (isASCIILetter(s[j]) || s[j] >= '0' && s[j] <= '9') {
			j++
		}
		if j < len(s) && s[j] == '*' {
			j++
		}
		if j < len(s) && s[j] == ' ' {
			j++
		}
		i = j
	}
	return out.String()
}

func plainInline(s string) string {
	p := newInlineParser()
	p.parseSegment(s, false, false)
	text, _, _, _ := p.result()
	return text
}
