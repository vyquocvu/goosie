package engine_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

// The overflowing SHY/ZWSP fixtures Chromium 1.63 produced in
// scripts/verify-br087-text-parity.js (and the dedicated
// scripts/verify-shybreak-geometry.js anchor): 42 a's, the opportunity, 48
// b's, 16px, 200px box. A Latin word with no opportunity stays one line
// (TestLongUnbrokenLatinWordKeepsEveryGlyph).

const (
	shyPrefix = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"       // 42
	shySuffix = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" // 48
)

func shyBody(t *testing.T, inner string) *engine.Session {
	t.Helper()
	return cjkSession(t, cjkFonts(t), `<div style="width: 200px; font-size: 16px">`+inner+`</div>`)
}

func lineText(lines [][]string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.Join(l, "")
	}
	return out
}

func shyPaintedRunes(t *testing.T, sess *engine.Session) string {
	t.Helper()
	var b strings.Builder
	for _, g := range cjkGlyphs(t, sess) {
		b.WriteRune(g.Rune)
	}
	return b.String()
}

func hyphenGlyph(glyphs []paint.GlyphRun) (paint.GlyphRun, bool) {
	for _, g := range glyphs {
		if g.Rune == '-' {
			return g, true
		}
	}
	return paint.GlyphRun{}, false
}

// Chromium broke at the soft hyphen and inked U+002D at the end of the
// overflowing a-run (5.328px in Arial; Goosie's Times hyphen is the same
// substitution, measured from this engine's face). The b-run starts the next
// line. The Range-rect artifact of a zero-width extra line is not a line box.
// qc:BR-087
func TestSoftHyphenWrapsAndPaintsAHyphen(t *testing.T) {
	sess := shyBody(t, shyPrefix+"\u00AD"+shySuffix)
	got := lineText(cjkLines(t, sess))
	if len(got) != 2 {
		t.Fatalf("SHY line count = %d texts=%q, want 2 (Chromium BREAK-shy wraps at U+00AD)", len(got), got)
	}
	if got[0] != shyPrefix+"-" {
		t.Errorf("line 0 = %q, want the 42 a's plus a taken hyphen", got[0])
	}
	if got[1] != shySuffix {
		t.Errorf("line 1 = %q, want the 48 b's", got[1])
	}
	glyphs := cjkGlyphs(t, sess)
	h, ok := hyphenGlyph(glyphs)
	if !ok {
		t.Fatal("painted glyphs have no U+002D: a taken soft hyphen must draw a hyphen")
	}
	if h.X <= 0 {
		t.Errorf("hyphen X = %d, want after the overflowing a-run", h.X)
	}
	for _, g := range glyphs {
		if g.Rune == '\u00AD' {
			t.Fatalf("painted U+00AD at X=%d: the taken opportunity is a hyphen, not an ignorable", g.X)
		}
	}
	if n := utf8.RuneCountInString(shyPaintedRunes(t, sess)); n != 42+1+48 {
		t.Errorf("painted runes = %d, want 91 (42 a's + hyphen + 48 b's)", n)
	}
}

// The same fixture with U+200B is a wrap with no ink: Chromium BREAK-zws
// lines=2, first line ends with the ZWSP, second carries the b-run.
// qc:BR-087
func TestZeroWidthSpaceWrapsWithoutAHyphen(t *testing.T) {
	sess := shyBody(t, shyPrefix+"\u200B"+shySuffix)
	got := lineText(cjkLines(t, sess))
	if len(got) != 2 {
		t.Fatalf("ZWSP line count = %d texts=%q, want 2 (Chromium BREAK-zws)", len(got), got)
	}
	if got[0] != shyPrefix+"\u200B" {
		t.Errorf("line 0 = %q, want the 42 a's closed by U+200B", got[0])
	}
	if got[1] != shySuffix {
		t.Errorf("line 1 = %q, want the 48 b's", got[1])
	}
	if _, ok := hyphenGlyph(cjkGlyphs(t, sess)); ok {
		t.Error("ZWSP wrap painted U+002D: only a taken soft hyphen draws a hyphen")
	}
}

// A short word with a SHY in the middle never wraps, so the SHY stays
// default-ignorable: same width as the letters alone, one fragment, no hyphen
// glyph. Chromium a+SHY = a = 8.906px on the same anchor.
// qc:BR-087
func TestUnusedSoftHyphenStaysInvisibleAndUnsplit(t *testing.T) {
	fonts := cjkFonts(t)
	with := cjkSession(t, fonts, `<div style="width: 200px; font-size: 16px">aa`+"\u00AD"+`bb</div>`)
	without := cjkSession(t, fonts, `<div style="width: 200px; font-size: 16px">aabb</div>`)
	lines := cjkLines(t, with)
	got := lineText(lines)
	if len(got) != 1 {
		t.Fatalf("fitting SHY line count = %d texts=%q, want 1", len(got), got)
	}
	if len(lines[0]) != 1 {
		t.Errorf("fitting SHY fragments = %q, want one box: an unused opportunity must not split the run", lines[0])
	}
	if strings.Contains(got[0], "-") {
		t.Errorf("fitting SHY text = %q, want no hyphen: the opportunity was not taken", got[0])
	}
	if utf8.RuneCountInString(got[0]) != 5 { // aa + SHY + bb
		t.Errorf("fitting SHY runes = %q (%d), want the original 5 including U+00AD", got[0], utf8.RuneCountInString(got[0]))
	}
	if w, n := sessWidth(t, fonts, "aa\u00ADbb"), sessWidth(t, fonts, "aabb"); w != n {
		t.Errorf("width(aa SHY bb)=%v width(aabb)=%v, want equal: unused SHY costs nothing", w, n)
	}
	if _, ok := hyphenGlyph(cjkGlyphs(t, with)); ok {
		t.Error("unused SHY painted U+002D")
	}
	if _, ok := hyphenGlyph(cjkGlyphs(t, without)); ok {
		t.Error("control 'aabb' painted a hyphen")
	}
}

// word-break: keep-all removes CJK opportunities and nothing else: the same
// overflowing Latin+SHY word still wraps at the SHY.
// qc:BR-087
func TestKeepAllDoesNotSuppressSoftHyphen(t *testing.T) {
	inner := `<div style="width: 200px; font-size: 16px; word-break: keep-all">` + shyPrefix + "\u00AD" + shySuffix + `</div>`
	sess := cjkSession(t, cjkFonts(t), inner)
	got := lineText(cjkLines(t, sess))
	if len(got) != 2 || got[0] != shyPrefix+"-" || got[1] != shySuffix {
		t.Fatalf("keep-all SHY lines = %q, want [aaaa…-, bbbb…] (keep-all is CJK-only)", got)
	}
}

// white-space: nowrap / text-wrap: nowrap must not take the opportunity.
// qc:BR-087
func TestNowrapSuppressesSoftHyphen(t *testing.T) {
	inner := `<div style="width: 200px; font-size: 16px; white-space: nowrap">` + shyPrefix + "\u00AD" + shySuffix + `</div>`
	sess := cjkSession(t, cjkFonts(t), inner)
	lines := cjkLines(t, sess)
	got := lineText(lines)
	if len(got) != 1 {
		t.Fatalf("nowrap SHY line count = %d texts=%q, want 1", len(got), got)
	}
	if len(lines[0]) != 1 {
		t.Errorf("nowrap SHY fragments = %q, want one box: nowrap must not even open the opportunity", lines[0])
	}
	if strings.Contains(got[0], "-") {
		t.Errorf("nowrap SHY text = %q, want the unused SHY, not a hyphen", got[0])
	}
}
