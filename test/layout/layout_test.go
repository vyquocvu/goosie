package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

func session(t *testing.T, html string, viewportW float32) *layout.Arena {
	t.Helper()
	doc := dom.Parse(html)
	styles := style.Resolve(doc, nil)
	arena := layout.Build(doc, styles)
	layout.Block(arena, layout.ObjectID(1), viewportW)
	return arena
}

func findByTag(arena *layout.Arena, tag string) *layout.Object {
	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node != nil && obj.Node.Data == tag && obj.Style != nil {
			return obj
		}
	}
	return nil
}

func findAllByTag(arena *layout.Arena, tag string) []*layout.Object {
	var result []*layout.Object
	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node != nil && obj.Node.Data == tag && obj.Style != nil {
			result = append(result, obj)
		}
	}
	return result
}

func TestBlockLayout(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0; padding: 10px;">
<div style="width: 200px; height: 100px;"></div>
</body></html>`, 800)

	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	if div.W != 200 {
		t.Errorf("div.W = %v, want 200", div.W)
	}
	if div.H != 100 {
		t.Errorf("div.H = %v, want 100", div.H)
	}
	// Div should be positioned inside body's padding
	if div.X != 10 {
		t.Errorf("div.X = %v, want 10", div.X)
	}
	if div.Y != 10 {
		t.Errorf("div.Y = %v, want 10", div.Y)
	}
}

func TestAutoWidth(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0; padding: 10px;">
<div style="height: 50px;"></div>
</body></html>`, 800)

	body := findByTag(arena, "body")
	if body == nil {
		t.Fatal("body not found")
	}
	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	// Div auto width should fill body's content area: 800 - 10 - 10 = 780
	if div.W != 780 {
		t.Errorf("div.W = %v, want 780 (auto width in 800px viewport with 10px padding)", div.W)
	}
}

func TestAutoHeight(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0; padding: 10px;">
<div style="height: 100px;"></div>
</body></html>`, 800)

	body := findByTag(arena, "body")
	if body == nil {
		t.Fatal("body not found")
	}
	// Body auto height should be: padding-top (10) + child height (100) + padding-bottom (10) = 120
	if body.H != 120 {
		t.Errorf("body.H = %v, want 120 (auto height from children + padding)", body.H)
	}
}

func TestMarginCollapsing(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="margin: 20px; height: 50px;"></div>
<div style="margin: 30px; height: 50px;"></div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) < 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	div1, div2 := divs[0], divs[1]

	// First div: margin-top 20px, positioned inside body (no padding)
	if div1.Y != 20 {
		t.Errorf("div1.Y = %v, want 20", div1.Y)
	}

	// Second div: margins collapse between siblings.
	// div1 bottom margin = 20, div2 top margin = 30, collapsed = max(20, 30) = 30
	// div2.Y = div1.Y + div1.H + collapsed margin = 20 + 50 + 30 = 100
	if div2.Y != 100 {
		t.Errorf("div2.Y = %v, want 100 (margin collapsing)", div2.Y)
	}
}
