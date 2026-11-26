package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestStaticDefaultImportModuleRunsWithModuleScope(t *testing.T) {
	const pageURL = "https://example.test/forms/index.html"
	rt, err := js.New(js.Options{URL: pageURL})
	if err != nil {
		t.Fatal(err)
	}
	fetch := func(base, target string) ([]byte, error) {
		if base != pageURL || target != "https://example.test/forms/types.js" {
			t.Fatalf("unexpected module fetch base=%q url=%q", base, target)
		}
		return []byte(`export default ["text", "email"];`), nil
	}
	s, err := NewSession(`<!doctype html><script type="module">
import types from "./types.js";
const moduleOnly = types.join(",");
window.moduleResult = moduleOnly;
</script>`, nil, 320, WithJS(rt), WithScripts(pageURL, fetch))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got, err := rt.Evaluate(`window.moduleResult`); err != nil || got != "text,email" {
		t.Fatalf("module result = %q, err = %v", got, err)
	}
	if got, evalErr := rt.Evaluate(`typeof moduleOnly`); evalErr != nil || got != "undefined" {
		t.Fatalf("module binding escaped its scope: %q, err=%v", got, evalErr)
	}
}

func TestNamespaceReExportImportKeepsNamespaceFlag(t *testing.T) {
	const pageURL = "https://example.test/index.html"
	rt, err := js.New(js.Options{URL: pageURL})
	if err != nil {
		t.Fatal(err)
	}
	modules := map[string]string{
		"https://example.test/named.js":    `export const named = "named";`,
		"https://example.test/reexport.js": `export * as bag from "./named.js";`,
	}
	fetch := func(base, target string) (ModuleFetchResult, error) {
		body, ok := modules[target]
		if !ok {
			t.Fatalf("unexpected module fetch base=%q url=%q", base, target)
		}
		return ModuleFetchResult{
			Body:       []byte(body),
			Headers:    map[string]string{"Content-Type": "text/javascript"},
			StatusCode: 200,
		}, nil
	}
	s, err := NewSession(`<!doctype html><script type="module">
import { bag } from "./reexport.js";
window.moduleBag = bag.named;
</script>`, nil, 320, WithJS(rt), WithScripts(pageURL, nil), WithModuleFetcher(fetch))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got, err := rt.Evaluate(`window.moduleBag`); err != nil || got != "named" {
		t.Fatalf("namespace re-export import = %q, err = %v; want %q", got, err, "named")
	}
}

func TestDynamicImportIsRejectedBeforeParsing(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	s := &Session{jsRT: rt, scriptFetch: func(string, string) ([]byte, error) { return nil, nil }}
	graphBytes, moduleCount := 0, 0
	err = s.executeDefaultImportModule(rt, `import("./chunk.js");export{l as __webpack_require__};`, "https://example.test/runtime.js", "runtime.js", time.Now().Add(time.Second), &graphBytes, &moduleCount)
	if err == nil || !strings.Contains(err.Error(), "supported static default-import subset") {
		t.Fatalf("dynamic import error = %v", err)
	}
}

func TestModuleScriptSupportsNamedExportSyntax(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	s := &Session{jsRT: rt, scriptFetch: func(base, target string) ([]byte, error) {
		if target != "https://example.test/a.js" {
			t.Fatalf("unexpected module fetch %q", target)
		}
		return []byte(`export const answer = 42;`), nil
	}}
	graphBytes, moduleCount := 0, 0
	deadline := time.Now().Add(time.Second)
	if err := s.executeDefaultImportModule(rt, `import { answer } from "./a.js"; window.moduleAnswer = answer;`, "https://example.test/main.js", "main.js", deadline, &graphBytes, &moduleCount); err != nil {
		t.Fatalf("named export was refused: %v", err)
	}
	got, err := rt.Evaluate(`window.moduleAnswer`)
	if err != nil || got != "42" {
		t.Fatalf("named import result = %q, err = %v", got, err)
	}
}
