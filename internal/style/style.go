package style

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

// Display is the CSS display property value.
type Display uint8

const (
	DisplayNone Display = iota
	DisplayBlock
	DisplayInline
	DisplayInlineBlock
	DisplayFlex
	DisplayInlineFlex
	DisplayListItem
	DisplayTable
	DisplayTableRow
	DisplayTableCell
	DisplayTableRowGroup
	DisplayTableHeaderGroup
	DisplayTableFooterGroup
	DisplayTableColumn
	DisplayTableColumnGroup
	DisplayTableCaption
)

// Position is the CSS position property value.
type Position uint8

const (
	PositionStatic Position = iota
	PositionRelative
	PositionAbsolute
	PositionFixed
	PositionSticky
)

// Overflow is the CSS overflow property value.
type Overflow uint8

const (
	OverflowVisible Overflow = iota
	OverflowHidden
	OverflowScroll
	OverflowAuto
)

// Float is the CSS float property value.
type Float uint8

const (
	FloatNone Float = iota
	FloatLeft
	FloatRight
)

// Clear is the CSS clear property value.
type Clear uint8

const (
	ClearNone Clear = iota
	ClearLeft
	ClearRight
	ClearBoth
)

// TextAlign is the CSS text-align property value.
type TextAlign uint8

const (
	TextAlignLeft TextAlign = iota
	TextAlignRight
	TextAlignCenter
	TextAlignJustify
)

// WhiteSpace is the CSS white-space property value.
type WhiteSpace uint8

const (
	WhiteSpaceNormal WhiteSpace = iota
	WhiteSpaceNowrap
	WhiteSpacePre
	WhiteSpacePrewrite
	WhiteSpacePreline
)

// FontWeight is the CSS font-weight.
type FontWeight uint16

const (
	WeightNormal FontWeight = 400
	WeightBold   FontWeight = 700
)

// FontStyle is the CSS font-style.
type FontStyle uint8

const (
	FontStyleNormal FontStyle = iota
	FontStyleItalic
	FontStyleOblique
)

// TextDecoration is the CSS text-decoration.
type TextDecoration uint8

const (
	TextDecorationNone      TextDecoration = 0
	TextDecorationUnderline TextDecoration = 1 << iota
	TextDecorationOverline
	TextDecorationLineThrough
)

// BoxSizing is the CSS box-sizing property.
type BoxSizing uint8

const (
	BoxSizingContentBox BoxSizing = iota
	BoxSizingBorderBox
)

// ListStyleType is the CSS list-style-type.
type ListStyleType uint8

const (
	ListStyleNone ListStyleType = iota
	ListStyleDisc
	ListStyleCircle
	ListStyleSquare
	ListStyleDecimal
)

// FlexDirection is the CSS flex-direction.
type FlexDirection uint8

const (
	FlexRow FlexDirection = iota
	FlexRowReverse
	FlexColumn
	FlexColumnReverse
)

// FlexWrap is the CSS flex-wrap.
type FlexWrap uint8

const (
	FlexNowrap FlexWrap = iota
	FlexWrapValue
	FlexWrapReverse
)

// ComputedStyle holds every CSS property fully resolved to concrete values.
// Every length is float32 in CSS pixels; no parse step remains for layout.
type ComputedStyle struct {
	Display      Display
	Position     Position
	Float        Float
	Clear        Clear
	Overflow     Overflow
	OverflowX    Overflow
	OverflowY    Overflow
	BoxSizing    BoxSizing
	Visibility   string
	Opacity      float32
	ZIndex       int32
	HasZIndex    bool

	Width      float32
	Height     float32
	MinWidth   float32
	MinHeight  float32
	MaxWidth   float32
	MaxHeight  float32

	MarginTop    float32
	MarginRight  float32
	MarginBottom float32
	MarginLeft   float32

	PaddingTop    float32
	PaddingRight  float32
	PaddingBottom float32
	PaddingLeft   float32

	BorderTopWidth    float32
	BorderRightWidth  float32
	BorderBottomWidth float32
	BorderLeftWidth   float32

	BorderTopStyle    string
	BorderRightStyle  string
	BorderBottomStyle string
	BorderLeftStyle   string

	BorderTopColor    css.Color
	BorderRightColor  css.Color
	BorderBottomColor css.Color
	BorderLeftColor   css.Color

	Top    float32
	Right  float32
	Bottom float32
	Left   float32

	Color            css.Color
	BackgroundColor  css.Color

	FontFamily    string
	FontSize      float32
	FontWeight    FontWeight
	FontStyle     FontStyle
	LineHeight    float32
	TextAlign     TextAlign
	TextIndent    float32
	TextTransform string
	TextDecoration TextDecoration
	WhiteSpace    WhiteSpace
	WordSpacing   float32
	LetterSpacing float32

	VerticalAlign string

	ListStyleType ListStyleType

	FlexDirection FlexDirection
	FlexWrap      FlexWrap
	JustifyContent string
	AlignItems     string
	AlignSelf      string
	FlexGrow       float32
	FlexShrink     float32
	FlexBasis      float32
	Gap            float32

	TableLayout string
	BorderCollapse bool
}

// DefaultStyle returns the initial computed style with CSS defaults.
func DefaultStyle() ComputedStyle {
	return ComputedStyle{
		Display:    DisplayInline,
		Position:   PositionStatic,
		Opacity:    1.0,
		Width:      -1,
		Height:     -1,
		MinWidth:   0,
		MinHeight:  0,
		MaxWidth:   -1,
		MaxHeight:  -1,
		Color:      css.Color{R: 0, G: 0, B: 0, A: 255},
		FontSize:   16,
		FontWeight: WeightNormal,
		LineHeight: 1.2,
		FontFamily: "Go Regular",
		Visibility: "visible",
		WhiteSpace: WhiteSpaceNormal,
	}
}

// UserAgentStylesheet returns the default UA stylesheet.
func UserAgentStylesheet() *css.Stylesheet {
	return css.Parse(uaCSS)
}

var uaCSS = `
html, body, div, span, h1, h2, h3, h4, h5, h6, p, a, img,
ul, ol, li, table, thead, tbody, tr, td, th, form, input, button,
select, textarea, label, header, footer, nav, main, section, article,
aside, figure, figcaption, pre, code, blockquote, dl, dt, dd,
address, details, summary, dialog, fieldset, legend {
	box-sizing: border-box;
}
html { display: block; }
head { display: none; }
body { display: block; margin: 8px; }
div { display: block; }
span { display: inline; }
p { display: block; margin-top: 1em; margin-bottom: 1em; }
h1 { display: block; font-size: 2em; font-weight: 700; margin-top: 0.67em; margin-bottom: 0.67em; }
h2 { display: block; font-size: 1.5em; font-weight: 700; margin-top: 0.83em; margin-bottom: 0.83em; }
h3 { display: block; font-size: 1.17em; font-weight: 700; margin-top: 1em; margin-bottom: 1em; }
h4 { display: block; font-weight: 700; margin-top: 1.33em; margin-bottom: 1.33em; }
h5 { display: block; font-size: 0.83em; font-weight: 700; margin-top: 1.67em; margin-bottom: 1.67em; }
h6 { display: block; font-size: 0.67em; font-weight: 700; margin-top: 2.33em; margin-bottom: 2.33em; }
ul, ol { display: block; margin-top: 1em; margin-bottom: 1em; padding-left: 40px; }
li { display: list-item; }
a { color: #0000ee; }
table { display: table; border-collapse: separate; border-spacing: 2px; }
thead { display: table-header-group; }
tbody { display: table-row-group; }
tr { display: table-row; }
td { display: table-cell; padding: 1px; }
th { display: table-cell; padding: 1px; font-weight: 700; }
caption { display: table-caption; text-align: center; }
colgroup { display: table-column-group; }
col { display: table-column; }
pre { display: block; white-space: pre; font-family: monospace; margin: 1em 0; }
code { font-family: monospace; }
blockquote { display: block; margin: 1em 40px; }
hr { display: block; margin-top: 0.5em; margin-bottom: 0.5em; border-top: 1px solid; }
br { display: inline; }
img { display: inline; }
b, strong { font-weight: 700; }
i, em { font-style: italic; }
u { text-decoration: underline; }
small { font-size: 0.83em; }
sub { font-size: 0.83em; vertical-align: sub; }
sup { font-size: 0.83em; vertical-align: super; }
header, footer { display: block; }
nav { display: block; }
main { display: block; }
section { display: block; }
article { display: block; }
aside { display: block; }
figure { display: block; margin: 1em 40px; }
figcaption { display: block; }
details { display: block; }
summary { display: block; }
dialog { display: none; }
fieldset { display: block; margin: 0 2px; padding: 0.35em 0.75em 0.625em; border: 2px groove; }
legend { display: block; padding: 0 0.25em; }
form { display: block; }
label { display: inline; }
input, select, textarea, button { display: inline; }
dl { display: block; margin-top: 1em; margin-bottom: 1em; }
dt { display: block; font-weight: 700; }
dd { display: block; margin-left: 40px; }
address { display: block; font-style: italic; }
`

// Resolve computes the style for every element in the document.
func Resolve(doc *dom.Document, sheets []*css.Stylesheet) map[dom.NodeID]*ComputedStyle {
	allSheets := []*css.Stylesheet{UserAgentStylesheet()}
	allSheets = append(allSheets, sheets...)

	result := make(map[dom.NodeID]*ComputedStyle)
	for c := doc.Node.FirstChild; c != nil; c = c.NextSibling {
		resolveNode(c, allSheets, result)
	}
	return result
}

func resolveNode(n *dom.Node, sheets []*css.Stylesheet, result map[dom.NodeID]*ComputedStyle) {
	if n == nil {
		return
	}
	if n.Element() {
		parentStyle := findParentStyle(n, result)
		cs := computeStyle(n, sheets, parentStyle)
		result[n.ID] = cs
	} else if n.Type == 2 {
		parentStyle := findParentStyle(n, result)
		if parentStyle != nil {
			textStyle := inheritStyle(parentStyle)
			result[n.ID] = textStyle
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		resolveNode(c, sheets, result)
	}
}

func findParentStyle(n *dom.Node, result map[dom.NodeID]*ComputedStyle) *ComputedStyle {
	for p := n.Parent; p != nil; p = p.Parent {
		if cs, ok := result[p.ID]; ok {
			return cs
		}
	}
	return nil
}

func inheritStyle(parent *ComputedStyle) *ComputedStyle {
	cs := *parent
	cs.Display = DisplayInline
	return &cs
}

func computeStyle(n *dom.Node, sheets []*css.Stylesheet, parent *ComputedStyle) *ComputedStyle {
	cs := DefaultStyle()
	if parent != nil {
		inheritFromParent(&cs, parent)
	}

	parentFontSize := float32(16)
	if parent != nil {
		parentFontSize = parent.FontSize
	}

	for _, sheet := range sheets {
		for _, rule := range sheet.Rules {
			for i, sel := range rule.Selectors {
				if sel.Matches(n) {
					a, b, c := sel.Specificity()
					_ = rule.SelectorStrs[i]
					applyDeclarations(&cs, rule.Declarations, a, b, c, false, parentFontSize)
				}
			}
		}
	}

	applyInlineStyle(&cs, n.GetAttribute("style"), parentFontSize)

	return &cs
}

func inheritFromParent(cs *ComputedStyle, parent *ComputedStyle) {
	cs.Color = parent.Color
	cs.FontSize = parent.FontSize
	cs.FontFamily = parent.FontFamily
	cs.FontWeight = parent.FontWeight
	cs.FontStyle = parent.FontStyle
	cs.LineHeight = parent.LineHeight
	cs.TextAlign = parent.TextAlign
	cs.Visibility = parent.Visibility
	cs.WhiteSpace = parent.WhiteSpace
	cs.WordSpacing = parent.WordSpacing
	cs.LetterSpacing = parent.LetterSpacing
	cs.TextIndent = parent.TextIndent
	cs.TextTransform = parent.TextTransform
	cs.TextDecoration = parent.TextDecoration
	cs.VerticalAlign = parent.VerticalAlign
	cs.ListStyleType = parent.ListStyleType
}

func applyInlineStyle(cs *ComputedStyle, inline string, parentFontSize float32) {
	if inline == "" {
		return
	}
	decls := parseInlineDeclarations(inline)
	applyDeclarations(cs, decls, 1, 0, 0, true, parentFontSize)
}

func parseInlineDeclaration(s string) css.Declaration {
	colonIdx := strings.Index(s, ":")
	if colonIdx < 0 {
		return css.Declaration{}
	}
	prop := strings.TrimSpace(s[:colonIdx])
	val := strings.TrimSpace(s[colonIdx+1:])
	important := false
	if idx := strings.Index(strings.ToLower(val), "!important"); idx >= 0 {
		important = true
		val = strings.TrimSpace(val[:idx])
	}
	return css.Declaration{
		Property:  strings.ToLower(prop),
		Value:     val,
		Parsed:    css.ParseValue(val),
		Important: important,
	}
}

func parseInlineDeclarations(s string) []css.Declaration {
	var decls []css.Declaration
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		decls = append(decls, parseInlineDeclaration(part))
	}
	return decls
}

type specificity struct {
	a, b, c int
	inline  bool
}

func applyDeclarations(cs *ComputedStyle, decls []css.Declaration, a, b, c int, inline bool, parentFontSize float32) {
	for _, d := range decls {
		applyProperty(cs, d.Property, d.Value, d.Parsed, parentFontSize)
	}
}

func applyProperty(cs *ComputedStyle, prop, value string, parsed css.Value, parentFontSize float32) {
	switch prop {
	case "display":
		cs.Display = parseDisplay(value)
	case "position":
		cs.Position = parsePosition(value)
	case "float":
		cs.Float = parseFloatProp(value)
	case "clear":
		cs.Clear = parseClear(value)
	case "overflow":
		cs.Overflow = parseOverflowProp(value)
		cs.OverflowX = cs.Overflow
		cs.OverflowY = cs.Overflow
	case "overflow-x":
		cs.OverflowX = parseOverflowProp(value)
	case "overflow-y":
		cs.OverflowY = parseOverflowProp(value)
	case "box-sizing":
		if value == "border-box" {
			cs.BoxSizing = BoxSizingBorderBox
		}
	case "visibility":
		cs.Visibility = value
	case "opacity":
		cs.Opacity = float32(parsed.Num)
	case "z-index":
		if value == "auto" {
			cs.HasZIndex = false
		} else {
			cs.HasZIndex = true
			cs.ZIndex = int32(parsed.Num)
		}

	case "width":
		cs.Width = resolveLength(parsed, -1)
	case "height":
		cs.Height = resolveLength(parsed, -1)
	case "min-width":
		cs.MinWidth = resolveLength(parsed, 0)
	case "min-height":
		cs.MinHeight = resolveLength(parsed, 0)
	case "max-width":
		cs.MaxWidth = resolveLength(parsed, -1)
	case "max-height":
		cs.MaxHeight = resolveLength(parsed, -1)

	case "margin":
		t, r, b, l := parseBoxShorthand(value)
		cs.MarginTop = t; cs.MarginRight = r; cs.MarginBottom = b; cs.MarginLeft = l
	case "margin-top":
		cs.MarginTop = resolveLength(parsed, 0)
	case "margin-right":
		cs.MarginRight = resolveLength(parsed, 0)
	case "margin-bottom":
		cs.MarginBottom = resolveLength(parsed, 0)
	case "margin-left":
		cs.MarginLeft = resolveLength(parsed, 0)

	case "padding":
		t, r, b, l := parseBoxShorthand(value)
		cs.PaddingTop = t; cs.PaddingRight = r; cs.PaddingBottom = b; cs.PaddingLeft = l
	case "padding-top":
		cs.PaddingTop = resolveLength(parsed, 0)
	case "padding-right":
		cs.PaddingRight = resolveLength(parsed, 0)
	case "padding-bottom":
		cs.PaddingBottom = resolveLength(parsed, 0)
	case "padding-left":
		cs.PaddingLeft = resolveLength(parsed, 0)

	case "border":
		w, s, c := parseBorderShorthand(value)
		cs.BorderTopWidth = w; cs.BorderRightWidth = w; cs.BorderBottomWidth = w; cs.BorderLeftWidth = w
		cs.BorderTopStyle = s; cs.BorderRightStyle = s; cs.BorderBottomStyle = s; cs.BorderLeftStyle = s
		cs.BorderTopColor = c; cs.BorderRightColor = c; cs.BorderBottomColor = c; cs.BorderLeftColor = c
	case "border-width":
		t, r, b, l := parseBoxShorthand(value)
		cs.BorderTopWidth = t; cs.BorderRightWidth = r; cs.BorderBottomWidth = b; cs.BorderLeftWidth = l
	case "border-top-width":
		cs.BorderTopWidth = resolveLength(parsed, 0)
	case "border-right-width":
		cs.BorderRightWidth = resolveLength(parsed, 0)
	case "border-bottom-width":
		cs.BorderBottomWidth = resolveLength(parsed, 0)
	case "border-left-width":
		cs.BorderLeftWidth = resolveLength(parsed, 0)
	case "border-style":
		t, r, b, l := parseBorderStyles(value)
		cs.BorderTopStyle = t; cs.BorderRightStyle = r; cs.BorderBottomStyle = b; cs.BorderLeftStyle = l
	case "border-top-style":
		cs.BorderTopStyle = value
	case "border-right-style":
		cs.BorderRightStyle = value
	case "border-bottom-style":
		cs.BorderBottomStyle = value
	case "border-left-style":
		cs.BorderLeftStyle = value
	case "border-color":
		colors := parseBorderColors(value)
		cs.BorderTopColor = colors[0]; cs.BorderRightColor = colors[1]
		cs.BorderBottomColor = colors[2]; cs.BorderLeftColor = colors[3]
	case "border-top-color":
		cs.BorderTopColor = parseColorValue(value)
	case "border-right-color":
		cs.BorderRightColor = parseColorValue(value)
	case "border-bottom-color":
		cs.BorderBottomColor = parseColorValue(value)
	case "border-left-color":
		cs.BorderLeftColor = parseColorValue(value)
	case "border-top":
		w, s, c := parseBorderShorthand(value)
		cs.BorderTopWidth = w; cs.BorderTopStyle = s; cs.BorderTopColor = c
	case "border-right":
		w, s, c := parseBorderShorthand(value)
		cs.BorderRightWidth = w; cs.BorderRightStyle = s; cs.BorderRightColor = c
	case "border-bottom":
		w, s, c := parseBorderShorthand(value)
		cs.BorderBottomWidth = w; cs.BorderBottomStyle = s; cs.BorderBottomColor = c
	case "border-left":
		w, s, c := parseBorderShorthand(value)
		cs.BorderLeftWidth = w; cs.BorderLeftStyle = s; cs.BorderLeftColor = c
	case "border-radius":
		// simplified: store as border-top-left-radius for now

	case "top":
		cs.Top = resolveLength(parsed, 0)
	case "right":
		cs.Right = resolveLength(parsed, 0)
	case "bottom":
		cs.Bottom = resolveLength(parsed, 0)
	case "left":
		cs.Left = resolveLength(parsed, 0)

	case "color":
		cs.Color = parseColorValue(value)
	case "background-color":
		cs.BackgroundColor = parseColorValue(value)
	case "background":
		cs.BackgroundColor = parseColorValue(value)

	case "font-family":
		cs.FontFamily = parseFontFamily(value)
	case "font-size":
		cs.FontSize = resolveFontSize(parsed, parentFontSize)
	case "font-weight":
		cs.FontWeight = parseFontWeight(value)
	case "font-style":
		cs.FontStyle = parseFontStyle(value)
	case "font":
		parseFontShorthand(cs, value, parentFontSize)
	case "line-height":
		cs.LineHeight = resolveLineHeight(parsed, cs.FontSize)
	case "text-align":
		cs.TextAlign = parseTextAlign(value)
	case "text-indent":
		cs.TextIndent = resolveLength(parsed, 0)
	case "text-transform":
		cs.TextTransform = value
	case "text-decoration":
		cs.TextDecoration = parseTextDecoration(value)
	case "white-space":
		cs.WhiteSpace = parseWhiteSpace(value)
	case "word-spacing":
		cs.WordSpacing = resolveLength(parsed, 0)
	case "letter-spacing":
		cs.LetterSpacing = resolveLength(parsed, 0)

	case "vertical-align":
		cs.VerticalAlign = value

	case "list-style-type":
		cs.ListStyleType = parseListStyleType(value)
	case "list-style":
		cs.ListStyleType = parseListStyleType(value)

	case "flex-direction":
		cs.FlexDirection = parseFlexDirection(value)
	case "flex-wrap":
		cs.FlexWrap = parseFlexWrap(value)
	case "justify-content":
		cs.JustifyContent = value
	case "align-items":
		cs.AlignItems = value
	case "align-self":
		cs.AlignSelf = value
	case "flex-grow":
		cs.FlexGrow = float32(parsed.Num)
	case "flex-shrink":
		cs.FlexShrink = float32(parsed.Num)
	case "flex-basis":
		cs.FlexBasis = resolveLength(parsed, -1)
	case "gap":
		cs.Gap = resolveLength(parsed, 0)
	case "flex":
		parseFlexShorthand(cs, value)

	case "table-layout":
		cs.TableLayout = value
	case "border-collapse":
		cs.BorderCollapse = value == "collapse"
	case "border-spacing":
		// simplified
	}
}

func resolveLength(v css.Value, auto float32) float32 {
	if v.Type == css.ValueKeyword && (v.Str == "auto" || v.Str == "") {
		return auto
	}
	return v.ToLength()
}

func resolveFontSize(v css.Value, parent float32) float32 {
	switch v.Type {
	case css.ValueKeyword:
		switch v.Str {
		case "xx-small":
			return 9
		case "x-small":
			return 10
		case "small":
			return 13
		case "medium":
			return 16
		case "large":
			return 18
		case "x-large":
			return 24
		case "xx-large":
			return 32
		case "smaller":
			return parent * 0.83
		case "larger":
			return parent * 1.2
		}
	case css.ValuePercentage:
		return parent * float32(v.Num) / 100
	case css.ValueLength:
		if v.Unit == "em" || v.Unit == "rem" || v.Unit == "ex" {
			return parent * float32(v.Num)
		}
		return v.ToLength()
	}
	return float32(v.Num)
}

func resolveLineHeight(v css.Value, fontSize float32) float32 {
	switch v.Type {
	case css.ValueNumber:
		return fontSize * float32(v.Num)
	case css.ValuePercentage:
		return fontSize * float32(v.Num) / 100
	case css.ValueLength:
		return v.ToLength()
	case css.ValueKeyword:
		if v.Str == "normal" {
			return fontSize * 1.2
		}
	}
	return fontSize * 1.2
}

func parseDisplay(v string) Display {
	switch v {
	case "none":
		return DisplayNone
	case "block":
		return DisplayBlock
	case "inline":
		return DisplayInline
	case "inline-block":
		return DisplayInlineBlock
	case "flex":
		return DisplayFlex
	case "inline-flex":
		return DisplayInlineFlex
	case "list-item":
		return DisplayListItem
	case "table":
		return DisplayTable
	case "table-row":
		return DisplayTableRow
	case "table-cell":
		return DisplayTableCell
	case "table-row-group":
		return DisplayTableRowGroup
	case "table-header-group":
		return DisplayTableHeaderGroup
	case "table-footer-group":
		return DisplayTableFooterGroup
	case "table-column":
		return DisplayTableColumn
	case "table-column-group":
		return DisplayTableColumnGroup
	case "table-caption":
		return DisplayTableCaption
	}
	return DisplayInline
}

func parsePosition(v string) Position {
	switch v {
	case "relative":
		return PositionRelative
	case "absolute":
		return PositionAbsolute
	case "fixed":
		return PositionFixed
	case "sticky":
		return PositionSticky
	}
	return PositionStatic
}

func parseFloatProp(v string) Float {
	switch v {
	case "left":
		return FloatLeft
	case "right":
		return FloatRight
	}
	return FloatNone
}

func parseClear(v string) Clear {
	switch v {
	case "left":
		return ClearLeft
	case "right":
		return ClearRight
	case "both":
		return ClearBoth
	}
	return ClearNone
}

func parseOverflowProp(v string) Overflow {
	switch v {
	case "hidden":
		return OverflowHidden
	case "scroll":
		return OverflowScroll
	case "auto":
		return OverflowAuto
	}
	return OverflowVisible
}

func parseTextAlign(v string) TextAlign {
	switch v {
	case "right":
		return TextAlignRight
	case "center":
		return TextAlignCenter
	case "justify":
		return TextAlignJustify
	}
	return TextAlignLeft
}

func parseWhiteSpace(v string) WhiteSpace {
	switch v {
	case "nowrap":
		return WhiteSpaceNowrap
	case "pre":
		return WhiteSpacePre
	case "pre-wrap":
		return WhiteSpacePrewrite
	case "pre-line":
		return WhiteSpacePreline
	}
	return WhiteSpaceNormal
}

func parseFontWeight(v string) FontWeight {
	switch v {
	case "bold":
		return WeightBold
	case "bolder":
		return WeightBold
	case "lighter":
		return 100
	case "normal":
		return WeightNormal
	}
	if parsed := css.ParseValue(v); parsed.Type == css.ValueNumber {
		return FontWeight(parsed.Num)
	}
	return WeightNormal
}

func parseFontStyle(v string) FontStyle {
	switch v {
	case "italic":
		return FontStyleItalic
	case "oblique":
		return FontStyleOblique
	}
	return FontStyleNormal
}

func parseFontFamily(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "\"'")
	if idx := strings.Index(v, ","); idx >= 0 {
		v = strings.TrimSpace(v[:idx])
		v = strings.Trim(v, "\"'")
	}
	return v
}

func parseTextDecoration(v string) TextDecoration {
	var td TextDecoration
	for _, part := range strings.Fields(v) {
		switch part {
		case "underline":
			td |= TextDecorationUnderline
		case "overline":
			td |= TextDecorationOverline
		case "line-through":
			td |= TextDecorationLineThrough
		case "none":
			return TextDecorationNone
		}
	}
	return td
}

func parseListStyleType(v string) ListStyleType {
	switch v {
	case "none":
		return ListStyleNone
	case "disc":
		return ListStyleDisc
	case "circle":
		return ListStyleCircle
	case "square":
		return ListStyleSquare
	case "decimal":
		return ListStyleDecimal
	}
	return ListStyleDisc
}

func parseFlexDirection(v string) FlexDirection {
	switch v {
	case "row-reverse":
		return FlexRowReverse
	case "column":
		return FlexColumn
	case "column-reverse":
		return FlexColumnReverse
	}
	return FlexRow
}

func parseFlexWrap(v string) FlexWrap {
	switch v {
	case "wrap":
		return FlexWrapValue
	case "wrap-reverse":
		return FlexWrapReverse
	}
	return FlexNowrap
}

func parseColorValue(v string) css.Color {
	c, _ := css.ParseColor(v)
	return c
}

func parseBoxShorthand(v string) (top, right, bottom, left float32) {
	parts := strings.Fields(v)
	switch len(parts) {
	case 1:
		val := resolveLength(css.ParseValue(parts[0]), 0)
		return val, val, val, val
	case 2:
		tb := resolveLength(css.ParseValue(parts[0]), 0)
		rl := resolveLength(css.ParseValue(parts[1]), 0)
		return tb, rl, tb, rl
	case 3:
		top = resolveLength(css.ParseValue(parts[0]), 0)
		rl := resolveLength(css.ParseValue(parts[1]), 0)
		bottom = resolveLength(css.ParseValue(parts[2]), 0)
		return top, rl, bottom, rl
	case 4:
		top = resolveLength(css.ParseValue(parts[0]), 0)
		right = resolveLength(css.ParseValue(parts[1]), 0)
		bottom = resolveLength(css.ParseValue(parts[2]), 0)
		left = resolveLength(css.ParseValue(parts[3]), 0)
		return
	}
	return
}

func parseBorderShorthand(v string) (width float32, style string, color css.Color) {
	parts := strings.Fields(v)
	for _, p := range parts {
		lower := strings.ToLower(p)
		if isBorderStyle(lower) {
			style = lower
		} else if _, ok := css.ParseColor(lower); ok {
			color, _ = css.ParseColor(lower)
		} else {
			width = resolveLength(css.ParseValue(p), 0)
		}
	}
	if width == 0 && style != "" && style != "none" && style != "hidden" {
		width = 3
	}
	return
}

func isBorderStyle(v string) bool {
	switch v {
	case "none", "hidden", "dotted", "dashed", "solid",
		"double", "groove", "ridge", "inset", "outset":
		return true
	}
	return false
}

func parseBorderStyles(v string) (top, right, bottom, left string) {
	parts := strings.Fields(v)
	switch len(parts) {
	case 1:
		return parts[0], parts[0], parts[0], parts[0]
	case 2:
		return parts[0], parts[1], parts[0], parts[1]
	case 3:
		return parts[0], parts[1], parts[2], parts[1]
	case 4:
		return parts[0], parts[1], parts[2], parts[3]
	}
	return
}

func parseBorderColors(v string) [4]css.Color {
	parts := strings.Fields(v)
	var colors [4]css.Color
	switch len(parts) {
	case 1:
		c, _ := css.ParseColor(parts[0])
		return [4]css.Color{c, c, c, c}
	case 2:
		t, _ := css.ParseColor(parts[0])
		r, _ := css.ParseColor(parts[1])
		return [4]css.Color{t, r, t, r}
	case 3:
		colors[0], _ = css.ParseColor(parts[0])
		colors[1], _ = css.ParseColor(parts[1])
		colors[2], _ = css.ParseColor(parts[2])
		colors[3] = colors[1]
	case 4:
		colors[0], _ = css.ParseColor(parts[0])
		colors[1], _ = css.ParseColor(parts[1])
		colors[2], _ = css.ParseColor(parts[2])
		colors[3], _ = css.ParseColor(parts[3])
	}
	return colors
}

func parseFontShorthand(cs *ComputedStyle, v string, parentFontSize float32) {
	parts := strings.Fields(v)
	for _, p := range parts {
		lower := strings.ToLower(p)
		if lower == "bold" {
			cs.FontWeight = WeightBold
		} else if lower == "italic" {
			cs.FontStyle = FontStyleItalic
		} else if lower == "oblique" {
			cs.FontStyle = FontStyleOblique
		} else if lower == "normal" {
			// skip
		} else if _, ok := css.ParseColor(lower); ok {
			// skip color in font shorthand for now
		} else if parsed := css.ParseValue(p); parsed.Type == css.ValueLength || parsed.Type == css.ValueNumber {
			if cs.FontSize == 16 {
				cs.FontSize = resolveFontSize(parsed, parentFontSize)
			}
		} else {
			cs.FontFamily = parseFontFamily(p)
		}
	}
}

func parseFlexShorthand(cs *ComputedStyle, v string) {
	parts := strings.Fields(v)
	switch len(parts) {
	case 1:
		if parts[0] == "none" {
			cs.FlexGrow = 0
			cs.FlexShrink = 0
			cs.FlexBasis = -1
		} else if parts[0] == "auto" {
			cs.FlexGrow = 1
			cs.FlexShrink = 1
			cs.FlexBasis = -1
		} else {
			cs.FlexGrow = float32(css.ParseValue(parts[0]).Num)
			cs.FlexShrink = 1
			cs.FlexBasis = 0
		}
	case 2:
		cs.FlexGrow = float32(css.ParseValue(parts[0]).Num)
		cs.FlexShrink = float32(css.ParseValue(parts[1]).Num)
		cs.FlexBasis = 0
	case 3:
		cs.FlexGrow = float32(css.ParseValue(parts[0]).Num)
		cs.FlexShrink = float32(css.ParseValue(parts[1]).Num)
		cs.FlexBasis = resolveLength(css.ParseValue(parts[2]), -1)
	}
}
