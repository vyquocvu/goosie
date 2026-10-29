package layout

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vyquocvu/goosie/internal/style"
)

// bidiRun is one same-level stretch of text, in rune offsets into the
// original string. Level 0 is LTR and 1 is RTL; a full UAX #9 embedding
// would raise the level for nested embeds, which this subset does not do.
type bidiRun struct {
	Start, End int // rune offsets into the original text
	Level      int // 0=LTR, 1=RTL (higher for embedded)
}

// resolveBidi splits text into maximal same-level runs given a base level
// (0 = LTR, 1 = RTL). With override (unicode-bidi: bidi-override) every
// rune is forced to the base level, so pure text collapses to a single run.
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

// bidiLevels classifies every rune of text into a bidi level. Neutrals
// (spaces, punctuation) take the base level; a full UAX #9 would resolve
// them against surrounding strong types first, which is why "abc 123" in
// an RTL paragraph splits into separate runs around each space.
func bidiLevels(text string, baseLevel int, override bool) []int {
	runes := []rune(text)
	levels := make([]int, len(runes))
	for i, r := range runes {
		if override {
			levels[i] = baseLevel
			continue
		}
		levels[i] = bidiCharLevel(r, baseLevel)
	}
	return levels
}

// bidiCharLevel resolves one rune: strong RTL letters and Arabic-Indic
// digits ride at level 1, strong LTR letters and European digits at level
// 0 even when the base level is RTL, everything else takes the base.
func bidiCharLevel(r rune, base int) int {
	switch {
	case isBidiANChar(r), isBidiRTLChar(r):
		return 1
	case isBidiENChar(r), unicode.IsLetter(r):
		return 0
	default:
		return base
	}
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

// plaintextBaseLevel picks the paragraph level the way unicode-bidi:
// plaintext does (UAX #9 P2/P3): the direction of the first strong
// directional character, digits and neutrals skipped, falling back to
// baseLevel when the text has no strong character at all.
func plaintextBaseLevel(text string, baseLevel int) int {
	for _, r := range text {
		switch {
		case isBidiANChar(r), isBidiENChar(r):
			continue
		case isBidiRTLChar(r):
			return 1
		case unicode.IsLetter(r):
			return 0
		}
	}
	return baseLevel
}

// bidiLineRuns reorders one line's word runs into the sequence the RTL
// placement loop must walk, and reports whether that loop still applies.
//
// The placement loop puts each run it visits to the left of the previous
// one, so it needs the mirror of the visual order. The visual order keeps
// maximal level-0 stretches (Latin words, digits) in logical left-to-right
// order and reverses each maximal level-1 stretch with the RTL flow. That
// is a deliberate simplification of UAX #9 L2: an LTR stretch keeps its
// logical position instead of being re-embedded one level deeper, so "hello
// שלום" in an RTL paragraph puts the Hebrew on the right rather than the
// left. It is also only a word-level reorder — the glyphs inside a single
// run are still painted in logical order, so a Hebrew word's letters render
// mirrored until the paint path learns to shape RTL runs.
//
// The second return is false when the line's paragraph level resolves to
// LTR (unicode-bidi: plaintext with LTR-first content) and the caller must
// place the runs with the ordinary left-to-right loop.
func bidiLineRuns(st *style.ComputedStyle, runs []textRun) ([]textRun, bool) {
	base := 1
	override := false
	plaintext := false
	if st != nil {
		override = st.UnicodeBidi == "bidi-override"
		plaintext = st.UnicodeBidi == "plaintext"
	}
	var sb strings.Builder
	for i := range runs {
		if !runs[i].atomic {
			sb.WriteString(runs[i].text)
		}
	}
	joined := sb.String()
	if plaintext {
		base = plaintextBaseLevel(joined, base)
		if base == 0 {
			return runs, false
		}
	}
	levels := bidiLevels(joined, base, override)
	runLevel := make([]int, len(runs))
	off := 0
	for i := range runs {
		if runs[i].atomic || off >= len(levels) {
			// An atomic box carries no text of its own; it flows with the
			// base direction the way a neutral would.
			runLevel[i] = base
			continue
		}
		runLevel[i] = levels[off]
		off += utf8.RuneCountInString(runs[i].text)
	}
	// Visual order: walk the runs logically, reversing each maximal
	// level-1 stretch, then mirror the whole sequence for the placement
	// loop.
	visual := make([]textRun, 0, len(runs))
	for i := 0; i < len(runs); {
		j := i + 1
		for j < len(runs) && runLevel[j] == runLevel[i] {
			j++
		}
		if runLevel[i] == 0 {
			visual = append(visual, runs[i:j]...)
		} else {
			for k := j - 1; k >= i; k-- {
				visual = append(visual, runs[k])
			}
		}
		i = j
	}
	out := make([]textRun, len(visual))
	for k, r := range visual {
		out[len(visual)-1-k] = r
	}
	return out, true
}
