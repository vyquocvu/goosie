package main

import (
	"context"
	"fmt"
	"log"
	urlpkg "net/url"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/profile"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/ui"
	ghtml "golang.org/x/net/html"
)

func main() {
	prof, err := profile.Open(profile.Options{})
	if err != nil {
		log.Fatalf("failed to open profile: %v", err)
	}
	bookmarks, err := profile.NewBookmarkStore(prof)
	if err != nil {
		log.Fatalf("failed to open bookmarks: %v", err)
	}
	history, err := profile.NewHistoryStore(prof)
	if err != nil {
		log.Fatalf("failed to open history: %v", err)
	}
	settingsStore, err := profile.NewSettingsStore(prof)
	if err != nil {
		log.Fatalf("failed to open settings: %v", err)
	}
	storage, err := profile.NewStorageStore(prof)
	if err != nil {
		log.Fatalf("failed to open storage: %v", err)
	}
	networkService := net.NewService(net.ServiceOptions{
		Cache: net.NewHTTPCache(filepath.Join(prof.Root(), "cache"), prof.Private()),
	})
	fetcher := net.NewFetcherWithService(networkService)
	parser := dom.NewParser()
	browser := ui.NewBrowserWithDependencies(ui.BrowserDependencies{
		Profile:       prof,
		Bookmarks:     bookmarks,
		History:       history,
		SettingsStore: settingsStore,
		Storage:       storage,
		Network:       networkService,
	})
	browser.RendererFactory = func() ui.HTMLRenderer {
		return renderer.NewRenderer(1000, 700)
	}

	// Create a cancellable context for page loads
	var currentLoadCtx context.Context
	var currentLoadCancel context.CancelFunc

	// Set up navigation callback
	browser.SetNavigationCallback(func(url string) {
		// Cancel any ongoing page load
		if currentLoadCancel != nil {
			currentLoadCancel()
		}

		// Create new context for this load
		currentLoadCtx, currentLoadCancel = context.WithCancel(context.Background())

		// Load page asynchronously
		loadPageAsync(browser, fetcher, parser, url, currentLoadCtx)
	})

	// Show browser window
	browser.Show()
}

// pageLoadResult represents the result of an async page load
type pageLoadResult struct {
	html string
	err  error
}

// loadPageAsync fetches and displays a web page asynchronously
func loadPageAsync(browser *ui.Browser, fetcher *net.Fetcher, parser *dom.Parser, url string, ctx context.Context) {
	log.Printf("Navigating to: %s", url)

	// Update browser state on main thread
	browser.NavigateTo(url)

	// Show loading indicator on main thread
	browser.ShowLoading()

	// Launch background goroutine for fetch and render
	go func() {
		// Resolve URL if it's relative or needs resolution
		resolvedURL := url
		if activeTab := browser.ActiveTab(); activeTab != nil {
			if renderer := activeTab.GetRenderer(); renderer != nil {
				resolvedURL = renderer.ResolveURL(url)
			}
		}

		// Handle anchor links (scroll within current page)
		if strings.HasPrefix(url, "#") {
			log.Printf("Anchor link detected: %s - scrolling within current page", url)
			// For now, just hide loading and return (anchor scrolling can be implemented later)
			browser.HideLoading()
			return
		}

		// Fetch the page in background
		html, err := fetcher.FetchWithContext(ctx, resolvedURL, nil)

		// Check if context was cancelled
		if ctx.Err() != nil {
			log.Printf("Page load cancelled for: %s", url)
			return
		}

		if err != nil {
			// Fallback to mock HTML for example.com if network is unavailable
			log.Printf("Network error (%v), checking if example.com for mock HTML", err)
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
				// Update UI on main thread with error
				updateUIWithError(browser, err, resolvedURL)
				return
			}
		}

		// Update UI on main thread with content
		updateUIWithContent(browser, fetcher, html, resolvedURL)
	}()
}

// updateUIWithError updates the UI with an error message
func updateUIWithError(browser *ui.Browser, err error, url string) {
	log.Printf("Error loading page %s: %v", url, err)
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
	_ = browser.RenderHTMLContent(errorHTML) // Ignore error for simplicity
	browser.HideLoading()
}

// updateUIWithContent updates the UI with HTML content
func updateUIWithContent(browser *ui.Browser, fetcher *net.Fetcher, html string, url string) {
	log.Printf("Rendering page content")

	// Fyne widgets are thread-safe and can be updated from any goroutine
	// Render HTML using the canvas-based renderer
	err := browser.RenderHTMLContent(html)
	if err != nil {
		log.Printf("Error rendering HTML: %v", err)
		browser.SetContent("Error rendering HTML: " + err.Error())
		browser.HideLoading()
		return
	}

	log.Printf("Page loaded successfully")

	// Update tab title
	if title, ok := extractTitle(html); ok {
		fyne.CurrentApp().SendNotification(&fyne.Notification{
			Title:   "Goosie",
			Content: "Page loaded: " + title,
		})
		browser.UpdateActiveTabTitle(title)
	}

	// Hide loading indicator
	browser.HideLoading()

	// Get or create JS runtime for the active tab
	tab := browser.ActiveTab()
	if tab != nil {
		if tab.GetJSRuntime() == nil {
			jsRuntime := js.NewRuntime()
			tab.SetJSRuntime(jsRuntime)
		}

		jsRuntime := tab.GetJSRuntime()

		// Wire up the real HTTP fetcher so fetch() makes actual network requests
		jsRuntime.SetFetcher(fetcher)

		// Wire up the DOM mutation callback to re-render the HTML content on dynamic updates
		jsRuntime.SetDOMMutationCallback(func(mutatedHTML string) {
			log.Printf("DOM mutated by JS, triggering UI re-render")
			if err := tab.RenderHTML(mutatedHTML); err != nil {
				log.Printf("Error rendering mutated HTML: %v", err)
			}
		})

		// Set HTML content for JS runtime (enables document.getElementById etc.)
		jsRuntime.SetHTMLContent(html)

		// Execute inline <script> tags found in the page
		scripts := extractInlineScripts(html)
		for _, script := range scripts {
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

// loadPage fetches and displays a web page (deprecated - use loadPageAsync)
func loadPage(browser *ui.Browser, fetcher *net.Fetcher, parser *dom.Parser, url string) {
	loadPageAsync(browser, fetcher, parser, url, context.Background())
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
