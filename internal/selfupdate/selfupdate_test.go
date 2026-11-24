package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVersionComparison(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"2.2.0", "2.2.0", false},
		{"2.1.10", "2.1.2", false},
		{"2.1.2", "2.1.10", true},
		{"2.2.0-dev", "2.2.0", true},
		{"2.2.0", "2.2.0-rc1", false},
		{"v2.2.0", "2.3.0", true},
	}
	for _, tt := range tests {
		got, err := (Release{Version: tt.latest}).NewerThan(tt.current)
		if err != nil {
			t.Fatalf("NewerThan(%q, %q): %v", tt.latest, tt.current, err)
		}
		if got != tt.want {
			t.Errorf("NewerThan(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
	if _, err := (Release{Version: "not-a-version"}).NewerThan("2.2.0"); err == nil {
		t.Fatal("expected an error for an invalid release version")
	}
}

func TestAssetName(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "tbible-2.2.0-linux-amd64.tar.gz"},
		{"linux", "arm64", "tbible-2.2.0-linux-arm64.tar.gz"},
		{"darwin", "amd64", "tbible-2.2.0-darwin-amd64.tar.gz"},
		{"darwin", "arm64", "tbible-2.2.0-darwin-arm64.tar.gz"},
		{"windows", "amd64", "tbible-2.2.0-windows-amd64.zip"},
	}
	for _, tt := range tests {
		got, err := assetName(tt.goos, tt.goarch, "2.2.0")
		if err != nil {
			t.Fatalf("assetName(%s, %s): %v", tt.goos, tt.goarch, err)
		}
		if got != tt.want {
			t.Errorf("assetName(%s, %s) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
	}
	for _, tc := range [][2]string{{"windows", "arm64"}, {"plan9", "amd64"}} {
		if _, err := assetName(tc[0], tc[1], "2.2.0"); err == nil {
			t.Errorf("expected an error for %s/%s", tc[0], tc[1])
		}
	}
}

func TestFetchVerified(t *testing.T) {
	payload := []byte("#!/bin/sh\necho tbible 9.9.9\n")
	cfg, rel := fakeReleaseServer(t, "9.9.9", payload, false)

	if rel.Version != "9.9.9" {
		t.Fatalf("release version = %q", rel.Version)
	}
	newer, err := rel.NewerThan("2.2.0")
	if err != nil || !newer {
		t.Fatalf("NewerThan(2.2.0) = %v, %v", newer, err)
	}

	dl, err := cfg.Fetch(context.Background(), rel)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer dl.Cleanup()
	got, err := os.ReadFile(dl.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("staged binary = %q, want %q", got, payload)
	}
	wantName := "tbible"
	if runtime.GOOS == "windows" {
		wantName = "tbible.exe"
	}
	if filepath.Base(dl.Path) != wantName {
		t.Errorf("staged name = %q, want %q", filepath.Base(dl.Path), wantName)
	}
}

func TestFetchRejectsChecksumMismatch(t *testing.T) {
	payload := []byte("fake update binary")
	cfg, rel := fakeReleaseServer(t, "9.9.9", payload, true)
	if _, err := cfg.Fetch(context.Background(), rel); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Fetch error = %v, want checksum mismatch", err)
	}
}

func TestInstallReplacesTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Windows helper swap is exercised by the release smoke test")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "tbible")
	staged := filepath.Join(dir, "staged")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(staged, target); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("target contents = %q, want %q", got, "new")
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("target is not executable: %v", info.Mode())
	}
}

func fakeReleaseServer(t *testing.T, version string, payload []byte, corrupt bool) (Config, Release) {
	t.Helper()
	asset, err := assetName(runtime.GOOS, runtime.GOARCH, version)
	if err != nil {
		t.Skipf("no test asset for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	archive := archiveForPlatform(t, payload)
	manifest := checksumManifest(t, asset, archive, corrupt)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/"+defaultRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v" + version,
			"assets": []map[string]string{
				{"name": asset},
				{"name": checksumName(version)},
			},
		})
	})
	prefix := "/download/v" + version + "/"
	mux.HandleFunc(prefix+asset, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc(prefix+checksumName(version), func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(manifest)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := Config{
		APIBase:       srv.URL,
		ReleaseBase:   srv.URL + "/download",
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		SkipVerifyRun: true,
	}
	rel, err := cfg.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	return cfg, rel
}

func archiveForPlatform(t *testing.T, payload []byte) []byte {
	t.Helper()
	if runtime.GOOS == "windows" {
		return zipArchive(t, "tbible.exe", payload)
	}
	return tarGzArchive(t, "tbible", payload)
}

func tarGzArchive(t *testing.T, name string, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{
		Name:     name,
		Mode:     0o755,
		Size:     int64(len(payload)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipArchive(t *testing.T, name string, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func checksumManifest(t *testing.T, asset string, payload []byte, corrupt bool) []byte {
	t.Helper()
	sum := sha256.Sum256(payload)
	if corrupt {
		sum[0] ^= 0xff
	}
	return []byte(fmt.Sprintf("%x  %s\n", sum, asset))
}
