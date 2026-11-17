package engine

import (
	"strconv"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func (s *Session) computedStyleValues(n *dom.Node) map[string]string {
	if s == nil || n == nil {
		return nil
	}
	var cs *style.ComputedStyle
	var boxW, boxH float32
	hasBox := false
	if n.Doc == s.Doc {
		s.refreshStylesForCSSOMRead()
		s.opMu.Lock()
		cs = s.Styles[n.ID]
		if s.Arena != nil && cs != nil {
			if id, ok := s.Arena.ForNode(n.ID); ok {
				if o := s.Arena.Get(id); o != nil {
					boxW, boxH, hasBox = o.W, o.H, true
				}
			}
		}
		s.opMu.Unlock()
	} else if n.Doc != nil {
		sheets, err := s.collectSheets(n.Doc, nil)
		if err != nil {
			return nil
		}
		cs = style.NewResolver(sheets).ResolveViewport(n.Doc, s.styleViewport(), s.customFonts)[n.ID]
	}
	if cs == nil {
		return nil
	}
	return styleValuesFrom(cs, boxW, boxH, hasBox)
}

func styleValuesFrom(cs *style.ComputedStyle, boxW, boxH float32, hasBox bool) map[string]string {
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
	if cs.BorderTopStyle == cs.BorderRightStyle && cs.BorderTopStyle == cs.BorderBottomStyle && cs.BorderTopStyle == cs.BorderLeftStyle {
		put("border-style", cs.BorderTopStyle)
	}
	put("border-right-style", cs.BorderRightStyle)
	put("border-bottom-style", cs.BorderBottomStyle)
	put("border-left-style", cs.BorderLeftStyle)
	put("border-top-color", cssColor(cs.BorderTopColor))
	put("border-right-color", cssColor(cs.BorderRightColor))
	put("border-bottom-color", cssColor(cs.BorderBottomColor))
	put("border-left-color", cssColor(cs.BorderLeftColor))

	for name, value := range cs.Custom {
		put(name, value)
	}
	return out
}

func (s *Session) pseudoStyleValues(n *dom.Node, pseudo string) map[string]string {
	if s == nil || n == nil || pseudo == "" || n.Doc != s.Doc {
		return nil
	}
	s.refreshStylesForCSSOMRead()
	cs, ok := s.pseudoStyleForRead(n, pseudo)
	if !ok {
		return nil
	}
	values := styleValuesFrom(cs, 0, 0, false)
	if values == nil {
		return nil
	}
	if cs.TextTransform != "" {
		values["text-transform"] = cs.TextTransform
	}
	if text := style.PseudoContentValue(pseudo, cs); text != "" {
		values["content"] = text
	}
	if _, ok := values["width"]; !ok {
		values["width"] = "auto"
	}
	if _, ok := values["height"]; !ok {
		values["height"] = "auto"
	}
	if pseudo == "first-line" {
		values["width"] = "auto"
		values["height"] = "auto"
		values["opacity"] = "1"
		values["visibility"] = "visible"
	}
	return values
}

func (s *Session) pseudoStyleForRead(n *dom.Node, pseudo string) (*style.ComputedStyle, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if cs := s.PseudoStyles[style.PseudoKey{NodeID: n.ID, Pseudo: pseudo}]; cs != nil {
		return cs, true
	}
	return s.styleResolver.PseudoStyleForRead(n, pseudo, s.Styles, s.customFonts)
}

func (s *Session) refreshStylesForCSSOMRead() {
	if s == nil || !s.styleDirty.Swap(false) {
		return
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.styleResolver == nil || s.Doc == nil {
		return
	}
	styles := s.styleResolver.ResolveViewport(s.Doc, s.styleViewport(), s.customFonts)
	s.Styles = styles
	s.workCounts.styleResolves.Add(1)
	s.workCounts.styleNodeVisits.Add(uint64(len(styles)))
	s.PseudoStyles = s.styleResolver.ResolvePseudoElements(s.Doc, styles, s.customFonts)
}

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

func formatCSSNumber(v float64) string {
	if v == float64(int64(v)) && v >= -1e9 && v <= 1e9 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 32)
}

func cssColor(c css.Color) string {
	if c.A == 255 {
		return "rgb(" + strconv.Itoa(int(c.R)) + ", " + strconv.Itoa(int(c.G)) + ", " + strconv.Itoa(int(c.B)) + ")"
	}
	return "rgba(" + strconv.Itoa(int(c.R)) + ", " + strconv.Itoa(int(c.G)) + ", " + strconv.Itoa(int(c.B)) + ", " + formatCSSNumber(float64(c.A)/255) + ")"
}

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
	case style.DisplayInlineTable:
		return "inline-table"
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
	case style.TextAlignStart:
		return "start"
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
