package style

import (
	"reflect"

	"github.com/vyquocvu/goosie/internal/css"
)

const (
	firstLineFontFamily uint8 = 1 << iota
	firstLineFontSize
	firstLineFontWeight
	firstLineFontStyle
	firstLineLineHeight
	firstLineLetterSpacing
	firstLineWordSpacing
)

const firstLineFontShorthandMask = firstLineFontFamily | firstLineFontSize | firstLineFontWeight | firstLineFontStyle | firstLineLineHeight

func (cs *ComputedStyle) FirstLineSpecified() uint8 {
	if cs == nil {
		return 0
	}
	return cs.firstLineSpecified
}

func firstLineLonghandMask(prop string) uint8 {
	switch prop {
	case "font-family":
		return firstLineFontFamily
	case "font-size":
		return firstLineFontSize
	case "font-weight":
		return firstLineFontWeight
	case "font-style":
		return firstLineFontStyle
	case "line-height":
		return firstLineLineHeight
	case "letter-spacing":
		return firstLineLetterSpacing
	case "word-spacing":
		return firstLineWordSpacing
	default:
		return 0
	}
}

func applyFirstLineInitial(cs *ComputedStyle, prop string) {
	initial := DefaultStyle()
	switch prop {
	case "font-family":
		cs.FontFamily, cs.CustomFamily, cs.CustomFontIdx = initial.FontFamily, initial.CustomFamily, initial.CustomFontIdx
	case "font-size":
		cs.FontSize = initial.FontSize
	case "font-weight":
		cs.FontWeight = initial.FontWeight
	case "font-style":
		cs.FontStyle = initial.FontStyle
	case "line-height":
		cs.LineHeight, cs.LineHeightRatio, cs.LineHeightEm = initial.LineHeight, initial.LineHeightRatio, initial.LineHeightEm
	case "letter-spacing":
		cs.LetterSpacing = initial.LetterSpacing
	case "word-spacing":
		cs.WordSpacing = initial.WordSpacing
	}
}

func FirstLineMetrics(base, firstLine *ComputedStyle) *ComputedStyle {
	if base == nil || firstLine == nil {
		return base
	}
	mask := firstLine.FirstLineSpecified() &^ base.FirstLineSpecified()
	if mask == 0 {
		return base
	}
	effective := *base
	if mask&firstLineFontFamily != 0 {
		effective.FontFamily = firstLine.FontFamily
		effective.CustomFamily = firstLine.CustomFamily
		effective.CustomFontIdx = firstLine.CustomFontIdx
	}
	if mask&firstLineFontSize != 0 {
		effective.FontSize = firstLine.FontSize
	}
	if mask&firstLineFontWeight != 0 {
		effective.FontWeight = firstLine.FontWeight
	}
	if mask&firstLineFontStyle != 0 {
		effective.FontStyle = firstLine.FontStyle
	}
	if mask&firstLineLineHeight != 0 {
		effective.LineHeight = firstLine.LineHeight
		effective.LineHeightRatio = firstLine.LineHeightRatio
		effective.LineHeightEm = firstLine.LineHeightEm
		if effective.FontSize != firstLine.FontSize {
			if effective.LineHeightRatio > 0 {
				effective.LineHeight = effective.LineHeightRatio * effective.FontSize
			}
			if effective.LineHeightEm > 0 {
				effective.LineHeight = effective.LineHeightEm * effective.FontSize
			}
		}
	}
	if mask&firstLineLetterSpacing != 0 {
		effective.LetterSpacing = firstLine.LetterSpacing
	}
	if mask&firstLineWordSpacing != 0 {
		effective.WordSpacing = firstLine.WordSpacing
	}
	if sameFirstLineMetrics(effective, *base) {
		return base
	}
	return &effective
}

func sameFirstLineMetrics(a, b ComputedStyle) bool {
	return a.FontFamily == b.FontFamily && a.CustomFamily == b.CustomFamily && a.CustomFontIdx == b.CustomFontIdx &&
		a.FontSize == b.FontSize && a.FontWeight == b.FontWeight && a.FontStyle == b.FontStyle &&
		a.LineHeight == b.LineHeight && a.LineHeightRatio == b.LineHeightRatio && a.LineHeightEm == b.LineHeightEm &&
		a.LetterSpacing == b.LetterSpacing && a.WordSpacing == b.WordSpacing
}

func FirstLinePaint(base, firstLine *ComputedStyle) *ComputedStyle {
	if base == nil || firstLine == nil || sameFirstLinePaint(*base, *firstLine) {
		return base
	}
	effective := *base
	effective.Color = firstLine.Color
	effective.BackgroundColor = firstLine.BackgroundColor
	effective.BackgroundGradient = firstLine.BackgroundGradient
	effective.BackgroundRadialGradient = firstLine.BackgroundRadialGradient
	effective.BackgroundImage = firstLine.BackgroundImage
	effective.BackgroundRepeat = firstLine.BackgroundRepeat
	effective.BackgroundRepeatY = firstLine.BackgroundRepeatY
	effective.BackgroundSize = firstLine.BackgroundSize
	effective.BackgroundOrigin = firstLine.BackgroundOrigin
	effective.BackgroundClip = firstLine.BackgroundClip
	effective.BgSizeW = firstLine.BgSizeW
	effective.BgSizeH = firstLine.BgSizeH
	effective.BgSizeWPct = firstLine.BgSizeWPct
	effective.BgSizeHPct = firstLine.BgSizeHPct
	effective.BackgroundPosXMode = firstLine.BackgroundPosXMode
	effective.BackgroundPosYMode = firstLine.BackgroundPosYMode
	effective.BackgroundPosX = firstLine.BackgroundPosX
	effective.BackgroundPosY = firstLine.BackgroundPosY
	effective.BgPosXPct = firstLine.BgPosXPct
	effective.BgPosYPct = firstLine.BgPosYPct
	effective.TextDecoration = firstLine.TextDecoration
	effective.TextDecorationStyle = firstLine.TextDecorationStyle
	effective.TextDecorationColor = firstLine.TextDecorationColor
	effective.TextDecorationThickness = firstLine.TextDecorationThickness
	effective.TextUnderlineOffset = firstLine.TextUnderlineOffset
	effective.TextShadow = firstLine.TextShadow
	effective.TextTransform = firstLine.TextTransform
	return &effective
}

func sameFirstLinePaint(a, b ComputedStyle) bool {
	return a.Color == b.Color && a.BackgroundColor == b.BackgroundColor &&
		reflect.DeepEqual(a.BackgroundGradient, b.BackgroundGradient) &&
		reflect.DeepEqual(a.BackgroundRadialGradient, b.BackgroundRadialGradient) &&
		a.BackgroundImage == b.BackgroundImage && a.BackgroundRepeat == b.BackgroundRepeat && a.BackgroundRepeatY == b.BackgroundRepeatY &&
		a.BackgroundSize == b.BackgroundSize && a.BackgroundOrigin == b.BackgroundOrigin && a.BackgroundClip == b.BackgroundClip &&
		a.BgSizeW == b.BgSizeW && a.BgSizeH == b.BgSizeH && a.BgSizeWPct == b.BgSizeWPct && a.BgSizeHPct == b.BgSizeHPct &&
		a.BackgroundPosXMode == b.BackgroundPosXMode && a.BackgroundPosYMode == b.BackgroundPosYMode &&
		a.BackgroundPosX == b.BackgroundPosX && a.BackgroundPosY == b.BackgroundPosY && a.BgPosXPct == b.BgPosXPct && a.BgPosYPct == b.BgPosYPct &&
		a.TextDecoration == b.TextDecoration && a.TextDecorationStyle == b.TextDecorationStyle &&
		a.TextDecorationColor == b.TextDecorationColor && a.TextDecorationThickness == b.TextDecorationThickness &&
		a.TextUnderlineOffset == b.TextUnderlineOffset && reflect.DeepEqual(a.TextShadow, b.TextShadow) && a.TextTransform == b.TextTransform
}

func filterFirstLineProperties(cs *ComputedStyle) {
	cs.MarginTop = 0
	cs.MarginRight = 0
	cs.MarginBottom = 0
	cs.MarginLeft = 0
	cs.MarginLeftAuto = false
	cs.MarginRightAuto = false
	cs.PaddingTop = 0
	cs.PaddingRight = 0
	cs.PaddingBottom = 0
	cs.PaddingLeft = 0
	cs.BorderTopWidth = 0
	cs.BorderRightWidth = 0
	cs.BorderBottomWidth = 0
	cs.BorderLeftWidth = 0
	cs.BorderTopStyle = ""
	cs.BorderRightStyle = ""
	cs.BorderBottomStyle = ""
	cs.BorderLeftStyle = ""
	cs.BorderTopColor = css.Color{}
	cs.BorderRightColor = css.Color{}
	cs.BorderBottomColor = css.Color{}
	cs.BorderLeftColor = css.Color{}
	cs.BorderRadius = [4][2]float32{}
	cs.BorderImage = css.BorderImage{}

	cs.Width = 0
	cs.Height = 0
	cs.MinWidth = 0
	cs.MinHeight = 0
	cs.MaxWidth = 0
	cs.MaxHeight = 0

	cs.Display = DisplayInline
	cs.Position = PositionStatic
	cs.Float = FloatNone
	cs.Clear = ClearNone
	cs.Overflow = OverflowVisible
	cs.OverflowX = OverflowVisible
	cs.OverflowY = OverflowVisible

	cs.Top = 0
	cs.Right = 0
	cs.Bottom = 0
	cs.Left = 0
	cs.HasTop = false
	cs.HasRight = false
	cs.HasBottom = false
	cs.HasLeft = false

	cs.Opacity = 0
	cs.ZIndex = 0
	cs.HasZIndex = false
	cs.HasClip = false
	cs.Visibility = ""

	cs.TextIndent = 0
}
