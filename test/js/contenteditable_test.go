package js_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestContentEditableReflectionAndInheritance(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="root"><span id="inherit"></span><span id="stop" contenteditable="false"><b id="stopped"></b></span></div><div id="invalid" contenteditable="falſe"><i id="invalidChild"></i></div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`
		var root = document.getElementById('root');
		var values = [];
		values.push(root.contentEditable, root.isContentEditable);
		root.setAttribute('contenteditable', 'TrUe');
		values.push(root.contentEditable, root.isContentEditable,
			document.getElementById('inherit').isContentEditable);
		values.push(document.getElementById('stopped').isContentEditable);
		values.push(document.getElementById('invalid').contentEditable,
			document.getElementById('invalidChild').isContentEditable);
		document.getElementById('inherit').contentEditable = 'PLAINTEXT-ONLY';
		values.push(document.getElementById('inherit').contentEditable,
			document.getElementById('inherit').getAttribute('contenteditable'));
		document.getElementById('inherit').contentEditable = 'inherit';
		values.push(document.getElementById('inherit').contentEditable,
			document.getElementById('inherit').hasAttribute('contenteditable'));
		var invalidSetter = false;
		try { root.contentEditable = 'not-an-enum'; } catch (e) { invalidSetter = e.name === 'SyntaxError'; }
		values.push(invalidSetter, root.contentEditable);
		JSON.stringify(values);
	`)
	if err != nil {
		t.Fatalf("evaluate contenteditable properties: %v", err)
	}
	want := `["inherit",false,"true",true,true,false,"inherit",false,"plaintext-only","plaintext-only","inherit",false,true,"true"]`
	if got != want {
		t.Fatalf("contenteditable values = %s, want %s", got, want)
	}
}

func TestDocumentDesignModeAndSelectionBindings(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="blocked" contenteditable="false"></div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`
		var states = [document.designMode, document.body.isContentEditable];
		document.designMode = 'ON';
		states.push(document.designMode, document.body.isContentEditable,
			document.getElementById('blocked').isContentEditable,
			document.getSelection() === window.getSelection());
		document.designMode = 'invalid';
		states.push(document.designMode, document.body.isContentEditable);
		JSON.stringify(states);
	`)
	if err != nil {
		t.Fatalf("evaluate designMode: %v", err)
	}
	want := `["off",false,"on",true,false,true,"off",false]`
	if got != want {
		t.Fatalf("designMode values = %s, want %s", got, want)
	}
}

func TestContentEditableWordDeletionTargetRangeCrossesInlineText(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable>hel<i>lo wo</i><b>rld</b></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var host = document.getElementById('host');
		var lastText = host.lastChild.firstChild;
		var selection = document.getSelection();
		selection.collapse(lastText, lastText.length);
		window.wordDeleteTarget = null;
		window.wordDeleteEvent = null;
		host.addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.wordDeleteTarget = [range.startContainer.textContent, range.startOffset,
				range.endContainer.textContent, range.endOffset];
			window.wordDeleteEvent = event;
		});
	`); err != nil {
		t.Fatalf("set up word-deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteWordBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify([window.wordDeleteTarget, window.wordDeleteEvent.getTargetRanges().length])`)
	if err != nil {
		t.Fatalf("read target range: %v", err)
	}
	const want = `[["lo wo",3,"rld",3],0]`
	if got != want {
		t.Fatalf("word-deletion target range = %s, want %s", got, want)
	}
}

func TestContentEditableForwardWordDeletionRetainsOneCollapsedSeparator(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable><p>abc def   s</p></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`(function(){var host=document.getElementById('host'),text=host.firstChild.firstChild;getSelection().collapse(text,4);host.addEventListener('beforeinput',function(event){var range=event.getTargetRanges()[0];window.wordRange=[range.startOffset,range.endOffset]})})()`); err != nil {
		t.Fatalf("set up forward word-deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteWordForward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.wordRange)`)
	if err != nil {
		t.Fatalf("read forward word-deletion range: %v", err)
	}
	if got != `[4,9]` {
		t.Fatalf("forward word-deletion range = %s, want [4,9]", got)
	}
}

func TestContentEditableForwardDeleteTargetRangeAcrossBlockWhitespace(t *testing.T) {
	cases := []struct {
		name   string
		html   string
		text   string
		offset int
		want   string
	}{
		{
			name:   "normal trailing and leading spaces collapse across blocks",
			html:   `<div contenteditable><p>abc   </p><p>   def</p></div>`,
			text:   `host.firstChild.firstChild`,
			offset: 4,
			want:   `["abc   ",3,"   def",3]`,
		},
		{
			name:   "preserved current spaces collapse only next normal block spaces",
			html:   `<div contenteditable><pre>abc   </pre><p>   def</p></div>`,
			text:   `host.firstChild.firstChild`,
			offset: 6,
			want:   `["abc   ",6,"   def",3]`,
		},
		{
			name:   "normal spaces cross into a nested block",
			html:   `<div contenteditable>abc   <p>   def</p></div>`,
			text:   `host.firstChild`,
			offset: 4,
			want:   `["abc   ",3,"   def",3]`,
		},
		{
			name:   "plain text joins a nested block at its start",
			html:   `<div contenteditable>abc<p>def</p></div>`,
			text:   `host.firstChild`,
			offset: 3,
			want:   `["abc",3,"def",0]`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := domtest.Parse(`<html><body>` + tc.html + `</body></html>`)
			host := doc.Body.FirstChild
			rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
			if err != nil {
				t.Fatalf("create runtime: %v", err)
			}
			defer rt.Close()
			setup := fmt.Sprintf(`(function(){var host=document.querySelector('[contenteditable]'),text=%s,selection=getSelection();selection.collapse(text,%d);host.addEventListener('beforeinput',function(event){var range=event.getTargetRanges()[0];window.forwardDeleteTarget=[range.startContainer.data,range.startOffset,range.endContainer.data,range.endOffset]})})()`, tc.text, tc.offset)
			if _, err := rt.Evaluate(setup); err != nil {
				t.Fatalf("set up forward-delete selection: %v", err)
			}
			if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentForward") {
				t.Fatal("dispatch trusted beforeinput")
			}
			got, err := rt.Evaluate(`JSON.stringify(window.forwardDeleteTarget)`)
			if err != nil {
				t.Fatalf("read target range: %v", err)
			}
			if got != tc.want {
				t.Fatalf("forward-delete target range = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestContentEditablePreLineForwardDeleteTargetRangeIncludesFollowingBoundary(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><p style="white-space:pre-line">abc &#10;def</p></div></body></html>`)
	host := doc.Body.FirstChild
	text := host.FirstChild.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`(function(){var host=document.querySelector('[contenteditable]'),text=host.firstChild.firstChild;getSelection().collapse(text,3);host.addEventListener('beforeinput',function(event){var range=event.getTargetRanges()[0];window.rangeResult=[range.startOffset,range.endOffset]})})()`); err != nil {
		t.Fatalf("set up pre-line forward-delete selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentForward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.rangeResult)`)
	if err != nil {
		t.Fatalf("read target range: %v", err)
	}
	if got != `[3,6]` {
		t.Fatalf("pre-line forward-delete target range = %s, want [3,6] for text %q", got, text.DataContent)
	}
}

func TestContentEditableForwardDeleteTargetRangeStartsBeforeBreakAcrossBlocks(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable>abc<br><p>def</p></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`(function(){var host=document.querySelector('[contenteditable]'),text=host.firstChild;getSelection().collapse(text,3);host.addEventListener('beforeinput',function(event){var range=event.getTargetRanges()[0];window.rangeResult=[range.startContainer.nodeName,range.startOffset,range.endContainer.nodeName,range.endOffset]})})()`); err != nil {
		t.Fatalf("set up cross-block forward-delete selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentForward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.rangeResult)`)
	if err != nil {
		t.Fatalf("read target range: %v", err)
	}
	if got != `["DIV",1,"#text",0]` {
		t.Fatalf("cross-block forward-delete target range = %s, want [\"DIV\",1,\"#text\",0]", got)
	}
}

func TestContentEditableForwardDeleteTargetRangeCrossesBreakFromElementCaret(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><img><br><p>def</p></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`(function(){var host=document.querySelector('[contenteditable]');getSelection().collapse(host,1);host.addEventListener('beforeinput',function(event){var range=event.getTargetRanges()[0];window.rangeResult=[range.startContainer.nodeName,range.startOffset,range.endContainer.nodeName,range.endOffset]})})()`); err != nil {
		t.Fatalf("set up element-boundary forward-delete selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentForward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.rangeResult)`)
	if err != nil {
		t.Fatalf("read target range: %v", err)
	}
	if got != `["DIV",1,"#text",0]` {
		t.Fatalf("element-boundary forward-delete target range = %s, want [\"DIV\",1,\"#text\",0]", got)
	}
}

func TestContentEditableForwardDeleteStopsBeforeNonEditableInline(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><p>abc<span contenteditable="false">def</span>ghi</p></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`(function(){var host=document.querySelector('[contenteditable]'),text=host.firstChild.firstChild;getSelection().collapse(text,3);host.addEventListener('beforeinput',function(event){var range=event.getTargetRanges()[0];window.rangeResult=[range.startContainer.nodeName,range.startOffset,range.endContainer.nodeName,range.endOffset]})})()`); err != nil {
		t.Fatalf("set up noneditable-inline forward-delete selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentForward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.rangeResult)`)
	if err != nil {
		t.Fatalf("read target range: %v", err)
	}
	if got != `["#text",3,"#text",3]` {
		t.Fatalf("noneditable-inline forward-delete target range = %s, want collapsed text boundary", got)
	}
}

func TestContentEditableForwardDeleteTargetRangeSkipsNonEditableBlock(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><p><br></p><p contenteditable="false">abc</p><p>def</p></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`(function(){var host=document.querySelector('[contenteditable]'),empty=host.firstChild;getSelection().collapse(empty,0);host.addEventListener('beforeinput',function(event){var range=event.getTargetRanges()[0];window.rangeResult=[range.startContainer.nodeName,range.startOffset,range.endContainer.nodeName,range.endOffset]})})()`); err != nil {
		t.Fatalf("set up forward-delete selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentForward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.rangeResult)`)
	if err != nil {
		t.Fatalf("read target range: %v", err)
	}
	if got != `["P",0,"DIV",1]` {
		t.Fatalf("forward-delete target range = %s, want [\"P\",0,\"DIV\",1]", got)
	}
}

func TestContentEditableForwardDeleteTargetRangeIncludesEmptyPlaceholderBeforeNonEditableBlock(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><p><br></p><p contenteditable="false">abc</p><p>def</p></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`(function(){var host=document.querySelector('[contenteditable]'),last=host.lastChild;getSelection().collapse(last,0);host.addEventListener('beforeinput',function(event){var range=event.getTargetRanges()[0];window.rangeResult=[range.startContainer.nodeName,range.startOffset,range.endContainer.nodeName,range.endOffset]})})()`); err != nil {
		t.Fatalf("set up forward-delete selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentForward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.rangeResult)`)
	if err != nil {
		t.Fatalf("read target range: %v", err)
	}
	if got != `["P",0,"DIV",1]` {
		t.Fatalf("forward-delete target range = %s, want [\"P\",0,\"DIV\",1]", got)
	}
}

func TestContentEditableDeleteTargetRangesPreserveMultipleTableCellSelections(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><table><tr><td>cell1</td><td>cell2</td></tr><tr><td>cell3</td><td>cell4</td></tr></table></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`(function(){var host=document.querySelector('[contenteditable]'),cells=host.querySelectorAll('td'),selection=getSelection();selection.removeAllRanges();var first=document.createRange();first.selectNode(cells[0]);selection.addRange(first);var last=document.createRange();last.selectNode(cells[3]);selection.addRange(last);window.tableCellTargetRanges=[];host.addEventListener('beforeinput',function(event){tableCellTargetRanges=event.getTargetRanges().map(function(range){return [range.startContainer.nodeName,range.startOffset,range.endContainer.nodeName,range.endOffset]})})})()`); err != nil {
		t.Fatalf("set up multiple table-cell selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentForward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.tableCellTargetRanges)`)
	if err != nil {
		t.Fatalf("read table-cell target ranges: %v", err)
	}
	if got != `[["TR",0,"TR",1],["TR",1,"TR",2]]` {
		t.Fatalf("multiple table-cell target ranges = %s, want [[\"TR\",0,\"TR\",1],[\"TR\",1,\"TR\",2]]", got)
	}
}

func TestContentEditableMultiCellDeleteTargetRangesIncludePartialTextRange(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><table><tr><td>cell1</td><td>cell2</td></tr><tr><td>cell3</td><td>cell4</td></tr></table></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`(function(){var host=document.querySelector('[contenteditable]'),cells=host.querySelectorAll('td'),selection=getSelection();selection.removeAllRanges();var first=document.createRange();first.selectNode(cells[0]);selection.addRange(first);var partial=document.createRange();partial.setStart(cells[1].firstChild,1);partial.setEnd(cells[1].firstChild,4);selection.addRange(partial);var last=document.createRange();last.selectNode(cells[3]);selection.addRange(last);host.addEventListener('beforeinput',function(event){window.tableCellTargetRanges=event.getTargetRanges().map(function(range){return [range.startContainer.nodeName,range.startOffset,range.endContainer.nodeName,range.endOffset]})})})()`); err != nil {
		t.Fatalf("set up mixed table-cell selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentForward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.tableCellTargetRanges)`)
	if err != nil {
		t.Fatalf("read table-cell target ranges: %v", err)
	}
	if got != `[["TR",0,"TR",1],["#text",1,"#text",4],["TR",1,"TR",2]]` {
		t.Fatalf("mixed table-cell target ranges = %s, want [[\"TR\",0,\"TR\",1],[\"#text\",1,\"#text\",4],[\"TR\",1,\"TR\",2]]", got)
	}
}

func TestContentEditableDeleteTargetRangeExpandsAcrossSingleItemDefinitionLists(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable><dl><dt>list-item1</dt></dl><dl><dt>list-item2</dt></dl></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`(function(){var host=document.getElementById('host'),items=host.querySelectorAll('dt'),first=items[0].firstChild,last=items[1].firstChild;getSelection().setBaseAndExtent(first,first.data.length,last,last.data.length);host.addEventListener('beforeinput',function(event){var range=event.getTargetRanges()[0];window.definitionListTarget=[range.startContainer.data,range.startOffset,range.endContainer.data,range.endOffset]})})()`); err != nil {
		t.Fatalf("set up cross-definition-list deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.definitionListTarget)`)
	if err != nil {
		t.Fatalf("read definition-list target range: %v", err)
	}
	if want := `["list-item1",0,"list-item2",10]`; got != want {
		t.Fatalf("cross-definition-list target range = %s, want %s", got, want)
	}
}

func TestContentEditableDeleteTargetRangeEndsAtDefinitionItemBoundaryForMultiItemList(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable><dl><dt>list-item1</dt><dt>list-item2</dt></dl><dl><dt>list-item3</dt></dl></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`(function(){var host=document.getElementById('host'),items=host.querySelectorAll('dt'),start=items[1].firstChild,end=items[2].firstChild;getSelection().setBaseAndExtent(start,start.data.length,end,end.data.length);host.addEventListener('beforeinput',function(event){var range=event.getTargetRanges()[0];window.definitionListTarget=[range.startContainer.data,range.startOffset,range.endContainer.nodeName,range.endOffset]})})()`); err != nil {
		t.Fatalf("set up multi-item definition-list selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.definitionListTarget)`)
	if err != nil {
		t.Fatalf("read definition-list target range: %v", err)
	}
	if want := `["list-item2",10,"DT",0]`; got != want {
		t.Fatalf("multi-item definition-list target range = %s, want %s", got, want)
	}
}

func TestContentEditableForwardDeleteTargetRangeEndsBeforeLeadingNonEditableInline(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><p><br></p><p><span contenteditable="false">abc</span>def</p></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`(function(){var host=document.querySelector('[contenteditable]'),first=host.firstChild;getSelection().collapse(first,0);host.addEventListener('beforeinput',function(event){var range=event.getTargetRanges()[0];window.rangeResult=[range.startContainer.nodeName,range.startOffset,range.endContainer.nodeName,range.endOffset]})})()`); err != nil {
		t.Fatalf("set up forward-delete selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentForward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.rangeResult)`)
	if err != nil {
		t.Fatalf("read target range: %v", err)
	}
	if got != `["P",0,"P",0]` {
		t.Fatalf("forward-delete target range = %s, want [\"P\",0,\"P\",0]", got)
	}
}

func TestContentEditableBackspaceTargetRangeDeletesLastAdjacentLineBreak(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable><p>abc<br><br>def</p></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var host = document.getElementById('host');
		var text = host.firstChild.lastChild;
		document.getSelection().collapse(text, 0);
		window.breakRange = null;
		host.addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.breakRange = [range.startContainer.nodeName, range.startOffset,
				range.endContainer.nodeName, range.endOffset];
		});
	`); err != nil {
		t.Fatalf("set up selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.breakRange)`)
	if err != nil {
		t.Fatalf("read target range: %v", err)
	}
	if want := `["P",2,"P",3]`; got != want {
		t.Fatalf("Backspace target range = %s, want %s", got, want)
	}
}

func TestContentEditableBackspaceTargetRangeRespectsPreLineNewlines(t *testing.T) {
	doc := domtest.Parse("<html><body><div contenteditable><p style=\"white-space: pre-line\">abc \n \n def</p></div></body></html>")
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var text = document.querySelector('p').firstChild;
		document.getSelection().collapse(text, 8);
		window.preLineTargetRange = null;
		document.querySelector('div').addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.preLineTargetRange = [range.startOffset, range.endOffset];
		});
	`); err != nil {
		t.Fatalf("set up pre-line deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.preLineTargetRange)`)
	if err != nil {
		t.Fatalf("read pre-line target range: %v", err)
	}
	if want := `[5,8]`; got != want {
		t.Fatalf("pre-line target range = %s, want %s", got, want)
	}
}

func TestContentEditableBackspaceTargetRangeDeletesPreviousAtomicChild(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><p>abc<img><img>def</p></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var paragraph = document.querySelector('p');
		document.getSelection().collapse(paragraph, 2);
		window.atomicTargetRange = null;
		document.querySelector('div').addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.atomicTargetRange = [range.startOffset, range.endOffset];
		});
	`); err != nil {
		t.Fatalf("set up atomic-child deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.atomicTargetRange)`)
	if err != nil {
		t.Fatalf("read atomic-child target range: %v", err)
	}
	if want := `[1,2]`; got != want {
		t.Fatalf("atomic-child target range = %s, want %s", got, want)
	}
}

func TestContentEditableBackspaceTargetRangeSpansTrailingListBreaks(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><ul><li>list-item1<br><br></li><li>list-item2</li></ul></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var text = document.querySelector('li + li').firstChild;
		document.getSelection().collapse(text, 0);
		window.listBreakTargetRange = null;
		document.querySelector('div').addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.listBreakTargetRange = [range.startContainer.nodeName, range.startOffset,
				range.endContainer.textContent, range.endOffset];
		});
	`); err != nil {
		t.Fatalf("set up list-break deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.listBreakTargetRange)`)
	if err != nil {
		t.Fatalf("read list-break target range: %v", err)
	}
	if want := `["LI",1,"list-item2",0]`; got != want {
		t.Fatalf("list-break target range = %s, want %s", got, want)
	}
}

func TestContentEditableBackspaceTargetRangeIncludesSpaceBeforeHorizontalRule(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><div>abc <hr>def</div></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var block = document.querySelector('div div');
		var text = block.lastChild;
		document.getSelection().collapse(text, 0);
		window.hrTargetRange = null;
		document.querySelector('div[contenteditable]').addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.hrTargetRange = [range.startContainer.textContent, range.startOffset,
				range.endContainer.nodeName, range.endOffset];
		});
	`); err != nil {
		t.Fatalf("set up horizontal-rule deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.hrTargetRange)`)
	if err != nil {
		t.Fatalf("read horizontal-rule target range: %v", err)
	}
	if want := `["abc ",3,"DIV",2]`; got != want {
		t.Fatalf("horizontal-rule target range = %s, want %s", got, want)
	}
}

func TestContentEditableBackspaceTargetRangeDeletesLastBreakBeforeParagraph(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><p>abc<br><br></p><p>def</p></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var text = document.querySelector('p + p').firstChild;
		document.getSelection().collapse(text, 0);
		window.paragraphBreakTargetRange = null;
		document.querySelector('div[contenteditable]').addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.paragraphBreakTargetRange = [range.startContainer.nodeName, range.startOffset,
				range.endContainer.textContent, range.endOffset];
		});
	`); err != nil {
		t.Fatalf("set up paragraph-break deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.paragraphBreakTargetRange)`)
	if err != nil {
		t.Fatalf("read paragraph-break target range: %v", err)
	}
	if want := `["P",2,"def",0]`; got != want {
		t.Fatalf("paragraph-break target range = %s, want %s", got, want)
	}
}

func TestContentEditableBackspaceTargetRangeIncludesSpaceAfterHorizontalRule(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><div>abc<hr> def</div></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var block = document.querySelector('div div');
		var text = block.lastChild;
		document.getSelection().collapse(text, 1);
		window.hrFollowingSpaceTargetRange = null;
		document.querySelector('div[contenteditable]').addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.hrFollowingSpaceTargetRange = [range.startContainer.nodeName, range.startOffset,
				range.endContainer.textContent, range.endOffset];
		});
	`); err != nil {
		t.Fatalf("set up post-rule whitespace deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.hrFollowingSpaceTargetRange)`)
	if err != nil {
		t.Fatalf("read post-rule whitespace target range: %v", err)
	}
	if want := `["DIV",1," def",1]`; got != want {
		t.Fatalf("post-rule whitespace target range = %s, want %s", got, want)
	}
}

func TestContentEditableBackspaceTargetRangeIncludesBreakBeforeHorizontalRule(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><div>abc<br><hr>def</div></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var block = document.querySelector('div div');
		document.getSelection().collapse(block.lastChild, 0);
		window.hrBreakTargetRange = null;
		document.querySelector('div[contenteditable]').addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.hrBreakTargetRange = [range.startContainer.nodeName, range.startOffset,
				range.endContainer.nodeName, range.endOffset];
		});
	`); err != nil {
		t.Fatalf("set up break-rule deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.hrBreakTargetRange)`)
	if err != nil {
		t.Fatalf("read break-rule target range: %v", err)
	}
	if want := `["DIV",1,"DIV",3]`; got != want {
		t.Fatalf("break-rule target range = %s, want %s", got, want)
	}
}

func TestContentEditableBackspaceTargetRangeIncludesWhitespaceAfterBreak(t *testing.T) {
	for _, caretOffset := range []int{0, 1} {
		t.Run(fmt.Sprintf("caret-%d", caretOffset), func(t *testing.T) {
			doc := domtest.Parse(`<html><body><div contenteditable><p>abc<br> def</p></div></body></html>`)
			host := doc.Body.FirstChild
			rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
			if err != nil {
				t.Fatalf("create runtime: %v", err)
			}
			defer rt.Close()
			if _, err := rt.Evaluate(fmt.Sprintf(`
				var text = document.querySelector('p').lastChild;
				document.getSelection().collapse(text, %d);
				window.breakSpaceTargetRange = null;
				document.querySelector('div[contenteditable]').addEventListener('beforeinput', function(event) {
					var range = event.getTargetRanges()[0];
					window.breakSpaceTargetRange = [range.startContainer.nodeName, range.startOffset,
						range.endContainer.textContent, range.endOffset];
				});
			`, caretOffset)); err != nil {
				t.Fatalf("set up post-break whitespace deletion selection: %v", err)
			}
			if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
				t.Fatal("dispatch trusted beforeinput")
			}
			got, err := rt.Evaluate(`JSON.stringify(window.breakSpaceTargetRange)`)
			if err != nil {
				t.Fatalf("read post-break whitespace target range: %v", err)
			}
			if want := `["P",1," def",1]`; got != want {
				t.Fatalf("post-break whitespace target range = %s, want %s", got, want)
			}
		})
	}
}

func TestContentEditableBackspaceTargetRangeRespectsInlineSiblingBeforeBlock(t *testing.T) {
	for _, markup := range []string{
		`<div contenteditable>abc<p>def</p></div>`,
		`<div contenteditable><b>abc</b><p><b>def</b></p></div>`,
	} {
		t.Run(markup, func(t *testing.T) {
			doc := domtest.Parse(`<html><body>` + markup + `</body></html>`)
			host := doc.Body.FirstChild
			rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
			if err != nil {
				t.Fatalf("create runtime: %v", err)
			}
			defer rt.Close()
			if _, err := rt.Evaluate(`
				var text = document.querySelector('p').textContent === 'def'
					? document.querySelector('p').firstChild.nodeType === 3
						? document.querySelector('p').firstChild
						: document.querySelector('p').firstChild.firstChild
					: null;
				document.getSelection().collapse(text, 0);
				window.inlineBlockTargetRange = null;
				document.querySelector('[contenteditable]').addEventListener('beforeinput', function(event) {
					var range = event.getTargetRanges()[0];
					window.inlineBlockTargetRange = [range.startContainer.textContent, range.startOffset,
						range.endContainer.textContent, range.endOffset];
				});
			`); err != nil {
				t.Fatalf("set up inline-block deletion selection: %v", err)
			}
			if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
				t.Fatal("dispatch trusted beforeinput")
			}
			got, err := rt.Evaluate(`JSON.stringify(window.inlineBlockTargetRange)`)
			if err != nil {
				t.Fatalf("read inline-block target range: %v", err)
			}
			if want := `["abc",3,"def",0]`; got != want {
				t.Fatalf("inline-block target range = %s, want %s", got, want)
			}
		})
	}
}

func TestContentEditableBackspaceTargetRangeRespectsBlockBeforeInlineSibling(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><p>abc</p>def</div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var text = document.querySelector('p').nextSibling;
		document.getSelection().collapse(text, 0);
		window.blockInlineTargetRange = null;
		document.querySelector('[contenteditable]').addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.blockInlineTargetRange = [range.startContainer.textContent, range.startOffset,
				range.endContainer.textContent, range.endOffset];
		});
	`); err != nil {
		t.Fatalf("set up block-inline deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.blockInlineTargetRange)`)
	if err != nil {
		t.Fatalf("read block-inline target range: %v", err)
	}
	if want := `["abc",3,"def",0]`; got != want {
		t.Fatalf("block-inline target range = %s, want %s", got, want)
	}
}

func TestContentEditableBackspaceTargetRangeSpansInlineIntoList(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable>abc<ul><li>def</li></ul>ghi</div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var text = document.querySelector('li').firstChild;
		document.getSelection().collapse(text, 0);
		window.inlineListTargetRange = null;
		document.querySelector('[contenteditable]').addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.inlineListTargetRange = [range.startContainer.textContent, range.startOffset,
				range.endContainer.textContent, range.endOffset];
		});
	`); err != nil {
		t.Fatalf("set up inline-list deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.inlineListTargetRange)`)
	if err != nil {
		t.Fatalf("read inline-list target range: %v", err)
	}
	if want := `["abc",3,"def",0]`; got != want {
		t.Fatalf("inline-list target range = %s, want %s", got, want)
	}
}

func TestContentEditableBackspaceTargetRangeJoinsEmptyParagraph(t *testing.T) {
	for _, markup := range []string{
		`<div contenteditable><p>abc</p><p><br></p></div>`,
		`<div contenteditable><p>abc</p><p><span></span><br></p></div>`,
		`<div contenteditable><p>abc<span contenteditable="false">def</span></p><p><br></p></div>`,
	} {
		t.Run(markup, func(t *testing.T) {
			doc := domtest.Parse(`<html><body>` + markup + `</body></html>`)
			host := doc.Body.FirstChild
			rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
			if err != nil {
				t.Fatalf("create runtime: %v", err)
			}
			defer rt.Close()
			if _, err := rt.Evaluate(`
				var empty = document.querySelector('p + p');
				var point = empty.firstChild && empty.firstChild.localName === 'span' ? empty.firstChild : empty;
				document.getSelection().collapse(point, 0);
				window.emptyParagraphTargetRange = null;
				document.querySelector('[contenteditable]').addEventListener('beforeinput', function(event) {
					var range = event.getTargetRanges()[0];
					window.emptyParagraphTargetRange = [range.startContainer.nodeName, range.startOffset,
						range.endContainer.nodeName, range.endOffset];
				});
			`); err != nil {
				t.Fatalf("set up empty-paragraph deletion selection: %v", err)
			}
			if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
				t.Fatal("dispatch trusted beforeinput")
			}
			got, err := rt.Evaluate(`JSON.stringify(window.emptyParagraphTargetRange)`)
			if err != nil {
				t.Fatalf("read empty-paragraph target range: %v", err)
			}
			want := `["#text",3,"P",1]`
			if strings.Contains(markup, `contenteditable="false"`) {
				want = `["P",2,"P",1]`
			} else if strings.Contains(markup, `<span></span>`) {
				want = `["#text",3,"P",2]`
			}
			if got != want {
				t.Fatalf("empty-paragraph target range = %s, want %s", got, want)
			}
		})
	}
}

func TestContentEditableBackspaceTargetRangeStopsAtNonEditableInline(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><p>abc<span contenteditable="false">def</span>ghi</p></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var text = document.querySelector('p').lastChild;
		document.getSelection().collapse(text, 0);
		window.nonEditableInlineTargetRange = null;
		document.querySelector('[contenteditable]').addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.nonEditableInlineTargetRange = [range.startContainer.textContent, range.startOffset,
				range.endContainer.textContent, range.endOffset];
		});
	`); err != nil {
		t.Fatalf("set up non-editable inline deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.nonEditableInlineTargetRange)`)
	if err != nil {
		t.Fatalf("read non-editable inline target range: %v", err)
	}
	if want := `["ghi",0,"ghi",0]`; got != want {
		t.Fatalf("non-editable inline target range = %s, want %s", got, want)
	}
}

func TestContentEditableBackspaceTargetRangeStopsAfterNonEditableBlock(t *testing.T) {
	doc := domtest.Parse(`<html><body><div contenteditable><p>abc</p><p contenteditable="false">def</p><p><br></p></div></body></html>`)
	host := doc.Body.FirstChild
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var block = document.querySelectorAll('p')[2];
		document.getSelection().collapse(block, 0);
		window.nonEditableBlockTargetRange = null;
		document.querySelector('[contenteditable]').addEventListener('beforeinput', function(event) {
			var range = event.getTargetRanges()[0];
			window.nonEditableBlockTargetRange = [range.startContainer.nodeName, range.startOffset,
				range.endContainer.nodeName, range.endOffset];
		});
	`); err != nil {
		t.Fatalf("set up non-editable block deletion selection: %v", err)
	}
	if !rt.DispatchTrustedInputEvent(host, nil, "deleteContentBackward") {
		t.Fatal("dispatch trusted beforeinput")
	}
	got, err := rt.Evaluate(`JSON.stringify(window.nonEditableBlockTargetRange)`)
	if err != nil {
		t.Fatalf("read non-editable block target range: %v", err)
	}
	if want := `["DIV",2,"P",1]`; got != want {
		t.Fatalf("non-editable block target range = %s, want %s", got, want)
	}
}

func TestContentEditableExecCommandFormattingAndInsertText(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable>abcdef</div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`
		var host = document.getElementById('host');
		var text = host.firstChild;
		var selection = document.getSelection();
		selection.collapse(text, 1);
		selection.extend(text, 4);
		var commands = [document.execCommand('bold'), document.execCommand('italic'), document.execCommand('underline')];
		var formatted = host.innerHTML;
		var selected = selection.toString();
		commands.push(document.execCommand('insertText', false, 'XYZ'));
		commands.push(document.execCommand('delete'));
		JSON.stringify([commands, formatted, selected, host.innerHTML, selection.isCollapsed]);
	`)
	if err != nil {
		t.Fatalf("evaluate editing commands: %v", err)
	}
	want := `[[true,true,true,true,true],"a<b><i><u>bcd</u></i></b>ef","bcd","a<b><i><u>XY</u></i></b>ef",true]`
	if got != want {
		t.Fatalf("editing command result = %s, want %s", got, want)
	}
}

func TestContentEditableExecCommandListsColorsLinksAndClipboard(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable>abc</div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`
		var host = document.getElementById('host');
		var selection = document.getSelection();
		var text = host.firstChild;
		selection.collapse(text, 3);
		var inputs = [];
		host.addEventListener('input', function(e) {
			inputs.push([e.inputType, e.data, e.dataTransfer && e.dataTransfer.getData('text/plain')]);
		});
		var ordered = document.execCommand('insertOrderedList');
		var orderedHTML = host.innerHTML;
		var unordered = document.execCommand('insertUnorderedList');
		var unorderedHTML = host.innerHTML;
		host.innerHTML = 'abc';
		text = host.firstChild;
		selection.removeAllRanges();
		selection.collapse(host, 0);
		selection.extend(host, 1);
		var color = document.execCommand('backColor', false, '#0000FF88');
		var colorHTML = host.innerHTML;
		var colorEvent = inputs[2];
		host.innerHTML = 'abc';
		text = host.firstChild;
		selection.removeAllRanges();
		selection.collapse(text, 1);
		selection.extend(text, 2);
		var linked = document.execCommand('createLink', false, 'https://example.com/');
		var linkedHTML = host.innerHTML;
		var unlinked = document.execCommand('unlink');
		var unlinkedHTML = host.innerHTML;
		host.innerHTML = 'abcdef';
		text = host.firstChild;
		selection.removeAllRanges();
		selection.collapse(text, 1);
		selection.extend(text, 4);
		var copied = document.execCommand('copy');
		var cut = document.execCommand('cut');
		var cutHTML = host.innerHTML;
		var pasted = document.execCommand('paste');
		JSON.stringify([ordered, orderedHTML, unordered, unorderedHTML, color, colorEvent,
			linked, linkedHTML, unlinked, unlinkedHTML, copied, cut, cutHTML, pasted,
			host.textContent, inputs.slice(-2)]);
	`)
	if err != nil {
		t.Fatalf("evaluate legacy editing commands: %v", err)
	}
	want := `[true,"<ol><li>abc</li></ol>",true,"<ul><li>abc</li></ul>",true,["formatBackColor","rgba(0, 0, 255, 0.533)",null],true,"a<a href=\"https://example.com/\">b</a>c",true,"abc",true,true,"aef",true,"abcdef",[["deleteByCut",null,null],["insertFromPaste",null,"bcd"]]]`
	if got != want {
		t.Fatalf("legacy editing command result = %s, want %s", got, want)
	}
}

func TestContentEditableCollapsedFormattingToggleAndInsertText(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable>abc</div><div id="formatted" contenteditable><b>abc</b></div><div id="boundary" contenteditable></div><div id="nested" contenteditable><b id="bold" class="bold"><i data-kind="italic">abc</i><u>end</u></b></div><div id="nested-reverse" contenteditable><i><b>abc</b></i></div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`
		var host = document.getElementById('host');
		var text = host.firstChild;
		var selection = document.getSelection();
		selection.collapse(text, 1);
		var boldOn = document.execCommand('bold');
		var firstInsert = document.execCommand('insertText', false, 'X');
		var boldOff = document.execCommand('bold');
		var secondInsert = document.execCommand('insertText', false, 'Y');
		var formattedHost = document.getElementById('formatted');
		var formattedText = formattedHost.firstElementChild.firstChild;
		selection.collapse(formattedText, 1);
		var existingBoldOff = document.execCommand('bold');
		var unboldInsert = document.execCommand('insertText', false, 'Z');
		var boundary = document.getElementById('boundary');
		selection.collapse(boundary, 0);
		var boundaryBold = document.execCommand('bold');
		var boundaryInsert = document.execCommand('insertText', false, 'Q');
		var nested = document.getElementById('nested');
		var nestedText = nested.querySelector('i').firstChild;
		selection.collapse(nestedText, 1);
		var nestedBoldOff = document.execCommand('bold');
		var nestedInsert = document.execCommand('insertText', false, 'W');
		var nestedReverse = document.getElementById('nested-reverse');
		var nestedReverseText = nestedReverse.querySelector('b').firstChild;
		selection.collapse(nestedReverseText, 1);
		var nestedReverseBoldOff = document.execCommand('bold');
		var nestedReverseInsert = document.execCommand('insertText', false, 'V');
		JSON.stringify([boldOn, firstInsert, boldOff, secondInsert, host.innerHTML,
			existingBoldOff, unboldInsert, formattedHost.innerHTML, boundaryBold, boundaryInsert, boundary.innerHTML,
			nestedBoldOff, nestedInsert, nested.innerHTML,
			nestedReverseBoldOff, nestedReverseInsert, nestedReverse.innerHTML,
			selection.isCollapsed, selection.anchorNode.parentNode.localName, selection.anchorOffset]);
	`)
	if err != nil {
		t.Fatalf("evaluate collapsed formatting toggle: %v", err)
	}
	want := `[true,true,true,true,"a<b>X</b>Ybc",true,true,"<b>a</b>Z<b>bc</b>",true,true,"<b>Q</b>",true,true,"<b id=\"bold\" class=\"bold\"><i data-kind=\"italic\">a</i></b><i data-kind=\"italic\">W</i><b class=\"bold\"><i data-kind=\"italic\">bc</i><u>end</u></b>",true,true,"<i><b>a</b>V<b>bc</b></i>",true,"i",1]`
	if got != want {
		t.Fatalf("collapsed formatting toggle = %s, want %s", got, want)
	}
}

func TestContentEditableDeleteAcrossTextNodes(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable><b>abc</b><i>def</i></div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`
		var host = document.getElementById('host');
		var first = host.firstChild.firstChild;
		var last = host.lastChild.firstChild;
		var selection = document.getSelection();
		selection.collapse(first, 1);
		selection.extend(last, 2);
		var deleted = document.execCommand('delete');
		JSON.stringify([deleted, host.innerHTML, host.textContent,
			selection.isCollapsed, selection.anchorNode === first, selection.anchorOffset]);
	`)
	if err != nil {
		t.Fatalf("evaluate cross-node deletion: %v", err)
	}
	want := `[true,"<b>a</b><i>f</i>","af",true,true,1]`
	if got != want {
		t.Fatalf("cross-node deletion = %s, want %s", got, want)
	}
}

func TestContentEditableDeleteCollapsedCaretAndCommandQueries(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable>foo</div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`
		var host = document.getElementById('host');
		var text = host.firstChild;
		var selection = document.getSelection();
		selection.collapse(text, 3);
		var results = [
			document.queryCommandSupported('delete'),
			document.queryCommandEnabled('delete'),
			document.queryCommandIndeterm('delete'),
			document.queryCommandState('delete'),
			document.queryCommandValue('delete'),
			document.execCommand('delete'),
			host.textContent, selection.isCollapsed, selection.anchorNode === text, selection.anchorOffset
		];
		JSON.stringify(results);
	`)
	if err != nil {
		t.Fatalf("evaluate collapsed-caret deletion: %v", err)
	}
	want := `[true,true,false,false,"",true,"fo",true,true,2]`
	if got != want {
		t.Fatalf("collapsed-caret deletion and command queries = %s, want %s", got, want)
	}
}

func TestContentEditableFormattingCommandStateAndIndeterminateQueries(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable><b>bold</b><i>italic</i><span>plain</span></div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`(function(){
		var bold = document.querySelector('b').firstChild;
		var italic = document.querySelector('i').firstChild;
		var plain = document.querySelector('span').firstChild;
		var selection = document.getSelection();
		selection.collapse(bold, 2);
		var collapsed = [document.queryCommandState('bold'), document.queryCommandState('italic'),
			document.queryCommandIndeterm('bold')];
		var boldRange = document.createRange();
		boldRange.selectNodeContents(document.querySelector('b'));
		selection.removeAllRanges(); selection.addRange(boldRange);
		var uniformlyBold = [document.queryCommandState('bold'), document.queryCommandIndeterm('bold')];
		var mixed = document.createRange();
		mixed.setStart(bold, 0); mixed.setEnd(italic, italic.length);
		selection.removeAllRanges(); selection.addRange(mixed);
		var mixedState = [document.queryCommandState('bold'), document.queryCommandIndeterm('bold'),
			document.queryCommandState('italic'), document.queryCommandIndeterm('italic')];
		selection.collapse(plain, 2);
		document.execCommand('bold');
		var typing = [document.queryCommandState('bold'), document.queryCommandIndeterm('bold')];
		return JSON.stringify([collapsed, uniformlyBold, mixedState, typing]);
	})()`)
	if err != nil {
		t.Fatalf("evaluate formatting command queries: %v", err)
	}
	want := `[[true,false,false],[true,false],[false,true,false,true],[true,false]]`
	if got != want {
		t.Fatalf("formatting command queries = %s, want %s", got, want)
	}
}

func TestContentEditableBackspaceRemovesAtomicCustomElementWithReaction(t *testing.T) {
	doc := domtest.Parse(`<html><body></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`(function(){
		var disconnects=0;
		customElements.define('x-empty-atom', class extends HTMLElement {
			disconnectedCallback(){ disconnects++; }
		});
		var host=document.createElement('div');
		host.contentEditable='true';
		var atom=document.createElement('x-empty-atom');
		host.appendChild(atom);
		document.body.appendChild(host);
		host.focus();
		document.getSelection().collapse(host,1);
		var deleted=document.execCommand('delete');
		return [deleted,disconnects,atom.isConnected,host.childNodes.length].join(',');
	})()`)
	if err != nil {
		t.Fatalf("evaluate atomic contenteditable deletion: %v", err)
	}
	if got != "true,1,false,0" {
		t.Fatalf("atomic contenteditable deletion = %s, want %s", got, "true,1,false,0")
	}
}

func TestContentEditableDeleteCollapsedAtInlineBoundary(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="test-container"><div id="host" contenteditable></div><p>test</p></div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`
		var host = document.getElementById('host');
		host.innerHTML = '<span>foo</span>{}<span>bar</span>';
		var marker = host.childNodes[1];
		host.removeChild(marker);
		var firstText = host.firstChild.firstChild;
		var selection = document.getSelection();
		selection.collapse(host, 1);
		selection.extend(host, 1);
		var deleted = document.execCommand('delete');
		JSON.stringify([deleted, host.innerHTML, selection.anchorNode === firstText, selection.anchorOffset]);
	`)
	if err != nil {
		t.Fatalf("evaluate inline-boundary deletion: %v", err)
	}
	want := `[true,"<span>fo</span><span>bar</span>",true,2]`
	if got != want {
		t.Fatalf("inline-boundary deletion = %s, want %s", got, want)
	}
}

func TestContentEditableDeleteSelectedElementNode(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable>before<span id="selected">gone<b>inside</b></span>after</div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`
		var host = document.getElementById('host');
		var selected = document.getElementById('selected');
		var range = document.createRange();
		range.selectNode(selected);
		document.getSelection().addRange(range);
		var inputCount = 0;
		host.addEventListener('input', function() { inputCount++; });
		var deleted = document.execCommand('delete');
		var selection = document.getSelection();
		JSON.stringify([deleted, host.innerHTML, selected.parentNode === null,
			selection.isCollapsed, selection.anchorNode === host, selection.anchorOffset, inputCount]);
	`)
	if err != nil {
		t.Fatalf("evaluate element deletion: %v", err)
	}
	want := `[true,"beforeafter",true,true,true,1,1]`
	if got != want {
		t.Fatalf("delete selected element = %s, want %s", got, want)
	}
}

func TestContentEditableInsertTextAcrossTextNodes(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable><b>abc</b><i>def</i></div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`
		var host = document.getElementById('host');
		var first = host.firstChild.firstChild;
		var last = host.lastChild.firstChild;
		var selection = document.getSelection();
		selection.collapse(first, 1);
		selection.extend(last, 2);
		var inserted = document.execCommand('insertText', false, 'X');
		JSON.stringify([inserted, host.innerHTML, host.textContent,
			selection.isCollapsed, selection.anchorNode === first, selection.anchorOffset]);
	`)
	if err != nil {
		t.Fatalf("evaluate cross-node insertText: %v", err)
	}
	want := `[true,"<b>aX</b><i>f</i>","aXf",true,true,2]`
	if got != want {
		t.Fatalf("cross-node insertText = %s, want %s", got, want)
	}
}

func TestContentEditableFormatAcrossTextNodes(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable><b>abc</b><i>def</i></div></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	got, err := rt.Evaluate(`
		var host = document.getElementById('host');
		var first = host.firstChild.firstChild;
		var last = host.lastChild.firstChild;
		var selection = document.getSelection();
		selection.collapse(first, 2);
		selection.extend(last, 2);
		var formatted = document.execCommand('bold');
		JSON.stringify([formatted, host.innerHTML, selection.toString(),
			selection.anchorOffset, selection.focusOffset]);
	`)
	if err != nil {
		t.Fatalf("evaluate cross-node format: %v", err)
	}
	want := `[true,"<b>ab<b>c</b></b><i><b>de</b>f</i>","cde",0,2]`
	if got != want {
		t.Fatalf("cross-node format = %s, want %s", got, want)
	}
}
