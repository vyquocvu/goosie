package renderer_test

import (
	"testing"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// Media Queries Level 4 range syntax, as used by modern stylesheets
// (e.g. iana.org's (width <= 1000px) / (width < 1200px) rules).
func TestMediaQueryRangeSyntax(t *testing.T) {
	mq := renderer.NewMediaQueryEvaluator(1000, 800)

	cases := []struct {
		prelude string
		want    bool
		desc    string
	}{
		// Feature on the left.
		{"(width <= 1000px)", true, "width <= boundary matches at boundary"},
		{"(width < 1000px)", false, "width < boundary does not match at boundary"},
		{"(width < 1200px)", true, "width < larger matches"},
		{"(width > 800px)", true, "width > smaller matches"},
		{"(width >= 1000px)", true, "width >= boundary matches at boundary"},
		{"(width >= 1001px)", false, "width >= above boundary does not match"},
		{"(height <= 800px)", true, "height range matches"},
		{"(height > 800px)", false, "height range respects strict compare"},
		// Value on the left (reversed form).
		{"(1000px >= width)", true, "reversed >= matches at boundary"},
		{"(1200px < width)", false, "reversed < does not match"},
		{"(900px < width)", true, "reversed < matches"},
		// Chained form.
		{"(400px <= width <= 1000px)", true, "chain matches in range"},
		{"(400px <= width <= 999px)", false, "chain fails upper bound"},
		{"(1001px <= width <= 1200px)", false, "chain fails lower bound"},
		// Combined with classic conditions via and.
		{"screen and (width <= 1000px)", true, "media type and range"},
		{"(min-width: 500px) and (width <= 1000px)", true, "classic and range"},
		{"not (width > 1000px)", true, "negated range"},
		// Unknown features still do not match.
		{"(hover <= 1)", false, "unknown feature stays false"},
		// Malformed expressions do not match.
		{"(width <=)", false, "missing operand"},
		{"(<= 600px)", false, "missing feature"},
	}
	for _, tc := range cases {
		if got := mq.Evaluate(tc.prelude); got != tc.want {
			t.Errorf("Evaluate(%q) at 1000x800 = %v, want %v (%s)",
				tc.prelude, got, tc.want, tc.desc)
		}
	}
}

// The iana.org/about failure mode: at a 1000px viewport the compact-layout
// rules must now apply, while at 1280px the desktop rules still win.
func TestMediaQueryRangeSyntaxIanaBehavior(t *testing.T) {
	desktop := renderer.NewMediaQueryEvaluator(1280, 800)
	compact := renderer.NewMediaQueryEvaluator(1000, 800)

	if !compact.Evaluate("(width <= 1000px)") {
		t.Error("compact rule (width <= 1000px) should match at 1000px")
	}
	if desktop.Evaluate("(width <= 1000px)") {
		t.Error("compact rule (width <= 1000px) should not match at 1280px")
	}
	if !desktop.Evaluate("(width < 1200px)") || compact.Evaluate("not (width < 1200px)") == false {
		// desktop matches (width < 1200px) is false at 1280; compact matches it at 1000.
		t.Log("(width < 1200px): desktop=false is expected; verified compact below")
	}
	if !compact.Evaluate("(width < 1200px)") {
		t.Error("(width < 1200px) should match at 1000px")
	}
	if desktop.Evaluate("(width < 1200px)") {
		t.Error("(width < 1200px) should not match at 1280px")
	}
}

func TestMediaQueryRangeAspectRatio(t *testing.T) {
	mq := renderer.NewMediaQueryEvaluator(1600, 900) // 16:9
	if !mq.Evaluate("(aspect-ratio >= 16/9)") {
		t.Error("(aspect-ratio >= 16/9) should match a 16:9 viewport")
	}
	if mq.Evaluate("(aspect-ratio < 16/9)") {
		t.Error("(aspect-ratio < 16/9) should not match a 16:9 viewport")
	}
}
