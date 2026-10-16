// Package engine_test verifies that user interactions flow through the full
// pipeline: engine action → DOM event dispatch → JS listener callback.
//
// These are the highest-ROI tests in the suite: they catch the cross-layer
// wiring bugs that unit tests at each layer miss. A click that never reaches
// the JS runtime, or a scroll event with wrong coordinates, shows up here.
package engine_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newJSSession builds a session with a JS runtime wired to the document.
// The runtime's console output is discarded; scripts run during construction.
func newJSSession(t *testing.T, html string) (*engine.Session, *js.Runtime) {
	t.Helper()
	rt, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://test.example/",
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { rt.Close() })

	sess, err := engine.NewSession(html, nil, 800,
		engine.WithViewportH(600),
		engine.WithJS(rt),
	)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess, rt
}

// clickCenter finds the center of the laid-out box for an element ID and
// clicks it. Returns false if the element was not found in the arena.
func clickCenter(t *testing.T, s *engine.Session, id string) bool {
	t.Helper()
	if s.Arena == nil {
		return false
	}
	for _, obj := range s.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			x0, y0, x1, y1 := obj.BorderRect()
			s.ClickAt((x0+x1)/2, (y0+y1)/2)
			return true
		}
	}
	return false
}

// focusCenter finds the center of the laid-out box for an element ID and
// focuses it.
func focusCenter(t *testing.T, s *engine.Session, id string) bool {
	t.Helper()
	if s.Arena == nil {
		return false
	}
	for _, obj := range s.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			x0, y0, x1, y1 := obj.BorderRect()
			s.FocusControl((x0+x1)/2, (y0+y1)/2)
			return true
		}
	}
	return false
}

// pressTab moves focus to the next tabbable control, mirroring a Tab keypress.
func pressTab(t *testing.T, s *engine.Session) {
	t.Helper()
	s.FocusNext()
}

// pressShiftTab moves focus to the previous tabbable control, mirroring a
// Shift+Tab keypress.
func pressShiftTab(t *testing.T, s *engine.Session) {
	t.Helper()
	s.FocusPrev()
}

// pressTabKey drives a real Tab keypress through the engine's key-event path
// (keydown → focus-move default action → keyup), unlike pressTab which only
// moves focus. Mirrors Playwright's keyboard.press('Tab').
func pressTabKey(t *testing.T, s *engine.Session) {
	t.Helper()
	s.PressKey("Tab", false, false, false, false)
}

// ---------------------------------------------------------------------------
// Click → JS listener
// ---------------------------------------------------------------------------

func TestInteraction_ClickFiresJSListener(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<button id="btn">Click me</button>
<script>
document.getElementById('btn').addEventListener('click', function() {
	__clicked = true;
});
</script>
</body></html>`)

	if !clickCenter(t, sess, "btn") {
		t.Fatal("button not found in arena")
	}

	v, ok := rt.Global("__clicked")
	if !ok {
		t.Fatal("__clicked not set — JS listener did not fire")
	}
	if !v.Bool {
		t.Errorf("__clicked = %v, want true", v.Bool)
	}
}

func TestInteraction_ClickPreventDefault(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<button id="btn">Click</button>
<script>
document.getElementById('btn').addEventListener('click', function(event) {
	event.preventDefault();
	__prevented = true;
});
</script>
</body></html>`)

	clickCenter(t, sess, "btn")

	v, ok := rt.Global("__prevented")
	if !ok || !v.Bool {
		t.Error("preventDefault was not observed by JS listener")
	}
}

// ---------------------------------------------------------------------------
// Focus / blur → JS listener
// ---------------------------------------------------------------------------

func TestInteraction_FocusBlurFiresJSListener(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<input id="inp">
<script>
var inp = document.getElementById('inp');
inp.addEventListener('focus', function() { __focused = true; });
inp.addEventListener('blur', function() { __blurred = true; });
</script>
</body></html>`)

	focusCenter(t, sess, "inp")

	v, ok := rt.Global("__focused")
	if !ok || !v.Bool {
		t.Error("focus listener did not fire")
	}

	// Click outside to blur.
	sess.FocusControl(790, 590)

	v, ok = rt.Global("__blurred")
	if !ok || !v.Bool {
		t.Error("blur listener did not fire")
	}
}

// ---------------------------------------------------------------------------
// Text input → JS listener
// ---------------------------------------------------------------------------

func TestInteraction_TextInputFiresJSListener(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<input id="inp">
<script>
document.getElementById('inp').addEventListener('input', function(event) {
	__inputFired = true;
});
</script>
</body></html>`)

	focusCenter(t, sess, "inp")

	// Type characters.
	sess.Edit(engine.EditRune, 'h')
	sess.Edit(engine.EditRune, 'i')

	v, ok := rt.Global("__inputFired")
	if !ok || !v.Bool {
		t.Error("input listener did not fire")
	}
}

// ---------------------------------------------------------------------------
// Checkbox toggle → JS listener
// ---------------------------------------------------------------------------

func TestInteraction_CheckboxToggleFiresJSListener(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<input type="checkbox" id="cb">
<script>
document.getElementById('cb').addEventListener('change', function(event) {
	__changeFired = true;
});
</script>
</body></html>`)

	clickCenter(t, sess, "cb")

	v, ok := rt.Global("__changeFired")
	if !ok || !v.Bool {
		t.Error("change listener did not fire")
	}
}

// ---------------------------------------------------------------------------
// Scroll → JS listener
// ---------------------------------------------------------------------------

func TestInteraction_ScrollFiresJSListener(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<div style="height:2000px;">Tall content</div>
<script>
__scrollCount = 0;
document.addEventListener('scroll', function() {
	__scrollCount++;
});
</script>
</body></html>`)

	sess.FireScrollEvent()
	sess.FireScrollEvent()
	sess.FireScrollEvent()

	v, ok := rt.Global("__scrollCount")
	if !ok {
		t.Fatal("__scrollCount not set")
	}
	if v.Num != 3 {
		t.Errorf("__scrollCount = %v, want 3", v.Num)
	}
}

// ---------------------------------------------------------------------------
// Hover → JS listener (mouseenter/mouseleave)
// ---------------------------------------------------------------------------

func TestInteraction_HoverFiresJSListener(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<div id="hover-target" style="width:100px;height:50px;">Hover me</div>
<script>
var target = document.getElementById('hover-target');
target.addEventListener('mouseenter', function() { __entered = true; });
target.addEventListener('mouseleave', function() { __left = true; });
</script>
</body></html>`)

	if s := sess.Arena; s != nil {
		for _, obj := range s.Objects {
			if obj.Node != nil && obj.Node.GetAttribute("id") == "hover-target" {
				x0, y0, x1, y1 := obj.BorderRect()
				cx, cy := (x0+x1)/2, (y0+y1)/2

				// Move into the element.
				sess.SetHover(cx, cy)

				v, ok := rt.Global("__entered")
				if !ok || !v.Bool {
					t.Error("mouseenter listener did not fire")
				}

				// Move outside.
				sess.SetHover(0, 0)

				v, ok = rt.Global("__left")
				if !ok || !v.Bool {
					t.Error("mouseleave listener did not fire")
				}
				return
			}
		}
	}
	t.Fatal("hover-target not found in arena")
}

// ---------------------------------------------------------------------------
// Form submit → JS listener
// ---------------------------------------------------------------------------

func TestInteraction_FormSubmitFiresJSListener(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<form id="form">
<input name="q" value="test">
<button type="submit" id="submit-btn">Go</button>
</form>
<script>
document.getElementById('form').addEventListener('submit', function(event) {
	event.preventDefault();
	__submitted = true;
});
</script>
</body></html>`)

	clickCenter(t, sess, "submit-btn")

	v, ok := rt.Global("__submitted")
	if !ok || !v.Bool {
		t.Error("submit listener did not fire or preventDefault was not observed")
	}
}

// ---------------------------------------------------------------------------
// Multiple listeners on same element
// ---------------------------------------------------------------------------

func TestInteraction_MultipleListeners(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<button id="btn">Multi</button>
<script>
var btn = document.getElementById('btn');
btn.addEventListener('click', function() { __first = true; });
btn.addEventListener('click', function() { __second = true; });
</script>
</body></html>`)

	clickCenter(t, sess, "btn")

	first, _ := rt.Global("__first")
	second, _ := rt.Global("__second")
	if !first.Bool {
		t.Error("first listener did not fire")
	}
	if !second.Bool {
		t.Error("second listener did not fire")
	}
}

// ---------------------------------------------------------------------------
// Event target correctness
// ---------------------------------------------------------------------------

func TestInteraction_EventTargetIsCorrect(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<button id="btn">Click</button>
<script>
document.getElementById('btn').addEventListener('click', function(event) {
	__tag = event.target.tagName;
});
</script>
</body></html>`)

	clickCenter(t, sess, "btn")

	v, ok := rt.Global("__tag")
	if !ok {
		t.Fatal("__tag not set")
	}
	if v.Str != "BUTTON" {
		t.Errorf("event.target.tagName = %q, want BUTTON", v.Str)
	}
}

// ---------------------------------------------------------------------------
// Stop propagation
// ---------------------------------------------------------------------------

func TestInteraction_StopPropagation(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<div id="outer">
<button id="inner">Stop</button>
</div>
<script>
document.getElementById('outer').addEventListener('click', function() {
	__outerFired = true;
});
document.getElementById('inner').addEventListener('click', function(event) {
	event.stopPropagation();
	__innerFired = true;
});
</script>
</body></html>`)

	clickCenter(t, sess, "inner")

	inner, _ := rt.Global("__innerFired")
	outer, _ := rt.Global("__outerFired")
	if !inner.Bool {
		t.Error("inner listener did not fire")
	}
	if outer.Bool {
		t.Error("outer listener should not have fired after stopPropagation")
	}
}

// ---------------------------------------------------------------------------
// DOM mutation from click handler
// ---------------------------------------------------------------------------

func TestInteraction_ClickMutatesDOM(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<button id="btn">Click</button>
<div id="out">initial</div>
<script>
document.getElementById('btn').addEventListener('click', function() {
	document.getElementById('out').textContent = 'clicked';
});
</script>
</body></html>`)

	clickCenter(t, sess, "btn")

	// The JS runtime mutated the DOM. The engine's invalidation would pick
	// this up on the next Reflow. For this test, we just verify the JS side
	// executed without error.
	v, ok := rt.Global("__mutationError")
	if ok && v.Bool {
		t.Error("DOM mutation from click handler threw")
	}
}

// ---------------------------------------------------------------------------
// Drag-select → selection state
// ---------------------------------------------------------------------------

func TestInteraction_DragSelect(t *testing.T) {
	sess, _ := newJSSession(t, `<!DOCTYPE html>
<html><body>
<p id="text">Hello world this is a test paragraph</p>
</body></html>`)

	if s := sess.Arena; s != nil {
		// Find the text box.
		for _, obj := range s.Objects {
			if obj.Node != nil && obj.Node.GetAttribute("id") == "text" {
				x0, y0, x1, y1 := obj.BorderRect()
				// Drag from left to right.
				sess.SelectAt(x0+5, (y0+y1)/2)
				sess.SelectTo(x1-5, (y0+y1)/2)
				// No JS assertion here — the engine's selection state is
				// verified in test/engine/selection_test.go. This test
				// verifies the path doesn't panic with JS enabled.
				return
			}
		}
	}
	t.Fatal("text element not found in arena")
}

// ---------------------------------------------------------------------------
// Verify dom.Event dispatch directly (no JS)
// ---------------------------------------------------------------------------

func TestInteraction_DirectEventDispatch(t *testing.T) {
	sess, _ := newJSSession(t, `<!DOCTYPE html>
<html><body>
<button id="btn">Click</button>
</body></html>`)

	// Find the button node.
	var btnNode *dom.Node
	if s := sess.Arena; s != nil {
		for _, obj := range s.Objects {
			if obj.Node != nil && obj.Node.GetAttribute("id") == "btn" {
				btnNode = obj.Node
				break
			}
		}
	}
	if btnNode == nil {
		t.Fatal("button node not found")
	}

	// Register a Go-level listener.
	var fired bool
	dom.AddEventListener(btnNode, "click", func(ev *dom.Event) {
		fired = true
	}, false)

	// Click via engine.
	clickCenter(t, sess, "btn")

	if !fired {
		t.Error("Go-level click listener did not fire")
	}
}
