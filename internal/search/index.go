package search

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sort"
	"unsafe"

	"github.com/petealeon/tbible/v2/internal/corpus"
)

const (
	indexMagic   = "TBI2"
	indexVersion = 1
	indexHeader  = 64
	dictRecord   = 20
	// Postings are varint pairs: delta verse id, term frequency.
)

type posting struct {
	verse uint32
	tf    uint16
}

// tokenEntry is kept deliberately small. Postings live either in parallel
// arrays (for freshly built indexes) or as byte ranges into postBytes (for
// indexes read from disk/embedded data). postLen is a count for arrays and a
// byte length for postBytes; docFreq is always the posting count.
type tokenEntry struct {
	token   string
	docFreq uint32
	postOff uint32
	postLen uint32
}

// Index is an immutable searched index.
type Index struct {
	tokens      []tokenEntry
	postVerses  []uint32
	postTFs     []uint16
	postBytes   []byte
	docLen      []uint16
	verseBook   []uint16
	totalTokens uint64
	avgLen      float64
	verseCount  int
}

func (ix *Index) postingAt(e *tokenEntry, i int) posting {
	off := int(e.postOff) + i
	return posting{verse: ix.postVerses[off], tf: ix.postTFs[off]}
}

// forEachPosting visits every posting for e in verse order. Opened indexes
// decode the compact varint section on demand instead of materialising ~3.7 MB
// of parallel arrays at startup.
func (ix *Index) forEachPosting(e *tokenEntry, fn func(posting)) {
	if ix.postBytes != nil {
		forEachPostingBytes(ix.postBytes[e.postOff:e.postOff+e.postLen], fn)
		return
	}
	for j := 0; j < int(e.docFreq); j++ {
		fn(ix.postingAt(e, j))
	}
}

// Build indexes every verse of b.
func Build(b *corpus.Bible) *Index {
	ix := &Index{
		docLen:     make([]uint16, len(b.Verses)),
		verseBook:  make([]uint16, len(b.Verses)),
		verseCount: len(b.Verses),
	}
	byToken := make(map[string][]posting, 20000)
	totalPostings := 0
	for i := range b.Verses {
		v := &b.Verses[i]
		toks := Tokenize(v.Text)
		if len(toks) > math.MaxUint16 {
			toks = toks[:math.MaxUint16]
		}
		ix.docLen[i] = uint16(len(toks))
		ix.verseBook[i] = v.BookID
		ix.totalTokens += uint64(len(toks))
		if len(toks) == 0 {
			continue
		}
		counts := make(map[string]uint16, len(toks))
		for _, t := range toks {
			counts[t]++
		}
		for t, n := range counts {
			byToken[t] = append(byToken[t], posting{verse: uint32(i), tf: n})
			totalPostings++
		}
	}
	keys := make([]string, 0, len(byToken))
	for k := range byToken {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ix.tokens = make([]tokenEntry, len(keys))
	ix.postVerses = make([]uint32, totalPostings)
	ix.postTFs = make([]uint16, totalPostings)
	off := 0
	for i, k := range keys {
		ps := byToken[k]
		ix.tokens[i] = tokenEntry{
			token:   k,
			docFreq: uint32(len(ps)),
			postOff: uint32(off),
			postLen: uint32(len(ps)),
		}
		for j, p := range ps {
			ix.postVerses[off+j] = p.verse
			ix.postTFs[off+j] = p.tf
		}
		off += len(ps)
	}
	if ix.verseCount > 0 {
		ix.avgLen = float64(ix.totalTokens) / float64(ix.verseCount)
	}
	return ix
}

// Tokens returns the number of distinct indexed terms.
func (ix *Index) Tokens() int { return len(ix.tokens) }

// Verses returns the number of indexed verses.
func (ix *Index) Verses() int { return ix.verseCount }

// CheckCorpus verifies that the index describes b verse for verse. It guards
// against an executable whose embedded corpus and index were built from
// different sources.
func (ix *Index) CheckCorpus(b *corpus.Bible) error {
	if b == nil {
		return fmt.Errorf("index: nil corpus")
	}
	if len(b.Verses) != ix.verseCount {
		return fmt.Errorf("index: %d verses, corpus has %d", ix.verseCount, len(b.Verses))
	}
	for i := range b.Verses {
		if ix.verseBook[i] != b.Verses[i].BookID {
			return fmt.Errorf("index: verse %d is in book %d, corpus says %d", i, ix.verseBook[i], b.Verses[i].BookID)
		}
	}
	return nil
}

// Write serialises the index.
func (ix *Index) Write(w io.Writer) (int64, error) {
	// Token string blob and dictionary.
	var tokenBlob []byte
	dict := make([]byte, 0, len(ix.tokens)*dictRecord)
	for _, t := range ix.tokens {
		off := uint32(len(tokenBlob))
		tokenBlob = append(tokenBlob, t.token...)
		var r [dictRecord]byte
		binary.LittleEndian.PutUint32(r[0:4], off)
		binary.LittleEndian.PutUint32(r[4:8], uint32(len(t.token)))
		binary.LittleEndian.PutUint32(r[8:12], t.docFreq)
		// Posting offset and length are filled in below.
		dict = append(dict, r[:]...)
	}

	var postings []byte
	if ix.postBytes != nil {
		// Indexes read from disk already hold the canonical varint section;
		// dictionary entries carry its byte offsets and lengths.
		postings = ix.postBytes
		for i, t := range ix.tokens {
			binary.LittleEndian.PutUint32(dict[i*dictRecord+12:i*dictRecord+16], t.postOff)
			binary.LittleEndian.PutUint32(dict[i*dictRecord+16:i*dictRecord+20], t.postLen)
		}
	} else {
		var tmp [binary.MaxVarintLen64]byte
		for i := range ix.tokens {
			e := &ix.tokens[i]
			off := uint32(len(postings))
			prev := uint32(0)
			for j := 0; j < int(e.docFreq); j++ {
				p := ix.postingAt(e, j)
				n := binary.PutUvarint(tmp[:], uint64(p.verse-prev))
				postings = append(postings, tmp[:n]...)
				n = binary.PutUvarint(tmp[:], uint64(p.tf))
				postings = append(postings, tmp[:n]...)
				prev = p.verse
			}
			binary.LittleEndian.PutUint32(dict[i*dictRecord+12:i*dictRecord+16], off)
			binary.LittleEndian.PutUint32(dict[i*dictRecord+16:i*dictRecord+20], uint32(len(postings))-off)
		}
	}

	docLen := make([]byte, len(ix.docLen)*2)
	for i, n := range ix.docLen {
		binary.LittleEndian.PutUint16(docLen[i*2:i*2+2], n)
	}
	books := make([]byte, len(ix.verseBook)*2)
	for i, id := range ix.verseBook {
		binary.LittleEndian.PutUint16(books[i*2:i*2+2], id)
	}

	off := uint32(indexHeader)
	next := func(n int) uint32 {
		o := off
		off += uint32(n)
		off = (off + 7) &^ 7
		return o
	}
	tokenOff := next(len(tokenBlob))
	dictOff := next(len(dict))
	postOff := next(len(postings))
	docOff := next(len(docLen))
	bookOff := next(len(books))
	total := off

	hdr := make([]byte, indexHeader)
	copy(hdr[0:4], indexMagic)
	binary.LittleEndian.PutUint16(hdr[4:6], indexVersion)
	binary.LittleEndian.PutUint16(hdr[6:8], indexHeader)
	binary.LittleEndian.PutUint32(hdr[8:12], uint32(len(ix.tokens)))
	binary.LittleEndian.PutUint32(hdr[12:16], uint32(len(postings)))
	binary.LittleEndian.PutUint32(hdr[16:20], uint32(ix.verseCount))
	binary.LittleEndian.PutUint32(hdr[20:24], uint32(len(tokenBlob)))
	binary.LittleEndian.PutUint32(hdr[24:28], dictOff)
	binary.LittleEndian.PutUint32(hdr[28:32], uint32(len(dict)))
	binary.LittleEndian.PutUint32(hdr[32:36], postOff)
	binary.LittleEndian.PutUint32(hdr[36:40], uint32(len(postings)))
	binary.LittleEndian.PutUint32(hdr[40:44], docOff)
	binary.LittleEndian.PutUint32(hdr[44:48], uint32(len(docLen)))
	binary.LittleEndian.PutUint32(hdr[48:52], bookOff)
	binary.LittleEndian.PutUint32(hdr[52:56], uint32(len(books)))
	binary.LittleEndian.PutUint64(hdr[56:64], ix.totalTokens)

	wb := bufio.NewWriterSize(w, 256*1024)
	sections := [][]byte{hdr, tokenBlob, dict, postings, docLen, books}
	offsets := []uint32{0, tokenOff, dictOff, postOff, docOff, bookOff}
	written := uint32(0)
	for i, sec := range sections {
		if pad := offsets[i] - written; pad > 0 {
			if _, err := wb.Write(make([]byte, pad)); err != nil {
				return 0, err
			}
			written += pad
		}
		if _, err := wb.Write(sec); err != nil {
			return 0, err
		}
		written += uint32(len(sec))
	}
	if pad := total - written; pad > 0 {
		if _, err := wb.Write(make([]byte, pad)); err != nil {
			return 0, err
		}
	}
	if err := wb.Flush(); err != nil {
		return 0, err
	}
	return int64(total), nil
}

// Open parses an index blob. The returned Index owns copies of its strings.
func Open(data []byte) (*Index, error) {
	return open(data, false)
}

// OpenBorrowed parses an index blob without copying dictionary strings. data
// must remain alive and unmodified for as long as the Index is used.
func OpenBorrowed(data []byte) (*Index, error) {
	return open(data, true)
}

func open(data []byte, borrow bool) (*Index, error) {
	if len(data) < indexHeader {
		return nil, fmt.Errorf("index: truncated header")
	}
	if string(data[0:4]) != indexMagic {
		return nil, fmt.Errorf("index: bad magic")
	}
	if v := binary.LittleEndian.Uint16(data[4:6]); v != indexVersion {
		return nil, fmt.Errorf("index: unsupported version %d", v)
	}
	if h := binary.LittleEndian.Uint16(data[6:8]); h != indexHeader {
		return nil, fmt.Errorf("index: unsupported header size %d", h)
	}
	tokenCount64 := int64(binary.LittleEndian.Uint32(data[8:12]))
	postingsLen64 := int64(binary.LittleEndian.Uint32(data[12:16]))
	verseCount64 := int64(binary.LittleEndian.Uint32(data[16:20]))
	tokenBlobLen64 := int64(binary.LittleEndian.Uint32(data[20:24]))
	dictOff64 := int64(binary.LittleEndian.Uint32(data[24:28]))
	dictLen64 := int64(binary.LittleEndian.Uint32(data[28:32]))
	postOff64 := int64(binary.LittleEndian.Uint32(data[32:36]))
	docOff64 := int64(binary.LittleEndian.Uint32(data[40:44]))
	docLen64 := int64(binary.LittleEndian.Uint32(data[44:48]))
	bookOff64 := int64(binary.LittleEndian.Uint32(data[48:52]))
	bookLen64 := int64(binary.LittleEndian.Uint32(data[52:56]))
	totalTokens := binary.LittleEndian.Uint64(data[56:64])

	check := func(name string, off, size int64) error {
		if off < indexHeader || size < 0 || off > int64(len(data)) || size > int64(len(data))-off || off%8 != 0 {
			return fmt.Errorf("index: %s section out of range", name)
		}
		return nil
	}
	for _, s := range []struct {
		name     string
		off, len int64
	}{
		{"tokens", indexHeader, tokenBlobLen64},
		{"dictionary", dictOff64, dictLen64},
		{"postings", postOff64, postingsLen64},
		{"doc lengths", docOff64, docLen64},
		{"books", bookOff64, bookLen64},
	} {
		if err := check(s.name, s.off, s.len); err != nil {
			return nil, err
		}
	}
	if dictLen64 != tokenCount64*dictRecord {
		return nil, fmt.Errorf("index: dictionary size mismatch")
	}
	if docLen64 != verseCount64*2 || bookLen64 != verseCount64*2 {
		return nil, fmt.Errorf("index: verse table size mismatch")
	}
	if verseCount64 == 0 {
		return nil, fmt.Errorf("index: no verses")
	}

	tokenCount := int(tokenCount64)
	verseCount := int(verseCount64)
	tokenBlobLen := int(tokenBlobLen64)
	dictOff := int(dictOff64)
	dictLen := int(dictLen64)
	postOff := int(postOff64)
	docOff := int(docOff64)
	bookOff := int(bookOff64)

	tokenBlob := data[indexHeader : indexHeader+tokenBlobLen]
	dict := data[dictOff : dictOff+dictLen]
	postSec := data[postOff:int(postOff64+postingsLen64)]

	ix := &Index{
		tokens:      make([]tokenEntry, tokenCount),
		docLen:      make([]uint16, verseCount),
		verseBook:   make([]uint16, verseCount),
		verseCount:  verseCount,
		totalTokens: totalTokens,
	}
	ix.avgLen = float64(totalTokens) / float64(verseCount)
	str := func(off, n int) (string, error) {
		if off < 0 || n < 0 || off > len(tokenBlob) || n > len(tokenBlob)-off {
			return "", fmt.Errorf("index: token out of range")
		}
		if n == 0 {
			return "", nil
		}
		raw := tokenBlob[off : off+n]
		if borrow {
			return unsafe.String(unsafe.SliceData(raw), len(raw)), nil
		}
		return string(raw), nil
	}

	if borrow {
		// The embedded blob is process-lifetime, so borrow its postings too.
		ix.postBytes = postSec
	} else {
		ix.postBytes = append([]byte(nil), postSec...)
	}

	// Validate the dictionary and every posting range eagerly. Keeping this
	// pass at startup preserves corruption detection without retaining decoded
	// posting arrays.
	prevToken := ""
	for i := 0; i < tokenCount; i++ {
		r := dict[i*dictRecord:]
		tok, err := str(int(binary.LittleEndian.Uint32(r[0:4])), int(binary.LittleEndian.Uint32(r[4:8])))
		if err != nil {
			return nil, err
		}
		if tok == "" {
			return nil, fmt.Errorf("index: empty token")
		}
		if i > 0 && tok <= prevToken {
			return nil, fmt.Errorf("index: dictionary is not sorted at %q", tok)
		}
		prevToken = tok
		df := int(binary.LittleEndian.Uint32(r[8:12]))
		poff := int(binary.LittleEndian.Uint32(r[12:16]))
		plen := int(binary.LittleEndian.Uint32(r[16:20]))
		if df <= 0 || df > verseCount {
			return nil, fmt.Errorf("index: document frequency out of range")
		}
		if poff > len(postSec) || plen > len(postSec)-poff {
			return nil, fmt.Errorf("index: postings out of range")
		}
		if err := validatePostings(postSec[poff:poff+plen], df, verseCount); err != nil {
			return nil, err
		}
		ix.tokens[i] = tokenEntry{
			token:   tok,
			docFreq: uint32(df),
			postOff: uint32(poff),
			postLen: uint32(plen),
		}
	}

	var tokenSum uint64
	for i := 0; i < verseCount; i++ {
		n := binary.LittleEndian.Uint16(data[docOff+i*2:])
		ix.docLen[i] = n
		tokenSum += uint64(n)
		book := binary.LittleEndian.Uint16(data[bookOff+i*2:])
		if book == 0 {
			return nil, fmt.Errorf("index: verse %d has no book", i)
		}
		ix.verseBook[i] = book
	}
	if tokenSum != totalTokens {
		return nil, fmt.Errorf("index: token total mismatch")
	}
	return ix, nil
}

// validatePostings checks a posting byte range without materialising it. It
// keeps startup corruption detection identical to the old eager decoder.
func validatePostings(b []byte, count, verseCount int) error {
	pos := 0
	prev := uint32(0)
	for i := 0; i < count; i++ {
		delta, n := binary.Uvarint(b[pos:])
		if n <= 0 || delta > math.MaxUint32 {
			return fmt.Errorf("index: bad posting delta")
		}
		pos += n
		tf, n := binary.Uvarint(b[pos:])
		if n <= 0 || tf == 0 || tf > math.MaxUint16 {
			return fmt.Errorf("index: bad posting frequency")
		}
		pos += n
		if i == 0 {
			prev = uint32(delta)
		} else {
			if delta == 0 || uint64(prev)+delta > math.MaxUint32 {
				return fmt.Errorf("index: postings are not increasing")
			}
			prev += uint32(delta)
		}
		if int(prev) >= verseCount {
			return fmt.Errorf("index: posting verse out of range")
		}
	}
	if pos != len(b) {
		return fmt.Errorf("index: trailing posting bytes")
	}
	return nil
}

// forEachPostingBytes scans a validated varint posting range.
func forEachPostingBytes(b []byte, fn func(posting)) {
	pos := 0
	prev := uint32(0)
	first := true
	for pos < len(b) {
		delta, n := binary.Uvarint(b[pos:])
		if n <= 0 {
			return
		}
		pos += n
		tf, n := binary.Uvarint(b[pos:])
		if n <= 0 {
			return
		}
		pos += n
		if first {
			prev = uint32(delta)
			first = false
		} else {
			prev += uint32(delta)
		}
		fn(posting{verse: prev, tf: uint16(tf)})
	}
}

func (ix *Index) findToken(tok string) int {
	i := sort.Search(len(ix.tokens), func(i int) bool { return ix.tokens[i].token >= tok })
	if i < len(ix.tokens) && ix.tokens[i].token == tok {
		return i
	}
	return -1
}

func (ix *Index) prefixRange(prefix string) (int, int) {
	lo := sort.Search(len(ix.tokens), func(i int) bool { return ix.tokens[i].token >= prefix })
	hi := lo
	for hi < len(ix.tokens) && len(ix.tokens[hi].token) >= len(prefix) && ix.tokens[hi].token[:len(prefix)] == prefix {
		hi++
	}
	return lo, hi
}
