package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
)

// qc:BR-077
func TestViewportResizeReflowsAndDropsOldWidthGeometry(t *testing.T) {
	s, err := engine.NewSession(`<html><head><style>
		html, body { margin: 0; padding: 0; }
		#box { width: 100%; height: 10px; background: #123456; }
		p { margin: 0; font-size: 16px; }
	</style></head><body><div id="box"></div><p id="copy">`+
		"alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau"+`</p></body></html>`,
		nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)

	box := s.Doc.ElementByID("box")
	copyNode := s.Doc.ElementByID("copy")
	boxRect := func() (float32, float32) {
		id, ok := s.Arena.ForNode(box.ID)
		if !ok {
			t.Fatal("box has no layout object")
		}
		o := s.Arena.Get(id)
		return o.W, o.H
	}
	copyHeight := func() float32 {
		id, ok := s.Arena.ForNode(copyNode.ID)
		if !ok {
			t.Fatal("paragraph has no layout object")
		}
		return s.Arena.Get(id).H
	}
	commands := func() []frame.Rect {
		var out []frame.Rect
		for _, cmd := range s.Paint(1).All() {
			out = append(out, cmd.Rect)
		}
		return out
	}

	wideW, _ := boxRect()
	wideH := copyHeight()
	wideCmds := commands()
	if wideW != 800 {
		t.Fatalf("wide box width = %v, want the 800px viewport", wideW)
	}

	if err := s.Reflow(320); err != nil {
		t.Fatal(err)
	}
	narrowW, _ := boxRect()
	narrowH := copyHeight()
	if narrowW != 320 {
		t.Fatalf("narrow box width = %v, want the 320px viewport", narrowW)
	}
	if narrowH <= wideH {
		t.Fatalf("narrow paragraph height = %v, want more wrapped lines than the %v wide height", narrowH, wideH)
	}
	for _, r := range commands() {
		if r.X1 > 320 {
			t.Fatalf("command at %v still reaches past the 320px viewport", r)
		}
	}

	if err := s.Reflow(800); err != nil {
		t.Fatal(err)
	}
	backW, _ := boxRect()
	backH := copyHeight()
	if backW != wideW || backH != wideH {
		t.Fatalf("wide return geometry = (%v, %v), want (%v, %v)", backW, backH, wideW, wideH)
	}
	backCmds := commands()
	if len(backCmds) != len(wideCmds) {
		t.Fatalf("wide return has %d commands, want the original %d", len(backCmds), len(wideCmds))
	}
	for i := range wideCmds {
		if backCmds[i] != wideCmds[i] {
			t.Fatalf("command %d = %v after the round trip, want the original %v", i, backCmds[i], wideCmds[i])
		}
	}
}
