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
	"github.com/vyquocvu/goosie/internal/viewsource"
)

type navigationTrust uint8

const (
	trustedNavigation navigationTrust = iota
	untrustedNavigation
)

type navigationTrustContextKey struct{}

type navigationSourceContextKey struct{}

type navigationTargetContextKey struct{}

type scriptNavigationContextKey struct{}

var errInternalNavigationFromContent = errors.New("goosie: web content cannot open internal browser pages")

var errNavigationSchemeFromContent = errors.New("goosie: web content cannot navigate to this URL scheme")

func withNavigationTrust(ctx context.Context, trust navigationTrust) context.Context {
	return context.WithValue(ctx, navigationTrustContextKey{}, trust)
}

func navigationTrustFromContext(ctx context.Context) navigationTrust {
	if ctx == nil {
		return trustedNavigation
	}
	if trust, ok := ctx.Value(navigationTrustContextKey{}).(navigationTrust); ok {
		return trust
	}
	return trustedNavigation
}

func withNavigationSource(ctx context.Context, source string) context.Context {
	return context.WithValue(ctx, navigationSourceContextKey{}, source)
}

func navigationSourceFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	source, _ := ctx.Value(navigationSourceContextKey{}).(string)
	return source
}

func withNavigationTarget(ctx context.Context, target string) context.Context {
	return context.WithValue(ctx, navigationTargetContextKey{}, target)
}

func navigationTargetFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	target, _ := ctx.Value(navigationTargetContextKey{}).(string)
	return target
}

func withScriptNavigation(ctx context.Context) context.Context {
	return context.WithValue(ctx, scriptNavigationContextKey{}, true)
}

func scriptNavigationFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	scriptNavigation, _ := ctx.Value(scriptNavigationContextKey{}).(bool)
	return scriptNavigation
}

func navigationSchemeAllowed(source, target string) bool {
	targetURL, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return false
	}
	if targetURL.Scheme == "" {
		sourceURL, sourceErr := url.Parse(strings.TrimSpace(source))
		if sourceErr != nil {
			return false
		}
		targetURL = sourceURL.ResolveReference(targetURL)
	}
	switch strings.ToLower(targetURL.Scheme) {
	case "http", "https":
		return true
	case "file":
		sourceURL, sourceErr := url.Parse(strings.TrimSpace(source))
		return sourceErr == nil && strings.EqualFold(sourceURL.Scheme, "file")
	default:
		return false
	}
}

func unwrapViewSourceNavigation(rawURL string) string {
	for {
		target, ok := viewsource.IsViewSourceURL(strings.TrimSpace(rawURL))
		if !ok {
			return strings.TrimSpace(rawURL)
		}
		rawURL = target
	}
}

func isInternalNavigationURL(rawURL string) bool {
	target := unwrapViewSourceNavigation(rawURL)
	if strings.HasPrefix(strings.ToLower(target), "goosie://") {
		return true
	}
	u, err := url.Parse(target)
	return err == nil && strings.EqualFold(u.Scheme, "goosie")
}

const trustedNavigationPromptFlag uint64 = 1 << 63

func trustedNavigationPromptKey(tabID uint64) uint64 {
	return tabID | trustedNavigationPromptFlag
}

func (f *framePath) markTrustedNavigationPrompt(tabID uint64) {
	f.resubmitMu.Lock()
	defer f.resubmitMu.Unlock()
	if f.resubmits == nil {
		f.resubmits = make(map[uint64]*pendingResubmit)
	}
	f.resubmits[trustedNavigationPromptKey(tabID)] = nil
}

func (f *framePath) clearTrustedNavigationPrompt(tabID uint64) {
	f.resubmitMu.Lock()
	defer f.resubmitMu.Unlock()
	delete(f.resubmits, trustedNavigationPromptKey(tabID))
}

func (f *framePath) hasTrustedNavigationPrompt(tabID uint64) bool {
	f.resubmitMu.Lock()
	defer f.resubmitMu.Unlock()
	_, ok := f.resubmits[trustedNavigationPromptKey(tabID)]
	return ok
}

func (f *framePath) hasPendingResubmit(tabID uint64) bool {
	f.resubmitMu.Lock()
	defer f.resubmitMu.Unlock()
	_, ok := f.resubmits[tabID]
	return ok
}

func (f *framePath) navigationTrustForTab(tab *tabs.Tab) navigationTrust {
	if tab == nil {
		return untrustedNavigation
	}
	u, err := url.Parse(strings.TrimSpace(tab.URL()))
	if err != nil || !strings.EqualFold(u.Scheme, "goosie") {
		if f.hasPendingResubmit(tab.ID()) || f.hasTrustedNavigationPrompt(tab.ID()) {
			return trustedNavigation
		}
		return untrustedNavigation
	}
	if strings.EqualFold(u.Host, "reader") {
		return untrustedNavigation
	}
	return trustedNavigation
}

func (f *framePath) navigationTrustForTarget(tab *tabs.Tab, rawURL string) navigationTrust {
	if f.navigationTrustForTab(tab) == trustedNavigation {
		return trustedNavigation
	}
	if tab == nil {
		return untrustedNavigation
	}
	source, sourceErr := url.Parse(strings.TrimSpace(tab.URL()))
	target, targetErr := url.Parse(strings.TrimSpace(rawURL))
	if sourceErr != nil || targetErr != nil || !strings.EqualFold(source.Scheme, "goosie") || !strings.EqualFold(source.Host, "reader") {
		return untrustedNavigation
	}
	if strings.EqualFold(target.Scheme, "goosie") && strings.EqualFold(target.Host, "reader") {
		return trustedNavigation
	}
	return untrustedNavigation
}

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
	replaceHistory bool
	traverse       int
	traverseBase   int
	traverseScroll int32
	transition     history.Transition
	request        navRequest
	requestedURL   string
	historySeed    historySeed
	historyDelta   int
}

type navRequest = toolbar.Request

func historyStateAt(seed historySeed, h *toolbar.History) []byte {
	if h == nil {
		return nil
	}
	_, states, _, index := h.Snapshot()
	if index >= 0 && index < len(states) {
		return append([]byte(nil), states[index]...)
	}
	if seed.Index >= 0 && seed.Index < len(seed.States) {
		return append([]byte(nil), seed.States[seed.Index]...)
	}
	return nil
}

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
	f.navigateTabTransitionWithTrust(rawURL, transition, trustedNavigation)
}

func (f *framePath) navigateTabTransitionWithTrust(rawURL string, transition history.Transition, trust navigationTrust) {
	f.navigateTabRequestWithTrustAndHistory(rawURL, transition, navRequest{}, trust, false)
}

func (f *framePath) navigateTabReplaceWithTrust(rawURL string, transition history.Transition, trust navigationTrust) {
	f.navigateTabRequestWithTrustAndHistory(rawURL, transition, navRequest{}, trust, true)
}

func (f *framePath) navigateTabRequest(rawURL string, transition history.Transition, req navRequest) {
	f.navigateTabRequestWithTrust(rawURL, transition, req, trustedNavigation)
}

func (f *framePath) navigateTabRequestWithTrust(rawURL string, transition history.Transition, req navRequest, trust navigationTrust) {
	f.navigateTabRequestWithTrustAndHistory(rawURL, transition, req, trust, false)
}

func (f *framePath) navigateTabRequestWithTrustAndHistory(rawURL string, transition history.Transition, req navRequest, trust navigationTrust, replaceHistory bool) {
	f.navigateTabRequestWithTrustAndHistoryMode(rawURL, transition, req, trust, replaceHistory, false)
}

func (f *framePath) navigateScriptTabRequest(rawURL string, replaceHistory bool) {
	f.navigateTabRequestWithTrustAndHistoryMode(rawURL, history.TransitionLink, navRequest{}, untrustedNavigation, replaceHistory, true)
}

func (f *framePath) navigateTabRequestWithTrustAndHistoryMode(rawURL string, transition history.Transition, req navRequest, trust navigationTrust, replaceHistory, scriptNavigation bool, seeds ...windowSeed) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	var winSeed windowSeed
	if len(seeds) > 0 {
		winSeed = seeds[0]
	}
	if f.windowContexts != nil {
		if winSeed.Handle == "" {
			winSeed.Handle = f.windowContexts.handleForTab(tab.ID())
		}
		if winSeed.Handle == "" {
			winSeed.Handle = f.windowContexts.allocateRoot()
		}
		f.windowContexts.register(winSeed.Handle, tab.ID(), winSeed.Name)
	}
	docSeed := historySeed{Window: winSeed}
	if action, ok := resubmitAction(rawURL); ok && trust == trustedNavigation {
		f.resolveResubmitAction(tab, action)
		return
	}
	f.clearTrustedNavigationPrompt(tab.ID())
	tab.History().SetCurrentScroll(f.currentHistoryScroll(tab))
	f.dropResubmit(tab.ID())
	f.closeFind()
	if f.toolbar != nil {
		f.toolbar.CloseSuggestions()
	}
	u := normalizeAddress(rawURL, f.currentPreferences().SearchURLTemplate)
	if winSeed.Blank {
		u = strings.TrimSpace(rawURL)
	}

	tab.Nav.Mu.Lock()
	if tab.Nav.Cancel != nil {
		tab.Nav.Cancel()
	}
	ctx, cancel := context.WithCancel(f.navContext())
	ctx = withNavigationTrust(ctx, trust)
	ctx = withNavigationSource(ctx, tab.URL())
	ctx = withNavigationTarget(ctx, rawURL)
	if scriptNavigation {
		ctx = withScriptNavigation(ctx)
	}
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
	if winSeed.Blank {
		client = blankDocumentClient{}
	}
	go func() {
		defer f.navWg.Done()
		layer, _, bgColor, sess, finalURL, err := f.loadDocumentCtxWithHistory(ctx, client, u, f.contentWidth(), f.contentHeight(), float32(f.config.dpr), f.downloadDirectory(), true, true, docSeed, req)
		res := navResult{tabID: tab.ID(), serial: serial, url: u, transition: transition, request: req, requestedURL: u, replaceHistory: replaceHistory}
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
	return f.loadDocumentCtxWithHistory(ctx, client, u, viewportW, viewportH, scale, downloadDir, deferImages, deferFonts, historySeed{}, reqs...)
}

func (f *framePath) loadDocumentCtxWithHistory(ctx context.Context, client net.HTTP, u string, viewportW, viewportH int, scale float32, downloadDir string, deferImages, deferFonts bool, seed historySeed, reqs ...navRequest) (*frame.Layer, paint.SceneSpec, frame.Color, *engine.Session, string, error) {
	if navigationTrustFromContext(ctx) == untrustedNavigation {
		if isInternalNavigationURL(u) {
			return nil, paint.SceneSpec{}, frame.Color(0), nil, "", errInternalNavigationFromContent
		}
		target := u
		if scriptNavigationFromContext(ctx) {
			target = navigationTargetFromContext(ctx)
		}
		blankPopup := seed.Window.Blank && isAboutBlankURL(target)
		if !blankPopup && !navigationSchemeAllowed(navigationSourceFromContext(ctx), target) {
			return nil, paint.SceneSpec{}, frame.Color(0), nil, "", errNavigationSchemeFromContent
		}
	}
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
	extra := []any{f.downloads, storage, tabID, appMode, f.userscripts, f.perms, broadcastHub, f.mediaBackend, policy, f.windowContexts}
	if clipboard := platform.NewClipboard(); clipboard != nil {
		extra = append(extra, clipboard)
	}
	if f.currentPreferences().AskDownloadLocation {
		extra = append(extra, f.window)
	}
	if len(reqs) > 0 {
		extra = append(extra, reqs[0])
	}
	if len(seed.URLs) > 0 || seed.Window.Handle != "" {
		extra = append(extra, seed)
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

	if result.downloadPath != "" {
		tab.SetLoading(false)
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
		tab.SetLoading(false)
		if f.toolbar != nil && f.tabMgr.Active() == tab {
			f.toolbar.SetLoading(false)
		}
		if f.toolbar != nil && f.downloads != nil {
			f.toolbar.SetDownloadStats(f.downloads.ActiveCount(), f.downloads.TotalCount(), f.downloads.OverallProgress())
		}
		return
	}
	if result.err != nil {
		tab.SetLoading(false)
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
				if !errors.Is(result.err, errInternalNavigationFromContent) && !errors.Is(result.err, errNavigationSchemeFromContent) {
					tab.SetURL(result.url)
				}
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
			tab.SetLoading(false)
			f.markTrustedNavigationPrompt(tab.ID())
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
	if f.windowContexts != nil && result.session != nil {
		if handle := result.session.WindowHandle(); handle != "" {
			origin := result.session.WindowOrigin()
			if origin == "" {
				origin = browsingContextOrigin(result.url)
			}
			f.windowContexts.bindSession(handle, result.session, origin)
			if sess, messages := f.windowContexts.takePending(result.tabID); sess != nil {
				for _, message := range messages {
					sess.DeliverWindowMessage(message)
				}
			}
		}
	}
	tab.SetSession(result.session)
	tab.SetBGColor(result.bgColor)
	tab.SetFavicon(nil)
	tab.SetURL(result.url)
	tabTitle := docTitle
	if tabTitle == "" {
		tabTitle = result.url
	}
	tab.SetTitle(tabTitle)
	if oldSession != nil && oldSession != result.session {
		oldSession.Close()
	}
	req := toolbar.Request{}
	if result.request.IsPost() && result.url == result.requestedURL {
		req = result.request
	}
	if result.replaceHistory {
		tab.History().ReplaceCurrentState(result.url, req, nil, false)
	} else if !result.noHistory {
		tab.History().PushState(result.url, req, nil, 0, false)
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
	if traversed && result.session != nil {
		state := historyStateAt(result.historySeed, tab.History())
		urls, _, _, index := tab.History().Snapshot()
		result.session.ApplyHistoryEntry(result.url, state, index, len(urls), result.traverse)
	}
	if !tab.Incognito() && f.history != nil && !strings.HasPrefix(strings.ToLower(result.url), "goosie://") {
		transition := result.transition
		if transition == "" {
			transition = history.TransitionOther
		}
		f.history.RecordVisit(result.url, tabTitle, transition)
	}

	if f.tabMgr.Active() == tab {
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
		tab.SetLoading(false)
		f.syncTabToToolbar()
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
	tab.SetLoading(false)
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
	if tab == nil || delta == 0 {
		return
	}
	tab.History().SetCurrentScroll(f.currentHistoryScroll(tab))
	url, req, scroll, state, _, base, ok := tab.History().PeekEntry(delta)
	if !ok || url == "" {
		return
	}
	if req.IsPost() && f.promptResubmit(tab, url, req, delta, base, history.TransitionOther, scroll) {
		return
	}
	if tab.History().SameDocumentTraversal(delta) {
		f.applyLocalHistoryTraversal(tab, delta, base, url, state, scroll)
		return
	}
	urls, states, same, _ := tab.History().Snapshot()
	targetIndex := base + delta
	if targetIndex < 0 || targetIndex >= len(urls) {
		return
	}
	seed := historySeed{URLs: urls, States: states, SameDocument: same, Index: targetIndex}
	f.navigateTabNoHistoryTraverseRequestWithTrustMode(url, delta, base, history.TransitionOther, scroll, navRequest{}, trustedNavigation, false, seed)
}

func (f *framePath) currentHistoryScroll(tab *tabs.Tab) int32 {
	if tab == nil {
		return 0
	}
	f.scrollMu.Lock()
	defer f.scrollMu.Unlock()
	if f.sched != nil && f.tabMgr != nil && f.tabMgr.Active() == tab {
		return f.sched.ScrollOffset().Y
	}
	return tab.ScrollY()
}

func (f *framePath) applyLocalHistoryTraversal(tab *tabs.Tab, delta, base int, targetURL string, state []byte, scroll int32) {
	tab.History().SetCurrentScroll(f.currentHistoryScroll(tab))
	if !tab.History().Go(delta, base) {
		return
	}
	tab.History().SetCurrentScroll(scroll)
	tab.SetURL(targetURL)
	urls, _, _, index := tab.History().Snapshot()
	f.restoreHistoryScroll(tab, scroll)
	if sess := tab.Session(); sess != nil {
		sess.ApplyHistoryEntry(targetURL, state, index, len(urls), delta)
	}
	if f.tabMgr.Active() == tab {
		f.syncTabToToolbar()
		f.publishAccessibility()
	}
	f.saveActiveSession()
}

func (f *framePath) restoreHistoryScroll(tab *tabs.Tab, y int32) {
	if f.sched == nil || tab == nil {
		return
	}
	f.scrollMu.Lock()
	defer f.scrollMu.Unlock()
	if f.tabMgr != nil && f.tabMgr.Active() != tab {
		return
	}
	if y < 0 {
		y = 0
	}
	tab.SetScrollY(y)
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: y},
		Size:   f.devSize(),
	})
	f.lastSyncedScrollY = y
	if sess := tab.Session(); sess != nil {
		sess.SyncViewportScroll(0, float64(y))
	}
}

func (f *framePath) navigateTabNoHistory(rawURL string) {
	f.navigateTabNoHistoryTraverse(rawURL, 0, 0, history.TransitionOther, 0)
}

func (f *framePath) navigateTabNoHistoryTraverse(rawURL string, delta, base int, transition history.Transition, restoreScroll int32) {
	f.navigateTabNoHistoryTraverseRequest(rawURL, delta, base, transition, restoreScroll, navRequest{})
}

func (f *framePath) navigateTabNoHistoryTraverseRequest(rawURL string, delta, base int, transition history.Transition, restoreScroll int32, req navRequest) {
	f.navigateTabNoHistoryTraverseRequestWithTrust(rawURL, delta, base, transition, restoreScroll, req, trustedNavigation)
}

func (f *framePath) navigateTabNoHistoryTraverseRequestWithTrust(rawURL string, delta, base int, transition history.Transition, restoreScroll int32, req navRequest, trust navigationTrust) {
	f.navigateTabNoHistoryTraverseRequestWithTrustMode(rawURL, delta, base, transition, restoreScroll, req, trust, false)
}

func (f *framePath) navigateTabNoHistoryTraverseRequestWithTrustMode(rawURL string, delta, base int, transition history.Transition, restoreScroll int32, req navRequest, trust navigationTrust, scriptNavigation bool, seeds ...historySeed) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	seed := historySeed{}
	if len(seeds) > 0 {
		seed = seeds[0]
	} else if urls, states, same, index := tab.History().Snapshot(); len(urls) > 0 {
		seed = historySeed{URLs: urls, States: states, SameDocument: same, Index: index}
	}
	if f.windowContexts != nil {
		if seed.Window.Handle == "" {
			seed.Window.Handle = f.windowContexts.handleForTab(tab.ID())
		}
		if seed.Window.Handle == "" {
			seed.Window.Handle = f.windowContexts.allocateRoot()
		}
		f.windowContexts.register(seed.Window.Handle, tab.ID(), seed.Window.Name)
	}
	f.clearTrustedNavigationPrompt(tab.ID())
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
	ctx = withNavigationTrust(ctx, trust)
	ctx = withNavigationSource(ctx, tab.URL())
	ctx = withNavigationTarget(ctx, rawURL)
	if scriptNavigation {
		ctx = withScriptNavigation(ctx)
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
	}
	f.publishAccessibility()

	f.navWg.Add(1)
	client := f.clientForTab(tab)
	go func() {
		defer f.navWg.Done()
		layer, _, bgColor, sess, finalURL, err := f.loadDocumentCtxWithHistory(ctx, client, u, f.contentWidth(), f.contentHeight(), float32(f.config.dpr), f.downloadDirectory(), true, true, seed, req)
		res := navResult{tabID: tab.ID(), serial: serial, url: u, noHistory: true, traverse: delta, traverseBase: base, traverseScroll: restoreScroll, transition: transition, request: req, requestedURL: u, historySeed: seed}
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

func (f *framePath) handleScriptNavigation(tab *tabs.Tab, request js.NavigationRequest) {
	if tab == nil {
		return
	}
	switch request.Kind {
	case js.NavigationAssign, js.NavigationReplace:
		if request.URL == "" {
			return
		}
		if sameDocumentURL(tab.URL(), request.URL) {
			f.applySameDocumentScriptNavigation(tab, request)
			return
		}
		if request.Kind == js.NavigationReplace {
			f.navigateScriptTabRequest(request.URL, true)
		} else {
			f.navigateScriptTabRequest(request.URL, false)
		}
	case js.NavigationHistoryPush, js.NavigationHistoryReplace:
		f.applyHistoryStateNavigation(tab, request)
	case js.NavigationHistoryTraverse:
		f.traverseTab(request.Delta)
	case js.NavigationReload:
		f.reloadTabForScript()
	case js.NavigationOpenWindow:
		f.openWindowForScript(tab, request)
	case js.NavigationCloseWindow:
		if f.windowContexts != nil {
			f.closeTab(f.windowContexts.tabIDForHandle(request.TargetHandle))
		}
	case js.NavigationFocusWindow:
		if f.windowContexts != nil {
			if id := f.windowContexts.tabIDForHandle(request.TargetHandle); id != 0 {
				f.switchTab(id)
			}
		}
	}
}

func (f *framePath) openWindowForScript(source *tabs.Tab, request js.NavigationRequest) {
	if f.tabMgr == nil || request.URL == "" {
		return
	}
	if !validScriptPopupURL(request.URL) {
		return
	}
	name := request.Target
	if strings.EqualFold(name, "_blank") {
		name = ""
	}
	seed := windowSeed{Handle: request.TargetHandle, Name: name}
	if f.windowContexts != nil {
		if seed.Handle == "" {
			seed.Handle = f.windowContexts.allocateRoot()
		}
	} else if seed.Handle == "" {
		return
	}
	if sess := source.Session(); sess != nil {
		if !request.NoOpener {
			seed.OpenerHandle = sess.WindowHandle()
			seed.OpenerName = sess.WindowName()
		}
		if isAboutBlankURL(request.URL) && !request.NoOpener {
			seed.Origin = sess.WindowOrigin()
		}
	}
	if isAboutBlankURL(request.URL) && !request.NoOpener && seed.Origin == "" {
		seed.Origin = browsingContextOrigin(source.URL())
	}
	seed.Blank = isAboutBlankURL(request.URL)
	trust := f.navigationTrustForTarget(source, request.URL)
	if name != "" && !request.NoOpener {
		if existing := f.findNamedTab(name); existing != nil {
			f.switchTab(existing.ID())
			if f.windowContexts != nil {
				existingHandle := f.windowContexts.handleForTab(existing.ID())
				f.windowContexts.register(seed.Handle, existing.ID(), name)
				f.windowContexts.aliasHandle(existingHandle, seed.Handle)
			}
			f.navigateTabRequestWithTrustAndHistoryMode(request.URL, history.TransitionLink, navRequest{}, trust, false, false, seed)
			return
		}
	}
	f.newTab()
	active := f.tabMgr.Active()
	if active == nil || active == source {
		return
	}
	active.SetName(name)
	if f.windowContexts != nil {
		f.windowContexts.register(seed.Handle, active.ID(), name)
	}
	f.navigateTabRequestWithTrustAndHistoryMode(request.URL, history.TransitionLink, navRequest{}, trust, false, false, seed)
}

func validScriptPopupURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return false
	}
	if isAboutBlankURL(raw) {
		return true
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return false
	}
	return u.Hostname() != ""
}

func (f *framePath) applyHistoryStateNavigation(tab *tabs.Tab, request js.NavigationRequest) {
	scroll := f.currentHistoryScroll(tab)
	tab.History().SetCurrentScroll(scroll)
	if request.Kind == js.NavigationHistoryPush {
		tab.History().PushState(request.URL, navRequest{}, request.State, scroll, true)
	} else {
		tab.History().ReplaceCurrentState(request.URL, navRequest{}, request.State, true)
	}
	tab.SetURL(request.URL)
	urls, _, _, index := tab.History().Snapshot()
	if sess := tab.Session(); sess != nil {
		sess.ApplyHistoryEntry(request.URL, request.State, index, len(urls), 0)
	}
	if f.tabMgr.Active() == tab {
		f.syncTabToToolbar()
		f.publishAccessibility()
	}
	f.saveActiveSession()
}

func sameDocumentURL(current, target string) bool {
	a, errA := url.Parse(current)
	b, errB := url.Parse(target)
	if errA != nil || errB != nil || a == nil || b == nil {
		return false
	}
	if a.EscapedFragment() == b.EscapedFragment() {
		return false
	}
	a.Fragment, a.RawFragment = "", ""
	b.Fragment, b.RawFragment = "", ""
	return a.String() == b.String()
}

func (f *framePath) applySameDocumentScriptNavigation(tab *tabs.Tab, request js.NavigationRequest) {
	tab.SetURL(request.URL)
	if request.Kind == js.NavigationReplace {
		tab.History().ReplaceCurrentRequest(request.URL, toolbar.Request{})
	} else {
		tab.History().Push(request.URL)
	}
	if f.tabMgr.Active() == tab {
		f.syncTabToToolbar()
		f.scrollToAnchor(tab, request.URL)
		f.repaintTab(tab, frame.Rect{})
		f.publishAccessibility()
	}
	f.saveActiveSession()
}

func (f *framePath) reloadTab() {
	f.reloadTabWithTrust(trustedNavigation)
}

func (f *framePath) reloadTabWithTrust(trust navigationTrust) {
	f.reloadTabWithTrustMode(trust, false)
}

func (f *framePath) reloadTabForScript() {
	f.reloadTabWithTrustMode(untrustedNavigation, true)
}

func (f *framePath) reloadTabWithTrustMode(trust navigationTrust, scriptNavigation bool) {
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
	f.navigateTabNoHistoryTraverseRequestWithTrustMode(url, 0, 0, history.TransitionReload, 0, navRequest{}, trust, scriptNavigation)
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
	t.SetHistory(toolbar.RestoreHistoryWithStates(ts.History, ts.HistoryStates, ts.HistorySameDocument, ts.HistoryIndex))
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
		entries, states, sameDocument, idx := tab.History().Snapshot()
		st.Tabs = append(st.Tabs, session.TabState{
			URL:                 tab.URL(),
			Title:               tab.Title(),
			History:             entries,
			HistoryStates:       states,
			HistorySameDocument: sameDocument,
			HistoryIndex:        idx,
			ScrollX:             int(f.sched.Viewport().Offset.X),
			ScrollY:             int(tab.ScrollY()),
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
