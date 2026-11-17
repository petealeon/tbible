// Package search builds and queries the embedded full-text index.
package search

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// TokenSpan is one folded token and the byte range of its source text. The
// source range is useful for highlighting: folding can change a token's
// length (for example "æ" becomes "ae").
type TokenSpan struct {
	Text  string
	Start int
	End   int
}

// Tokenize splits text into folded search tokens. Letters and digits are
// kept; everything else separates tokens. Diacritics and common Latin
// ligatures are folded so "Caesar" finds "Cæsar".
func Tokenize(s string) []string {
	spans := TokenizeSpans(s)
	out := make([]string, len(spans))
	for i := range spans {
		out[i] = spans[i].Text
	}
	return out
}

// TokenizeSpans is Tokenize with source byte ranges preserved.
func TokenizeSpans(s string) []TokenSpan {
	out := make([]TokenSpan, 0, 16)
	var b strings.Builder
	start := -1
	flush := func(end int) {
		if b.Len() == 0 {
			start = -1
			return
		}
		out = append(out, TokenSpan{Text: b.String(), Start: start, End: end})
		b.Reset()
		start = -1
	}
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		next := i + size
		if r == utf8.RuneError && size == 1 {
			flush(i)
			i = next
			continue
		}
		folded, token := foldRune(r)
		if !token {
			flush(i)
			i = next
			continue
		}
		if start < 0 {
			start = i
		}
		b.WriteString(folded)
		i = next
	}
	flush(len(s))
	return out
}

func foldRune(r rune) (string, bool) {
	switch r {
	case 'æ', 'Æ':
		return "ae", true
	case 'œ', 'Œ':
		return "oe", true
	case 'ß':
		return "ss", true
	case 'ø', 'Ø':
		return "o", true
	case 'ð', 'Ð':
		return "d", true
	case 'þ', 'Þ':
		return "th", true
	case 'ł', 'Ł':
		return "l", true
	}
	r = unicode.ToLower(r)
	if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
		return "", false
	}
	if r < 128 {
		return string(r), true
	}
	var b strings.Builder
	for _, d := range norm.NFD.String(string(r)) {
		if unicode.Is(unicode.Mn, d) {
			continue
		}
		b.WriteRune(d)
	}
	return b.String(), b.Len() > 0
}

// Normalize returns a space-separated folded token string.
func Normalize(s string) string {
	return strings.Join(Tokenize(s), " ")
}

// Fold tokenizes a single word. It returns the empty string when the input
// has no searchable characters.
func Fold(s string) string {
	t := Tokenize(s)
	if len(t) == 0 {
		return ""
	}
	return strings.Join(t, " ")
}
