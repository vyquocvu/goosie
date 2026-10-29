package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestInnerTextOmitsReplacedElementFallbackContent(t *testing.T) {
	doc := domtest.Parse(`<textarea id="textarea">control value</textarea>
		<iframe id="iframe">frame fallback</iframe>
		<audio id="audio" style="display: block">audio fallback</audio>
		<video id="video">video fallback</video>
		<canvas id="canvas">canvas fallback</canvas>
		<img id="image" alt="image alternative">
		<object id="object">object fallback</object>`)
	rt := newDOMRuntime(t, doc)
	if got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('textarea').innerText,
		document.getElementById('iframe').innerText,
		document.getElementById('audio').innerText,
		document.getElementById('video').innerText,
		document.getElementById('canvas').innerText,
		document.getElementById('image').innerText,
		document.getElementById('object').innerText,
		document.getElementById('textarea').outerText,
		document.getElementById('iframe').outerText,
		document.getElementById('audio').outerText,
		document.getElementById('video').outerText,
		document.getElementById('canvas').outerText,
		document.getElementById('image').outerText,
		document.getElementById('object').outerText
	])`); err != nil || got != `["","","","","","","","","","","","","",""]` {
		t.Fatalf("innerText/outerText replaced-element content = %q, err %v", got, err)
	}
}
