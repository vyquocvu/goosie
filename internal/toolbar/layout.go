package toolbar

import "github.com/vyquocvu/goosie/internal/frame"

const (
	ToolbarHeight = 40
	ButtonSize    = 32
	ButtonGap     = 4
	BarPadding    = 8
	// MinAddressWidth is the narrowest the address bar may become before the
	// toolbar collapses buttons into the main menu instead of shrinking it.
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

// Layout is the single source of truth for toolbar geometry at a given width.
// Buttons lists the visible buttons in paint order; Rects maps each visible
// button (always including ButtonMenu) to its rect. Address is the URL field.
type Layout struct {
	Buttons []Button
	Rects   map[Button]frame.Rect
	Address frame.Rect
	Menu    frame.Rect
}

// ComputeLayout lays the toolbar out for width. The menu button is always
// right-anchored. As width shrinks, Downloads collapses first, then Reload,
// Forward, Back — in that order — so the address bar keeps MinAddressWidth and
// no two controls overlap. Collapsed actions live in the main menu.
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
	// Richest first; Downloads collapses before any left button.
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
	// Degenerate width: keep menu and a non-inverted address, no buttons.
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

// ButtonRect returns the rect of a visible button, or the zero rect when the
// button is collapsed at this width. Callers that paint or hit-test should
// iterate ComputeLayout().Buttons rather than probe collapsed buttons.
func ButtonRect(btn Button, toolbarWidth int32) frame.Rect {
	return ComputeLayout(toolbarWidth).Rects[btn]
}

// AddressBarRect returns the address field rect for the width.
func AddressBarRect(toolbarWidth int32) frame.Rect {
	return ComputeLayout(toolbarWidth).Address
}
