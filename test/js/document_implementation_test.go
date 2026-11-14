package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func detachedRuntime(t *testing.T, html string) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(html)
	return newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
}

// qc:BR-208
func TestCreateHTMLDocumentBuildsTheStandardSkeleton(t *testing.T) {
	rt := detachedRuntime(t, `<!DOCTYPE html><html><head><title>Main</title></head><body><p id="a">main text</p></body></html>`)
	cases := []struct{ name, script, want string }{
		{"member exists", `typeof document.implementation.createHTMLDocument`, "function"},
		{"is a document node", `String(document.implementation.createHTMLDocument("Scratch").nodeType)`, "9"},
		{"two root children", `(function(){var d=document.implementation.createHTMLDocument("Scratch");return String(d.childNodes.length)})()`, "2"},
		{"doctype precedes html", `(function(){var d=document.implementation.createHTMLDocument("Scratch");return [d.firstChild.nodeType,d.firstChild.name,d.lastChild.nodeType,d.lastChild.nodeName].join(",")})()`, "10,html,1,HTML"},
		{"head and body under html", `(function(){var d=document.implementation.createHTMLDocument("Scratch");return [d.head.nodeName,d.body.nodeName,String(d.head.parentNode===d.documentElement),String(d.body.parentNode===d.documentElement)].join(",")})()`, "HEAD,BODY,true,true"},
		{"title is the argument", `String(document.implementation.createHTMLDocument("Scratch").title)`, "Scratch"},
		{"empty title is empty", `(function(){var d=document.implementation.createHTMLDocument("");return [String(d.title===""),String(d.body.firstChild===null),String(d.body.childNodes.length)].join(",")})()`, "true,true,0"},
		{"own documentElement identity", `(function(){var d=document.implementation.createHTMLDocument("Scratch");return [String(d.documentElement.ownerDocument===d),String(d.ownerDocument===null)].join(",")})()`, "true,true"},
		{"live document untouched", `(function(){document.implementation.createHTMLDocument("Scratch");return [String(document.getElementById("a")!==null),String(document.body.childNodes.length),String(document.getElementsByTagName("form").length)].join(",")})()`, "true,1,0"},
	}
	for _, c := range cases {
		if got := evalString(t, rt, c.script); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	if got := evalString(t, rt, `(function(){return [document.firstChild.nodeType,document.firstChild.name,document.lastChild.nodeType,document.lastChild.nodeName].join(",")})()`); got != "10,html,1,HTML" {
		t.Errorf("live document doctype/html order: got %q want %q", got, "10,html,1,HTML")
	}
}

func TestCreateCDATASectionOnXMLDocument(t *testing.T) {
	rt := detachedRuntime(t, `<!DOCTYPE html><html><body></body></html>`)
	got := evalString(t, rt, `
		var xml = new Document();
		var section = xml.createCDATASection('value');
		var htmlThrows = false;
	var dataThrows = false;
		try { document.createCDATASection('value'); } catch (e) { htmlThrows = e.name === 'NotSupportedError'; }
		try { xml.createCDATASection('bad]]>value'); } catch (e) { dataThrows = e.name === 'InvalidCharacterError'; }
		[section.nodeType, section.data, String(section.ownerDocument === xml), String(htmlThrows), String(dataThrows)].join(',');
	`)
	want := "4,value,true,true,true"
	if got != want {
		t.Fatalf("createCDATASection result = %q, want %q", got, want)
	}
}

func TestDOMElementNameValidationUsesDOMRules(t *testing.T) {
	rt := detachedRuntime(t, `<!DOCTYPE html><html><body></body></html>`)
	cases := []struct {
		name string
		js   string
		want string
	}{
		{"DOMImplementation accepts parser-compatible punctuation", `document.implementation.createDocument(null, 'f}oo').documentElement.localName`, "f}oo"},
		{"DOMImplementation accepts U+FFFF in an element name", `document.implementation.createDocument(null, '\uFFFFfoo').documentElement.localName`, "\uFFFFfoo"},
		{"DOMImplementation accepts a less-than sign after an ASCII start", `document.implementation.createDocument(null, 'f<oo').documentElement.localName`, "f<oo"},
		{"createElementNS shares the DOM name rules", `document.createElementNS(null, 'f}oo').localName`, "f}oo"},
		{"namespace and local name are extracted", `document.createElementNS('urn:test', 'pre:local').localName`, "local"},
		{"invalid trailing greater-than throws", `(function(){try{document.implementation.createDocument(null,'foo>');return 'no error'}catch(e){return e.name}})()`, "InvalidCharacterError"},
		{"missing prefix namespace throws", `(function(){try{document.createElementNS(null,'pre:local');return 'no error'}catch(e){return e.name}})()`, "NamespaceError"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := evalString(t, rt, tc.js); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// qc:BR-208
func TestJQueryCreateHTMLDocumentProbeIsTheExactExpression(t *testing.T) {
	rt := detachedRuntime(t, `<!DOCTYPE html><html><head><title>Main</title></head><body><p>main</p></body></html>`)
	got := evalString(t, rt, `(function(){
		var E = document, _t, flag;
		flag = ((_t = E.implementation.createHTMLDocument("").body).innerHTML = "<form></form><form></form>", 2 === _t.childNodes.length);
		return String(flag)
	})()`)
	if got != "true" {
		t.Fatalf("jQuery's createHTMLDocument probe returned %q, not true; with that flag false jQuery takes a different buildFragment path and the measured pages' markup never reaches the DOM", got)
	}
	if again := evalString(t, rt, `(function(){return [String(document.body.childNodes.length),String(document.getElementsByTagName("form").length),String(document.implementation.createHTMLDocument("").body.childNodes.length)].join(",")})()`); again != "1,0,0" {
		t.Errorf("detached innerHTML scoping: got %q want %q", again, "1,0,0")
	}
	checked := evalString(t, rt, `(function(){
		var d = document.implementation.createHTMLDocument(""), input = d.createElement("input");
		input.setAttribute("type", "checkbox");
		input.checked = true;
		d.body.appendChild(input);
		var clone = input.cloneNode(true);
		return [String(d.body.childNodes.length), String(clone.checked), String(clone !== input), String(clone.ownerDocument === d), String(input.parentNode === d.body)]
	})()`)
	if want := "1,true,true,true,true"; checked != want {
		t.Errorf("detached clone chain: got %q want %q", checked, want)
	}
}

// qc:BR-208
func TestParseHTMLBaseHrefBranchRunsAgainstTheDetachedHead(t *testing.T) {
	rt := detachedRuntime(t, `<!DOCTYPE html><html><head><title>Main</title></head><body></body></html>`)
	got := evalString(t, rt, `(function(){
		var E = document, t, r;
		t = E.implementation.createHTMLDocument("");
		r = t.createElement("base");
		r.href = E.location.href;
		t.head.appendChild(r);
		return [String(r.href), String(t.head.childNodes.length), String(t.head.querySelector("base") !== null), String(t.querySelector("base") === r), String(E.querySelector("base") === null), String(t.head.lastChild.tagName)]
	})()`)
	want := strings.Join([]string{
		"https://example.test/page", "2", "true", "true", "true", "BASE",
	}, ",")
	if got != want {
		t.Errorf("parseHTML base branch: got %q want %q", got, want)
	}
}

// qc:BR-208
func TestDetachedDocumentIsIsolatedButIdentityStable(t *testing.T) {
	rt := detachedRuntime(t, `<!DOCTYPE html><html><head><title>Main</title></head><body><div id="live">x</div></body></html>`)
	got := evalString(t, rt, `(function(){
		var d = document.implementation.createHTMLDocument("Scratch"), el = d.createElement("div");
		el.setAttribute("id", "probe");
		el.setAttribute("class", "p1");
		d.body.appendChild(el);
		return [
			String(d !== document),
			String(d.getElementById("probe") === el),
			String(d.getElementById("live") === null),
			String(document.getElementById("probe") === null),
			String(document.getElementById("live") !== null),
			String(el.ownerDocument === d),
			String(el.parentNode === d.body),
			String(d.getElementsByTagName("div").length),
			String(d.getElementsByClassName("p1").length),
			String(document.getElementsByTagName("div").length),
			String(d.querySelector("#probe") === el),
			String(d.querySelector(".p1") === el),
			String(d.querySelectorAll("div").length),
			String(document.querySelectorAll("div").length)
		]
	})()`)
	want := strings.Join([]string{
		"true",
		"true",
		"true",
		"true",
		"true",
		"true",
		"true",
		"1",
		"1",
		"1",
		"true",
		"true",
		"1",
		"1",
	}, ",")
	if got != want {
		t.Errorf("detached document isolation: got %q want %q", got, want)
	}
}

// qc:BR-208
func TestDetachedDocumentConstructors(t *testing.T) {
	rt := detachedRuntime(t, `<!DOCTYPE html><html><head></head><body></body></html>`)
	got := evalString(t, rt, `(function(){
		var d = document.implementation.createHTMLDocument("Scratch");
		var text = d.createTextNode("hi"), comment = d.createComment("c"), frag = d.createDocumentFragment();
		frag.appendChild(d.createElement("b"));
		frag.appendChild(d.createElement("i"));
		d.body.appendChild(frag);
		return [
			String(text.nodeType), text.nodeValue, String(text.ownerDocument === d),
			String(comment.nodeType), comment.nodeValue,
			String(frag.nodeType), String(frag.childNodes.length),
			String(d.body.childNodes.length),
			d.body.innerHTML,
			String(d.body.lastChild.tagName)
		]
	})()`)
	want := strings.Join([]string{
		"3", "hi", "true",
		"8", "c",
		"11", "0",
		"2",
		"<b></b><i></i>",
		"I",
	}, ",")
	if got != want {
		t.Errorf("detached constructors: got %q want %q", got, want)
	}
}

// qc:BR-208
func TestDetachedDocumentTitleIsTextAndIsWritable(t *testing.T) {
	rt := detachedRuntime(t, `<!DOCTYPE html><html><head></head><body></body></html>`)
	got := evalString(t, rt, `(function(){
		var d = document.implementation.createHTMLDocument("<img src=x onerror=boom>");
		var readBack = d.title, headChildren = d.head.childNodes.length, imgLeaked = d.head.querySelector("img") !== null;
		d.title = "renamed";
		return [String(readBack), String(headChildren), String(imgLeaked), String(d.title), String(d.head.firstChild.textContent), String(d.head.childNodes.length)]
	})()`)
	want := strings.Join([]string{
		"<img src=x onerror=boom>", "1", "false", "renamed", "renamed", "1",
	}, ",")
	if got != want {
		t.Errorf("detached title: got %q want %q", got, want)
	}
}

// qc:BR-208
func TestDocumentImplementationOmitsUnimplementedMembers(t *testing.T) {
	rt := detachedRuntime(t, `<!DOCTYPE html><html><head><title>Main</title></head><body></body></html>`)
	got := evalString(t, rt, `[
		Object.keys(document.implementation).join(","),
		typeof document.implementation.createDocument,
		typeof document.implementation.hasFeature,
		typeof document.implementation.createHTMLDocument,
		String(typeof document.title) + "," + String(document.title)
	].join("|")`)
	want := "createHTMLDocument,createDocumentType,createDocument|function|undefined|function|string,Main"
	if got != want {
		t.Errorf("implementation members: got %q want %q", got, want)
	}
}

// qc:BR-208
func TestCreateElementOnTheLiveDocumentStillUsesTheLiveTree(t *testing.T) {
	rt := detachedRuntime(t, `<!DOCTYPE html><html><head></head><body></body></html>`)
	got := evalString(t, rt, `(function(){
		var d = document.implementation.createHTMLDocument("Scratch");
		var live = document.createElement("section");
		live.setAttribute("id", "added-live");
		var scratch = d.createElement("section");
		scratch.setAttribute("id", "added-scratch");
		d.body.appendChild(scratch);
		return [
			String(document.getElementById("added-live") === null),
			String(document.getElementById("added-scratch") === null),
			String(live.ownerDocument === document),
			String(scratch.ownerDocument === document),
			String(scratch.parentNode !== null)
		]
	})()`)
	want := strings.Join([]string{"true", "true", "true", "false", "true"}, ",")
	if got != want {
		t.Errorf("live vs detached createElement: got %q want %q", got, want)
	}
}
