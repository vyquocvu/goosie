package main

import (
	"fmt"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

// qc:BR-106
// TestJourneyPostFormCarriesBody covers the other half of BR-106: a POST
// submission must reach the server as a request body, not as a query string,
// and the response's cookie must outlive the submission. A browser that
// navigates to the action URL without the payload loses every login and
// checkout on the web.
func TestJourneyPostFormCarriesBody(t *testing.T) {
	var reqMu sync.Mutex
	var posted bool
	var method, contentType, rawBody string
	var requestQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/submit":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read body", http.StatusBadRequest)
				return
			}
			reqMu.Lock()
			posted = true
			method = r.Method
			contentType = r.Header.Get("Content-Type")
			rawBody = string(body)
			requestQuery = r.URL.Query()
			reqMu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "receipt", Value: "1", Path: "/"})
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="done">Posted</h1></body></html>`)
		case "/echo":
			if _, err := r.Cookie("receipt"); err != nil {
				fmt.Fprint(w, `<!doctype html><html><body><h1 id="missing">No cookie</h1></body></html>`)
				return
			}
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="echo">Cookie</h1></body></html>`)
		default:
			fmt.Fprint(w, `<!doctype html><html><body>
<form id="f" action="/submit" method="POST">
  <input type="text" id="who" name="who" value="Ada Lovelace">
  <input type="hidden" name="n" value="7">
  <button type="submit" id="send" name="go" value="1">Send</button>
</form>
</body></html>`)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/form"
	browser.navigateViaAddressBar(t, pageURL)
	browser.waitFor(t, "post form available in AX tree", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "send")
		return ok && browser.activeURL() == pageURL
	})

	browser.clickNode(t, "send")
	browser.waitFor(t, "the POST response replaces the form", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "done")
		return ok && browser.activeURL() == server.URL+"/submit"
	})

	reqMu.Lock()
	gotPosted := posted
	gotMethod, gotContentType, gotBody := method, contentType, rawBody
	gotQuery := requestQuery
	reqMu.Unlock()

	if !gotPosted {
		t.Fatal("the server never received the submitted form")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("submission method = %q, want POST", gotMethod)
	}
	if !strings.HasPrefix(gotContentType, "application/x-www-form-urlencoded") {
		t.Errorf("submission Content-Type = %q, want application/x-www-form-urlencoded", gotContentType)
	}
	if len(gotQuery) != 0 {
		t.Errorf("POST request carried query %v; the payload belongs in the body", gotQuery)
	}
	payload, err := url.ParseQuery(gotBody)
	if err != nil {
		t.Fatalf("submission body %q is not form-encoded: %v", gotBody, err)
	}
	for name, want := range map[string]string{"who": "Ada Lovelace", "n": "7", "go": "1"} {
		if got := payload.Get(name); got != want {
			t.Errorf("submitted %s = %q, want %q (body %q)", name, got, want, gotBody)
		}
	}

	// The response cookie is a normal cookie: the next navigation must send it.
	browser.navigateViaAddressBar(t, server.URL+"/echo")
	browser.waitFor(t, "the cookie set by the POST response reaches the next request", func() bool {
		_, seen := findAXNode(browser.window.AccessibilityTree(), "echo")
		return seen
	})
}

// qc:BR-106
// TestJourneyPostRedirectBecomesGet records the redirect rule that makes POST
// safe to follow: 301/302/303 turn the redirected request into a GET, while the
// POST itself still reaches the server with its body.
func TestJourneyPostRedirectBecomesGet(t *testing.T) {
	var reqMu sync.Mutex
	var methods, bodies []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submit":
			body, _ := io.ReadAll(r.Body)
			reqMu.Lock()
			methods = append(methods, r.Method)
			bodies = append(bodies, string(body))
			reqMu.Unlock()
			http.Redirect(w, r, "/landed", http.StatusFound)
		case "/landed":
			reqMu.Lock()
			methods = append(methods, r.Method)
			bodies = append(bodies, "")
			reqMu.Unlock()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="landed">Landed</h1></body></html>`)
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<!doctype html><html><body>
<form id="f" action="/submit" method="POST">
  <input type="text" id="q" name="q" value="hello">
  <button type="submit" id="go">Go</button>
</form>
</body></html>`)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/form"
	browser.navigateViaAddressBar(t, pageURL)
	browser.waitFor(t, "redirecting form available in AX tree", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "go")
		return ok && browser.activeURL() == pageURL
	})

	browser.clickNode(t, "go")
	browser.waitFor(t, "the redirect target is displayed under its own URL", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "landed")
		return ok && browser.activeURL() == server.URL+"/landed"
	})

	reqMu.Lock()
	gotMethods := append([]string(nil), methods...)
	gotBodies := append([]string(nil), bodies...)
	reqMu.Unlock()

	if len(gotMethods) != 2 {
		t.Fatalf("the redirect chain produced %d requests (%v), want 2", len(gotMethods), gotMethods)
	}
	if gotMethods[0] != http.MethodPost {
		t.Errorf("first request method = %q, want POST", gotMethods[0])
	}
	if gotBodies[0] != "q=hello" {
		t.Errorf("first request body = %q, want q=hello", gotBodies[0])
	}
	if gotMethods[1] != http.MethodGet {
		t.Errorf("redirected request method = %q, want GET", gotMethods[1])
	}
	if gotBodies[1] != "" {
		t.Errorf("redirected request body = %q, want empty", gotBodies[1])
	}
}

// qc:BR-106
// TestJourneySubmitterOverridesDestination follows a form whose two submit
// buttons lead to different places with different methods. The activated
// button decides, exactly as sites rely on for "Save draft" versus "Publish".
func TestJourneySubmitterOverridesDestination(t *testing.T) {
	var reqMu sync.Mutex
	var savedMethod, savedBody string
	var publishedQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/saved":
			body, _ := io.ReadAll(r.Body)
			reqMu.Lock()
			savedMethod = r.Method
			savedBody = string(body)
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="saved">Saved</h1></body></html>`)
		case "/published":
			reqMu.Lock()
			publishedQuery = r.URL.Query()
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="published">Published</h1></body></html>`)
		default:
			fmt.Fprint(w, `<!doctype html><html><body>
<form id="f" action="/published" method="GET">
  <input type="text" id="title" name="title" value="draft">
  <button type="submit" id="save" name="action" value="save" formaction="/saved" formmethod="POST">Save</button>
  <button type="submit" id="publish" name="action" value="publish">Publish</button>
</form>
</body></html>`)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/form"
	openForm := func() {
		t.Helper()
		browser.navigateViaAddressBar(t, pageURL)
		browser.waitFor(t, "both destination buttons available in AX tree", func() bool {
			_, save := findAXNode(browser.window.AccessibilityTree(), "save")
			_, publish := findAXNode(browser.window.AccessibilityTree(), "publish")
			return save && publish && browser.activeURL() == pageURL
		})
	}

	openForm()
	browser.clickNode(t, "save")
	browser.waitFor(t, "the save button's own destination is displayed", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "saved")
		return ok && browser.activeURL() == server.URL+"/saved"
	})

	openForm()
	browser.clickNode(t, "publish")
	browser.waitFor(t, "the publish button falls back to the form's destination", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "published")
		return ok && strings.HasPrefix(browser.activeURL(), server.URL+"/published?")
	})

	reqMu.Lock()
	gotSavedMethod, gotSavedBody := savedMethod, savedBody
	gotPublishedQuery := publishedQuery
	reqMu.Unlock()

	if gotSavedMethod != http.MethodPost {
		t.Errorf("save method = %q, want POST from formmethod", gotSavedMethod)
	}
	saved, err := url.ParseQuery(gotSavedBody)
	if err != nil {
		t.Fatalf("save body %q is not form-encoded: %v", gotSavedBody, err)
	}
	if saved.Get("action") != "save" || saved.Get("title") != "draft" {
		t.Errorf("save body = %q, want action=save and title=draft", gotSavedBody)
	}
	if got := gotPublishedQuery.Get("action"); got != "publish" {
		t.Errorf("publish action = %q, want publish", got)
	}
	if got := gotPublishedQuery.Get("title"); got != "draft" {
		t.Errorf("publish title = %q, want draft", got)
	}
}

// qc:BR-105
// TestJourneySubmitterNoValidateSkipsValidation covers the escape hatch sites
// use for "Skip" or "Save draft": an empty required field must not block the
// button that opts out with formnovalidate, while the normal submit stays on
// the page and focuses the missing value.
func TestJourneySubmitterNoValidateSkipsValidation(t *testing.T) {
	var reqMu sync.Mutex
	var draftQuery url.Values
	var submitted int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/draft":
			reqMu.Lock()
			draftQuery = r.URL.Query()
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="draft">Draft</h1></body></html>`)
		case "/submit":
			reqMu.Lock()
			submitted++
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="submitted">Submitted</h1></body></html>`)
		default:
			fmt.Fprint(w, `<!doctype html><html><body>
<form id="f" action="/submit" method="GET">
  <input type="text" id="q" name="q" required>
  <button type="submit" id="skip" name="skip" value="1" formaction="/draft" formnovalidate>Skip</button>
  <button type="submit" id="send">Send</button>
</form>
</body></html>`)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/form"
	browser.navigateViaAddressBar(t, pageURL)
	browser.waitFor(t, "validating form available in AX tree", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "send")
		return ok && browser.activeURL() == pageURL
	})

	// The ordinary submit button is still blocked by the empty required field.
	browser.clickNode(t, "send")
	browser.waitFor(t, "the invalid submit focuses the missing control", func() bool {
		return isNodeFocused(browser.window.AccessibilityTree(), "q")
	})
	reqMu.Lock()
	gotSubmitted := submitted
	reqMu.Unlock()
	if gotSubmitted != 0 || browser.activeURL() != pageURL {
		t.Fatalf("invalid submit reached %q after %d requests; want to stay on the form", browser.activeURL(), gotSubmitted)
	}

	// The activating button's formnovalidate lets the same data through.
	browser.clickNode(t, "skip")
	browser.waitFor(t, "the opt-out button reaches its own destination", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "draft")
		return ok && strings.HasPrefix(browser.activeURL(), server.URL+"/draft?")
	})

	reqMu.Lock()
	gotQuery := draftQuery
	gotSubmitted = submitted
	reqMu.Unlock()
	if gotSubmitted != 0 {
		t.Errorf("formnovalidate submission also hit the form action (%d times)", gotSubmitted)
	}
	if got := gotQuery.Get("skip"); got != "1" {
		t.Errorf("draft skip = %q, want 1", got)
	}
	if got := gotQuery.Get("q"); got != "" {
		t.Errorf("draft q = %q, want the empty required value", got)
	}
}

// qc:BR-106
// TestJourneyTextPlainEncoderReachesServer submits a text/plain form through the
// real browser path: the server must receive text/plain and the literal
// name=value lines, with no percent-encoding and CRLF line endings.
func TestJourneyTextPlainEncoderReachesServer(t *testing.T) {
	var reqMu sync.Mutex
	var posted bool
	var method, contentType, rawBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/draft" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read body", http.StatusBadRequest)
				return
			}
			reqMu.Lock()
			posted = true
			method = r.Method
			contentType = r.Header.Get("Content-Type")
			rawBody = string(body)
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="done">Draft saved</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body>
<form id="f" action="/draft" method="POST" enctype="text/plain">
  <input type="text" id="who" name="who" value="Ada Lovelace">
  <textarea id="note" name="note">line1
line2</textarea>
  <button type="submit" id="send" name="go" value="1">Save draft</button>
</form>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/form"
	browser.navigateViaAddressBar(t, pageURL)
	browser.waitFor(t, "text/plain form available in AX tree", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "send")
		return ok && browser.activeURL() == pageURL
	})

	browser.clickNode(t, "send")
	browser.waitFor(t, "the draft response replaces the form", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "done")
		return ok && browser.activeURL() == server.URL+"/draft"
	})

	reqMu.Lock()
	gotPosted := posted
	gotMethod, gotContentType, gotBody := method, contentType, rawBody
	reqMu.Unlock()

	if !gotPosted {
		t.Fatal("the server never received the submitted form")
	}
	if gotMethod != http.MethodPost {
		t.Errorf("submission method = %q, want POST", gotMethod)
	}
	if gotContentType != "text/plain" {
		t.Errorf("submission Content-Type = %q, want text/plain", gotContentType)
	}
	want := "who=Ada Lovelace\r\nnote=line1\r\nline2\r\ngo=1\r\n"
	if gotBody != want {
		t.Errorf("submission body = %q, want %q", gotBody, want)
	}
}

// qc:BR-105
// TestJourneyUserInvalidFollowsBlockedSubmit covers the user-facing validation
// state: :invalid paints immediately, a blocked submission promotes the field
// to :user-invalid, and correcting it flips the same field to :user-valid. The
// pixels prove the host re-resolved styles at each transition rather than
// reusing the previous computed style.
func TestJourneyUserInvalidFollowsBlockedSubmit(t *testing.T) {
	var reqMu sync.Mutex
	var submitted int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/never" {
			reqMu.Lock()
			submitted++
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="done">Sent</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><head><style>
input { width: 220px; height: 24px; padding: 0; border: 0; background-color: rgb(0, 0, 255); }
input:invalid { background-color: rgb(255, 0, 0); }
input:user-invalid { background-color: rgb(255, 165, 0); }
input:user-valid { background-color: rgb(0, 128, 0); }
</style></head><body>
<form id="f" action="/never" method="GET">
  <input type="email" id="mail" name="mail" required>
  <button type="submit" id="send">Send</button>
</form>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/form"
	browser.navigateViaAddressBar(t, pageURL)
	browser.waitFor(t, "required email control available in AX tree", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "mail")
		return ok && browser.activeURL() == pageURL
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
	orange := func(c color.RGBA) bool { return c.R >= 200 && c.G >= 100 && c.G < 200 && c.B < 80 }
	green := func(c color.RGBA) bool { return c.R < 80 && c.G >= 100 && c.B < 80 }

	browser.waitFor(t, "the empty required field starts as :invalid", func() bool {
		c, ok := sample()
		return ok && red(c)
	})

	browser.clickNode(t, "send")
	browser.waitFor(t, "a blocked submit promotes the field to :user-invalid", func() bool {
		c, ok := sample()
		return ok && orange(c)
	})
	reqMu.Lock()
	gotSubmitted := submitted
	reqMu.Unlock()
	if gotSubmitted != 0 {
		t.Fatalf("the server received %d request(s) for an invalid form, want 0", gotSubmitted)
	}
	if got := browser.activeURL(); got != pageURL {
		t.Fatalf("blocked submit navigated to %q, want to stay on %q", got, pageURL)
	}

	browser.clickNode(t, "mail")
	for _, ch := range "a@b.co" {
		browser.pressKey(t, ch)
	}
	browser.waitFor(t, "correcting the field flips it to :user-valid", func() bool {
		c, ok := sample()
		return ok && green(c)
	})

	// The form is now submittable, so the same button must navigate.
	browser.clickNode(t, "send")
	browser.waitFor(t, "the corrected form submits", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "done")
		return ok && strings.HasPrefix(browser.activeURL(), server.URL+"/never")
	})
}

// qc:BR-106
// TestJourneyMultipartFormCarriesSelectedFile drives a real multipart upload:
// page script selects a File, the host submit path assembles the body from the
// live FileList, and the server parses the same filename, content type, bytes
// and text fields. A URL-encoded fallback would be silently wrong here.
func TestJourneyMultipartFormCarriesSelectedFile(t *testing.T) {
	type upload struct {
		contentType string
		title       string
		filename    string
		fileType    string
		fileData    string
	}
	var reqMu sync.Mutex
	var got *upload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/upload" {
			record := &upload{contentType: r.Header.Get("Content-Type")}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				http.Error(w, "bad multipart: "+err.Error(), http.StatusBadRequest)
				return
			}
			record.title = r.FormValue("title")
			if files := r.MultipartForm.File["attachment"]; len(files) > 0 {
				header := files[0]
				record.filename = header.Filename
				record.fileType = header.Header.Get("Content-Type")
				file, err := header.Open()
				if err != nil {
					http.Error(w, "open part: "+err.Error(), http.StatusInternalServerError)
					return
				}
				data, err := io.ReadAll(file)
				file.Close()
				if err != nil {
					http.Error(w, "read part: "+err.Error(), http.StatusInternalServerError)
					return
				}
				record.fileData = string(data)
			}
			reqMu.Lock()
			got = record
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="done">Uploaded</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body>
<form id="f" action="/upload" method="POST" enctype="multipart/form-data">
  <input type="text" id="title" name="title" value="Report">
  <input type="file" id="upload" name="attachment">
  <button type="submit" id="send">Upload</button>
</form>
<script>
  (function(){
    var transfer = new DataTransfer();
    transfer.items.add(new File(["hello world"], "note.txt", {type: "text/plain"}));
    document.getElementById("upload").files = transfer.files;
  })();
</script>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/form"
	browser.navigateViaAddressBar(t, pageURL)
	browser.waitFor(t, "multipart form available in AX tree", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "title")
		return ok && browser.activeURL() == pageURL
	})

	browser.clickNode(t, "send")
	browser.waitFor(t, "the upload navigates to its result", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "done")
		return ok && strings.HasPrefix(browser.activeURL(), server.URL+"/upload")
	})

	reqMu.Lock()
	defer reqMu.Unlock()
	if got == nil {
		t.Fatal("the server never received the multipart request")
	}
	if !strings.HasPrefix(got.contentType, "multipart/form-data; boundary=") {
		t.Fatalf("Content-Type = %q, want multipart/form-data with a boundary", got.contentType)
	}
	if got.title != "Report" {
		t.Fatalf("title part = %q, want Report", got.title)
	}
	if got.filename != "note.txt" {
		t.Fatalf("filename = %q, want note.txt", got.filename)
	}
	if got.fileType != "text/plain" {
		t.Fatalf("file content type = %q, want text/plain", got.fileType)
	}
	if got.fileData != "hello world" {
		t.Fatalf("file bytes = %q, want hello world", got.fileData)
	}
}
