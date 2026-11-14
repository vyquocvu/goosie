package layout

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vyquocvu/goosie/internal/style"
)

type bidiRun struct {
	Start, End int
	Level      int
}

type bidiDir uint8

const (
	dirNeutral bidiDir = iota
	dirLTR
	dirRTL
)

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

func isBidiRTLChar(r rune) bool {
	switch {
	case r >= 0x0590 && r <= 0x05FF, r >= 0xFB1D && r <= 0xFB4F,
		r >= 0x0600 && r <= 0x06FF, r >= 0x0750 && r <= 0x077F,
		r >= 0xFB50 && r <= 0xFDFF, r >= 0xFE70 && r <= 0xFEFF:
		return true
	}
	return false
}

func isBidiENChar(r rune) bool {
	return (r >= 0x0030 && r <= 0x0039) || (r >= 0xFF10 && r <= 0xFF19)
}

func isBidiANChar(r rune) bool {
	return (r >= 0x0660 && r <= 0x0669) || (r >= 0x06F0 && r <= 0x06F9)
}

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

func isBidiStrongRTL(r rune) bool {
	_, d := bidiCharLevel(r, 0)
	return d == dirRTL
}

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
			n = 1
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
	for i := range spans {
		s := &spans[i]
		if len(s.glyphIdx) == 0 {
			continue
		}
		obj := a.Get(s.run.obj)
		identity := true
		hasOddLevel := false
		order := make([]int, len(s.glyphIdx))
		objLevels := make([]byte, len(s.glyphIdx))
		for k, logical := range s.glyphIdx {
			order[k] = logical - s.start
			objLevels[k] = byte(levels[logical])
			if objLevels[k]&1 == 1 {
				hasOddLevel = true
			}
			if logical != s.start+k {
				identity = false
			}
		}
		if !identity {
			obj.GlyphOrder = order
		}
		if hasOddLevel {
			obj.BidiLevels = objLevels
		}
	}
	return visual, base == 1
}
