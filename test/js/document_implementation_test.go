package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// document.implementation.createHTMLDocument, and the detached document it
// returns.
//
// The evidence is a measured throw, not a spec checklist. With every Node API in
// this task's cluster already in place, the real jquery-3.6.0.min.js (89,501
// bytes) inlined into a page still died while the library was installing itself:
//
//	TypeError: Cannot read property 'createHTMLDocument' of undefined or null
//
// because jQuery's load-time support probe is an unconditional statement:
//
//	y.createHTMLDocument = ((_t = E.implementation.createHTMLDocument("")
//	                          .body).innerHTML = "<form></form><form></form>",
//	                       2 === _t.childNodes.length)
//
// and S.parseHTML reaches for the same member again for every multi-tag
// fragment. Past that throw `jQuery` is never defined, so the page's own
// `jQuery(function($){...})` dies as "jQuery is not defined" and the render
// never gets further than the server HTML. TestJQueryCreateHTMLDocumentProbeIs
// TheExactExpression pins that statement; the rest pin the semantics it needs.
//
// Deliberate omissions, each named in docs/tasks/TASK-047-DOM-JQUERY-APIS.md:
// implementation.createDocument and implementation.hasFeature stay absent (neither
// jQuery nor any of the five saved pages calls them, and a stub would advertise a
// document-type path this engine cannot run), and the returned document is a
// scratch tree only - never styled, laid out or painted, and never the runtime's
// `document`.
//
// Every assertion reads through String(...)/explicit comparisons, because Array
// join turns null and undefined into empty slots and would hide exactly the
// distinction these tests care about: a member that is missing vs a member that
// answers null.

func detachedRuntime(t *testing.T, html string) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(html)
	return newRuntime(t, js.Options{URL: "https://example.test/page", DOM: doc})
}

// The standard's createHTMLDocument(title) builds a document whose content is a
// doctype plus html > (head > title) and an empty body. The assertions are that
// shape, the HTML casing browsers use for nodeName, and the fact that the live
// document is untouched by it.
func TestCreateHTMLDocumentBuildsTheStandardSkeleton(t *testing.T) {
	rt := detachedRuntime(t, `<!DOCTYPE html><html><head><title>Main</title></head><body><p id="a">main text</p></body></html>`)
	cases := []struct{ name, script, want string }{
		{"member exists", `typeof document.implementation.createHTMLDocument`, "function"},
		{"is a document node", `String(document.implementation.createHTMLDocument("Scratch").nodeType)`, "9"},
		{"two root children", `(function(){var d=document.implementation.createHTMLDocument("Scratch");return String(d.childNodes.length)})()`, "2"},
		{"html first, doctype present", `(function(){var d=document.implementation.createHTMLDocument("Scratch");return [d.firstChild.nodeType,d.firstChild.nodeName,d.lastChild.nodeType,d.lastChild.nodeName].join(",")})()`, "1,HTML,10,#doctype"},
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
	// The doctype-after-html child order is this parser's, not a choice made
	// here: a live document parsed from the same source shows the same two
	// children in the same order, so a detached document is the tree the engine
	// already builds rather than a second shape to keep in sync.
	if got := evalString(t, rt, `(function(){return [document.firstChild.nodeType,document.lastChild.nodeType,document.lastChild.nodeName].join(",")})()`); got != "1,10,#doctype" {
		t.Errorf("live document child order differs from the detached document's: got %q want %q - one of the two parse paths changed", got, "1,10,#doctype")
	}
}

// jQuery's own probe, quoted rather than paraphrased, because it is the
// statement whose throw stopped the library from installing: the body of a
// detached document must take innerHTML, the two forms must arrive as two child
// nodes, and the comparison must be exactly true - that boolean is stored in
// y.createHTMLDocument and branched on by every later innerHTML/parseHTML path.
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
	// The forms belong to the document they were written into. If innerHTML had
	// resolved the host document instead of the node's own, the live body would
	// have grown; and because each call returns a *new* document, a second one
	// starts with an empty body - a scratch tree jQuery can reuse without
	// inheriting the previous scratch's content.
	if again := evalString(t, rt, `(function(){return [String(document.body.childNodes.length),String(document.getElementsByTagName("form").length),String(document.implementation.createHTMLDocument("").body.childNodes.length)].join(",")})()`); again != "1,0,0" {
		t.Errorf("detached innerHTML scoping: got %q want %q", again, "1,0,0")
	}
	// The block that runs only when that flag is true, in jQuery's own shape: an
	// input appended to a detached document and cloned must carry its checked
	// state, and the clone must be a different node with the same owner.
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

// S.parseHTML's base-href branch, quoted from the same library:
//
//	((r = (t = E.implementation.createHTMLDocument("")).createElement("base")
//	  ).href = E.location.href, t.head.appendChild(r))
//
// It needs createElement on the detached document, an href write that reads back,
// a live location.href on the *page* document, and head as a real node whose
// children include the base.
//
// One caveat is asserted rather than hidden: `r.href = ...` lands as the JS
// property jQuery wrote, not as a `href` attribute, because this engine reflects
// IDL attributes only for a few members (internal/js/js.go has no href
// accessor). That is pre-existing, it is not on a measured blocker path - the
// base only matters to a browser's URL resolution, and this engine never
// resolves a relative URL through a detached scratch document - and it is listed
// in docs/tasks/TASK-047-DOM-JQUERY-APIS.md with the other named omissions.
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

// The detached document is isolated but identity-stable: elements created in it
// answer ownerDocument with that exact object (not a look-alike wrapper), its
// id/tag/class indexes and querySelector search only its own subtree, and the
// live document cannot see it. jQuery's data cache and Sizzle's context test
// both key off those identities.
//
// The query path matters here in a way it did not before this file: a query
// rooted in a detached document used to be answered from the live document's
// indexes filtered by "is this node under that root", which is always none and
// is reported as a handled miss - a silent "not present" for content that is
// present. See indexDocFor in internal/js/js.go.
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
		"true", // a different document object
		"true", // its own getElementById finds the created node
		"true", // and never the live document's node
		"true", // the live document cannot see the detached id
		"true", // the live id still resolves in the live document
		"true", // ownerDocument is that object by identity
		"true", // parenting works inside it
		"1",    // its own div only; the live one is in another tree
		"1",    // class index scoped to it
		"1",    // the live document still sees only its own div
		"true", // querySelector answers from the detached tree
		"true", // by id and by class alike
		"1",    // querySelectorAll too
		"1",    // and the live document's query is unaffected
	}, ",")
	if got != want {
		t.Errorf("detached document isolation: got %q want %q", got, want)
	}
}

// The rest of the constructor set the detached document has to answer, because
// S.parseHTML and jQuery's clean/clone helpers call them on whatever document is
// in hand rather than on the live one. Node types are the standard's 3/8/11, and
// a fragment still means "its children move, it stays empty".
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
		"11", "0", // the fragment emptied itself
		"2", // body holds b and i
		"<b></b><i></i>",
		"I",
	}, ",")
	if got != want {
		t.Errorf("detached constructors: got %q want %q", got, want)
	}
}

// title is the head's title element, read and written, and a title argument
// containing markup stays text: the skeleton is built by parsing, so an
// unescaped "<" in the argument would otherwise arrive as an element.
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

// createDocument and hasFeature are named absent members: an implementation that
// answered them would advertise a document-type/feature path this engine does not
// implement, and a page branching on hasFeature would take that branch in
// silence. Pinning the absence means adding a stub later is a decision with a
// failing assertion behind it rather than a quiet change of behaviour.
//
// The last case records the asymmetry the detached document's `title` accessor
// creates on purpose: the live document's `document.title` is the host-seeded
// string (js.Options.Title, read back by Runtime.Title for the browser chrome),
// so it answers "" for a page whose <head> has a <title>. Deriving it from the
// title element would fight that host path, and the sweep found one read of it
// across the five saved pages, inside a third-party snippet and not on a boot
// path, so it stays as it is - named in the task doc.
func TestDocumentImplementationOmitsUnimplementedMembers(t *testing.T) {
	rt := detachedRuntime(t, `<!DOCTYPE html><html><head><title>Main</title></head><body></body></html>`)
	got := evalString(t, rt, `[
		Object.keys(document.implementation).join(","),
		typeof document.implementation.createDocument,
		typeof document.implementation.hasFeature,
		typeof document.implementation.createHTMLDocument,
		String(typeof document.title) + "," + String(document.title)
	].join("|")`)
	want := "createHTMLDocument|undefined|undefined|function|string,"
	if got != want {
		t.Errorf("implementation members: got %q want %q", got, want)
	}
}

// createElement on the live document still allocates into the live tree, which
// is what the live id/tag indexes search, and a node made in a scratch document
// is not reachable from it. The two documents coexist without merging.
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
