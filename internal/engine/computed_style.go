package engine

import (
	"strconv"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

// computedStyleValues renders one node's cascade result as CSS declaration
// values, which is the shape internal/js asks for so that package can answer
// getComputedStyle without importing internal/style (the archtest import table
// does not allow that edge).
//
// The rule this function obeys is the one the API's contract sets: report what
// the engine resolved, and nothing it did not. Every value here comes from a
// field the session's own style pass filled, or from the layout box the arena
// holds. A property the engine does not model is left out of the map, so the
// binding reads it back as the empty string instead of inventing an initial.
//
// It returns nil when the node has no entry in the document's style map: a node
// a script created after the last resolve has no cascade result to name yet, and
// "I have not resolved this node" is information the caller can use, while an
// empty map would look like "this node resolved to nothing".
func (s *Session) computedStyleValues(n *dom.Node) map[string]string {
	if s == nil || n == nil {
		return nil
	}
	s.opMu.Lock()
	cs := s.Styles[n.ID]
	var boxW, boxH float32
	hasBox := false
	if s.Arena != nil && cs != nil {
		if id, ok := s.Arena.ForNode(n.ID); ok {
			if o := s.Arena.Get(id); o != nil {
				boxW, boxH, hasBox = o.W, o.H, true
			}
		}
	}
	s.opMu.Unlock()
	if cs == nil {
		return nil
	}

	out := map[string]string{}
	put := func(name, value string) {
		if value == "" {
			return
		}
		out[name] = value
	}

	put("display", displayKeyword(cs.Display))
	put("position", positionKeyword(cs.Position))
	put("float", floatKeyword(cs.Float))
	put("clear", clearKeyword(cs.Clear))
	put("box-sizing", boxSizingKeyword(cs.BoxSizing))
	put("visibility", cs.Visibility)
	put("overflow", overflowKeyword(cs.Overflow))
	put("overflow-x", overflowKeyword(cs.OverflowX))
	put("overflow-y", overflowKeyword(cs.OverflowY))
	put("direction", cs.Direction)
	put("text-align", textAlignKeyword(cs.TextAlign))
	put("white-space", whiteSpaceKeyword(cs.WhiteSpace))
	put("font-style", fontStyleKeyword(cs.FontStyle))

	// Numbers and lengths: each rendered the way CSS spells the used value.
	out["opacity"] = formatCSSNumber(float64(cs.Opacity))
	if cs.HasZIndex {
		put("z-index", strconv.FormatInt(int64(cs.ZIndex), 10))
	} else {
		put("z-index", "auto")
	}
	put("font-size", px(cs.FontSize))
	if cs.FontFamily.String() != "" {
		family := cs.FontFamily.String()
		if cs.CustomFamily != "" {
			family = cs.CustomFamily
		}
		put("font-family", family)
	}
	if cs.FontWeight > 0 {
		put("font-weight", strconv.FormatInt(int64(cs.FontWeight), 10))
	}
	if cs.LineHeight > 0 {
		put("line-height", px(cs.LineHeight))
	} else {
		put("line-height", "normal")
	}
	put("color", cssColor(cs.Color))
	put("background-color", cssColor(cs.BackgroundColor))

	// Box metrics. A laid-out node reports the used value from its layout box,
	// which is what a browser reports; a node the cascade knows but layout
	// skipped reports the cascade length, because that is all the engine has and
	// it is still the engine's own answer. See the deviation note in
	// docs/tasks/TASK-047-DOM-JQUERY-APIS.md: the arena keeps the content width
	// in W, so a border-box element's reported width is short by its padding and
	// border.
	if hasBox {
		put("width", px(boxW))
		put("height", px(boxH))
	} else {
		if cs.Width >= 0 {
			put("width", px(cs.Width))
		}
		if cs.Height >= 0 {
			put("height", px(cs.Height))
		}
	}

	put("margin-top", px(cs.MarginTop))
	put("margin-right", px(cs.MarginRight))
	put("margin-bottom", px(cs.MarginBottom))
	put("margin-left", px(cs.MarginLeft))
	put("padding-top", px(cs.PaddingTop))
	put("padding-right", px(cs.PaddingRight))
	put("padding-bottom", px(cs.PaddingBottom))
	put("padding-left", px(cs.PaddingLeft))
	put("border-top-width", px(cs.BorderTopWidth))
	put("border-right-width", px(cs.BorderRightWidth))
	put("border-bottom-width", px(cs.BorderBottomWidth))
	put("border-left-width", px(cs.BorderLeftWidth))
	put("border-top-style", cs.BorderTopStyle)
	put("border-right-style", cs.BorderRightStyle)
	put("border-bottom-style", cs.BorderBottomStyle)
	put("border-left-style", cs.BorderLeftStyle)
	put("border-top-color", cssColor(cs.BorderTopColor))
	put("border-right-color", cssColor(cs.BorderRightColor))
	put("border-bottom-color", cssColor(cs.BorderBottomColor))
	put("border-left-color", cssColor(cs.BorderLeftColor))

	// Custom properties are the one case where the engine keeps the author's
	// raw text, so it can be handed back raw: a page reading var() values out of
	// getComputedStyle is reading the declaration, not a resolved length.
	for name, value := range cs.Custom {
		put(name, value)
	}
	return out
}

// px renders a CSS pixel length the way a browser's computed style spells it:
// whole numbers without a fraction, and the unit always present because a
// computed length is never unitless.
func px(v float32) string {
	if v == 0 {
		return "0px"
	}
	f := float64(v)
	if f == float64(int64(f)) && f >= -1e9 && f <= 1e9 {
		return strconv.FormatInt(int64(f), 10) + "px"
	}
	return strconv.FormatFloat(f, 'f', -1, 32) + "px"
}

// formatCSSNumber renders a unitless number, trimming the trailing zeros Go's
// shortest representation would still print for a whole value.
func formatCSSNumber(v float64) string {
	if v == float64(int64(v)) && v >= -1e9 && v <= 1e9 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 32)
}

// cssColor renders a resolved colour the way getComputedStyle reports it: rgb()
// when the alpha channel is full, rgba() otherwise, and the two forms use the
// comma syntax browsers return rather than the modern CSS one.
func cssColor(c css.Color) string {
	if c.A == 255 {
		return "rgb(" + strconv.Itoa(int(c.R)) + ", " + strconv.Itoa(int(c.G)) + ", " + strconv.Itoa(int(c.B)) + ")"
	}
	return "rgba(" + strconv.Itoa(int(c.R)) + ", " + strconv.Itoa(int(c.G)) + ", " + strconv.Itoa(int(c.B)) + ", " + formatCSSNumber(float64(c.A)/255) + ")"
}

// The keyword tables below exist because internal/style's enums have no
// String(): the style package keeps values as compact integers for layout, and
// a page only ever sees the CSS spelling. Each maps the engine's zero value to
// the property's initial keyword, which is exactly how the resolver seeds an
// unset property.
func displayKeyword(d style.Display) string {
	switch d {
	case style.DisplayNone:
		return "none"
	case style.DisplayBlock:
		return "block"
	case style.DisplayInline:
		return "inline"
	case style.DisplayInlineBlock:
		return "inline-block"
	case style.DisplayFlex:
		return "flex"
	case style.DisplayInlineFlex:
		return "inline-flex"
	case style.DisplayListItem:
		return "list-item"
	case style.DisplayTable:
		return "table"
	case style.DisplayTableRow:
		return "table-row"
	case style.DisplayTableCell:
		return "table-cell"
	case style.DisplayTableRowGroup:
		return "table-row-group"
	case style.DisplayTableHeaderGroup:
		return "table-header-group"
	case style.DisplayTableFooterGroup:
		return "table-footer-group"
	case style.DisplayTableColumn:
		return "table-column"
	case style.DisplayTableColumnGroup:
		return "table-column-group"
	case style.DisplayTableCaption:
		return "table-caption"
	case style.DisplayGrid:
		return "grid"
	case style.DisplayInlineGrid:
		return "inline-grid"
	case style.DisplayContents:
		return "contents"
	}
	return ""
}

func positionKeyword(p style.Position) string {
	switch p {
	case style.PositionStatic:
		return "static"
	case style.PositionRelative:
		return "relative"
	case style.PositionAbsolute:
		return "absolute"
	case style.PositionFixed:
		return "fixed"
	case style.PositionSticky:
		return "sticky"
	}
	return ""
}

func floatKeyword(f style.Float) string {
	switch f {
	case style.FloatNone:
		return "none"
	case style.FloatLeft:
		return "left"
	case style.FloatRight:
		return "right"
	}
	return ""
}

func clearKeyword(c style.Clear) string {
	switch c {
	case style.ClearNone:
		return "none"
	case style.ClearLeft:
		return "left"
	case style.ClearRight:
		return "right"
	case style.ClearBoth:
		return "both"
	}
	return ""
}

func overflowKeyword(o style.Overflow) string {
	switch o {
	case style.OverflowVisible:
		return "visible"
	case style.OverflowHidden:
		return "hidden"
	case style.OverflowScroll:
		return "scroll"
	case style.OverflowAuto:
		return "auto"
	}
	return ""
}

func boxSizingKeyword(b style.BoxSizing) string {
	switch b {
	case style.BoxSizingContentBox:
		return "content-box"
	case style.BoxSizingBorderBox:
		return "border-box"
	}
	return ""
}

func textAlignKeyword(t style.TextAlign) string {
	switch t {
	case style.TextAlignLeft:
		return "left"
	case style.TextAlignRight:
		return "right"
	case style.TextAlignCenter:
		return "center"
	case style.TextAlignJustify:
		return "justify"
	}
	return ""
}

func whiteSpaceKeyword(w style.WhiteSpace) string {
	switch w {
	case style.WhiteSpaceNormal:
		return "normal"
	case style.WhiteSpaceNowrap:
		return "nowrap"
	case style.WhiteSpacePre:
		return "pre"
	case style.WhiteSpacePrewrite:
		return "pre-wrap"
	case style.WhiteSpacePreline:
		return "pre-line"
	}
	return ""
}

func fontStyleKeyword(f style.FontStyle) string {
	switch f {
	case style.FontStyleNormal:
		return "normal"
	case style.FontStyleItalic:
		return "italic"
	case style.FontStyleOblique:
		return "oblique"
	}
	return ""
}
