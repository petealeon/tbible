// Command tbible is a fast, keyboard-driven King James Bible for the terminal.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/embedded"
	"github.com/petealeon/tbible/v2/internal/ref"
	"github.com/petealeon/tbible/v2/internal/search"
	"github.com/petealeon/tbible/v2/internal/tui"
	"github.com/petealeon/tbible/v2/internal/version"
)

func main() {
	var (
		showVersion = flag.Bool("version", false, "print the version and exit")
		query       = flag.String("search", "", "search the KJV and print ranked verses")
		read        = flag.String("read", "", "print a reference, chapter, or verse range")
		asJSON      = flag.Bool("json", false, "emit JSON for --search or --read")
		plain       = flag.Bool("plain", false, "omit verse numbers, headings, and styling")
		scopeFlag   = flag.String("scope", "all", "search scope: all, ot, nt, or book")
		bookFlag    = flag.String("book", "", "restrict search to a book (implies --scope book)")
		limit       = flag.Int("limit", 25, "maximum search results")
	)
	flag.Usage = usage
	flag.Parse()

	if *showVersion {
		fmt.Printf("tbible %s\n", version.Version)
		return
	}

	switch {
	case *query != "":
		runSearchCommand(*query, *scopeFlag, *bookFlag, *limit, *asJSON, *plain)
	case *read != "":
		b, err := embedded.Bible()
		if err != nil {
			fail(err)
		}
		if err := runRead(b, *read, *asJSON, *plain); err != nil {
			fail(err)
		}
	case flag.NArg() > 0:
		runSearchCommand(strings.Join(flag.Args(), " "), *scopeFlag, *bookFlag, *limit, *asJSON, *plain)
	default:
		// The TUI is long-lived and keeps the embedded corpus and index eager.
		// A lower GC target cuts steady-state RSS by roughly 3-4 MB without
		// adding first-search latency; respect an explicit GOGC override.
		if _, ok := os.LookupEnv("GOGC"); !ok {
			debug.SetGCPercent(50)
		}
		if err := tui.New(); err != nil {
			fail(err)
		}
	}
}

func runSearchCommand(raw, scopeFlag, bookFlag string, limit int, asJSON, plain bool) {
	b, ix, err := embedded.Load()
	if err != nil {
		fail(err)
	}
	if err := runSearch(b, ix, raw, scopeFlag, bookFlag, limit, asJSON, plain); err != nil {
		fail(err)
	}
}

func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, `tbible %s — King James Bible in the terminal

Usage:
  tbible                      launch the interactive reader
  tbible --read REF           print a chapter, verse, or range
  tbible --search QUERY       rank verses for a word, phrase, or reference
  tbible QUERY                shorthand for --search

Options:
`, version.Version)
	flag.PrintDefaults()
	fmt.Fprint(out, `
Query syntax:
  shepherd            all words must appear
  shep*               prefix match
  "still waters"      exact phrase
  love -world         exclude a word
  John 3:16           references work in search too

Examples:
  tbible --read "John 3:16"
  tbible --read "Psalms 23" --plain
  tbible --search "faith" --scope nt --limit 10
  tbible --search "\"still waters\"" --json
`)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "tbible:", err)
	os.Exit(1)
}

type searchHit struct {
	Reference string  `json:"reference"`
	Book      string  `json:"book"`
	Chapter   int     `json:"chapter"`
	Verse     int     `json:"verse"`
	Text      string  `json:"text"`
	Score     float64 `json:"score"`
}

type searchOutput struct {
	Query   string      `json:"query"`
	Scope   string      `json:"scope"`
	Total   int         `json:"total"`
	Relaxed bool        `json:"relaxed,omitempty"`
	Hits    []searchHit `json:"hits"`
}

func runSearch(b *corpus.Bible, ix *search.Index, raw, scopeName, bookName string, limit int, asJSON, plain bool) error {
	scope, bookID, scopeLabel, err := parseScope(b, scopeName, bookName)
	if err != nil {
		return err
	}
	if limit <= 0 {
		limit = 25
	}

	// A query that parses as a reference is answered from the canon directly,
	// so "John 3:16" and "John 3:16; Romans 5:8" work from --search too.
	if hits, isRef := referenceHits(b, raw, scope, bookID); isRef {
		total := len(hits)
		if total > limit {
			hits = hits[:limit]
		}
		out := searchOutput{Query: raw, Scope: scopeLabel, Total: total, Hits: hits}
		if out.Hits == nil {
			out.Hits = []searchHit{}
		}
		if asJSON {
			return writeJSON(out)
		}
		printSearchHeader(raw, total, scopeLabel, false)
		if total == 0 {
			fmt.Println("No verses matched that reference in the selected scope.")
			return nil
		}
		for _, h := range out.Hits {
			fmt.Printf("%-24s %s\n", h.Reference, h.Text)
		}
		if total > len(out.Hits) {
			fmt.Printf("\n… %d more. Use --limit to show more.\n", total-len(out.Hits))
		}
		return nil
	}

	q := search.ParseQuery(raw)
	q.Scope = scope
	q.BookID = bookID
	res := ix.Search(q, func(i int) string { return b.Verses[i].Text }, search.Options{
		Scope:  scope,
		BookID: bookID,
		Limit:  limit,
	})

	out := searchOutput{Query: raw, Scope: scopeLabel, Total: res.Total, Relaxed: res.Relaxed, Hits: []searchHit{}}
	for _, h := range res.Hits {
		v := &b.Verses[h.Verse]
		bk := b.BookByID(v.BookID)
		name := "?"
		if bk != nil {
			name = bk.Name
		}
		out.Hits = append(out.Hits, searchHit{
			Reference: fmt.Sprintf("%s %d:%d", name, v.Chapter, v.Number),
			Book:      name,
			Chapter:   int(v.Chapter),
			Verse:     int(v.Number),
			Text:      v.Text,
			Score:     h.Score,
		})
	}

	if asJSON {
		return writeJSON(out)
	}

	printSearchHeader(raw, res.Total, scopeLabel, res.Relaxed)
	if res.Total == 0 {
		fmt.Println("No matches. Try fewer words, a prefix (shep*), or a quoted phrase.")
		return nil
	}
	for _, h := range out.Hits {
		text := h.Text
		if !plain {
			text = highlightPlain(text, q)
		}
		fmt.Printf("%-24s %s\n", h.Reference, text)
	}
	if res.Total > len(out.Hits) {
		fmt.Printf("\n… %d more. Use --limit to show more.\n", res.Total-len(out.Hits))
	}
	return nil
}

func printSearchHeader(raw string, total int, scopeLabel string, relaxed bool) {
	suffix := " · " + scopeLabel
	if relaxed {
		suffix += " · relaxed (not all terms matched)"
	}
	fmt.Printf("%s — %d verse%s%s\n\n", raw, total, plural(total), suffix)
}

// referenceHits reports whether raw is a reference and, if so, returns the
// verses it names. A parsed reference that the scope excludes is still a
// reference result: it yields no hits rather than falling through to text.
func referenceHits(b *corpus.Bible, raw string, scope search.Scope, bookID uint16) ([]searchHit, bool) {
	refs, err := ref.Parse(b, raw, 0)
	if err != nil || len(refs) == 0 {
		return nil, false
	}
	hits := []searchHit{}
	seen := make(map[string]struct{})
	for _, r := range refs {
		for _, v := range r.Verses(b) {
			if !bookInScope(b, v.BookID, scope, bookID) {
				continue
			}
			key := fmt.Sprintf("%d:%d:%d", v.BookID, v.Chapter, v.Number)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			bk := b.BookByID(v.BookID)
			name := "?"
			if bk != nil {
				name = bk.Name
			}
			hits = append(hits, searchHit{
				Reference: fmt.Sprintf("%s %d:%d", name, v.Chapter, v.Number),
				Book:      name,
				Chapter:   int(v.Chapter),
				Verse:     int(v.Number),
				Text:      v.Text,
			})
		}
	}
	return hits, true
}

func bookInScope(b *corpus.Bible, id uint16, scope search.Scope, bookID uint16) bool {
	switch scope {
	case search.ScopeOT, search.ScopeNT:
		bk := b.BookByID(id)
		if bk == nil {
			return false
		}
		if scope == search.ScopeOT {
			return bk.Testament == 0
		}
		return bk.Testament == 1
	case search.ScopeBook:
		return id == bookID
	default:
		return true
	}
}

// highlightPlain bolds whole-token matches with ANSI when stdout is a terminal.
// It uses the index tokenizer, so folded forms such as "Cæsar" are found too.
func highlightPlain(text string, q search.Query) string {
	if !isTerminal(os.Stdout) {
		return text
	}
	return highlightMatches(text, q)
}

// matchMask marks the bytes of text that a query matches. Excluded terms are
// never positive matches, so they leave the mask clear.
func matchMask(text string, q search.Query) []bool {
	used := make([]bool, len(text))
	for _, sp := range search.TokenizeSpans(text) {
		if !q.MatchesToken(sp.Text) {
			continue
		}
		for i := sp.Start; i < sp.End && i < len(used); i++ {
			used[i] = true
		}
	}
	return used
}

func highlightMatches(text string, q search.Query) string {
	used := matchMask(text, q)
	any := false
	for _, m := range used {
		if m {
			any = true
			break
		}
	}
	if !any {
		return text
	}

	var sb strings.Builder
	in := false
	for i := 0; i < len(text); i++ {
		m := i < len(used) && used[i]
		if m != in {
			if in {
				sb.WriteString("\x1b[0m")
			} else {
				sb.WriteString("\x1b[1;33m")
			}
			in = m
		}
		sb.WriteByte(text[i])
	}
	if in {
		sb.WriteString("\x1b[0m")
	}
	return sb.String()
}

type readVerse struct {
	Book    string `json:"book"`
	Chapter int    `json:"chapter"`
	Verse   int    `json:"verse"`
	Text    string `json:"text"`
}

type readPassage struct {
	Reference string      `json:"reference"`
	Book      string      `json:"book"`
	Chapter   int         `json:"chapter"`
	Verses    []readVerse `json:"verses"`
}

type readOutput struct {
	Reference string        `json:"reference"`
	Passages  []readPassage `json:"passages"`
}

// readSegment is a contiguous run of verses within one chapter.
type readSegment struct {
	label   string
	chapter uint16
	whole   bool
	verses  []*corpus.Verse
}

func runRead(b *corpus.Bible, input string, asJSON, plain bool) error {
	refs, err := ref.Parse(b, input, 0)
	if err != nil {
		return err
	}
	segments := readSegments(b, refs)
	if len(segments) == 0 {
		return fmt.Errorf("no verses found for %q", input)
	}

	labels := make([]string, len(refs))
	for i, r := range refs {
		labels[i] = r.Label(b)
	}
	out := readOutput{Reference: strings.Join(labels, "; "), Passages: []readPassage{}}
	for _, seg := range segments {
		v0 := seg.verses[0]
		name := bookName(b, v0.BookID)
		p := readPassage{
			Reference: seg.label,
			Book:      name,
			Chapter:   int(seg.chapter),
			Verses:    []readVerse{},
		}
		for _, v := range seg.verses {
			p.Verses = append(p.Verses, readVerse{
				Book: name, Chapter: int(v.Chapter), Verse: int(v.Number), Text: v.Text,
			})
		}
		out.Passages = append(out.Passages, p)
	}

	if asJSON {
		return writeJSON(out)
	}

	if plain {
		for _, seg := range segments {
			for _, v := range seg.verses {
				fmt.Println(v.Text)
			}
		}
		return nil
	}

	for i, seg := range segments {
		if i > 0 {
			fmt.Println()
		}
		name := bookName(b, seg.verses[0].BookID)
		if seg.whole {
			fmt.Printf("%s\n\n", seg.label)
			for j, v := range seg.verses {
				// A blank line before paragraph breaks keeps chapters readable.
				if j > 0 && v.Flags&corpus.FlagParagraph != 0 {
					fmt.Println()
				}
				for _, h := range v.Headings() {
					fmt.Printf("%s\n\n", h.Text)
				}
				fmt.Printf("%d %s\n", v.Number, v.Text)
			}
			continue
		}
		for _, v := range seg.verses {
			for _, h := range v.Headings() {
				fmt.Printf("%s\n\n", h.Text)
			}
			fmt.Printf("%s %d:%d\n%s\n\n", name, v.Chapter, v.Number, v.Text)
		}
	}
	return nil
}

// readSegments flattens parsed references into per-chapter segments, removing
// duplicates when references overlap. Labels describe only the included
// verses, so a cross-chapter range reads as "John 3:16-36", "John 4",
// "John 5:1-2".
func readSegments(b *corpus.Bible, refs []ref.Reference) []readSegment {
	seen := make(map[string]struct{})
	var out []readSegment
	for _, r := range refs {
		var cur *readSegment
		flush := func() {
			if cur != nil && len(cur.verses) > 0 {
				out = append(out, *cur)
			}
			cur = nil
		}
		for _, v := range r.Verses(b) {
			key := fmt.Sprintf("%d:%d:%d", v.BookID, v.Chapter, v.Number)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			if cur == nil || cur.chapter != v.Chapter {
				flush()
				cur = &readSegment{chapter: v.Chapter}
			}
			cur.verses = append(cur.verses, v)
		}
		flush()
	}

	for i := range out {
		seg := &out[i]
		v0 := seg.verses[0]
		first := v0.Number
		last := seg.verses[len(seg.verses)-1].Number
		c := b.ChapterAt(v0.BookID, seg.chapter)
		seg.whole = first == 1 && c != nil && last == c.VerseCount
		base := ref.Reference{BookID: v0.BookID, Chapter: seg.chapter}
		seg.label = base.LabelForChapter(b, seg.chapter, first, last, seg.whole)
	}
	return out
}

func bookName(b *corpus.Bible, id uint16) string {
	if bk := b.BookByID(id); bk != nil {
		return bk.Name
	}
	return "?"
}

func parseScope(b *corpus.Bible, scopeName, bookName string) (search.Scope, uint16, string, error) {
	if bookName != "" {
		book, err := lookupBook(b, bookName)
		if err != nil {
			return 0, 0, "", err
		}
		return search.ScopeBook, book.ID, book.Name, nil
	}
	switch strings.ToLower(strings.TrimSpace(scopeName)) {
	case "", "all":
		return search.ScopeAll, 0, "All books", nil
	case "ot", "old", "old testament":
		return search.ScopeOT, 0, "Old Testament", nil
	case "nt", "new", "new testament":
		return search.ScopeNT, 0, "New Testament", nil
	case "book":
		return 0, 0, "", fmt.Errorf("--scope book needs --book NAME")
	default:
		return 0, 0, "", fmt.Errorf("unknown scope %q (use all, ot, nt, or book)", scopeName)
	}
}

func lookupBook(b *corpus.Bible, name string) (*corpus.Book, error) {
	books := ref.FindBooks(name, 5)
	for _, info := range books {
		if strings.EqualFold(info.Name, name) {
			return b.BookByID(info.ID), nil
		}
	}
	if len(books) > 0 {
		return b.BookByID(books[0].ID), nil
	}
	return nil, fmt.Errorf("unknown book %q", name)
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
