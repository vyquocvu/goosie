package engine_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestScriptExecutionSetsGlobal(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><head><script>var greeting = "hello";</script></head><body></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	v, ok := rt.Global("greeting")
	if !ok {
		t.Fatal("script did not set global greeting")
	}
	if v.Str != "hello" {
		t.Errorf("greeting = %q, want %q", v.Str, "hello")
	}
}

func TestScriptsRunInDocumentOrder(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><body>
<script>var order = "first";</script>
<script>order += ",second";</script>
<script>order += ",third";</script>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	v, ok := rt.Global("order")
	if !ok {
		t.Fatal("scripts did not set global order")
	}
	if v.Str != "first,second,third" {
		t.Errorf("order = %q, want %q", v.Str, "first,second,third")
	}
}

func TestScriptCanChangeDocumentTitle(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/", Title: "Initial"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><head><script>document.title = "Changed by JS";</script></head><body></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if got := sess.JSTitle(); got != "Changed by JS" {
		t.Errorf("JSTitle() = %q, want %q", got, "Changed by JS")
	}
}

func TestBrokenScriptDoesNotStopRendering(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><body>
<script>throw new Error("boom");</script>
<script>var afterError = true;</script>
<p>Content</p>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatalf("session build failed after broken script: %v", err)
	}
	defer sess.Close()

	v, ok := rt.Global("afterError")
	if !ok || !v.Bool {
		t.Error("script after a throwing script did not run")
	}
}

func TestConsoleOutputReachesHostWriter(t *testing.T) {
	var buf bytes.Buffer
	rt, err := js.New(js.Options{URL: "https://example.test/", Console: &buf})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><body>
<script>console.log("from page");</script>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if got := strings.TrimSpace(buf.String()); got != "from page" {
		t.Errorf("console output = %q, want %q", got, "from page")
	}
}

func TestNoJSRuntimeSkipsScriptExecution(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
<script>var shouldNotExist = true;</script>
<p>Content</p>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if sess.JSTitle() != "" {
		t.Error("JSTitle() should be empty without a JS runtime")
	}
}
