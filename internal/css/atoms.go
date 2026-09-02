package css

// Enumerated property atoms for hot-path CSS style fields.
// Conversions happen at parse boundaries (applyDeclaration); layout code
// compares uint8 values instead of strings.

// ---------------------------------------------------------------------------
// DisplayAtom
// ---------------------------------------------------------------------------

type DisplayAtom uint8

const (
	DisplayAtomUnset DisplayAtom = iota // zero value: not yet resolved
	DisplayAtomBlock
	DisplayAtomNone
	DisplayAtomInline
	DisplayAtomInlineBlock
	DisplayAtomFlex
	DisplayAtomGrid
	DisplayAtomFlowRoot
	DisplayAtomTable
	DisplayAtomTableRow
	DisplayAtomTableCell
	DisplayAtomListItem
	DisplayAtomTableHeaderGroup
	DisplayAtomTableRowGroup
	DisplayAtomTableFooterGroup
)

func DisplayAtomFromString(s string) DisplayAtom {
	switch s {
	case "none":
		return DisplayAtomNone
	case "block":
		return DisplayAtomBlock
	case "inline":
		return DisplayAtomInline
	case "inline-block":
		return DisplayAtomInlineBlock
	case "flex":
		return DisplayAtomFlex
	case "grid":
		return DisplayAtomGrid
	case "flow-root":
		return DisplayAtomFlowRoot
	case "table":
		return DisplayAtomTable
	case "table-row":
		return DisplayAtomTableRow
	case "table-cell":
		return DisplayAtomTableCell
	case "list-item":
		return DisplayAtomListItem
	case "table-header-group":
		return DisplayAtomTableHeaderGroup
	case "table-row-group":
		return DisplayAtomTableRowGroup
	case "table-footer-group":
		return DisplayAtomTableFooterGroup
	default:
		return DisplayAtomBlock
	}
}

func (d DisplayAtom) String() string {
	switch d {
	case DisplayAtomUnset:
		return ""
	case DisplayAtomNone:
		return "none"
	case DisplayAtomBlock:
		return "block"
	case DisplayAtomInline:
		return "inline"
	case DisplayAtomInlineBlock:
		return "inline-block"
	case DisplayAtomFlex:
		return "flex"
	case DisplayAtomGrid:
		return "grid"
	case DisplayAtomFlowRoot:
		return "flow-root"
	case DisplayAtomTable:
		return "table"
	case DisplayAtomTableRow:
		return "table-row"
	case DisplayAtomTableCell:
		return "table-cell"
	case DisplayAtomListItem:
		return "list-item"
	case DisplayAtomTableHeaderGroup:
		return "table-header-group"
	case DisplayAtomTableRowGroup:
		return "table-row-group"
	case DisplayAtomTableFooterGroup:
		return "table-footer-group"
	default:
		return "block"
	}
}

// ---------------------------------------------------------------------------
// PositionAtom
// ---------------------------------------------------------------------------

type PositionAtom uint8

const (
	PositionAtomStatic PositionAtom = iota
	PositionAtomRelative
	PositionAtomAbsolute
	PositionAtomFixed
	PositionAtomSticky
)

func PositionAtomFromString(s string) PositionAtom {
	switch s {
	case "relative":
		return PositionAtomRelative
	case "absolute":
		return PositionAtomAbsolute
	case "fixed":
		return PositionAtomFixed
	case "sticky":
		return PositionAtomSticky
	default:
		return PositionAtomStatic
	}
}

func (p PositionAtom) String() string {
	switch p {
	case PositionAtomStatic:
		return "static"
	case PositionAtomRelative:
		return "relative"
	case PositionAtomAbsolute:
		return "absolute"
	case PositionAtomFixed:
		return "fixed"
	case PositionAtomSticky:
		return "sticky"
	default:
		return "static"
	}
}

// ---------------------------------------------------------------------------
// FloatAtom
// ---------------------------------------------------------------------------

type FloatAtom uint8

const (
	FloatAtomNone FloatAtom = iota
	FloatAtomLeft
	FloatAtomRight
)

func FloatAtomFromString(s string) FloatAtom {
	switch s {
	case "left":
		return FloatAtomLeft
	case "right":
		return FloatAtomRight
	default:
		return FloatAtomNone
	}
}

func (f FloatAtom) String() string {
	switch f {
	case FloatAtomNone:
		return "none"
	case FloatAtomLeft:
		return "left"
	case FloatAtomRight:
		return "right"
	default:
		return "none"
	}
}

// ---------------------------------------------------------------------------
// VisibilityAtom
// ---------------------------------------------------------------------------

type VisibilityAtom uint8

const (
	VisibilityAtomVisible VisibilityAtom = iota
	VisibilityAtomHidden
	VisibilityAtomCollapse
)

func VisibilityAtomFromString(s string) VisibilityAtom {
	switch s {
	case "hidden":
		return VisibilityAtomHidden
	case "collapse":
		return VisibilityAtomCollapse
	default:
		return VisibilityAtomVisible
	}
}

func (v VisibilityAtom) String() string {
	switch v {
	case VisibilityAtomVisible:
		return "visible"
	case VisibilityAtomHidden:
		return "hidden"
	case VisibilityAtomCollapse:
		return "collapse"
	default:
		return "visible"
	}
}

// ---------------------------------------------------------------------------
// FontStyleAtom
// ---------------------------------------------------------------------------

type FontStyleAtom uint8

const (
	FontStyleAtomNormal FontStyleAtom = iota
	FontStyleAtomItalic
)

func FontStyleAtomFromString(s string) FontStyleAtom {
	switch s {
	case "italic", "oblique":
		return FontStyleAtomItalic
	default:
		return FontStyleAtomNormal
	}
}

func (f FontStyleAtom) String() string {
	switch f {
	case FontStyleAtomNormal:
		return "normal"
	case FontStyleAtomItalic:
		return "italic"
	default:
		return "normal"
	}
}

// ---------------------------------------------------------------------------
// TextAlignAtom
// ---------------------------------------------------------------------------

type TextAlignAtom uint8

const (
	TextAlignAtomStart TextAlignAtom = iota
	TextAlignAtomLeft
	TextAlignAtomRight
	TextAlignAtomCenter
	TextAlignAtomJustify
	TextAlignAtomEnd
)

func TextAlignAtomFromString(s string) TextAlignAtom {
	switch s {
	case "left":
		return TextAlignAtomLeft
	case "right":
		return TextAlignAtomRight
	case "center":
		return TextAlignAtomCenter
	case "justify":
		return TextAlignAtomJustify
	case "start":
		return TextAlignAtomStart
	case "end":
		return TextAlignAtomEnd
	default:
		return TextAlignAtomStart
	}
}

func (t TextAlignAtom) String() string {
	switch t {
	case TextAlignAtomStart:
		return "start"
	case TextAlignAtomLeft:
		return "left"
	case TextAlignAtomRight:
		return "right"
	case TextAlignAtomCenter:
		return "center"
	case TextAlignAtomJustify:
		return "justify"
	case TextAlignAtomEnd:
		return "end"
	default:
		return "start"
	}
}

// ---------------------------------------------------------------------------
// TextDecorationAtom
// ---------------------------------------------------------------------------

type TextDecorationAtom uint8

const (
	TextDecorationAtomNone TextDecorationAtom = iota
	TextDecorationAtomUnderline
	TextDecorationAtomLineThrough
	TextDecorationAtomOverline
)

func TextDecorationAtomFromString(s string) TextDecorationAtom {
	switch s {
	case "underline":
		return TextDecorationAtomUnderline
	case "line-through":
		return TextDecorationAtomLineThrough
	case "overline":
		return TextDecorationAtomOverline
	default:
		return TextDecorationAtomNone
	}
}

func (t TextDecorationAtom) String() string {
	switch t {
	case TextDecorationAtomNone:
		return "none"
	case TextDecorationAtomUnderline:
		return "underline"
	case TextDecorationAtomLineThrough:
		return "line-through"
	case TextDecorationAtomOverline:
		return "overline"
	default:
		return "none"
	}
}

// ---------------------------------------------------------------------------
// TextTransformAtom
// ---------------------------------------------------------------------------

type TextTransformAtom uint8

const (
	TextTransformAtomNone TextTransformAtom = iota
	TextTransformAtomUppercase
	TextTransformAtomLowercase
	TextTransformAtomCapitalize
)

func TextTransformAtomFromString(s string) TextTransformAtom {
	switch s {
	case "uppercase":
		return TextTransformAtomUppercase
	case "lowercase":
		return TextTransformAtomLowercase
	case "capitalize":
		return TextTransformAtomCapitalize
	default:
		return TextTransformAtomNone
	}
}

func (t TextTransformAtom) String() string {
	switch t {
	case TextTransformAtomNone:
		return "none"
	case TextTransformAtomUppercase:
		return "uppercase"
	case TextTransformAtomLowercase:
		return "lowercase"
	case TextTransformAtomCapitalize:
		return "capitalize"
	default:
		return "none"
	}
}

// ---------------------------------------------------------------------------
// WhiteSpaceAtom
// ---------------------------------------------------------------------------

type WhiteSpaceAtom uint8

const (
	WhiteSpaceAtomNormal WhiteSpaceAtom = iota
	WhiteSpaceAtomNoWrap
	WhiteSpaceAtomPre
	WhiteSpaceAtomPreWrap
	WhiteSpaceAtomPreLine
)

func WhiteSpaceAtomFromString(s string) WhiteSpaceAtom {
	switch s {
	case "nowrap":
		return WhiteSpaceAtomNoWrap
	case "pre":
		return WhiteSpaceAtomPre
	case "pre-wrap", "break-spaces":
		return WhiteSpaceAtomPreWrap
	case "pre-line":
		return WhiteSpaceAtomPreLine
	default:
		return WhiteSpaceAtomNormal
	}
}

func (w WhiteSpaceAtom) String() string {
	switch w {
	case WhiteSpaceAtomNormal:
		return "normal"
	case WhiteSpaceAtomNoWrap:
		return "nowrap"
	case WhiteSpaceAtomPre:
		return "pre"
	case WhiteSpaceAtomPreWrap:
		return "pre-wrap"
	case WhiteSpaceAtomPreLine:
		return "pre-line"
	default:
		return "normal"
	}
}

// ---------------------------------------------------------------------------
// BackgroundRepeatAtom
// ---------------------------------------------------------------------------

type BackgroundRepeatAtom uint8

const (
	BackgroundRepeatAtomRepeat BackgroundRepeatAtom = iota
	BackgroundRepeatAtomRepeatX
	BackgroundRepeatAtomRepeatY
	BackgroundRepeatAtomNoRepeat
)

func BackgroundRepeatAtomFromString(s string) BackgroundRepeatAtom {
	switch s {
	case "repeat-x":
		return BackgroundRepeatAtomRepeatX
	case "repeat-y":
		return BackgroundRepeatAtomRepeatY
	case "no-repeat":
		return BackgroundRepeatAtomNoRepeat
	default:
		return BackgroundRepeatAtomRepeat
	}
}

func (b BackgroundRepeatAtom) String() string {
	switch b {
	case BackgroundRepeatAtomRepeat:
		return "repeat"
	case BackgroundRepeatAtomRepeatX:
		return "repeat-x"
	case BackgroundRepeatAtomRepeatY:
		return "repeat-y"
	case BackgroundRepeatAtomNoRepeat:
		return "no-repeat"
	default:
		return "repeat"
	}
}

// ---------------------------------------------------------------------------
// BackgroundPositionAtom
// ---------------------------------------------------------------------------

type BackgroundPositionAtom uint8

const (
	BackgroundPositionAtomCenter BackgroundPositionAtom = iota
	BackgroundPositionAtomTopLeft
	BackgroundPositionAtomTopCenter
	BackgroundPositionAtomTopRight
	BackgroundPositionAtomCenterLeft
	BackgroundPositionAtomCenterRight
	BackgroundPositionAtomBottomLeft
	BackgroundPositionAtomBottomCenter
	BackgroundPositionAtomBottomRight
)

func BackgroundPositionAtomFromString(s string) BackgroundPositionAtom {
	switch s {
	case "top left":
		return BackgroundPositionAtomTopLeft
	case "top center":
		return BackgroundPositionAtomTopCenter
	case "top right":
		return BackgroundPositionAtomTopRight
	case "center left":
		return BackgroundPositionAtomCenterLeft
	case "center right":
		return BackgroundPositionAtomCenterRight
	case "bottom left":
		return BackgroundPositionAtomBottomLeft
	case "bottom center":
		return BackgroundPositionAtomBottomCenter
	case "bottom right":
		return BackgroundPositionAtomBottomRight
	default:
		return BackgroundPositionAtomCenter
	}
}

func (b BackgroundPositionAtom) String() string {
	switch b {
	case BackgroundPositionAtomCenter:
		return "center"
	case BackgroundPositionAtomTopLeft:
		return "top left"
	case BackgroundPositionAtomTopCenter:
		return "top center"
	case BackgroundPositionAtomTopRight:
		return "top right"
	case BackgroundPositionAtomCenterLeft:
		return "center left"
	case BackgroundPositionAtomCenterRight:
		return "center right"
	case BackgroundPositionAtomBottomLeft:
		return "bottom left"
	case BackgroundPositionAtomBottomCenter:
		return "bottom center"
	case BackgroundPositionAtomBottomRight:
		return "bottom right"
	default:
		return "center"
	}
}

// ---------------------------------------------------------------------------
// BackgroundSizeAtom
// ---------------------------------------------------------------------------

type BackgroundSizeAtom uint8

const (
	BackgroundSizeAtomAuto BackgroundSizeAtom = iota
	BackgroundSizeAtomCover
	BackgroundSizeAtomContain
)

func BackgroundSizeAtomFromString(s string) BackgroundSizeAtom {
	switch s {
	case "cover":
		return BackgroundSizeAtomCover
	case "contain":
		return BackgroundSizeAtomContain
	default:
		return BackgroundSizeAtomAuto
	}
}

func (b BackgroundSizeAtom) String() string {
	switch b {
	case BackgroundSizeAtomAuto:
		return "auto"
	case BackgroundSizeAtomCover:
		return "cover"
	case BackgroundSizeAtomContain:
		return "contain"
	default:
		return "auto"
	}
}

// ---------------------------------------------------------------------------
// BackgroundAttachmentAtom
// ---------------------------------------------------------------------------

type BackgroundAttachmentAtom uint8

const (
	BackgroundAttachmentAtomScroll BackgroundAttachmentAtom = iota
	BackgroundAttachmentAtomFixed
	BackgroundAttachmentAtomLocal
)

func BackgroundAttachmentAtomFromString(s string) BackgroundAttachmentAtom {
	switch s {
	case "fixed":
		return BackgroundAttachmentAtomFixed
	case "local":
		return BackgroundAttachmentAtomLocal
	default:
		return BackgroundAttachmentAtomScroll
	}
}

func (b BackgroundAttachmentAtom) String() string {
	switch b {
	case BackgroundAttachmentAtomScroll:
		return "scroll"
	case BackgroundAttachmentAtomFixed:
		return "fixed"
	case BackgroundAttachmentAtomLocal:
		return "local"
	default:
		return "scroll"
	}
}

// ---------------------------------------------------------------------------
// ListStyleTypeAtom
// ---------------------------------------------------------------------------

type ListStyleTypeAtom uint8

const (
	ListStyleTypeAtomNone ListStyleTypeAtom = iota
	ListStyleTypeAtomDisc
	ListStyleTypeAtomCircle
	ListStyleTypeAtomSquare
	ListStyleTypeAtomDecimal
)

func ListStyleTypeAtomFromString(s string) ListStyleTypeAtom {
	switch s {
	case "disc":
		return ListStyleTypeAtomDisc
	case "circle":
		return ListStyleTypeAtomCircle
	case "square":
		return ListStyleTypeAtomSquare
	case "decimal":
		return ListStyleTypeAtomDecimal
	default:
		return ListStyleTypeAtomNone
	}
}

func (l ListStyleTypeAtom) String() string {
	switch l {
	case ListStyleTypeAtomNone:
		return "none"
	case ListStyleTypeAtomDisc:
		return "disc"
	case ListStyleTypeAtomCircle:
		return "circle"
	case ListStyleTypeAtomSquare:
		return "square"
	case ListStyleTypeAtomDecimal:
		return "decimal"
	default:
		return "none"
	}
}

// ---------------------------------------------------------------------------
// ListStylePositionAtom
// ---------------------------------------------------------------------------

type ListStylePositionAtom uint8

const (
	ListStylePositionAtomOutside ListStylePositionAtom = iota
	ListStylePositionAtomInside
)

func ListStylePositionAtomFromString(s string) ListStylePositionAtom {
	switch s {
	case "inside":
		return ListStylePositionAtomInside
	default:
		return ListStylePositionAtomOutside
	}
}

func (l ListStylePositionAtom) String() string {
	switch l {
	case ListStylePositionAtomOutside:
		return "outside"
	case ListStylePositionAtomInside:
		return "inside"
	default:
		return "outside"
	}
}

// ---------------------------------------------------------------------------
// OverflowAtom
// ---------------------------------------------------------------------------

type OverflowAtom uint8

const (
	OverflowAtomVisible OverflowAtom = iota
	OverflowAtomHidden
	OverflowAtomScroll
	OverflowAtomAuto
)

func OverflowAtomFromString(s string) OverflowAtom {
	switch s {
	case "hidden":
		return OverflowAtomHidden
	case "scroll":
		return OverflowAtomScroll
	case "auto":
		return OverflowAtomAuto
	default:
		return OverflowAtomVisible
	}
}

func (o OverflowAtom) String() string {
	switch o {
	case OverflowAtomVisible:
		return "visible"
	case OverflowAtomHidden:
		return "hidden"
	case OverflowAtomScroll:
		return "scroll"
	case OverflowAtomAuto:
		return "auto"
	default:
		return "visible"
	}
}
