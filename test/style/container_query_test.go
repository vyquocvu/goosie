package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func parseAndResolve(html, cssText string) (*dom.Document, map[dom.NodeID]*style.ComputedStyle) {
	doc := domtest.Parse(html)
	sheet := css.Parse(cssText)
	res := style.ResolveViewport(doc, []*css.Stylesheet{sheet}, style.Viewport{W: 1000, H: 800}, nil)
	return doc, res
}

func findElementByClass(n *dom.Node, className string) *dom.Node {
	if n == nil {
		return nil
	}
	if n.Element() {
		for _, c := range n.ClassList() {
			if c == className {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElementByClass(c, className); found != nil {
			return found
		}
	}
	if n.ShadowRoot != nil {
		for c := n.ShadowRoot.FirstChild; c != nil; c = c.NextSibling {
			if found := findElementByClass(c, className); found != nil {
				return found
			}
		}
	}
	return nil
}

func TestContainerProperties_Parsed(t *testing.T) {
	html := `
		<div class="c1" style="container-type: inline-size; container-name: my-panel"></div>
		<div class="c2" style="container: sidebar / size"></div>
		<div class="c3" style="container: widget"></div>
	`
	doc, res := parseAndResolve(html, "")

	c1 := findElementByClass(&doc.Node, "c1")
	if c1 == nil || res[c1.ID] == nil {
		t.Fatalf("c1 not found or unstyled")
	}
	if res[c1.ID].ContainerType != "inline-size" {
		t.Errorf("c1 container-type = %q, want 'inline-size'", res[c1.ID].ContainerType)
	}
	if res[c1.ID].ContainerName != "my-panel" {
		t.Errorf("c1 container-name = %q, want 'my-panel'", res[c1.ID].ContainerName)
	}

	c2 := findElementByClass(&doc.Node, "c2")
	if c2 == nil || res[c2.ID] == nil {
		t.Fatalf("c2 not found or unstyled")
	}
	if res[c2.ID].ContainerType != "size" {
		t.Errorf("c2 container-type = %q, want 'size'", res[c2.ID].ContainerType)
	}
	if res[c2.ID].ContainerName != "sidebar" {
		t.Errorf("c2 container-name = %q, want 'sidebar'", res[c2.ID].ContainerName)
	}

	c3 := findElementByClass(&doc.Node, "c3")
	if c3 == nil || res[c3.ID] == nil {
		t.Fatalf("c3 not found or unstyled")
	}
	if res[c3.ID].ContainerType != "normal" {
		t.Errorf("c3 container-type = %q, want 'normal'", res[c3.ID].ContainerType)
	}
	if res[c3.ID].ContainerName != "widget" {
		t.Errorf("c3 container-name = %q, want 'widget'", res[c3.ID].ContainerName)
	}
}

func TestContainerQuery_SizeMatching(t *testing.T) {
	html := `
		<div class="container-small" style="container-type: inline-size; width: 300px;">
			<div class="card card-small">Small Card</div>
		</div>
		<div class="container-large" style="container-type: inline-size; width: 700px;">
			<div class="card card-large">Large Card</div>
		</div>
	`
	cssText := `
		.card {
			color: black;
		}
		@container (min-width: 500px) {
			.card {
				color: red;
			}
		}
	`
	doc, res := parseAndResolve(html, cssText)

	smallCard := findElementByClass(&doc.Node, "card-small")
	largeCard := findElementByClass(&doc.Node, "card-large")

	if smallCard == nil || res[smallCard.ID] == nil {
		t.Fatalf("smallCard not found or unstyled")
	}
	if largeCard == nil || res[largeCard.ID] == nil {
		t.Fatalf("largeCard not found or unstyled")
	}

	// Small card container is 300px < 500px -> should remain black (0, 0, 0)
	if res[smallCard.ID].Color.R != 0 {
		t.Errorf("smallCard color = %+v, want black", res[smallCard.ID].Color)
	}

	// Large card container is 700px >= 500px -> should be red (255, 0, 0)
	if res[largeCard.ID].Color.R != 255 || res[largeCard.ID].Color.G != 0 {
		t.Errorf("largeCard color = %+v, want red", res[largeCard.ID].Color)
	}
}

func TestContainerQuery_NamedMatching(t *testing.T) {
	html := `
		<div style="container: sidebar / inline-size; width: 400px;">
			<div class="widget">Sidebar Widget</div>
		</div>
		<div style="container: content / inline-size; width: 400px;">
			<div class="widget content-widget">Content Widget</div>
		</div>
	`
	cssText := `
		.widget {
			font-weight: 400;
		}
		@container sidebar (max-width: 500px) {
			.widget {
				font-weight: 700;
			}
		}
	`
	doc, res := parseAndResolve(html, cssText)

	sidebarWidget := findElementByClass(&doc.Node, "widget")
	contentWidget := findElementByClass(&doc.Node, "content-widget")

	if sidebarWidget == nil || res[sidebarWidget.ID] == nil {
		t.Fatalf("sidebarWidget not found or unstyled")
	}
	if contentWidget == nil || res[contentWidget.ID] == nil {
		t.Fatalf("contentWidget not found or unstyled")
	}

	// sidebarWidget matches @container sidebar -> font-weight: 700
	if res[sidebarWidget.ID].FontWeight != style.WeightBold {
		t.Errorf("sidebarWidget weight = %v, want 700", res[sidebarWidget.ID].FontWeight)
	}

	// contentWidget does not match "sidebar" container name -> font-weight: 400
	if res[contentWidget.ID].FontWeight != style.WeightNormal {
		t.Errorf("contentWidget weight = %v, want 400", res[contentWidget.ID].FontWeight)
	}
}

func TestContainerQuery_RangeSyntax(t *testing.T) {
	html := `
		<div style="container-type: inline-size; width: 450px;">
			<div class="card1">Card 1</div>
		</div>
		<div style="container-type: inline-size; width: 250px;">
			<div class="card2">Card 2</div>
		</div>
	`
	cssText := `
		@container (300px <= width <= 600px) {
			.card1, .card2 {
				color: blue;
			}
		}
	`
	doc, res := parseAndResolve(html, cssText)

	card1 := findElementByClass(&doc.Node, "card1")
	card2 := findElementByClass(&doc.Node, "card2")

	// card1 container is 450px: 300 <= 450 <= 600 -> true, color blue (0, 0, 255)
	if res[card1.ID].Color.B != 255 {
		t.Errorf("card1 color = %+v, want blue", res[card1.ID].Color)
	}

	// card2 container is 250px: not in [300, 600] -> false, color black
	if res[card2.ID].Color.B != 0 {
		t.Errorf("card2 color = %+v, want black", res[card2.ID].Color)
	}
}

func TestContainerQuery_RelativeUnitsInDeclaration(t *testing.T) {
	html := `
		<div style="container-type: inline-size; width: 500px; height: 300px;">
			<div class="item" style="padding-left: 10cqw; padding-top: 10cqh;">Item</div>
		</div>
	`
	doc, res := parseAndResolve(html, "")

	item := findElementByClass(&doc.Node, "item")
	if item == nil || res[item.ID] == nil {
		t.Fatalf("item not found")
	}

	// 10cqw of 500px = 50px
	if res[item.ID].PaddingLeft != 50 {
		t.Errorf("item padding-left = %v, want 50", res[item.ID].PaddingLeft)
	}

	// 10cqh of 300px = 30px
	if res[item.ID].PaddingTop != 30 {
		t.Errorf("item padding-top = %v, want 30", res[item.ID].PaddingTop)
	}
}
