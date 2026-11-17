package engine_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

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

func appendedStyle(marker, decl string) string {
	return fmt.Sprintf("<script>var %s = document.createElement('style'); %s.textContent = '%s'; document.head.appendChild(%s);</script>", marker, marker, decl, marker)
}

func TestAlternateStylesheetIsNotFetched(t *testing.T) {
	srv := newSheetServer()
	srv.text["https://example.test/main.css"] = "p { color: rgb(1, 2, 3); }"
	srv.text["https://example.test/draft.css"] = "html { background-color: rgb(0, 128, 0); }"
	html := `<!doctype html><head>
<link rel="stylesheet" href="main.css">
<link rel="alternate stylesheet" title="Draft" href="draft.css">
</head><body><p>Hi</p></html>`
	sess := newCSSSession(t, newCSSRuntime(t), html, srv.link)
	if srv.attempted("https://example.test/draft.css") != 0 {
		t.Fatalf("alternate stylesheet fetches = %d, want 0", srv.attempted("https://example.test/draft.css"))
	}
	if srv.count("https://example.test/main.css") == 0 {
		t.Fatal("preferred stylesheet was not fetched")
	}
	r, g, b := colorOf(t, sess, "p")
	if r != 1 || g != 2 || b != 3 {
		t.Fatalf("p color = %d,%d,%d, want 1,2,3", r, g, b)
	}
}

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
