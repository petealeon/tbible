package store_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/petealeon/tbible/v2/internal/store"
)

func tempState(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "state.json")
}

func writeState(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissingAndEmpty(t *testing.T) {
	path := tempState(t)
	st, err := store.LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Warning() != "" {
		t.Fatalf("missing file warning = %q", st.Warning())
	}
	if !st.Settings().VerseNumbers || st.Settings().CopyFormat != "ref-text" {
		t.Fatalf("defaults = %+v", st.Settings())
	}

	writeState(t, path, "  \n")
	st, err = store.LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Warning() != "" {
		t.Fatalf("empty file warning = %q", st.Warning())
	}
}

func TestCorruptStateIsBackedUp(t *testing.T) {
	path := tempState(t)
	writeState(t, path, "{not json")

	st, err := store.LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Warning() == "" {
		t.Fatal("corrupt state produced no warning")
	}
	backup := path + ".corrupt"
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if got := st.Snapshot(); len(got.Bookmarks) != 0 || got.Version != 1 {
		t.Fatalf("recovered state = %+v", got)
	}

	// Saving replaces the working file without touching the backup.
	st.TouchHistory(store.VerseItem{Key: "43:3:16", Ref: "John 3:16"})
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Warning() != "" {
		t.Fatalf("reloaded warning = %q", reloaded.Warning())
	}
	if got := reloaded.History(); len(got) != 1 || got[0].Key != "43:3:16" {
		t.Fatalf("reloaded history = %+v", got)
	}

	// A second corrupt file gets a numbered backup instead of overwriting.
	writeState(t, path, "{also bad")
	if _, err := store.LoadPath(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backup + ".1"); err != nil {
		t.Fatalf("numbered backup missing: %v", err)
	}
}

func TestFutureVersionIsBackedUp(t *testing.T) {
	path := tempState(t)
	writeState(t, path, `{"version":99,"bookmarks":[{"key":"43:3:16"}]}`)

	st, err := store.LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Warning() == "" {
		t.Fatal("future version produced no warning")
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if got := st.Bookmarks(); len(got) != 0 {
		t.Fatalf("future state leaked into memory: %+v", got)
	}
}

func TestLegacyAndNullState(t *testing.T) {
	path := tempState(t)
	writeState(t, path, `{
		"bookmarks": null,
		"notes": null,
		"history": [{"key":"43:3:16","ref":"John 3:16"}],
		"saved_searches": null,
		"settings": {}
	}`)

	st, err := store.LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Warning() != "" {
		t.Fatalf("legacy state warning = %q", st.Warning())
	}
	snap := st.Snapshot()
	if snap.Version != 1 {
		t.Fatalf("version = %d", snap.Version)
	}
	if snap.Settings.CopyFormat != "ref-text" {
		t.Fatalf("copy format = %q", snap.Settings.CopyFormat)
	}
	if len(snap.History) != 1 || snap.History[0].Key != "43:3:16" {
		t.Fatalf("history = %+v", snap.History)
	}

	// Saving a loaded null document must emit arrays, not null.
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"bookmarks", "notes", "saved_searches"} {
		if got := string(raw[key]); got != "[]" {
			t.Errorf("%s = %s, want []", key, got)
		}
	}
}

func TestSavedSearchScopes(t *testing.T) {
	st, err := store.LoadPath(tempState(t))
	if err != nil {
		t.Fatal(err)
	}
	add := func(q, scope string, book uint16) {
		t.Helper()
		if err := st.AddSavedSearchScoped(q, scope, book); err != nil {
			t.Fatal(err)
		}
	}
	add("faith", store.ScopeAll, 0)
	add("faith", store.ScopeNT, 0)
	add("faith", store.ScopeOT, 0)
	add("faith", store.ScopeBook, 43)
	add("faith", store.ScopeBook, 0) // missing book normalises to all and dedupes

	got := st.SavedSearches()
	if len(got) != 4 {
		t.Fatalf("saved = %+v", got)
	}
	if got[0].Scope != store.ScopeAll || got[0].BookID != 0 {
		t.Fatalf("newest = %+v", got[0])
	}

	// Re-adding the same identity moves it to the front instead of duplicating.
	add("faith", store.ScopeNT, 0)
	got = st.SavedSearches()
	if len(got) != 4 || got[0].Scope != store.ScopeNT {
		t.Fatalf("after dedupe = %+v", got)
	}

	if err := st.RemoveSavedSearchScoped("faith", store.ScopeNT, 0); err != nil {
		t.Fatal(err)
	}
	got = st.SavedSearches()
	if len(got) != 3 {
		t.Fatalf("after scoped delete = %+v", got)
	}
	for _, ss := range got {
		if ss.Scope == store.ScopeNT {
			t.Fatalf("NT search survived: %+v", got)
		}
	}

	// Legacy unscoped deletion removes every matching query.
	if err := st.RemoveSavedSearch("faith"); err != nil {
		t.Fatal(err)
	}
	if got := st.SavedSearches(); len(got) != 0 {
		t.Fatalf("after delete = %+v", got)
	}
}

func TestSavedSearchScopeNormalisationOnLoad(t *testing.T) {
	path := tempState(t)
	writeState(t, path, `{
		"version": 1,
		"saved_searches": [
			{"query":"faith","scope":"bogus","book_id":9},
			{"query":"hope","scope":"book","book_id":0},
			{"query":"love","scope":"nt"}
		]
	}`)
	st, err := store.LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	got := st.SavedSearches()
	if len(got) != 3 {
		t.Fatalf("saved = %+v", got)
	}
	if got[0].Scope != store.ScopeAll || got[0].BookID != 0 {
		t.Fatalf("bogus scope = %+v", got[0])
	}
	if got[1].Scope != store.ScopeAll || got[1].BookID != 0 {
		t.Fatalf("empty book scope = %+v", got[1])
	}
	if got[2].Scope != store.ScopeNT {
		t.Fatalf("NT scope = %+v", got[2])
	}
}

func TestBulkBookmarks(t *testing.T) {
	st, err := store.LoadPath(tempState(t))
	if err != nil {
		t.Fatal(err)
	}
	items := []store.VerseItem{
		{Key: "43:3:16", Ref: "John 3:16"},
		{Key: "43:3:17", Ref: "John 3:17"},
		{Key: "43:3:18", Ref: "John 3:18"},
	}
	added, err := st.ToggleBookmarks(items)
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatal("bulk add reported removal")
	}
	got := st.Bookmarks()
	if len(got) != 3 {
		t.Fatalf("bookmarks = %+v", got)
	}
	if got[0].Key != "43:3:16" || got[2].Key != "43:3:18" {
		t.Fatalf("bookmark order = %+v", got)
	}
	for _, it := range items {
		if !st.IsBookmarked(it.Key) {
			t.Fatalf("%s not bookmarked", it.Key)
		}
	}

	// A partly-bookmarked set adds only the missing entries.
	extra := []store.VerseItem{{Key: "43:3:17"}, {Key: "43:3:19"}}
	if added, err := st.ToggleBookmarks(extra); err != nil || !added {
		t.Fatalf("partial add = %v, %v", added, err)
	}
	if got := st.Bookmarks(); len(got) != 4 {
		t.Fatalf("after partial add = %+v", got)
	}

	if added, err := st.ToggleBookmarks(items); err != nil || added {
		t.Fatalf("bulk remove = %v, %v", added, err)
	}
	if got := st.Bookmarks(); len(got) != 1 || got[0].Key != "43:3:19" {
		t.Fatalf("after remove = %+v", got)
	}

	if added, err := st.ToggleBookmarks(nil); err != nil || added {
		t.Fatalf("empty toggle = %v, %v", added, err)
	}
}

func TestTouchThenSave(t *testing.T) {
	path := tempState(t)
	st, err := store.LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	st.TouchHistory(store.VerseItem{Key: "43:3:16", Ref: "John 3:16"})
	st.TouchLastRead(store.VerseItem{Key: "45:8:28", Ref: "Romans 8:28"})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("touch wrote the state file: %v", err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := store.LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.History(); len(got) != 1 || got[0].Key != "43:3:16" {
		t.Fatalf("history = %+v", got)
	}
	last, ok := reloaded.LastRead()
	if !ok || last.Key != "45:8:28" {
		t.Fatalf("last read = %+v, %v", last, ok)
	}
}

func TestHistoryDeduplicatesAndCaps(t *testing.T) {
	st, err := store.LoadPath(tempState(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddHistory(store.VerseItem{Key: "43:3:16"}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddHistory(store.VerseItem{Key: "43:3:16"}); err != nil {
		t.Fatal(err)
	}
	if got := st.History(); len(got) != 1 {
		t.Fatalf("dedupe history = %+v", got)
	}
	for i := 0; i < 105; i++ {
		st.TouchHistory(store.VerseItem{Key: string(rune('a'+i%26)) + string(rune('0'+i/26))})
	}
	if got := st.History(); len(got) != 100 {
		t.Fatalf("history cap = %d, want 100", len(got))
	}
}

func TestStateJSONShape(t *testing.T) {
	path := tempState(t)
	st, err := store.LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddSavedSearchScoped("faith", store.ScopeNT, 0); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"version", "bookmarks", "notes", "history", "saved_searches", "last_read", "settings"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("state file is missing %q", key)
		}
	}
}

func TestDefaultPathHonoursEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TBIBLE_STATE_DIR", dir)
	got, err := store.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "state.json"); got != want {
		t.Fatalf("DefaultPath = %q, want %q", got, want)
	}
}
