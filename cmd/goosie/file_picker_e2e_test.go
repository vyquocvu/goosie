package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
)

type fakeFilePicker struct {
	mu       sync.Mutex
	paths    []string
	chosen   bool
	err      error
	calls    int
	accept   string
	multiple bool
}

func (p *fakeFilePicker) ChooseFiles(accept string, multiple bool) ([]string, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.accept, p.multiple = accept, multiple
	return append([]string(nil), p.paths...), p.chosen, p.err
}

func (p *fakeFilePicker) selection() (paths []string, chosen bool, calls int, accept string, multiple bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.paths...), p.chosen, p.calls, p.accept, p.multiple
}

func (p *fakeFilePicker) setSelection(paths []string, chosen bool) {
	p.mu.Lock()
	p.paths, p.chosen = paths, chosen
	p.mu.Unlock()
}

func TestE2EFilePickerSelectionReachesScript(t *testing.T) {
	dir := t.TempDir()
	textPath := filepath.Join(dir, "picked.txt")
	binaryPath := filepath.Join(dir, "picked.bin")
	if err := os.WriteFile(textPath, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binaryPath, []byte{0x00, 0x01}, 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<!doctype html><html><body style="margin: 0">
<input id="upload" type="file" name="upload" multiple accept=".txt,.bin">
<p id="out">none</p>
<p id="count">0</p>
<script>
var input = document.getElementById('upload');
var out = document.getElementById('out');
var count = document.getElementById('count');
input.addEventListener('change', function() {
  out.textContent = 'change:' + Array.from(input.files).map(function(f) {
    return f.name + ':' + f.size + ':' + f.type;
  }).join(',');
  count.textContent = String(Number(count.textContent) + 1);
});
</script>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	cw, ok := browser.f.window.(*chromeWindow)
	if !ok {
		t.Fatalf("window type = %T, want *chromeWindow", browser.f.window)
	}
	picker := &fakeFilePicker{paths: []string{textPath, binaryPath}, chosen: true}
	cw.filePicker = picker

	browser.navigateViaAddressBar(t, server.URL+"/picker")
	browser.waitFor(t, "picker page text", func() bool {
		return axContainsLabel(browser.window.AccessibilityTree(), "none")
	})

	browser.clickElementByID(t, "upload")
	browser.waitFor(t, "picked files reach the change handler", func() bool {
		return axContainsLabel(browser.window.AccessibilityTree(),
			"change:picked.txt:3:text/plain,picked.bin:2:application/octet-stream")
	})
	_, _, calls, accept, multiple := picker.selection()
	if calls != 1 || accept != ".txt,.bin" || !multiple {
		t.Fatalf("picker call = calls=%d accept=%q multiple=%t, want 1/.txt,.bin/true", calls, accept, multiple)
	}

	picker.setSelection(nil, false)
	browser.clickElementByID(t, "upload")
	browser.waitFor(t, "second picker request", func() bool {
		_, _, calls, _, _ := picker.selection()
		return calls == 2
	})
	if node, found := findAXNode(browser.window.AccessibilityTree(), "count"); !found || axNodeText(node) != "1" {
		t.Fatalf("cancel changed the selection: count node = %+v (found %v)", node, found)
	}
}

func axNodeText(node ax.Node) string {
	text := node.Label + node.Value
	for _, child := range node.Children {
		text += axNodeText(child)
	}
	return text
}
