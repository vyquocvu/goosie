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
// prefixes host-shaped values (dotted names, IPs, localhost, ports) with https://,
// and searches everything else through the profile's engine.
func normalizeURL(raw string) string {
	return normalizeAddress(raw, preferences.DefaultSearchURLTemplate)
}

// searchURLFor builds the profile engine's search URL for a query, falling
// back to the default engine when the stored template is malformed.
func searchURLFor(template, query string) string {
	if result, err := preferences.SearchURL(template, query); err == nil {
		return result
	}
	return "https://www.google.com/search?q=" + url.QueryEscape(query)
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
		return searchURLFor(searchTemplate, raw)
	}
	// Only an unambiguous host shape dials: a single bare word like "golang",
	// a phrase with punctuation, or Unicode text is a search query.
	if isHostShaped(raw) {
		return "https://" + raw
	}
	return searchURLFor(searchTemplate, raw)
}

// isHostShaped reports whether a scheme-less, whitespace-free address-bar
// value names a dialable host before any path, query, or fragment: localhost,
// a full IPv4 address, a bracketed IPv6 address, or a dotted DNS name (ASCII
// or IDN). An optional :port must be numeric and in range. Values that fail
// the grammar - single labels ("golang"), underscores, empty or oversized
// ports, stray punctuation, partial IPs like "1.2.3", bare "/path" starts -
// are search queries, matching mainstream browser behavior.
func isHostShaped(raw string) bool {
	authority := raw
	if i := strings.IndexAny(raw, "/?#"); i >= 0 {
		authority = raw[:i]
	}
	if authority == "" {
		return false
	}
	if strings.HasPrefix(authority, "[") {
		end := strings.IndexByte(authority, ']')
		if end <= 0 || !isIPv6Literal(authority[1:end]) {
			return false
		}
		rest := authority[end+1:]
		return rest == "" || (strings.HasPrefix(rest, ":") && isPortNumber(rest[1:]))
	}
	host := authority
	if j := strings.LastIndexByte(authority, ':'); j >= 0 {
		host = authority[:j]
		if !isPortNumber(authority[j+1:]) {
			return false
		}
	}
	if strings.EqualFold(host, "localhost") || isIPv4Literal(host) {
		return true
	}
	return isDottedHostName(host)
}

// isPortNumber accepts 1-5 decimal digits naming a value at most 65535. An
// empty port ("example.com:") fails, so a dangling colon searches.
func isPortNumber(s string) bool {
	if s == "" || len(s) > 5 {
		return false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
		n = n*10 + int(c-'0')
	}
	return n <= 65535
}

// isIPv4Literal accepts exactly four decimal labels, each at most 255.
func isIPv4Literal(host string) bool {
	labels := strings.Split(host, ".")
	if len(labels) != 4 {
		return false
	}
	for _, l := range labels {
		if l == "" || len(l) > 3 {
			return false
		}
		n := 0
		for i := 0; i < len(l); i++ {
			c := l[i]
			if c < '0' || c > '9' {
				return false
			}
			n = n*10 + int(c-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}

// isIPv6Literal accepts the hex, colon, dot, and percent-zone characters of a
// bracketed address and requires at least one colon: enough to recognize
// [::1]:8080 without full RFC 4291 parsing.
func isIPv6Literal(s string) bool {
	if !strings.ContainsRune(s, ':') {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		case r == ':' || r == '.' || r == '%':
		default:
			return false
		}
	}
	return true
}

// isDottedHostName accepts a multi-label DNS name: labels of ASCII letters,
// digits, hyphens, and non-ASCII (IDN) characters, none empty and none
// starting or ending with a hyphen. The last label must not be all digits, so
// partial addresses like "1.2.3" search instead of dialing.
func isDottedHostName(host string) bool {
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for i, l := range labels {
		if l == "" || strings.HasPrefix(l, "-") || strings.HasSuffix(l, "-") {
			return false
		}
		allDigits := true
		for _, r := range l {
			switch {
			case r >= '0' && r <= '9':
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-':
				allDigits = false
			case r >= 0x80:
				allDigits = false
			default:
				return false
			}
		}
		if i == len(labels)-1 && allDigits {
			return false
		}
	}
	return true
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
