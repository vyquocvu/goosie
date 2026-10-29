package dom_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

// find returns the first element whose class attribute contains wantClass.
func find(root *dom.Node, wantClass string) *dom.Node {
	var el *dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if el != nil {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() {
				for _, cl := range c.ClassList() {
					if cl == wantClass {
						el = c
						return
					}
				}
				walk(c)
			}
		}
	}
	walk(root)
	return el
}

func TestConnectedCheckedRadiosAreExclusivePerGroup(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><form id="two">
<input id="other-form" type="radio" name="choice" checked>
</form><form id="one">
<input id="first" type="radio" name="choice" checked>
<input id="last" type="radio" name="choice" checked>
</form>
<input id="unnamed-a" type="radio" checked><input id="unnamed-b" type="radio" checked>`)
	byID := make(map[string]*dom.Node)
	doc.Node.Walk(func(node *dom.Node) bool {
		if node.Element() && node.HasAttribute("id") {
			byID[node.GetAttribute("id")] = node
		}
		return true
	})
	for _, id := range []string{"first", "last", "other-form", "unnamed-a", "unnamed-b"} {
		if byID[id] == nil {
			t.Fatalf("radio %q was not parsed", id)
		}
	}
	if byID["first"].InputChecked() {
		t.Error("earlier checked radio in the same group remained checked")
	}
	if !byID["last"].InputChecked() || !byID["other-form"].InputChecked() {
		t.Error("last radio and radio in the independent form should be checked")
	}
	if !byID["unnamed-a"].InputChecked() || !byID["unnamed-b"].InputChecked() {
		t.Error("radios without a nonempty name must not form a group")
	}
	if !byID["first"].HasAttribute("checked") {
		t.Error("radio arbitration must preserve the checked content attribute default")
	}
	if !byID["first"].InputCheckednessDirty {
		t.Error("radio arbitration must dirty the checkedness of the automatically unchecked peer")
	}
}

func TestFormPointerAssociatesControlsFromInsideTableForm(t *testing.T) {
	doc := domtest.Parse(`<table><form id="table-form"><tr><td><input id="table-input" value="original"></td></tr></form></table>`)
	form := doc.ElementByID("table-form")
	control := doc.ElementByID("table-input")
	if form == nil || control == nil {
		t.Fatal("table form or its input was not parsed")
	}
	if control.FormOwner != form {
		t.Fatalf("parser-assigned form owner = %p, want table form %p", control.FormOwner, form)
	}
	for parent := control.Parent; parent != nil; parent = parent.Parent {
		if parent == form {
			t.Fatal("table input should be associated by the form element pointer, not nested in the form")
		}
	}
}

// TestNestedListItemStaysInSublist pins HTML5 list-item scope: an <li> may only
// close an <li> in the same list, never one in an ancestor list. Bootstrap-style
// navbars rely on this — the malformed <li><li> markup below must keep the
// dropdown items inside the (display:none) .dropdown-menu <ul>, not leak them
// into the outer .navbar-nav list where they would render.
func TestNestedListItemStaysInSublist(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body>
<ul class="navbar-nav">
<li class="nav-item dropdown">
<a href="#">Design</a>
<ul class="dropdown-menu">
<li class="nav-item"><a href="#">Freebies</a>
<li><li class="nav-item"><a href="#">Products</a>
</ul>
</ul>
</body></html>`)

	dropdown := find(&doc.Node, "dropdown-menu")
	if dropdown == nil {
		t.Fatal("no .dropdown-menu element found")
	}
	txt := dropdown.TextContent()
	for _, want := range []string{"Freebies", "Products"} {
		if !strings.Contains(txt, want) {
			t.Errorf(".dropdown-menu does not contain %q (leaked to ancestor list); subtree text = %q", want, txt)
		}
	}

	// The outer navbar-nav <li> must hold only the "Design" toggle directly;
	// the dropdown items belong one level deeper.
	navbar := find(&doc.Node, "navbar-nav")
	if navbar == nil {
		t.Fatal("no .navbar-nav element found")
	}
	outerText := ""
	for _, li := range navbar.ChildElements() {
		outerText += li.TextContent()
	}
	if strings.Count(outerText, "Freebies") != 1 {
		t.Errorf("expected exactly one Freebies under navbar-nav (inside the sublist), got %q", outerText)
	}
}

func TestMisnestedFormInsideTemplateTree(t *testing.T) {
	doc := domtest.Parse(`<div>A<template><br>BC<form>D<div>E</form>F</div>G</template>H</div>`)
	wrapper := doc.ElementByID("missing")
	_ = wrapper
	var template *dom.Node
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Data == "template" {
				template = child
				return
			}
			walk(child)
		}
	}
	walk(&doc.Node)
	if template == nil {
		t.Fatal("template not parsed")
	}
	t.Logf("template text=%q", template.TemplateContent().TextContent())
	var dump func(*dom.Node, int)
	dump = func(n *dom.Node, depth int) {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			t.Logf("%s%q %q", strings.Repeat(" ", depth), child.Data, child.DataContent)
			if child.Data == "template" {
				dump(child.TemplateContent(), depth+1)
			} else {
				dump(child, depth+1)
			}
		}
	}
	dump(&doc.Node, 0)
}

func TestHTMLInputPreprocessingNormalizesCRAndDataDropsNull(t *testing.T) {
	doc := domtest.Parse("\uFEFF<!doctype html>\r\n<p id=\"target\" title=\"a\rb\">x\r\ny\rz\x00</p>")
	p := doc.ElementByID("target")
	if p == nil {
		t.Fatal("parser did not create target paragraph")
	}
	if got, want := p.GetAttribute("title"), "a\nb"; got != want {
		t.Errorf("preprocessed attribute = %q, want %q", got, want)
	}
	if got, want := p.TextContent(), "x\ny\nz"; got != want {
		t.Errorf("preprocessed text = %q, want %q", got, want)
	}
}

func TestMetaCharsetSetsDocumentCharacterSet(t *testing.T) {
	doc := domtest.Parse(`<meta http-equiv="Content-Type" content="text/html; charset=koi8-r" charset="iso-8859-15">`)
	if got, want := doc.CharacterSet, "ISO-8859-15"; got != want {
		t.Errorf("character set = %q, want %q", got, want)
	}

	doc = domtest.Parse(`<meta http-equiv="Content-Type" content="text/html; charset=windows-1251"><meta charset=windows-1250>`)
	if got, want := doc.CharacterSet, "windows-1250"; got != want {
		t.Errorf("character set = %q, want %q", got, want)
	}
}

func TestNoscriptInHeadUsesDocumentScriptingFlag(t *testing.T) {
	const markup = `<!doctype html><head><noscript><head><style></style>`
	limits := dom.ParseLimits{Nodes: 1000, Depth: 128, Attributes: 64, AttributeBytes: 1 << 20}

	withoutScripting, err := dom.ParseBoundedWithHandlersAndScripting(markup, limits, false, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	withoutNodes := withoutScripting.ElementsByTagName("noscript")
	if len(withoutNodes) == 0 {
		t.Fatal("scripting-disabled document has no noscript element")
	}
	noscript := withoutNodes[0]
	if noscript == nil || noscript.FirstChild == nil || !noscript.FirstChild.Element() || noscript.FirstChild.Data != "style" {
		if noscript.FirstChild == nil {
			t.Fatal("scripting-disabled noscript has no first child, want style element")
		}
		t.Fatalf("scripting-disabled noscript child = type %d name %q text %q, want style element", noscript.FirstChild.Type, noscript.FirstChild.Data, noscript.FirstChild.DataContent)
	}

	withScripting := domtest.Parse(markup)
	withNodes := withScripting.ElementsByTagName("noscript")
	if len(withNodes) == 0 {
		t.Fatal("scripting-enabled document has no noscript element")
	}
	noscript = withNodes[0]
	if noscript == nil || noscript.FirstChild == nil || !noscript.FirstChild.Text() {
		t.Fatalf("scripting-enabled noscript child = %#v, want raw text", noscript)
	}
}

func TestParserRunsScriptsInDeclarativeShadowTemplatesButNotNestedTemplates(t *testing.T) {
	const markup = `<!doctype html><html><body><div id="host"><template shadowrootmode="open"><script id="shadow-script">window.ready = true;</script><template><script id="nested-inert">window.nested = true;</script></template></template></div><template><script id="inert">window.inert = true;</script></template></body></html>`
	limits := dom.ParseLimits{Nodes: 1000, Depth: 128, Attributes: 64, AttributeBytes: 1 << 20}
	var executed []string
	_, err := dom.ParseBoundedWithScriptHandler(markup, limits, func(_ *dom.Document, script *dom.Node) {
		executed = append(executed, script.GetAttribute("id"))
	})
	if err != nil {
		t.Fatalf("ParseBoundedWithScriptHandler: %v", err)
	}
	if got, want := strings.Join(executed, ","), "shadow-script"; got != want {
		t.Fatalf("parser script callbacks = %q, want %q", got, want)
	}
}

func TestMalformedMetaCharsetIsIgnored(t *testing.T) {
	doc := domtest.Parse(`<meta http-equiv="Content-Type" content='charset="windows-1251'>`)
	if got, want := doc.CharacterSet, "UTF-8"; got != want {
		t.Errorf("character set = %q, want %q", got, want)
	}
}

func TestCompletedHTMLDocumentCreatesImpliedBody(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><meta charset="utf-8"><title>upload</title><script>void 0</script>`)
	if doc.HTML == nil || doc.Body == nil || !doc.Body.Element() || doc.Body.Data != "body" {
		t.Fatalf("implied body = %#v, want an HTML body element", doc.Body)
	}
	if doc.Body.Parent != doc.HTML {
		t.Fatalf("implied body parent = %#v, want documentElement", doc.Body.Parent)
	}

	frameset, err := dom.ParseBounded(`<!doctype html><frameset><frame></frameset>`, dom.ParseLimits{
		Nodes: 64, Depth: 16, Attributes: 16, AttributeBytes: 256,
	})
	if err != nil {
		t.Fatal(err)
	}
	if frameset.Body != nil {
		t.Fatalf("frameset document body = %#v, want nil", frameset.Body)
	}
}

func TestTextDocumentPreservesMarkupAndCharacterReferences(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	pre := doc.NewElement("pre")
	pre.AppendChild(doc.NewText("<b>&#9733;</b>"))
	body.AppendChild(pre)
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	if got := body.TextContent(); got != "<b>&#9733;</b>" {
		t.Fatalf("plain text body content = %q", got)
	}
}

func TestHTMLNULHandlingFollowsTokenizerState(t *testing.T) {
	doc := domtest.Parse("<p id=\"text\">&\x00auml;|&a\x00uml;|&auml\x00;|a\x00b</p><p id=\"attr\" title=\"&a\x00uml;\">x</p>")
	text := doc.ElementByID("text")
	if text == nil {
		t.Fatal("text fixture missing")
	}
	if got, want := text.TextContent(), "&auml;|&auml;|ä;|ab"; got != want {
		t.Errorf("data-state NUL/reference output = %q, want %q", got, want)
	}
	attr := doc.ElementByID("attr")
	if attr == nil {
		t.Fatal("attribute fixture missing")
	}
	if got, want := attr.GetAttribute("title"), "&a\uFFFDuml;"; got != want {
		t.Errorf("attribute-state NUL output = %q, want %q", got, want)
	}
}

func TestHTMLNamedCharacterReferencesIncludeMultiCodePointNames(t *testing.T) {
	doc := domtest.Parse("<!doctype html><p id=\"target\">&nGt; &nLt;</p>")
	p := doc.ElementByID("target")
	if p == nil {
		t.Fatal("parser did not create target paragraph")
	}
	if got, want := p.TextContent(), "≫⃒ ≪⃒"; got != want {
		t.Errorf("decoded HTML named references = %q, want %q", got, want)
	}
}

func TestHTMLCharacterReferencesUseAttributeAmbiguousAmpersandRules(t *testing.T) {
	doc := domtest.Parse(`<p id="target" data-value="&copy=1|&copyx|&copy x|&amp;">&copy=1|&copyx|&copy x|&amp;</p>`)
	p := doc.ElementByID("target")
	if p == nil {
		t.Fatal("parser did not create target paragraph")
	}
	if got, want := p.GetAttribute("data-value"), "&copy=1|&copyx|© x|&"; got != want {
		t.Errorf("attribute character references = %q, want %q", got, want)
	}
	if got, want := p.TextContent(), "©=1|©x|© x|&"; got != want {
		t.Errorf("data character references = %q, want %q", got, want)
	}
}

func TestParseFragmentBoundedUsesContextInsertionAndTokenizerState(t *testing.T) {
	limits := dom.ParseLimits{Nodes: 128, Depth: 16, Attributes: 16, AttributeBytes: 256}

	owner := dom.NewDocument()
	tableContext := owner.NewElement("table")
	tableFragment, err := dom.ParseFragmentBounded(tableContext, `<tr><td>cell</td></tr>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	section := tableFragment.FirstChild
	if section == nil || section.Data != "tbody" || section.FirstChild == nil || section.FirstChild.Data != "tr" || section.FirstChild.FirstChild.TextContent() != "cell" {
		t.Fatalf("table-context fragment = %#v, want tbody > tr > td(cell)", section)
	}

	textareaContext := owner.NewElement("textarea")
	textareaFragment, err := dom.ParseFragmentBounded(textareaContext, `<b>&amp;</b></textarea><i>x</i>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	if textareaFragment.FirstChild == nil || !textareaFragment.FirstChild.Text() || textareaFragment.FirstChild.DataContent != `<b>&</b></textarea><i>x</i>` {
		t.Fatalf("textarea-context text = %#v, want all fragment input as text with the RCDATA entity decoded", textareaFragment.FirstChild)
	}
	if textareaFragment.LastChild != textareaFragment.FirstChild {
		t.Fatalf("textarea-context fragment = %#v, want one RCDATA text node", textareaFragment)
	}

	svgContext := owner.NewElement("path")
	svgContext.Namespace = dom.NSSVG
	svgContext.NamespaceURI = "http://www.w3.org/2000/svg"
	svgFragment, err := dom.ParseFragmentBounded(svgContext, `<![CDATA[x]]><g/>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	if svgFragment.FirstChild == nil || !svgFragment.FirstChild.Text() || svgFragment.FirstChild.DataContent != "x" {
		t.Fatalf("SVG-context CDATA node = %#v, want text x", svgFragment.FirstChild)
	}
	if svgFragment.LastChild == nil || svgFragment.LastChild.Data != "g" || svgFragment.LastChild.Namespace != dom.NSSVG {
		t.Fatalf("SVG-context child = %#v, want an SVG <g>", svgFragment.LastChild)
	}

	htmlContext := owner.NewElement("div")
	htmlFragment, err := dom.ParseFragmentBounded(htmlContext, `<![CDATA[y]]>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	if htmlFragment.FirstChild == nil || htmlFragment.FirstChild.Type != dom.NodeComment || htmlFragment.FirstChild.DataContent != "[CDATA[y]]" {
		t.Fatalf("HTML-context CDATA fallback = %#v, want bogus comment", htmlFragment.FirstChild)
	}
}

func TestTemplateInsertionModeDropsHeadBodyAndFramesetWrappers(t *testing.T) {
	doc := domtest.Parse(`<template id="t"><head><title>title</title></head><body><p id="p">body</p></body><frameset></frameset></template>`)
	template := doc.ElementByID("t")
	if template == nil {
		t.Fatal("parser did not create template")
	}
	content := template.TemplateContent()
	var tags []string
	for _, child := range content.ChildElements() {
		tags = append(tags, child.Data)
	}
	if got, want := strings.Join(tags, ","), "title,p"; got != want {
		var body []string
		for c := doc.Body.FirstChild; c != nil; c = c.NextSibling {
			body = append(body, c.Data+"="+c.TextContent())
		}
		t.Fatalf("template content children = %q, want %q; document body children: %q", got, want, body)
	}
	children := content.ChildElements()
	if len(children) != 2 || children[1].GetAttribute("id") != "p" || children[1].TextContent() != "body" {
		t.Fatalf("template content did not retain parsed paragraph: %#v", children)
	}
}
