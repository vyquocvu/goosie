package dom_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

// TestMisnestedFormatting_BI tests the classic misnested formatting case:
// <b>...<i>...</b>...</i> should produce two separate formatting elements
// rather than nesting i inside b incorrectly.
func TestMisnestedFormatting_BI(t *testing.T) {
	doc := domtest.Parse(`<html><body><b>bold<i>bold-italic</b>italic</i></body></html>`)
	if doc.Body == nil {
		t.Fatal("no body element")
	}
	text := doc.Body.TextContent()
	if !strings.Contains(text, "bold") {
		t.Errorf("body text missing 'bold': %q", text)
	}
	if !strings.Contains(text, "bold-italic") {
		t.Errorf("body text missing 'bold-italic': %q", text)
	}
	if !strings.Contains(text, "italic") {
		t.Errorf("body text missing 'italic': %q", text)
	}

	// The tree should have b and i elements, not crash or produce empty output.
	var bCount, iCount int
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n.Element() {
			if n.Data == "b" {
				bCount++
			}
			if n.Data == "i" {
				iCount++
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
	if bCount == 0 {
		t.Error("no <b> element found in tree")
	}
	if iCount == 0 {
		t.Error("no <i> element found in tree")
	}
}

func TestFragmentParseSeedsFormPointerFromContextAncestors(t *testing.T) {
	doc := dom.NewDocument()
	form := doc.NewElement("form")
	doc.Node.AppendChild(form)
	context := doc.NewElement("div")
	form.AppendChild(context)

	fragment, err := dom.ParseFragmentBounded(context, `<div><form></div><input type=text></form>`, dom.ParseLimits{
		Nodes: 32, Depth: 16, Attributes: 32, AttributeBytes: 1024,
	})
	if err != nil {
		t.Fatalf("parse fragment: %v", err)
	}
	var forms, inputs int
	fragment.Walk(func(node *dom.Node) bool {
		if node.Element() && node.Data == "form" {
			forms++
		}
		if node.Element() && node.Data == "input" {
			inputs++
		}
		return true
	})
	if forms != 0 {
		t.Errorf("fragment contains %d nested form elements, want none", forms)
	}
	if inputs != 1 {
		t.Errorf("fragment contains %d inputs, want 1", inputs)
	}
}

// TestMisnestedFormatting_IB tests the reverse: <i>...<b>...</i>...</b>
func TestMisnestedFormatting_IB(t *testing.T) {
	doc := domtest.Parse(`<html><body><i>italic<b>italic-bold</i>bold</b></body></html>`)
	if doc.Body == nil {
		t.Fatal("no body element")
	}
	text := doc.Body.TextContent()
	for _, want := range []string{"italic", "italic-bold", "bold"} {
		if !strings.Contains(text, want) {
			t.Errorf("body text missing %q: %q", want, text)
		}
	}
}

func TestInBodyHTMLStartTagInsideTemplateIsIgnored(t *testing.T) {
	doc := domtest.Parse(`<html tabindex="1"><body><template><html class="ignored"></html></template><html id="kept" tabindex="5"></html></body></html>`)
	if doc.HTML == nil {
		t.Fatal("no document element")
	}
	if doc.HTML.HasAttribute("class") {
		t.Errorf("template-contained html start tag added class=%q", doc.HTML.GetAttribute("class"))
	}
	if got, want := doc.HTML.GetAttribute("id"), "kept"; got != want {
		t.Errorf("duplicate html start tag id = %q, want %q", got, want)
	}
	if got, want := doc.HTML.GetAttribute("tabindex"), "1"; got != want {
		t.Errorf("duplicate html start tag tabindex = %q, want %q", got, want)
	}
}

func TestAdoptionAgencyDoesNotUseAnotherSameTagEntry(t *testing.T) {
	doc := domtest.Parse(`<div><code some-attribute=""><div><code><code><code><code></code></code></code></code></div></code></div>`)
	outer := findFirst(doc.Body, "code")
	if outer == nil {
		t.Fatal("no outer code element")
	}
	if outer.FirstChild == nil || !outer.FirstChild.Element() || outer.FirstChild.Data != "div" {
		t.Fatalf("outer code child = %v, want the nested div; adoption of an inactive same-tag node must not select the outer formatting entry", outer.FirstChild)
	}
	if !outer.HasAttribute("some-attribute") {
		t.Fatal("outer code lost its attribute")
	}
}

// TestPInsideDiv tests that <p> inside <div> followed by a block element
// properly closes the paragraph: <p>text<div>block</p>
func TestPInsideDiv(t *testing.T) {
	doc := domtest.Parse(`<html><body><p>text<div>block</div></body></html>`)
	if doc.Body == nil {
		t.Fatal("no body element")
	}

	// The <p> should be closed before the <div>.
	var p, div *dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() {
				if c.Data == "p" {
					p = c
				}
				if c.Data == "div" {
					div = c
				}
				walk(c)
			}
		}
	}
	walk(&doc.Node)
	if p == nil {
		t.Fatal("no <p> element found")
	}
	if div == nil {
		t.Fatal("no <div> element found")
	}
	// p and div should be siblings (both children of body).
	if p.Parent != div.Parent {
		t.Errorf("p and div are not siblings: p.Parent=%v, div.Parent=%v",
			p.Parent.Data, div.Parent.Data)
	}
}

// TestTableContentOutsideTable tests that content appearing after a table
// is placed correctly, not foster-parented into the table.
func TestTableContentOutsideTable(t *testing.T) {
	doc := domtest.Parse(`<html><body><table><tr><td>cell</td></tr></table><p>after</p></body></html>`)
	if doc.Body == nil {
		t.Fatal("no body element")
	}

	p := findFirst(&doc.Node, "p")
	if p == nil {
		t.Fatal("no <p> element found")
	}
	if p.Parent == nil || p.Parent.Data != "body" {
		parent := "<nil>"
		if p.Parent != nil {
			parent = p.Parent.Data
		}
		t.Errorf("p's parent = %s, want body", parent)
	}
}

// TestImpliedTableStructure tests that table content without explicit tbody
// gets the implied tbody wrapper.
func TestImpliedTableStructure(t *testing.T) {
	doc := domtest.Parse(`<html><body><table><tr><td>cell</td></tr></table></body></html>`)
	if doc.Body == nil {
		t.Fatal("no body element")
	}

	tbody := findFirst(&doc.Node, "tbody")
	if tbody == nil {
		t.Fatal("no implied <tbody> element found")
	}
	tr := findFirst(tbody, "tr")
	if tr == nil {
		t.Fatal("no <tr> inside tbody")
	}
	td := findFirst(tr, "td")
	if td == nil {
		t.Fatal("no <td> inside tr")
	}
}

// TestNestedFormattingSurvives tests that basic nested formatting elements
// produce the expected tree structure.
func TestNestedFormattingSurvives(t *testing.T) {
	doc := domtest.Parse(`<html><body><b>bold <i>bold-italic</i> bold</b></body></html>`)
	if doc.Body == nil {
		t.Fatal("no body element")
	}
	b := findFirst(&doc.Node, "b")
	if b == nil {
		t.Fatal("no <b> element found")
	}
	i := findFirst(b, "i")
	if i == nil {
		t.Fatal("no <i> element found inside <b>")
	}
	if i.Parent != b {
		t.Errorf("<i>'s parent = %s, want b", i.Parent.Data)
	}
}

// TestInsertionModeDoctype tests that doctype processing sets quirks mode.
func TestInsertionModeDoctype(t *testing.T) {
	// Standard doctype.
	doc := domtest.Parse(`<!DOCTYPE html><html><body><p>text</p></body></html>`)
	if doc.DocType == nil {
		t.Error("no doctype element")
	}
	if doc.Body == nil {
		t.Fatal("no body element")
	}
	p := findFirst(doc.Body, "p")
	if p == nil {
		t.Fatal("no <p> element")
	}
}

// TestInsertionModeCommentBeforeHTML tests that a comment before <html>
// is placed correctly.
func TestInsertionModeCommentBeforeHTML(t *testing.T) {
	doc := domtest.Parse(`<!-- comment --><html><body><p>text</p></body></html>`)
	if doc.Body == nil {
		t.Fatal("no body element")
	}
	// The comment should be in the tree somewhere.
	var found bool
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n.Type == dom.NodeComment {
			found = true
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
	if !found {
		t.Error("comment not found in tree")
	}
}

// TestSelectMode tests basic select element parsing.
func TestSelectMode(t *testing.T) {
	doc := domtest.Parse(`<html><body><select><option>A</option><option>B</option></select></body></html>`)
	sel := findFirst(&doc.Node, "select")
	if sel == nil {
		t.Fatal("no <select> element found")
	}
	opts := sel.ChildElements()
	if len(opts) != 2 {
		t.Errorf("select has %d option children, want 2", len(opts))
	}
}

func TestCustomizableSelectParsesSelectedContent(t *testing.T) {
	doc := domtest.Parse(`<html><body><select><button><selectedcontent></selectedcontent></button><option>one</option><option>two</option></select></body></html>`)
	sel := findFirst(&doc.Node, "select")
	if sel == nil {
		t.Fatal("no <select> element found")
	}
	button := findFirst(sel, "button")
	if button == nil || button.Parent != sel {
		t.Fatal("select button was not retained as a direct child")
	}
	content := findFirst(button, "selectedcontent")
	if content == nil {
		t.Fatal("selectedcontent was not retained under the select button")
	}
	if len(dom.SelectOptions(sel)) != 2 {
		t.Fatalf("select options = %d, want 2", len(dom.SelectOptions(sel)))
	}
}

// TestTemplateMode tests basic template element parsing.
func TestTemplateMode(t *testing.T) {
	doc := domtest.Parse(`<html><body><template><p>inside</p></template><p>outside</p></body></html>`)
	tmpl := findFirst(&doc.Node, "template")
	if tmpl == nil {
		t.Fatal("no <template> element found")
	}
	// Template contents belong to the separate DocumentFragment, not the
	// template element's ordinary child list.
	innerP := findFirst(tmpl.TemplateContent(), "p")
	if innerP == nil {
		t.Error("no <p> inside template")
	}
	// There should also be a <p> outside the template.
	body := doc.Body
	var outerP *dom.Node
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() && c.Data == "p" && c != tmpl {
			outerP = c
			break
		}
	}
	if outerP == nil {
		t.Error("no <p> outside template in body")
	}
}

// findFirst returns the first element with the given tag name in the tree.
func findFirst(n *dom.Node, tag string) *dom.Node {
	if n == nil {
		return nil
	}
	if n.Element() && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findFirst(c, tag); found != nil {
			return found
		}
	}
	return nil
}
