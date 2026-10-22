package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func TestSVGShapeRendering(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body>
<svg width="200" height="150">
  <rect x="10" y="20" width="80" height="40" fill="red" stroke="black"></rect>
  <circle cx="150" cy="50" r="30" fill="blue"></circle>
  <line x1="10" y1="100" x2="190" y2="100" stroke="green" stroke-width="2"></line>
</svg>
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}

	list := dl.Build(1)
	if list == nil {
		t.Fatalf("Build returned nil display list")
	}

	commands := list.All()
	if len(commands) == 0 {
		t.Fatalf("expected non-empty display commands for SVG shapes")
	}

	var hasRed, hasBlue, hasGreen, hasBlack bool
	redColor := frame.RGB(255, 0, 0)
	blueColor := frame.RGB(0, 0, 255)
	greenColor := frame.RGB(0, 128, 0)
	blackColor := frame.RGB(0, 0, 0)

	for _, cmd := range commands {
		if cmd.Kind == paint.CmdFill {
			switch cmd.Color {
			case redColor:
				hasRed = true
			case blueColor:
				hasBlue = true
			case greenColor:
				hasGreen = true
			case blackColor:
				hasBlack = true
			}
		}
	}

	if !hasRed {
		t.Errorf("expected red fill for <rect>")
	}
	if !hasBlue {
		t.Errorf("expected blue fill for <circle>")
	}
	if !hasGreen {
		t.Errorf("expected green stroke for <line>")
	}
	if !hasBlack {
		t.Errorf("expected black stroke for <rect>")
	}
}

func TestSVGPathRendering(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body>
<svg width="100" height="100">
  <path d="M 10 10 L 90 10 L 90 90 Z" fill="yellow" stroke="black"></path>
</svg>
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}

	list := dl.Build(1)
	commands := list.All()
	if len(commands) == 0 {
		t.Fatalf("expected commands for SVG path")
	}

	var hasYellow bool
	yellowColor := frame.RGB(255, 255, 0)
	for _, cmd := range commands {
		if cmd.Kind == paint.CmdFill && cmd.Color == yellowColor {
			hasYellow = true
			break
		}
	}
	if !hasYellow {
		t.Errorf("expected yellow fill for <path>")
	}
}

func TestSVGEllipseAndPolygon(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body>
<svg width="200" height="200">
  <ellipse cx="60" cy="60" rx="40" ry="20" fill="purple"></ellipse>
  <polygon points="100,20 180,60 140,120" fill="cyan"></polygon>
</svg>
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}

	list := dl.Build(1)
	commands := list.All()

	var hasPurple, hasCyan bool
	purpleColor := frame.RGB(128, 0, 128)
	cyanColor := frame.RGB(0, 255, 255)

	for _, cmd := range commands {
		if cmd.Kind == paint.CmdFill {
			if cmd.Color == purpleColor {
				hasPurple = true
			} else if cmd.Color == cyanColor {
				hasCyan = true
			}
		}
	}

	if !hasPurple {
		t.Errorf("expected purple fill for <ellipse>")
	}
	if !hasCyan {
		t.Errorf("expected cyan fill for <polygon>")
	}
}
