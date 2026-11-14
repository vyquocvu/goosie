package css_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

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
		{`[data-x=a\"b]`, "n1"},
		{`[data-x="a\ b"]`, "n4 n10"},
		{`[data-x="a\41 b"]`, "n5"},
		{`[data-x=\41]`, "n9"},
		{`[data-x="\22"]`, "n6"},
		{`[data-x="a\\b"]`, "n7"},
		{`[data-x="a b"]`, "n4 n10"},
		{`[data-x$="]"]`, "c2"},
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
	cases := []string{
		`[data-x="a\Ab"]`,
		`[data-x="\"]`,
		`[data-x="a]`,
		`[data-x="a\bb"]`,
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
	for _, sel := range []string{`[data-x="a"b"]`, `[data-x='a'b']`, "[data-x=\"a\\\"b\"c]", `[data-x="a"b"], [data-x="a b"]`} {
		if _, err := attrSelQuery(t, sel); err == nil {
			t.Errorf("%s parsed, Chromium throws SyntaxError for it", sel)
		}
	}
}

// qc:BR-268
func TestCommaInsideQuotedAttributeValueIsOneSelector(t *testing.T) {
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
