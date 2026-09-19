package layout_test

import (
	"testing"
)

// TestFlexOrderReordersRow checks the CSS `order` property on a row flex line:
// items must be placed by their order value (DOM order for ties), not source
// order.
func TestFlexOrderReordersRow(t *testing.T) {
	// DOM order A,B,C but order 3,2,1 reverses the visual line to C,B,A.
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; width: 600px;">
  <span style="order: 3; width: 100px;">A</span>
  <span style="order: 2; width: 100px;">B</span>
  <span style="order: 1; width: 100px;">C</span>
</div>
</body></html>`, 800)

	spans := findAllByTag(arena, "span")
	if len(spans) != 3 {
		t.Fatalf("found %d spans, want 3", len(spans))
	}
	// findAllByTag walks the arena in creation (DOM) order: A, B, C.
	a, b, c := spans[0], spans[1], spans[2]
	// Visual order must be C, B, A (order values 1, 2, 3).
	if !(c.X < b.X && b.X < a.X) {
		t.Errorf("order not applied: X(C)=%v X(B)=%v X(A)=%v, want C<B<A", c.X, b.X, a.X)
	}
}
