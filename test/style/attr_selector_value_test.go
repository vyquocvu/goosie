package style_test

import (
	"strconv"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// These are the cascade oracles for GOOSIE-CSSSELECT-001: the rule was always in
// the stylesheet, so the style layer is where the loss becomes a missing band of
// colour - the defect measured Chromium computing a 760x24 rgb(142,68,173) band
// where Goosie painted none.
//
// Every sheet declares one rule per attribute value in the fixture, so the cases
// pin three things at once: the string a quoted value denotes, the value that
// spelling must not reach, and the comma and hex-escape shapes the selector-list
// split used to break.

// attrValueColors resolves the fixture cascade and returns each element class's
// computed background, so a case can name the value it expects to be matched.
// The values are written in HTML, where a backslash is an ordinary character, so
// each one is what the DOM really holds.
func attrValueColors(t *testing.T, sheet string) map[string]css.Color {
	t.Helper()
	const html = `<html><body>` +
		`<div class=quote data-x='a"b'></div>` +
		`<div class=plainquote data-x='ab'></div>` +
		`<div class=apostrophe data-x="a'b"></div>` +
		`<div class=onebackslash data-x='a\b'></div>` +
		`<div class=twobackslashes data-x='a\\b'></div>` +
		`<div class=space data-x='a b'></div>` +
		`<div class=comma data-x='a,b'></div>` +
		`<div class=hexescape data-x='aAb'></div>` +
		`</body></html>`
	doc := domtest.Parse(html)
	styles := style.Resolve(doc, []*css.Stylesheet{css.Parse(sheet)}, nil)
	out := map[string]css.Color{}
	for _, cls := range []string{"quote", "plainquote", "apostrophe", "onebackslash", "twobackslashes", "space", "comma", "hexescape"} {
		nodes := doc.ElementsByClassName(cls)
		if len(nodes) != 1 {
			t.Fatalf(".%s resolved to %d element(s), want 1", cls, len(nodes))
		}
		st, ok := styles[nodes[0].ID]
		if !ok {
			t.Fatalf("no computed style for .%s", cls)
		}
		out[cls] = st.BackgroundColor
	}
	return out
}

func attrValueIs(c css.Color, r, g, b int) bool {
	return c.R == uint8(r) && c.G == uint8(g) && c.B == uint8(b) && c.A != 0
}

func attrValueOf(c css.Color) string {
	return strconv.Itoa(int(c.R)) + "," + strconv.Itoa(int(c.G)) + "," +
		strconv.Itoa(int(c.B)) + "," + strconv.Itoa(int(c.A))
}

// qc:BR-268
func TestCascadeAppliesRuleWhoseAttributeValueHoldsAnEscapedQuote(t *testing.T) {
	colors := attrValueColors(t,
		`.quote[data-x="a\"b"]{background:#8e44ad}`+
			`.apostrophe[data-x='a\'b']{background:#f0f}`+
			`.onebackslash[data-x="a\\b"]{background:#0f0}`+
			`.twobackslashes[data-x="a\\\\b"]{background:#00ffff}`)

	// The rule the defect measured: the escape denotes `"`, so this is rgb(142,68,173).
	if !attrValueIs(colors["quote"], 142, 68, 173) {
		t.Errorf(`.quote with [data-x="a\"b"] computed rgba(%s), Chromium computes rgb(142,68,173)`, attrValueOf(colors["quote"]))
	}
	if !attrValueIs(colors["apostrophe"], 255, 0, 255) {
		t.Errorf(`.apostrophe with [data-x='a\\'b'] computed rgba(%s), Chromium computes rgb(255,0,255)`, attrValueOf(colors["apostrophe"]))
	}
	// An escaped backslash denotes one backslash, and two of them denote two -
	// the same selector text must not answer for both values.
	if !attrValueIs(colors["onebackslash"], 0, 255, 0) {
		t.Errorf(`.onebackslash with [data-x="a\\b"] computed rgba(%s), want rgb(0,255,0)`, attrValueOf(colors["onebackslash"]))
	}
	if !attrValueIs(colors["twobackslashes"], 0, 255, 255) {
		t.Errorf(`.twobackslashes with [data-x="a\\\\b"] computed rgba(%s), want rgb(0,255,255)`, attrValueOf(colors["twobackslashes"]))
	}
	// Nothing else gets styled: a match that ignored the escape, or ignored the
	// attribute entirely, would paint these bands too.
	for _, cls := range []string{"plainquote", "space", "comma"} {
		if colors[cls].A != 0 {
			t.Errorf(".%s computed rgba(%s), want no background at all", cls, attrValueOf(colors[cls]))
		}
	}
}

// qc:BR-268
func TestCascadeAppliesRuleWithHexEscapeAndCommaInAttributeValue(t *testing.T) {
	colors := attrValueColors(t,
		`.hexescape[data-x="a\41 b"]{background:#ff7f00}`+
			`.space[data-x="a\ b"]{background:#00ff00}`+
			`.comma[data-x="a,b"]{background:#0000ff}`)

	// \41 is 'A' and the single space after it terminates the escape, so this
	// rule denotes `aAb` - the value the fixture holds.
	if !attrValueIs(colors["hexescape"], 255, 127, 0) {
		t.Errorf(`.hexescape with [data-x="a\41 b"] computed rgba(%s), Chromium computes rgb(255,127,0)`, attrValueOf(colors["hexescape"]))
	}
	// An escaped space is a space, not the end of the selector.
	if !attrValueIs(colors["space"], 0, 255, 0) {
		t.Errorf(`.space with [data-x="a\ b"] computed rgba(%s), Chromium computes rgb(0,255,0)`, attrValueOf(colors["space"]))
	}
	// The comma inside the quotes is one byte of the value, not a selector-list
	// separator: before the fix this sheet arrived as two broken selectors and
	// the element kept no colour at all.
	if !attrValueIs(colors["comma"], 0, 0, 255) {
		t.Errorf(`.comma with [data-x="a,b"] computed rgba(%s), Chromium computes rgb(0,0,255)`, attrValueOf(colors["comma"]))
	}
	// The spelling, not the string: nothing here may reach `ab`.
	if colors["plainquote"].A != 0 {
		t.Errorf(".plainquote computed rgba(%s), want no background at all", attrValueOf(colors["plainquote"]))
	}
}
