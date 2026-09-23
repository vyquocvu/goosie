package download_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/download"
)

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
	for _, name := range []string{"doc.pdf", "doc (1).pdf"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}
