// Package store persists user data (bookmarks, notes, history, settings) as
// a small JSON file. It deliberately has no dependency on SQLite so the app
// stays a single static binary.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// stateVersion is the on-disk schema version. Version 1 is the only schema
// this build reads; unrecognised files are backed up rather than overwritten.
const stateVersion = 1

// Saved-search scopes. These are persisted as short strings so the file stays
// readable and stable if the internal search enum changes.
const (
	ScopeAll  = "all"
	ScopeOT   = "ot"
	ScopeNT   = "nt"
	ScopeBook = "book"
)

// VerseItem is a saved location.
type VerseItem struct {
	Key  string    `json:"key"`
	Ref  string    `json:"ref"`
	Text string    `json:"text,omitempty"`
	At   time.Time `json:"at"`
}

// Note is a user note attached to a verse.
type Note struct {
	Key  string    `json:"key"`
	Ref  string    `json:"ref"`
	Body string    `json:"body"`
	At   time.Time `json:"at"`
}

// SavedSearch is a named query with the scope it was saved under.
type SavedSearch struct {
	Query  string    `json:"query"`
	Scope  string    `json:"scope,omitempty"`
	BookID uint16    `json:"book_id,omitempty"`
	At     time.Time `json:"at"`
}

// Same reports whether ss identifies the same query and scope.
func (ss SavedSearch) Same(query, scope string, bookID uint16) bool {
	return ss.Query == query && ss.Scope == scope && ss.BookID == bookID
}

// Settings holds persistent display preferences.
type Settings struct {
	VerseNumbers bool   `json:"verse_numbers"`
	Footnotes    bool   `json:"footnotes"`
	CopyFormat   string `json:"copy_format"`
}

// State is the complete on-disk document.
type State struct {
	Version       int           `json:"version"`
	Bookmarks     []VerseItem   `json:"bookmarks"`
	Notes         []Note        `json:"notes"`
	History       []VerseItem   `json:"history"`
	SavedSearches []SavedSearch `json:"saved_searches"`
	LastRead      VerseItem     `json:"last_read"`
	Settings      Settings      `json:"settings"`
}

// Store is a loaded state document.
type Store struct {
	mu      sync.Mutex
	path    string
	state   State
	warning string
}

// DefaultPath returns the platform configuration path for state.json.
func DefaultPath() (string, error) {
	if dir := os.Getenv("TBIBLE_STATE_DIR"); dir != "" {
		return filepath.Join(dir, "state.json"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", fmt.Errorf("locate config directory: %w", err)
		}
		dir = filepath.Join(home, ".tbible")
	}
	return filepath.Join(dir, "tbible", "state.json"), nil
}

// Load reads the state file, returning an empty store when it does not exist.
func Load() (*Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return LoadPath(path)
}

// LoadPath reads a specific state file. A missing or empty file yields an
// empty store. A corrupt or future-version file is moved aside and an empty
// store is returned with Warning set, so a bad file never blocks startup or
// gets silently overwritten.
func LoadPath(path string) (*Store, error) {
	s := &Store{path: path}
	s.resetLocked()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return s, nil
	}

	var loaded State
	if err := json.Unmarshal(data, &loaded); err != nil {
		s.recoverCorrupt(fmt.Sprintf("state file is unreadable: %v", err))
		return s, nil
	}
	if loaded.Version == 0 {
		// Files written before the version field existed are schema 1.
		loaded.Version = stateVersion
	}
	if loaded.Version != stateVersion {
		s.recoverCorrupt(fmt.Sprintf("state file has unsupported version %d", loaded.Version))
		return s, nil
	}
	if loaded.Settings.CopyFormat == "" {
		loaded.Settings.CopyFormat = "ref-text"
	}
	s.state = loaded
	// Unmarshalling a document with null arrays leaves nil slices; callers
	// append to them safely, but initialise for predictable JSON output.
	if s.state.Bookmarks == nil {
		s.state.Bookmarks = []VerseItem{}
	}
	if s.state.Notes == nil {
		s.state.Notes = []Note{}
	}
	if s.state.History == nil {
		s.state.History = []VerseItem{}
	}
	if s.state.SavedSearches == nil {
		s.state.SavedSearches = []SavedSearch{}
	}
	for i := range s.state.SavedSearches {
		scope, bookID := normalizeScope(s.state.SavedSearches[i].Scope, s.state.SavedSearches[i].BookID)
		s.state.SavedSearches[i].Scope = scope
		s.state.SavedSearches[i].BookID = bookID
	}
	return s, nil
}

func (s *Store) resetLocked() {
	s.state = State{
		Version:  stateVersion,
		Settings: Settings{VerseNumbers: true, CopyFormat: "ref-text"},
	}
	s.warning = ""
}

// recoverCorrupt moves the current file aside and resets to defaults. A
// failure to back up is reported in the warning instead of failing startup.
func (s *Store) recoverCorrupt(reason string) {
	backup, err := s.backupCorrupt()
	s.resetLocked()
	if err != nil {
		s.warning = fmt.Sprintf("%s; starting with an empty library (could not back up: %v)", reason, err)
		return
	}
	s.warning = fmt.Sprintf("%s; starting with an empty library (backup saved as %s)", reason, backup)
}

func (s *Store) backupCorrupt() (string, error) {
	base := s.path + ".corrupt"
	dst := base
	for i := 1; i < 1000; i++ {
		if _, err := os.Stat(dst); errors.Is(err, os.ErrNotExist) {
			break
		}
		dst = fmt.Sprintf("%s.%d", base, i)
	}
	if err := os.Rename(s.path, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// Warning returns a user-facing message when state had to be recovered.
func (s *Store) Warning() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.warning
}

// Path returns the backing file path.
func (s *Store) Path() string { return s.path }

// Save writes the state atomically.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(&s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Snapshot returns a copy of the state for display.
func (s *Store) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.state
	out.Bookmarks = append([]VerseItem(nil), s.state.Bookmarks...)
	out.Notes = append([]Note(nil), s.state.Notes...)
	out.History = append([]VerseItem(nil), s.state.History...)
	out.SavedSearches = append([]SavedSearch(nil), s.state.SavedSearches...)
	return out
}

// Bookmarks returns bookmarks, newest first.
func (s *Store) Bookmarks() []VerseItem { return s.Snapshot().Bookmarks }

// Notes returns notes, newest first.
func (s *Store) Notes() []Note { return s.Snapshot().Notes }

// History returns reading history, newest first.
func (s *Store) History() []VerseItem { return s.Snapshot().History }

// SavedSearches returns saved searches, newest first.
func (s *Store) SavedSearches() []SavedSearch { return s.Snapshot().SavedSearches }

// Settings returns current settings.
func (s *Store) Settings() Settings { return s.Snapshot().Settings }

// SetSettings replaces display preferences.
func (s *Store) SetSettings(v Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Settings = v
	return s.saveLocked()
}

// ToggleBookmark adds or removes a bookmark and reports whether it now exists.
func (s *Store) ToggleBookmark(v VerseItem) (bool, error) {
	return s.ToggleBookmarks([]VerseItem{v})
}

// ToggleBookmarks adds every item when at least one is not bookmarked, and
// removes them all when every item is already bookmarked. It reports whether
// the items now exist.
func (s *Store) ToggleBookmarks(items []VerseItem) (bool, error) {
	if len(items) == 0 {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	add := false
	for _, it := range items {
		if !s.bookmarkedLocked(it.Key) {
			add = true
			break
		}
	}
	if add {
		// Insert in reverse so the first item ends up newest.
		for i := len(items) - 1; i >= 0; i-- {
			it := items[i]
			if s.bookmarkedLocked(it.Key) {
				continue
			}
			if it.At.IsZero() {
				it.At = time.Now()
			}
			s.state.Bookmarks = append([]VerseItem{it}, s.state.Bookmarks...)
		}
		return true, s.saveLocked()
	}

	remove := make(map[string]struct{}, len(items))
	for _, it := range items {
		remove[it.Key] = struct{}{}
	}
	out := s.state.Bookmarks[:0]
	for _, b := range s.state.Bookmarks {
		if _, ok := remove[b.Key]; !ok {
			out = append(out, b)
		}
	}
	s.state.Bookmarks = out
	return false, s.saveLocked()
}

func (s *Store) bookmarkedLocked(key string) bool {
	for _, b := range s.state.Bookmarks {
		if b.Key == key {
			return true
		}
	}
	return false
}

// IsBookmarked reports whether key is bookmarked.
func (s *Store) IsBookmarked(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bookmarkedLocked(key)
}

// TouchHistory updates history and last-read in memory without saving. Callers
// that record many locations in quick succession can debounce Save.
func (s *Store) TouchHistory(v VerseItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addHistoryLocked(v)
}

func (s *Store) addHistoryLocked(v VerseItem) {
	out := s.state.History[:0]
	for _, h := range s.state.History {
		if h.Key != v.Key {
			out = append(out, h)
		}
	}
	v.At = time.Now()
	s.state.History = append([]VerseItem{v}, out...)
	if len(s.state.History) > 100 {
		s.state.History = s.state.History[:100]
	}
	s.state.LastRead = v
}

// AddHistory records a location, deduplicating by key, and saves immediately.
func (s *Store) AddHistory(v VerseItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addHistoryLocked(v)
	return s.saveLocked()
}

// TouchLastRead updates the reading position in memory without saving.
func (s *Store) TouchLastRead(v VerseItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.LastRead = v
}

// SetLastRead records the current reading position without touching history.
func (s *Store) SetLastRead(v VerseItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.LastRead = v
	return s.saveLocked()
}

// LastRead returns the most recent location.
func (s *Store) LastRead() (VerseItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.LastRead, s.state.LastRead.Key != ""
}

// Note returns the note for key.
func (s *Store) Note(key string) (Note, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range s.state.Notes {
		if n.Key == key {
			return n, true
		}
	}
	return Note{}, false
}

// SetNote creates, updates, or deletes a note when body is empty.
func (s *Store) SetNote(key, ref, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.state.Notes[:0]
	for _, n := range s.state.Notes {
		if n.Key != key {
			out = append(out, n)
		}
	}
	s.state.Notes = out
	if body != "" {
		s.state.Notes = append([]Note{{Key: key, Ref: ref, Body: body, At: time.Now()}}, s.state.Notes...)
	}
	return s.saveLocked()
}

// AddSavedSearch records a query under the global scope, deduplicating.
func (s *Store) AddSavedSearch(q string) error {
	return s.AddSavedSearchScoped(q, ScopeAll, 0)
}

// AddSavedSearchScoped records a query and the scope it was run under,
// deduplicating on the full identity so the same words can be saved once per
// scope.
func (s *Store) AddSavedSearchScoped(q, scope string, bookID uint16) error {
	scope, bookID = normalizeScope(scope, bookID)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.state.SavedSearches[:0]
	for _, ss := range s.state.SavedSearches {
		if !ss.Same(q, scope, bookID) {
			out = append(out, ss)
		}
	}
	entry := SavedSearch{Query: q, Scope: scope, BookID: bookID, At: time.Now()}
	s.state.SavedSearches = append([]SavedSearch{entry}, out...)
	if len(s.state.SavedSearches) > 50 {
		s.state.SavedSearches = s.state.SavedSearches[:50]
	}
	return s.saveLocked()
}

func normalizeScope(scope string, bookID uint16) (string, uint16) {
	switch scope {
	case ScopeOT, ScopeNT, ScopeBook:
		if scope == ScopeBook && bookID == 0 {
			return ScopeAll, 0
		}
		return scope, bookID
	case ScopeAll, "":
		return ScopeAll, 0
	default:
		return ScopeAll, 0
	}
}

// RemoveSavedSearch deletes every saved query matching q, regardless of scope.
func (s *Store) RemoveSavedSearch(q string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.state.SavedSearches[:0]
	for _, ss := range s.state.SavedSearches {
		if ss.Query != q {
			out = append(out, ss)
		}
	}
	s.state.SavedSearches = out
	return s.saveLocked()
}

// RemoveSavedSearchScoped deletes one scoped saved query.
func (s *Store) RemoveSavedSearchScoped(q, scope string, bookID uint16) error {
	scope, bookID = normalizeScope(scope, bookID)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.state.SavedSearches[:0]
	for _, ss := range s.state.SavedSearches {
		if !ss.Same(q, scope, bookID) {
			out = append(out, ss)
		}
	}
	s.state.SavedSearches = out
	return s.saveLocked()
}
