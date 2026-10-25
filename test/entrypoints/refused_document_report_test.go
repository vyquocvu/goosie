package entrypoints

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The other branch of GOOSIE-LIVE-001's silence, and the bigger one: six of the
// eight zero-output pages were never a layout failure at all. The server refused
// Goosie - 403 for apnews.com, nytimes.com, japantimes.co.jp, etsy.com and
// newegg.com, 401 for reuters.com - and answered with a small interstitial
// document that the pipeline renders like any other page. rc 0, no stderr, and a
// sweep label that says "the engine laid out nothing" when the truth is "the
// source would not send the page".
//
// The status is on the response the loader already holds. These two cases pin
// both ends: a refusal names the status and the size of what was rendered
// instead of the page, and an accepted answer stays silent so the line keeps
// meaning something.

func TestGoosieScreenshotReportsARefusedDocument(t *testing.T) {
	bin := buildBinary(t, "goosie")

	body := `<html><body><h1>Just a moment...</h1></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
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
	// Reporting a refusal must not turn the run into a failure or cost the
	// interstitial its pixels: what is rendered is unchanged.
	if _, err := os.Stat(png); err != nil {
		t.Fatalf("no PNG written for a refused document: %v", err)
	}

	got := stderr.String()
	if !strings.Contains(got, "the source refused") {
		t.Fatalf("a 403 document reported no reason on stderr; stderr was %q", got)
	}
	if n := strings.Count(got, "the source refused"); n != 1 {
		t.Fatalf("refusal lines = %d, want exactly 1: %q", n, got)
	}
	line := ""
	for _, l := range strings.Split(got, "\n") {
		if strings.Contains(l, "the source refused") {
			line = l
			break
		}
	}
	for _, want := range []string{server.URL, "HTTP 403", fmt.Sprintf("%d bytes", len(body))} {
		if !strings.Contains(line, want) {
			t.Errorf("refusal line %q does not name %q", line, want)
		}
	}
}

func TestGoosieScreenshotStaysSilentOnAnAcceptedDocument(t *testing.T) {
	bin := buildBinary(t, "goosie")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body><h1>Served normally</h1></body></html>`))
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
	if strings.Contains(stderr.String(), "the source refused") {
		t.Fatalf("a 200 answer reported a refusal: %q", stderr.String())
	}
}
