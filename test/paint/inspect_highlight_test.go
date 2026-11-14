package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
)

const inspectHTML = `<html><head><style>` +
	`html,body{margin:0}` +
	`#target{position:absolute;left:10px;top:20px;width:100px;height:40px}` +
	`</style></head><body>` +
	`<p id="caption">hello</p>` +
	`<div id="target"></div>` +
	`</body></html>`

func inspectSession(t *testing.T) *engine.Session {
	t.Helper()
	s, err := engine.NewSession(inspectHTML, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func objectByID(t *testing.T, s *engine.Session, id string) *layout.Object {
	t.Helper()
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			return obj
		}
	}
	t.Fatalf("element #%s not found in the arena", id)
	return nil
}

// qc:BR-142
func TestInspectHighlightAbsentByDefault(t *testing.T) {
	s := inspectSession(t)
	cmds := popupCommands(t, s)
	if got := fillsOfColor(cmds, paint.InspectHighlightColor); len(got) != 0 {
		t.Errorf("an un-inspected page painted %v highlight overlays, want none", got)
	}
}

// qc:BR-142
func TestInspectHighlightCoversNodeBorderRect(t *testing.T) {
	s := inspectSession(t)
	obj := objectByID(t, s, "target")
	s.SetInspectHighlight(obj.Node)
	cmds := popupCommands(t, s)

	overlays := fillsOfColor(cmds, paint.InspectHighlightColor)
	if len(overlays) != 1 {
		t.Fatalf("the inspector painted %v highlight overlays, want exactly one", overlays)
	}
	want := frame.RectF4(10, 20, 110, 60).ToDevice(1.0)
	if overlays[0] != want {
		t.Errorf("highlight overlay = %v, want the node's border rect %v", overlays[0], want)
	}

	lastText := -1
	overlayIdx := -1
	for i, cmd := range cmds {
		if cmd.Kind == paint.CmdText && len(cmd.Text.Glyphs) > 0 {
			lastText = i
		}
		if cmd.Kind == paint.CmdFill && cmd.Color == paint.InspectHighlightColor {
			overlayIdx = i
		}
	}
	if lastText < 0 {
		t.Fatal("the page painted no document text to compare the overlay against")
	}
	if overlayIdx <= lastText {
		t.Errorf("highlight overlay at %d is not after the last document glyph at %d", overlayIdx, lastText)
	}
}

// qc:BR-142
func TestInspectHighlightClearRemovesOverlay(t *testing.T) {
	s := inspectSession(t)
	obj := objectByID(t, s, "target")
	s.SetInspectHighlight(obj.Node)
	if got := len(fillsOfColor(popupCommands(t, s), paint.InspectHighlightColor)); got != 1 {
		t.Fatalf("after SetInspectHighlight the page painted %d overlays, want 1", got)
	}
	s.ClearInspectHighlight()
	if got := fillsOfColor(popupCommands(t, s), paint.InspectHighlightColor); len(got) != 0 {
		t.Errorf("after ClearInspectHighlight the page still painted %v overlays, want none", got)
	}
}
