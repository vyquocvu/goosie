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
	if e.IsTrusted() {
		t.Error("script-created event is trusted, want false")
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

func TestUserAgentEventTrustAndPageTransitionInterface(t *testing.T) {
	trusted := dom.NewTrustedEvent("DOMContentLoaded", true, false)
	if !trusted.IsTrusted() || trusted.InterfaceName() != "Event" {
		t.Fatalf("trusted event state = (%t, %q), want (true, Event)", trusted.IsTrusted(), trusted.InterfaceName())
	}
	page := dom.NewPageTransitionEvent("pageshow", true, true, false)
	if !page.IsTrusted() || page.InterfaceName() != "PageTransitionEvent" || page.Persisted() {
		t.Fatalf("pageshow state = (%t, %q, %t), want (true, PageTransitionEvent, false)", page.IsTrusted(), page.InterfaceName(), page.Persisted())
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

func TestDispatchListenerMutationUsesInvocationSnapshot(t *testing.T) {
	_, _, _, target := buildTree(t)
	var calls []string
	added := func(*dom.Event) { calls = append(calls, "added") }
	var once func(*dom.Event)
	once = func(*dom.Event) {
		calls = append(calls, "once")
		dom.RemoveEventListenerByKey(target, "click", "once", false)
		dom.AddEventListenerWithKey(target, "click", added, false, "added")
	}
	dom.AddEventListener(target, "click", func(*dom.Event) { calls = append(calls, "capture") }, true)
	dom.AddEventListenerWithKey(target, "click", once, false, "once")

	dom.DispatchEvent(target, dom.NewEvent("click", true, false))
	if len(calls) != 2 || calls[0] != "capture" || calls[1] != "once" {
		t.Fatalf("first dispatch calls = %v, want [capture once]", calls)
	}
	calls = nil
	dom.DispatchEvent(target, dom.NewEvent("click", true, false))
	if len(calls) != 2 || calls[0] != "capture" || calls[1] != "added" {
		t.Fatalf("second dispatch calls = %v, want [capture added]", calls)
	}
}

func TestDispatchSkipsListenerRemovedFromActiveSnapshot(t *testing.T) {
	_, _, _, target := buildTree(t)
	var calls []string
	listener2 := func(*dom.Event) { calls = append(calls, "listener2") }
	listener3 := func(*dom.Event) { calls = append(calls, "listener3") }
	listener1 := func(*dom.Event) {
		calls = append(calls, "listener1")
		dom.RemoveEventListenerByKey(target, "change", "listener1", false)
		dom.RemoveEventListenerByKey(target, "change", "listener2", false)
		dom.AddEventListenerWithKey(target, "change", listener3, false, "listener3")
	}
	dom.AddEventListenerWithKey(target, "change", listener1, false, "listener1")
	dom.AddEventListenerWithKey(target, "change", listener2, false, "listener2")

	dom.DispatchEvent(target, dom.NewEvent("change", false, false))
	if len(calls) != 1 || calls[0] != "listener1" {
		t.Fatalf("first dispatch calls = %v, want [listener1]", calls)
	}
	calls = nil
	dom.DispatchEvent(target, dom.NewEvent("change", false, false))
	if len(calls) != 1 || calls[0] != "listener3" {
		t.Fatalf("second dispatch calls = %v, want [listener3]", calls)
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

func TestStopPropagationAtTargetCaptureSkipsTargetBubbleGroup(t *testing.T) {
	_, _, _, target := buildTree(t)
	var order []string
	dom.AddEventListener(target, "click", func(e *dom.Event) {
		order = append(order, "capture-stop")
		e.StopPropagation()
	}, true)
	dom.AddEventListener(target, "click", func(*dom.Event) {
		order = append(order, "capture-sibling")
	}, true)
	dom.AddEventListener(target, "click", func(*dom.Event) {
		order = append(order, "bubble")
	}, false)

	dom.DispatchEvent(target, dom.NewEvent("click", true, false))
	if len(order) != 2 || order[0] != "capture-stop" || order[1] != "capture-sibling" {
		t.Fatalf("target listener order = %v, want [capture-stop capture-sibling]", order)
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

func TestRemoveEventListenerByKey(t *testing.T) {
	_, _, _, div := buildTree(t)

	called := false
	cb := func(e *dom.Event) { called = true }
	key := "test-key-1"
	dom.AddEventListenerWithKey(div, "click", cb, false, key)
	dom.RemoveEventListenerByKey(div, "click", key, false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if called {
		t.Error("removed listener should not fire")
	}
}

func TestRemoveEventListenerByKeyCaptureMismatch(t *testing.T) {
	_, _, _, div := buildTree(t)

	called := false
	cb := func(e *dom.Event) { called = true }
	key := "test-key-capture"
	dom.AddEventListenerWithKey(div, "click", cb, true, key)
	dom.RemoveEventListenerByKey(div, "click", key, false) // capture mismatch

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if !called {
		t.Error("listener should still fire: capture flag does not match")
	}
}

func TestRemoveEventListenerByKeyPreservesOthers(t *testing.T) {
	_, _, _, div := buildTree(t)

	var order []int
	cb1 := func(e *dom.Event) { order = append(order, 1) }
	cb2 := func(e *dom.Event) { order = append(order, 2) }
	dom.AddEventListenerWithKey(div, "click", cb1, false, "key1")
	dom.AddEventListenerWithKey(div, "click", cb2, false, "key2")

	dom.RemoveEventListenerByKey(div, "click", "key1", false)

	event := dom.NewEvent("click", true, false)
	dom.DispatchEvent(div, event)

	if len(order) != 1 || order[0] != 2 {
		t.Errorf("order = %v, want [2] (only second listener should fire)", order)
	}
}

func TestDataTransferGetDataURLAliasesAndURIListParameters(t *testing.T) {
	dt := dom.NewDataTransfer()
	uriList := "# comment\nhttps://a.example/1\nhttps://b.example/2"
	dt.SetData("text/uri-list", uriList)

	if got := dt.GetData(" url "); got != "https://a.example/1" {
		t.Errorf(`GetData(" url ") = %q, want first URI-list URL`, got)
	}
	if got := dt.GetData("text/uri-list;charset=utf-8"); got != uriList {
		t.Errorf("GetData with MIME parameter = %q, want original URI-list payload", got)
	}
}
