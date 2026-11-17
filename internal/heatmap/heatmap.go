// Package heatmap aggregates search matches into per-book and per-chapter
// concentrations. It is intentionally pure: callers decide how to render the
// resulting scores.
package heatmap

import (
	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/search"
)

// Mode controls how strength is normalised.
type Mode uint8

const (
	// ModeDensity normalises by verse count, so a small book with many local
	// matches can stand out as strongly as a large book with many total hits.
	ModeDensity Mode = iota
	// ModeCount normalises by the largest book or chapter total.
	ModeCount
)

// Label returns a user-facing name for the mode.
func (m Mode) Label() string {
	if m == ModeCount {
		return "Count"
	}
	return "Density"
}

// Options controls aggregation.
type Options struct {
	Mode   Mode
	Scope  search.Scope
	BookID uint16
}

// Chapter is one chapter's contribution to a heatmap.
type Chapter struct {
	Number   uint16
	Matches  int
	Verses   int
	Density  float64
	Score    float64 // 0..1, relative to the strongest chapter in this book.
	TopVerse uint16
	TopScore float64
}

// Book is one book's contribution to a heatmap.
type Book struct {
	ID        uint16
	Name      string
	Abbrev    string
	Testament uint8
	Matches   int
	Verses    int
	Density   float64
	Score     float64 // 0..1, relative to the strongest book in the report.
	InScope   bool
	Chapters  []Chapter
}

// Report is a complete heatmap for one search result set.
type Report struct {
	Mode       Mode
	Total      int
	Books      []Book
	MaxMatches int
}

// Build aggregates every hit in hits across b. Hits outside the corpus are
// ignored, and books are returned in canonical order even when they have no
// matches.
func Build(b *corpus.Bible, hits []search.Hit, opts Options) *Report {
	r := &Report{Mode: opts.Mode}
	if b == nil {
		return r
	}

	r.Books = make([]Book, 0, len(b.Books))
	bookIndex := make(map[uint16]int, len(b.Books))
	for i := range b.Books {
		bk := &b.Books[i]
		bookIndex[bk.ID] = len(r.Books)
		book := Book{
			ID:        bk.ID,
			Name:      bk.Name,
			Abbrev:    bk.Abbrev,
			Testament: bk.Testament,
			InScope:   inScope(bk, opts),
		}
		for ch := 1; ch <= int(bk.ChapterCount); ch++ {
			c := b.ChapterAt(bk.ID, uint16(ch))
			verses := 0
			if c != nil {
				verses = int(c.VerseCount)
			}
			book.Chapters = append(book.Chapters, Chapter{Number: uint16(ch), Verses: verses})
		}
		book.Verses = sumVerses(book.Chapters)
		book.Density = density(book.Matches, book.Verses)
		r.Books = append(r.Books, book)
	}

	for _, h := range hits {
		if h.Verse < 0 || h.Verse >= len(b.Verses) {
			continue
		}
		v := &b.Verses[h.Verse]
		bi, ok := bookIndex[v.BookID]
		if !ok {
			continue
		}
		book := &r.Books[bi]
		ci := int(v.Chapter) - 1
		if ci < 0 || ci >= len(book.Chapters) {
			continue
		}
		book.Matches++
		ch := &book.Chapters[ci]
		ch.Matches++
		if h.Score > ch.TopScore || (h.Score == ch.TopScore && (ch.TopVerse == 0 || v.Number < ch.TopVerse)) {
			ch.TopScore = h.Score
			ch.TopVerse = v.Number
		}
		r.Total++
	}

	maxBook := 0.0
	for i := range r.Books {
		book := &r.Books[i]
		book.Density = density(book.Matches, book.Verses)
		maxChapter := 0.0
		for j := range book.Chapters {
			ch := &book.Chapters[j]
			ch.Density = density(ch.Matches, ch.Verses)
			if opts.Mode == ModeCount {
				maxChapter = max(maxChapter, float64(ch.Matches))
			} else {
				maxChapter = max(maxChapter, ch.Density)
			}
		}
		for j := range book.Chapters {
			ch := &book.Chapters[j]
			if maxChapter > 0 {
				if opts.Mode == ModeCount {
					ch.Score = float64(ch.Matches) / maxChapter
				} else {
					ch.Score = ch.Density / maxChapter
				}
			}
		}
		r.MaxMatches = max(r.MaxMatches, book.Matches)
		switch opts.Mode {
		case ModeCount:
			maxBook = max(maxBook, float64(book.Matches))
		default:
			maxBook = max(maxBook, book.Density)
		}
	}
	if maxBook > 0 {
		for i := range r.Books {
			book := &r.Books[i]
			if opts.Mode == ModeCount {
				book.Score = float64(book.Matches) / maxBook
			} else {
				book.Score = book.Density / maxBook
			}
		}
	}
	return r
}

// Bucket resamples chapter scores into cells buckets for fixed-width
// sparklines. Wide buckets keep their strongest chapter; when there are more
// cells than chapters, each chapter is repeated so short books do not appear
// artificially sparse.
func Bucket(chapters []Chapter, cells int) []float64 {
	if cells <= 0 || len(chapters) == 0 {
		return nil
	}
	out := make([]float64, cells)
	n := len(chapters)
	if cells >= n {
		for cell := 0; cell < cells; cell++ {
			out[cell] = chapters[cell*n/cells].Score
		}
		return out
	}
	for cell := 0; cell < cells; cell++ {
		start := cell * n / cells
		end := (cell + 1) * n / cells
		for i := start; i < end; i++ {
			out[cell] = max(out[cell], chapters[i].Score)
		}
	}
	return out
}

func inScope(bk *corpus.Book, opts Options) bool {
	switch opts.Scope {
	case search.ScopeOT:
		return bk.Testament == 0
	case search.ScopeNT:
		return bk.Testament == 1
	case search.ScopeBook:
		return bk.ID == opts.BookID
	default:
		return true
	}
}

func sumVerses(chapters []Chapter) int {
	n := 0
	for i := range chapters {
		n += chapters[i].Verses
	}
	return n
}

func density(matches, verses int) float64 {
	if verses <= 0 {
		return 0
	}
	return float64(matches) / float64(verses)
}
