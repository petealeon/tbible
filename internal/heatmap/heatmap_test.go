package heatmap_test

import (
	"math"
	"testing"

	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/heatmap"
	"github.com/petealeon/tbible/v2/internal/search"
)

func testBible() *corpus.Bible {
	b := &corpus.Bible{
		Books: []corpus.Book{
			{ID: 1, Name: "Alpha", Abbrev: "Al", Testament: 0, ChapterCount: 2, FirstChapter: 0},
			{ID: 2, Name: "Beta", Abbrev: "Be", Testament: 1, ChapterCount: 1, FirstChapter: 2},
		},
		Chapters: []corpus.Chapter{
			{BookID: 1, Number: 1, FirstVerse: 0, VerseCount: 2},
			{BookID: 1, Number: 2, FirstVerse: 2, VerseCount: 1},
			{BookID: 2, Number: 1, FirstVerse: 3, VerseCount: 2},
		},
		Verses: []corpus.Verse{
			{BookID: 1, Chapter: 1, Number: 1, Text: "x"},
			{BookID: 1, Chapter: 1, Number: 2, Text: "x"},
			{BookID: 1, Chapter: 2, Number: 1, Text: "x"},
			{BookID: 2, Chapter: 1, Number: 1, Text: "x"},
			{BookID: 2, Chapter: 1, Number: 2, Text: "x"},
		},
	}
	b.Prepare()
	return b
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestBuildDensity(t *testing.T) {
	b := testBible()
	hits := []search.Hit{
		{Verse: 0, Score: 1},
		{Verse: 1, Score: 3},
		{Verse: 3, Score: 2},
		{Verse: 99, Score: 9},
	}
	r := heatmap.Build(b, hits, heatmap.Options{Mode: heatmap.ModeDensity})

	if r.Total != 3 || r.MaxMatches != 2 {
		t.Fatalf("total=%d max=%d, want 3/2", r.Total, r.MaxMatches)
	}
	if len(r.Books) != 2 || r.Books[0].Name != "Alpha" || r.Books[1].Name != "Beta" {
		t.Fatalf("books = %+v", r.Books)
	}
	first := r.Books[0]
	if first.Matches != 2 || first.Verses != 3 || !near(first.Density, 2.0/3.0) {
		t.Fatalf("Alpha stats = %+v", first)
	}
	if !near(first.Score, 1) {
		t.Fatalf("Alpha density score = %v, want 1", first.Score)
	}
	if !near(r.Books[1].Score, 0.75) {
		t.Fatalf("Beta density score = %v, want 0.75", r.Books[1].Score)
	}
	ch := first.Chapters[0]
	if ch.Matches != 2 || ch.Verses != 2 || !near(ch.Density, 1) || !near(ch.Score, 1) {
		t.Fatalf("Alpha 1 stats = %+v", ch)
	}
	if ch.TopVerse != 2 || !near(ch.TopScore, 3) {
		t.Fatalf("Alpha 1 top = %d/%v, want 2/3", ch.TopVerse, ch.TopScore)
	}
	if first.Chapters[1].Matches != 0 || first.Chapters[1].Score != 0 {
		t.Fatalf("Alpha 2 stats = %+v", first.Chapters[1])
	}
}

func TestBuildCountAndScope(t *testing.T) {
	b := testBible()
	hits := []search.Hit{
		{Verse: 0, Score: 1},
		{Verse: 1, Score: 3},
		{Verse: 3, Score: 2},
	}
	r := heatmap.Build(b, hits, heatmap.Options{
		Mode:  heatmap.ModeCount,
		Scope: search.ScopeNT,
	})

	if !near(r.Books[0].Score, 1) || !near(r.Books[1].Score, 0.5) {
		t.Fatalf("count scores = %v/%v", r.Books[0].Score, r.Books[1].Score)
	}
	if r.Books[0].InScope || !r.Books[1].InScope {
		t.Fatalf("scope flags = %v/%v", r.Books[0].InScope, r.Books[1].InScope)
	}
	if r.Mode != heatmap.ModeCount || r.Mode.Label() != "Count" {
		t.Fatalf("mode = %v/%q", r.Mode, r.Mode.Label())
	}
	if heatmap.ModeDensity.Label() != "Density" {
		t.Fatal("density mode has an unexpected label")
	}
}

func TestBuildBookScopeAndEmpty(t *testing.T) {
	b := testBible()
	r := heatmap.Build(b, []search.Hit{{Verse: 3, Score: 1}}, heatmap.Options{
		Scope:  search.ScopeBook,
		BookID: 2,
	})
	if r.Books[0].InScope || !r.Books[1].InScope {
		t.Fatalf("book scope flags = %v/%v", r.Books[0].InScope, r.Books[1].InScope)
	}

	empty := heatmap.Build(b, nil, heatmap.Options{})
	if empty.Total != 0 || empty.MaxMatches != 0 {
		t.Fatalf("empty report = %+v", empty)
	}
	for i := range empty.Books {
		if empty.Books[i].Score != 0 {
			t.Fatalf("empty book %s score = %v", empty.Books[i].Name, empty.Books[i].Score)
		}
	}
	if nilReport := heatmap.Build(nil, nil, heatmap.Options{}); nilReport == nil || len(nilReport.Books) != 0 {
		t.Fatalf("nil corpus report = %+v", nilReport)
	}
}

func TestBucket(t *testing.T) {
	chapters := []heatmap.Chapter{{Score: 1}, {Score: 0}, {Score: 0.5}}
	got := heatmap.Bucket(chapters, 4)
	want := []float64{1, 1, 0, 0.5}
	if len(got) != len(want) {
		t.Fatalf("bucket length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if !near(got[i], want[i]) {
			t.Fatalf("bucket %d = %v, want %v", i, got[i], want[i])
		}
	}
	if pooled := heatmap.Bucket(chapters, 2); !near(pooled[0], 1) || !near(pooled[1], 0.5) {
		t.Fatalf("max-pooled buckets = %v", pooled)
	}
	if heatmap.Bucket(chapters, 0) != nil || heatmap.Bucket(nil, 3) != nil {
		t.Fatal("empty bucket requests should return nil")
	}
}
