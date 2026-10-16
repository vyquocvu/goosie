package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

// buildTree creates: doc -> html -> body -> div and returns all four nodes.
func buildTree(t *testing.T) (doc *dom.Document, htmlNode, body, div *dom.Node) {
	t.Helper()
	doc = dom.NewDocument()
	htmlNode = doc.NewElement("html")
	doc.Node.AppendChild(htmlNode)
	body = doc.NewElement("body")
	htmlNode.AppendChild(body)
	div = doc.NewElement("div")
	body.AppendChild(div)
	return
}

func TestEventCreation(t *testing.T) {
	e := dom.NewEvent("click", true, true)
	if e.Type != "click" {
		t.Errorf("Type = %q, want %q", e.Type, "click")
	}
	if !e.Bubbles() {
		t.Error("Bubbles() = false, want true")
	}
	if !e.Cancelable() {
		t.Error("Cancelable() = false, want true")
	}
	if e.DefaultPrevented() {
		t.Error("DefaultPrevented() = true, want false")
	}
	if e.Target() != nil {
		t.Error("Target() should be nil before dispatch")
	}
	if e.CurrentTarget() != nil {
		t.Error("CurrentTarget() should be nil before dispatch")
	}
	if e.Phase() != dom.NonePhase {
		t.Errorf("Phase() = %d, want NonePhase (0)", e.Phase())
	}
	if e.TimeStamp().IsZero() {
		t.Error("TimeStamp() should not be zero")
	}
}

func TestAddAndDispatchEvent(t *testing.T) {
	_, _, _, div := buildTree(t)

	called := false
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		called = true
	}, false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if !called {
		t.Error("listener was not called")
	}
}

func TestEventBubbling(t *testing.T) {
	_, _, body, div := buildTree(t)

	var order []string
	dom.AddEventListener(body, "click", func(e *dom.Event) {
		order = append(order, "body")
	}, false)
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		order = append(order, "div")
	}, false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if len(order) != 2 || order[0] != "div" || order[1] != "body" {
		t.Errorf("bubble order = %v, want [div, body]", order)
	}
}

func TestEventCapturing(t *testing.T) {
	doc, htmlNode, body, div := buildTree(t)

	var order []string
	dom.AddEventListener(&doc.Node, "click", func(e *dom.Event) {
		order = append(order, "doc")
	}, true)
	dom.AddEventListener(htmlNode, "click", func(e *dom.Event) {
		order = append(order, "html")
	}, true)
	dom.AddEventListener(body, "click", func(e *dom.Event) {
		order = append(order, "body")
	}, true)
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		order = append(order, "div")
	}, true)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	// Capture fires top-down: doc, html, body. Then at-target: div.
	if len(order) != 4 {
		t.Fatalf("capture order length = %d, want 4: %v", len(order), order)
	}
	if order[0] != "doc" || order[1] != "html" || order[2] != "body" || order[3] != "div" {
		t.Errorf("capture order = %v, want [doc, html, body, div]", order)
	}
}

func TestStopPropagation(t *testing.T) {
	_, _, body, div := buildTree(t)

	bodyCalled := false
	dom.AddEventListener(body, "click", func(e *dom.Event) {
		bodyCalled = true
	}, false)
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		e.StopPropagation()
	}, false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if bodyCalled {
		t.Error("body listener should not have been called after stopPropagation")
	}
}

func TestStopImmediatePropagation(t *testing.T) {
	_, _, _, div := buildTree(t)

	secondCalled := false
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		e.StopImmediatePropagation()
	}, false)
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		secondCalled = true
	}, false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if secondCalled {
		t.Error("second listener on same target should not fire after stopImmediatePropagation")
	}
}

func TestPreventDefault(t *testing.T) {
	_, _, _, div := buildTree(t)

	dom.AddEventListener(div, "click", func(e *dom.Event) {
		e.PreventDefault()
	}, false)

	event := dom.NewEvent("click", true, true) // cancelable
	result := dom.DispatchEvent(div, event)

	if result {
		t.Error("DispatchEvent should return false after preventDefault on cancelable event")
	}
	if !event.DefaultPrevented() {
		t.Error("DefaultPrevented() should be true")
	}
}

func TestPreventDefaultOnNonCancelable(t *testing.T) {
	_, _, _, div := buildTree(t)

	dom.AddEventListener(div, "click", func(e *dom.Event) {
		e.PreventDefault()
	}, false)

	event := dom.NewEvent("click", true, false) // NOT cancelable
	result := dom.DispatchEvent(div, event)

	if !result {
		t.Error("DispatchEvent should return true for non-cancelable event even after preventDefault")
	}
	if event.DefaultPrevented() {
		t.Error("DefaultPrevented() should remain false for non-cancelable event")
	}
}

func TestRemoveEventListener(t *testing.T) {
	_, _, _, div := buildTree(t)

	called := false
	cb := func(e *dom.Event) {
		called = true
	}
	dom.AddEventListener(div, "click", cb, false)
	dom.RemoveEventListener(div, "click", cb, false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if called {
		t.Error("removed listener should not fire")
	}
}

func TestMultipleListeners(t *testing.T) {
	_, _, _, div := buildTree(t)

	var order []int
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		order = append(order, 1)
	}, false)
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		order = append(order, 2)
	}, false)
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		order = append(order, 3)
	}, false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if len(order) != 3 {
		t.Fatalf("expected 3 listeners to fire, got %d", len(order))
	}
	for i, v := range order {
		if v != i+1 {
			t.Errorf("order[%d] = %d, want %d", i, v, i+1)
		}
	}
}

func TestCaptureAndBubbleListeners(t *testing.T) {
	_, _, body, div := buildTree(t)

	var order []string
	dom.AddEventListener(body, "click", func(e *dom.Event) {
		order = append(order, "body-capture")
	}, true)
	dom.AddEventListener(body, "click", func(e *dom.Event) {
		order = append(order, "body-bubble")
	}, false)
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		order = append(order, "div-capture")
	}, true)
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		order = append(order, "div-bubble")
	}, false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	// Capture phase: body-capture fires first (top-down).
	// At-target: div-capture then div-bubble (registration order).
	// Bubble phase: body-bubble fires last.
	want := []string{"body-capture", "div-capture", "div-bubble", "body-bubble"}
	if len(order) != len(want) {
		t.Fatalf("order length = %d, want %d: %v", len(order), len(want), order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %q, want %q (full: %v)", i, order[i], want[i], order)
			break
		}
	}
}

func TestMouseEventFields(t *testing.T) {
	me := dom.NewMouseEvent("click", true, true, 10, 20, 100, 200, 2)
	if me.Type != "click" {
		t.Errorf("Type = %q, want %q", me.Type, "click")
	}
	if me.ClientX != 10 {
		t.Errorf("ClientX = %d, want 10", me.ClientX)
	}
	if me.ClientY != 20 {
		t.Errorf("ClientY = %d, want 20", me.ClientY)
	}
	if me.ScreenX != 100 {
		t.Errorf("ScreenX = %d, want 100", me.ScreenX)
	}
	if me.ScreenY != 200 {
		t.Errorf("ScreenY = %d, want 200", me.ScreenY)
	}
	if me.Button != 2 {
		t.Errorf("Button = %d, want 2", me.Button)
	}
	if !me.Bubbles() {
		t.Error("Bubbles() = false, want true")
	}
	if !me.Cancelable() {
		t.Error("Cancelable() = false, want true")
	}

	// Test modifier keys default to false.
	if me.AltKey || me.CtrlKey || me.ShiftKey || me.MetaKey {
		t.Error("modifier keys should default to false")
	}

	// Test that MouseEvent can be dispatched as an Event.
	_, _, _, div := buildTree(t)
	called := false
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		called = true
	}, false)
	dom.DispatchEvent(div, &me.Event)
	if !called {
		t.Error("MouseEvent dispatch should trigger Event listeners")
	}
}

func TestKeyboardEventFields(t *testing.T) {
	ke := dom.NewKeyboardEvent("keydown", true, true, "Enter", "Enter")
	if ke.Type != "keydown" {
		t.Errorf("Type = %q, want %q", ke.Type, "keydown")
	}
	if ke.Key != "Enter" {
		t.Errorf("Key = %q, want %q", ke.Key, "Enter")
	}
	if ke.Code != "Enter" {
		t.Errorf("Code = %q, want %q", ke.Code, "Enter")
	}
	if !ke.Bubbles() {
		t.Error("Bubbles() = false, want true")
	}
	if !ke.Cancelable() {
		t.Error("Cancelable() = false, want true")
	}
	if ke.Repeat {
		t.Error("Repeat should default to false")
	}
	if ke.AltKey || ke.CtrlKey || ke.ShiftKey || ke.MetaKey {
		t.Error("modifier keys should default to false")
	}
}

func TestEventPhase(t *testing.T) {
	doc, htmlNode, body, div := buildTree(t)

	phases := make(map[string]dom.EventPhase)

	dom.AddEventListener(&doc.Node, "click", func(e *dom.Event) {
		phases["doc"] = e.Phase()
	}, true)
	dom.AddEventListener(htmlNode, "click", func(e *dom.Event) {
		phases["html"] = e.Phase()
	}, true)
	dom.AddEventListener(body, "click", func(e *dom.Event) {
		phases["body"] = e.Phase()
	}, false)
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		phases["div"] = e.Phase()
	}, false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if phases["doc"] != dom.CapturingPhase {
		t.Errorf("doc phase = %d, want CapturingPhase (%d)", phases["doc"], dom.CapturingPhase)
	}
	if phases["html"] != dom.CapturingPhase {
		t.Errorf("html phase = %d, want CapturingPhase (%d)", phases["html"], dom.CapturingPhase)
	}
	if phases["div"] != dom.AtTarget {
		t.Errorf("div phase = %d, want AtTarget (%d)", phases["div"], dom.AtTarget)
	}
	if phases["body"] != dom.BubblingPhase {
		t.Errorf("body phase = %d, want BubblingPhase (%d)", phases["body"], dom.BubblingPhase)
	}
}

func TestNonBubblingEvent(t *testing.T) {
	_, _, body, div := buildTree(t)

	bodyCalled := false
	dom.AddEventListener(body, "custom", func(e *dom.Event) {
		bodyCalled = true
	}, false)
	dom.AddEventListener(div, "custom", func(e *dom.Event) {
		// target listener
	}, false)

	event := dom.NewEvent("custom", false, false) // does NOT bubble
	dom.DispatchEvent(div, event)

	if bodyCalled {
		t.Error("non-bubbling event should not reach parent")
	}
}

func TestDispatchReturnsFalseWhenCancelled(t *testing.T) {
	_, _, _, div := buildTree(t)

	dom.AddEventListener(div, "submit", func(e *dom.Event) {
		e.PreventDefault()
	}, false)

	event := dom.NewEvent("submit", false, true) // cancelable
	result := dom.DispatchEvent(div, event)

	if result {
		t.Error("DispatchEvent should return false when event is cancelled")
	}
}

func TestDispatchReturnsTrueWhenNotCancelled(t *testing.T) {
	_, _, _, div := buildTree(t)

	event := dom.NewEvent("click", true, false)
	result := dom.DispatchEvent(div, event)

	if !result {
		t.Error("DispatchEvent should return true when event is not cancelled")
	}
}

func TestDuplicateListenerRejected(t *testing.T) {
	_, _, _, div := buildTree(t)

	count := 0
	cb := func(e *dom.Event) {
		count++
	}
	dom.AddEventListener(div, "click", cb, false)
	dom.AddEventListener(div, "click", cb, false) // duplicate

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if count != 1 {
		t.Errorf("listener fired %d times, want 1 (duplicate should be rejected)", count)
	}
}

func TestStopPropagationDoesNotAffectSameTarget(t *testing.T) {
	_, _, _, div := buildTree(t)

	firstCalled := false
	secondCalled := false
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		firstCalled = true
		e.StopPropagation() // stops bubbling but NOT other listeners on same target
	}, false)
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		secondCalled = true
	}, false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if !firstCalled {
		t.Error("first listener should have been called")
	}
	if !secondCalled {
		t.Error("second listener should still fire (StopPropagation does not stop same-target listeners)")
	}
}

func TestEventTargetSetAfterDispatch(t *testing.T) {
	_, _, _, div := buildTree(t)

	var capturedTarget *dom.Node
	dom.AddEventListener(div, "click", func(e *dom.Event) {
		capturedTarget = e.Target()
	}, false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if capturedTarget != div {
		t.Error("event target should be the div node")
	}
}

func TestCurrentTargetDuringDispatch(t *testing.T) {
	_, _, body, div := buildTree(t)

	var targetDuringBubble *dom.Node
	dom.AddEventListener(body, "click", func(e *dom.Event) {
		targetDuringBubble = e.CurrentTarget()
	}, false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if targetDuringBubble != body {
		t.Error("currentTarget should be body during bubble phase on body")
	}
}

func TestGetNodeListeners(t *testing.T) {
	_, _, _, div := buildTree(t)

	cb1 := func(e *dom.Event) {}
	cb2 := func(e *dom.Event) {}

	dom.AddEventListener(div, "click", cb1, false)
	dom.AddEventListener(div, "click", cb2, true)
	dom.AddEventListener(div, "keydown", cb1, false)

	listeners := dom.GetNodeListeners(div)
	if len(listeners) != 3 {
		t.Fatalf("GetNodeListeners returned %d listeners, want 3", len(listeners))
	}
}

func TestRemoveNonexistentListenerIsNoop(t *testing.T) {
	_, _, _, div := buildTree(t)

	cb1 := func(e *dom.Event) {}
	cb2 := func(e *dom.Event) {}

	dom.AddEventListener(div, "click", cb1, false)
	dom.RemoveEventListener(div, "click", cb2, false) // cb2 was never added

	listeners := dom.GetNodeListeners(div)
	if len(listeners) != 1 {
		t.Errorf("removing nonexistent listener changed count: got %d, want 1", len(listeners))
	}
}
