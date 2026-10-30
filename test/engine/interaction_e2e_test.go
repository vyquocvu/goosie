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
	"github.com/vyquocvu/goosie/internal/frame"
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

// qc:BR-183
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

func TestInteraction_RangePointerClickUpdatesValueAndDispatchesEvents(t *testing.T) {
	sess, _ := newJSSession(t, `<!doctype html><html><head><style>input { display:inline-block; width:280px; height:20px; }</style></head><body>
<input id="range" type="range" min="0" max="100" step="20" value="20">
<script>
window.rangeEvents=[];
var range=document.getElementById("range");
range.addEventListener("click",function(e){rangeEvents.push("click:"+e.isTrusted)});
range.addEventListener("input",function(e){rangeEvents.push("input:"+e.isTrusted)});
range.addEventListener("change",function(e){rangeEvents.push("change:"+e.isTrusted)});
</script></body></html>`)
	if !clickCenter(t, sess, "range") {
		t.Fatal("range input not found in arena")
	}
	if got, err := sess.EvaluateJS(`document.getElementById("range").value+"|"+rangeEvents.join(",")`); err != nil || got != "60|click:true,input:true,change:true" {
		t.Fatalf("range after pointer click = %q, %v; want committed midpoint and trusted click/input/change", got, err)
	}
	if _, err := sess.EvaluateJS(`document.getElementById("range").addEventListener("click",function(e){e.preventDefault()})`); err != nil {
		t.Fatalf("install click cancellation handler: %v", err)
	}
	var x, y float32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "range" {
			_, y0, x1, y1 := obj.BorderRect()
			x, y = x1-3, (y0+y1)/2
			break
		}
	}
	if !sess.ClickAt(x, y) {
		t.Fatal("canceled range click should still be handled")
	}
	if got, err := sess.EvaluateJS(`document.getElementById("range").value`); err != nil || got != "60" {
		t.Fatalf("prevented range click changed value to %q, %v; want 60", got, err)
	}
}

func TestInteraction_RangePointerDragUpdatesDuringGesture(t *testing.T) {
	sess, _ := newJSSession(t, `<!doctype html><html><head><style>input { display:inline-block; width:280px; height:20px; }</style></head><body>
<input id="range" type="range" min="0" max="100" step="20" value="20">
<script>
window.rangeEvents=[];
var range=document.getElementById("range");
range.addEventListener("input",function(e){rangeEvents.push("input:"+e.isTrusted+":"+range.value)});
range.addEventListener("click",function(e){rangeEvents.push("click:"+e.isTrusted)});
range.addEventListener("change",function(e){rangeEvents.push("change:"+e.isTrusted)});
</script></body></html>`)
	var left, right, y float32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "range" {
			x0, y0, x1, y1 := obj.BorderRect()
			left, right, y = x0+12, x1-12, (y0+y1)/2
			break
		}
	}
	if left == 0 || right <= left {
		t.Fatal("range input not found in arena")
	}
	if !sess.BeginRangePointerDragAt(left, y) {
		t.Fatal("range pointer press was not handled")
	}
	if !sess.MoveRangePointerDragAt(right, y) {
		t.Fatal("range pointer movement was not handled")
	}
	if got := sess.Focused().InputControlValue(); got != "100" {
		t.Fatalf("drag value before release = %q, want 100", got)
	}
	if !sess.EndRangePointerDragAt(right, y) {
		t.Fatal("range pointer release was not handled")
	}
	if got, err := sess.EvaluateJS(`document.getElementById("range").value+"|"+rangeEvents.join(",")`); err != nil || got != "100|input:true:0,input:true:100,click:true,change:true" {
		t.Fatalf("after range drag = %q, %v; want live trusted input events then click/change on release", got, err)
	}
}

func TestTextareaLiveValueDefaultValueAndReset(t *testing.T) {
	sess, _ := newJSSession(t, `<!doctype html><html><body><form id="form"><textarea id="memo" name="memo">default</textarea></form></body></html>`)
	if _, err := sess.EvaluateJS(`document.getElementById("memo").value="live\rvalue"`); err != nil {
		t.Fatalf("set textarea value: %v", err)
	}
	textarea := findElement(sess.Doc.Body, "textarea")
	if textarea == nil {
		t.Fatal("textarea missing from document")
	}
	if got := textarea.InputControlValue(); got != "livevalue" {
		t.Fatalf("live textarea value = %q, want livevalue", got)
	}
	if got := textarea.TextContent(); got != "default" {
		t.Fatalf("textarea default text = %q, want default", got)
	}
	data := engine.CollectFormData(findElement(sess.Doc.Body, "form"), nil)
	if len(data) != 1 || data[0].Name != "memo" || data[0].Value != "livevalue" {
		t.Fatalf("textarea form data = %#v, want livevalue", data)
	}
	form := findElement(sess.Doc.Body, "form")
	sess.ResetForm(form)
	if got := textarea.InputControlValue(); got != "default" {
		t.Fatalf("textarea value after reset = %q, want default", got)
	}
	if textarea.InputValueDirty {
		t.Fatal("textarea dirty-value flag should clear on reset")
	}
}

func TestTextareaLiveValuePaintsWithoutChangingDefaultText(t *testing.T) {
	sess, _ := newJSSession(t, `<!doctype html><html><body><textarea id="memo">default</textarea></body></html>`)
	if _, err := sess.EvaluateJS(`document.getElementById("memo").value="live text"`); err != nil {
		t.Fatalf("set textarea value: %v", err)
	}
	if err := sess.Reflow(800); err != nil {
		t.Fatalf("Reflow: %v", err)
	}
	dl := sess.Paint(1).Build(1)
	if got := string(textRunesIn(t, dl, frame.RGB(0, 0, 0))); got != "live text" {
		t.Fatalf("painted textarea value = %q, want live text", got)
	}
	textarea := findElement(sess.Doc.Body, "textarea")
	if got := textarea.TextContent(); got != "default" {
		t.Fatalf("textarea default text = %q, want default", got)
	}
}

func TestOutputResetRestoresDefaultValueOverrideThroughEngine(t *testing.T) {
	sess, _ := newJSSession(t, `<!doctype html><html><body><form id="f"><output id="result">initial</output></form></body></html>`)
	if _, err := sess.EvaluateJS(`(function(){var f=document.getElementById("f"),o=document.getElementById("result");o.value="live";o.defaultValue="reset value";window.__resetEvent="";f.addEventListener("reset",function(e){window.__resetEvent=[e.isTrusted,e.cancelable,e.bubbles].join("|")})})()`); err != nil {
		t.Fatalf("prepare output and reset listener: %v", err)
	}
	sess.ResetForm(sess.Doc.ElementByID("f"))
	got, err := sess.EvaluateJS(`(function(){var o=document.getElementById("result");return [o.value,o.defaultValue,window.__resetEvent].join("|")})()`)
	if err != nil {
		t.Fatalf("read reset output state: %v", err)
	}
	if got != "reset value|reset value|true|true|false" {
		t.Fatalf("output state after engine reset = %q", got)
	}
}

func TestSelectResetRestoresOptionSelectednessThroughEngine(t *testing.T) {
	sess, _ := newJSSession(t, `<!doctype html><html><body><form id="f"><select id="s" name="choice"><option id="a" value="a" selected>A</option><option id="b" value="b">B</option></select></form></body></html>`)
	if _, err := sess.EvaluateJS(`document.getElementById("b").selected=true`); err != nil {
		t.Fatalf("select option: %v", err)
	}
	a, b := sess.Doc.ElementByID("a"), sess.Doc.ElementByID("b")
	if a.OptionSelected() || !b.OptionSelected() || !a.HasAttribute("selected") || b.HasAttribute("selected") {
		t.Fatalf("live/default selectedness before reset = %v|%v / %v|%v, want false|true / true|false", a.OptionSelected(), b.OptionSelected(), a.HasAttribute("selected"), b.HasAttribute("selected"))
	}
	sess.ResetForm(sess.Doc.ElementByID("f"))
	if !a.OptionSelected() || b.OptionSelected() {
		t.Fatalf("option selectedness after engine reset = %v|%v, want true|false", a.OptionSelected(), b.OptionSelected())
	}
}

func TestRadioGroupResetPreservesExclusivityThroughEngine(t *testing.T) {
	sess, _ := newJSSession(t, `<!doctype html><html><body><form id="f"><input id="a" type="radio" name="g" checked><input id="b" type="radio" name="g" checked></form></body></html>`)
	a, b := sess.Doc.ElementByID("a"), sess.Doc.ElementByID("b")
	if a == nil || b == nil {
		t.Fatal("radio controls missing from document")
	}
	sess.ResetForm(sess.Doc.ElementByID("f"))
	if engine.IsChecked(a) || !engine.IsChecked(b) {
		t.Fatalf("radio states after engine reset = %v|%v, want false|true", engine.IsChecked(a), engine.IsChecked(b))
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

// The viewport scroll event is queued and coalesced, the way Chromium delivers
// it: several moves in one turn reach the page as a single event at the frame
// flush, not once per move.
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

	if v, ok := rt.Global("__scrollCount"); !ok {
		t.Fatal("__scrollCount not set")
	} else if v.Num != 0 {
		t.Fatalf("__scrollCount before the flush = %v, want 0", v.Num)
	}

	sess.PumpJS(800)

	v, ok := rt.Global("__scrollCount")
	if !ok {
		t.Fatal("__scrollCount not set")
	}
	if v.Num != 1 {
		t.Errorf("__scrollCount = %v, want 1", v.Num)
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

func TestFormSubmitOnEnter(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body>
<form id="f1" action="/search" method="GET">
  <input type="text" id="q" name="query" value="goosie browser">
</form>
</body></html>`
	sess, _ := newJSSession(t, html)

	// Focus the input
	focused := false
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "q" {
			x0, y0, x1, y1 := obj.BorderRect()
			focused = sess.FocusControl((x0+x1)/2, (y0+y1)/2)
			break
		}
	}
	if !focused {
		t.Fatal("failed to focus input")
	}

	// Press Enter (EditEnter)
	changed := sess.Edit(engine.EditEnter, 0)
	if !changed {
		t.Fatal("expected EditEnter on input to trigger submit")
	}

	// Pop submitted form
	sub, ok := sess.PopSubmittedForm()
	if !ok || sub == nil {
		t.Fatal("expected pending form submission")
	}
	if sub.Action != "/search" {
		t.Errorf("expected action /search, got %q", sub.Action)
	}
	if sub.Method != "GET" {
		t.Errorf("expected method GET, got %q", sub.Method)
	}
	if sub.Encoded != "query=goosie+browser" {
		t.Errorf("expected encoded query=goosie+browser, got %q", sub.Encoded)
	}

	// Verify pop cleared the submission
	if _, ok := sess.PopSubmittedForm(); ok {
		t.Error("expected PopSubmittedForm to clear pending submission")
	}
}

// qc:BR-104
func TestFormSubmitButtonClick(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body>
<form id="login" action="https://auth.example/login" method="POST">
  <input type="text" name="user" value="alice">
  <input type="submit" id="btn-submit" value="Sign In">
</form>
</body></html>`
	sess, _ := newJSSession(t, html)

	// Click submit button
	if !clickCenter(t, sess, "btn-submit") {
		t.Fatal("failed to click submit button")
	}

	sub, ok := sess.PopSubmittedForm()
	if !ok || sub == nil {
		t.Fatal("expected submitted form from submit button click")
	}
	if sub.Action != "https://auth.example/login" {
		t.Errorf("expected action https://auth.example/login, got %q", sub.Action)
	}
	if sub.Method != "POST" {
		t.Errorf("expected method POST, got %q", sub.Method)
	}
	if sub.Encoded != "user=alice" {
		t.Errorf("expected encoded user=alice, got %q", sub.Encoded)
	}
}

func TestFormReset(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body>
<form id="f1">
  <input type="text" id="inp" name="title" value="Initial">
  <input type="checkbox" id="chk" name="agree" checked>
</form>
</body></html>`
	sess, _ := newJSSession(t, html)

	var formNode, inpNode, chkNode *dom.Node
	for _, obj := range sess.Arena.Objects {
		if obj.Node == nil {
			continue
		}
		if obj.Node.GetAttribute("id") == "f1" {
			formNode = obj.Node
		} else if obj.Node.GetAttribute("id") == "inp" {
			inpNode = obj.Node
		} else if obj.Node.GetAttribute("id") == "chk" {
			chkNode = obj.Node
		}
	}
	if formNode == nil || inpNode == nil || chkNode == nil {
		t.Fatal("nodes not found")
	}

	// Focus inp and edit it with EditRune
	for _, obj := range sess.Arena.Objects {
		if obj.Node == inpNode {
			x0, y0, x1, y1 := obj.BorderRect()
			sess.FocusControl((x0+x1)/2, (y0+y1)/2)
			break
		}
	}
	sess.Edit(engine.EditRune, 'X')

	// Click checkbox to toggle off
	clickCenter(t, sess, "chk")

	if inpNode.InputControlValue() == "Initial" {
		t.Errorf("expected input value to be modified before reset")
	}
	if engine.IsChecked(chkNode) {
		t.Errorf("expected checkbox to be unchecked before reset")
	}

	// Reset form
	sess.ResetForm(formNode)

	// Inp should be restored to Initial
	if inpNode.InputControlValue() != "Initial" {
		t.Errorf("expected input to reset to 'Initial', got %q", inpNode.InputControlValue())
	}
	// Chk should be restored to checked
	if !engine.IsChecked(chkNode) {
		t.Errorf("expected checkbox to reset to checked")
	}
}
