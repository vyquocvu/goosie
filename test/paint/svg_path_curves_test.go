package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func svgCommands(t *testing.T, svg string) []paint.DisplayCmd {
	t.Helper()
	html := `<!DOCTYPE html><html><body style="margin:0">` + svg + `</body></html>`
	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}
	return dl.Build(1).All()
}

func fillsOf(cmds []paint.DisplayCmd, color frame.Color) (count int, minX, minY, maxX, maxY float32) {
	first := true
	for _, cmd := range cmds {
		if cmd.Kind != paint.CmdFill || cmd.Color != color {
			continue
		}
		count++
		x0, y0 := float32(cmd.Rect.X0), float32(cmd.Rect.Y0)
		x1, y1 := float32(cmd.Rect.X1), float32(cmd.Rect.Y1)
		if first {
			minX, minY, maxX, maxY = x0, y0, x1, y1
			first = false
			continue
		}
		if x0 < minX {
			minX = x0
		}
		if y0 < minY {
			minY = y0
		}
		if x1 > maxX {
			maxX = x1
		}
		if y1 > maxY {
			maxY = y1
		}
	}
	return count, minX, minY, maxX, maxY
}

func TestSVGCubicPathFills(t *testing.T) {
	red := frame.RGB(255, 0, 0)
	cmds := svgCommands(t,
		`<svg width="100" height="100"><path d="M10 50 C10 10 90 10 90 50 Z" fill="red"></path></svg>`)

	count, minX, minY, maxX, _ := fillsOf(cmds, red)
	if count == 0 {
		t.Fatal("cubic path painted nothing")
	}
	if minY > 30 {
		t.Errorf("fill top = %v, want the curve's bulge near y=20", minY)
	}
	if maxX-minX < 60 {
		t.Errorf("fill spans %v px, want the full 80 px curve", maxX-minX)
	}
}

func TestSVGArcPathFills(t *testing.T) {
	blue := frame.RGB(0, 0, 255)
	cmds := svgCommands(t,
		`<svg width="100" height="100"><path d="M10 50 A40 40 0 0 1 90 50 Z" fill="blue"></path></svg>`)

	count, minX, _, maxX, _ := fillsOf(cmds, blue)
	if count == 0 {
		t.Fatal("arc path painted nothing")
	}
	if maxX-minX < 60 {
		t.Errorf("fill spans %v px, want the arc's 80 px width", maxX-minX)
	}
}

func TestSVGPathSubpathsStaySeparate(t *testing.T) {
	green := frame.RGB(0, 128, 0)
	cmds := svgCommands(t,
		`<svg width="100" height="100"><path d="M10 10 h20 v20 h-20 Z M60 10 h20 v20 h-20 Z" fill="green"></path></svg>`)

	count, _, _, _, _ := fillsOf(cmds, green)
	if count == 0 {
		t.Fatal("path painted nothing")
	}
	for _, cmd := range cmds {
		if cmd.Kind != paint.CmdFill || cmd.Color != green {
			continue
		}
		if cmd.Rect.X0 >= 32 && cmd.Rect.X1 <= 58 {
			t.Fatalf("fill in the gap between subpaths: rect %v", cmd.Rect)
		}
	}
}

func TestSVGViewBoxScalesCoordinates(t *testing.T) {
	blue := frame.RGB(0, 0, 255)
	cmds := svgCommands(t,
		`<svg width="200" height="200" viewBox="0 0 100 100"><path d="M0 0 h10 v10 h-10 Z" fill="blue"></path></svg>`)

	count, _, _, maxX, maxY := fillsOf(cmds, blue)
	if count == 0 {
		t.Fatal("viewBox path painted nothing")
	}
	if maxX < 19.5 || maxX > 21.5 {
		t.Errorf("fill right edge = %v, want ~20 (10 user units at 2x)", maxX)
	}
	if maxY < 19.5 || maxY > 21.5 {
		t.Errorf("fill bottom edge = %v, want ~20 (10 user units at 2x)", maxY)
	}
}
