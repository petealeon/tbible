package search_test

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/embedded"
	"github.com/petealeon/tbible/v2/internal/search"
)

func testBible() *corpus.Bible {
	return &corpus.Bible{Verses: []corpus.Verse{
		{BookID: 1, Chapter: 1, Number: 1, Text: "alpha gamma"},
		{BookID: 1, Chapter: 1, Number: 2, Text: "alpha delta"},
		{BookID: 2, Chapter: 1, Number: 1, Text: "beta epsilon"},
		{BookID: 2, Chapter: 1, Number: 2, Text: "zeta eta theta"},
		{BookID: 1, Chapter: 2, Number: 1, Text: "alpha beta gamma"},
	}}
}

func testIndex(t *testing.T) (*search.Index, func(int) string) {
	t.Helper()
	b := testBible()
	return search.Build(b), func(i int) string { return b.Verses[i].Text }
}

func TestTokenizeSpans(t *testing.T) {
	text := "Cæsar's wife"
	spans := search.TokenizeSpans(text)
	want := []string{"caesar", "s", "wife"}
	got := make([]string, len(spans))
	for i, sp := range spans {
		got[i] = sp.Text
		if sp.Start < 0 || sp.End <= sp.Start || sp.End > len(text) {
			t.Fatalf("span %d out of bounds: %+v", i, sp)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
	if source := text[spans[0].Start:spans[0].End]; source != "Cæsar" {
		t.Fatalf("first source span = %q, want Cæsar", source)
	}
	if plain := search.Tokenize(text); !reflect.DeepEqual(plain, want) {
		t.Fatalf("Tokenize = %v, want %v", plain, want)
	}
	if spans := search.TokenizeSpans("..."); len(spans) != 0 {
		t.Fatalf("punctuation tokenized to %+v", spans)
	}
}

func TestParseQuery(t *testing.T) {
	q := search.ParseQuery(`Love, "still waters" -world faith`)
	if !reflect.DeepEqual(q.Terms, []string{"love", "faith"}) {
		t.Errorf("terms = %v", q.Terms)
	}
	if !reflect.DeepEqual(q.Exclude, []string{"world"}) {
		t.Errorf("exclude = %v", q.Exclude)
	}
	if len(q.Phrases) != 1 || !reflect.DeepEqual(q.Phrases[0], []string{"still", "waters"}) {
		t.Errorf("phrases = %v", q.Phrases)
	}

	prefix := search.ParseQuery("shep*")
	if !reflect.DeepEqual(prefix.Terms, []string{"shep*"}) {
		t.Errorf("prefix terms = %v", prefix.Terms)
	}

	folded := search.ParseQuery(`"Cæsar's"`)
	if len(folded.Phrases) != 1 || !reflect.DeepEqual(folded.Phrases[0], []string{"caesar", "s"}) {
		t.Errorf("folded phrase = %v", folded.Phrases)
	}

	open := search.ParseQuery(`alpha "beta gamma`)
	if len(open.Phrases) != 1 || !reflect.DeepEqual(open.Phrases[0], []string{"beta", "gamma"}) {
		t.Errorf("unclosed phrase = %v", open.Phrases)
	}
}

func TestMatchesToken(t *testing.T) {
	q := search.ParseQuery(`shep* "still waters" -world`)
	for _, tok := range []string{"shepherd", "shepherds", "still", "waters"} {
		if !q.MatchesToken(tok) {
			t.Errorf("%q did not match", tok)
		}
	}
	for _, tok := range []string{"world", "good", ""} {
		if q.MatchesToken(tok) {
			t.Errorf("%q unexpectedly matched", tok)
		}
	}
}

func TestSearchSynthetic(t *testing.T) {
	ix, text := testIndex(t)

	res := ix.Search(search.ParseQuery("alpha"), text, search.Options{Limit: 10})
	if res.Total != 3 || len(res.Hits) != 3 || res.Relaxed {
		t.Fatalf("alpha: total=%d hits=%d relaxed=%v", res.Total, len(res.Hits), res.Relaxed)
	}
	if len(res.AllHits) != res.Total {
		t.Fatalf("alpha: all hits = %d, want %d", len(res.AllHits), res.Total)
	}

	// No verse contains both words, so the leave-one-out fallback reports the
	// verses matching either one.
	res = ix.Search(search.ParseQuery("alpha epsilon"), text, search.Options{Limit: 10})
	if !res.Relaxed || res.Total != 4 {
		t.Fatalf("relaxed: total=%d relaxed=%v", res.Total, res.Relaxed)
	}

	res = ix.Search(search.ParseQuery(`"alpha beta"`), text, search.Options{Limit: 10})
	if res.Total != 1 || res.Hits[0].Verse != 4 {
		t.Fatalf("phrase: total=%d hits=%+v", res.Total, res.Hits)
	}
	if res.Words == nil || !reflect.DeepEqual(res.Words, []string{"alpha", "beta"}) {
		t.Fatalf("phrase words = %v", res.Words)
	}

	res = ix.Search(search.ParseQuery("alpha -beta"), text, search.Options{Limit: 10})
	if res.Total != 2 {
		t.Fatalf("exclusion: total=%d hits=%+v", res.Total, res.Hits)
	}
	for _, h := range res.Hits {
		if h.Verse == 4 {
			t.Fatalf("exclusion kept verse %d", h.Verse)
		}
	}

	res = ix.Search(search.ParseQuery("alph*"), text, search.Options{Limit: 10})
	if res.Total != 3 {
		t.Fatalf("prefix: total=%d", res.Total)
	}

	// Scopes are applied before ranking and relaxation.
	res = ix.Search(search.ParseQuery("alpha epsilon"), text, search.Options{Scope: search.ScopeBook, BookID: 2, Limit: 10})
	if res.Total != 1 || res.Hits[0].Verse != 2 {
		t.Fatalf("scoped relaxed: total=%d hits=%+v", res.Total, res.Hits)
	}

	if res := ix.Search(search.ParseQuery("nope"), text, search.Options{Limit: 10}); res.Total != 0 {
		t.Fatalf("missing term matched %d verses", res.Total)
	}
	if res := ix.Search(search.Query{}, text, search.Options{Limit: 10}); res.Total != 0 {
		t.Fatalf("empty query matched %d verses", res.Total)
	}

	if got := ix.Suggest("al", 10); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("Suggest = %v", got)
	}
	if got := ix.Suggest("", 10); got != nil {
		t.Fatalf("empty prefix = %v", got)
	}
	if got := ix.Suggest("al", 0); got != nil {
		t.Fatalf("zero limit = %v", got)
	}
	if got := ix.Suggest("zz", 10); len(got) != 0 {
		t.Fatalf("unknown prefix = %v", got)
	}
}

func TestSearchKeepsAllHitsBeyondLimit(t *testing.T) {
	ix, text := testIndex(t)
	res := ix.Search(search.ParseQuery("alpha"), text, search.Options{Limit: 1})
	if len(res.Hits) != 1 {
		t.Fatalf("display hits = %d, want 1", len(res.Hits))
	}
	if len(res.AllHits) != res.Total || res.Total != 3 {
		t.Fatalf("all hits = %d, total = %d, want 3", len(res.AllHits), res.Total)
	}
	if res.AllHits[0] != res.Hits[0] {
		t.Fatalf("top hit = %+v, all hits[0] = %+v", res.Hits[0], res.AllHits[0])
	}
}

func TestIndexRoundTrip(t *testing.T) {
	b := testBible()
	ix := search.Build(b)
	var buf bytes.Buffer
	n, err := ix.Write(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if int(n) != buf.Len() {
		t.Fatalf("Write reported %d bytes, wrote %d", n, buf.Len())
	}

	got, err := search.Open(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got.Verses() != ix.Verses() || got.Tokens() != ix.Tokens() {
		t.Fatalf("round trip sizes: %d/%d vs %d/%d", got.Verses(), got.Tokens(), ix.Verses(), ix.Tokens())
	}
	text := func(i int) string { return b.Verses[i].Text }
	res := got.Search(search.ParseQuery("alpha"), text, search.Options{Limit: 10})
	if res.Total != 3 {
		t.Fatalf("round-trip search total = %d", res.Total)
	}

	if err := got.CheckCorpus(b); err != nil {
		t.Fatal(err)
	}
	if err := got.CheckCorpus(nil); err == nil {
		t.Fatal("CheckCorpus accepted a nil corpus")
	}
	other := testBible()
	other.Verses[0].BookID = 9
	if err := got.CheckCorpus(other); err == nil {
		t.Fatal("CheckCorpus accepted a mismatched corpus")
	}
}

func TestIndexWriteRoundTripAfterOpen(t *testing.T) {
	b := testBible()
	ix := search.Build(b)
	var original bytes.Buffer
	if _, err := ix.Write(&original); err != nil {
		t.Fatal(err)
	}
	data := append([]byte(nil), original.Bytes()...)

	for _, tc := range []struct {
		name string
		open func([]byte) (*search.Index, error)
	}{
		{"copied", search.Open},
		{"borrowed", search.OpenBorrowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opened, err := tc.open(data)
			if err != nil {
				t.Fatal(err)
			}
			var again bytes.Buffer
			if _, err := opened.Write(&again); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(again.Bytes(), data) {
				t.Fatalf("rewritten index differs from original (%d vs %d bytes)", again.Len(), len(data))
			}
		})
	}
}

func TestOpenBorrowedIndexMatchesOpen(t *testing.T) {
	b := testBible()
	ix := search.Build(b)
	var buf bytes.Buffer
	if _, err := ix.Write(&buf); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	copied, err := search.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	borrowed, err := search.OpenBorrowed(data)
	if err != nil {
		t.Fatal(err)
	}
	text := func(i int) string { return b.Verses[i].Text }
	for _, q := range []string{"alpha", "alph*", "alpha beta", "alpha -delta", `"alpha gamma"`} {
		want := copied.Search(search.ParseQuery(q), text, search.Options{Limit: 10})
		got := borrowed.Search(search.ParseQuery(q), text, search.Options{Limit: 10})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("query %q differs: got %+v, want %+v", q, got, want)
		}
	}
}

func BenchmarkEmbeddedSearch(b *testing.B) {
	bible, ix, err := embedded.Load()
	if err != nil {
		b.Fatal(err)
	}
	text := func(i int) string { return bible.Verses[i].Text }
	q := search.ParseQuery("shepherd")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ix.Search(q, text, search.Options{Limit: 25})
	}
}

func TestIndexRejectsCorruptData(t *testing.T) {
	ix := search.Build(testBible())
	var buf bytes.Buffer
	if _, err := ix.Write(&buf); err != nil {
		t.Fatal(err)
	}
	valid := buf.Bytes()
	corrupt := func(mutate func([]byte)) []byte {
		out := append([]byte(nil), valid...)
		mutate(out)
		return out
	}
	put16 := func(off int, v uint16) func([]byte) {
		return func(b []byte) { binary.LittleEndian.PutUint16(b[off:off+2], v) }
	}
	put32 := func(off int, v uint32) func([]byte) {
		return func(b []byte) { binary.LittleEndian.PutUint32(b[off:off+4], v) }
	}

	cases := []struct {
		name string
		data []byte
	}{
		{"nil", nil},
		{"short header", valid[:10]},
		{"bad magic", corrupt(func(b []byte) { copy(b[0:4], "NOPE") })},
		{"bad version", corrupt(put16(4, 99))},
		{"bad header size", corrupt(put16(6, 12))},
		{"no verses", corrupt(put32(16, 0))},
		{"token count mismatch", corrupt(put32(8, 99))},
		{"dictionary size mismatch", corrupt(put32(28, 3))},
		{"postings out of range", corrupt(put32(12, ^uint32(0)))},
		{"doc table wrong size", corrupt(put32(44, 2))},
		{"book table wrong size", corrupt(put32(52, 2))},
		{"token blob past end", corrupt(put32(20, ^uint32(0)))},
		{"dictionary offset past end", corrupt(put32(24, uint32(len(valid))+8))},
		{"postings offset past end", corrupt(put32(32, uint32(len(valid))+8))},
		{"doc offset past end", corrupt(put32(40, uint32(len(valid))+8))},
		{"book offset past end", corrupt(put32(48, uint32(len(valid))+8))},
		{"unaligned section", corrupt(put32(24, 9))},
		{"token total mismatch", corrupt(func(b []byte) {
			binary.LittleEndian.PutUint64(b[56:64], binary.LittleEndian.Uint64(b[56:64])+1)
		})},
		{"posting count mismatch", corrupt(func(b []byte) {
			dictOff := int(binary.LittleEndian.Uint32(b[24:28]))
			df := binary.LittleEndian.Uint32(b[dictOff+8 : dictOff+12])
			if df > 1 {
				df--
			} else {
				df++
			}
			binary.LittleEndian.PutUint32(b[dictOff+8:dictOff+12], df)
		})},
		{"bad posting frequency", corrupt(func(b []byte) {
			postOff := int(binary.LittleEndian.Uint32(b[32:36]))
			b[postOff+1] = 0
		})},
		{"truncated body", valid[:len(valid)-8]},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := search.Open(c.data)
			if err == nil {
				t.Fatalf("Open accepted corrupt index: %+v", got)
			}
		})
	}
}

func FuzzIndexOpen(f *testing.F) {
	ix := search.Build(testBible())
	var buf bytes.Buffer
	if _, err := ix.Write(&buf); err != nil {
		f.Fatal(err)
	}
	f.Add(buf.Bytes())
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		ix, err := search.Open(data)
		if err != nil {
			return
		}
		text := func(int) string { return "alpha beta" }
		res := ix.Search(search.ParseQuery("alpha"), text, search.Options{Limit: 5})
		if res.Total < 0 {
			t.Fatalf("negative total %d", res.Total)
		}
		for _, h := range res.Hits {
			if h.Verse < 0 || h.Verse >= ix.Verses() {
				t.Fatalf("hit %d outside %d verses", h.Verse, ix.Verses())
			}
		}
		for _, h := range res.AllHits {
			if h.Verse < 0 || h.Verse >= ix.Verses() {
				t.Fatalf("all hit %d outside %d verses", h.Verse, ix.Verses())
			}
		}
	})
}

func FuzzParseQuery(f *testing.F) {
	for _, seed := range []string{
		"", "faith", `"still waters"`, "shep*", "love -world",
		`"Cæsar's"`, "alpha beta gamma", `unclosed "phrase`, "-",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		q := search.ParseQuery(raw)
		for _, tok := range search.Tokenize(raw) {
			_ = q.MatchesToken(tok)
		}
	})
}
