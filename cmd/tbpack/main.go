// Command tbpack builds the downloadable translation packs and the catalog
// that describes them. Sources are public-domain eBible.org USFM archives.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	catalogpkg "github.com/petealeon/tbible/v2/internal/catalog"
	"github.com/petealeon/tbible/v2/internal/corpus"
	"github.com/petealeon/tbible/v2/internal/packs"
	"github.com/petealeon/tbible/v2/internal/search"
	"github.com/petealeon/tbible/v2/internal/usfm"
)

const (
	packTag    = "packs-v1"
	releaseURL = "https://github.com/petealeon/tbible/releases/download/" + packTag
)

type source struct {
	id        string
	name      string
	zip       string
	url       string
	sourceURL string
	license   string
	webRomans bool
}

var sources = []source{
	{
		id: "asv", name: "American Standard Version", zip: "eng-asv_usfm.zip",
		url:       "https://ebible.org/Scriptures/eng-asv_usfm.zip",
		sourceURL: "https://ebible.org/eng-asv", license: "Public domain (1901)",
	},
	{
		id: "web", name: "World English Bible", zip: "eng-web_usfm.zip",
		url:       "https://ebible.org/Scriptures/eng-web_usfm.zip",
		sourceURL: "https://ebible.org/eng-web", license: "Public domain",
		webRomans: true,
	},
	{
		id: "ylt", name: "Young's Literal Translation", zip: "engylt_usfm.zip",
		url:       "https://ebible.org/Scriptures/engylt_usfm.zip",
		sourceURL: "https://ebible.org/engylt", license: "Public domain (1898)",
	},
	{
		id: "wmb", name: "World Messianic Bible", zip: "engwmb_usfm.zip",
		url:       "https://ebible.org/Scriptures/engwmb_usfm.zip",
		sourceURL: "https://ebible.org/engwmb", license: "Public domain",
		webRomans: true,
	},
}

func main() {
	var (
		srcDir  = flag.String("src-dir", "data/packs", "directory containing source USFM zips")
		outDir  = flag.String("out-dir", "dist/packs", "directory for generated .tbp files")
		catalog = flag.String("catalog", "internal/catalog/data/catalog.json", "catalog output")
		updated = flag.String("updated", "2026-11-21", "catalog update date")
	)
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}
	out := catalogpkg.Catalog{
		Version:      1,
		Updated:      *updated,
		Source:       releaseURL + "/catalog.json",
		Translations: []catalogpkg.Entry{},
	}
	for _, s := range sources {
		entry, err := buildPack(s, *srcDir, *outDir, *updated)
		if err != nil {
			fatal(fmt.Errorf("%s: %w", s.id, err))
		}
		out.Translations = append(out.Translations, entry)
	}
	sort.Slice(out.Translations, func(i, j int) bool {
		return out.Translations[i].Name < out.Translations[j].Name
	})
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := writeAtomic(filepath.Join(*catalog+".tmp"), *catalog, data); err != nil {
		fatal(err)
	}
	fmt.Printf("catalog written: %s (%d translations)\n", *catalog, len(out.Translations))
}

func buildPack(s source, srcDir, outDir, updated string) (catalogpkg.Entry, error) {
	src := filepath.Join(srcDir, s.zip)
	opts := []usfm.Option{usfm.CanonicalOnly()}
	if s.webRomans {
		opts = append(opts, usfm.WebRomans())
	}
	b, err := usfm.ParseZipWith(src, opts...)
	if err != nil {
		return catalogpkg.Entry{}, err
	}
	if err := b.Validate(); err != nil {
		return catalogpkg.Entry{}, err
	}
	ix := search.Build(b)
	if err := ix.CheckCorpus(b); err != nil {
		return catalogpkg.Entry{}, err
	}

	var corpusBuf, indexBuf bytes.Buffer
	if _, err := corpus.Write(&corpusBuf, b); err != nil {
		return catalogpkg.Entry{}, err
	}
	if _, err := ix.Write(&indexBuf); err != nil {
		return catalogpkg.Entry{}, err
	}
	var packBuf bytes.Buffer
	n, err := packs.Write(&packBuf, packs.Content{
		Manifest: packs.Manifest{
			ID: s.id, Name: s.name, Language: "English",
			Source: s.sourceURL, License: s.license,
		},
		Corpus: corpusBuf.Bytes(),
		Index:  indexBuf.Bytes(),
	})
	if err != nil {
		return catalogpkg.Entry{}, err
	}
	if n != int64(packBuf.Len()) {
		return catalogpkg.Entry{}, fmt.Errorf("pack size mismatch: wrote %d, buffered %d", n, packBuf.Len())
	}

	decoded, err := packs.Decode(packBuf.Bytes())
	if err != nil {
		return catalogpkg.Entry{}, err
	}
	checkBible, err := corpus.Open(decoded.Corpus)
	if err != nil {
		return catalogpkg.Entry{}, err
	}
	checkIndex, err := search.Open(decoded.Index)
	if err != nil {
		return catalogpkg.Entry{}, err
	}
	if err := checkIndex.CheckCorpus(checkBible); err != nil {
		return catalogpkg.Entry{}, err
	}

	dst := filepath.Join(outDir, s.id+packs.Extension)
	if err := writeAtomic(dst+".tmp", dst, packBuf.Bytes()); err != nil {
		return catalogpkg.Entry{}, err
	}
	sum := sha256.Sum256(packBuf.Bytes())
	fmt.Printf("%-4s %-28s %6d verses  %6.1f MB  sha256 %s\n",
		s.id, s.name, len(checkBible.Verses), float64(len(packBuf.Bytes()))/(1<<20), hex.EncodeToString(sum[:]))

	return catalogpkg.Entry{
		ID: s.id, Name: s.name, Language: "English",
		Description: s.license,
		License:     s.license,
		Source:      s.sourceURL,
		URL:         releaseURL + "/" + s.id + packs.Extension,
		SHA256:      hex.EncodeToString(sum[:]),
		Size:        int64(len(packBuf.Bytes())),
		Built:       updated,
	}, nil
}

func writeAtomic(tmp, dst string, data []byte) error {
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "tbpack:", err)
	os.Exit(1)
}
