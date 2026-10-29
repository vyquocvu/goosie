package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/paint"
)

func findElement(root *dom.Node, data string) *dom.Node {
	if root.Data == data && root.Element() {
		return root
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if got := findElement(c, data); got != nil {
			return got
		}
	}
	return nil
}

func newFocusSession(t *testing.T, html string) *engine.Session {
	t.Helper()
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return s
}

// clickPoint returns the centre of the laid-out element with the given id.
func clickPoint(t *testing.T, s *engine.Session, id string) (float32, float32) {
	t.Helper()
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			x0, y0, x1, y1 := obj.BorderRect()
			return (x0 + x1) / 2, (y0 + y1) / 2
		}
	}
	t.Fatalf("element %q not found in arena", id)
	return 0, 0
}

const twoInputsHTML = `<html><body style="margin: 0;">` +
	`<input id="a" value="ab"><input id="b" value="cd"></body></html>`

func TestSelectAllControlSelectsAndReplacesLiveValue(t *testing.T) {
	s, _ := newJSSession(t, `<html><body><input id="field" value="abcd"></body></html>`)
	field := s.Doc.ElementByID("field")
	if !s.FocusNode(field) {
		t.Fatal("input should receive focus")
	}
	if !s.SelectAllControl() {
		t.Fatal("SelectAllControl should select the input value")
	}
	if got := s.FocusedText(); got != "abcd" {
		t.Fatalf("selected text = %q, want %q", got, "abcd")
	}
	if got, err := s.EvaluateJS(`document.getElementById("field").selectionStart+"|"+document.getElementById("field").selectionEnd`); err != nil || got != "0|4" {
		t.Fatalf("script-visible selection = %q, %v; want 0|4", got, err)
	}
	if !s.Edit(engine.EditRune, 'x') {
		t.Fatal("typing should replace the selected text")
	}
	if got := field.InputControlValue(); got != "x" {
		t.Fatalf("value after replacement = %q, want x", got)
	}
	if got, err := s.EvaluateJS(`document.getElementById("field").selectionStart+"|"+document.getElementById("field").selectionEnd`); err != nil || got != "1|1" {
		t.Fatalf("collapsed script-visible selection = %q, %v; want 1|1", got, err)
	}
}

func TestDisabledFieldsetMovesFocusOffDescendantDuringFramePump(t *testing.T) {
	s, _ := newJSSession(t, `<fieldset id="group"><legend>Group</legend><input id="field"></fieldset>`)
	field := s.Doc.ElementByID("field")
	if !s.FocusNode(field) {
		t.Fatal("input should receive focus before its fieldset is disabled")
	}
	if _, err := s.EvaluateJS(`document.getElementById('group').disabled=true`); err != nil {
		t.Fatalf("disable fieldset: %v", err)
	}
	s.PumpJS(800)
	if s.Focused() == field {
		t.Fatal("disabled fieldset descendant kept focus after the rendering update")
	}
}

func TestSummaryCanReceiveProgrammaticFocus(t *testing.T) {
	s := newFocusSession(t, `<details><summary id="summary">Open</summary><p>Content</p></details>`)
	summary := s.Doc.ElementByID("summary")
	if summary == nil {
		t.Fatal("summary element missing from document")
	}
	if !s.FocusNode(summary) {
		t.Fatal("summary should be programmatically focusable")
	}
	if s.Focused() != summary {
		t.Fatalf("focused node = %v, want summary", s.Focused())
	}
}

func TestLocationHashOpensDetailsAroundTargetAndScrollsToIt(t *testing.T) {
	s, _ := newJSSession(t, `<div style="height: 2000px">spacer</div><details id="details"><div id="target">target</div></details>`)
	if _, err := s.EvaluateJS(`window.location.hash = "#target"`); err != nil {
		t.Fatalf("set location hash: %v", err)
	}
	if !s.Doc.ElementByID("details").HasAttribute("open") {
		t.Fatal("fragment navigation left the target's details ancestor closed")
	}
	if got, err := s.EvaluateJS(`String(window.pageYOffset > 0)`); err != nil || got != "true" {
		t.Fatalf("fragment navigation pageYOffset = %q, %v; want a positive scroll position", got, err)
	}
}

func TestPointerPositionsTextInputCaret(t *testing.T) {
	s, _ := newJSSession(t, `<html><body><input id="field" value="abcd"></body></html>`)
	field := s.Doc.ElementByID("field")
	var x0, x1, y float32
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node == field {
			x0, _, x1, y = obj.ContentRect()
			_, y0, _, y1 := obj.BorderRect()
			y = (y0 + y1) / 2
			break
		}
	}
	if x1 <= x0 {
		t.Fatal("text input has no laid-out content box")
	}
	if !s.FocusControl(x0, y) {
		t.Fatal("click should focus input and place caret")
	}
	if got, err := s.EvaluateJS(`document.getElementById("field").selectionStart+"|"+document.getElementById("field").selectionEnd`); err != nil || got != "0|0" {
		t.Fatalf("left-edge selection = %q, %v; want 0|0", got, err)
	}
	if !s.FocusControl(x1-1, y) {
		t.Fatal("click on focused input should move caret")
	}
	if got, err := s.EvaluateJS(`document.getElementById("field").selectionStart+"|"+document.getElementById("field").selectionEnd`); err != nil || got != "4|4" {
		t.Fatalf("right-edge selection = %q, %v; want 4|4", got, err)
	}
}

func TestTextareaRowsColsSetIntrinsicControlSize(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin:0"><textarea id="default">a</textarea><textarea id="sized" cols="10" rows="4">a</textarea></body></html>`)
	contentSize := func(id string) (float32, float32) {
		node := s.Doc.ElementByID(id)
		if node == nil {
			t.Fatalf("textarea %q missing", id)
		}
		for i := range s.Arena.Objects {
			obj := &s.Arena.Objects[i]
			if obj.Node == node {
				_, _, x1, y1 := obj.ContentRect()
				return x1 - obj.X - obj.BorderLeft - obj.PaddingLeft,
					y1 - obj.Y - obj.BorderTop - obj.PaddingTop
			}
		}
		t.Fatalf("textarea %q has no layout object", id)
		return 0, 0
	}
	defaultW, defaultH := contentSize("default")
	sizedW, sizedH := contentSize("sized")
	if defaultW <= sizedW {
		t.Fatalf("cols defaults should make the default textarea wider: default=%g cols=%d, cols=10=%g cols=%d", defaultW, s.Doc.ElementByID("default").TextareaCols(), sizedW, s.Doc.ElementByID("sized").TextareaCols())
	}
	if sizedH <= defaultH {
		t.Fatalf("rows=4 should make the textarea taller than default rows=2: default=%g, rows=4=%g", defaultH, sizedH)
	}
	if got := s.Styles[s.Doc.ElementByID("default").ID].FontFamily; got != frame.FontCourier {
		t.Fatalf("textarea UA font family = %v, want monospace", got)
	}
}

func TestShiftArrowExtendsTextControlSelectionAndTypingReplacesIt(t *testing.T) {
	s, _ := newJSSession(t, `<html><body><input id="field" value="abcd"></body></html>`)
	field := s.Doc.ElementByID("field")
	if !s.FocusNode(field) {
		t.Fatal("input should receive focus")
	}
	if s.EditWithShift(engine.EditLeft, 0, true) {
		t.Fatal("selection movement should not report a value change")
	}
	if got, err := s.EvaluateJS(`(function(){var e=document.getElementById("field");return [e.selectionStart,e.selectionEnd,e.selectionDirection].join("|")})()`); err != nil || got != "3|4|backward" {
		t.Fatalf("shift selection = %q, %v; want 3|4|backward", got, err)
	}
	if got := s.FocusedText(); got != "d" {
		t.Fatalf("selected text = %q, want d", got)
	}
	if !s.Edit(engine.EditRune, 'X') {
		t.Fatal("typing should replace the shift-selected character")
	}
	if got := field.InputControlValue(); got != "abcX" {
		t.Fatalf("value after replacement = %q, want abcX", got)
	}
}

func TestFocusControlHit(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	if s.Focused() != nil {
		t.Fatal("session starts with focus")
	}
	x, y := clickPoint(t, s, "a")
	if !s.FocusControl(x, y) {
		t.Fatal("focusing a control should report change")
	}
	if n := s.Focused(); n == nil || n.GetAttribute("id") != "a" {
		t.Fatalf("focused = %v, want input#a", n)
	}
}

func TestRangeInputKeyboardStepsDispatchTrustedInput(t *testing.T) {
	s, _ := newJSSession(t, `<html><body><input id="range" type="range" min="0" max="100" step="20" value="20"></body></html>`)
	if _, err := s.EvaluateJS(`(function(){var r=document.getElementById("range");r.focus();window.rangeEvents=[];r.addEventListener("input",function(e){rangeEvents.push((e instanceof InputEvent)+":"+e.isTrusted+":"+e.bubbles+":"+e.cancelable)})})()`); err != nil {
		t.Fatalf("install range handlers: %v", err)
	}
	if !s.Edit(engine.EditRight, 0) {
		t.Fatal("EditRight should change the range value in the browser edit path")
	}
	if got := s.Focused().InputControlValue(); got != "40" {
		t.Fatalf("EditRight value = %q, want 40", got)
	}
	if !s.PressKey("ArrowRight", false, false, false, false) {
		t.Fatal("ArrowRight should change the range value")
	}
	if got, err := s.EvaluateJS(`document.getElementById("range").value+"|"+rangeEvents.join(",")`); err != nil || got != "60|true:true:true:false,true:true:true:false" {
		t.Fatalf("after ArrowRight = %q, %v; want trusted InputEvents at value 60", got, err)
	}
	if !s.PressKey("Home", false, false, false, false) {
		t.Fatal("Home should move the range value to its minimum")
	}
	if got := s.Focused().InputControlValue(); got != "0" {
		t.Fatalf("Home value = %q, want 0", got)
	}
	if !s.PressKey("PageUp", false, false, false, false) || s.Focused().InputControlValue() != "100" {
		t.Fatalf("PageUp value = %q, want 100", s.Focused().InputControlValue())
	}
	if !s.PressKey("PageDown", false, false, false, false) || s.Focused().InputControlValue() != "0" {
		t.Fatalf("PageDown value = %q, want 0", s.Focused().InputControlValue())
	}
	if _, err := s.EvaluateJS(`document.getElementById("range").addEventListener("keydown",function(e){if(e.key==="ArrowRight")e.preventDefault()})`); err != nil {
		t.Fatalf("install cancellation handler: %v", err)
	}
	if s.PressKey("ArrowRight", false, false, false, false) {
		t.Fatal("prevented ArrowRight should not report a value change")
	}
	if got := s.Focused().InputControlValue(); got != "0" {
		t.Fatalf("prevented ArrowRight value = %q, want 0", got)
	}
}

func TestFocusControlBlur(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if !s.FocusControl(790, 590) {
		t.Fatal("blur should report change")
	}
	if s.Focused() != nil {
		t.Fatalf("focused = %v, want nil", s.Focused())
	}
	if s.FocusControl(790, 590) {
		t.Fatal("blur while unfocused should not report change")
	}
}

func TestFocusControlSwitch(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	x, y = clickPoint(t, s, "b")
	if !s.FocusControl(x, y) {
		t.Fatal("switching focus should report change")
	}
	if n := s.Focused(); n == nil || n.GetAttribute("id") != "b" {
		t.Fatalf("focused = %v, want input#b", n)
	}
}

func TestEditInsert(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if !s.Edit(engine.EditRune, 'x') {
		t.Fatal("insert should report change")
	}
	if got := s.Focused().InputControlValue(); got != "abx" {
		t.Fatalf("value = %q, want %q", got, "abx")
	}
	if got := s.Focused().GetAttribute("value"); got != "ab" {
		t.Fatalf("default value attribute = %q, want %q", got, "ab")
	}
}

func TestTextControlEditRefreshesPatternValidityForCSS(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body style="margin:0"><input id="field" pattern="[0-9]+" value="1"></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	x, y := clickPoint(t, s, "field")
	if !s.FocusControl(x, y) {
		t.Fatal("input should receive focus")
	}
	if !s.Edit(engine.EditRune, 'x') {
		t.Fatal("typing should update the input")
	}
	if got, err := s.EvaluateJS(`(function(){var c=document.getElementById("field");return c.validity.patternMismatch&&c.matches(":invalid")})()`); err != nil || got != "true" {
		t.Fatalf("pattern validity after engine edit = %q, err %v", got, err)
	}
	if _, err := s.EvaluateJS(`(function(){var c=document.getElementById("field");c.type="email";c.value="user@example.com"})()`); err != nil {
		t.Fatalf("prepare email input: %v", err)
	}
	if !s.Edit(engine.EditRune, '@') {
		t.Fatal("typing should update the email input")
	}
	if got, err := s.EvaluateJS(`(function(){var c=document.getElementById("field");return c.validity.typeMismatch&&c.matches(":invalid")})()`); err != nil || got != "true" {
		t.Fatalf("type validity after engine edit = %q, err %v", got, err)
	}
}

func TestContentEditableEngineTextEntryUsesSelectionAndInputEvents(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">ab</div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var t=document.getElementById("edit").firstChild;var r=document.createRange();r.setStart(t,0);r.collapse(true);var q=getSelection();q.removeAllRanges();q.addRange(r);var host=document.getElementById("edit");host.addEventListener("beforeinput",function(e){window.beforeType=e.inputType+":"+e.data+":"+(e instanceof InputEvent)+":"+e.isTrusted+":"+(e.dataTransfer===null)});host.addEventListener("input",function(e){window.inputType=e.inputType+":"+e.data+":"+(e instanceof InputEvent)+":"+e.isTrusted+":"+(e.dataTransfer===null)})})()`); err != nil {
		t.Fatalf("prepare editing selection: %v", err)
	}
	if s.Edit(engine.EditRight, 0) {
		t.Fatal("caret movement alone should not report a document change")
	}
	if !s.Edit(engine.EditRune, 'X') {
		t.Fatal("typing into contenteditable should change the document")
	}
	if got := edit.TextContent(); got != "aXb" {
		t.Fatalf("contenteditable text = %q, want %q", got, "aXb")
	}
	if got, err := s.EvaluateJS(`window.beforeType+"|"+window.inputType`); err != nil || got != "insertText:X:true:true:true|insertText:X:true:true:true" {
		t.Fatalf("input events = %q, err %v", got, err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("backspace at the Selection caret should delete the preceding character")
	}
	if got := edit.TextContent(); got != "ab" {
		t.Fatalf("contenteditable text after backspace = %q, want %q", got, "ab")
	}
	if _, err := s.EvaluateJS(`(function(){var t=document.getElementById("edit").firstChild;var r=document.createRange();r.setStart(t,1);r.collapse(true);var q=getSelection();q.removeAllRanges();q.addRange(r)})()`); err != nil {
		t.Fatalf("position forward-delete caret: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete at the Selection caret should delete the following character")
	}
	if got := edit.TextContent(); got != "a" {
		t.Fatalf("contenteditable text after Delete = %q, want %q", got, "a")
	}
	if got, err := s.EvaluateJS(`window.beforeType+"|"+window.inputType`); err != nil || got != "deleteContentForward:null:true:true:true|deleteContentForward:null:true:true:true" {
		t.Fatalf("Delete InputEvents = %q, err %v", got, err)
	}
}

func TestFocusingContentEditableCreatesCaretAtFirstText(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">start</div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if got, err := s.EvaluateJS(`(function(){var selection=getSelection();return selection.anchorNode===document.getElementById("edit").firstChild&&selection.anchorOffset===0&&selection.isCollapsed})()`); err != nil || got != "true" {
		t.Fatalf("focused contenteditable caret = %q, err %v; want first text node offset zero", got, err)
	}
	if !s.Edit(engine.EditRune, 'X') {
		t.Fatal("typing at the focused contenteditable caret should change the document")
	}
	if got := edit.TextContent(); got != "Xstart" {
		t.Fatalf("contenteditable text after typing = %q, want Xstart", got)
	}
}

func TestContentEditableBackspaceJoinsAdjacentBlocksWithTargetRange(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><p>abc</p><p>def</p></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),p=edit.lastChild,range=document.createRange(),selection=getSelection();range.setStart(p.firstChild,0);range.collapse(true);selection.removeAllRanges();selection.addRange(range);window.deleteEvents=[];edit.addEventListener("beforeinput",function(event){var ranges=event.getTargetRanges();deleteEvents.push("beforeinput:"+event.inputType+":"+ranges.length+":"+ranges[0].startContainer.data+":"+ranges[0].startOffset+":"+ranges[0].endContainer.data+":"+ranges[0].endOffset)});edit.addEventListener("input",function(event){deleteEvents.push("input:"+event.inputType+":"+event.getTargetRanges().length)})})()`); err != nil {
		t.Fatalf("prepare block-start deletion: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace at the second paragraph start should join the blocks")
	}
	if got := edit.TextContent(); got != "abcdef" {
		t.Fatalf("contenteditable text after block join = %q, want abcdef", got)
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+deleteEvents.join("|")`); err != nil || got != `<p>abcdef</p>|beforeinput:deleteContentBackward:1:abc:3:def:0|input:deleteContentBackward:0` {
		t.Fatalf("block join HTML and Input Events = %q, err %v", got, err)
	}
}

func TestContentEditableBackspaceRemovesInvisibleLeadingWhitespaceWithoutInputEvents(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><p>   abc def</p></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := s.Doc.ElementByID("edit")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){
		var host=document.getElementById("edit"),text=host.firstChild.firstChild;
		window.leadingWhitespaceEvents=[];
		host.addEventListener("beforeinput",function(e){leadingWhitespaceEvents.push("before:"+e.inputType)});
		host.addEventListener("input",function(e){leadingWhitespaceEvents.push("input:"+e.inputType)});
		function press(shift,ctrl,alt,meta){text.data="   abc def";var range=document.createRange(),selection=getSelection();range.setStart(text,3);range.collapse(true);selection.removeAllRanges();selection.addRange(range);window.__goosieTestDriverKeyDefault("Backspace",shift,ctrl,alt,meta)}
		press(true,false,false,false);press(false,true,false,false);press(false,false,true,false);press(false,false,false,true);
	})()`); err != nil {
		t.Fatalf("run modified Backspace at collapsed leading whitespace: %v", err)
	}
	if got, err := s.EvaluateJS(`edit.innerHTML+"|"+leadingWhitespaceEvents.join("|")`); err != nil || got != `<p>abc def</p>|` {
		t.Fatalf("modified Backspace at rendered text start = %q, err %v", got, err)
	}
}

func TestContentEditableBeforeInputExposesStaticTargetRange(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">abc</div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var text=document.getElementById("edit").firstChild;var range=document.createRange();range.setStart(text,1);range.setEnd(text,2);var selection=getSelection();selection.removeAllRanges();selection.addRange(range);document.getElementById("edit").addEventListener("beforeinput",function(event){var ranges=event.getTargetRanges();window.targetRangeResult=[ranges.length,ranges[0] instanceof StaticRange,ranges[0].startOffset,ranges[0].endOffset].join(":")})})()`); err != nil {
		t.Fatalf("prepare contenteditable target range: %v", err)
	}
	if !s.Edit(engine.EditRune, 'X') {
		t.Fatal("typing into the selected contenteditable text should change the document")
	}
	if got, err := s.EvaluateJS(`window.targetRangeResult`); err != nil || got != "1:true:1:2" {
		t.Fatalf("beforeinput target ranges = %q, err %v; want 1:true:1:2", got, err)
	}
	if _, err := s.EvaluateJS(`(function(){var text=document.getElementById("edit").firstChild;var selection=getSelection();selection.collapse(text,2);window.targetRangeResult=""})()`); err != nil {
		t.Fatalf("prepare collapsed deletion caret: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should delete the character before the contenteditable caret")
	}
	if got, err := s.EvaluateJS(`window.targetRangeResult`); err != nil || got != "1:true:1:2" {
		t.Fatalf("Backspace target range = %q, err %v; want 1:true:1:2", got, err)
	}
}

func TestContentEditableBeforeInputShrinksSelectedTextChildTargetRange(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><p>abc</p></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),p=edit.firstChild,selection=getSelection();selection.setBaseAndExtent(p,0,p,1);edit.addEventListener("beforeinput",function(event){var ranges=event.getTargetRanges(),range=ranges[0];window.targetRangeResult=[ranges.length,range.startContainer.data,range.startOffset,range.endContainer.data,range.endOffset].join(":")})})()`); err != nil {
		t.Fatalf("prepare element-boundary text selection: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should delete the selected contenteditable text")
	}
	if got, err := s.EvaluateJS(`window.targetRangeResult`); err != nil || got != "1:abc:0:abc:3" {
		t.Fatalf("beforeinput target range = %q, err %v; want 1:abc:0:abc:3", got, err)
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != "<p><br></p>" {
		t.Fatalf("contenteditable after deleting the whole paragraph = %q, err %v; want <p><br></p>", got, err)
	}
}

func TestContentEditableListSelectionTargetsSingleItemText(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>list-item1</li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),list=edit.firstChild,selection=getSelection();selection.setBaseAndExtent(list,0,list,1);edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];window.listTargetRange=[range.startContainer.data,range.startOffset,range.endContainer.data,range.endOffset].join(":")})})()`); err != nil {
		t.Fatalf("prepare selected list item: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should delete the selected contenteditable list item")
	}
	if got, err := s.EvaluateJS(`window.listTargetRange`); err != nil || got != "list-item1:0:list-item1:10" {
		t.Fatalf("beforeinput list target range = %q, err %v; want list-item1:0:list-item1:10", got, err)
	}
}

func TestContentEditableForwardDeleteTargetRangeSkipsTrailingListBreak(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ol><li>list-item1<br></li><li>list-item2</li></ol></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),text=edit.querySelector("li").firstChild,selection=getSelection();selection.collapse(text,text.length);window.listDeleteSelection=[selection.anchorNode.nodeName,selection.anchorOffset,selection.focusNode.nodeName,selection.focusOffset].join(":");edit.addEventListener("beforeinput",function(event){var ranges=event.getTargetRanges(),range=ranges[0];window.listDeleteTarget=[ranges.length,range.startContainer.nodeName,range.startContainer.data,range.startOffset,range.endContainer.nodeName,range.endContainer.data,range.endOffset].join(":")})})()`); err != nil {
		t.Fatalf("prepare caret before trailing list break: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete should run at the end of a list item with a trailing break")
	}
	if got, err := s.EvaluateJS(`window.listDeleteTarget`); err != nil || got != "1:#text:list-item1:10:#text:list-item2:0" {
		selection, _ := s.EvaluateJS(`window.listDeleteSelection`)
		t.Fatalf("beforeinput list target range = %q (selection %q), err %v; want text range from item1 end to item2 start", got, selection, err)
	}
}

func TestContentEditableForwardDeleteRemovesAtomicImageAtElementBoundary(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">abc<img><img>def</div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),selection=getSelection();selection.collapse(edit,2);window.atomicDeleteInputCount=0;edit.addEventListener("input",function(){atomicDeleteInputCount++})})()`); err != nil {
		t.Fatalf("prepare caret between atomic images: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		state, stateErr := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+atomicDeleteInputCount+"|"+getSelection().anchorNode.nodeName+":"+getSelection().anchorOffset`)
		t.Fatalf("Delete between images should remove the following image; state=%q err=%v", state, stateErr)
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+atomicDeleteInputCount`); err != nil || got != `abc<img>def|1` {
		t.Fatalf("contenteditable after atomic image deletion = %q, err %v; want one image removed and one input event", got, err)
	}
}

func TestContentEditableForwardDeleteJoinsBlocksAcrossCollapsedWhitespace(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><p>abc   </p><p>   def</p></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),text=edit.firstChild.firstChild,selection=getSelection();selection.collapse(text,3);window.blockDeleteInputCount=0;edit.addEventListener("input",function(){blockDeleteInputCount++})})()`); err != nil {
		t.Fatalf("prepare caret before trailing spaces: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete at collapsed block whitespace should join the paragraphs")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+blockDeleteInputCount`); err != nil || got != `<p>abcdef</p>|1` {
		t.Fatalf("contenteditable after whitespace block join = %q, err %v; want merged text and one input event", got, err)
	}
}

func TestContentEditableForwardDeleteJoinsInlineRunIntoParagraphFirstLine(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">abc<p>def<br>ghi</p></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil {
		t.Fatal("contenteditable host is missing")
	}
	if _, err := s.EvaluateJS(`document.getElementById("edit").focus()`); err != nil {
		t.Fatalf("focus contenteditable host through the DOM API: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),text=edit.firstChild,selection=getSelection();selection.collapse(text,text.length);window.inlineBlockDelete=[];edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];inlineBlockDelete.push("beforeinput:"+range.startContainer.data+":"+range.startOffset+":"+range.endContainer.data+":"+range.endOffset)});edit.addEventListener("input",function(event){inlineBlockDelete.push("input:"+event.inputType)})})()`); err != nil {
		t.Fatalf("prepare inline-to-block caret: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete at inline-to-block boundary should join the first line")
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+inlineBlockDelete.join("|")`)
	if err != nil {
		t.Fatalf("read inline-to-block deletion result: %v", err)
	}
	if got != `abcdef<p>ghi</p>|beforeinput:abc:3:def:0|input:deleteContentForward` && got != `abcdef<br><p>ghi</p>|beforeinput:abc:3:def:0|input:deleteContentForward` {
		t.Fatalf("contenteditable inline-to-block deletion = %q, want the first line joined and second line retained", got)
	}
}

func TestContentEditableForwardDeletePreservesPreformattedSpacesWhenJoiningParagraph(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><p>abc   </p><pre>   def</pre></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),text=edit.firstChild.firstChild,selection=getSelection();selection.collapse(text,3)})()`); err != nil {
		t.Fatalf("prepare caret before trailing spaces: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete at paragraph boundary should join the preformatted block")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != `<p>abc &nbsp; def</p>` {
		t.Fatalf("contenteditable after paragraph/pre join = %q, err %v; want visible spaces preserved with a non-breaking space", got, err)
	}
}

func TestContentEditableForwardDeleteRemovesCollapsedWhitespaceAroundHorizontalRule(t *testing.T) {
	for _, tc := range []struct {
		name string
		html string
	}{
		{name: "before rule", html: `<div>abc <hr>def</div>`},
		{name: "after rule", html: `<div>abc<hr> def</div>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := js.New(js.Options{})
			if err != nil {
				t.Fatal(err)
			}
			s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">`+tc.html+`</div></body></html>`, nil, 800, engine.WithJS(rt))
			if err != nil {
				_ = rt.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			edit := findElement(&s.Doc.Node, "div")
			if edit == nil || !s.FocusNode(edit) {
				t.Fatal("contenteditable host should receive focus")
			}
			if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),text=edit.firstChild.firstChild;getSelection().collapse(text,3)})()`); err != nil {
				t.Fatalf("prepare caret before horizontal rule: %v", err)
			}
			if !s.Edit(engine.EditDelete, 0) {
				t.Fatal("Delete at horizontal rule should remove the rule")
			}
			if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != `<div>abcdef</div>` {
				t.Fatalf("contenteditable after horizontal-rule deletion = %q, err %v; want adjacent collapsed whitespace removed", got, err)
			}
		})
	}
}

func TestContentEditableSelectedHorizontalRuleRemovesAdjacentCollapsedWhitespace(t *testing.T) {
	for _, action := range []engine.EditAction{engine.EditBackspace, engine.EditDelete} {
		t.Run(map[engine.EditAction]string{engine.EditBackspace: "Backspace", engine.EditDelete: "Delete"}[action], func(t *testing.T) {
			rt, err := js.New(js.Options{})
			if err != nil {
				t.Fatal(err)
			}
			s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">abc <hr> def</div></body></html>`, nil, 800, engine.WithJS(rt))
			if err != nil {
				_ = rt.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			edit := findElement(&s.Doc.Node, "div")
			if edit == nil || !s.FocusNode(edit) {
				t.Fatal("contenteditable host should receive focus")
			}
			if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");getSelection().setBaseAndExtent(edit,1,edit,2)})()`); err != nil {
				t.Fatalf("select horizontal rule: %v", err)
			}
			if !s.Edit(action, 0) {
				t.Fatal("deleting the selected horizontal rule should change the document")
			}
			if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != "abcdef" {
				t.Fatalf("contenteditable after selected horizontal-rule deletion = %q, err %v; want adjacent collapsed whitespace removed", got, err)
			}
		})
	}
}

func TestContentEditableTypingOverSelectedHorizontalRuleRemovesAdjacentWhitespace(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">abc <hr>def</div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");var range=document.createRange();range.setStart(edit.childNodes[0],4);range.setEnd(edit.childNodes[2],0);var selection=getSelection();selection.removeAllRanges();selection.addRange(range)})()`); err != nil {
		t.Fatalf("select horizontal rule between text nodes: %v", err)
	}
	if !s.InsertText("a") {
		t.Fatal("typing over the selected horizontal rule should change the document")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != "abcadef" {
		t.Fatalf("contenteditable after typing over selected horizontal rule = %q, err %v; want adjacent whitespace removed", got, err)
	}
}

func TestContentEditableSelectionFromHostTextIntoParagraphJoinsFirstLine(t *testing.T) {
	for _, typed := range []bool{false, true} {
		name := "Backspace"
		if typed {
			name = "Typing"
		}
		t.Run(name, func(t *testing.T) {
			rt, err := js.New(js.Options{})
			if err != nil {
				t.Fatal(err)
			}
			s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><div>abc<p>def<br>ghi</p></div></div></body></html>`, nil, 800, engine.WithJS(rt))
			if err != nil {
				_ = rt.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			edit := findElement(&s.Doc.Node, "div")
			if edit == nil || !s.FocusNode(edit) {
				t.Fatal("contenteditable host should receive focus")
			}
			if _, err := s.EvaluateJS(`(function(){var content=document.getElementById("edit").firstChild;var range=document.createRange();range.setStart(content.firstChild,3);range.setEnd(content.lastChild.firstChild,0);var selection=getSelection();selection.removeAllRanges();selection.addRange(range)})()`); err != nil {
				t.Fatalf("select from host text to paragraph start: %v", err)
			}
			changed := s.Edit(engine.EditBackspace, 0)
			if typed {
				changed = s.InsertText("a")
			}
			if !changed {
				t.Fatal("editing a selection across the paragraph boundary should change the document")
			}
			want := "<div>abcdef<p>ghi</p></div>"
			if typed {
				want = "<div>abcadef<p>ghi</p></div>"
			}
			if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != want {
				t.Fatalf("contenteditable after boundary edit = %q, err %v; want %q", got, err, want)
			}
		})
	}
}

func TestContentEditableSelectionFromParagraphIntoFollowingTextJoinsParagraph(t *testing.T) {
	for _, typed := range []bool{false, true} {
		name := "Backspace"
		if typed {
			name = "Typing"
		}
		t.Run(name, func(t *testing.T) {
			rt, err := js.New(js.Options{})
			if err != nil {
				t.Fatal(err)
			}
			s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><div><p>abc</p>def</div></div></body></html>`, nil, 800, engine.WithJS(rt))
			if err != nil {
				_ = rt.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			edit := findElement(&s.Doc.Node, "div")
			if edit == nil || !s.FocusNode(edit) {
				t.Fatal("contenteditable host should receive focus")
			}
			if _, err := s.EvaluateJS(`(function(){var content=document.getElementById("edit").firstChild;var range=document.createRange();range.setStart(content.firstChild.firstChild,3);range.setEnd(content.lastChild,0);var selection=getSelection();selection.removeAllRanges();selection.addRange(range)})()`); err != nil {
				t.Fatalf("select from paragraph end to following text: %v", err)
			}
			changed := s.Edit(engine.EditBackspace, 0)
			if typed {
				changed = s.InsertText("a")
			}
			if !changed {
				t.Fatal("editing a selection across the paragraph boundary should change the document")
			}
			want := "<div><p>abcdef</p></div>"
			if typed {
				want = "<div><p>abcadef</p></div>"
			}
			if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != want {
				t.Fatalf("contenteditable after boundary edit = %q, err %v; want %q", got, err, want)
			}
		})
	}
}

func TestContentEditableSelectionFromTextIntoListJoinsFirstListItem(t *testing.T) {
	for _, typed := range []bool{false, true} {
		name := "Backspace"
		if typed {
			name = "Typing"
		}
		t.Run(name, func(t *testing.T) {
			rt, err := js.New(js.Options{})
			if err != nil {
				t.Fatal(err)
			}
			s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><div>abc<ul><li>def</li></ul>ghi</div></div></body></html>`, nil, 800, engine.WithJS(rt))
			if err != nil {
				_ = rt.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			edit := findElement(&s.Doc.Node, "div")
			if edit == nil || !s.FocusNode(edit) {
				t.Fatal("contenteditable host should receive focus")
			}
			if _, err := s.EvaluateJS(`(function(){var content=document.getElementById("edit").firstChild;var range=document.createRange();range.setStart(content.firstChild,3);range.setEnd(content.querySelector("li").firstChild,0);var selection=getSelection();selection.removeAllRanges();selection.addRange(range)})()`); err != nil {
				t.Fatalf("select from inline text to first list item: %v", err)
			}
			changed := s.Edit(engine.EditBackspace, 0)
			if typed {
				changed = s.InsertText("a")
			}
			if !changed {
				t.Fatal("editing a selection across the list boundary should change the document")
			}
			want := "<div>abcdefghi</div>"
			if typed {
				want = "<div>abcadefghi</div>"
			}
			if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != want {
				t.Fatalf("contenteditable after list boundary edit = %q, err %v; want %q", got, err, want)
			}
		})
	}
}

func TestContentEditableSelectionAcrossTablesKeepsSelectedCellSlots(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><table><tr><td>abc</td><td>def</td></tr></table><table><tr><td>ghi</td><td>jkl</td></tr></table></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML='<table><tr><td>abc</td><td>def</td></tr></table><table><tr><td>ghi</td><td>jkl</td></tr></table>';var tables=edit.querySelectorAll("table"),start=tables[0].querySelectorAll("td")[0].firstChild,end=tables[tables.length-1].querySelectorAll("td")[1].firstChild,range=document.createRange();range.setStart(start,2);range.setEnd(end,1);var selection=getSelection();selection.removeAllRanges();selection.addRange(range);window.__tableCount=tables.length})()`); err != nil {
		t.Fatalf("select across adjacent tables: %v", err)
	}
	if count, err := s.EvaluateJS(`window.__tableCount`); err != nil || count != "2" {
		t.Fatalf("adjacent table fragment parse produced %q tables, err %v", count, err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("deleting a selection across table cells should change the document")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != `<table><tbody><tr><td>ab</td><td><br></td></tr></tbody></table><table><tbody><tr><td><br></td><td>kl</td></tr></tbody></table>` {
		count, _ := s.EvaluateJS(`document.getElementById("edit").querySelectorAll("table").length`)
		t.Fatalf("contenteditable after cross-table deletion = %q (%s tables), err %v", got, count, err)
	}
}

func TestContentEditableArrowRightThenDeleteUsesAdvancedUnicodeCaret(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><p>helloשלום</p></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),text=edit.firstChild.firstChild;getSelection().collapse(text,4);edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];window.deleteTarget=[range.startOffset,range.endOffset]})})()`); err != nil {
		t.Fatalf("prepare caret before Hebrew text: %v", err)
	}
	s.Edit(engine.EditRight, 0)
	if got, err := s.EvaluateJS(`getSelection().anchorOffset`); err != nil || got != "5" {
		t.Fatalf("caret after ArrowRight = %q, err %v; want offset 5", got, err)
	}
	s.Edit(engine.EditDelete, 0)
	if got, err := s.EvaluateJS(`document.getElementById("edit").textContent+"|"+deleteTarget.join(":")`); err != nil || got != "helloלום|5:6" {
		t.Fatalf("Unicode Delete result = %q, err %v; want deletion at the advanced caret", got, err)
	}
}

func TestContentEditableSelectionDeleteRemovesEmptiedNestedList(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>list-item1</li><ul><li>list-item2</li></ul></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),items=edit.querySelectorAll("li"),start=items[0].firstChild,end=items[1].firstChild,selection=getSelection();selection.setBaseAndExtent(start,0,end,end.length);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare selection through nested list: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete should remove selected text across the nested list")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><li><br></li></ul>|1" {
		t.Fatalf("contenteditable after nested-list selection deletion = %q, err %v; want outer list placeholder and one input event", got, err)
	}
}

func TestContentEditableSelectionDeleteRemovesEmptyItemAfterNestedList(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><ul><li>list-item1</li></ul><li>list-item2</li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),items=edit.querySelectorAll("li"),start=items[0].firstChild,end=items[1].firstChild,selection=getSelection();selection.setBaseAndExtent(start,0,end,end.length);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare selection across nested list sibling: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete should remove selected text after nested list content")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><ul><li><br></li></ul></ul>|1" {
		t.Fatalf("contenteditable after nested-list sibling deletion = %q, err %v; want nested-list placeholder and one input event", got, err)
	}
}

func TestContentEditableSelectionJoinPreservesNestedListPlaceholder(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>left</li><li><ul><li>right</li></ul></li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),items=edit.querySelectorAll("li"),start=items[0].firstChild,end=items[2].firstChild,selection=getSelection();selection.setBaseAndExtent(start,0,end,end.length);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare selection through nested list: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete should join the selected outer item with the following nested list")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><li><ul><li><br></li></ul></li></ul>|1" {
		t.Fatalf("contenteditable after nested-list join = %q, err %v; want preserved nested placeholder and one input event", got, err)
	}
}

func TestContentEditableSelectionDeleteRemovesItemBeforeNestedListSibling(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>list-item1</li><ul><li>list-item2</li></ul></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),items=edit.querySelectorAll("li"),start=items[0].firstChild,end=items[1],selection=getSelection();selection.setBaseAndExtent(start,0,end,0);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare selection before nested-list sibling: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete should remove the selected list item before nested-list content")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><ul><li>list-item2</li></ul></ul>|1" {
		t.Fatalf("contenteditable after deleting item before nested list = %q, err %v; want nested list and one input event", got, err)
	}
}

func TestContentEditableSelectionDeleteJoinsAdjacentDifferentLists(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>left</li></ul><ol><li>right</li></ol></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),left=edit.querySelector("ul > li").firstChild,right=edit.querySelector("ol > li"),selection=getSelection();selection.setBaseAndExtent(left,left.length,right,0);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare cross-list selection: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete should join list items across adjacent list roots")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><li>leftright</li></ul>|1" {
		t.Fatalf("contenteditable after cross-list join = %q, err %v; want one merged item and one input event", got, err)
	}
}

func TestContentEditableCrossListJoinRetainsFollowingLine(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>left</li></ul><ol><li>right<br>second line</li></ol></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),left=edit.querySelector("ul > li").firstChild,right=edit.querySelector("ol > li"),selection=getSelection();selection.setBaseAndExtent(left,left.length,right,0);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare cross-list line-break selection: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete should join the first line across adjacent list roots")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><li>leftright</li></ul><ol><li>second line</li></ol>|1" {
		t.Fatalf("contenteditable after cross-list line join = %q, err %v; want joined first line, retained second line, and one input event", got, err)
	}
}

func TestContentEditableSelectingNoneditableListPreservesListPlaceholder(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li contenteditable="false">locked</li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),selection=getSelection();selection.setBaseAndExtent(edit,0,edit,1);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare noneditable list selection: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should clear the selected list while preserving its structure")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><li><br></li></ul>|1" {
		t.Fatalf("contenteditable after selecting a noneditable list = %q, err %v; want list placeholder and one input event", got, err)
	}
}

func TestContentEditableSelectingTrailingListItemPreservesPlaceholder(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>first</li><li>last</li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),list=edit.firstChild,selection=getSelection();selection.setBaseAndExtent(list,1,list,2)})()`); err != nil {
		t.Fatalf("prepare trailing list-item selection: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should clear the selected trailing list item")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != "<ul><li>first</li><li><br></li></ul>" {
		t.Fatalf("contenteditable after selecting trailing list item = %q, err %v; want trailing list placeholder", got, err)
	}
}

func TestContentEditableDeleteSelectionMergesAdjacentParagraphs(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><p>abc</p><p>def</p></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),first=edit.firstChild.firstChild,last=edit.lastChild.firstChild,selection=getSelection();selection.setBaseAndExtent(first,2,last,1)})()`); err != nil {
		t.Fatalf("prepare cross-paragraph selection: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should delete the selected text across paragraphs")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != "<p>abef</p>" {
		t.Fatalf("contenteditable after cross-paragraph deletion = %q, err %v; want <p>abef</p>", got, err)
	}
}

func TestContentEditableDeleteSelectionMergesAdjacentDefinitionLists(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><dl><dt>abc</dt></dl><dl><dt>def</dt></dl></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),first=edit.firstChild.firstChild.firstChild,last=edit.lastChild.firstChild.firstChild,selection=getSelection();selection.setBaseAndExtent(first,2,last,1)})()`); err != nil {
		t.Fatalf("prepare cross-definition-list selection: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should delete the selected text across definition lists")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != "<dl><dt>abef</dt></dl>" {
		t.Fatalf("contenteditable after cross-definition-list deletion = %q, err %v; want <dl><dt>abef</dt></dl>", got, err)
	}
}

func TestContentEditableDeleteSelectionMergesDifferentDefinitionListItems(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><dl><dt>alpha</dt></dl><dl><dd>beta</dd></dl></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),first=edit.firstChild.firstChild.firstChild,last=edit.lastChild.firstChild.firstChild,selection=getSelection();selection.setBaseAndExtent(first,first.data.length,last,0)})()`); err != nil {
		t.Fatalf("prepare cross-type definition-list selection: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should join different definition item types across lists")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != "<dl><dt>alphabeta</dt></dl>" {
		t.Fatalf("contenteditable after cross-type definition-list join = %q, err %v; want <dl><dt>alphabeta</dt></dl>", got, err)
	}
}

func TestContentEditableDeleteSelectionRemovesTermBeforeFollowingDefinition(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><dl><dt>term</dt></dl><dl><dd>definition</dd></dl></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),term=edit.querySelector("dt").firstChild,definition=edit.querySelector("dd").firstChild;getSelection().setBaseAndExtent(term,0,definition,0)})()`); err != nil {
		t.Fatalf("prepare selection of definition term: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should remove the selected term and preserve its following definition")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != "<dl><dd>definition</dd></dl>" {
		t.Fatalf("contenteditable after term deletion = %q, err %v; want <dl><dd>definition</dd></dl>", got, err)
	}
}

func TestContentEditableDeleteSelectionRemovesTermAtDefinitionElementBoundary(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><dl><dt>kept</dt><dt>term</dt></dl><dl><dd>definition</dd></dl></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),term=edit.querySelector("dl dt:last-child").firstChild,definition=edit.querySelector("dd");getSelection().setBaseAndExtent(term,0,definition,0)})()`); err != nil {
		t.Fatalf("prepare element-boundary definition selection: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should remove the selected term")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != "<dl><dt>kept</dt></dl><dl><dd>definition</dd></dl>" {
		t.Fatalf("contenteditable after element-boundary term deletion = %q, err %v", got, err)
	}
}

func TestContentEditableBackspaceAndDeleteJoinDefinitionListsWithLists(t *testing.T) {
	cases := []struct {
		name, html, placeCaret, want string
		action                       engine.EditAction
	}{
		{
			name:       "Backspace joins a definition item into the preceding unordered list",
			html:       `<ul><li>one</li></ul><dl><dt>two</dt></dl>`,
			placeCaret: `var text=edit.querySelector("dt").firstChild;selection.collapse(text,0)`,
			want:       `<ul><li>onetwo</li></ul>`,
			action:     engine.EditBackspace,
		},
		{
			name:       "Delete joins the first definition item into the preceding list item and preserves later definitions",
			html:       `<ul><li>one</li></ul><dl><dd>two</dd><dt>keep</dt></dl>`,
			placeCaret: `var text=edit.querySelector("li").firstChild;selection.collapse(text,text.data.length)`,
			want:       `<ul><li>onetwo</li></ul><dl><dt>keep</dt></dl>`,
			action:     engine.EditDelete,
		},
		{
			name:       "Backspace joins the following list item into the final definition item",
			html:       `<dl><dt>one</dt><dd>two</dd></dl><ul><li>three</li></ul>`,
			placeCaret: `var text=edit.querySelector("ul li").firstChild;selection.collapse(text,0)`,
			want:       `<dl><dt>one</dt><dd>twothree</dd></dl>`,
			action:     engine.EditBackspace,
		},
		{
			name:       "Delete joins the following list item into the final definition item",
			html:       `<dl><dt>one</dt><dd>two</dd></dl><ul><li>three</li></ul>`,
			placeCaret: `var text=edit.querySelector("dd").firstChild;selection.collapse(text,text.length)`,
			want:       `<dl><dt>one</dt><dd>twothree</dd></dl>`,
			action:     engine.EditDelete,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := js.New(js.Options{})
			if err != nil {
				t.Fatal(err)
			}
			s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">`+tc.html+`</div></body></html>`, nil, 800, engine.WithJS(rt))
			if err != nil {
				_ = rt.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			edit := findElement(&s.Doc.Node, "div")
			if edit == nil || !s.FocusNode(edit) {
				t.Fatal("contenteditable host should receive focus")
			}
			if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),selection=getSelection();` + tc.placeCaret + `})()`); err != nil {
				t.Fatalf("position caret: %v", err)
			}
			if !s.Edit(tc.action, 0) {
				t.Fatal("list join should mutate the editing host")
			}
			if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != tc.want {
				t.Fatalf("contenteditable after list join = %q, err %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestContentEditableDeleteSelectionRemovesEmptyTrailingDefinitionList(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><dl><dt>list-item1</dt></dl><dl><dt>list-item3</dt><dd>list-item4</dd></dl></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),first=edit.firstChild.firstChild.firstChild,last=edit.lastChild.lastChild.firstChild,selection=getSelection();selection.setBaseAndExtent(first,0,last,last.data.length)})()`); err != nil {
		t.Fatalf("prepare cross-definition-list selection: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should delete a selection spanning definition lists")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != "<dl><dt><br></dt></dl>" {
		t.Fatalf("contenteditable after cross-definition-list selection deletion = %q, err %v; want <dl><dt><br></dt></dl>", got, err)
	}
}

func TestContentEditableBackspaceJoinsNestedListParagraph(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>list-item1</li><li><p>list-item2</p></li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),text=edit.querySelector("li p").firstChild,range=document.createRange(),selection=getSelection();range.setStart(text,0);range.collapse(true);selection.removeAllRanges();selection.addRange(range);window.inputCount=0;edit.addEventListener("input",function(){inputCount++})})()`); err != nil {
		t.Fatalf("prepare nested list paragraph caret: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace at the nested paragraph start should join the list items")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+inputCount`); err != nil || got != "<ul><li>list-item1list-item2</li></ul>|1" {
		t.Fatalf("contenteditable after nested list join = %q, err %v; want joined item and one input event", got, err)
	}
}

func TestContentEditableDeleteJoinsAdjacentListItems(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>first</li><li>second</li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),text=edit.querySelector("li").firstChild,selection=getSelection();selection.collapse(text,text.length);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare list-item end caret: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete at the end of a list item should merge the next item")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><li>firstsecond</li></ul>|1" {
		t.Fatalf("contenteditable after forward list join = %q, err %v; want merged item and one input event", got, err)
	}
}

func TestContentEditableDeleteJoinsListItemParagraph(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li><p>first</p></li><li>second</li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),text=edit.querySelector("p").firstChild,selection=getSelection();selection.collapse(text,text.length);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare list-item paragraph caret: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete at the end of a list-item paragraph should join the next item")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><li><p>firstsecond</p></li></ul>|1" {
		t.Fatalf("contenteditable after paragraph list join = %q, err %v; want merged paragraph and one input event", got, err)
	}
}

func TestContentEditableDeleteJoinsPlainListItemIntoFollowingParagraph(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>first</li><li><p>second</p></li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),text=edit.querySelector("li").firstChild,selection=getSelection();selection.collapse(text,text.length);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare list-item end caret: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete at the end of a plain list item should join a following paragraph item")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><li>firstsecond</li></ul>|1" {
		t.Fatalf("contenteditable after forward plain-to-paragraph list join = %q, err %v; want merged item and one input event", got, err)
	}
}

func TestContentEditableBackspaceJoinsIntoPreviousListItemParagraph(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li><p>first</p></li><li>second</li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),text=edit.querySelector("li + li").firstChild,selection=getSelection();selection.collapse(text,0);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare second list-item caret: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace at the start of a list item should join into the preceding paragraph")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><li><p>firstsecond</p></li></ul>|1" {
		t.Fatalf("contenteditable after backward paragraph list join = %q, err %v; want merged paragraph and one input event", got, err)
	}
}

func TestContentEditableListJoinsSplitAtLineBreak(t *testing.T) {
	cases := []struct {
		name  string
		html  string
		setup string
		key   engine.EditAction
	}{
		{
			name:  "forward delete",
			html:  `<ul><li>first</li><li>second<br>third</li></ul>`,
			setup: `var text=edit.querySelector("li").firstChild;selection.collapse(text,text.length)`,
			key:   engine.EditDelete,
		},
		{
			name:  "backspace",
			html:  `<ul><li>first</li><li>second<br>third</li></ul>`,
			setup: `var text=edit.querySelector("li + li").firstChild;selection.collapse(text,0)`,
			key:   engine.EditBackspace,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := js.New(js.Options{})
			if err != nil {
				t.Fatal(err)
			}
			s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">`+tc.html+`</div></body></html>`, nil, 800, engine.WithJS(rt))
			if err != nil {
				_ = rt.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			edit := findElement(&s.Doc.Node, "div")
			if edit == nil || !s.FocusNode(edit) {
				t.Fatal("contenteditable host should receive focus")
			}
			if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),selection=getSelection();` + tc.setup + `;window.inputCount=0;edit.addEventListener("input",function(){inputCount++})})()`); err != nil {
				t.Fatalf("prepare list line-break caret: %v", err)
			}
			if !s.Edit(tc.key, 0) {
				t.Fatal("list deletion should merge text before the break and preserve the following line")
			}
			if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+inputCount`); err != nil || got != "<ul><li>firstsecond</li><li>third</li></ul>|1" {
				t.Fatalf("contenteditable after list line-break join = %q, err %v; want split list items and one input event", got, err)
			}
		})
	}
}

func TestContentEditableBackspaceLineBreakTargetRange(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>first</li><li>second<br>third</li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),text=edit.querySelector("li + li").firstChild,selection=getSelection();selection.collapse(text,0);window.selectionInfo=[selection.anchorNode.nodeName,selection.anchorOffset].join(":");window.targetRanges=[];edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];targetRanges.push([range.startContainer.nodeName,range.startOffset,range.endContainer.nodeName,range.endOffset].join(":"))})})()`); err != nil {
		t.Fatalf("prepare line-break Backspace target range: %v", err)
	}
	if !rt.DispatchTrustedBeforeInputEvent(edit, nil, "deleteContentBackward", false, true) {
		t.Fatal("direct beforeinput dispatch should succeed")
	}
	if got, err := s.EvaluateJS(`targetRanges[0]+"|"+window.selectionInfo`); err != nil || got != "LI:1:#text:0|#text:0" {
		t.Fatalf("direct Backspace line-break target range and caret = %q, err %v; want LI:1:#text:0|#text:0", got, err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace at the start of a list item should join it with the previous item")
	}
	if got, err := s.EvaluateJS(`targetRanges[1]+"|"+window.selectionInfo`); err != nil || got != "LI:1:#text:0|#text:0" {
		t.Fatalf("Backspace line-break target range and initial caret = %q, err %v; want LI:1:#text:0|#text:0", got, err)
	}
}

func TestContentEditableNestedCrossListTargetRange(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li><ul><li>left</li></ul></li></ul><ul><li>right</li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),lists=edit.querySelectorAll("ul"),start=lists[1].firstChild.firstChild,end=lists[2].firstChild,selection=getSelection();selection.setBaseAndExtent(start,0,end,0);edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];window.nestedListTargetRange=[range.startContainer.data,range.startOffset,range.endContainer.data,range.endOffset].join(":")})})()`); err != nil {
		t.Fatalf("prepare nested cross-list selection: %v", err)
	}
	if !rt.DispatchTrustedBeforeInputEvent(edit, nil, "deleteContentBackward", false, true) {
		t.Fatal("direct beforeinput dispatch should succeed")
	}
	if got, err := s.EvaluateJS(`window.nestedListTargetRange`); err != nil || got != "left:0:right:0" {
		t.Fatalf("nested cross-list target range = %q, err %v; want left:0:right:0", got, err)
	}
}

func TestContentEditableBackspaceRemovesEmptySoleListItem(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li><br></li></ul></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),item=edit.querySelector("li"),selection=getSelection();selection.collapse(item,0);window.emptyItemEvents=[];edit.addEventListener("beforeinput",function(event){var ranges=event.getTargetRanges();emptyItemEvents.push(event.inputType+":"+(ranges[0].startContainer===edit)+":"+ranges[0].startOffset+":"+ranges[0].endOffset)});edit.addEventListener("input",function(event){emptyItemEvents.push("input:"+event.inputType)})})()`); err != nil {
		t.Fatalf("prepare empty list-item caret: %v", err)
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should remove the sole empty list item")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+emptyItemEvents.join("|")`); err != nil || got != `<br>|deleteContentBackward:true:0:1|input:deleteContentBackward` {
		t.Fatalf("contenteditable after removing empty list item = %q, err %v; want host placeholder, outer-list target range, and one input event", got, err)
	}
}

func TestContentEditableEmptyNestedListCreatesOuterListItemPlaceholder(t *testing.T) {
	for _, test := range []struct {
		name string
		html string
		want string
	}{
		{
			name: "sole outer list item",
			html: `<html><body><div id="edit" contenteditable="true"><ol><ul><li><br></li></ul></ol></div></body></html>`,
			want: `<ol><li><br></li></ol>`,
		},
		{
			name: "preserve following outer list item",
			html: `<html><body><div id="edit" contenteditable="true"><ol><ul><li><br></li></ul><li>next</li></ol></div></body></html>`,
			want: `<ol><li><br></li><li>next</li></ol>`,
		},
	} {
		for _, action := range []engine.EditAction{engine.EditBackspace, engine.EditDelete} {
			t.Run(test.name+map[engine.EditAction]string{engine.EditBackspace: "/backspace", engine.EditDelete: "/delete"}[action], func(t *testing.T) {
				rt, err := js.New(js.Options{})
				if err != nil {
					t.Fatal(err)
				}
				s, err := engine.NewSession(test.html, nil, 800, engine.WithJS(rt))
				if err != nil {
					_ = rt.Close()
					t.Fatal(err)
				}
				t.Cleanup(func() { s.Close() })
				edit := findElement(&s.Doc.Node, "div")
				if edit == nil || !s.FocusNode(edit) {
					t.Fatal("contenteditable host should receive focus")
				}
				if _, err := s.EvaluateJS(`(function(){var item=document.querySelector("ol > ul > li"),selection=getSelection();selection.collapse(item,0);window.emptyItemInputCount=0;document.getElementById("edit").addEventListener("input",function(){emptyItemInputCount++})})()`); err != nil {
					t.Fatalf("prepare empty nested list-item caret: %v", err)
				}
				if !s.Edit(action, 0) {
					t.Fatal("deleting from the empty nested item should update the outer ordered list")
				}
				if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+emptyItemInputCount`); err != nil || got != test.want+"|1" {
					t.Fatalf("contenteditable after empty nested-list deletion = %q, err %v; want %q|1", got, err, test.want)
				}
			})
		}
	}
}

func TestContentEditableCrossListJoinUsesLastNestedLeftItem(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li><ul><li>left</li></ul></li></ul><ol><li>right</li></ol></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),left=edit.querySelector("ul ul > li").firstChild,right=edit.querySelector("ol > li"),selection=getSelection();selection.setBaseAndExtent(left,0,right,0);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare nested-left cross-list selection: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete should join an adjacent list item into the last nested left item")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><li><ul><li>right</li></ul></li></ul>|1" {
		t.Fatalf("contenteditable after nested-left cross-list join = %q, err %v; want joined nested item and one input event", got, err)
	}
}

func TestContentEditableCrossListJoinFlattensFirstNestedRightItem(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>left</li></ul><ol><li><ul><li>right</li></ul></li></ol></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),left=edit.querySelector("ul > li").firstChild,right=edit.querySelector("ol ul li").firstChild,selection=getSelection();selection.setBaseAndExtent(left,left.length,right,0);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare cross-list selection into nested right item: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete should join the first nested right item into the preceding list")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ul><li>leftright</li></ul>|1" {
		t.Fatalf("contenteditable after nested-right cross-list join = %q, err %v; want flattened joined item and one input event", got, err)
	}
}

func TestContentEditableCrossListDeleteKeepsRightRootWhenStartItemEmpties(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><ul><li>left</li></ul><ol><li><ul><li>right</li></ul></li></ol></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := findElement(&s.Doc.Node, "div")
	if edit == nil || !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit"),left=edit.firstChild.firstChild.firstChild,right=edit.lastChild.firstChild.firstChild.firstChild.firstChild,selection=getSelection();selection.setBaseAndExtent(left,0,right,0);window.listInputCount=0;edit.addEventListener("input",function(){listInputCount++})})()`); err != nil {
		t.Fatalf("prepare empty-start cross-list selection: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete should retain the right list when the selected start item becomes empty")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+listInputCount`); err != nil || got != "<ol><li><ul><li>right</li></ul></li></ol>|1" {
		t.Fatalf("contenteditable after empty-start cross-list delete = %q, err %v; want right list and one input event", got, err)
	}
}

func TestContentEditableSelectingNestedListKeepsItsPlaceholderItem(t *testing.T) {
	for _, test := range []struct {
		name string
		html string
		list string
		want string
	}{
		{
			name: "list directly under list",
			html: `<html><body><div id="edit" contenteditable="true"><ol><ul><li>item</li></ul></ol></div></body></html>`,
			list: `document.querySelector("ol > ul")`,
			want: `<ol><ul><li><br></li></ul></ol>`,
		},
		{
			name: "list nested in list item",
			html: `<html><body><div id="edit" contenteditable="true"><ol><li><ul><li>item</li></ul></li></ol></div></body></html>`,
			list: `document.querySelector("li > ul")`,
			want: `<ol><li><ul><li><br></li></ul></li></ol>`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			rt, err := js.New(js.Options{})
			if err != nil {
				t.Fatal(err)
			}
			s, err := engine.NewSession(test.html, nil, 800, engine.WithJS(rt))
			if err != nil {
				_ = rt.Close()
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			edit := findElement(&s.Doc.Node, "div")
			if edit == nil || !s.FocusNode(edit) {
				t.Fatal("contenteditable host should receive focus")
			}
			if _, err := s.EvaluateJS(`(function(){var list=` + test.list + `,parent=list.parentNode,offset=Array.prototype.indexOf.call(parent.childNodes,list),selection=getSelection();selection.setBaseAndExtent(parent,offset,parent,offset+1);window.nestedListInputCount=0;document.getElementById("edit").addEventListener("input",function(){nestedListInputCount++})})()`); err != nil {
				t.Fatalf("prepare nested-list selection: %v", err)
			}
			if !s.Edit(engine.EditDelete, 0) {
				t.Fatal("Delete should clear selected nested-list content")
			}
			if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+nestedListInputCount`); err != nil || got != test.want+"|1" {
				t.Fatalf("contenteditable after selecting nested list = %q, err %v; want %q|1", got, err, test.want)
			}
		})
	}
}

func TestTextControlKeyboardEditsUseInputEventsAndHonorCancellation(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><input id="field" value="abcd"></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	field := s.Doc.ElementByID("field")
	if !s.FocusNode(field) {
		t.Fatal("text input should receive focus")
	}
	s.Edit(engine.EditHome, 0)
	s.Edit(engine.EditRight, 0)
	s.Edit(engine.EditRight, 0)
	if _, err := s.EvaluateJS(`(function(){var f=document.getElementById("field");window.editEvents=[];for(const type of ["beforeinput","input"])f.addEventListener(type,function(e){editEvents.push(type+":"+e.inputType+":"+e.data+":"+(e instanceof InputEvent)+":"+e.isTrusted+":"+e.cancelable)})})()`); err != nil {
		t.Fatalf("prepare event listeners: %v", err)
	}
	if !s.Edit(engine.EditRune, 'X') {
		t.Fatal("text insertion should change the value")
	}
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("Backspace should delete the preceding character")
	}
	if !s.Edit(engine.EditDelete, 0) {
		state, evalErr := s.EvaluateJS(`document.getElementById("field").value+":"+document.getElementById("field").selectionStart+":"+document.getElementById("field").selectionEnd`)
		t.Fatalf("Delete should delete the character after the caret; state=%q err=%v", state, evalErr)
	}
	if got := field.InputControlValue(); got != "abd" {
		t.Fatalf("value after insert, Backspace, Delete = %q, want %q", got, "abd")
	}
	if got, err := s.EvaluateJS(`editEvents.join("|")`); err != nil || got != "beforeinput:insertText:X:true:true:true|input:insertText:X:true:true:false|beforeinput:deleteContentBackward:null:true:true:true|input:deleteContentBackward:null:true:true:false|beforeinput:deleteContentForward:null:true:true:true|input:deleteContentForward:null:true:true:false" {
		t.Fatalf("text control input events = %q, err %v", got, err)
	}
	if _, err := s.EvaluateJS(`document.getElementById("field").addEventListener("beforeinput",function(e){if(e.inputType==="insertText")e.preventDefault()})`); err != nil {
		t.Fatalf("prepare canceled insertion: %v", err)
	}
	if s.Edit(engine.EditRune, 'Z') {
		t.Fatal("cancelled beforeinput should suppress insertion")
	}
	if got := field.InputControlValue(); got != "abd" {
		t.Fatalf("value after canceled insertion = %q, want abd", got)
	}
}

func TestPasteTextInputEventsExposePlainTextDataTransferForContentEditable(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><input id="field" value="start:"><div id="edit" contenteditable="true">ab</div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	field, edit := s.Doc.ElementByID("field"), s.Doc.ElementByID("edit")
	if !s.FocusNode(field) {
		t.Fatal("text input should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var f=document.getElementById("field");f.setSelectionRange(f.value.length);window.controlPasteEvents=[];window.clipboardPasteEvents=[];f.addEventListener("paste",function(e){savedClipboardTransfer=e.clipboardData;clipboardPasteEvents.push((e instanceof ClipboardEvent)+":"+(e instanceof Event)+":"+(e.clipboardData instanceof DataTransfer)+":"+e.clipboardData.getData("text/plain")+":"+e.bubbles+":"+e.cancelable+":"+e.isTrusted)});for(const type of ["beforeinput","input"])f.addEventListener(type,function(e){controlPasteEvents.push(type+":"+e.inputType+":"+e.data+":"+(e instanceof InputEvent)+":"+e.isTrusted+":"+(e.dataTransfer===null))})})()`); err != nil {
		t.Fatalf("prepare text-control paste listeners: %v", err)
	}
	if start, end, _, ok := rt.TextControlSelection(field); !ok || start != 6 || end != 6 {
		t.Fatalf("runtime textarea selection = %d:%d (ok=%v), want 6:6", start, end, ok)
	}
	if !s.PasteText("clip") {
		t.Fatal("paste into text input should change its value")
	}
	if got := field.InputControlValue(); got != "start:clip" {
		t.Fatalf("text input after paste = %q, want start:clip", got)
	}
	if got, err := s.EvaluateJS(`controlPasteEvents.join("|")`); err != nil || got != "beforeinput:insertFromPaste:clip:true:true:true|input:insertFromPaste:clip:true:true:true" {
		t.Fatalf("text-control paste events = %q, err %v", got, err)
	}
	if got, err := s.EvaluateJS(`clipboardPasteEvents.join("|")+":"+savedClipboardTransfer.getData("text/plain")`); err != nil || got != "true:true:true:clip:true:true:true:" {
		t.Fatalf("ClipboardEvent paste payload and lifetime = %q, err %v", got, err)
	}
	if _, err := s.EvaluateJS(`document.getElementById("field").addEventListener("paste",function(e){e.preventDefault()})`); err != nil {
		t.Fatalf("prepare canceled paste listener: %v", err)
	}
	if s.PasteText("blocked") {
		t.Fatal("prevented paste event should suppress the default edit")
	}
	if got := field.InputControlValue(); got != "start:clip" {
		t.Fatalf("text input after canceled paste = %q, want start:clip", got)
	}
	if got, err := s.EvaluateJS(`controlPasteEvents.length+":"+clipboardPasteEvents.length`); err != nil || got != "2:2" {
		t.Fatalf("canceled paste should dispatch paste without beforeinput/input, got %q err=%v", got, err)
	}
	if !s.FocusNode(edit) {
		t.Fatal("contenteditable should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var host=document.getElementById("edit"),text=host.firstChild,r=document.createRange();r.setStart(text,1);r.collapse(true);var sel=getSelection();sel.removeAllRanges();sel.addRange(r);window.editPasteEvents=[];host.addEventListener("beforeinput",function(e){var dt=e.dataTransfer;var text=dt&&dt.getData("text/plain");if(dt){try{dt.setData("text/plain","changed")}catch(_){}}window.savedPasteTransfer=dt;editPasteEvents.push(e.inputType+":"+e.data+":"+(dt instanceof DataTransfer)+":"+text+":"+(dt&&dt.getData("text/plain"))+":"+e.isTrusted+":"+e.cancelable)});host.addEventListener("input",function(e){editPasteEvents.push(e.inputType+":"+e.data+":"+(e.dataTransfer===null)+":"+e.isTrusted+":"+e.cancelable)})})()`); err != nil {
		t.Fatalf("prepare contenteditable paste listeners: %v", err)
	}
	if !s.PasteText("clip") {
		t.Fatal("paste into contenteditable should change its text")
	}
	if got := edit.TextContent(); got != "aclipb" {
		t.Fatalf("contenteditable after paste = %q, want aclipb", got)
	}
	if got, err := s.EvaluateJS(`editPasteEvents.join("|")+";"+savedPasteTransfer.getData("text/plain")`); err != nil || got != "insertFromPaste:null:true:clip:clip:true:true|insertFromPaste:clip:true:true:false;" {
		t.Fatalf("contenteditable paste events and post-dispatch DataTransfer = %q, err %v", got, err)
	}
}

func TestTextControlScriptSelectionDrivesClipboardAndPaste(t *testing.T) {
	var s *engine.Session
	rt, err := js.New(js.Options{OnTestDriverKeyDefault: func(string, bool, bool, bool, bool) {}})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err = engine.NewSession(`<html><body><textarea id="field">abcdef</textarea></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	field := s.Doc.ElementByID("field")
	if !s.FocusNode(field) {
		t.Fatal("textarea should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var field=document.getElementById("field");field.setSelectionRange(1,4);field.addEventListener("input",function(event){window.pastedData=event.data})})()`); err != nil {
		t.Fatalf("select textarea text from script: %v", err)
	}
	if _, err := s.EvaluateJS(`window.__goosieTestDriverKeyDefault("c",false,true,false,false);window.__goosieTestDriverKeyDefault("v",false,true,false,false)`); err != nil {
		t.Fatalf("dispatch testdriver copy/paste defaults: %v", err)
	}
	if got := field.InputControlValue(); got != "abcdef" {
		t.Fatalf("textarea value after selected copy/paste = %q, want abcdef", got)
	}
	if got, err := s.EvaluateJS(`window.pastedData`); err != nil || got != "bcd" {
		t.Fatalf("paste InputEvent.data = %q, err %v; want bcd", got, err)
	}
}

func TestCopyAndCutSelectionDispatchClipboardAndInputEvents(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><input id="field" value="abcd"><input id="cancel" value="xyz"><div id="edit" contenteditable="true"><b>abc</b></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	field := s.Doc.ElementByID("field")
	if !s.FocusNode(field) || !s.SelectAllControl() {
		t.Fatal("text input selection should be established")
	}
	if _, err := s.EvaluateJS(`(function(){var f=document.getElementById("field");window.clipboardEvents=[];f.addEventListener("copy",function(e){savedCopyTransfer=e.clipboardData;clipboardEvents.push("copy:"+(e instanceof ClipboardEvent)+":"+e.isTrusted+":"+e.clipboardData.getData("text/plain"));e.clipboardData.setData("text/plain","copy override")});f.addEventListener("cut",function(e){savedCutTransfer=e.clipboardData;clipboardEvents.push("cut:"+(e instanceof ClipboardEvent)+":"+e.isTrusted+":"+e.clipboardData.getData("text/plain"));e.clipboardData.setData("text/plain","cut override")});for(const type of ["beforeinput","input"])f.addEventListener(type,function(e){clipboardEvents.push(type+":"+e.inputType+":"+(e instanceof InputEvent)+":"+e.isTrusted+":"+e.cancelable)})})()`); err != nil {
		t.Fatalf("prepare clipboard event listeners: %v", err)
	}
	if got := s.CopySelection(); got != "copy override" {
		t.Fatalf("copy payload = %q, want handler-provided text", got)
	}
	if got, changed := s.CutSelection(); got != "cut override" || !changed {
		t.Fatalf("cut result = (%q, %v), want handler-provided text and deletion", got, changed)
	}
	if got := field.InputControlValue(); got != "" {
		t.Fatalf("text input after cut = %q, want empty", got)
	}
	if got, err := s.EvaluateJS(`clipboardEvents.join("|")+":"+savedCopyTransfer.getData("text/plain")+":"+savedCutTransfer.getData("text/plain")`); err != nil || got != "copy:true:true:abcd|cut:true:true:abcd|beforeinput:deleteByCut:true:true:true|input:deleteByCut:true:true:false::" {
		t.Fatalf("text-control clipboard event sequence = %q, err %v", got, err)
	}

	cancel := s.Doc.ElementByID("cancel")
	if !s.FocusNode(cancel) || !s.SelectAllControl() {
		t.Fatal("cancel test selection should be established")
	}
	if _, err := s.EvaluateJS(`document.getElementById("cancel").addEventListener("cut",function(e){e.preventDefault()});`); err != nil {
		t.Fatalf("prepare canceled cut: %v", err)
	}
	if _, changed := s.CutSelection(); changed {
		t.Fatal("prevented cut event should suppress deletion")
	}
	if got := cancel.InputControlValue(); got != "xyz" {
		t.Fatalf("text input after canceled cut = %q, want xyz", got)
	}

	edit := s.Doc.ElementByID("edit")
	if !s.FocusNode(edit) {
		t.Fatal("contenteditable should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var text=document.getElementById("edit").firstChild.firstChild,r=document.createRange();r.setStart(text,1);r.setEnd(text,2);var sel=getSelection();sel.removeAllRanges();sel.addRange(r);window.editCutEvents=[];var host=document.getElementById("edit");host.addEventListener("cut",function(e){editCutEvents.push((e.clipboardData instanceof DataTransfer)+":"+e.clipboardData.getData("text/plain")+":"+e.clipboardData.getData("text/html"))});host.addEventListener("beforeinput",function(e){editCutEvents.push(e.inputType+":"+e.data+":"+(e instanceof InputEvent))});host.addEventListener("input",function(e){editCutEvents.push(e.inputType+":"+e.data+":"+e.isTrusted)})})()`); err != nil {
		t.Fatalf("prepare contenteditable cut: %v", err)
	}
	if plain, html := s.CopySelectionData(); plain != "b" || html != "<b>b</b>" {
		t.Fatalf("contenteditable copy = (%q, %q), want plain b and rich markup", plain, html)
	}
	if plain, html, changed := s.CutSelectionData(); plain != "b" || html != "<b>b</b>" || !changed {
		t.Fatalf("contenteditable cut result = (%q, %q, %v), want plain/rich selection and deletion", plain, html, changed)
	}
	if got := edit.TextContent(); got != "ac" {
		t.Fatalf("contenteditable text after cut = %q, want ac", got)
	}
	if got, err := s.EvaluateJS(`editCutEvents.join("|")`); err != nil || got != "true:b:<b>b</b>|deleteByCut:null:true|deleteByCut:null:true" {
		t.Fatalf("contenteditable cut events = %q, err %v", got, err)
	}
}

func TestContentEditableRichPasteUsesBoundedHTMLInsertion(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">ab</div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := s.Doc.ElementByID("edit")
	if !s.FocusNode(edit) {
		t.Fatal("contenteditable should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var host=document.getElementById("edit"),text=host.firstChild,r=document.createRange();r.setStart(text,1);r.collapse(true);var sel=getSelection();sel.removeAllRanges();sel.addRange(r);window.richPasteEvents=[];host.addEventListener("paste",function(e){richPasteEvents.push("paste:"+e.clipboardData.getData("text/plain")+":"+e.clipboardData.getData("text/html"))});host.addEventListener("beforeinput",function(e){richPasteEvents.push("before:"+e.inputType+":"+e.data+":"+e.dataTransfer.getData("text/plain")+":"+e.dataTransfer.getData("text/html")+":"+e.isTrusted)});host.addEventListener("input",function(e){richPasteEvents.push("input:"+e.inputType+":"+e.data+":"+e.isTrusted)})})()`); err != nil {
		t.Fatalf("prepare rich paste: %v", err)
	}
	if !s.PasteData("rich", "<b>rich</b>") {
		t.Fatal("rich paste should change the editing host")
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != "a<b>rich</b>b" {
		t.Fatalf("contenteditable HTML after rich paste = %q, err %v", got, err)
	}
	if got, err := s.EvaluateJS(`richPasteEvents.join("|")`); err != nil || got != "paste:rich:<b>rich</b>|before:insertFromPaste:null:rich:<b>rich</b>:true|input:insertFromPaste:null:true" {
		t.Fatalf("rich paste event payloads = %q, err %v", got, err)
	}
}

func TestTestDriverKeyDefaultDeletesFocusedContentEditableSelection(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">hello world</div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`(function(){var host=document.getElementById("edit"),text=host.firstChild,range=document.createRange(),selection=getSelection();window.deleteEvents=[];host.addEventListener("beforeinput",function(e){deleteEvents.push(e.type+":"+e.inputType)});host.addEventListener("input",function(e){deleteEvents.push(e.type+":"+e.inputType)});host.focus();range.setStart(text,6);range.setEnd(text,8);selection.removeAllRanges();selection.addRange(range);window.__goosieTestDriverKeyDefault("Backspace",false,true,false,false)})()`); err != nil {
		t.Fatalf("run testdriver deletion: %v", err)
	}
	if got, err := s.EvaluateJS(`deleteEvents.join("|")`); err != nil || got != "beforeinput:deleteContentBackward|input:deleteContentBackward" {
		t.Fatalf("testdriver deletion events = %q, err %v", got, err)
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").textContent`); err != nil || got != "hello rld" {
		t.Fatalf("testdriver deletion text = %q, err %v", got, err)
	}
	if _, err := s.EvaluateJS(`window.__goosieTestDriverKeyDefault("B",true,false,false,false);window.__goosieTestDriverKeyDefault("z",false,false,false,true);window.__goosieTestDriverKeyDefault("Z",true,false,false,true)`); err != nil {
		t.Fatalf("run shifted typing and history key defaults: %v", err)
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").textContent`); err != nil || got != "hello Brld" {
		t.Fatalf("shifted testdriver typing text = %q, err %v", got, err)
	}
	if got, err := s.EvaluateJS(`deleteEvents.slice(2).join("|")`); err != nil || got != "beforeinput:insertText|input:insertText|beforeinput:historyUndo|input:historyUndo|beforeinput:historyRedo|input:historyRedo" {
		t.Fatalf("shift and history key events = %q, err %v", got, err)
	}
}

func TestTestDriverKeyDefaultBoldDispatchesInputEvents(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">abc</div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`(function(){var host=document.getElementById("edit"),selection=getSelection();selection.selectAllChildren(host);window.boldEvents=[];host.addEventListener("beforeinput",function(e){boldEvents.push(e.type+":"+e.inputType)});host.addEventListener("input",function(e){boldEvents.push(e.type+":"+e.inputType)});host.focus();window.__goosieTestDriverKeyDefault("b",false,false,false,true)})()`); err != nil {
		t.Fatalf("run testdriver bold shortcut: %v", err)
	}
	if got, err := s.EvaluateJS(`boldEvents.join("|")`); err != nil || got != "beforeinput:formatBold|input:formatBold" {
		t.Fatalf("testdriver bold events = %q, err %v", got, err)
	}
	if got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`); err != nil || got != "<b>abc</b>" {
		t.Fatalf("testdriver bold markup = %q, err %v", got, err)
	}
}

func TestContentEditableBeforeInputCanCancelEngineTextEntry(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">ab</div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	edit := s.Doc.ElementByID("edit")
	if !s.FocusNode(edit) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var t=document.getElementById("edit").firstChild;var r=document.createRange();r.setStart(t,1);r.collapse(true);var q=getSelection();q.removeAllRanges();q.addRange(r);document.getElementById("edit").addEventListener("beforeinput",function(e){e.preventDefault()})})()`); err != nil {
		t.Fatalf("prepare canceled edit: %v", err)
	}
	if s.Edit(engine.EditRune, 'X') {
		t.Fatal("prevented beforeinput should not report a text change")
	}
	if got := edit.TextContent(); got != "ab" {
		t.Fatalf("canceled contenteditable text = %q, want %q", got, "ab")
	}
}

func TestContentEditableCaretMovesAcrossTextNodes(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true"><span>a</span><span>b</span></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	host := s.Doc.ElementByID("edit")
	if !s.FocusNode(host) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var host=document.getElementById("edit"),t=host.lastChild.firstChild,r=document.createRange();r.setStart(t,0);r.collapse(true);var q=getSelection();q.removeAllRanges();q.addRange(r)})()`); err != nil {
		t.Fatalf("position caret: %v", err)
	}
	if s.Edit(engine.EditLeft, 0) {
		t.Fatal("caret movement should not report a document mutation")
	}
	if !s.Edit(engine.EditRune, 'X') {
		t.Fatal("typing after cross-node Left should change the document")
	}
	if got := host.TextContent(); got != "aXb" {
		t.Fatalf("text after cross-node Left = %q, want %q", got, "aXb")
	}
	if s.Edit(engine.EditRight, 0) {
		t.Fatal("caret movement should not report a document mutation")
	}
	if !s.Edit(engine.EditRune, 'Y') {
		t.Fatal("typing after cross-node Right should change the document")
	}
	if got := host.TextContent(); got != "aXbY" {
		t.Fatalf("text after cross-node Right = %q, want %q", got, "aXbY")
	}
}

func TestContentEditableCaretDoesNotSplitSurrogatePair(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := engine.NewSession(`<html><body><div id="edit" contenteditable="true">A😀B</div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	host := s.Doc.ElementByID("edit")
	if !s.FocusNode(host) {
		t.Fatal("contenteditable host should receive focus")
	}
	if _, err := s.EvaluateJS(`(function(){var t=document.getElementById("edit").firstChild,r=document.createRange();r.setStart(t,1);r.collapse(true);var q=getSelection();q.removeAllRanges();q.addRange(r)})()`); err != nil {
		t.Fatalf("position caret before supplementary character: %v", err)
	}
	s.Edit(engine.EditRight, 0)
	if !s.Edit(engine.EditRune, 'X') {
		t.Fatal("typing after supplementary character should change the document")
	}
	if got := host.TextContent(); got != "A😀XB" {
		t.Fatalf("text after code point caret movement = %q, want %q", got, "A😀XB")
	}
	if _, err := s.EvaluateJS(`(function(){var t=document.getElementById("edit").firstChild,r=document.createRange();r.setStart(t,1);r.collapse(true);var q=getSelection();q.removeAllRanges();q.addRange(r)})()`); err != nil {
		t.Fatalf("position caret before supplementary character: %v", err)
	}
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("forward Delete should remove the complete supplementary character")
	}
	if got := host.TextContent(); got != "AXB" {
		t.Fatalf("text after surrogate-pair deletion = %q, want %q", got, "AXB")
	}
}

func TestEditBackspace(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("backspace should report change")
	}
	if got := s.Focused().InputControlValue(); got != "a" {
		t.Fatalf("value = %q, want %q", got, "a")
	}
	s.Edit(engine.EditBackspace, 0)
	if got := s.Focused().InputControlValue(); got != "" {
		t.Fatalf("value = %q, want empty", got)
	}
	if s.Edit(engine.EditBackspace, 0) {
		t.Fatal("backspace on empty should not report change")
	}
}

func TestEditDeleteForwardInControl(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.Edit(engine.EditLeft, 0)
	if !s.Edit(engine.EditDelete, 0) {
		t.Fatal("Delete should remove the character after the input caret")
	}
	if got := s.Focused().InputControlValue(); got != "a" {
		t.Fatalf("input value after Delete = %q, want %q", got, "a")
	}
}

func TestEditArrowsAndHomeEnd(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y) // caret at 2
	if s.Edit(engine.EditLeft, 0) {
		t.Fatal("caret move should not report change")
	}
	s.Edit(engine.EditRune, '-') // "a-b"
	s.Edit(engine.EditHome, 0)
	s.Edit(engine.EditRune, '>') // ">a-b"
	if got := s.Focused().InputControlValue(); got != ">a-b" {
		t.Fatalf("value = %q, want %q", got, ">a-b")
	}
	s.Edit(engine.EditEnd, 0)
	s.Edit(engine.EditRune, '<') // ">a-b<"
	if got := s.Focused().InputControlValue(); got != ">a-b<" {
		t.Fatalf("value = %q, want %q", got, ">a-b<")
	}
	if s.Edit(engine.EditRight, 0) {
		t.Fatal("right at end should not report change")
	}
}

func TestEditEscapeBlurs(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if !s.Edit(engine.EditEscape, 0) {
		t.Fatal("escape should report change")
	}
	if s.Focused() != nil {
		t.Fatal("escape should blur")
	}
}

func TestEditEnterInputIgnored(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if s.Edit(engine.EditEnter, 0) {
		t.Fatal("enter on input should not report change")
	}
}

const textareaHTML = `<html><body style="margin: 0;">` +
	`<textarea id="ta">hi</textarea></body></html>`

func TestEditEnterTextarea(t *testing.T) {
	s := newFocusSession(t, textareaHTML)
	x, y := clickPoint(t, s, "ta")
	if !s.FocusControl(x, y) {
		t.Fatal("textarea not focused")
	}
	if !s.Edit(engine.EditEnter, 0) {
		t.Fatal("enter on textarea should report change")
	}
	ta := s.Focused()
	if got := ta.InputControlValue(); got != "hi\n" {
		t.Fatalf("textarea live value = %q, want %q", got, "hi\n")
	}
	if got := ta.TextContent(); got != "hi" {
		t.Fatalf("textarea default text = %q, want unchanged %q", got, "hi")
	}
}

func TestEditMaxlength(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="a" value="abc" maxlength="3"></body></html>`)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if s.Edit(engine.EditRune, 'd') {
		t.Fatal("insert past maxlength should not report change")
	}
	if got := s.Focused().InputControlValue(); got != "abc" {
		t.Fatalf("value = %q, want %q", got, "abc")
	}
}

func TestEditNoFocusNoop(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	if s.Edit(engine.EditRune, 'x') {
		t.Fatal("edit without focus should not report change")
	}
}

func TestValueSurvivesReflow(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.Edit(engine.EditRune, 'z')
	if err := s.Reflow(800); err != nil {
		t.Fatalf("Reflow: %v", err)
	}
	if got := s.Focused().InputControlValue(); got != "abz" {
		t.Fatalf("value after reflow = %q, want %q", got, "abz")
	}
}

func textRunesIn(t *testing.T, dl *paint.LayerDL, color frame.Color) []rune {
	t.Helper()
	var out []rune
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == color {
			for _, g := range c.Text.Glyphs {
				out = append(out, g.Rune)
			}
		}
	}
	return out
}

func countFills(t *testing.T, dl *paint.LayerDL, color frame.Color) int {
	t.Helper()
	n := 0
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Color == color {
			n++
		}
	}
	return n
}

func TestPaintFocusedShowsValueCaretAndRing(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	if !s.FocusControl(x, y) {
		t.Fatal("focus not gained")
	}
	dl := s.Paint(1).Build(1)
	// Each input paints its value as one black text run.
	valueRun := ""
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == frame.RGB(0, 0, 0) {
			run := make([]rune, 0, len(c.Text.Glyphs))
			for _, g := range c.Text.Glyphs {
				run = append(run, g.Rune)
			}
			if string(run) == "ab" {
				valueRun = string(run)
			}
		}
	}
	if valueRun != "ab" {
		t.Fatalf("no black text run with the focused value %q", valueRun)
	}
	if n := countFills(t, dl, frame.RGB(0, 103, 244)); n < 4 {
		t.Fatalf("focus ring strips = %d, want >= 4", n)
	}
	caret := 0
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Color == frame.RGB(0, 0, 0) &&
			c.Rect.X1-c.Rect.X0 == 1 && c.Rect.Y1 > c.Rect.Y0 {
			caret++
		}
	}
	if caret == 0 {
		t.Fatal("no 1px caret bar in display list")
	}
}

func TestPaintUnfocusedNoRing(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	dl := s.Paint(1).Build(1)
	if n := countFills(t, dl, frame.RGB(0, 103, 244)); n != 0 {
		t.Fatalf("focus ring present without focus (%d strips)", n)
	}
}

// ringBounds returns the union rect of the focus-ring strips (color 0,103,244)
// in a built display list, or false when no ring is drawn.
func ringBounds(dl *paint.LayerDL) (frame.Rect, bool) {
	var r frame.Rect
	found := false
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Color == frame.RGB(0, 103, 244) {
			if !found {
				r = c.Rect
				found = true
				continue
			}
			if c.Rect.X0 < r.X0 {
				r.X0 = c.Rect.X0
			}
			if c.Rect.Y0 < r.Y0 {
				r.Y0 = c.Rect.Y0
			}
			if c.Rect.X1 > r.X1 {
				r.X1 = c.Rect.X1
			}
			if c.Rect.Y1 > r.Y1 {
				r.Y1 = c.Rect.Y1
			}
		}
	}
	return r, found
}

// TestPaintKeyboardTabMovesRing is the GOO-04 visual verification: driving a
// Tab keypress through PressKey must move the painted focus ring from the
// first input to the second, proving keyboard → SetFocus → paint end to end.
func TestPaintKeyboardTabMovesRing(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	if s.Focused() != nil {
		t.Fatal("session should start unfocused")
	}

	// Tab from nothing focuses #a: ring appears around #a's box.
	s.PressKey("Tab", false, false, false, false)
	dlA := s.Paint(1).Build(1)
	ringA, okA := ringBounds(dlA)
	if !okA {
		t.Fatal("no focus ring after first Tab")
	}

	// Tab again focuses #b: the ring must move (its vertical band changes).
	s.PressKey("Tab", false, false, false, false)
	dlB := s.Paint(1).Build(1)
	ringB, okB := ringBounds(dlB)
	if !okB {
		t.Fatal("no focus ring after second Tab")
	}
	if ringA == ringB {
		t.Fatalf("focus ring did not move after Tab: both at %v", ringA)
	}

	// Shift+Tab moves back toward #a: the ring must move again.
	s.PressKey("Tab", true, false, false, false)
	dlBack := s.Paint(1).Build(1)
	ringBack, okBack := ringBounds(dlBack)
	if !okBack {
		t.Fatal("no focus ring after Shift+Tab")
	}
	if ringBack == ringB {
		t.Fatalf("Shift+Tab did not move the ring: still at %v", ringB)
	}
}

func TestPaintPlaceholderOnlyWhenEmpty(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">
<input id="full" value="ab" placeholder="hint"><input id="empty" placeholder="hint"></body></html>`)
	grey := frame.RGB(117, 117, 117)
	x, y := clickPoint(t, s, "full")
	s.FocusControl(x, y)
	dl := s.Paint(1).Build(1)
	foundValue := false
	hints := 0
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == frame.RGB(0, 0, 0) {
			for _, g := range c.Text.Glyphs {
				if g.Rune == 'a' || g.Rune == 'b' {
					foundValue = true
				}
			}
		}
	}
	if !foundValue {
		t.Fatal("value glyphs missing")
	}
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == grey {
			for _, g := range c.Text.Glyphs {
				if g.Rune == 'h' {
					hints++
				}
			}
		}
	}
	if hints != 1 {
		t.Fatalf("placeholder 'h' glyphs = %d, want 1 (empty control only)", hints)
	}
}

func TestPaintPasswordDots(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">
<input id="pw" type="password" value="abc"></body></html>`)
	x, y := clickPoint(t, s, "pw")
	s.FocusControl(x, y)
	dl := s.Paint(1).Build(1)
	got := textRunesIn(t, dl, frame.RGB(0, 0, 0))
	if string(got) != "•••" {
		t.Fatalf("password glyphs = %q, want dots", string(got))
	}
}
