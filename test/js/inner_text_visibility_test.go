package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestInnerTextHonorsVisibilityAndVisibleOverrides(t *testing.T) {
	doc := domtest.Parse(`<div id="root-hidden" style="visibility:hidden">hidden</div>
		<div id="child-hidden">123<span style="visibility:hidden">hidden</span>456</div>
		<div id="override-hidden" style="visibility:hidden">hidden<span style="visibility:visible">shown</span></div>
		<table id="group-hidden"><tbody style="visibility:collapse"><tr><td>hidden</td></tr></tbody></table>
		<table id="group-override"><tbody style="visibility:collapse"><tr><td style="visibility:visible">shown</td></tr></tbody></table>
		<table id="row-hidden"><tr style="visibility:collapse"><td>hidden</td></tr></table>
		<table id="cell-hidden"><tr><td style="visibility:collapse">hidden</td></tr></table>`)
	rt := newDOMRuntime(t, doc)
	if got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('root-hidden').innerText,
		document.getElementById('child-hidden').innerText,
		document.getElementById('override-hidden').innerText,
		document.getElementById('group-hidden').innerText,
		document.getElementById('group-override').innerText,
		document.getElementById('row-hidden').innerText,
		document.getElementById('cell-hidden').innerText
	])`); err != nil || got != `["","123456","shown","","shown","",""]` {
		t.Fatalf("innerText visibility handling = %q, err %v", got, err)
	}
}
