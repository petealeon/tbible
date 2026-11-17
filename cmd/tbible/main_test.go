package main

import (
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/petealeon/tbible/v2/internal/embedded"
	"github.com/petealeon/tbible/v2/internal/ref"
	"github.com/petealeon/tbible/v2/internal/search"
)

func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	os.Stdout = old
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if runErr != nil {
		t.Fatal(runErr)
	}
	return string(data)
}

func TestReferenceHits(t *testing.T) {
	b, err := embedded.Bible()
	if err != nil {
		t.Fatal(err)
	}

	hits, ok := referenceHits(b, "John 3:16", search.ScopeAll, 0)
	if !ok || len(hits) != 1 {
		t.Fatalf("single reference = %+v, %v", hits, ok)
	}
	if hits[0].Reference != "John 3:16" || !strings.Contains(hits[0].Text, "For God so loved") {
		t.Fatalf("hit = %+v", hits[0])
	}

	hits, ok = referenceHits(b, "John 3:16; Romans 5:8", search.ScopeAll, 0)
	if !ok || len(hits) != 2 {
		t.Fatalf("multi reference = %+v, %v", hits, ok)
	}
	if hits[0].Reference != "John 3:16" || hits[1].Reference != "Romans 5:8" {
		t.Fatalf("multi refs = %+v", hits)
	}

	hits, ok = referenceHits(b, "John 3-5", search.ScopeAll, 0)
	if !ok || len(hits) != 36+54+47 {
		t.Fatalf("chapter range has %d hits (%v)", len(hits), ok)
	}
	if hits[0].Reference != "John 3:1" || hits[len(hits)-1].Reference != "John 5:47" {
		t.Fatalf("chapter range bounds = %q .. %q", hits[0].Reference, hits[len(hits)-1].Reference)
	}

	// A reference outside the selected scope is still recognised, but empty.
	hits, ok = referenceHits(b, "John 3:16", search.ScopeOT, 0)
	if !ok || len(hits) != 0 {
		t.Fatalf("out-of-scope reference = %+v, %v", hits, ok)
	}
	hits, ok = referenceHits(b, "John 3:16", search.ScopeBook, 43)
	if !ok || len(hits) != 1 {
		t.Fatalf("book scope = %+v, %v", hits, ok)
	}
	hits, ok = referenceHits(b, "John 3:16", search.ScopeBook, 40)
	if !ok || len(hits) != 0 {
		t.Fatalf("wrong book scope = %+v, %v", hits, ok)
	}

	if hits, ok := referenceHits(b, "shepherd", search.ScopeAll, 0); ok || hits != nil {
		t.Fatalf("keyword treated as reference: %+v, %v", hits, ok)
	}
}

func TestReadSegments(t *testing.T) {
	b, err := embedded.Bible()
	if err != nil {
		t.Fatal(err)
	}
	segments := func(input string) []readSegment {
		t.Helper()
		refs, err := ref.Parse(b, input, 0)
		if err != nil {
			t.Fatal(err)
		}
		return readSegments(b, refs)
	}

	segs := segments("John 3:16-5:2")
	if len(segs) != 3 {
		t.Fatalf("cross-chapter segments = %+v", segs)
	}
	want := []struct {
		label  string
		verses int
		whole  bool
	}{
		{"John 3:16-36", 21, false},
		{"John 4", 54, true},
		{"John 5:1-2", 2, false},
	}
	for i, w := range want {
		if segs[i].label != w.label || len(segs[i].verses) != w.verses || segs[i].whole != w.whole {
			t.Errorf("segment %d = %q (%d verses, whole=%v), want %+v",
				i, segs[i].label, len(segs[i].verses), segs[i].whole, w)
		}
	}

	segs = segments("John 3:16; Romans 5:8")
	if len(segs) != 2 || segs[0].label != "John 3:16" || segs[1].label != "Romans 5:8" {
		t.Fatalf("multi segments = %+v", segs)
	}

	segs = segments("John 3-5")
	if len(segs) != 3 {
		t.Fatalf("chapter range segments = %+v", segs)
	}
	if segs[0].label != "John 3" || segs[1].label != "John 4" || segs[2].label != "John 5" {
		t.Fatalf("chapter labels = %q, %q, %q", segs[0].label, segs[1].label, segs[2].label)
	}
	if !segs[0].whole || !segs[1].whole || !segs[2].whole {
		t.Fatal("whole-chapter range split into partial segments")
	}

	// Overlapping references do not duplicate verses.
	segs = segments("John 3:16; John 3:16-18")
	unique := map[uint16]struct{}{}
	for _, seg := range segs {
		for _, v := range seg.verses {
			if _, dup := unique[v.Number]; dup {
				t.Fatalf("duplicate verse %d in %+v", v.Number, segs)
			}
			unique[v.Number] = struct{}{}
		}
	}
	if len(unique) != 3 {
		t.Fatalf("overlapping refs covered %d verses, want 3", len(unique))
	}
}

func TestMatchMask(t *testing.T) {
	q := search.ParseQuery(`shep* "still waters" -world`)
	text := "The shepherd still waters world"
	got := maskWords(text, matchMask(text, q))
	if !reflect.DeepEqual(got, []string{"shepherd", "still", "waters"}) {
		t.Fatalf("masked words = %v", got)
	}

	// Folding means the source bytes for a ligature are matched whole.
	q = search.ParseQuery("caesar")
	text = "Cæsar's things"
	if got := maskWords(text, matchMask(text, q)); !reflect.DeepEqual(got, []string{"Cæsar"}) {
		t.Fatalf("folded mask = %v", got)
	}

	q = search.ParseQuery("-world")
	if got := maskWords("world", matchMask("world", q)); len(got) != 0 {
		t.Fatalf("exclusion masked %v", got)
	}
}

// maskWords reconstructs the contiguous runs marked in a match mask.
func maskWords(text string, mask []bool) []string {
	var out []string
	start := -1
	for i := 0; i <= len(text); i++ {
		on := i < len(text) && mask[i]
		if on && start < 0 {
			start = i
		}
		if !on && start >= 0 {
			out = append(out, text[start:i])
			start = -1
		}
	}
	return out
}

func TestHighlightMatches(t *testing.T) {
	q := search.ParseQuery("shepherd")
	out := highlightMatches("The shepherd sings", q)
	if !strings.Contains(out, "\x1b[1;33mshepherd\x1b[0m") {
		t.Fatalf("highlight = %q", out)
	}
	if got := highlightMatches("nothing here", q); got != "nothing here" {
		t.Fatalf("unmatched text = %q", got)
	}
}

func TestParseScope(t *testing.T) {
	b, err := embedded.Bible()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		scope string
		book  string
		want  search.Scope
		id    uint16
		label string
		err   bool
	}{
		{"all", "", search.ScopeAll, 0, "All books", false},
		{"ot", "", search.ScopeOT, 0, "Old Testament", false},
		{"New Testament", "", search.ScopeNT, 0, "New Testament", false},
		{"all", "John", search.ScopeBook, 43, "John", false},
		{"book", "", 0, 0, "", true},
		{"bogus", "", 0, 0, "", true},
		{"all", "Not a book", 0, 0, "", true},
	}
	for _, c := range cases {
		scope, id, label, err := parseScope(b, c.scope, c.book)
		if c.err {
			if err == nil {
				t.Errorf("%q/%q: expected an error", c.scope, c.book)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q/%q: %v", c.scope, c.book, err)
			continue
		}
		if scope != c.want || id != c.id || label != c.label {
			t.Errorf("%q/%q = %v, %d, %q", c.scope, c.book, scope, id, label)
		}
	}
}

func TestReadJSONSchema(t *testing.T) {
	b, err := embedded.Bible()
	if err != nil {
		t.Fatal(err)
	}
	raw := captureStdout(t, func() error {
		return runRead(b, "John 3:16-5:2", true, false)
	})
	var out readOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	if out.Reference != "John 3:16-5:2" {
		t.Fatalf("reference = %q", out.Reference)
	}
	if len(out.Passages) != 3 {
		t.Fatalf("passages = %+v", out.Passages)
	}
	if out.Passages[0].Reference != "John 3:16-36" || len(out.Passages[0].Verses) != 21 {
		t.Fatalf("first passage = %+v", out.Passages[0])
	}
	if out.Passages[2].Reference != "John 5:1-2" || len(out.Passages[2].Verses) != 2 {
		t.Fatalf("last passage = %+v", out.Passages[2])
	}

	raw = captureStdout(t, func() error {
		return runRead(b, "John 3:16; Romans 5:8", true, false)
	})
	if !strings.Contains(raw, `"passages"`) || strings.Contains(raw, `"verses": null`) {
		t.Fatalf("multi-read JSON = %s", raw)
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Passages) != 2 {
		t.Fatalf("multi passages = %+v", out.Passages)
	}
}

func TestSearchReferenceJSON(t *testing.T) {
	b, ix, err := embedded.Load()
	if err != nil {
		t.Fatal(err)
	}
	raw := captureStdout(t, func() error {
		return runSearch(b, ix, "John 3:16", "all", "", 25, true, true)
	})
	var out searchOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	if out.Total != 1 || len(out.Hits) != 1 || out.Hits[0].Reference != "John 3:16" {
		t.Fatalf("reference search = %+v", out)
	}

	raw = captureStdout(t, func() error {
		return runSearch(b, ix, "John 3:16", "ot", "", 25, true, true)
	})
	if !strings.Contains(raw, `"hits": []`) {
		t.Fatalf("empty scoped hits should be [], got %s", raw)
	}

	raw = captureStdout(t, func() error {
		return runSearch(b, ix, "shep*", "all", "", 5, true, true)
	})
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	if out.Total == 0 || len(out.Hits) == 0 || len(out.Hits) > 5 {
		t.Fatalf("prefix search = %+v", out)
	}
}
