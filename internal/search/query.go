package search

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// Scope limits a search.
type Scope uint8

const (
	ScopeAll Scope = iota
	ScopeOT
	ScopeNT
	ScopeBook
)

// Options controls a search.
type Options struct {
	Scope  Scope
	BookID uint16
	Limit  int
}

// Query is a parsed search expression.
type Query struct {
	Raw      string
	Terms    []string
	Prefixes []string
	Phrases  [][]string
	Exclude  []string
	Scope    Scope
	BookID   uint16
}

// ParseQuery splits a query into terms, prefixes, phrases, and exclusions.
// Supported syntax is intentionally small:
//
//	word            exact token
//	word*           prefix
//	"exact phrase"  contiguous tokens
//	-word           exclusion
//
// Scope chips in the UI are the primary way to filter; callers may also set
// Query.Scope.
func ParseQuery(raw string) Query {
	q := Query{Raw: raw}
	runes := []rune(raw)
	var cur []rune
	var phrase []rune
	inQuote := false
	neg := false
	flush := func() {
		s := strings.TrimSpace(string(cur))
		cur = cur[:0]
		if s == "" {
			return
		}
		if inQuote {
			phrase = []rune(s)
			return
		}
		p := parseToken(s)
		if neg {
			q.Exclude = append(q.Exclude, p...)
			neg = false
			return
		}
		q.Terms = append(q.Terms, p...)
	}
	for _, r := range runes {
		if r == '"' {
			if inQuote {
				inQuote = false
				addPhrase(&q, string(phrase))
				phrase = phrase[:0]
			} else {
				flush()
				inQuote = true
			}
			continue
		}
		if inQuote {
			phrase = append(phrase, r)
			continue
		}
		if unicode.IsSpace(r) || r == ',' || r == ';' {
			flush()
			continue
		}
		if r == '-' && len(cur) == 0 {
			neg = true
			continue
		}
		cur = append(cur, r)
	}
	if inQuote {
		addPhrase(&q, string(phrase))
	} else {
		flush()
	}
	q.Terms = dedupe(q.Terms)
	q.Prefixes = dedupe(q.Prefixes)
	q.Exclude = dedupe(q.Exclude)
	return q
}

func parseToken(s string) []string {
	prefix := strings.HasSuffix(s, "*")
	if prefix {
		s = strings.TrimRight(s, "*")
	}
	toks := Tokenize(s)
	if len(toks) == 0 {
		return nil
	}
	if prefix {
		return []string{toks[len(toks)-1] + "*"}
	}
	return toks
}

func addPhrase(q *Query, s string) {
	toks := Tokenize(s)
	if len(toks) > 0 {
		q.Phrases = append(q.Phrases, toks)
	}
}

func dedupe(in []string) []string {
	if len(in) < 2 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// MatchesToken reports whether a folded token matches a positive query term,
// prefix, or phrase member. Excluded terms never match.
func (q Query) MatchesToken(tok string) bool {
	if tok == "" {
		return false
	}
	match := func(t string) bool {
		if strings.HasSuffix(t, "*") {
			p := strings.TrimSuffix(t, "*")
			return p != "" && strings.HasPrefix(tok, p)
		}
		return t == tok
	}
	for _, t := range q.Terms {
		if match(t) {
			return true
		}
	}
	for _, p := range q.Prefixes {
		if p != "" && strings.HasPrefix(tok, p) {
			return true
		}
	}
	for _, phrase := range q.Phrases {
		for _, t := range phrase {
			if match(t) {
				return true
			}
		}
	}
	return false
}

// Hit is one ranked verse.
type Hit struct {
	Verse int
	Score float64
}

// Result is the outcome of a search.
type Result struct {
	// Hits is the ranked, limited result list intended for display.
	Hits []Hit
	// AllHits contains every ranked match, including any beyond Hits. It lets
	// callers aggregate the complete result set without rerunning the search.
	AllHits []Hit
	Total   int
	Relaxed bool
	Query   Query
	// Words lists folded tokens that should be highlighted.
	Words []string
}

// Search runs q against the index. text returns the display text of a verse;
// it is required for exact-phrase verification and may be nil when the query
// has no phrases.
func (ix *Index) Search(q Query, text func(int) string, opts Options) Result {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	res := Result{Query: q}

	// Every positive token, including phrase members, must be present before
	// scoring. Prefix entries are expanded inside postingLists.
	terms := append([]string(nil), q.Terms...)
	for _, p := range q.Phrases {
		terms = append(terms, p...)
	}
	terms = dedupe(terms)
	if len(terms) == 0 {
		return res
	}

	lists := make([][]weighted, 0, len(terms))
	for _, t := range terms {
		prefix := strings.HasSuffix(t, "*")
		t = strings.TrimSuffix(t, "*")
		l := ix.postingList(t, prefix)
		if len(l) == 0 {
			return res
		}
		lists = append(lists, l)
	}

	scores := ix.intersect(lists, opts)
	if len(scores) == 0 && len(lists) > 1 {
		// Leave-one-out fallback: prefer verses that match all but one term
		// over a plain union, which lets a single common term dominate.
		scores = make(map[uint32]float64)
		for i := range lists {
			part := make([][]weighted, 0, len(lists)-1)
			part = append(part, lists[:i]...)
			part = append(part, lists[i+1:]...)
			for v, s := range ix.intersect(part, opts) {
				if cur, ok := scores[v]; !ok || s > cur {
					scores[v] = s
				}
			}
		}
		if len(scores) > 0 {
			res.Relaxed = true
		} else {
			scores = ix.union(lists, opts)
			res.Relaxed = len(scores) > 0
		}
	}
	if len(scores) == 0 {
		return res
	}

	// Phrase verification and exclusions.
	excluded := ix.exclusionSet(q.Exclude)
	if len(q.Phrases) > 0 && text != nil {
		for v := range scores {
			if !containsAllPhrases(text(int(v)), q.Phrases) {
				delete(scores, v)
				continue
			}
			scores[v] += 8 * float64(len(q.Phrases))
		}
	}
	for v := range scores {
		if _, bad := excluded[v]; bad {
			delete(scores, v)
		}
	}

	res.Total = len(scores)
	hits := make([]Hit, 0, len(scores))
	for v, s := range scores {
		hits = append(hits, Hit{Verse: int(v), Score: s})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Verse < hits[j].Verse
	})
	res.AllHits = hits
	if len(hits) > opts.Limit {
		hits = hits[:opts.Limit]
	}
	res.Hits = hits
	res.Words = terms
	for i := range res.Words {
		res.Words[i] = strings.TrimSuffix(res.Words[i], "*")
	}
	return res
}

type weighted struct {
	verse  uint32
	weight float64
}

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

func (ix *Index) score(e *tokenEntry, p posting) float64 {
	if int(p.verse) >= len(ix.docLen) {
		return 0
	}
	n := float64(ix.verseCount)
	df := float64(e.docFreq)
	idf := math.Log(1 + (n-df+0.5)/(df+0.5))
	tf := float64(p.tf)
	dl := float64(ix.docLen[p.verse])
	avg := ix.avgLen
	if avg <= 0 {
		avg = 1
	}
	tfNorm := tf * (bm25K1 + 1) / (tf + bm25K1*(1-bm25B+bm25B*dl/avg))
	return idf * tfNorm
}

func (ix *Index) postingList(token string, prefix bool) []weighted {
	if !prefix {
		i := ix.findToken(token)
		if i < 0 {
			return nil
		}
		e := &ix.tokens[i]
		out := make([]weighted, int(e.docFreq))
		j := 0
		ix.forEachPosting(e, func(p posting) {
			out[j] = weighted{verse: p.verse, weight: ix.score(e, p)}
			j++
		})
		return out
	}
	lo, hi := ix.prefixRange(token)
	merged := make(map[uint32]float64)
	for i := lo; i < hi; i++ {
		e := &ix.tokens[i]
		ix.forEachPosting(e, func(p posting) {
			merged[p.verse] += ix.score(e, p) * 0.75
		})
	}
	out := make([]weighted, 0, len(merged))
	for v, w := range merged {
		out = append(out, weighted{verse: v, weight: w})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].verse < out[j].verse })
	return out
}

func (ix *Index) allowed(v uint32, opts Options) bool {
	if int(v) >= len(ix.verseBook) {
		return false
	}
	switch opts.Scope {
	case ScopeOT:
		return ix.verseBook[v] >= 1 && ix.verseBook[v] <= 39
	case ScopeNT:
		return ix.verseBook[v] >= 40 && ix.verseBook[v] <= 66
	case ScopeBook:
		return ix.verseBook[v] == opts.BookID
	default:
		return true
	}
}

func (ix *Index) intersect(lists [][]weighted, opts Options) map[uint32]float64 {
	if len(lists) == 0 {
		return nil
	}
	// Start from the shortest list.
	sort.Slice(lists, func(i, j int) bool { return len(lists[i]) < len(lists[j]) })
	scores := make(map[uint32]float64, len(lists[0]))
	for _, p := range lists[0] {
		if ix.allowed(p.verse, opts) {
			scores[p.verse] = p.weight
		}
	}
	for _, list := range lists[1:] {
		if len(scores) == 0 {
			return scores
		}
		next := make(map[uint32]float64, len(scores))
		for _, p := range list {
			if s, ok := scores[p.verse]; ok && ix.allowed(p.verse, opts) {
				next[p.verse] = s + p.weight
			}
		}
		scores = next
	}
	return scores
}

func (ix *Index) union(lists [][]weighted, opts Options) map[uint32]float64 {
	scores := make(map[uint32]float64)
	for _, list := range lists {
		for _, p := range list {
			if ix.allowed(p.verse, opts) {
				scores[p.verse] += p.weight
			}
		}
	}
	return scores
}

func (ix *Index) exclusionSet(terms []string) map[uint32]struct{} {
	out := make(map[uint32]struct{})
	for _, t := range terms {
		prefix := strings.HasSuffix(t, "*")
		t = strings.TrimSuffix(t, "*")
		for _, p := range ix.postingList(t, prefix) {
			out[p.verse] = struct{}{}
		}
	}
	return out
}

func containsAllPhrases(text string, phrases [][]string) bool {
	norm := " " + Normalize(text) + " "
	for _, p := range phrases {
		if !strings.Contains(norm, " "+strings.Join(p, " ")+" ") {
			return false
		}
	}
	return true
}

// Suggest returns dictionary terms starting with prefix, useful for
// diagnostics and future autocomplete.
func (ix *Index) Suggest(prefix string, limit int) []string {
	if limit <= 0 || prefix == "" {
		return nil
	}
	lo, hi := ix.prefixRange(prefix)
	if hi-lo > limit {
		hi = lo + limit
	}
	out := make([]string, 0, hi-lo)
	for i := lo; i < hi; i++ {
		out = append(out, ix.tokens[i].token)
	}
	return out
}
