package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

func TestInlineFragmentsCapture(t *testing.T) {
	arena := session(t, `<div style="width:400px"><span style="border:3px solid blue">word</span></div>`, 400)
	layout.Inline(arena, layout.ObjectID(1))
	span := findByTag(arena, "span")
	if span == nil {
		t.Fatal("no span box in arena")
	}
	if len(span.InlineFragments) != 1 {
		t.Fatalf("single-line span: got %d fragments, want 1", len(span.InlineFragments))
	}

	arena2 := session(t, `<div style="width:40px"><span style="border:3px solid blue">aa bb cc dd ee ff</span></div>`, 40)
	layout.Inline(arena2, layout.ObjectID(1))
	span2 := findByTag(arena2, "span")
	if span2 == nil {
		t.Fatal("no wrapping span box in arena")
	}
	if len(span2.InlineFragments) < 2 {
		t.Fatalf("wrapping span: got %d fragments, want >= 2", len(span2.InlineFragments))
	}
	for i := 1; i < len(span2.InlineFragments); i++ {
		if span2.InlineFragments[i].Y0 <= span2.InlineFragments[i-1].Y0 {
			t.Fatalf("fragments not in line order at %d: %v then %v",
				i, span2.InlineFragments[i-1], span2.InlineFragments[i])
		}
	}

	arena3 := session(t, `<div style="width:40px"><span>aa bb cc dd</span></div>`, 40)
	layout.Inline(arena3, layout.ObjectID(1))
	span3 := findByTag(arena3, "span")
	if span3 == nil {
		t.Fatal("no borderless span box in arena")
	}
	if len(span3.InlineFragments) != 0 {
		t.Fatalf("borderless span: got %d fragments, want 0", len(span3.InlineFragments))
	}
}

func TestInlineFragmentsEdgeExpansion(t *testing.T) {
	arena := session(t, `<div style="width:400px"><span style="border:3px solid blue">word</span></div>`, 400)
	layout.Inline(arena, layout.ObjectID(1))
	span := findByTag(arena, "span")
	if span == nil || len(span.InlineFragments) != 1 {
		t.Fatalf("need a single-line span with one fragment, got %v", span)
	}
	f := span.InlineFragments[0]
	x0, y0, x1, y1 := span.BorderRect()
	if !almostEqual(f.X0, x0-3) || !almostEqual(f.Y0, y0-3) || !almostEqual(f.X1, x1+3) || !almostEqual(f.Y1, y1+3) {
		t.Fatalf("fragment %v should be the content union (%v,%v,%v,%v) expanded by 3px on every edge",
			f, x0, y0, x1, y1)
	}
}
