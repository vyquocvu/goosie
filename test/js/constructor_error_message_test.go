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

// qc:BR-307
func TestConstructibleDOMNodeConstructorsReturnRealNodes(t *testing.T) {
	r := ctorProbeRuntime(t)
	res := captureJSON(t, r, `(function(){
		var out = {};
		var names = ['Text','Comment','DocumentFragment'];
		for (var i=0;i<names.length;i++){
			var n = names[i], F = globalThis[n];
			var row = {present: typeof F, fname: F.name, protoCtorEq: String(!!(F.prototype && F.prototype.constructor === F))};
			try { F(); row.call = 'no'; } catch (e) { row.call = e.name + ': ' + e.message; }
			out[n] = row;
		}
		var t = new Text();
		out.newText = {nodeType: String(t.nodeType), nodeName: t.nodeName, data: t.data, toString: Object.prototype.toString.call(t), instanceofText: String(t instanceof Text), instanceofNode: String(t instanceof Node)};
		var th = new Text('hi'); out.newTextHi = {data: th.data, textContent: th.textContent};
		var tu = new Text(undefined); out.newTextUndef = {data: tu.data};
		var tn = new Text(null); out.newTextNull = {data: tn.data};
		var cu = new Comment(undefined); out.newCommentUndef = {data: cu.data};
		var c = new Comment();
		out.newComment = {nodeType: String(c.nodeType), nodeName: c.nodeName, data: c.data, toString: Object.prototype.toString.call(c), instanceofComment: String(c instanceof Comment)};
		var ch = new Comment('hi'); out.newCommentHi = {data: ch.data};
		var df = new DocumentFragment();
		out.newFrag = {nodeType: String(df.nodeType), nodeName: df.nodeName, toString: Object.prototype.toString.call(df), instanceofFrag: String(df instanceof DocumentFragment)};
		document.body.appendChild(new Text('appended'));
		out.appended = {last: String(document.body.lastChild.textContent)};
		var frag = new DocumentFragment(); frag.appendChild(new Text('f')); document.body.appendChild(frag);
		out.fragMoved = {last: String(document.body.lastChild.textContent)};
		return JSON.stringify(out);
	})()`)
	newOpMsg := func(n string) string {
		return "TypeError: Failed to construct '" + n + "': Please use the 'new' operator, this DOM object constructor cannot be called as a function."
	}
	for _, n := range []string{"Text", "Comment", "DocumentFragment"} {
		row := res[n]
		if row["present"] != "function" {
			t.Errorf("typeof %s = %q, want function", n, row["present"])
		}
		if row["call"] != newOpMsg(n) {
			t.Errorf("%s() threw %q, want %q", n, row["call"], newOpMsg(n))
		}
		if row["fname"] != n {
			t.Errorf("%s.name = %q, want %q", n, row["fname"], n)
		}
		if row["protoCtorEq"] != "true" {
			t.Errorf("%s.prototype.constructor === %s is false", n, n)
		}
	}
	tt := res["newText"]
	if tt["nodeType"] != "3" || tt["nodeName"] != "#text" || tt["data"] != "" {
		t.Errorf("new Text() = %s/%s/data %q, want 3/#text/", tt["nodeType"], tt["nodeName"], tt["data"])
	}
	if tt["toString"] != "[object Text]" {
		t.Errorf("Object.prototype.toString.call(new Text()) = %q, want [object Text]", tt["toString"])
	}
	if tt["instanceofText"] != "true" || tt["instanceofNode"] != "true" {
		t.Errorf("new Text() instanceof Text/Node = %s/%s, want true/true", tt["instanceofText"], tt["instanceofNode"])
	}
	if res["newTextHi"]["data"] != "hi" || res["newTextHi"]["textContent"] != "hi" {
		t.Errorf("new Text('hi') data/textContent = %q/%q, want hi/hi", res["newTextHi"]["data"], res["newTextHi"]["textContent"])
	}
	if res["newTextUndef"]["data"] != "" {
		t.Errorf("new Text(undefined) data = %q, want empty (WebIDL default)", res["newTextUndef"]["data"])
	}
	if res["newTextNull"]["data"] != "null" {
		t.Errorf("new Text(null) data = %q, want the literal 'null'", res["newTextNull"]["data"])
	}
	if res["newCommentUndef"]["data"] != "" {
		t.Errorf("new Comment(undefined) data = %q, want empty (WebIDL default)", res["newCommentUndef"]["data"])
	}
	cc := res["newComment"]
	if cc["nodeType"] != "8" || cc["nodeName"] != "#comment" || cc["data"] != "" || cc["toString"] != "[object Comment]" || cc["instanceofComment"] != "true" {
		t.Errorf("new Comment() = %s/%s/data %q/%s/inst %s, want 8/#comment//'[object Comment]'/true", cc["nodeType"], cc["nodeName"], cc["data"], cc["toString"], cc["instanceofComment"])
	}
	if res["newCommentHi"]["data"] != "hi" {
		t.Errorf("new Comment('hi') data = %q, want hi", res["newCommentHi"]["data"])
	}
	df := res["newFrag"]
	if df["nodeType"] != "11" || df["nodeName"] != "#document-fragment" || df["toString"] != "[object DocumentFragment]" || df["instanceofFrag"] != "true" {
		t.Errorf("new DocumentFragment() = %s/%s/%s/inst %s, want 11/#document-fragment/'[object DocumentFragment]'/true", df["nodeType"], df["nodeName"], df["toString"], df["instanceofFrag"])
	}
	if res["appended"]["last"] != "appended" {
		t.Errorf("appended new Text('appended') read back %q, want appended", res["appended"]["last"])
	}
	if res["fragMoved"]["last"] != "f" {
		t.Errorf("DocumentFragment children did not move in; body.lastChild = %q, want f", res["fragMoved"]["last"])
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
