package renderer

import (
	"bytes"
	"image/color"
	"strings"
	"sync/atomic"
	"unsafe"

	"golang.org/x/net/html"

	"github.com/vyquocvu/goosie/internal/css"
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
	ImageData           *image.ImageData // For `<img>` elements
	BackgroundImageData *image.ImageData // For CSS background-image
}

// Style represents computed styles for a node (placeholder for future CSS support)
type Style struct {
	Display              css.DisplayAtom      // "block", "inline", "none", etc.
	Visibility           css.VisibilityAtom    // "visible", "hidden", "collapse"
	FontSize             float32
	FontWeight           string
	Color                color.Color
	BackgroundColor      color.Color
	BackgroundImage      string // "url(...)" or "none"
	BackgroundRepeat     css.BackgroundRepeatAtom     // "repeat", "no-repeat", "repeat-x", "repeat-y"
	BackgroundPosition   css.BackgroundPositionAtom   // "top left", "top center", "center", etc.
	BackgroundSize       css.BackgroundSizeAtom       // "auto", "cover", "contain"
	BackgroundAttachment css.BackgroundAttachmentAtom // "scroll", "fixed"
	BackgroundImageData  *image.ImageData
	Width                string
	Height          string
	FontFamily      string
	Opacity         float32
	TextAlign       css.TextAlignAtom        // "left", "right", "center", "justify"
	LetterSpacing   float32
	LineHeight      float32
	FontStyle       css.FontStyleAtom         // "normal", "italic"
	TextDecoration  css.TextDecorationAtom    // "none", "underline", "line-through"
	TextTransform   css.TextTransformAtom     // "none", "uppercase", "lowercase", "capitalize"

	// Positioning
	Position css.PositionAtom // "static", "relative", "absolute", "fixed", "sticky"
	Top      string
	Right    string
	Bottom   string
	Left     string
	ZIndex   int

	// Float and Clear
	Float css.FloatAtom // "none", "left", "right"
	Clear string        // "none", "left", "right", "both"

	// Overflow
	Overflow     css.OverflowAtom // "visible", "hidden", "scroll", "auto"
	OverflowX    string           // "visible", "hidden", "scroll", "auto"
	OverflowY    string           // "visible", "hidden", "scroll", "auto"
	TextOverflow string           // "clip", "ellipsis"

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

	// Box alignment properties (flex/grid)
	JustifyItems string
	JustifySelf  string

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
	WhiteSpace        css.WhiteSpaceAtom        // "normal", "nowrap", "pre", "pre-wrap", "pre-line"
	WordBreak         string                     // "normal", "break-all", "keep-all", "break-word"
	ListStyleType     css.ListStyleTypeAtom      // "disc", "circle", "square", "decimal", "none"
	ListStylePosition css.ListStylePositionAtom  // "inside", "outside"
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
		ComputedStyle: &Style{Opacity: 1.0},
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

// RemoveAttribute deletes an attribute. It is a no-op on nil nodes or when
// the attribute is absent.
func (n *RenderNode) RemoveAttribute(key string) {
	if n == nil {
		return
	}
	delete(n.Attrs, key)
}

// classes returns the space-separated class list from the class attribute.
func (n *RenderNode) classes() []string {
	if class, ok := n.Attrs["class"]; ok && class != "" {
		return strings.Fields(class)
	}
	return nil
}

// id returns the value of the id attribute.
func (n *RenderNode) id() string {
	return n.Attrs["id"]
}

// IsBlock returns true if the element is a block-level element
func (n *RenderNode) IsBlock() bool {
	if n.ComputedStyle != nil {
		if n.ComputedStyle.Float == css.FloatAtomLeft || n.ComputedStyle.Float == css.FloatAtomRight {
			return true
		}
		if n.ComputedStyle.Position == css.PositionAtomAbsolute || n.ComputedStyle.Position == css.PositionAtomFixed {
			return true
		}
		disp := n.ComputedStyle.Display
		switch disp {
		case css.DisplayAtomBlock, css.DisplayAtomFlex, css.DisplayAtomGrid,
			css.DisplayAtomTable, css.DisplayAtomFlowRoot, css.DisplayAtomListItem,
			css.DisplayAtomTableHeaderGroup, css.DisplayAtomTableRowGroup,
			css.DisplayAtomTableFooterGroup, css.DisplayAtomTableRow, css.DisplayAtomTableCell:
			return true
		case css.DisplayAtomInline, css.DisplayAtomInlineBlock:
			return false
		}
		// DisplayAtomNone means display:none — not block
		if disp == css.DisplayAtomNone {
			return false
		}
	}
	// Form elements and tables should be treated as block-level for proper layout
	switch n.TagName {
	case "div", "p", "h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "li", "body", "html", "header",
		"footer", "section", "article", "aside",
		"nav", "main", "pre", "blockquote",
		"dl", "dt", "dd",
		"input", "textarea", "button", "table", "form",
		"thead", "tbody", "tfoot", "tr", "td", "th":
		return true
	}
	return false
}

// IsFixedOrSticky returns true if the node or any of its ancestors has position: fixed or position: sticky.
func IsFixedOrSticky(node *RenderNode) bool {
	for n := node; n != nil; n = n.Parent {
		if n.ComputedStyle != nil && (n.ComputedStyle.Position == css.PositionAtomFixed || n.ComputedStyle.Position == css.PositionAtomSticky) {
			return true
		}
	}
	return false
}

// isFixedOrSticky is an internal package helper alias for IsFixedOrSticky.
func isFixedOrSticky(node *RenderNode) bool {
	return IsFixedOrSticky(node)
}

// GetImageData returns the image data for the node safely across goroutines.
func (n *RenderNode) GetImageData() *image.ImageData {
	if n == nil {
		return nil
	}
	return (*image.ImageData)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&n.ImageData))))
}

// SetImageData sets the image data for the node safely across goroutines.
func (n *RenderNode) SetImageData(data *image.ImageData) {
	if n == nil {
		return
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&n.ImageData)), unsafe.Pointer(data))
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

// processTextNode handles text node processing. Runs of spaces/tabs/CRs
// collapse to a single space, but newlines are preserved so white-space
// aware layout (pre, pre-line) can honor them; wrapping modes collapse the
// newlines later via collapseWhiteSpace/splitTextForWrapping.
func processTextNode(htmlNode *html.Node) *RenderNode {
	if htmlNode.Data == "" {
		return nil
	}
	node := NewRenderNode(NodeTypeText)

	var builder strings.Builder
	inWhitespace := false
	for _, r := range htmlNode.Data {
		switch r {
		case ' ', '\t', '\r':
			if !inWhitespace {
				builder.WriteByte(' ')
				inWhitespace = true
			}
		case '\n':
			builder.WriteByte('\n')
			inWhitespace = false
		default:
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

	switch htmlNode.Data {
	case "style", "script", "noscript", "template":
		// Script and style element contents are code/metadata, not renderable text.
		return node
	}

	if htmlNode.Data == "svg" {
		var buf bytes.Buffer
		if err := html.Render(&buf, htmlNode); err == nil {
			svgData := buf.Bytes()
			w, h := 0, 0
			if wAttr, ok := node.GetAttribute("width"); ok {
				w = int(parseLength(wAttr, 16))
			}
			if hAttr, ok := node.GetAttribute("height"); ok {
				h = int(parseLength(hAttr, 16))
			}
			if rgba, err := image.RasterizeSVG(svgData, w, h); err == nil && rgba != nil {
				node.ImageData = &image.ImageData{
					Image:  rgba,
					Width:  rgba.Bounds().Dx(),
					Height: rgba.Bounds().Dy(),
					Format: "svg",
					State:  image.StateLoaded,
				}
			}
		}
		return node
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
		Styles:              make(map[string]string),
		ImageData:           n.ImageData,
		BackgroundImageData: n.BackgroundImageData,
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
