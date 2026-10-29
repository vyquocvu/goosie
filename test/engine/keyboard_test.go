package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// ---------------------------------------------------------------------------
// GOO-04: real keyboard-event injection.
//
// These verify the engine's PressKey path: keydown on the old focus → default
// action (focus move / activation) → keyup on the new focus, with event.key
// observable to JS, preventDefault able to block the default action, and Enter
// activating the focused control.
// ---------------------------------------------------------------------------

// TestKeyboard_EventKeyObservable asserts event.key reaches a JS listener.
func TestKeyboard_EventKeyObservable(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<input id="a">
<script>
__keys = '';
document.getElementById('a').addEventListener('keydown', function(e){ __keys += 'd:'+e.key+','; });
document.getElementById('a').addEventListener('keyup', function(e){ __keys += 'u:'+e.key+','; });
</script>
</body></html>`)

	focusCenter(t, sess, "a")
	sess.PressKey("Tab", false, false, false, false)

	v, _ := rt.Global("__keys")
	want := "d:Tab,u:Tab,"
	if v.Str != want {
		t.Errorf("__keys = %q, want %q", v.Str, want)
	}
}

// TestKeyboard_TabMovesFocusWithKeyOrder asserts the full ordering: keydown on
// the source, then blur/focus, then keyup on the destination.
func TestKeyboard_TabMovesFocusWithKeyOrder(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<input id="a"><input id="b">
<script>
__log = '';
function L(s){ if(__log) __log += ','; __log += s; }
['a','b'].forEach(function(id){ var el=document.getElementById(id);
  el.addEventListener('keydown',function(e){L(id+':keydown:'+e.key);});
  el.addEventListener('keyup',function(e){L(id+':keyup:'+e.key);});
  el.addEventListener('focus',function(){L(id+':focus');});
  el.addEventListener('blur',function(){L(id+':blur');});
});
</script>
</body></html>`)

	focusCenter(t, sess, "a")
	sess.PressKey("Tab", false, false, false, false)

	v, _ := rt.Global("__log")
	want := "a:focus,a:keydown:Tab,a:blur,b:focus,b:keyup:Tab"
	if v.Str != want {
		t.Errorf("__log = %q,\n want  %q", v.Str, want)
	}
}

// TestKeyboard_TabFromNothingFocusesFirst asserts Tab with no focused control
// still drives keydown and starts the cycle at the first tabbable.
func TestKeyboard_TabFromNothingFocusesFirst(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<input id="a"><input id="b">
<script>
__log = '';
function L(s){ if(__log) __log += ','; __log += s; }
['a','b'].forEach(function(id){ var el=document.getElementById(id);
  el.addEventListener('focus',function(){L(id+':focus');});
  el.addEventListener('blur',function(){L(id+':blur');});
});
</script>
</body></html>`)

	sess.PressKey("Tab", false, false, false, false)

	if f := sess.Focused(); f == nil || f.GetAttribute("id") != "a" {
		var id string
		if f != nil {
			id = f.GetAttribute("id")
		}
		t.Errorf("after Tab from nothing, focus = %q, want a", id)
	}
	v, _ := rt.Global("__log")
	if v.Str != "a:focus" {
		t.Errorf("__log = %q, want a:focus", v.Str)
	}
}

// TestKeyboard_ShiftTabMovesBack asserts Shift+Tab moves to the previous
// control and fires blur on the source before focus on the destination.
func TestKeyboard_ShiftTabMovesBack(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<input id="a"><input id="b">
<script>
__log = '';
function L(s){ if(__log) __log += ','; __log += s; }
['a','b'].forEach(function(id){ var el=document.getElementById(id);
  el.addEventListener('keydown',function(e){L(id+':keydown:'+e.key);});
  el.addEventListener('keyup',function(e){L(id+':keyup:'+e.key);});
  el.addEventListener('focus',function(){L(id+':focus');});
  el.addEventListener('blur',function(){L(id+':blur');});
});
</script>
</body></html>`)

	focusCenter(t, sess, "b")
	sess.PressKey("Tab", true, false, false, false) // shift

	v, _ := rt.Global("__log")
	want := "b:focus,b:keydown:Tab,b:blur,a:focus,a:keyup:Tab"
	if v.Str != want {
		t.Errorf("__log = %q,\n want  %q", v.Str, want)
	}
}

// TestKeyboard_PreventDefaultBlocksTab asserts a keydown listener that calls
// preventDefault stops the focus move but keyup still fires on the source.
func TestKeyboard_PreventDefaultBlocksTab(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<input id="a"><input id="b">
<script>
__log = '';
function L(s){ if(__log) __log += ','; __log += s; }
var a = document.getElementById('a');
a.addEventListener('keydown',function(e){ L('a:keydown'); e.preventDefault(); });
a.addEventListener('keyup',function(e){ L('a:keyup:'+e.key); });
a.addEventListener('blur',function(){ L('a:blur'); });
document.getElementById('b').addEventListener('focus',function(){ L('b:focus'); });
</script>
</body></html>`)

	focusCenter(t, sess, "a")
	sess.PressKey("Tab", false, false, false, false)

	v, _ := rt.Global("__log")
	// No a:blur, no b:focus — the default action was prevented.
	want := "a:keydown,a:keyup:Tab"
	if v.Str != want {
		t.Errorf("__log = %q, want %q", v.Str, want)
	}
	if f := sess.Focused(); f == nil || f.GetAttribute("id") != "a" {
		t.Error("focus should remain on a after prevented Tab")
	}
}

// TestKeyboard_EnterActivates asserts Enter on a focused checkbox toggles it
// via the shared click default action, and the keydown/keyup both fire.
func TestKeyboard_EnterActivates(t *testing.T) {
	sess, rt := newJSSession(t, `<!DOCTYPE html>
<html><body>
<input type="checkbox" id="cb">
<script>
__log = '';
function L(s){ if(__log) __log += ','; __log += s; }
var cb = document.getElementById('cb');
cb.addEventListener('keydown',function(e){ L('keydown:'+e.key); });
cb.addEventListener('keyup',function(e){ L('keyup:'+e.key); });
cb.addEventListener('change',function(){ L('change:'+ (cb.checked ? 'true':'false')); });
</script>
</body></html>`)

	focusCenter(t, sess, "cb")

	if !sess.PressKey("Enter", false, false, false, false) {
		t.Fatal("Enter activation default was not performed")
	}

	v, _ := rt.Global("__log")
	// Enter on a checkbox toggles it (default action) between keydown and keyup.
	if !strings.Contains(v.Str, "keydown:Enter") || !strings.Contains(v.Str, "keyup:Enter") {
		t.Errorf("__log = %q, want keydown:Enter and keyup:Enter", v.Str)
	}
	if !strings.Contains(v.Str, "change:true") {
		t.Errorf("__log = %q, want change:true from Enter activation", v.Str)
	}
}

// qc:BR-182
func TestKeyboard_ActivationTracksActiveAndDispatchesClick(t *testing.T) {
	sess, _ := newJSSession(t, `<!DOCTYPE html><html><body>
<div id="parent"><button id="target">Activate</button></div>
</body></html>`)
	target := nodeByID(&sess.Doc.Node, "target")
	parent := nodeByID(&sess.Doc.Node, "parent")
	if target == nil || parent == nil {
		t.Fatal("activation nodes not found")
	}
	if !sess.FocusNode(target) {
		t.Fatal("could not focus target")
	}
	var downActive, clickActive, upActive bool
	clicks := 0
	dom.AddEventListener(target, "keydown", func(*dom.Event) {
		downActive = css.IsActive(target) && css.IsActive(parent)
	}, false)
	dom.AddEventListener(target, "click", func(*dom.Event) {
		clicks++
		clickActive = css.IsActive(target) && css.IsActive(parent)
	}, false)
	dom.AddEventListener(target, "keyup", func(*dom.Event) {
		upActive = css.IsActive(target) && css.IsActive(parent)
	}, false)

	sess.PressKey("Enter", false, false, false, false)
	if !downActive || !clickActive || !upActive {
		t.Fatalf(":active during key operation: keydown=%v click=%v keyup=%v", downActive, clickActive, upActive)
	}
	if clicks != 1 {
		t.Fatalf("click events = %d, want 1", clicks)
	}
	if css.IsActive(target) || css.IsActive(parent) {
		t.Fatal(":active remained after keyboard activation completed")
	}

	// Space activates buttons as well; the same transient active state wraps
	// the complete key operation and the generated click event.
	downActive, clickActive, upActive = false, false, false
	if !sess.PressKey(" ", false, false, false, false) {
		t.Fatal("Space activation default was not performed")
	}
	if !downActive || !clickActive || !upActive || clicks != 2 {
		t.Fatalf("Space activation: keydown=%v click=%v keyup=%v clicks=%d", downActive, clickActive, upActive, clicks)
	}
}

func TestKeyboard_PreventDefaultBlocksSpaceActivation(t *testing.T) {
	sess, _ := newJSSession(t, `<!DOCTYPE html><html><body><input type="checkbox" id="cb"></body></html>`)
	if !focusCenter(t, sess, "cb") {
		t.Fatal("could not focus checkbox")
	}
	cb := nodeByID(&sess.Doc.Node, "cb")
	if cb == nil {
		t.Fatal("checkbox not found")
	}
	clicks := 0
	dom.AddEventListener(cb, "keydown", func(e *dom.Event) { e.PreventDefault() }, false)
	dom.AddEventListener(cb, "click", func(*dom.Event) { clicks++ }, false)
	if sess.PressKey(" ", false, false, false, false) {
		t.Fatal("prevented Space reported an activation")
	}
	if clicks != 0 || engine.IsChecked(cb) {
		t.Fatalf("prevented Space produced clicks=%d checked=%v; want no activation", clicks, engine.IsChecked(cb))
	}
	if css.IsActive(cb) {
		t.Fatal(":active remained after prevented key operation")
	}
}

func TestKeyboard_HeldSpaceActiveStateClearsOnSequentialFocusChange(t *testing.T) {
	sess, _ := newJSSession(t, `<!DOCTYPE html><html><body>
<input type="checkbox" id="cb"><input id="next">
</body></html>`)
	cb := nodeByID(&sess.Doc.Node, "cb")
	if cb == nil || !sess.FocusNode(cb) {
		t.Fatal("could not find and focus checkbox")
	}
	sess.SetKeyboardActive(cb, true)
	if !css.IsActive(cb) {
		t.Fatal("held Space did not mark checkbox :active")
	}
	sess.FocusNext()
	if f := sess.Focused(); f == nil || f.GetAttribute("id") != "next" {
		t.Fatalf("Tab focus target = %v, want next input", f)
	}
	if css.IsActive(cb) {
		t.Fatal(":active remained after sequential focus moved away")
	}
}

func TestKeyboard_RadioGroupHasOneSequentialFocusStop(t *testing.T) {
	sess, _ := newJSSession(t, `<!DOCTYPE html><html><body>
<input id="start">
<form>
  <input type="radio" name="group" id="a">
  <button id="middle">middle</button>
  <input type="radio" name="group" id="b" checked>
  <input type="radio" name="group" id="c">
</form>
<input id="end">
</body></html>`)
	start := nodeByID(&sess.Doc.Node, "start")
	if start == nil || !sess.FocusNode(start) {
		t.Fatal("could not focus start input")
	}
	sess.FocusNext()
	if f := sess.Focused(); f == nil || f.GetAttribute("id") != "middle" {
		t.Fatalf("first Tab target = %v, want the control before the checked radio", f)
	}
	sess.FocusNext()
	if f := sess.Focused(); f == nil || f.GetAttribute("id") != "b" {
		t.Fatalf("second Tab target = %v, want checked radio b", f)
	}
	sess.FocusNext()
	if f := sess.Focused(); f == nil || f.GetAttribute("id") != "end" {
		t.Fatalf("third Tab target = %v, want end input", f)
	}
	// A programmatically focused non-selected member moves to the group's
	// sequential stop when Tab is pressed.
	a := nodeByID(&sess.Doc.Node, "a")
	if a == nil || !sess.FocusNode(a) {
		t.Fatal("could not programmatically focus radio a")
	}
	sess.FocusNext()
	if f := sess.Focused(); f == nil || f.GetAttribute("id") != "b" {
		t.Fatalf("Tab after programmatic radio focus = %v, want checked member b", f)
	}
}

func TestKeyboard_EnterLinkActivationHonorsClickPreventDefault(t *testing.T) {
	sess, _ := newJSSession(t, `<!DOCTYPE html><html><body><a id="link" href="/next">next</a></body></html>`)
	link := nodeByID(&sess.Doc.Node, "link")
	if link == nil || !sess.FocusNode(link) {
		t.Fatal("could not find and focus link")
	}
	clicks := 0
	prevent := false
	dom.AddEventListener(link, "click", func(e *dom.Event) {
		clicks++
		if prevent {
			e.PreventDefault()
		}
	}, false)
	if !sess.PressKey("Enter", false, false, false, false) {
		t.Fatal("uncanceled link click did not report activation")
	}
	if clicks != 1 {
		t.Fatalf("link click events = %d, want 1", clicks)
	}
	prevent = true
	if sess.PressKey("Enter", false, false, false, false) {
		t.Fatal("canceled link click reported activation")
	}
	if clicks != 2 {
		t.Fatalf("canceled link click events = %d, want 2 dispatched events", clicks)
	}
}

func nodeByID(n *dom.Node, id string) *dom.Node {
	if n == nil {
		return nil
	}
	if n.Element() && n.GetAttribute("id") == id {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := nodeByID(child, id); found != nil {
			return found
		}
	}
	return nil
}

// Compile-time guard: engine.EditAction is used elsewhere; ensure import stays.
var _ = engine.EditRune
