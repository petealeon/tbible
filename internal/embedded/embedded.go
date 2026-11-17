// Package embedded exposes the compiled-in KJV corpus and search index.
// Data files are produced by cmd/tbgen.
package embedded

import (
	_ "embed"
	"sync"

	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/search"
)

//go:embed data/kjv.tbc
var corpusBlob []byte

//go:embed data/kjv.tbi
var indexBlob []byte

var (
	bibleOnce sync.Once
	bible     *corpus.Bible
	bibleErr  error

	indexOnce sync.Once
	index     *search.Index
	indexErr  error
)

// Bible returns the embedded KJV corpus. The result is shared and read-only.
func Bible() (*corpus.Bible, error) {
	bibleOnce.Do(func() {
		// The embedded blob is static and process-lifetime, so borrowed views
		// avoid copying ~4.3 MB of verse text into the heap.
		bible, bibleErr = corpus.OpenBorrowed(corpusBlob)
	})
	return bible, bibleErr
}

// Index returns the embedded search index. The result is shared and read-only.
func Index() (*search.Index, error) {
	indexOnce.Do(func() {
		index, indexErr = search.OpenBorrowed(indexBlob)
	})
	return index, indexErr
}

// Load returns both embedded artefacts after checking that the index matches
// the corpus. Callers that need both should prefer it to separate calls.
func Load() (*corpus.Bible, *search.Index, error) {
	b, err := Bible()
	if err != nil {
		return nil, nil, err
	}
	ix, err := Index()
	if err != nil {
		return nil, nil, err
	}
	if err := ix.CheckCorpus(b); err != nil {
		return nil, nil, err
	}
	return b, ix, nil
}

// CorpusBytes exposes the raw embedded corpus for verification tools.
func CorpusBytes() []byte { return corpusBlob }

// IndexBytes exposes the raw embedded index for verification tools.
func IndexBytes() []byte { return indexBlob }
