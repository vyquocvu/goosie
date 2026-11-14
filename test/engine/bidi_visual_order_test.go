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

func wordX(t *testing.T, sess *engine.Session, word string) float32 {
	t.Helper()
	return wordBox(t, sess, word).X
}

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

func visualRunes(glyphs []paint.GlyphRun) string {
	cp := append([]paint.GlyphRun(nil), glyphs...)
	sort.SliceStable(cp, func(i, j int) bool { return cp[i].X < cp[j].X })
	var sb strings.Builder
	for _, g := range cp {
		sb.WriteRune(g.Rune)
	}
	return sb.String()
}

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

// qc:BR-087
func TestRtlParenthesesWrapTheLatinIsland(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t), `<div style="width:200px" dir="rtl">(abc)</div>`)
	glyphs := runFor(t, bidiGlyphRuns(t, sess), "(abc)")
	if got := visualRunes(glyphs); got != "(abc)" {
		t.Errorf("the line reads %q, want %q (Chromium: bidi-reversed then mirrored brackets)", got, "(abc)")
	}
	assertDrawnAdvances(t, sess, "(abc)", glyphs)
}

// qc:BR-087
func TestBidiMirrorBracketsInRtl(t *testing.T) {
	fonts := cjkFonts(t)
	sess := cjkSession(t, fonts, `<div style="width:200px" dir="rtl">(abc)</div>`)
	runs := bidiGlyphRuns(t, sess)
	var glyphs []paint.GlyphRun
	for _, r := range runs {
		if len(r) > 0 {
			glyphs = r
			break
		}
	}
	if got := visualRunes(glyphs); got != "(abc)" {
		t.Errorf("visual = %q, want %q (mirrored brackets in RTL)", got, "(abc)")
	}
}

// qc:BR-087
func TestBidiNoMirrorInLtr(t *testing.T) {
	fonts := cjkFonts(t)
	sess := cjkSession(t, fonts, `<div style="width:200px">(abc)</div>`)
	runs := bidiGlyphRuns(t, sess)
	var glyphs []paint.GlyphRun
	for _, r := range runs {
		if len(r) > 0 {
			glyphs = r
			break
		}
	}
	if got := visualRunes(glyphs); got != "(abc)" {
		t.Errorf("LTR visual = %q, want %q (no mirroring in LTR context)", got, "(abc)")
	}
}

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

// qc:BR-087
func TestBidiSelectionRtlFullWordReturnsLogicalText(t *testing.T) {
	sess := cjkSession(t, cjkFonts(t), `<div style="width:200px" dir="rtl">مرحبا</div>`)
	box := wordBox(t, sess, "مرحبا")
	x0, y0, x1, y1 := box.BorderRect()
	midY := (y0 + y1) / 2

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
