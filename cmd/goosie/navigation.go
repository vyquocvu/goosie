package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/history"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/manifest"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/session"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// navResult is the outcome of a per-tab navigation, delivered on navResults.
type navResult struct {
	tabID          uint64
	serial         uint64
	layer          *frame.Layer
	bgColor        frame.Color
	session        *engine.Session
	err            error
	url            string
	downloadPath   string
	downloadPaused bool
	noHistory      bool
	// traverse commits a back/forward step on success: the delta (-1/+1) and
	// the cursor index the peek saw. A failed traversal leaves the cursor
	// where it was instead of stranding it on a page that never loaded.
	traverse     int
	traverseBase int
	// traverseScroll is the offset the target entry was left at. A committed
	// traversal restores it so Back returns the reader to where they were.
	traverseScroll int32
	// transition labels the visit this navigation records in global history.
	transition history.Transition
	// request is what the load sent, and requestedURL is the address it was
	// requested for before any redirect. Together they decide whether the
	// history entry replays a POST or a GET.
	request      navRequest
	requestedURL string
}

// navRequest is the method and payload of one navigation. It shares its shape
// with toolbar.Request so a history entry can be replayed and recorded without
// a second representation. The zero value is a GET; form submission with
// method="POST" carries the encoded controls.
type navRequest = toolbar.Request

// downloadDone reports a completed download instead of a rendered document;
// the navigation goroutine converts it into navResult.downloadPath.
type downloadDone struct{ path string }

func (e downloadDone) Error() string { return "downloaded to " + e.path }

// downloadPaused reports a download stopped by the user with its partial file
// kept on disk. The navigation ends quietly; the download stays resumable from
// the downloads page.
type downloadPaused struct{ id string }

func (downloadPaused) Error() string { return "download paused" }

// imgResult reports that a session's deferred images have landed; the drain
// goroutine reflows and repaints the owning tab if it is still showing the
// same navigation.
type imgResult struct {
	tabID   uint64
	serial  uint64
	session *engine.Session
}

// syncTabToToolbar copies the active tab's state into the toolbar for display.
func (f *framePath) syncTabToToolbar() {
	if f.toolbar == nil || f.tabMgr == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	tab.Nav.Mu.Lock()
	loading := tab.Nav.Loading
	tab.Nav.Mu.Unlock()
	f.toolbar.SyncFromTab(tab.URL(), loading, tab.Error(), tab.History())
	f.toolbar.SetIncognito(tab.Incognito())
	f.publishAccessibility()
}

// clientForTab returns the ephemeral private client for incognito tabs or the default client.
func (f *framePath) clientForTab(tab *tabs.Tab) net.HTTP {
	if tab != nil && tab.Incognito() && f.privateClient != nil {
		return f.privateClient
	}
	return f.client
}

// newIncognitoTab opens a new private incognito tab.
func (f *framePath) newIncognitoTab() *tabs.Tab {
	tab := f.tabMgr.NewIncognitoTab()
	dev := frame.Size{W: int32(f.contentWidth()), H: int32(f.contentHeight())}
	_, layer := blankLayer(dev)
	tab.SetLayer(layer)
	tab.SetBGColor(frame.RGB(40, 44, 52))
	f.switchTab(tab.ID())
	return tab
}

// navContext roots one navigation load: the run scope when it exists so
// shutdown cancels in-flight loads, Background for bare test harnesses.
func (f *framePath) navContext() context.Context {
	if f.runCtx != nil {
		return f.runCtx
	}
	return context.Background()
}

// navigateTab loads a URL on the active tab. Omnibox submissions and
// programmatic navigations default to the typed transition.
func (f *framePath) navigateTab(rawURL string) {
	f.navigateTabTransition(rawURL, history.TransitionTyped)
}

// navigateTabTransition loads a URL and labels the visit it records.
func (f *framePath) navigateTabTransition(rawURL string, transition history.Transition) {
	f.navigateTabRequest(rawURL, transition, navRequest{})
}

// navigateTabRequest loads rawURL with an explicit request. Only a form
// submission with method="POST" fills req; every other caller reaches the
// server through navigateTabTransition's GET.
func (f *framePath) navigateTabRequest(rawURL string, transition history.Transition, req navRequest) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	// The resubmit prompt's own links are decisions, not destinations: handle
	// them before starting a load so they never enter the history trail.
	if action, ok := resubmitAction(rawURL); ok {
		f.resolveResubmitAction(tab, action)
		return
	}
	// Remember where this page was left before the load replaces it, so a Back
	// to this history entry can return to the same offset.
	if f.sched != nil {
		tab.History().SetCurrentScroll(f.sched.Viewport().Offset.Y)
	}
	f.dropResubmit(tab.ID())
	f.closeFind()
	// A navigation start invalidates the dropdown even when it did not come
	// from an omnibox submit (link click, internal-page action).
	if f.toolbar != nil {
		f.toolbar.CloseSuggestions()
	}
	u := normalizeAddress(rawURL, f.currentPreferences().SearchURLTemplate)

	tab.Nav.Mu.Lock()
	if tab.Nav.Cancel != nil {
		tab.Nav.Cancel()
	}
	ctx, cancel := context.WithCancel(f.navContext())
	if !tab.ShieldsEnabled() {
		ctx = net.WithShieldsDisabled(ctx, true)
	}
	tab.Nav.Cancel = cancel

	tab.Nav.Serial++
	serial := tab.Nav.Serial
	tab.Nav.Loading = true
	tab.Nav.Mu.Unlock()

	tab.SetLoading(true)
	tab.SetError("")
	if f.toolbar != nil {
		f.toolbar.SetLoading(true)
		f.toolbar.SetError("")
	}
	f.publishAccessibility()

	f.navWg.Add(1)
	client := f.clientForTab(tab)
	go func() {
		defer f.navWg.Done()
		layer, _, bgColor, sess, finalURL, err := f.loadDocumentCtx(ctx, client, u, f.contentWidth(), f.contentHeight(), float32(f.config.dpr), f.downloadDirectory(), true, true, req)
		res := navResult{tabID: tab.ID(), serial: serial, url: u, transition: transition, request: req, requestedURL: u}
		if finalURL != "" {
			// A redirect lands the final URL: the address bar and the
			// history record what displayed, not what was typed.
			res.url = finalURL
		}
		var dd downloadDone
		var dp downloadPaused
		if errors.As(err, &dd) {
			res.downloadPath = dd.path
		} else if errors.As(err, &dp) {
			res.downloadPaused = true
		} else {
			res.err = err
		}
		res.layer, res.bgColor, res.session = layer, bgColor, sess
		f.navResults <- res
	}()
}

// loadDocumentCtx loads an internal page (goosie://) or a network/file resource.
// reqs carries an explicit request for a form submission; omitted means GET.
func (f *framePath) loadDocumentCtx(ctx context.Context, client net.HTTP, u string, viewportW, viewportH int, scale float32, downloadDir string, deferImages, deferFonts bool, reqs ...navRequest) (*frame.Layer, paint.SceneSpec, frame.Color, *engine.Session, string, error) {
	policy := scriptPolicy{timeout: f.scriptTimeout}
	if strings.HasPrefix(strings.ToLower(u), "goosie://") {
		// The unresponsive prompt's actions reload the page under a chosen
		// script budget; every other goosie:// URL is an internal page.
		target, promptPolicy, ok := unresponsiveScriptPolicy(u)
		if !ok {
			return f.renderInternalPage(ctx, u, viewportW, viewportH, scale)
		}
		u, policy = target, promptPolicy
	}
	if client == nil {
		client = f.client
	}
	storage := f.storage
	if client == f.privateClient {
		storage = f.incognitoStorage
	}
	broadcastHub := f.broadcastHubFor(client)
	tabID := ""
	if f.tabMgr != nil {
		if active := f.tabMgr.Active(); active != nil {
			tabID = strconv.FormatUint(active.ID(), 10)
		}
	}
	appMode := appearanceMode{dark: f.isDarkMode(), highContrast: f.isHighContrast()}
	extra := []any{f.downloads, storage, tabID, appMode, f.userscripts, f.perms, broadcastHub, f.mediaBackend, policy}
	if clipboard := platform.NewClipboard(); clipboard != nil {
		extra = append(extra, clipboard)
	}
	if f.currentPreferences().AskDownloadLocation {
		extra = append(extra, f.window)
	}
	if len(reqs) > 0 {
		extra = append(extra, reqs[0])
	}
	return loadURLCtx(ctx, client, f.fonts, u, viewportW, viewportH, scale, downloadDir, deferImages, deferFonts, extra...)
}

func (f *framePath) broadcastHubFor(client net.HTTP) *js.BroadcastHub {
	if client == f.privateClient {
		return f.incognitoBroadcastHub
	}
	return f.broadcastHub
}

// drainNavResults reads navigation results from the channel and applies them.
func (f *framePath) drainNavResults() {
	for result := range f.navResults {
		f.applyNavResult(result)
	}
}

// applyNavResult applies a completed navigation result to its tab.
func (f *framePath) applyNavResult(result navResult) {
	tab := f.tabMgr.TabByID(result.tabID)
	if tab == nil {
		// The tab was closed while its load was in flight. The session this result
		// carries was built for it and nothing else will ever reach it, so it has
		// to be released here: a session now owns media, and an unreleased one is a
		// decoder, a temporary file and possibly audio with no tab to stop it from.
		if result.session != nil {
			result.session.Close()
		}
		return
	}

	tab.Nav.Mu.Lock()
	if tab.Nav.Serial != result.serial {
		tab.Nav.Mu.Unlock()
		// Superseded by a newer navigation to the same tab: same reasoning as a
		// vanished tab. The newer load owns its own session.
		if result.session != nil {
			result.session.Close()
		}
		return
	}
	tab.Nav.Loading = false
	tab.Nav.Mu.Unlock()

	tab.SetLoading(false)
	if result.downloadPath != "" {
		if f.toolbar != nil && f.tabMgr.Active() == tab {
			f.toolbar.SetLoading(false)
		}
		fmt.Fprintf(os.Stderr, "goosie: saved %s\n", result.downloadPath)
		if f.toolbar != nil && f.downloads != nil {
			f.toolbar.SetDownloadStats(f.downloads.ActiveCount(), f.downloads.TotalCount(), f.downloads.OverallProgress())
		}
		return
	}
	if result.downloadPaused {
		if f.toolbar != nil && f.tabMgr.Active() == tab {
			f.toolbar.SetLoading(false)
		}
		if f.toolbar != nil && f.downloads != nil {
			f.toolbar.SetDownloadStats(f.downloads.ActiveCount(), f.downloads.TotalCount(), f.downloads.OverallProgress())
		}
		return
	}
	if result.err != nil {
		if errors.Is(result.err, errDownloadSaveCanceled) {
			tab.SetError("")
			if f.toolbar != nil && f.tabMgr.Active() == tab {
				f.toolbar.SetLoading(false)
				f.toolbar.SetError("")
			}
			return
		}
		tab.SetError(result.err.Error())
		if f.toolbar != nil && f.tabMgr.Active() == tab {
			f.toolbar.SetLoading(false)
			f.toolbar.SetError(result.err.Error())
		}
		fmt.Fprintln(os.Stderr, result.err)

		// Build in-viewport error page so the tab is not left blank or showing obsolete content.
		if !errors.Is(result.err, context.Canceled) {
			errLayer, errSess, errBG := f.buildErrorPage(result.url, result.err)
			if errLayer != nil {
				oldSession := tab.Session()
				tab.SetLayer(errLayer)
				tab.SetSession(errSess)
				tab.SetBGColor(errBG)
				tab.SetURL(result.url)
				tab.SetTitle("Problem loading page")
				if oldSession != nil && oldSession != errSess {
					oldSession.Close()
				}
				if f.tabMgr.Active() == tab {
					f.syncTabToToolbar()
					f.sched.SetPlan(frame.FramePlan{
						Serial:     result.serial,
						Layers:     []*frame.Layer{errLayer},
						Background: tab.BGColor(),
					})
					f.sched.SetViewport(frame.Viewport{
						Offset: frame.Point{},
						Size:   f.devSize(),
					})
					f.publishAccessibility()
				}
			}
		}
		return
	}

	// A script that ran past its deadline is not a page the user chose to
	// display. Show the unresponsive-script prompt instead of committing it;
	// its actions decide how the page is retried.
	if result.session != nil && result.session.HasScriptTimeout() {
		if f.installUnresponsivePrompt(tab, result.url, result.session.ScriptTimeoutCount(), result.serial, result.session) {
			return
		}
	}

	// Inspect the result document before publishing it to the active tab. Once
	// visible, pointer and script events may mutate its DOM on the chrome thread.
	manifestURL := ""
	faviconURL := ""
	docTitle := documentTitle(result.session)
	if result.session != nil && result.session.Doc != nil {
		faviconURL = faviconURLFor(&result.session.Doc.Node, result.url)
		if f.client != nil {
			manifestURL = findManifestLink(&result.session.Doc.Node, result.url)
		}
	}

	oldSession := tab.Session()
	tab.SetLayer(result.layer)
	tab.SetSession(result.session)
	tab.SetBGColor(result.bgColor)
	// A new document owns the icon column: the previous site's icon must not
	// label it while the new one is still in flight.
	tab.SetFavicon(nil)
	tab.SetURL(result.url)
	tabTitle := result.url
	if strings.HasPrefix(strings.ToLower(result.url), "goosie://") {
		title, _ := f.buildInternalPageHTML(result.url)
		if title != "" {
			tabTitle = title
		}
	} else if docTitle != "" {
		// A loaded document with a <title> labels its tab with the title;
		// the address bar still owns the URL.
		tabTitle = docTitle
	}
	tab.SetTitle(tabTitle)
	if oldSession != nil && oldSession != result.session {
		// The replaced document's realm tears down with it.
		oldSession.Close()
	}
	if !result.noHistory {
		// A redirect means the response came from another address, and the
		// client may have turned a 302 into a GET; recording a POST there would
		// offer a resubmit prompt for a page that was never stored that way.
		req := toolbar.Request{}
		if result.request.IsPost() && result.url == result.requestedURL {
			req = result.request
		}
		tab.History().PushRequest(result.url, req)
	}
	traversed := false
	if result.traverse != 0 {
		// The peeked back/forward step commits only now that its page
		// displayed; a stale commit (another navigation landed first) drops.
		// A redirect rewrites the entry it landed on with the final URL.
		if tab.History().Step(result.traverse, result.traverseBase) {
			traversed = true
			tab.History().ReplaceCurrent(result.url)
			if result.request.IsPost() && result.url != result.requestedURL {
				tab.History().ClearCurrentRequest()
			}
		}
	}
	if !tab.Incognito() && f.history != nil && !strings.HasPrefix(strings.ToLower(result.url), "goosie://") {
		transition := result.transition
		if transition == "" {
			transition = history.TransitionOther
		}
		f.history.RecordVisit(result.url, tabTitle, transition)
	}

	if f.tabMgr.Active() == tab {
		f.syncTabToToolbar()
		f.sched.SetPlan(frame.FramePlan{
			Serial:     result.serial,
			Layers:     []*frame.Layer{result.layer},
			Background: tab.BGColor(),
		})
		if !result.noHistory {
			// A fresh navigation opens at the top: without the reset the
			// new page inherited whatever offset the old one scrolled to.
			// Reloads keep the current offset; traversals restore the target
			// entry's own below.
			tab.SetScrollY(0)
			f.sched.SetViewport(frame.Viewport{
				Offset: frame.Point{},
				Size:   f.devSize(),
			})
			// A session restore's own load keeps its saved offset: the reset
			// above is for user-initiated navigations.
			f.restoreMarkedScroll(tab)

			// If the URL has a fragment (#anchor), scroll to the element
			if href := result.url; strings.Contains(href, "#") {
				f.scrollToAnchor(tab, href)
			}
		} else if traversed {
			f.restoreTraversalScroll(tab, result)
		}
		f.publishAccessibility()
	} else if traversed {
		// A traversal that lands on a background tab still records where it
		// opened; switching back to it must not show the departing page's
		// offset. The viewport itself stays with the active tab.
		f.restoreTraversalScroll(tab, result)
	}

	// The first frame is out with the document and CSS only; fonts and images
	// arrive on their own goroutines and repaint when they land.
	if sess := result.session; sess != nil {
		// Bridge JS viewport scrolls to the raster scheduler so window.scrollTo()
		// moves visible pixels, not just the JS runtime's internal offset.
		sess.SetViewportScrollCallback(func(x, y float64) {
			vp := f.sched.Viewport()
			vp.Offset = frame.Point{X: int32(x), Y: int32(y)}
			f.sched.SetViewport(vp)
		})
		tabID, serial := result.tabID, result.serial
		if sess.DeferredFontsPending() > 0 {
			go sess.LoadDeferredFonts(func(loaded int) {
				if loaded == 0 {
					return
				}
				sess.RefreshAfterFonts()
				f.imgResults <- imgResult{tabID: tabID, serial: serial, session: sess}
			})
		}
		if sess.DeferredImagesPending() > 0 {
			go sess.LoadDeferredImages(func(loaded int) {
				if loaded == 0 {
					return
				}
				f.imgResults <- imgResult{tabID: tabID, serial: serial, session: sess}
			})
		}
		if manifestURL != "" {
			go func(mURL string, t *tabs.Tab) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				resp, err := f.client.Get(ctx, mURL)
				if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 400 {
					return
				}
				m, err := manifest.Parse(resp.Body)
				if err != nil || m == nil {
					return
				}
				t.SetManifest(m)
				if t.IsStandaloneApp() && m.EffectiveName() != "" {
					t.SetTitle(m.EffectiveName())
				}
			}(manifestURL, tab)
		}
		if faviconURL != "" {
			go f.loadFavicon(tab, result.serial, faviconURL)
		}
	}
	f.saveActiveSession()
}

// restoreTraversalScroll returns a committed Back/Forward landing to the
// offset that history entry was left at. The offset is clamped to the displayed
// document: a page that shrank since is shown from its top, not parked past its
// end, because the scheduler would silently clamp the pixels but leave the
// recorded offset pointing at nothing.
func (f *framePath) restoreTraversalScroll(tab *tabs.Tab, result navResult) {
	y := result.traverseScroll
	if result.layer != nil {
		if maxY := result.layer.Bounds.Y1 - f.devSize().H; y > maxY {
			y = maxY
		}
	}
	if y < 0 {
		y = 0
	}
	tab.SetScrollY(y)
	if f.sched == nil || f.tabMgr.Active() != tab {
		return
	}
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: y},
		Size:   f.devSize(),
	})
	if sess := result.session; sess != nil {
		// A restored page's script runtime must see the same offset the reader
		// does, or window.scrollY disagrees with the pixels.
		sess.SyncViewportScroll(0, float64(y))
	}
}

// drainImageResults reads deferred-image completions and applies them.
func (f *framePath) drainImageResults() {
	for res := range f.imgResults {
		f.applyImageResult(res)
	}
}

// applyImageResult reflows and repaints the tab whose deferred images just
// landed, unless a newer navigation (or a reload that rebuilt the session)
// has superseded it.
func (f *framePath) applyImageResult(res imgResult) {
	tab := f.tabMgr.TabByID(res.tabID)
	if tab == nil || tab.Session() != res.session {
		return
	}
	tab.Nav.Mu.Lock()
	cur := tab.Nav.Serial
	tab.Nav.Mu.Unlock()
	if cur != res.serial {
		return
	}
	if err := tab.Session().Reflow(float32(f.contentWidth())); err != nil {
		fmt.Fprintln(os.Stderr, "goosie: reflow after images:", err)
		return
	}
	f.repaintTab(tab, frame.Rect{})
}

// toggleBookmark stars or unstars the active tab's page and persists immediately.
func (f *framePath) toggleBookmark() {
	tab := f.tabMgr.Active()
	if tab == nil || tab.URL() == "" || f.bookmarks == nil {
		return
	}
	f.bookmarks.Toggle(tab.URL(), tab.Title())
	if err := f.bookmarks.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "goosie: save bookmarks: %v\n", err)
	}
}

// traverseTab handles back/forward on the active tab. The cursor moves only
// when the traversal's load succeeds (committed in applyNavResult): peeking
// here keeps a failed load from stranding the cursor on a page that never
// displayed.
func (f *framePath) traverseTab(delta int) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	// Remember where this page is being left, so a Forward back onto it lands
	// at the same offset instead of the top.
	if f.sched != nil {
		tab.History().SetCurrentScroll(f.sched.Viewport().Offset.Y)
	}
	var url string
	var req toolbar.Request
	var scroll int32
	var base int
	var ok bool
	if delta < 0 {
		url, req, scroll, base, ok = tab.History().PeekBackState()
	} else if delta > 0 {
		url, req, scroll, base, ok = tab.History().PeekForwardState()
	}
	if !ok || url == "" {
		return
	}
	if req.IsPost() && f.promptResubmit(tab, url, req, delta, base, history.TransitionOther, scroll) {
		return
	}
	f.navigateTabNoHistoryTraverse(url, delta, base, history.TransitionOther, scroll)
}

// navigateTabNoHistory loads a URL without pushing to history.
func (f *framePath) navigateTabNoHistory(rawURL string) {
	f.navigateTabNoHistoryTraverse(rawURL, 0, 0, history.TransitionOther, 0)
}

// navigateTabNoHistoryTraverse is navigateTabNoHistory carrying a traversal
// commit: on success the history cursor steps by delta from base and the
// viewport returns to restoreScroll, the offset that entry was left at.
func (f *framePath) navigateTabNoHistoryTraverse(rawURL string, delta, base int, transition history.Transition, restoreScroll int32) {
	f.navigateTabNoHistoryTraverseRequest(rawURL, delta, base, transition, restoreScroll, navRequest{})
}

// navigateTabNoHistoryTraverseRequest is navigateTabNoHistoryTraverse with the
// request to replay. A POST entry in the trail needs the same method and body
// the original submission sent, not a bare GET.
func (f *framePath) navigateTabNoHistoryTraverseRequest(rawURL string, delta, base int, transition history.Transition, restoreScroll int32, req navRequest) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	f.dropResubmit(tab.ID())
	f.closeFind()
	if f.toolbar != nil {
		f.toolbar.CloseSuggestions()
	}
	u := normalizeAddress(rawURL, f.currentPreferences().SearchURLTemplate)

	tab.Nav.Mu.Lock()
	if tab.Nav.Cancel != nil {
		tab.Nav.Cancel()
	}
	ctx, cancel := context.WithCancel(f.navContext())
	tab.Nav.Cancel = cancel
	tab.Nav.Serial++
	serial := tab.Nav.Serial
	tab.Nav.Loading = true
	tab.Nav.Mu.Unlock()

	tab.SetLoading(true)
	tab.SetError("")
	if f.toolbar != nil {
		f.toolbar.SetLoading(true)
	}
	f.publishAccessibility()

	f.navWg.Add(1)
	client := f.clientForTab(tab)
	go func() {
		defer f.navWg.Done()
		layer, _, bgColor, sess, finalURL, err := f.loadDocumentCtx(ctx, client, u, f.contentWidth(), f.contentHeight(), float32(f.config.dpr), f.downloadDirectory(), true, true, req)
		res := navResult{tabID: tab.ID(), serial: serial, url: u, noHistory: true, traverse: delta, traverseBase: base, traverseScroll: restoreScroll, transition: transition, request: req, requestedURL: u}
		if finalURL != "" {
			res.url = finalURL
		}
		var dd downloadDone
		var dp downloadPaused
		if errors.As(err, &dd) {
			res.downloadPath = dd.path
		} else if errors.As(err, &dp) {
			res.downloadPaused = true
		} else {
			res.err = err
		}
		res.layer, res.bgColor, res.session = layer, bgColor, sess
		f.navResults <- res
	}()
}

// reloadTab re-fetches the active tab's current history entry. Reloading a
// POST result asks first: repeating the request can repeat its side effects.
func (f *framePath) reloadTab() {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	url := tab.URL()
	var req toolbar.Request
	if current, r, ok := tab.History().CurrentRequest(); ok {
		if current != "" {
			url = current
		}
		req = r
	}
	if url == "" {
		return
	}
	if req.IsPost() && f.promptResubmit(tab, url, req, 0, 0, history.TransitionReload, 0) {
		return
	}
	f.navigateTabNoHistoryTraverse(url, 0, 0, history.TransitionReload, 0)
}

// stopTab cancels only the active tab's in-flight navigation. Advancing the
// serial before canceling makes any result already queued by that load stale,
// so it cannot replace the document that is still displayed.
func (f *framePath) stopTab() {
	if f.tabMgr == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}

	tab.Nav.Mu.Lock()
	if !tab.Nav.Loading {
		tab.Nav.Mu.Unlock()
		return
	}
	cancel := tab.Nav.Cancel
	tab.Nav.Cancel = nil
	tab.Nav.Serial++
	tab.Nav.Loading = false
	tab.Nav.Mu.Unlock()

	if cancel != nil {
		cancel()
	}
	tab.SetLoading(false)
	tab.SetError("")
	if f.tabMgr.Active() == tab {
		f.syncTabToToolbar()
	}
}

// scrollToAnchor scrolls the active tab's viewport to an element with matching ID.
// It extracts the fragment from the URL and finds the corresponding layout object.
func (f *framePath) scrollToAnchor(tab *tabs.Tab, href string) {
	sess := tab.Session()
	if sess == nil {
		return
	}

	// Extract fragment from URL
	u, err := url.Parse(href)
	if err != nil {
		return
	}
	fragment := u.Fragment
	if fragment == "" {
		return
	}

	// Find target element by scrolling to its position
	targetY := sess.ScrollToID(fragment)
	if targetY > 0 {
		tab.SetScrollY(int32(targetY))
		f.sched.SetViewport(frame.Viewport{
			Offset: frame.Point{X: 0, Y: int32(targetY)},
			Size:   f.devSize(),
		})
	}
}

// applySessionState rebuilds the tab set from a saved session. The first saved
// tab overwrites the fresh New Tab; background tabs stay unloaded until shown.
func (f *framePath) applySessionState(st session.State) {
	tabsList := f.tabMgr.Tabs()
	if len(tabsList) == 0 || len(st.Tabs) == 0 {
		return
	}
	applyTabState(tabsList[0], st.Tabs[0])
	for _, ts := range st.Tabs[1:] {
		applyTabState(f.tabMgr.NewTab(), ts)
	}
	if st.Active > 0 {
		restored := f.tabMgr.Tabs()
		if st.Active < len(restored) {
			f.tabMgr.SwitchTo(restored[st.Active].ID())
		}
	}
	if active := f.tabMgr.Active(); active != nil && active.URL() != "" {
		f.restoredActiveURL = active.URL()
		f.markRestoredScroll(active)
	}
}

func applyTabState(t *tabs.Tab, ts session.TabState) {
	t.SetURL(ts.URL)
	t.SetTitle(ts.Title)
	t.SetHistory(toolbar.RestoreHistory(ts.History, ts.HistoryIndex))
	t.SetScrollY(int32(ts.ScrollY))
	if t.Title() == "" {
		t.SetTitle("New Tab")
	}
}

// markRestoredScroll records the active restored tab's offset so the load that
// session restore starts keeps it: a fresh navigation resets the viewport to
// the top, which would otherwise drop the restored position (Bug C4).
func (f *framePath) markRestoredScroll(tab *tabs.Tab) {
	if tab == nil {
		return
	}
	f.restoredScrollTabID = tab.ID()
	f.restoredScrollY = tab.ScrollY()
}

// restoreMarkedScroll re-applies the marked restore offset right after a
// navigation's top-of-page reset, and consumes the intent so any later
// navigation to the same tab opens at the top as usual.
func (f *framePath) restoreMarkedScroll(tab *tabs.Tab) {
	if f.restoredScrollTabID == 0 || f.restoredScrollTabID != tab.ID() {
		return
	}
	f.restoredScrollTabID = 0
	if f.restoredScrollY == 0 {
		return
	}
	tab.SetScrollY(f.restoredScrollY)
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: f.restoredScrollY},
		Size:   f.devSize(),
	})
	f.restoredScrollY = 0
}

// sessionState snapshots the open tabs for saving.
func sessionState(f *framePath) session.State {
	var st session.State
	activeTab := f.tabMgr.Active()
	savedActiveIdx := 0
	for _, tab := range f.tabMgr.Tabs() {
		if tab.Incognito() {
			continue // Incognito tabs are ephemeral and never persisted
		}
		if tab == activeTab {
			savedActiveIdx = len(st.Tabs)
		}
		entries, idx := tab.History().Entries()
		st.Tabs = append(st.Tabs, session.TabState{
			URL:          tab.URL(),
			Title:        tab.Title(),
			History:      entries,
			HistoryIndex: idx,
			// ScrollX is persisted for the state format; the engine has no
			// horizontal scroll yet, so the viewport offset's X is 0.
			ScrollX: int(f.sched.Viewport().Offset.X),
			ScrollY: int(tab.ScrollY()),
		})
	}
	st.Active = savedActiveIdx
	return st
}

// defaultSessionSnapshotEvery is the crash-journal cadence: an abrupt kill
// loses at most this much tab state.
const defaultSessionSnapshotEvery = 30 * time.Second

// crashedSessionState returns the journal awaiting a restore decision.
func (f *framePath) crashedSessionState() *session.State {
	if f == nil {
		return nil
	}
	f.crashedMu.RLock()
	defer f.crashedMu.RUnlock()
	return f.crashedSession
}

func (f *framePath) setCrashedSession(st *session.State) {
	f.crashedMu.Lock()
	defer f.crashedMu.Unlock()
	f.crashedSession = st
}

// runSessionSnapshots journals the open tabs until ctx is cancelled, so a
// crash loses at most one interval rather than everything since the last
// navigation. Private and paced runs never write the journal.
func (f *framePath) runSessionSnapshots(ctx context.Context) {
	if f == nil || ctx == nil || f.config.private || f.config.paced() || f.config.sessionActiveFile == "" || f.sessionDisabled.Load() {
		return
	}
	every := f.sessionSnapshotEvery
	if every <= 0 {
		every = defaultSessionSnapshotEvery
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.saveActiveSession()
		}
	}
}

// saveActiveSession persists the current open tab state to sessionActiveFile.
// This acts as a crash recovery journal: on clean exit the file is deleted. If the
// process crashes or is killed, the file survives so the session can be
// recovered on next launch.
func (f *framePath) saveActiveSession() {
	if f == nil || f.config.private || f.config.paced() || f.config.sessionActiveFile == "" || f.sessionDisabled.Load() {
		return
	}
	f.sessionMu.Lock()
	defer f.sessionMu.Unlock()
	if f.sessionDisabled.Load() {
		return
	}
	if f.crashedSessionState() != nil {
		// Do not overwrite the crashed journal while the restore prompt is active.
		return
	}
	if f.tabMgr == nil {
		return
	}
	st := sessionState(f)
	if len(st.Tabs) == 0 {
		return
	}
	// Do not persist a state consisting solely of the restore prompt itself
	if len(st.Tabs) == 1 && strings.HasPrefix(strings.ToLower(st.Tabs[0].URL), "goosie://restore") {
		return
	}
	_ = session.Save(f.config.sessionActiveFile, st)
}

// restoreCrashedSession restores the tabs from a previous crashed session.
func (f *framePath) restoreCrashedSession(st session.State) {
	if len(st.Tabs) == 0 || f.tabMgr == nil {
		return
	}
	tabsList := f.tabMgr.Tabs()
	// If the active/only tab is the restore page, overwrite it with the first crashed tab.
	if len(tabsList) == 1 && strings.HasPrefix(strings.ToLower(tabsList[0].URL()), "goosie://restore") {
		f.applySessionState(st)
	} else {
		for _, ts := range st.Tabs {
			t := f.tabMgr.NewTab()
			applyTabState(t, ts)
		}
		if st.Active >= 0 {
			all := f.tabMgr.Tabs()
			idx := len(all) - len(st.Tabs) + st.Active
			if idx >= 0 && idx < len(all) {
				f.tabMgr.SwitchTo(all[idx].ID())
			}
		}
	}
	if active := f.tabMgr.Active(); active != nil && active.URL() != "" && !strings.HasPrefix(strings.ToLower(active.URL()), "goosie://restore") {
		f.markRestoredScroll(active)
		f.navigateTab(active.URL())
	}
	f.saveActiveSession()
}

// maxTabTitleRunes bounds the title a document can hand the chrome. An
// unbounded <title> would be measured glyph by glyph on every tab-bar frame
// and written into the saved session verbatim.
const maxTabTitleRunes = 160

// documentTitle returns the display title of a loaded document: the text of
// its first <title> with non-empty content, whitespace collapsed to single
// spaces and capped to maxTabTitleRunes. Returns "" when the document has no
// usable title, which leaves the tab showing its URL.
//
// Called on the navigation drain before the session becomes visible to events,
// the same window findManifestLink reads the tree in.
func documentTitle(sess *engine.Session) string {
	if sess == nil || sess.Doc == nil {
		return ""
	}
	for _, node := range sess.Doc.ElementsByTagName("title") {
		title := strings.Join(strings.Fields(node.TextContent()), " ")
		if title == "" {
			continue
		}
		if r := []rune(title); len(r) > maxTabTitleRunes {
			title = string(r[:maxTabTitleRunes-1]) + "…"
		}
		return title
	}
	return ""
}

// findManifestLink searches the DOM tree for a <link rel="manifest" href="..."> tag and resolves its URL.
func findManifestLink(root *dom.Node, baseURL string) string {
	if root == nil {
		return ""
	}
	var href string
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if href != "" || n == nil {
			return
		}
		if strings.EqualFold(n.Data, "link") {
			isManifest := false
			linkHref := ""
			for _, attr := range n.Attr {
				if strings.EqualFold(attr.Name, "rel") {
					for _, val := range strings.Fields(attr.Value) {
						if strings.EqualFold(val, "manifest") {
							isManifest = true
						}
					}
				} else if strings.EqualFold(attr.Name, "href") {
					linkHref = strings.TrimSpace(attr.Value)
				}
			}
			if isManifest && linkHref != "" {
				href = linkHref
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if href == "" {
		return ""
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}
