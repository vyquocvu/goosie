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
	"github.com/vyquocvu/goosie/internal/manifest"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/session"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// navResult is the outcome of a per-tab navigation, delivered on navResults.
type navResult struct {
	tabID        uint64
	serial       uint64
	layer        *frame.Layer
	bgColor      frame.Color
	session      *engine.Session
	err          error
	url          string
	downloadPath string
	noHistory    bool
	// traverse commits a back/forward step on success: the delta (-1/+1) and
	// the cursor index the peek saw. A failed traversal leaves the cursor
	// where it was instead of stranding it on a page that never loaded.
	traverse     int
	traverseBase int
}

// downloadDone reports a completed download instead of a rendered document;
// the navigation goroutine converts it into navResult.downloadPath.
type downloadDone struct{ path string }

func (e downloadDone) Error() string { return "downloaded to " + e.path }

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
	dev := frame.Size{W: int32(f.config.width), H: int32(f.config.height)}
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

// navigateTab loads a URL on the active tab.
func (f *framePath) navigateTab(rawURL string) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
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
		layer, _, bgColor, sess, finalURL, err := f.loadDocumentCtx(ctx, client, u, f.config.width, f.config.height, float32(f.config.dpr), f.downloadDirectory(), true, true)
		res := navResult{tabID: tab.ID(), serial: serial, url: u}
		if finalURL != "" {
			// A redirect lands the final URL: the address bar and the
			// history record what displayed, not what was typed.
			res.url = finalURL
		}
		var dd downloadDone
		if errors.As(err, &dd) {
			res.downloadPath = dd.path
		} else {
			res.err = err
		}
		res.layer, res.bgColor, res.session = layer, bgColor, sess
		f.navResults <- res
	}()
}

// loadDocumentCtx loads an internal page (goosie://) or a network/file resource.
func (f *framePath) loadDocumentCtx(ctx context.Context, client net.HTTP, u string, viewportW, viewportH int, scale float32, downloadDir string, deferImages, deferFonts bool) (*frame.Layer, paint.SceneSpec, frame.Color, *engine.Session, string, error) {
	if strings.HasPrefix(strings.ToLower(u), "goosie://") {
		return f.renderInternalPage(ctx, u, viewportW, viewportH, scale)
	}
	if client == nil {
		client = f.client
	}
	storage := f.storage
	if client == f.privateClient {
		storage = f.incognitoStorage
	}
	tabID := ""
	if f.tabMgr != nil {
		if active := f.tabMgr.Active(); active != nil {
			tabID = strconv.FormatUint(active.ID(), 10)
		}
	}
	appMode := appearanceMode{dark: f.isDarkMode(), highContrast: f.isHighContrast()}
	extra := []any{f.downloads, storage, tabID, appMode, f.userscripts, f.perms, f.mediaBackend}
	if f.currentPreferences().AskDownloadLocation {
		extra = append(extra, f.window)
	}
	return loadURLCtx(ctx, client, f.fonts, u, viewportW, viewportH, scale, downloadDir, deferImages, deferFonts, extra...)
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
						Size:   f.config.devSize(),
					})
					f.publishAccessibility()
				}
			}
		}
		return
	}

	// Inspect the result document before publishing it to the active tab. Once
	// visible, pointer and script events may mutate its DOM on the chrome thread.
	manifestURL := ""
	if result.session != nil && result.session.Doc != nil && f.client != nil {
		manifestURL = findManifestLink(&result.session.Doc.Node, result.url)
	}

	oldSession := tab.Session()
	tab.SetLayer(result.layer)
	tab.SetSession(result.session)
	tab.SetBGColor(result.bgColor)
	tab.SetURL(result.url)
	tabTitle := result.url
	if strings.HasPrefix(strings.ToLower(result.url), "goosie://") {
		title, _ := f.buildInternalPageHTML(result.url)
		if title != "" {
			tabTitle = title
		}
	}
	tab.SetTitle(tabTitle)
	if oldSession != nil && oldSession != result.session {
		// The replaced document's realm tears down with it.
		oldSession.Close()
	}
	if !result.noHistory {
		tab.History().Push(result.url)
	}
	if result.traverse != 0 {
		// The peeked back/forward step commits only now that its page
		// displayed; a stale commit (another navigation landed first) drops.
		// A redirect rewrites the entry it landed on with the final URL.
		if tab.History().Step(result.traverse, result.traverseBase) {
			tab.History().ReplaceCurrent(result.url)
		}
	}
	if !tab.Incognito() && f.history != nil && !strings.HasPrefix(strings.ToLower(result.url), "goosie://") {
		f.history.Record(result.url, result.url)
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
			// Reloads and traversals keep their offset (per-entry restore
			// is a later feature, not silent loss).
			tab.SetScrollY(0)
			f.sched.SetViewport(frame.Viewport{
				Offset: frame.Point{},
				Size:   f.config.devSize(),
			})

			// If the URL has a fragment (#anchor), scroll to the element
			if href := result.url; strings.Contains(href, "#") {
				f.scrollToAnchor(tab, href)
			}
		}
		f.publishAccessibility()
	}

	// The first frame is out with the document and CSS only; fonts and images
	// arrive on their own goroutines and repaint when they land.
	if sess := result.session; sess != nil {
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
	}
	f.saveActiveSession()
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
	if err := tab.Session().Reflow(float32(f.config.width)); err != nil {
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
	var url string
	var base int
	var ok bool
	if delta < 0 {
		url, base, ok = tab.History().PeekBack()
	} else if delta > 0 {
		url, base, ok = tab.History().PeekForward()
	}
	if !ok || url == "" {
		return
	}
	f.navigateTabNoHistoryTraverse(url, delta, base)
}

// navigateTabNoHistory loads a URL without pushing to history.
func (f *framePath) navigateTabNoHistory(rawURL string) {
	f.navigateTabNoHistoryTraverse(rawURL, 0, 0)
}

// navigateTabNoHistoryTraverse is navigateTabNoHistory carrying a traversal
// commit: on success the history cursor steps by delta from base.
func (f *framePath) navigateTabNoHistoryTraverse(rawURL string, delta, base int) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
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
		layer, _, bgColor, sess, finalURL, err := f.loadDocumentCtx(ctx, client, u, f.config.width, f.config.height, float32(f.config.dpr), f.downloadDirectory(), true, true)
		res := navResult{tabID: tab.ID(), serial: serial, url: u, noHistory: true, traverse: delta, traverseBase: base}
		if finalURL != "" {
			res.url = finalURL
		}
		var dd downloadDone
		if errors.As(err, &dd) {
			res.downloadPath = dd.path
		} else {
			res.err = err
		}
		res.layer, res.bgColor, res.session = layer, bgColor, sess
		f.navResults <- res
	}()
}

// reloadTab re-fetches the active tab's URL.
func (f *framePath) reloadTab() {
	tab := f.tabMgr.Active()
	if tab == nil || tab.URL() == "" {
		return
	}
	f.navigateTabNoHistory(tab.URL())
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
			Size:   f.config.devSize(),
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
	}
}

func applyTabState(t *tabs.Tab, ts session.TabState) {
	t.SetURL(ts.URL)
	t.SetTitle(ts.Title)
	t.SetHistory(toolbar.RestoreHistory(ts.History, ts.HistoryIndex))
	if t.Title() == "" {
		t.SetTitle("New Tab")
	}
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
		})
	}
	st.Active = savedActiveIdx
	return st
}

// saveActiveSession persists the current open tab state to sessionActiveFile.
// This acts as a crash recovery journal: on clean exit the file is deleted. If the
// process crashes or is killed, the file survives so the session can be
// recovered on next launch.
func (f *framePath) saveActiveSession() {
	if f == nil || f.config.private || f.config.paced() || f.config.sessionActiveFile == "" {
		return
	}
	if f.crashedSession != nil {
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
		f.navigateTab(active.URL())
	}
	f.saveActiveSession()
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
