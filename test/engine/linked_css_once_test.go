package engine_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

// GOOSIE-LIVE-001 measured one navigation asking for 33 stylesheets 927 to
// 1,059 times (https://www.gov.vn, 2026-11-13, viewport 1280): every
// parser-blocking script re-collected the sheet set and every collection
// re-requested every sheet, so 9.6s to 29.8s of fetch work ran against the 6s
// per-pass budget and the links a pass never reached were asked for by the next
// one. Chromium issues 36 stylesheet requests for 36 distinct URLs on that
// page. These oracles pin the rule the fix installs: one request per URL per
// navigation, and a collection only when the document's stylesheet state or
// the media options moved.

// sheetServer answers the three URLs the fixtures link, and counts every
// request and attempt it sees. failOnce URLs refuse their first attempt, which
// is how the retry rule gets measured rather than assumed.
type sheetServer struct {
	mu       sync.Mutex
	requests map[string]int
	attempts map[string]int
	failOnce map[string]bool
	text     map[string]string
}

func newSheetServer(failOnce ...string) *sheetServer {
	s := &sheetServer{
		requests: map[string]int{},
		attempts: map[string]int{},
		failOnce: map[string]bool{},
		text: map[string]string{
			"https://example.test/a.css":     "p { color: #010000; }",
			"https://example.test/b.css":     "h1 { color: #000200; }",
			"https://example.test/c.css":     "p { color: #020000; }",
			"https://example.test/theme.css": "@import url(\"b.css\"); p { color: #010000; }",
		},
	}
	for _, u := range failOnce {
		s.failOnce[u] = true
	}
	return s
}

func (s *sheetServer) link(base, href string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[href]++
	s.attempts[href]++
	if s.failOnce[href] && s.attempts[href] == 1 {
		return "", fmt.Errorf("first attempt refused")
	}
	text, ok := s.text[href]
	if !ok {
		return "", fmt.Errorf("no sheet at %s", href)
	}
	return text, nil
}

func (s *sheetServer) count(href string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[href]
}

func (s *sheetServer) attempted(href string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempts[href]
}

func (s *sheetServer) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, v := range s.requests {
		n += v
	}
	return n
}

func newCSSRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	return rt
}

func newCSSSession(t *testing.T, rt *js.Runtime, html string, linker engine.CSSLinker) *engine.Session {
	t.Helper()
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithJS(rt),
		engine.WithLinkedCSS("https://example.test/", linker))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}

// colorOf reads one tag's resolved colour, failing the test when the tag is
// missing or unresolved.
func colorOf(t *testing.T, sess *engine.Session, tag string) (int, int, int) {
	t.Helper()
	nodes := sess.Doc.ElementsByTagName(tag)
	if len(nodes) != 1 {
		t.Fatalf("found %d <%s> nodes, want 1", len(nodes), tag)
	}
	st, ok := sess.Styles[nodes[0].ID]
	if !ok {
		t.Fatalf("<%s> has no resolved style", tag)
	}
	return int(st.Color.R), int(st.Color.G), int(st.Color.B)
}

// appendedStyle is the script mutation that makes a collection pass legitimate:
// a new style element changes the document's stylesheet state, so the next
// script's collection has to run and has to see the sheets it already holds.
func appendedStyle(marker, decl string) string {
	return fmt.Sprintf("<script>var %s = document.createElement('style'); %s.textContent = '%s'; document.head.appendChild(%s);</script>", marker, marker, decl, marker)
}

// TestLinkedSheetsRequestedOnceAcrossScriptPasses is the storm itself: three
// sheets plus two scripts that each add a style element, so more than one
// collection pass runs and every one of them used to ask for every sheet. One
// request per URL is Chromium's measured profile, and all four declarations
// must still cascade - a memo that traded fewer requests for missing rules
// fails here too.
// qc:BR-282
func TestLinkedSheetsRequestedOnceAcrossScriptPasses(t *testing.T) {
	srv := newSheetServer()
	html := `<html><head>
<link rel="stylesheet" href="/a.css">
<link rel="stylesheet" href="/b.css">
` + appendedStyle("s1", "div { color: #050000 }") + `
<link rel="stylesheet" href="/c.css">
` + appendedStyle("s2", "div { color: #060000 }") + `
</head><body><p>x</p><h1>y</h1><div>z</div></body></html>`
	sess := newCSSSession(t, newCSSRuntime(t), html, srv.link)

	for _, href := range []string{
		"https://example.test/a.css",
		"https://example.test/b.css",
		"https://example.test/c.css",
	} {
		if got := srv.count(href); got != 1 {
			t.Errorf("%s requested %d times, want 1 (one request per URL per navigation)", href, got)
		}
	}
	if got := srv.total(); got != 3 {
		t.Errorf("total stylesheet requests = %d, want 3", got)
	}
	// Document order is a.css, b.css, s1, c.css, s2: c.css is the last sheet
	// declaring p, the injected styles only declare div, and b.css only h1.
	if r, g, b := colorOf(t, sess, "p"); r != 2 || g != 0 || b != 0 {
		t.Errorf("p color = (%d,%d,%d), want (2,0,0) from c.css", r, g, b)
	}
	if r, g, b := colorOf(t, sess, "h1"); r != 0 || g != 2 || b != 0 {
		t.Errorf("h1 color = (%d,%d,%d), want (0,2,0) from b.css", r, g, b)
	}
	if r, g, b := colorOf(t, sess, "div"); r != 6 || g != 0 || b != 0 {
		t.Errorf("div color = (%d,%d,%d), want (6,0,0) from the last injected style", r, g, b)
	}
}

// TestDuplicateLinkedSheetHrefRequestedOnce pins the memo against the shape a
// real CMS emits - the same stylesheet linked twice. Chromium dedupes the
// request while keeping both link elements, so one fetch with the rule applied
// is the only passing shape.
// qc:BR-282
func TestDuplicateLinkedSheetHrefRequestedOnce(t *testing.T) {
	srv := newSheetServer()
	html := `<html><head>
<link rel="stylesheet" href="/a.css">
<link rel="stylesheet" href="/a.css">
</head><body><p>x</p></body></html>`
	sess := newCSSSession(t, newCSSRuntime(t), html, srv.link)

	if got := srv.count("https://example.test/a.css"); got != 1 {
		t.Fatalf("a.css requested %d times, want 1 for two identical hrefs", got)
	}
	if r, g, b := colorOf(t, sess, "p"); r != 1 || g != 0 || b != 0 {
		t.Errorf("p color = (%d,%d,%d), want (1,0,0) from the sheet both links hold", r, g, b)
	}
}

// TestImportedSheetRequestedOnceAcrossPasses covers the @import half of the
// rule: an imported target belongs to the navigation's fetch profile, so a
// later collection splices it from the memo instead of asking again. b.css
// carries the only declaration that reaches h1, so an expansion that dropped
// the import shows up as a missing colour rather than only a count.
// qc:BR-282
func TestImportedSheetRequestedOnceAcrossPasses(t *testing.T) {
	srv := newSheetServer()
	html := `<html><head>
<link rel="stylesheet" href="/theme.css">
` + appendedStyle("s1", "div { color: #050000 }") + `
` + appendedStyle("s2", "div { color: #060000 }") + `
</head><body><p>x</p><h1>y</h1><div>z</div></body></html>`
	sess := newCSSSession(t, newCSSRuntime(t), html, srv.link)

	for _, href := range []string{"https://example.test/theme.css", "https://example.test/b.css"} {
		if got := srv.count(href); got != 1 {
			t.Errorf("%s requested %d times, want 1 across the collection passes", href, got)
		}
	}
	if r, g, b := colorOf(t, sess, "h1"); r != 0 || g != 2 || b != 0 {
		t.Errorf("h1 color = (%d,%d,%d), want (0,2,0) from the imported b.css", r, g, b)
	}
	if r, g, b := colorOf(t, sess, "p"); r != 1 || g != 0 || b != 0 {
		t.Errorf("p color = (%d,%d,%d), want (1,0,0) from the importing sheet", r, g, b)
	}
}

// TestScriptRewrittenHrefFetchesOnlyTheNewSheet is the guard against reading
// the memo too broadly: pointing a link at another URL changes the document's
// stylesheet state, so that URL is a new key, fetched once, and its rule wins.
// qc:BR-282
func TestScriptRewrittenHrefFetchesOnlyTheNewSheet(t *testing.T) {
	srv := newSheetServer()
	html := `<html><head>
<link id="l" rel="stylesheet" href="/a.css">
<script>document.getElementById('l').href = '/c.css';</script>
</head><body><p>x</p></body></html>`
	sess := newCSSSession(t, newCSSRuntime(t), html, srv.link)

	if got := srv.count("https://example.test/c.css"); got != 1 {
		t.Errorf("c.css requested %d times, want 1 (a rewritten href is a new URL)", got)
	}
	if got := srv.count("https://example.test/a.css"); got > 1 {
		t.Errorf("a.css requested %d times, want at most 1", got)
	}
	if r, g, b := colorOf(t, sess, "p"); r != 2 || g != 0 || b != 0 {
		t.Errorf("p color = (%d,%d,%d), want (2,0,0) from the sheet the script pointed the link at", r, g, b)
	}
}

// TestFailedSheetFetchIsRetriedOnLaterPass records what the memo deliberately
// does not remember: a failed fetch holds no charged bytes, so the next
// collection asks for the URL again and the sheet lands. Measured on the live
// page, 0 of the 33 stylesheet URLs failed, so retries were not part of the
// storm - this pins the decision rather than a Chromium number.
// qc:BR-282
func TestFailedSheetFetchIsRetriedOnLaterPass(t *testing.T) {
	srv := newSheetServer("https://example.test/a.css")
	html := `<html><head>
<link rel="stylesheet" href="/a.css">
` + appendedStyle("s1", "div { color: #050000 }") + `
` + appendedStyle("s2", "div { color: #060000 }") + `
</head><body><p>x</p><div>z</div></body></html>`
	sess := newCSSSession(t, newCSSRuntime(t), html, srv.link)

	if got := srv.attempted("https://example.test/a.css"); got < 2 {
		t.Errorf("a.css attempted %d times, want a retry after the first failure", got)
	}
	if r, g, b := colorOf(t, sess, "p"); r != 1 || g != 0 || b != 0 {
		t.Errorf("p color = (%d,%d,%d), want (1,0,0) once the retried fetch succeeded", r, g, b)
	}
}

// TestPureReadScriptsCollectTheSheetSetOnce separates the fix's second half
// from its first: with the memo alone a quiet page still walks the document and
// re-parses every sheet once per script. Nothing here moves the stylesheet
// state, so only two collections remain - the first script's, because nothing
// was installed yet, and the load's final one, which covers links that appear
// after the last script. Before this slice every script added a collection.
// qc:BR-282
func TestPureReadScriptsCollectTheSheetSetOnce(t *testing.T) {
	srv := newSheetServer()
	read := `<script>document.getElementsByTagName('link').length;</script>`
	html := `<html><head>
<link rel="stylesheet" href="/a.css">
` + read + read + read + `
</head><body><p>x</p></body></html>`
	sess := newCSSSession(t, newCSSRuntime(t), html, srv.link)

	if got := sess.WorkCounts().CSSCollections; got != 2 {
		t.Errorf("CSS collections = %d, want 2 (the first script plus the load collection; nothing moved in between)", got)
	}
	if got := srv.total(); got != 1 {
		t.Errorf("total stylesheet requests = %d, want 1", got)
	}
	if r, g, b := colorOf(t, sess, "p"); r != 1 || g != 0 || b != 0 {
		t.Errorf("p color = (%d,%d,%d), want (1,0,0) from a.css", r, g, b)
	}
}

// TestMutatingScriptsCollectOncePerChange is the same counter read the other
// way: three scripts that each add a style element do owe three collections,
// because each changed what the next script's CSSOM read must see. A fix that
// collected only once per navigation would fail here.
// qc:BR-282
func TestMutatingScriptsCollectOncePerChange(t *testing.T) {
	srv := newSheetServer()
	html := `<html><head>
<link rel="stylesheet" href="/a.css">
` + appendedStyle("s1", "div { color: #050000 }") + `
` + appendedStyle("s2", "div { color: #060000 }") + `
` + appendedStyle("s3", "div { color: #070000 }") + `
</head><body><p>x</p><div>z</div></body></html>`
	sess := newCSSSession(t, newCSSRuntime(t), html, srv.link)

	got := sess.WorkCounts().CSSCollections
	if got < 3 {
		t.Errorf("CSS collections = %d, want at least 3 (one per stylesheet change)", got)
	}
	if got > 4 {
		t.Errorf("CSS collections = %d, want at most 4 (three changes plus the load collection)", got)
	}
	if srv.count("https://example.test/a.css") != 1 {
		t.Errorf("a.css requested %d times, want 1 across those collections", srv.count("https://example.test/a.css"))
	}
	if r, g, b := colorOf(t, sess, "div"); r != 7 || g != 0 || b != 0 {
		t.Errorf("div color = (%d,%d,%d), want (7,0,0) from the last injected style", r, g, b)
	}
}

// TestMixedContentLinkedSheetFetchesNothing keeps the refusal in front of the
// memo: an http sheet on an https page is reported and never requested, so no
// collection pass can turn mixed content into a memo hit.
// qc:BR-282
func TestMixedContentLinkedSheetFetchesNothing(t *testing.T) {
	srv := newSheetServer()
	var mu sync.Mutex
	var hits int
	linker := func(base, href string) (string, error) {
		mu.Lock()
		hits++
		mu.Unlock()
		return srv.link(base, href)
	}
	html := `<html><head>
<link rel="stylesheet" href="http://other.example/a.css">
` + appendedStyle("s1", "div { color: #050000 }") + `
` + appendedStyle("s2", "div { color: #060000 }") + `
</head><body><p>x</p><div>z</div></body></html>`
	sess := newCSSSession(t, newCSSRuntime(t), html, linker)

	mu.Lock()
	got := hits
	mu.Unlock()
	if got != 0 {
		t.Errorf("linker called %d times, want 0 for a mixed-content stylesheet", got)
	}
	var reports int
	for _, m := range sess.MixedContentReports() {
		if strings.Contains(m.URL, "other.example/a.css") {
			reports++
		}
	}
	if reports == 0 {
		t.Errorf("mixed-content stylesheet was not reported: %v", sess.MixedContentReports())
	}
}
