package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func downloadServer(t *testing.T, body string, contentType string, disposition string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		if disposition != "" {
			w.Header().Set("Content-Disposition", disposition)
		}
		_, _ = w.Write([]byte(body))
	}))
}

func screenshotDownloadArgs(url, out, dir string) []string {
	return []string{"-screenshot", "-url", url, "-out", out, "-download-dir", dir,
		"-backend", "headless", "-width", "600", "-height", "400", "-dpr", "1"}
}

// qc:BR-303
func TestScreenshotOfAttachmentReportsTheDownload(t *testing.T) {
	server := downloadServer(t, "%PDF-1.4 not a real pdf body", "application/pdf", `attachment; filename="doc.pdf"`)
	defer server.Close()
	out := filepath.Join(t.TempDir(), "shot.png")
	dir := t.TempDir()

	err := run(screenshotDownloadArgs(server.URL+"/doc.pdf", out, dir))
	if err == nil {
		t.Fatal("screenshot of an attachment returned no error")
	}
	if !errors.Is(err, errScreenshotDownload) {
		t.Fatalf("errors.Is(err, errScreenshotDownload) = false, want the download signal: %v", err)
	}
	if got := exitCodeFor(err); got != 3 {
		t.Fatalf("exitCodeFor(err) = %d, want 3", got)
	}
	msg := err.Error()
	for _, want := range []string{server.URL + "/doc.pdf", "Content-Disposition", "attachment", "doc.pdf"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message lost %q: %q", want, msg)
		}
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("screenshot file exists after a download route: %v", statErr)
	}
	saved, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("read download dir: %v", readErr)
	}
	if len(saved) != 1 {
		t.Fatalf("download dir holds %d entries, want the one saved attachment", len(saved))
	}
}

// qc:BR-303
func TestScreenshotOfTypedBinaryReportsTheDownload(t *testing.T) {
	server := downloadServer(t, "raw octet bytes", "application/octet-stream", "")
	defer server.Close()
	out := filepath.Join(t.TempDir(), "shot.png")

	err := run(screenshotDownloadArgs(server.URL+"/file.bin", out, t.TempDir()))
	if err == nil {
		t.Fatal("screenshot of a typed binary returned no error")
	}
	if !errors.Is(err, errScreenshotDownload) {
		t.Fatalf("want the download signal, got %v", err)
	}
	if got := exitCodeFor(err); got != 3 {
		t.Fatalf("exitCodeFor(err) = %d, want 3", got)
	}
	msg := err.Error()
	if !strings.Contains(msg, "Content-Type") || !strings.Contains(msg, "application/octet-stream") {
		t.Errorf("error should name the Content-Type that triggered the download: %q", msg)
	}
	if strings.Contains(msg, "Content-Disposition") {
		t.Errorf("no Content-Disposition was sent, but the error cites one: %q", msg)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("screenshot file exists after a download route: %v", statErr)
	}
}

// qc:BR-303
func TestScreenshotOfRenderedPageStaysSuccessful(t *testing.T) {
	server := downloadServer(t, `<!doctype html><html><body style="margin:0"><div style="width:400px;height:300px;margin:20px;background:rgb(200,0,0)"></div></body></html>`, "text/html; charset=utf-8", "")
	defer server.Close()
	out := filepath.Join(t.TempDir(), "shot.png")

	if err := run(screenshotDownloadArgs(server.URL+"/page.html", out, t.TempDir())); err != nil {
		t.Fatalf("screenshot of a rendered page failed: %v", err)
	}
	if got := ink(t, out); got < 100_000 {
		t.Fatalf("ink = %d px, want the 120,000 px box the page paints", got)
	}
}

// qc:BR-303
func TestExitCodeForDistinguishesFailureKinds(t *testing.T) {
	dl := screenshotDownloadError{dd: downloadDone{
		path:               filepath.Join(t.TempDir(), "doc.pdf"),
		url:                "http://example.test/doc.pdf",
		contentType:        "application/pdf",
		contentDisposition: `attachment; filename="doc.pdf"`,
	}}
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"usage", fmt.Errorf("%w: bad flag", errUsage), 2},
		{"download route", dl, 3},
		{"render failure", errors.New("goosie: build session: paint failed"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exitCodeFor(tc.err); got != tc.want {
				t.Errorf("exitCodeFor(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}
