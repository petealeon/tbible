// Package library resolves translations: the embedded KJV plus any installed
// packs. Loaded corpora and indexes are kept in a small LRU cache because a
// decoded pack is several times larger than its compressed file.
package library

import (
	"errors"
	"fmt"
	"sync"

	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/embedded"
	"github.com/petealeon/tbible/v2/internal/packs"
	"github.com/petealeon/tbible/v2/internal/search"
)

const (
	KJVId   = "kjv"
	KJVName = "King James Version"
	// maxCached bounds resident memory to roughly four translations.
	maxCached = 4
)

// Translation describes one selectable Bible.
type Translation struct {
	ID   string
	Name string
	KJV  bool
}

type loaded struct {
	bible *corpus.Bible
	index *search.Index
}

// Library owns the pack directory and the LRU cache.
type Library struct {
	mu    sync.Mutex
	dir   string
	cache map[string]*loaded
	order []string
}

// New creates a library rooted at dir.
func New(dir string) *Library {
	return &Library{dir: dir, cache: make(map[string]*loaded)}
}

// Default creates a library in the platform pack directory.
func Default() (*Library, error) {
	dir, err := packs.Dir()
	if err != nil {
		return nil, err
	}
	return New(dir), nil
}

// Dir returns the pack directory.
func (l *Library) Dir() string { return l.dir }

// Available lists the embedded KJV followed by installed packs.
func (l *Library) Available() ([]Translation, error) {
	out := []Translation{{ID: KJVId, Name: KJVName, KJV: true}}
	ids, err := packs.Installed(l.dir)
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		if id == KJVId {
			continue
		}
		m, err := packs.LoadManifest(l.dir, id)
		if err != nil {
			continue
		}
		out = append(out, Translation{ID: id, Name: m.Name})
	}
	return out, nil
}

// Get returns the corpus and index for id. The KJV is served from the embedded
// blobs; other translations are decoded from their packs and cached.
func (l *Library) Get(id string) (*corpus.Bible, *search.Index, error) {
	if id == "" || id == KJVId {
		return embedded.Load()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if item, ok := l.cache[id]; ok {
		l.touch(id)
		return item.bible, item.index, nil
	}
	c, err := packs.Load(l.dir, id)
	if err != nil {
		return nil, nil, fmt.Errorf("translation %q: %w", id, err)
	}
	bible, err := corpus.Open(c.Corpus)
	if err != nil {
		return nil, nil, fmt.Errorf("translation %q corpus: %w", id, err)
	}
	index, err := search.Open(c.Index)
	if err != nil {
		return nil, nil, fmt.Errorf("translation %q index: %w", id, err)
	}
	l.cache[id] = &loaded{bible: bible, index: index}
	l.order = append(l.order, id)
	l.evict()
	return bible, index, nil
}

func (l *Library) touch(id string) {
	for i, v := range l.order {
		if v == id {
			copy(l.order[i:], l.order[i+1:])
			l.order[len(l.order)-1] = id
			return
		}
	}
	l.order = append(l.order, id)
}

func (l *Library) evict() {
	for len(l.order) > maxCached {
		id := l.order[0]
		l.order = l.order[1:]
		delete(l.cache, id)
	}
}

// Invalidate drops a translation from the cache, e.g. after reinstalling it.
func (l *Library) Invalidate(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.cache, id)
	for i, v := range l.order {
		if v == id {
			l.order = append(l.order[:i], l.order[i+1:]...)
			return
		}
	}
}

// Name returns a human-readable name for an installed translation.
func (l *Library) Name(id string) string {
	if id == "" || id == KJVId {
		return KJVName
	}
	m, err := packs.LoadManifest(l.dir, id)
	if err == nil && m.Name != "" {
		return m.Name
	}
	return id
}

// Has reports whether a translation can currently be opened.
func (l *Library) Has(id string) bool {
	if id == "" || id == KJVId {
		return true
	}
	_, err := packs.LoadManifest(l.dir, id)
	return err == nil
}

// ErrUnknown is returned for ids that are neither embedded nor installed.
var ErrUnknown = errors.New("translation is not installed")
