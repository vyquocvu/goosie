package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestShapeProperties(t *testing.T) {
	st := styleFor(t, `<html><body>
<div id="target" style="shape-outside: circle(50px at 50% 50%); shape-margin: 10px;">Content</div>
</body></html>`, "", "div")

	if st == nil {
		t.Fatal("target style not found")
	}
	if st.ShapeOutside != "circle(50px at 50% 50%)" {
		t.Errorf("ShapeOutside = %q, want circle(50px at 50%%%% 50%%%%)", st.ShapeOutside)
	}
	if st.ShapeMargin != 10 {
		t.Errorf("ShapeMargin = %v, want 10", st.ShapeMargin)
	}
}

func TestDynamicViewportUnits(t *testing.T) {
	doc := domtest.Parse(`<html><body>
<div id="box" style="width: 50dvw; height: 25dvh; margin-left: 10vi; margin-top: 5vb;">Content</div>
</body></html>`)

	vp := style.Viewport{W: 1000, H: 800}
	styles := style.ResolveViewport(doc, nil, vp, nil)

	box := doc.ElementByID("box")
	if box == nil {
		t.Fatal("box element not found")
	}
	target := styles[box.ID]
	if target == nil {
		t.Fatal("box style not found")
	}
	// 50dvw on 1000px viewport = 500px
	if target.Width != 500 {
		t.Errorf("Width = %v, want 500", target.Width)
	}
	// 25dvh on 800px viewport = 200px
	if target.Height != 200 {
		t.Errorf("Height = %v, want 200", target.Height)
	}
	// 10vi on 1000px = 100px
	if target.MarginLeft != 100 {
		t.Errorf("MarginLeft = %v, want 100", target.MarginLeft)
	}
	// 5vb on 800px = 40px
	if target.MarginTop != 40 {
		t.Errorf("MarginTop = %v, want 40", target.MarginTop)
	}
}
