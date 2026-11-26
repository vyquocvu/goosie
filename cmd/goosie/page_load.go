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

func imageOption(deferred bool, base string, fetch engine.ImageFetcher) engine.Option {
	if deferred {
		return engine.WithDeferredImages(base, fetch)
	}
	return engine.WithImages(base, fetch)
}

func fontOption(deferred bool, base string, fetch engine.FontFetcher, reg engine.FontRegistry) engine.Option {
	if deferred {
		return engine.WithDeferredCustomFontLoading(base, fetch, reg)
	}
	return engine.WithCustomFontLoading(base, fetch, reg)
}

type jsTransportContext struct {
	context.Context
	values context.Context
}

func (c jsTransportContext) Value(key any) any {
	if value := c.Context.Value(key); value != nil {
		return value
	}
	return c.values.Value(key)
}

type jsTransport struct {
	client      net.HTTP
	ctx         context.Context
	csp         *js.CSPPolicy
	documentURL string
}

func (t jsTransport) Fetch(ctx context.Context, url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	if ctx == nil {
		ctx = t.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	} else if t.ctx != nil {
		ctx = jsTransportContext{Context: ctx, values: t.ctx}
	}
	resolvedURL := resolveHref(t.documentURL, url)
	if !t.csp.AllowsConnect(resolvedURL, t.documentURL) {
		return nil, fmt.Errorf("goosie: fetch blocked by Content-Security-Policy")
	}
	var resp *net.Response
	var err error
	switch method {
	case "GET", "":
		resp, err = t.client.Get(ctx, resolvedURL)
	case "POST":
		contentType := "text/plain;charset=UTF-8"
		for k, v := range headers {
			if strings.EqualFold(k, "Content-Type") {
				contentType = v
				break
			}
		}
		resp, err = t.client.Post(ctx, resolvedURL, contentType, bytes.NewReader(body))
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

func subresourceCtx(ctx context.Context, base string) context.Context {
	if _, guarded := net.AddressGuardFromContext(ctx); guarded {
		return ctx
	}
	if engine.InitiatorIsLocal(base) {
		return ctx
	}
	return net.WithAddressGuard(ctx, net.AddressGuard{Blocked: engine.AddressBlocked})
}

func normalizeURL(raw string) string {
	return normalizeAddress(raw, preferences.DefaultSearchURLTemplate)
}

func searchURLFor(template, query string) string {
	if result, err := preferences.SearchURL(template, query); err == nil {
		return result
	}
	return "https://www.google.com/search?q=" + url.QueryEscape(query)
}

func normalizeAddress(raw, searchTemplate string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if target, isVS := viewsource.IsViewSourceURL(raw); isVS {
		return viewsource.WrapViewSourceURL(normalizeAddress(target, searchTemplate))
	}
	lower := strings.ToLower(raw)
	for _, scheme := range []string{"https://", "http://", "file://"} {
		if strings.HasPrefix(lower, scheme) {
			return scheme + raw[len(scheme):]
		}
	}
	if strings.HasPrefix(lower, "goosie://") {
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
	if strings.ContainsAny(raw, " \t\r\n") {
		return searchURLFor(searchTemplate, raw)
	}
	if isHostShaped(raw) {
		return "https://" + raw
	}
	return searchURLFor(searchTemplate, raw)
}

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
