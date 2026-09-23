package toolbar

import (
	"fmt"
	"strings"

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
	buttonColor    = frame.RGB(80, 80, 80)
	buttonBg       = frame.RGB(232, 232, 232)
	barBg          = frame.RGB(255, 255, 255)
	barBorder      = frame.RGB(210, 210, 210)
	barFocusBorder = frame.RGB(0, 120, 215)
	toolbarBg      = frame.RGB(245, 245, 245)
	textColor      = frame.RGB(33, 33, 33)
	placeholderColor = frame.RGB(150, 150, 150)
	cursorColor    = frame.RGB(0, 120, 215)
	disabledColor  = frame.RGB(190, 190, 190)
	selectionColor = frame.RGB(180, 210, 250)
	separatorColor = frame.RGB(200, 200, 200)
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
	FindActive bool
	FindQuery  string
	FindTotal  int
	FindIndex  int
	Bounds     frame.Rect
	OnNavigate   func(string)
	OnTraverse   func(int)
	OnReload     func()
	OnFindChanged func(string)
	OnFindNext    func(bool)
	OnFindClose   func()
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

func (s *State) SetHistory(h *History) {
	s.History = h
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

// OpenFind reuses the address bar as the find field, restoring the previous
// query. Total and index reset until the host recomputes them.
func (s *State) OpenFind() {
	if s.FindActive {
		return
	}
	s.FindActive = true
	s.FindTotal = 0
	s.FindIndex = 0
	s.Focus = FocusAddress
	s.Input = s.FindQuery
	s.Cursor = len([]rune(s.Input))
	s.SelStart = s.Cursor
	s.SelEnd = s.Cursor
}

// CloseFind restores the address bar to the current URL and keeps the query
// for the next OpenFind.
func (s *State) CloseFind() {
	if !s.FindActive {
		return
	}
	s.FindActive = false
	s.FindQuery = s.Input
	s.FindTotal = 0
	s.FindIndex = 0
	s.Input = s.URL
	s.Cursor = len([]rune(s.Input))
	s.SelStart = s.Cursor
	s.SelEnd = s.Cursor
	s.Focus = FocusNone
}

func (s *State) HandleClick(pos frame.Point, button surface.Button) {
	if button != surface.ButtonLeft {
		return
	}
	if pos.Y >= ToolbarHeight {
		if s.FindActive {
			s.CloseFind()
			if s.OnFindClose != nil {
				s.OnFindClose()
			}
			return
		}
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
		if s.FindActive {
			s.CloseFind()
			if s.OnFindClose != nil {
				s.OnFindClose()
			}
			return
		}
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
	if s.FindActive {
		s.handleFindKey(key, mods)
		return
	}
	if s.Focus != FocusAddress {
		return
	}
	if key == '\r' || key == '\n' {
		if mods&(surface.ModCommand|surface.ModControl) == 0 {
			s.submitInput()
		}
		return
	}
	s.editKey(key, mods)
}

// handleFindKey edits the find query. Enter steps to the next match,
// Shift+Enter to the previous, Esc closes, and any other edit reports the new
// query to the host.
func (s *State) handleFindKey(key rune, mods surface.KeyMod) {
	switch key {
	case 0x1b:
		s.CloseFind()
		if s.OnFindClose != nil {
			s.OnFindClose()
		}
		return
	case '\r', '\n':
		if s.OnFindNext != nil {
			s.OnFindNext(mods&surface.ModShift != 0)
		}
		return
	}
	before := s.Input
	s.editKey(key, mods)
	if s.Input != before {
		s.FindQuery = s.Input
		if s.OnFindChanged != nil {
			s.OnFindChanged(s.Input)
		}
	}
}

// editKey applies address-bar editing for one key event; it is shared between
// URL entry and find mode.
func (s *State) editKey(key rune, mods surface.KeyMod) {
	cmd := mods&surface.ModCommand != 0
	ctrl := mods&surface.ModControl != 0
	shift := mods&surface.ModShift != 0
	opt := mods&surface.ModOption != 0

	if key == 0x7f || key == '\b' {
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

func (s *State) Draw(backing *frame.Bitmap, oy int32) {
	if backing == nil || backing.Empty() {
		return
	}
	w := int32(backing.W)
	clip := backing.Bounds()

	bg := frame.Rect4(0, oy, w, oy+ToolbarHeight)
	backing.FillRect(bg, toolbarBg, nil)

	backing.FillRect(frame.Rect4(0, oy+ToolbarHeight-1, w, oy+ToolbarHeight), separatorColor, nil)

	s.drawButton(backing, ButtonBack, w, clip, oy)
	s.drawButton(backing, ButtonForward, w, clip, oy)
	s.drawButton(backing, ButtonReload, w, clip, oy)
	s.drawAddressBar(backing, w, clip, oy)

	if s.Loading {
		progressColor := frame.RGB(70, 140, 220)
		lineY := oy + ToolbarHeight - 3
		backing.FillRect(frame.Rect4(0, lineY, w, lineY+2), progressColor, nil)
	}
}

func (s *State) drawButton(backing *frame.Bitmap, btn Button, toolbarW int32, clip frame.Rect, oy int32) {
	r := ButtonRect(btn, toolbarW)
	r = frame.Rect4(r.X0, r.Y0+oy, r.X1, r.Y1+oy)
	fg := buttonColor
	if btn == ButtonBack && !s.History.CanBack() {
		fg = disabledColor
	}
	if btn == ButtonForward && !s.History.CanForward() {
		fg = disabledColor
	}
	cx := (r.X0 + r.X1) / 2
	cy := (r.Y0 + r.Y1) / 2
	radius := int32(13)
	s.drawCircle(backing, cx, cy, radius, buttonBg)
	switch btn {
	case ButtonBack:
		s.drawArrow(backing, cx-4, cy, -1, fg, clip)
	case ButtonForward:
		s.drawArrow(backing, cx+4, cy, 1, fg, clip)
	case ButtonReload:
		s.drawReload(backing, cx, cy, fg, clip)
	}
}

func (s *State) drawCircle(backing *frame.Bitmap, cx, cy, radius int32, c frame.Color) {
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= radius*radius {
				px := cx + dx
				py := cy + dy
				backing.FillRect(frame.Rect4(px, py, px+1, py+1), c, nil)
			}
		}
	}
}

func (s *State) drawArrow(backing *frame.Bitmap, x, y int32, dir int32, c frame.Color, clip frame.Rect) {
	for i := int32(0); i < 5; i++ {
		px := x + dir*i
		py := y - 4 + i
		if py >= 0 {
			backing.FillRect(frame.Rect4(px, py, px+1, py+1), c, nil)
			backing.FillRect(frame.Rect4(px, py+1, px+1, py+2), c, nil)
		}
		py2 := y + 4 - i
		if py2 >= 0 {
			backing.FillRect(frame.Rect4(px, py2-1, px+1, py2), c, nil)
			backing.FillRect(frame.Rect4(px, py2, px+1, py2+1), c, nil)
		}
	}
	_ = clip
}

func (s *State) drawReload(backing *frame.Bitmap, cx, cy int32, c frame.Color, clip frame.Rect) {
	r := int32(6)
	for angle := 0; angle < 300; angle += 3 {
		rad := float64(angle) * 3.14159 / 180.0
		sin, cos := sinCos(rad)
		px := cx + int32(float64(r)*cos)
		py := cy + int32(float64(r)*sin)
		if px >= 0 && py >= 0 {
			backing.FillRect(frame.Rect4(px, py, px+1, py+1), c, nil)
		}
	}
	arrowX := cx + r - 1
	arrowY := cy - r
	backing.FillRect(frame.Rect4(arrowX, arrowY-1, arrowX+3, arrowY), c, nil)
	backing.FillRect(frame.Rect4(arrowX+2, arrowY-1, arrowX+3, arrowY+2), c, nil)
	_ = clip
}

func sinCos(rad float64) (float64, float64) {
	sin := 0.0
	cos := 1.0
	x := rad
	for x > 6.28318 {
		x -= 6.28318
	}
	for x < -6.28318 {
		x += 6.28318
	}
	s := x
	c := x
	s2 := s * s
	c2 := c * c
	for i := 1; i <= 5; i++ {
		s *= -s2 / float64(2*i*(2*i+1))
		sin += s
		c *= -c2 / float64(2*i*(2*i-1))
		cos += c
	}
	return sin, cos
}

func (s *State) drawAddressBar(backing *frame.Bitmap, toolbarW int32, clip frame.Rect, oy int32) {
	r := AddressBarRect(toolbarW)
	r = frame.Rect4(r.X0, r.Y0+oy, r.X1, r.Y1+oy)

	border := barBorder
	if s.Focus == FocusAddress {
		border = barFocusBorder
	}
	s.drawRoundedRect(backing, r, 6, barBg, border)

	text := s.Input
	if s.Focus == FocusNone {
		if s.Error != "" {
			text = s.Error
		} else {
			text = s.URL
		}
	}
	textCol := textColor
	if s.Focus == FocusNone && s.Error != "" {
		textCol = frame.RGB(200, 50, 50)
	} else if text == "" && s.FindActive {
		text = "Find in page..."
		textCol = placeholderColor
	} else if text == "" && s.Focus == FocusNone {
		text = "Enter URL..."
		textCol = placeholderColor
	}

	textX := r.X0 + BarPadding + 4
	textY := r.Y0 + (ButtonSize-textSize)/2 + textSize

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
			backing.FillRect(frame.Rect4(selX0, r.Y0+3, selX1, r.Y1-3), selectionColor, nil)
		}
	}

	s.drawText(backing, text, textX, textY, textCol, clip)

	if s.FindActive {
		counter := fmt.Sprintf("%d/%d", s.FindIndex+1, s.FindTotal)
		if s.FindTotal == 0 {
			counter = "0/0"
			if strings.TrimSpace(s.Input) == "" {
				counter = ""
			}
		}
		if counter != "" {
			cx := r.X1 - BarPadding - 4 - s.measureText(counter)
			if cx > textX+s.measureText(text)+8 {
				s.drawText(backing, counter, cx, textY, placeholderColor, clip)
			}
		}
	}

	if s.Focus == FocusAddress {
		cursorX := textX + s.measureText(text[:s.cursorByte()])
		backing.FillRect(frame.Rect4(cursorX, r.Y0+5, cursorX+1, r.Y1-5), cursorColor, nil)
	}
}

func (s *State) drawRoundedRect(backing *frame.Bitmap, r frame.Rect, radius int32, fill, border frame.Color) {
	corner := func(cx, cy int32, quadrant int) {
		for dy := int32(0); dy <= radius; dy++ {
			for dx := int32(0); dx <= radius; dx++ {
				dist := dx*dx + dy*dy
				if dist <= radius*radius {
					var px, py int32
					switch quadrant {
					case 0:
						px, py = cx-radius+dx, cy-radius+dy
					case 1:
						px, py = cx+radius-dx, cy-radius+dy
					case 2:
						px, py = cx-radius+dx, cy+radius-dy
					case 3:
						px, py = cx+radius-dx, cy+radius-dy
					}
					if px >= r.X0 && px < r.X1 && py >= r.Y0 && py < r.Y1 {
						backing.FillRect(frame.Rect4(px, py, px+1, py+1), fill, nil)
					}
				}
			}
		}
	}
	corner(r.X0, r.Y0, 0)
	corner(r.X1-1, r.Y0, 1)
	corner(r.X0, r.Y1-1, 2)
	corner(r.X1-1, r.Y1-1, 3)

	backing.FillRect(frame.Rect4(r.X0+radius, r.Y0, r.X1-radius, r.Y0+1), border, nil)
	backing.FillRect(frame.Rect4(r.X0+radius, r.Y1-1, r.X1-radius, r.Y1), border, nil)
	backing.FillRect(frame.Rect4(r.X0, r.Y0+radius, r.X0+1, r.Y1-radius), border, nil)
	backing.FillRect(frame.Rect4(r.X1-1, r.Y0+radius, r.X1, r.Y1-radius), border, nil)

	backing.FillRect(frame.Rect4(r.X0+radius, r.Y0+1, r.X1-radius, r.Y1-1), fill, nil)
	backing.FillRect(frame.Rect4(r.X0+1, r.Y0+radius, r.X0+radius, r.Y1-radius), fill, nil)
	backing.FillRect(frame.Rect4(r.X1-radius, r.Y0+radius, r.X1-1, r.Y1-radius), fill, nil)

	s.drawRoundedBorder(backing, r, radius, border)
}

func (s *State) drawRoundedBorder(backing *frame.Bitmap, r frame.Rect, radius int32, c frame.Color) {
	drawArc := func(cx, cy int32, startAngle, endAngle int) {
		for a := startAngle; a <= endAngle; a += 2 {
			rad := float64(a) * 3.14159 / 180.0
			sin, cos := sinCos(rad)
			px := cx + int32(float64(radius)*cos)
			py := cy + int32(float64(radius)*sin)
			if px >= r.X0 && px < r.X1 && py >= r.Y0 && py < r.Y1 {
				backing.FillRect(frame.Rect4(px, py, px+1, py+1), c, nil)
			}
		}
	}
	drawArc(r.X0+radius, r.Y0+radius, 90, 180)
	drawArc(r.X1-radius-1, r.Y0+radius, 0, 90)
	drawArc(r.X0+radius, r.Y1-radius-1, 180, 270)
	drawArc(r.X1-radius-1, r.Y1-radius-1, 270, 360)
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
