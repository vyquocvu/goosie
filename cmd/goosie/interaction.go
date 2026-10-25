package main

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
	"github.com/vyquocvu/goosie/internal/viewsource"
)

// focusClick focuses or blurs a form control under a content-area click, handles
// activatable controls (checkbox, radio, submit/reset button), and repaints.
func (f *framePath) focusClick(contentX, contentY int32) bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return false
	}
	dx, dy := f.docPoint(contentX, contentY)

	// Check if click hit an activatable control (checkbox, radio, submit button, reset button)
	// The click is a trusted gesture, and marking it as one around the dispatch is
	// what lets a page's own click handler start playback: the engine refuses a
	// play() that arrives from a timer or a synthetic event, and only the host can
	// tell the difference.
	tab.Session().SetMediaActivation(true)
	activated := tab.Session().ClickAt(dx, dy)
	tab.Session().SetMediaActivation(false)
	if activated {
		if sub, ok := tab.Session().PopSubmittedForm(); ok {
			f.handleFormSubmission(tab, sub)
			return true
		}
		sess := tab.Session()
		if sess.CanDeferControlEditReflow() {
			f.repaintTab(tab, frame.Rect{})
			return true
		}
		var err error
		if sess.HasPseudoClassRule("checked") || sess.HasPseudoClassRule("focus") || sess.HasPseudoClassRule("focus-visible") || sess.HasPseudoClassRule("active") {
			err = sess.RestyleAndReflow(float32(f.config.width))
		} else {
			err = sess.Reflow(float32(f.config.width))
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
		}
		f.repaintTab(tab, frame.Rect{})
		return true
	}

	sess := tab.Session()
	if !sess.FocusControl(dx, dy) {
		return false
	}
	f.setWindowIME(sess.Focused() != nil)
	if !sess.CanDeferControlEditReflow() && (sess.HasPseudoClassRule("focus") || sess.HasPseudoClassRule("focus-within") || sess.HasPseudoClassRule("focus-visible") || sess.HasPseudoClassRule("active")) {
		if err := sess.RestyleAndReflow(float32(f.config.width)); err != nil {
			fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
		}
	}
	f.repaintTab(tab, frame.Rect{})
	return true
}

// hoverContent updates page hover state when the pointer changes its topmost
// element. Re-resolve only when author CSS observes :hover.
func (f *framePath) hoverContent(contentX, contentY int32) {
	if f.tabMgr == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}
	sess := tab.Session()
	dx, dy := f.docPoint(contentX, contentY)
	if !sess.SetHover(dx, dy) {
		return
	}
	if !sess.HasPseudoClassRule("hover") || sess.CanDeferControlEditReflow() {
		return
	}
	if err := sess.RestyleAndReflow(float32(f.config.width)); err != nil {
		fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
	}
	f.repaintTab(tab, frame.Rect{})
}

// setContentActive updates :active from primary-pointer input. Restyling is
// deferred to the click/focus path on press so one interaction performs one
// style/layout pass when both :active and :focus rules are present.
func (f *framePath) setContentActive(contentX, contentY int32, pressed bool) bool {
	if f.tabMgr == nil {
		return false
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return false
	}
	dx, dy := f.docPoint(contentX, contentY)
	return tab.Session().SetActiveAt(dx, dy, pressed)
}

// refreshActiveContent applies a pending active-state change after event routing
// when no focus/control path already included it in its pipeline update.
func (f *framePath) refreshActiveContent() {
	if f.tabMgr == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil || !tab.Session().HasPseudoClassRule("active") {
		return
	}
	if err := tab.Session().RestyleAndReflow(float32(f.config.width)); err != nil {
		fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
	}
	f.repaintTab(tab, frame.Rect{})
}

// setWindowIME turns the platform input context on while a document control has
// focus and off whenever focus moves anywhere else, so an IME can only compose
// into the document.
func (f *framePath) setWindowIME(on bool) {
	if f.window != nil {
		f.window.SetIME(on)
	}
}

// imeEvent routes a platform composition event to the active tab's session. A
// marked update repaints the preview; a commit reflows the value. Both are
// consumed whether or not the session was waiting, because the platform input
// context is only ever on for the active tab's control.
func (f *framePath) imeEvent(ev surface.Event) bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return false
	}
	switch ev.IME {
	case surface.IMEMarked:
		if tab.Session().SetMarked(ev.Text) {
			f.repaintTab(tab, frame.Rect{})
		}
		return true
	case surface.IMECommit:
		if tab.Session().CommitText(ev.Text) {
			f.reflowAfterControlEdit(tab)
		}
		f.repaintTab(tab, frame.Rect{})
		return true
	}
	return false
}

// dragSelect drives text selection from content-area pointer events. A press
// starts a selection unless a form control is focused, motion extends the
// active one, and release just ends the gesture.
func (f *framePath) dragSelect(action surface.PointerAction, contentX, contentY int32) bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return false
	}
	switch action {
	case surface.PointerPress:
		if tab.Session().Focused() != nil {
			return false
		}
		dx, dy := f.docPoint(contentX, contentY)
		oldR, oldOK := tab.Session().SelectionBounds()
		changed := tab.Session().SelectAt(dx, dy)
		f.selDrag = true
		if changed {
			f.repaintTab(tab, f.selectionDamage(tab, oldR, oldOK))
		}
		return true
	case surface.PointerMotion:
		if !f.selDrag {
			return false
		}
		dx, dy := f.docPoint(contentX, contentY)
		oldR, oldOK := tab.Session().SelectionBounds()
		if !tab.Session().SelectTo(dx, dy) {
			return true
		}
		f.repaintTab(tab, f.selectionDamage(tab, oldR, oldOK))
		return true
	case surface.PointerRelease:
		wasDragging := f.selDrag
		f.selDrag = false
		return wasDragging
	}
	return false
}

// copySelection puts the selected text on the system clipboard.
func (f *framePath) copySelection() bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil || !tab.Session().HasSelection() {
		return false
	}
	if cb := platform.NewClipboard(); cb != nil {
		cb.Write(tab.Session().SelectionText())
	}
	return true
}

func (f *framePath) reflowAfterControlEdit(tab *tabs.Tab) {
	if tab == nil || tab.Session() == nil {
		return
	}
	sess := tab.Session()
	// Edit dispatches input synchronously. If its handler changed the DOM,
	// PumpJS will re-resolve styles and reflow the final state on the next loop
	// turn. Reflowing here as well would do a full layout immediately before
	// that authoritative pass.
	if sess.CanDeferControlEditReflow() {
		return
	}
	var err error
	if sess.HasPseudoClassRule("placeholder-shown") {
		err = sess.RestyleAndReflow(float32(f.config.width))
	} else if sess.FocusedInputValueDoesNotAffectLayout() {
		return
	} else {
		err = sess.Reflow(float32(f.config.width))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
	}
}

// contentKey applies a keyboard edit to the focused control. The event is
// consumed whenever a control is focused so the address bar is not the only
// text sink; only edits that can affect layout trigger a reflow.
func (f *framePath) contentKey(key rune, mods surface.KeyMod) bool {
	if f.tabMgr == nil {
		return false
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return false
	}

	cmd := mods&surface.ModCommand != 0 || mods&surface.ModControl != 0
	// A focused video owns the media keys before scrolling and before the page:
	// space on a playing video is a pause, not a page down. The press is a trusted
	// gesture, which is what lets the element start playing in response to it.
	if !cmd && f.mediaKey(tab, key) {
		return true
	}
	if !cmd {
		sess := tab.Session()
		focused := sess.Focused()
		enter := key == '\r' || key == '\n'
		space := key == ' '
		if focused != nil && (enter && engine.IsActivatable(focused) || space && engine.IsSpaceActivatable(focused)) {
			keyName := "Enter"
			if space {
				keyName = " "
			}
			sess.SetMediaActivation(true)
			activated := sess.PressKey(keyName, mods&surface.ModShift != 0, false, mods&surface.ModOption != 0, false)
			sess.SetMediaActivation(false)
			if activated && enter && focused.Data == "a" {
				if href := focused.GetAttribute("href"); href != "" {
					f.handleLinkClick(href)
					return true
				}
			}
			if sub, ok := sess.PopSubmittedForm(); ok {
				f.handleFormSubmission(tab, sub)
				return true
			}
			if !sess.CanDeferControlEditReflow() {
				var err error
				if sess.HasPseudoClassRule("checked") || sess.HasPseudoClassRule("focus") || sess.HasPseudoClassRule("focus-visible") || sess.HasPseudoClassRule("active") {
					err = sess.RestyleAndReflow(float32(f.config.width))
				} else {
					err = sess.Reflow(float32(f.config.width))
				}
				if err != nil {
					fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
				}
			}
			f.repaintTab(tab, frame.Rect{})
			return true
		}
	}
	if !cmd && !tab.Session().FocusedEditable() {
		if f.scrollContentKey(tab, key, mods&surface.ModShift != 0) {
			return true
		}
	} else if !cmd && (key == keyPageUp || key == keyPageDown) {
		// Page keys must never be treated as printable private-use characters by
		// an editable control. Textarea-specific paging is not implemented yet.
		return true
	}

	// Bare Tab drives the full key-event path (keydown → focus-move → keyup).
	// It must work with nothing focused so it can start the tab cycle, so this
	// is checked before the focused-control bail below. Cmd/Ctrl+Tab is a
	// browser-tab shortcut and is consumed earlier in the chrome window.
	if key == '\t' && !cmd {
		shift := mods&surface.ModShift != 0
		alt := mods&surface.ModOption != 0
		sess := tab.Session()
		oldFocus := sess.Focused()
		sess.PressKey("Tab", shift, false, alt, false)
		focusChanged := sess.Focused() != oldFocus
		observesFocus := sess.HasPseudoClassRule("focus") || sess.HasPseudoClassRule("focus-within") || sess.HasPseudoClassRule("focus-visible")
		if focusChanged && observesFocus && !sess.CanDeferControlEditReflow() {
			if err := sess.RestyleAndReflow(float32(f.config.width)); err != nil {
				fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
			}
		} else {
			f.reflowAfterControlEdit(tab)
		}
		f.repaintTab(tab, frame.Rect{})
		return true
	}

	if tab.Session().Focused() == nil {
		if cmd && (key == 'a' || key == 'A') {
			if tab.Session().SelectAll() {
				f.repaintTab(tab, frame.Rect{})
				return true
			}
		}
		return false
	}

	if cmd {
		switch key {
		case 'v', 'V':
			if tab.Session().FocusedEditable() {
				if cb := platform.NewClipboard(); cb != nil {
					clipText := cb.Read()
					if clipText != "" {
						if tab.Session().InsertText(clipText) {
							f.reflowAfterControlEdit(tab)
							f.repaintTab(tab, frame.Rect{})
							return true
						}
					}
				}
			}
		case 'a', 'A':
			if tab.Session().FocusedEditable() {
				if tab.Session().SelectAllControl() {
					f.repaintTab(tab, frame.Rect{})
					return true
				}
			} else {
				if tab.Session().SelectAll() {
					f.repaintTab(tab, frame.Rect{})
					return true
				}
			}
		case 'c', 'C':
			if tab.Session().FocusedEditable() {
				txt := tab.Session().FocusedText()
				if txt != "" {
					if cb := platform.NewClipboard(); cb != nil {
						cb.Write(txt)
						return true
					}
				}
			}
		}
		return false
	}

	var action engine.EditAction
	switch {
	case key == 0xF702:
		action = engine.EditLeft
	case key == 0xF703:
		action = engine.EditRight
	case key == 0xF700:
		action = engine.EditUp
	case key == 0xF701:
		action = engine.EditDown
	case key == 0xF704:
		action = engine.EditHome
	case key == 0xF705:
		action = engine.EditEnd
	case key == 0x7f || key == '\b':
		action = engine.EditBackspace
	case key == '\r' || key == '\n':
		action = engine.EditEnter
	case key == 0x1b:
		action = engine.EditEscape
	case key >= 0x20:
		action = engine.EditRune
	default:
		return false
	}

	changed := tab.Session().Edit(action, key)
	if sub, ok := tab.Session().PopSubmittedForm(); ok {
		f.handleFormSubmission(tab, sub)
		return true
	}
	if changed {
		if action == engine.EditEscape {
			f.setWindowIME(false)
		}
		f.reflowAfterControlEdit(tab)
	}
	f.repaintTab(tab, frame.Rect{})
	return true
}

// scrollContentKey handles document navigation keys when an editable control does
// not own focus. The viewport stores device pixels, so a page step uses its device
// height and the active layer supplies the legal bottom edge.
func (f *framePath) scrollContentKey(tab *tabs.Tab, key rune, shift bool) bool {
	if f.sched == nil || tab == nil || tab.Layer() == nil {
		return false
	}
	vp := f.sched.Viewport()
	page := vp.Size.H * 9 / 10
	if page < 1 {
		page = 1
	}
	switch key {
	case ' ':
		if shift {
			return f.scrollContentTo(tab, vp, vp.Offset.Y-page)
		}
		return f.scrollContentTo(tab, vp, vp.Offset.Y+page)
	case keyPageUp:
		return f.scrollContentTo(tab, vp, vp.Offset.Y-page)
	case keyPageDown:
		return f.scrollContentTo(tab, vp, vp.Offset.Y+page)
	case 0xF704: // Home
		return f.scrollContentTo(tab, vp, 0)
	case 0xF705: // End
		return f.scrollContentTo(tab, vp, tab.Layer().Bounds.Y1-vp.Size.H)
	default:
		return false
	}
}

func (f *framePath) scrollContentTo(tab *tabs.Tab, vp frame.Viewport, y int32) bool {
	if tab == nil || tab.Layer() == nil || f.sched == nil {
		return false
	}
	maxY := tab.Layer().Bounds.Y1 - vp.Size.H
	if maxY < 0 {
		maxY = 0
	}
	if y < 0 {
		y = 0
	} else if y > maxY {
		y = maxY
	}
	if vp.Offset.Y != y {
		vp.Offset.Y = y
		f.sched.SetViewport(vp)
		tab.SetScrollY(y)
		f.publishAccessibility()
	}
	return true
}

// handleFormSubmission dispatches a form submission by resolving action and encoding parameters.
func (f *framePath) handleFormSubmission(tab *tabs.Tab, sub *engine.FormSubmission) {
	if tab == nil || sub == nil {
		return
	}
	targetURL := resolveHref(tab.URL(), sub.Action)
	if targetURL == "" {
		targetURL = tab.URL()
	}
	method := strings.ToUpper(sub.Method)
	if method == "" || method == "GET" {
		if sub.Encoded != "" {
			if strings.Contains(targetURL, "?") {
				targetURL = targetURL + "&" + sub.Encoded
			} else {
				targetURL = targetURL + "?" + sub.Encoded
			}
		}
		f.navigateTab(targetURL)
	} else if method == "POST" {
		f.navigateTab(targetURL)
	}
}

// handleContextMenu constructs the context menu for the clicked element.
func (f *framePath) handleContextMenu(contentX, contentY, winX, winY int32) {
	cw, ok := f.window.(*chromeWindow)
	if !ok || cw.contextMenu == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}
	docX, docY := f.docPoint(contentX, contentY)
	linkURL := tab.Session().HitTestLink(docX, docY)
	if linkURL != "" {
		linkURL = resolveHref(tab.URL(), linkURL)
	}
	imageURL := tab.Session().HitTestImage(docX, docY)
	if imageURL != "" {
		imageURL = resolveHref(tab.URL(), imageURL)
	}
	selText := tab.Session().SelectionText()
	isEditable := tab.Session().FocusedEditable()
	focusedText := tab.Session().FocusedText()
	_, _, canBack := tab.History().PeekBack()
	_, _, canForward := tab.History().PeekForward()

	items := BuildContextMenuItems(linkURL, imageURL, selText, focusedText, isEditable, canBack, canForward)
	winW := int32(f.config.width) * cw.sc()
	winH := int32(f.config.height) * cw.sc()
	if cw.chromeBitmap != nil {
		winW = int32(cw.chromeBitmap.W)
	}
	if cw.outBitmap != nil {
		winH = int32(cw.outBitmap.H)
	}
	cw.contextMenu.Show(winX, winY, winW, winH, items, cw.sc())
}

// handleContextMenuAction executes the action chosen from the context menu.
func (f *framePath) handleContextMenuAction(action, data string) {
	switch action {
	case "open_new_tab":
		f.newTab()
		if data != "" {
			f.navigateTab(data)
		}
	case "copy_link", "copy_image":
		if cb := platform.NewClipboard(); cb != nil {
			cb.Write(data)
		}
	case "open_image_tab":
		f.newTab()
		if data != "" {
			f.navigateTab(data)
		}
	case "copy":
		if data != "" {
			if cb := platform.NewClipboard(); cb != nil {
				cb.Write(data)
			}
		} else {
			f.copySelection()
		}
	case "cut":
		if tab := f.tabMgr.Active(); tab != nil && tab.Session() != nil {
			txt := tab.Session().FocusedText()
			if cb := platform.NewClipboard(); cb != nil {
				cb.Write(txt)
			}
			tab.Session().SelectAllControl()
			tab.Session().InsertText("")
			f.repaintTab(tab, frame.Rect{})
		}
	case "paste":
		if cb := platform.NewClipboard(); cb != nil {
			if text := cb.Read(); text != "" {
				if tab := f.tabMgr.Active(); tab != nil && tab.Session() != nil {
					if tab.Session().InsertText(text) {
						if err := tab.Session().Reflow(float32(f.config.width)); err != nil {
							fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
						}
					}
					f.repaintTab(tab, frame.Rect{})
				}
			}
		}
	case "select_all":
		if tab := f.tabMgr.Active(); tab != nil && tab.Session() != nil {
			if tab.Session().FocusedEditable() {
				tab.Session().SelectAllControl()
			} else {
				tab.Session().SelectAll()
			}
			f.repaintTab(tab, frame.Rect{})
		}
	case "search_google":
		if data != "" {
			searchURL := "https://www.google.com/search?q=" + url.QueryEscape(data)
			f.navigateTab(searchURL)
		}
	case "back":
		f.traverseTab(-1)
	case "forward":
		f.traverseTab(1)
	case "reload":
		f.reloadTab()
	case "view_source":
		f.viewSourceCurrentTab()
	case "inspect":
		f.navigateTab("goosie://inspect")
	case "print_pdf":
		f.printActiveTabToPDF()
	}
}

// viewSourceCurrentTab opens the view-source representation of the current tab in a new tab.
func (f *framePath) viewSourceCurrentTab() {
	if f.tabMgr == nil {
		return
	}
	active := f.tabMgr.Active()
	if active == nil || active.URL() == "" {
		return
	}
	f.newTab()
	f.navigateTab(viewsource.WrapViewSourceURL(active.URL()))
}

// printActiveTabToPDF exports the currently active tab document to PDF.
func (f *framePath) printActiveTabToPDF() {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}
	outPath := filepath.Join(f.downloadDirectory(), "print.pdf")
	if err := ExportSessionToPDF(tab.Session(), f.fonts, float32(f.config.dpr), outPath); err != nil {
		fmt.Fprintf(os.Stderr, "goosie: print to PDF error: %v\n", err)
		return
	}
	fmt.Printf("goosie: document printed to PDF at %s\n", outPath)
}

const (
	// minZoom and maxZoom bound the page zoom factor; zoomStep is one
	// Cmd+=/Cmd+- step.
	minZoom  = 0.25
	maxZoom  = 5.0
	zoomStep = 0.1
)

func clampZoom(z float64) float64 {
	if z < minZoom {
		return minZoom
	}
	if z > maxZoom {
		return maxZoom
	}
	return z
}

// ZoomReset restores the page to 100%.
func (f *framePath) ZoomReset() {
	f.handleZoom(0)
}

// ZoomPercent reports the current zoom as a whole-percent label, e.g. "125%".
func (f *framePath) ZoomPercent() string {
	return toolbar.FormatZoomPercent(f.zoom)
}

func (f *framePath) handleZoom(delta float64) {
	if f.zoom <= 0 || math.IsNaN(f.zoom) || math.IsInf(f.zoom, 0) {
		f.zoom = 1.0
	}
	switch {
	case delta == 0:
		f.zoom = 1.0
	case math.IsNaN(delta):
		// A NaN step has no direction; keep the current level.
	default:
		f.zoom = clampZoom(f.zoom + delta)
	}
	if f.toolbar != nil {
		f.toolbar.SetZoom(f.zoom)
	}
	f.handleResize(f.config.width, f.config.height)
}

func (f *framePath) handleLinkClick(href string) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	f.navigateTab(resolveHref(tab.URL(), href))
}

func (f *framePath) hitTestLink(contentX, contentY int32) string {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return ""
	}
	docX, docY := f.docPoint(contentX, contentY)
	return tab.Session().HitTestLink(docX, docY)
}

// hoverCursor resolves the pointer shape for buttonless motion over the
// document: the hand over a link, the I-beam over editable or selectable
// text, and the default arrow everywhere else. Link wins over text because
// a link's words are text too but only the link is clickable.
func (f *framePath) hoverCursor(contentX, contentY int32) surface.Cursor {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return surface.CursorDefault
	}
	dx, dy := f.docPoint(contentX, contentY)
	if tab.Session().HitTestLink(dx, dy) != "" {
		return surface.CursorPointer
	}
	if tab.Session().HasControlAt(dx, dy) || tab.Session().HasTextAt(dx, dy) {
		return surface.CursorText
	}
	return surface.CursorDefault
}

// runFind recomputes the match set for a new find query on the active tab and
// jumps to the first hit.
func (f *framePath) runFind(query string) {
	if f.toolbar == nil {
		return
	}
	f.findMatches = nil
	f.findIdx = 0
	f.toolbar.ResetFindCounts()
	if strings.TrimSpace(query) == "" {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}
	f.findMatches = tab.Session().Find(query)
	f.toolbar.SetFindResult(len(f.findMatches), 0)
	if len(f.findMatches) > 0 {
		f.scrollToMatch(0)
	}
}

// findStep moves to the next (or previous) match, wrapping around.
func (f *framePath) findStep(backward bool) {
	if len(f.findMatches) == 0 {
		return
	}
	if backward {
		f.findIdx--
		if f.findIdx < 0 {
			f.findIdx = len(f.findMatches) - 1
		}
	} else {
		f.findIdx++
		if f.findIdx >= len(f.findMatches) {
			f.findIdx = 0
		}
	}
	f.toolbar.SetFindResult(len(f.findMatches), f.findIdx)
	f.scrollToMatch(f.findIdx)
}

// scrollToMatch centers the match's line in the upper part of the viewport.
func (f *framePath) scrollToMatch(idx int) {
	if idx < 0 || idx >= len(f.findMatches) {
		return
	}
	m := f.findMatches[idx]
	effectiveDPR := float32(f.config.dpr) * float32(f.zoom)
	size := f.config.devSize()
	offsetY := int32(m.Y0*effectiveDPR) - size.H/4
	if offsetY < 0 {
		offsetY = 0
	}
	f.sched.SetViewport(frame.Viewport{Offset: frame.Point{Y: offsetY}, Size: size})
}

// closeFind drops the match state; navigation and tab switches end find mode.
func (f *framePath) closeFind() {
	f.findMatches = nil
	f.findIdx = 0
	if f.toolbar == nil {
		return
	}
	f.toolbar.ResetFindCounts()
	if active, _, _ := f.toolbar.FindCounts(); active {
		f.toolbar.CloseFind()
	}
}

// switchTab saves the current tab's scroll and switches to the given tab.
func (f *framePath) switchTab(id uint64) {
	f.closeFind()
	f.setWindowIME(false)
	cur := f.tabMgr.Active()
	if cur != nil {
		vp := f.sched.Viewport()
		cur.SetScrollY(vp.Offset.Y)
		// The departing tab stops making noise. Nothing resumes it when the user
		// comes back, which is the same bargain a real browser strikes: a tab that
		// restarts on its own is a tab that starts playing without being asked.
		f.pauseMedia(cur)
	}
	f.tabMgr.SwitchTo(id)
	newTab := f.tabMgr.Active()
	if newTab == nil {
		return
	}
	f.ensureActiveTabVisible()
	f.syncTabToToolbar()
	if newTab.Layer() != nil {
		f.sched.SetPlan(frame.FramePlan{
			Serial:     newTab.Nav.Serial,
			Layers:     []*frame.Layer{newTab.Layer()},
			Background: newTab.BGColor(),
		})
	}
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: newTab.ScrollY()},
		Size:   f.config.devSize(),
	})
	f.publishAccessibility()
}

// newTab creates a new tab and switches to it.
func (f *framePath) newTab() {
	if f.tabMgr == nil {
		return
	}
	f.closeFind()
	cur := f.tabMgr.Active()
	if cur != nil {
		vp := f.sched.Viewport()
		cur.SetScrollY(vp.Offset.Y)
	}
	tab := f.tabMgr.NewTab()
	f.tabMgr.SwitchTo(tab.ID())
	f.ensureActiveTabVisible()
	f.syncTabToToolbar()
	_, layer := blankLayer(f.config.devSize())
	tab.SetLayer(layer)
	tab.SetBGColor(frame.RGB(255, 255, 255))
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor(),
	})
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: 0},
		Size:   f.config.devSize(),
	})
	f.publishAccessibility()
}

// ensureActiveTabVisible scrolls a clipped tab strip just enough to show the
// active tab: keyboard switches, new tabs and session restores land on tabs
// the pointer could never reach otherwise.
func (f *framePath) ensureActiveTabVisible() {
	if f.tabMgr == nil {
		return
	}
	n := int32(f.tabMgr.Count())
	w := int32(f.config.width)
	idx := f.tabMgr.ActiveIndex()
	f.tabMgr.SetTabScroll(tabs.EnsureTabVisible(idx, n, w, f.tabMgr.TabScroll()), n, w)
}

// closeTab closes a tab by ID and switches to the remaining active tab.
func (f *framePath) closeTab(id uint64) {
	if f.tabMgr == nil {
		return
	}
	f.closeFind()
	if tab := f.tabMgr.TabByID(id); tab != nil {
		tab.Nav.Mu.Lock()
		if tab.Nav.Cancel != nil {
			tab.Nav.Cancel()
			tab.Nav.Cancel = nil
		}
		tab.Nav.Mu.Unlock()
		if sess := tab.Session(); sess != nil {
			sess.Close()
		}
	}
	f.tabMgr.CloseTab(id)
	if f.tabMgr.Count() > 0 {
		tab := f.tabMgr.Active()
		f.ensureActiveTabVisible()
		f.syncTabToToolbar()
		if tab.Layer() != nil {
			f.sched.SetPlan(frame.FramePlan{
				Serial:     tab.Nav.Serial,
				Layers:     []*frame.Layer{tab.Layer()},
				Background: tab.BGColor(),
			})
		}
		f.sched.SetViewport(frame.Viewport{
			Offset: frame.Point{Y: tab.ScrollY()},
			Size:   f.config.devSize(),
		})
		f.publishAccessibility()
	}
}

// reopenTab restores the most recently closed tab and loads its URL.
func (f *framePath) reopenTab() {
	if f.tabMgr == nil {
		return
	}
	f.closeFind()
	cur := f.tabMgr.Active()
	if cur != nil {
		vp := f.sched.Viewport()
		cur.SetScrollY(vp.Offset.Y)
	}
	tab, ok := f.tabMgr.ReopenTab()
	if !ok || tab == nil {
		return
	}
	f.ensureActiveTabVisible()
	f.syncTabToToolbar()
	_, layer := blankLayer(f.config.devSize())
	tab.SetLayer(layer)
	tab.SetBGColor(frame.RGB(255, 255, 255))
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor(),
	})
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: tab.ScrollY()},
		Size:   f.config.devSize(),
	})
	f.publishAccessibility()
	if u := tab.URL(); u != "" {
		f.navigateTabNoHistory(u)
	}
}

// duplicateTab clones the active tab and navigates it to the same page.
func (f *framePath) duplicateTab() {
	if f.tabMgr == nil {
		return
	}
	f.closeFind()
	active := f.tabMgr.Active()
	if active == nil {
		return
	}
	vp := f.sched.Viewport()
	active.SetScrollY(vp.Offset.Y)

	tab, ok := f.tabMgr.DuplicateTab(active.ID())
	if !ok || tab == nil {
		return
	}
	f.ensureActiveTabVisible()
	f.syncTabToToolbar()
	_, layer := blankLayer(f.config.devSize())
	tab.SetLayer(layer)
	tab.SetBGColor(frame.RGB(255, 255, 255))
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor(),
	})
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: tab.ScrollY()},
		Size:   f.config.devSize(),
	})
	f.publishAccessibility()
	if u := tab.URL(); u != "" {
		f.navigateTabNoHistory(u)
	}
}

// toggleReaderMode switches the active tab between Reader Mode and original view.
func (f *framePath) toggleReaderMode() {
	if f.tabMgr == nil {
		return
	}
	active := f.tabMgr.Active()
	if active == nil {
		return
	}
	currURL := active.URL()
	if strings.HasPrefix(strings.ToLower(currURL), "goosie://reader") {
		if u, err := url.Parse(currURL); err == nil {
			if orig := u.Query().Get("url"); orig != "" {
				f.navigateTab(orig)
				return
			}
		}
		return
	}
	if currURL != "" && !strings.HasPrefix(strings.ToLower(currURL), "goosie://") {
		f.navigateTab("goosie://reader?url=" + url.QueryEscape(currURL))
	}
}
