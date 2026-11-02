package main

import (
	"fmt"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/surface"
)

// TestJourneyIMECompositionThenSubmit follows the input method through a real
// form: candidate updates reach the control and the accessibility tree, Enter
// while a candidate is marked must not navigate, and the committed text is what
// the server receives. A browser that submits a half-composed word is unusable
// for Vietnamese and Japanese typing regardless of how correct the rest is.
func TestJourneyIMECompositionThenSubmit(t *testing.T) {
	var reqMu sync.Mutex
	var submits int
	var received url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/apply" {
			reqMu.Lock()
			submits++
			received = r.URL.Query()
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="done">Applied</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body>
<form id="f" action="/apply" method="GET">
  <input type="text" id="q" name="q">
  <button type="submit" id="send">Apply</button>
</form>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/form"
	browser.navigateViaAddressBar(t, pageURL)
	browser.waitFor(t, "form control available in AX tree", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "q")
		return ok && browser.activeURL() == pageURL
	})

	browser.clickNode(t, "q")
	browser.waitFor(t, "input owns focus", func() bool {
		return isNodeFocused(browser.window.AccessibilityTree(), "q")
	})

	// Marked text is the live candidate. It must be visible to paint and AX
	// before it is committed, exactly as a user sees it while choosing.
	browser.inject(t, surface.Event{Kind: surface.EvIME, IME: surface.IMEMarked, Text: "tiế"})
	browser.waitFor(t, "first candidate is visible in the accessibility value", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "q")
		return ok && node.Value == "tiế"
	})
	browser.inject(t, surface.Event{Kind: surface.EvIME, IME: surface.IMEMarked, Text: "tiếng"})
	browser.waitFor(t, "refined candidate replaces the previous one", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "q")
		return ok && node.Value == "tiếng"
	})

	// Enter while composing belongs to the input method. It must not submit,
	// and the candidate must survive so the next edit still applies to it.
	browser.pressKey(t, '\r')
	browser.waitFor(t, "Enter during composition left the candidate intact", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "q")
		return ok && node.Value == "tiếng" && browser.activeURL() == pageURL
	})
	reqMu.Lock()
	midSubmits := submits
	reqMu.Unlock()
	if midSubmits != 0 {
		t.Fatalf("the server saw %d submission(s) before the composition was committed, want 0", midSubmits)
	}

	browser.inject(t, surface.Event{Kind: surface.EvIME, IME: surface.IMECommit, Text: "tiếng"})
	browser.waitFor(t, "committed text replaces the candidate", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "q")
		sess := browser.f.tabMgr.Active().Session()
		return ok && node.Value == "tiếng" && sess.Marked() == ""
	})

	// With the composition finished Enter is an ordinary implicit submission.
	browser.pressKey(t, '\r')
	browser.waitFor(t, "committed text reaches the server exactly once", func() bool {
		reqMu.Lock()
		defer reqMu.Unlock()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "done")
		return ok && submits == 1 && received.Get("q") == "tiếng"
	})

	reqMu.Lock()
	count := submits
	params := received
	reqMu.Unlock()
	if count != 1 {
		t.Fatalf("the server saw %d submissions, want exactly 1", count)
	}
	if got := params.Get("q"); got != "tiếng" {
		t.Fatalf("submitted q = %q, want the committed composition %q", got, "tiếng")
	}

	browser.waitFor(t, "tiles to finish rasterizing", func() bool {
		last := browser.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0
	})
}

// qc:BR-107
func TestJourneyOnlyActivatedSubmitterIsSent(t *testing.T) {
	var reqMu sync.Mutex
	var submissions []url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/save" {
			reqMu.Lock()
			submissions = append(submissions, r.URL.Query())
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="done">Saved</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body>
<form id="editor" action="/save" method="GET">
  <input type="text" id="title" name="title" value="draft">
  <button type="submit" id="btn_save" name="action" value="save">Save</button>
  <button type="submit" id="btn_publish" name="action" value="publish">Publish</button>
</form>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/form"
	openForm := func() {
		t.Helper()
		browser.navigateViaAddressBar(t, pageURL)
		browser.waitFor(t, "both submit buttons available in AX tree", func() bool {
			_, save := findAXNode(browser.window.AccessibilityTree(), "btn_save")
			_, publish := findAXNode(browser.window.AccessibilityTree(), "btn_publish")
			return save && publish && browser.activeURL() == pageURL
		})
	}

	// Clicking Save sends Save's own pair and not Publish's.
	openForm()
	browser.clickNode(t, "btn_save")
	browser.waitFor(t, "the save submission arrives", func() bool {
		reqMu.Lock()
		defer reqMu.Unlock()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "done")
		return ok && len(submissions) == 1
	})
	// Clicking Publish on a fresh page sends Publish's pair, proving the pair
	// follows the activation rather than being pinned to tree order.
	openForm()
	browser.clickNode(t, "btn_publish")
	browser.waitFor(t, "the publish submission arrives", func() bool {
		reqMu.Lock()
		defer reqMu.Unlock()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "done")
		return ok && len(submissions) == 2
	})

	reqMu.Lock()
	got := append([]url.Values(nil), submissions...)
	reqMu.Unlock()
	if len(got) != 2 {
		t.Fatalf("the server saw %d submissions, want exactly 2", len(got))
	}
	want := []string{"save", "publish"}
	for i, action := range want {
		params := got[i]
		if params.Get("action") != action {
			t.Errorf("submission %d action = %q, want %q", i+1, params.Get("action"), action)
		}
		if other := map[string]string{"save": "publish", "publish": "save"}[action]; params.Get("action") == other {
			t.Errorf("submission %d carried the other button's action", i+1)
		}
		if params.Get("title") != "draft" {
			t.Errorf("submission %d title = %q, want draft", i+1, params.Get("title"))
		}
	}

	browser.waitFor(t, "tiles to finish rasterizing", func() bool {
		last := browser.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0
	})
}

// TestJourneyInvalidStyleFollowsTyping covers the visible half of form
// validation. Author CSS commonly paints "input:invalid"; that state changes
// with the control's value, not with its box, so a repaint that reuses the old
// computed style leaves the error colour behind by exactly one keystroke - or
// forever, when the field never changes size.
func TestJourneyInvalidStyleFollowsTyping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><style>
input { width: 220px; height: 24px; padding: 0; border: 0; }
input:invalid { background-color: rgb(255, 0, 0); }
</style></head><body>
<form id="f" action="/never" method="GET">
  <input type="email" id="mail" name="mail">
</form>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/form"
	browser.navigateViaAddressBar(t, pageURL)
	browser.waitFor(t, "email control available in AX tree", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "mail")
		return ok && browser.activeURL() == pageURL
	})
	browser.clickNode(t, "mail")
	browser.waitFor(t, "email control owns focus", func() bool {
		return isNodeFocused(browser.window.AccessibilityTree(), "mail")
	})

	snapshotPath := filepath.Join(t.TempDir(), "frame.png")
	sample := func() (color.RGBA, bool) {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "mail")
		if !ok {
			return color.RGBA{}, false
		}
		if err := browser.window.WritePNG(snapshotPath); err != nil {
			return color.RGBA{}, false
		}
		f, err := os.Open(snapshotPath)
		if err != nil {
			return color.RGBA{}, false
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			return color.RGBA{}, false
		}
		x := int(node.X1) - 6
		y := int((node.Y0 + node.Y1) / 2)
		r, g, b, a := img.At(x, y).RGBA()
		return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}, true
	}
	red := func(c color.RGBA) bool { return c.R >= 200 && c.G < 80 && c.B < 80 }

	// An empty, non-required email field is valid: no error colour yet.
	browser.waitFor(t, "empty email field is not painted as invalid", func() bool {
		c, ok := sample()
		return ok && !red(c)
	})

	for _, ch := range "abc" {
		browser.pressKey(t, ch)
	}
	browser.waitFor(t, "typing an invalid address repaints the error background", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "mail")
		if !ok || node.Value != "abc" {
			return false
		}
		c, ok := sample()
		return ok && red(c)
	})

	for i := 0; i < 3; i++ {
		browser.pressKey(t, 0x7f)
	}
	for _, ch := range "a@b.co" {
		browser.pressKey(t, ch)
	}
	deadline := time.Now().Add(e2eWaitLimit)
	var typed string
	for {
		if node, ok := findAXNode(browser.window.AccessibilityTree(), "mail"); ok {
			typed = node.Value
		}
		if typed == "a@b.co" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("typed value = %q, want a@b.co", typed)
		}
		time.Sleep(10 * time.Millisecond)
	}
	deadline = time.Now().Add(e2eWaitLimit)
	for {
		c, ok := sample()
		if ok && !red(c) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a valid address is still painted with the error background %v (value %q)", c, browser.f.tabMgr.Active().Session().FocusedText())
		}
		time.Sleep(20 * time.Millisecond)
	}

	browser.waitFor(t, "tiles to finish rasterizing", func() bool {
		last := browser.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0
	})
}
