package renderer

// DisplayType represents the display type of a layout box
type DisplayType string

const (
	// DisplayBlock represents a block-level box
	DisplayBlock DisplayType = "block"
	// DisplayInline represents an inline box
	DisplayInline DisplayType = "inline"
	// DisplayNone represents a box that should not be rendered
	DisplayNone DisplayType = "none"
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
	NodeID   int64       // ID of the corresponding RenderNode
	Box      Rect        // Computed box dimensions and position
	Display  DisplayType // Display type (block, inline, none)
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
