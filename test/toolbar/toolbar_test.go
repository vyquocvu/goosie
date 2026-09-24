package toolbar_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestTraversalDoesNotCommitBeforeLoad(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Navigate("https://a.com")
	s.Navigate("https://b.com")
	r := toolbar.ButtonRect(toolbar.ButtonBack, 800)
	s.HandleClick(frame.Point{X: r.X0 + 1, Y: r.Y0 + 1}, surface.ButtonLeft)
	if got := s.History.Current(); got != "https://b.com" {
		t.Fatalf("history changed before load succeeded: %q", got)
	}
}

func TestHistoryPushAndCurrent(t *testing.T) {
	h := toolbar.NewHistory()
	if got := h.Current(); got != "" {
		t.Fatalf("new history Current() = %q, want empty", got)
	}
	h.Push("https://a.com")
	if got := h.Current(); got != "https://a.com" {
		t.Fatalf("Current() = %q, want https://a.com", got)
	}
	h.Push("https://b.com")
	if got := h.Current(); got != "https://b.com" {
		t.Fatalf("Current() = %q, want https://b.com", got)
	}
}

func TestHistoryBackForward(t *testing.T) {
	h := toolbar.NewHistory()
	h.Push("https://a.com")
	h.Push("https://b.com")
	h.Push("https://c.com")

	if !h.CanBack() {
		t.Fatal("CanBack() = false after three pushes")
	}
	if h.CanForward() {
		t.Fatal("CanForward() = true at the end")
	}

	url, ok := h.Back()
	if !ok || url != "https://b.com" {
		t.Fatalf("Back() = (%q, %v), want (https://b.com, true)", url, ok)
	}
	url, ok = h.Back()
	if !ok || url != "https://a.com" {
		t.Fatalf("Back() = (%q, %v), want (https://a.com, true)", url, ok)
	}
	if h.CanBack() {
		t.Fatal("CanBack() = true at the start")
	}
	_, ok = h.Back()
	if ok {
		t.Fatal("Back() succeeded at the start of history")
	}

	url, ok = h.Forward()
	if !ok || url != "https://b.com" {
		t.Fatalf("Forward() = (%q, %v), want (https://b.com, true)", url, ok)
	}
	url, ok = h.Forward()
	if !ok || url != "https://c.com" {
		t.Fatalf("Forward() = (%q, %v), want (https://c.com, true)", url, ok)
	}
	if h.CanForward() {
		t.Fatal("CanForward() = true at the end after forward navigation")
	}
}

func TestHistoryPushTruncatesForward(t *testing.T) {
	h := toolbar.NewHistory()
	h.Push("https://a.com")
	h.Push("https://b.com")
	h.Push("https://c.com")
	h.Back()
	h.Back()

	h.Push("https://d.com")
	if got := h.Current(); got != "https://d.com" {
		t.Fatalf("Current() = %q after push, want https://d.com", got)
	}
	if h.CanForward() {
		t.Fatal("CanForward() = true after push truncated forward list")
	}
	_, ok := h.Back()
	if !ok {
		t.Fatal("Back() failed after truncation")
	}
	if got := h.Current(); got != "https://a.com" {
		t.Fatalf("Current() = %q after back, want https://a.com", got)
	}
}

func TestInputInsertAndDelete(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusAddress

	s.InsertRune('h')
	s.InsertRune('i')
	if s.Input != "hi" {
		t.Fatalf("Input = %q, want hi", s.Input)
	}
	if s.Cursor != 2 {
		t.Fatalf("Cursor = %d, want 2", s.Cursor)
	}

	s.DeleteBackward()
	if s.Input != "h" {
		t.Fatalf("Input = %q after delete, want h", s.Input)
	}
	if s.Cursor != 1 {
		t.Fatalf("Cursor = %d after delete, want 1", s.Cursor)
	}
}

func TestInputIgnoresWhenUnfocused(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusNone

	s.InsertRune('x')
	if s.Input != "" {
		t.Fatalf("InsertRune when unfocused changed Input to %q", s.Input)
	}
}

func TestCursorMovement(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusAddress
	s.Input = "hello"
	s.Cursor = 5

	s.MoveCursorLeft()
	if s.Cursor != 4 {
		t.Fatalf("MoveCursorLeft: Cursor = %d, want 4", s.Cursor)
	}
	s.MoveCursorStart()
	if s.Cursor != 0 {
		t.Fatalf("MoveCursorStart: Cursor = %d, want 0", s.Cursor)
	}
	s.MoveCursorLeft()
	if s.Cursor != 0 {
		t.Fatalf("MoveCursorLeft at start: Cursor = %d, want 0", s.Cursor)
	}
	s.MoveCursorRight()
	if s.Cursor != 1 {
		t.Fatalf("MoveCursorRight: Cursor = %d, want 1", s.Cursor)
	}
	s.MoveCursorEnd()
	if s.Cursor != 5 {
		t.Fatalf("MoveCursorEnd: Cursor = %d, want 5", s.Cursor)
	}
	s.MoveCursorRight()
	if s.Cursor != 5 {
		t.Fatalf("MoveCursorRight at end: Cursor = %d, want 5", s.Cursor)
	}
}

func TestHandleKeySubmit(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusAddress
	s.Input = "https://example.com"
	s.Cursor = len("https://example.com")

	var navigated string
	s.OnNavigate = func(url string) { navigated = url }

	s.HandleKey('\r')
	if navigated != "https://example.com" {
		t.Fatalf("OnNavigate = %q, want https://example.com", navigated)
	}
}

func TestHandleKeySubmitEmpty(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusAddress
	s.Input = ""
	s.Cursor = 0

	called := false
	s.OnNavigate = func(url string) { called = true }

	s.HandleKey('\r')
	if called {
		t.Fatal("OnNavigate called for empty input")
	}
}

func TestHandleKeyBackspace(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusAddress
	s.Input = "abc"
	s.Cursor = 3

	s.HandleKey(0x7f)
	if s.Input != "ab" {
		t.Fatalf("Input = %q after backspace, want ab", s.Input)
	}
}

func TestHandleKeyCtrlShortcuts(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusAddress
	s.Input = "hello"
	s.Cursor = 5

	s.HandleKey(0x01) // Ctrl-A: start
	if s.Cursor != 0 {
		t.Fatalf("Ctrl-A: Cursor = %d, want 0", s.Cursor)
	}
	s.HandleKey(0x05) // Ctrl-E: end
	if s.Cursor != 5 {
		t.Fatalf("Ctrl-E: Cursor = %d, want 5", s.Cursor)
	}
	s.HandleKey(0x02) // Ctrl-B: left
	if s.Cursor != 4 {
		t.Fatalf("Ctrl-B: Cursor = %d, want 4", s.Cursor)
	}
	s.HandleKey(0x06) // Ctrl-F: right
	if s.Cursor != 5 {
		t.Fatalf("Ctrl-F: Cursor = %d, want 5", s.Cursor)
	}
}

func TestHandleKeyCtrlK(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusAddress
	s.Input = "hello world"
	s.Cursor = 5

	s.HandleKey(0x0b) // Ctrl-K: kill to end
	if s.Input != "hello" {
		t.Fatalf("Input = %q after Ctrl-K, want hello", s.Input)
	}
}

func TestHandleKeyInsertChar(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusAddress
	s.Input = ""
	s.Cursor = 0

	s.HandleKey('a')
	s.HandleKey('b')
	s.HandleKey('c')
	if s.Input != "abc" {
		t.Fatalf("Input = %q, want abc", s.Input)
	}
}

func TestHandleKeyIgnoresWhenUnfocused(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusNone

	called := false
	s.OnNavigate = func(url string) { called = true }

	s.HandleKey('\r')
	if called {
		t.Fatal("HandleKey submitted when unfocused")
	}
}

func TestClickAddressBarFocuses(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.URL = "https://example.com"
	s.Input = "https://example.com"

	bar := toolbar.AddressBarRect(800)
	mid := frame.Point{X: (bar.X0 + bar.X1) / 2, Y: (bar.Y0 + bar.Y1) / 2}

	s.HandleClick(mid, surface.ButtonLeft)
	if s.Focus != toolbar.FocusAddress {
		t.Fatalf("Focus = %d after address bar click, want FocusAddress", s.Focus)
	}
}

func TestClickOutsideUnfocuses(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusAddress

	below := frame.Point{X: 100, Y: toolbar.ToolbarHeight + 10}
	s.HandleClick(below, surface.ButtonLeft)
	if s.Focus != toolbar.FocusNone {
		t.Fatalf("Focus = %d after click below toolbar, want FocusNone", s.Focus)
	}
}

func TestClickBackButton(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.History.Push("https://a.com")
	s.History.Push("https://b.com")

	var navigated string
	s.OnTraverse = func(delta int) {
		if delta != -1 {
			t.Fatalf("back delta = %d", delta)
		}
		navigated = "https://a.com"
	}

	rect := toolbar.ButtonRect(toolbar.ButtonBack, 800)
	mid := frame.Point{X: (rect.X0 + rect.X1) / 2, Y: (rect.Y0 + rect.Y1) / 2}
	s.HandleClick(mid, surface.ButtonLeft)

	if navigated != "https://a.com" {
		t.Fatalf("OnNavigate = %q, want https://a.com", navigated)
	}
}

func TestClickForwardButton(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.History.Push("https://a.com")
	s.History.Push("https://b.com")
	s.History.Back()

	var navigated string
	s.OnTraverse = func(delta int) {
		if delta != 1 {
			t.Fatalf("forward delta = %d", delta)
		}
		navigated = "https://b.com"
	}

	rect := toolbar.ButtonRect(toolbar.ButtonForward, 800)
	mid := frame.Point{X: (rect.X0 + rect.X1) / 2, Y: (rect.Y0 + rect.Y1) / 2}
	s.HandleClick(mid, surface.ButtonLeft)

	if navigated != "https://b.com" {
		t.Fatalf("OnNavigate = %q, want https://b.com", navigated)
	}
}

func TestClickReloadButton(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.URL = "https://example.com"

	var navigated string
	s.OnReload = func() { navigated = s.URL }

	rect := toolbar.ButtonRect(toolbar.ButtonReload, 800)
	mid := frame.Point{X: (rect.X0 + rect.X1) / 2, Y: (rect.Y0 + rect.Y1) / 2}
	s.HandleClick(mid, surface.ButtonLeft)

	if navigated != "https://example.com" {
		t.Fatalf("OnNavigate = %q, want https://example.com", navigated)
	}
}

func TestClickRightButtonIgnored(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusAddress

	bar := toolbar.AddressBarRect(800)
	mid := frame.Point{X: (bar.X0 + bar.X1) / 2, Y: (bar.Y0 + bar.Y1) / 2}

	s.HandleClick(mid, surface.ButtonRight)
	if s.Focus != toolbar.FocusAddress {
		t.Fatalf("Focus changed on right click, want still FocusAddress")
	}
}

func TestNavigateUpdatesState(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Navigate("https://first.com")
	s.Navigate("https://example.com")

	if s.URL != "https://example.com" {
		t.Fatalf("URL = %q, want https://example.com", s.URL)
	}
	if s.Input != "https://example.com" {
		t.Fatalf("Input = %q, want https://example.com", s.Input)
	}
	if s.Focus != toolbar.FocusNone {
		t.Fatalf("Focus = %d after Navigate, want FocusNone", s.Focus)
	}
	if !s.History.CanBack() {
		t.Fatal("CanBack() = false after two Navigates")
	}
}

func TestLayoutButtonRects(t *testing.T) {
	w := int32(800)
	back := toolbar.ButtonRect(toolbar.ButtonBack, w)
	fwd := toolbar.ButtonRect(toolbar.ButtonForward, w)
	reload := toolbar.ButtonRect(toolbar.ButtonReload, w)

	if back.X0 < 0 || back.Y0 < 0 {
		t.Fatalf("back button rect has negative coords: %v", back)
	}
	if fwd.X0 <= back.X0 {
		t.Fatalf("forward button not to the right of back: %v vs %v", fwd, back)
	}
	if reload.X0 <= fwd.X0 {
		t.Fatalf("reload button not to the right of forward: %v vs %v", reload, fwd)
	}
	if back.H() != toolbar.ButtonSize || back.W() != toolbar.ButtonSize {
		t.Fatalf("back button size = %dx%d, want %dx%d", back.W(), back.H(), toolbar.ButtonSize, toolbar.ButtonSize)
	}
}

func TestLayoutAddressBar(t *testing.T) {
	w := int32(800)
	bar := toolbar.AddressBarRect(w)
	reload := toolbar.ButtonRect(toolbar.ButtonReload, w)

	if bar.X0 <= reload.X1 {
		t.Fatalf("address bar overlaps reload button: bar.X0=%d <= reload.X1=%d", bar.X0, reload.X1)
	}
	if bar.X1 > w {
		t.Fatalf("address bar extends past toolbar width: bar.X1=%d > %d", bar.X1, w)
	}
	if bar.H() != toolbar.ButtonSize {
		t.Fatalf("address bar height = %d, want %d", bar.H(), toolbar.ButtonSize)
	}
}

func TestDeleteBackwardAtStart(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusAddress
	s.Input = "abc"
	s.Cursor = 0

	s.DeleteBackward()
	if s.Input != "abc" {
		t.Fatalf("DeleteBackward at cursor 0 changed Input to %q", s.Input)
	}
}

func TestInsertInMiddle(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Focus = toolbar.FocusAddress
	s.Input = "hllo"
	s.Cursor = 1

	s.InsertRune('e')
	if s.Input != "hello" {
		t.Fatalf("Input = %q after mid-insert, want hello", s.Input)
	}
	if s.Cursor != 2 {
		t.Fatalf("Cursor = %d after mid-insert, want 2", s.Cursor)
	}
}

func TestOnTraverseCallback(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Navigate("https://a.com")
	s.Navigate("https://b.com")

	// OnTraverse should be called with the delta; simulate what the real
	// traverse does by moving the history index.
	s.OnTraverse = func(delta int) {
		if delta < 0 {
			s.History.Back()
		} else if delta > 0 {
			s.History.Forward()
		}
	}

	// Click back button.
	r := toolbar.ButtonRect(toolbar.ButtonBack, 800)
	s.HandleClick(frame.Point{X: r.X0 + 1, Y: r.Y0 + 1}, surface.ButtonLeft)
	if got := s.History.Current(); got != "https://a.com" {
		t.Fatalf("after back: Current() = %q, want https://a.com", got)
	}

	// Click forward button.
	r = toolbar.ButtonRect(toolbar.ButtonForward, 800)
	s.HandleClick(frame.Point{X: r.X0 + 1, Y: r.Y0 + 1}, surface.ButtonLeft)
	if got := s.History.Current(); got != "https://b.com" {
		t.Fatalf("after forward: Current() = %q, want https://b.com", got)
	}
}

func TestOnReloadCallback(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.Navigate("https://example.com")

	called := false
	s.OnReload = func() { called = true }

	r := toolbar.ButtonRect(toolbar.ButtonReload, 800)
	s.HandleClick(frame.Point{X: r.X0 + 1, Y: r.Y0 + 1}, surface.ButtonLeft)
	if !called {
		t.Fatal("OnReload not called")
	}
}

func TestLoadingState(t *testing.T) {
	s := toolbar.NewState(800, nil)
	if s.Loading {
		t.Fatal("Loading = true initially")
	}
	s.SetLoading(true)
	if !s.Loading {
		t.Fatal("SetLoading(true) did not set Loading")
	}
	s.SetLoading(false)
	if s.Loading {
		t.Fatal("SetLoading(false) did not clear Loading")
	}
}

func TestErrorState(t *testing.T) {
	s := toolbar.NewState(800, nil)
	if s.Error != "" {
		t.Fatalf("Error = %q initially, want empty", s.Error)
	}
	s.Error = "fetch failed"
	if s.Error != "fetch failed" {
		t.Fatalf("Error = %q, want 'fetch failed'", s.Error)
	}
}

func TestFindOpenSwapsInputAndFocus(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.URL = "https://example.com"

	s.OpenFind()
	if !s.FindActive {
		t.Fatal("FindActive = false after OpenFind")
	}
	if s.Focus != toolbar.FocusAddress {
		t.Fatalf("Focus = %d after OpenFind, want FocusAddress", s.Focus)
	}
	if s.Input != "" {
		t.Fatalf("Input = %q after first OpenFind, want empty", s.Input)
	}
}

func TestFindQueryRestoredOnReopen(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.URL = "https://example.com"

	s.OpenFind()
	s.HandleKey('n')
	s.HandleKey('e')
	s.HandleKey('e')
	s.HandleKey('d')
	if s.Input != "need" {
		t.Fatalf("Input = %q while typing, want need", s.Input)
	}
	s.HandleKey(0x1b) // Esc closes and saves.
	if s.FindActive {
		t.Fatal("FindActive still true after Esc")
	}

	s.OpenFind()
	if s.Input != "need" {
		t.Fatalf("Input = %q on reopen, want previous query need", s.Input)
	}
}

func TestFindCloseRestoresURL(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.URL = "https://example.com"

	s.OpenFind()
	s.HandleKey('q')
	s.CloseFind()
	if s.FindActive {
		t.Fatal("FindActive still true after CloseFind")
	}
	if s.Input != "https://example.com" {
		t.Fatalf("Input = %q after CloseFind, want the URL", s.Input)
	}
	if s.FindQuery != "q" {
		t.Fatalf("FindQuery = %q after CloseFind, want q", s.FindQuery)
	}
	if s.Focus != toolbar.FocusNone {
		t.Fatalf("Focus = %d after CloseFind, want FocusNone", s.Focus)
	}
}

func TestFindEnterStepsAndShiftReverses(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.OpenFind()

	var steps []bool
	s.OnFindNext = func(backward bool) { steps = append(steps, backward) }

	s.HandleKey('\r')
	if len(steps) != 1 || steps[0] {
		t.Fatalf("first Enter = %v, want one forward step", steps)
	}
	s.HandleKeyEvent('\r', surface.ModShift)
	if len(steps) != 2 || !steps[1] {
		t.Fatalf("Shift+Enter = %v, want one backward step", steps)
	}
}

func TestFindEscCallsClose(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.OpenFind()

	closed := false
	s.OnFindClose = func() { closed = true }

	s.HandleKey(0x1b)
	if !closed {
		t.Fatal("OnFindClose not called on Esc")
	}
	if s.FindActive {
		t.Fatal("FindActive still true after Esc")
	}
}

func TestFindTypingReportsQuery(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.OpenFind()

	var got string
	s.OnFindChanged = func(query string) { got = query }

	s.HandleKey('h')
	s.HandleKey('i')
	if got != "hi" {
		t.Fatalf("OnFindChanged = %q after two keys, want hi", got)
	}
	if s.FindQuery != "hi" {
		t.Fatalf("FindQuery = %q, want hi", s.FindQuery)
	}
}

func TestFindOutsideClickCloses(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.URL = "https://example.com"
	s.OpenFind()

	closed := false
	s.OnFindClose = func() { closed = true }

	s.HandleClick(frame.Point{X: 400, Y: toolbar.ToolbarHeight + 5}, surface.ButtonLeft)
	if s.FindActive {
		t.Fatal("FindActive still true after outside click")
	}
	if !closed {
		t.Fatal("OnFindClose not called on outside click")
	}
	if s.Input != "https://example.com" {
		t.Fatalf("Input = %q after outside click, want the URL", s.Input)
	}
}

func TestCursorAtShapes(t *testing.T) {
	s := toolbar.NewState(800, nil)

	back := toolbar.ButtonRect(toolbar.ButtonBack, 800)
	if c := s.CursorAt(frame.Point{X: back.X0 + 1, Y: back.Y0 + 1}); c != surface.CursorPointer {
		t.Fatalf("CursorAt over Back = %v, want CursorPointer", c)
	}
	addr := toolbar.AddressBarRect(800)
	if c := s.CursorAt(frame.Point{X: addr.X0 + 1, Y: addr.Y0 + 1}); c != surface.CursorText {
		t.Fatalf("CursorAt over the address bar = %v, want CursorText", c)
	}
	if c := s.CursorAt(frame.Point{X: 400, Y: int32(toolbar.ToolbarHeight) + 5}); c != surface.CursorDefault {
		t.Fatalf("CursorAt below the toolbar = %v, want CursorDefault", c)
	}
}

func TestCursorAtScalesWithDevicePixels(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.SetScale(2)

	// CursorAt takes device pixels, like HandleClick: a hover at twice the
	// logical Back-button rect must still read as the button.
	back := toolbar.ButtonRect(toolbar.ButtonBack, 800)
	pos := frame.Point{X: (back.X0+1)*2 + back.W()/2, Y: (back.Y0 + 1) * 2}
	if c := s.CursorAt(pos); c != surface.CursorPointer {
		t.Fatalf("CursorAt(%v) at scale 2 = %v, want CursorPointer", pos, c)
	}
}

func TestFindBackspaceStillReportsQuery(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.OpenFind()
	s.HandleKey('a')
	s.HandleKey('b')

	cleared := ""
	s.OnFindChanged = func(query string) { cleared = query }

	s.HandleKey(0x7f)
	if cleared != "a" {
		t.Fatalf("OnFindChanged = %q after backspace, want a", cleared)
	}
	if s.FindQuery != "a" {
		t.Fatalf("FindQuery = %q after backspace, want a", s.FindQuery)
	}
}
