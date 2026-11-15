package js_test

import (
	"encoding/json"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

func abstractCtorNames() []string {
	return []string{
		"Node", "Element", "Attr", "CharacterData",
		"StyleSheetList", "CSSRuleList", "MediaList",
		"DocumentType", "ProcessingInstruction", "HTMLCollection", "NodeList",
	}
}

func ctorProbeRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r := newDOMRuntime(t, testDOM(t))
	return r
}

func captureJSON(t *testing.T, r *js.Runtime, script string) map[string]map[string]string {
	t.Helper()
	got, err := r.Evaluate(script)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	var out map[string]map[string]string
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("unmarshal %q: %v", got, err)
	}
	return out
}

// qc:BR-298
func TestNewOnAbstractInterfaceThrowsPrefixedIllegalConstructor(t *testing.T) {
	r := ctorProbeRuntime(t)
	res := captureJSON(t, r, `(function(){
		var names = ['Node','Element','Attr','CharacterData','StyleSheetList','CSSRuleList','MediaList','DocumentType','ProcessingInstruction','HTMLCollection','NodeList'];
		var out = {};
		for (var i=0;i<names.length;i++){
			var n = names[i], F = globalThis[n];
			try { new F(); out[n] = {thrown:'no'}; }
			catch (e) { out[n] = {thrown:'yes', errName: e.name, message: e.message}; }
		}
		return JSON.stringify(out);
	})()`)
	for _, n := range abstractCtorNames() {
		row := res[n]
		if row["thrown"] != "yes" {
			t.Errorf("new %s() did not throw", n)
			continue
		}
		if row["errName"] != "TypeError" {
			t.Errorf("new %s() threw %q, want TypeError", n, row["errName"])
		}
		want := "Failed to construct '" + n + "': Illegal constructor"
		if row["message"] != want {
			t.Errorf("new %s() message = %q, want %q", n, row["message"], want)
		}
	}
}

// qc:BR-298
func TestCallingAbstractInterfaceThrowsBareIllegalConstructor(t *testing.T) {
	r := ctorProbeRuntime(t)
	res := captureJSON(t, r, `(function(){
		var names = ['Node','Element','Attr','CharacterData','StyleSheetList','CSSRuleList','MediaList','DocumentType','ProcessingInstruction','HTMLCollection','NodeList'];
		var out = {};
		for (var i=0;i<names.length;i++){
			var n = names[i], F = globalThis[n];
			try { F(); out[n] = {thrown:'no'}; }
			catch (e) { out[n] = {thrown:'yes', errName: e.name, message: e.message}; }
		}
		return JSON.stringify(out);
	})()`)
	for _, n := range abstractCtorNames() {
		row := res[n]
		if row["thrown"] != "yes" {
			t.Errorf("%s() call did not throw", n)
			continue
		}
		if row["errName"] != "TypeError" {
			t.Errorf("%s() threw %q, want TypeError", n, row["errName"])
		}
		if row["message"] != "Illegal constructor" {
			t.Errorf("%s() message = %q, want bare %q", n, row["message"], "Illegal constructor")
		}
	}
}

// qc:BR-306
func TestCSSOMAbstractAndShadowRootConstructorsUseTheSharedLaw(t *testing.T) {
	r := ctorProbeRuntime(t)
	res := captureJSON(t, r, `(function(){
		var names = ['CSSRule','CSSStyleRule','ShadowRoot'];
		var out = {};
		for (var i=0;i<names.length;i++){
			var n = names[i], F = globalThis[n];
			var row = {present: typeof F, fname: F && F.name, protoCtorEq: String(!!(F && F.prototype && F.prototype.constructor === F))};
			try { F(); row.call = 'no'; } catch (e) { row.call = e.name + ': ' + e.message; }
			try { new F(); row.news = 'no'; } catch (e) { row.news = e.name + ': ' + e.message; }
			out[n] = row;
		}
		return JSON.stringify(out);
	})()`)
	for _, n := range []string{"CSSRule", "CSSStyleRule", "ShadowRoot"} {
		row := res[n]
		if row["present"] != "function" {
			t.Errorf("typeof %s = %q, want function", n, row["present"])
			continue
		}
		if row["call"] != "TypeError: Illegal constructor" {
			t.Errorf("%s() threw %q, want TypeError: Illegal constructor", n, row["call"])
		}
		want := "TypeError: Failed to construct '" + n + "': Illegal constructor"
		if row["news"] != want {
			t.Errorf("new %s() threw %q, want %q", n, row["news"], want)
		}
		if row["fname"] != n {
			t.Errorf("%s.name = %q, want %q", n, row["fname"], n)
		}
		if row["protoCtorEq"] != "true" {
			t.Errorf("%s.prototype.constructor === %s is false", n, n)
		}
	}
}

// qc:BR-306
func TestCSSStyleSheetConstructorRequiresTheNewOperator(t *testing.T) {
	r := ctorProbeRuntime(t)
	res := captureJSON(t, r, `(function(){
		var row = {};
		try { CSSStyleSheet(); row.call = 'no'; }
		catch (e) { row.call = e.name + ': ' + e.message; }
		try { var s = new CSSStyleSheet(); row.news = 'object'; row.instanceof = String(s instanceof CSSStyleSheet); }
		catch (e) { row.news = e.name + ': ' + e.message; }
		return JSON.stringify({CSSStyleSheet: row});
	})()`)
	want := "TypeError: Failed to construct 'CSSStyleSheet': Please use the 'new' operator, this DOM object constructor cannot be called as a function."
	if res["CSSStyleSheet"]["call"] != want {
		t.Errorf("CSSStyleSheet() threw %q, want %q", res["CSSStyleSheet"]["call"], want)
	}
	if res["CSSStyleSheet"]["news"] != "object" {
		t.Errorf("new CSSStyleSheet() = %q, want object", res["CSSStyleSheet"]["news"])
	}
	if res["CSSStyleSheet"]["instanceof"] != "true" {
		t.Errorf("new CSSStyleSheet() instanceof CSSStyleSheet = %q, want true", res["CSSStyleSheet"]["instanceof"])
	}
}

// qc:BR-298
func TestAbstractInterfaceConstructorNameAndPrototypeSurviveTheBranch(t *testing.T) {
	r := ctorProbeRuntime(t)
	res := captureJSON(t, r, `(function(){
		var names = ['Node','Element','Attr','CharacterData','StyleSheetList','CSSRuleList','MediaList','DocumentType','ProcessingInstruction','HTMLCollection','NodeList'];
		var out = {};
		for (var i=0;i<names.length;i++){
			var n = names[i], F = globalThis[n];
			out[n] = {fname: F.name, protoCtorEq: String(!!(F.prototype && F.prototype.constructor === F))};
		}
		return JSON.stringify(out);
	})()`)
	for _, n := range abstractCtorNames() {
		row := res[n]
		if row["fname"] != n {
			t.Errorf("%s.name = %q, want %q", n, row["fname"], n)
		}
		if row["protoCtorEq"] != "true" {
			t.Errorf("%s.prototype.constructor === %s is false", n, n)
		}
	}
}
