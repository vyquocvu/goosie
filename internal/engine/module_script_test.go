package engine

import (
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

func TestModuleScriptRejectsUnsupportedExportSyntax(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	s := &Session{jsRT: rt, scriptFetch: func(string, string) ([]byte, error) { return nil, nil }}
	graphBytes, moduleCount := 0, 0
	if err := s.executeDefaultImportModule(rt, `export const answer = 42;`, "https://example.test/a.js", "a.js", time.Now().Add(time.Second), &graphBytes, &moduleCount); err == nil {
		t.Fatal("unsupported named export was accepted as a module")
	}
}
