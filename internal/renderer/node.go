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
	ID            int64 // Unique node identifier
	Type          NodeType
	TagName       string            // HTML tag name (e.g., "div", "p", "h1")
	Text          string            // Text content for text nodes
	Attrs         map[string]string // HTML attributes
	Styles        map[string]string // CSS styles
	Children      []*RenderNode     // Child nodes
	Parent        *RenderNode       // Parent node
	ComputedStyle *Style
	Box           *Box
	ImageData     *image.ImageData // For `<img>` elements
}

// Style represents computed styles for a node (placeholder for future CSS support)
type Style struct {
	Display         string // "block", "inline", "none", etc.
	Visibility      string // "visible", "hidden", "collapse"
	FontSize        float32
	FontWeight      string
	Color           color.Color
	BackgroundColor color.Color
	Width           string
	Height          string
	FontFamily      string
	Opacity         float32
	TextAlign       string // "left", "right", "center", "justify"
	LetterSpacing   float32
	LineHeight      float32
	FontStyle       string // "normal", "italic"
	TextDecoration  string // "none", "underline", "line-through"
	TextTransform   string // "none", "uppercase", "lowercase", "capitalize"

	// Positioning
	Position string // "static", "relative", "absolute", "fixed", "sticky"
	Top      string
	Right    string
	Bottom   string
	Left     string
	ZIndex   int

	// Float and Clear
	Float string // "none", "left", "right"
	Clear string // "none", "left", "right", "both"

	// Overflow
	Overflow     string // "visible", "hidden", "scroll", "auto"
	OverflowX    string // "visible", "hidden", "scroll", "auto"
	OverflowY    string // "visible", "hidden", "scroll", "auto"
	TextOverflow string // "clip", "ellipsis"

	// Box sizing
	BoxSizing string // "content-box", "border-box"

	// Min/Max constraints
	MinWidth  string
	MaxWidth  string
	MinHeight string
	MaxHeight string

	// Box model properties
	MarginTop    string
	MarginRight  string
	MarginBottom string
	MarginLeft   string

	PaddingTop    string
	PaddingRight  string
	PaddingBottom string
	PaddingLeft   string

	BorderTopWidth    string
	BorderRightWidth  string
	BorderBottomWidth string
	BorderLeftWidth   string

	BorderTopStyle    string
	BorderRightStyle  string
	BorderBottomStyle string
	BorderLeftStyle   string

	BorderTopColor    color.Color
	BorderRightColor  color.Color
	BorderBottomColor color.Color
	BorderLeftColor   color.Color

	// Flexbox container properties
	FlexDirection  string // "row", "row-reverse", "column", "column-reverse"
	FlexWrap       string // "nowrap", "wrap", "wrap-reverse"
	JustifyContent string // "flex-start", "flex-end", "center", "space-between", "space-around", "space-evenly"
	AlignItems     string // "flex-start", "flex-end", "center", "stretch", "baseline"
	AlignContent   string // "flex-start", "flex-end", "center", "stretch", "space-between", "space-around"
	Gap            string // Gap between flex/grid items
	RowGap         string // Row gap for grid/flex
	ColumnGap      string // Column gap for grid/flex

	// Grid Container properties
	GridTemplateColumns string
	GridTemplateRows    string

	// Grid Item properties
	GridColumnStart string
	GridColumnEnd   string
	GridRowStart    string
	GridRowEnd      string

	// Flexbox item properties
	FlexGrow   float32 // How much item should grow
	FlexShrink float32 // How much item should shrink (default 1)
	FlexBasis  string  // Initial main size ("auto", length, percentage)
	AlignSelf  string  // Override align-items for this item
	Order      int     // Order of flex item

	// CSS custom properties (variables) inherited from this element's cascade
	CustomProperties map[string]string

	// Visual properties
	BorderRadius      string // Shorthand or individual corner radii
	BoxShadow         string // Box shadow specification
	TextShadow        string // Text shadow specification
	Transform         string // CSS transform functions
	TransformOrigin   string // Transform origin point
	Transition        string // CSS transition specification
	Cursor            string // Cursor type
	VerticalAlign     string // "baseline", "top", "middle", "bottom", "text-top", "text-bottom", "sub", "super"
	WhiteSpace        string // "normal", "nowrap", "pre", "pre-wrap", "pre-line"
	WordBreak         string // "normal", "break-all", "keep-all", "break-word"
	ListStyleType     string // "disc", "circle", "square", "decimal", "none"
	ListStylePosition string // "inside", "outside"
	TableLayout       string // "auto", "fixed"
	BorderCollapse    string // "collapse", "separate"
	BorderSpacing     string // Length value for collapsed borders
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
		Styles:        make(map[string]string),
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
	if n.ComputedStyle != nil && n.ComputedStyle.Display != "" {
		disp := n.ComputedStyle.Display
		if disp == "block" || disp == "flex" || disp == "grid" || disp == "table" {
			return true
		}
		if disp == "inline" || disp == "inline-block" || disp == "inline-flex" || disp == "inline-grid" {
			return false
		}
	}
	blockElements := map[string]bool{
		"div": true, "p": true, "h1": true, "h2": true, "h3": true,
		"h4": true, "h5": true, "h6": true, "ul": true, "ol": true,
		"li": true, "body": true, "html": true, "header": true,
		"footer": true, "section": true, "article": true, "aside": true,
		"nav": true, "main": true, "pre": true, "blockquote": true,
		"dl": true, "dt": true, "dd": true,
		// Form elements and tables should be treated as block-level for proper layout
		"input": true, "textarea": true, "button": true, "table": true, "form": true,
		"thead": true, "tbody": true, "tfoot": true, "tr": true, "td": true, "th": true,
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
	if htmlNode.Data == "" {
		return nil
	}
	node := NewRenderNode(NodeTypeText)

	var builder strings.Builder
	inWhitespace := false
	for _, r := range htmlNode.Data {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inWhitespace {
				builder.WriteByte(' ')
				inWhitespace = true
			}
		} else {
			builder.WriteRune(r)
			inWhitespace = false
		}
	}
	node.Text = builder.String()

	return node
}

// processElementNode handles element node processing
func processElementNode(htmlNode *html.Node) *RenderNode {
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

// Clone creates a deep copy of the RenderNode tree
func (n *RenderNode) Clone() *RenderNode {
	if n == nil {
		return nil
	}

	clone := &RenderNode{
		ID:        n.ID,
		Type:      n.Type,
		TagName:   n.TagName,
		Text:      n.Text,
		Attrs:     make(map[string]string),
		Styles:    make(map[string]string),
		ImageData: n.ImageData,
	}

	for k, v := range n.Attrs {
		clone.Attrs[k] = v
	}

	for k, v := range n.Styles {
		clone.Styles[k] = v
	}

	if n.ComputedStyle != nil {
		computedStyle := *n.ComputedStyle
		clone.ComputedStyle = &computedStyle
	}

	for _, child := range n.Children {
		childClone := child.Clone()
		childClone.Parent = clone
		clone.Children = append(clone.Children, childClone)
	}

	return clone
}
