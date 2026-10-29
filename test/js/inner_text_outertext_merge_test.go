package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestOuterTextMergesOnlyAdjacentTextNodes(t *testing.T) {
	doc := domtest.Parse(`<div id="root"></div>`)
	rt := newDOMRuntime(t, doc)
	got, err := rt.Evaluate(`(function(){
		var root=document.getElementById('root');
		root.append('A','B',document.createElement('span'),'D','E');
		root.childNodes[2].outerText='Replaced';
		return JSON.stringify([
			root.innerHTML,
			root.childNodes.length,
			root.childNodes[0].data,
			root.childNodes[1].data,
			root.childNodes[2].data
		]);
	})()`)
	if err != nil {
		t.Fatalf("replace outerText: %v", err)
	}
	if got != `["ABReplacedDE",3,"A","BReplacedD","E"]` {
		t.Fatalf("outerText adjacent text merge = %s", got)
	}
}

func TestOuterTextEmptyStringCreatesTextNodeAndThrowsWhenDetached(t *testing.T) {
	doc := domtest.Parse(`<div id="root"></div>`)
	rt := newDOMRuntime(t, doc)
	got, err := rt.Evaluate(`(function(){
		var root=document.getElementById('root');
		var lone=document.createElement('span'); root.appendChild(lone); lone.outerText='';
		var emptyText=root.childNodes[0];
		var surrounded=document.createElement('div');
		surrounded.append('1','2',document.createElement('span'),'3','4');
		surrounded.childNodes[2].outerText='';
		var detached=document.createElement('span'), threw=false;
		try { detached.outerText=''; } catch (e) { threw=e instanceof DOMException && e.name==='NoModificationAllowedError'; }
		return JSON.stringify([
			root.childNodes.length, emptyText.nodeType, emptyText.data,
			surrounded.innerHTML, surrounded.childNodes.length,
			surrounded.childNodes[0].data, surrounded.childNodes[1].data, surrounded.childNodes[2].data,
			threw
		]);
	})()`)
	if err != nil {
		t.Fatalf("set empty/detached outerText: %v", err)
	}
	if got != `[1,3,"","1234",3,"1","23","4",true]` {
		t.Fatalf("outerText empty/detached behavior = %s", got)
	}
}
