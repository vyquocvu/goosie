// Package entrypoints verifies that both real-document executables enforce
// the guarded rendering contracts: viewport validation, bounded input, tile
// cache budgeting, and checked painting.
package entrypoints

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles the named command and returns its path.
func buildBinary(t *testing.T, name string) string {
	t.Helper()
	tmp := t.TempDir()
	bin := filepath.Join(tmp, name)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/"+name)
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, out)
	}
	return bin
}

// TestGoosieHeadlessRejectsInvalidViewport verifies that goosie-headless
// validates viewport dimensions before attempting to render.
func TestGoosieHeadlessRejectsInvalidViewport(t *testing.T) {
	bin := buildBinary(t, "goosie-headless")

	tmp := t.TempDir()
	html := filepath.Join(tmp, "test.html")
	if err := os.WriteFile(html, []byte("<html><body>test</body></html>"), 0644); err != nil {
		t.Fatal(err)
	}
	png := filepath.Join(tmp, "out.png")

	cases := []struct {
		name string
		args []string
	}{
		{"zero width", []string{"-in", html, "-out", png, "-width", "0"}},
		{"negative height", []string{"-in", html, "-out", png, "-height", "-1"}},
		{"dpr zero", []string{"-in", html, "-out", png, "-dpr", "0"}},
		{"dpr too large", []string{"-in", html, "-out", png, "-dpr", "9"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("%s succeeded, want error", tc.name)
			}
			// Just verify it failed - the error message varies.
			if len(out) == 0 {
				t.Errorf("expected error output")
			}
		})
	}
}

// TestGoosieHeadlessRejectsOversizedDocument verifies that goosie-headless
// rejects documents whose rendered extent exceeds tile limits.
func TestGoosieHeadlessRejectsOversizedDocument(t *testing.T) {
	bin := buildBinary(t, "goosie-headless")

	tmp := t.TempDir()
	html := filepath.Join(tmp, "oversized.html")
	content := `<!DOCTYPE html><html><body><div style="width:65537px;height:65536px;background:red"></div></body></html>`
	if err := os.WriteFile(html, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	png := filepath.Join(tmp, "out.png")

	cmd := exec.Command(bin, "-in", html, "-out", png)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("oversized document succeeded, want error\noutput: %s", out)
	}
}

// TestGoosieHeadlessValidDocument verifies that a valid small document renders
// successfully through the guarded pipeline.
func TestGoosieHeadlessValidDocument(t *testing.T) {
	bin := buildBinary(t, "goosie-headless")

	tmp := t.TempDir()
	html := filepath.Join(tmp, "valid.html")
	content := `<!DOCTYPE html><html><body><div style="width:100px;height:100px;background:blue">test</div></body></html>`
	if err := os.WriteFile(html, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	png := filepath.Join(tmp, "out.png")

	cmd := exec.Command(bin, "-in", html, "-out", png)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("valid document failed: %v\n%s", err, out)
	}

	if _, err := os.Stat(png); err != nil {
		t.Errorf("PNG not created: %v", err)
	}
}

// TestGoosieScreenshotRejectsInvalidViewport verifies that goosie -screenshot
// validates viewport dimensions before attempting to render.
func TestGoosieScreenshotRejectsInvalidViewport(t *testing.T) {
	bin := buildBinary(t, "goosie")

	tmp := t.TempDir()
	png := filepath.Join(tmp, "out.png")

	// Start a test server to serve a minimal HTML page.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>test</body></html>"))
	}))
	defer server.Close()

	cases := []struct {
		name string
		args []string
	}{
		{"zero width", []string{"-screenshot", "-url", server.URL, "-out", png, "-width", "0"}},
		{"negative height", []string{"-screenshot", "-url", server.URL, "-out", png, "-height", "-1"}},
		{"dpr NaN", []string{"-screenshot", "-url", server.URL, "-out", png, "-dpr", "NaN"}},
		{"dpr too large", []string{"-screenshot", "-url", server.URL, "-out", png, "-dpr", "9"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("%s succeeded, want error", tc.name)
			}
			if len(out) == 0 {
				t.Errorf("expected error output")
			}
		})
	}
}

// TestGoosieScreenshotValidDocument verifies that goosie -screenshot renders
// a valid document successfully.
func TestGoosieScreenshotValidDocument(t *testing.T) {
	bin := buildBinary(t, "goosie")

	tmp := t.TempDir()
	png := filepath.Join(tmp, "out.png")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><div style="width:100px;height:100px;background:red">test</div></body></html>`))
	}))
	defer server.Close()

	cmd := exec.Command(bin, "-screenshot", "-url", server.URL, "-out", png, "-width", "800", "-height", "600")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("screenshot failed: %v\n%s", err, out)
	}

	if _, err := os.Stat(png); err != nil {
		t.Errorf("PNG not created: %v", err)
	}
}

// TestGoosieHeadlessRejectsOversizedInput verifies that goosie-headless
// rejects documents that exceed the byte limit.
func TestGoosieHeadlessRejectsOversizedInput(t *testing.T) {
	bin := buildBinary(t, "goosie-headless")

	tmp := t.TempDir()
	html := filepath.Join(tmp, "huge.html")
	// Create a file just over 8 MiB.
	hugeContent := strings.Repeat("x", 8<<20+1)
	if err := os.WriteFile(html, []byte(hugeContent), 0644); err != nil {
		t.Fatal(err)
	}
	png := filepath.Join(tmp, "out.png")

	cmd := exec.Command(bin, "-in", html, "-out", png)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("oversized input succeeded, want error\noutput: %s", out)
	}
	if !strings.Contains(string(out), "document exceeds") {
		t.Errorf("error %q does not mention document size limit", string(out))
	}
}
