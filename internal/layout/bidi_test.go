package layout

import "testing"

// spanOf returns the bidiRun covering the rune span [start, end) of text,
// or nil when no single run covers it whole.
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
	// "hello" is runes 0-4 and stays LTR; the space and "שלום" (runes 5-9)
	// take the RTL base level and merge into one run.
	if runs[0].Start != 0 || runs[0].End != 5 || runs[0].Level != 0 {
		t.Errorf("run 0 = %+v, want [0,5) at level 0", runs[0])
	}
	if runs[1].Start != 5 || runs[1].End != 10 || runs[1].Level != 1 {
		t.Errorf("run 1 = %+v, want [5,10) at level 1", runs[1])
	}
}

func TestResolveBidiDigitsStayLTR(t *testing.T) {
	runs := resolveBidi("abc 123 שלום", 1, false)
	// "123" occupies runes 4-6; it must be exactly one run at level 0,
	// never merged into the RTL tail or reversed.
	digits := spanOf(runs, 4, 7)
	if digits == nil {
		t.Fatalf("resolveBidi(abc 123 שלום, 1) = %+v, no run covers the digits", runs)
	}
	if digits.Start != 4 || digits.End != 7 || digits.Level != 0 {
		t.Errorf("digits run = %+v, want [4,7) at level 0", *digits)
	}
	hebrew := spanOf(runs, 8, 12)
	if hebrew == nil || hebrew.Level != 1 {
		t.Errorf("Hebrew tail not in a level-1 run: %+v", runs)
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
		{"שלום hello", 0, 1}, // first strong char is Hebrew
		{"hello שלום", 1, 0}, // first strong char is Latin
		{"123 שלום", 0, 1},   // digits are skipped, not strong
		{"  ...  ", 1, 1},    // no strong char at all keeps the fallback
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
