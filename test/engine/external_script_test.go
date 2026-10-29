package engine_test

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

// External <script src> is how dynamic pages boot: the HTML is a shell and
// the app arrives as bundles. The engine ran inline scripts only, so every
// such page stayed a blank shell.
func TestExternalScriptsRunInDocumentOrder(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	fetcher := func(base, src string) ([]byte, error) {
		if !strings.HasPrefix(src, "https://example.test/") {
			t.Errorf("src not resolved against base: %q", src)
		}
		return []byte(`order += ",ext";`), nil
	}
	html := `<html><body>
<script>var order = "first";</script>
<script src="/app.js"></script>
<script>order += ",third";</script>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt), engine.WithScripts("https://example.test/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	v, ok := rt.Global("order")
	if !ok {
		t.Fatal("scripts did not set global order")
	}
	if v.Str != "first,ext,third" {
		t.Errorf("order = %q, want %q", v.Str, "first,ext,third")
	}
}

func TestDeferredDataScriptRunsBeforeDOMContentLoaded(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	var fetched atomic.Int64
	fetcher := func(base, src string) ([]byte, error) {
		fetched.Add(1)
		return nil, nil
	}
	html := `<html><head><script>var order = []; document.addEventListener("DOMContentLoaded", function() { order.push("DOMContentLoaded"); });</script>
<script defer src="data:text/javascript,order.push('defer');setTimeout(() => order.push('timeout'), 0)"></script>
<script defer src="data:text/javascript;base64,d2luZG93LmRlZmVycmVkQmFzZTY0U2NyaXB0PXRydWU="></script>
<script defer src="data:text/plain,window.badDataMimeRan=true"></script></head><body></body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt), engine.WithScripts("https://example.test/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if err := rt.Run(`window.deferredOrder = order.join(",");`, "test"); err != nil {
		t.Fatal(err)
	}
	if got, ok := rt.Global("deferredOrder"); !ok || got.Str != "defer,timeout,DOMContentLoaded" {
		t.Fatalf("deferred script/lifecycle/timer order = %+v, want defer,timeout,DOMContentLoaded", got)
	}
	if got := fetched.Load(); got != 0 {
		t.Fatalf("data URL script used network fetcher %d times", got)
	}
	if got, ok := rt.Global("deferredBase64Script"); !ok || !got.Bool {
		t.Fatalf("base64 data script result = %+v, want true", got)
	}
	if _, ok := rt.Global("badDataMimeRan"); ok {
		t.Fatal("non-JavaScript data MIME type executed")
	}
}

func TestExternalScriptFailureIsNonFatal(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	fetcher := func(base, src string) ([]byte, error) {
		return nil, fmt.Errorf("cdn down")
	}
	html := `<html><body>
<script>var order = "first";</script>
<script src="https://cdn.test/app.js"></script>
<script>order += ",third";</script>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt), engine.WithScripts("https://example.test/", fetcher))
	if err != nil {
		t.Fatalf("one dead bundle must not fail the load: %v", err)
	}
	defer sess.Close()

	v, ok := rt.Global("order")
	if !ok || v.Str != "first,third" {
		t.Fatalf("order = %+v, want first,third", v)
	}
}

func TestParserBlockingExternalScriptsRunSequentially(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	const count = 8
	var inFlight, peak, fetched int64
	fetcher := func(base, src string) ([]byte, error) {
		cur := atomic.AddInt64(&inFlight, 1)
		for {
			old := atomic.LoadInt64(&peak)
			if cur <= old || atomic.CompareAndSwapInt64(&peak, old, cur) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		atomic.AddInt64(&inFlight, -1)
		atomic.AddInt64(&fetched, 1)
		return []byte(`order += ",x";`), nil
	}
	var tags strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&tags, `<script src="/s%d.js"></script>`, i)
	}
	html := `<html><body><script>var order = "";</script>` + tags.String() + `</body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt), engine.WithScripts("https://example.test/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if got := atomic.LoadInt64(&fetched); got != count {
		t.Fatalf("fetched %d scripts, want %d", got, count)
	}
	if p := atomic.LoadInt64(&peak); p != 1 {
		t.Fatalf("peak concurrent fetches = %d, want 1 while parser-blocking scripts execute", p)
	}
	v, _ := rt.Global("order")
	if want := strings.Repeat(",x", count); v.Str != want {
		t.Fatalf("order = %q, want %q (document order during parser blocking)", v.Str, want)
	}
}

func TestExternalScriptCountCapped(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	var fetched int64
	fetcher := func(base, src string) ([]byte, error) {
		atomic.AddInt64(&fetched, 1)
		return []byte(``), nil
	}
	var tags strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&tags, `<script src="/s%d.js"></script>`, i)
	}
	html := `<html><body>` + tags.String() + `</body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt), engine.WithScripts("https://example.test/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if got := atomic.LoadInt64(&fetched); got != 32 {
		t.Fatalf("fetched %d scripts, want capped 32", got)
	}
}

func TestExternalModuleScriptRunsInModuleScope(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	var fetched int64
	fetcher := func(base, src string) ([]byte, error) {
		if base != "https://example.test/" || src != "https://example.test/mod.js" {
			t.Fatalf("unexpected module fetch base=%q src=%q", base, src)
		}
		atomic.AddInt64(&fetched, 1)
		return []byte(`window.order = "module-ran";`), nil
	}
	html := `<html><body>
<script>var order = "classic";</script>
<script type="module" src="/mod.js"></script>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt), engine.WithScripts("https://example.test/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if got := atomic.LoadInt64(&fetched); got != 1 {
		t.Fatalf("module dependency fetched %d times, want 1", got)
	}
	v, _ := rt.Global("order")
	if v.Str != "module-ran" {
		t.Fatalf("order = %q, want module-ran", v.Str)
	}
}
