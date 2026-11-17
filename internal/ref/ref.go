// Package ref parses human-friendly scripture references.
package ref

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/petealeon/tbible/v2/internal/corpus"
)

// Reference is a parsed scripture reference. Verse is zero for a whole
// chapter, EndVerse is zero for a single verse, and EndChapter extends a range
// across chapters. EndChapter is zero or equal to Chapter for ranges within
// one chapter.
type Reference struct {
	BookID     uint16
	Chapter    uint16
	Verse      uint16
	EndChapter uint16
	EndVerse   uint16
}

// WholeChapter reports whether the reference starts at a chapter boundary.
func (r Reference) WholeChapter() bool { return r.Verse == 0 }

// LastChapter returns the final chapter touched by the reference.
func (r Reference) LastChapter() uint16 {
	if r.EndChapter > r.Chapter {
		return r.EndChapter
	}
	return r.Chapter
}

// Label formats a reference for display, e.g. "John 3:16-18" or "John 3-5".
func (r Reference) Label(b *corpus.Bible) string {
	bk := b.BookByID(r.BookID)
	name := "?"
	if bk != nil {
		name = bk.Name
	}
	if r.Verse == 0 {
		if r.EndChapter > r.Chapter {
			return fmt.Sprintf("%s %d-%d", name, r.Chapter, r.EndChapter)
		}
		return fmt.Sprintf("%s %d", name, r.Chapter)
	}
	if r.EndChapter > r.Chapter {
		end := r.EndVerse
		if end == 0 {
			if c := b.ChapterAt(r.BookID, r.EndChapter); c != nil {
				end = c.VerseCount
			}
		}
		return fmt.Sprintf("%s %d:%d-%d:%d", name, r.Chapter, r.Verse, r.EndChapter, end)
	}
	if r.EndVerse > r.Verse {
		return fmt.Sprintf("%s %d:%d-%d", name, r.Chapter, r.Verse, r.EndVerse)
	}
	return fmt.Sprintf("%s %d:%d", name, r.Chapter, r.Verse)
}

// Verses returns every verse covered by the reference, in canon order. The
// result is nil when the reference does not resolve.
func (r Reference) Verses(b *corpus.Bible) []*corpus.Verse {
	if b == nil || r.BookID == 0 {
		return nil
	}
	book := b.BookByID(r.BookID)
	if book == nil {
		return nil
	}
	startChapter := r.Chapter
	if startChapter == 0 {
		startChapter = 1
	}
	if startChapter > book.ChapterCount {
		return nil
	}
	endChapter := r.LastChapter()
	if endChapter == 0 || endChapter > book.ChapterCount {
		endChapter = book.ChapterCount
	}
	if endChapter < startChapter {
		endChapter = startChapter
	}

	var out []*corpus.Verse
	for ch := startChapter; ch <= endChapter; ch++ {
		c := b.ChapterAt(r.BookID, ch)
		if c == nil {
			continue
		}
		first, last := uint16(1), c.VerseCount
		if ch == startChapter && r.Verse != 0 {
			first = r.Verse
		}
		if ch == endChapter && r.Verse != 0 {
			if r.EndVerse != 0 {
				last = r.EndVerse
			} else {
				last = r.Verse
			}
		}
		if first < 1 {
			first = 1
		}
		if last > c.VerseCount {
			last = c.VerseCount
		}
		for n := int(first); n <= int(last); n++ {
			if v := b.VerseAt(r.BookID, ch, uint16(n)); v != nil {
				out = append(out, v)
			}
		}
	}
	return out
}

// LabelForChapter formats the portion of r that lies in one chapter.
func (r Reference) LabelForChapter(b *corpus.Bible, chapter, first, last uint16, whole bool) string {
	seg := Reference{BookID: r.BookID, Chapter: chapter}
	if !whole {
		seg.Verse = first
		if last > first {
			seg.EndVerse = last
		}
	}
	return seg.Label(b)
}

// Key returns a stable storage key, e.g. "43:3:16".
func (r Reference) Key() string {
	if r.Verse == 0 {
		return fmt.Sprintf("%d:%d:0", r.BookID, r.Chapter)
	}
	return fmt.Sprintf("%d:%d:%d", r.BookID, r.Chapter, r.Verse)
}

type alias struct {
	text string
	id   uint16
}

var (
	rawAliases     []alias // may contain spaces
	compactAliases []alias // spaces and periods removed
)

func init() {
	seenRaw := map[string]uint16{}
	seenCompact := map[string]uint16{}
	add := func(s string, id uint16) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return
		}
		if _, ok := seenRaw[s]; !ok {
			seenRaw[s] = id
			rawAliases = append(rawAliases, alias{s, id})
		}
		c := compact(s)
		if c == "" {
			return
		}
		if _, ok := seenCompact[c]; !ok {
			seenCompact[c] = id
			compactAliases = append(compactAliases, alias{c, id})
		}
	}
	for _, b := range corpus.Canonical {
		add(b.Name, b.ID)
		add(b.Abbrev, b.ID)
		add(b.USFM, b.ID)
	}
	extras := map[string]string{
		"psalm": "Psalms", "psa": "Psalms", "ps": "Psalms", "pslm": "Psalms",
		"song of songs": "Song of Solomon", "canticles": "Song of Solomon", "song": "Song of Solomon",
		"revelations": "Revelation", "rev": "Revelation", "apocalypse": "Revelation",
		"eccl": "Ecclesiastes", "eccles": "Ecclesiastes", "qoheleth": "Ecclesiastes",
		"isa": "Isaiah", "jer": "Jeremiah", "lam": "Lamentations", "ezek": "Ezekiel",
		"dan": "Daniel", "hos": "Hosea", "obad": "Obadiah", "jon": "Jonah", "mic": "Micah",
		"nah": "Nahum", "hab": "Habakkuk", "zeph": "Zephaniah", "hag": "Haggai",
		"zech": "Zechariah", "mal": "Malachi",
		"matt": "Matthew", "mt": "Matthew", "mk": "Mark", "mrk": "Mark",
		"lk": "Luke", "luk": "Luke", "jn": "John", "jhn": "John",
		"rom": "Romans", "gal": "Galatians", "eph": "Ephesians", "php": "Philippians",
		"phil": "Philippians", "col": "Colossians", "tit": "Titus", "phlm": "Philemon",
		"phm": "Philemon", "philem": "Philemon", "heb": "Hebrews", "jas": "James",
		"1jn": "1 John", "2jn": "2 John", "3jn": "3 John",
		"1pet": "1 Peter", "2pet": "2 Peter", "1pe": "1 Peter", "2pe": "2 Peter",
		"1thess": "1 Thessalonians", "2thess": "2 Thessalonians",
		"1cor": "1 Corinthians", "2cor": "2 Corinthians",
		"1tim": "1 Timothy", "2tim": "2 Timothy",
		"1kgs": "1 Kings", "2kgs": "2 Kings", "1ki": "1 Kings", "2ki": "2 Kings",
		"1sam": "1 Samuel", "2sam": "2 Samuel", "1sa": "1 Samuel", "2sa": "2 Samuel",
		"1chr": "1 Chronicles", "2chr": "2 Chronicles",
	}
	for s, name := range extras {
		if id := idForName(name); id != 0 {
			add(s, id)
		}
	}
	// Ordinal words and Roman numerals for numbered books.
	for _, b := range corpus.Canonical {
		var n int
		var rest string
		if len(b.Name) > 2 && b.Name[1] == ' ' && b.Name[0] >= '1' && b.Name[0] <= '3' {
			n = int(b.Name[0] - '0')
			rest = b.Name[2:]
		} else {
			continue
		}
		roman := []string{"", "i", "ii", "iii"}[n]
		word := []string{"", "first", "second", "third"}[n]
		add(roman+" "+strings.ToLower(rest), b.ID)
		add(word+" "+strings.ToLower(rest), b.ID)
		add(strconv.Itoa(n)+strings.ToLower(rest), b.ID)
		add(roman+strings.ToLower(rest), b.ID)
		add(word+strings.ToLower(rest), b.ID)
	}
	// Shorter aliases never shadow longer ones.
	sort.SliceStable(rawAliases, func(i, j int) bool { return len(rawAliases[i].text) > len(rawAliases[j].text) })
	sort.SliceStable(compactAliases, func(i, j int) bool { return len(compactAliases[i].text) > len(compactAliases[j].text) })
}

func idForName(name string) uint16 {
	for _, b := range corpus.Canonical {
		if b.Name == name {
			return b.ID
		}
	}
	return 0
}

func compact(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '\t', '\n', '.', '\'':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Parse parses one or more references. currentBook is used for relative
// references such as "3:16".
func Parse(b *corpus.Bible, input string, currentBook uint16) ([]Reference, error) {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return nil, fmt.Errorf("empty reference")
	}
	input = strings.NewReplacer("–", "-", "—", "-", "\u00a0", " ").Replace(input)
	parts := strings.Split(input, ";")
	out := make([]Reference, 0, len(parts))
	ctx := currentBook
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		refs, book, err := parseOne(b, part, ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, refs...)
		if book != 0 {
			ctx = book
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no references")
	}
	return out, nil
}

func parseOne(b *corpus.Bible, s string, ctx uint16) ([]Reference, uint16, error) {
	// Try a literal alias prefix first, which preserves spaces between
	// chapter and verse ("john 3 16").
	if id, rest, ok := matchRaw(s); ok {
		refs, err := parseTail(b, id, rest)
		return refs, id, err
	}
	// Compact forms such as jn3.16 and 1cor13:4.
	if id, rest, ok := matchCompact(s); ok {
		refs, err := parseTail(b, id, rest)
		return refs, id, err
	}
	// Relative reference.
	if ctx != 0 {
		if refs, err := parseTail(b, ctx, s); err == nil {
			return refs, ctx, nil
		}
	}
	return nil, 0, fmt.Errorf("unknown book in %q", s)
}

func matchRaw(s string) (uint16, string, bool) {
	for _, a := range rawAliases {
		if !strings.HasPrefix(s, a.text) {
			continue
		}
		if len(s) > len(a.text) {
			c := s[len(a.text)]
			if c != ' ' && c != ':' && c != '.' && (c < '0' || c > '9') {
				continue
			}
		}
		rest := strings.TrimLeft(s[len(a.text):], " .:,;")
		return a.id, rest, true
	}
	return 0, "", false
}

func matchCompact(s string) (uint16, string, bool) {
	c := compact(s)
	for _, a := range compactAliases {
		if !strings.HasPrefix(c, a.text) {
			continue
		}
		rest := c[len(a.text):]
		if rest == "" {
			return a.id, "", true
		}
		if rest[0] < '0' || rest[0] > '9' {
			continue
		}
		return a.id, rest, true
	}
	return 0, "", false
}

func parseTail(b *corpus.Bible, id uint16, tail string) ([]Reference, error) {
	book := b.BookByID(id)
	if book == nil {
		return nil, fmt.Errorf("unknown book")
	}
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return []Reference{{BookID: id, Chapter: 1}}, nil
	}
	// In single-chapter books a bare number is a verse: "Jude 3" means
	// Jude 1:3, not a third chapter.
	if book.ChapterCount == 1 && !strings.ContainsAny(tail, ":-") {
		if f := strings.Fields(tail); len(f) == 1 {
			if n, err := strconv.Atoi(f[0]); err == nil && n >= 1 {
				if c := b.ChapterAt(id, 1); c != nil && n <= int(c.VerseCount) {
					return []Reference{{BookID: id, Chapter: 1, Verse: uint16(n)}}, nil
				}
			}
		}
	}

	// "3 16", "3:16", "3.16", "3:16-18", and "3-5".
	tail = strings.ReplaceAll(tail, ".", ":")
	var ch, v1, endCh, v2 uint16
	if strings.ContainsAny(tail, " :-") && !strings.Contains(tail, "-") {
		// Whitespace-separated chapter and verse.
		f := strings.Fields(tail)
		if len(f) == 2 {
			a, e1 := strconv.Atoi(f[0])
			c, e2 := strconv.Atoi(f[1])
			if e1 == nil && e2 == nil {
				ch, v1 = uint16(a), uint16(c)
			}
		}
	}
	if ch == 0 {
		var err error
		ch, v1, endCh, v2, err = parseNumeric(tail)
		if err != nil {
			// Compact remainder like "316" (john316): split into chapter
			// and verse using the book's real limits.
			if ref, ok := splitDigits(b, id, tail); ok {
				return []Reference{ref}, nil
			}
			return nil, err
		}
	}
	if ch == 0 {
		return nil, fmt.Errorf("invalid reference %q", tail)
	}
	if ch > book.ChapterCount {
		return nil, fmt.Errorf("%s has %d chapters", book.Name, book.ChapterCount)
	}
	if v1 == 0 {
		if endCh > book.ChapterCount {
			endCh = book.ChapterCount
		}
		if endCh != 0 && endCh < ch {
			return nil, fmt.Errorf("invalid chapter range in %q", tail)
		}
		return []Reference{{BookID: id, Chapter: ch, EndChapter: endCh}}, nil
	}
	vc := b.ChapterAt(id, ch)
	if vc == nil {
		return nil, fmt.Errorf("%s %d not found", book.Name, ch)
	}
	if v1 > vc.VerseCount {
		return nil, fmt.Errorf("%s %d has %d verses", book.Name, ch, vc.VerseCount)
	}
	if endCh == 0 {
		endCh = ch
	}
	if endCh > book.ChapterCount {
		return nil, fmt.Errorf("%s has %d chapters", book.Name, book.ChapterCount)
	}
	if endCh < ch {
		return nil, fmt.Errorf("invalid verse range in %q", tail)
	}
	endVC := vc
	if endCh != ch {
		endVC = b.ChapterAt(id, endCh)
		if endVC == nil {
			return nil, fmt.Errorf("%s %d not found", book.Name, endCh)
		}
	}
	if v2 == 0 {
		if endCh == ch {
			endCh = 0
		} else {
			v2 = endVC.VerseCount
		}
	}
	if v2 > endVC.VerseCount {
		v2 = endVC.VerseCount
	}
	if endCh == ch && v2 != 0 && v2 < v1 {
		v1, v2 = v2, v1
	}
	return []Reference{{BookID: id, Chapter: ch, Verse: v1, EndChapter: endCh, EndVerse: v2}}, nil
}

func parseNumeric(s string) (ch, v1, endCh, v2 uint16, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, 0, 0, fmt.Errorf("empty reference")
	}
	main, tail, hasRange := strings.Cut(s, "-")
	ch, v1, err = parsePoint(main)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if !hasRange {
		return ch, v1, 0, 0, nil
	}
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return 0, 0, 0, 0, fmt.Errorf("invalid range %q", s)
	}
	if strings.ContainsAny(tail, ": ") {
		ec, ev, e := parsePoint(tail)
		if e != nil {
			return 0, 0, 0, 0, e
		}
		if v1 == 0 {
			return 0, 0, 0, 0, fmt.Errorf("invalid range %q", s)
		}
		return ch, v1, ec, ev, nil
	}
	n, e := strconv.Atoi(tail)
	if e != nil || n < 1 || n > 200 {
		return 0, 0, 0, 0, fmt.Errorf("invalid range %q", s)
	}
	if v1 == 0 {
		if n > 150 {
			return 0, 0, 0, 0, fmt.Errorf("invalid chapter range %q", s)
		}
		endCh = uint16(n)
	} else {
		v2 = uint16(n)
	}
	return ch, v1, endCh, v2, nil
}

func parsePoint(s string) (ch, v uint16, err error) {
	s = strings.TrimSpace(s)
	var verseStr string
	if i := strings.IndexAny(s, ": "); i >= 0 {
		verseStr = strings.TrimSpace(s[i+1:])
		s = strings.TrimSpace(s[:i])
	}
	n, e := strconv.Atoi(s)
	if e != nil || n < 1 || n > 150 {
		return 0, 0, fmt.Errorf("invalid chapter %q", s)
	}
	ch = uint16(n)
	if verseStr != "" {
		m, e := strconv.Atoi(verseStr)
		if e != nil || m < 1 || m > 200 {
			return 0, 0, fmt.Errorf("invalid verse %q", verseStr)
		}
		v = uint16(m)
	}
	return ch, v, nil
}

// splitDigits handles compact references without a separator, e.g. john316.
func splitDigits(b *corpus.Bible, id uint16, s string) (Reference, bool) {
	book := b.BookByID(id)
	if book == nil {
		return Reference{}, false
	}
	for i := len(s) - 1; i >= 1; i-- {
		ch, e1 := strconv.Atoi(s[:i])
		v, e2 := strconv.Atoi(s[i:])
		if e1 != nil || e2 != nil || ch < 1 || ch > int(book.ChapterCount) || v < 1 {
			continue
		}
		c := b.ChapterAt(id, uint16(ch))
		if c != nil && v <= int(c.VerseCount) {
			return Reference{BookID: id, Chapter: uint16(ch), Verse: uint16(v)}, true
		}
	}
	return Reference{}, false
}

// FindBooks returns books that match a name or abbreviation prefix.
func FindBooks(query string, limit int) []*corpus.BookInfo {
	if limit <= 0 {
		limit = 10
	}
	q := compact(query)
	if q == "" {
		return nil
	}
	type scored struct {
		info  *corpus.BookInfo
		score int
	}
	var hits []scored
	for i := range corpus.Canonical {
		b := &corpus.Canonical[i]
		best := 0
		for _, name := range []string{b.Name, b.Abbrev, b.USFM} {
			c := compact(name)
			switch {
			case c == q:
				best = 3
			case strings.HasPrefix(c, q):
				best = 2
			case strings.Contains(c, q):
				best = 1
			}
			if best == 3 {
				break
			}
		}
		if best > 0 {
			hits = append(hits, scored{b, best})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].info.ID < hits[j].info.ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]*corpus.BookInfo, len(hits))
	for i := range hits {
		out[i] = hits[i].info
	}
	return out
}

// ParseKey reads a storage key produced by Reference.Key.
func ParseKey(key string) (Reference, bool) {
	parts := strings.Split(key, ":")
	if len(parts) != 3 {
		return Reference{}, false
	}
	b, e1 := strconv.Atoi(parts[0])
	c, e2 := strconv.Atoi(parts[1])
	v, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil || b < 1 || b > 66 || c < 1 || v < 0 {
		return Reference{}, false
	}
	return Reference{BookID: uint16(b), Chapter: uint16(c), Verse: uint16(v)}, true
}
