package renderer

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
)

// DisplayCommandKind identifies the type of a backend-neutral display command.
// M5.1: These commands form the stable contract between layout and rendering.
type DisplayCommandKind uint8

const (
	// CmdRect fills an axis-aligned rectangle.
	CmdRect DisplayCommandKind = iota
	// CmdBorder draws per-side borders within a bounding box.
	CmdBorder
	// CmdText draws a run of styled text.
	CmdText
	// CmdImage draws a decoded image within a bounding box.
	CmdImage
	// CmdPushClip starts a clipping region.
	CmdPushClip
	// CmdPopClip ends the most recent clipping region.
	CmdPopClip
	// CmdPushTransform starts a transform group.
	CmdPushTransform
	// CmdPopTransform ends the most recent transform group.
	CmdPopTransform
	// CmdPushOpacity starts an opacity group.
	CmdPushOpacity
	// CmdPopOpacity ends the most recent opacity group.
	CmdPopOpacity
	// CmdPushStackingContext starts a stacking context.
	CmdPushStackingContext
	// CmdPopStackingContext ends the most recent stacking context.
	CmdPopStackingContext
)

// String returns a human-readable name for the command kind.
func (k DisplayCommandKind) String() string {
	switch k {
	case CmdRect:
		return "Rect"
	case CmdBorder:
		return "Border"
	case CmdText:
		return "Text"
	case CmdImage:
		return "Image"
	case CmdPushClip:
		return "PushClip"
	case CmdPopClip:
		return "PopClip"
	case CmdPushTransform:
		return "PushTransform"
	case CmdPopTransform:
		return "PopTransform"
	case CmdPushOpacity:
		return "PushOpacity"
	case CmdPopOpacity:
		return "PopOpacity"
	case CmdPushStackingContext:
		return "PushStackingContext"
	case CmdPopStackingContext:
		return "PopStackingContext"
	default:
		return fmt.Sprintf("Unknown(%d)", k)
	}
}

// RectF is a float32 axis-aligned rectangle used in display commands.
// All coordinates are in layout-space pixels.
type RectF struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	W float32 `json:"w"`
	H float32 `json:"h"`
}

// Contains reports whether point (px, py) lies within the rectangle.
func (r RectF) Contains(px, py float32) bool {
	return px >= r.X && px < r.X+r.W && py >= r.Y && py < r.Y+r.H
}

// Intersects reports whether r and other overlap.
func (r RectF) Intersects(other RectF) bool {
	return r.X < other.X+other.W && r.X+r.W > other.X &&
		r.Y < other.Y+other.H && r.Y+r.H > other.Y
}

// BorderStyle identifies a CSS border drawing style.
type BorderStyle uint8

const (
	BorderNone BorderStyle = iota
	BorderSolid
	BorderDashed
	BorderDotted
)

// String returns the CSS name for the border style.
func (s BorderStyle) String() string {
	switch s {
	case BorderNone:
		return "none"
	case BorderSolid:
		return "solid"
	case BorderDashed:
		return "dashed"
	case BorderDotted:
		return "dotted"
	default:
		return "unknown"
	}
}

// MarshalJSON implements json.Marshaler.
func (s BorderStyle) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (s *BorderStyle) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	switch str {
	case "none":
		*s = BorderNone
	case "solid":
		*s = BorderSolid
	case "dashed":
		*s = BorderDashed
	case "dotted":
		*s = BorderDotted
	default:
		*s = BorderNone
	}
	return nil
}

// BorderSide describes one edge of a border.
type BorderSide struct {
	Width float32     `json:"width"`
	Color color.Color `json:"color"`
	Style BorderStyle `json:"style"`
}

// --- Per-command data structs ---

// RectCommand fills an axis-aligned rectangle with a solid color.
type RectCommand struct {
	Bounds RectF       `json:"bounds"`
	Color  color.Color `json:"color"`
}

// BorderCommand draws per-side borders within a bounding box.
type BorderCommand struct {
	Bounds RectF      `json:"bounds"`
	Top    BorderSide `json:"top"`
	Right  BorderSide `json:"right"`
	Bottom BorderSide `json:"bottom"`
	Left   BorderSide `json:"left"`
}

// TextCommand draws a run of styled text.
type TextCommand struct {
	Bounds        RectF       `json:"bounds"`
	Text          string      `json:"text"`
	FontSize      float32     `json:"font_size"`
	Color         color.Color `json:"color"`
	Bold          bool        `json:"bold"`
	Italic        bool        `json:"italic"`
	Underline     bool        `json:"underline"`
	Strikethrough bool        `json:"strikethrough"`
}

// ImageCommand draws a decoded image within a bounding box.
type ImageCommand struct {
	Bounds RectF  `json:"bounds"`
	Src    string `json:"src"`
	Alt    string `json:"alt"`
}

// ClipCommand defines a clipping region.
type ClipCommand struct {
	Bounds RectF `json:"bounds"`
}

// TransformMatrix is a 2D affine transform matrix stored as 6 float32 values:
//
//	| A  C  E |
//	| B  D  F |
//	| 0  0  1 |
type TransformMatrix struct {
	A, B, C, D, E, F float32
}

// IsIdentity reports whether the matrix is the identity transform.
func (m TransformMatrix) IsIdentity() bool {
	return m.A == 1 && m.B == 0 && m.C == 0 && m.D == 1 && m.E == 0 && m.F == 0
}

// Mul returns the product m * n.
func (m TransformMatrix) Mul(n TransformMatrix) TransformMatrix {
	return TransformMatrix{
		A: m.A*n.A + m.C*n.B,
		B: m.B*n.A + m.D*n.B,
		C: m.A*n.C + m.C*n.D,
		D: m.B*n.C + m.D*n.D,
		E: m.A*n.E + m.C*n.F + m.E,
		F: m.B*n.E + m.D*n.F + m.F,
	}
}

// Inverse returns the inverse matrix, or nil if the matrix is singular.
func (m TransformMatrix) Inverse() *TransformMatrix {
	det := m.A*m.D - m.B*m.C
	if det == 0 {
		return nil
	}
	invDet := 1 / det
	return &TransformMatrix{
		A: m.D * invDet,
		B: -m.B * invDet,
		C: -m.C * invDet,
		D: m.A * invDet,
		E: (m.C*m.F - m.D*m.E) * invDet,
		F: (m.B*m.E - m.A*m.F) * invDet,
	}
}

// TranslateMatrix returns a translation matrix.
func TranslateMatrix(tx, ty float32) TransformMatrix {
	return TransformMatrix{A: 1, B: 0, C: 0, D: 1, E: tx, F: ty}
}

// ScaleMatrix returns a scale matrix.
func ScaleMatrix(sx, sy float32) TransformMatrix {
	return TransformMatrix{A: sx, B: 0, C: 0, D: sy, E: 0, F: 0}
}

// RotateMatrix returns a rotation matrix for angle radians.
func RotateMatrix(angle float32) TransformMatrix {
	cos := float32(math.Cos(float64(angle)))
	sin := float32(math.Sin(float64(angle)))
	return TransformMatrix{A: cos, B: sin, C: -sin, D: cos, E: 0, F: 0}
}

// MarshalJSON implements json.Marshaler for TransformMatrix.
func (m TransformMatrix) MarshalJSON() ([]byte, error) {
	return json.Marshal([6]float32{m.A, m.B, m.C, m.D, m.E, m.F})
}

// UnmarshalJSON implements json.Unmarshaler for TransformMatrix.
func (m *TransformMatrix) UnmarshalJSON(data []byte) error {
	var arr [6]float32
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	m.A, m.B, m.C, m.D, m.E, m.F = arr[0], arr[1], arr[2], arr[3], arr[4], arr[5]
	return nil
}

// TransformCommand applies an affine transform to its children.
type TransformCommand struct {
	Matrix TransformMatrix `json:"matrix"`
}

// OpacityCommand applies a uniform opacity to its children.
type OpacityCommand struct {
	Opacity float32 `json:"opacity"`
}

// StackingContextCommand begins a stacking context with a z-index.
type StackingContextCommand struct {
	ZIndex    int32 `json:"z_index"`
	Isolation bool  `json:"isolation"`
}

// DisplayCommand is a single backend-neutral paint command stored by value.
//
// The Kind field selects which data field is active. Only the data field
// matching Kind is meaningful; all others are zero-valued. This design avoids
// interface allocations in the hot display-list storage while keeping each
// command type independently testable and serializable.
type DisplayCommand struct {
	Kind            DisplayCommandKind     `json:"kind"`
	Rect            RectCommand            `json:"rect,omitempty"`
	Border          BorderCommand          `json:"border,omitempty"`
	Text            TextCommand            `json:"text,omitempty"`
	Image           ImageCommand           `json:"image,omitempty"`
	Clip            ClipCommand            `json:"clip,omitempty"`
	Transform       TransformCommand       `json:"transform,omitempty"`
	Opacity         OpacityCommand         `json:"opacity,omitempty"`
	StackingContext StackingContextCommand `json:"stacking_context,omitempty"`
}

// DisplayCommandList is a contiguous slice of DisplayCommand values.
// Unlike the legacy DisplayList which uses []*PaintCommand, this stores
// commands by value to reduce pointer density and GC pressure.
type DisplayCommandList struct {
	cmds []DisplayCommand
}

// NewDisplayCommandList creates an empty display command list.
func NewDisplayCommandList() *DisplayCommandList {
	return &DisplayCommandList{cmds: make([]DisplayCommand, 0)}
}

// Add appends a command to the list.
func (dl *DisplayCommandList) Add(cmd DisplayCommand) {
	dl.cmds = append(dl.cmds, cmd)
}

// Clear removes all commands.
func (dl *DisplayCommandList) Clear() {
	dl.cmds = dl.cmds[:0]
}

// Len returns the number of commands.
func (dl *DisplayCommandList) Len() int {
	return len(dl.cmds)
}

// At returns the command at index i. Panics if out of range.
func (dl *DisplayCommandList) At(i int) DisplayCommand {
	return dl.cmds[i]
}

// Commands returns the underlying slice for iteration.
func (dl *DisplayCommandList) Commands() []DisplayCommand {
	return dl.cmds
}

// --- Color JSON helpers ---

// colorJSON is a JSON-friendly representation of color.Color.
type colorJSON struct {
	R uint8 `json:"r"`
	G uint8 `json:"g"`
	B uint8 `json:"b"`
	A uint8 `json:"a"`
}

// --- Custom JSON for types with color.Color fields ---

// RectCommandJSON is the JSON representation of RectCommand.
type rectCommandJSON struct {
	Bounds RectF      `json:"bounds"`
	Color  *colorJSON `json:"color"`
}

// MarshalJSON implements json.Marshaler for RectCommand.
func (c RectCommand) MarshalJSON() ([]byte, error) {
	var cj *colorJSON
	if c.Color != nil {
		r, g, b, a := c.Color.RGBA()
		cj = &colorJSON{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
	}
	return json.Marshal(rectCommandJSON{Bounds: c.Bounds, Color: cj})
}

// UnmarshalJSON implements json.Unmarshaler for RectCommand.
func (c *RectCommand) UnmarshalJSON(data []byte) error {
	var j rectCommandJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	c.Bounds = j.Bounds
	if j.Color != nil {
		c.Color = color.RGBA{R: j.Color.R, G: j.Color.G, B: j.Color.B, A: j.Color.A}
	}
	return nil
}

// MarshalJSON implements json.Marshaler for BorderSide.
type borderSideJSON struct {
	Width float32     `json:"width"`
	Color *colorJSON  `json:"color"`
	Style BorderStyle `json:"style"`
}

func (s BorderSide) MarshalJSON() ([]byte, error) {
	var cj *colorJSON
	if s.Color != nil {
		r, g, b, a := s.Color.RGBA()
		cj = &colorJSON{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
	}
	return json.Marshal(borderSideJSON{Width: s.Width, Color: cj, Style: s.Style})
}

func (s *BorderSide) UnmarshalJSON(data []byte) error {
	var j borderSideJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	s.Width = j.Width
	s.Style = j.Style
	if j.Color != nil {
		s.Color = color.RGBA{R: j.Color.R, G: j.Color.G, B: j.Color.B, A: j.Color.A}
	}
	return nil
}

// MarshalJSON implements json.Marshaler for TextCommand.
type textCommandJSON struct {
	Bounds        RectF      `json:"bounds"`
	Text          string     `json:"text"`
	FontSize      float32    `json:"font_size"`
	Color         *colorJSON `json:"color"`
	Bold          bool       `json:"bold"`
	Italic        bool       `json:"italic"`
	Underline     bool       `json:"underline"`
	Strikethrough bool       `json:"strikethrough"`
}

func (c TextCommand) MarshalJSON() ([]byte, error) {
	var cj *colorJSON
	if c.Color != nil {
		r, g, b, a := c.Color.RGBA()
		cj = &colorJSON{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
	}
	return json.Marshal(textCommandJSON{
		Bounds: c.Bounds, Text: c.Text, FontSize: c.FontSize, Color: cj,
		Bold: c.Bold, Italic: c.Italic, Underline: c.Underline, Strikethrough: c.Strikethrough,
	})
}

func (c *TextCommand) UnmarshalJSON(data []byte) error {
	var j textCommandJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	c.Bounds = j.Bounds
	c.Text = j.Text
	c.FontSize = j.FontSize
	c.Bold = j.Bold
	c.Italic = j.Italic
	c.Underline = j.Underline
	c.Strikethrough = j.Strikethrough
	if j.Color != nil {
		c.Color = color.RGBA{R: j.Color.R, G: j.Color.G, B: j.Color.B, A: j.Color.A}
	}
	return nil
}

// --- DisplayCommandList JSON ---

// MarshalJSON implements json.Marshaler for DisplayCommandList.
func (dl *DisplayCommandList) MarshalJSON() ([]byte, error) {
	return json.Marshal(dl.cmds)
}

// UnmarshalJSON implements json.Unmarshaler for DisplayCommandList.
func (dl *DisplayCommandList) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &dl.cmds)
}
