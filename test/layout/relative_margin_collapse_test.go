package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

// A relatively positioned box is in-flow: its margins collapse with its
// parent and siblings exactly like a static box (CSS 2.1 §8.3.1). The
// relative offset applies afterwards and does not take part in collapsing.
func divY(t *testing.T, divStyle string) float32 {
	t.Helper()
	doc := domtest.Parse(`<html><body><div style="` + divStyle + `">x</div></body></html>`)
	styles := style.Resolve(doc, nil, nil)
	arena := layout.Build(doc, styles, nil)
	layout.Block(arena, layout.ObjectID(1), 800, 0)
	layout.Positioning(arena, layout.ObjectID(1), 800, 0)
	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	return div.Y
}

func TestRelativeFirstChildCollapsesWithParent(t *testing.T) {
	base := "margin: 16px 0; border: 1px solid black; height: 96px; width: 480px;"
	staticY := divY(t, base)
	relY := divY(t, base+" position: relative;")
	// Body carries the UA 8px margin; the div's 16px margin collapses with
	// it, so the border-top edge lands at max(8, 16) = 16 in both cases.
	if !almostEqual(staticY, 16) {
		t.Fatalf("static div.Y = %v, want 16", staticY)
	}
	if !almostEqual(relY, staticY) {
		t.Fatalf("relative div.Y = %v, want static position %v (margins must collapse before the relative shift)", relY, staticY)
	}
}

func TestRelativeSiblingsCollapse(t *testing.T) {
	html := `<html><body style="margin: 0;"><div style="margin: 0 0 20px 0; height: 10px;"></div><div style="margin: 16px 0 0 0; height: 10px; position: relative;"></div></body></html>`
	doc := domtest.Parse(html)
	styles := style.Resolve(doc, nil, nil)
	arena := layout.Build(doc, styles, nil)
	layout.Block(arena, layout.ObjectID(1), 800, 0)
	layout.Positioning(arena, layout.ObjectID(1), 800, 0)
	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	// Adjacent margins collapse to max(20, 16) = 20: second border-top at
	// 10 (first height) + 20 = 30.
	if !almostEqual(divs[1].Y, 30) {
		t.Fatalf("second div.Y = %v, want 30 (collapsed max(20,16) after 10px first box)", divs[1].Y)
	}
}
