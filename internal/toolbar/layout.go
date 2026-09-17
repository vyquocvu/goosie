package toolbar

import "github.com/vyquocvu/goosie/internal/frame"

const (
	ToolbarHeight = 40
	ButtonSize    = 32
	ButtonGap     = 4
	BarPadding    = 8
)

type Button int

const (
	ButtonBack Button = iota
	ButtonForward
	ButtonReload
	ButtonGo
)

func ButtonRect(btn Button, toolbarWidth int32) frame.Rect {
	x := int32(ButtonGap) + int32(btn)*(ButtonSize+ButtonGap)
	y := int32((ToolbarHeight - ButtonSize) / 2)
	return frame.Rect4(x, y, x+ButtonSize, y+ButtonSize)
}

func AddressBarRect(toolbarWidth int32) frame.Rect {
	x0 := int32(3*(ButtonSize+ButtonGap)) + ButtonGap
	x1 := toolbarWidth - ButtonSize - 2*ButtonGap
	y0 := int32((ToolbarHeight - ButtonSize) / 2)
	y1 := y0 + ButtonSize
	return frame.Rect4(x0, y0, x1, y1)
}
