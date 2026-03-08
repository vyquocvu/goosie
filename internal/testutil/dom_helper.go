package testutil

import (
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// DOMHelper provides utility functions for creating DOM nodes
type DOMHelper struct{}

// NewDOMHelper creates a new DOMHelper
func NewDOMHelper() *DOMHelper {
	return &DOMHelper{}
}

// CreateElement creates a new element node
func (h *DOMHelper) CreateElement(tagName string, attrs map[string]string, children ...*html.Node) *html.Node {
	node := &html.Node{
		Type:     html.ElementNode,
		Data:     tagName,
		DataAtom: atom.Lookup([]byte(tagName)),
	}

	for k, v := range attrs {
		node.Attr = append(node.Attr, html.Attribute{Key: k, Val: v})
	}

	for _, child := range children {
		node.AppendChild(child)
	}

	return node
}

// CreateTextNode creates a new text node
func (h *DOMHelper) CreateTextNode(text string) *html.Node {
	return &html.Node{
		Type: html.TextNode,
		Data: text,
	}
}

// CreateDocument creates a basic HTML document structure
func (h *DOMHelper) CreateDocument(bodyChildren ...*html.Node) *html.Node {
	doc := &html.Node{Type: html.DocumentNode}
	htmlNode := h.CreateElement("html", nil)
	doc.AppendChild(htmlNode)
	
	body := h.CreateElement("body", nil)
	htmlNode.AppendChild(body)

	for _, child := range bodyChildren {
		body.AppendChild(child)
	}

	return doc
}
