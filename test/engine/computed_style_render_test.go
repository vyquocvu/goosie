package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/paint"
)

// The engine half of window.getComputedStyle and of the document lifecycle: what
// the session resolves, and what the render does with it.
//
// internal/js may only import {internal/dom, internal/media} (internal/archtest
// rules.go), so every value a page sees through getComputedStyle is produced
// here. Two things could not be pinned in test/js, and both are the point of this
// file:
//
//   - That the numbers are the resolver's and the layout box's rather than the
//     binding's invention. test/js/computed_style_test.go feeds a stub map, and a
//     stub that agrees with itself proves nothing about the cascade. The values
//     below are what a real `<style>` block plus a real layout pass produce,
//     including a used width (784px, which no declaration in the page says) and a
//     colour the engine normalised (rgb(0, 0, 0)).
//   - That firing DOMContentLoaded/load reaches the render. A content script's
//     whole trick is to change the DOM from that handler; a session that ran the
//     handler without re-resolving style would paint the page as if nothing had
//     happened, and the only symptom would be a blank or a stuck ad.

func engineComputed(t *testing.T, rt *js.Runtime, src string) string {
	t.Helper()
	v, err := rt.Evaluate(src)
	if err != nil {
		t.Fatalf("evaluate %s: %v", src, err)
	}
	return v
}

// The cascade under test: a stylesheet rule, an inline style that overrides one
// of its properties, and a node with nothing of its own so inheritance and the
// layout box are the only sources a value can come from.
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
		// Inherited from body through the cascade: the value can only be there if
		// the resolver ran and its result reached the binding.
		{"inherited font-size", `getComputedStyle(document.getElementById("bare")).fontSize`, "16px"},
		{"inherited colour normalised", `getComputedStyle(document.getElementById("bare")).color`, "rgb(0, 0, 0)"},
		// The rule's own values, in the spelling CSS uses.
		{"cascade font-size", `getComputedStyle(document.getElementById("cascaded")).fontSize`, "18px"},
		{"cascade display", `getComputedStyle(document.getElementById("cascaded")).display`, "block"},
		{"cascade text-align", `getComputedStyle(document.getElementById("cascaded")).textAlign`, "center"},
		{"cascade font-weight", `getComputedStyle(document.getElementById("cascaded")).fontWeight`, "400"},
		{"initial background is transparent", `getComputedStyle(document.getElementById("cascaded")).backgroundColor`, "rgba(0, 0, 0, 0)"},
		{"initial z-index is auto", `getComputedStyle(document.getElementById("cascaded")).zIndex`, "auto"},
		{"initial visibility", `getComputedStyle(document.getElementById("cascaded")).visibility`, "visible"},
		{"unset line-height is normal", `getComputedStyle(document.getElementById("bare")).lineHeight`, "normal"},
		{"unset position is static", `getComputedStyle(document.getElementById("bare")).position`, "static"},
		// Used values from the layout box, which is the part of this an author
		// cannot write: 784px is the body's content width in an 800px viewport
		// (8px margin each side), and no declaration in the page says it.
		{"used width of a block", `getComputedStyle(document.getElementById("bare")).width`, "784px"},
		{"declared width survives as the used value", `getComputedStyle(document.getElementById("cascaded")).width`, "200px"},
		{"used height from the line box", `getComputedStyle(document.getElementById("cascaded")).height`, "21.6px"},
		{"zero margin is a length", `getComputedStyle(document.getElementById("cascaded")).marginTop`, "0px"},
		// The inline style wins, and it wins in the author's own spelling:
		// internal/js cannot import internal/css, so it re-renders nothing and the
		// merge is the node's declaration over the snapshot. The decision and its
		// cost are recorded in internal/js/computed_style.go.
		{"inline beats cascade", `getComputedStyle(document.getElementById("cascaded")).color`, "#ee0000"},
		{"inline beats cascade by name", `getComputedStyle(document.getElementById("cascaded")).getPropertyValue("color")`, "#ee0000"},
		// A property this engine does not model has two shapes and both are
		// asserted, because they are different contracts: getPropertyValue
		// answers the empty string (CSSOM), an own property is absent (undefined).
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

// getComputedStyle performs a synchronous style refresh for script-created and
// script-mutated nodes, so CSSOM reads observe the current tree before the next
// paint pump runs.
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
		"block|16px|string|string", // resolved UA display and inherited font size
		"block",                    // the declarations the page wrote are known at once
		"120px",
		"#0f0",
		"string", // inherited font size remains resolved after the mutation
	}, " / ")
	if got != want {
		t.Errorf("unresolved node computed style: got %q want %q", got, want)
	}
}

// visiblePaintText collects the text a display list would actually draw, so a
// render assertion is about content rather than about command counts, which move
// for reasons unrelated to a reveal.
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

// The lifecycle has to reach the render. Pages reveal content from their
// DOMContentLoaded handler all the time, and the engine used to run scripts at a
// point where nothing could still change the load-time layout the session had
// already resolved.
//
// The assertions cover the three quiet ways this fails: the handler never runs
// (no dispatch at all), it runs but the session does not re-resolve (the paint
// list keeps the pre-reveal tree), or it re-resolves but readyState is wrong, so
// a handler that checks document.readyState first bails out.
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
	// The revealed node is styled and laid out, not merely present in the tree:
	// its span reports the inline display the cascade gives it once the
	// display:none is gone, which is what makes the paint list grow.
	if got := engineComputed(t, rt, `getComputedStyle(document.getElementById("reveal")).display`); got != "inline" {
		t.Errorf("revealed node display: got %q want %q", got, "inline")
	}
	if got := engineComputed(t, rt, `getComputedStyle(document.getElementById("reveal-holder")).display`); got != "block" {
		t.Errorf("revealed holder display: got %q want %q", got, "block")
	}
}

// The other direction: a lifecycle handler that hides content must also take it
// out of the render. Without the mutation-triggered re-resolve the page keeps
// painting text its own script removed from view, which shows up as "the engine
// renders things a browser does not".
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

// Scripts that never touch the DOM must not cost a second resolve, and a script
// that does must cost exactly one. The session asks FireDocumentLifecycle
// whether the tree changed and only then re-runs style+layout, so both halves of
// that sentence are measurable with the pipeline counters: a page whose handler
// writes nothing resolves exactly as many times as a page with no handler at
// all, and a page whose handler reveals content resolves once more, not once per
// mutation.
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
