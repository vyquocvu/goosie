package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestInnerTextSettersConvertNewlinesToBreakElements(t *testing.T) {
	doc := domtest.Parse(`<div id="inner">old</div><pre id="pre"></pre>
		<div id="parent"><span id="target">old</span><i>end</i></div>
		<svg id="svg"><text>stay</text></svg>`)
	rt := newDOMRuntime(t, doc)
	if got, err := rt.Evaluate(`(function(){
		var inner=document.getElementById('inner');
		inner.innerText='abc\r\n\ndef';
		var pre=document.getElementById('pre');
		pre.innerText='x\ry';
		var target=document.getElementById('target');
		target.outerText='a\r\nb';
		var undefinedValue=document.createElement('div');
		undefinedValue.innerText=undefined;
		var nullValue=document.createElement('div');
		nullValue.innerText=null;
		document.getElementById('svg').innerText='ignored';
		return JSON.stringify([
			inner.innerHTML,
			pre.innerHTML,
			document.getElementById('parent').innerHTML,
			undefinedValue.innerHTML,
			nullValue.innerHTML,
			document.getElementById('svg').innerHTML
		]);
	})()`); err != nil {
		t.Fatalf("set innerText/outerText: %v", err)
	} else if got != `["abc<br><br>def","x<br>y","a<br>b<i>end</i>","undefined","","<text>stay</text>"]` {
		t.Fatalf("innerText/outerText newline replacement = %s", got)
	}
}
