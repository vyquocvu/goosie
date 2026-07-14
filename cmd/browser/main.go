package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
	"github.com/vyquocvu/goosie/internal/engine/session"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/memory"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/profile"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/ui"
	ghtml "golang.org/x/net/html"
)

func main() {
	headlessFlag := flag.Bool("headless", false, "Run in headless mode without a UI window")
	urlFlag := flag.String("url", "", "URL to open on startup")
	screenshotFlag := flag.String("screenshot", "", "File path to save a screenshot (only in headless mode)")
	flag.Parse()

	prof, err := profile.Open(profile.Options{})
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
		a = test.NewApp()
		w = a.NewWindow("Goosie Headless")
		w.Resize(fyne.NewSize(1000, 700))
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
		Session:       sessionStore,
		SettingsStore: settingsStore,
		Storage:       storage,
		Network:       networkService,
		Memory:        memMgr,
		App:           a,
		Window:        w,
	})
	browser.RendererFactory = func() ui.HTMLRenderer {
		return renderer.NewRenderer(1000, 700)
	}

	// Channel to signal when initial page load is complete (used in headless mode)
	pageLoaded := make(chan bool, 1)

	// Set up navigation callback
	browser.SetNavigationCallback(func(url string) {
		load, ctx := navSession.Navigate(context.Background(), url)
		loadPageAsync(browser, fetcher, parser, load, ctx, navSession, func() {
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
			loadPageAsync(browser, fetcher, parser, load, ctx, navSession, func() {
				pageLoaded <- true
			})

			<-pageLoaded

			// Give Fyne's UI thread a brief moment to process the layout changes
			time.Sleep(100 * time.Millisecond)

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
			loadPageAsync(browser, fetcher, parser, load, ctx, navSession, nil)
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
func loadPageAsync(browser *ui.Browser, fetcher *net.Fetcher, parser *dom.Parser, load navigation.Load, ctx context.Context, sess *session.Session, onComplete func()) {
	log.Printf("Navigation %s started: %s", load.ID, load.URL)

	// Update browser state on main thread
	browser.NavigateTo(load.URL)

	// Show loading indicator on main thread
	browser.ShowLoading()
	if activeTab := browser.ActiveTab(); activeTab != nil {
		if r := activeTab.GetRenderer(); r != nil {
			r.SetSubmitting(true)
		}
	}

	navID := load.ID
	url := load.URL

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

		// Resolve URL if it's relative or needs resolution
		resolvedURL := url
		if activeTab := browser.ActiveTab(); activeTab != nil {
			if renderer := activeTab.GetRenderer(); renderer != nil {
				resolvedURL = renderer.ResolveURL(url)
			}
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

		var html string
		if fetchErr != nil {
			// Fallback to mock HTML for example.com if network is unavailable
			log.Printf("Navigation %s network error (%v), checking if example.com for mock HTML", navID, fetchErr)
			if resolvedURL == "https://example.com" {
				html = `<!DOCTYPE html>
<html>
<head>
    <title>Example Domain</title>
</head>
<body>
    <div>
        <h1>Example Domain</h1>
        <p id="main-content">This domain is for use in illustrative examples in documents. You may use this domain in literature without prior coordination or asking for permission.</p>
        <p><a href="https://www.iana.org/domains/example">More information...</a></p>
    </div>
</body>
</html>`
			} else {
				updateUIWithError(browser, sess, navID, fetchErr, resolvedURL)
				return
			}
		} else {
			// Read the stream directly into a string. This performs one allocation
			// for the body bytes plus one string conversion, instead of the previous
			// bytes.Buffer path which allocated repeatedly during growth.
			data, readErr := io.ReadAll(stream)
			stream.Close()
			if readErr != nil {
				log.Printf("Navigation %s stream read error: %v", navID, readErr)
				updateUIWithError(browser, sess, navID, readErr, resolvedURL)
				return
			}
			html = string(data)

			// Handle error status codes: generate fallback HTML for empty error bodies.
			if meta.Status >= 400 && strings.TrimSpace(html) == "" {
				html = fmt.Sprintf(
					"<html><body><h1>%d %s</h1><p>The server returned an error.</p></body></html>",
					meta.Status, strings.TrimSpace(fmt.Sprintf("%d", meta.Status)),
				)
			}
		}

		updateUIWithContent(ctx, browser, fetcher, sess, navID, html, resolvedURL, sess, parser)
	}()
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
func updateUIWithContent(ctx context.Context, browser *ui.Browser, fetcher *net.Fetcher, sess *session.Session, navID navigation.ID, html string, url string, navSession *session.Session, parser *dom.Parser) {
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

		// Set the page origin and default capability policy so that
		// JS runtime APIs (localStorage, fetch, window.open, etc.) are
		// gated behind the appropriate capabilities.
		jsRuntime.SetOrigin(originFromURL(url))
		jsRuntime.SetEnforcer(js.NewScriptEnforcer(js.DefaultSecurePolicy()))

		// Wire up the real HTTP fetcher so fetch() makes actual network requests
		jsRuntime.SetFetcher(fetcher)

		// Wire up window.open to navigate in the current browsing context.
		jsRuntime.OnOpenWindow = func(url, name string) {
			log.Printf("Popup (window.open): %s (name=%s)", url, name)
			load, ctx := navSession.Navigate(context.Background(), url)
			loadPageAsync(browser, fetcher, parser, load, ctx, navSession, nil)
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

// loadPage fetches and displays a web page (deprecated - use loadPageAsync).
func loadPage(browser *ui.Browser, fetcher *net.Fetcher, parser *dom.Parser, url string) {
	s := session.New()
	load, ctx := s.Navigate(context.Background(), url)
	loadPageAsync(browser, fetcher, parser, load, ctx, s, nil)
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
