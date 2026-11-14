package engine_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

const inspectEngineHTML = `<html><head><style>` +
	`html,body{margin:0}` +
	`#target{position:absolute;left:10px;top:20px;width:100px;height:40px}` +
	`</style></head><body>` +
	`<p id="caption">hello</p>` +
	`<div id="target"><span>inner</span></div>` +
	`</body></html>`

func inspectEngineSession(t *testing.T) *engine.Session {
	t.Helper()
	s, err := engine.NewSession(inspectEngineHTML, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func domFingerprint(s *engine.Session) string {
	var b strings.Builder
	s.Doc.Node.Walk(func(n *dom.Node) bool {
		fmt.Fprintf(&b, "%d|%d|%s|%s", n.ID, n.Type, n.Data, n.DataContent)
		for _, a := range n.Attr {
			fmt.Fprintf(&b, "|%s=%s", a.Name, a.Value)
		}
		b.WriteByte('\n')
		return true
	})
	return b.String()
}

func inspectOverlayCount(t *testing.T, s *engine.Session) int {
	t.Helper()
	dl, err := s.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	n := 0
	for _, cmd := range dl.Build(1).All() {
		if cmd.Kind == paint.CmdFill && cmd.Color == paint.InspectHighlightColor {
			n++
		}
	}
	return n
}

// qc:BR-142
func TestInspectHighlightDoesNotMutateDOM(t *testing.T) {
	s := inspectEngineSession(t)
	target := s.Doc.ElementByID("target")
	if target == nil {
		t.Fatal("element #target not found")
	}

	beforeFP := domFingerprint(s)
	beforeCount, beforeDepth := s.DOMStats()

	s.SetInspectHighlight(target)
	if got := inspectOverlayCount(t, s); got != 1 {
		t.Fatalf("after SetInspectHighlight the page painted %d overlays, want 1", got)
	}
	s.ClearInspectHighlight()
	if got := inspectOverlayCount(t, s); got != 0 {
		t.Fatalf("after ClearInspectHighlight the page painted %d overlays, want 0", got)
	}

	afterFP := domFingerprint(s)
	afterCount, afterDepth := s.DOMStats()
	if beforeFP != afterFP {
		t.Errorf("the DOM changed under inspection:\nbefore:\n%s\nafter:\n%s", beforeFP, afterFP)
	}
	if beforeCount != afterCount || beforeDepth != afterDepth {
		t.Errorf("DOMStats changed: (%d,%d) -> (%d,%d)", beforeCount, beforeDepth, afterCount, afterDepth)
	}
}

// qc:BR-142
func TestInspectHighlightSkipsDetachedNode(t *testing.T) {
	s := inspectEngineSession(t)
	detached := &dom.Node{ID: 999999, Type: dom.NodeElement, Data: "div"}
	s.SetInspectHighlight(detached)
	if got := inspectOverlayCount(t, s); got != 0 {
		t.Errorf("a detached node painted %d overlays, want none", got)
	}
}
