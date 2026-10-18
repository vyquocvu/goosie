package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/userscript"
)

func TestEngine_UserScriptDocumentStart(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><body><script>
var result = "";
if (window.__injectedByUserscript) {
  result = "start_ok_and_inline";
} else {
  result = "failed_no_injection";
}
</script></body></html>`

	us := &userscript.Script{
		Name:    "Start Injector",
		RunAt:   userscript.RunAtDocumentStart,
		Code:    `window.__injectedByUserscript = true;`,
		Enabled: true,
	}

	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithUserScripts([]*userscript.Script{us}),
	)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	v, ok := rt.Global("result")
	if !ok {
		t.Fatal("global 'result' not found")
	}
	if v.Str != "start_ok_and_inline" {
		t.Errorf("result = %q, want 'start_ok_and_inline'", v.Str)
	}
}

func TestEngine_UserScriptDocumentEnd(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><body><script>
var flow = "page_executed";
</script></body></html>`

	us := &userscript.Script{
		Name:    "End Manipulator",
		RunAt:   userscript.RunAtDocumentEnd,
		Code:    `window.flow = "overridden_by_userscript";`,
		Enabled: true,
	}

	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithUserScripts([]*userscript.Script{us}),
	)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	v, ok := rt.Global("flow")
	if !ok {
		t.Fatal("global 'flow' not found")
	}
	if v.Str != "overridden_by_userscript" {
		t.Errorf("flow = %q, want 'overridden_by_userscript'", v.Str)
	}
}

func TestEngine_UserScriptDisabled(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><body><script>
var flag = "untouched";
</script></body></html>`

	us := &userscript.Script{
		Name:    "Disabled Script",
		RunAt:   userscript.RunAtDocumentEnd,
		Code:    `window.flag = "modified";`,
		Enabled: false, // Disabled!
	}

	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithUserScripts([]*userscript.Script{us}),
	)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	v, ok := rt.Global("flag")
	if !ok {
		t.Fatal("global 'flag' not found")
	}
	if v.Str != "untouched" {
		t.Errorf("flag = %q, want 'untouched'", v.Str)
	}
}
