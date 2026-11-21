package catalog

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbedded(t *testing.T) {
	c, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if c.Version < 1 || len(c.Translations) == 0 {
		t.Fatalf("embedded catalog is empty: %+v", c)
	}
	if c.Source == "" {
		t.Fatal("embedded catalog has no refresh source")
	}
	for _, e := range c.Translations {
		if e.ID == "" || e.Name == "" || e.URL == "" || len(e.SHA256) != 64 || e.Size <= 0 {
			t.Fatalf("incomplete catalog entry: %+v", e)
		}
	}
	if e, ok := c.Find("WEB"); !ok || e.ID != "web" {
		t.Fatalf("Find(WEB) = %+v, %v", e, ok)
	}
}

func TestRefreshAndDownload(t *testing.T) {
	const payload = "pack bytes"
	doc := fmt.Sprintf(`{
		"version": 1,
		"source": "http://example/catalog",
		"translations": [{
			"id": "asv",
			"name": "American Standard Version",
			"url": "PACK_URL",
			"sha256": "%s",
			"size": 10
		}]
	}`, strings.Repeat("a", 64))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/catalog":
			fmt.Fprint(w, strings.ReplaceAll(doc, "PACK_URL", "http://"+r.Host+"/pack"))
		case "/pack":
			fmt.Fprint(w, payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := Refresh(context.Background(), srv.URL+"/catalog")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Translations) != 1 || c.Translations[0].ID != "asv" {
		t.Fatalf("refreshed catalog = %+v", c)
	}
	e, ok := c.Find("ASV")
	if !ok || e.Size != 10 {
		t.Fatalf("Find(ASV) = %+v, %v", e, ok)
	}
	got, err := Download(context.Background(), e.URL)
	if err != nil || string(got) != payload {
		t.Fatalf("Download = %q, %v", got, err)
	}
	if _, err := Download(context.Background(), srv.URL+"/missing"); err == nil {
		t.Fatal("HTTP 404 was accepted")
	}
}

func TestRefreshRequiresCompleteDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":1,"translations":[]}`)
	}))
	defer srv.Close()
	if _, err := Refresh(context.Background(), srv.URL); err == nil {
		t.Fatal("empty catalog was accepted")
	}
}
