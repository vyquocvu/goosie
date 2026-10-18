package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestScrollSnapProperties(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	container := doc.NewElement("div")
	container.SetAttribute("id", "container")
	body.AppendChild(container)

	item := doc.NewElement("div")
	item.SetAttribute("id", "item")
	container.AppendChild(item)

	sheet := css.Parse(`
		#container {
			scroll-snap-type: x mandatory;
			scroll-padding: 10px 20px;
		}
		#item {
			scroll-snap-align: center;
			scroll-snap-stop: always;
			scroll-margin: 5px;
		}
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	sCont := styles[container.ID]
	if sCont.ScrollSnapType != "x" {
		t.Errorf("expected ScrollSnapType x, got %q", sCont.ScrollSnapType)
	}
	if sCont.ScrollSnapStrictness != "mandatory" {
		t.Errorf("expected ScrollSnapStrictness mandatory, got %q", sCont.ScrollSnapStrictness)
	}
	if sCont.ScrollPaddingTop != 10 || sCont.ScrollPaddingRight != 20 || sCont.ScrollPaddingBottom != 10 || sCont.ScrollPaddingLeft != 20 {
		t.Errorf("unexpected ScrollPadding: T=%v R=%v B=%v L=%v",
			sCont.ScrollPaddingTop, sCont.ScrollPaddingRight, sCont.ScrollPaddingBottom, sCont.ScrollPaddingLeft)
	}

	sItem := styles[item.ID]
	if sItem.ScrollSnapAlign != "center" {
		t.Errorf("expected ScrollSnapAlign center, got %q", sItem.ScrollSnapAlign)
	}
	if sItem.ScrollSnapStop != "always" {
		t.Errorf("expected ScrollSnapStop always, got %q", sItem.ScrollSnapStop)
	}
	if sItem.ScrollMarginTop != 5 || sItem.ScrollMarginRight != 5 || sItem.ScrollMarginBottom != 5 || sItem.ScrollMarginLeft != 5 {
		t.Errorf("unexpected ScrollMargin: T=%v R=%v B=%v L=%v",
			sItem.ScrollMarginTop, sItem.ScrollMarginRight, sItem.ScrollMarginBottom, sItem.ScrollMarginLeft)
	}
}

func TestScrollSnapDefaults(t *testing.T) {
	def := style.DefaultStyle()
	if def.ScrollSnapType != "none" {
		t.Errorf("expected default ScrollSnapType none, got %q", def.ScrollSnapType)
	}
	if def.ScrollSnapStrictness != "proximity" {
		t.Errorf("expected default ScrollSnapStrictness proximity, got %q", def.ScrollSnapStrictness)
	}
	if def.ScrollSnapAlign != "none" {
		t.Errorf("expected default ScrollSnapAlign none, got %q", def.ScrollSnapAlign)
	}
	if def.ScrollSnapStop != "normal" {
		t.Errorf("expected default ScrollSnapStop normal, got %q", def.ScrollSnapStop)
	}
}
