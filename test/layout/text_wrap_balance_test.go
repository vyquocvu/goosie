package layout_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

type balanceWord struct {
	x, y, w float32
	text    string
}

func balanceWordGroups(t *testing.T, style, text string) [][]balanceWord {
	t.Helper()
	html := fmt.Sprintf(`<!DOCTYPE html><html><body style="margin: 0;"><div style="%s">%s</div></body></html>`, style, text)
	arena := session(t, html, 800)
	var words []balanceWord
	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node == nil || !obj.Node.Text() {
			continue
		}
		word := strings.TrimSpace(obj.Node.DataContent)
		if word == "" {
			continue
		}
		words = append(words, balanceWord{x: obj.X, y: obj.Y, w: obj.W, text: word})
	}
	if len(words) == 0 {
		t.Fatal("no text words were laid out")
	}
	sort.SliceStable(words, func(i, j int) bool {
		if words[i].y != words[j].y {
			return words[i].y < words[j].y
		}
		return words[i].x < words[j].x
	})
	var groups [][]balanceWord
	var current []balanceWord
	for _, w := range words {
		if len(current) == 0 || sameBalanceLine(current[0], w) {
			current = append(current, w)
			continue
		}
		groups = append(groups, current)
		current = []balanceWord{w}
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

func sameBalanceLine(a, b balanceWord) bool {
	dy := a.y - b.y
	if dy < 0 {
		dy = -dy
	}
	return dy < 8
}

func joinBalanceWords(words []balanceWord) string {
	parts := make([]string, 0, len(words))
	for _, w := range words {
		parts = append(parts, w.text)
	}
	return strings.Join(parts, " ")
}

func assertBalanceGaps(t *testing.T, label string, groups [][]balanceWord) {
	t.Helper()
	for li, group := range groups {
		for i := 1; i < len(group); i++ {
			gap := group[i].x - (group[i-1].x + group[i-1].w)
			if gap <= 0.5 {
				t.Fatalf("%s line %d has no space between %q and %q: gap=%.2f", label, li, group[i-1].text, group[i].text, gap)
			}
		}
	}
}

// The want below is Chromium 153.0.8010.12's measured break, not Goosie's own output.
func TestTextWrapBalanceKeepsTheSpaceAtARejoinedBreak(t *testing.T) {
	const text = "The best place to find government services and information"
	const head = "width: 700px; font-weight: 700; font-size: 55px; line-height: 1.07;"

	groups := balanceWordGroups(t, head+" text-wrap: balance;", text)
	var lines []string
	for _, g := range groups {
		lines = append(lines, joinBalanceWords(g))
	}
	want := []string{
		"The best place to find",
		"government services",
		"and information",
	}
	if len(lines) != len(want) {
		t.Fatalf("balanced lines = %q, want %q", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("balanced line %d = %q, want %q (all: %q)", i, lines[i], want[i], lines)
		}
	}
	assertBalanceGaps(t, "balanced", groups)
}

func TestTextWrapBalancePreservesLineCountAndWordGaps(t *testing.T) {
	const text = "The best place to find government services and information"
	for _, width := range []int{900, 700, 620, 520, 460, 380, 300} {
		head := fmt.Sprintf("width: %dpx; font-weight: 700; font-size: 55px; line-height: 1.07;", width)
		plain := balanceWordGroups(t, head, text)
		balanced := balanceWordGroups(t, head+" text-wrap: balance;", text)
		if len(balanced) != len(plain) {
			t.Errorf("width %d: balancing changed the line count from %d to %d: plain %q balanced %q",
				width, len(plain), len(balanced), joinBalanceWords(flattenBalanceWords(plain)), joinBalanceWords(flattenBalanceWords(balanced)))
		}
		assertBalanceGaps(t, fmt.Sprintf("width %d balanced", width), balanced)
		assertBalanceGaps(t, fmt.Sprintf("width %d plain", width), plain)
	}
}

func flattenBalanceWords(groups [][]balanceWord) []balanceWord {
	var all []balanceWord
	for _, g := range groups {
		all = append(all, g...)
	}
	return all
}
