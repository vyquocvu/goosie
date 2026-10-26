package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// window.getComputedStyle, document.defaultView, and the lookup chain jQuery's
// .css() runs through them.
//
// The count that puts this here: jQuery 3.6 calls getComputedStyle in three
// places and one of them is every `.css()` read a page makes, reached through a
// chain of four names, each of which has to be there:
//
//	Re = function (e) {
//	  var t = e.ownerDocument.defaultView;
//	  return t && t.opener || (t = C), t.getComputedStyle(e)
//	}
//	...
//	(n = n || Re(e)) && ("" !== (a = n.getPropertyValue(t) || n[t]))
//
// element.ownerDocument -> document.defaultView -> window.getComputedStyle ->
// an object answering BOTH getPropertyValue("font-size") and .fontSize. Miss
// any one and jQuery does not throw: it concludes the property is unset and
// keeps the page's `display:none`. A silent wrong answer is worse than a
// TypeError, so the tests below pin the chain and the empty-string contract
// separately.
//
// What the object reports is limited to what this engine can name: the cascade
// result the session resolved for that node plus the node's own style
// attribute. Nothing is derived, so an unmodelled property reads "" and the
// tests assert that too.

// styleProvider installs a stub cascade for a js.Runtime, keyed by the element
// id the test markup gives the node. The engine supplies the real conversion
// through the same hook; internal/js cannot import internal/style, so a test
// that wants cascade behaviour has to be the cascade.
func styleProvider(rt *js.Runtime, byID map[string]map[string]string) {
	rt.SetComputedStyleProvider(func(node *dom.Node) map[string]string {
		if node == nil {
			return nil
		}
		if id := node.GetAttribute("id"); id != "" {
			if vals, ok := byID[id]; ok {
				return vals
			}
		}
		return nil
	})
}

func styleRuntime(t *testing.T, html string, byID map[string]map[string]string) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(html)
	rt := newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
	if byID != nil {
		styleProvider(rt, byID)
	}
	return rt
}

const computedDoc = `<html><head></head><body>
<div id="art" style="color: rgb(1, 2, 3)">body</div>
<p id="plain">x</p>
</body></html>`

var computedCascade = map[string]map[string]string{
	"art": {
		"display":      "block",
		"font-size":    "14px",
		"width":        "600px",
		"text-align":   "center",
		"margin-left":  "0px",
		"border-width": "1px",
	},
	"plain": {
		"display":   "block",
		"font-size": "16px",
	},
}

// Both spellings of a property name are the same answer. jQuery reads
// `styles.fontSize` first and only then `getPropertyValue("font-size")`, and a
// page that finds neither reads "" and gives up.
// qc:BR-205
func TestGetComputedStyleAnswersKebabAndCamelNames(t *testing.T) {
	rt := styleRuntime(t, computedDoc, computedCascade)
	if err := rt.Run(`
	var el = document.getElementById("art");
	var cs = window.getComputedStyle(el);
	var out = [
		cs.getPropertyValue("font-size"),
		cs.fontSize,
		cs.display,
		cs.getPropertyValue("text-align"),
		cs.textAlign,
		cs.getPropertyValue("margin-left"),
		cs.marginLeft
	].join("|");`, "inline"); err != nil {
		t.Fatal(err)
	}
	got := evalString(t, rt, `out`)
	want := "14px|14px|block|center|center|0px|0px"
	if got != want {
		t.Fatalf("getComputedStyle reads = %q, want %q", got, want)
	}
}

// getPropertyValue also takes the camelCase spelling, because that is what
// jQuery's `.css()` passes after running the name through its own cache.
// qc:BR-205
func TestGetComputedStylePropertyValueAcceptsCamelCase(t *testing.T) {
	rt := styleRuntime(t, computedDoc, computedCascade)
	if got := evalString(t, rt, `
		var cs = getComputedStyle(document.getElementById("plain"));
		cs.getPropertyValue("fontSize") + "/" + cs.getPropertyValue("font-size")`); got != "16px/16px" {
		t.Fatalf("getPropertyValue camel reads = %q, want %q", got, "16px/16px")
	}
}

// The node's own style attribute wins over the cascade map, and a node the
// provider has no answer for still reports its inline declarations: a script
// that writes `el.style.display = "none"` and reads it back in the same run is
// asking about a declaration the engine holds.
// qc:BR-205
func TestGetComputedStyleMergesInlineOverCascade(t *testing.T) {
	rt := styleRuntime(t, computedDoc, computedCascade)
	if got := evalString(t, rt, `getComputedStyle(document.getElementById("art")).color`); got != "rgb(1, 2, 3)" {
		t.Fatalf("inline color = %q, want the element's own declaration", got)
	}
	// A created element is not in the session's cascade map at all.
	if got := evalString(t, rt, `
		var d = document.createElement("div");
		d.setAttribute("style", "display:none; font-size: 20px");
		getComputedStyle(d).display + "/" + getComputedStyle(d).fontSize`); got != "none/20px" {
		t.Fatalf("created element reads = %q, want none/20px", got)
	}
	// Writing through .style is visible to a later getComputedStyle, which is
	// what makes jQuery's show/hide round trip work.
	if got := evalString(t, rt, `
		var p = document.getElementById("plain");
		p.style.display = "none";
		getComputedStyle(p).display + "/" + getComputedStyle(p).getPropertyValue("display")`); got != "none/none" {
		t.Fatalf("after a style write = %q, want none/none", got)
	}
}

// Engine values only, in the two shapes "no value" actually has here.
// getPropertyValue answers the empty string for a name the engine does not
// carry, while the accessor for that same name is absent, so it reads
// `undefined` - which is why the third element below is a `typeof`: a plain
// `[..].join("|")` renders undefined as an empty cell and would not tell the
// two apart. CSSOM would answer "" for both; the deviation is recorded as
// GOOSIE-CSSOMTASK-001 in docs/tasks/TASK-047-DOM-JQUERY-APIS.md.
// getPropertyPriority has no important flags to report. Inventing "initial"
// values would make a missing cascade look like a resolved one.
// qc:BR-205
func TestGetComputedStyleOnlyReportsEngineValues(t *testing.T) {
	rt := styleRuntime(t, computedDoc, computedCascade)
	if err := rt.Run(`
		var cs = getComputedStyle(document.getElementById("art"));
		var out = [
			cs.getPropertyValue("line-height"),
			cs.getPropertyValue("not-a-property"),
			typeof cs.zIndex,
			cs.getPropertyPriority("display")
		].join("|");
		var undef = typeof cs.lineheight;`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `out`); got != "||undefined|" {
		t.Fatalf("unmodelled properties = %q, want %q (two empty getPropertyValue reads, an absent accessor, no priority)", got, "||undefined|")
	}
	// Undefined reads stay undefined rather than becoming the string "initial".
	if got := evalString(t, rt, `undef`); got != "undefined" {
		t.Fatalf("an absent property reads %q, want %q", got, "undefined")
	}
}

// A runtime with no provider at all - a host that never wired the engine's
// cascade - still answers with the element's inline declarations and an empty
// string for everything else, instead of throwing.
// qc:BR-205
func TestGetComputedStyleWithoutProviderFallsBackToInline(t *testing.T) {
	rt := styleRuntime(t, computedDoc, nil)
	if got := evalString(t, rt, `
		var cs = getComputedStyle(document.getElementById("art"));
		[cs.color, cs.getPropertyValue("display"), cs.fontSize].join("|")`); got != "rgb(1, 2, 3)||" {
		t.Fatalf("no-provider reads = %q, want %q", got, "rgb(1, 2, 3)||")
	}
}

// The whole jQuery `.css()` lookup, quoted from the library, against a real
// element. This is the test that says the four names fit together: a binding
// that works when called directly but not through ownerDocument.defaultView is
// the failure the page actually sees.
// qc:BR-205
func TestJQueryCssLookupChainResolves(t *testing.T) {
	rt := styleRuntime(t, computedDoc, computedCascade)
	src := `
	var C = window;                       // jQuery's own alias for window
	var Re = function (e) {
		var t = e.ownerDocument.defaultView;
		return t && t.opener || (t = C), t.getComputedStyle(e)
	};
	var styles = Re(document.getElementById("art"));
	var name = "font-size";
	var a = "";
	if ((styles) && ("" !== (a = styles.getPropertyValue(name) || styles[name]))) {
		window.cssResult = a;
	} else {
		window.cssResult = "MISS";
	}
	// The same call for a property nobody resolved answers nothing: getPropertyValue
	// gives the empty string and the direct read is absent, so jQuery's fallback
	// ends on undefined instead of on an invented initial value.
	var styles2 = Re(document.getElementById("art"));
	var b = styles2.getPropertyValue("line-height") || styles2.lineHeight;
	window.cssMissing = [styles2.getPropertyValue("line-height"), String(b), typeof styles2.lineHeight].join("|");
	window.selfIdent = String(document.defaultView === window) + "," +
		String(document.getElementById("art").ownerDocument.defaultView === window);`
	if err := rt.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `String(window.cssResult)`); got != "14px" {
		t.Fatalf("jQuery chain returned %q, want %q", got, "14px")
	}
	if got := evalString(t, rt, `String(window.cssMissing)`); got != "|undefined|undefined" {
		t.Fatalf("unmodelled property through the chain = %q, want %q", got, "|undefined|undefined")
	}
	if got := evalString(t, rt, `String(window.selfIdent)`); got != "true,true" {
		t.Fatalf("defaultView identity = %q, want %q", got, "true,true")
	}
}

// defaultView is the window object, and window.top is itself, so Sizzle's
// support probe
//
//	(n = C.defaultView) && n.top !== n && (n.addEventListener ? ... : ...)
//
// ends at the same place it does in a browser instead of taking the
// iframe/opener branch. addEventListener on that object is what the next task
// cluster pins.
// qc:BR-205
func TestDefaultViewIsTheWindowItself(t *testing.T) {
	rt := styleRuntime(t, computedDoc, computedCascade)
	if err := rt.Run(`
	var n = document.defaultView;
	var out = [
		String(n === window),
		String(n.top === n),
		String(n.parent === n),
		String(n.opener),
		typeof n.addEventListener,
		typeof n.getComputedStyle,
		String(n.document === document)
	].join("|");`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `out`); got != "true|true|true|null|function|function|true" {
		t.Fatalf("defaultView shape = %q, want %q", got, "true|true|true|null|function|function|true")
	}
}

// A missing or wrong-typed argument is a TypeError naming the parameter, the
// same shape the browser gives, so a page that guards with try/catch learns
// what failed instead of receiving an empty style object it will trust.
// qc:BR-205
func TestGetComputedStyleRejectsNonElementArguments(t *testing.T) {
	rt := styleRuntime(t, computedDoc, computedCascade)
	for _, src := range []string{
		`getComputedStyle()`,
		`getComputedStyle(null)`,
		`getComputedStyle("div")`,
		`getComputedStyle(42)`,
	} {
		if err := rt.Run(src, "inline"); err == nil {
			t.Fatalf("%s succeeded; a non-element argument must throw", src)
		} else if !strings.Contains(err.Error(), "TypeError") {
			t.Fatalf("%s failed with %v, want a TypeError", src, err)
		}
	}
	// A pseudo-element argument asks for a box the layout tree does not carry
	// as a node: the object exists, and every property in it is empty.
	if got := evalString(t, rt, `
		var cs = getComputedStyle(document.getElementById("art"), "::before");
		String(cs === null) + "/" + cs.getPropertyValue("content") + "/" + cs.display`); got != "false//block" {
		t.Fatalf("pseudo-element call = %q, want an object with no pseudo values", got)
	}
}
