package layout

import (
	"strings"
	"testing"
)

func spanOf(runs []bidiRun, start, end int) *bidiRun {
	for i := range runs {
		if runs[i].Start <= start && runs[i].End >= end {
			return &runs[i]
		}
	}
	return nil
}

func TestResolveBidiPureLTR(t *testing.T) {
	runs := resolveBidi("hello", 0, false)
	if len(runs) != 1 {
		t.Fatalf("resolveBidi(hello, 0) = %+v, want 1 run", runs)
	}
	if runs[0].Start != 0 || runs[0].End != 5 || runs[0].Level != 0 {
		t.Errorf("run = %+v, want [0,5) at level 0", runs[0])
	}
}

func TestResolveBidiPureRTL(t *testing.T) {
	runs := resolveBidi("שלום", 0, false)
	if len(runs) != 1 {
		t.Fatalf("resolveBidi(שלום, 0) = %+v, want 1 run", runs)
	}
	if runs[0].Start != 0 || runs[0].End != 4 || runs[0].Level != 1 {
		t.Errorf("run = %+v, want [0,4) at level 1", runs[0])
	}
}

func TestResolveBidiMixedBaseRTL(t *testing.T) {
	runs := resolveBidi("hello שלום", 1, false)
	if len(runs) != 2 {
		t.Fatalf("resolveBidi(hello שלום, 1) = %+v, want 2 runs", runs)
	}
	if runs[0].Start != 0 || runs[0].End != 5 || runs[0].Level != 2 {
		t.Errorf("run 0 = %+v, want [0,5) at level 2", runs[0])
	}
	if runs[1].Start != 5 || runs[1].End != 10 || runs[1].Level != 1 {
		t.Errorf("run 1 = %+v, want [5,10) at level 1", runs[1])
	}
}

func TestResolveBidiDigitsStayLTR(t *testing.T) {
	runs := resolveBidi("abc 123 שלום", 1, false)
	digits := spanOf(runs, 4, 7)
	if digits == nil {
		t.Fatalf("resolveBidi(abc 123 שלום, 1) = %+v, no run covers the digits", runs)
	}
	if digits.Level != 2 || digits.End > 7 {
		t.Errorf("digits run = %+v, want a level-2 run ending at or before rune 7", *digits)
	}
	hebrew := spanOf(runs, 8, 12)
	if hebrew == nil || hebrew.Level != 1 {
		t.Errorf("Hebrew tail not in a level-1 run: %+v", runs)
	}
}

func TestBidiLevelsSeparateENFromAN(t *testing.T) {
	levels := bidiLevels("123 ٤٥٦", 1, false)
	want := []int{2, 2, 2, 1, 2, 2, 2}
	for i, w := range want {
		if levels[i] != w {
			t.Errorf("bidiLevels(%q, 1) = %v, want %v: the neutral between an EN and an AN island takes the base level, so the two stay apart", "123 ٤٥٦", levels, want)
			break
		}
	}
	joined := bidiLevels("abc 123", 1, false)
	if joined[3] != 2 {
		t.Errorf("the space between two LTR islands resolved to level %d, want 2 so they read as one island", joined[3])
	}
}

func TestBidiVisualPermutationMatchesTheReference(t *testing.T) {
	cases := []struct {
		text       string
		base       int
		override   bool
		wantVisual string
	}{
		{"مرحبا abc 123", 1, false, "abc 123 ابحرم"},
		{"مرحبا 123 ٤٥٦ abc", 1, false, "abc ٤٥٦ 123 ابحرم"},
		{"שלום مرحبا", 1, false, "ابحرم םולש"},
		{"مرحبا 12,34", 1, false, "12,34 ابحرم"},
		{"(abc)", 1, false, ")abc("},
		{"مرحبا abc", 1, true, "cba ابحرم"},
		{"مرحبا abc", 0, false, "ابحرم abc"},
		{"abc def", 1, false, "abc def"},
	}
	for _, c := range cases {
		runes := []rune(c.text)
		perm := bidiVisualPermutation(bidiLevels(c.text, c.base, c.override))
		var sb strings.Builder
		for _, i := range perm {
			sb.WriteRune(runes[i])
		}
		if sb.String() != c.wantVisual {
			t.Errorf("visual order of %q at base %d (override=%v) = %q, want %q", c.text, c.base, c.override, sb.String(), c.wantVisual)
		}
	}
}

func TestResolveBidiOverrideForcesBaseLevel(t *testing.T) {
	runs := resolveBidi("hello שלום", 1, true)
	if len(runs) != 1 || runs[0].Level != 1 || runs[0].Start != 0 || runs[0].End != 10 {
		t.Errorf("override at base 1 = %+v, want one [0,10) run at level 1", runs)
	}
	runs = resolveBidi("שלום hello", 0, true)
	if len(runs) != 1 || runs[0].Level != 0 || runs[0].Start != 0 || runs[0].End != 10 {
		t.Errorf("override at base 0 = %+v, want one [0,10) run at level 0", runs)
	}
}

func TestPlaintextBaseLevel(t *testing.T) {
	cases := []struct {
		text string
		fallback,
		want int
	}{
		{"שלום hello", 0, 1},
		{"hello שלום", 1, 0},
		{"123 שלום", 0, 1},
		{"  ...  ", 1, 1},
		{"", 0, 0},
	}
	for _, c := range cases {
		if got := plaintextBaseLevel(c.text, c.fallback); got != c.want {
			t.Errorf("plaintextBaseLevel(%q, %d) = %d, want %d", c.text, c.fallback, got, c.want)
		}
	}
}

func TestResolveBidiTilesText(t *testing.T) {
	for _, text := range []string{"abc 123 שלום", "hello שלום", "שלום עולם", "a,"} {
		runs := resolveBidi(text, 1, false)
		next := 0
		for _, r := range runs {
			if r.Start != next {
				t.Fatalf("resolveBidi(%q, 1) runs %+v do not tile: run starts at %d, want %d", text, runs, r.Start, next)
			}
			next = r.End
		}
		if next != len([]rune(text)) {
			t.Errorf("resolveBidi(%q, 1) runs %+v end at %d, want %d", text, runs, next, len([]rune(text)))
		}
	}
}
