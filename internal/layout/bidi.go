package layout

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vyquocvu/goosie/internal/style"
)

// bidiRun is one same-level stretch of text, in rune offsets into the
// original string. Level 0 is LTR, 1 is RTL and 2 is an LTR embed inside an
// RTL line; a full UAX #9 would raise the level further for nested embeds,
// which this subset does not do.
type bidiRun struct {
	Start, End int // rune offsets into the original text
	Level      int // 0, 1 or 2
}

// bidiDir is what a neutral is resolved against: the direction of the strong
// text surrounding it.
type bidiDir uint8

const (
	dirNeutral bidiDir = iota
	dirLTR
	dirRTL
)

// resolveBidi splits text into maximal same-level runs given a base level
// (0 = LTR, 1 = RTL). With override (unicode-bidi: bidi-override) every rune
// is forced to the base level, so pure text collapses to a single run.
// The runs come back in logical order and tile the whole text.
func resolveBidi(text string, baseLevel int, override bool) []bidiRun {
	levels := bidiLevels(text, baseLevel, override)
	if len(levels) == 0 {
		return nil
	}
	runs := make([]bidiRun, 0, 4)
	start := 0
	for i := 1; i <= len(levels); i++ {
		if i == len(levels) || levels[i] != levels[start] {
			runs = append(runs, bidiRun{Start: start, End: i, Level: levels[start]})
			start = i
		}
	}
	return runs
}

// bidiLevels classifies every rune of text into a resolved level. Strong types
// take the lowest level of their own parity at or above the base; a neutral
// (space, punctuation, symbol) joins the strong direction on both sides when
// those agree and falls back to the base level when they differ or one side is
// missing. That is the rule the reference answers with: Chromium put "abc 123"
// in one left-to-right island inside an RTL line (a=708.438 … 3=756.469) but
// kept "123", "٤٥٦" and "abc" as three islands in the reversed order
// (abc=678.719 ٤٥٦=708.969 123=738.672), because Arabic-Indic digits are
// right-to-left for neutral resolution even though they display ascending.
func bidiLevels(text string, baseLevel int, override bool) []int {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	levels := make([]int, len(runes))
	dirs := make([]bidiDir, len(runes))
	if override {
		for i := range levels {
			levels[i] = baseLevel
			dirs[i] = dirNeutral
		}
		return levels
	}
	for i, r := range runes {
		levels[i], dirs[i] = bidiCharLevel(r, baseLevel)
	}
	// Neutrals: one pass over each maximal neutral sequence, resolved against
	// the nearest strong rune on either side. N2 (a sequence touching the line
	// boundary) is deliberately the base level here, which is what the
	// reference does for "(abc)" in an RTL line: (=794.672 and )=763.547 ride
	// the whole-line reversal rather than joining the Latin island.
	for i := 0; i < len(runes); {
		if dirs[i] != dirNeutral {
			i++
			continue
		}
		j := i
		for j < len(runes) && dirs[j] == dirNeutral {
			j++
		}
		left, right := dirNeutral, dirNeutral
		if i > 0 {
			left = dirs[i-1]
		}
		if j < len(runes) {
			right = dirs[j]
		}
		d := dirNeutral
		if left != dirNeutral && left == right {
			d = left
		}
		for k := i; k < j; k++ {
			if d == dirLTR {
				levels[k] = evenLevelAtLeast(baseLevel)
			} else if d == dirRTL {
				levels[k] = oddLevelAtLeast(baseLevel)
			} else {
				levels[k] = baseLevel
			}
		}
		i = j
	}
	return levels
}

// bidiCharLevel resolves one rune to its level and its direction for the
// neutral rules. Arabic and Hebrew letters are right-to-left; Latin letters
// and European digits are left-to-right; Arabic-Indic digits display
// left-to-right (Chromium measures ٤=708.969 ٥=717.375 ٦=725.797 in an RTL
// line) but are right-to-left for the neutrals around them.
func bidiCharLevel(r rune, base int) (int, bidiDir) {
	switch {
	case isBidiANChar(r):
		return evenLevelAtLeast(base), dirRTL
	case isBidiRTLChar(r):
		return oddLevelAtLeast(base), dirRTL
	case isBidiENChar(r):
		return evenLevelAtLeast(base), dirLTR
	case unicode.IsLetter(r):
		return evenLevelAtLeast(base), dirLTR
	default:
		return base, dirNeutral
	}
}

func evenLevelAtLeast(base int) int {
	if base%2 == 0 {
		return base
	}
	return base + 1
}

func oddLevelAtLeast(base int) int {
	if base%2 == 1 {
		return base
	}
	return base + 1
}

// isBidiRTLChar covers the Hebrew and Arabic letter blocks and their
// presentation forms. Other RTL scripts (Thaana, Syriac, NKo, ...) fall
// through to the letter fallback and come out LTR — a known gap of the
// range-based subset.
func isBidiRTLChar(r rune) bool {
	switch {
	case r >= 0x0590 && r <= 0x05FF, r >= 0xFB1D && r <= 0xFB4F, // Hebrew
		r >= 0x0600 && r <= 0x06FF, r >= 0x0750 && r <= 0x077F, // Arabic
		r >= 0xFB50 && r <= 0xFDFF, r >= 0xFE70 && r <= 0xFEFF:
		return true
	}
	return false
}

// isBidiENChar covers European digits including the full-width forms.
func isBidiENChar(r rune) bool {
	return (r >= 0x0030 && r <= 0x0039) || (r >= 0xFF10 && r <= 0xFF19)
}

// isBidiANChar covers Arabic-Indic digits.
func isBidiANChar(r rune) bool {
	return (r >= 0x0660 && r <= 0x0669) || (r >= 0x06F0 && r <= 0x06F9)
}

// needsBidi reports whether a line of text can have its order or anchoring
// changed by the bidi rules. Pure left-to-right text - the overwhelming
// majority of the web - never enters the resolver, so the reorder costs
// nothing on those pages.
func needsBidi(style *style.ComputedStyle, runs []textRun) bool {
	if len(runs) == 0 {
		return false
	}
	override := style != nil && (style.UnicodeBidi == "bidi-override" || style.UnicodeBidi == "visual")
	rtl := style != nil && style.Direction == "rtl"
	if override {
		return true
	}
	if rtl {
		for _, r := range runs {
			if !r.atomic && strings.ContainsFunc(r.text, func(c rune) bool { _, d := bidiCharLevel(c, 1); return d != dirLTR }) {
				return true
			}
		}
		return false
	}
	for _, r := range runs {
		if r.atomic {
			continue
		}
		if strings.ContainsFunc(r.text, isBidiStrongRTL) {
			return true
		}
	}
	return false
}

// isBidiStrongRTL reports whether the rune is a right-to-left strong type, so
// an LTR line containing it still needs its glyphs reordered.
func isBidiStrongRTL(r rune) bool {
	_, d := bidiCharLevel(r, 0)
	return d == dirRTL
}

// bidiVisualPermutation returns the visual order of the runes whose levels are
// given: perm[slot] is the logical index drawn at that slot. Reversing each
// maximal sequence of levels at or above h, from the deepest level down to 1,
// is rule L2, and it is what makes the reference put "abc 123" left of a
// reversed Arabic word in one RTL line and ")abc(" for "(abc)".
func bidiVisualPermutation(levels []int) []int {
	n := len(levels)
	perm := make([]int, n)
	for i := range perm {
		perm[i] = i
	}
	highest := 0
	for _, l := range levels {
		if l > highest {
			highest = l
		}
	}
	for h := highest; h >= 1; h-- {
		for i := 0; i < n; {
			if levels[perm[i]] < h {
				i++
				continue
			}
			j := i
			for j < n && levels[perm[j]] >= h {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				perm[a], perm[b] = perm[b], perm[a]
			}
			i = j
		}
	}
	return perm
}

// plaintextBaseLevel picks the paragraph level the way unicode-bidi:
// plaintext does (UAX #9 P2/P3): the direction of the first strong
// directional character, digits and neutrals skipped, falling back to
// baseLevel when the text has no strong character at all.
func plaintextBaseLevel(text string, baseLevel int) int {
	for _, r := range text {
		if isBidiRTLChar(r) {
			return 1
		}
		if unicode.IsLetter(r) {
			return 0
		}
	}
	return baseLevel
}

// bidiReorder resolves the line's bidi levels, stamps each word object with the
// glyph order paint must draw, and returns the line's runs in visual
// left-to-right order together with the direction the line is anchored from.
//
// The two outputs come from one computation on purpose: the reference reverses
// a line's *runs* and its *glyphs* with the same level list, so computing them
// apart is how a line ends up with reversed word order and unreversed words.
// Anchoring and order are separate: a line of only Latin under direction:rtl
// keeps its logical order (Chromium measures "abc def" unchanged) and only
// moves to the right edge.
//
// Not covered, and recorded as defects: bidi mirroring of bracket glyphs,
// nested embeds and isolates below this single line, and the visual-to-logical
// mapping a caret or selection needs inside a reordered word.
func bidiReorder(st *style.ComputedStyle, a *Arena, runs []textRun) ([]textRun, bool) {
	base := 0
	if st != nil && st.Direction == "rtl" {
		base = 1
	}
	override := false
	if st != nil {
		override = st.UnicodeBidi == "bidi-override" || st.UnicodeBidi == "visual"
	}
	var sb strings.Builder
	for i := range runs {
		if runs[i].atomic {
			// An atomic box carries no text of its own. U+FFFC OBJECT
			// REPLACEMENT CHARACTER is a bidi neutral, so the box takes the
			// line's base level and is ordered among the words the way a
			// same-direction run is.
			sb.WriteRune(0xFFFC)
			continue
		}
		sb.WriteString(runs[i].text)
	}
	joined := sb.String()
	if st != nil && st.UnicodeBidi == "plaintext" {
		base = plaintextBaseLevel(joined, base)
	}
	levels := bidiLevels(joined, base, override)
	if len(levels) == 0 {
		return runs, base == 1
	}
	perm := bidiVisualPermutation(levels)

	// Slot ranges each word owns in the logical string, so one pass over the
	// permutation can both order the words and order the glyphs inside each.
	type span struct {
		run      textRun
		start    int
		end      int
		glyphIdx []int
	}
	spans := make([]span, 0, len(runs))
	owner := make([]int, len(levels))
	off := 0
	for _, r := range runs {
		n := utf8.RuneCountInString(r.text)
		if r.atomic {
			n = 1 // the U+FFFC slot standing in for the box
		}
		spans = append(spans, span{run: r, start: off, end: off + n})
		for k := off; k < off+n && k < len(owner); k++ {
			owner[k] = len(spans) - 1
		}
		off += n
	}
	visual := make([]textRun, 0, len(runs))
	ordered := make([]bool, len(spans))
	for _, logical := range perm {
		i := owner[logical]
		spans[i].glyphIdx = append(spans[i].glyphIdx, logical)
		if !ordered[i] {
			ordered[i] = true
			visual = append(visual, spans[i].run)
		}
	}
	// Stamp the glyph order. An identity order is left nil so paint keeps its
	// fast path and no slice is retained for the overwhelmingly common case.
	for i := range spans {
		s := &spans[i]
		if len(s.glyphIdx) == 0 {
			continue
		}
		identity := true
		for k, logical := range s.glyphIdx {
			if logical != s.start+k {
				identity = false
				break
			}
		}
		if identity {
			continue
		}
		order := make([]int, len(s.glyphIdx))
		for k, logical := range s.glyphIdx {
			order[k] = logical - s.start
		}
		a.Get(s.run.obj).GlyphOrder = order
	}
	return visual, base == 1
}
