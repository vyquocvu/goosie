package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

// document.cookie is the switch dynamic pages read before booting: Wikipedia
// parses its preference cookie, then flips client-nojs to client-js. With no
// cookie binding the read throws, the flip never happens, and the page stays
// in its no-JS layout with expanded dropdowns pushing the article down.
func TestDocumentCookieEmpty(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if err := r.Run(`var out = document.cookie === "";`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || !v.Bool {
		t.Fatalf("empty cookie = %+v, want true", v)
	}
}

func TestDocumentCookieRoundTrip(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	src := `
document.cookie = "a=1";
document.cookie = "b=2";
var out = document.cookie;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "a=1; b=2" {
		t.Fatalf("cookie = %+v, want a=1; b=2", v)
	}
}

func TestDocumentCookieOverwrite(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	src := `
document.cookie = "a=1";
document.cookie = "a=2";
var out = document.cookie;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "a=2" {
		t.Fatalf("cookie = %+v, want a=2", v)
	}
}

func TestDocumentCookieDeleteViaExpiry(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	src := `
document.cookie = "a=1";
document.cookie = "b=2";
document.cookie = "a=gone; expires=Thu, 01 Jan 1970 00:00:00 GMT";
var out = document.cookie;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "b=2" {
		t.Fatalf("cookie = %+v, want b=2", v)
	}
}

func TestDocumentCookieIgnoresAttributes(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	src := `
document.cookie = "a=1; path=/; domain=example.test";
var out = document.cookie;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "a=1" {
		t.Fatalf("cookie = %+v, want a=1", v)
	}
}

// Wikipedia's exact startup read: match on an empty jar must yield null,
// not a throw, so the boot proceeds to the className flip.
func TestWikipediaCookieMatch(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://en.wikipedia.org/wiki/Web_browser"})
	src := `
var cookie = document.cookie.match(/(?:^|; )enwikimwclientpreferences=([^;]+)/);
var out = (cookie === null);`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || !v.Bool {
		t.Fatalf("cookie match = %+v, want true", v)
	}
}

// A host-supplied store backs the binding when present: reads come from it
// (HttpOnly excluded by the store) and writes go to it.
func TestDocumentCookieUsesStore(t *testing.T) {
	store := &memCookieStore{}
	r := newRuntime(t, js.Options{URL: "https://example.test/", Cookies: store})
	src := `
document.cookie = "s=1";
var out = document.cookie + "/" + document.cookie;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "s=1/s=1" {
		t.Fatalf("store cookie = %+v, want s=1/s=1", v)
	}
	if got := store.stored; got != "s=1" {
		t.Fatalf("store received %q, want s=1", got)
	}
}

type memCookieStore struct {
	jar    []string
	stored string
}

func (m *memCookieStore) CookiesForDocument(url string) string {
	out := ""
	for i, kv := range m.jar {
		if i > 0 {
			out += "; "
		}
		out += kv
	}
	return out
}

func (m *memCookieStore) SetCookieFromDocument(url, assignment string) {
	m.stored = assignment
	m.jar = append(m.jar, assignment)
}
