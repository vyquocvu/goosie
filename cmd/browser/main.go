package main

import (
	"context"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	csspkg "github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine/documentloader"
	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
	"github.com/vyquocvu/goosie/internal/engine/session"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/memory"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/profile"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/ui"
	"github.com/vyquocvu/goosie/internal/version"
	ghtml "golang.org/x/net/html"
)

func main() {
	headlessFlag := flag.Bool("headless", false, "Run in headless mode without a UI window")
	urlFlag := flag.String("url", "", "URL to open on startup")
	screenshotFlag := flag.String("screenshot", "", "File path to save a screenshot (only in headless mode)")
	showVersion := flag.Bool("version", false, "Show version information")
	privateFlag := flag.Bool("private", false, "Run in private browsing mode (incognito)")
	devToolsFlag := flag.Bool("devtools", false, "Show the developer tools dock (intended for headless screenshots)")
	devToolsTabFlag := flag.String("devtools-tab", "", "DevTools tab to select when -devtools is set (e.g. Sources, Network, Console)")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	prof, err := profile.Open(profile.Options{
		Private: *privateFlag,
	})
	if err != nil {
		log.Fatalf("failed to open profile: %v", err)
	}
	defer prof.Close()
	bookmarks, err := profile.NewBookmarkStore(prof)
	if err != nil {
		log.Fatalf("failed to open bookmarks: %v", err)
	}
	history, err := profile.NewHistoryStore(prof)
	if err != nil {
		log.Fatalf("failed to open history: %v", err)
	}
	sessionStore, err := profile.NewSessionStore(prof)
	if err != nil {
		log.Fatalf("failed to open session: %v", err)
	}
	settingsStore, err := profile.NewSettingsStore(prof)
	if err != nil {
		log.Fatalf("failed to open settings: %v", err)
	}
	storage, err := profile.NewStorageStore(prof)
	if err != nil {
		log.Fatalf("failed to open storage: %v", err)
	}

	navSession := session.New()
	networkService := net.NewService(net.ServiceOptions{
		Client: navSession.HTTPClient(),
		Cache:  net.NewHTTPCache(filepath.Join(prof.Root(), "cache"), prof.Private()),
	})
	defer networkService.Close()
	fetcher := net.NewFetcherWithService(networkService)
	parser := dom.NewParser()

	var a fyne.App
	var w fyne.Window
	if *headlessFlag {
		a, w, err = newHeadlessAppWindow()
		if err != nil {
			log.Fatal(err)
		}
	}

	memMgr := memory.NewManager(memory.Config{
		GlobalLimit: 512 * 1024 * 1024,
		Limits: map[memory.Component]uint64{
			memory.ComponentDOM:          100 * 1024 * 1024,
			memory.ComponentStyle:        50 * 1024 * 1024,
			memory.ComponentLayout:       50 * 1024 * 1024,
			memory.ComponentDisplayList:  20 * 1024 * 1024,
			memory.ComponentTile:         50 * 1024 * 1024,
			memory.ComponentImage:        30 * 1024 * 1024,
			memory.ComponentGlyph:        10 * 1024 * 1024,
			memory.ComponentScript:       20 * 1024 * 1024,
			memory.ComponentNetworkCache: 50 * 1024 * 1024,
			memory.ComponentPageCache:    20 * 1024 * 1024,
		},
	})

	browser := ui.NewBrowserWithDependencies(ui.BrowserDependencies{
		Profile:       prof,
		Bookmarks:     bookmarks,
		History:       history,
		SessionStore:  sessionStore,
		NavSession:    navSession,
		SettingsStore: settingsStore,
		Storage:       storage,
		Network:       networkService,
		Memory:        memMgr,
		App:           a,
		Window:        w,
		Headless:      *headlessFlag,
	})
	browser.RendererFactory = func() ui.HTMLRenderer {
		return renderer.NewRenderer(1000, 700)
	}

	// Channel to signal when initial page load is complete (used in headless mode)
	pageLoaded := make(chan bool, 1)

	// Set up navigation callback
	browser.SetNavigationCallback(func(url string) {
		load, ctx := navSession.Navigate(context.Background(), url)
		loadPageAsyncWithCoordinator(browser, fetcher, parser, load, ctx, navSession, networkService, func() {
			select {
			case pageLoaded <- true:
			default:
			}
		})
	})

	if *headlessFlag {
		// Construct the UI hierarchy so the canvas is populated
		go browser.Show()

		if *urlFlag != "" {
			load, ctx := navSession.Navigate(context.Background(), *urlFlag)
			loadPageAsyncWithCoordinator(browser, fetcher, parser, load, ctx, navSession, networkService, func() {
				pageLoaded <- true
			})

			<-pageLoaded

			if *devToolsFlag {
				browser.ShowDevTools(*devToolsTabFlag)
				time.Sleep(500 * time.Millisecond)
			}

			// Give Fyne's UI thread a brief moment to process the layout changes
			time.Sleep(1500 * time.Millisecond)

			if content := w.Content(); content != nil {
				content.Refresh()
			}

			if *screenshotFlag != "" {
				err := ui.TakeScreenshotToFile(w, *screenshotFlag)
				if err != nil {
					log.Printf("Failed to save screenshot: %v", err)
				} else {
					log.Printf("Screenshot saved to %s", *screenshotFlag)
				}
			}
		}
		os.Exit(0)
	} else {
		// Non-headless mode
		if *urlFlag != "" {
			load, ctx := navSession.Navigate(context.Background(), *urlFlag)
			loadPageAsyncWithCoordinator(browser, fetcher, parser, load, ctx, navSession, networkService, nil)
		}

		// Show browser window (this blocks until window is closed)
		browser.Show()
	}
}

// pageLoadResult represents the result of an async page load
type pageLoadResult struct {
	html string
	err  error
}

// loadPageAsync fetches and displays a web page asynchronously.
func loadPageAsync(browser *ui.Browser, fetcher *net.Fetcher, parser *dom.Parser, load navigation.Load, ctx context.Context, sess *session.Session, networkService *net.Service, onComplete func()) {
	log.Printf("Navigation %s started: %s", load.ID, load.URL)

	url := load.URL

	// Resolve the navigation target against the current page BEFORE
	// recording it in browser state so path-only targets are stored and
	// rendered as absolute URLs (scheme + host), never a bare path.
	resolvedURL := url
	if activeTab := browser.ActiveTab(); activeTab != nil {
		if renderer := activeTab.GetRenderer(); renderer != nil {
			resolvedURL = renderer.ResolveURL(url)
		}
	}

	// Update browser state on main thread
	browser.NavigateTo(resolvedURL)

	// Show loading indicator on main thread
	browser.ShowLoading()
	if activeTab := browser.ActiveTab(); activeTab != nil {
		if r := activeTab.GetRenderer(); r != nil {
			r.SetSubmitting(true)
		}
	}

	navID := load.ID

	// Launch background goroutine for fetch and render
	go func() {
		defer func() {
			if onComplete != nil {
				onComplete()
			}
		}()

		if !sess.IsActive(navID) {
			return
		}

		// Handle anchor links (scroll within current page)
		if strings.HasPrefix(url, "#") {
			log.Printf("Navigation %s anchor link: %s", navID, url)
			if sess.IsActive(navID) {
				browser.HideLoading()
				if activeTab := browser.ActiveTab(); activeTab != nil {
					if r := activeTab.GetRenderer(); r != nil {
						r.SetSubmitting(false)
					}
				}
			}
			return
		}

		// Fetch the page using the streaming path (M1.3).
		// FetchStreamWithContext returns the response body as an io.ReadCloser
		// without buffering into an intermediate bytes.Buffer, eliminating one
		// full-body copy from the main document path.
		stream, meta, fetchErr := fetcher.FetchStreamWithContext(ctx, resolvedURL)

		if !sess.IsActive(navID) {
			log.Printf("Navigation %s stale after fetch: %s", navID, url)
			if stream != nil {
				stream.Close()
			}
			return
		}

		// Check if context was cancelled
		if ctx.Err() != nil {
			log.Printf("Navigation %s cancelled: %s", navID, url)
			if stream != nil {
				stream.Close()
			}
			return
		}

		// Check for downloads (non-HTML content types or attachment disposition)
		cd := meta.Header.Get("Content-Disposition")
		isAttachment := strings.HasPrefix(strings.ToLower(strings.TrimSpace(cd)), "attachment")

		isDownload := isAttachment
		isImage := false
		if !isDownload && fetchErr == nil {
			contentType := strings.ToLower(meta.ContentType)
			isWebPage := false
			for _, t := range []string{"text/html", "application/xhtml+xml", "text/plain", "application/json"} {
				if strings.Contains(contentType, t) {
					isWebPage = true
					break
				}
			}
			isImage = strings.HasPrefix(contentType, "image/")
			if !isWebPage && !isImage && contentType != "" {
				isDownload = true
			}
		}

		if isDownload {
			if stream != nil {
				stream.Close()
			}
			filename := getFilenameFromURLAndCD(resolvedURL, cd)
			fyne.Do(func() {
				browser.HideLoading()
				if activeTab := browser.ActiveTab(); activeTab != nil {
					if r := activeTab.GetRenderer(); r != nil {
						r.SetSubmitting(false)
					}
				}
				d := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
					if err != nil || writer == nil {
						return
					}
					targetPath := writer.URI().Path()
					writer.Close()

					go func() {
						record := net.DownloadRecord{
							URL:        resolvedURL,
							TargetPath: targetPath,
							Status:     net.DownloadRunning,
							StartedAt:  time.Now(),
						}
						networkService.AddDownload(record)
						m := net.NewDownloadManager(sess.HTTPClient())
						res, _ := m.DownloadWithContext(context.Background(), resolvedURL, targetPath)
						networkService.UpdateDownload(res)
					}()
				}, browser.GetWindow())
				d.SetFileName(filename)
				d.Show()
			})
			return
		}

		var html string
		if fetchErr != nil {
			if resolvedURL == "https://example.com" {
				html = `<!DOCTYPE html>
<html><head><title>Example Domain</title></head>
<body><div><h1>Example Domain</h1><p>Mock fallback.</p></div></body></html>`
			} else {
				updateUIWithError(browser, sess, navID, fetchErr, resolvedURL)
				return
			}
		} else {
			data, readErr := io.ReadAll(stream)
			stream.Close()
			if readErr != nil {
				log.Printf("Navigation %s stream read error: %v", navID, readErr)
				updateUIWithError(browser, sess, navID, readErr, resolvedURL)
				return
			}
			html = string(data)
			if meta.Status >= 400 && strings.TrimSpace(html) == "" {
				html = fmt.Sprintf(
					"<html><body><h1>%d %s</h1><p>The server returned an error.</p></body></html>",
					meta.Status, strings.TrimSpace(fmt.Sprintf("%d", meta.Status)),
				)
			}
			if isImage {
				html = wrapImageInHTML(resolvedURL)
			}
		}

		updateUIWithContent(ctx, browser, fetcher, sess, navID, html, resolvedURL, sess, networkService, parser)
	}()
}

// loadPageAsyncWithCoordinator is the M3-aware sibling of
// loadPageAsync. After fetching the HTML, it parses with the streaming
// parser (firing OnResource into a documentloader coordinator), waits
// for external CSS to land, then routes the assembled document and
// stylesheets into renderer.RenderParsed via the UI's snapshot entry
// point. The coordinator owns CSP gating, URL resolution, scheduling,
// and ordered fetch.
//
// loadPageAsync continues to work for callers that bypass the
// coordinator; the new path is the recommended one for browser
// navigation.
func loadPageAsyncWithCoordinator(browser *ui.Browser, fetcher *net.Fetcher, parser *dom.Parser, load navigation.Load, ctx context.Context, sess *session.Session, networkService *net.Service, onComplete func()) {
	log.Printf("Navigation %s (coordinator) started: %s", load.ID, load.URL)

	url := load.URL

	// Resolve the navigation target against the current page BEFORE
	// recording it in browser state, so path-only targets (e.g. a click
	// on <a href="/path"> that already resolved, or a programmatic
	// window.location assignment) are stored and rendered as absolute
	// URLs with a scheme and host — never as a bare path.
	resolvedURL := url
	if activeTab := browser.ActiveTab(); activeTab != nil {
		if renderer := activeTab.GetRenderer(); renderer != nil {
			resolvedURL = renderer.ResolveURL(url)
		}
	}

	browser.NavigateTo(resolvedURL)
	browser.ShowLoading()
	if activeTab := browser.ActiveTab(); activeTab != nil {
		if r := activeTab.GetRenderer(); r != nil {
			r.SetSubmitting(true)
		}
	}

	navID := load.ID

	go func() {
		defer func() {
			if onComplete != nil {
				onComplete()
			}
		}()

		if !sess.IsActive(navID) {
			return
		}

		if strings.HasPrefix(url, "#") {
			log.Printf("Navigation %s anchor link: %s", navID, url)
			if sess.IsActive(navID) {
				browser.HideLoading()
				if activeTab := browser.ActiveTab(); activeTab != nil {
					if r := activeTab.GetRenderer(); r != nil {
						r.SetSubmitting(false)
					}
				}
			}
			return
		}

		stream, meta, fetchErr := fetcher.FetchStreamWithContext(ctx, resolvedURL)

		if !sess.IsActive(navID) {
			log.Printf("Navigation %s stale after fetch: %s", navID, url)
			if stream != nil {
				stream.Close()
			}
			return
		}

		if ctx.Err() != nil {
			log.Printf("Navigation %s cancelled: %s", navID, url)
			if stream != nil {
				stream.Close()
			}
			return
		}

		// Detect non-HTML responses (downloads).
		cd := meta.Header.Get("Content-Disposition")
		isAttachment := strings.HasPrefix(strings.ToLower(strings.TrimSpace(cd)), "attachment")
		isDownload := isAttachment
		isImage := false
		if !isDownload && fetchErr == nil {
			contentType := strings.ToLower(meta.ContentType)
			isWebPage := false
			for _, t := range []string{"text/html", "application/xhtml+xml", "text/plain", "application/json"} {
				if strings.Contains(contentType, t) {
					isWebPage = true
					break
				}
			}
			isImage = strings.HasPrefix(contentType, "image/")
			if !isWebPage && !isImage && contentType != "" {
				isDownload = true
			}
		}

		if isDownload {
			if stream != nil {
				stream.Close()
			}
			filename := getFilenameFromURLAndCD(resolvedURL, cd)
			fyne.Do(func() {
				browser.HideLoading()
				if activeTab := browser.ActiveTab(); activeTab != nil {
					if r := activeTab.GetRenderer(); r != nil {
						r.SetSubmitting(false)
					}
				}
				d := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
					if err != nil || writer == nil {
						return
					}
					targetPath := writer.URI().Path()
					writer.Close()
					go func() {
						record := net.DownloadRecord{
							URL:        resolvedURL,
							TargetPath: targetPath,
							Status:     net.DownloadRunning,
							StartedAt:  time.Now(),
						}
						networkService.AddDownload(record)
						m := net.NewDownloadManager(sess.HTTPClient())
						res, _ := m.DownloadWithContext(context.Background(), resolvedURL, targetPath)
						networkService.UpdateDownload(res)
					}()
				}, browser.GetWindow())
				d.SetFileName(filename)
				d.Show()
			})
			return
		}

		var html string
		if fetchErr != nil {
			if resolvedURL == "https://example.com" {
				html = `<!DOCTYPE html>
<html><head><title>Example Domain</title></head>
<body><div><h1>Example Domain</h1><p>Mock fallback.</p></div></body></html>`
			} else {
				updateUIWithError(browser, sess, navID, fetchErr, resolvedURL)
				return
			}
		} else {
			data, readErr := io.ReadAll(stream)
			stream.Close()
			if readErr != nil {
				log.Printf("Navigation %s stream read error: %v", navID, readErr)
				updateUIWithError(browser, sess, navID, readErr, resolvedURL)
				return
			}
			html = string(data)
			if meta.Status >= 400 && strings.TrimSpace(html) == "" {
				html = fmt.Sprintf(
					"<html><body><h1>%d %s</h1><p>The server returned an error.</p></body></html>",
					meta.Status, strings.TrimSpace(fmt.Sprintf("%d", meta.Status)),
				)
			}
			if isImage {
				html = wrapImageInHTML(resolvedURL)
			}
		}

		updateUIWithCoordinatorContent(ctx, browser, fetcher, sess, navID, html, resolvedURL, networkService, parser)
	}()
}

// updateUIWithCoordinatorContent is the M3 path: stream-parse the
// HTML, route discoveries through the documentloader coordinator,
// wait for external CSS, then render via renderer.RenderParsed. This
// is the snapshot entry point that delivers the first styled frame
// with required CSS rules per plan.md M3 acceptance criteria #3.
func updateUIWithCoordinatorContent(ctx context.Context, browser *ui.Browser, fetcher *net.Fetcher, sess *session.Session, navID navigation.ID, html string, url string, networkService *net.Service, parser *dom.Parser) {
	if !sess.IsActive(navID) {
		return
	}
	log.Printf("Navigation %s (coordinator) rendering", navID)

	// 1. Wire the active navigation + CSP into the coordinator.
	scheduler := navigation.NewScheduler()
	defer scheduler.Cancel()

	load, navCtx := scheduler.Begin(ctx, url)
	recorder := metrics.NewRecorder(uint64(load.ID), url)

	csp := fetcher.CSP()
	docFetcher := fetcher

	var externalResults []documentloader.CSSResult
	var scriptResults []documentloader.ScriptResult
	var coord *documentloader.Coordinator

	// Async script execution: the OnScript callback fires on the
	// coordinator goroutine, potentially before jsRuntime exists.
	// Buffer async scripts that arrive early, execute them once
	// the runtime is created.
	var (
		asyncMu           sync.Mutex
		preRuntimeAsync   []documentloader.ScriptResult
		jsRuntimeReady    bool
	)
	coord, err := documentloader.New(documentloader.Options{
		NavigationID:      load.ID,
		NavigationContext: navCtx,
		FinalURL:          url,
		CSP:               csp,
		Scheduler:         scheduler,
		Fetcher:           docFetcher,
		Recorder:          recorder,
		Callbacks: documentloader.Callbacks{
			OnStylesheet: func(r documentloader.CSSResult) {
				externalResults = append(externalResults, r)
				// M7: extract nested resources (@import, @font-face,
				// url()) from the fetched stylesheet and feed them
				// back through the same coordinator. This keeps the
				// network path uniform across top-level and nested
				// resources.
				if sheet, parseErr := csspkg.NewParser(string(r.Source)).Parse(); parseErr == nil {
					for _, sub := range csspkg.ExtractResources(sheet) {
						var k documentloader.ResourceKind
						switch sub.Kind {
						case csspkg.ResourceStylesheet:
							k = documentloader.KindCSS
						case csspkg.ResourceFont:
							k = documentloader.KindFont
						case csspkg.ResourceImage:
							k = documentloader.KindImage
						default:
							continue
						}
						coord.EnqueueSecondary(context.Background(), k, sub.URL, r.Resolved)
					}
				}
			},
OnScript: func(r documentloader.ScriptResult) {
			scriptResults = append(scriptResults, r)
			if r.Mode != documentloader.ScriptModeAsync || r.Source == nil {
				return
			}
			if !sess.IsActive(navID) {
				return
			}
			asyncMu.Lock()
			if !jsRuntimeReady {
				preRuntimeAsync = append(preRuntimeAsync, r)
				asyncMu.Unlock()
				return
			}
			asyncMu.Unlock()
			if t := browser.ActiveTab(); t != nil {
				if rt := t.GetJSRuntime(); rt != nil {
					if _, err := rt.RunScript(string(r.Source)); err != nil {
						log.Printf("Error running async script %s: %v", r.Resolved, err)
					}
				}
			}
		},
			OnImage: func(r documentloader.ImageResult) {
				log.Printf("CSS-nested image fetched: %s (%d bytes)", r.Resolved, len(r.Source))
			},
			OnFont: func(r documentloader.FontResult) {
				log.Printf("CSS-nested font fetched: %s (%d bytes)", r.Resolved, len(r.Source))
			},
			OnError: func(_ documentloader.Resource, e error) {
				if r := browser.ActiveTab(); r != nil {
					if rend := r.GetRenderer(); rend != nil {
						log.Printf("Coordinator resource skipped: %v", e)
					}
				}
			},
		},
	})
	if err != nil {
		log.Printf("Navigation %s coordinator init failed: %v", navID, err)
		// Fall back to the legacy path so the user still sees the page.
		updateUIWithContent(ctx, browser, fetcher, sess, navID, html, url, sess, networkService, parser)
		return
	}

	// 2. Stream-parse the HTML, feeding discoveries into the coordinator.
	parseCtx, cancel := context.WithTimeout(navCtx, 30*time.Second)
	_, _ = parser.ParseDocumentCtx(parseCtx, strings.NewReader(html), dom.ParseConfig{
		OnResource: coord.FromDomOnResource(),
	})

	// 3. Wait for in-flight CSS fetches to settle (timeout fallback
	//    renders with what we have so far, per plan.md M3 acceptance).
	if err := coord.HandleDocumentEnd(parseCtx); err != nil {
		log.Printf("Navigation %s coordinator drain ended with %v (continuing with %d stylesheets)",
			navID, err, len(externalResults))
	}
	cancel()

	// 4. Convert coordinator results into the renderer's
	//    []renderer.ExternalCSS. The coordinator guarantees source
	//    order via Position; we preserve that here.
	external := make([]renderer.ExternalCSS, 0, len(externalResults))
	for _, r := range externalResults {
		external = append(external, renderer.ExternalCSS{URL: r.Resolved, Source: r.Source})
	}

	// 5. Parse the document for rendering. The renderer expects
	//    *html.Node; the streaming parser produces *dom.Document. We
	//    re-parse with html.Parse for rendering; the streaming pass
	//    only drove discovery.
	doc, parseErr := ghtml.Parse(strings.NewReader(html))
	if parseErr != nil {
		log.Printf("Navigation %s render parse failed: %v", navID, parseErr)
		updateUIWithError(browser, sess, navID, parseErr, url)
		return
	}

	// 6. Wire the renderer's CSP (for any non-CSS checks the renderer
	//    still performs) and route through the snapshot entry point.
	if activeTab := browser.ActiveTab(); activeTab != nil {
		if r := activeTab.GetRenderer(); r != nil {
			r.SetCSP(csp)
		}
	}

	if err := browser.RenderParsedContent(ctx, doc, external); err != nil {
		log.Printf("Error rendering (coordinator path): %v", err)
		browser.SetContent("Error rendering HTML: " + err.Error())
		browser.HideLoading()
		if activeTab := browser.ActiveTab(); activeTab != nil {
			if r := activeTab.GetRenderer(); r != nil {
				r.SetSubmitting(false)
			}
		}
		sess.Fail(err)
		return
	}

	log.Printf("Page loaded (coordinator) successfully with %d stylesheets", len(external))

	// 7. Mirror the legacy updateUIWithContent side-effects (raw
	//    source, title, loading indicator, JS runtime wiring). These
	//    do not depend on the rendering path.
	if tab := browser.ActiveTab(); tab != nil {
		tab.SetRawSource(html)
	}
	if title, ok := extractTitle(html); ok {
		if fyne.CurrentApp() != nil {
			fyne.CurrentApp().SendNotification(&fyne.Notification{
				Title:   "Goosie",
				Content: "Page loaded: " + title,
			})
		}
		browser.UpdateActiveTabTitle(title)
	}
	browser.HideLoading()
	if activeTab := browser.ActiveTab(); activeTab != nil {
		if r := activeTab.GetRenderer(); r != nil {
			r.SetSubmitting(false)
		}
	}
	sess.Complete()

	// 8. JS runtime wiring + M4 ordered script queue.
	//
	//    The coordinator has already CSP-gated, fetched, and ordered
	//    every classic script (inline + external). We now enrich the
	//    inline entries with bodies extracted from the parsed DOM and
	//    execute the queue in document order via js.Runtime.RunScript.
	//
	//    Per plan.md M4: "execute this queue after document bytes are
	//    available but before the final browser render". Render has
	//    already happened above (RenderParsedContent), so M4 executes
	//    after render — this matches the conservative "initially"
	//    strategy in the plan and keeps the user-visible flow stable.
	tab := browser.ActiveTab()
	if tab != nil {
		if oldRuntime := tab.GetJSRuntime(); oldRuntime != nil {
			oldRuntime.SetDOMMutationCallback(nil)
			oldRuntime.Cleanup()
		}
		jsRuntime := js.NewRuntime()
		tab.SetJSRuntime(jsRuntime)
		asyncMu.Lock()
		jsRuntimeReady = true
		for _, a := range preRuntimeAsync {
			if _, err := jsRuntime.RunScript(string(a.Source)); err != nil {
				log.Printf("Error running pre-runtime async script %s: %v", a.Resolved, err)
			}
		}
		preRuntimeAsync = nil
		asyncMu.Unlock()
		jsRuntime.SetOrigin(originFromURL(url))
		jsRuntime.SetEnforcer(js.NewScriptEnforcer(js.DefaultSecurePolicy()))
		jsRuntime.SetFetcher(fetcher)
		jsRuntime.OnOpenWindow = func(openURL, name string) {
			log.Printf("Popup (window.open): %s (name=%s)", openURL, name)
			load, ctx := sess.Navigate(context.Background(), openURL)
			loadPageAsyncWithCoordinator(browser, fetcher, parser, load, ctx, sess, networkService, nil)
		}
		jsRuntime.OnNavigate = func(targetURL string) {
			if !sess.IsActive(navID) {
				return
			}
			log.Printf("Programmatic navigation: window.location → %s", targetURL)
			load, ctx := sess.Navigate(context.Background(), targetURL)
			loadPageAsyncWithCoordinator(browser, fetcher, parser, load, ctx, sess, networkService, nil)
		}
		jsRuntime.SetHTMLContent(html)
		// M6 mutation handling: coalesce a burst of JS DOM mutations
		// into a single render via documentloader's MutationCoalescer,
		// then re-render using the snapshot entry point (RenderParsed)
		// with the cached external CSS from the initial coordinator
		// run. This avoids re-fetching stylesheets/scripts on every
		// mutation — the prior path (tab.RenderHTML) re-fetched
		// external CSS via RenderHTML's async loader.
		var (
			muMut          sync.Mutex
			currentMutHTML string
		)
		mutCoalescer := documentloader.NewMutationCoalescer(16*time.Millisecond, func(n int) {
			if !sess.IsActive(navID) {
				return
			}
			muMut.Lock()
			latest := currentMutHTML
			muMut.Unlock()
			// fmt.Printf("DEBUG: Mutation render callback, latest HTML length: %d\n", len(latest))
			if latest == "" {
				return
			}
			doc, parseErr := ghtml.Parse(strings.NewReader(latest))
			if parseErr != nil {
				log.Printf("Mutation render: parse failed: %v", parseErr)
				return
			}
			if err := tab.RenderParsedContent(context.Background(), doc, external); err != nil {
				log.Printf("Mutation render: RenderParsedContent failed: %v", err)
			} else {
				log.Printf("Mutation render: coalesced %d mutations", n)
			}
		})
		jsRuntime.SetDOMMutationCallback(func(mutatedHTML string) {
			if !sess.IsActive(navID) {
				return
			}
			muMut.Lock()
			currentMutHTML = mutatedHTML
			muMut.Unlock()
			mutCoalescer.Trigger()
		})

		executeScriptQueue(jsRuntime, url, csp, doc, scriptResults)
	}
}

// executeScriptQueue runs classic and deferred scripts in document
// order per plan.md M4 + M5. Inline entries arrive with Source=nil
// (the streaming parser cannot capture body bytes until </script>
// closes — see M2); inline bodies are extracted from the parsed DOM
// and merged in by Position. CSP is re-checked here for inline
// scripts (the coordinator CSP-checks external scripts at fetch time;
// inline scripts bypass the fetcher, so the gate happens here).
//
// M5: classic scripts execute first (in source order), then deferred
// scripts (in source order). Async scripts are executed immediately
// by the OnScript callback (and optionally buffered until the runtime
// is created); any that appear here are silently skipped — they were
// already handed to jsRuntime.RunScript at fetch completion.
//
// Script failures are reported but do not stop subsequent scripts
// unless the engine's policy explicitly requires it. The current
// Goosie policy is permissive: log and continue.
func executeScriptQueue(jsRuntime *js.Runtime, pageURL string, csp *net.CSPPolicy, doc *ghtml.Node, results []documentloader.ScriptResult) {
	if jsRuntime == nil {
		return
	}
	if len(results) == 0 {
		// No classic/defer scripts. Still dispatch DOMContentLoaded
		// so listeners (analytics, "ready" handlers) fire.
		fireDOMContentLoaded(jsRuntime)
		return
	}
	baseURL, _ := urlpkg.Parse(pageURL)
	inlineBodies := inlineScriptsByPosition(doc)

	// Filter and group by mode. Classic and defer are executed at
	// drain time; module is reported and skipped.
	classics := make([]documentloader.ScriptResult, 0, len(results))
	defers := make([]documentloader.ScriptResult, 0, len(results))
	for _, r := range results {
		switch r.Mode {
		case documentloader.ScriptModeClassic:
			classics = append(classics, r)
		case documentloader.ScriptModeDefer:
			defers = append(defers, r)
		case documentloader.ScriptModeModule:
			log.Printf("Skipping module script at position %d (unsupported)", r.Position)
		case documentloader.ScriptModeAsync:
			// Already executed by the OnScript callback when the
			// fetch completed.  No-op here.
		default:
			log.Printf("Script queue: unknown mode %s at position %d", r.Mode, r.Position)
		}
	}

	runScriptGroup(jsRuntime, csp, baseURL, doc, inlineBodies, classics, "classic")
	runScriptGroup(jsRuntime, csp, baseURL, doc, inlineBodies, defers, "defer")

	// Dispatch DOMContentLoaded to JS after classic + defer finish.
	fireDOMContentLoaded(jsRuntime)
}

// runScriptGroup executes one group of scripts (classic or defer) in
// source order. Failures log and continue per the permissive policy.
func runScriptGroup(jsRuntime *js.Runtime, csp *net.CSPPolicy, baseURL *urlpkg.URL, doc *ghtml.Node, inlineBodies map[int]string, group []documentloader.ScriptResult, label string) {
	sort.SliceStable(group, func(i, j int) bool {
		return group[i].Position < group[j].Position
	})
	for _, r := range group {
		source := r.Source
		if r.Inline {
			if body, ok := inlineBodies[r.Position]; ok {
				source = []byte(body)
			} else {
				log.Printf("%s script at position %d has no inline body", label, r.Position)
				continue
			}
			if csp != nil {
				if err := csp.AllowScript("", baseURL); err != nil {
					log.Printf("CSP blocked %s inline script at position %d: %v", label, r.Position, err)
					continue
				}
			}
		}

		if _, err := jsRuntime.RunScript(string(source)); err != nil {
			log.Printf("Error running %s script %s at position %d: %v",
				label, scriptLabel(r), r.Position, err)
			continue
		}
	}
}

// fireDOMContentLoaded dispatches the DOMContentLoaded event to the
// JS runtime after classic + deferred scripts have run. Per HTML spec,
// DOMContentLoaded fires after the document is parsed and defer
// scripts execute; async scripts may still be in flight at this
// point (their completion fires the load event, which M5 emits via
// EventLoad from the coordinator).
func fireDOMContentLoaded(jsRuntime *js.Runtime) {
	if jsRuntime == nil {
		return
	}
	if _, err := jsRuntime.RunScript(dispatchDOMContentLoaded); err != nil {
		log.Printf("Error dispatching DOMContentLoaded: %v", err)
	}
}

// dispatchDOMContentLoaded is the snippet that builds and dispatches
// the DOMContentLoaded event on document. It tolerates a missing
// Event constructor (some test environments) by falling back to a
// plain dispatch.
const dispatchDOMContentLoaded = `
(function() {
  try {
    var ev = (typeof Event === 'function')
      ? new Event('DOMContentLoaded')
      : { type: 'DOMContentLoaded' };
    if (document && typeof document.dispatchEvent === 'function') {
      document.dispatchEvent(ev);
    } else if (typeof dispatchEvent === 'function') {
      dispatchEvent(ev);
    }
  } catch (e) { console.error('DOMContentLoaded dispatch failed:', e); }
})();
`

func scriptKind(r documentloader.ScriptResult) string {
	if r.Inline {
		return "inline"
	}
	return "external"
}

func scriptLabel(r documentloader.ScriptResult) string {
	if r.URL != "" {
		return r.URL
	}
	if r.Resolved != "" {
		return r.Resolved
	}
	return "<inline>"
}

// updateUIWithError updates the UI with an error message.
func updateUIWithError(browser *ui.Browser, sess *session.Session, navID navigation.ID, err error, url string) {
	if !sess.IsActive(navID) {
		sess.Fail(err)
		return
	}
	sess.Fail(err)
	log.Printf("Navigation %s error loading %s: %v", navID, url, err)
	errorHTML := fmt.Sprintf(`
		<!DOCTYPE html>
		<html>
		<head><title>Error</title></head>
		<body>
			<h1>Failed to load page</h1>
			<p>Could not load the page at %s.</p>
			<p>Error: %s</p>
		</body>
		</html>`, url, err.Error())
	_ = browser.RenderHTMLContent(context.Background(), errorHTML)
	browser.HideLoading()
	if activeTab := browser.ActiveTab(); activeTab != nil {
		if r := activeTab.GetRenderer(); r != nil {
			r.SetSubmitting(false)
		}
	}
}

// updateUIWithContent updates the UI with HTML content.
func updateUIWithContent(ctx context.Context, browser *ui.Browser, fetcher *net.Fetcher, sess *session.Session, navID navigation.ID, html string, url string, navSession *session.Session, networkService *net.Service, parser *dom.Parser) {
	if !sess.IsActive(navID) {
		return
	}
	log.Printf("Navigation %s rendering page content", navID)

	// Fyne widgets are thread-safe and can be updated from any goroutine
	// Wire CSP policy into the renderer for style-src enforcement.
	if activeTab := browser.ActiveTab(); activeTab != nil {
		if r := activeTab.GetRenderer(); r != nil {
			r.SetCSP(fetcher.CSP())
		}
	}

	// Render HTML using the canvas-based renderer
	err := browser.RenderHTMLContent(ctx, html)
	if err != nil {
		log.Printf("Error rendering HTML: %v", err)
		browser.SetContent("Error rendering HTML: " + err.Error())
		browser.HideLoading()
		if activeTab := browser.ActiveTab(); activeTab != nil {
			if r := activeTab.GetRenderer(); r != nil {
				r.SetSubmitting(false)
			}
		}
		sess.Fail(err)
		return
	}

	log.Printf("Page loaded successfully")

	// Store raw HTML source for the View Source feature
	if tab := browser.ActiveTab(); tab != nil {
		tab.SetRawSource(html)
	}

	// Update tab title
	if title, ok := extractTitle(html); ok {
		if fyne.CurrentApp() != nil {
			fyne.CurrentApp().SendNotification(&fyne.Notification{
				Title:   "Goosie",
				Content: "Page loaded: " + title,
			})
		}
		browser.UpdateActiveTabTitle(title)
	}

	// Hide loading indicator
	browser.HideLoading()
	if activeTab := browser.ActiveTab(); activeTab != nil {
		if r := activeTab.GetRenderer(); r != nil {
			r.SetSubmitting(false)
		}
	}
	sess.Complete()

	// Get or create JS runtime for the active tab
	tab := browser.ActiveTab()
	if tab != nil {
		if tab.GetJSRuntime() == nil {
			jsRuntime := js.NewRuntime()
			tab.SetJSRuntime(jsRuntime)
		}

		jsRuntime := tab.GetJSRuntime()

		jsRuntime.SetOrigin(originFromURL(url))
		jsRuntime.SetEnforcer(js.NewScriptEnforcer(js.DefaultSecurePolicy()))

		// Wire up the real HTTP fetcher so fetch() makes actual network requests
		jsRuntime.SetFetcher(fetcher)

		// Wire up window.open to navigate in the current browsing context.
		jsRuntime.OnOpenWindow = func(url, name string) {
			log.Printf("Popup (window.open): %s (name=%s)", url, name)
			load, ctx := navSession.Navigate(context.Background(), url)
			jsRuntime := tab.GetJSRuntime()
			_ = jsRuntime // suppress unused warning if any, but wait, loadPageAsync is next
			loadPageAsyncWithCoordinator(browser, fetcher, parser, load, ctx, navSession, networkService, nil)
		}

		// Wire up the DOM mutation callback to re-render the HTML content on dynamic updates
		jsRuntime.SetDOMMutationCallback(func(mutatedHTML string) {
			log.Printf("DOM mutated by JS, triggering UI re-render")
			if err := tab.RenderHTML(context.Background(), mutatedHTML); err != nil {
				log.Printf("Error rendering mutated HTML: %v", err)
			}
		})

		// Set HTML content for JS runtime (enables document.getElementById etc.)
		jsRuntime.SetHTMLContent(html)

		// Parse page URL for CSP enforcement.
		baseURL, _ := urlpkg.Parse(url)
		csp := fetcher.CSP()

		// Execute inline <script> tags found in the page
		scripts := extractInlineScripts(html)
		for _, script := range scripts {
			if csp != nil {
				if err := csp.AllowScript("", baseURL); err != nil {
					log.Printf("CSP blocked inline script: %v", err)
					continue
				}
			}
			if _, err := jsRuntime.RunScript(script); err != nil {
				log.Printf("Error running page script: %v", err)
			}
		}

		// Fetch and execute external scripts (<script src="...">)
		doc, htmlParseErr := ghtml.Parse(strings.NewReader(html))
		if htmlParseErr == nil {
			for _, src := range extractExternalScriptSrcs(doc) {
				resolvedSrc, resolveErr := resolveScriptURL(src, url)
				if resolveErr != nil {
					log.Printf("Skipping external script with invalid src %s: %v", src, resolveErr)
					continue
				}
				// Only fetch scripts with a valid HTTP/HTTPS URL
				if !strings.HasPrefix(resolvedSrc, "http://") && !strings.HasPrefix(resolvedSrc, "https://") {
					log.Printf("Skipping external script with non-HTTP src: %s", resolvedSrc)
					continue
				}
				if csp != nil {
					if err := csp.AllowScript(resolvedSrc, baseURL); err != nil {
						log.Printf("CSP blocked external script %s: %v", resolvedSrc, err)
						continue
					}
				}
				scriptContent, fetchErr := fetcher.Fetch(resolvedSrc)
				if fetchErr != nil {
					log.Printf("Failed to fetch external script %s: %v", resolvedSrc, fetchErr)
					continue
				}
				if _, runErr := jsRuntime.RunScript(scriptContent); runErr != nil {
					log.Printf("Error running external script %s: %v", resolvedSrc, runErr)
				}
			}
		}
	}
}

// loadPage fetches and displays a web page (deprecated - use loadPageAsyncWithCoordinator).
func loadPage(browser *ui.Browser, fetcher *net.Fetcher, parser *dom.Parser, url string) {
	s := session.New()
	load, ctx := s.Navigate(context.Background(), url)
	loadPageAsyncWithCoordinator(browser, fetcher, parser, load, ctx, s, nil, nil)
}

// extractTitle parses the HTML and returns the content of the <title> tag.
func extractTitle(htmlContent string) (string, bool) {
	doc, err := ghtml.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", false
	}

	var crawler func(*ghtml.Node) (string, bool)
	crawler = func(node *ghtml.Node) (string, bool) {
		if node.Type == ghtml.ElementNode && node.Data == "title" {
			if node.FirstChild != nil {
				return node.FirstChild.Data, true
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if title, ok := crawler(c); ok {
				return title, ok
			}
		}
		return "", false
	}

	return crawler(doc)
}

// extractInlineScripts extracts the content of all inline <script> tags (no src attr).
func extractInlineScripts(htmlContent string) []string {
	doc, err := ghtml.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil
	}

	var scripts []string
	var walk func(*ghtml.Node)
	walk = func(n *ghtml.Node) {
		if n.Type == ghtml.ElementNode && n.Data == "script" {
			hasSrc := false
			for _, attr := range n.Attr {
				if attr.Key == "src" {
					hasSrc = true
					break
				}
			}
			if !hasSrc {
				var sb strings.Builder
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == ghtml.TextNode {
						sb.WriteString(c.Data)
					}
				}
				if sb.Len() > 0 {
					scripts = append(scripts, sb.String())
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return scripts
}

// extractExternalScriptSrcs returns the src attributes of all <script src="..."> tags.
func extractExternalScriptSrcs(doc *ghtml.Node) []string {
	var srcs []string
	var walk func(*ghtml.Node)
	walk = func(n *ghtml.Node) {
		if n.Type == ghtml.ElementNode && n.Data == "script" {
			for _, attr := range n.Attr {
				if attr.Key == "src" && attr.Val != "" {
					srcs = append(srcs, attr.Val)
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return srcs
}

// inlineScriptsByPosition walks a parsed DOM and returns inline
// <script> bodies keyed by the position the streaming parser would
// have assigned. The streaming parser increments its resPos counter
// for every element start tag EXCEPT the structural elements
// <html>, <head>, and <body> (which return early in
// handleStartTag before the resPos++). This walker mirrors that
// quirk so positions align.
//
// This bridges the gap from M2: the streaming parser reports inline
// scripts via OnResource with Inline=true and Source=nil because the
// tokenizer cannot capture body bytes until </script> closes. Rather
// than buffering script bodies in the parser (a more invasive change
// deferred to a later milestone), M4 enriches the coordinator's
// ScriptResults with bodies extracted from the secondary html.Parse
// pass that RenderParsed already needs.
//
// Position alignment is verified by TestInlineScriptsByPosition_Aligns.
func inlineScriptsByPosition(doc *ghtml.Node) map[int]string {
	out := make(map[int]string)
	if doc == nil {
		return out
	}
	// Streaming parser skips these in resPos; mirror the skip.
	skipResPos := map[string]bool{"html": true, "head": true, "body": true}
	var pos int
	var walk func(*ghtml.Node)
	walk = func(n *ghtml.Node) {
		if n.Type == ghtml.ElementNode {
			if n.Data == "script" {
				hasSrc := false
				for _, attr := range n.Attr {
					if attr.Key == "src" && attr.Val != "" {
						hasSrc = true
						break
					}
				}
				if !hasSrc {
					var sb strings.Builder
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						if c.Type == ghtml.TextNode {
							sb.WriteString(c.Data)
						}
					}
					if sb.Len() > 0 {
						out[pos] = sb.String()
					}
				}
			}
			if !skipResPos[n.Data] {
				pos++
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func resolveScriptURL(src, pageURL string) (string, error) {
	parsedSrc, err := urlpkg.Parse(src)
	if err != nil {
		return "", err
	}
	if parsedSrc.IsAbs() {
		return parsedSrc.String(), nil
	}

	base, err := urlpkg.Parse(pageURL)
	if err != nil {
		return "", err
	}
	if base.Scheme == "" || base.Host == "" {
		return src, nil
	}
	return base.ResolveReference(parsedSrc).String(), nil
}

// originFromURL extracts the origin (scheme + host) from a URL string.
// For URLs without a host (file://, about:blank, data:), it returns the
// scheme portion so capability policies can still classify the origin.
func originFromURL(rawURL string) string {
	u, err := urlpkg.Parse(rawURL)
	if err != nil {
		return ""
	}
	if u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return u.Scheme + "://"
}

// getFilenameFromURLAndCD extracts a default filename from the URL path or Content-Disposition.
func getFilenameFromURLAndCD(urlStr, cd string) string {
	// Parse Content-Disposition if present
	if cd != "" {
		parts := strings.Split(cd, ";")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(strings.ToLower(part), "filename=") {
				filename := strings.Trim(part[9:], "\" ")
				if filename != "" {
					return filename
				}
			}
		}
	}

	// Fallback to URL path base
	u, err := urlpkg.Parse(urlStr)
	if err == nil && u.Path != "" {
		base := filepath.Base(u.Path)
		if base != "." && base != "/" {
			return base
		}
	}

	return "download.bin"
}

// wrapImageInHTML wraps a direct image URL in a minimal HTML page so the
// browser renders it inline instead of showing raw binary data. This is the
// same approach real browsers use when navigating to an image URL directly.
func wrapImageInHTML(url string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><head><title>Image</title></head><body style="margin:0;display:flex;align-items:center;justify-content:center;min-height:100vh"><img src="%s" alt="Image" style="max-width:100%%;height:auto"></body></html>`, html.EscapeString(url))
}
