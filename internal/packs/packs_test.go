package packs

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func samplePack(t *testing.T, id string, corpusData []byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	n, err := Write(&buf, Content{
		Manifest: Manifest{ID: id, Name: strings.ToUpper(id) + " Sample", Language: "English"},
		Corpus:   corpusData,
		Index:    []byte("index payload"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(buf.Len()) {
		t.Fatalf("Write reported %d bytes, buffered %d", n, buf.Len())
	}
	data := buf.Bytes()
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:])
}

func TestWriteDecodeAndVerify(t *testing.T) {
	data, sum := samplePack(t, "sample", []byte("corpus payload"))
	if got := SHA256(data); got != sum {
		t.Fatalf("SHA256 = %s, want %s", got, sum)
	}
	c, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if c.Manifest.ID != "sample" || c.Manifest.Format != Magic || c.Manifest.Version != FormatVersion {
		t.Fatalf("manifest = %+v", c.Manifest)
	}
	if string(c.Corpus) != "corpus payload" || string(c.Index) != "index payload" {
		t.Fatalf("decoded payloads = %q / %q", c.Corpus, c.Index)
	}

	dir := t.TempDir()
	if _, err := VerifyAndInstall(dir, data, sum); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(Path(dir, "sample")); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("installed file mismatch: %v", err)
	}
	ids, err := Installed(dir)
	if err != nil || len(ids) != 1 || ids[0] != "sample" {
		t.Fatalf("Installed = %v, %v", ids, err)
	}
	if err := Remove(dir, "sample"); err != nil {
		t.Fatal(err)
	}
	if ids, _ := Installed(dir); len(ids) != 0 {
		t.Fatalf("Installed after remove = %v", ids)
	}
}

func TestVerifyRejectsBadHashAndCorruptContent(t *testing.T) {
	data, sum := samplePack(t, "sample", []byte("corpus payload"))
	dir := t.TempDir()
	if _, err := VerifyAndInstall(dir, data, sum); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(Path(dir, "sample"))

	if _, err := VerifyAndInstall(dir, []byte("tampered"), sum); err == nil {
		t.Fatal("bad whole-file hash was accepted")
	}
	after, _ := os.ReadFile(Path(dir, "sample"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed install replaced an existing pack")
	}

	// A correct whole-file hash still cannot bypass internal section hashes.
	c, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := corruptManifestHash(t, data, c.Manifest.CorpusSHA256)
	badSum := sha256.Sum256(corrupt)
	if _, err := VerifyAndInstall(dir, corrupt, hex.EncodeToString(badSum[:])); err == nil {
		t.Fatal("corrupt corpus section was accepted")
	}
	after, _ = os.ReadFile(Path(dir, "sample"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed install modified an existing pack")
	}
}

// corruptManifestHash rewrites the uncompressed manifest so its corpus hash no
// longer matches the payload, then recompresses the whole pack.
func corruptManifestHash(t *testing.T, data []byte, hash string) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	_ = zr.Close()
	old := []byte(`"` + hash + `"`)
	newHash := []byte(`"` + strings.Repeat("0", len(hash)) + `"`)
	if !bytes.Contains(body, old) {
		t.Fatal("manifest hash not found in pack body")
	}
	body = bytes.Replace(body, old, newHash, 1)

	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	if _, err := zw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestDecodeRejectsTrailingDataAndWrongVersion(t *testing.T) {
	data, _ := samplePack(t, "sample", []byte("corpus payload"))
	if _, err := Decode(append(data, 0x00)); err == nil {
		t.Fatal("trailing data was accepted")
	}
	var buf bytes.Buffer
	if _, err := Write(&buf, Content{
		Manifest: Manifest{ID: "sample", Name: "Sample", Format: Magic, Version: FormatVersion + 1},
		Corpus:   []byte("x"),
		Index:    []byte("y"),
	}); err != nil {
		t.Fatal(err)
	}
	// Write forces the current version, so tamper with a decoded body instead.
	c, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if c.Manifest.Version != FormatVersion {
		t.Fatalf("Write did not force the format version: %d", c.Manifest.Version)
	}
	if got := filepath.Base(Path(t.TempDir(), "../web")); got != "web.tbp" {
		t.Fatalf("Path did not sanitize id: %s", got)
	}
}
