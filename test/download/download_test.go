package download_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/download"
)

// qc:BR-149
func TestShouldDownloadAttachment(t *testing.T) {
	cases := []struct {
		ct, disp string
		want     bool
	}{
		{"text/html", "attachment; filename=a.bin", true},
		{"text/html", "", false},
		{"text/html", "inline", false},
		{"application/octet-stream", "", true},
		{"application/pdf", "", true},
		{"application/xhtml+xml", "", false},
		{"text/plain", "", false},
		{"text/html; charset=utf-8", "", false},
		{"", "", false},
		{"IMAGE/PNG", "", true},
	}
	for _, c := range cases {
		if got := download.ShouldDownload(c.ct, c.disp); got != c.want {
			t.Errorf("ShouldDownload(%q, %q) = %v, want %v", c.ct, c.disp, got, c.want)
		}
	}
}

// qc:BR-149
func TestFileNameFromDisposition(t *testing.T) {
	got := download.FileName(`attachment; filename="report.pdf"`, "https://x.com/blob")
	if got != "report.pdf" {
		t.Fatalf("FileName = %q, want report.pdf", got)
	}
}

func TestFileNameFromURLPath(t *testing.T) {
	got := download.FileName("", "https://x.com/files/archive.tar.gz?q=1")
	if got != "archive.tar.gz" {
		t.Fatalf("FileName = %q, want archive.tar.gz", got)
	}
}

// qc:BR-151
func TestFileNameFallbackAndSanitize(t *testing.T) {
	if got := download.FileName("", "https://x.com/"); got != "download" {
		t.Fatalf("FileName root URL = %q, want download", got)
	}
	if got := download.FileName(`attachment; filename="../../etc/passwd"`, "https://x.com/"); got != "passwd" {
		t.Fatalf("FileName traversal = %q, want passwd", got)
	}
	if got := download.FileName("", "https://x.com/a/b%2Fc.txt"); got != "c.txt" || filepath.Base(got) != got {
		t.Fatalf("FileName = %q, want a single sanitized component", got)
	}
	if got := download.FileName("attachment; filename=\"bad\x7fname.txt\"", "https://x.com/"); got != "badname.txt" {
		t.Fatalf("FileName control = %q, want badname.txt", got)
	}
	for _, name := range []string{".", ".."} {
		if got := download.FileName(`attachment; filename="`+name+`"`, "https://x.com/"); got != "download" {
			t.Fatalf("FileName(%q) = %q, want download", name, got)
		}
	}
}

func TestSaveWritesFile(t *testing.T) {
	dir := t.TempDir()
	p, err := download.Save(dir, "file.bin", []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(p) != dir {
		t.Fatalf("path %q outside dir %q", p, dir)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello" {
		t.Fatalf("content = %q, want hello", b)
	}
}

// qc:BR-151
func TestSaveCannotEscapeDownloadDirectory(t *testing.T) {
	cases := []struct{ name, want string }{
		{"../escape.txt", "escape.txt"},
		{`..\..\escape.txt`, "escape.txt"},
		{"sub/dir/file.txt", "file.txt"},
		{"bad\x01name.txt", "badname.txt"},
		{"bad\x7fname.txt", "badname.txt"},
		{"", "download"},
		{".", "download"},
		{"..", "download"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "downloads")
			path, err := download.Save(dir, tc.name, []byte("x"))
			if err != nil {
				t.Fatalf("Save(%q) = %v", tc.name, err)
			}
			if got := filepath.Dir(path); got != dir {
				t.Fatalf("Save(%q) wrote %q outside %q", tc.name, path, dir)
			}
			if base := filepath.Base(path); base != tc.want {
				t.Fatalf("Save(%q) base = %q, want %q", tc.name, base, tc.want)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("Save(%q) reported %q but nothing was written: %v", tc.name, path, err)
			}
			if _, err := os.Stat(filepath.Join(root, "escape.txt")); err == nil {
				t.Fatalf("Save(%q) wrote escape.txt beside the download directory", tc.name)
			}
		})
	}
}

// qc:BR-151
func TestSaveStreamCannotEscapeDownloadDirectory(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"../escape.txt", "escape.txt"},
		{`..\..\escape.txt`, "escape.txt"},
		{"sub/dir/file.txt", "file.txt"},
		{"bad\x7fname.txt", "badname.txt"},
		{".", "download"},
		{"..", "download"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "downloads")
			path, n, _, err := download.SaveStream(context.Background(), dir, tc.name, bytes.NewReader([]byte("x")), nil)
			if err != nil {
				t.Fatalf("SaveStream(%q) = %v", tc.name, err)
			}
			if n != 1 {
				t.Fatalf("SaveStream(%q) wrote %d bytes, want 1", tc.name, n)
			}
			if got := filepath.Dir(path); got != dir {
				t.Fatalf("SaveStream(%q) wrote %q outside %q", tc.name, path, dir)
			}
			if base := filepath.Base(path); base != tc.want {
				t.Fatalf("SaveStream(%q) base = %q, want %q", tc.name, base, tc.want)
			}
			if _, err := os.Stat(filepath.Join(root, "escape.txt")); err == nil {
				t.Fatalf("SaveStream(%q) wrote escape.txt beside the download directory", tc.name)
			}
		})
	}
}

// qc:BR-150
func TestSaveDeduplicatesNames(t *testing.T) {
	dir := t.TempDir()
	if _, err := download.Save(dir, "doc.pdf", []byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := download.Save(dir, "doc.pdf", []byte("b")); err != nil {
		t.Fatal(err)
	}
	p3, err := download.Save(dir, "doc.pdf", []byte("c"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p3) != "doc (2).pdf" {
		t.Fatalf("third save = %q, want doc (2).pdf", p3)
	}
	want := map[string]string{"doc.pdf": "a", "doc (1).pdf": "b", "doc (2).pdf": "c"}
	for name, content := range want {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
		if string(got) != content {
			t.Fatalf("%s content = %q, want %q (earlier download overwritten)", name, got, content)
		}
	}
}

// qc:BR-152
func TestSaveUnwritableDirectoryReturnsErrorAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(blocker, "downloads")

	if _, err := download.Save(dir, "file.bin", []byte("data")); err == nil {
		t.Fatal("Save into uncreatable dir returned nil error")
	}
	path, n, _, err := download.SaveStream(context.Background(), dir, "file.bin", bytes.NewReader([]byte("data")), nil)
	if err == nil {
		t.Fatalf("SaveStream into uncreatable dir returned nil error (path %q, %d bytes)", path, n)
	}
	if path != "" {
		t.Fatalf("SaveStream reported path %q after failure", path)
	}
	if _, statErr := os.Stat(dir); statErr == nil {
		t.Fatalf("download dir %q exists after failure", dir)
	}
	got, readErr := os.ReadFile(blocker)
	if readErr != nil || string(got) != "keep" {
		t.Fatalf("blocker content = %q (err %v), want keep", got, readErr)
	}

	recovered, err := download.Save(filepath.Join(root, "downloads"), "file.bin", []byte("later"))
	if err != nil {
		t.Fatalf("download after failure: %v", err)
	}
	if b, err := os.ReadFile(recovered); err != nil || string(b) != "later" {
		t.Fatalf("recovered download = %q (err %v), want later", b, err)
	}
}
