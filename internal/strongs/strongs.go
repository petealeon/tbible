// Package strongs holds the Strong's lexicon bundled with tbible.
//
// The source data is STEPBible's TBESH (Hebrew) and TBESG (Greek) lexicons,
// which extend the public-domain Strong's numbering with lemma, transliteration
// and a concise definition. STEPBible publishes them under CC BY 4.0; see
// NOTICE.md. The build pipeline trims the source to the fields the
// reader shows and stores the result as gzip-compressed JSON.
package strongs

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"regexp"
	"sort"
	"strings"
)

// Entry is one lexicon headword, keyed by its base Strong's number.
type Entry struct {
	Number     uint16 `json:"n"`
	Greek      bool   `json:"g,omitempty"`
	Lemma      string `json:"l,omitempty"`
	Translit   string `json:"t,omitempty"`
	Gloss      string `json:"s,omitempty"`
	Definition string `json:"d,omitempty"`
}

// Tag formats the entry's number as H7225 or G3056.
func (e Entry) Tag() string {
	if e.Greek {
		return fmt.Sprintf("G%04d", e.Number)
	}
	return fmt.Sprintf("H%04d", e.Number)
}

type document struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

const documentVersion = 1

// Dictionary is a read-only lexicon keyed by number and language.
type Dictionary struct {
	m map[uint32]Entry
}

func key(greek bool, n uint16) uint32 {
	k := uint32(n)
	if greek {
		k |= 1 << 31
	}
	return k
}

// Lookup returns the entry for a Strong's number.
func (d *Dictionary) Lookup(greek bool, n uint16) (Entry, bool) {
	if d == nil || d.m == nil {
		return Entry{}, false
	}
	e, ok := d.m[key(greek, n)]
	return e, ok
}

// LookupTag resolves a tag such as "H7225" or "G3056".
func (d *Dictionary) LookupTag(tag string) (Entry, bool) {
	greek, n, ok := ParseTag(tag)
	if !ok {
		return Entry{}, false
	}
	return d.Lookup(greek, n)
}

// Len reports the number of headwords.
func (d *Dictionary) Len() int {
	if d == nil {
		return 0
	}
	return len(d.m)
}

// ParseTag splits a Strong's tag into its language and base number.
func ParseTag(tag string) (greek bool, n uint16, ok bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false, 0, false
	}
	switch tag[0] {
	case 'H', 'h':
	case 'G', 'g':
		greek = true
	default:
		return false, 0, false
	}
	digits := tag[1:]
	for len(digits) > 0 && (digits[len(digits)-1] < '0' || digits[len(digits)-1] > '9') {
		digits = digits[:len(digits)-1]
	}
	if digits == "" {
		return false, 0, false
	}
	var v int
	for _, r := range digits {
		v = v*10 + int(r-'0')
		if v > 65535 {
			return false, 0, false
		}
	}
	if v == 0 {
		return false, 0, false
	}
	return greek, uint16(v), true
}

// Parse reads the gzip-compressed JSON dictionary produced by WriteTo.
func Parse(r io.Reader) (*Dictionary, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return ParseJSON(zr)
}

// ParseJSON reads the uncompressed dictionary document.
func ParseJSON(r io.Reader) (*Dictionary, error) {
	var doc document
	dec := json.NewDecoder(bufio.NewReaderSize(r, 256*1024))
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("strongs: %w", err)
	}
	if doc.Version != documentVersion {
		return nil, fmt.Errorf("strongs: unsupported version %d", doc.Version)
	}
	d := &Dictionary{m: make(map[uint32]Entry, len(doc.Entries))}
	for _, e := range doc.Entries {
		if e.Number == 0 {
			continue
		}
		d.m[key(e.Greek, e.Number)] = e
	}
	return d, nil
}

// WriteTo serialises d as gzip-compressed JSON. Entries are sorted so builds
// are reproducible.
func (d *Dictionary) WriteTo(w io.Writer) (int64, error) {
	entries := make([]Entry, 0, d.Len())
	for _, e := range d.m {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Greek != entries[j].Greek {
			return !entries[i].Greek
		}
		return entries[i].Number < entries[j].Number
	})
	zw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return 0, err
	}
	zw.Name = "strongs.json"
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(document{Version: documentVersion, Entries: entries}); err != nil {
		zw.Close()
		return 0, err
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	return int64(len(entries)), nil
}

var tagRE = regexp.MustCompile(`<[^>]*>`)

// Build parses STEPBible TBESH and TBESG text files. The first headword for a
// base number wins; extended entries such as H7225G are folded into H7225.
func Build(hebrew, greek io.Reader) (*Dictionary, error) {
	d := &Dictionary{m: make(map[uint32]Entry, 14000)}
	if err := parseLexicon(hebrew, false, d); err != nil {
		return nil, fmt.Errorf("TBESH: %w", err)
	}
	if err := parseLexicon(greek, true, d); err != nil {
		return nil, fmt.Errorf("TBESG: %w", err)
	}
	return d, nil
}

func parseLexicon(r io.Reader, greek bool, d *Dictionary) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 128*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		line = strings.TrimPrefix(line, "\ufeff")
		if line == "" || line[0] == ' ' || line[0] == '$' || line[0] == '=' {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		num, ok := baseNumber(fields[0])
		if !ok {
			continue
		}
		k := key(greek, num)
		if _, exists := d.m[k]; exists {
			continue
		}
		e := Entry{Number: num, Greek: greek}
		if len(fields) > 3 {
			e.Lemma = strings.TrimSpace(fields[3])
		}
		if len(fields) > 4 {
			e.Translit = strings.TrimSpace(fields[4])
		}
		if len(fields) > 6 {
			e.Gloss = strings.TrimSpace(fields[6])
		}
		if len(fields) > 7 {
			e.Definition = cleanDefinition(fields[7])
		}
		if e.Definition == "" && e.Gloss != "" {
			e.Definition = e.Gloss
		}
		d.m[k] = e
	}
	return sc.Err()
}

func baseNumber(field string) (uint16, bool) {
	field = strings.TrimSpace(field)
	if len(field) < 2 || (field[0] != 'H' && field[0] != 'G' && field[0] != 'h' && field[0] != 'g') {
		return 0, false
	}
	v := 0
	seen := false
	for i := 1; i < len(field); i++ {
		c := field[i]
		if c < '0' || c > '9' {
			break
		}
		seen = true
		v = v*10 + int(c-'0')
		if v > 65535 {
			return 0, false
		}
	}
	if !seen || v == 0 {
		return 0, false
	}
	return uint16(v), true
}

var (
	brRE    = regexp.MustCompile(`(?i)<br\s*/?>`)
	wsRE    = regexp.MustCompile(`[ \t]+`)
	blankRE = regexp.MustCompile(`\n{3,}`)
)

// cleanDefinition turns the lexicon's HTML fragment into plain text with one
// line per sense. Definitions are capped so the embedded dictionary stays
// compact; the first few senses are the useful ones.
func cleanDefinition(s string) string {
	if s == "" {
		return ""
	}
	s = brRE.ReplaceAllString(s, "\n")
	s = html.UnescapeString(s)
	s = tagRE.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(wsRE.ReplaceAllString(lines[i], " "))
	}
	s = strings.Join(lines, "\n")
	s = blankRE.ReplaceAllString(s, "\n\n")
	s = strings.TrimSpace(s)
	const max = 8000
	if len(s) > max {
		cut := s[:max]
		if i := strings.LastIndexByte(cut, '\n'); i > max/2 {
			cut = cut[:i]
		}
		s = strings.TrimSpace(cut) + "\n…"
	}
	return s
}
