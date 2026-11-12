package main

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/history"
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
	if tab.Session().BeginRangePointerDragAt(dx, dy) {
		f.setWindowIME(false)
		f.reflowAfterControlEdit(tab)
		f.repaintTab(tab, frame.Rect{})
		return true
	}

	// Check if click hit an activatable control (checkbox, radio, submit button, reset button)
	// The click is a trusted gesture, and marking it as one around the dispatch is
	// what lets a page's own click handler start playback: the engine refuses a
	// play() that arrives from a timer or a synthetic event, and only the host can
	// tell the difference.
	tab.Session().SetUserActivation(true)
	activated := tab.Session().ClickAt(dx, dy)
	tab.Session().SetUserActivation(false)
	if activated {
		if f.serviceFilePicker(tab) {
			return true
		}
		if sub, ok := tab.Session().PopSubmittedForm(); ok {
			f.handleFormSubmission(tab, sub)
			return true
		}
		f.reflowAfterControlChange(tab)
		return true
	}

	sess := tab.Session()
	if !sess.FocusControl(dx, dy) {
		return false
	}
	f.setWindowIME(sess.Focused() != nil)
	if !sess.CanDeferControlEditReflow() && (sess.HasPseudoClassRule("focus") || sess.HasPseudoClassRule("focus-within") || sess.HasPseudoClassRule("focus-visible") || sess.HasPseudoClassRule("active")) {
		if err := sess.RestyleAndReflow(float32(f.contentWidth())); err != nil {
			fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
		}
	}
	f.repaintTab(tab, frame.Rect{})
	return true
}

type contentDragGesture struct {
	source       *dom.Node
	tab          *tabs.Tab
	session      *engine.Session
	startX       int32
	startY       int32
	started      bool
	startAttempt bool
}

// pointerDrag routes a content-area pointer gesture into the engine's HTML
// drag-and-drop state machine. A gesture becomes a drag only after five CSS
// pixels of movement; an un-dragged gesture on the original element falls
// back to the normal click path on release.
func (f *framePath) pointerDrag(action surface.PointerAction, contentX, contentY int32) bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		f.contentDrag = nil
		return false
	}
	sess := tab.Session()
	if g := f.contentDrag; g != nil && action != surface.PointerPress {
		tab, sess = g.tab, g.session
		if tab == nil || sess == nil {
			f.contentDrag = nil
			return false
		}
	}
	switch action {
	case surface.PointerPress:
		f.contentDrag = nil
		dx, dy := f.docPoint(contentX, contentY)
		source := sess.DraggableElementAt(dx, dy)
		if source == nil {
			return false
		}
		f.contentDrag = &contentDragGesture{source: source, tab: tab, session: sess, startX: contentX, startY: contentY}
		return true
	case surface.PointerMotion:
		g := f.contentDrag
		if g == nil {
			return false
		}
		dx := float64(contentX - g.startX)
		dy := float64(contentY - g.startY)
		if !g.started && dx*dx+dy*dy < 25 {
			return true
		}
		if !g.started && !g.startAttempt {
			g.startAttempt = true
			startX, startY := f.docPoint(g.startX, g.startY)
			if sess.DraggableElementAt(startX, startY) == g.source {
				sess.SetUserActivation(true)
				g.started = sess.StartDrag(startX, startY)
				sess.SetUserActivation(false)
			}
			if g.started {
				f.repaintDrag(tab)
			}
		}
		if g.started {
			docX, docY := f.docPoint(contentX, contentY)
			sess.DragTo(docX, docY)
			f.repaintDrag(tab)
		}
		return true
	case surface.PointerRelease:
		g := f.contentDrag
		if g == nil {
			return false
		}
		f.contentDrag = nil
		if g.started {
			docX, docY := f.docPoint(contentX, contentY)
			sess.EndDrag(docX, docY)
			f.repaintDrag(tab)
			return true
		}
		releaseX, releaseY := f.docPoint(contentX, contentY)
		if sess.DraggableElementAt(releaseX, releaseY) == g.source {
			if href := f.hitTestLink(g.startX, g.startY); href != "" {
				f.handleLinkClick(href)
			} else {
				f.focusClick(g.startX, g.startY)
			}
		}
		return true
	}
	return false
}

// cancelPointerDrag implements Escape cancellation for an active drag. A
// merely pending pointer gesture is discarded without synthesizing a click.
func (f *framePath) cancelPointerDrag() bool {
	g := f.contentDrag
	if g == nil {
		return false
	}
	f.contentDrag = nil
	if !g.started {
		return true
	}
	if g.session == nil || g.tab == nil {
		return true
	}
	g.session.CancelDrag()
	f.repaintDrag(g.tab)
	return true
}

func (f *framePath) repaintDrag(tab *tabs.Tab) {
	if f.sched != nil && tab != nil {
		f.repaintTab(tab, frame.Rect{})
	}
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
	// No :hover rules means no paint change; repainting anyway flashed the
	// window over viewport-sized CSS-pixel damage. Pending DOM mutations are
	// painted by the next PumpJS turn, not by a partial repaint here.
	if !sess.HasPseudoClassRule("hover") || sess.CanDeferControlEditReflow() {
		return
	}
	if sess.IsHoverGeometryNeutral() {
		if err := sess.RestyleForHover(); err != nil {
			fmt.Fprintln(os.Stderr, "goosie: restyle:", err)
		}
	} else {
		if err := sess.RestyleAndReflow(float32(f.contentWidth())); err != nil {
			fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
		}
	}
	// Empty damage diffs display lists in device pixels; the hovered border
	// box is CSS pixels, misses descendant ink, and spans the document for body.
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
	// Pre-compute damage rect before reflow
	prevRect := tab.Session().FocusDamageRect()
	if err := tab.Session().RestyleAndReflow(float32(f.contentWidth())); err != nil {
		fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
	}
	// Union old and new damage rects for accurate repaint
	newRect := tab.Session().FocusDamageRect()
	damageRect := prevRect.Union(newRect)
	f.repaintTab(tab, damageRect)
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
// marked update synchronizes the active composition passage and reflows text
// controls as needed; a commit replaces it with final text. Both are consumed
// whether or not the session was waiting, because the platform input context
// is only ever on for the active tab's control.
func (f *framePath) imeEvent(ev surface.Event) bool {
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return false
	}
	switch ev.IME {
	case surface.IMEMarked:
		if tab.Session().SetMarked(ev.Text) {
			f.reflowAfterControlEdit(tab)
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
		dx, dy := f.docPoint(contentX, contentY)
		if tab.Session().MoveRangePointerDragAt(dx, dy) {
			f.reflowAfterControlEdit(tab)
			if newR, ok := tab.Session().SelectionBounds(); ok {
				f.repaintTab(tab, f.selectionDamage(tab, newR, true))
			} else {
				f.repaintTab(tab, frame.Rect{})
			}
			return true
		}
		if !f.selDrag {
			return false
		}
		oldR, oldOK := tab.Session().SelectionBounds()
		if !tab.Session().SelectTo(dx, dy) {
			return true
		}
		f.repaintTab(tab, f.selectionDamage(tab, oldR, oldOK))
		return true
	case surface.PointerRelease:
		dx, dy := f.docPoint(contentX, contentY)
		if tab.Session().EndRangePointerDragAt(dx, dy) {
			if newR, ok := tab.Session().SelectionBounds(); ok {
				f.repaintTab(tab, f.selectionDamage(tab, newR, true))
			} else {
				f.repaintTab(tab, frame.Rect{})
			}
			return true
		}
		wasDragging := f.selDrag
		f.selDrag = false
		return wasDragging
	}
	return false
}

// extendDocumentSelection moves the page selection head with a Shift+Arrow
// press, anchored where the selection started. Applies only when no editable
// control owns the key; the caller checks that.
func (f *framePath) extendDocumentSelection(tab *tabs.Tab, key rune) bool {
	var dir engine.SelectDirection
	switch key {
	case 0xF702:
		dir = engine.SelectLeft
	case 0xF703:
		dir = engine.SelectRight
	case 0xF700:
		dir = engine.SelectUp
	case 0xF701:
		dir = engine.SelectDown
	default:
		return false
	}
	sess := tab.Session()
	oldR, oldOK := sess.SelectionBounds()
	if !sess.ExtendSelectionByKey(dir) {
		return false
	}
	f.repaintTab(tab, f.selectionDamage(tab, oldR, oldOK))
	return true
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
	if sess.HasPseudoClassRule("placeholder-shown") || sess.HasValidityClassRules() {
		err = sess.RestyleAndReflow(float32(f.contentWidth()))
	} else if sess.FocusedInputValueDoesNotAffectLayout() {
		return
	} else {
		err = sess.Reflow(float32(f.contentWidth()))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
	}
}

// contentKeyAction routes a key phase to the content path. Presses keep the
// historical editing and scrolling behavior; releases only finish a held
// Space/Enter activation, so a release can never type or scroll.
func (f *framePath) contentKeyAction(key rune, mods surface.KeyMod, action surface.KeyAction) bool {
	if action == surface.KeyRelease {
		return f.releaseContentKey(key, mods)
	}
	return f.contentKey(key, mods)
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
			sess.SetUserActivation(true)
			sess.KeyDown(keyName, mods&surface.ModShift != 0, false, mods&surface.ModOption != 0, false)
			sess.SetUserActivation(false)
			// The activation itself waits for release, so the control stays
			// painted :active for as long as the key is physically held. A
			// cancelled keydown painted nothing and needs no style pass.
			if sess.HasHeldKey(keyName) && !sess.HeldKeyPrevented(keyName) {
				f.refreshActiveContent()
			}
			return true
		}
	}
	if !cmd && mods&surface.ModShift != 0 && !tab.Session().FocusedEditable() && tab.Session().FocusedSelect() == nil {
		// Shift+Arrow extends the page selection. It must run before the scroll
		// block, which would otherwise read the held Shift as a scroll gesture.
		if f.extendDocumentSelection(tab, key) {
			return true
		}
	}
	if !cmd && !tab.Session().FocusedEditable() && tab.Session().FocusedSelect() == nil {
		if f.scrollContentKey(tab, key, mods&surface.ModShift != 0) {
			return true
		}
	} else if !cmd && (key == keyPageUp || key == keyPageDown) {
		focused := tab.Session().Focused()
		if focused == nil || focused.Data != "input" || !strings.EqualFold(focused.GetAttribute("type"), "range") {
			// Page keys must never be treated as printable private-use characters
			// by text controls. Range controls consume them as larger value steps.
			return true
		}
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
			if err := sess.RestyleAndReflow(float32(f.contentWidth())); err != nil {
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
					clipHTML := ""
					if rich, ok := cb.(interface{ ReadHTML() string }); ok {
						clipHTML = rich.ReadHTML()
					}
					if clipText != "" || clipHTML != "" {
						if tab.Session().PasteData(clipText, clipHTML) {
							f.reflowAfterControlEdit(tab)
							f.repaintTab(tab, frame.Rect{})
							return true
						}
					}
				}
			}
		case 'a', 'A':
			if tab.Session().SelectAllInFocus() {
				f.repaintTab(tab, frame.Rect{})
				return true
			}
		case 'c', 'C':
			if cb := platform.NewClipboard(); cb != nil {
				if txt, html := tab.Session().CopySelectionData(); txt != "" || html != "" {
					if rich, ok := cb.(interface{ WriteRich(string, string) }); ok {
						rich.WriteRich(txt, html)
					} else {
						cb.Write(txt)
					}
					return true
				}
			}
		case 'x', 'X':
			if cb := platform.NewClipboard(); cb != nil {
				if txt, html, changed := tab.Session().CutSelectionData(); txt != "" || html != "" || changed {
					if txt != "" || html != "" {
						if rich, ok := cb.(interface{ WriteRich(string, string) }); ok {
							rich.WriteRich(txt, html)
						} else {
							cb.Write(txt)
						}
					}
					if changed {
						f.reflowAfterControlEdit(tab)
					}
					f.repaintTab(tab, frame.Rect{})
					return true
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
	case key == keyPageUp:
		action = engine.EditPageUp
	case key == keyPageDown:
		action = engine.EditPageDown
	case key == 0x7f || key == '\b':
		action = engine.EditBackspace
	case key == 0xF728:
		action = engine.EditDelete
	case key == '\r' || key == '\n':
		action = engine.EditEnter
	case key == 0x1b:
		action = engine.EditEscape
	case key >= 0x20:
		action = engine.EditRune
	default:
		return false
	}

	sess := tab.Session()
	if sess.FocusedSelect() != nil && sess.MoveSelect(action) {
		f.reflowAfterControlEdit(tab)
		f.repaintTab(tab, frame.Rect{})
		return true
	}

	changed := tab.Session().EditWithShift(action, key, mods&surface.ModShift != 0)
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

// releaseContentKey applies the key-up phase of a held Space or Enter. The
// press deferred activation to this point so the control could stay :active
// while the key was down; the release now runs the click and its browser
// defaults. A release for any other key, or for a key whose press the page
// cancelled, is not ours to consume.
func (f *framePath) releaseContentKey(key rune, mods surface.KeyMod) bool {
	if f.tabMgr == nil {
		return false
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return false
	}
	if mods&surface.ModCommand != 0 || mods&surface.ModControl != 0 {
		return false
	}
	enter := key == '\r' || key == '\n'
	space := key == ' '
	if !enter && !space {
		return false
	}
	keyName := "Enter"
	if space {
		keyName = " "
	}
	sess := tab.Session()
	if !sess.HasHeldKey(keyName) {
		return false
	}
	prevented := sess.HeldKeyPrevented(keyName)
	focused := sess.Focused()
	sess.SetUserActivation(true)
	activated := sess.KeyUp(keyName, mods&surface.ModShift != 0, false, mods&surface.ModOption != 0, false)
	sess.SetUserActivation(false)
	if !activated {
		if !prevented {
			f.refreshActiveContent()
		}
		return true
	}
	return f.finishKeyActivation(tab, focused, enter)
}

// finishKeyActivation applies the browser defaults that follow a keyboard
// activation: an anchor follows its href, a queued file picker opens, a
// submitted form is handed to the navigation path, and anything else restyles
// for the states the click changed.
func (f *framePath) finishKeyActivation(tab *tabs.Tab, focused *dom.Node, enter bool) bool {
	sess := tab.Session()
	if sess == nil {
		return false
	}
	if enter && focused != nil && focused.Data == "a" {
		if href := focused.GetAttribute("href"); href != "" {
			f.handleLinkClick(href)
			return true
		}
	}
	if f.serviceFilePicker(tab) {
		return true
	}
	if sub, ok := sess.PopSubmittedForm(); ok {
		f.handleFormSubmission(tab, sub)
		return true
	}
	f.reflowAfterControlChange(tab)
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
		// Sync the JS runtime so window.scrollY reflects the painted position.
		if sess := tab.Session(); sess != nil {
			sess.SyncViewportScroll(float64(vp.Offset.X), float64(y))
		}
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
	// target="_blank" submits into a new browsing context. _self, _top,
	// _parent and the empty target all resolve to this tab at the top level.
	// A named frame target has no host-side transport yet, so it stays here
	// rather than silently opening the wrong context.
	if strings.EqualFold(strings.TrimSpace(sub.Target), "_blank") {
		f.newTab()
	}
	if method == "" || method == "GET" {
		if sub.Encoded != "" {
			if strings.Contains(targetURL, "?") {
				targetURL = targetURL + "&" + sub.Encoded
			} else {
				targetURL = targetURL + "?" + sub.Encoded
			}
		}
		f.navigateTabTransition(targetURL, history.TransitionLink)
	} else if method == "POST" {
		contentType := sub.Enctype
		if contentType == "" {
			contentType = "application/x-www-form-urlencoded"
		}
		f.navigateTabRequest(targetURL, history.TransitionLink, navRequest{
			Method:      "POST",
			ContentType: contentType,
			Body:        sub.Encoded,
		})
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
	winW := int32(f.contentWidth()) * cw.sc()
	winH := int32(f.contentHeight()) * cw.sc()
	cw.extentMu.Lock()
	chrome, out := cw.chromeBitmap, cw.outBitmap
	cw.extentMu.Unlock()
	if chrome != nil {
		winW = int32(chrome.W)
	}
	if out != nil {
		winH = int32(out.H)
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
						if err := tab.Session().Reflow(float32(f.contentWidth())); err != nil {
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
	case "bookmarks":
		f.navigateTab("goosie://bookmarks")
	case "history":
		f.navigateTab("goosie://history")
	case "downloads":
		f.navigateTab("goosie://downloads")
	case "settings":
		f.navigateTab("goosie://settings")
	case "print_pdf":
		f.printActiveTabToPDF()
	case "new_window":
		if f.newWindow != nil {
			f.newWindow()
		}
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
	f.handleResize(f.contentWidth(), f.contentHeight())
}

func (f *framePath) handleLinkClick(href string) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	f.navigateTabTransition(resolveHref(tab.URL(), href), history.TransitionLink)
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
	f.findMu.Lock()
	defer f.findMu.Unlock()
	if f.toolbar == nil {
		return
	}
	f.findQuery = query
	f.findMatches = nil
	f.findIdx = 0
	f.toolbar.ResetFindCounts()
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}
	sess := tab.Session()
	f.findSignature = findSignatureOf(sess)
	if strings.TrimSpace(query) == "" {
		sess.ClearFind()
		f.repaintTab(tab, frame.Rect{})
		return
	}
	f.findMatches = sess.Find(query)
	f.toolbar.SetFindResult(len(f.findMatches), 0)
	if len(f.findMatches) == 0 {
		sess.SetFindActive(-1)
	} else {
		sess.SetFindActive(0)
		f.scrollToMatchLocked(0)
	}
	f.repaintTab(tab, frame.Rect{})
}

// findSignature is the pipeline work a find pass saw. A change means the
// document was restyled or relaid out since the matches were computed.
type findSignature struct {
	styleResolves uint64
	layoutBuilds  uint64
}

func findSignatureOf(sess *engine.Session) findSignature {
	wc := sess.WorkCounts()
	return findSignature{styleResolves: wc.StyleResolves, layoutBuilds: wc.LayoutBuilds}
}

// refreshFind keeps an open find in step with the document. A page script that
// adds, removes, or reflows text bumps the style/layout counters, which is the
// cue to recompute matches without waiting for the next keystroke. The active
// index is preserved when the new set still has it, so a highlight does not
// jump to a different hit just because an unrelated node appeared.
func (f *framePath) refreshFind() {
	f.findMu.Lock()
	defer f.findMu.Unlock()
	if f.toolbar == nil || strings.TrimSpace(f.findQuery) == "" {
		return
	}
	active, _, idx := f.toolbar.FindCounts()
	if !active {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}
	sess := tab.Session()
	sig := findSignatureOf(sess)
	if sig == f.findSignature {
		return
	}
	f.findSignature = sig
	matches := sess.Find(f.findQuery)
	f.findMatches = matches
	if len(matches) == 0 {
		f.findIdx = 0
		f.toolbar.SetFindResult(0, 0)
		sess.SetFindActive(-1)
		f.repaintTab(tab, frame.Rect{})
		return
	}
	if idx < 0 {
		idx = 0
	} else if idx >= len(matches) {
		idx = len(matches) - 1
	}
	f.findIdx = idx
	f.toolbar.SetFindResult(len(matches), idx)
	sess.SetFindActive(idx)
	f.repaintTab(tab, frame.Rect{})
}

// findStep moves to the next (or previous) match, wrapping around.
func (f *framePath) findStep(backward bool) {
	f.findMu.Lock()
	defer f.findMu.Unlock()
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
	f.scrollToMatchLocked(f.findIdx)
	if tab := f.tabMgr.Active(); tab != nil && tab.Session() != nil {
		tab.Session().SetFindActive(f.findIdx)
		f.repaintTab(tab, frame.Rect{})
	}
}

// scrollToMatchLocked brings the match's line into view, clamped to the
// document so the last hits cannot scroll past the end, and records the offset
// on the tab and the JS runtime the same way a wheel or key scroll does. The
// caller holds findMu.
func (f *framePath) scrollToMatchLocked(idx int) {
	if idx < 0 || idx >= len(f.findMatches) {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || f.sched == nil {
		return
	}
	m := f.findMatches[idx]
	effectiveDPR := float32(f.config.dpr) * float32(f.zoom)
	size := f.devSize()
	y := int32(m.Y0*effectiveDPR) - size.H/4
	maxY := int32(0)
	if layer := tab.Layer(); layer != nil {
		maxY = layer.Bounds.Y1 - size.H
	}
	if maxY < 0 {
		maxY = 0
	}
	if y < 0 {
		y = 0
	} else if y > maxY {
		y = maxY
	}
	vp := f.sched.Viewport()
	if vp.Offset.Y == y && vp.Size == size {
		return
	}
	vp.Offset.Y = y
	vp.Size = size
	f.sched.SetViewport(vp)
	tab.SetScrollY(y)
	if sess := tab.Session(); sess != nil {
		sess.SyncViewportScroll(float64(vp.Offset.X), float64(y))
	}
	f.publishAccessibility()
}

// closeFind drops the match state; navigation and tab switches end find mode.
func (f *framePath) closeFind() {
	f.findMu.Lock()
	defer f.findMu.Unlock()
	f.findMatches = nil
	f.findIdx = 0
	f.findQuery = ""
	f.findSignature = findSignature{}
	if f.tabMgr != nil {
		if tab := f.tabMgr.Active(); tab != nil && tab.Session() != nil {
			tab.Session().ClearFind()
		}
	}
	if f.toolbar == nil {
		return
	}
	f.toolbar.ResetFindCounts()
	if active, _, _ := f.toolbar.FindCounts(); active {
		f.toolbar.CloseFind()
	}
}

// clearFindHighlights repaints the active tab after CloseFind drops the
// session's match marks, so the yellow and orange boxes disappear at once.
func (f *framePath) clearFindHighlights() {
	if f.tabMgr == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}
	f.repaintTab(tab, frame.Rect{})
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
		Size:   f.devSize(),
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
	_, layer := blankLayer(f.devSize())
	tab.SetLayer(layer)
	tab.SetBGColor(frame.RGB(255, 255, 255))
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor(),
	})
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: 0},
		Size:   f.devSize(),
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
	w := int32(f.contentWidth())
	idx := f.tabMgr.ActiveIndex()
	f.tabMgr.SetTabScroll(tabs.EnsureTabVisible(idx, n, w, f.tabMgr.TabScroll()), n, w)
}

// releaseTabState stops a tab's in-flight navigation and closes the document
// behind it. Batch closures must do this for every tab they remove, exactly as
// closing one tab does, or the loads keep running against tabs nothing paints.
func releaseTabState(tab *tabs.Tab) {
	if tab == nil {
		return
	}
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

// closeTab closes a tab by ID and switches to the remaining active tab.
func (f *framePath) closeTab(id uint64) {
	if f.tabMgr == nil {
		return
	}
	f.closeFind()
	if tab := f.tabMgr.TabByID(id); tab != nil {
		f.dropResubmit(tab.ID())
		releaseTabState(tab)
	}
	f.tabMgr.CloseTab(id)
	f.showActiveTabAfterBatch()
}

// closeOtherTabs closes every unpinned tab except keepID.
func (f *framePath) closeOtherTabs(keepID uint64) {
	if f.tabMgr == nil {
		return
	}
	f.closeFind()
	for _, tab := range f.tabMgr.Tabs() {
		if tab.ID() == keepID || tab.Pinned() {
			continue
		}
		f.dropResubmit(tab.ID())
		releaseTabState(tab)
	}
	f.tabMgr.CloseOtherTabs(keepID)
	f.showActiveTabAfterBatch()
}

// closeTabsToRight closes every unpinned tab positioned after fromID.
func (f *framePath) closeTabsToRight(fromID uint64) {
	if f.tabMgr == nil {
		return
	}
	f.closeFind()
	tabList := f.tabMgr.Tabs()
	fromIdx := -1
	for i, tab := range tabList {
		if tab.ID() == fromID {
			fromIdx = i
			break
		}
	}
	if fromIdx < 0 {
		return
	}
	for _, tab := range tabList[fromIdx+1:] {
		if tab.Pinned() {
			continue
		}
		f.dropResubmit(tab.ID())
		releaseTabState(tab)
	}
	f.tabMgr.CloseTabsToRight(fromID)
	f.showActiveTabAfterBatch()
}

// toggleTabPin pins or unpins a tab and keeps the reordered strip's active tab
// on screen.
func (f *framePath) toggleTabPin(id uint64) {
	if f.tabMgr == nil {
		return
	}
	f.tabMgr.TogglePin(id)
	f.ensureActiveTabVisible()
	f.syncTabToToolbar()
}

// showActiveTabAfterBatch puts the surviving active tab back on screen after a
// closure removed one or more tabs.
func (f *framePath) showActiveTabAfterBatch() {
	if f.tabMgr == nil || f.tabMgr.Count() == 0 {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
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
		Size:   f.devSize(),
	})
	f.publishAccessibility()
}

// handleTabContextMenuAction executes an action chosen from a tab's menu. The
// tab id travels in Data so the action lands on the tab that was right-clicked.
func (f *framePath) handleTabContextMenuAction(action, data string) {
	switch action {
	case "tab_new":
		f.newTab()
		return
	case "tab_reopen":
		f.reopenTab()
		return
	}
	id, err := strconv.ParseUint(data, 10, 64)
	if err != nil || f.tabMgr == nil {
		return
	}
	switch action {
	case "tab_reload":
		if f.tabMgr.TabByID(id) != nil {
			f.switchTab(id)
			f.reloadTab()
		}
	case "tab_duplicate":
		if f.tabMgr.TabByID(id) != nil {
			f.switchTab(id)
			f.duplicateTab()
		}
	case "tab_toggle_pin":
		f.toggleTabPin(id)
	case "tab_close":
		f.closeTab(id)
	case "tab_close_others":
		f.closeOtherTabs(id)
	case "tab_close_right":
		f.closeTabsToRight(id)
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
	_, layer := blankLayer(f.devSize())
	tab.SetLayer(layer)
	tab.SetBGColor(frame.RGB(255, 255, 255))
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor(),
	})
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: tab.ScrollY()},
		Size:   f.devSize(),
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
	_, layer := blankLayer(f.devSize())
	tab.SetLayer(layer)
	tab.SetBGColor(frame.RGB(255, 255, 255))
	f.sched.SetPlan(frame.FramePlan{
		Serial:     tab.Nav.Serial,
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor(),
	})
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: tab.ScrollY()},
		Size:   f.devSize(),
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
