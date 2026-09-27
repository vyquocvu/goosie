package js_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

// Sub-project 1 gives a page a window and a document stub and nothing else.
// These tests fix what those stubs must answer to, and - just as important -
// what they must refuse. An API that is quietly undefined is a page that
// silently does nothing; a browser that cannot tell the difference teaches
// nobody why their page is broken.

func TestDocumentTitleRoundTripsThroughTheHost(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/a", Title: "Initial"})
	if got := r.Title(); got != "Initial" {
		t.Fatalf("Title() = %q before any script ran, want the seeded %q", got, "Initial")
	}
	if err := r.Run(`document.title = "After script";`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := r.Title(); got != "After script" {
		t.Errorf("Title() = %q, want %q: the tab title is how a user sees a page at all", got, "After script")
	}
}

func TestWindowLocationIsTheDocumentsURL(t *testing.T) {
	const url = "https://example.test:8443/path?q=1#frag"
	r := newRuntime(t, js.Options{URL: url})
	if err := r.Run(`var here = window.location.href;`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, ok := r.Global("here")
	if !ok || v.Str != url {
		t.Errorf("window.location.href = %+v %v, want the document URL %q", v, ok, url)
	}
}

// console.log is the one part of the developer tools the engine can honour
// today, and it goes to a writer the host handed over. Nothing in this package
// may reach os.Stderr by itself: a per-document output channel that writes to a
// process stream is global state with a log level on it.
func TestConsoleWritesToTheHostWriter(t *testing.T) {
	var buf bytes.Buffer
	r := newRuntime(t, js.Options{URL: "https://example.test/a", Console: &buf})
	if err := r.Run(`console.log("loaded", 3, true);`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "loaded 3 true" {
		t.Errorf("console recorded %q, want %q", got, "loaded 3 true")
	}
}

func TestConsoleWithNoWriterIsDiscardedNotFatal(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/a"})
	if err := r.Run(`console.warn("nobody listening");`, "inline"); err != nil {
		t.Errorf("console.warn with no Console writer returned %v, want the call to succeed and discard", err)
	}
}

// These belong to sub-projects 2 and 3. They have to fail with a message that
// names the missing API so the gap stays visible from a page, and so a test
// that exercises them today reports "not implemented" rather than a pass.
func TestUnsupportedWebAPIsFailLoudly(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/a"})
	for _, tc := range []struct{ src, missing string }{
		{`setTimeout(function () {}, 0);`, "setTimeout"},
		{`document.querySelector("p");`, "querySelector"},
		{`fetch("/data.json");`, "fetch"},
	} {
		err := r.Run(tc.src, "inline")
		if err == nil {
			t.Errorf("%s succeeded, want an error naming the unsupported API", tc.src)
			continue
		}
		if !strings.Contains(err.Error(), tc.missing) {
			t.Errorf("%s failed with %q, which does not name the missing %s", tc.src, err, tc.missing)
		}
	}
}

// Zero global state is not a style preference at this boundary: two tabs run
// two realms, and a binding a page set in one must not appear in the other.
// This is the test that would have caught the process-wide @font-face and
// viewport registries, one sub-system at a time.
func TestRuntimesAreIsolatedFromEachOther(t *testing.T) {
	first := newRuntime(t, js.Options{URL: "https://example.test/a"})
	second := newRuntime(t, js.Options{URL: "https://example.test/b"})

	if err := first.Run(`var shared = "only in the first page";`, "inline"); err != nil {
		t.Fatalf("first page Run: %v", err)
	}
	if _, ok := second.Global("shared"); ok {
		t.Errorf("the second document can read a global the first one set: realms are shared across tabs")
	}
	if err := second.Run(`var shared = "second";`, "inline"); err != nil {
		t.Fatalf("second page Run: %v", err)
	}
	a, _ := first.Global("shared")
	b, _ := second.Global("shared")
	if a.Str != "only in the first page" || b.Str != "second" {
		t.Errorf("shared = %q and %q, want each realm to keep its own value", a.Str, b.Str)
	}
}

// Every tab loads on its own goroutine, so two realms running at once is the
// normal case rather than a stress case. Run under -race: this test's value is
// in the report it does not make.
func TestConcurrentRuntimesDoNotInterfere(t *testing.T) {
	const pages = 8
	var wg sync.WaitGroup
	errs := make([]error, pages)
	values := make([]js.Value, pages)
	for i := 0; i < pages; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := js.New(js.Options{URL: "https://example.test/p"})
			if err != nil {
				errs[i] = err
				return
			}
			defer r.Close()
			if err := r.Run(`var n = 0; for (var i = 0; i < 200; i++) { n += i; }`, "inline"); err != nil {
				errs[i] = err
				return
			}
			values[i], _ = r.Global("n")
		}(i)
	}
	wg.Wait()
	for i := 0; i < pages; i++ {
		if errs[i] != nil {
			t.Fatalf("page %d: %v", i, errs[i])
		}
		if values[i].Num != 19900 {
			t.Errorf("page %d computed n = %v, want 19900", i, values[i].Num)
		}
	}
}
