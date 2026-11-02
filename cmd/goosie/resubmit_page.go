package main

import (
	"fmt"
	"html"
	"image"
	"net/url"
	"strings"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/history"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// resubmitPageTitle labels the tab while the POST confirmation is on screen.
const resubmitPageTitle = "Confirm Form Resubmission"

// tabDisplay is the document a resubmit prompt replaced. It is kept alive so
// Cancel can put the exact page back - same DOM, scroll offset and focus -
// instead of reloading it, which would lose state and could itself be a POST.
type tabDisplay struct {
	layer   *frame.Layer
	session *engine.Session
	url     string
	title   string
	bg      frame.Color
	favicon image.Image
	scrollY int32
	offset  frame.Point
}

// pendingResubmit is one tab's unanswered POST confirmation.
type pendingResubmit struct {
	request    toolbar.Request
	target     string
	delta      int
	base       int
	transition history.Transition
	// scroll is the offset the target history entry was left at, restored once
	// the confirmed resubmit lands.
	scroll   int32
	previous tabDisplay
}

// resubmitAction recognises the prompt's own action links. It returns false for
// a plain goosie://resubmit URL so direct visits still render a page.
func resubmitAction(rawURL string) (action string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !strings.EqualFold(u.Scheme, "goosie") || !strings.EqualFold(u.Host, "resubmit") {
		return "", false
	}
	switch strings.ToLower(u.Query().Get("action")) {
	case "confirm":
		return "confirm", true
	case "cancel":
		return "cancel", true
	}
	return "", false
}

func formatResubmitPageHTML(target string, req toolbar.Request) string {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = "POST"
	}
	shown := "this page"
	if target != "" {
		shown = "<code>" + html.EscapeString(target) + "</code>"
	}
	body := fmt.Sprintf(`<div class="card">
      <h1>Confirm Form Resubmission</h1>
      <p>Goosie submitted %s with %s. Resubmitting it may repeat an action such as a purchase, a message, or a deletion.</p>
      <p style="margin:20px 0;">
        <a id="resubmit-confirm" class="btn-action" href="goosie://resubmit?action=confirm">Resubmit</a>
        <a id="resubmit-cancel" class="btn-action secondary" href="goosie://resubmit?action=cancel">Cancel</a>
      </p>
      <p style="color:#5f6368;font-size:13px;">Cancel leaves the page you were reading unchanged.</p>
    </div>`, shown, html.EscapeString(method))
	return internalPageTemplate("", resubmitPageTitle, body)
}

func (f *framePath) buildResubmitPage(target string, req toolbar.Request) (*frame.Layer, *engine.Session, frame.Color) {
	if f.fonts == nil || f.contentWidth() <= 0 {
		return nil, nil, frame.RGB(255, 255, 255)
	}
	w := f.contentWidth()
	h := f.contentHeight()
	if w <= 0 {
		w = 800
	}
	if h <= 0 {
		h = 600
	}
	sess, err := engine.NewSession(formatResubmitPageHTML(target, req), nil, float32(w),
		engine.WithMetrics(f.fonts),
		engine.WithViewportH(float32(h)),
		engine.WithDarkMode(f.isDarkMode()),
		engine.WithHighContrast(f.isHighContrast()),
	)
	if err != nil {
		return nil, nil, frame.RGB(255, 255, 255)
	}
	scale := float32(f.config.dpr)
	if scale < 1 {
		scale = 1
	}
	list, err := sess.PaintChecked(scale)
	if err != nil {
		sess.Close()
		return nil, nil, frame.RGB(255, 255, 255)
	}
	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		sess.Close()
		return nil, nil, frame.RGB(255, 255, 255)
	}
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)
	return layer, sess, sess.BackgroundColor()
}

// promptResubmit replaces the active document with the confirmation. It reports
// whether the prompt is showing; false means the caller should perform an
// ordinary GET because there is nothing safe to keep behind a cancel.
func (f *framePath) promptResubmit(tab *tabs.Tab, target string, req toolbar.Request, delta, base int, transition history.Transition, scroll int32) bool {
	if tab == nil {
		return false
	}
	previous := tabDisplay{
		layer:   tab.Layer(),
		session: tab.Session(),
		url:     tab.URL(),
		title:   tab.Title(),
		bg:      tab.BGColor(),
		favicon: tab.Favicon(),
		scrollY: tab.ScrollY(),
	}
	if f.sched != nil {
		previous.offset = f.sched.Viewport().Offset
	}
	if previous.layer == nil || previous.session == nil {
		return false
	}

	f.resubmitMu.Lock()
	if f.resubmits == nil {
		f.resubmits = make(map[uint64]*pendingResubmit)
	}
	if _, exists := f.resubmits[tab.ID()]; exists {
		f.resubmitMu.Unlock()
		return true
	}
	f.resubmits[tab.ID()] = &pendingResubmit{
		request:    req,
		target:     target,
		delta:      delta,
		base:       base,
		transition: transition,
		scroll:     scroll,
		previous:   previous,
	}
	f.resubmitMu.Unlock()

	layer, sess, bg := f.buildResubmitPage(target, req)
	if layer == nil || sess == nil {
		// The prompt could not be built: keep the page and fall through to a
		// normal navigation on the caller's next branch.
		f.resubmitMu.Lock()
		delete(f.resubmits, tab.ID())
		f.resubmitMu.Unlock()
		return false
	}

	tab.Nav.Mu.Lock()
	if tab.Nav.Cancel != nil {
		tab.Nav.Cancel()
		tab.Nav.Cancel = nil
	}
	tab.Nav.Serial++
	serial := tab.Nav.Serial
	tab.Nav.Loading = false
	tab.Nav.Mu.Unlock()

	tab.SetLayer(layer)
	tab.SetSession(sess)
	tab.SetBGColor(bg)
	tab.SetURL(target)
	tab.SetTitle(resubmitPageTitle)
	tab.SetFavicon(nil)
	if f.tabMgr != nil && f.tabMgr.Active() == tab {
		if f.toolbar != nil {
			f.syncTabToToolbar()
		}
		f.sched.SetPlan(frame.FramePlan{
			Serial:     serial,
			Layers:     []*frame.Layer{layer},
			Background: bg,
		})
		f.sched.SetViewport(frame.Viewport{
			Offset: frame.Point{},
			Size:   f.devSize(),
		})
		f.publishAccessibility()
	}
	return true
}

// resolveResubmitAction applies the user's choice. The pending entry is taken
// first, so a double click cannot confirm or cancel twice.
func (f *framePath) resolveResubmitAction(tab *tabs.Tab, action string) {
	tabID := tab.ID()
	f.resubmitMu.Lock()
	if f.resubmits == nil {
		f.resubmitMu.Unlock()
		return
	}
	pending := f.resubmits[tabID]
	delete(f.resubmits, tabID)
	f.resubmitMu.Unlock()
	if pending == nil {
		return
	}
	switch action {
	case "cancel":
		f.restoreResubmit(tab, pending)
	case "confirm":
		if pending.previous.session != nil {
			pending.previous.session.Close()
		}
		f.navigateTabNoHistoryTraverseRequest(pending.target, pending.delta, pending.base, pending.transition, pending.scroll, pending.request)
	}
}

// restoreResubmit puts the replaced document back exactly as it was. The proof
// session is closed; the kept session never left the tab's ownership.
func (f *framePath) restoreResubmit(tab *tabs.Tab, pending *pendingResubmit) {
	if tab == nil || pending == nil {
		return
	}
	current := tab.Session()
	previous := pending.previous
	if current != nil && current != previous.session {
		current.Close()
	}

	tab.Nav.Mu.Lock()
	tab.Nav.Serial++
	serial := tab.Nav.Serial
	tab.Nav.Loading = false
	tab.Nav.Mu.Unlock()

	tab.SetLayer(previous.layer)
	tab.SetSession(previous.session)
	tab.SetBGColor(previous.bg)
	tab.SetURL(previous.url)
	tab.SetTitle(previous.title)
	tab.SetFavicon(previous.favicon)
	tab.SetScrollY(previous.scrollY)
	if f.tabMgr != nil && f.tabMgr.Active() == tab {
		if f.toolbar != nil {
			f.syncTabToToolbar()
		}
		f.sched.SetPlan(frame.FramePlan{
			Serial:     serial,
			Layers:     []*frame.Layer{previous.layer},
			Background: previous.bg,
		})
		f.sched.SetViewport(frame.Viewport{
			Offset: previous.offset,
			Size:   f.devSize(),
		})
		if previous.session != nil {
			previous.session.SyncViewportScroll(float64(previous.offset.X), float64(previous.offset.Y))
		}
		f.publishAccessibility()
	}
}

// dropResubmit abandons an unanswered prompt for a tab: the replaced document
// is no longer reachable, so it is closed with the pending decision.
func (f *framePath) dropResubmit(tabID uint64) {
	f.resubmitMu.Lock()
	pending := f.resubmits[tabID]
	delete(f.resubmits, tabID)
	f.resubmitMu.Unlock()
	if pending != nil && pending.previous.session != nil {
		pending.previous.session.Close()
	}
}
