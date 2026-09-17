package toolbar

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

type Focus int

const (
	FocusNone Focus = iota
	FocusAddress
)

const textSize = 14

var (
	buttonColor = frame.RGB(100, 100, 100)
	barBg       = frame.RGB(255, 255, 255)
	barBorder   = frame.RGB(200, 200, 200)
	toolbarBg   = frame.RGB(240, 240, 240)
	textColor   = frame.RGB(33, 33, 33)
	cursorColor = frame.RGB(0, 120, 215)
	disabledColor = frame.RGB(180, 180, 180)
)

type State struct {
	URL        string
	Input      string
	Cursor     int
	Focus      Focus
	History    *History
	Loading    bool
	Bounds     frame.Rect
	OnNavigate func(string)

	fonts *raster.Fonts
}

func NewState(width int32, fonts *raster.Fonts) *State {
	return &State{
		History: NewHistory(),
		Bounds:  frame.Rect4(0, 0, width, ToolbarHeight),
		fonts:   fonts,
	}
}

func (s *State) SetBounds(width int32) {
	s.Bounds = frame.Rect4(0, 0, width, ToolbarHeight)
}

func (s *State) Navigate(url string) {
	s.URL = url
	s.Input = url
	s.Cursor = len([]rune(url))
	s.History.Push(url)
	s.Focus = FocusNone
}

func (s *State) SetLoading(loading bool) {
	s.Loading = loading
}

func (s *State) HandleClick(pos frame.Point, button surface.Button) {
	if button != surface.ButtonLeft {
		return
	}
	if pos.Y >= ToolbarHeight {
		if s.Focus == FocusAddress {
			s.Focus = FocusNone
		}
		return
	}
	w := s.Bounds.W()
	switch {
	case rectContains(ButtonRect(ButtonBack, w), pos):
		if url, ok := s.History.Back(); ok && s.OnNavigate != nil {
			s.OnNavigate(url)
		}
	case rectContains(ButtonRect(ButtonForward, w), pos):
		if url, ok := s.History.Forward(); ok && s.OnNavigate != nil {
			s.OnNavigate(url)
		}
	case rectContains(ButtonRect(ButtonReload, w), pos):
		if s.URL != "" && s.OnNavigate != nil {
			s.OnNavigate(s.URL)
		}
	case rectContains(AddressBarRect(w), pos):
		s.Focus = FocusAddress
		s.Cursor = len([]rune(s.Input))
	default:
		if s.Focus == FocusAddress {
			s.Focus = FocusNone
		}
	}
}

func (s *State) HandleKey(r rune) {
	if s.Focus != FocusAddress {
		return
	}
	switch r {
	case '\r':
		s.submitInput()
	case 0x7f, '\b':
		s.DeleteBackward()
	case 0x01:
		s.MoveCursorStart()
	case 0x05:
		s.MoveCursorEnd()
	case 0x02:
		s.MoveCursorLeft()
	case 0x06:
		s.MoveCursorRight()
	case 0x0b:
		s.Input = s.Input[:s.cursorByte()]
		s.Cursor = len([]rune(s.Input))
	default:
		if r >= 0x20 {
			s.InsertRune(r)
		}
	}
}

func (s *State) submitInput() {
	url := s.Input
	if url == "" {
		return
	}
	if s.OnNavigate != nil {
		s.OnNavigate(url)
	}
}

func (s *State) cursorByte() int {
	runes := []rune(s.Input)
	if s.Cursor >= len(runes) {
		return len(s.Input)
	}
	return len(string(runes[:s.Cursor]))
}

func rectContains(r frame.Rect, p frame.Point) bool {
	return p.X >= r.X0 && p.X < r.X1 && p.Y >= r.Y0 && p.Y < r.Y1
}

func (s *State) Draw(backing *frame.Bitmap) {
	if backing == nil || backing.Empty() {
		return
	}
	w := int32(backing.W)
	clip := backing.Bounds()

	bg := frame.Rect4(0, 0, w, ToolbarHeight)
	backing.FillRect(bg, toolbarBg, nil)

	s.drawButton(backing, ButtonBack, w, clip)
	s.drawButton(backing, ButtonForward, w, clip)
	s.drawButton(backing, ButtonReload, w, clip)
	s.drawAddressBar(backing, w, clip)
}

func (s *State) drawButton(backing *frame.Bitmap, btn Button, toolbarW int32, clip frame.Rect) {
	r := ButtonRect(btn, toolbarW)
	fg := buttonColor
	if btn == ButtonBack && !s.History.CanBack() {
		fg = disabledColor
	}
	if btn == ButtonForward && !s.History.CanForward() {
		fg = disabledColor
	}
	cx := (r.X0 + r.X1) / 2
	cy := (r.Y0 + r.Y1) / 2
	switch btn {
	case ButtonBack:
		s.drawArrow(backing, cx-4, cy, -1, fg, clip)
	case ButtonForward:
		s.drawArrow(backing, cx+4, cy, 1, fg, clip)
	case ButtonReload:
		s.drawReload(backing, cx, cy, fg, clip)
	}
}

func (s *State) drawArrow(backing *frame.Bitmap, x, y int32, dir int32, c frame.Color, clip frame.Rect) {
	for i := int32(0); i < 6; i++ {
		px := x + dir*i
		py := y - 3 + i
		if py >= 0 {
			backing.FillRect(frame.Rect4(px, py, px+1, py+1), c, nil)
		}
		py2 := y + 3 - i
		if py2 >= 0 {
			backing.FillRect(frame.Rect4(px, py2, px+1, py2+1), c, nil)
		}
	}
	_ = clip
}

func (s *State) drawReload(backing *frame.Bitmap, cx, cy int32, c frame.Color, clip frame.Rect) {
	r := int32(5)
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			dist := dx*dx + dy*dy
			if dist >= (r-1)*(r-1) && dist <= (r+1)*(r+1) {
				px := cx + dx
				py := cy + dy
				if px >= 0 && py >= 0 {
					backing.FillRect(frame.Rect4(px, py, px+1, py+1), c, nil)
				}
			}
		}
	}
	arrowX := cx + r
	arrowY := cy - r
	backing.FillRect(frame.Rect4(arrowX-1, arrowY, arrowX+2, arrowY+1), c, nil)
	backing.FillRect(frame.Rect4(arrowX+1, arrowY, arrowX+2, arrowY+3), c, nil)
	_ = clip
}

func (s *State) drawAddressBar(backing *frame.Bitmap, toolbarW int32, clip frame.Rect) {
	r := AddressBarRect(toolbarW)
	backing.FillRect(r, barBg, nil)

	border := frame.RGB(200, 200, 200)
	backing.FillRect(frame.Rect4(r.X0, r.Y0, r.X1, r.Y0+1), border, nil)
	backing.FillRect(frame.Rect4(r.X0, r.Y1-1, r.X1, r.Y1), border, nil)
	backing.FillRect(frame.Rect4(r.X0, r.Y0, r.X0+1, r.Y1), border, nil)
	backing.FillRect(frame.Rect4(r.X1-1, r.Y0, r.X1, r.Y1), border, nil)

	text := s.Input
	if s.Focus == FocusNone {
		text = s.URL
	}
	if text == "" && s.Focus == FocusNone {
		text = "Enter URL..."
	}

	textX := r.X0 + BarPadding
	textY := r.Y0 + (ButtonSize-textSize)/2 + textSize
	s.drawText(backing, text, textX, textY, textColor, clip)

	if s.Focus == FocusAddress {
		cursorX := textX + s.measureText(text[:s.cursorByte()])
		backing.FillRect(frame.Rect4(cursorX, r.Y0+4, cursorX+1, r.Y1-4), cursorColor, nil)
	}
}

func (s *State) drawText(backing *frame.Bitmap, text string, x, y int32, c frame.Color, clip frame.Rect) {
	if s.fonts == nil {
		return
	}
	penX := x
	for _, rn := range text {
		g := s.fonts.Glyph(textSize, rn)
		if !g.Ok || g.Mask == nil {
			penX += g.Advance
			continue
		}
		origin := frame.Point{X: penX + g.Bounds.X0, Y: y - textSize + g.Bounds.Y0}
		backing.BlitMask(g.Mask, origin, c, clip, nil)
		penX += g.Advance
	}
}

func (s *State) measureText(text string) int32 {
	if s.fonts == nil {
		return int32(len(text)) * 7
	}
	var w int32
	for _, rn := range text {
		w += s.fonts.GlyphAdvance(textSize, rn)
	}
	return w
}
