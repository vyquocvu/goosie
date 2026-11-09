package engine_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
)

// BR-087, second half: the visual order of an RTL line.
//
// Every expectation here is the geometry Chromium 1.63 gave the same fixture in
// scripts/verify-br087-bidi.js (2026-11-04). Its content box is x=600..800, so
// subtract 600 to compare against these boxes at x=0..200. The quoted numbers
// are in docs/roadmap-v2.md.
//
//	AR "مرحبا" x=[793.688 785.859 777.391 773.484 769.813] descending=true
//	LA "abc"   x=[708.438 717.328 726.234] ascending=true
//	EN "123"   x=[738.672 747.578 756.469] ascending=true
//	visual     ="abc 123 ابحرم"  right-anchored=true (rightmost edge 800.001)
//
// Goosie before this slice painted every word's runes left to right from a
// left-anchored line - "glyphs-painted: م@4 ر@9 ح@17 ب@26 ا@38" for the same run -
// and ordered the words as "مرحبا abc 123".
//
// What the oracles read is the painted glyph list, because that is what the
// rasterizer places: each entry carries the rune it draws and the x it draws it
// at. Sorting a run by x gives the string the line actually reads, which is how
// the reference reports it above; the slice's own order is not asserted, since
// nothing downstream depends on it.
//
// Not covered here, and recorded as defects: mirroring of bracket glyphs (only
// their order is asserted), nested embeds and isolates below a single line, and
// where a caret or selection highlight lands inside a reordered word.

// bidiWords returns the laid-out word fragments of one box, left to right as
// they sit on the line, skipping the collapsible spaces that carry no ink.
func bidiWords(t *testing.T, sess *engine.Session) []string {
	t.Helper()
	var out []string
	for i := range sess.Arena.Objects {
		o := &sess.Arena.Objects[i]
		if o.Node == nil || o.Node.Type != dom.NodeText {
			continue
		}
		if strings.TrimSpace(o.Node.DataContent) == "" {
			continue
		}
		out = append(out, o.Node.DataContent)
	}
	sort.SliceStable(out, func(i, j int) bool { return wordX(t, sess, out[i]) < wordX(t, sess, out[j]) })
	return out
}

// wordX returns the left edge of the fragment carrying that word.
func wordX(t *testing.T, sess *engine.Session, word string) float32 {
	t.Helper()
	return wordBox(t, sess, word).X
}

// wordBox returns the laid-out box carrying that word. A word that appears more
// than once is resolved to its leftmost box, which is the one the caller can
// name unambiguously in a single-line fixture.
func wordBox(t *testing.T, sess *engine.Session, word string) *layout.Object {
	t.Helper()
	var best *layout.Object
	for i := range sess.Arena.Objects {
		o := &sess.Arena.Objects[i]
		if o.Node == nil || o.Node.Type != dom.NodeText || o.Node.DataContent != word {
			continue
		}
		if best == nil || o.X < best.X {
			best = o
		}
	}
	if best == nil {
		t.Fatalf("no laid-out fragment carries %q", word)
	}
	return best
}

// lineEdges returns the left edge of the leftmost word and the right edge of
// the rightmost word, in CSS px inside the box's own coordinate space.
func lineEdges(t *testing.T, sess *engine.Session) (left, right float32) {
	t.Helper()
	left, right = 1e9, -1e9
	for i := range sess.Arena.Objects {
		o := &sess.Arena.Objects[i]
		if o.Node == nil || o.Node.Type != dom.NodeText || strings.TrimSpace(o.Node.DataContent) == "" {
			continue
		}
		if o.X < left {
			left = o.X
		}
		if o.X+o.W > right {
			right = o.X + o.W
		}
	}
	return left, right
}

// bidiGlyphRuns returns one slice per painted text run, in the order the
// builder emitted them.
func bidiGlyphRuns(t *testing.T, sess *engine.Session) [][]paint.GlyphRun {
	t.Helper()
	var out [][]paint.GlyphRun
	for _, cmd := range sess.Paint(1).Build(1).All() {
		if cmd.Kind == paint.CmdText && len(cmd.Text.Glyphs) > 0 {
			out = append(out, cmd.Text.Glyphs)
		}
	}
	return out
}

// runFor finds the painted run carrying the same runes as word. It matches on
// the rune set rather than the order, because the order is exactly what these
// tests are about.
func runFor(t *testing.T, runs [][]paint.GlyphRun, word string) []paint.GlyphRun {
	t.Helper()
	want := []rune(word)
	for _, r := range runs {
		if len(r) == len(want) && sameRunes(r, want) {
			return r
		}
	}
	t.Fatalf("no painted run carries the runes of %q; runs = %v", word, summarize(runs))
	return nil
}

func sameRunes(glyphs []paint.GlyphRun, want []rune) bool {
	got := make([]rune, 0, len(glyphs))
	for _, g := range glyphs {
		got = append(got, g.Rune)
	}
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// visualRunes reads a run the way its ink reads: the runes sorted by the x each
// one is placed at. Chromium's anchor reports the same string.
func visualRunes(glyphs []paint.GlyphRun) string {
	cp := append([]paint.GlyphRun(nil), glyphs...)
	sort.SliceStable(cp, func(i, j int) bool { return cp[i].X < cp[j].X })
	var sb strings.Builder
	for _, g := range cp {
		sb.WriteRune(g.Rune)
	}
	return sb.String()
}

// xsOfVisual lists a run's glyph x values in reading order.
func xsOfVisual(glyphs []paint.GlyphRun) []int {
	cp := append([]paint.GlyphRun(nil), glyphs...)
	sort.SliceStable(cp, func(i, j int) bool { return cp[i].X < cp[j].X })
	out := make([]int, 0, len(cp))
	for _, g := range cp {
		out = append(out, int(g.X))
	}
	return out
}

func summarize(runs [][]paint.GlyphRun) string {
	var parts []string
	for _, r := range runs {
		parts = append(parts, visualRunes(r))
	}
	return strings.Join(parts, " | ")
}

func strictlyAscending(xs []int) bool {
	for i := 1; i < len(xs); i++ {
		if xs[i] <= xs[i-1] {
			return false
		}
	}
	return true
}

// assertDrawnAdvances checks that each visual slot sits where the runes drawn
// before it, by their own advances, put it. A reordered word drawn with the
// logical rune's advance instead drifts several pixels by its tail: Chromium
// measures ا=769.813+3.672 then ب=773.484, and the runes of مرحبا run from 3.7
// to 8.5px wide.
func assertDrawnAdvances(t *testing.T, sess *engine.Session, word string, glyphs []paint.GlyphRun) {
	t.Helper()
	box := wordBox(t, sess, word)
	cp := append([]paint.GlyphRun(nil), glyphs...)
	sort.SliceStable(cp, func(i, j int) bool { return cp[i].X < cp[j].X })
	pen := int32(0)
	for k, g := range cp {
		want := int32(box.X) + (pen+32)>>6
		if d := g.X - want; d > 1 || d < -1 {
			t.Errorf("slot %d (%q) of %q lands at x=%d, want %d (delta %d): the pen must advance by the rune it draws", k, string(g.Rune), word, g.X, want, d)
		}
		pen += sess.Arena.Metrics.GlyphAdvanceFixed(g.Size, g.Rune, g.Slot)
	}
}

// Chromium: dir=rtl, 200px box, "مرحبا" alone - the word is the whole line, its
// first rune sits rightmost (م=793.688, ا=769.813) and the line ends at the
// content-box right edge (rightmost edge x=800.001).
// qc:BR-087
func TestRtlArabicWordPaintsRightToLeftAndAnchorsAtTheRightEdge(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t), `<div style="width:200px" dir="rtl">مرحبا</div>`)
	glyphs := runFor(t, bidiGlyphRuns(t, sess), "مرحبا")
	if got := visualRunes(glyphs); got != "ابحرم" {
		t.Errorf("the line reads %q, want %q: Chromium's visual order for this run is ابحرم (م=793.688 … ا=769.813, descending)", got, "ابحرم")
	}
	if xs := xsOfVisual(glyphs); !strictlyAscending(xs) {
		t.Errorf("placed glyph x = %v, want each rune left of the next in reading order", xs)
	}
	assertDrawnAdvances(t, sess, "مرحبا", glyphs)
	_, right := lineEdges(t, sess)
	if d := 200 - right; d > 2 || d < -2 {
		t.Errorf("line right edge = %v, want ~200 (Chromium 800.001 in a 600..800 content box); delta %v", right, d)
	}
}

// Chromium: dir=rtl "مرحبا abc 123" gives visual ="abc 123 ابحرم": the Latin and
// the digits are one ascending island to the LEFT of the reversed Arabic,
// because the space between two left-to-right types joins them.
// qc:BR-087
func TestRtlMixedLineOrdersWordsByVisualIslands(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t), `<div style="width:200px" dir="rtl">مرحبا abc 123</div>`)
	got := bidiWords(t, sess)
	want := []string{"abc", "123", "مرحبا"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("visual word order = %v, want %v (Chromium: abc=708.438 123=738.672 مرحبا=769.813..793.688)", got, want)
	}
	runs := bidiGlyphRuns(t, sess)
	arabic := runFor(t, runs, "مرحبا")
	if got := visualRunes(arabic); got != "ابحرم" {
		t.Errorf("the Arabic word reads %q, want %q", got, "ابحرم")
	}
	assertDrawnAdvances(t, sess, "مرحبا", arabic)
	for _, w := range []string{"abc", "123"} {
		if got := visualRunes(runFor(t, runs, w)); got != w {
			t.Errorf("the %q island reads %q, want %q (Chromium a=708.438 b=717.328 c=726.234, 1=738.672 2=747.578 3=756.469)", w, got, w)
		}
	}
}

// Chromium: dir=rtl "مرحبا 123 ٤٥٦ abc" gives visual ="abc ٤٥٦ 123 ابحرم" with
// every numeric and Latin island ascending (abc=678.719 ٤٥٦=708.969
// 123=738.672). The three islands swap order while each keeps its own, because
// a neutral between two different directions takes the base level.
// qc:BR-087
func TestRtlEnAndAnIslandsReverseAsIslands(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t), `<div style="width:200px" dir="rtl">مرحبا 123 ٤٥٦ abc</div>`)
	got := bidiWords(t, sess)
	want := []string{"abc", "٤٥٦", "123", "مرحبا"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("visual word order = %v, want %v (Chromium: abc=678.719 ٤٥٦=708.969 123=738.672 Arabic rightmost)", got, want)
	}
	runs := bidiGlyphRuns(t, sess)
	for _, w := range []string{"abc", "123", "٤٥٦"} {
		if got := visualRunes(runFor(t, runs, w)); got != w {
			t.Errorf("the %q island reads %q, want %q: Arabic-Indic digits display left to right (٤=708.969 ٥=717.375 ٦=725.797) even though they are right-to-left for the neutrals around them", w, got, w)
		}
	}
	if got := visualRunes(runFor(t, runs, "مرحبا")); got != "ابحرم" {
		t.Errorf("the Arabic word reads %q, want %q", got, "ابحرم")
	}
}

// Chromium: dir=rtl with only Latin ("abc def") keeps the logical order
// (visual==logical=true, a=747.516 … f=795.547) and only moves the anchor to the
// right edge. A reorder rule must not reverse what it does not understand.
// qc:BR-087
func TestRtlPureLatinKeepsLogicalOrderAndAnchorsRight(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t), `<div style="width:200px" dir="rtl">abc def</div>`)
	got := bidiWords(t, sess)
	if strings.Join(got, " ") != "abc def" {
		t.Errorf("visual word order = %v, want [abc def] (Chromium: visual==logical for an all-LTR line)", got)
	}
	runs := bidiGlyphRuns(t, sess)
	for _, w := range []string{"abc", "def"} {
		glyphs := runFor(t, runs, w)
		if got := visualRunes(glyphs); got != w {
			t.Errorf("the %q word reads %q, want %q", w, got, w)
		}
		assertDrawnAdvances(t, sess, w, glyphs)
	}
	_, right := lineEdges(t, sess)
	if d := 200 - right; d > 2 || d < -2 {
		t.Errorf("right edge of the last word = %v, want ~200 (Chromium rightmost edge 800 in a 600..800 box)", right)
	}
}

// Chromium: dir=rtl "שלום مرحبا" reverses BOTH scripts and puts the logical
// first word rightmost: Hebrew at 767.953..788.875, Arabic at 733.328..757.203.
// qc:BR-087
func TestRtlHebrewAndArabicBothReverse(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t), `<div style="width:200px" dir="rtl">שלום مرحبا</div>`)
	got := bidiWords(t, sess)
	want := []string{"مرحبا", "שלום"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("visual word order = %v, want %v (Hebrew is logically first, so it is rightmost)", got, want)
	}
	runs := bidiGlyphRuns(t, sess)
	for _, c := range []struct{ logical, visual string }{{"שלום", "םולש"}, {"مرحبا", "ابحرم"}} {
		glyphs := runFor(t, runs, c.logical)
		if got := visualRunes(glyphs); got != c.visual {
			t.Errorf("%q reads %q, want %q: both scripts are level-1 runs and reverse", c.logical, got, c.visual)
		}
		assertDrawnAdvances(t, sess, c.logical, glyphs)
	}
}

// Chromium: dir=rtl with unicode-bidi: bidi-override reverses the Latin too -
// a=756.469 b=747.578 c=739.578, visual ="cba ابحرم". Override is what proves
// the reversal is level-driven rather than script-driven.
// qc:BR-087
func TestRtlBidiOverrideReversesLatinToo(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t),
		`<div style="width:200px; unicode-bidi: bidi-override" dir="rtl">مرحبا abc</div>`)
	got := bidiWords(t, sess)
	want := []string{"abc", "مرحبا"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("visual word order = %v, want %v (Chromium visual=\"cba ابحرم\")", got, want)
	}
	runs := bidiGlyphRuns(t, sess)
	if got := visualRunes(runFor(t, runs, "abc")); got != "cba" {
		t.Errorf("the overridden Latin reads %q, want %q: bidi-override forces the base level on every rune (a=756.469 b=747.578 c=739.578)", got, "cba")
	}
	if got := visualRunes(runFor(t, runs, "مرحبا")); got != "ابحرم" {
		t.Errorf("the Arabic word reads %q, want %q", got, "ابحرم")
	}
}

// Chromium: the same string in an LTR box keeps the word order ("مرحبا abc 123",
// leftmost=600) and reverses only the Arabic in place
// (م=623.875 … ا=600, "abc"=634.625…).
// qc:BR-087
func TestLtrArabicWordReversesInPlace(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t), `<div style="width:200px">مرحبا abc 123</div>`)
	got := bidiWords(t, sess)
	want := []string{"مرحبا", "abc", "123"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("visual word order = %v, want %v (an LTR paragraph does not swap its runs)", got, want)
	}
	runs := bidiGlyphRuns(t, sess)
	arabic := runFor(t, runs, "مرحبا")
	if got := visualRunes(arabic); got != "ابحرم" {
		t.Errorf("the Arabic word reads %q, want %q even in an LTR line", got, "ابحرم")
	}
	assertDrawnAdvances(t, sess, "مرحبا", arabic)
	left, _ := lineEdges(t, sess)
	if left > 2 {
		t.Errorf("the line starts at x=%v, want the content-box left edge (Chromium leftmost 600 in a 600..800 box)", left)
	}
}

// Chromium: dir=rtl "(abc)" gives visual =")abc(" - (=794.672 (rightmost),
// a=768.875, b=777.766, c=786.672, )=763.547 (leftmost). The parentheses are
// neutrals at the base level, so they ride the whole-line reversal while the
// Latin island keeps its order. Only the order is asserted: mirroring the glyph
// shapes is a separate defect.
// qc:BR-087
func TestRtlParenthesesWrapTheLatinIsland(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t), `<div style="width:200px" dir="rtl">(abc)</div>`)
	glyphs := runFor(t, bidiGlyphRuns(t, sess), "(abc)")
	if got := visualRunes(glyphs); got != ")abc(" {
		t.Errorf("the line reads %q, want %q (Chromium: )=763.547 a=768.875 b=777.766 c=786.672 (=794.672)", got, ")abc(")
	}
	assertDrawnAdvances(t, sess, "(abc)", glyphs)
}

// Chromium: text-align moves the anchor, never the order. dir=rtl with
// text-align:left gives leftmost x=600, center gives 669.781..730.204 (equal
// 69.8px gaps), right gives the same geometry as the default.
// qc:BR-087
func TestRtlTextAlignMovesTheAnchorNotTheOrder(t *testing.T) {
	for _, c := range []struct{ align, leftmost, rightmost string }{
		{"left", "abc", "مرحبا"},
		{"center", "abc", "مرحبا"},
		{"right", "abc", "مرحبا"},
	} {
		sess := cjkSession(t, cjkFonts(t), `<div style="width:200px; text-align:`+c.align+`" dir="rtl">مرحبا abc</div>`)
		got := bidiWords(t, sess)
		if strings.Join(got, " ") != strings.Join([]string{c.leftmost, c.rightmost}, " ") {
			t.Errorf("align %s: visual word order = %v, want [%s %s] (Chromium reverses nothing here)", c.align, got, c.leftmost, c.rightmost)
		}
		if g := visualRunes(runFor(t, bidiGlyphRuns(t, sess), "مرحبا")); g != "ابحرم" {
			t.Errorf("align %s: the Arabic word reads %q, want %q", c.align, g, "ابحرم")
		}
		left, right := lineEdges(t, sess)
		switch c.align {
		case "left":
			if left > 2 {
				t.Errorf("align left: leftmost fragment x = %v, want ~0 (Chromium 600 in a 600..800 box)", left)
			}
		case "right":
			if d := 200 - right; d > 2 || d < -2 {
				t.Errorf("align right: rightmost fragment edge = %v, want ~200 (Chromium 800.001)", right)
			}
		case "center":
			if d := (200 - right) - left; d > 2 || d < -2 {
				t.Errorf("align center: free space is %v on the left and %v on the right, want equal (Chromium 69.781 / 69.796)", left, 200-right)
			}
		}
	}
}

// Chromium: dir=rtl with unicode-bidi: plaintext and Latin first resolves the
// paragraph to LTR - leftmost x=600, "abc" ascending at 600..617.797, the
// Arabic reversed at 630.25..654.125.
// qc:BR-087
func TestPlaintextResolvesToLtrAndAnchorsLeft(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t),
		`<div style="width:200px; unicode-bidi: plaintext" dir="rtl">abc مرحبا</div>`)
	got := bidiWords(t, sess)
	if strings.Join(got, " ") != "abc مرحبا" {
		t.Errorf("visual word order = %v, want [abc مرحبا] (a plaintext line whose first strong character is Latin is LTR)", got)
	}
	left, _ := lineEdges(t, sess)
	if left > 2 {
		t.Errorf("the line starts at x=%v, want the content-box left edge (Chromium a=600)", left)
	}
	if g := visualRunes(runFor(t, bidiGlyphRuns(t, sess), "مرحبا")); g != "ابحرم" {
		t.Errorf("the Arabic word reads %q, want %q inside an LTR-resolved line", g, "ابحرم")
	}
}

// Chromium: dir=rtl selection within an RTL word must map through the glyph
// permutation. The full-word selection should return the complete logical text.
// qc:BR-087
func TestBidiSelectionRtlFullWordReturnsLogicalText(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t), `<div style="width:200px" dir="rtl">مرحبا</div>`)
	box := wordBox(t, sess, "مرحبا")
	x0, y0, x1, y1 := box.BorderRect()
	midY := (y0 + y1) / 2

	// Select from before the word to after the word (full selection).
	sess.SelectAt(x0-10, midY)
	sess.SelectTo(x1+10, midY)
	if !sess.HasSelection() {
		t.Fatal("full-word selection should be active")
	}
	selText := sess.SelectionText()
	if selText != "مرحبا" {
		t.Errorf("full-word selection = %q, want %q (all logical runes in document order)", selText, "مرحبا")
	}
}

// Chromium anchors an RTL line on the content box, not the border box: with
// 30px of inline padding the last glyph's right edge is 770.001 in a box whose
// border box ends at 800 (scripts/verify-br087-bidi.js u_padding).
// qc:BR-087
func TestRtlStartAnchorSitsInsideThePadding(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t), `<div style="width:200px; padding:0 30px" dir="rtl">مرحبا abc</div>`)
	left, right := lineEdges(t, sess)
	if d := 230 - right; d > 2 || d < -2 {
		t.Errorf("right edge of the line = %v, want ~230 (Chromium 770.001 inside the content box 630..770 of a 200px box with 30px padding each side)", right)
	}
	if left < 30 {
		t.Errorf("left edge of the line = %v, want it inside the 30px padding (content box starts at x=30)", left)
	}
}
