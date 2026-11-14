package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

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
	for _, c := range []struct{ cls, why string }{
		{"splitCascade", "normal from a class, size from an inline style"},
		{"keywordFirst", "normal declared before the size"},
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
	if st := got["kidOfNormal"]; st.LineHeight > 0 {
		t.Errorf(".kidOfNormal: LineHeight = %g, want the parent's `normal` to inherit as the keyword, not as a number (Chromium computes `normal` on the child)", st.LineHeight)
	}
	if st := got["lengthChild"]; st.LineHeight != 50 {
		t.Errorf(".lengthChild: LineHeight = %g, want the 50px length inherited resolved (Chromium)", st.LineHeight)
	}
	if st := got["ratioChild"]; st.LineHeight != 96 {
		t.Errorf(".ratioChild: LineHeight = %g, want 96 = 1.5 x this element's own 64px (Chromium)", st.LineHeight)
	}
}

// qc:BR-271
func TestFontShorthandResetsAnInheritedLineHeight(t *testing.T) {
	got := lineHeights(t)
	if st := got["shNoLine"]; st.LineHeight > 0 {
		t.Errorf(".shNoLine: LineHeight = %g, want the normal state (<= 0) because the shorthand resets the inherited 50px (Chromium computes `normal`)", st.LineHeight)
	}
	if st := got["shRatio"]; st.LineHeight != 48 {
		t.Errorf(".shRatio: LineHeight = %g, want 48 = 1.5 x 32px (Chromium)", st.LineHeight)
	}
	if st := got["shLength"]; st.LineHeight != 30 {
		t.Errorf(".shLength: LineHeight = %g, want the 30px the shorthand carries (Chromium)", st.LineHeight)
	}
}
