package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"net/url"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	imgdecode "github.com/vyquocvu/goosie/internal/image"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/tabs"
)

// faviconTimeout bounds one icon fetch. Icons are cosmetic: a hanging origin
// must not keep a tab's icon request alive beyond the navigation that asked.
const faviconTimeout = 5 * time.Second

// faviconURLFor returns the URL a tab should fetch for its icon: only an icon
// the document declared. There is deliberately no implicit /favicon.ico
// request: visiting a page would otherwise add a hidden request to every origin
// (and to every origin a test server stands in for), while the tab strip's
// placeholder already covers sites that publish no icon.
func faviconURLFor(doc *dom.Node, pageURL string) string {
	return findFaviconLink(doc, pageURL)
}

// findFaviconLink searches the DOM for a <link rel="icon"> (or the Apple touch
// icon as a fallback) and resolves its href against the document URL. A
// standard icon anywhere in the head beats an Apple icon, matching the priority
// browsers give the explicit favicon.
func findFaviconLink(root *dom.Node, baseURL string) string {
	if root == nil {
		return ""
	}
	var standard, apple string
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if n == nil {
			return
		}
		if standard == "" && strings.EqualFold(n.Data, "link") {
			var rel, href string
			for _, attr := range n.Attr {
				switch {
				case strings.EqualFold(attr.Name, "rel"):
					rel = attr.Value
				case strings.EqualFold(attr.Name, "href"):
					href = strings.TrimSpace(attr.Value)
				}
			}
			if href != "" {
				if isStandardIconRel(rel) {
					standard = href
				} else if apple == "" && isAppleIconRel(rel) {
					apple = href
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)

	href := standard
	if href == "" {
		href = apple
	}
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

func isStandardIconRel(rel string) bool {
	for _, tok := range strings.Fields(strings.ToLower(rel)) {
		if tok == "icon" {
			return true
		}
	}
	return false
}

func isAppleIconRel(rel string) bool {
	for _, tok := range strings.Fields(strings.ToLower(rel)) {
		switch tok {
		case "apple-touch-icon", "apple-touch-icon-precomposed":
			return true
		}
	}
	return false
}

// fetchFavicon downloads and decodes one icon through the admission caps in
// internal/image (encoded size, dimensions and pixel count are all checked
// before decode).
func fetchFavicon(ctx context.Context, client net.HTTP, rawURL string) (image.Image, error) {
	if client == nil {
		return nil, errors.New("favicon: no HTTP client")
	}
	if rawURL == "" {
		return nil, errors.New("favicon: empty URL")
	}
	resp, err := client.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("favicon: empty response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("favicon: status %d", resp.StatusCode)
	}
	return imgdecode.Decode(resp.Body)
}

// loadFavicon fetches a tab's icon on the caller's goroutine and stores it only
// if no newer navigation superseded the one that requested it. It runs off the
// chrome thread; the draw path picks the image up on the next vsync.
func (f *framePath) loadFavicon(tab *tabs.Tab, serial uint64, rawURL string) {
	if tab == nil || rawURL == "" {
		return
	}
	client := f.clientForTab(tab)
	ctx, cancel := context.WithTimeout(context.Background(), faviconTimeout)
	defer cancel()
	icon, err := fetchFavicon(ctx, client, rawURL)
	if err != nil || icon == nil {
		return
	}
	tab.Nav.Mu.Lock()
	current := tab.Nav.Serial
	tab.Nav.Mu.Unlock()
	if current != serial {
		return
	}
	tab.SetFavicon(icon)
}
