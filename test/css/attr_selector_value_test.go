package css_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

// These are the matching oracles for GOOSIE-CSSSELECT-001: a quoted attribute
// selector value holds CSS escapes, and the string the selector denotes is what
// has to be compared against the DOM. Goosie cut the quote characters off with
// a cutset Trim and then compared the raw bytes, so `[data-x="a\"b"]` looked for
// a literal backslash, matched nothing, and the element kept its unstyled box -
// while `[data-x="a"b"]`, which is a syntax error, cut down to `a"b` and matched
// the very element its own spelling cannot name.
//
// Every expectation below is Chromium's answer for this exact document,
// measured with Playwright on 2026-11-12, including the throws and the shapes
// that parse to nothing. The three scans that had to learn about quotes are the
// condition's closing ']', the comma split of a selector list, and the value's
// own unquoting.

// attrSelDoc is the fixture the oracle tables run against. The attribute values
// are spelled in HTML, where a backslash is an ordinary character, so each one
// is the string the DOM really holds.
func attrSelDoc() *dom.Document {
	return domtest.Parse(`<html><body>` +
		`<div id=n1 data-x='a"b'></div>` +
		`<div id=n2 data-x="a'b"></div>` +
		`<div id=n3 data-x="a'b"></div>` +
		`<div id=n4 data-x='a b'></div>` +
		`<div id=n5 data-x='aAb'></div>` +
		`<div id=n6 data-x='"'></div>` +
		`<div id=n7 data-x='a\b'></div>` +
		`<div id=n8 data-x='a]b'></div>` +
		`<div id=n9 data-x=A></div>` +
		`<div id=n10 data-x='a b'></div>` +
		`<div id=n11 data-x='a\\b'></div>` +
		`<div id=c1 data-x='a,b'></div>` +
		`<div id=c2 data-x='tail]'></div>` +
		`</body></html>`)
}

// attrSelQuery runs one selector list the way the query API does: a rejected
// list yields the error, and an accepted one yields the matching ids in
// document order.
func attrSelQuery(t *testing.T, sel string) (string, error) {
	t.Helper()
	sels, err := css.ParseSelectorList(sel)
	if err != nil {
		return "", err
	}
	var ids []string
	for _, n := range elements(attrSelDoc()) {
		for _, s := range sels {
			if s.Matches(n) {
				ids = append(ids, n.GetAttribute("id"))
				break
			}
		}
	}
	return strings.Join(ids, " "), nil
}

// elements returns every element of the fixture in document order.
func elements(doc *dom.Document) []*dom.Node {
	var out []*dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(&doc.Node)
	return out
}

// qc:BR-268
func TestAttributeSelectorValueIsUnescapedBeforeMatching(t *testing.T) {
	cases := []struct {
		sel  string
		want string
	}{
		{`[data-x="a\"b"]`, "n1"},
		{`[data-x='a\'b']`, "n2 n3"},
		{`[data-x=a\"b]`, "n1"}, // unquoted: the escape decodes the same way
		{`[data-x="a\ b"]`, "n4 n10"},
		{`[data-x="a\41 b"]`, "n5"},  // \41 is 'A', the space terminates it
		{`[data-x=\41]`, "n9"},       // unquoted hex escape
		{`[data-x="\22"]`, "n6"},     // the value is a lone double quote
		{`[data-x="a\\b"]`, "n7"},    // one escaped backslash
		{`[data-x="a b"]`, "n4 n10"}, // control: a plain quoted value still works
		{`[data-x$="]"]`, "c2"},      // ']' inside the value, suffix match
	}
	for _, tc := range cases {
		got, err := attrSelQuery(t, tc.sel)
		if err != nil {
			t.Errorf("%s: query API rejected a legal selector: %v", tc.sel, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s matched [%s], Chromium matches [%s]", tc.sel, got, tc.want)
		}
	}
}

// qc:BR-268
func TestAttributeSelectorValueThatIsAbsentMatchesNothing(t *testing.T) {
	// These are the shapes that parse but name no element: an escape the value
	// does not contain, and a string that never closes, which CSS error recovery
	// ends at the input rather than treating as a selector error. Chromium
	// answers with an empty node list for all four, so a Goosie that made any of
	// them match would be matching a spelling rather than a string.
	cases := []string{
		`[data-x="a\Ab"]`, // '«b': \Ab is one hex escape, not 'A' then 'b'
		`[data-x="\"]`,    // the escaped quote keeps the ']' inside the string
		`[data-x="a]`,     // unterminated string, recovered at end of input
		`[data-x="a\bb"]`, // control: a legal selector for a value nobody holds
	}
	for _, sel := range cases {
		got, err := attrSelQuery(t, sel)
		if err != nil {
			t.Errorf("%s: query API rejected a recoverable selector: %v", sel, err)
			continue
		}
		if got != "" {
			t.Errorf("%s matched [%s], want no element at all", sel, got)
		}
	}
}

// qc:BR-268
func TestAttributeSelectorThrowsForAQuotedValueWithTrailingBytes(t *testing.T) {
	// Chromium throws SyntaxError for `[data-x="a"b"]`: the string closes at the
	// second quote and `b"` is left over, so the selector has no grammar. Goosie
	// used to cut both quotes off that spelling and match the element holding
	// `a"b` - the one element its own text cannot name.
	for _, sel := range []string{`[data-x="a"b"]`, `[data-x='a'b']`, "[data-x=\"a\\\"b\"c]", `[data-x="a"b"], [data-x="a b"]`} {
		if _, err := attrSelQuery(t, sel); err == nil {
			t.Errorf("%s parsed, Chromium throws SyntaxError for it", sel)
		}
	}
}

// qc:BR-268
func TestCommaInsideQuotedAttributeValueIsOneSelector(t *testing.T) {
	// The comma belongs to the value, so the list holds one alternative matching
	// c1, and a two-alternative list keeps both of its selectors intact.
	for _, tc := range []struct{ sel, want string }{
		{`[data-x="a,b"]`, "c1"},
		{`[data-x="a,b"], [data-x="a b"]`, "n4 n10 c1"},
	} {
		got, err := attrSelQuery(t, tc.sel)
		if err != nil {
			t.Errorf("%s: query API rejected a legal selector: %v", tc.sel, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s matched [%s], Chromium matches [%s]", tc.sel, got, tc.want)
		}
	}
}

// qc:BR-268
func TestStylesheetRuleWithCommaInAttributeValueKeepsItsDeclarations(t *testing.T) {
	// The same comma-blind scan sits between a stylesheet and its rules, so this
	// is the shape that loses a declaration rather than a match: the rule arrives
	// as two broken selectors and the element keeps no background at all.
	sheet := css.Parse(`.d[data-x="a,b"]{background:#f00;width:80px;height:20px}` + "\n" +
		`.after{background:#0f0;width:80px;height:20px}` + "\n")
	mustNotLeakCSS(t, sheet)

	if got := strings.Join(selectorStrs(sheet), " "); got != `.d[data-x="a,b"] .after` {
		t.Errorf("sheet compiled selectors [%s], want the two declared rules intact", got)
	}
	for _, sel := range []string{`.d[data-x="a,b"]`, `.after`} {
		if got := len(declarationsFor(t, sheet, sel)); got != 3 {
			t.Errorf("%s kept %d declaration(s), want 3: %+v", sel, got, declarationsFor(t, sheet, sel))
		}
	}
}
