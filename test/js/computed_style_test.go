package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

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

// qc:BR-205
func TestGetComputedStylePropertyValueAcceptsCamelCase(t *testing.T) {
	rt := styleRuntime(t, computedDoc, computedCascade)
	if got := evalString(t, rt, `
		var cs = getComputedStyle(document.getElementById("plain"));
		cs.getPropertyValue("fontSize") + "/" + cs.getPropertyValue("font-size")`); got != "16px/16px" {
		t.Fatalf("getPropertyValue camel reads = %q, want %q", got, "16px/16px")
	}
}

// qc:BR-205
func TestGetComputedStyleMergesInlineOverCascade(t *testing.T) {
	rt := styleRuntime(t, computedDoc, computedCascade)
	if got := evalString(t, rt, `getComputedStyle(document.getElementById("art")).color`); got != "rgb(1, 2, 3)" {
		t.Fatalf("inline color = %q, want the element's own declaration", got)
	}
	if got := evalString(t, rt, `
		var d = document.createElement("div");
		d.setAttribute("style", "display:none; font-size: 20px");
		getComputedStyle(d).display + "/" + getComputedStyle(d).fontSize`); got != "none/20px" {
		t.Fatalf("created element reads = %q, want none/20px", got)
	}
	if got := evalString(t, rt, `
		var p = document.getElementById("plain");
		p.style.display = "none";
		getComputedStyle(p).display + "/" + getComputedStyle(p).getPropertyValue("display")`); got != "none/none" {
		t.Fatalf("after a style write = %q, want none/none", got)
	}
}

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
	if got := evalString(t, rt, `undef`); got != "undefined" {
		t.Fatalf("an absent property reads %q, want %q", got, "undefined")
	}
}

// qc:BR-205
func TestGetComputedStyleWithoutProviderFallsBackToInline(t *testing.T) {
	rt := styleRuntime(t, computedDoc, nil)
	if got := evalString(t, rt, `
		var cs = getComputedStyle(document.getElementById("art"));
		[cs.color, cs.getPropertyValue("display"), cs.fontSize].join("|")`); got != "rgb(1, 2, 3)||" {
		t.Fatalf("no-provider reads = %q, want %q", got, "rgb(1, 2, 3)||")
	}
}

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
	if got := evalString(t, rt, `
		var cs = getComputedStyle(document.getElementById("art"), "::before");
		String(cs === null) + "/" + cs.getPropertyValue("content") + "/" + cs.display`); got != "false//block" {
		t.Fatalf("pseudo-element call = %q, want an object with no pseudo values", got)
	}
}
