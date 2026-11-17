package embedded

import (
	"strings"
	"testing"
	"time"

	"github.com/petealeon/tbible/v2/internal/search"
)

func TestEmbeddedCorpus(t *testing.T) {
	b, err := Bible()
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Verses) != 31102 {
		t.Fatalf("verses = %d", len(b.Verses))
	}
	v := b.VerseAt(43, 3, 16)
	if v == nil || !strings.HasPrefix(v.Text, "For God so loved") {
		t.Fatalf("John 3:16 = %+v", v)
	}
	// Styles survive the round trip.
	v = b.VerseAt(40, 5, 3)
	if len(v.RedLetter()) == 0 {
		t.Error("Matthew 5:3 lost red-letter spans")
	}
}

func TestEmbeddedSearch(t *testing.T) {
	b, err := Bible()
	if err != nil {
		t.Fatal(err)
	}
	ix, err := Index()
	if err != nil {
		t.Fatal(err)
	}
	text := func(i int) string { return b.Verses[i].Text }
	label := func(i int) string {
		v := &b.Verses[i]
		return b.Books[v.BookID-1].Name + " " + itoa(int(v.Chapter)) + ":" + itoa(int(v.Number))
	}

	cases := []struct {
		q    string
		want string
	}{
		{"shepherd", "Psalms 23:1"},
		{`"still waters"`, "Psalms 23:2"},
		{"caesar", "Matthew 22:21"},
		{"faith", "Romans 1:17"},
		{"shep*", "Psalms 23:1"},
		{"For God so loved", "John 3:16"},
	}
	for _, c := range cases {
		r := ix.Search(search.ParseQuery(c.q), text, search.Options{Limit: 20})
		if len(r.Hits) == 0 {
			t.Errorf("%q: no hits", c.q)
			continue
		}
		found := false
		for _, h := range r.Hits {
			if label(h.Verse) == c.want {
				found = true
				break
			}
		}
		if !found {
			labels := make([]string, 0, 8)
			for _, h := range r.Hits[:min(8, len(r.Hits))] {
				labels = append(labels, label(h.Verse))
			}
			t.Errorf("%q: %s not in %v", c.q, c.want, labels)
		}
	}

	// Scope restricts by testament.
	r := ix.Search(search.ParseQuery("shepherd"), text, search.Options{Limit: 50, Scope: search.ScopeNT})
	if len(r.Hits) > 0 && r.Hits[0].Verse < 30000 {
		t.Logf("NT search first hit: %s", label(r.Hits[0].Verse))
	}
	for _, h := range r.Hits {
		if b.Verses[h.Verse].BookID < 40 {
			t.Errorf("NT scope returned OT verse %s", label(h.Verse))
		}
	}

	// Exclusions remove verses containing the excluded term.
	r = ix.Search(search.ParseQuery("love -world"), text, search.Options{Limit: 50})
	for _, h := range r.Hits {
		if strings.Contains(strings.ToLower(b.Verses[h.Verse].Text), "world") {
			t.Errorf("exclusion failed: %s", label(h.Verse))
		}
	}

	// A rough performance check: 100 representative searches.
	start := time.Now()
	for i := 0; i < 100; i++ {
		ix.Search(search.ParseQuery([]string{"faith", "love", "shepherd", "grace", "salvation"}[i%5]), text, search.Options{Limit: 30})
	}
	d := time.Since(start)
	t.Logf("100 searches: %s (%s/search)", d, d/100)
	if d > 2*time.Second {
		t.Errorf("search too slow: %s", d)
	}
}

func TestEmbeddedStartup(t *testing.T) {
	start := time.Now()
	if _, err := Bible(); err != nil {
		t.Fatal(err)
	}
	if _, err := Index(); err != nil {
		t.Fatal(err)
	}
	d := time.Since(start)
	t.Logf("cold load: %s", d)
	if d > 500*time.Millisecond {
		t.Errorf("cold load too slow: %s", d)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
