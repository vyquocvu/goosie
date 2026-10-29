package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestInnerTextOmitsNonRenderedSVGContent(t *testing.T) {
	doc := domtest.Parse(`<div id="stop"><svg><stop>abc</stop></svg></div>
		<div id="text"><svg><text>hello</text></svg></div>`)
	rt := newDOMRuntime(t, doc)
	got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('stop').innerText,
		document.getElementById('text').innerText
	])`)
	if err != nil {
		t.Fatalf("read SVG innerText: %v", err)
	}
	if got != `["","hello"]` {
		t.Fatalf("SVG innerText = %s, want empty text for non-rendered SVG stop and rendered SVG text content", got)
	}
}
