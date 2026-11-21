// Package packs implements tbible's data-only translation packs. A pack is a
// single gzip-compressed file containing a JSON manifest and the binary corpus
// and search index for one translation. Packs never contain executable code.
package packs

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	// Magic identifies the uncompressed pack body.
	Magic = "TBP1"
	// FormatVersion is incremented when the container layout changes.
	FormatVersion = 1
	// Extension is the on-disk suffix for installed packs.
	Extension = ".tbp"
)

// Manifest describes the contents of a pack. Hashes cover the uncompressed
// corpus and index, while the catalog's sha256 covers the whole .tbp file.
type Manifest struct {
	Format       string `json:"format"`
	Version      int    `json:"version"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Language     string `json:"language,omitempty"`
	Source       string `json:"source,omitempty"`
	License      string `json:"license,omitempty"`
	CorpusSize   int    `json:"corpus_size"`
	IndexSize    int    `json:"index_size"`
	CorpusSHA256 string `json:"corpus_sha256"`
	IndexSHA256  string `json:"index_sha256"`
}

// Content is a decoded pack.
type Content struct {
	Manifest Manifest
	Corpus   []byte
	Index    []byte
}

// Write encodes c into w as a gzip-compressed pack. It returns the number of
// compressed bytes written.
func Write(w io.Writer, c Content) (int64, error) {
	if c.Manifest.ID == "" {
		return 0, errors.New("pack: empty translation id")
	}
	m := c.Manifest
	m.Format = Magic
	m.Version = FormatVersion
	m.CorpusSize = len(c.Corpus)
	m.IndexSize = len(c.Index)
	m.CorpusSHA256 = hexHash(c.Corpus)
	m.IndexSHA256 = hexHash(c.Index)

	meta, err := json.Marshal(m)
	if err != nil {
		return 0, err
	}
	var body bytes.Buffer
	if _, err := body.WriteString(Magic + "\n"); err != nil {
		return 0, err
	}
	writeSection := func(b []byte) error {
		var n [4]byte
		binary.LittleEndian.PutUint32(n[:], uint32(len(b)))
		if _, err := body.Write(n[:]); err != nil {
			return err
		}
		_, err := body.Write(b)
		return err
	}
	if err := writeSection(meta); err != nil {
		return 0, err
	}
	if err := writeSection(c.Corpus); err != nil {
		return 0, err
	}
	if err := writeSection(c.Index); err != nil {
		return 0, err
	}

	cw := &countWriter{w: w}
	zw, err := gzip.NewWriterLevel(cw, gzip.BestCompression)
	if err != nil {
		return 0, err
	}
	zw.Name = m.ID + Extension
	if _, err := zw.Write(body.Bytes()); err != nil {
		zw.Close()
		return cw.n, err
	}
	if err := zw.Close(); err != nil {
		return cw.n, err
	}
	return cw.n, nil
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// Read decodes a pack, verifying both section hashes.
func Read(r io.Reader) (*Content, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("pack: %w", err)
	}
	defer zr.Close()
	body, err := io.ReadAll(io.LimitReader(zr, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("pack: read: %w", err)
	}
	return parseBody(body)
}

// Decode decodes an in-memory pack.
func Decode(data []byte) (*Content, error) { return Read(bytes.NewReader(data)) }

func parseBody(body []byte) (*Content, error) {
	if !bytes.HasPrefix(body, []byte(Magic+"\n")) {
		return nil, errors.New("pack: bad magic")
	}
	rest := body[len(Magic)+1:]
	section := func() ([]byte, error) {
		if len(rest) < 4 {
			return nil, errors.New("pack: truncated section header")
		}
		n := int(binary.LittleEndian.Uint32(rest[:4]))
		rest = rest[4:]
		if n < 0 || n > len(rest) {
			return nil, errors.New("pack: truncated section")
		}
		b := rest[:n]
		rest = rest[n:]
		return b, nil
	}
	meta, err := section()
	if err != nil {
		return nil, err
	}
	corpus, err := section()
	if err != nil {
		return nil, err
	}
	index, err := section()
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, errors.New("pack: trailing data")
	}
	var m Manifest
	if err := json.Unmarshal(meta, &m); err != nil {
		return nil, fmt.Errorf("pack: manifest: %w", err)
	}
	if m.Format != Magic || m.Version != FormatVersion {
		return nil, fmt.Errorf("pack: unsupported format %q version %d", m.Format, m.Version)
	}
	if m.ID == "" || m.Name == "" {
		return nil, errors.New("pack: manifest missing id or name")
	}
	if len(corpus) != m.CorpusSize || len(index) != m.IndexSize {
		return nil, errors.New("pack: section size mismatch")
	}
	if hexHash(corpus) != m.CorpusSHA256 || hexHash(index) != m.IndexSHA256 {
		return nil, errors.New("pack: checksum mismatch")
	}
	return &Content{Manifest: m, Corpus: corpus, Index: index}, nil
}

func hexHash(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// SHA256 returns the hash of a complete .tbp file, as used by the catalog.
func SHA256(data []byte) string { return hexHash(data) }

// Dir returns the directory where packs are installed. TBIBLE_PACKS_DIR
// overrides the platform config location.
func Dir() (string, error) {
	if dir := os.Getenv("TBIBLE_PACKS_DIR"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", err
		}
		base = filepath.Join(home, ".tbible")
	}
	return filepath.Join(base, "tbible", "packs"), nil
}

// Path returns the install path for an id.
func Path(dir, id string) string {
	return filepath.Join(dir, cleanID(id)+Extension)
}

func cleanID(id string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(id)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Verify checks a downloaded file against the catalog hash, then installs it
// atomically. A bad hash never touches an existing installation.
func VerifyAndInstall(dir string, data []byte, expectedSHA string) (Manifest, error) {
	if expectedSHA == "" {
		return Manifest{}, errors.New("pack: catalog has no checksum")
	}
	if got := SHA256(data); !strings.EqualFold(got, expectedSHA) {
		return Manifest{}, fmt.Errorf("pack: checksum mismatch (got %s, want %s)", got, expectedSHA)
	}
	c, err := Decode(data)
	if err != nil {
		return Manifest{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Manifest{}, err
	}
	final := Path(dir, c.Manifest.ID)
	tmp, err := os.CreateTemp(dir, ".install-*"+Extension)
	if err != nil {
		return Manifest{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return Manifest{}, err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return Manifest{}, err
	}
	if err := tmp.Close(); err != nil {
		return Manifest{}, err
	}
	// Windows cannot rename over an existing file. The replacement has already
	// been fully verified, so removing the old pack immediately before the
	// rename is safe; on Unix the rename replaces it atomically.
	if runtime.GOOS == "windows" {
		if err := os.Remove(final); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Manifest{}, err
		}
	}
	if err := os.Rename(tmpName, final); err != nil {
		return Manifest{}, err
	}
	return c.Manifest, nil
}

// Load reads an installed pack's manifest without loading its corpus.
func LoadManifest(dir, id string) (Manifest, error) {
	data, err := os.ReadFile(Path(dir, id))
	if err != nil {
		return Manifest{}, err
	}
	c, err := Decode(data)
	if err != nil {
		return Manifest{}, err
	}
	return c.Manifest, nil
}

// Load reads and decodes an installed pack.
func Load(dir, id string) (*Content, error) {
	data, err := os.ReadFile(Path(dir, id))
	if err != nil {
		return nil, err
	}
	return Decode(data)
}

// Installed lists the ids of installed packs.
func Installed(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), Extension) {
			continue
		}
		id := strings.TrimSuffix(e.Name(), Extension)
		if cleanID(id) == id && id != "" {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Remove deletes an installed pack.
func Remove(dir, id string) error {
	if cleanID(id) != id || id == "" {
		return errors.New("pack: invalid translation id")
	}
	err := os.Remove(Path(dir, id))
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("pack: translation is not installed")
	}
	return err
}
