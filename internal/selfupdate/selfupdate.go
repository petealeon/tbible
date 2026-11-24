// Package selfupdate implements explicit CLI updates from published GitHub
// releases. It is never called during startup or from the TUI.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRepo            = "petealeon/tbible"
	defaultAPIBase         = "https://api.github.com"
	maxAssetBytes    int64 = 96 << 20
	maxManifestBytes       = 1 << 20
)

// Config controls where releases are read from and which platform asset is
// selected. Zero values select the canonical repository and current platform.
type Config struct {
	APIBase       string
	ReleaseBase   string
	Repo          string
	HTTP          *http.Client
	GOOS          string
	GOARCH        string
	SkipVerifyRun bool
}

func (c Config) normalize() Config {
	if c.Repo == "" {
		c.Repo = defaultRepo
	}
	if c.APIBase == "" {
		c.APIBase = defaultAPIBase
	}
	c.APIBase = strings.TrimRight(c.APIBase, "/")
	if c.ReleaseBase == "" {
		c.ReleaseBase = "https://github.com/" + c.Repo + "/releases/download"
	}
	c.ReleaseBase = strings.TrimRight(c.ReleaseBase, "/")
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 60 * time.Second}
	}
	if c.GOOS == "" {
		c.GOOS = runtime.GOOS
	}
	if c.GOARCH == "" {
		c.GOARCH = runtime.GOARCH
	}
	return c
}

// Release is the stable part of a GitHub release needed for an update.
type Release struct {
	Tag     string
	Version string
	Assets  map[string]struct{}
}

// NewerThan reports whether the release is newer than current.
func (r Release) NewerThan(current string) (bool, error) {
	latest, err := parseVersion(r.Version)
	if err != nil {
		return false, err
	}
	installed, err := parseVersion(current)
	if err != nil {
		return false, err
	}
	return compareVersion(latest, installed) > 0, nil
}

type semver struct {
	major, minor, patch int
	pre                 string
}

func (v semver) core() string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}

func parseVersion(raw string) (semver, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	var v semver
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		v.pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("invalid version %q", raw)
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return semver{}, fmt.Errorf("invalid version %q", raw)
		}
		switch i {
		case 0:
			v.major = n
		case 1:
			v.minor = n
		case 2:
			v.patch = n
		}
	}
	return v, nil
}

func compareVersion(a, b semver) int {
	if a.major != b.major {
		if a.major < b.major {
			return -1
		}
		return 1
	}
	if a.minor != b.minor {
		if a.minor < b.minor {
			return -1
		}
		return 1
	}
	if a.patch != b.patch {
		if a.patch < b.patch {
			return -1
		}
		return 1
	}
	if a.pre == b.pre {
		return 0
	}
	if a.pre == "" {
		return 1
	}
	if b.pre == "" {
		return -1
	}
	return strings.Compare(a.pre, b.pre)
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

// Latest reads the newest non-prerelease release from GitHub.
func (c Config) Latest(ctx context.Context) (Release, error) {
	c = c.normalize()
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.APIBase, c.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "tbible-selfupdate")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("fetch latest release: %s", resp.Status)
	}

	var gr githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxManifestBytes)).Decode(&gr); err != nil {
		return Release{}, fmt.Errorf("decode latest release: %w", err)
	}
	version := strings.TrimSpace(strings.TrimPrefix(gr.TagName, "v"))
	if _, err := parseVersion(version); err != nil {
		return Release{}, err
	}
	rel := Release{
		Tag:     gr.TagName,
		Version: version,
		Assets:  make(map[string]struct{}, len(gr.Assets)),
	}
	for _, asset := range gr.Assets {
		rel.Assets[asset.Name] = struct{}{}
	}
	if len(rel.Assets) == 0 {
		return Release{}, fmt.Errorf("release %s has no assets", rel.Tag)
	}
	return rel, nil
}

// Download is a verified, staged update binary. Cleanup removes its temp dir.
type Download struct {
	Path string
	dir  string
}

// Cleanup removes the staged download.
func (d *Download) Cleanup() {
	if d != nil && d.dir != "" {
		_ = os.RemoveAll(d.dir)
	}
}

// Fetch downloads and verifies the asset for the configured platform.
func (c Config) Fetch(ctx context.Context, rel Release) (*Download, error) {
	c = c.normalize()
	fileVersion := strings.TrimPrefix(rel.Tag, "v")
	asset, err := assetName(c.GOOS, c.GOARCH, fileVersion)
	if err != nil {
		return nil, err
	}
	manifest := checksumName(fileVersion)
	if _, ok := rel.Assets[asset]; !ok {
		return nil, fmt.Errorf("release %s has no asset %s", rel.Tag, asset)
	}
	if _, ok := rel.Assets[manifest]; !ok {
		return nil, fmt.Errorf("release %s has no checksum manifest", rel.Tag)
	}

	dir, err := os.MkdirTemp("", "tbible-update-")
	if err != nil {
		return nil, err
	}
	d := &Download{dir: dir}
	assetPath := filepath.Join(dir, asset)
	manifestPath := filepath.Join(dir, manifest)
	if err := c.download(ctx, c.assetURL(fileVersion, asset), assetPath, maxAssetBytes); err != nil {
		d.Cleanup()
		return nil, fmt.Errorf("download %s: %w", asset, err)
	}
	if err := c.download(ctx, c.assetURL(fileVersion, manifest), manifestPath, maxManifestBytes); err != nil {
		d.Cleanup()
		return nil, fmt.Errorf("download checksum manifest: %w", err)
	}
	if err := verifyChecksum(assetPath, manifestPath, asset); err != nil {
		d.Cleanup()
		return nil, err
	}

	expected := "tbible"
	if c.GOOS == "windows" {
		expected = "tbible.exe"
	}
	staged := filepath.Join(dir, expected)
	if err := extractAsset(assetPath, asset, staged, expected); err != nil {
		d.Cleanup()
		return nil, err
	}
	if !c.SkipVerifyRun {
		if err := verifyRun(ctx, staged, rel.Version); err != nil {
			d.Cleanup()
			return nil, err
		}
	}
	d.Path = staged
	return d, nil
}

func (c Config) assetURL(version, name string) string {
	return fmt.Sprintf("%s/v%s/%s", c.ReleaseBase, version, name)
}

func assetName(goos, goarch, version string) (string, error) {
	switch goos {
	case "linux", "darwin":
		switch goarch {
		case "amd64", "arm64":
			return fmt.Sprintf("tbible-%s-%s-%s.tar.gz", version, goos, goarch), nil
		default:
			return "", fmt.Errorf("no update asset for %s/%s", goos, goarch)
		}
	case "windows":
		if goarch != "amd64" {
			return "", fmt.Errorf("no update asset for %s/%s", goos, goarch)
		}
		return fmt.Sprintf("tbible-%s-windows-amd64.zip", version), nil
	default:
		return "", fmt.Errorf("no update asset for %s/%s", goos, goarch)
	}
}

func checksumName(version string) string {
	return "tbible-" + version + "-checksums.txt"
}

func (c Config) download(ctx context.Context, url, dst string, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "tbible-selfupdate")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s", resp.Status)
	}

	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, limit+1))
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if n > limit {
		return fmt.Errorf("asset exceeds %d bytes", limit)
	}
	return nil
}

func verifyChecksum(assetPath, manifestPath, asset string) error {
	f, err := os.Open(manifestPath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(io.LimitReader(f, maxManifestBytes))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") != asset {
			continue
		}
		expected, err := hex.DecodeString(fields[0])
		if err != nil || len(expected) != sha256.Size {
			return fmt.Errorf("invalid checksum for %s in manifest", asset)
		}
		actual, err := hashFile(assetPath)
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, expected) {
			return fmt.Errorf("checksum mismatch for %s", asset)
		}
		return nil
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("no checksum found for %s", asset)
}

func hashFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func extractAsset(assetPath, asset, dest, expected string) error {
	switch {
	case strings.HasSuffix(asset, ".tar.gz"):
		return extractTarGz(assetPath, dest, expected)
	case strings.HasSuffix(asset, ".zip"):
		return extractZip(assetPath, dest, expected)
	default:
		return fmt.Errorf("unsupported update asset %q", asset)
	}
}

func extractTarGz(assetPath, dest, expected string) error {
	f, err := os.Open(assetPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		if baseName(hdr.Name) != expected {
			continue
		}
		if hdr.Size > maxAssetBytes {
			return fmt.Errorf("update binary exceeds %d bytes", maxAssetBytes)
		}
		if err := writeExecutable(dest, tr); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("archive does not contain %s", expected)
}

func extractZip(assetPath, dest, expected string) error {
	zr, err := zip.OpenReader(assetPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, entry := range zr.File {
		if entry.FileInfo().IsDir() || baseName(entry.Name) != expected {
			continue
		}
		if entry.UncompressedSize64 > uint64(maxAssetBytes) {
			return fmt.Errorf("update binary exceeds %d bytes", maxAssetBytes)
		}
		rc, err := entry.Open()
		if err != nil {
			return err
		}
		err = writeExecutable(dest, rc)
		closeErr := rc.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		return nil
	}
	return fmt.Errorf("archive does not contain %s", expected)
}

func writeExecutable(dest string, src io.Reader) error {
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(src, maxAssetBytes+1))
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if n > maxAssetBytes {
		return fmt.Errorf("update binary exceeds %d bytes", maxAssetBytes)
	}
	if n == 0 {
		return errors.New("update binary is empty")
	}
	return nil
}

func baseName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

func verifyRun(ctx context.Context, staged, version string) error {
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(runCtx, staged, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("staged binary failed --version: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), version) {
		return fmt.Errorf("staged binary reported %q, expected %q", strings.TrimSpace(string(out)), version)
	}
	return nil
}

// ExecutablePath returns the real path of the running executable, resolving
// symlinks so an installed link keeps pointing at the updated binary.
func ExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved, nil
	}
	return filepath.Abs(exe)
}
