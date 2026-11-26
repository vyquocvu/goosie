package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestJourneyStaticESModulesOverLocalHTTP(t *testing.T) {
	const want = "default|named|alias|true|named|default|star|named|alias|A:B|side"
	modules := map[string]string{
		"/main.js": `import def, { named, alias as renamed } from "/named.js";
import * as ns from "/named.js";
import "/side.js";
import { viaAlias, viaDefault, viaStar, bag, cycleValue } from "/reexport.js";
document.getElementById("result").textContent = [
  def, named, renamed, ns.named === named,
  viaAlias, viaDefault, viaStar, bag.named, bag.alias,
  cycleValue, window.sideEffect
].join("|");
`,
		"/named.js": `export default "default";
export const named = "named";
export const alias = "alias";
`,
		"/side.js": `window.sideEffect = "side";
`,
		"/reexport.js": `export { named as viaAlias, default as viaDefault } from "/named.js";
export * from "/star.js";
export * as bag from "/named.js";
export { cycleValue } from "/cycle-a.js";
`,
		"/star.js": `export const viaStar = "star";
`,
		"/cycle-a.js": `import { b } from "/cycle-b.js";
export const a = "A";
export const cycleValue = a + ":" + b;
`,
		"/cycle-b.js": `import { a } from "/cycle-a.js";
export const seeA = "seesA";
export const b = "B";
`,
	}
	var mu sync.Mutex
	requests := make(map[string]int)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		mu.Lock()
		requests[path]++
		mu.Unlock()
		if path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>Static modules</title><h2 id="result">pending</h2><script type="module" src="/main.js"></script>`)
			return
		}
		source, ok := modules[path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		fmt.Fprint(w, source)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)
	browser.navigateViaAddressBar(t, server.URL+"/")
	browser.waitFor(t, "static module graph to execute", func() bool {
		result, ok := findAXNode(browser.window.AccessibilityTree(), "result")
		return ok && result.Label == want
	})

	session := browser.f.tabMgr.Active().Session()
	if session == nil {
		t.Fatal("module page has no session")
	}
	if errs := session.ScriptErrors(); len(errs) > 0 {
		t.Fatalf("module graph script errors: %v", errs)
	}
	if got, err := session.EvaluateJS(`document.getElementById("result").textContent`); err != nil || got != want {
		t.Fatalf("module result = %q, %v; want %q", got, err, want)
	}

	mu.Lock()
	defer mu.Unlock()
	for path := range modules {
		if requests[path] != 1 {
			t.Errorf("module %s requests = %d, want 1", path, requests[path])
		}
	}
	if requests["/"] != 1 {
		t.Errorf("document requests = %d, want 1", requests["/"])
	}
}
