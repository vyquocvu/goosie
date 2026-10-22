package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

func findNodeByTag(n *dom.Node, tag string) *dom.Node {
	if n == nil {
		return nil
	}
	if n.Element() && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findNodeByTag(c, tag); found != nil {
			return found
		}
	}
	return nil
}

func TestDeclarativeShadowDOM_OpenMode(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <my-card id="card">
    <template shadowrootmode="open">
      <div class="card-inner">Shadow Card Body</div>
      <slot></slot>
    </template>
    <span class="slotted-item">Slotted Content</span>
  </my-card>
</body>
</html>`

	doc := domtest.Parse(html)
	card := findNodeByTag(&doc.Node, "my-card")
	if card == nil {
		t.Fatal("my-card not found in document")
	}

	if card.ShadowRoot == nil {
		t.Fatal("expected card.ShadowRoot to be non-nil")
	}
	if card.ShadowRoot.Mode != "open" {
		t.Errorf("expected shadow root mode 'open', got '%s'", card.ShadowRoot.Mode)
	}

	// Verify shadow root element children: div.card-inner and slot
	srChild := card.ShadowRoot.FirstChild
	for srChild != nil && !srChild.Element() {
		srChild = srChild.NextSibling
	}
	if srChild == nil || srChild.Data != "div" {
		t.Fatalf("expected first shadow element child to be 'div', got %+v", srChild)
	}
	slotChild := srChild.NextSibling
	for slotChild != nil && !slotChild.Element() {
		slotChild = slotChild.NextSibling
	}
	if slotChild == nil || slotChild.Data != "slot" {
		t.Fatalf("expected second shadow element child to be 'slot', got %+v", slotChild)
	}

	// Verify light DOM child: span.slotted-item, and that template is NOT in the light DOM
	lightChild := card.FirstChild
	for lightChild != nil && !lightChild.Element() {
		lightChild = lightChild.NextSibling
	}
	if lightChild == nil || lightChild.Data != "span" {
		t.Fatalf("expected light element child to be 'span', got %+v", lightChild)
	}
	nextElem := lightChild.NextSibling
	for nextElem != nil && !nextElem.Element() {
		nextElem = nextElem.NextSibling
	}
	if nextElem != nil {
		t.Errorf("expected no additional element siblings in light DOM, got %+v", nextElem)
	}
}

func TestDeclarativeShadowDOM_ClosedMode(t *testing.T) {
	html := `<div><my-secret><template shadowrootmode="closed"><p>Secret content</p></template></my-secret></div>`
	doc := domtest.Parse(html)
	secret := findNodeByTag(&doc.Node, "my-secret")
	if secret == nil {
		t.Fatal("my-secret not found")
	}
	if secret.ShadowRoot == nil {
		t.Fatal("expected secret.ShadowRoot to be non-nil")
	}
	if secret.ShadowRoot.Mode != "closed" {
		t.Errorf("expected mode 'closed', got '%s'", secret.ShadowRoot.Mode)
	}
	if secret.ShadowRoot.FirstChild == nil || secret.ShadowRoot.FirstChild.Data != "p" {
		t.Fatalf("expected shadow child 'p', got %+v", secret.ShadowRoot.FirstChild)
	}
	if secret.FirstChild != nil {
		t.Errorf("expected empty light DOM for my-secret, got %+v", secret.FirstChild)
	}
}

func TestDeclarativeShadowDOM_LegacyAttribute(t *testing.T) {
	html := `<div><custom-widget><template shadowroot="open"><h1>Title</h1></template></custom-widget></div>`
	doc := domtest.Parse(html)
	widget := findNodeByTag(&doc.Node, "custom-widget")
	if widget == nil {
		t.Fatal("custom-widget not found")
	}
	if widget.ShadowRoot == nil {
		t.Fatal("expected widget.ShadowRoot to be non-nil from legacy shadowroot attribute")
	}
	if widget.ShadowRoot.FirstChild == nil || widget.ShadowRoot.FirstChild.Data != "h1" {
		t.Fatalf("expected shadow child 'h1', got %+v", widget.ShadowRoot.FirstChild)
	}
}

func TestDeclarativeShadowDOM_OrdinaryTemplatePreserved(t *testing.T) {
	html := `<div><template id="template-id"><p>Template content</p></template></div>`
	doc := domtest.Parse(html)
	div := findNodeByTag(&doc.Node, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	if div.ShadowRoot != nil {
		t.Fatal("ordinary template must not create shadow root on parent")
	}
	tmpl := findNodeByTag(&doc.Node, "template")
	if tmpl == nil {
		t.Fatal("expected template element to exist in tree when not shadowrootmode")
	}
}
