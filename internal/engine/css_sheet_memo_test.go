package engine

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

// The subresource budget is the other half of why the storm blanked the page:
// every collection charged the bytes of every sheet again, so 927 requests for
// 33 stylesheets were charged as if they were 927 distinct sheets. The memo
// holds one copy of each text, so one copy is what the budget must lose. Only
// an in-package test can read the charge, and the number is small enough to
// name exactly.
// qc:BR-282
func TestLinkedSheetBytesChargedOncePerNavigation(t *testing.T) {
	sheets := map[string]string{
		"https://e/a.css": strings.Repeat("a { color: red } ", 100),
		"https://e/b.css": strings.Repeat("b { color: blue } ", 50),
	}
	var requests atomic.Int64
	linker := func(base, href string) (string, error) {
		text, ok := sheets[href]
		if !ok {
			return "", fmt.Errorf("no sheet at %s", href)
		}
		requests.Add(1)
		return text, nil
	}
	rt, err := js.New(js.Options{URL: "https://e/x/page.html"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	mutate := func(decl string) string {
		return fmt.Sprintf("<script>var s = document.createElement('style'); s.textContent = '%s'; document.head.appendChild(s);</script>", decl)
	}
	html := "<html><head>" +
		`<link rel="stylesheet" href="/a.css"><link rel="stylesheet" href="/b.css">` +
		mutate("div { color: green }") + mutate("div { color: olive }") + mutate("div { color: teal }") +
		"</head><body><p>x</p></body></html>"
	s, err := NewSession(html, nil, 800,
		WithJS(rt),
		WithLinkedCSS("https://e/x/page.html", linker))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if got := s.WorkCounts().CSSCollections; got < 4 {
		t.Fatalf("CSS collections = %d, want the four passes the three mutations plus the load owe", got)
	}
	want := int64(0)
	for _, text := range sheets {
		want += int64(len(text))
	}
	s.sub.mu.Lock()
	used := s.sub.used
	s.sub.mu.Unlock()
	if used != want {
		t.Fatalf("subresource bytes used = %d, want %d (each sheet's text once, whatever the pass count); requests = %d, collections = %d",
			used, want, requests.Load(), s.WorkCounts().CSSCollections)
	}
	if got := requests.Load(); got != int64(len(sheets)) {
		t.Fatalf("linker called %d times, want %d", got, len(sheets))
	}
}

// A nil memo is what the callers that collect outside any session get, and it
// must keep the pre-memo behaviour of asking for every URL every pass. The
// @import unit tests in this package run that way, so the shape is load-bearing
// rather than theoretical.
// qc:BR-282
func TestNilSheetMemoAlwaysMisses(t *testing.T) {
	var cache *cssTextCache
	if _, ok := cache.lookup("https://e/x/a.css"); ok {
		t.Fatal("nil memo reported a hit")
	}
	cache.store("https://e/x/a.css", "p { color: red }")
	if _, ok := cache.lookup("https://e/x/a.css"); ok {
		t.Fatal("nil memo kept text it was told to store")
	}
}
