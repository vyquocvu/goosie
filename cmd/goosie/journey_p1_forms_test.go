package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
)

func isNodeFocused(nodes []ax.Node, id string) bool {
	node, ok := findAXNode(nodes, id)
	return ok && node.Focused
}

// qc:BR-097
func TestJourneyTextInputEditingAndCaret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body>
<form id="editform">
  <input type="text" id="txtinput" name="field" value="abc">
</form>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/input.html"
	browser.navigateViaAddressBar(t, pageURL)

	browser.waitFor(t, "input element available in AX tree", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "txtinput")
		return ok && browser.activeURL() == pageURL && node.Value == "abc"
	})

	browser.clickNode(t, "txtinput")
	browser.waitFor(t, "input focused and initial value loaded", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "txtinput")
		return ok && node.Focused && node.Value == "abc"
	})

	browser.pressKey(t, 'd')
	browser.pressKey(t, 'e')
	browser.waitFor(t, "typing appended characters", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "txtinput")
		return ok && node.Value == "abcde"
	})

	browser.pressKey(t, 0x7f)
	browser.waitFor(t, "backspace removed last character", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "txtinput")
		return ok && node.Value == "abcd"
	})

	browser.pressKey(t, 0xF702)
	browser.pressKey(t, 'X')
	browser.waitFor(t, "character inserted at caret position", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "txtinput")
		return ok && node.Value == "abcXd"
	})

	browser.pressKey(t, 0xF704)
	browser.pressKey(t, 'Z')
	browser.waitFor(t, "character inserted at start", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "txtinput")
		return ok && node.Value == "ZabcXd"
	})

	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: 'a', Mods: surface.ModCommand})
	browser.pressKey(t, 'Q')
	browser.waitFor(t, "select-all and typing replaced the whole value", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "txtinput")
		return ok && node.Value == "Q"
	})

	inpNode, ok := findAXNode(browser.window.AccessibilityTree(), "txtinput")
	if !ok {
		t.Fatal("txtinput AXNode missing")
	}
	browser.inject(t, surface.Event{
		Kind: surface.EvAXSetValue,
		Pos:  frame.Point{X: int32((inpNode.X0 + inpNode.X1) / 2), Y: int32((inpNode.Y0 + inpNode.Y1) / 2)},
		Text: "final",
	})
	browser.waitFor(t, "select-all and replace updated value", func() bool {
		node, found := findAXNode(browser.window.AccessibilityTree(), "txtinput")
		return found && node.Value == "final"
	})

	browser.waitFor(t, "tiles to finish rasterizing", func() bool {
		last := browser.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0
	})
}

// qc:BR-098
func TestJourneyTextareaMultilineInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body>
<form id="taform">
  <textarea id="myta" name="bio"></textarea>
</form>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/textarea.html"
	browser.navigateViaAddressBar(t, pageURL)

	browser.waitFor(t, "textarea available in AX tree", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "myta")
		return ok && browser.activeURL() == pageURL
	})

	browser.clickNode(t, "myta")
	browser.waitFor(t, "textarea focused", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "myta")
		return ok && node.Focused
	})

	for _, ch := range "line1" {
		browser.pressKey(t, ch)
	}
	browser.pressKey(t, '\r')

	for _, ch := range "line2" {
		browser.pressKey(t, ch)
	}
	browser.pressKey(t, '\r')

	for _, ch := range "line3" {
		browser.pressKey(t, ch)
	}

	browser.waitFor(t, "textarea contains multiline text with newlines", func() bool {
		node, ok := findAXNode(browser.window.AccessibilityTree(), "myta")
		return ok && node.Value == "line1\nline2\nline3"
	})

	node, _ := findAXNode(browser.window.AccessibilityTree(), "myta")
	lines := strings.Split(node.Value, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), node.Value)
	}
	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Fatalf("unexpected line contents: %v", lines)
	}

	browser.pressKey(t, '\r')
	for _, ch := range "end" {
		browser.pressKey(t, ch)
	}
	browser.waitFor(t, "textarea appended fourth line", func() bool {
		n, ok := findAXNode(browser.window.AccessibilityTree(), "myta")
		return ok && n.Value == "line1\nline2\nline3\nend"
	})

	allNode, _ := findAXNode(browser.window.AccessibilityTree(), "myta")
	allLines := strings.Split(allNode.Value, "\n")
	if len(allLines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(allLines))
	}
	if allLines[3] != "end" {
		t.Fatalf("unexpected fourth line: %v", allLines)
	}

	browser.waitFor(t, "tiles to finish rasterizing", func() bool {
		last := browser.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0
	})
}

// qc:BR-099
func TestJourneyCheckboxToggleClickAndSpace(t *testing.T) {
	var reqMu sync.Mutex
	var lastEvents string
	var lastChecked bool
	var submitCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/chk-submit" {
			reqMu.Lock()
			submitCount++
			lastEvents = r.URL.Query().Get("events")
			lastChecked = r.URL.Query().Has("agreed")
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="done">Done</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body>
<form id="chkform" action="/chk-submit" method="GET">
  <input type="checkbox" id="chk" name="agreed" value="yes">
  <input type="hidden" id="ev_count" name="events" value="0">
  <button type="submit" id="sub_chk">Submit</button>
</form>
<script>
var c = 0;
var chk = document.getElementById("chk");
chk.addEventListener("change", function() {
  c++;
  document.getElementById("ev_count").setAttribute("value", String(c));
});
</script>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/checkbox.html"
	browser.navigateViaAddressBar(t, pageURL)

	browser.waitFor(t, "form loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "sub_chk")
		return ok && browser.activeURL() == pageURL
	})

	browser.clickElementByID(t, "chk")

	browser.pressKey(t, '\t')
	browser.pressKey(t, ' ')

	browser.clickElementByID(t, "chk")

	browser.clickNode(t, "sub_chk")
	browser.waitFor(t, "server received form submission", func() bool {
		reqMu.Lock()
		defer reqMu.Unlock()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "done")
		return ok && submitCount == 1 && lastEvents == "3" && lastChecked == true
	})

	reqMu.Lock()
	count := submitCount
	events := lastEvents
	checked := lastChecked
	reqMu.Unlock()

	if count != 1 {
		t.Fatalf("submitCount = %d, want 1", count)
	}
	if events != "3" {
		t.Fatalf("events count = %q, want 3", events)
	}
	if !checked {
		t.Fatalf("expected agreed checkbox to be submitted as checked")
	}

	browser.waitFor(t, "tiles to finish rasterizing", func() bool {
		last := browser.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0
	})
}

// qc:BR-100
func TestJourneyRadioGroupSelection(t *testing.T) {
	var reqMu sync.Mutex
	var submittedValues []string
	var submitCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/submit" {
			reqMu.Lock()
			submitCount++
			submittedValues = r.URL.Query()["color"]
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="done">Done</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body>
<form id="colorform" action="/submit" method="GET">
  <input type="radio" id="r_red" name="color" value="red" checked>
  <input type="radio" id="r_green" name="color" value="green">
  <input type="radio" id="r_blue" name="color" value="blue">
  <button type="submit" id="subbtn">Submit</button>
</form>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/radios.html"
	browser.navigateViaAddressBar(t, pageURL)

	browser.waitFor(t, "form loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "subbtn")
		return ok && browser.activeURL() == pageURL
	})

	browser.clickElementByID(t, "r_green")

	browser.clickNode(t, "subbtn")
	browser.waitFor(t, "navigation to /submit completes", func() bool {
		reqMu.Lock()
		defer reqMu.Unlock()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "done")
		return ok && submitCount == 1 && len(submittedValues) == 1 && submittedValues[0] == "green"
	})

	reqMu.Lock()
	count := submitCount
	values := submittedValues
	reqMu.Unlock()

	if count != 1 {
		t.Errorf("submitCount = %d, want 1", count)
	}
	if len(values) != 1 || values[0] != "green" {
		t.Errorf("submitted color values = %v, want exactly ['green']", values)
	}

	browser.waitFor(t, "tiles to finish rasterizing", func() bool {
		last := browser.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0
	})
}

// qc:BR-101
func TestJourneySelectOptionChange(t *testing.T) {
	var reqMu sync.Mutex
	var submits int
	var received url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/picked" {
			reqMu.Lock()
			submits++
			received = r.URL.Query()
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="done">Picked</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body style="margin: 0;">`+
			`<form id="f" action="/picked" method="GET">`+
			`<select id="sel" name="opt">`+
			`<option value="1">One</option>`+
			`<option value="2">Two</option>`+
			`<option value="3">Three</option>`+
			`</select>`+
			`<input type="submit" id="send" value="Send">`+
			`</form>`+
			`<div style="height: 1200px;"></div>`+
			`</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/pick.html"
	browser.navigateViaAddressBar(t, pageURL)

	browser.waitFor(t, "page loaded and the form is in the accessibility tree", func() bool {
		if browser.activeURL() != pageURL {
			return false
		}
		_, ok := findAXNode(browser.window.AccessibilityTree(), "send")
		return ok
	})

	focusOnSelect := func() bool {
		tab := browser.f.tabMgr.Active()
		if tab == nil || tab.Session() == nil {
			return false
		}
		n := tab.Session().Focused()
		return n != nil && n.Element() && n.Data == "select"
	}

	browser.clickNode(t, "sel")
	browser.waitFor(t, "select owns focus", focusOnSelect)

	scrollAtFocus := browser.f.sched.Viewport().Offset.Y
	rasterAtFocus := browser.f.loop.Stats().Rasterize
	sess := browser.f.tabMgr.Active().Session()
	selectedValue := sess.FocusedSelectValue

	record := func(key rune, name, want string) {
		browser.pressKey(t, key)
		browser.waitFor(t, name+" moves the selection to "+want+" without scrolling the page", func() bool {
			return selectedValue() == want &&
				browser.f.sched.Viewport().Offset.Y == scrollAtFocus &&
				focusOnSelect()
		})
	}

	if got := selectedValue(); got != "1" {
		t.Fatalf("before any key the select reports value %q, want 1", got)
	}
	record(0xF701, "ArrowDown", "2")
	record(0xF704, "Home", "1")
	record(0xF705, "End", "3")

	browser.clickNode(t, "send")

	browser.waitFor(t, "the moved option reaches the server", func() bool {
		reqMu.Lock()
		defer reqMu.Unlock()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "done")
		return ok && submits == 1 && received.Get("opt") != ""
	})

	reqMu.Lock()
	count := submits
	params := received
	reqMu.Unlock()

	if count != 1 {
		t.Fatalf("the server saw %d submissions, want exactly 1", count)
	}
	if got := params.Get("opt"); got != "3" {
		t.Fatalf("ArrowDown, Home, End then submit sent opt=%q, want opt=3: the last selection the user made must be the value sent", got)
	}
	if len(params) != 1 {
		t.Fatalf("submission carried %d parameters (%v), want only opt", len(params), params)
	}
	if got := browser.f.sched.Viewport().Offset.Y; got != scrollAtFocus {
		t.Fatalf("the select keys moved the viewport from %d to %d device px", scrollAtFocus, got)
	}
	if got := browser.f.loop.Stats().Rasterize; got == rasterAtFocus {
		t.Fatalf("no tile was rasterized across three selection changes (counter stayed at %d); the painted label cannot have followed the selection", got)
	}
}

// qc:BR-102
func TestJourneyFormTabNavigationOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><body>
<form id="tform">
  <input type="text" id="f1" value="one">
  <input type="text" id="f2" value="two">
  <textarea id="f3">three</textarea>
  <button type="button" id="f4">four</button>
</form>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/taborder.html"
	browser.navigateViaAddressBar(t, pageURL)

	browser.waitFor(t, "page controls available in AX tree", func() bool {
		tree := browser.window.AccessibilityTree()
		_, o1 := findAXNode(tree, "f1")
		_, o2 := findAXNode(tree, "f2")
		_, o3 := findAXNode(tree, "f3")
		_, o4 := findAXNode(tree, "f4")
		return o1 && o2 && o3 && o4 && browser.activeURL() == pageURL
	})

	browser.clickNode(t, "f1")
	browser.waitFor(t, "f1 focused", func() bool {
		return isNodeFocused(browser.window.AccessibilityTree(), "f1")
	})

	browser.pressKey(t, '\t')
	browser.waitFor(t, "tab moved focus to f2", func() bool {
		return isNodeFocused(browser.window.AccessibilityTree(), "f2")
	})

	browser.pressKey(t, '\t')
	browser.waitFor(t, "tab moved focus to f3", func() bool {
		return isNodeFocused(browser.window.AccessibilityTree(), "f3")
	})

	browser.pressKey(t, '\t')
	browser.waitFor(t, "tab moved focus to f4", func() bool {
		return isNodeFocused(browser.window.AccessibilityTree(), "f4")
	})

	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: '\t', Mods: surface.ModShift})
	browser.waitFor(t, "shift+tab moved focus back to f3", func() bool {
		return isNodeFocused(browser.window.AccessibilityTree(), "f3")
	})

	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: '\t', Mods: surface.ModShift})
	browser.waitFor(t, "shift+tab moved focus back to f2", func() bool {
		return isNodeFocused(browser.window.AccessibilityTree(), "f2")
	})

	browser.inject(t, surface.Event{Kind: surface.EvKey, Key: '\t', Mods: surface.ModShift})
	browser.waitFor(t, "shift+tab moved focus back to f1", func() bool {
		return isNodeFocused(browser.window.AccessibilityTree(), "f1")
	})

	browser.waitFor(t, "tiles to finish rasterizing", func() bool {
		last := browser.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0
	})
}

// qc:BR-103
func TestJourneyFormSubmitOnEnterKey(t *testing.T) {
	var reqMu sync.Mutex
	var submitCount int
	var receivedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/search" {
			reqMu.Lock()
			submitCount++
			receivedQuery = r.URL.Query().Get("q")
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="result-head">Search Results</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body>
<form id="searchform" action="/search" method="GET">
  <input type="text" id="queryinput" name="q" value="goosie-browser">
</form>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/searchpage.html"
	browser.navigateViaAddressBar(t, pageURL)

	browser.waitFor(t, "search input available in AX tree", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "queryinput")
		return ok && browser.activeURL() == pageURL
	})

	browser.clickNode(t, "queryinput")
	browser.waitFor(t, "queryinput focused", func() bool {
		return isNodeFocused(browser.window.AccessibilityTree(), "queryinput")
	})

	browser.pressKey(t, '\r')

	browser.waitFor(t, "search results loaded and server received submit exactly once", func() bool {
		reqMu.Lock()
		defer reqMu.Unlock()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "result-head")
		return ok && submitCount == 1 && receivedQuery == "goosie-browser"
	})

	reqMu.Lock()
	count := submitCount
	query := receivedQuery
	reqMu.Unlock()

	if count != 1 {
		t.Fatalf("expected server to receive form submission exactly once, got %d", count)
	}
	if query != "goosie-browser" {
		t.Fatalf("expected query parameter 'goosie-browser', got %q", query)
	}

	browser.waitFor(t, "tiles to finish rasterizing", func() bool {
		last := browser.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0
	})
}

// qc:BR-104
func TestJourneyFormSubmitOnButtonClick(t *testing.T) {
	var reqMu sync.Mutex
	var submitCount int
	var receivedParams url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/submit-all" {
			reqMu.Lock()
			submitCount++
			receivedParams = r.URL.Query()
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="success">Submission Succeeded</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body>
<form id="fullform" action="/submit-all" method="GET">
  <input type="hidden" name="token" value="secret123">
  <input type="text" id="uname" name="user" value="alice">
  <input type="checkbox" id="chk_agree" name="agree" value="yes" checked>
  <input type="checkbox" id="chk_skip" name="skip" value="no">
  <input type="radio" id="role_admin" name="role" value="admin" checked>
  <input type="radio" id="role_user" name="role" value="user">
  <textarea id="notes" name="notes">my notes</textarea>
  <button type="submit" id="btn_submit">Send</button>
</form>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/fullform.html"
	browser.navigateViaAddressBar(t, pageURL)

	browser.waitFor(t, "form submit button available in AX tree", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "btn_submit")
		return ok && browser.activeURL() == pageURL
	})

	browser.clickNode(t, "btn_submit")

	browser.waitFor(t, "destination loaded and parameters received", func() bool {
		reqMu.Lock()
		defer reqMu.Unlock()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "success")
		return ok && submitCount == 1 && len(receivedParams) == 5
	})

	reqMu.Lock()
	params := receivedParams
	count := submitCount
	reqMu.Unlock()

	if count != 1 {
		t.Fatalf("expected exactly 1 submission, got %d", count)
	}

	expected := map[string]string{
		"token": "secret123",
		"user":  "alice",
		"agree": "yes",
		"role":  "admin",
		"notes": "my notes",
	}

	if len(params) != len(expected) {
		t.Fatalf("expected %d parameters, got %d (%v)", len(expected), len(params), params)
	}

	for k, wantVal := range expected {
		if got := params.Get(k); got != wantVal {
			t.Errorf("param %q = %q, want %q", k, got, wantVal)
		}
	}

	if params.Has("skip") {
		t.Errorf("unchecked checkbox 'skip' should not be submitted, got %v", params["skip"])
	}
	if len(params["role"]) != 1 {
		t.Errorf("role count = %d, want 1", len(params["role"]))
	}

	browser.waitFor(t, "tiles to finish rasterizing", func() bool {
		last := browser.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0
	})
}

// qc:BR-106
func TestJourneyFormMethodAndEncoding(t *testing.T) {
	var reqMu sync.Mutex
	var receivedMethod string
	var receivedQuery url.Values
	var receivedRawQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/search-enc" {
			reqMu.Lock()
			receivedMethod = r.Method
			receivedQuery = r.URL.Query()
			receivedRawQuery = r.URL.RawQuery
			reqMu.Unlock()
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="done">Results</h1></body></html>`)
			return
		}
		fmt.Fprint(w, `<!doctype html><html><body>
<form id="encform" action="/search-enc" method="GET">
  <input type="text" id="q_input" name="query" value="goosie engine &amp; fast?">
  <input type="text" id="tag_input" name="tag" value="tiếng việt #1">
  <button type="submit" id="btn_search">Search</button>
</form>
</body></html>`)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)

	pageURL := server.URL + "/encoding.html"
	browser.navigateViaAddressBar(t, pageURL)

	browser.waitFor(t, "encoding form loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "btn_search")
		return ok && browser.activeURL() == pageURL
	})

	browser.clickNode(t, "btn_search")

	browser.waitFor(t, "results loaded and request captured", func() bool {
		reqMu.Lock()
		defer reqMu.Unlock()
		_, ok := findAXNode(browser.window.AccessibilityTree(), "done")
		return ok && receivedMethod == "GET" && len(receivedQuery) == 2
	})

	reqMu.Lock()
	m := receivedMethod
	q := receivedQuery
	raw := receivedRawQuery
	reqMu.Unlock()

	if m != "GET" {
		t.Errorf("HTTP method = %q, want GET", m)
	}

	if got := q.Get("query"); got != "goosie engine & fast?" {
		t.Errorf("decoded 'query' = %q, want %q", got, "goosie engine & fast?")
	}
	if got := q.Get("tag"); got != "tiếng việt #1" {
		t.Errorf("decoded 'tag' = %q, want %q", got, "tiếng việt #1")
	}

	if !strings.Contains(raw, "%26") {
		t.Errorf("raw query %q missing percent-encoded ampersand (%%26)", raw)
	}
	if !strings.Contains(raw, "%3F") {
		t.Errorf("raw query %q missing percent-encoded question mark (%%3F)", raw)
	}
	if !strings.Contains(raw, "%23") {
		t.Errorf("raw query %q missing percent-encoded hash (%%23)", raw)
	}

	browser.waitFor(t, "tiles to finish rasterizing", func() bool {
		last := browser.f.loop.Stats().Last
		return last.Needed == 0 && last.PaintingBytes == 0
	})
}
