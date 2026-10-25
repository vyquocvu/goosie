package entrypoints

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// GOOSIE-LIVE-001 is not one bug about pixels: it is nine real pages that paint
// nothing with rc 0 and an empty stderr, so nothing downstream - the sweep's
// report, the operator, the bug - could say why. The engine held the reason
// (uncaught exceptions) and dropped it. These two cases pin the ends of the new
// path at the level the defect was observed: the binary, on a served document.
//
// The unit tests in ./cmd/goosie pin what reportScriptErrors writes; only this
// file can pin that the document load calls it at all.

func TestGoosieScreenshotReportsUncaughtScriptErrors(t *testing.T) {
	bin := buildBinary(t, "goosie")

	body := `<html><body><h1>Static heading</h1>` +
		`<script>var ce; ce.createElement("fieldset");</script></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	tmp := t.TempDir()
	png := filepath.Join(tmp, "out.png")
	var stderr bytes.Buffer
	cmd := exec.Command(bin, "-backend", "headless", "-width", "800", "-height", "600",
		"-dpr", "1", "-screenshot", "-out", png, "-url", server.URL+"/")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("screenshot run failed: %v\nstderr: %s", err, stderr.String())
	}
	if _, err := os.Stat(png); err != nil {
		t.Fatalf("no PNG written: reporting must not cost the page its pixels (%v)", err)
	}

	got := stderr.String()
	if !strings.Contains(got, "uncaught script error") {
		t.Fatalf("a document whose script threw produced no reason on stderr; stderr was %q", got)
	}
	if n := strings.Count(got, "uncaught script error"); n != 1 {
		t.Fatalf("report lines = %d, want exactly 1: %q", n, got)
	}
	// The line has to be actionable, which means naming both the document and
	// the script that broke, plus what the page reached for.
	if !strings.Contains(got, server.URL) {
		t.Errorf("report %q does not name the document it came from", got)
	}
	if !strings.Contains(got, "createElement") {
		t.Errorf("report %q does not name the failure the page hit", got)
	}
}

func TestGoosieScreenshotStaysSilentForACleanDocument(t *testing.T) {
	bin := buildBinary(t, "goosie")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><h1>Static heading</h1><script>ok = 1;</script></body></html>`))
	}))
	defer server.Close()

	tmp := t.TempDir()
	png := filepath.Join(tmp, "out.png")
	var stderr bytes.Buffer
	cmd := exec.Command(bin, "-backend", "headless", "-width", "800", "-height", "600",
		"-dpr", "1", "-screenshot", "-out", png, "-url", server.URL+"/")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("screenshot run failed: %v\nstderr: %s", err, stderr.String())
	}
	if strings.Contains(stderr.String(), "uncaught script error") {
		t.Fatalf("a clean document reported script errors on stderr: %q", stderr.String())
	}
}
