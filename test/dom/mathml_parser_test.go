package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

const mathMLNamespace = "http://www.w3.org/1998/Math/MathML"

func TestHTMLParserMathMLNamespacesAndAttributeAdjustments(t *testing.T) {
	doc := domtest.Parse(`<div id="host"><MATH><MI MATHVARIANT="BOLD"></MI><semantics DEFINITIONurl="www.example.org/FOO"></semantics></MATH></div>`)
	host := doc.ElementByID("host")
	if host == nil || host.FirstChild == nil {
		t.Fatal("MathML subtree was not parsed")
	}
	math := host.FirstChild
	if math.Data != "math" || math.Namespace != dom.NSMathML || math.NamespaceURI != mathMLNamespace {
		t.Fatalf("MathML root = (%q, %q), want math in MathML namespace", math.Data, math.NamespaceURI)
	}
	mi := math.FirstChild
	if mi == nil || mi.Data != "mi" || mi.Namespace != dom.NSMathML {
		t.Fatalf("MathML child = %#v, want lowercase mi in MathML namespace", mi)
	}
	if got := mi.GetAttribute("mathvariant"); got != "BOLD" {
		t.Errorf("MathML attribute value = %q, want BOLD", got)
	}
	semantics := mi.NextSibling
	if semantics == nil || semantics.Data != "semantics" || !semantics.HasAttribute("definitionURL") {
		t.Fatalf("MathML semantics attributes = %#v, want adjusted definitionURL", semantics)
	}
	if got := semantics.GetAttribute("definitionURL"); got != "www.example.org/FOO" {
		t.Errorf("definitionURL = %q, want original value", got)
	}
}

func TestMathMLTextAndAnnotationIntegrationPoints(t *testing.T) {
	doc := domtest.Parse(`<div id="text"><math><mtext><Span>x</Span><mi>y</mi></mtext></math></div><div id="point"><math><annotation-xml encoding="TEXT/HTML"><p id="inside">x</p></annotation-xml><annotation-xml><p id="outside">y</p></annotation-xml></math></div>`)
	text := doc.ElementByID("text")
	if text == nil {
		t.Fatal("mtext fixture missing")
	}
	mtext := text.FirstChild.FirstChild
	span := mtext.FirstChild
	if mtext.Namespace != dom.NSMathML || span.Data != "span" || span.Namespace != dom.NSHTML {
		t.Fatalf("mtext integration subtree = mtext(%q) > span(%q), want MathML > HTML", mtext.Namespace, span.Namespace)
	}
	mi := span.NextSibling
	if mi == nil || mi.Data != "mi" || mi.Namespace != dom.NSHTML {
		t.Fatalf("mi token at mtext integration point = %#v, want HTML-namespace mi", mi)
	}

	inside := doc.ElementByID("inside")
	if inside == nil || inside.Parent == nil || inside.Parent.Data != "annotation-xml" || inside.Namespace != dom.NSHTML {
		t.Fatalf("HTML annotation integration child = %#v, want HTML p inside annotation-xml", inside)
	}
	outside := doc.ElementByID("outside")
	if outside == nil || outside.Parent != doc.ElementByID("point") || outside.Namespace != dom.NSHTML {
		t.Fatalf("non-integration breakout child = %#v, want HTML p fostered out to host", outside)
	}
}
