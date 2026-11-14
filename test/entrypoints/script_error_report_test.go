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

// qc:BR-190
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
	if !strings.Contains(got, server.URL) {
		t.Errorf("report %q does not name the document it came from", got)
	}
	if !strings.Contains(got, "createElement") {
		t.Errorf("report %q does not name the failure the page hit", got)
	}
}

// qc:BR-190
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
