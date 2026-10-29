package wpt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestDisableWPTHarnessOutputPrependsRunnerSetting(t *testing.T) {
	source := []byte("add_completion_callback(function () {});\n")
	got := string(disableWPTHarnessOutput(source))
	want := "setup({output:false});\n" + string(source)
	if got != want {
		t.Fatalf("disabled testharness report = %q, want %q", got, want)
	}
}

func TestWPTTestDriverActionSequenceDeletesAcrossInlineBlockBoundary(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="abc<p>def<br>ghi</p>";edit.focus();var text=edit.firstChild;getSelection().collapse(text,text.length);window.keyDefaultLog=[];edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];keyDefaultLog.push("beforeinput:"+range.startContainer.data+":"+range.startOffset+":"+range.endContainer.data+":"+range.endOffset)});edit.addEventListener("input",function(event){keyDefaultLog.push("input:"+event.inputType)})})()`); err != nil {
		t.Fatalf("prepare WPT editing selection: %v", err)
	}
	if s.Focused() != s.Doc.Body.FirstChild {
		t.Fatalf("DOM focus state = %v, want the contenteditable host", s.Focused())
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"}]}])`); err != nil {
		t.Fatalf("run WPT Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+keyDefaultLog.join("|")`)
	if err != nil {
		t.Fatalf("read WPT action result: %v", err)
	}
	want := `abcdef<p>ghi</p>|beforeinput:abc:3:def:0|input:deleteContentForward`
	if got != want {
		t.Fatalf("WPT action-sequence forward-delete = %q, want %q", got, want)
	}
}

func TestWPTTestDriverForwardDeleteJoinsFirstLineAcrossNestedBlocks(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="<div>abc<p>def<br>ghi</p></div>";edit.focus();var text=edit.firstChild.firstChild;getSelection().collapse(text,text.length);window.editEvents=[];edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];editEvents.push("beforeinput:"+range.startContainer.data+":"+range.startOffset+":"+range.endContainer.data+":"+range.endOffset)});edit.addEventListener("input",function(event){editEvents.push("input:"+event.inputType)})})()`); err != nil {
		t.Fatalf("prepare nested-block selection: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"}]}])`); err != nil {
		t.Fatalf("run WPT Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+editEvents.join("|")`)
	if err != nil {
		t.Fatalf("read nested-block deletion result: %v", err)
	}
	want := `<div>abcdef<p>ghi</p></div>|beforeinput:abc:3:def:0|input:deleteContentForward`
	if got != want {
		t.Fatalf("nested-block forward-delete = %q, want %q", got, want)
	}
}

func TestWPTTestDriverForwardDeleteConsumesBreakBeforeNestedBlock(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="<div>abc<br><p>def<br>ghi</p></div>";edit.focus();var text=edit.firstChild.firstChild;getSelection().collapse(text,text.length);window.editEvents=[];edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];editEvents.push("beforeinput:"+range.startContainer.nodeName+":"+range.startOffset+":"+range.endContainer.data+":"+range.endOffset)});edit.addEventListener("input",function(event){editEvents.push("input:"+event.inputType)})})()`); err != nil {
		t.Fatalf("prepare break boundary selection: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"}]}])`); err != nil {
		t.Fatalf("run WPT Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+editEvents.join("|")`)
	if err != nil {
		t.Fatalf("read break-boundary deletion result: %v", err)
	}
	want := `<div>abcdef<p>ghi</p></div>|beforeinput:DIV:1:def:0|input:deleteContentForward`
	if got != want {
		t.Fatalf("break-boundary forward-delete = %q, want %q", got, want)
	}
}

func TestWPTTestDriverForwardDeleteJoinsFollowingInlineTextIntoBlock(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="<p>abc   </p>   def";edit.focus();var text=edit.firstChild.firstChild;getSelection().collapse(text,3);window.editEvents=[];edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];editEvents.push("beforeinput:"+range.startOffset+":"+range.endOffset)});edit.addEventListener("input",function(event){editEvents.push("input:"+event.inputType)})})()`); err != nil {
		t.Fatalf("prepare inline-text boundary selection: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"}]}])`); err != nil {
		t.Fatalf("run WPT Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+editEvents.join("|")`)
	if err != nil {
		t.Fatalf("read inline-text deletion result: %v", err)
	}
	want := `<p>abcdef</p>|beforeinput:3:3|input:deleteContentForward`
	if got != want {
		t.Fatalf("inline-text forward-delete = %q, want %q", got, want)
	}
}

func TestWPTTestDriverForwardDeleteJoinsTextAfterTrailingPlaceholderBreak(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="<div><p><img src=x><br></p>def</div>";edit.focus();var p=edit.firstChild.firstChild;getSelection().collapse(p,1);window.editEvents=[];edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];editEvents.push("beforeinput:"+range.startContainer.nodeName+":"+range.startOffset+":"+range.endContainer.data+":"+range.endOffset)});edit.addEventListener("input",function(event){editEvents.push("input:"+event.inputType)})})()`); err != nil {
		t.Fatalf("prepare placeholder-break selection: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"}]}])`); err != nil {
		t.Fatalf("run WPT Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+editEvents.join("|")`)
	if err != nil {
		t.Fatalf("read placeholder-break deletion result: %v", err)
	}
	want := `<div><p><img src="x">def</p></div>|beforeinput:P:1:def:0|input:deleteContentForward`
	if got != want {
		t.Fatalf("placeholder-break forward-delete = %q, want %q", got, want)
	}
}

func TestWPTTestDriverForwardDeleteFlattensNextSingleItemList(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="<div>abc<ul><li>def</li></ul>ghi</div>";edit.focus();var text=edit.firstChild.firstChild;getSelection().collapse(text,text.length);window.editEvents=[];edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];editEvents.push("beforeinput:"+range.startContainer.data+":"+range.startOffset+":"+range.endContainer.data+":"+range.endOffset)});edit.addEventListener("input",function(event){editEvents.push("input:"+event.inputType)})})()`); err != nil {
		t.Fatalf("prepare list-boundary selection: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"}]}])`); err != nil {
		t.Fatalf("run WPT Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+editEvents.join("|")`)
	if err != nil {
		t.Fatalf("read list-boundary deletion result: %v", err)
	}
	want := `<div>abcdefghi</div>|beforeinput:abc:3:def:0|input:deleteContentForward`
	if got != want {
		t.Fatalf("list-boundary forward-delete = %q, want %q", got, want)
	}
}

func TestWPTTestDriverForwardDeleteJoinsFollowingTextIntoLastListItem(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="<div>abc<ul><li>def</li></ul>ghi</div>";edit.focus();var text=edit.querySelector("li").firstChild;getSelection().collapse(text,text.length);window.editEvents=[];edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];editEvents.push("beforeinput:"+range.startContainer.data+":"+range.startOffset+":"+range.endContainer.data+":"+range.endOffset)});edit.addEventListener("input",function(event){editEvents.push("input:"+event.inputType)})})()`); err != nil {
		t.Fatalf("prepare list-item boundary selection: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"}]}])`); err != nil {
		t.Fatalf("run WPT Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+editEvents.join("|")`)
	if err != nil {
		t.Fatalf("read list-item deletion result: %v", err)
	}
	want := `<div>abc<ul><li>defghi</li></ul></div>|beforeinput:def:3:ghi:0|input:deleteContentForward`
	if got != want {
		t.Fatalf("list-item forward-delete = %q, want %q", got, want)
	}
}

func TestWPTTestDriverForwardDeleteTrimsCollapsedSpaceBeforeJoiningListText(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="abc <ul><li>  def  </li></ul> ghi";edit.focus();var text=edit.querySelector("li").firstChild;getSelection().collapse(text,text.length);window.editEvents=[];edit.addEventListener("input",function(event){editEvents.push(event.inputType)})})()`); err != nil {
		t.Fatalf("prepare list whitespace selection: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"}]}])`); err != nil {
		t.Fatalf("run WPT Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+editEvents.join("|")`)
	if err != nil {
		t.Fatalf("read list whitespace deletion result: %v", err)
	}
	want := `abc <ul><li>  defghi</li></ul>|deleteContentForward`
	if got != want {
		t.Fatalf("list whitespace forward-delete = %q, want %q", got, want)
	}
}

func TestWPTTestDriverForwardDeletePreservesSelectedTableCell(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="<table><tr><td>cell1</td><td>cell2</td></tr></table>";edit.focus();var row=edit.querySelector("tr"),range=document.createRange();range.setStart(row,0);range.setEnd(row,1);var selection=getSelection();selection.removeAllRanges();selection.addRange(range);window.editEvents=[];edit.addEventListener("input",function(event){editEvents.push(event.inputType)})})()`); err != nil {
		t.Fatalf("prepare selected table-cell range: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"}]}])`); err != nil {
		t.Fatalf("run WPT Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+editEvents.join("|")`)
	if err != nil {
		t.Fatalf("read selected table-cell deletion result: %v", err)
	}
	want := `<table><tbody><tr><td><br></td><td>cell2</td></tr></tbody></table>|deleteContentForward`
	if got != want {
		t.Fatalf("selected table-cell forward-delete = %q, want %q", got, want)
	}
}

func TestWPTTestDriverForwardDeletePreservesMultipleSelectedTableCells(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="<table><tr><td>cell1</td><td>cell2</td></tr><tr><td>cell3</td><td>cell4</td></tr></table>";edit.focus();var cells=edit.querySelectorAll("td"),selection=getSelection();selection.removeAllRanges();var first=document.createRange();first.selectNode(cells[0]);selection.addRange(first);var last=document.createRange();last.selectNode(cells[3]);selection.addRange(last);window.editEvents=[];edit.addEventListener("input",function(event){editEvents.push(event.inputType)})})()`); err != nil {
		t.Fatalf("prepare multiple selected table cells: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"}]}])`); err != nil {
		t.Fatalf("run WPT Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+editEvents.join("|")`)
	if err != nil {
		t.Fatalf("read multiple table-cell deletion result: %v", err)
	}
	want := `<table><tbody><tr><td><br></td><td>cell2</td></tr><tr><td>cell3</td><td><br></td></tr></tbody></table>|deleteContentForward`
	if got != want {
		t.Fatalf("multiple selected table-cell forward-delete = %q, want %q", got, want)
	}
}

func TestWPTTestDriverForwardDeletePreservesMixedTableCellRanges(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="<table><tr><td>cell1</td><td>cell2</td></tr><tr><td>cell3</td><td>cell4</td></tr></table>";edit.focus();var cells=edit.querySelectorAll("td"),selection=getSelection();selection.removeAllRanges();var first=document.createRange();first.selectNode(cells[0]);selection.addRange(first);var partial=document.createRange();partial.setStart(cells[1].firstChild,1);partial.setEnd(cells[1].firstChild,4);selection.addRange(partial);var last=document.createRange();last.selectNode(cells[3]);selection.addRange(last);window.editEvents=[];edit.addEventListener("input",function(event){editEvents.push(event.inputType)})})()`); err != nil {
		t.Fatalf("prepare mixed selected table-cell ranges: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"}]}])`); err != nil {
		t.Fatalf("run WPT Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+editEvents.join("|")`)
	if err != nil {
		t.Fatalf("read mixed table-cell deletion result: %v", err)
	}
	want := `<table><tbody><tr><td><br></td><td>c2</td></tr><tr><td>cell3</td><td><br></td></tr></tbody></table>|deleteContentForward`
	if got != want {
		t.Fatalf("mixed selected table-cell forward-delete = %q, want %q", got, want)
	}
}

func TestWPTTestDriverForwardDeleteStopsAtNonEditableBlockAfterPlaceholder(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="<p><br></p><p contenteditable=false>abc</p><p>def</p>";edit.focus();getSelection().collapse(edit.lastChild,0)})()`); err != nil {
		t.Fatalf("prepare noneditable block boundary: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"}]}])`); err != nil {
		t.Fatalf("run WPT Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`)
	if err != nil {
		t.Fatalf("read noneditable boundary result: %v", err)
	}
	want := `<p contenteditable="false">abc</p><p>def</p>`
	if got != want {
		t.Fatalf("forward-delete across noneditable block = %q, want %q", got, want)
	}
}

func TestWPTTestDriverBackspaceRemovesPreLineWhitespaceRun(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML='<p style="white-space:pre-line">abc \ndef</p>';edit.focus();var text=edit.firstChild.firstChild;getSelection().collapse(text,5)})()`); err != nil {
		t.Fatalf("prepare pre-line Backspace selection: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE003"},{type:"keyUp",value:"\uE003"}]}])`); err != nil {
		t.Fatalf("run WPT Backspace action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").textContent`)
	if err != nil {
		t.Fatalf("read pre-line Backspace result: %v", err)
	}
	if want := `abcdef`; got != want {
		t.Fatalf("Backspace across pre-line whitespace = %q, want %q", got, want)
	}
}

func TestWPTTestDriverBackspaceJoinsParagraphAcrossLeadingSpaces(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="<p>abc   </p><p>   def</p>";edit.focus();var text=edit.lastChild.firstChild;getSelection().collapse(text,3)})()`); err != nil {
		t.Fatalf("prepare Backspace selection: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE003"},{type:"keyUp",value:"\uE003"}]}])`); err != nil {
		t.Fatalf("run WPT Backspace action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML`)
	if err != nil {
		t.Fatalf("read Backspace result: %v", err)
	}
	if want := `<p>abcdef</p>`; got != want {
		t.Fatalf("Backspace across leading paragraph spaces = %q, want %q", got, want)
	}
}

func TestWPTTestDriverMetaDeleteRemovesFollowingWord(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rt.EnableTestUserActivationHook()
	s, err := engine.NewSession(`<!doctype html><html><head></head><body><div id="edit" contenteditable></div></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.EvaluateJS(`window.test_driver={};`); err != nil {
		t.Fatalf("initialize test_driver: %v", err)
	}
	if _, err := s.EvaluateJS(wptTestDriverInputShim); err != nil {
		t.Fatalf("install testdriver input shim: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){var edit=document.getElementById("edit");edit.innerHTML="<p>abc def   s</p>";edit.focus();var text=edit.firstChild.firstChild;getSelection().collapse(text,4);window.editEvents=[];edit.addEventListener("beforeinput",function(event){var range=event.getTargetRanges()[0];editEvents.push("beforeinput:"+range.startOffset+":"+range.endOffset+":"+event.inputType)});edit.addEventListener("input",function(event){editEvents.push("input:"+event.inputType)})})()`); err != nil {
		t.Fatalf("prepare Meta+Delete selection: %v", err)
	}
	if _, err := s.EvaluateJS(`test_driver.action_sequence([{type:"key",actions:[{type:"keyDown",value:"\uE03D"},{type:"keyDown",value:"\uE017"},{type:"keyUp",value:"\uE017"},{type:"keyUp",value:"\uE03D"}]}])`); err != nil {
		t.Fatalf("run WPT Meta+Delete action: %v", err)
	}
	got, err := s.EvaluateJS(`document.getElementById("edit").innerHTML+"|"+editEvents.join("|")`)
	if err != nil {
		t.Fatalf("read Meta+Delete result: %v", err)
	}
	want := `<p>abc </p>|beforeinput:4:11:deleteSoftLineForward|input:deleteSoftLineForward`
	if got != want {
		t.Fatalf("Meta+Delete forward-word deletion = %q, want %q", got, want)
	}
}

func TestWPTTestHarnessRunnerExecutesAndReportsTests(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "html", "dom", "elements", "global-attributes")
	resourceDir := filepath.Join(root, "resources")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resourceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	harness := `var __tests = [];
function assert_equals(actual, expected, message) {
  if (actual !== expected) throw new Error(message + ": got " + actual + ", want " + expected);
}

function assert_implements_optional(condition, message) {
  if (!condition) { var error = new Error(message); error.optional = true; throw error; }
}

function test(callback, name) {
  var result = {name: name, status: 0, message: ""};
  try { callback(); } catch (error) { result.status = error.optional ? 4 : 1; result.message = String(error); }
  __tests.push(result);
}
function add_completion_callback(callback) {
  window.addEventListener("load", function () { callback(__tests, {status: 0}); });
}`
	if err := os.WriteFile(filepath.Join(resourceDir, "testharness.js"), []byte(harness), 0o644); err != nil {
		t.Fatal(err)
	}
	page := func(body string) string {
		return `<!doctype html><body id="target"><script src="/resources/testharness.js"></script><script>` + body + `</script></body>`
	}
	if err := os.WriteFile(filepath.Join(testDir, "pass.html"), []byte(page(`test(function () { assert_equals(document.body.id, "target", "body id"); }, "body id is exposed");`)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "fail.html"), []byte(page(`test(function () { assert_equals(document.body.id, "wrong", "body id"); }, "bad body id is reported"); __tests.push({name: "stack-only failure", status: 1, message: null, stack: "captured assertion stack"});`)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "optional.html"), []byte(page(`test(function () { assert_implements_optional(false, "optional API unavailable"); }, "optional feature is classified as skipped");`)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "linked.css"), []byte(".linked { width: 37px; }"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkedPage := `<!doctype html><head><link rel="stylesheet" href="linked.css"><script src="/resources/testharness.js"></script></head><body><div class="linked"></div><script>test(function () { assert_equals(document.querySelector(".linked").offsetWidth, 37, "linked stylesheet width"); }, "linked stylesheets are fetched by the WPT harness");</script></body>`
	if err := os.WriteFile(filepath.Join(testDir, "linked-css.html"), []byte(linkedPage), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := NewRunner(Config{
		WPTDir:         root,
		Directories:    []string{"html/dom/elements/global-attributes"},
		OutputDir:      filepath.Join(root, "results"),
		Timeout:        3,
		ViewportWidth:  800,
		ViewportHeight: 600,
	}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 4 || result.Passed != 2 || result.Failed != 1 || result.Skipped != 1 {
		t.Fatalf("testharness result = total %d, pass %d, fail %d, skip %d; want 4, 2, 1, 1; results: %+v", result.Total, result.Passed, result.Failed, result.Skipped, result.Results)
	}
	var sawFailure, sawOptionalSkip bool
	for _, test := range result.Results {
		if test.Path == "html/dom/elements/global-attributes/fail.html" {
			sawFailure = test.Status == "fail" && strings.Contains(test.Message, "bad body id is reported") && strings.Contains(test.Message, "captured assertion stack")
		}
		if test.Path == "html/dom/elements/global-attributes/optional.html" {
			sawOptionalSkip = test.Status == "skip" && strings.Contains(test.Message, "optional API unavailable")
		}
	}
	if !sawFailure {
		t.Fatalf("failure details not reported: %+v", result.Results)
	}
	if !sawOptionalSkip {
		t.Fatalf("optional feature was not reported as a skip: %+v", result.Results)
	}
}

func TestWPTTestHarnessRunnerParsesXHTMLAndRunsCDATAClassicScripts(t *testing.T) {
	root := t.TempDir()
	testDir := filepath.Join(root, "domparsing")
	resourceDir := filepath.Join(root, "resources")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	harness := `var __tests = [];
function assert_equals(actual, expected, message) {
  if (actual !== expected) throw new Error(message + ": got " + actual + ", want " + expected);
}
function test(callback, name) {
  var result = {name: name, status: 0, message: ""};
  try { callback(); } catch (error) { result.status = 1; result.message = String(error); }
  __tests.push(result);
}
function add_completion_callback(callback) {
  window.addEventListener("load", function () { callback(__tests, {status: 0}); });
}`
	if err := os.WriteFile(filepath.Join(resourceDir, "testharness.js"), []byte(harness), 0o644); err != nil {
		t.Fatal(err)
	}
	page := `<?xml version="1.0"?>
<html xmlns="http://www.w3.org/1999/xhtml"><head><script src="/resources/testharness.js"></script></head><body><div id="target"/><script><![CDATA[
test(function () {
  assert_equals(document.contentType, "application/xhtml+xml", "document content type");
  assert_equals(document.documentElement.namespaceURI, "http://www.w3.org/1999/xhtml", "document element namespace");
  assert_equals(document.getElementById("target").localName, "div", "XHTML element lookup");
}, "XHTML document and CDATA script are active");
]]></script></body></html>`
	if err := os.WriteFile(filepath.Join(testDir, "xhtml-runner.xhtml"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewRunner(Config{
		WPTDir: root, Directories: []string{"domparsing"},
		OutputDir: filepath.Join(root, "results"), Timeout: 3,
		ViewportWidth: 800, ViewportHeight: 600,
	}).Run()
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Passed != 1 {
		t.Fatalf("XHTML WPT result = total %d, passed %d; details: %+v", result.Total, result.Passed, result.Results)
	}
}

func TestAnyJSTestDocumentLoadsHarnessDependenciesAndSource(t *testing.T) {
	source := []byte("// META: title=sample\n// META: script=../support/helper.js\n'use strict';\ntest(() => assert_true(true));\n")
	document := string(anyJSTestDocument("FileAPI/blob/sample.any.js", source))
	for _, fragment := range []string{
		"self.GLOBAL =",
		"isWindow: function() { return true; }",
		"isWorker: function() { return false; }",
		"isShadowRealm: function() { return false; }",
		"/resources/testharness.js",
		"/resources/testharnessreport.js",
		"../support/helper.js",
		"/FileAPI/blob/sample.any.js",
	} {
		if !strings.Contains(document, fragment) {
			t.Errorf("generated .any.js document does not contain %q: %s", fragment, document)
		}
	}
	if strings.Contains(document, "META: title") || strings.Contains(document, "'use strict'") {
		t.Fatalf("generated .any.js document inlined metadata or source unexpectedly: %s", document)
	}
	if strings.Index(document, "self.GLOBAL =") > strings.Index(document, "/resources/testharness.js") {
		t.Fatalf("generated .any.js document installs GLOBAL after the harness: %s", document)
	}
}
