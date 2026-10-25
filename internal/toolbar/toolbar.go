package toolbar

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

type Focus int

const (
	FocusNone Focus = iota
	FocusAddress
)

// SuggestionKind distinguishes search suggestions, bookmarks, and history entries.
type SuggestionKind int

const (
	SuggestSearch SuggestionKind = iota
	SuggestBookmark
	SuggestHistory
)

// Suggestion is one autocompleted destination displayed in the omnibox dropdown.
type Suggestion struct {
	Title string
	URL   string
	Kind  SuggestionKind
}

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
	buttonColor      = frame.RGB(95, 95, 95)
	barBg            = frame.RGB(240, 240, 241)
	barBorder        = frame.RGB(212, 212, 216)
	barFocusBg       = frame.RGB(255, 255, 255)
	barFocusBorder   = frame.RGB(0, 120, 215)
	toolbarBg        = frame.RGB(255, 255, 255)
	textColor        = frame.RGB(33, 33, 33)
	placeholderColor = frame.RGB(140, 140, 140)
	cursorColor      = frame.RGB(0, 120, 215)
	disabledColor    = frame.RGB(205, 205, 205)
	selectionColor   = frame.RGB(180, 210, 250)
	separatorColor   = frame.RGB(222, 222, 222)
	// secureColor marks the https lock: distinct from body text so the
	// origin signal reads at a glance rather than as another word.
	secureColor = frame.RGB(30, 130, 70)
)

// Clipboard is the platform clipboard interface. The darwin package provides
// the real implementation; tests use nopClipboard.
type Clipboard interface {
	Read() string
	Write(string)
}

type nopClipboardImpl struct{}

func (nopClipboardImpl) Read() string { return "" }
func (nopClipboardImpl) Write(string) {}

type State struct {
	// mu serializes the three threads sharing the chrome: the event pump
	// (clicks/keys), the navigation drain (loading flags, URL sync, history
	// swaps), and the UI loop (Draw). Public methods lock; same-package
	// helpers assume the lock is held. Host callbacks (OnNavigate and
	// friends) and clipboard calls always run after unlock: the host calls
	// straight back into these methods.
	mu            sync.Mutex
	URL           string
	Input         string
	Cursor        int
	SelStart      int
	SelEnd        int
	Focus         Focus
	History       *History
	Loading       bool
	Error         string
	FindActive    bool
	FindQuery     string
	FindTotal     int
	FindIndex     int
	Bounds        frame.Rect
	OnNavigate    func(string)
	OnTraverse    func(int)
	OnReload      func()
	OnStop        func()
	OnFindChanged func(string)
	OnFindNext    func(bool)
	OnFindClose   func()
	Clipboard     Clipboard

	Suggestions     []Suggestion
	SelectedSuggest int
	OnQuerySuggest  func(string) []Suggestion

	DownloadsCount   int
	ActiveDownloads  int
	DownloadProgress float64
	OnDownloads      func()
	Incognito        bool

	Zoom           float64
	ZoomBadgeUntil time.Time
	OnResetZoom    func()

	fonts *raster.Fonts
	scale int32
	// dragging tracks an in-progress address-bar drag selection: press sets
	// dragAnchor, motion extends to the cursor, release ends it.
	dragging   bool
	dragAnchor int
}

func NewState(width int32, fonts *raster.Fonts) *State {
	return &State{
		History:         NewHistory(),
		Bounds:          frame.Rect4(0, 0, width, ToolbarHeight),
		fonts:           fonts,
		Clipboard:       nopClipboardImpl{},
		scale:           1,
		SelectedSuggest: -1,
		Zoom:            1.0,
	}
}

// SetScale sets the device pixels per logical pixel the chrome draws at.
// Layout and hit testing stay in logical pixels; only Draw multiplies.
func (s *State) SetScale(n int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n < 1 {
		n = 1
	}
	s.scale = n
	s.Bounds = frame.Rect4(0, 0, s.Bounds.W(), ToolbarHeight*n)
}

func (s *State) sc() int32 {
	if s.scale < 1 {
		return 1
	}
	return s.scale
}

func (s *State) SetHistory(h *History) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.History = h
}

func (s *State) SetBounds(width int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Bounds = frame.Rect4(0, 0, width, ToolbarHeight*s.sc())
}

// BoundsWidth reports the full window device width the last resize delivered.
// The tab strip spans the same width, so the host's tab-bar hit testing
// derives its logical strip width from here rather than from the click point.
func (s *State) BoundsWidth() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Bounds.W()
}

func (s *State) Navigate(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.URL = url
	s.Input = url
	s.Cursor = len([]rune(url))
	s.SelStart = s.Cursor
	s.SelEnd = s.Cursor
	if s.History != nil {
		s.History.Push(url)
	}
	s.Focus = FocusNone
	s.dragging = false
}

func (s *State) SetLoading(loading bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Loading = loading
}

// AccessibilitySnapshot returns the current address-bar and navigation state
// for the native accessibility tree. The history cursor has its own lock, so
// both availability checks happen while this toolbar snapshot is stable.
func (s *State) AccessibilitySnapshot() (value string, focused, findActive, loading, canBack, canForward bool, downloads int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value = s.URL
	if s.Focus == FocusAddress {
		value = s.Input
		focused = true
	}
	findActive = s.FindActive
	loading = s.Loading
	if s.History != nil {
		canBack = s.History.CanBack()
		canForward = s.History.CanForward()
	}
	downloads = s.ActiveDownloads
	return
}

// SetAccessibilityValue applies an AXValue write to the address field without
// submitting it. Navigation remains an explicit Enter/Go action, matching a
// person typing into the omnibox.
func (s *State) SetAccessibilityValue(value string) {
	s.mu.Lock()
	var closeFind func()
	if s.FindActive {
		s.closeFindLocked()
		closeFind = s.OnFindClose
	}
	s.Focus = FocusAddress
	s.Input = sanitizePaste(value)
	s.Cursor = len([]rune(s.Input))
	s.SelStart = s.Cursor
	s.SelEnd = s.Cursor
	s.dragging = false
	s.closeSuggestionsLocked()
	s.mu.Unlock()
	if closeFind != nil {
		closeFind()
	}
}

// SetError records a load error for the address bar to display.
func (s *State) SetError(err string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Error = err
}

// SetDownloadStats updates the toolbar download indicators.
func (s *State) SetDownloadStats(activeCount, totalCount int, progress float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ActiveDownloads = activeCount
	s.DownloadsCount = totalCount
	s.DownloadProgress = progress
}

// SetIncognito updates whether the toolbar draws in private incognito dark mode.
func (s *State) SetIncognito(incognito bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Incognito = incognito
}

// SetZoom updates the current zoom level and activates the transient zoom badge.
func (s *State) SetZoom(z float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Zoom = z
	s.ZoomBadgeUntil = time.Now().Add(2500 * time.Millisecond)
}

// ResetZoom resets the zoom to 100%.
func (s *State) ResetZoom() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Zoom = 1.0
	s.ZoomBadgeUntil = time.Time{}
}

// ZoomPercent reports the current zoom as a whole-percent label, e.g. "125%".
func (s *State) ZoomPercent() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return FormatZoomPercent(s.Zoom)
}

// FormatZoomPercent renders a zoom factor as a whole-percent label.
func FormatZoomPercent(z float64) string {
	return fmt.Sprintf("%d%%", int(z*100+0.5))
}

func (s *State) zoomBadgeRect(r frame.Rect, sc int32) (frame.Rect, bool) {
	if s.FindActive || s.Zoom <= 0 || (s.Zoom == 1.0 && time.Now().After(s.ZoomBadgeUntil)) {
		return frame.Rect{}, false
	}
	zoomStr := FormatZoomPercent(s.Zoom)
	badgeW := s.measureText(zoomStr) + 12*sc
	if badgeW < 36*sc {
		badgeW = 36 * sc
	}
	badgeX1 := r.X1 - 8*sc
	badgeX0 := badgeX1 - badgeW
	badgeY0 := r.Y0 + 4*sc
	badgeY1 := r.Y1 - 4*sc
	return frame.Rect4(badgeX0, badgeY0, badgeX1, badgeY1), true
}

// GetFocus reports where keyboard input currently goes.
func (s *State) GetFocus() Focus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Focus
}

// SyncFromTab copies one tab's display state into the toolbar: URL, loading
// flag, error, and the history object the buttons read. It replaces the
// direct field writes the navigation drain used to do racily.
func (s *State) SyncFromTab(url string, loading bool, errStr string, h *History) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.URL = url
	s.Input = url
	s.Cursor = len([]rune(url))
	s.SelStart = s.Cursor
	s.SelEnd = s.Cursor
	s.History = h
	s.Loading = loading
	s.Error = errStr
	s.dragging = false
}

// FindCounts reports find-mode state for the host to display.
func (s *State) FindCounts() (active bool, total, index int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.FindActive, s.FindTotal, s.FindIndex
}

// SetFindResult publishes one find pass outcome.
func (s *State) SetFindResult(total, index int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FindTotal = total
	s.FindIndex = index
}

// ResetFindCounts clears a finished find pass.
func (s *State) ResetFindCounts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FindTotal = 0
	s.FindIndex = 0
}

// OpenFind reuses the address bar as the find field, restoring the previous
// query. Total and index reset until the host recomputes them.
func (s *State) OpenFind() {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.dragging = false
}

// CloseFind restores the address bar to the current URL and keeps the query
// for the next OpenFind.
func (s *State) closeFindLocked() {
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

// CursorAt returns the pointer shape for a hover at pos, in device pixels
// relative to the toolbar's top edge - the same coordinate contract as
// HandleClick. The nav buttons ask for the hand, the editable bar for the
// text I-beam, and everything else for the default arrow.
func (s *State) CursorAt(pos frame.Point) surface.Cursor {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pos.Y >= int32(ToolbarHeight)*s.sc() {
		return surface.CursorDefault
	}
	sc := s.sc()
	lp := frame.Point{X: pos.X / sc, Y: pos.Y / sc}
	w := s.Bounds.W() / sc
	switch {
	case rectContains(ButtonRect(ButtonBack, w), lp),
		rectContains(ButtonRect(ButtonForward, w), lp),
		rectContains(ButtonRect(ButtonReload, w), lp),
		rectContains(ButtonRect(ButtonDownloads, w), lp):
		return surface.CursorPointer
	case rectContains(AddressBarRect(w), lp):
		r := frame.Rect4(AddressBarRect(w).X0*sc, AddressBarRect(w).Y0*sc, AddressBarRect(w).X1*sc, AddressBarRect(w).Y1*sc)
		if zRect, ok := s.zoomBadgeRect(r, sc); ok && rectContains(zRect, pos) {
			return surface.CursorPointer
		}
		return surface.CursorText
	}
	return surface.CursorDefault
}

func (s *State) HandleClick(pos frame.Point, button surface.Button) {
	s.HandlePointer(pos, button, surface.PointerPress)
}

// HandlePointer routes a full press/motion/release gesture. Presses on the
// buttons fire, presses in the address bar focus and start a drag selection,
// motion extends it, and release ends it - the same shape as a normal input.
func (s *State) HandlePointer(pos frame.Point, button surface.Button, action surface.PointerAction) {
	s.mu.Lock()
	pending := s.handlePointerLocked(pos, button, action)
	s.mu.Unlock()
	for _, fn := range pending {
		fn()
	}
}

// blurLocked leaves the address bar like a normal input: focus goes, the drag
// ends, and an uncommitted URL edit reverts to the committed URL so the next
// focus starts from what the page shows.
func (s *State) blurLocked() {
	s.Focus = FocusNone
	s.dragging = false
	s.closeSuggestionsLocked()
	if !s.FindActive {
		s.Input = s.URL
		s.Cursor = len([]rune(s.Input))
	}
	s.clearSelection()
}

// FocusURL focuses the address bar and selects all, exiting find mode. It is
// the Cmd+L entry point: host callbacks return for the caller to fire after
// unlock.
func (s *State) FocusURL() {
	s.mu.Lock()
	pending := s.focusURLLocked()
	s.mu.Unlock()
	for _, fn := range pending {
		fn()
	}
}

func (s *State) focusURLLocked() []func() {
	var pending []func()
	if s.FindActive {
		s.closeFindLocked()
		if s.OnFindClose != nil {
			cb := s.OnFindClose
			pending = append(pending, func() { cb() })
		}
	}
	s.Focus = FocusAddress
	s.Input = s.URL
	s.selectAllLocked()
	s.dragging = false
	if s.OnQuerySuggest != nil && s.Input != "" {
		s.Suggestions = s.OnQuerySuggest(s.Input)
		s.SelectedSuggest = -1
	}
	return pending
}

// displayTextLocked reports what drawAddressBar paints, so hit testing maps
// onto the same string the pixels show.
func (s *State) displayTextLocked() string {
	if s.Focus == FocusAddress || s.FindActive {
		return s.Input
	}
	if s.Error != "" {
		return s.Error
	}
	if s.URL != "" {
		return s.URL
	}
	return s.Input
}

// addressTextXDev returns the device-pixel X where the address text starts,
// mirroring drawAddressBar: the default padding, or the lock-icon offset when
// an origin mark is drawn.
func (s *State) addressTextXDev(toolbarW int32) int32 {
	sc := s.sc()
	lr := AddressBarRect(toolbarW)
	x := lr.X0*sc + (BarPadding+4)*sc
	if s.Error == "" && (isSecureScheme(s.URL) || isInsecureScheme(s.URL)) {
		x = lr.X0*sc + 22*sc
	}
	return x
}

// hitCursorDev maps a device-pixel X to a rune index in the displayed text,
// clamping drags that run past either end. Boundaries advance in 26.6 fixed
// point exactly like drawText and measureText, and the nearest boundary wins
// so a click lands on the closer side of a glyph.
func (s *State) hitCursorDev(devX int32, toolbarW int32) int {
	sc := s.sc()
	text := s.displayTextLocked()
	runes := []rune(text)
	n := len(runes)
	textX := s.addressTextXDev(toolbarW)
	if devX <= textX {
		return 0
	}
	if s.fonts == nil {
		adv := int32(7) * sc
		if adv <= 0 {
			adv = 1
		}
		off := int((devX - textX + adv/2) / adv)
		if off < 0 {
			return 0
		}
		if off > n {
			return n
		}
		return off
	}
	size := int32(textSize) * sc
	var pen int32
	best := 0
	bestDist := devX - textX
	if bestDist < 0 {
		bestDist = -bestDist
	}
	for i, rn := range runes {
		pen += s.fonts.GlyphAdvanceFixed(size, rn, frame.FontSlot{})
		b := textX + (pen+32)>>6
		d := b - devX
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			best = i + 1
		}
		// Boundaries only move right, so once the overshoot exceeds the
		// best distance no later boundary can win.
		if b > devX && b-devX > bestDist {
			break
		}
	}
	return best
}

// pressAddressLocked focuses the bar and starts a drag. A press on an
// unfocused bar selects all (so typing replaces and Cmd+C copies the URL);
// the press point stays the drag anchor so a press-and-drag still selects a
// range instead of keeping the select-all. A press on a focused bar places
// the caret.
func (s *State) pressAddressLocked(devX int32, toolbarW int32) {
	if s.FindActive {
		s.Focus = FocusAddress
		at := s.hitCursorDev(devX, toolbarW)
		s.Cursor = at
		s.SelStart = at
		s.SelEnd = at
		s.dragging = true
		s.dragAnchor = at
		return
	}
	if s.Focus != FocusAddress {
		s.Focus = FocusAddress
		s.Input = s.URL
		s.selectAllLocked()
		s.dragging = true
		s.dragAnchor = s.hitCursorDev(devX, toolbarW)
		if s.OnQuerySuggest != nil && s.Input != "" {
			s.Suggestions = s.OnQuerySuggest(s.Input)
			s.SelectedSuggest = -1
		}
		return
	}
	at := s.hitCursorDev(devX, toolbarW)
	s.Cursor = at
	s.SelStart = at
	s.SelEnd = at
	s.dragging = true
	s.dragAnchor = at
}

// dragAddressLocked extends the press's selection to the current pointer.
func (s *State) dragAddressLocked(devX int32, toolbarW int32) {
	if !s.dragging || s.Focus != FocusAddress {
		return
	}
	at := s.hitCursorDev(devX, toolbarW)
	s.Cursor = at
	s.SelStart = s.dragAnchor
	s.SelEnd = at
}

// handleClickLocked applies one click; host callbacks return for the caller
// to fire after unlock.
func (s *State) handleClickLocked(pos frame.Point, button surface.Button) []func() {
	return s.handlePointerLocked(pos, button, surface.PointerPress)
}

// handlePointerLocked applies one pointer phase; host callbacks return for
// the caller to fire after unlock.
func (s *State) handlePointerLocked(pos frame.Point, button surface.Button, action surface.PointerAction) []func() {
	var pending []func()
	fire := func(fn func()) {
		pending = append(pending, fn)
	}
	sc := s.sc()
	// Release always ends a drag, wherever it lands.
	if action == surface.PointerRelease {
		s.dragging = false
		if button != surface.ButtonLeft && button != surface.ButtonNone {
			return nil
		}
		// A release outside the chrome defocuses like a press there; a
		// release inside needs no further work.
		if pos.Y >= int32(ToolbarHeight)*sc {
			if s.FindActive {
				s.closeFindLocked()
				if s.OnFindClose != nil {
					cb := s.OnFindClose
					fire(func() { cb() })
				}
				return pending
			}
			if s.Focus == FocusAddress {
				s.blurLocked()
			}
			return pending
		}
		return pending
	}
	if action == surface.PointerMotion {
		// A drag that runs outside the bar clamps to the nearer end, like a
		// normal input; anything else is a hover.
		if s.dragging && s.Focus == FocusAddress {
			w := s.Bounds.W() / sc
			s.dragAddressLocked(pos.X, w)
		}
		return nil
	}
	if button != surface.ButtonLeft {
		return nil
	}
	if pos.Y >= int32(ToolbarHeight)*sc {
		if s.FindActive {
			s.closeFindLocked()
			if s.OnFindClose != nil {
				cb := s.OnFindClose
				fire(func() { cb() })
			}
			return pending
		}
		if s.Focus == FocusAddress {
			s.blurLocked()
		}
		return nil
	}
	lp := frame.Point{X: pos.X / sc, Y: pos.Y / sc}
	w := s.Bounds.W() / sc
	switch {
	case rectContains(ButtonRect(ButtonBack, w), lp):
		s.dragging = false
		if s.History != nil && s.History.CanBack() && s.OnTraverse != nil {
			cb := s.OnTraverse
			fire(func() { cb(-1) })
		}
	case rectContains(ButtonRect(ButtonForward, w), lp):
		s.dragging = false
		if s.History != nil && s.History.CanForward() && s.OnTraverse != nil {
			cb := s.OnTraverse
			fire(func() { cb(1) })
		}
	case rectContains(ButtonRect(ButtonReload, w), lp):
		s.dragging = false
		if s.Loading {
			if s.OnStop != nil {
				cb := s.OnStop
				fire(func() { cb() })
			}
		} else if s.OnReload != nil {
			cb := s.OnReload
			fire(func() { cb() })
		}
	case rectContains(ButtonRect(ButtonDownloads, w), lp):
		s.dragging = false
		if s.OnDownloads != nil {
			cb := s.OnDownloads
			fire(func() { cb() })
		}
	case rectContains(AddressBarRect(w), lp):
		r := frame.Rect4(AddressBarRect(w).X0*sc, AddressBarRect(w).Y0*sc, AddressBarRect(w).X1*sc, AddressBarRect(w).Y1*sc)
		if zRect, ok := s.zoomBadgeRect(r, sc); ok && rectContains(zRect, pos) {
			s.Zoom = 1.0
			s.ZoomBadgeUntil = time.Time{}
			if s.OnResetZoom != nil {
				cb := s.OnResetZoom
				fire(func() { cb() })
			}
			return pending
		}
		s.pressAddressLocked(pos.X, w)
	default:
		if s.FindActive {
			s.closeFindLocked()
			if s.OnFindClose != nil {
				cb := s.OnFindClose
				fire(func() { cb() })
			}
			return pending
		}
		if s.Focus == FocusAddress {
			s.blurLocked()
		}
	}
	return pending
}

// HandleKey dispatches a key event with no modifier information. It exists
// for backward compatibility; HandleKeyEvent is the full entry point.
func (s *State) HandleKey(r rune) {
	s.mu.Lock()
	pending := s.handleKeyEventLocked(r, 0)
	s.mu.Unlock()
	for _, fn := range pending {
		fn()
	}
}

// HandleKeyEvent dispatches a key event with modifier flags.
func (s *State) HandleKeyEvent(key rune, mods surface.KeyMod) {
	s.mu.Lock()
	pending := s.handleKeyEventLocked(key, mods)
	s.mu.Unlock()
	for _, fn := range pending {
		fn()
	}
}

// handleKeyEventLocked dispatches a key event; clipboard reads and host
// callbacks return for the caller to run after unlock.
func (s *State) handleKeyEventLocked(key rune, mods surface.KeyMod) []func() {
	if s.FindActive {
		return s.handleFindKeyLocked(key, mods)
	}
	if s.Focus != FocusAddress {
		return nil
	}
	if key == '\r' || key == '\n' {
		if mods&(surface.ModCommand|surface.ModControl) == 0 {
			return s.submitInputLocked()
		}
		return nil
	}
	if key == 0x1b {
		// Esc closes suggestions dropdown if open, or reverts uncommitted edit.
		if len(s.Suggestions) > 0 {
			s.closeSuggestionsLocked()
			return nil
		}
		s.blurLocked()
		return nil
	}
	beforeInput := s.Input
	pending := s.clipboardPending(s.editKeyLocked(key, mods))
	if s.Input != beforeInput {
		if s.OnQuerySuggest != nil && s.Input != "" {
			s.Suggestions = s.OnQuerySuggest(s.Input)
			s.SelectedSuggest = -1
		} else {
			s.closeSuggestionsLocked()
		}
	}
	return pending
}

// clipboardPending wraps deferred clipboard work from an edit into a
// post-unlock closure.
func (s *State) clipboardPending(wantPaste bool, copyText string, cb Clipboard) []func() {
	if (copyText == "" && !wantPaste) || cb == nil {
		return nil
	}
	return []func(){func() { s.finishClipboard(wantPaste, copyText, cb) }}
}

// handleFindKeyLocked edits the find query. Enter steps to the next match,
// Shift+Enter to the previous, Esc closes, and any other edit reports the new
// query to the host.
func (s *State) handleFindKeyLocked(key rune, mods surface.KeyMod) []func() {
	switch key {
	case 0x1b:
		s.closeFindLocked()
		if s.OnFindClose != nil {
			cb := s.OnFindClose
			return []func(){func() { cb() }}
		}
		return nil
	case '\r', '\n':
		if s.OnFindNext != nil {
			cb := s.OnFindNext
			back := mods&surface.ModShift != 0
			return []func(){func() { cb(back) }}
		}
		return nil
	}
	before := s.Input
	pending := s.clipboardPending(s.editKeyLocked(key, mods))
	if s.Input != before {
		s.FindQuery = s.Input
		if s.OnFindChanged != nil {
			cb, query := s.OnFindChanged, s.Input
			pending = append(pending, func() { cb(query) })
		}
	}
	return pending
}

// editKeyLocked applies address-bar editing for one key event; it is shared
// between URL entry and find mode. Clipboard work cannot run under the lock,
// so a paste request and a copied string return for the caller to finish
// after unlock.
func (s *State) editKeyLocked(key rune, mods surface.KeyMod) (wantPaste bool, copyText string, cb Clipboard) {
	cmd := mods&surface.ModCommand != 0
	ctrl := mods&surface.ModControl != 0
	shift := mods&surface.ModShift != 0
	opt := mods&surface.ModOption != 0
	// Ctrl+C/V/X/A mirror Cmd on non-macOS platforms where the command key
	// does not exist; a normal input accepts both.
	editMod := cmd || ctrl

	if key == 0x7f || key == '\b' {
		s.deleteBackwardLocked()
		return
	}

	// Backward compat: bare control codes (1-26) without modifier flags are
	// treated as Ctrl+letter from the old key path.
	if !cmd && !shift && !opt && key >= 1 && key <= 26 {
		ctrl = true
		key = 'a' + (key-1)%26
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
		if len(s.Suggestions) > 0 {
			s.selectPrevSuggestLocked()
			return
		}
		s.moveCursorStart(shift)
	case keyDown:
		if len(s.Suggestions) > 0 {
			s.selectNextSuggestLocked()
			return
		}
		s.moveCursorEnd(shift)
	case keyHome:
		s.moveCursorStart(shift)
	case keyEnd:
		s.moveCursorEnd(shift)

	case 'a', 'A':
		if cmd {
			s.selectAllLocked()
		} else if ctrl {
			s.moveCursorStart(shift)
		} else {
			s.insertChar(key)
		}
	case 'c', 'C':
		if editMod {
			copyText, cb = s.copyLocked()
		} else {
			s.insertChar(key)
		}
	case 'v', 'V':
		if editMod {
			wantPaste = true
			cb = s.Clipboard
		} else {
			s.insertChar(key)
		}
	case 'x', 'X':
		if editMod {
			copyText, cb = s.cutLocked()
		} else {
			s.insertChar(key)
		}
	case 'z', 'Z':
		if editMod {
			// Undo: no-op for now
		} else {
			s.insertChar(key)
		}

	default:
		if ctrl && !cmd {
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
				s.deleteForwardLocked()
			case 'w':
				s.deleteWordBackward()
			}
		} else if key >= 0x20 {
			s.insertChar(key)
		}
	}
	return wantPaste, copyText, cb
}

func (s *State) insertChar(r rune) {
	s.deleteSelection()
	s.insertRuneLocked(r)
}

func (s *State) submitInputLocked() []func() {
	url := s.Input
	if s.SelectedSuggest >= 0 && s.SelectedSuggest < len(s.Suggestions) {
		url = s.Suggestions[s.SelectedSuggest].URL
	}
	s.closeSuggestionsLocked()
	if url == "" || s.OnNavigate == nil {
		return nil
	}
	cb := s.OnNavigate
	return []func(){func() { cb(url) }}
}

// finishClipboard runs deferred clipboard work after unlock: a copied string
// writes out, a paste request reads back in under a fresh lock.
func (s *State) finishClipboard(wantPaste bool, copyText string, cb Clipboard) {
	if copyText != "" && cb != nil {
		cb.Write(copyText)
	}
	if wantPaste && cb != nil {
		if text := cb.Read(); text != "" {
			s.mu.Lock()
			s.pasteTextLocked(text)
			s.mu.Unlock()
		}
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if backing == nil || backing.Empty() {
		return
	}
	sc := s.sc()
	w := int32(backing.W) / sc
	clip := backing.Bounds()
	barH := int32(ToolbarHeight) * sc

	bg := frame.Rect4(0, oy, int32(backing.W), oy+barH)
	bgCol := toolbarBg
	sepCol := separatorColor
	if s.Incognito {
		bgCol = frame.RGB(40, 44, 52)
		sepCol = frame.RGB(32, 35, 42)
	}
	backing.FillRect(bg, bgCol, nil)

	backing.FillRect(frame.Rect4(0, oy+barH-1, int32(backing.W), oy+barH), sepCol, nil)

	s.drawButton(backing, ButtonBack, w, clip, oy)
	s.drawButton(backing, ButtonForward, w, clip, oy)
	s.drawButton(backing, ButtonReload, w, clip, oy)
	s.drawButton(backing, ButtonDownloads, w, clip, oy)
	s.drawAddressBar(backing, w, clip, oy)

	if s.Loading {
		progressColor := frame.RGB(70, 140, 220)
		lineY := oy + barH - 3*sc
		backing.FillRect(frame.Rect4(0, lineY, int32(backing.W), lineY+2*sc), progressColor, nil)
	}
}

func (s *State) drawButton(backing *frame.Bitmap, btn Button, toolbarW int32, clip frame.Rect, oy int32) {
	sc := s.sc()
	r := ButtonRect(btn, toolbarW)
	ox := r.X0 * sc
	oyd := oy + r.Y0*sc
	fg := buttonColor
	if s.Incognito {
		fg = frame.RGB(200, 204, 212)
		if btn == ButtonBack && (s.History == nil || !s.History.CanBack()) {
			fg = frame.RGB(90, 94, 102)
		}
		if btn == ButtonForward && (s.History == nil || !s.History.CanForward()) {
			fg = frame.RGB(90, 94, 102)
		}
	} else {
		if btn == ButtonBack && (s.History == nil || !s.History.CanBack()) {
			fg = disabledColor
		}
		if btn == ButtonForward && (s.History == nil || !s.History.CanForward()) {
			fg = disabledColor
		}
	}
	cx := int32(ButtonSize) / 2
	cy := int32(ButtonSize) / 2
	switch btn {
	case ButtonBack:
		s.drawArrow(backing, ox, oyd, cx-4, cy, -1, sc, fg, clip)
	case ButtonForward:
		s.drawArrow(backing, ox, oyd, cx+4, cy, 1, sc, fg, clip)
	case ButtonReload:
		if s.Loading {
			s.drawStop(backing, ox, oyd, cx, cy, sc, fg)
		} else {
			s.drawReload(backing, ox, oyd, cx, cy, sc, fg, clip)
		}
	case ButtonDownloads:
		s.drawDownloads(backing, ox, oyd, cx, cy, sc, fg, clip)
	}
}

func (s *State) drawDownloads(backing *frame.Bitmap, ox, oy, cx, cy, sc int32, c frame.Color, clip frame.Rect) {
	fcx := float64(ox) + float64(cx)*float64(sc)
	fcy := float64(oy) + float64(cy)*float64(sc)
	thick := 1.8 * float64(sc)
	if thick < 1.5 {
		thick = 1.5
	}

	// Tray: bottom horizontal bar and two upturned sides
	trayHalfW := 6.0 * float64(sc)
	trayBottomY := fcy + 4.5*float64(sc)
	trayTopY := fcy + 1.5*float64(sc)
	drawLineAA(backing, fcx-trayHalfW, trayBottomY, fcx+trayHalfW, trayBottomY, thick, c)
	drawLineAA(backing, fcx-trayHalfW, trayTopY, fcx-trayHalfW, trayBottomY, thick, c)
	drawLineAA(backing, fcx+trayHalfW, trayTopY, fcx+trayHalfW, trayBottomY, thick, c)

	// Downward arrow stem
	stemTopY := fcy - 6.0*float64(sc)
	stemBottomY := fcy + 1.0*float64(sc)
	drawLineAA(backing, fcx, stemTopY, fcx, stemBottomY, thick, c)

	// Downward arrow head
	tipY := fcy + 3.0*float64(sc)
	baseY := fcy - 0.5*float64(sc)
	headHalfW := 3.5 * float64(sc)
	fillTriangleAA(backing, fcx, tipY, fcx-headHalfW, baseY, fcx+headHalfW, baseY, c)

	// Active download progress indicator
	if s.ActiveDownloads > 0 {
		activeColor := frame.RGB(0, 120, 215)
		barY := oy + (int32(ButtonSize)-3)*sc
		barW := int32(ButtonSize-6) * sc
		barX := ox + 3*sc
		backing.FillRect(frame.Rect4(barX, barY, barX+barW, barY+2*sc), frame.RGB(210, 210, 215), nil)
		p := s.DownloadProgress
		if p <= 0 {
			p = 100.0
		}
		fillW := int32(float64(barW) * (p / 100.0))
		if fillW < 2*sc {
			fillW = 2 * sc
		}
		if fillW > barW {
			fillW = barW
		}
		backing.FillRect(frame.Rect4(barX, barY, barX+fillW, barY+2*sc), activeColor, nil)
	}
	_ = clip
}

func (s *State) drawArrow(backing *frame.Bitmap, ox, oy, x, y, dir, sc int32, c frame.Color, clip frame.Rect) {
	// Filled triangle with AA coverage, in device pixels. Logical icon:
	// tip 7px past center, base 5px behind center with half-height 6px.
	fx := float64(ox) + float64(x)*float64(sc)
	fy := float64(oy) + float64(y)*float64(sc)
	tipX := fx + float64(dir)*7*float64(sc)
	baseX := fx - float64(dir)*5*float64(sc)
	halfH := 6 * float64(sc)
	fillTriangleAA(backing, tipX, fy, baseX, fy-halfH, baseX, fy+halfH, c)
	// Short stem so back/forward read as arrows, not triangles.
	stemLen := 5 * float64(sc)
	thick := 2 * float64(sc)
	if thick < 2 {
		thick = 2
	}
	x0 := baseX
	x1 := baseX - float64(dir)*stemLen
	drawLineAA(backing, x0, fy, x1, fy, thick, c)
	_ = clip
}

func (s *State) drawStop(backing *frame.Bitmap, ox, oy, cx, cy, sc int32, c frame.Color) {
	const halfSize = 5
	x := ox + cx*sc
	y := oy + cy*sc
	size := int32(halfSize) * sc
	backing.FillRect(frame.Rect4(x-size, y-size, x+size, y+size), c, nil)
}

func (s *State) drawReload(backing *frame.Bitmap, ox, oy, cx, cy, sc int32, c frame.Color, clip frame.Rect) {
	fcx := float64(ox) + float64(cx)*float64(sc)
	fcy := float64(oy) + float64(cy)*float64(sc)
	rOut := 6 * float64(sc)
	thick := 1.8 * float64(sc)
	if thick < 1.5 {
		thick = 1.5
	}
	rMid := rOut - thick/2
	// 300° sweep with a 60° gap at the top-right, matching the old icon.
	for py := int(fcy - rOut - 1); py <= int(fcy+rOut+1); py++ {
		for px := int(fcx - rOut - 1); px <= int(fcx+rOut+1); px++ {
			dx := float64(px) + 0.5 - fcx
			dy := float64(py) + 0.5 - fcy
			dist := absF(sqrtF(dx*dx+dy*dy) - rMid)
			if dist > thick/2+0.5 {
				continue
			}
			ang := atan2F(dy, dx) * 180 / 3.14159
			if ang < 0 {
				ang += 360
			}
			// Gap centered at -60° (300°): skip 270°..330°.
			if ang >= 270 && ang <= 330 {
				continue
			}
			cov := thick/2 + 0.5 - dist
			blendDot(backing, px, py, c, cov)
		}
	}

	// Arrow head at the end of the sweep (240°) pointing tangentially clockwise.
	rad := 240.0 * 3.14159 / 180.0
	sin, cos := sinF(rad), cosF(rad)
	tipX := fcx + rMid*cos
	tipY := fcy + rMid*sin
	// Clockwise tangent.
	tx, ty := -sin, cos
	headLen := 5 * float64(sc)
	headHalf := 3.2 * float64(sc)
	bx := tipX + tx*headLen
	by := tipY + ty*headLen
	// Perpendicular.
	px_, py_ := -ty, tx
	fillTriangleAA(backing, tipX, tipY,
		bx+px_*headHalf, by+py_*headHalf,
		bx-px_*headHalf, by-py_*headHalf, c)
	_ = clip
}

func blendDot(backing *frame.Bitmap, x, y int, c frame.Color, cov float64) {
	if cov <= 0 {
		return
	}
	if cov > 1 {
		cov = 1
	}
	backing.Set(x, y, frame.BlendOver(backing.At(x, y), frame.ScaleColor(c, float32(cov))))
}

func drawLineAA(backing *frame.Bitmap, x0, y0, x1, y1, thick float64, c frame.Color) {
	minX := int(floorF(minF(x0, x1) - thick/2 - 1))
	maxX := int(ceilF(maxF(x0, x1) + thick/2 + 1))
	minY := int(floorF(minF(y0, y1) - thick/2 - 1))
	maxY := int(ceilF(maxF(y0, y1) + thick/2 + 1))
	dx := x1 - x0
	dy := y1 - y0
	len2 := dx*dx + dy*dy
	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			fx := float64(px) + 0.5
			fy := float64(py) + 0.5
			t := 0.0
			if len2 > 0 {
				t = ((fx-x0)*dx + (fy-y0)*dy) / len2
				if t < 0 {
					t = 0
				} else if t > 1 {
					t = 1
				}
			}
			ddx := fx - (x0 + t*dx)
			ddy := fy - (y0 + t*dy)
			dist := sqrtF(ddx*ddx + ddy*ddy)
			cov := thick/2 + 0.5 - dist
			blendDot(backing, px, py, c, cov)
		}
	}
}

func fillTriangleAA(backing *frame.Bitmap, ax, ay, bx, by, cx, cy float64, c frame.Color) {
	minX := int(floorF(minF(ax, minF(bx, cx)) - 1))
	maxX := int(ceilF(maxF(ax, maxF(bx, cx)) + 1))
	minY := int(floorF(minF(ay, minF(by, cy)) - 1))
	maxY := int(ceilF(maxF(ay, maxF(by, cy)) + 1))
	// Edge lengths for distance normalization.
	abx, aby := bx-ax, by-ay
	bcx, bcy := cx-bx, cy-by
	cax, cay := ax-cx, ay-cy
	abLen := sqrtF(abx*abx + aby*aby)
	bcLen := sqrtF(bcx*bcx + bcy*bcy)
	caLen := sqrtF(cax*cax + cay*cay)
	if abLen == 0 || bcLen == 0 || caLen == 0 {
		return
	}
	// Winding: inside when all cross products share the sign of the area.
	area := (bx-ax)*(cy-ay) - (by-ay)*(cx-ax)
	positive := area > 0
	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			fx := float64(px) + 0.5
			fy := float64(py) + 0.5
			c1 := (bx-ax)*(fy-ay) - (by-ay)*(fx-ax)
			c2 := (cx-bx)*(fy-by) - (cy-by)*(fx-bx)
			c3 := (ax-cx)*(fy-cy) - (ay-cy)*(fx-cx)
			inside := c1 >= 0 && c2 >= 0 && c3 >= 0
			if positive {
				inside = c1 >= 0 && c2 >= 0 && c3 >= 0
			} else {
				inside = c1 <= 0 && c2 <= 0 && c3 <= 0
			}
			if !inside {
				continue
			}
			d1 := absF(c1) / abLen
			d2 := absF(c2) / bcLen
			d3 := absF(c3) / caLen
			m := minF(d1, minF(d2, d3))
			blendDot(backing, px, py, c, m+0.5)
		}
	}
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func floorF(v float64) float64    { return math.Floor(v) }
func ceilF(v float64) float64     { return math.Ceil(v) }
func sqrtF(v float64) float64     { return math.Sqrt(v) }
func sinF(v float64) float64      { return math.Sin(v) }
func cosF(v float64) float64      { return math.Cos(v) }
func atan2F(y, x float64) float64 { return math.Atan2(y, x) }

func (s *State) drawAddressBar(backing *frame.Bitmap, toolbarW int32, clip frame.Rect, oy int32) {
	sc := s.sc()
	lr := AddressBarRect(toolbarW)
	r := frame.Rect4(lr.X0*sc, oy+lr.Y0*sc, lr.X1*sc, oy+lr.Y1*sc)

	fill := barBg
	border := barBorder
	if s.Incognito {
		fill = frame.RGB(32, 35, 42)
		border = frame.RGB(60, 64, 72)
		if s.Focus == FocusAddress {
			fill = frame.RGB(40, 44, 52)
			border = frame.RGB(100, 160, 240)
		}
	} else if s.Focus == FocusAddress {
		fill = barFocusBg
		border = barFocusBorder
	}
	s.drawRoundedRect(backing, r, r.H()/2, fill, border)

	text := s.Input
	if s.Focus == FocusNone {
		if s.Error != "" {
			text = s.Error
		} else {
			text = s.URL
		}
	}
	textCol := textColor
	if s.Incognito {
		textCol = frame.RGB(240, 240, 240)
	}
	if s.Focus == FocusNone && s.Error != "" {
		textCol = frame.RGB(200, 50, 50)
	} else if text == "" && s.FindActive {
		text = "Find in page..."
		if s.Incognito {
			textCol = frame.RGB(140, 144, 154)
		} else {
			textCol = placeholderColor
		}
	} else if text == "" && s.Focus == FocusNone {
		text = "Enter URL..."
		if s.Incognito {
			textCol = frame.RGB(140, 144, 154)
		} else {
			textCol = placeholderColor
		}
	}

	textX := r.X0 + (BarPadding+4)*sc
	// Origin marks: a lock for https, a broken lock for http, nothing for
	// origins off the network. A shown error suppresses both — a certificate
	// failure must never sit next to a reassuring lock. Cursor, selection and
	// the find counter all derive from textX, so shifting it here moves the
	// whole row right of either icon.
	if s.Error == "" {
		switch {
		case isSecureScheme(s.URL):
			fcx := float64(r.X0) + 11*float64(sc)
			fcy := float64(r.Y0+r.Y1)/2 - 1.5*float64(sc)
			s.drawPadlock(backing, fcx, fcy, sc, secureColor, false)
			textX = r.X0 + 22*sc
		case isInsecureScheme(s.URL):
			fcx := float64(r.X0) + 11*float64(sc)
			fcy := float64(r.Y0+r.Y1)/2 - 1.5*float64(sc)
			s.drawPadlock(backing, fcx, fcy, sc, buttonColor, true)
			textX = r.X0 + 22*sc
		}
	}
	size := int32(textSize) * sc
	textY := s.baselineY(r, size)

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
			backing.FillRect(frame.Rect4(selX0, r.Y0+3*sc, selX1, r.Y1-3*sc), selectionColor, nil)
		}
	}

	s.drawText(backing, text, textX, textY, textCol, clip)

	if zRect, ok := s.zoomBadgeRect(r, sc); ok {
		zoomStr := FormatZoomPercent(s.Zoom)
		badgeBg := frame.RGB(232, 240, 254)
		badgeFg := frame.RGB(26, 115, 232)
		if s.Incognito {
			badgeBg = frame.RGB(50, 54, 62)
			badgeFg = frame.RGB(138, 180, 248)
		}
		s.drawRoundedRect(backing, zRect, zRect.H()/2, badgeBg, badgeBg)
		textW := s.measureText(zoomStr)
		tx := zRect.X0 + (zRect.W()-textW)/2
		ty := textY
		s.drawText(backing, zoomStr, tx, ty, badgeFg, clip)
	}

	if s.FindActive {
		counter := fmt.Sprintf("%d/%d", s.FindIndex+1, s.FindTotal)
		if s.FindTotal == 0 {
			counter = "0/0"
			if strings.TrimSpace(s.Input) == "" {
				counter = ""
			}
		}
		if counter != "" {
			cx := r.X1 - (BarPadding+4)*sc - s.measureText(counter)
			if cx > textX+s.measureText(text)+8*sc {
				s.drawText(backing, counter, cx, textY, placeholderColor, clip)
			}
		}
	}

	if s.Focus == FocusAddress {
		cursorX := textX + s.measureText(text[:s.cursorByte()])
		backing.FillRect(frame.Rect4(cursorX, r.Y0+5*sc, cursorX+sc, r.Y1-5*sc), cursorColor, nil)
	}
}

// isInsecureScheme reports whether url uses plain http, case-insensitively
// and without allocating. "https://" fails at the fifth byte (s vs :), so no
// separate exclusion is needed.
func isInsecureScheme(url string) bool {
	const p = "http://"
	if len(url) < len(p) {
		return false
	}
	for i := 0; i < len(p); i++ {
		c := url[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != p[i] {
			return false
		}
	}
	return true
}

// isSecureScheme reports whether url uses https, case-insensitively and
// without allocating: the address bar checks it on every Draw.
func isSecureScheme(url string) bool {
	const p = "https://"
	if len(url) < len(p) {
		return false
	}
	for i := 0; i < len(p); i++ {
		c := url[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != p[i] {
			return false
		}
	}
	return true
}

// drawPadlock paints a padlock icon for an origin mark: an arched shackle over
// a rounded body, ~12x13 logical pixels centered near (fcx, fcy) in device
// pixels. open breaks the shackle on the upper left for the http mark. Same
// pixel techniques as the reload icon (arc sweep + fill), so no new
// rasterizer machinery.
func (s *State) drawPadlock(backing *frame.Bitmap, fcx, fcy float64, sc int32, c frame.Color, open bool) {
	fsc := float64(sc)
	rOut := 5 * fsc
	thick := 1.8 * fsc
	if thick < 1.5 {
		thick = 1.5
	}
	rMid := rOut - thick/2
	// Shackle: the upper half of the ring (angles 180..360), whose legs land
	// on the body's top edge at the shackle center line. Open starts the arc
	// at 230, leaving the upper-left quadrant missing.
	lo := 180.0
	if open {
		lo = 230.0
	}
	for py := int(fcy - rOut - 1); py <= int(fcy+1); py++ {
		for px := int(fcx - rOut - 1); px <= int(fcx+rOut+1); px++ {
			dx := float64(px) + 0.5 - fcx
			dy := float64(py) + 0.5 - fcy
			dist := absF(sqrtF(dx*dx+dy*dy) - rMid)
			if dist > thick/2+0.5 {
				continue
			}
			ang := atan2F(dy, dx) * 180 / 3.14159
			if ang < 0 {
				ang += 360
			}
			if ang < lo || ang > 360 {
				continue
			}
			blendDot(backing, px, py, c, thick/2+0.5-dist)
		}
	}
	if open {
		// The surviving right leg plugs into the body it hangs from.
		drawLineAA(backing, fcx+rMid, fcy-1, fcx+rMid, fcy+3*fsc, thick, c)
	}
	// Body: rounded box from the shackle line down 8 logical pixels.
	body := frame.Rect4(int32(fcx-5*fsc+0.5), int32(fcy+0.5), int32(fcx+5*fsc+0.5), int32(fcy+8*fsc+0.5))
	rad := frame.Corners{
		TL: frame.Radius{X: float32(2 * sc), Y: float32(2 * sc)},
		TR: frame.Radius{X: float32(2 * sc), Y: float32(2 * sc)},
		BR: frame.Radius{X: float32(2 * sc), Y: float32(2 * sc)},
		BL: frame.Radius{X: float32(2 * sc), Y: float32(2 * sc)},
	}
	backing.FillRounded(body, rad, c, nil)
}

func (s *State) drawRoundedRect(backing *frame.Bitmap, r frame.Rect, radius int32, fill, border frame.Color) {
	if radius > r.H()/2 {
		radius = r.H() / 2
	}
	if radius > r.W()/2 {
		radius = r.W() / 2
	}
	rad := frame.Corners{
		TL: frame.Radius{X: float32(radius), Y: float32(radius)},
		TR: frame.Radius{X: float32(radius), Y: float32(radius)},
		BR: frame.Radius{X: float32(radius), Y: float32(radius)},
		BL: frame.Radius{X: float32(radius), Y: float32(radius)},
	}
	backing.FillRounded(r, rad, fill, nil)
	thick := s.sc()
	if thick < 1 {
		thick = 1
	}
	backing.StrokeRounded(r, rad, thick, thick, thick, thick, border, border, border, border, nil)
}

func (s *State) baselineY(r frame.Rect, size int32) int32 {
	if s.fonts != nil {
		asc, desc, _ := s.fonts.LineMetrics(size, frame.FontSlot{})
		if asc+desc > 0 {
			if desc < 0 {
				desc = -desc
			}
			return r.Y0 + (r.H()-(asc+desc))/2 + asc
		}
	}
	return r.Y0 + (r.H()-size)/2 + size*4/5
}

func (s *State) drawText(backing *frame.Bitmap, text string, x, y int32, c frame.Color, clip frame.Rect) {
	if s.fonts == nil {
		return
	}
	size := int32(textSize) * s.sc()
	// Fractional pen: accumulate 26.6 advances and round only glyph positions,
	// matching measureText and paint/builder.go so cursor/selection agree.
	var pen int32
	// The zero slot is the embedded Go face: the chrome draws the same type it
	// always has, whatever fonts the host has. y is the baseline; Bounds is
	// already relative to the pen.
	for _, rn := range text {
		g := s.fonts.Glyph(size, rn, frame.FontSlot{})
		adv := s.fonts.GlyphAdvanceFixed(size, rn, frame.FontSlot{})
		penX := x + (pen+32)>>6
		if !g.Ok || g.Mask == nil {
			pen += adv
			continue
		}
		origin := frame.Point{X: penX + g.Bounds.X0, Y: y + g.Bounds.Y0}
		backing.BlitMask(g.Mask, origin, c, clip, nil)
		pen += adv
	}
}

func (s *State) measureText(text string) int32 {
	if s.fonts == nil {
		return int32(len(text)) * 7 * s.sc()
	}
	size := int32(textSize) * s.sc()
	var pen int32
	for _, rn := range text {
		pen += s.fonts.GlyphAdvanceFixed(size, rn, frame.FontSlot{})
	}
	return (pen + 32) >> 6
}

// CloseFind leaves find mode and restores the address bar.
func (s *State) CloseFind() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeFindLocked()
}

func (s *State) selectNextSuggestLocked() {
	if len(s.Suggestions) == 0 {
		return
	}
	s.SelectedSuggest++
	if s.SelectedSuggest >= len(s.Suggestions) {
		s.SelectedSuggest = 0
	}
}

func (s *State) selectPrevSuggestLocked() {
	if len(s.Suggestions) == 0 {
		return
	}
	s.SelectedSuggest--
	if s.SelectedSuggest < 0 {
		s.SelectedSuggest = len(s.Suggestions) - 1
	}
}

func (s *State) closeSuggestionsLocked() {
	s.Suggestions = nil
	s.SelectedSuggest = -1
}

// CloseSuggestions clears the current suggestions dropdown.
func (s *State) CloseSuggestions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeSuggestionsLocked()
}

// HasSuggestions reports whether the suggestions dropdown is currently visible.
func (s *State) HasSuggestions() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Suggestions) > 0
}

// SuggestionCount reports how many suggestions are in the dropdown.
func (s *State) SuggestionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Suggestions)
}

// SelectedSuggestion reports the index of the highlighted suggestion, or -1.
func (s *State) SelectedSuggestion() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.SelectedSuggest
}

// SuggestionsRect returns the bounding rectangle of the suggestions dropdown.
func (s *State) SuggestionsRect(topOffset int32) frame.Rect {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Suggestions) == 0 {
		return frame.Rect{}
	}
	sc := s.sc()
	w := s.Bounds.W() / sc
	bar := AddressBarRect(w)
	itemH := int32(36) * sc
	totalH := int32(len(s.Suggestions)) * itemH
	return frame.Rect4(bar.X0*sc, topOffset, bar.X1*sc, topOffset+totalH)
}

// HitSuggestion reports which suggestion was clicked, if any, and closes the dropdown.
func (s *State) HitSuggestion(pos frame.Point, topOffset int32) (Suggestion, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Suggestions) == 0 {
		return Suggestion{}, false
	}
	sc := s.sc()
	w := s.Bounds.W() / sc
	bar := AddressBarRect(w)
	barX0 := bar.X0 * sc
	barX1 := bar.X1 * sc
	if pos.X < barX0 || pos.X >= barX1 {
		return Suggestion{}, false
	}
	itemH := int32(36) * sc
	relY := pos.Y - topOffset
	if relY < 0 {
		return Suggestion{}, false
	}
	idx := int(relY / itemH)
	if idx >= 0 && idx < len(s.Suggestions) {
		sug := s.Suggestions[idx]
		s.closeSuggestionsLocked()
		return sug, true
	}
	return Suggestion{}, false
}

// HoverSuggestion updates the highlighted suggestion based on pointer movement.
func (s *State) HoverSuggestion(pos frame.Point, topOffset int32) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Suggestions) == 0 {
		return -1
	}
	sc := s.sc()
	w := s.Bounds.W() / sc
	bar := AddressBarRect(w)
	barX0 := bar.X0 * sc
	barX1 := bar.X1 * sc
	if pos.X < barX0 || pos.X >= barX1 {
		return -1
	}
	itemH := int32(36) * sc
	relY := pos.Y - topOffset
	if relY < 0 {
		return -1
	}
	idx := int(relY / itemH)
	if idx >= 0 && idx < len(s.Suggestions) {
		s.SelectedSuggest = idx
		return idx
	}
	return -1
}

// DrawSuggestions renders the autocomplete dropdown box beneath the address bar.
func (s *State) DrawSuggestions(backing *frame.Bitmap, topOffset int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Suggestions) == 0 || backing == nil {
		return
	}
	sc := s.sc()
	w := s.Bounds.W() / sc
	bar := AddressBarRect(w)
	itemH := int32(36) * sc
	totalH := int32(len(s.Suggestions)) * itemH
	box := frame.Rect4(bar.X0*sc, topOffset, bar.X1*sc, topOffset+totalH)

	bgCol := frame.RGB(255, 255, 255)
	borderCol := frame.RGB(218, 220, 224)
	s.drawRoundedRect(backing, box, 4*sc, bgCol, borderCol)

	clip := box
	fontSize := int32(13) * sc

	for i, sug := range s.Suggestions {
		rowY0 := topOffset + int32(i)*itemH
		rowY1 := rowY0 + itemH
		rowRect := frame.Rect4(box.X0, rowY0, box.X1, rowY1)

		if i == s.SelectedSuggest {
			selectedBg := frame.RGB(232, 240, 254)
			backing.FillRect(rowRect, selectedBg, nil)
		} else if i > 0 {
			sepCol := frame.RGB(241, 243, 244)
			backing.FillRect(frame.Rect4(box.X0, rowY0, box.X1, rowY0+sc), sepCol, nil)
		}

		badgeText := "[Search]"
		badgeCol := frame.RGB(26, 115, 232)
		switch sug.Kind {
		case SuggestBookmark:
			badgeText = "[★ Bookmark]"
			badgeCol = frame.RGB(227, 116, 0)
		case SuggestHistory:
			badgeText = "[History]"
			badgeCol = frame.RGB(95, 99, 104)
		}

		textY := s.baselineY(rowRect, fontSize)
		curX := rowRect.X0 + 12*sc
		s.drawText(backing, badgeText, curX, textY, badgeCol, clip)
		curX += s.measureText(badgeText) + 8*sc

		titleText := sug.Title
		if titleText == "" {
			titleText = sug.URL
		}
		titleCol := frame.RGB(32, 33, 36)
		s.drawText(backing, titleText, curX, textY, titleCol, clip)
		curX += s.measureText(titleText) + 12*sc

		if sug.Title != "" && sug.URL != "" && sug.Title != sug.URL {
			urlCol := frame.RGB(128, 134, 139)
			s.drawText(backing, sug.URL, curX, textY, urlCol, clip)
		}
	}
}
