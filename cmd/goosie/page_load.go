package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/preferences"
	"github.com/vyquocvu/goosie/internal/viewsource"
)

type appearanceMode struct {
	dark         bool
	highContrast bool
}

// imageOption picks the synchronous or deferred image path for a load.
func imageOption(deferred bool, base string, fetch engine.ImageFetcher) engine.Option {
	if deferred {
		return engine.WithDeferredImages(base, fetch)
	}
	return engine.WithImages(base, fetch)
}

// fontOption picks the synchronous or deferred font path for a load.
func fontOption(deferred bool, base string, fetch engine.FontFetcher, reg engine.FontRegistry) engine.Option {
	if deferred {
		return engine.WithDeferredCustomFontLoading(base, fetch, reg)
	}
	return engine.WithCustomFontLoading(base, fetch, reg)
}

// jsTransport adapts the browser HTTP client to the js.HTTPFetcher contract
// the fetch()/XMLHttpRequest bindings call. GET maps straight through; POST
// carries the Content-Type header (defaulting to the spec's text/plain for
// string bodies) because the client interface exposes only Get and Post.
// Other methods and per-request headers beyond Content-Type are not carried
// by this interface yet, so those requests fail through the bindings' error
// paths rather than silently misbehaving.
type jsTransport struct {
	client net.HTTP
	ctx    context.Context
}

func (t jsTransport) Fetch(url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var resp *net.Response
	var err error
	switch method {
	case "GET", "":
		resp, err = t.client.Get(ctx, url)
	case "POST":
		contentType := "text/plain;charset=UTF-8"
		for k, v := range headers {
			if strings.EqualFold(k, "Content-Type") {
				contentType = v
				break
			}
		}
		resp, err = t.client.Post(ctx, url, contentType, bytes.NewReader(body))
	default:
		return nil, fmt.Errorf("goosie: fetch method %q is not supported", method)
	}
	if err != nil {
		return nil, err
	}
	out := &js.FetchResponse{
		Status:     resp.StatusCode,
		StatusText: http.StatusText(resp.StatusCode),
		Headers:    resp.Headers,
		Body:       resp.Body,
		URL:        resp.URL,
		OK:         resp.StatusCode >= 200 && resp.StatusCode < 300,
	}
	return out, nil
}

// subresourceCtx is where the browser's two security halves meet, and the only
// place they can: internal/engine owns the private-network rule but cannot
// import internal/net, and internal/net owns the dial but must stay policy-free.
// A subresource fetched for a public page therefore carries the engine's address
// predicate down to the transport, which resolves the name once, judges every
// answer, and dials the address it judged - so a host name that resolves to
// 169.254.169.254 is refused even though the URL itself looked public.
//
// A local initiator (127.0.0.1, a development server, a file:// document) gets
// ctx back untouched. That is the same distinction resolveSubresourceURL makes
// about names, and guarding it would refuse a page its own stylesheet.
func subresourceCtx(ctx context.Context, base string) context.Context {
	if engine.InitiatorIsLocal(base) {
		return ctx
	}
	return net.WithAddressGuard(ctx, net.AddressGuard{Blocked: engine.AddressBlocked})
}

// normalizeURL normalizes address bar input into either a dialable URL or a search query.
// It trims surrounding whitespace, normalizes supported schemes (http, https, file),
// treats queries with whitespace or search/punctuation symbols as web searches,
// and prefixes bare domains or IPs with https://.
func normalizeURL(raw string) string {
	return normalizeAddress(raw, preferences.DefaultSearchURLTemplate)
}

// normalizeAddress resolves an address-bar value using the profile's search
// engine when the input is a query rather than a URL.
func normalizeAddress(raw, searchTemplate string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if target, isVS := viewsource.IsViewSourceURL(raw); isVS {
		return viewsource.WrapViewSourceURL(normalizeAddress(target, searchTemplate))
	}
	// Case-insensitive scheme check for http, https, and file.
	lower := strings.ToLower(raw)
	for _, scheme := range []string{"https://", "http://", "file://"} {
		if strings.HasPrefix(lower, scheme) {
			return scheme + raw[len(scheme):]
		}
	}
	if strings.HasPrefix(lower, "goosie://") {
		// Lowercase only the page name (host); the query carries case-sensitive
		// action parameters (URLs, titles, file paths) and must survive intact.
		rest := raw[len("goosie://"):]
		host := rest
		if i := strings.IndexAny(rest, "/?#"); i >= 0 {
			host = rest[:i]
		}
		return "goosie://" + strings.ToLower(host) + rest[len(host):]
	}
	if strings.HasPrefix(lower, "about:") {
		target := strings.TrimPrefix(lower, "about:")
		if target == "" {
			target = "version"
		}
		return "goosie://" + target
	}
	// If input contains whitespace (space, tab, newline), it is a search query.
	if strings.ContainsAny(raw, " \t\r\n") {
		if result, err := preferences.SearchURL(searchTemplate, raw); err == nil {
			return result
		}
		return "https://www.google.com/search?q=" + url.QueryEscape(raw)
	}
	// If host portion (before any '/') contains symbols invalid in a domain name
	// (such as '?', '+', '&', '=', '%', '!', '^', '*', '(', ')', '<', '>', ';'),
	// treat it as a search query.
	hostPart := raw
	if slash := strings.IndexByte(raw, '/'); slash >= 0 {
		hostPart = raw[:slash]
	}
	if strings.ContainsAny(hostPart, "?+&=%!^*()<>;\"'") {
		if result, err := preferences.SearchURL(searchTemplate, raw); err == nil {
			return result
		}
		return "https://www.google.com/search?q=" + url.QueryEscape(raw)
	}
	// Bare domain, IP, or localhost: add https:// prefix
	return "https://" + raw
}

// resolveHref turns a clicked link's href into an absolute URL against the
// document that owns it, per RFC 3986. An unparseable piece falls back to the
// raw href so address-bar normalization still gets a chance.
func resolveHref(base, href string) string {
	if base == "" {
		return href
	}
	b, err := url.Parse(base)
	if err != nil {
		return href
	}
	r, err := url.Parse(href)
	if err != nil {
		return href
	}
	return b.ResolveReference(r).String()
}
