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

func TestExternalScriptsFetchConcurrently(t *testing.T) {
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
	start := time.Now()
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt), engine.WithScripts("https://example.test/", fetcher))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if got := atomic.LoadInt64(&fetched); got != count {
		t.Fatalf("fetched %d scripts, want %d", got, count)
	}
	if p := atomic.LoadInt64(&peak); p < 4 {
		t.Fatalf("peak concurrent fetches = %d, want >= 4", p)
	}
	if serial := time.Duration(count) * 100 * time.Millisecond; elapsed >= serial {
		t.Fatalf("elapsed %v >= serial bound %v", elapsed, serial)
	}
	v, _ := rt.Global("order")
	if want := strings.Repeat(",x", count); v.Str != want {
		t.Fatalf("order = %q, want %q (document order despite concurrent fetch)", v.Str, want)
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

func TestModuleScriptsSkipped(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	var fetched int64
	fetcher := func(base, src string) ([]byte, error) {
		atomic.AddInt64(&fetched, 1)
		return []byte(`order = "module-ran";`), nil
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

	if got := atomic.LoadInt64(&fetched); got != 0 {
		t.Fatalf("module bundle must not even be fetched, got %d", got)
	}
	v, _ := rt.Global("order")
	if v.Str != "classic" {
		t.Fatalf("order = %q, want classic", v.Str)
	}
}
