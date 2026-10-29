package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestInnerTextPreservesWhitespaceAroundAtomicInlineContent(t *testing.T) {
	doc := domtest.Parse(`<div id="input">abc <input> def</div>
		<div id="image">abc <img> def</div>
		<div id="canvas"><canvas></canvas> abc</div>
		<div id="leading"><img> abc</div>
		<div id="trailing">abc <img></div>
		<div id="inline-block">abc <span style="display:inline-block"></span> def</div>
		<div id="inline-flex">abc <span style="display:inline-flex"></span> def</div>
		<div id="inline-grid">abc <span style="display:inline-grid"></span> def</div>
		<div id="atomic-before-block"><input> <div>abc</div></div>
		<div id="text-before-block">abc <input> <div>def</div></div>`)
	rt := newDOMRuntime(t, doc)
	if got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('input').innerText,
		document.getElementById('image').innerText,
		document.getElementById('canvas').innerText,
		document.getElementById('leading').innerText,
		document.getElementById('trailing').innerText,
		document.getElementById('inline-block').innerText,
		document.getElementById('inline-flex').innerText,
		document.getElementById('inline-grid').innerText,
		document.getElementById('atomic-before-block').innerText,
		document.getElementById('text-before-block').innerText
	])`); err != nil || got != `["abc  def","abc  def"," abc"," abc","abc ","abc  def","abc  def","abc  def","abc","abc \ndef"]` {
		t.Fatalf("innerText whitespace around atomic inline content = %q, err %v", got, err)
	}
}
