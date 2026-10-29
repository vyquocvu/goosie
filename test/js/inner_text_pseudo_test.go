package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestInnerTextAppliesResolvedFirstLineAndFirstLetterTransforms(t *testing.T) {
	doc := domtest.Parse(`<div id="line">abc<br>def</div><div id="wrapped" style="width:0">abc def</div><div id="letter">abc def</div>`)
	rt := newDOMRuntime(t, doc)
	rt.SetPseudoStyleProvider(func(node *dom.Node, pseudo string) map[string]string {
		if node == nil {
			return nil
		}
		if node.GetAttribute("id") == "line" && pseudo == "first-line" {
			return map[string]string{"text-transform": "uppercase"}
		}
		if node.GetAttribute("id") == "wrapped" && pseudo == "first-line" {
			return map[string]string{"text-transform": "uppercase"}
		}
		if node.GetAttribute("id") == "letter" && pseudo == "first-letter" {
			return map[string]string{"text-transform": "uppercase"}
		}
		return nil
	})
	if got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('line').innerText,
		document.getElementById('wrapped').innerText,
		document.getElementById('letter').innerText
	])`); err != nil || got != `["ABC\ndef","ABC def","Abc def"]` {
		t.Fatalf("pseudo-styled innerText = %q, err %v", got, err)
	}
}

func TestInnerTextRootBreakAndClosedDetails(t *testing.T) {
	doc := domtest.Parse(`<br id="break"><div id="closed"><details><summary>abc</summary>123</details></div>
		<div id="open"><details open><summary>abc</summary>123</details></div>
		<div id="empty-option">a<select><option></select>bc</div>`)
	rt := newDOMRuntime(t, doc)
	if got, err := rt.Evaluate(`JSON.stringify([
		document.getElementById('break').innerText,
		document.getElementById('closed').innerText,
		document.getElementById('open').innerText,
		document.getElementById('empty-option').innerText
	])`); err != nil || got != `["","abc","abc\n123","a\nbc"]` {
		t.Fatalf("root br/details innerText = %q, err %v", got, err)
	}
}

func TestInnerTextTransformUsesInheritedLanguage(t *testing.T) {
	doc := domtest.Parse(`<div><div lang="tr" style="text-transform:uppercase">i ı</div>
		<div style="text-transform:uppercase">Maß</div></div>`)
	rt := newDOMRuntime(t, doc)
	if got, err := rt.Evaluate(`document.body.innerText`); err != nil || got != "İ I\nMASS" {
		t.Fatalf("locale-aware innerText transform = %q, err %v", got, err)
	}
}
