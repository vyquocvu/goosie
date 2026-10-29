package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestInnerTextSelectOptionRowsAndEmptyGroupBoundary(t *testing.T) {
	doc := domtest.Parse(`<select id="one" size="1"><option>abc</option><option>def</option></select>
		<select id="two" size="2"><option>abc</option><option>def</option></select>
		<select id="grouped"><optgroup label="g"><option>one</option><option>two</option></optgroup></select>
		<select id="empty"></select>
		<div id="empty-group">a<select><optgroup></select>bc</div>
		<div id="empty-option">a<select><option></select>bc</div>
		<div id="outside-empty-group">a<optgroup></optgroup>bc</div>
		<div id="outside-group">a<optgroup>123</optgroup>bc</div>
		<div id="outside-option">a<option></option>bc</div>
		<select id="raw">stray<option>kept</option></select>`)
	rt := newDOMRuntime(t, doc)
	got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('one').innerText,
		document.getElementById('two').innerText,
		document.getElementById('grouped').innerText,
		document.getElementById('empty').innerText,
		document.getElementById('empty-group').innerText,
		document.getElementById('empty-option').innerText,
		document.getElementById('outside-empty-group').innerText,
		document.getElementById('outside-group').innerText,
		document.getElementById('outside-option').innerText,
		document.getElementById('raw').innerText
	])`)
	if err != nil {
		t.Fatalf("read select innerText: %v", err)
	}
	const want = `["abc\ndef","abc\ndef","one\ntwo","","a\nbc","a\nbc","a\nbc","a\n123\nbc","a\nbc","kept"]`
	if got != want {
		t.Fatalf("select innerText = %s, want %s", got, want)
	}
}
