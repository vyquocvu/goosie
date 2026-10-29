package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestInnerTextAppliesInheritedTextTransform(t *testing.T) {
	doc := domtest.Parse(`<div id="parent" style="text-transform: uppercase"><span id="child">abc</span><span id="lower" style="text-transform: lowercase">XYZ</span></div>`)
	rt := newDOMRuntime(t, doc)
	if got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('parent').innerText,
		document.getElementById('parent').outerText,
		document.getElementById('child').innerText,
		document.getElementById('child').outerText,
		document.getElementById('lower').innerText,
		document.getElementById('lower').outerText
	])`); err != nil || got != `["ABCxyz","ABCxyz","ABC","ABC","xyz","xyz"]` {
		t.Fatalf("innerText transform = %q, err %v", got, err)
	}
}

func TestInnerTextOmitsHiddenChildrenAndSeparatesBlockChildren(t *testing.T) {
	doc := domtest.Parse(`<div id="root">abc<div style="display: block">def</div><span style="display: none">hidden</span></div>`)
	rt := newDOMRuntime(t, doc)
	if got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('root').innerText,
		document.getElementById('root').outerText
	])`); err != nil || got != `["abc\ndef","abc\ndef"]` {
		t.Fatalf("rendered text for hidden/block children = %q, err %v", got, err)
	}
}

func TestInnerTextEmptyBlockBoundariesRespectWhiteSpace(t *testing.T) {
	doc := domtest.Parse(`<div id="normal" style="white-space: normal">abc <hr>def</div>
		<div id="normal-after" style="white-space: normal">abc<hr> def</div>
		<div id="normal-between" style="white-space: normal">abc<hr> <hr>def</div>
		<div id="normal-div" style="white-space: normal">abc <div></div> def</div>
		<div id="pre" style="white-space: pre">abc <hr>def</div>
		<div id="pre-after" style="white-space: pre">abc<hr> def</div>
		<div id="pre-between" style="white-space: pre">abc<hr> <hr>def</div>
		<div id="pre-line" style="white-space: pre-line">abc
<hr>def</div>
		<div id="pre-line-after" style="white-space: pre-line">abc<hr>
def</div>
		<div id="pre-line-between" style="white-space: pre-line">abc<hr> <hr>def</div>`)
	rt := newDOMRuntime(t, doc)
	if got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('normal').innerText,
		document.getElementById('normal-after').innerText,
		document.getElementById('normal-between').innerText,
		document.getElementById('normal-div').innerText,
		document.getElementById('pre').innerText,
		document.getElementById('pre-after').innerText,
		document.getElementById('pre-between').innerText,
		document.getElementById('pre-line').innerText,
		document.getElementById('pre-line-after').innerText,
		document.getElementById('pre-line-between').innerText
	])`); err != nil || got != `["abc\ndef","abc\ndef","abc\ndef","abc\ndef","abc \ndef","abc\n def","abc\n \ndef","abc\n\ndef","abc\n\ndef","abc\ndef"]` {
		t.Fatalf("innerText empty-block boundaries = %q, err %v", got, err)
	}
}

func TestInnerTextPreservedNewlineBoundariesAcrossBlocks(t *testing.T) {
	doc := domtest.Parse(`<div id="editable"></div>`)
	rt := newDOMRuntime(t, doc)
	if got, err := rt.Evaluate(`var div = document.getElementById('editable');
		div.setAttribute('contenteditable', 'true');
		var div = document.getElementById('editable');
		var actual = [];
		for (const whiteSpace of ['pre-line', 'pre', 'pre-wrap']) {
			div.style.whiteSpace = whiteSpace;
			div.innerHTML = 'abc\n<hr>def'; actual.push(div.innerText);
			div.innerHTML = 'abc<hr>\n<hr>def'; actual.push(div.innerText);
			div.innerHTML = 'abc<div></div>\n<div></div>def'; actual.push(div.innerText);
		}
		JSON.stringify(actual)`); err != nil || got != `["abc\n\ndef","abc\n\n\ndef","abc\n\n\ndef","abc\n\ndef","abc\n\n\ndef","abc\n\n\ndef","abc\n\ndef","abc\n\n\ndef","abc\n\n\ndef"]` {
		t.Fatalf("contenteditable preserved-newline boundary text = %q, err %v", got, err)
	}
}

func TestInnerTextCollapsesNormalWhitespaceAcrossInlineRuns(t *testing.T) {
	doc := domtest.Parse(`<div id="normal">&#9;  abc &#9; <span>&#9; def&#13;ghi</span>&#10;&#9;jkl &#9; </div>
		<div id="inline">abc<span> </span><span> &#9; </span>def</div>
		<div id="pre-line" style="white-space: pre-line">&#9; abc &#9;
 &#9; def &#9;
 </div>
		<div id="pre" style="white-space: pre">&#9; abc &#9;</div>`)
	rt := newDOMRuntime(t, doc)
	if got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('normal').innerText,
		document.getElementById('inline').innerText,
		document.getElementById('pre-line').innerText,
		document.getElementById('pre').innerText
	])`); err != nil || got != `["abc def ghi jkl","abc def","abc\ndef\n","\t abc \t"]` {
		t.Fatalf("innerText whitespace normalization = %q, err %v", got, err)
	}
}
