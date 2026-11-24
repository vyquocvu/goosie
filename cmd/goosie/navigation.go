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
	traverse       int
	traverseBase   int
	traverseScroll int32
	transition     history.Transition
	request        navRequest
	requestedURL   string
}

type navRequest = toolbar.Request

type downloadDone struct {
	path               string
	url                string
	contentType        string
	contentDisposition string
}

func (e downloadDone) Error() string { return "downloaded to " + e.path }

func (e downloadDone) trigger() string {
	if e.contentDisposition != "" {
		return "Content-Disposition: " + e.contentDisposition
	}
	if e.contentType != "" {
		return "Content-Type: " + e.contentType
	}
	return "the response was classified as a download"
}

type downloadPaused struct{ id string }

func (downloadPaused) Error() string { return "download paused" }

type imgResult struct {
	tabID   uint64
	serial  uint64
	session *engine.Session
}

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

func (f *framePath) clientForTab(tab *tabs.Tab) net.HTTP {
	if tab != nil && tab.Incognito() && f.privateClient != nil {
		return f.privateClient
	}
	return f.client
}

func (f *framePath) newIncognitoTab() *tabs.Tab {
	tab := f.tabMgr.NewIncognitoTab()
	dev := frame.Size{W: int32(f.contentWidth()), H: int32(f.contentHeight())}
	_, layer := blankLayer(dev)
	tab.SetLayer(layer)
	tab.SetBGColor(frame.RGB(40, 44, 52))
	f.switchTab(tab.ID())
	return tab
}

func (f *framePath) navContext() context.Context {
	if f.runCtx != nil {
		return f.runCtx
	}
	return context.Background()
}

func (f *framePath) navigateTab(rawURL string) {
	f.navigateTabTransition(rawURL, history.TransitionTyped)
}

func (f *framePath) navigateTabTransition(rawURL string, transition history.Transition) {
	f.navigateTabRequest(rawURL, transition, navRequest{})
}

func (f *framePath) navigateTabRequest(rawURL string, transition history.Transition, req navRequest) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	if action, ok := resubmitAction(rawURL); ok {
		f.resolveResubmitAction(tab, action)
		return
	}
	if f.sched != nil {
		tab.History().SetCurrentScroll(f.sched.Viewport().Offset.Y)
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

func (f *framePath) loadDocumentCtx(ctx context.Context, client net.HTTP, u string, viewportW, viewportH int, scale float32, downloadDir string, deferImages, deferFonts bool, reqs ...navRequest) (*frame.Layer, paint.SceneSpec, frame.Color, *engine.Session, string, error) {
	policy := scriptPolicy{timeout: f.scriptTimeout}
	if strings.HasPrefix(strings.ToLower(u), "goosie://") {
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

func (f *framePath) drainNavResults() {
	for result := range f.navResults {
		f.applyNavResult(result)
	}
}

func (f *framePath) applyNavResult(result navResult) {
	tab := f.tabMgr.TabByID(result.tabID)
	if tab == nil {
		if result.session != nil {
			result.session.Close()
		}
		return
	}

	tab.Nav.Mu.Lock()
	if tab.Nav.Serial != result.serial {
		tab.Nav.Mu.Unlock()
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

	if result.session != nil && result.session.HasScriptTimeout() {
		if f.installUnresponsivePrompt(tab, result.url, result.session.ScriptTimeoutCount(), result.serial, result.session) {
			return
		}
	}

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
	tab.SetFavicon(nil)
	tab.SetURL(result.url)
	tabTitle := result.url
	if strings.HasPrefix(strings.ToLower(result.url), "goosie://") {
		title, _ := f.buildInternalPageHTML(result.url)
		if title != "" {
			tabTitle = title
		}
	} else if docTitle != "" {
		tabTitle = docTitle
	}
	tab.SetTitle(tabTitle)
	if oldSession != nil && oldSession != result.session {
		oldSession.Close()
	}
	if !result.noHistory {
		req := toolbar.Request{}
		if result.request.IsPost() && result.url == result.requestedURL {
			req = result.request
		}
		tab.History().PushRequest(result.url, req)
	}
	traversed := false
	if result.traverse != 0 {
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
		f.syncContentFocus(tab)
		f.sched.SetPlan(frame.FramePlan{
			Serial:     result.serial,
			Layers:     []*frame.Layer{result.layer},
			Background: tab.BGColor(),
		})
		if !result.noHistory {
			tab.SetScrollY(0)
			f.sched.SetViewport(frame.Viewport{
				Offset: frame.Point{},
				Size:   f.devSize(),
			})
			f.restoreMarkedScroll(tab)

			if href := result.url; strings.Contains(href, "#") {
				f.scrollToAnchor(tab, href)
			}
		} else if traversed {
			f.restoreTraversalScroll(tab, result)
		}
		f.publishAccessibility()
	} else if traversed {
		f.restoreTraversalScroll(tab, result)
	}

	if sess := result.session; sess != nil {
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
		sess.SyncViewportScroll(0, float64(y))
	}
}

func (f *framePath) drainImageResults() {
	for res := range f.imgResults {
		f.applyImageResult(res)
	}
}

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

func (f *framePath) traverseTab(delta int) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
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

func (f *framePath) navigateTabNoHistory(rawURL string) {
	f.navigateTabNoHistoryTraverse(rawURL, 0, 0, history.TransitionOther, 0)
}

func (f *framePath) navigateTabNoHistoryTraverse(rawURL string, delta, base int, transition history.Transition, restoreScroll int32) {
	f.navigateTabNoHistoryTraverseRequest(rawURL, delta, base, transition, restoreScroll, navRequest{})
}

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

func (f *framePath) scrollToAnchor(tab *tabs.Tab, href string) {
	sess := tab.Session()
	if sess == nil {
		return
	}

	u, err := url.Parse(href)
	if err != nil {
		return
	}
	fragment := u.Fragment
	if fragment == "" {
		return
	}

	targetY := sess.ScrollToID(fragment)
	if targetY > 0 {
		tab.SetScrollY(int32(targetY))
		f.sched.SetViewport(frame.Viewport{
			Offset: frame.Point{X: 0, Y: int32(targetY)},
			Size:   f.devSize(),
		})
	}
}

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

func (f *framePath) markRestoredScroll(tab *tabs.Tab) {
	if tab == nil {
		return
	}
	f.restoredScrollTabID = tab.ID()
	f.restoredScrollY = tab.ScrollY()
}

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

func sessionState(f *framePath) session.State {
	var st session.State
	activeTab := f.tabMgr.Active()
	savedActiveIdx := 0
	for _, tab := range f.tabMgr.Tabs() {
		if tab.Incognito() {
			continue
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
			ScrollX:      int(f.sched.Viewport().Offset.X),
			ScrollY:      int(tab.ScrollY()),
		})
	}
	st.Active = savedActiveIdx
	return st
}

const defaultSessionSnapshotEvery = 30 * time.Second

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
		return
	}
	if f.tabMgr == nil {
		return
	}
	st := sessionState(f)
	if len(st.Tabs) == 0 {
		return
	}
	if len(st.Tabs) == 1 && strings.HasPrefix(strings.ToLower(st.Tabs[0].URL), "goosie://restore") {
		return
	}
	_ = session.Save(f.config.sessionActiveFile, st)
}

func (f *framePath) restoreCrashedSession(st session.State) {
	if len(st.Tabs) == 0 || f.tabMgr == nil {
		return
	}
	tabsList := f.tabMgr.Tabs()
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

const maxTabTitleRunes = 160

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
