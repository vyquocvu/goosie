package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

// The query API is the surface a page actually touches, and it carries a second
// contract on top of matching: Selectors API 1 makes document.querySelector and
// Element.matches raise a SyntaxError for a selector the grammar cannot express.
// Both halves are oracled here for GOOSIE-CSSSELECT-001, and the expected array
// is Chromium's own output for the same script, measured 2026-11-12.

const attrValueHTML = `<html><body>` +
	`<div id=q data-x='a"b'></div>` +
	`<div id=plain data-x='ab'></div>` +
	`<div id=apos data-x="a'b"></div>` +
	`<div id=one data-x='a\b'></div>` +
	`<div id=two data-x='a\\b'></div>` +
	`<div id=space data-x='a b'></div>` +
	`<div id=hex data-x='aAb'></div>` +
	`<div id=quoteonly data-x='"'></div>` +
	`<div id=bracket data-x='a]b'></div>` +
	`<div id=comma data-x='a,b'></div>` +
	`</body></html>`

const attrValueScript = `
	const out = [];
	const list = (sel) => {
		try { return Array.from(document.querySelectorAll(sel)).map(n => n.id).join(' '); }
		catch (e) { return 'THROW ' + e.name; }
	};
	out.push(list('[data-x="a\\"b"]'));
	out.push(list("[data-x='a\\'b']"));
	out.push(list('[data-x="a\\\\b"]'));
	out.push(list('[data-x="a\\ b"]'));
	out.push(list('[data-x="a\\41 b"]'));
	out.push(list('[data-x="\\22"]'));
	out.push(list('[data-x="a]b"]'));
	out.push(list('[data-x="a,b"]'));
	out.push(list('[data-x="a\\"b"], [data-x="a,b"]'));
	out.push(list('[data-x="a"b"]'));
	const q = document.getElementById('q');
	const two = document.getElementById('two');
	const bool = (v) => (v ? 'T' : 'F');
	out.push(bool(q.matches('[data-x="a\\"b"]')));
	out.push(bool(q.matches('[data-x="ab"]')));
	out.push(bool(two.matches('[data-x="a\\\\b"]')));
	out.push(bool(two.matches('[data-x="a\\b"]')));
	let threw = 'F';
	try { q.matches('[data-x="a"b"]'); } catch (e) { threw = 'T'; }
	out.push(threw);
	JSON.stringify(out);
`

// qc:BR-268
func TestQueryAPIUnescapesQuotedAttributeValues(t *testing.T) {
	rt := newDOMRuntime(t, domtest.Parse(attrValueHTML))
	got, err := rt.Evaluate(attrValueScript)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// Chromium's answer for this script, entry by entry:
	//   0-3  the escaped quote, apostrophe, backslash and space each denote one
	//        string, and each reaches only the element holding it
	//   4-5  \41 is 'A', and \22 is a lone double quote
	//   6-7  ']' and ',' are bytes of the value, not condition or list ends
	//   8    both alternatives of a legal list survive
	//   9    a quoted value with trailing bytes throws
	//   10-13 matches() on the same needles, including the negatives: `two` holds
	//        two backslashes, so `[data-x="a\\b"]` (which denotes one) must not
	//        reach it, and `[data-x="a\b"]` denotes a backspace, not a backslash
	//   14   matches() throws where querySelectorAll throws
	want := `["q","apos","one","space","hex","quoteonly","bracket","comma","q comma",` +
		`"THROW SyntaxError","T","F","F","F","T"]`
	if got != want {
		t.Errorf("query API returned\n  %v\nChromium returns\n  %s", got, want)
	}
}
