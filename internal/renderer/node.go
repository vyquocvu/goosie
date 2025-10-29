package renderer

import (
	"image/color"
	"strings"
	"sync/atomic"

	"golang.org/x/net/html"

	"github.com/vyquocvu/goosie/internal/image"
)

// NodeType represents the type of render node
type NodeType int

const (
	// NodeTypeElement represents an HTML element node
	NodeTypeElement NodeType = iota
	// NodeTypeText represents a text node
	NodeTypeText
)

// nodeIDCounter is used to generate unique node IDs
var nodeIDCounter int64

// RenderNode represents a node in the render tree
type RenderNode struct {
	ID            int64             // Unique node identifier
	Type          NodeType
	TagName       string            // HTML tag name (e.g., "div", "p", "h1")
	Text          string            // Text content for text nodes
	Attrs         map[string]string // HTML attributes
	Children      []*RenderNode     // Child nodes
	Parent        *RenderNode       // Parent node
	ComputedStyle *Style
	Box           *Box
	ImageData     *image.ImageData // For `<img>` elements
}

// Style represents computed styles for a node (placeholder for future CSS support)
type Style struct {
	Display         string      // "block", "inline", "none", etc.
	FontSize        float32
	FontWeight      string
	Color           color.Color
	BackgroundColor color.Color
	Width           string
	Margin          string
	FontFamily      string
	Opacity         float32
}

// Box represents the layout box for a render node
type Box struct {
	X             float32 // X position
	Y             float32 // Y position
	Width         float32 // Width
	Height        float32 // Height
	PaddingTop    float32
	PaddingRight  float32
	PaddingBottom float32
	PaddingLeft   float32
}

// NewRenderNode creates a new render node with a unique ID
func NewRenderNode(nodeType NodeType) *RenderNode {
	return &RenderNode{
		ID:            atomic.AddInt64(&nodeIDCounter, 1),
		Type:          nodeType,
		Attrs:         make(map[string]string),
		Children:      make([]*RenderNode, 0),
		Box:           &Box{},
		ComputedStyle: &Style{},
	}
}

// AddChild adds a child node to this node
func (n *RenderNode) AddChild(child *RenderNode) {
	child.Parent = n
	n.Children = append(n.Children, child)
}

// GetAttribute returns the value of an attribute
func (n *RenderNode) GetAttribute(key string) (string, bool) {
	val, ok := n.Attrs[key]
	return val, ok
}

// SetAttribute sets an attribute value
func (n *RenderNode) SetAttribute(key, value string) {
	n.Attrs[key] = value
}

// IsBlock returns true if the element is a block-level element
func (n *RenderNode) IsBlock() bool {
	blockElements := map[string]bool{
		"div": true, "p": true, "h1": true, "h2": true, "h3": true,
		"h4": true, "h5": true, "h6": true, "ul": true, "ol": true,
		"li": true, "body": true, "html": true, "header": true,
		"footer": true, "section": true, "article": true, "aside": true,
		"nav": true, "main": true, "pre": true, "blockquote": true,
	}
	return blockElements[n.TagName]
}

// BuildRenderTree builds a render tree from an HTML node
func BuildRenderTree(htmlNode *html.Node) *RenderNode {
	if htmlNode == nil {
		return nil
	}
	switch htmlNode.Type {
	case html.CommentNode, html.DoctypeNode:
		return nil
	case html.TextNode:
		return processTextNode(htmlNode)
	case html.ElementNode:
		return processElementNode(htmlNode)
	default:
		return nil
	}
}

// processTextNode handles text node processing
func processTextNode(htmlNode *html.Node) *RenderNode {
	trimmedText := strings.TrimSpace(htmlNode.Data)
	if trimmedText == "" {
		return nil
	}
	node := NewRenderNode(NodeTypeText)
	normalizedText := strings.Join(strings.Fields(htmlNode.Data), " ")
	node.Text = normalizedText
	return node
}

// processElementNode handles element node processing
func processElementNode(htmlNode *html.Node) *RenderNode {
	nonVisibleTags := map[string]bool{
		"script": true, "style": true, "meta": true, "link": true,
		"head": true, "noscript": true, "template": true, "iframe": true,
		"title": true, "base": true,
	}
	if nonVisibleTags[htmlNode.Data] {
		return nil
	}
	node := NewRenderNode(NodeTypeElement)
	node.TagName = htmlNode.Data
	for _, attr := range htmlNode.Attr {
		node.SetAttribute(attr.Key, attr.Val)
	}
	for child := htmlNode.FirstChild; child != nil; child = child.NextSibling {
		childNode := BuildRenderTree(child)
		if childNode != nil {
			node.AddChild(childNode)
		}
	}
	return node
}
