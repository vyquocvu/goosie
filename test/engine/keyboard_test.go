package engine_test

import (
	"strings"
	"testing"

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

	sess.PressKey("Enter", false, false, false, false)

	v, _ := rt.Global("__log")
	// Enter on a checkbox toggles it (default action) between keydown and keyup.
	if !strings.Contains(v.Str, "keydown:Enter") || !strings.Contains(v.Str, "keyup:Enter") {
		t.Errorf("__log = %q, want keydown:Enter and keyup:Enter", v.Str)
	}
	if !strings.Contains(v.Str, "change:true") {
		t.Errorf("__log = %q, want change:true from Enter activation", v.Str)
	}
}

// Compile-time guard: engine.EditAction is used elsewhere; ensure import stays.
var _ = engine.EditRune
