package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestJS_DeclarativeShadowDOMAccess(t *testing.T) {
	doc := domtest.Parse(`<!DOCTYPE html>
<html>
<body>
  <my-card id="host">
    <template shadowrootmode="open">
      <div id="inner">Inside Shadow</div>
    </template>
    <span>Light</span>
  </my-card>
</body>
</html>`)

	rt, err := js.New(js.Options{
		URL: "https://example.com",
		DOM: doc,
	})
	if err != nil {
		t.Fatalf("failed to create js runtime: %v", err)
	}
	defer rt.Close()

	err = rt.Run(`
		var host = document.getElementById("host");
		var sr = host ? host.shadowRoot : null;
		var mode = sr ? sr.mode : null;
	`, "inline")
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	modeVal, ok := rt.Global("mode")
	if !ok {
		t.Fatal("mode variable not found")
	}
	if modeVal.Str != "open" {
		t.Fatalf("expected host.shadowRoot.mode to be 'open', got %v", modeVal)
	}

	// Verify that closed mode shadow root returns null in JS
	docClosed := domtest.Parse(`<!DOCTYPE html>
<html>
<body>
  <my-secret id="secret">
    <template shadowrootmode="closed">
      <div>Hidden</div>
    </template>
  </my-secret>
</body>
</html>`)

	rtClosed, err := js.New(js.Options{
		URL: "https://example.com",
		DOM: docClosed,
	})
	if err != nil {
		t.Fatalf("failed to create closed js runtime: %v", err)
	}
	defer rtClosed.Close()

	err = rtClosed.Run(`
		var secret = document.getElementById("secret");
		var closedRoot = secret ? secret.shadowRoot : undefined;
	`, "inline")
	if err != nil {
		t.Fatalf("eval error on closed shadow root: %v", err)
	}

	closedVal, ok := rtClosed.Global("closedRoot")
	if !ok {
		t.Fatal("closedRoot variable not found")
	}
	if closedVal.Kind != js.KindNull {
		t.Fatalf("expected closed shadowRoot to be KindNull, got kind %v", closedVal.Kind)
	}
}
