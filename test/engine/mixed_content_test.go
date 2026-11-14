package engine_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestMixedContentBlocking(t *testing.T) {
	var fetched int64
	fetcher := func(base, url string) ([]byte, error) {
		atomic.AddInt64(&fetched, 1)
		return nil, nil
	}
	html := `<img src="http://cdn.example.com/img.png">`
	sess, err := engine.NewSession(html, nil, 400,
		engine.WithImages("https://secure.example.com/page", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil {
		t.Fatal("session is nil")
	}
	if n := atomic.LoadInt64(&fetched); n != 0 {
		t.Fatalf("mixed-content image was fetched %d times", n)
	}
	if got := mixedURLs(sess, engine.MixedContentImage); len(got) != 1 || got[0] != "http://cdn.example.com/img.png" {
		t.Fatalf("mixed-content image reports = %v, want the http URL once", got)
	}
}

func TestMixedContentAllowsSameScheme(t *testing.T) {
	var fetched int64
	fetcher := func(base, url string) ([]byte, error) {
		atomic.AddInt64(&fetched, 1)
		return nil, nil
	}
	html := `<img src="https://cdn.example.com/img.png">`
	sess, err := engine.NewSession(html, nil, 400,
		engine.WithImages("https://secure.example.com/page", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil {
		t.Fatal("session is nil")
	}
	if n := atomic.LoadInt64(&fetched); n != 1 {
		t.Fatalf("same-scheme image fetch count = %d, want 1", n)
	}
	if reports := sess.MixedContentReports(); len(reports) != 0 {
		t.Fatalf("mixed-content reports = %v, want none", reports)
	}
}

func TestMixedContentBlocksActiveSubresources(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://secure.example.com/page"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	var cssFetches, scriptFetches int64
	html := `<html><head><link rel="stylesheet" href="http://cdn.example.com/a.css"></head>` +
		`<body><script src="http://cdn.example.com/a.js"></script></body></html>`
	sess, err := engine.NewSession(html, nil, 400,
		engine.WithJS(rt),
		engine.WithLinkedCSS("https://secure.example.com/page", func(base, url string) (string, error) {
			atomic.AddInt64(&cssFetches, 1)
			return "body { background: red }", nil
		}),
		engine.WithScripts("https://secure.example.com/page", func(base, url string) ([]byte, error) {
			atomic.AddInt64(&scriptFetches, 1)
			return []byte("window.ran = true;"), nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt64(&cssFetches); n != 0 {
		t.Fatalf("mixed-content stylesheet was fetched %d times", n)
	}
	if n := atomic.LoadInt64(&scriptFetches); n != 0 {
		t.Fatalf("mixed-content script was fetched %d times", n)
	}
	if got := mixedURLs(sess, engine.MixedContentStylesheet); len(got) != 1 || got[0] != "http://cdn.example.com/a.css" {
		t.Fatalf("stylesheet reports = %v, want the http URL once", got)
	}
	if got := mixedURLs(sess, engine.MixedContentScript); len(got) != 1 || got[0] != "http://cdn.example.com/a.js" {
		t.Fatalf("script reports = %v, want the http URL once", got)
	}
}

func TestMixedContentBlocksIFrame(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://secure.example.com/page"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	var loads int
	html := `<body><script>
var f = document.createElement('iframe');
f.src = 'http://cdn.example.com/frame.html';
document.body.appendChild(f);
</script></body>`
	sess, err := engine.NewSession(html, nil, 400,
		engine.WithJS(rt),
		engine.WithLinkedCSS("https://secure.example.com/page", nil),
		engine.WithIFrameDocumentLoader(func(base, src string) (*dom.Document, error) {
			loads++
			return nil, nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	if loads != 0 {
		t.Fatalf("mixed-content iframe reached the loader %d times", loads)
	}
	if got := mixedURLs(sess, engine.MixedContentIFrame); len(got) != 1 || got[0] != "http://cdn.example.com/frame.html" {
		t.Fatalf("iframe reports = %v, want the http URL once", got)
	}
}

func TestMixedContentAllowsInsecureResourcesOnInsecureDocument(t *testing.T) {
	rt, err := js.New(js.Options{URL: "http://insecure.example.com/page"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	var cssFetches, scriptFetches, imgFetches int64
	html := `<html><head><link rel="stylesheet" href="http://cdn.example.com/a.css"></head>` +
		`<body><img src="http://cdn.example.com/a.png">` +
		`<script src="http://cdn.example.com/a.js"></script></body></html>`
	sess, err := engine.NewSession(html, nil, 400,
		engine.WithJS(rt),
		engine.WithLinkedCSS("http://insecure.example.com/page", func(base, url string) (string, error) {
			atomic.AddInt64(&cssFetches, 1)
			return "body { background: red }", nil
		}),
		engine.WithImages("http://insecure.example.com/page", func(base, url string) ([]byte, error) {
			atomic.AddInt64(&imgFetches, 1)
			return nil, nil
		}),
		engine.WithScripts("http://insecure.example.com/page", func(base, url string) ([]byte, error) {
			atomic.AddInt64(&scriptFetches, 1)
			return []byte("void 0;"), nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt64(&cssFetches); n < 1 {
		t.Fatalf("insecure stylesheet fetch count = %d, want at least 1", n)
	}
	if n := atomic.LoadInt64(&scriptFetches); n != 1 {
		t.Fatalf("insecure script fetch count = %d, want 1", n)
	}
	if n := atomic.LoadInt64(&imgFetches); n != 1 {
		t.Fatalf("insecure image fetch count = %d, want 1", n)
	}
	if reports := sess.MixedContentReports(); len(reports) != 0 {
		t.Fatalf("mixed-content reports = %v, want none for an insecure document", reports)
	}
}

func TestMixedContentAllowsSecureRelativeReferences(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://secure.example.com/page"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	var cssFetches, scriptFetches, imgFetches int64
	html := `<html><head><link rel="stylesheet" href="//cdn.example.com/a.css"></head>` +
		`<body><img src="/a.png"><script src="./a.js"></script></body></html>`
	sess, err := engine.NewSession(html, nil, 400,
		engine.WithJS(rt),
		engine.WithLinkedCSS("https://secure.example.com/page", func(base, url string) (string, error) {
			atomic.AddInt64(&cssFetches, 1)
			return "body { background: red }", nil
		}),
		engine.WithImages("https://secure.example.com/page", func(base, url string) ([]byte, error) {
			atomic.AddInt64(&imgFetches, 1)
			return nil, nil
		}),
		engine.WithScripts("https://secure.example.com/page", func(base, url string) ([]byte, error) {
			atomic.AddInt64(&scriptFetches, 1)
			return []byte("void 0;"), nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt64(&cssFetches); n < 1 {
		t.Fatalf("scheme-relative stylesheet fetch count = %d, want at least 1", n)
	}
	if n := atomic.LoadInt64(&scriptFetches); n != 1 {
		t.Fatalf("relative script fetch count = %d, want 1", n)
	}
	if n := atomic.LoadInt64(&imgFetches); n != 1 {
		t.Fatalf("relative image fetch count = %d, want 1", n)
	}
	if reports := sess.MixedContentReports(); len(reports) != 0 {
		t.Fatalf("mixed-content reports = %v, want none", reports)
	}
}

func TestMixedContentReportsAreBounded(t *testing.T) {
	var html string
	for i := 0; i < 40; i++ {
		html += fmt.Sprintf(`<img src="http://cdn.example.com/%d.png">`, i)
	}
	var fetched int64
	sess, err := engine.NewSession(html, nil, 400,
		engine.WithImages("https://secure.example.com/page", func(base, url string) ([]byte, error) {
			atomic.AddInt64(&fetched, 1)
			return nil, nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt64(&fetched); n != 0 {
		t.Fatalf("blocked images reached the fetcher %d times", n)
	}
	if got := len(sess.MixedContentReports()); got != 32 {
		t.Fatalf("mixed-content reports = %d, want the 32-entry bound", got)
	}
}

func mixedURLs(sess *engine.Session, kind engine.MixedContentKind) []string {
	var urls []string
	for _, report := range sess.MixedContentReports() {
		if report.Kind == kind {
			urls = append(urls, report.URL)
		}
	}
	return urls
}
