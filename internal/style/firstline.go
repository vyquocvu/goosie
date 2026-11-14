package style

import "github.com/vyquocvu/goosie/internal/css"

func filterFirstLineProperties(cs *ComputedStyle) {
	cs.MarginTop = 0
	cs.MarginRight = 0
	cs.MarginBottom = 0
	cs.MarginLeft = 0
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
