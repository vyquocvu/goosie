package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// `line-height: normal` is not a length. It is the height the face reports for
// the element that owns it, and it inherits as the keyword: Chromium computes
// `normal` on a 64px block that declares it and on the 64px child of a 16px
// block that does, and takes the height from that block's own font. Resolving
// the keyword into a number at the declaration - 1.2 x whatever font size the
// cascade had applied by then - gave a 64px paragraph the 16px body's height
// and made the answer depend on which declaration ran first.
//
// Every expectation below is Chromium's own computed `line-height` string,
// measured with Playwright on 2026-11-12 on this document at an 800x600
// viewport: `.sizeFirst`/`.keywordFirst`/`.splitCascade` all compute `normal`
// (18px and 74px boxes), `.kidOfNormal` computes `normal`, `.lengthChild`
// computes the inherited `50px`, `.shNoLine` computes `normal` where its parent
// says `50px`, and `.shRatio`/`.shLength` compute `48px`/`30px`.
const lineHeightNormalHTML = `<html><body style="font:16px sans-serif">
	<div class="splitCascade" style="font-size:64px">x</div>
	<div class="keywordFirst">x</div>
	<div class="sizeFirst">x</div>
	<div class="normalParent"><span class="kidOfNormal">x</span></div>
	<p class="lengthParent" style="line-height:50px"><span class="lengthChild">x</span></p>
	<p class="ratioParent"><span class="ratioChild">x</span></p>
	<div class="shParent" style="line-height:50px"><span class="shNoLine">a</span><span class="shRatio">b</span><span class="shLength">c</span></div>
</body></html>`

const lineHeightNormalSheet = `.splitCascade { line-height: normal; }
.keywordFirst { line-height: normal; font-size: 64px; }
.sizeFirst { font-size: 64px; line-height: normal; }
.normalParent { line-height: normal; }
.kidOfNormal { font-size: 64px; }
.lengthChild { font-size: 64px; }
.ratioParent { line-height: 1.5; }
.ratioChild { font-size: 64px; }
.shNoLine { font: 32px sans-serif; }
.shRatio { font: 32px/1.5 sans-serif; }
.shLength { font: 32px/30px sans-serif; }`

func lineHeights(t *testing.T) map[string]*style.ComputedStyle {
	t.Helper()
	doc := domtest.Parse(lineHeightNormalHTML)
	sheet := css.ParseForViewport(lineHeightNormalSheet, 800)
	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)
	out := map[string]*style.ComputedStyle{}
	for _, cls := range []string{
		"splitCascade", "keywordFirst", "sizeFirst", "normalParent", "kidOfNormal",
		"lengthChild", "ratioChild", "shNoLine", "shRatio", "shLength",
	} {
		nodes := doc.ElementsByClassName(cls)
		if len(nodes) == 0 {
			t.Fatalf("no .%s node", cls)
		}
		st, ok := styles[nodes[0].ID]
		if !ok || st == nil {
			t.Fatalf("no style for .%s", cls)
		}
		out[cls] = st
	}
	return out
}

// qc:BR-271
func TestNormalLineHeightStaysTheKeywordWhereverItIsDeclared(t *testing.T) {
	got := lineHeights(t)
	// A negative or zero LineHeight is the state the initial value uses: the
	// height comes from the face, per element. Anything else has frozen a
	// number, and which number depends on the font size the cascade had applied
	// when the declaration ran.
	for _, c := range []struct{ cls, why string }{
		// The size arrives from an inline style, so the class's `normal` was
		// resolved while the element's size was still the parent's 16px.
		{"splitCascade", "normal from a class, size from an inline style"},
		// Same block, keyword first: the size had not been applied yet.
		{"keywordFirst", "normal declared before the size"},
		// Same block, size first - the spelling that did resolve to the
		// element's own size, and so answered a different number than its two
		// siblings above.
		{"sizeFirst", "normal declared after the size"},
	} {
		if st := got[c.cls]; st.LineHeight > 0 {
			t.Errorf(".%s (%s): LineHeight = %g, want the normal state (<= 0) so the face decides (Chromium computes `normal`)", c.cls, c.why, st.LineHeight)
		}
	}
}

// qc:BR-271
func TestNormalLineHeightInheritsAsTheKeywordNotAsAnAncestorsNumber(t *testing.T) {
	got := lineHeights(t)
	if st := got["normalParent"]; st.LineHeight > 0 {
		t.Errorf(".normalParent: LineHeight = %g, want the normal state (<= 0)", st.LineHeight)
	}
	// The 64px child of that block has to reach the normal state too. Freezing
	// the keyword turns inheritance into handing down the parent's height, which
	// is what made a 64px paragraph lay out at 19px.
	if st := got["kidOfNormal"]; st.LineHeight > 0 {
		t.Errorf(".kidOfNormal: LineHeight = %g, want the parent's `normal` to inherit as the keyword, not as a number (Chromium computes `normal` on the child)", st.LineHeight)
	}
	// The contrast that keeps the change honest: a declared length is a number
	// and does inherit resolved - Chromium computes `50px` on this child.
	if st := got["lengthChild"]; st.LineHeight != 50 {
		t.Errorf(".lengthChild: LineHeight = %g, want the 50px length inherited resolved (Chromium)", st.LineHeight)
	}
	// And a unitless number still re-derives per element: 1.5 over a 64px child
	// is 96px, measured.
	if st := got["ratioChild"]; st.LineHeight != 96 {
		t.Errorf(".ratioChild: LineHeight = %g, want 96 = 1.5 x this element's own 64px (Chromium)", st.LineHeight)
	}
}

// qc:BR-271
func TestFontShorthandResetsAnInheritedLineHeight(t *testing.T) {
	got := lineHeights(t)
	// The `font` shorthand resets every longhand it leaves out, so a shorthand
	// with no `/line-height` part drops the ancestor's 50px: Chromium computes
	// `normal` and gives the span a 37px box where 50px would have been.
	if st := got["shNoLine"]; st.LineHeight > 0 {
		t.Errorf(".shNoLine: LineHeight = %g, want the normal state (<= 0) because the shorthand resets the inherited 50px (Chromium computes `normal`)", st.LineHeight)
	}
	// The forms the shorthand does carry resolve against its own size: 1.5 x 32
	// and 30px, both measured as computed values.
	if st := got["shRatio"]; st.LineHeight != 48 {
		t.Errorf(".shRatio: LineHeight = %g, want 48 = 1.5 x 32px (Chromium)", st.LineHeight)
	}
	if st := got["shLength"]; st.LineHeight != 30 {
		t.Errorf(".shLength: LineHeight = %g, want the 30px the shorthand carries (Chromium)", st.LineHeight)
	}
}
