package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

type pseudoCall struct {
	id     string
	pseudo string
}

func pseudoRecorder(rt *js.Runtime, byKey map[string]map[string]string, seen *[]pseudoCall) {
	rt.SetPseudoStyleProvider(func(node *dom.Node, pseudo string) map[string]string {
		id := ""
		if node != nil {
			id = node.GetAttribute("id")
		}
		*seen = append(*seen, pseudoCall{id: id, pseudo: pseudo})
		if vals, ok := byKey[id+"\x00"+pseudo]; ok {
			return vals
		}
		return nil
	})
}

func pseudoRuntime(t *testing.T, html string, element map[string]map[string]string, pseudo map[string]map[string]string, seen *[]pseudoCall) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(html)
	rt := newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
	styleProvider(rt, element)
	pseudoRecorder(rt, pseudo, seen)
	return rt
}

const pseudoDoc = `<html><head></head><body>
<div id="host" style="color: rgb(1, 2, 3)">host</div>
<p id="other">x</p>
</body></html>`

var pseudoElementCascade = map[string]map[string]string{
	"host": {
		"display":   "block",
		"font-size": "16px",
		"width":     "600px",
		"color":     "rgb(10, 20, 30)",
	},
	"other": {"display": "block"},
}

func pseudoAnswerTable() map[string]map[string]string {
	return map[string]map[string]string{
		"host\x00before": {
			"content":   `"PRE"`,
			"color":     "rgb(200, 0, 0)",
			"font-size": "12px",
			"display":   "inline-block",
			"width":     "40px",
		},
		"host\x00after":      {"content": `""`, "color": "rgb(7, 8, 9)"},
		"host\x00first-line": {"color": "rgb(40, 50, 60)"},
		"other\x00before":    {"color": "rgb(1, 1, 1)"},
	}
}

func evalJS(t *testing.T, rt *js.Runtime, src string) string {
	t.Helper()
	v, err := rt.Evaluate(src)
	if err != nil {
		t.Fatalf("evaluate %s: %v", src, err)
	}
	return v
}

// qc:BR-322
func TestGetComputedStylePseudoArgumentResolvesThePseudoStyle(t *testing.T) {
	var seen []pseudoCall
	rt := pseudoRuntime(t, pseudoDoc, pseudoElementCascade, pseudoAnswerTable(), &seen)

	got := evalJS(t, rt, `(function(){
		var cs = getComputedStyle(document.getElementById("host"), "::before");
		return [
			cs.content,
			cs.color,
			cs.fontSize,
			cs.display,
			cs.getPropertyValue("width"),
			String(cs.color === cs.getPropertyValue("color")),
			String(typeof cs.parentRule),
			cs.getPropertyValue("height")
		].join("|");
	})()`)
	want := `"PRE"|rgb(200, 0, 0)|12px|inline-block|40px|true|object|`
	if got != want {
		t.Errorf("pseudo read: got %q want %q", got, want)
	}
	if len(seen) != 1 || seen[0].id != "host" || seen[0].pseudo != "before" {
		t.Fatalf("pseudo provider calls: %+v, want exactly one (host, before)", seen)
	}
}

// qc:BR-322
func TestGetComputedStylePseudoArgumentDoesNotReadTheElementStyleAttribute(t *testing.T) {
	var seen []pseudoCall
	rt := pseudoRuntime(t, pseudoDoc, pseudoElementCascade, pseudoAnswerTable(), &seen)

	got := evalJS(t, rt, `getComputedStyle(document.getElementById("host"), "::before").color`)
	if got != "rgb(200, 0, 0)" {
		t.Fatalf("pseudo colour: got %q want the authored pseudo value", got)
	}
	got = evalJS(t, rt, `getComputedStyle(document.getElementById("host")).color`)
	if got != "rgb(1, 2, 3)" {
		t.Errorf("element colour lost the style attribute: got %q want rgb(1, 2, 3)", got)
	}
}

// qc:BR-322
func TestGetComputedStylePseudoArgumentSpellingsCanonicalise(t *testing.T) {
	cases := []struct{ src, want string }{
		{"::before", "before"},
		{":before", "before"},
		{"::BEFORE", "before"},
		{":After", "after"},
		{"before", "before"},
		{"first-line", "first-line"},
		{"::FIRST-LETTER", "first-letter"},
		{"::part(widget)", "part"},
		{"::slotted(div)", "slotted"},
		{"::highlight(x)", "highlight"},
	}
	for _, c := range cases {
		var seen []pseudoCall
		rt := pseudoRuntime(t, pseudoDoc, pseudoElementCascade, pseudoAnswerTable(), &seen)
		evalJS(t, rt, `getComputedStyle(document.getElementById("other"), `+`"`+c.src+`")`)
		if len(seen) != 1 || seen[0].pseudo != c.want {
			t.Errorf("argument %q resolved to %+v, want one call with name %q", c.src, seen, c.want)
		}
	}
}

// qc:BR-322
func TestGetComputedStylePseudoArgumentFallsBackToTheElementStyle(t *testing.T) {
	cases := []string{`""`, `" "`, `" :before"`, `" ::before "`, `"nope"`, `"null"`, `"42"`, "null", "undefined"}
	for _, src := range cases {
		var seen []pseudoCall
		rt := pseudoRuntime(t, pseudoDoc, pseudoElementCascade, pseudoAnswerTable(), &seen)
		got := evalJS(t, rt, `getComputedStyle(document.getElementById("host"), `+src+`).color`)
		if got != "rgb(1, 2, 3)" {
			t.Errorf("argument %s answered %q, want the element style attribute", src, got)
		}
		for _, call := range seen {
			if call.pseudo != "" {
				t.Errorf("argument %s reached the pseudo provider as %q", src, call.pseudo)
			}
		}
	}
	for _, src := range []string{``, `, undefined`} {
		var seen []pseudoCall
		rt := pseudoRuntime(t, pseudoDoc, pseudoElementCascade, pseudoAnswerTable(), &seen)
		got := evalJS(t, rt, `getComputedStyle(document.getElementById("host")`+src+`).color`)
		if got != "rgb(1, 2, 3)" {
			t.Errorf("one-argument form answered %q, want the element style attribute", got)
		}
		if len(seen) != 0 {
			t.Errorf("one-argument form reached the pseudo provider: %+v", seen)
		}
	}
}

// qc:BR-322
func TestGetComputedStyleInvalidPseudoArgumentIsEmptyDeclaration(t *testing.T) {
	cases := []string{"::nope", ":hover", "::-moz-none", ":::before", "::before ", "::before;", "::before extra", "::part()", "::part", ":: before"}
	for _, src := range cases {
		var seen []pseudoCall
		rt := pseudoRuntime(t, pseudoDoc, pseudoElementCascade, pseudoAnswerTable(), &seen)
		got := evalJS(t, rt, `(function(){
			var cs = getComputedStyle(document.getElementById("host"), "`+src+`");
			return [
				String(cs === null),
				String(typeof cs),
				String(cs.length),
				cs.getPropertyValue("color"),
				cs.color,
				cs.item(0)
			].join("|");
		})()`)
		if got != "false|object|0|||" {
			t.Errorf("invalid argument %q: got %q want an empty declaration", src, got)
		}
		if len(seen) != 0 {
			t.Errorf("invalid argument %q reached the pseudo provider: %+v", src, seen)
		}
	}
}

// qc:BR-322
func TestGetComputedStylePseudoDeclarationListsItsOwnNames(t *testing.T) {
	var seen []pseudoCall
	rt := pseudoRuntime(t, pseudoDoc, pseudoElementCascade, pseudoAnswerTable(), &seen)

	got := evalJS(t, rt, `(function(){
		var cs = getComputedStyle(document.getElementById("host"), "::before");
		var names = Array.prototype.slice.call(cs);
		var readable = names.length === cs.length && cs.item(0) === cs[0] && cs.parentRule === null;
		for (var i = 0; i < names.length; i++) {
			if (cs[names[i]] !== cs.getPropertyValue(names[i])) { readable = false; }
			if (cs[camel(names[i])] !== cs.getPropertyValue(names[i])) { readable = false; }
		}
		function camel(n){ return n.replace(/-([a-z])/g, function(m,c){ return c.toUpperCase(); }); }
		return [names.join(","), String(readable), String(cs.length), cs.item(99999)].join(" / ");
	})()`)
	want := "color,content,display,font-size,width / true / 5 / "
	if got != want {
		t.Errorf("pseudo declaration list: got %q want %q", got, want)
	}
}

// qc:BR-322
func TestGetComputedStylePseudoWithoutAResolvedStyleFallsBackToTheElement(t *testing.T) {
	var seen []pseudoCall
	rt := pseudoRuntime(t, pseudoDoc, pseudoElementCascade, pseudoAnswerTable(), &seen)

	got := evalJS(t, rt, `(function(){
		var cs = getComputedStyle(document.getElementById("host"), "::marker");
		return [cs.getPropertyValue("display"), cs.getPropertyValue("width"), cs.color].join("|");
	})()`)
	if got != "block|600px|rgb(1, 2, 3)" {
		t.Errorf("unrecorded pseudo fell back wrongly: got %q", got)
	}
	if len(seen) != 1 || seen[0].pseudo != "marker" {
		t.Errorf("provider calls: %+v, want one marker call", seen)
	}
}
