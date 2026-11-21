package library

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/petealeon/tbible/v2/internal/embedded"
	"github.com/petealeon/tbible/v2/internal/packs"
)

func testPackData(t *testing.T, id string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := packs.Write(&buf, packs.Content{
		Manifest: packs.Manifest{ID: id, Name: id + " translation", Language: "English"},
		Corpus:   embedded.CorpusBytes(),
		Index:    embedded.IndexBytes(),
	}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func installTestPack(t *testing.T, dir, id string) {
	t.Helper()
	installPackData(t, dir, id, testPackData(t, id))
}

func installPackData(t *testing.T, dir, id string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packs.Path(dir, id), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAvailableGetAndName(t *testing.T) {
	dir := t.TempDir()
	installTestPack(t, dir, "test")
	l := New(dir)

	available, err := l.Available()
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 2 || available[0].ID != KJVId || !available[0].KJV || available[1].ID != "test" {
		t.Fatalf("Available = %+v", available)
	}
	b, ix, err := l.Get("test")
	if err != nil || b == nil || ix == nil {
		t.Fatalf("Get(test) = %v, %v, %v", b, ix, err)
	}
	if err := ix.CheckCorpus(b); err != nil {
		t.Fatal(err)
	}
	if got := l.Name("test"); got != "test translation" {
		t.Fatalf("Name(test) = %q", got)
	}
	if !l.Has("test") || !l.Has(KJVId) || l.Has("missing") {
		t.Fatal("Has reported the wrong availability")
	}
	if _, _, err := l.Get("missing"); err == nil {
		t.Fatal("Get(missing) succeeded")
	}
}

func TestLRUCacheEvictsLeastRecentlyUsedPack(t *testing.T) {
	dir := t.TempDir()
	// One compressed payload is enough: the library keys its cache by the id
	// passed to Get, and the packs are data-only.
	data := testPackData(t, "a")
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		installPackData(t, dir, id, data)
	}
	l := New(dir)
	for _, id := range []string{"a", "b", "c", "d"} {
		if _, _, err := l.Get(id); err != nil {
			t.Fatal(err)
		}
	}
	// Make a the most recently used before adding a fifth translation.
	if _, _, err := l.Get("a"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Get("e"); err != nil {
		t.Fatal(err)
	}
	if len(l.cache) != maxCached || len(l.order) != maxCached {
		t.Fatalf("cache size = %d, order = %d", len(l.cache), len(l.order))
	}
	if _, ok := l.cache["b"]; ok {
		t.Fatal("least recently used pack was not evicted")
	}
	for _, id := range []string{"a", "c", "d", "e"} {
		if _, ok := l.cache[id]; !ok {
			t.Fatalf("recent pack %q was evicted: %s", id, fmt.Sprint(l.order))
		}
	}
	l.Invalidate("a")
	if _, ok := l.cache["a"]; ok {
		t.Fatal("Invalidate did not drop the cached pack")
	}
}
