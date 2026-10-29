package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestInnerTextTableRowsCellsAndWhitespace(t *testing.T) {
	doc := domtest.Parse(`<div id="table"><table><tr id="first"><td id="cell">abc<td>def<tr><td>ghi<td>jkl</table></div>
		<div id="pre"><table style="white-space:pre">  <td>abc</td>  </table></div>
		<table id="groups"><tfoot><tr><td>footer</tfoot><thead><tr><td style="visibility:collapse">thead</thead><tbody><tr><td>tbody</tbody></table>`)
	rt := newDOMRuntime(t, doc)
	got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('table').innerText,
		document.getElementById('first').innerText,
		document.getElementById('cell').innerText,
		document.getElementById('pre').innerText,
		document.getElementById('groups').innerText
	])`)
	if err != nil {
		t.Fatalf("read table innerText: %v", err)
	}
	if got != `["abc\tdef\nghi\tjkl","abc\tdef","abc","abc","footer\n\ntbody"]` {
		t.Fatalf("table innerText = %s", got)
	}
}
