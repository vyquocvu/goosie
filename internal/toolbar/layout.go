package toolbar

import "github.com/vyquocvu/goosie/internal/frame"

const (
	ToolbarHeight   = 40
	ButtonSize      = 32
	ButtonGap       = 4
	BarPadding      = 8
	MinAddressWidth = 64
)

type Button int

const (
	ButtonBack Button = iota
	ButtonForward
	ButtonReload
	ButtonDownloads
	ButtonMenu
	ButtonGo = ButtonDownloads
)

type Layout struct {
	Buttons []Button
	Rects   map[Button]frame.Rect
	Address frame.Rect
	Menu    frame.Rect
}

func ComputeLayout(width int32) Layout {
	y := int32((ToolbarHeight - ButtonSize) / 2)
	box := func(x0 int32) frame.Rect { return frame.Rect4(x0, y, x0+ButtonSize, y+ButtonSize) }
	menuX0 := width - ButtonSize - ButtonGap
	if menuX0 < 0 {
		menuX0 = 0
	}
	menu := box(menuX0)

	leftOrder := []Button{ButtonBack, ButtonForward, ButtonReload}
	type cand struct {
		left      int
		downloads bool
	}
	cands := []cand{{3, true}, {3, false}, {2, false}, {1, false}, {0, false}}
	for _, c := range cands {
		addrX1 := menuX0 - ButtonGap
		if c.downloads {
			addrX1 = menuX0 - ButtonSize - 2*ButtonGap
		}
		addrX0 := ButtonGap + int32(c.left)*(ButtonSize+ButtonGap)
		if addrX1-addrX0 < MinAddressWidth {
			continue
		}
		l := Layout{Rects: make(map[Button]frame.Rect), Menu: menu}
		for i := 0; i < c.left; i++ {
			b := leftOrder[i]
			l.Buttons = append(l.Buttons, b)
			l.Rects[b] = box(ButtonGap + int32(i)*(ButtonSize+ButtonGap))
		}
		l.Address = frame.Rect4(addrX0, y, addrX1, y+ButtonSize)
		if c.downloads {
			l.Buttons = append(l.Buttons, ButtonDownloads)
			l.Rects[ButtonDownloads] = box(menuX0 - ButtonSize - ButtonGap)
		}
		l.Buttons = append(l.Buttons, ButtonMenu)
		l.Rects[ButtonMenu] = menu
		return l
	}
	addrX1 := menuX0 - ButtonGap
	if addrX1 < ButtonGap+1 {
		addrX1 = ButtonGap + 1
	}
	return Layout{
		Buttons: []Button{ButtonMenu},
		Rects:   map[Button]frame.Rect{ButtonMenu: menu},
		Address: frame.Rect4(ButtonGap, y, addrX1, y+ButtonSize),
		Menu:    menu,
	}
}

func ButtonRect(btn Button, toolbarWidth int32) frame.Rect {
	return ComputeLayout(toolbarWidth).Rects[btn]
}

func AddressBarRect(toolbarWidth int32) frame.Rect {
	return ComputeLayout(toolbarWidth).Address
}
