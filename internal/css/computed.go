package css

import (
	"hash/fnv"
	"image/color"
	"math"
	"strconv"
	"sync"
)

// InheritedStyle contains CSS properties that propagate from parent to child
// per the CSS specification. This is embedded in ComputedStyle.
//
// M3.3: Separating inherited from non-inherited properties enables efficient
// style inheritance without copying non-inherited fields, and allows
// deduplication of identical inherited style groups via fingerprinting.
type InheritedStyle struct {
	// Color and visibility (inherited)
	Color      color.Color
	Visibility string // "visible", "hidden", "collapse"
	Opacity    float32

	// Font properties (inherited)
	FontSize   float32
	FontWeight string
	FontFamily string
	FontStyle  string // "normal", "italic", "oblique"

	// Text properties (inherited)
	TextAlign      string // "left", "right", "center", "justify"
	TextTransform  string // "none", "uppercase", "lowercase", "capitalize"
	TextDecoration string // "none", "underline", "line-through", "overline"
	TextIndent     string
	LetterSpacing  string
	WordSpacing    string
	WhiteSpace     string // "normal", "nowrap", "pre", "pre-wrap", "pre-line"
	VerticalAlign  string // "baseline", "top", "middle", "bottom"
	LineHeight     float32

	// List properties (inherited)
	ListStyleType     string // "disc", "circle", "square", "decimal", "none"
	ListStylePosition string // "inside", "outside"

	// Table properties (inherited)
	BorderCollapse string // "collapse", "separate"
	BorderSpacing  string
}

// NonInheritedStyle contains CSS properties that do NOT propagate from parent
// to child. Each element computes these independently.
type NonInheritedStyle struct {
	// Display and positioning (non-inherited)
	Display   string // "block", "inline", "flex", "grid", "none", etc.
	Position  string // "static", "relative", "absolute", "fixed", "sticky"
	Top       string
	Right     string
	Bottom    string
	Left      string
	ZIndex    int
	Float     string // "none", "left", "right"
	Clear     string // "none", "left", "right", "both"
	Overflow  string // "visible", "hidden", "scroll", "auto"
	OverflowX string
	OverflowY string

	// Box model (non-inherited)
	Width     string
	Height    string
	MinWidth  string
	MaxWidth  string
	MinHeight string
	MaxHeight string

	// Margin (non-inherited)
	MarginTop    string
	MarginRight  string
	MarginBottom string
	MarginLeft   string

	// Padding (non-inherited)
	PaddingTop    string
	PaddingRight  string
	PaddingBottom string
	PaddingLeft   string

	// Border (non-inherited)
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

	// Background (non-inherited)
	BackgroundColor    color.Color
	BackgroundImage    string
	BackgroundRepeat   string
	BackgroundPosition string
	BackgroundSize     string

	// Flexbox container (non-inherited)
	FlexDirection  string
	FlexWrap       string
	JustifyContent string
	AlignItems     string
	AlignContent   string
	Gap            string
	RowGap         string
	ColumnGap      string

	// Flexbox item (non-inherited)
	FlexGrow   float32
	FlexShrink float32
	FlexBasis  string
	AlignSelf  string
	Order      int

	// Grid (non-inherited)
	GridTemplateColumns string
	GridTemplateRows    string
	GridColumnStart     string
	GridColumnEnd       string
	GridRowStart        string
	GridRowEnd          string

	// Visual (non-inherited)
	BorderRadius string
	BoxSizing    string // "content-box", "border-box"
	Cursor       string
}

// ComputedStyle is the fully resolved style for an element. It separates
// inherited properties (which propagate to children) from non-inherited
// properties (which are computed per-element).
//
// M3.3: This replaces the per-element property map with typed fields,
// reducing allocations and enabling efficient inheritance and deduplication.
type ComputedStyle struct {
	Inherited    InheritedStyle
	NonInherited NonInheritedStyle
}

// Fingerprint returns a uint64 hash of the InheritedStyle for deduplication.
// Two InheritedStyle values with identical fields produce the same fingerprint.
func (s *InheritedStyle) Fingerprint() uint64 {
	h := fnv.New64a()
	// Write color components
	if s.Color != nil {
		r, g, b, a := s.Color.RGBA()
		var buf [16]byte
		buf[0] = byte(r >> 8)
		buf[1] = byte(r)
		buf[2] = byte(g >> 8)
		buf[3] = byte(g)
		buf[4] = byte(b >> 8)
		buf[5] = byte(b)
		buf[6] = byte(a >> 8)
		buf[7] = byte(a)
		h.Write(buf[:8])
	}
	// Write float fields as bits
	var fbuf [8]byte
	putFloat32(fbuf[:4], s.FontSize)
	putFloat32(fbuf[4:], s.LineHeight)
	h.Write(fbuf[:8])

	putFloat32(fbuf[:4], s.Opacity)
	h.Write(fbuf[:4])

	// Write string fields
	h.Write([]byte(s.Visibility))
	h.Write([]byte{0}) // separator
	h.Write([]byte(s.FontWeight))
	h.Write([]byte{0})
	h.Write([]byte(s.FontFamily))
	h.Write([]byte{0})
	h.Write([]byte(s.FontStyle))
	h.Write([]byte{0})
	h.Write([]byte(s.TextAlign))
	h.Write([]byte{0})
	h.Write([]byte(s.TextTransform))
	h.Write([]byte{0})
	h.Write([]byte(s.TextDecoration))
	h.Write([]byte{0})
	h.Write([]byte(s.TextIndent))
	h.Write([]byte{0})
	h.Write([]byte(s.LetterSpacing))
	h.Write([]byte{0})
	h.Write([]byte(s.WordSpacing))
	h.Write([]byte{0})
	h.Write([]byte(s.WhiteSpace))
	h.Write([]byte{0})
	h.Write([]byte(s.VerticalAlign))
	h.Write([]byte{0})
	h.Write([]byte(s.ListStyleType))
	h.Write([]byte{0})
	h.Write([]byte(s.ListStylePosition))
	h.Write([]byte{0})
	h.Write([]byte(s.BorderCollapse))
	h.Write([]byte{0})
	h.Write([]byte(s.BorderSpacing))

	return h.Sum64()
}

// Equal reports whether two InheritedStyle values are identical.
func (s *InheritedStyle) Equal(other *InheritedStyle) bool {
	if s == other {
		return true
	}
	if other == nil {
		return false
	}

	// Compare colors
	if !colorsEqual(s.Color, other.Color) {
		return false
	}

	// Compare floats
	if s.FontSize != other.FontSize || s.LineHeight != other.LineHeight || s.Opacity != other.Opacity {
		return false
	}

	// Compare strings
	return s.Visibility == other.Visibility &&
		s.FontWeight == other.FontWeight &&
		s.FontFamily == other.FontFamily &&
		s.FontStyle == other.FontStyle &&
		s.TextAlign == other.TextAlign &&
		s.TextTransform == other.TextTransform &&
		s.TextDecoration == other.TextDecoration &&
		s.TextIndent == other.TextIndent &&
		s.LetterSpacing == other.LetterSpacing &&
		s.WordSpacing == other.WordSpacing &&
		s.WhiteSpace == other.WhiteSpace &&
		s.VerticalAlign == other.VerticalAlign &&
		s.ListStyleType == other.ListStyleType &&
		s.ListStylePosition == other.ListStylePosition &&
		s.BorderCollapse == other.BorderCollapse &&
		s.BorderSpacing == other.BorderSpacing
}

// putFloat32 writes float32 bits to a byte slice.
func putFloat32(b []byte, f float32) {
	bits := math.Float32bits(f)
	b[0] = byte(bits >> 24)
	b[1] = byte(bits >> 16)
	b[2] = byte(bits >> 8)
	b[3] = byte(bits)
}

// colorsEqual compares two color.Color values.
func colorsEqual(a, b color.Color) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// DefaultInheritedStyle returns an InheritedStyle with CSS initial values.
func DefaultInheritedStyle() InheritedStyle {
	return InheritedStyle{
		Color:             color.Black,
		Visibility:        "visible",
		Opacity:           1.0,
		FontSize:          16.0,
		FontWeight:        "normal",
		FontFamily:        "",
		FontStyle:         "normal",
		TextAlign:         "left",
		TextTransform:     "none",
		TextDecoration:    "none",
		TextIndent:        "0",
		LetterSpacing:     "normal",
		WordSpacing:       "normal",
		WhiteSpace:        "normal",
		VerticalAlign:     "baseline",
		LineHeight:        0, // 0 means "normal" in CSS
		ListStyleType:     "disc",
		ListStylePosition: "outside",
		BorderCollapse:    "separate",
		BorderSpacing:     "0",
	}
}

// DefaultComputedStyle returns a ComputedStyle with CSS initial values.
func DefaultComputedStyle() ComputedStyle {
	return ComputedStyle{
		Inherited: DefaultInheritedStyle(),
		NonInherited: NonInheritedStyle{
			Display:    "inline",
			Position:   "static",
			Float:      "none",
			Clear:      "none",
			Overflow:   "visible",
			OverflowX:  "visible",
			OverflowY:  "visible",
			BoxSizing:  "content-box",
			FlexShrink: 1.0,
		},
	}
}

// InheritFrom copies inherited properties from parent into this NonInheritedStyle's
// sibling InheritedStyle. This is used to propagate inherited values from parent to child.
func (cs *ComputedStyle) InheritFrom(parent *InheritedStyle) {
	if parent == nil {
		return
	}
	cs.Inherited = *parent
}

// Clone creates a deep copy of the ComputedStyle.
func (cs *ComputedStyle) Clone() ComputedStyle {
	clone := *cs
	// Color fields are interfaces, need explicit copy
	if cs.Inherited.Color != nil {
		r, g, b, a := cs.Inherited.Color.RGBA()
		clone.Inherited.Color = color.RGBA{
			R: uint8(r >> 8),
			G: uint8(g >> 8),
			B: uint8(b >> 8),
			A: uint8(a >> 8),
		}
	}
	if cs.NonInherited.BackgroundColor != nil {
		r, g, b, a := cs.NonInherited.BackgroundColor.RGBA()
		clone.NonInherited.BackgroundColor = color.RGBA{
			R: uint8(r >> 8),
			G: uint8(g >> 8),
			B: uint8(b >> 8),
			A: uint8(a >> 8),
		}
	}
	if cs.NonInherited.BorderTopColor != nil {
		r, g, b, a := cs.NonInherited.BorderTopColor.RGBA()
		clone.NonInherited.BorderTopColor = color.RGBA{
			R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8),
		}
	}
	if cs.NonInherited.BorderRightColor != nil {
		r, g, b, a := cs.NonInherited.BorderRightColor.RGBA()
		clone.NonInherited.BorderRightColor = color.RGBA{
			R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8),
		}
	}
	if cs.NonInherited.BorderBottomColor != nil {
		r, g, b, a := cs.NonInherited.BorderBottomColor.RGBA()
		clone.NonInherited.BorderBottomColor = color.RGBA{
			R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8),
		}
	}
	if cs.NonInherited.BorderLeftColor != nil {
		r, g, b, a := cs.NonInherited.BorderLeftColor.RGBA()
		clone.NonInherited.BorderLeftColor = color.RGBA{
			R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8),
		}
	}
	return clone
}

// inheritedPropertySet maps property names to whether they are inherited.
var inheritedPropertySet = map[string]bool{
	"color":               true,
	"visibility":          true,
	"opacity":             true,
	"font-size":           true,
	"font-weight":         true,
	"font-family":         true,
	"font-style":          true,
	"text-align":          true,
	"text-transform":      true,
	"text-decoration":     true,
	"text-indent":         true,
	"letter-spacing":      true,
	"word-spacing":        true,
	"white-space":         true,
	"vertical-align":      true,
	"line-height":         true,
	"list-style-type":     true,
	"list-style-position": true,
	"border-collapse":     true,
	"border-spacing":      true,
}

// IsInheritedProperty returns true if the CSS property is inherited per spec.
func IsInheritedProperty(property string) bool {
	return inheritedPropertySet[property]
}

// ApplyDeclarationsToInherited applies matching declarations to an InheritedStyle.
// Only inherited properties are applied; others are ignored.
func ApplyDeclarationsToInherited(s *InheritedStyle, decls []Declaration) {
	for i := range decls {
		d := &decls[i]
		if d.Value == "" {
			continue
		}
		if !IsInheritedProperty(d.Property) {
			continue
		}
		applyInheritedDecl(s, d)
	}
}

// ApplyDeclarationsToNonInherited applies matching declarations to a NonInheritedStyle.
// Only non-inherited properties are applied; others are ignored.
func ApplyDeclarationsToNonInherited(s *NonInheritedStyle, decls []Declaration) {
	for i := range decls {
		d := &decls[i]
		if d.Value == "" {
			continue
		}
		if IsInheritedProperty(d.Property) {
			continue
		}
		applyNonInheritedDecl(s, d)
	}
}

// applyInheritedDecl applies a single declaration to an InheritedStyle.
func applyInheritedDecl(s *InheritedStyle, d *Declaration) {
	switch d.Property {
	case "color":
		s.Color = parseColor(d.Value)
	case "visibility":
		s.Visibility = d.Value
	case "opacity":
		if v, err := strconv.ParseFloat(d.Value, 32); err == nil {
			s.Opacity = float32(v)
		}
	case "font-size":
		if v, err := parseLength(d.Value); err == nil {
			s.FontSize = v
		}
	case "font-weight":
		s.FontWeight = d.Value
	case "font-family":
		s.FontFamily = d.Value
	case "font-style":
		s.FontStyle = d.Value
	case "text-align":
		s.TextAlign = d.Value
	case "text-transform":
		s.TextTransform = d.Value
	case "text-decoration":
		s.TextDecoration = d.Value
	case "text-indent":
		s.TextIndent = d.Value
	case "letter-spacing":
		s.LetterSpacing = d.Value
	case "word-spacing":
		s.WordSpacing = d.Value
	case "white-space":
		s.WhiteSpace = d.Value
	case "vertical-align":
		s.VerticalAlign = d.Value
	case "line-height":
		if v, err := parseLength(d.Value); err == nil {
			s.LineHeight = v
		}
	case "list-style-type":
		s.ListStyleType = d.Value
	case "list-style-position":
		s.ListStylePosition = d.Value
	case "border-collapse":
		s.BorderCollapse = d.Value
	case "border-spacing":
		s.BorderSpacing = d.Value
	}
}

// applyNonInheritedDecl applies a single declaration to a NonInheritedStyle.
func applyNonInheritedDecl(s *NonInheritedStyle, d *Declaration) {
	switch d.Property {
	case "display":
		s.Display = d.Value
	case "position":
		s.Position = d.Value
	case "top":
		s.Top = d.Value
	case "right":
		s.Right = d.Value
	case "bottom":
		s.Bottom = d.Value
	case "left":
		s.Left = d.Value
	case "z-index":
		if v, err := strconv.Atoi(d.Value); err == nil {
			s.ZIndex = v
		}
	case "float":
		s.Float = d.Value
	case "clear":
		s.Clear = d.Value
	case "overflow":
		s.Overflow = d.Value
	case "overflow-x":
		s.OverflowX = d.Value
	case "overflow-y":
		s.OverflowY = d.Value
	case "width":
		s.Width = d.Value
	case "height":
		s.Height = d.Value
	case "min-width":
		s.MinWidth = d.Value
	case "max-width":
		s.MaxWidth = d.Value
	case "min-height":
		s.MinHeight = d.Value
	case "max-height":
		s.MaxHeight = d.Value
	case "margin-top":
		s.MarginTop = d.Value
	case "margin-right":
		s.MarginRight = d.Value
	case "margin-bottom":
		s.MarginBottom = d.Value
	case "margin-left":
		s.MarginLeft = d.Value
	case "padding-top":
		s.PaddingTop = d.Value
	case "padding-right":
		s.PaddingRight = d.Value
	case "padding-bottom":
		s.PaddingBottom = d.Value
	case "padding-left":
		s.PaddingLeft = d.Value
	case "border-top-width":
		s.BorderTopWidth = d.Value
	case "border-right-width":
		s.BorderRightWidth = d.Value
	case "border-bottom-width":
		s.BorderBottomWidth = d.Value
	case "border-left-width":
		s.BorderLeftWidth = d.Value
	case "border-top-style":
		s.BorderTopStyle = d.Value
	case "border-right-style":
		s.BorderRightStyle = d.Value
	case "border-bottom-style":
		s.BorderBottomStyle = d.Value
	case "border-left-style":
		s.BorderLeftStyle = d.Value
	case "border-top-color":
		s.BorderTopColor = parseColor(d.Value)
	case "border-right-color":
		s.BorderRightColor = parseColor(d.Value)
	case "border-bottom-color":
		s.BorderBottomColor = parseColor(d.Value)
	case "border-left-color":
		s.BorderLeftColor = parseColor(d.Value)
	case "background-color":
		s.BackgroundColor = parseColor(d.Value)
	case "background-image":
		s.BackgroundImage = d.Value
	case "background-repeat":
		s.BackgroundRepeat = d.Value
	case "background-position":
		s.BackgroundPosition = d.Value
	case "background-size":
		s.BackgroundSize = d.Value
	case "flex-direction":
		s.FlexDirection = d.Value
	case "flex-wrap":
		s.FlexWrap = d.Value
	case "justify-content":
		s.JustifyContent = d.Value
	case "align-items":
		s.AlignItems = d.Value
	case "align-content":
		s.AlignContent = d.Value
	case "gap":
		s.Gap = d.Value
	case "row-gap":
		s.RowGap = d.Value
	case "column-gap":
		s.ColumnGap = d.Value
	case "flex-grow":
		if v, err := strconv.ParseFloat(d.Value, 32); err == nil {
			s.FlexGrow = float32(v)
		}
	case "flex-shrink":
		if v, err := strconv.ParseFloat(d.Value, 32); err == nil {
			s.FlexShrink = float32(v)
		}
	case "flex-basis":
		s.FlexBasis = d.Value
	case "align-self":
		s.AlignSelf = d.Value
	case "order":
		if v, err := strconv.Atoi(d.Value); err == nil {
			s.Order = v
		}
	case "grid-template-columns":
		s.GridTemplateColumns = d.Value
	case "grid-template-rows":
		s.GridTemplateRows = d.Value
	case "grid-column-start":
		s.GridColumnStart = d.Value
	case "grid-column-end":
		s.GridColumnEnd = d.Value
	case "grid-row-start":
		s.GridRowStart = d.Value
	case "grid-row-end":
		s.GridRowEnd = d.Value
	case "border-radius":
		s.BorderRadius = d.Value
	case "box-sizing":
		s.BoxSizing = d.Value
	case "cursor":
		s.Cursor = d.Value
	}
}

// parseColor parses a CSS color value into a color.Color.
// Supports named colors and #hex notation.
func parseColor(value string) color.Color {
	switch value {
	case "black":
		return color.Black
	case "white":
		return color.White
	case "red":
		return color.RGBA{R: 255, A: 255}
	case "green":
		return color.RGBA{G: 128, A: 255}
	case "blue":
		return color.RGBA{B: 255, A: 255}
	case "transparent":
		return color.RGBA{}
	}

	// Handle #hex
	if len(value) > 0 && value[0] == '#' {
		return parseHexColor(value)
	}

	return color.Black
}

// parseHexColor parses a hex color string like "#ff0000" or "#f00".
func parseHexColor(s string) color.Color {
	if len(s) < 2 || s[0] != '#' {
		return color.Black
	}
	hex := s[1:]

	var r, g, b, a uint8 = 0, 0, 0, 255

	switch len(hex) {
	case 3: // #rgb
		r = hexChar(hex[0]) * 17
		g = hexChar(hex[1]) * 17
		b = hexChar(hex[2]) * 17
	case 4: // #rgba
		r = hexChar(hex[0]) * 17
		g = hexChar(hex[1]) * 17
		b = hexChar(hex[2]) * 17
		a = hexChar(hex[3]) * 17
	case 6: // #rrggbb
		r = hexChar(hex[0])<<4 | hexChar(hex[1])
		g = hexChar(hex[2])<<4 | hexChar(hex[3])
		b = hexChar(hex[4])<<4 | hexChar(hex[5])
	case 8: // #rrggbbaa
		r = hexChar(hex[0])<<4 | hexChar(hex[1])
		g = hexChar(hex[2])<<4 | hexChar(hex[3])
		b = hexChar(hex[4])<<4 | hexChar(hex[5])
		a = hexChar(hex[6])<<4 | hexChar(hex[7])
	}

	return color.RGBA{R: r, G: g, B: b, A: a}
}

// hexChar converts a hex character to its numeric value.
func hexChar(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// parseLength parses a CSS length value (e.g., "16px", "1.5em") and returns
// the numeric value in pixels. For simplicity, em/rem units are treated as
// multipliers of 16px.
func parseLength(value string) (float32, error) {
	if value == "" {
		return 0, strconv.ErrSyntax
	}

	// Find where the numeric part ends
	numEnd := 0
	for numEnd < len(value) && (value[numEnd] >= '0' && value[numEnd] <= '9' || value[numEnd] == '.' || value[numEnd] == '-') {
		numEnd++
	}

	if numEnd == 0 {
		return 0, strconv.ErrSyntax
	}

	num, err := strconv.ParseFloat(value[:numEnd], 32)
	if err != nil {
		return 0, err
	}

	unit := value[numEnd:]
	switch unit {
	case "px", "":
		return float32(num), nil
	case "em", "rem":
		return float32(num) * 16.0, nil
	case "pt":
		return float32(num) * 1.333, nil
	case "%":
		return float32(num), nil // percentage kept as-is for now
	}

	return float32(num), nil
}

// --- StylePool: bounded deduplication of InheritedStyle ---

// StylePoolStats contains pool metrics for diagnostics.
type StylePoolStats struct {
	Entries    int // Current number of unique entries
	DedupCount int // Number of times Intern returned an existing entry
}

// stylePoolEntry is one entry in the style pool LRU.
type stylePoolEntry struct {
	fp    uint64
	style InheritedStyle
	prev  *stylePoolEntry
	next  *stylePoolEntry
}

// StylePool is a bounded cache for deduplicating identical InheritedStyle values.
// When an identical style is interned, the existing reference is returned,
// avoiding per-element allocation of inherited property groups.
//
// The pool uses LRU eviction when it reaches its capacity limit.
// Safe for concurrent use.
type StylePool struct {
	mu       sync.Mutex
	byFP     map[uint64]*stylePoolEntry
	head     *stylePoolEntry // most recently used
	tail     *stylePoolEntry // least recently used
	count    int
	capacity int
	dedup    int
}

// NewStylePool creates a StylePool with default capacity (1024).
func NewStylePool() *StylePool {
	return NewStylePoolWithLimit(1024)
}

// NewStylePoolWithLimit creates a StylePool with the given capacity.
func NewStylePoolWithLimit(capacity int) *StylePool {
	if capacity <= 0 {
		capacity = 1024
	}
	return &StylePool{
		byFP:     make(map[uint64]*stylePoolEntry, capacity),
		capacity: capacity,
	}
}

// Intern returns a pointer to the deduplicated InheritedStyle.
// If an identical style already exists in the pool, it returns the existing pointer.
// Otherwise, it inserts the new style and returns a pointer to the pooled copy.
func (p *StylePool) Intern(s *InheritedStyle) *InheritedStyle {
	if s == nil {
		return nil
	}

	fp := s.Fingerprint()

	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, ok := p.byFP[fp]; ok {
		// Verify actual equality (fingerprint collision check)
		if entry.style.Equal(s) {
			p.dedup++
			p.moveToFront(entry)
			return &entry.style
		}
		// Fingerprint collision — treat as new entry with different content
		// Use combined key to handle rare collisions
	}

	// Evict if at capacity
	if p.count >= p.capacity {
		p.evictTail()
	}

	// Insert new entry
	entry := &stylePoolEntry{
		fp:    fp,
		style: *s,
	}
	p.byFP[fp] = entry
	p.pushFront(entry)
	p.count++

	return &entry.style
}

// Stats returns current pool statistics.
func (p *StylePool) Stats() StylePoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return StylePoolStats{
		Entries:    p.count,
		DedupCount: p.dedup,
	}
}

// Reset clears the pool.
func (p *StylePool) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.byFP = make(map[uint64]*stylePoolEntry, p.capacity)
	p.head = nil
	p.tail = nil
	p.count = 0
	p.dedup = 0
}

// pushFront adds entry to the front of the LRU list.
func (p *StylePool) pushFront(entry *stylePoolEntry) {
	entry.prev = nil
	entry.next = p.head
	if p.head != nil {
		p.head.prev = entry
	}
	p.head = entry
	if p.tail == nil {
		p.tail = entry
	}
}

// removeEntry removes an entry from the LRU list.
func (p *StylePool) removeEntry(entry *stylePoolEntry) {
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		p.head = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		p.tail = entry.prev
	}
	entry.prev = nil
	entry.next = nil
}

// moveToFront moves an existing entry to the front.
func (p *StylePool) moveToFront(entry *stylePoolEntry) {
	if p.head == entry {
		return
	}
	p.removeEntry(entry)
	p.pushFront(entry)
}

// evictTail removes the least recently used entry. Must be called under lock.
func (p *StylePool) evictTail() {
	if p.tail == nil {
		return
	}
	entry := p.tail
	delete(p.byFP, entry.fp)
	p.removeEntry(entry)
	p.count--
}
