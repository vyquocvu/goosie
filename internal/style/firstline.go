package style

import "github.com/vyquocvu/goosie/internal/css"

// filterFirstLineProperties resets fields in cs that CSS Selectors §7.2 does
// not allow on ::first-line. Only font properties, color, background,
// text-decoration, text-transform, letter-spacing, word-spacing, and
// line-height apply; everything else (margin, padding, border, display, etc.)
// is reverted to its default value so the originating element's style wins.
func filterFirstLineProperties(cs *ComputedStyle) {
	// Box model: not allowed.
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

	// Sizing: not allowed.
	cs.Width = 0
	cs.Height = 0
	cs.MinWidth = 0
	cs.MinHeight = 0
	cs.MaxWidth = 0
	cs.MaxHeight = 0

	// Layout: not allowed.
	cs.Display = DisplayInline
	cs.Position = PositionStatic
	cs.Float = FloatNone
	cs.Clear = ClearNone
	cs.Overflow = OverflowVisible
	cs.OverflowX = OverflowVisible
	cs.OverflowY = OverflowVisible

	// Positioning insets: not allowed.
	cs.Top = 0
	cs.Right = 0
	cs.Bottom = 0
	cs.Left = 0
	cs.HasTop = false
	cs.HasRight = false
	cs.HasBottom = false
	cs.HasLeft = false

	// Visual properties not in the allowed list.
	cs.Opacity = 0
	cs.ZIndex = 0
	cs.HasZIndex = false
	cs.HasClip = false
	cs.Visibility = ""

	// Background image layers are allowed; the colour is allowed.
	// Reset nothing in Background* fields.

	// Text properties: only the allowed ones survive.
	// TextDecoration, TextTransform, LetterSpacing, WordSpacing, LineHeight
	// are all allowed and kept as-is.
	// TextIndent is NOT in the allowed list for ::first-line.
	cs.TextIndent = 0
}
