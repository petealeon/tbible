package tui

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/search"
)

// wrapRanges splits s into byte ranges, none wider than width terminal cells.
// It breaks at spaces where possible.
func wrapRanges(s string, width int) []corpus.Span {
	if width < 1 {
		width = 1
	}
	if s == "" {
		return []corpus.Span{{Start: 0, End: 0}}
	}
	var out []corpus.Span
	lineStart := 0
	cells := 0
	lastSpace := -1
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		w := runewidth.RuneWidth(r)
		if r == ' ' {
			lastSpace = i
		}
		if cells+w > width && i > lineStart {
			if lastSpace >= lineStart {
				out = append(out, corpus.Span{Start: lineStart, End: lastSpace})
				lineStart = lastSpace + 1
				for lineStart < len(s) && s[lineStart] == ' ' {
					lineStart++
				}
				i = lineStart
				cells = 0
				lastSpace = -1
				continue
			}
			out = append(out, corpus.Span{Start: lineStart, End: i})
			lineStart = i
			cells = 0
			lastSpace = -1
			continue
		}
		cells += w
		i += size
	}
	if lineStart < len(s) {
		out = append(out, corpus.Span{Start: lineStart, End: len(s)})
	}
	if len(out) == 0 {
		out = append(out, corpus.Span{Start: 0, End: len(s)})
	}
	return out
}

// wrapTextLines wraps user-authored text while preserving explicit newlines.
// Each returned string is safe to render as one terminal line.
func wrapTextLines(s string, width int) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSuffix(line, "\r")
		for _, rg := range wrapRanges(line, width) {
			out = append(out, line[rg.Start:rg.End])
		}
	}
	if len(out) == 0 {
		out = append(out, "")
	}
	return out
}

// truncateANSI truncates already-styled text to width cells. ansi.Truncate
// preserves escape sequences and closes any style left open at the cut, which
// the previous hand-rolled loop did not.
func truncateANSI(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "")
}

// clipSpans returns spans overlapping [a,b), clipped to it.
func clipSpans(spans []corpus.Span, a, b int) []corpus.Span {
	var out []corpus.Span
	for _, s := range spans {
		if s.End <= a || s.Start >= b {
			continue
		}
		if s.Start < a {
			s.Start = a
		}
		if s.End > b {
			s.End = b
		}
		if s.End > s.Start {
			out = append(out, s)
		}
	}
	return out
}

func covered(spans []corpus.Span, at int) bool {
	for _, s := range spans {
		if at >= s.Start && at < s.End {
			return true
		}
	}
	return false
}

// renderVerseRange renders text[a:b] with italic and red-letter styling.
func renderVerseRange(text string, a, b int, italics, red []corpus.Span) string {
	if a >= b {
		return ""
	}
	italics = clipSpans(italics, a, b)
	red = clipSpans(red, a, b)
	bounds := []int{a, b}
	for _, s := range italics {
		bounds = append(bounds, s.Start, s.End)
	}
	for _, s := range red {
		bounds = append(bounds, s.Start, s.End)
	}
	sort.Ints(bounds)
	var out strings.Builder
	for i := 0; i+1 < len(bounds); i++ {
		x, y := bounds[i], bounds[i+1]
		if y <= x {
			continue
		}
		seg := text[x:y]
		it := covered(italics, x)
		rd := covered(red, x)
		switch {
		case it && rd:
			out.WriteString(redItalic.Render(seg))
		case it:
			out.WriteString(italicStyle.Render(seg))
		case rd:
			out.WriteString(redStyle.Render(seg))
		default:
			out.WriteString(textStyle.Render(seg))
		}
	}
	return out.String()
}

// highlightWords bolds whole-token matches in plain text. Tokens are folded
// with the index tokenizer, so prefixes and ligatures behave as they do in
// search.
func highlightWords(text string, q search.Query) string {
	used := make([]bool, len(text))
	any := false
	for _, sp := range search.TokenizeSpans(text) {
		if !q.MatchesToken(sp.Text) {
			continue
		}
		any = true
		for i := sp.Start; i < sp.End && i < len(used); i++ {
			used[i] = true
		}
	}
	if !any {
		return textStyle.Render(text)
	}

	var out strings.Builder
	start := 0
	inMatch := false
	flush := func(end int) {
		if end <= start {
			return
		}
		if inMatch {
			out.WriteString(hitStyle.Render(text[start:end]))
		} else {
			out.WriteString(textStyle.Render(text[start:end]))
		}
		start = end
	}
	for i := 0; i < len(text); {
		m := i < len(used) && used[i]
		if m != inMatch {
			flush(i)
			inMatch = m
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
	}
	flush(len(text))
	return out.String()
}

// makeSnippet returns a one-line context window around the first query match.
func makeSnippet(text string, q search.Query, width int) string {
	if width <= 0 {
		return ""
	}
	best := -1
	for _, sp := range search.TokenizeSpans(text) {
		if q.MatchesToken(sp.Text) {
			best = sp.Start
			break
		}
	}
	start := 0
	if best > 32 {
		start = best - 24
		if sp := strings.LastIndexByte(text[:start], ' '); sp > 0 {
			start = sp + 1
		}
	}
	s := text[start:]
	prefix := ""
	if start > 0 {
		prefix = "…"
	}
	s = runewidth.Truncate(s, width-runewidth.StringWidth(prefix), "…")
	return prefix + s
}
