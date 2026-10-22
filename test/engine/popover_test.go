package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

func TestPopoverTarget_ButtonClickTogglesPopover(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body style="margin: 0;">
  <button id="btn" popovertarget="pop">Toggle Popover</button>
  <div id="pop" popover>Popover Content</div>
</body>
</html>`

	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	pop := s.Doc.ElementByID("pop")
	if pop == nil {
		t.Fatal("popover element not found")
	}
	if pop.IsPopoverOpen {
		t.Fatal("popover should initially be closed")
	}

	toggleEvents := 0
	dom.AddEventListener(pop, "toggle", func(e *dom.Event) {
		toggleEvents++
	}, false)

	// Click the button to open
	bx, by := clickPoint(t, s, "btn")
	if !s.ClickAt(bx, by) {
		t.Fatal("expected ClickAt on button to return true")
	}

	if !pop.IsPopoverOpen {
		t.Fatal("popover should be open after button click")
	}
	if toggleEvents != 1 {
		t.Fatalf("expected 1 toggle event, got %d", toggleEvents)
	}

	// Click the button again to close
	if !s.ClickAt(bx, by) {
		t.Fatal("expected ClickAt on button to return true")
	}

	if pop.IsPopoverOpen {
		t.Fatal("popover should be closed after second button click")
	}
	if toggleEvents != 2 {
		t.Fatalf("expected 2 toggle events, got %d", toggleEvents)
	}
}

func TestPopoverTarget_ActionShowAndHide(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body style="margin: 0;">
  <button id="showBtn" popovertarget="pop" popovertargetaction="show">Show</button>
  <button id="hideBtn" popovertarget="pop" popovertargetaction="hide">Hide</button>
  <div id="pop" popover>Popover Content</div>
</body>
</html>`

	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	pop := s.Doc.ElementByID("pop")
	showX, showY := clickPoint(t, s, "showBtn")
	hideX, hideY := clickPoint(t, s, "hideBtn")

	// Click show button
	s.ClickAt(showX, showY)
	if !pop.IsPopoverOpen {
		t.Fatal("expected popover to be open after show click")
	}

	// Click show button again (should stay open)
	s.ClickAt(showX, showY)
	if !pop.IsPopoverOpen {
		t.Fatal("expected popover to remain open after redundant show click")
	}

	// Click hide button
	s.ClickAt(hideX, hideY)
	if pop.IsPopoverOpen {
		t.Fatal("expected popover to be closed after hide click")
	}

	// Click hide button again (should stay closed)
	s.ClickAt(hideX, hideY)
	if pop.IsPopoverOpen {
		t.Fatal("expected popover to remain closed after redundant hide click")
	}
}

func TestPopover_LightDismissOnClickOutside(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body style="margin: 0;">
  <button id="btn" popovertarget="pop">Toggle</button>
  <div id="pop" popover="auto" style="display: block; width: 100px; height: 100px;">Popover</div>
  <div id="outside" style="display: block; width: 200px; height: 50px; margin-top: 200px;">Outside area</div>
</body>
</html>`

	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	pop := s.Doc.ElementByID("pop")
	btnX, btnY := clickPoint(t, s, "btn")

	// Open popover via button
	s.ClickAt(btnX, btnY)
	if !pop.IsPopoverOpen {
		t.Fatal("expected popover to be open")
	}

	// Click on outside area
	outX, outY := clickPoint(t, s, "outside")
	s.ClickAt(outX, outY)

	if pop.IsPopoverOpen {
		t.Fatal("expected auto popover to be light-dismissed on outside click")
	}
}

func TestPopover_EscapeKeyDismissesPopover(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body style="margin: 0;">
  <div id="pop" popover="auto">Popover Content</div>
</body>
</html>`

	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	pop := s.Doc.ElementByID("pop")
	s.ShowPopover(pop)
	if !pop.IsPopoverOpen {
		t.Fatal("expected popover to be open")
	}

	// Press Escape key
	s.PressKey("Escape", false, false, false, false)

	if pop.IsPopoverOpen {
		t.Fatal("expected Escape key to dismiss topmost auto popover")
	}
}

func TestPopover_EscapeOrderWithModalDialog(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body style="margin: 0;">
  <dialog id="dlg">Dialog Content</dialog>
  <div id="pop" popover="auto">Popover Content</div>
</body>
</html>`

	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	dlg := s.Doc.ElementByID("dlg")
	pop := s.Doc.ElementByID("pop")

	// Open dialog modally
	dlg.SetAttribute("open", "")
	dlg.IsModal = true

	// Open popover
	s.ShowPopover(pop)

	if !pop.IsPopoverOpen || !dlg.HasAttribute("open") {
		t.Fatal("expected both dialog and popover to be open")
	}

	// First Escape: closes the topmost auto popover, dialog remains open
	s.PressKey("Escape", false, false, false, false)
	if pop.IsPopoverOpen {
		t.Fatal("expected popover to be closed on first Escape")
	}
	if !dlg.HasAttribute("open") {
		t.Fatal("expected dialog to remain open after popover is closed on first Escape")
	}

	// Second Escape: closes the modal dialog
	s.PressKey("Escape", false, false, false, false)
	if dlg.HasAttribute("open") {
		t.Fatal("expected modal dialog to close on second Escape")
	}
}

func TestPopover_FormSubmissionPreventedByPopoverTarget(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body style="margin: 0;">
  <form id="myForm" action="/submit" method="POST">
    <button id="popBtn" popovertarget="myPop">Toggle Popover</button>
  </form>
  <div id="myPop" popover>Popover</div>
</body>
</html>`

	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	pop := s.Doc.ElementByID("myPop")
	bx, by := clickPoint(t, s, "popBtn")

	s.ClickAt(bx, by)

	if !pop.IsPopoverOpen {
		t.Fatal("popover should be open after button click")
	}

	if formSub, ok := s.PopSubmittedForm(); ok {
		t.Fatalf("expected no form submission when button has popovertarget, got %v", formSub)
	}
}
