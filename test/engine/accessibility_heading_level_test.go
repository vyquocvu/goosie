package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
)

// qc:BR-300
func TestAccessibilityNativeHeadingsCarryTheirLevel(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin: 0;">
<h1>alpha-one</h1>
<h2>beta-two</h2>
<h3>gamma-three</h3>
<h4>delta-four</h4>
<h5>epsilon-five</h5>
<h6>zeta-six</h6>
</body></html>`)
	tree := s.AccessibilityTree()
	for _, tc := range []struct {
		label string
		level int
	}{
		{"alpha-one", 1},
		{"beta-two", 2},
		{"gamma-three", 3},
		{"delta-four", 4},
		{"epsilon-five", 5},
		{"zeta-six", 6},
	} {
		node := axFind(tree, ax.RoleHeading, tc.label)
		if node == nil {
			t.Fatalf("heading %q missing: %+v", tc.label, axFlatten(tree))
		}
		if node.Level != tc.level {
			t.Errorf("heading %q level = %d, want %d", tc.label, node.Level, tc.level)
		}
	}
}

// qc:BR-300
func TestAccessibilityHeadingLevelHonoursValidAriaLevel(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin: 0;">
<h2 aria-level="5">override-five</h2>
<div role="heading" aria-level="3">aria-three</div>
</body></html>`)
	tree := s.AccessibilityTree()
	if node := axFind(tree, ax.RoleHeading, "override-five"); node == nil || node.Level != 5 {
		t.Errorf("native h2 with aria-level=5 level = %+v, want 5", node)
	}
	if node := axFind(tree, ax.RoleHeading, "aria-three"); node == nil || node.Level != 3 {
		t.Errorf("div role=heading aria-level=3 level = %+v, want 3", node)
	}
}

// qc:BR-300
func TestAccessibilityHeadingLevelDefaultsAndIgnoresOutOfRangeAriaLevel(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin: 0;">
<div role="heading">bare-aria</div>
<h1 role="heading">native-role</h1>
<h4 aria-level="0">zero-out</h4>
<h4 aria-level="9">nine-out</h4>
<h4 aria-level="abc">bad-aria</h4>
</body></html>`)
	tree := s.AccessibilityTree()
	if node := axFind(tree, ax.RoleHeading, "bare-aria"); node == nil || node.Level != 2 {
		t.Errorf("role=heading without aria-level level = %+v, want default 2", node)
	}
	if node := axFind(tree, ax.RoleHeading, "native-role"); node == nil || node.Level != 1 {
		t.Errorf("native h1 with role=heading level = %+v, want implicit 1", node)
	}
	for _, label := range []string{"zero-out", "nine-out", "bad-aria"} {
		node := axFind(tree, ax.RoleHeading, label)
		if node == nil || node.Level != 4 {
			t.Errorf("h4 with out-of-range/invalid aria-level %q level = %+v, want implicit 4", label, node)
		}
	}
}
