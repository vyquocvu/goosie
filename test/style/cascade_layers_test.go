package style_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func resolveTestDoc(html string, cssText string) (*dom.Document, map[dom.NodeID]*style.ComputedStyle) {
	doc := domtest.Parse(html)
	sheet := css.Parse(cssText)
	vp := style.Viewport{W: 1280, H: 800}
	styles := style.ResolveViewport(doc, []*css.Stylesheet{sheet}, vp, nil)
	return doc, styles
}

func findElementByID(n *dom.Node, id string) *dom.Node {
	if n == nil {
		return nil
	}
	if n.Element() && n.GetAttribute("id") == id {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElementByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

func TestCascadeLayers_StatementOrdering(t *testing.T) {
	// @layer reset, base, utilities explicitly declares layer order.
	// utilities is declared last, so it beats base and reset, regardless of specificity!
	cssText := `
@layer reset, base, utilities;

@layer utilities {
    .btn { color: #ff0000; }
}

@layer base {
    #action.btn { color: #0000ff; }
}

@layer reset {
    button#action.btn { color: #000000; }
}
`
	html := `<html><body><button id="action" class="btn">Click</button></body></html>`
	doc, styles := resolveTestDoc(html, cssText)
	btn := findElementByID(&doc.Node, "action")
	if btn == nil {
		t.Fatal("action button not found")
	}

	cs := styles[btn.ID]
	if cs == nil {
		t.Fatal("style for action button not resolved")
	}

	// Red is #ff0000 (RGB: 255, 0, 0, 255)
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("expected color from 'utilities' layer (red), got rgba(%d,%d,%d,%d)",
			cs.Color.R, cs.Color.G, cs.Color.B, cs.Color.A)
	}
}

func TestCascadeLayers_UnlayeredBeatsLayered(t *testing.T) {
	// Normal unlayered author styles must outrank all layered author styles,
	// even if the layered rule has much higher specificity.
	cssText := `
@layer theme {
    #box.item { background-color: #00ff00; }
}

div.item { background-color: #ffff00; }
`
	html := `<html><body><div id="box" class="item">Box</div></body></html>`
	doc, styles := resolveTestDoc(html, cssText)
	box := findElementByID(&doc.Node, "box")
	if box == nil {
		t.Fatal("box not found")
	}

	cs := styles[box.ID]
	if cs == nil {
		t.Fatal("style for box not resolved")
	}

	// Yellow is #ffff00 (RGB: 255, 255, 0, 255)
	if cs.BackgroundColor.R != 255 || cs.BackgroundColor.G != 255 || cs.BackgroundColor.B != 0 {
		t.Fatalf("expected unlayered color (yellow), got rgba(%d,%d,%d,%d)",
			cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B, cs.BackgroundColor.A)
	}
}

func TestCascadeLayers_ImportantReversesPrecedence(t *testing.T) {
	// For !important declarations:
	// 1. Layered styles beat unlayered styles.
	// 2. Earlier layers beat later layers.
	cssText := `
@layer reset, utilities;

/* Unlayered !important */
#alert { color: #000000 !important; }

/* utilities !important */
@layer utilities {
    #alert { color: #0000ff !important; }
}

/* reset !important: earlier layer than utilities */
@layer reset {
    .msg { color: #ff0000 !important; }
}
`
	html := `<html><body><div id="alert" class="msg">Alert</div></body></html>`
	doc, styles := resolveTestDoc(html, cssText)
	alert := findElementByID(&doc.Node, "alert")
	if alert == nil {
		t.Fatal("alert not found")
	}

	cs := styles[alert.ID]
	if cs == nil {
		t.Fatal("style for alert not resolved")
	}

	// Reset is the earliest layer, so its !important beats utilities and unlayered!
	// Red is #ff0000 (RGB: 255, 0, 0, 255)
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("expected reset !important (red) to win, got rgba(%d,%d,%d,%d)",
			cs.Color.R, cs.Color.G, cs.Color.B, cs.Color.A)
	}
}

func TestCascadeLayers_NestedLayers(t *testing.T) {
	// Nested @layer outer { @layer inner { ... } } should flatten to outer.inner
	cssText := `
@layer framework {
    @layer components {
        .card { padding-top: 20px; }
    }
}

@layer framework.components {
    .card { padding-top: 35px; }
}
`
	sheet := css.Parse(cssText)
	if len(sheet.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(sheet.Rules))
	}
	if sheet.Rules[0].Layer != "framework.components" {
		t.Errorf("expected layer 'framework.components', got '%s'", sheet.Rules[0].Layer)
	}
	if sheet.Rules[1].Layer != "framework.components" {
		t.Errorf("expected layer 'framework.components', got '%s'", sheet.Rules[1].Layer)
	}

	html := `<html><body><div class="card">Card</div></body></html>`
	doc, styles := resolveTestDoc(html, cssText)
	div := doc.Node.FirstChild
	for div != nil && !div.Element() {
		div = div.NextSibling
	}
	for div != nil && div.Data != "body" {
		div = div.FirstChild
	}
	if div != nil {
		card := div.FirstChild
		for card != nil && (!card.Element() || card.Data != "div") {
			card = card.NextSibling
		}
		if card != nil && styles[card.ID] != nil {
			if styles[card.ID].PaddingTop != 35 {
				t.Fatalf("expected padding-top 35px from later rule in same layer, got %f", styles[card.ID].PaddingTop)
			}
		}
	}
}

func TestCascadeLayers_AnonymousLayers(t *testing.T) {
	// Anonymous @layer blocks receive unique anonymous layer names and execute in source order
	cssText := `
@layer {
    .box { width: 100px; }
}

@layer {
    .box { width: 250px; }
}
`
	sheet := css.Parse(cssText)
	if len(sheet.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(sheet.Rules))
	}
	if !strings.HasPrefix(sheet.Rules[0].Layer, "anon#") {
		t.Errorf("expected anon layer prefix, got '%s'", sheet.Rules[0].Layer)
	}
	if !strings.HasPrefix(sheet.Rules[1].Layer, "anon#") {
		t.Errorf("expected anon layer prefix, got '%s'", sheet.Rules[1].Layer)
	}
	if sheet.Rules[0].Layer == sheet.Rules[1].Layer {
		t.Errorf("expected different anonymous layer names, got '%s'", sheet.Rules[0].Layer)
	}

	html := `<html><body><div class="box">Box</div></body></html>`
	doc, styles := resolveTestDoc(html, cssText)
	box := doc.Node.FirstChild
	for box != nil && box.Data != "body" {
		box = box.FirstChild
	}
	if box != nil {
		el := box.FirstChild
		for el != nil && (!el.Element() || el.Data != "div") {
			el = el.NextSibling
		}
		if el != nil && styles[el.ID] != nil {
			if styles[el.ID].Width != 250 {
				t.Fatalf("expected width 250 from second anonymous layer, got %f", styles[el.ID].Width)
			}
		}
	}
}
