package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestInnerTextParagraphBoundariesAndCollapsibleEdges(t *testing.T) {
	doc := domtest.Parse(`<div id="consecutive"><p>abc</p><p>def</p></div>
		<div id="implicit"><p>abc<p>def</div>
		<div id="before">abc<p>def</div>
		<div id="after"><p>abc</p>def</div>
		<div id="trailing"><p>abc</p>   </div>
		<div id="leading">   <p>abc</p></div>
		<div id="empty"><p>abc</p><p></p><p></p><p>def</p></div>
		<div id="nested">abc<div><p>123</p></div>def</div>
		<div id="inline-block">123<p style="display:inline-block">abc</p></div>`)
	rt := newDOMRuntime(t, doc)
	if got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('consecutive').innerText,
		document.getElementById('implicit').innerText,
		document.getElementById('before').innerText,
		document.getElementById('after').innerText,
		document.getElementById('trailing').innerText,
		document.getElementById('leading').innerText,
		document.getElementById('empty').innerText,
		document.getElementById('nested').innerText,
		document.getElementById('inline-block').innerText
	])`); err != nil || got != `["abc\n\ndef","abc\n\ndef","abc\n\ndef","abc\n\ndef","abc","abc","abc\n\ndef","abc\n\n123\n\ndef","123\n\nabc"]` {
		t.Fatalf("innerText paragraph boundaries = %q, err %v", got, err)
	}
}
