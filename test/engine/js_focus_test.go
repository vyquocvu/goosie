package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestProgrammaticFocusResolvesFocusWithin(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	html := `<html><head><style>
		#container { border: 15px solid blue }
		#container:focus-within { border-color: green }
		#focusme:focus { outline: none }
	</style></head><body>
		<div id="container"><div id="focusme" tabindex="1">focus me</div></div>
		<script>
			var focusme = document.getElementById("focusme");
			focusme.addEventListener("focus", function() { window.activeInFocus = document.activeElement.id; });
			focusme.focus();
		</script>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	container := sess.Doc.ElementByID("container")
	if container == nil {
		t.Fatal("focus container missing")
	}
	got := sess.Styles[container.ID].BorderTopColor
	if got.R != 0 || got.G != 128 || got.B != 0 || got.A != 255 {
		t.Fatalf(":focus-within border color = %+v, want CSS green", got)
	}
	activeInFocus, err := rt.Evaluate("window.activeInFocus")
	if err != nil || activeInFocus != "focusme" {
		t.Fatalf("document.activeElement inside focus handler = %q, err %v", activeInFocus, err)
	}
	if err := rt.Run(`document.getElementById("focusme").blur()`, "inline"); err != nil {
		t.Fatalf("blur script: %v", err)
	}
	if !sess.PumpJS(800) {
		t.Fatal("PumpJS did not re-resolve styles after programmatic blur")
	}
	got = sess.Styles[container.ID].BorderTopColor
	if got.R != 0 || got.G != 0 || got.B != 255 || got.A != 255 {
		t.Fatalf(":focus-within after blur = %+v, want blue", got)
	}
	activeAfterBlur, err := rt.Evaluate("document.activeElement.tagName")
	if err != nil || activeAfterBlur != "BODY" {
		t.Fatalf("document.activeElement after blur = %q, err %v", activeAfterBlur, err)
	}
}
