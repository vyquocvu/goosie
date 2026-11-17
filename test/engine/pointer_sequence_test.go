package engine_test

import (
	"strings"
	"testing"
)

func TestHostPressDispatchesPointerAndMouseSequence(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0">
		<div id="target" style="position:absolute;left:10px;top:10px;width:120px;height:60px;background:#eee">x</div>
	</body></html>`)
	if _, err := s.EvaluateJS(`(function(){
		window.evts = [];
		var el = document.getElementById('target');
		['pointerdown','mousedown','pointerup','mouseup','click'].forEach(function(typ){
			el.addEventListener(typ, function(e){
				var kind = e instanceof PointerEvent ? 'pointer:' + e.pointerType + ':' + e.buttons : 'mouse:' + e.buttons + ':' + e.detail;
				window.evts.push(typ + ':' + e.isTrusted + ':' + kind + ':' + e.clientX + ',' + e.clientY);
			});
		});
	})()`); err != nil {
		t.Fatalf("install listeners: %v", err)
	}
	x, y := clickPoint(t, s, "target")
	s.ClickAt(x, y)
	got, err := s.EvaluateJS(`window.evts.join("|")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	entries := strings.Split(got, "|")
	if len(entries) != 5 {
		t.Fatalf("events = %q, want five entries", got)
	}
	want := []string{"pointerdown:", "mousedown:", "pointerup:", "mouseup:", "click:"}
	for i, prefix := range want {
		if !strings.HasPrefix(entries[i], prefix) {
			t.Fatalf("event %d = %q, want prefix %q (all: %q)", i, entries[i], prefix, got)
		}
	}
	if !strings.Contains(entries[0], "true:pointer:mouse:1:") {
		t.Fatalf("pointerdown = %q, want trusted mouse pointer with buttons=1", entries[0])
	}
	if !strings.Contains(entries[1], "true:mouse:1:1:") {
		t.Fatalf("mousedown = %q, want trusted mouse event buttons=1 detail=1", entries[1])
	}
	if !strings.Contains(entries[2], "true:pointer:mouse:0:") {
		t.Fatalf("pointerup = %q, want released pointer buttons=0", entries[2])
	}
	if !strings.Contains(entries[3], "true:mouse:0:1:") {
		t.Fatalf("mouseup = %q, want trusted mouse event buttons=0 detail=1", entries[3])
	}
}

func TestDisabledControlKeepsPointerEventsOnly(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0">
		<button id="btn" disabled style="position:absolute;left:10px;top:10px;width:120px;height:40px">no</button>
	</body></html>`)
	if _, err := s.EvaluateJS(`(function(){
		window.evts = [];
		var el = document.getElementById('btn');
		['pointerdown','mousedown','pointerup','mouseup','click'].forEach(function(typ){
			el.addEventListener(typ, function(e){ window.evts.push(typ + ':' + e.isTrusted); });
		});
	})()`); err != nil {
		t.Fatalf("install listeners: %v", err)
	}
	x, y := clickPoint(t, s, "btn")
	s.ClickAt(x, y)
	got, err := s.EvaluateJS(`window.evts.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if got != "pointerdown:true,pointerup:true" {
		t.Fatalf("disabled control events = %q, want pointerdown then pointerup only", got)
	}
}

func TestLabelPressDispatchesSequenceBeforeActivation(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0">
		<input id="box" type="checkbox" style="position:absolute;left:10px;top:10px">
		<label id="words" for="box" style="position:absolute;left:10px;top:40px;width:120px;height:24px">Agree</label>
	</body></html>`)
	if _, err := s.EvaluateJS(`(function(){
		window.evts = [];
		document.addEventListener('pointerdown', function(e){ window.evts.push('pointerdown:' + e.target.id); }, true);
		document.addEventListener('mousedown', function(e){ window.evts.push('mousedown:' + e.target.id); }, true);
		document.addEventListener('click', function(e){ window.evts.push('click:' + e.target.id); }, true);
		document.addEventListener('change', function(e){ window.evts.push('change:' + e.target.id); }, true);
		document.addEventListener('focus', function(e){ window.evts.push('focus:' + e.target.id); }, true);
	})()`); err != nil {
		t.Fatalf("install listeners: %v", err)
	}
	x, y := clickPoint(t, s, "words")
	s.ClickAt(x, y)
	got, err := s.EvaluateJS(`window.evts.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	want := "pointerdown:words,mousedown:words,click:words,focus:box,click:box,change:box"
	if got != want {
		t.Fatalf("label press events = %q, want %q", got, want)
	}
}

func TestClickIsAPointerEventWithDetailOne(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0">
		<div id="target" style="position:absolute;left:10px;top:10px;width:120px;height:60px;background:#eee">x</div>
	</body></html>`)
	if _, err := s.EvaluateJS(`(function(){
		window.shape = 'none';
		document.getElementById('target').addEventListener('click', function(e){
			window.shape = [e instanceof PointerEvent, e instanceof MouseEvent, e.detail, e.which, e.buttons, e.pointerType].join('|');
		});
	})()`); err != nil {
		t.Fatalf("install listener: %v", err)
	}
	x, y := clickPoint(t, s, "target")
	s.ClickAt(x, y)
	got, err := s.EvaluateJS(`window.shape`)
	if err != nil {
		t.Fatalf("read shape: %v", err)
	}
	if got != "true|true|1|1|0|mouse" {
		t.Fatalf("click shape = %q, want a MouseEvent-compatible PointerEvent with detail 1, which 1, buttons 0, pointerType mouse", got)
	}
}

func TestSecondPressEmitsClickDetailTwoAndDblclick(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0">
		<div id="target" style="position:absolute;left:10px;top:10px;width:120px;height:60px;background:#eee">x</div>
	</body></html>`)
	if _, err := s.EvaluateJS(`(function(){
		window.evts = [];
		['mousedown','click','dblclick'].forEach(function(typ){
			document.getElementById('target').addEventListener(typ, function(e){
				var iface = e instanceof PointerEvent ? 'PointerEvent' : 'MouseEvent';
				window.evts.push(typ + ':' + e.detail + ':' + iface);
			});
		});
	})()`); err != nil {
		t.Fatalf("install listeners: %v", err)
	}
	x, y := clickPoint(t, s, "target")
	s.ClickAt(x, y)
	if _, err := s.EvaluateJS(`window.evts.push('/')`); err != nil {
		t.Fatalf("mark first press: %v", err)
	}
	s.ClickAt(x, y)
	got, err := s.EvaluateJS(`window.evts.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	want := "mousedown:1:MouseEvent,click:1:PointerEvent,/,mousedown:2:MouseEvent,click:2:PointerEvent,dblclick:2:MouseEvent"
	if got != want {
		t.Fatalf("double press events = %q, want %q", got, want)
	}
}

func TestFocusLandsBetweenMousedownAndPointerup(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0">
		<input id="box" type="text" value="hello" style="position:absolute;left:10px;top:10px;width:120px;height:24px">
	</body></html>`)
	if _, err := s.EvaluateJS(`(function(){
		window.evts = [];
		['pointerdown','mousedown','focus','pointerup','mouseup','click'].forEach(function(typ){
			document.addEventListener(typ, function(e){ window.evts.push(typ + ':' + e.target.id); }, true);
		});
	})()`); err != nil {
		t.Fatalf("install listeners: %v", err)
	}
	x, y := clickPoint(t, s, "box")
	s.ClickAt(x, y)
	got, err := s.EvaluateJS(`window.evts.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	want := "pointerdown:box,mousedown:box,focus:box,pointerup:box,mouseup:box,click:box"
	if got != want {
		t.Fatalf("press events = %q, want %q", got, want)
	}
}

func TestPreventDefaultOnMousedownSuppressesFocus(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0">
		<input id="box" type="text" value="hello" style="position:absolute;left:10px;top:10px;width:120px;height:24px">
	</body></html>`)
	if _, err := s.EvaluateJS(`(function(){
		window.evts = [];
		['pointerdown','mousedown','focus','pointerup','mouseup','click'].forEach(function(typ){
			document.addEventListener(typ, function(e){
				if (typ === 'mousedown') { e.preventDefault(); }
				window.evts.push(typ);
			}, true);
		});
	})()`); err != nil {
		t.Fatalf("install listeners: %v", err)
	}
	x, y := clickPoint(t, s, "box")
	s.ClickAt(x, y)
	got, err := s.EvaluateJS(`window.evts.join(",")`)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	want := "pointerdown,mousedown,pointerup,mouseup,click"
	if got != want {
		t.Fatalf("press events = %q, want %q with no focus event", got, want)
	}
	active, err := s.EvaluateJS(`String((document.activeElement && document.activeElement.id) || '')`)
	if err != nil {
		t.Fatalf("read activeElement: %v", err)
	}
	if active != "" {
		t.Fatalf("activeElement = %q, want no focused control after mousedown preventDefault", active)
	}
}

func TestPressOnDisabledControlLeavesFocusAlone(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0">
		<input id="box" type="text" value="hello" style="position:absolute;left:10px;top:10px;width:120px;height:24px">
		<button id="off" disabled style="position:absolute;left:10px;top:40px;width:120px;height:24px">off</button>
	</body></html>`)
	if _, err := s.EvaluateJS(`(function(){
		window.evts = [];
		['focus','blur'].forEach(function(typ){
			document.addEventListener(typ, function(e){ window.evts.push(typ + ':' + e.target.id); }, true);
		});
		document.getElementById('box').focus();
	})()`); err != nil {
		t.Fatalf("focus input: %v", err)
	}
	if _, err := s.EvaluateJS(`window.evts = []`); err != nil {
		t.Fatalf("clear events: %v", err)
	}
	x, y := clickPoint(t, s, "off")
	s.ClickAt(x, y)
	got, err := s.EvaluateJS(`window.evts.join(",") + '|' + String((document.activeElement && document.activeElement.id) || '')`)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if got != "|box" {
		t.Fatalf("disabled press state = %q, want no focus or blur events and box still focused", got)
	}
}

func TestClickReportsPressFocusChanged(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0">
		<input id="box" type="text" value="hello" style="position:absolute;left:10px;top:10px;width:120px;height:24px">
	</body></html>`)
	x, y := clickPoint(t, s, "box")
	s.ClickAt(x, y)
	if !s.PressFocusChanged() {
		t.Fatalf("press that focused a control did not report a press focus change")
	}
	s.ClickAt(x, y)
	if s.PressFocusChanged() {
		t.Fatalf("second press at the same caret position reported a press focus change")
	}
}

func TestThirdPressEmitsNoSecondDblclick(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0">
		<div id="target" style="position:absolute;left:10px;top:10px;width:120px;height:60px;background:#eee">x</div>
	</body></html>`)
	if _, err := s.EvaluateJS(`(function(){
		window.dbl = [];
		document.getElementById('target').addEventListener('dblclick', function(e){ window.dbl.push(e.detail); });
	})()`); err != nil {
		t.Fatalf("install listener: %v", err)
	}
	x, y := clickPoint(t, s, "target")
	s.ClickAt(x, y)
	s.ClickAt(x, y)
	s.ClickAt(x, y)
	got, err := s.EvaluateJS(`window.dbl.join(",")`)
	if err != nil {
		t.Fatalf("read dblclick: %v", err)
	}
	if got != "2" {
		t.Fatalf("dblclick details = %q, want only 2", got)
	}
}
