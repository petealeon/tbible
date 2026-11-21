// Package catalog holds the list of downloadable translations. The catalog is
// embedded so a fresh install can show what is available offline; refreshing
// it is an explicit user action and never happens in the background.
package catalog

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

//go:embed data/catalog.json
var embeddedJSON []byte

// Entry describes one downloadable translation pack.
type Entry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Language    string `json:"language,omitempty"`
	Description string `json:"description,omitempty"`
	License     string `json:"license,omitempty"`
	Source      string `json:"source,omitempty"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Built       string `json:"built,omitempty"`
}

// Catalog is the versioned list of packs.
type Catalog struct {
	Version      int     `json:"version"`
	Updated      string  `json:"updated,omitempty"`
	Source       string  `json:"source,omitempty"`
	Translations []Entry `json:"translations"`
}

// Embedded parses the catalog shipped in the binary.
func Embedded() (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(embeddedJSON, &c); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	if c.Version == 0 {
		return nil, fmt.Errorf("catalog: invalid version")
	}
	return &c, nil
}

// Bytes returns the embedded catalog JSON for verification tools.
func Bytes() []byte { return embeddedJSON }

// Find returns an entry by id.
func (c *Catalog) Find(id string) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	for _, e := range c.Translations {
		if strings.EqualFold(e.ID, id) {
			return e, true
		}
	}
	return Entry{}, false
}

// Refresh fetches a catalog from a URL. Callers must invoke this only in
// response to a direct user request.
func Refresh(ctx context.Context, url string) (*Catalog, error) {
	if url == "" {
		return nil, fmt.Errorf("catalog: empty URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "tbible-catalog/1")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("catalog: HTTP %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	if c.Version == 0 || len(c.Translations) == 0 {
		return nil, fmt.Errorf("catalog: incomplete document")
	}
	return &c, nil
}

// Download fetches a pack with a bounded body and timeout. Network access
// only happens when the user explicitly asks to install a translation.
func Download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "tbible-pack/1")
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download: HTTP %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}
