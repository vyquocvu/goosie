package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/paint"
)

func engineComputed(t *testing.T, rt *js.Runtime, src string) string {
	t.Helper()
	v, err := rt.Evaluate(src)
	if err != nil {
		t.Fatalf("evaluate %s: %v", src, err)
	}
	return v
}

const computedPage = `<!DOCTYPE html><html><head>
<style>
  body { font-size: 16px; }
  #cascaded { display:block; font-size:18px; color:#112233; width:200px; text-align:center; margin:0; padding:0; }
</style>
</head><body>
<div id="cascaded" style="color:#ee0000">cascaded</div>
<div id="bare">bare</div>
</body></html>`

// qc:BR-206
func TestGetComputedStyleReportsTheSessionsOwnResolve(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := engine.NewSession(computedPage, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	cases := []struct{ name, script, want string }{
		{"inherited font-size", `getComputedStyle(document.getElementById("bare")).fontSize`, "16px"},
		{"inherited colour normalised", `getComputedStyle(document.getElementById("bare")).color`, "rgb(0, 0, 0)"},
		{"cascade font-size", `getComputedStyle(document.getElementById("cascaded")).fontSize`, "18px"},
		{"cascade display", `getComputedStyle(document.getElementById("cascaded")).display`, "block"},
		{"cascade text-align", `getComputedStyle(document.getElementById("cascaded")).textAlign`, "center"},
		{"cascade font-weight", `getComputedStyle(document.getElementById("cascaded")).fontWeight`, "400"},
		{"initial background is transparent", `getComputedStyle(document.getElementById("cascaded")).backgroundColor`, "rgba(0, 0, 0, 0)"},
		{"initial z-index is auto", `getComputedStyle(document.getElementById("cascaded")).zIndex`, "auto"},
		{"initial visibility", `getComputedStyle(document.getElementById("cascaded")).visibility`, "visible"},
		{"unset line-height is normal", `getComputedStyle(document.getElementById("bare")).lineHeight`, "normal"},
		{"unset position is static", `getComputedStyle(document.getElementById("bare")).position`, "static"},
		{"used width of a block", `getComputedStyle(document.getElementById("bare")).width`, "784px"},
		{"declared width survives as the used value", `getComputedStyle(document.getElementById("cascaded")).width`, "200px"},
		{"used height from the line box", `getComputedStyle(document.getElementById("cascaded")).height`, "21.6px"},
		{"zero margin is a length", `getComputedStyle(document.getElementById("cascaded")).marginTop`, "0px"},
		{"inline beats cascade", `getComputedStyle(document.getElementById("cascaded")).color`, "#ee0000"},
		{"inline beats cascade by name", `getComputedStyle(document.getElementById("cascaded")).getPropertyValue("color")`, "#ee0000"},
		{"unmodelled by name", `getComputedStyle(document.getElementById("cascaded")).getPropertyValue("transform")`, ""},
		{"unmodelled as property", `typeof getComputedStyle(document.getElementById("cascaded")).transform`, "undefined"},
		{"unknown name", `getComputedStyle(document.getElementById("cascaded")).getPropertyValue("not-a-property")`, ""},
	}
	for _, c := range cases {
		if got := engineComputed(t, rt, c.script); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

// qc:BR-206
func TestGetComputedStyleForANodeTheSessionNeverResolved(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := engine.NewSession(computedPage, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	got := engineComputed(t, rt, `(function(){
		var d = document.createElement("div");
		document.body.appendChild(d);
		var cs = getComputedStyle(d);
		var empty = [
			cs.getPropertyValue("display"),
			cs.getPropertyValue("font-size"),
			typeof cs.display,
			typeof cs.fontSize
		].join("|");
		d.setAttribute("style", "display:block;width:120px;color:#0f0");
		var after = getComputedStyle(d);
		return [empty, after.getPropertyValue("display"), after.getPropertyValue("width"), after.color, typeof after.fontSize].join(" / ")
	})()`)
	want := strings.Join([]string{
		"block|16px|string|string",
		"block",
		"120px",
		"#0f0",
		"string",
	}, " / ")
	if got != want {
		t.Errorf("unresolved node computed style: got %q want %q", got, want)
	}
}

func visiblePaintText(t *testing.T, sess *engine.Session) string {
	t.Helper()
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	var b strings.Builder
	for _, cmd := range list.All() {
		if cmd.Kind != paint.CmdText {
			continue
		}
		for _, g := range cmd.Text.Glyphs {
			b.WriteRune(g.Rune)
		}
	}
	return b.String()
}

// qc:BR-206
func TestLifecycleRevealRepaintsTheDocument(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
<div id="server">SERVER TEXT</div>
<div id="reveal-holder" style="display:none"><span id="reveal">REVEALED TEXT</span></div>
<script>
document.addEventListener("DOMContentLoaded", function(){
  document.getElementById("reveal-holder").removeAttribute("style");
  window.__readyStateInHandler = document.readyState;
  window.__createdNodeType = String(document.createElement("p").nodeType);
});
window.addEventListener("load", function(){ window.__loadState = document.readyState; });
</script>
</body></html>`
	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	text := visiblePaintText(t, sess)
	if !strings.Contains(text, "SERVER TEXT") {
		t.Fatalf("the server text disappeared: %q", text)
	}
	if !strings.Contains(text, "REVEALED TEXT") {
		t.Errorf("the DOMContentLoaded reveal did not reach the paint list: %q", text)
	}
	if got := engineComputed(t, rt, `[window.__readyStateInHandler, window.__createdNodeType, window.__loadState, document.readyState, String(document.getElementById("reveal-holder").getAttribute("style") === null)].join("|")`); got != "interactive|1|complete|complete|true" {
		t.Errorf("lifecycle state seen by page handlers: got %q want %q", got, "interactive|1|complete|complete|true")
	}
	if got := engineComputed(t, rt, `getComputedStyle(document.getElementById("reveal")).display`); got != "inline" {
		t.Errorf("revealed node display: got %q want %q", got, "inline")
	}
	if got := engineComputed(t, rt, `getComputedStyle(document.getElementById("reveal-holder")).display`); got != "block" {
		t.Errorf("revealed holder display: got %q want %q", got, "block")
	}
}

// qc:BR-206
func TestLifecycleHideRemovesFromTheRender(t *testing.T) {
	html := `<!DOCTYPE html><html><head></head><body>
<div id="ad">AD CONTENT</div>
<div id="keep">KEEP CONTENT</div>
<script>
document.addEventListener("DOMContentLoaded", function(){
  document.getElementById("ad").style.display = "none";
});
</script>
</body></html>`
	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	text := visiblePaintText(t, sess)
	if strings.Contains(text, "AD CONTENT") {
		t.Errorf("display:none written from the lifecycle handler was ignored by the render: %q", text)
	}
	if !strings.Contains(text, "KEEP CONTENT") {
		t.Errorf("the sibling content disappeared too: %q", text)
	}
}

// qc:BR-206
func TestLifecycleReResolvesOnlyWhenTheHandlerMutated(t *testing.T) {
	const noHandler = "<!DOCTYPE html><html><head></head><body><div id=\"s\">STATIC CONTENT</div></body></html>"
	const inertHandler = "<!DOCTYPE html><html><head></head><body><div id=\"s\">STATIC CONTENT</div>" +
		"<script>document.addEventListener(\"DOMContentLoaded\", function(){ window.__ran = \"yes\"; });</script>" +
		"</body></html>"
	const revealingHandler = "<!DOCTYPE html><html><head></head><body><div id=\"h\" style=\"display:none\">HIDDEN CONTENT</div>" +
		"<script>document.addEventListener(\"DOMContentLoaded\", function(){ document.getElementById(\"h\").style.display = \"block\"; });</script>" +
		"</body></html>"

	counts := func(t *testing.T, html string, wantRan bool) engine.WorkCounts {
		t.Helper()
		rt, err := js.New(js.Options{URL: "https://example.test/page"})
		if err != nil {
			t.Fatal(err)
		}
		defer rt.Close()
		sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
		if err != nil {
			t.Fatal(err)
		}
		defer sess.Close()
		if wantRan {
			if v, err := rt.Evaluate(`window.__ran`); err != nil || v != "yes" {
				t.Fatalf("handler did not run: %q err=%v", v, err)
			}
		}
		return sess.WorkCounts()
	}

	bare := counts(t, noHandler, false)
	inert := counts(t, inertHandler, true)
	revealed := counts(t, revealingHandler, false)

	if inert.StyleResolves != bare.StyleResolves || inert.LayoutBuilds != bare.LayoutBuilds {
		t.Errorf("an inert lifecycle handler cost %d extra style resolves and %d extra layout builds: the session re-resolves on every fire instead of only after a DOM mutation",
			inert.StyleResolves-bare.StyleResolves, inert.LayoutBuilds-bare.LayoutBuilds)
	}
	if got, want := revealed.StyleResolves, bare.StyleResolves+1; got != want {
		t.Errorf("a revealing handler ran %d style resolves, want exactly %d (the load-time resolve plus one for the mutation)", got, want)
	}
	if got, want := revealed.LayoutBuilds, bare.LayoutBuilds+1; got != want {
		t.Errorf("a revealing handler ran %d layout builds, want exactly %d", got, want)
	}
}

// qc:BR-206
func TestGetComputedStyleListsTheNamesOfANodeInsertedThisTask(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	sess, err := engine.NewSession(computedPage, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	got := engineComputed(t, rt, `(function(){
		var d = document.createElement("div");
		document.body.appendChild(d);
		var cs = getComputedStyle(d);
		var names = Array.prototype.slice.call(cs);
		var readable = names.length === cs.length && cs.item(0) === cs[0];
		for (var i = 0; i < names.length; i++) {
			if (cs[names[i]] !== cs.getPropertyValue(names[i])) { readable = false; }
		}
		return [
			cs.getPropertyValue("display"),
			String(names.indexOf("display") >= 0),
			String(cs.parentRule === null),
			String(readable)
		].join("|");
	})()`)
	want := "block|true|true|true"
	if got != want {
		t.Errorf("same-task declaration list: got %q want %q", got, want)
	}
}
