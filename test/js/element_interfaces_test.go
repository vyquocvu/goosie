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
		// Chromium mints HTMLUnknownElement for a tag it does not know (measured
		// 2026-10-26: constructor.name "HTMLUnknownElement", still instanceof
		// HTMLElement). Goosie has no HTMLUnknownElement interface and answers
		// HTMLElement; §9.2 records that absence, so this pins only the part
		// both engines agree on.
		{"unknown tag inherits HTMLElement", `document.createElement('foo') instanceof HTMLElement`, "true"},
		{"standard tag outside the interface table answers HTMLElement", `document.createElement('section').constructor === HTMLElement`, "true"},
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
		{"attributes length follows writes", `(function(){var e=document.createElement('div');var n=e.attributes.length;e.setAttribute('id','x');return String(n)+','+e.attributes.length})()`, "0,1"},
		{"style identity", `(function(){var e=document.createElement('div');return e.style===e.style})()`, "true"},
		{"style setter canonicalises the attribute", `(function(){var e=document.createElement('div');e.style.color='red';return e.getAttribute('style')})()`, "color: red;"},
		{"style reads from the attribute", `(function(){var e=document.createElement('div');e.setAttribute('style','color: blue');return e.style.color})()`, "blue"},
	})
}

// qc:BR-242
func TestAnchorURLReflectionResolvesAgainstTheDocumentBase(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"root relative href", `document.getElementById('a').href`, "https://example.test/p?q=1"},
		{"hostname", `document.getElementById('a').hostname`, "example.test"},
		{"target default", `document.getElementById('a').target`, ""},
		{"rel default", `document.getElementById('a').rel`, ""},
		{"baseURI", `document.baseURI`, "https://example.test/docs/page.html"},
		{"dot relative", `(function(){var e=document.createElement('a');e.setAttribute('href','./x');return e.href})()`, "https://example.test/docs/x"},
		{"parent relative", `(function(){var e=document.createElement('a');e.setAttribute('href','../up/y');return e.href})()`, "https://example.test/up/y"},
		{"fragment", `(function(){var e=document.createElement('a');e.setAttribute('href','#f');return e.href+','+e.hash})()`, "https://example.test/docs/page.html#f,#f"},
		{"query only", `(function(){var e=document.createElement('a');e.setAttribute('href','?z=2');return e.href+','+e.search})()`, "https://example.test/docs/page.html?z=2,?z=2"},
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

// qc:BR-243
func TestInputValueStaysLiveAndDefaultValueReflectsTheAttribute(t *testing.T) {
	rt := interfacesRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"initial live value and default", `(function(){var e=document.getElementById('in');return [e.value,e.defaultValue,e.getAttribute('value')].join('|')})()`, "v0|v0|v0"},
		// TASK-030/031/037 contract: the live value is stored through the value
		// attribute, so defaultValue follows it. Chromium keeps the two apart;
		// §9 records that measured divergence.
		{"value write reaches the attribute and the default", `(function(){var e=document.getElementById('in');e.value='zz';return [e.value,e.defaultValue,e.getAttribute('value')].join('|')})()`, "zz|zz|zz"},
		{"defaultValue write sets the attribute", `(function(){var e=document.getElementById('in');e.value='zz';e.defaultValue='dd';return [e.value,e.defaultValue,e.getAttribute('value')].join('|')})()`, "dd|dd|dd"},
		{"setAttribute value becomes the live value", `(function(){var e=document.getElementById('in');e.value='zz';e.defaultValue='dd';e.setAttribute('value','aa');return [e.value,e.defaultValue,e.getAttribute('value')].join('|')})()`, "aa|aa|aa"},
		{"defaultValue null stores null", `(function(){var e=document.createElement('input');e.setAttribute('value','v');e.defaultValue=null;return e.getAttribute('value')+'|'+e.defaultValue})()`, "null|null"},
		{"no attribute reads empty", `(function(){var e=document.createElement('input');return [e.value,e.defaultValue,e.type].join('|')})()`, "||text"},
		{"checkbox without a value reads on", `(function(){var e=document.createElement('input');e.type='checkbox';return [e.type,e.getAttribute('type'),e.value].join('|')})()`, "checkbox|checkbox|on"},
		{"checkbox with a value reads it", `(function(){var e=document.createElement('input');e.type='checkbox';e.value='yes';return e.value})()`, "yes"},
		{"checked writes the attribute in this engine", `(function(){var e=document.createElement('input');e.type='checkbox';e.checked=true;return [e.checked,e.defaultChecked,e.getAttribute('checked')].join('|')})()`, "true|true|"},
		{"defaultChecked writes the attribute", `(function(){var e=document.createElement('input');e.type='checkbox';e.defaultChecked=true;return [e.checked,e.defaultChecked,e.getAttribute('checked')].join('|')})()`, "true|true|"},
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
