package wpt

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeWPTTarball(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		header := entry.header
		if err := tw.WriteHeader(&header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if len(entry.body) != 0 {
			if _, err := tw.Write(entry.body); err != nil {
				t.Fatalf("write tar body: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

type tarEntry struct {
	header tar.Header
	body   []byte
}

func TestExtractWPTTarballSkipsPAXGlobalHeader(t *testing.T) {
	const root = "wpt-11ae8c1179be2dd38070846b01c2e9338ef81770"
	wptTarball := makeWPTTarball(t, []tarEntry{
		{header: tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"comment": "archive metadata"}}},
		{header: tar.Header{Name: root + "/", Typeflag: tar.TypeDir, Mode: 0o755}},
		{header: tar.Header{Name: root + "/html/index.html", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len("<!doctype html>"))}, body: []byte("<!doctype html>")},
	})

	dest := t.TempDir()
	if err := extractWPTTarball(bytes.NewReader(wptTarball), dest); err != nil {
		t.Fatalf("extract archive: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "html", "index.html"))
	if err != nil {
		t.Fatalf("read extracted WPT file: %v", err)
	}
	if string(got) != "<!doctype html>" {
		t.Fatalf("extracted content = %q", got)
	}
}

func TestExtractWPTTarballRejectsArchiveWithoutFiles(t *testing.T) {
	wptTarball := makeWPTTarball(t, []tarEntry{{
		header: tar.Header{Name: "wpt-empty/", Typeflag: tar.TypeDir, Mode: 0o755},
	}})
	if err := extractWPTTarball(bytes.NewReader(wptTarball), t.TempDir()); err == nil || !strings.Contains(err.Error(), "no regular files") {
		t.Fatalf("extract empty archive error = %v, want no-regular-files error", err)
	}
}

func TestExtractWPTTarballRejectsTraversal(t *testing.T) {
	wptTarball := makeWPTTarball(t, []tarEntry{{
		header: tar.Header{Name: "wpt/../outside.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len("escaped"))},
		body:   []byte("escaped"),
	}})
	dest := t.TempDir()
	if err := extractWPTTarball(bytes.NewReader(wptTarball), dest); err == nil || !strings.Contains(err.Error(), "escapes extraction directory") {
		t.Fatalf("extract traversal error = %v, want escape error", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "outside.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside path stat error = %v, want not-exist", err)
	}
}
