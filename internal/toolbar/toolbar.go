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

// Special key codes matching the darwin shim's GOOSIE_KEY_* constants.
const (
	keyUp    = 0xF700
	keyDown  = 0xF701
	keyLeft  = 0xF702
	keyRight = 0xF703
	keyHome  = 0xF704
	keyEnd   = 0xF705
)

var (
	buttonColor    = frame.RGB(100, 100, 100)
	barBg          = frame.RGB(255, 255, 255)
	barBorder      = frame.RGB(200, 200, 200)
	toolbarBg      = frame.RGB(240, 240, 240)
	textColor      = frame.RGB(33, 33, 33)
	cursorColor    = frame.RGB(0, 120, 215)
	disabledColor  = frame.RGB(180, 180, 180)
	selectionColor = frame.RGB(180, 210, 250)
)

// Clipboard is the platform clipboard interface. The darwin package provides
// the real implementation; tests use nopClipboard.
type Clipboard interface {
	Read() string
	Write(string)
}

type nopClipboardImpl struct{}

func (nopClipboardImpl) Read() string      { return "" }
func (nopClipboardImpl) Write(string)      {}

type State struct {
	URL        string
	Input      string
	Cursor     int
	SelStart   int
	SelEnd     int
	Focus      Focus
	History    *History
	Loading    bool
	Error      string
	Bounds     frame.Rect
	OnNavigate func(string)
	OnTraverse func(int)
	OnReload   func()
	Clipboard  Clipboard

	fonts *raster.Fonts
}

func NewState(width int32, fonts *raster.Fonts) *State {
	return &State{
		History:   NewHistory(),
		Bounds:    frame.Rect4(0, 0, width, ToolbarHeight),
		fonts:     fonts,
		Clipboard: nopClipboardImpl{},
	}
}

func (s *State) SetBounds(width int32) {
	s.Bounds = frame.Rect4(0, 0, width, ToolbarHeight)
}

func (s *State) Navigate(url string) {
	s.URL = url
	s.Input = url
	s.Cursor = len([]rune(url))
	s.SelStart = s.Cursor
	s.SelEnd = s.Cursor
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
			s.clearSelection()
		}
		return
	}
	w := s.Bounds.W()
	switch {
	case rectContains(ButtonRect(ButtonBack, w), pos):
		if s.History.CanBack() && s.OnTraverse != nil {
			s.OnTraverse(-1)
		}
	case rectContains(ButtonRect(ButtonForward, w), pos):
		if s.History.CanForward() && s.OnTraverse != nil {
			s.OnTraverse(1)
		}
	case rectContains(ButtonRect(ButtonReload, w), pos):
		if s.OnReload != nil {
			s.OnReload()
		}
	case rectContains(AddressBarRect(w), pos):
		s.Focus = FocusAddress
		s.Cursor = len([]rune(s.Input))
		s.clearSelection()
	default:
		if s.Focus == FocusAddress {
			s.Focus = FocusNone
			s.clearSelection()
		}
	}
}

// HandleKey dispatches a key event with no modifier information. It exists
// for backward compatibility; HandleKeyEvent is the full entry point.
func (s *State) HandleKey(r rune) {
	s.HandleKeyEvent(r, 0)
}

// HandleKeyEvent dispatches a key event with modifier flags.
func (s *State) HandleKeyEvent(key rune, mods surface.KeyMod) {
	if s.Focus != FocusAddress {
		return
	}

	cmd := mods&surface.ModCommand != 0
	ctrl := mods&surface.ModControl != 0
	shift := mods&surface.ModShift != 0
	opt := mods&surface.ModOption != 0

	// Handle special keys before backward compat conversion.
	switch key {
	case '\r', '\n':
		if !cmd && !ctrl {
			s.submitInput()
		}
		return
	case 0x7f, '\b':
		s.DeleteBackward()
		return
	}

	// Backward compat: bare control codes (1-26) without modifier flags are
	// treated as Ctrl+letter from the old key path.
	if !cmd && !shift && !opt && key >= 1 && key <= 26 {
		ctrl = true
		key = 'a' + (key - 1) % 26
	}

	switch key {
	case keyLeft:
		if cmd {
			s.moveCursorStart(shift)
		} else if opt {
			s.moveWordLeft(shift)
		} else {
			s.moveCursorLeft(shift)
		}
	case keyRight:
		if cmd {
			s.moveCursorEnd(shift)
		} else if opt {
			s.moveWordRight(shift)
		} else {
			s.moveCursorRight(shift)
		}
	case keyUp:
		s.moveCursorStart(shift)
	case keyDown:
		s.moveCursorEnd(shift)
	case keyHome:
		s.moveCursorStart(shift)
	case keyEnd:
		s.moveCursorEnd(shift)

	case 'a':
		if cmd {
			s.SelectAll()
		} else if ctrl {
			s.moveCursorStart(shift)
		} else {
			s.insertChar(key)
		}
	case 'c':
		if cmd {
			s.Copy()
		} else {
			s.insertChar(key)
		}
	case 'v':
		if cmd {
			s.Paste()
		} else {
			s.insertChar(key)
		}
	case 'x':
		if cmd {
			s.Cut()
		} else {
			s.insertChar(key)
		}
	case 'z':
		if cmd {
			// Undo: no-op for now
		} else {
			s.insertChar(key)
		}

	default:
		if ctrl {
			switch key {
			case 'e':
				s.moveCursorEnd(shift)
			case 'b':
				s.moveCursorLeft(shift)
			case 'f':
				s.moveCursorRight(shift)
			case 'k':
				s.Input = s.Input[:s.cursorByte()]
				s.Cursor = len([]rune(s.Input))
				s.clearSelection()
			case 'd':
				s.DeleteForward()
			case 'w':
				s.deleteWordBackward()
			}
		} else if key >= 0x20 {
			s.insertChar(key)
		}
	}
}

func (s *State) insertChar(r rune) {
	s.deleteSelection()
	s.InsertRune(r)
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

func (s *State) hasSelection() bool {
	return s.SelStart != s.SelEnd
}

func (s *State) selMin() int {
	if s.SelStart < s.SelEnd {
		return s.SelStart
	}
	return s.SelEnd
}

func (s *State) selMax() int {
	if s.SelStart > s.SelEnd {
		return s.SelStart
	}
	return s.SelEnd
}

func (s *State) clearSelection() {
	s.SelStart = s.Cursor
	s.SelEnd = s.Cursor
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

	// Draw selection highlight behind text.
	if s.Focus == FocusAddress && s.hasSelection() {
		runes := []rune(text)
		sMin := s.selMin()
		sMax := s.selMax()
		if sMin < 0 {
			sMin = 0
		}
		if sMax > len(runes) {
			sMax = len(runes)
		}
		if sMin < sMax {
			selText := string(runes[sMin:sMax])
			selX0 := textX + s.measureText(string(runes[:sMin]))
			selX1 := selX0 + s.measureText(selText)
			backing.FillRect(frame.Rect4(selX0, r.Y0+2, selX1, r.Y1-2), selectionColor, nil)
		}
	}

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
	// The zero slot is the embedded Go face: the chrome draws the same type it
	// always has, whatever fonts the host has.
	for _, rn := range text {
		g := s.fonts.Glyph(textSize, rn, frame.FontSlot{})
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
		w += s.fonts.GlyphAdvance(textSize, rn, frame.FontSlot{})
	}
	return w
}
