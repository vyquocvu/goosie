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

const faviconTimeout = 5 * time.Second

func faviconURLFor(doc *dom.Node, pageURL string) string {
	return findFaviconLink(doc, pageURL)
}

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
