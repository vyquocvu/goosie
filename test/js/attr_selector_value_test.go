package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

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
	want := `["q","apos","one","space","hex","quoteonly","bracket","comma","q comma",` +
		`"THROW SyntaxError","T","F","F","F","T"]`
	if got != want {
		t.Errorf("query API returned\n  %v\nChromium returns\n  %s", got, want)
	}
}
