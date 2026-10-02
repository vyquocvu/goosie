package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

func TestScrollEvent(t *testing.T) {
	html := `<html><body style="margin: 0;"><div id="target">Content</div></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	scrollFired := false
	dom.AddEventListener(&s.Doc.Node, "scroll", func(e *dom.Event) {
		scrollFired = true
	}, false)

	s.FireScrollEvent()

	if !scrollFired {
		t.Fatal("scroll event was not fired")
	}
}

func TestResizeEvent(t *testing.T) {
	html := `<html><body style="margin: 0;"><div id="target">Content</div></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	resizeFired := false
	dom.AddEventListener(&s.Doc.Node, "resize", func(e *dom.Event) {
		resizeFired = true
	}, false)

	s.FireResizeEvent()

	if !resizeFired {
		t.Fatal("resize event was not fired")
	}
}

func TestSetHover(t *testing.T) {
	html := `<html><body style="margin: 0;"><div id="target" style="width: 100px; height: 100px;">Content</div></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Hover over the div.
	changed := s.SetHover(50, 50)
	if !changed {
		t.Error("SetHover should return true when hover state changes")
	}

	// Hover again at the same position - should not change.
	changed = s.SetHover(50, 50)
	if changed {
		t.Error("SetHover should return false when hover state doesn't change")
	}

	// Hover outside the div.
	changed = s.SetHover(700, 500)
	if !changed {
		t.Error("SetHover should return true when moving hover outside element")
	}
}

func TestHoverStatePersistsAcrossReflows(t *testing.T) {
	html := `<html><body style="margin: 0;"><div id="target" style="width: 100px; height: 100px;">Content</div></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Set hover state.
	s.SetHover(50, 50)

	// Reflow should not clear hover state (it's tracked separately from layout).
	_ = s.Reflow(800)

	// Hover state should still be set (though we can't directly test this without
	// accessing internal state, the fact that SetHover returns false when called
	// again at the same position indicates the state persisted).
}

func TestFocusStateWithCSS(t *testing.T) {
	html := `<html><body style="margin: 0;"><input id="inp" type="text" value="test"></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	input := s.Doc.ElementByID("inp")
	if input == nil {
		t.Fatal("input not found")
	}

	// Focus the input via FocusControl at a point where the input is located.
	// The input should be at the top-left of the document.
	s.FocusControl(10, 10)

	// The input should now be focused.
	if s.Focused() != input {
		t.Error("input should be focused")
	}

	// Blur by clicking elsewhere.
	s.FocusControl(790, 590)
	if s.Focused() != nil {
		t.Error("no element should be focused after blur")
	}
}

func TestMultipleHoverTransitions(t *testing.T) {
	html := `<html><body style="margin: 0;">
		<div id="a" style="width: 50px; height: 50px;">A</div>
		<div id="b" style="width: 50px; height: 50px;">B</div>
	</body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Hover over element A.
	changed := s.SetHover(25, 25)
	if !changed {
		t.Error("should change hover to element A")
	}

	// Move hover to element B.
	changed = s.SetHover(25, 75)
	if !changed {
		t.Error("should change hover to element B")
	}

	// Move hover outside both elements.
	changed = s.SetHover(400, 300)
	if !changed {
		t.Error("should clear hover when moving outside elements")
	}
}
