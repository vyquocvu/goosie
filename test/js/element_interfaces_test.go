package js_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// staticImageLoader answers data: sources with fixed dimensions. The real
// decode path (fetcher, Probe, pixel reservation) is an engine concern; this
// stub only proves the binding drives the injected loader and settles state.
type staticImageLoader struct{}

func (staticImageLoader) LoadScriptImage(src string, done func(js.LoadedImage)) {
	if strings.HasPrefix(src, "data:image/png") {
		done(js.LoadedImage{Src: src, Width: 1, Height: 1})
		return
	}
	done(js.LoadedImage{Src: src, Err: errors.New("test loader: unsupported source")})
}

// TASK-052 / GOOSIE-IDLREFLECT-001, GOOSIE-IMAGE-001.
//
// Every expectation in this file was measured in Chromium first with the
// throwaway Playwright probes under /tmp/t052 (probe1..probe6). The document
// URL is https://example.test/docs/page.html and the fixture matches §1:
// <div id=d class="c0">, <a id=a href="/p?q=1">link</a>, <img id=i src="x.png"
// width="10">, <input id=in value="v0">.
//
// Scripts return primitives because js.Runtime.Evaluate stringifies the result;
// arrays are joined explicitly so null and undefined cannot hide behind an
// empty Array slot (the lesson from test/js/document_implementation_test.go).

func interfacesRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(`<!DOCTYPE html><html><head><title>t</title></head><body>
		<div id="d" class="c0">x</div>
		<a id="a" href="/p?q=1">link</a>
		<img id="i" src="x.png" width="10">
		<input id="in" value="v0">
	</body></html>`)
	return newRuntime(t, js.Options{URL: "https://example.test/docs/page.html", DOM: doc, ImageLoader: staticImageLoader{}})
}

type jsCase struct {
	name   string
	script string
	want   string
}

func runJSCases(t *testing.T, rt *js.Runtime, cases []jsCase) {
	t.Helper()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := evalString(t, rt, tc.script); got != tc.want {
				t.Errorf("%s = %q, want %q (script %s)", tc.name, got, tc.want, tc.script)
			}
		})
	}
}

// qc:BR-236
func TestElementPrototypeChainIsInherited(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"div constructor is published", `typeof HTMLDivElement`, "function"},
		{"div prototype is not Object.prototype", `Object.getPrototypeOf(document.getElementById('d')) === Object.prototype`, "false"},
		{"div chain", `(function(){
			var c = Object.getPrototypeOf(document.getElementById('d')), out = [];
			while (c) { out.push(c.constructor && c.constructor.name); c = Object.getPrototypeOf(c); }
			return out.join('>');
		})()`, "HTMLDivElement>HTMLElement>Element>Node>EventTarget>Object"},
		{"anchor chain", `(function(){
			var c = Object.getPrototypeOf(document.getElementById('a')), out = [];
			while (c) { out.push(c.constructor && c.constructor.name); c = Object.getPrototypeOf(c); }
			return out.join('>');
		})()`, "HTMLAnchorElement>HTMLElement>Element>Node>EventTarget>Object"},
		{"image chain", `(function(){
			var c = Object.getPrototypeOf(document.getElementById('i')), out = [];
			while (c) { out.push(c.constructor && c.constructor.name); c = Object.getPrototypeOf(c); }
			return out.join('>');
		})()`, "HTMLImageElement>HTMLElement>Element>Node>EventTarget>Object"},
		{"input chain", `(function(){
			var c = Object.getPrototypeOf(document.getElementById('in')), out = [];
			while (c) { out.push(c.constructor && c.constructor.name); c = Object.getPrototypeOf(c); }
			return out.join('>');
		})()`, "HTMLInputElement>HTMLElement>Element>Node>EventTarget>Object"},
		{"created and parsed share one prototype", `Object.getPrototypeOf(document.createElement('div')) === Object.getPrototypeOf(document.getElementById('d'))`, "true"},
		// Unknown HTML names use HTMLUnknownElement while remaining in the
		// HTMLElement inheritance chain.
		{"unknown tag inherits HTMLElement", `document.createElement('foo') instanceof HTMLElement`, "true"},
		{"unknown tag has HTMLUnknownElement interface", `document.createElement('foo').constructor === HTMLUnknownElement`, "true"},
		{"custom element name uses HTMLElement", `document.createElement('x-widget').constructor === HTMLElement`, "true"},
		{"standard tag outside the interface table answers HTMLElement", `document.createElement('section').constructor === HTMLElement`, "true"},
		{"style string tag matches HTMLStyleElement", `String(document.createElement('style'))`, "[object HTMLStyleElement]"},
		{"foreign element string tag is Element", `String(document.createElementNS('urn:svg', 'style'))`, "[object Element]"},
		{"text node string tag matches Text", `String(document.createTextNode('x'))`, "[object Text]"},
		{"iframe sandbox forwards to stable token list", `(function(){var e=document.createElement('iframe');e.sandbox='allow-same-origin';return [e.hasAttribute('sandbox'),e.sandbox.value,e.sandbox===e.sandbox].join(',')})()`, "true,allow-same-origin,true"},
		{"div is HTMLElement", `document.getElementById('d') instanceof HTMLElement`, "true"},
		{"div is Element", `document.getElementById('d') instanceof Element`, "true"},
		{"div is Node", `document.getElementById('d') instanceof Node`, "true"},
		{"div is EventTarget", `document.getElementById('d') instanceof EventTarget`, "true"},
		{"div is HTMLDivElement", `document.getElementById('d') instanceof HTMLDivElement`, "true"},
		{"anchor is HTMLAnchorElement", `document.getElementById('a') instanceof HTMLAnchorElement`, "true"},
		{"anchor is HTMLElement", `document.getElementById('a') instanceof HTMLElement`, "true"},
		{"image is HTMLImageElement", `document.getElementById('i') instanceof HTMLImageElement`, "true"},
		{"input is HTMLInputElement", `document.getElementById('in') instanceof HTMLInputElement`, "true"},
		{"document is Document", `document instanceof Document`, "true"},
		{"document is Node", `document instanceof Node`, "true"},
		{"Text is not Element", `document.createTextNode('t') instanceof Element`, "false"},
		{"HTMLElement is an illegal constructor", `(function(){try{new HTMLElement();return 'no'}catch(e){return e instanceof TypeError}})()`, "true"},
		{"Element is an illegal constructor", `(function(){try{new Element();return 'no'}catch(e){return e instanceof TypeError}})()`, "true"},
		{"Node is an illegal constructor", `(function(){try{new Node();return 'no'}catch(e){return e instanceof TypeError}})()`, "true"},
		{"HTMLDivElement is an illegal constructor", `(function(){try{new HTMLDivElement();return 'no'}catch(e){return e instanceof TypeError}})()`, "true"},
		{"title accessor lives on HTMLElement.prototype", `(function(){
			var o = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'title');
			return o ? [!!o.get, !!o.set, o.enumerable, o.configurable].join(',') : 'null';
		})()`, "true,true,true,true"},
		{"className is inherited, not an own property", `Object.getOwnPropertyDescriptor(document.getElementById('d'), 'className') === undefined`, "true"},
	})
}

func TestOptionSelectednessIsLiveStateSeparateFromDefaultAttribute(t *testing.T) {
	doc := domtest.Parse(`<form id="f"><select id="s"><option id="a" selected>A</option><option id="b">B</option></select></form>`)
	rt := newRuntime(t, js.Options{DOM: doc})
	got := evalString(t, rt, `(function(){
		var a=document.getElementById('a'), b=document.getElementById('b'), f=document.getElementById('f');
		var initial=[a.selected,a.defaultSelected,b.selected].join(',');
		b.selected=true;
		var assigned=[a.selected,b.selected,a.hasAttribute('selected'),b.hasAttribute('selected')].join(',');
		a.removeAttribute('selected');
		a.setAttribute('selected','');
		var dirtyAttr=[a.selected,a.defaultSelected,b.selected].join(',');
		f.reset();
		return [initial,assigned,dirtyAttr,[a.selected,b.selected].join(',')].join('|');
	})()`)
	want := "true,true,false|false,true,true,false|false,true,true|true,false"
	if got != want {
		t.Fatalf("selectedness states = %q, want %q", got, want)
	}
}

func TestChangingSelectedAttributeValueWithoutTogglingPresenceIsNoOp(t *testing.T) {
	doc := domtest.Parse(`<select id="s"><option id="b" value="B" selected>B</option><option id="a" value="A" selected>A</option></select>`)
	rt := newRuntime(t, js.Options{DOM: doc})
	got := evalString(t, rt, `(function(){
		var s=document.getElementById('s'), b=document.getElementById('b'), a=document.getElementById('a');
		b.selected=true;
		a.setAttribute('selected','yes');
		return [s.value,b.selected,a.selected,a.defaultSelected,a.getAttribute('selected')].join(',');
	})()`)
	if got != "B,true,false,true,yes" {
		t.Fatalf("same-presence selected attribute write changed selectedness: %q", got)
	}
}

func TestSelectValueAndSelectedIndexUseLiveOptionState(t *testing.T) {
	doc := domtest.Parse(`<select id="s"><option id="a" value="A" selected>A</option><option id="b" value="B">B</option></select>`)
	rt := newRuntime(t, js.Options{DOM: doc})
	got := evalString(t, rt, `(function(){
		var s=document.getElementById('s'), a=document.getElementById('a'), b=document.getElementById('b');
		var initial=[s.value,s.selectedIndex].join(',');
		b.selected=true;
		var assigned=[s.value,s.selectedIndex,a.selected,b.selected,a.hasAttribute('selected')].join(',');
		s.value='A';
		var byValue=[s.value,s.selectedIndex,a.selected,b.selected].join(',');
		s.selectedIndex=-1;
		return [initial,assigned,byValue,[s.value,s.selectedIndex,a.defaultSelected,a.selected].join(',')].join('|');
	})()`)
	want := "A,0|B,1,false,true,true|A,0,true,false|,-1,true,false"
	if got != want {
		t.Fatalf("select current state = %q, want %q", got, want)
	}
}

func TestSelectOptionsAndSelectedOptionsCollections(t *testing.T) {
	doc := domtest.Parse(`<select id="s" multiple><option id="a" value="A" selected>A</option><option id="b" value="B">B</option></select>`)
	rt := newRuntime(t, js.Options{DOM: doc})
	got := evalString(t, rt, `(function(){
		var s=document.getElementById('s'), o=s.options, so=s.selectedOptions;
		var initial=[o===s.options,so===s.selectedOptions,o instanceof HTMLOptionsCollection,so instanceof HTMLCollection,o.length,o[0].id,o.item(1).id,o.namedItem('b').id,o.b.id,so.length,so[0].id].join(',');
		document.getElementById('b').selected=true;
		var liveSelected=[so.length,so[1].id].join(',');
		s.add(document.createElement('option'),1);
		o[1].id='middle'; o[1].value='M'; o[1].textContent='Middle';
		var added=[o.length,o[1].id,o[2].id].join(',');
		var extra=document.createElement('option'); extra.id='extra'; o.add(extra,1);
		var collectionMethods=[o.length,o[1].id].join(','); o.remove(1);
		var replacement=document.createElement('option'); replacement.id='replacement'; replacement.value='R'; o[0]=replacement;
		var replaced=[o.length,o[0].id,o[1].id].join(',');
		s.remove(1);
		var removed=[o.length,o[0].id,s.value].join(',');
		o.length=3;
		var grown=[o.length,o[2].tagName].join(',');
		o.length=1;
		o.remove();
		return [initial,liveSelected,added,collectionMethods,replaced,removed,grown,[o.length,so.length].join(',')].join('|');
	})()`)
	want := "true,true,true,true,2,a,b,b,b,1,a|2,b|3,middle,b|4,extra|3,replacement,middle|2,replacement,B|3,OPTION|0,0"
	if got != want {
		t.Fatalf("select collections = %q, want %q", got, want)
	}
}

func TestSelectIndexedSettersRunCustomElementReactions(t *testing.T) {
	doc := domtest.Parse(`<html><body></body></html>`)
	rt := newRuntime(t, js.Options{DOM: doc})
	got := evalString(t, rt, `(function(){
		var log=[];
		customElements.define('x-option-child', class extends HTMLElement {
			connectedCallback(){ log.push('connected'); }
			disconnectedCallback(){ log.push('disconnected'); }
		});
		var option=document.createElement('option');
		option.innerHTML='<x-option-child></x-option-child>';
		var select=document.createElement('select');
		document.body.appendChild(select);
		select[0]=option;
		var afterSelectSetter=log.join(',');
		select.options[0]=null;
		return afterSelectSetter+'|'+log.join(',')+'|'+select.length;
	})()`)
	if got != "connected|connected,disconnected|0" {
		t.Fatalf("indexed option setters reactions = %q", got)
	}
}

func TestOptionSelectedUpdatesSelectedContentCloneReactions(t *testing.T) {
	doc := domtest.Parse(`<html><body></body></html>`)
	rt := newRuntime(t, js.Options{DOM: doc})
	got := evalString(t, rt, `(function(){
		var log=[];
		customElements.define('x-option-child', class extends HTMLElement {
			constructor(){ super(); log.push('constructed'); }
			connectedCallback(){ log.push('connected'); }
		});
		document.body.innerHTML='<select><button><selectedcontent></selectedcontent></button><option>one</option><option><x-option-child>two</x-option-child></option></select>';
		var initial=log.join(',');
		document.querySelector('select').options[1].selected=true;
		return initial+'|'+log.join(',')+'|'+document.querySelector('selectedcontent').innerHTML;
	})()`)
	if got != "constructed,connected|constructed,connected,constructed,connected|<x-option-child>two</x-option-child>" {
		t.Fatalf("selectedcontent clone reactions = %q", got)
	}
}

func TestSelectMultipleAndSizeChangesRecomputeSelection(t *testing.T) {
	doc := domtest.Parse(`<form id="f"><select id="s" multiple><option id="a" selected>A</option><option id="b" selected>B</option><option id="c" selected>C</option></select><select id="sized" size="2"><option id="x">X</option><option id="y">Y</option></select></form>`)
	rt := newRuntime(t, js.Options{DOM: doc})
	got := evalString(t, rt, `(function(){
		var s=document.getElementById('s'), sized=document.getElementById('sized');
		var before=[s.type,s.multiple,s.length,s.selectedOptions.length].join(',');
		s.multiple=false;
		var after=[s.type,s.selectedOptions.length,[s.options[0].selected,s.options[1].selected,s.options[2].selected].join(','),[s.options[0].defaultSelected,s.options[1].defaultSelected,s.options[2].defaultSelected].join(',')].join('|');
		var sizeBefore=[sized.selectedIndex,sized.value].join(',');
		sized.size=1;
		return [before,after,sizeBefore,[sized.size,sized.selectedIndex,sized.value].join(',')].join('~');
	})()`)
	want := "select-multiple,true,3,3~select-one|1|true,false,false|true,true,true~-1,~1,0,X"
	if got != want {
		t.Fatalf("select multiple/size transitions = %q, want %q", got, want)
	}
}

func TestOptionFormIndexTextLabelAndDisabledIDL(t *testing.T) {
	doc := domtest.Parse(`<form id="f"><select><option id="o"> Alpha   Beta </option><option id="empty" label="">Visible text</option></select></form>`)
	rt := newRuntime(t, js.Options{DOM: doc})
	got := evalString(t, rt, `(function(){
		var o=document.getElementById('o');
		var before=[o.form.id,o.index,o.value,o.label,o.text,o.disabled].join(',');
		o.text='Gamma Delta';
		var empty=document.getElementById('empty');
		return [before,[o.text,o.value,o.label].join(','),empty.label].join('|');
	})()`)
	want := "f,0,Alpha Beta,Alpha Beta, Alpha   Beta ,false|Gamma Delta,Gamma Delta,Gamma Delta|"
	if got != want {
		t.Fatalf("option IDL properties = %q, want %q", got, want)
	}
}

// qc:BR-237
func TestStringAttributesReflectBothDirections(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"id default", `document.createElement('div').id`, ""},
		{"id attribute to property", `(function(){var e=document.createElement('div');e.setAttribute('id','x');return e.id})()`, "x"},
		{"id property to attribute", `(function(){var e=document.createElement('div');e.id='q2';return e.getAttribute('id')})()`, "q2"},
		{"id property resolves by id", `(function(){var e=document.createElement('div');e.id='q2';document.body.appendChild(e);return document.getElementById('q2')===e})()`, "true"},
		{"id null stores null", `(function(){var e=document.createElement('div');e.id=null;return e.getAttribute('id')+','+e.id})()`, "null,null"},
		{"id number stringifies", `(function(){var e=document.createElement('div');e.id=5;return e.getAttribute('id')})()`, "5"},
		{"id undefined stores undefined", `(function(){var e=document.createElement('div');e.id=undefined;return e.getAttribute('id')+','+e.id})()`, "undefined,undefined"},
		{"className default", `document.createElement('div').className`, ""},
		{"className property to class attribute", `(function(){var e=document.createElement('div');e.className='k';return e.getAttribute('class')})()`, "k"},
		{"class attribute to className", `(function(){var e=document.createElement('div');e.setAttribute('class','z');return e.className})()`, "z"},
		{"className null stores null", `(function(){var e=document.createElement('div');e.className=null;return e.getAttribute('class')})()`, "null"},
		{"className number stringifies", `(function(){var e=document.createElement('div');e.className=5;return e.getAttribute('class')})()`, "5"},
		{"title default", `document.createElement('div').title`, ""},
		{"title attribute to property", `(function(){var e=document.createElement('div');e.setAttribute('title','x');return e.title})()`, "x"},
		{"title property to attribute", `(function(){var e=document.createElement('div');e.title='t1';return e.getAttribute('title')})()`, "t1"},
		{"lang default", `document.createElement('div').lang`, ""},
		{"lang property to attribute", `(function(){var e=document.createElement('div');e.lang='vi';return e.getAttribute('lang')})()`, "vi"},
		{"lang attribute to property", `(function(){var e=document.createElement('div');e.setAttribute('lang','en');return e.lang})()`, "en"},
		{"slot default", `document.createElement('div').slot`, ""},
		{"slot property to attribute", `(function(){var e=document.createElement('div');e.slot='s1';return e.getAttribute('slot')})()`, "s1"},
		{"slot attribute to property", `(function(){var e=document.createElement('div');e.setAttribute('slot','s2');return e.slot})()`, "s2"},
		{"tagName uppercases", `document.getElementById('d').tagName`, "DIV"},
		{"nodeName uppercases for elements", `document.getElementById('d').nodeName`, "DIV"},
		{"created tagName uppercases", `document.createElement('span').tagName`, "SPAN"},
		{"dir default", `document.createElement('div').dir`, ""},
		{"dir known value lowercases", `(function(){var e=document.createElement('div');e.setAttribute('dir','LTR');return e.dir})()`, "ltr"},
		{"dir auto is known", `(function(){var e=document.createElement('div');e.setAttribute('dir','AUTO');return e.dir})()`, "auto"},
		{"dir unknown reads empty", `(function(){var e=document.createElement('div');e.setAttribute('dir','BoGuS');return e.dir})()`, ""},
		{"dir setter stores raw attribute", `(function(){var e=document.createElement('div');e.dir='BoGuS';return e.getAttribute('dir')+','+e.dir})()`, "BoGuS,"},
		{"dir empty reads empty", `(function(){var e=document.createElement('div');e.setAttribute('dir','');return e.dir})()`, ""},
	})
}

// qc:BR-238
func TestBooleanAttributesReflectPresence(t *testing.T) {
	rt := interfacesRuntime(t)
	cases := []struct{ tag, prop, attr string }{
		{"div", "hidden", "hidden"},
		{"img", "isMap", "ismap"},
		{"input", "disabled", "disabled"},
		{"input", "required", "required"},
		{"input", "multiple", "multiple"},
		{"input", "readOnly", "readonly"},
		{"input", "defaultChecked", "checked"},
		{"input", "autofocus", "autofocus"},
	}
	for _, c := range cases {
		tag, prop, attr := c.tag, c.prop, c.attr
		t.Run(prop, func(t *testing.T) {
			runJSCases(t, rt, []jsCase{
				{"default is false", `document.createElement('` + tag + `').` + prop, "false"},
				{"attribute false is still true", `(function(){var e=document.createElement('` + tag + `');e.setAttribute('` + attr + `','false');return e.` + prop + `})()`, "true"},
				{"true property stores empty attribute", `(function(){var e=document.createElement('` + tag + `');e.` + prop + `=true;return e.getAttribute('` + attr + `')})()`, ""},
				{"0 removes the attribute", `(function(){var e=document.createElement('` + tag + `');e.` + prop + `=true;e.` + prop + `=0;return String(e.getAttribute('` + attr + `'))})()`, "null"},
				{"'x' sets and reads true", `(function(){var e=document.createElement('` + tag + `');e.` + prop + `='x';return e.getAttribute('` + attr + `')+','+e.` + prop + `})()`, ",true"},
			})
		})
	}
}

// qc:BR-239
func TestUnsignedLongAttributesReflectWithTheirDefaults(t *testing.T) {
	rt := interfacesRuntime(t)
	type longCase struct{ tag, prop, attr, def string }
	for _, c := range []longCase{
		{"img", "width", "width", "0"},
		{"img", "height", "height", "0"},
		{"img", "hspace", "hspace", "0"},
		{"img", "vspace", "vspace", "0"},
		{"input", "maxLength", "maxlength", "-1"},
		{"input", "size", "size", "20"},
		{"div", "tabIndex", "tabindex", "-1"},
	} {
		c := c
		t.Run(c.prop, func(t *testing.T) {
			read := func(setup string) string {
				return `(function(){var e=document.createElement('` + c.tag + `');` + setup + `;return String(e.` + c.prop + `)})()`
			}
			runJSCases(t, rt, []jsCase{
				{"default", read(""), c.def},
				{"attribute 7", read(`e.setAttribute('` + c.attr + `','7')`), "7"},
				{"attribute 12.9 truncates", read(`e.setAttribute('` + c.attr + `','12.9')`), "12"},
				{"attribute 12px parses leading integer", read(`e.setAttribute('` + c.attr + `','12px')`), "12"},
				{"attribute spaces parse", read(`e.setAttribute('` + c.attr + `',' 13 ')`), "13"},
				{"empty attribute uses the default", read(`e.setAttribute('` + c.attr + `','')`), c.def},
			})
		})
	}
	runJSCases(t, rt, []jsCase{
		{"width negative attribute clamps", `(function(){var e=document.createElement('img');e.setAttribute('width','-5');return String(e.width)})()`, "0"},
		{"width setter stores decimal integer", `(function(){var e=document.createElement('img');e.width=42;return e.getAttribute('width')+','+e.width})()`, "42,42"},
		{"width negative setter stores 0", `(function(){var e=document.createElement('img');e.width=-3;return e.getAttribute('width')+','+e.width})()`, "0,0"},
		{"width setter truncates 4.9", `(function(){var e=document.createElement('img');e.width=4.9;return e.getAttribute('width')})()`, "4"},
		{"width non-numeric setter stores 0", `(function(){var e=document.createElement('img');e.width='8px';return e.getAttribute('width')+','+e.width})()`, "0,0"},
		{"maxLength negative attribute reads -1", `(function(){var e=document.createElement('input');e.setAttribute('maxlength','-5');return String(e.maxLength)})()`, "-1"},
		{"maxLength negative setter throws IndexSizeError", `(function(){var e=document.createElement('input');try{e.maxLength=-3;return 'no'}catch(x){return x.name}})()`, "IndexSizeError"},
		{"maxLength 4.9 truncates", `(function(){var e=document.createElement('input');e.maxLength=4.9;return e.getAttribute('maxlength')})()`, "4"},
		{"size negative attribute reads default", `(function(){var e=document.createElement('input');e.setAttribute('size','-5');return String(e.size)})()`, "20"},
		{"size negative setter resets to 20", `(function(){var e=document.createElement('input');e.size=-3;return e.getAttribute('size')+','+e.size})()`, "20,20"},
		{"size zero setter throws IndexSizeError", `(function(){var e=document.createElement('input');try{e.size=0;return 'no'}catch(x){return x.name}})()`, "IndexSizeError"},
		{"size 8px setter throws IndexSizeError", `(function(){var e=document.createElement('input');try{e.size='8px';return 'no'}catch(x){return x.name}})()`, "IndexSizeError"},
		{"tabIndex allows negatives", `(function(){var e=document.createElement('div');e.tabIndex=-3;return e.getAttribute('tabindex')+','+e.tabIndex})()`, "-3,-3"},
	})
}

// qc:BR-240
func TestEnumeratedAttributesReflectOnlyKnownValues(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"loading default", `document.createElement('img').loading`, "auto"},
		{"loading known lowercases", `(function(){var e=document.createElement('img');e.setAttribute('loading','LAZY');return e.loading})()`, "lazy"},
		{"loading unknown reads auto", `(function(){var e=document.createElement('img');e.setAttribute('loading','bogus');return e.loading})()`, "auto"},
		{"loading empty reads auto", `(function(){var e=document.createElement('img');e.setAttribute('loading','');return e.loading})()`, "auto"},
		{"loading setter stores raw", `(function(){var e=document.createElement('img');e.loading='LAZY';return e.getAttribute('loading')+','+e.loading})()`, "LAZY,lazy"},
		{"loading null setter stores null", `(function(){var e=document.createElement('img');e.loading=null;return e.getAttribute('loading')+','+e.loading})()`, "null,auto"},
		{"decoding default", `document.createElement('img').decoding`, "auto"},
		{"decoding known lowercases", `(function(){var e=document.createElement('img');e.setAttribute('decoding','SYNC');return e.decoding})()`, "sync"},
		{"decoding unknown reads auto", `(function(){var e=document.createElement('img');e.setAttribute('decoding','bogus');return e.decoding})()`, "auto"},
		{"decoding setter stores raw", `(function(){var e=document.createElement('img');e.decoding='SYNC';return e.getAttribute('decoding')+','+e.decoding})()`, "SYNC,sync"},
		{"crossOrigin default is null", `String(document.createElement('img').crossOrigin)`, "null"},
		{"crossOrigin anonymous", `(function(){var e=document.createElement('img');e.setAttribute('crossorigin','ANONYMOUS');return e.crossOrigin})()`, "anonymous"},
		{"crossOrigin use-credentials", `(function(){var e=document.createElement('img');e.setAttribute('crossorigin','use-credentials');return e.crossOrigin})()`, "use-credentials"},
		{"crossOrigin unknown reads anonymous", `(function(){var e=document.createElement('img');e.setAttribute('crossorigin','bogus');return e.crossOrigin})()`, "anonymous"},
		{"crossOrigin null setter removes the attribute", `(function(){var e=document.createElement('img');e.setAttribute('crossorigin','anonymous');e.crossOrigin=null;return String(e.getAttribute('crossorigin'))+','+String(e.crossOrigin)})()`, "null,null"},
		{"crossOrigin setter stores raw", `(function(){var e=document.createElement('img');e.crossOrigin='BOGUS2';return e.getAttribute('crossorigin')+','+e.crossOrigin})()`, "BOGUS2,anonymous"},
		{"referrerPolicy default", `document.createElement('img').referrerPolicy`, ""},
		{"referrerPolicy known", `(function(){var e=document.createElement('img');e.setAttribute('referrerpolicy','origin');return e.referrerPolicy})()`, "origin"},
		{"referrerPolicy unknown reads empty", `(function(){var e=document.createElement('img');e.setAttribute('referrerpolicy','BOGUS');return e.referrerPolicy})()`, ""},
		{"referrerPolicy setter stores raw", `(function(){var e=document.createElement('a');e.referrerPolicy='no-referrer';return e.getAttribute('referrerpolicy')+','+e.referrerPolicy})()`, "no-referrer,no-referrer"},
		{"type default", `document.createElement('input').type`, "text"},
		{"type known lowercases", `(function(){var e=document.createElement('input');e.setAttribute('type','CHECKBOX');return e.type})()`, "checkbox"},
		{"type keyword matching is ASCII case-insensitive", `(function(){var e=document.createElement('input');e.setAttribute('type','hİdden');return e.type})()`, "text"},
		{"type unknown reads text", `(function(){var e=document.createElement('input');e.setAttribute('type','bogus');return e.type})()`, "text"},
		{"type setter stores raw", `(function(){var e=document.createElement('input');e.type='CHECKBOX';return e.getAttribute('type')+','+e.type})()`, "CHECKBOX,checkbox"},
	})
}

// qc:BR-241
func TestClassListAttributesAndStyleAreLive(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"classList identity", `(function(){var e=document.createElement('div');return e.classList===e.classList})()`, "true"},
		{"classList after setAttribute", `(function(){var e=document.createElement('div');e.setAttribute('class','a b');return Array.prototype.join.call(e.classList,'+')})()`, "a+b"},
		{"classList length", `(function(){var e=document.createElement('div');e.setAttribute('class','a b');return String(e.classList.length)})()`, "2"},
		{"classList add reaches the attribute", `(function(){var e=document.createElement('div');e.classList.add('a');return e.getAttribute('class')})()`, "a"},
		{"classList remove reaches the attribute", `(function(){var e=document.createElement('div');e.setAttribute('class','a b');e.classList.remove('a');return e.className})()`, "b"},
		{"classList contains", `(function(){var e=document.createElement('div');e.setAttribute('class','a b');return e.classList.contains('b')})()`, "true"},
		{"classList toggle", `(function(){var e=document.createElement('div');e.classList.toggle('a');return e.className})()`, "a"},
		{"classList item", `(function(){var e=document.createElement('div');e.setAttribute('class','a b');return e.classList.item(1)})()`, "b"},
		{"classList value empty", `(function(){var e=document.createElement('div');return e.classList.value})()`, ""},
		{"classList add empty throws", `(function(){var e=document.createElement('div');try{e.classList.add('');return 'no'}catch(x){return x.name}})()`, "InvalidCharacterError"},
		{"className spaces store raw and classList parses", `(function(){var e=document.createElement('div');e.className='  c   d  ';return e.getAttribute('class')+'|'+Array.prototype.join.call(e.classList,',')})()`, "  c   d  |c,d"},
		{"attributes identity", `(function(){var e=document.createElement('div');return e.attributes===e.attributes})()`, "true"},
		{"attributes length", `(function(){var e=document.createElement('div');e.setAttribute('id','x');e.setAttribute('data-k','v');return String(e.attributes.length)})()`, "2"},
		{"attributes item", `(function(){var e=document.createElement('div');e.setAttribute('id','x');e.setAttribute('data-k','v');return [e.attributes.item(0).name,e.attributes.item(0).value].join(',')})()`, "id,x"},
		{"attributes numeric index", `(function(){var e=document.createElement('div');e.setAttribute('id','x');e.setAttribute('data-k','v');return e.attributes[0].name})()`, "id"},
		{"attributes getNamedItem", `(function(){var e=document.createElement('div');e.setAttribute('id','x');return [e.attributes.getNamedItem('id').name,e.attributes.getNamedItem('id').value].join(',')})()`, "id,x"},
		{"attributes keys are numeric strings", `(function(){var e=document.createElement('div');e.setAttribute('id','x');e.setAttribute('data-k','v');return Object.keys(e.attributes).join(',')})()`, "0,1"},
		{"attributes own names include readonly names", `(function(){var e=document.createElement('div');e.setAttribute('id','x');e.setAttribute('data-k','v');var d=Object.getOwnPropertyDescriptor(e.attributes,'id');return Object.getOwnPropertyNames(e.attributes).join(',')+'|'+d.enumerable+'|'+e.attributes.id.value})()`, "0,1,id,data-k|false|x"},
		{"attributes length follows writes", `(function(){var e=document.createElement('div');var n=e.attributes.length;e.setAttribute('id','x');return String(n)+','+e.attributes.length})()`, "0,1"},
		{"style identity", `(function(){var e=document.createElement('div');return e.style===e.style})()`, "true"},
		{"style setter canonicalises the attribute", `(function(){var e=document.createElement('div');e.style.color='red';return e.getAttribute('style')})()`, "color: red;"},
		{"style reads from the attribute", `(function(){var e=document.createElement('div');e.setAttribute('style','color: blue');return e.style.color})()`, "blue"},
		{"style cssText canonicalises an existing attribute", `(function(){var e=document.createElement('div');e.setAttribute('style','color: red');return e.style.cssText})()`, "color: red;"},
		{"invalid style declaration is ignored but attribute is preserved", `(function(){var e=document.createElement('div');e.setAttribute('style','color:: invalid');return e.style.cssText+'|'+e.style.color+'|'+e.getAttribute('style')})()`, "||color:: invalid"},
	})
}

// qc:BR-242
func TestAnchorURLReflectionResolvesAgainstTheDocumentBase(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"root relative href", `document.getElementById('a').href`, "https://example.test/p?q=1"},
		{"hostname", `document.getElementById('a').hostname`, "example.test"},
		{"anchor username getter", `(function(){var e=document.createElement('a');e.href='https://anonymous:pwd@example.test/path';return e.username})()`, "anonymous"},
		{"anchor password getter", `(function(){var e=document.createElement('a');e.href='https://anonymous:pwd@example.test/path';return e.password})()`, "pwd"},
		{"anchor credential getters preserve URL percent encoding", `(function(){var e=document.createElement('a');e.href='https://a%20b:p%40ss@example.test/path';return e.username+'|'+e.password})()`, "a%20b|p%40ss"},
		{"anchor username setter preserves password", `(function(){var e=document.createElement('a');e.href='https://anonymous:pwd@example.test/path';e.username='wellknown';return e.href})()`, "https://wellknown:pwd@example.test/path"},
		{"anchor password setter preserves username", `(function(){var e=document.createElement('a');e.href='https://anonymous:pwd@example.test/path';e.password='blubb';return e.href})()`, "https://anonymous:blubb@example.test/path"},
		{"area shares hyperlink URL credentials", `(function(){var e=document.createElement('area');e.href='https://anonymous:pwd@example.test/path';var before=e.username+'|'+e.password;e.username='wellknown';return before+'|'+e.href})()`, "anonymous|pwd|https://wellknown:pwd@example.test/path"},
		{"target default", `document.getElementById('a').target`, ""},
		{"rel default", `document.getElementById('a').rel`, ""},
		{"baseURI", `document.baseURI`, "https://example.test/docs/page.html"},
		{"dot relative", `(function(){var e=document.createElement('a');e.setAttribute('href','./x');return e.href})()`, "https://example.test/docs/x"},
		{"parent relative", `(function(){var e=document.createElement('a');e.setAttribute('href','../up/y');return e.href})()`, "https://example.test/up/y"},
		{"fragment", `(function(){var e=document.createElement('a');e.setAttribute('href','#f');return e.href+','+e.hash})()`, "https://example.test/docs/page.html#f,#f"},
		{"query only", `(function(){var e=document.createElement('a');e.setAttribute('href','?z=2');return e.href+','+e.search})()`, "https://example.test/docs/page.html?z=2,?z=2"},
		{"query Unicode and spaces are percent-encoded", `(function(){var e=document.createElement('a');e.setAttribute('href','?q=∉¬ &z=1');return e.href})()`, "https://example.test/docs/page.html?q=%E2%88%89%C2%AC%20&z=1"},
		{"Unicode path is percent-encoded", `(function(){var e=document.createElement('a');e.setAttribute('href','/\u1680javascript:alert(1)');return e.href})()`, "https://example.test/%E1%9A%80javascript:alert(1)"},
		{"Unicode path beginning with whitespace and colon is relative", `(function(){var e=document.createElement('a');e.setAttribute('href','\u1680javascript:alert(1)');return e.href})()`, "https://example.test/docs/%E1%9A%80javascript:alert(1)"},
		{"data URL stays opaque", `(function(){var e=document.createElement('a');e.setAttribute('href','data:text/plain,hi');return [e.href,e.protocol,e.pathname,e.origin].join('|')})()`, "data:text/plain,hi|data:|text/plain,hi|null"},
		// probe3.out: href keeps the raw text, protocol is the lone ":" that
		// survived parsing, and hostname/pathname/origin are empty.
		{"unparseable keeps the raw href", `(function(){var e=document.createElement('a');e.setAttribute('href','http://[::1');return [e.href,e.protocol,e.hostname,e.pathname,e.origin].join('|')})()`, "http://[::1|:|||"},
		{"scheme relative inherits https", `(function(){var e=document.createElement('a');e.setAttribute('href','//other.test/y');return e.href})()`, "https://other.test/y"},
		{"mailto is opaque", `(function(){var e=document.createElement('a');e.setAttribute('href','mailto:a@b');return [e.href,e.pathname,e.origin].join('|')})()`, "mailto:a@b|a@b|null"},
		{"javascript is opaque", `(function(){var e=document.createElement('a');e.setAttribute('href','javascript:void 0');return e.href+','+e.pathname})()`, "javascript:void 0,void 0"},
		{"scheme and host lowercase", `(function(){var e=document.createElement('a');e.setAttribute('href','HTTPS://UP.TEST/A');return e.href})()`, "https://up.test/A"},
		{"empty href resolves to the document URL", `(function(){var e=document.createElement('a');e.setAttribute('href','');return e.href})()`, "https://example.test/docs/page.html"},
		{"spaces-only href resolves to the document URL", `(function(){var e=document.createElement('a');e.setAttribute('href','   ');return e.href})()`, "https://example.test/docs/page.html"},
		{"space is percent-encoded", `(function(){var e=document.createElement('a');e.setAttribute('href','a b');return e.href+','+e.pathname})()`, "https://example.test/docs/a%20b,/docs/a%20b"},
		{"missing href reads empty", `(function(){var e=document.createElement('a');return [e.href,e.hostname,e.origin,e.toString()].join('|')})()`, "|||"},
		{"property write stores raw and reads absolute", `(function(){var e=document.createElement('a');e.href='./rel';return e.getAttribute('href')+'|'+e.href})()`, "./rel|https://example.test/docs/rel"},
		{"null property write stores null", `(function(){var e=document.createElement('a');e.href=null;return e.getAttribute('href')+'|'+e.href})()`, "null|https://example.test/docs/null"},
		{"toString is href", `(function(){var e=document.createElement('a');e.setAttribute('href','https://ex.test/x');return e.toString()})()`, "https://ex.test/x"},
		{"URL part setters rewrite href in order", `(function(){
			var e = document.createElement('a');
			e.href = 'https://ex.test/a?b=c#d';
			var out = [];
			e.hash = '#z'; out.push(e.href);
			e.search = '?q=9'; out.push(e.href);
			e.pathname = '/n'; out.push(e.href);
			e.hostname = 'o.test'; out.push(e.href);
			e.protocol = 'http:'; out.push(e.href);
			e.port = '81'; out.push(e.href);
			return out.join('|');
		})()`, "https://ex.test/a?b=c#z|https://ex.test/a?q=9#z|https://ex.test/n?q=9#z|https://o.test/n?q=9#z|http://o.test/n?q=9#z|http://o.test:81/n?q=9#z"},
		{"text getter", `document.getElementById('a').text`, "link"},
		{"text setter replaces children", `(function(){var e=document.createElement('a');e.text='T';return e.childNodes.length+','+e.textContent})()`, "1,T"},
	})
}

func TestAnchorURLQueryUsesOwningDocumentEncoding(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body><a id="a"></a></body></html>`)
	doc.CharacterSet = "windows-1252"
	rt := newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
	runJSCases(t, rt, []jsCase{
		{"legacy encoded query in href", `(function(){var a=document.getElementById('a');a.href='?ß';return a.href})()`, "https://example.test/page?%DF"},
		{"legacy encoded query in search", `(function(){var a=document.getElementById('a');a.href='?ß';return a.search})()`, "?%DF"},
		{"path remains UTF-8 encoded", `(function(){var a=document.getElementById('a');a.href='/ß';return a.href})()`, "https://example.test/%C3%9F"},
		{"search setter uses document encoding", `(function(){var a=document.createElement('a');a.search='?ß';return a.href})()`, "https://example.test/page?%DF"},
	})
}

// qc:BR-243
func TestInputValueAndDefaultValueUseSeparateLiveAndAttributeState(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"initial live value and default", `(function(){var e=document.getElementById('in');return [e.value,e.defaultValue,e.getAttribute('value')].join('|')})()`, "v0|v0|v0"},
		{"setAttribute updates clean live value", `(function(){var e=document.getElementById('in');e.setAttribute('value','aa');var v=[e.value,e.defaultValue].join('|');e.setAttribute('value','v0');return v+'|'+e.value})()`, "aa|aa|v0"},
		{"clean URL defaults are sanitized on first value read", `(function(){var e=document.createElement('input');e.type='url';e.setAttribute('value',' \taa\r\n ');return e.value+'|'+e.defaultValue})()`, "aa| \taa\r\n "},
		{"value write changes only live value", `(function(){var e=document.getElementById('in');e.value='zz';return [e.value,e.defaultValue,e.getAttribute('value')].join('|')})()`, "zz|v0|v0"},
		{"input value null uses legacy empty-string conversion", `(function(){var e=document.createElement('input');e.value='x';e.value=null;return e.value})()`, ""},
		{"file input value null clears without exposing a filename", `(function(){var e=document.createElement('input');e.type='file';e.value=null;return e.value})()`, ""},
		{"defaultValue write preserves dirty live value", `(function(){var e=document.getElementById('in');e.value='zz';e.defaultValue='dd';return [e.value,e.defaultValue,e.getAttribute('value')].join('|')})()`, "zz|dd|dd"},
		{"setAttribute does not replace dirty live value", `(function(){var e=document.getElementById('in');e.value='zz';e.defaultValue='dd';e.setAttribute('value','aa');return [e.value,e.defaultValue,e.getAttribute('value')].join('|')})()`, "zz|aa|aa"},
		{"defaultValue null stores null", `(function(){var e=document.createElement('input');e.setAttribute('value','v');e.defaultValue=null;return e.getAttribute('value')+'|'+e.defaultValue})()`, "null|null"},
		{"no attribute reads empty", `(function(){var e=document.createElement('input');return [e.value,e.defaultValue,e.type].join('|')})()`, "||text"},
		{"list reflects its target datalist or null", `(function(){var d=document.createElement('datalist'),e=document.createElement('input');d.id='suggestions';e.setAttribute('list','suggestions');document.body.appendChild(d);document.body.appendChild(e);var found=e.list===d;e.setAttribute('list','missing');return [found,e.list===null].join('|')})()`, "true|true"},
		{"leaving value mode copies the nonempty live value to the default", `(function(){var e=document.createElement('input');e.defaultValue='original';e.value='live';e.setAttribute('type','checkbox');return [e.value,e.defaultValue,e.getAttribute('value')].join('|')})()`, "live|live|live"},
		{"setAttributeNode uses the same value-mode transition", `(function(){var e=document.createElement('input');e.defaultValue='original';e.value='live';var a=document.createAttribute('type');a.value='checkbox';e.setAttributeNode(a);return [e.value,e.defaultValue].join('|')})()`, "live|live"},
		{"leaving value mode with an empty value preserves the default", `(function(){var e=document.createElement('input');e.defaultValue='original';e.value='';e.type='radio';return [e.value,e.defaultValue].join('|')})()`, "original|original"},
		{"reentering value mode restores the attribute and clears dirtiness", `(function(){var e=document.createElement('input');e.defaultValue='original';e.value='live';e.type='checkbox';e.defaultValue='new default';e.removeAttribute('type');e.setAttribute('value','after');return [e.value,e.defaultValue].join('|')})()`, "after|after"},
		{"invalid date value is sanitized without changing default", `(function(){var e=document.createElement('input');e.type='date';e.defaultValue='2024-02-29';e.value='2024-02-30';return [e.value,e.defaultValue,e.getAttribute('value')].join('|')})()`, "|2024-02-29|2024-02-29"},
		{"valueAsNumber reads and writes number values", `(function(){var e=document.createElement('input');e.type='number';e.valueAsNumber=12.5;return [e.value,e.valueAsNumber].join('|')})()`, "12.5|12.5"},
		{"valueAsNumber converts date milliseconds", `(function(){var e=document.createElement('input');e.type='date';e.valueAsNumber=0;return [e.value,e.valueAsNumber].join('|')})()`, "1970-01-01|0"},
		{"valueAsDate reads and writes UTC dates", `(function(){var e=document.createElement('input');e.type='date';e.valueAsDate=new Date(0);var first=e.value+'|'+e.valueAsDate.getTime();e.valueAsDate=null;return first+'|'+e.value+'|'+String(e.valueAsDate)})()`, "1970-01-01|0||null"},
		{"email value converts an IDN domain to ASCII", `(function(){var e=document.createElement('input');e.type='email';e.value='test@exämle.com';return e.value})()`, "test@xn--exmle-hra.com"},
		{"multiple email values trim and convert each IDN domain", `(function(){var e=document.createElement('input');e.type='email';e.multiple=true;e.value=' test@exämle.com , user@お.com ';return e.value})()`, "test@xn--exmle-hra.com,user@xn--t8j.com"},
		{"stepUp honors decimal step and stepDown reverses it", `(function(){var e=document.createElement('input');e.type='number';e.value='6.7';e.step='.3';e.stepUp();var up=e.value;e.stepDown();return up+'|'+e.value})()`, "6.9|6.6"},
		{"stepDown does not cross a positive minimum from an empty value", `(function(){var e=document.createElement('input');e.type='number';e.min='7';e.stepDown();return e.value})()`, "7"},
		{"valid number value preserves its text without changing default", `(function(){var e=document.createElement('input');e.type='number';e.defaultValue='2';e.value='1e2';return [e.value,e.defaultValue,e.getAttribute('value')].join('|')})()`, "1e2|2|2"},
		{"range invalid value defaults to midpoint", `(function(){var e=document.createElement('input');e.type='range';e.value='invalid';return e.value})()`, "50"},
		{"range step mismatch rounds toward positive infinity", `(function(){var e=document.createElement('input');e.type='range';e.min='0';e.max='100';e.step='20';e.value='50';return e.value})()`, "60"},
		{"range clamps and rounds within bounds", `(function(){var e=document.createElement('input');e.type='range';e.min='-10';e.max='10';e.step='3';e.value='100';return e.value})()`, "8"},
		{"color value uses opaque lowercase sRGB hex", `(function(){var e=document.createElement('input');e.type='color';e.value='#A0b1C2';return e.value})()`, "#a0b1c2"},
		{"color value parses CSS named colors", `(function(){var e=document.createElement('input');e.type='color';e.value='crimson';return e.value})()`, "#dc143c"},
		{"color value parses CSS rgb and hsl functions", `(function(){var a=document.createElement('input'),b=document.createElement('input');a.type=b.type='color';a.value='rgb(1,1,1,1)';b.value='hsl(150deg 100 53.5)';return a.value+'|'+b.value})()`, "#010101|#12ff88"},
		{"color value converts Display-P3 to sRGB", `(function(){var e=document.createElement('input');e.type='color';e.value='color(display-p3 .5 0 0)';return e.value})()`, "#8c0000"},
		{"color value resolves a CSS system color", `(function(){var e=document.createElement('input');e.type='color';e.value='ActiveBorder';return e.value})()`, "#767676"},
		{"color value rejects non-opaque colors", `(function(){var e=document.createElement('input');e.type='color';e.value='rgb(1 1 1 / 50%)';return e.value})()`, "#000000"},
		{"short hex color expands to opaque lowercase RGB", `(function(){var e=document.createElement('input');e.type='color';e.value='#abc';return e.value})()`, "#aabbcc"},
		{"text input strips line breaks from live value", `(function(){var e=document.createElement('input');e.defaultValue='x';e.value='a\r\nb';return [e.value,e.defaultValue].join('|')})()`, "ab|x"},
		{"checkbox without a value reads on", `(function(){var e=document.createElement('input');e.type='checkbox';return [e.type,e.getAttribute('type'),e.value].join('|')})()`, "checkbox|checkbox|on"},
		{"checkbox with a value reads it", `(function(){var e=document.createElement('input');e.type='checkbox';e.value='yes';return e.value})()`, "yes"},
		{"checked is dirty live state separate from defaultChecked", `(function(){var e=document.createElement('input');e.type='checkbox';e.defaultChecked=true;e.checked=false;var first=[e.checked,e.defaultChecked,e.hasAttribute('checked')].join('|');e.setAttribute('checked','');return first+'/'+[e.checked,e.defaultChecked].join('|')})()`, "false|true|true/false|true"},
		{"radio checked setter updates only current group checkedness", `(function(){var f=document.createElement('form'),a=document.createElement('input'),b=document.createElement('input');a.type=b.type='radio';a.name=b.name='group';a.defaultChecked=true;f.appendChild(a);f.appendChild(b);document.body.appendChild(f);b.checked=true;return [a.checked,b.checked,a.defaultChecked,b.defaultChecked,a.hasAttribute('checked'),b.hasAttribute('checked')].join('|')})()`, "false|true|true|false|true|false"},
		{"reportValidity accepts a required radio group with one selected member", `(function(){var f=document.createElement('form'),a=document.createElement('input'),b=document.createElement('input');a.type=b.type='radio';a.name=b.name='group';a.required=b.required=true;a.checked=true;f.appendChild(a);f.appendChild(b);document.body.appendChild(f);return [a.validity.valueMissing,b.validity.valueMissing,f.reportValidity()].join('|')})()`, "false|false|true"},
		{"detached radio checked setter enforces its tree group", `(function(){var f=document.createElement('form'),a=document.createElement('input'),b=document.createElement('input');a.type=b.type='radio';a.name=b.name='group';f.appendChild(a);f.appendChild(b);a.checked=true;b.checked=true;return [a.checked,b.checked].join('|')})()`, "false|true"},
		{"detached non-form radio insertion preserves checkedness", `(function(){var c=document.createElement('div'),a=document.createElement('input'),b=document.createElement('input');a.type=b.type='radio';a.name=b.name='group';a.checked=b.checked=true;c.appendChild(a);c.appendChild(b);return [a.checked,b.checked].join('|')})()`, "true|true"},
		{"detached form innerHTML radio insertion normalizes checkedness", `(function(){var f=document.createElement('form');f.innerHTML='<input type="radio" name="group" checked><input type="radio" name="group" checked>';return [f.children[0].checked,f.children[1].checked].join('|')})()`, "false|true"},
		{"detached form innerHTML resets prior radio checkedness state", `(function(){var f=document.createElement('form'),a=document.createElement('input'),b=document.createElement('input');a.type=b.type='radio';a.name=b.name='group';a.checked=b.checked=true;f.appendChild(a);f.appendChild(b);f.innerHTML='<input type="radio" name="group" checked><input type="radio" name="group" checked>';return [f.children[0].checked,f.children[1].checked].join('|')})()`, "false|true"},
		{"automatically unchecked radio keeps dirty checkedness", `(function(){var f=document.createElement('form'),a=document.createElement('input'),b=document.createElement('input');a.type=b.type='radio';a.name=b.name='group';a.defaultChecked=b.defaultChecked=true;f.appendChild(a);f.appendChild(b);document.body.appendChild(f);var initial=[a.checked,b.checked].join('|');a.removeAttribute('checked');a.setAttribute('checked','');return initial+'/'+[a.checked,b.checked].join('|')})()`, "false|true/false|true"},
		{"form reset preserves radio group exclusivity", `(function(){var f=document.createElement('form'),a=document.createElement('input'),b=document.createElement('input');a.type=b.type='radio';a.name=b.name='group';a.defaultChecked=b.defaultChecked=true;f.appendChild(a);f.appendChild(b);document.body.appendChild(f);f.reset();return [a.checked,b.checked].join('|')})()`, "false|true"},
		{"changing a checked radio name joins its new group", `(function(){var f=document.createElement('form'),a=document.createElement('input'),b=document.createElement('input');a.type=b.type='radio';a.name='a';b.name='b';a.checked=b.checked=true;f.appendChild(a);f.appendChild(b);document.body.appendChild(f);b.name='a';return [a.checked,b.checked].join('|')})()`, "false|true"},
		{"changing a checked checkbox type to radio joins the group", `(function(){var f=document.createElement('form'),a=document.createElement('input'),b=document.createElement('input');a.type='checkbox';a.name='g';a.checked=true;b.type='radio';b.name='g';b.checked=true;f.appendChild(a);f.appendChild(b);document.body.appendChild(f);a.type='radio';return [a.checked,b.checked].join('|')})()`, "false|true"},
		{"changing a checked radio form owner joins the new group", `(function(){var f1=document.createElement('form'),f2=document.createElement('form'),a=document.createElement('input'),b=document.createElement('input');f1.id='f1';f2.id='f2';a.type=b.type='radio';a.name=b.name='g';a.checked=b.checked=true;f1.appendChild(a);f2.appendChild(b);document.body.appendChild(f1);document.body.appendChild(f2);b.setAttribute('form','f1');return [a.checked,b.checked].join('|')})()`, "false|true"},
		{"connecting a checked detached radio joins the connected group", `(function(){var f=document.createElement('form'),a=document.createElement('input'),b=document.createElement('input');a.type=b.type='radio';a.name=b.name='g';a.checked=true;b.checked=true;f.appendChild(a);document.body.appendChild(f);f.appendChild(b);return [a.checked,b.checked].join('|')})()`, "false|true"},
		{"moving a radio updates its ancestor form owner", `(function(){var f1=document.createElement('form'),f2=document.createElement('form'),a=document.createElement('input');f1.id='f1';f2.id='f2';a.type='radio';a.name='g';f1.appendChild(a);document.body.appendChild(f1);document.body.appendChild(f2);f2.appendChild(a);return a.form===f2})()`, "true"},
		{"changing a shadow-tree radio name joins its shadow group", `(function(){var host=document.createElement('div'),root=host.attachShadow({mode:'open'}),a=document.createElement('input'),b=document.createElement('input');a.type=b.type='radio';a.name='a';b.name='b';a.checked=b.checked=true;root.appendChild(a);root.appendChild(b);document.body.appendChild(host);b.name='a';return [a.checked,b.checked].join('|')})()`, "false|true"},
		{"changing form id resolves an explicit radio owner", `(function(){var f=document.createElement('form'),a=document.createElement('input'),b=document.createElement('input');f.id='old';a.type=b.type='radio';a.name=b.name='g';b.setAttribute('form','new');a.checked=b.checked=true;f.appendChild(a);document.body.appendChild(f);document.body.appendChild(b);f.id='new';return [a.checked,b.checked,a.form&&a.form.id,b.form&&b.form.id].join('|')})()`, "false|true|new|new"},
		{"input clone copies live value and checkedness dirtiness", `(function(){var e=document.createElement('input');e.type='checkbox';e.defaultValue='default';e.defaultChecked=true;e.checked=false;var c=e.cloneNode();e.removeAttribute('checked');var v=document.createElement('input');v.defaultValue='default';v.value='live';var cv=v.cloneNode();return [c.value,c.checked,c.defaultValue,c.defaultChecked,e.checked,cv.value,cv.defaultValue].join('|')})()`, "default|false|default|true|false|live|default"},
		{"defaultChecked writes the attribute", `(function(){var e=document.createElement('input');e.type='checkbox';e.defaultChecked=true;return [e.checked,e.defaultChecked,e.getAttribute('checked')].join('|')})()`, "true|true|"},
	})
}

func TestMovingParserOwnedRadioRecomputesItsFormOwner(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><form id="one"><input id="moved" type="radio" name="g" checked></form><form id="two"><input id="stays" type="radio" name="g" checked></form>`)
	rt := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	runJSCases(t, rt, []jsCase{
		{"moving a parser-owned radio updates form owner and group", `(function(){var a=document.getElementById('moved'),b=document.getElementById('stays'),f=document.getElementById('two');f.appendChild(a);return [a.form===f,a.checked,b.checked].join('|')})()`, "true|true|false"},
	})
}

func TestParsedRequiredRadioGroupReportValidity(t *testing.T) {
	doc := domtest.Parse(`<form id="testForm"><input type="radio" name="foo" required checked><input type="radio" name="foo" required><input type="submit"></form>`)
	rt := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	runJSCases(t, rt, []jsCase{
		{"parsed selected radio satisfies group validity", `(function(){var f=document.getElementById('testForm'),a=f.elements[0],b=f.elements[1];return [f.elements.length,a.checked,b.checked,a.validity.valueMissing,b.validity.valueMissing,f.reportValidity()].join('|')})()`, "3|true|false|false|false|true"},
	})
}

// qc:BR-244
func TestImageConstructorCreatesADetachedHTMLImageElement(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"Image is published", `typeof Image`, "function"},
		{"Image.length is zero", `String(Image.length)`, "0"},
		{"Image shares the interface prototype", `Image.prototype === HTMLImageElement.prototype`, "true"},
		{"Image.prototype.constructor", `Image.prototype.constructor === HTMLImageElement`, "true"},
		{"Image inherits from Function.prototype", `Object.getPrototypeOf(Image) === Function.prototype`, "true"},
		{"HTMLImageElement is an illegal constructor", `(function(){try{new HTMLImageElement();return 'no'}catch(e){return e instanceof TypeError}})()`, "true"},
		{"new Image is an HTMLImageElement", `new Image() instanceof HTMLImageElement`, "true"},
		{"new Image is detached", `String(new Image().parentNode)`, "null"},
		{"new Image belongs to the document", `new Image().ownerDocument === document`, "true"},
		{"new Image tagName", `new Image().tagName`, "IMG"},
		{"new Image has no width attribute", `[new Image().hasAttribute('width'),new Image().hasAttribute('height'),new Image().width,new Image().height].join('|')`, "false|false|0|0"},
		{"new Image(30,40) writes both attributes", `(function(){var i=new Image(30,40);return [i.getAttribute('width'),i.getAttribute('height'),i.width,i.height].join('|')})()`, "30|40|30|40"},
		{"new Image(30) only writes width", `(function(){var i=new Image(30);return [i.width,i.height,String(i.getAttribute('height'))].join('|')})()`, "30|0|null"},
		{"new Image truncates and clamps", `(function(){var i=new Image(30.7,-2);return i.getAttribute('width')+'|'+i.getAttribute('height')})()`, "30|0"},
		{"new Image(null,null) stores zero", `(function(){var i=new Image(null,null);return i.getAttribute('width')+'|'+i.getAttribute('height')})()`, "0|0"},
		{"new Image is complete before a source", `new Image().complete`, "true"},
		{"img width attribute reflects", `(function(){var i=document.createElement('img');i.setAttribute('width','10');return String(i.width)})()`, "10"},
		{"img width property writes the attribute", `(function(){var i=document.createElement('img');i.width=25;return i.getAttribute('width')})()`, "25"},
		{"img width non-numeric write stores 0", `(function(){var i=document.createElement('img');i.width='30px';return String(i.width)+','+i.getAttribute('width')})()`, "0,0"},
		{"img width negative write stores 0", `(function(){var i=document.createElement('img');i.width=-5;return String(i.width)+','+i.getAttribute('width')})()`, "0,0"},
		{"img width invalid attribute reads 0", `(function(){var i=document.createElement('img');i.setAttribute('width','abc');return String(i.width)})()`, "0"},
	})
}

func TestTextControlSelectionAndRangeReplacement(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"document Selection stringifies selected text-control range", `(function(){var e=document.createElement('textarea');e.value='abcdef';document.body.appendChild(e);e.setSelectionRange(1,4);return getSelection().toString()})()`, "bcd"},
		{"input selection uses UTF-16 offsets and direction", `(function(){var e=document.createElement('input');e.value='A😀BC';e.setSelectionRange(1,3,'backward');return [e.selectionStart,e.selectionEnd,e.selectionDirection,e.value.slice(e.selectionStart,e.selectionEnd)].join('|')})()`, "1|3|backward|😀"},
		{"setRangeText select replaces UTF-16 range and resets direction", `(function(){var e=document.createElement('input');e.value='A😀BC';e.setSelectionRange(1,3,'backward');e.setRangeText('x',1,3,'select');return [e.value,e.selectionStart,e.selectionEnd,e.selectionDirection].join('|')})()`, "AxBC|1|2|none"},
		{"setRangeText preserve keeps selection around replacement", `(function(){var e=document.createElement('input');e.value='012345';e.setSelectionRange(4,5);e.setRangeText('XY');return [e.value,e.selectionStart,e.selectionEnd].join('|')})()`, "0123XY5|4|6"},
		{"setRangeText start and end modes", `(function(){var e=document.createElement('textarea');e.value='abcdef';e.setRangeText('XY',2,4,'start');var a=[e.value,e.selectionStart,e.selectionEnd].join('|');e.setRangeText('z',1,2,'end');return a+'/'+[e.value,e.selectionStart,e.selectionEnd].join('|')})()`, "abXYef|2|2/azXYef|2|2"},
		{"selection setters clamp and collapse reversed ranges", `(function(){var e=document.createElement('textarea');e.value='abc';e.setSelectionRange(20,1,'forward');return [e.selectionStart,e.selectionEnd,e.selectionDirection].join('|')})()`, "1|1|forward"},
		{"input value assignment moves caret to new end", `(function(){var e=document.createElement('input');e.value='abcd';return [e.selectionStart,e.selectionEnd].join('|')})()`, "4|4"},
		{"clean attribute value changes clamp selection", `(function(){var e=document.createElement('input');e.setAttribute('value','abcd');e.setSelectionRange(2,4);e.setAttribute('value','x');return [e.selectionStart,e.selectionEnd].join('|')})()`, "1|1"},
		{"clean textarea CharacterData changes clamp selection", `(function(){var e=document.createElement('textarea');e.textContent='abcd';e.setSelectionRange(2,4);e.firstChild.data='x';return [e.value,e.selectionStart,e.selectionEnd].join('|')})()`, "x|1|1"},
		{"dirty textarea child changes preserve the live selection", `(function(){var e=document.createElement('textarea');e.textContent='abcd';e.value='abcdef';e.setSelectionRange(4,6);e.firstChild.data='x';return [e.value,e.selectionStart,e.selectionEnd].join('|')})()`, "abcdef|4|6"},
		{"textarea clone copies dirty live value separately from child text", `(function(){var e=document.createElement('textarea');e.textContent='default';e.value='live';var c=e.cloneNode(true);return [c.value,c.defaultValue].join('|')})()`, "live|default"},
		{"unsupported input selection reports null and throws on method", `(function(){var e=document.createElement('input');e.type='checkbox';var threw=false;try{e.setSelectionRange(0,0)}catch(x){threw=x.name==='InvalidStateError'}return [e.selectionStart,e.selectionEnd,threw].join('|')})()`, "||true"},
		{"invalid input type uses text selection behavior", `(function(){var e=document.createElement('input');e.type='unknown';e.value='abc';e.setSelectionRange(1,2);return [e.type,e.selectionStart,e.selectionEnd].join('|')})()`, "text|1|2"},
		{"textarea setRangeText uses normalized API value", `(function(){var e=document.createElement('textarea');e.value='A\r\nB';e.setRangeText('x',0,2,'end');return [e.value,e.selectionStart,e.selectionEnd].join('|')})()`, "xB|1|1"},
	})
}

func TestTextareaSpecificIDLAttributes(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"type, rows, cols, wrap defaults and textLength", `(function(){var e=document.createElement('textarea');e.value='A😀';return [e.type,e.rows,e.cols,JSON.stringify(e.wrap),e.textLength].join('|')})()`, `textarea|2|20|""|3`},
		{"rows and cols require a positive parsed attribute", `(function(){var e=document.createElement('textarea');e.setAttribute('rows','0');e.setAttribute('cols','-4');return [e.rows,e.cols].join('|')})()`, "2|20"},
		{"positive dimensions parse leading non-negative integer", `(function(){var e=document.createElement('textarea');e.setAttribute('rows',' 3tail');e.setAttribute('cols','7px');return [e.rows,e.cols].join('|')})()`, "3|7"},
		{"dimension setters reflect unsigned long and positive fallback", `(function(){var e=document.createElement('textarea');e.rows=4.8;e.cols=0;return [e.rows,e.cols,e.getAttribute('rows'),e.getAttribute('cols')].join('|')})()`, "4|20|4|20"},
		{"dimension setter uses fallback after unsigned long conversion", `(function(){var e=document.createElement('textarea');e.cols=-1;return [e.cols,e.getAttribute('cols')].join('|')})()`, "20|20"},
		{"dimension getter accepts non-negative integer parser's plus sign", `(function(){var e=document.createElement('textarea');e.setAttribute('cols','+4');return e.cols})()`, "4"},
		{"dimension getter falls back above the IDL range", `(function(){var e=document.createElement('textarea');e.setAttribute('cols','2147483648');return e.cols})()`, "20"},
		{"wrap reflects its raw DOMString attribute", `(function(){var e=document.createElement('textarea');e.wrap='HARD';return [e.wrap,e.getAttribute('wrap')].join('|')})()`, "HARD|HARD"},
		{"type is readonly textarea state", `(function(){var e=document.createElement('textarea');return [e.type,Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'type').set].join('|')})()`, "textarea|"},
	})
}

func TestOutputValueDefaultValueAndFormReset(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"output reflects type name form for tokens labels and barred validity", `(function(){var f=document.createElement('form'),l=document.createElement('label'),o=document.createElement('output');o.id='result';o.name='answer';o.setAttribute('for','a b');o.textContent='start';l.htmlFor='result';f.appendChild(l);f.appendChild(o);document.body.appendChild(f);o.setCustomValidity('custom');return [o.type,o.name,o.form===f,o.htmlFor.value,o.htmlFor===o.htmlFor,f.elements.length,f.elements[0]===o,o.labels.length,o.labels[0]===l,o.willValidate,o.validity.customError,o.checkValidity()].join('|')})()`, "output|answer|true|a b|true|1|true|1|true|false|true|true"},
		{"defaultValue and value use the output override algorithm", `(function(){var o=document.createElement('output');o.textContent='initial';var initial=o.value==='initial'&&o.defaultValue==='initial';o.defaultValue='new default';var clean=o.value==='new default'&&o.defaultValue==='new default';o.value='live';var value=o.value==='live'&&o.defaultValue==='new default';o.defaultValue='reset value';var override=o.value==='live'&&o.defaultValue==='reset value';return [initial,clean,value,override].join('|')})()`, "true|true|true|true"},
		{"form reset restores output default and clears override", `(function(){var f=document.createElement('form'),o=document.createElement('output');o.textContent='initial';f.appendChild(o);document.body.appendChild(f);o.value='live';o.defaultValue='restored';var trusted=false;f.addEventListener('reset',function(e){trusted=e.isTrusted&&e.cancelable});f.reset();return [o.value,o.defaultValue,trusted].join('|')})()`, "restored|restored|true"},
		{"canceled form reset preserves output current value", `(function(){var f=document.createElement('form'),o=document.createElement('output');o.textContent='initial';f.appendChild(o);document.body.appendChild(f);o.value='live';f.addEventListener('reset',function(e){e.preventDefault()});f.reset();return [o.value,o.defaultValue].join('|')})()`, "live|initial"},
		{"reset button activation resets output", `(function(){var f=document.createElement('form'),o=document.createElement('output'),b=document.createElement('button');o.textContent='initial';b.type='reset';f.appendChild(o);f.appendChild(b);document.body.appendChild(f);o.value='live';b.click();return o.value})()`, "initial"},
		{"form reset restores checkbox default and clears checkedness dirtiness", `(function(){var f=document.createElement('form'),c=document.createElement('input');c.type='checkbox';c.defaultChecked=true;f.appendChild(c);document.body.appendChild(f);c.checked=false;var before=[c.checked,c.defaultChecked].join('|');f.reset();return before+'/'+[c.checked,c.defaultChecked].join('|')})()`, "false|true/true|true"},
		{"output value is excluded from form data", `(function(){var f=document.createElement('form'),o=document.createElement('output'),i=document.createElement('input');o.name='answer';o.value='42';i.name='query';i.value='q';f.appendChild(o);f.appendChild(i);document.body.appendChild(f);var d=new FormData(f);return [d.get('answer'),d.get('query'),d.has('answer')].join('|')})()`, "|q|false"},
	})
}

func TestCheckboxClickActivationStateAndEvents(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{{
		name: "click pre-activates, dispatches trusted events, and rolls back cancellation",
		script: `(function(){
			var c=document.createElement('input'); c.type='checkbox';
			document.body.appendChild(c);
			var events=[];
			c.addEventListener('click',function(e){events.push('click:'+c.checked+':'+c.indeterminate+':'+e.isTrusted)});
			c.addEventListener('input',function(e){events.push('input:'+c.checked+':'+c.indeterminate+':'+e.isTrusted)});
			c.addEventListener('change',function(e){events.push('change:'+c.checked+':'+c.indeterminate+':'+e.isTrusted)});
			c.indeterminate=true; c.click();
			var first=[c.checked,c.indeterminate,events.join('>')].join('|');
			c.checked=false; c.indeterminate=false; events=[];
			c.addEventListener('click',function(e){e.preventDefault()}); c.click();
			var canceled=[c.checked,c.indeterminate,events.length].join('|');
			c.addEventListener('click',function(e){e.preventDefault()});
			return first+'/'+canceled;
		})()`,
		want: "true|false|click:true:false:false>input:true:false:true>change:true:false:true/false|false|1",
	}})
}

func TestTextControlSelectEventIsQueuedAndTrusted(t *testing.T) {
	rt := interfacesRuntime(t)
	evalString(t, rt, `window.__selectEvents=[];var e=document.createElement('input');e.setAttribute('value','abc');document.body.addEventListener('select',function(ev){window.__selectEvents.push(ev.isTrusted+'|'+ev.bubbles+'|'+ev.target.selectionStart+':'+ev.target.selectionEnd)});document.body.appendChild(e);e.setSelectionRange(1,3);window.__selectInput=e;window.__beforeSelect=window.__selectEvents.length`)
	if got := evalString(t, rt, `window.__beforeSelect+'/'+window.__selectEvents.length`); got != "0/0" {
		t.Fatalf("select event fired synchronously: %q", got)
	}
	if !rt.Tick() {
		t.Fatal("queued select task was not available")
	}
	if got := evalString(t, rt, `window.__selectEvents.join(',')`); got != "true|true|1:3" {
		t.Fatalf("queued select event = %q", got)
	}
	evalString(t, rt, `window.__selectInput.setSelectionRange(1,3)`)
	rt.Tick()
	if got := evalString(t, rt, `window.__selectEvents.length`); got != "1" {
		t.Fatalf("unchanged selection emitted another event: %s", got)
	}
}

// qc:BR-245
func TestImageLoadStateAndEventsForADataSource(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"complete is false right after src", `(function(){
			var i = new Image(); var ev = [];
			i.onload = function(){ ev.push('load'); };
			i.onerror = function(){ ev.push('error'); };
			window.__img = i; window.__ev = ev;
			i.src = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8ADAAAHAAH/Cxg0AAAAAElFTkSuQmCC';
			return [i.complete,i.naturalWidth,String(i.currentSrc)].join('|');
		})()`, "false|0|"},
		{"load event and natural size after the turn", `(function(){
			if (window.__img.complete !== true) return 'not complete yet';
			return [window.__ev.join(','),window.__img.naturalWidth,window.__img.naturalHeight,window.__img.width,window.__img.height].join('|');
		})()`, "load|1|1|1|1"},
		{"currentSrc names the settled source", `window.__img.currentSrc === window.__img.src`, "true"},
		{"decode promise starts pending", `(function(){
			window.__decoded = 'pending';
			window.__img.decode().then(function(){ window.__decoded = 'resolved'; });
			return window.__decoded;
		})()`, "pending"},
		{"decode resolves after the turn", `window.__decoded`, "resolved"},
		{"decode without a source starts pending and rejects after the turn", `(function(){
			window.__decodedBad = 'pending';
			new Image().decode().then(function(){ window.__decodedBad = 'resolved'; }, function(e){ window.__decodedBad = 'rejected:' + e.name; });
			return window.__decodedBad;
		})()`, "pending"},
		{"decode without a source rejects EncodingError", `window.__decodedBad`, "rejected:EncodingError"},
		{"unparseable source fires error", `(function(){
			var i = new Image(); var ev = [];
			i.onload = function(){ ev.push('load'); };
			i.onerror = function(){ ev.push('error'); };
			i.src = 'http://[::1/x.png';
			window.__bad = i; window.__badev = ev;
			return [i.complete,String(i.currentSrc)].join('|');
		})()`, "false|"},
		{"unparseable source settles as an error", `(function(){
			return [window.__badev.join(','),window.__bad.complete,window.__bad.naturalWidth,window.__bad.getAttribute('src')].join('|');
		})()`, "error|true|0|http://[::1/x.png"},
		{"unparseable source keeps the raw currentSrc", `window.__bad.currentSrc`, "http://[::1/x.png"},
		{"empty source resolves to the document URL", `(function(){
			var i = new Image(); window.__empty = i; window.__emptyev = [];
			i.onerror = function(){ window.__emptyev.push('error'); };
			i.src = '';
			return [i.complete,i.src,i.getAttribute('src'),String(i.currentSrc)].join('|');
		})()`, "false|https://example.test/docs/page.html||"},
		{"empty source settles as an error", `(function(){
			return [window.__emptyev.join(','),window.__empty.complete,String(window.__empty.currentSrc)].join('|');
		})()`, "error|true|"},
	})
}

// qc:BR-246
func TestImageReflectionsBeyondTheSource(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"alt default and write", `(function(){var i=document.createElement('img');var a=i.alt;i.alt='a';return a+'|'+i.getAttribute('alt')})()`, "|a"},
		{"srcset and sizes reflect", `(function(){var i=document.createElement('img');i.srcset='a.png 1x';i.sizes='100vw';return i.srcset+'|'+i.sizes+'|'+i.getAttribute('srcset')})()`, "a.png 1x|100vw|a.png 1x"},
		{"name reflects", `(function(){var i=document.createElement('img');i.name='n';return i.name+'|'+i.getAttribute('name')})()`, "n|n"},
		{"useMap reflects", `(function(){var i=document.createElement('img');i.useMap='#m';return i.useMap+'|'+i.getAttribute('usemap')})()`, "#m|#m"},
		{"border default and write", `(function(){var i=document.createElement('img');var b=i.border;i.border='2';return b+'|'+i.getAttribute('border')})()`, "|2"},
		{"longDesc resolves as a URL", `(function(){var i=document.createElement('img');i.longDesc='/d';return i.getAttribute('longdesc')+'|'+i.longDesc})()`, "/d|https://example.test/d"},
		{"hspace and vspace reflect", `(function(){var i=document.createElement('img');i.hspace=3;i.vspace=4;return i.hspace+'|'+i.vspace})()`, "3|4"},
		{"isMap reflects", `(function(){var i=document.createElement('img');i.isMap=1;return i.isMap+'|'+i.getAttribute('ismap')})()`, "true|"},
		{"x and y are zero for a detached image", `(function(){var i=new Image();return i.x+'|'+i.y})()`, "0|0"},
		{"parsed image src is absolute", `document.getElementById('i').src`, "https://example.test/docs/x.png"},
		{"parsed image currentSrc is absolute", `document.getElementById('i').currentSrc`, "https://example.test/docs/x.png"},
		{"parsed image natural size is zero before a load", `document.getElementById('i').naturalWidth+'|'+document.getElementById('i').naturalHeight`, "0|0"},
		{"parsed image is complete", `document.getElementById('i').complete`, "true"},
	})
}

func TestImageButtonDimensionsAndLoadState(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"image input dimension attributes use the HTML parser", `(function(){
			var i=document.createElement('input'); i.type='image';
		i.setAttribute('width','-0.5'); i.setAttribute('height','10px');
		return [i.width,i.height].join('|');
		})()`, "0|10"},
	})
}

func TestInputShowPickerActivationAndMutability(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"showPicker requires transient activation", `(function(){
			var i=document.createElement('input'); i.type='date';
			try { i.showPicker(); return 'no error'; } catch(e) { return e.name; }
		})()`, "NotAllowedError"},
		{"disabled input fails before activation checks", `(function(){
			var i=document.createElement('input'); i.disabled=true;
			try { i.showPicker(); return 'no error'; } catch(e) { return e.name; }
		})()`, "InvalidStateError"},
	})
	rt.SetTransientUserActivation(true)
	runJSCases(t, rt, []jsCase{
		{"readonly date fails and preserves activation", `(function(){
			var i=document.createElement('input'); i.type='date'; i.readOnly=true;
			var name; try { i.showPicker(); } catch(e) { name=e.name; }
			return name+'|'+navigator.userActivation.isActive+'|'+navigator.userActivation.hasBeenActive;
		})()`, "InvalidStateError|true|true"},
		{"readonly ignored for button and successful call consumes activation without focusing", `(function(){
			var i=document.createElement('input'); i.type='button'; i.readOnly=true;
			document.body.appendChild(i); var focused=document.activeElement===i;
			var name=''; try { i.showPicker(); } catch(e) { name=e.name; }
			return name+'|'+focused+'|'+(document.activeElement===i)+'|'+navigator.userActivation.isActive+'|'+navigator.userActivation.hasBeenActive;
		})()`, "|false|false|false|true"},
	})
	rt.EnableTestUserActivationHook()
	runJSCases(t, rt, []jsCase{
		{"readonly applicability follows input state", `(function(){
			var ignored=['button','checkbox','color','file','hidden','image','radio','range','reset','submit'];
			var applicable=['text','search','tel','url','email','password','date','month','week','time','datetime-local','number'];
			var errors=[];
			for (var i=0;i<ignored.length;i++) {
				var el=document.createElement('input'); el.type=ignored[i]; el.readOnly=true;
				window.__goosieGrantUserActivation();
				try { el.showPicker(); } catch(e) { errors.push(ignored[i]+':'+e.name); }
				if (navigator.userActivation.isActive) errors.push(ignored[i]+':not-consumed');
			}
			for (var j=0;j<applicable.length;j++) {
				var el=document.createElement('input'); el.type=applicable[j]; el.readOnly=true;
				window.__goosieGrantUserActivation();
				try { el.showPicker(); errors.push(applicable[j]+':did-not-throw'); } catch(e) { if(e.name!=='InvalidStateError') errors.push(applicable[j]+':'+e.name); }
				if (!navigator.userActivation.isActive) errors.push(applicable[j]+':activation-consumed');
			}
			return errors.join(',');
		})()`, ""},
	})
}

func TestSelectShowPickerPreconditionsAndActivation(t *testing.T) {
	rt := interfacesRuntime(t)
	rt.EnableTestUserActivationHook()
	runJSCases(t, rt, []jsCase{
		{"disabled select throws before activation check", `(function(){
			var s=document.createElement('select');s.disabled=true;
			try{s.showPicker();return 'none'}catch(e){return e.name}
		})()`, "InvalidStateError"},
		{"detached select throws NotSupportedError before consuming activation", `(function(){
			var s=document.createElement('select');window.__goosieGrantUserActivation();
			try{s.showPicker();return 'none'}catch(e){return e.name+'|'+navigator.userActivation.isActive}
		})()`, "NotSupportedError|true"},
		{"rendered select consumes activation without focusing", `(function(){
			var s=document.createElement('select');document.body.appendChild(s);
			var focused=document.activeElement===s;window.__goosieGrantUserActivation();
			try{s.showPicker()}catch(e){return e.name}
			return focused+'|'+(document.activeElement===s)+'|'+navigator.userActivation.isActive+'|'+navigator.userActivation.hasBeenActive
		})()`, "false|false|false|true"},
		{"content-visibility hidden select is not rendered", `(function(){
			var p=document.createElement('div'),s=document.createElement('select');p.style.contentVisibility='hidden';p.appendChild(s);document.body.appendChild(p);
			window.__goosieGrantUserActivation();try{s.showPicker();return 'none'}catch(e){return e.name+'|'+navigator.userActivation.isActive}
		})()`, "NotSupportedError|true"},
	})
}

func TestTextareaLiveValueAndDefaultValue(t *testing.T) {
	rt := interfacesRuntime(t)
	got := evalString(t, rt, `(function(){
		var clean=document.createElement('textarea');
		clean.textContent='first\r\nline';
		var initial=clean.value==='first\nline'&&clean.defaultValue==='first\r\nline';
		clean.defaultValue='updated';
		var cleanSync=clean.value==='updated'&&clean.textContent==='updated';
		clean.value='live\rvalue';
		var live=clean.value==='livevalue'&&clean.defaultValue==='updated'&&clean.textContent==='updated';
		clean.defaultValue='later';
		var dirty=clean.value==='livevalue'&&clean.defaultValue==='later'&&clean.textContent==='later';
		var child=document.createElement('textarea');
		child.textContent='a';
		child.firstChild.data='b';
		var childSync=child.value==='b';
		return [initial,cleanSync,live,dirty,childSync].join('|');
	})()`)
	if got != "true|true|true|true|true" {
		t.Fatalf("textarea live/default value reflection = %q, want all checks true", got)
	}
}

func TestHTMLConstructorProxyPreservesInheritedHasInstance(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body></body></html>`)
	rt := newRuntime(t, js.Options{DOM: doc})
	got := evalString(t, rt, `(function() {
		var constructors = [HTMLElement, HTMLDivElement, HTMLButtonElement];
		var noOwnOverride = constructors.every(function(constructor) {
			return !Object.prototype.hasOwnProperty.call(constructor, Symbol.hasInstance);
		});
		var div = document.createElement('div');
		return [noOwnOverride, div instanceof HTMLElement, div instanceof HTMLDivElement, !(div instanceof HTMLButtonElement)].join('|');
	})()`)
	if got != "true|true|true|true" {
		t.Fatalf("HTML constructor proxy @@hasInstance behavior = %q, want inherited OrdinaryHasInstance checks", got)
	}
}
