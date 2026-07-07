package renderer

import "image/color"

// DisplayType represents the display type of a layout box
type DisplayType string

const (
	// DisplayBlock represents a block-level box
	DisplayBlock DisplayType = "block"
	// DisplayInline represents an inline box
	DisplayInline DisplayType = "inline"
	// DisplayNone represents a box that should not be rendered
	DisplayNone DisplayType = "none"
	// DisplayFlex represents a flex container
	DisplayFlex DisplayType = "flex"
	// DisplayGrid represents a grid container
	DisplayGrid DisplayType = "grid"
	// DisplayInlineBlock represents an inline-block box
	DisplayInlineBlock DisplayType = "inline-block"
)

// Rect represents a rectangular box with position and dimensions
type Rect struct {
	X      float32 // X position
	Y      float32 // Y position
	Width  float32 // Width
	Height float32 // Height
}

// LayoutBox represents a node in the layout tree
// Each LayoutBox corresponds to a RenderNode and contains computed layout information
type LayoutBox struct {
	NodeID   int64        // ID of the corresponding RenderNode
	Box      Rect         // Computed box dimensions and position
	Display  DisplayType  // Display type (block, inline, none)
	Children []*LayoutBox // Child layout boxes

	// Padding (for future CSS support)
	PaddingTop    float32
	PaddingRight  float32
	PaddingBottom float32
	PaddingLeft   float32

	// Margin (for future CSS support)
	MarginTop    float32
	MarginRight  float32
	MarginBottom float32
	MarginLeft   float32

	// Border (for box model support)
	BorderTopWidth    float32
	BorderRightWidth  float32
	BorderBottomWidth float32
	BorderLeftWidth   float32

	BorderTopStyle    string
	BorderRightStyle  string
	BorderBottomStyle string
	BorderLeftStyle   string

	BorderTopColor    color.Color
	BorderRightColor  color.Color
	BorderBottomColor color.Color
	BorderLeftColor   color.Color

	// Background
	BackgroundColor color.Color

	// Inline layout information
	LineBoxes []*LineBox // Line boxes for inline content (if this contains inline children)

	// Flexbox container properties
	FlexDirection  string  // "row", "row-reverse", "column", "column-reverse"
	FlexWrap       string  // "nowrap", "wrap", "wrap-reverse"
	JustifyContent string  // "flex-start", "flex-end", "center", "space-between", "space-around", "space-evenly"
	AlignItems     string  // "flex-start", "flex-end", "center", "stretch", "baseline"
	AlignContent   string  // "flex-start", "flex-end", "center", "stretch", "space-between", "space-around"
	Gap            float32 // Gap between flex/grid items

	// Grid container properties
	GridTemplateColumns string
	GridTemplateRows    string

	// Grid item properties
	GridColumnStart int
	GridColumnEnd   int
	GridRowStart    int
	GridRowEnd      int

	// Flexbox item properties
	FlexGrow   float32 // How much item should grow relative to others
	FlexShrink float32 // How much item should shrink relative to others
	FlexBasis  float32 // Initial main size (0 means auto)
	AlignSelf  string  // Override align-items for this item
	Order      int     // Order of flex item

	// CSS positioning
	Position string // "static", "relative", "absolute", "fixed", "sticky"
	Float    string // "none", "left", "right"
	Clear    string // "none", "left", "right", "both"
}

// NewLayoutBox creates a new layout box
func NewLayoutBox(nodeID int64) *LayoutBox {
	return &LayoutBox{
		NodeID:   nodeID,
		Box:      Rect{},
		Display:  DisplayBlock,
		Children: make([]*LayoutBox, 0),
	}
}

// AddChild adds a child layout box
func (lb *LayoutBox) AddChild(child *LayoutBox) {
	lb.Children = append(lb.Children, child)
}

// IsBlock returns true if this is a block-level box
func (lb *LayoutBox) IsBlock() bool {
	return lb.Display == DisplayBlock
}

// IsInline returns true if this is an inline box
func (lb *LayoutBox) IsInline() bool {
	return lb.Display == DisplayInline
}

// GetContentBox returns the content box (excluding padding)
func (lb *LayoutBox) GetContentBox() Rect {
	return Rect{
		X:      lb.Box.X + lb.PaddingLeft,
		Y:      lb.Box.Y + lb.PaddingTop,
		Width:  lb.Box.Width - lb.PaddingLeft - lb.PaddingRight,
		Height: lb.Box.Height - lb.PaddingTop - lb.PaddingBottom,
	}
}

// Contains checks if a point (x, y) is within this layout box
func (lb *LayoutBox) Contains(x, y float32) bool {
	return x >= lb.Box.X && x <= lb.Box.X+lb.Box.Width &&
		y >= lb.Box.Y && y <= lb.Box.Y+lb.Box.Height
}

// collectFixed recursively collects all layout boxes with position:fixed.
func collectFixed(box *LayoutBox, out *[]*LayoutBox) {
	if box == nil {
		return
	}
	if box.Position == "fixed" {
		*out = append(*out, box)
	}
	for _, child := range box.Children {
		collectFixed(child, out)
	}
}
