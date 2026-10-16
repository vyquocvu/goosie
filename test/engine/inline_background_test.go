package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// An inline element's background must be painted by the inline pass's per-word
// fragments, never by the element's own box. Since inlineSelfRects that box
// carries the union of its fragment rects, so filling it would cover the whole
// bounding box across every wrapped line.
func TestInlineElementBackgroundPaintsPerFragment(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("load fonts: %v", err)
	}
	// 120px wide so the span wraps onto several lines; a union-rect fill would
	// be visibly taller than one line box.
	html := `<p style="font:20px sans-serif;margin:0">` +
		`<span style="background:#ff0">aaa bbb ccc ddd eee fff ggg hhh</span></p>`
	sess, err := engine.NewSession(html, nil, 120,
		engine.WithMetrics(fonts), engine.WithViewportH(120))
	if err != nil {
		t.Fatalf("build session: %v", err)
	}
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("paint: %v", err)
	}

	yellow := frame.RGB(255, 255, 0)
	var fills []frame.Rect
	for _, cmd := range list.All() {
		if cmd.Kind == paint.CmdFill && cmd.Color == yellow {
			fills = append(fills, cmd.Rect)
		}
	}
	if len(fills) == 0 {
		t.Fatal("no background painted for the inline span; text fragments lost their fill")
	}
	if len(fills) < 3 {
		t.Fatalf("got %d yellow fill(s) for a span wrapping several lines, want one per fragment", len(fills))
	}
	for _, r := range fills {
		if r.H() > 30 {
			t.Fatalf("yellow fill %v is %dpx tall, taller than one 20px line box: "+
				"the inline element's union rect was filled instead of its fragments", r, r.H())
		}
	}
}
