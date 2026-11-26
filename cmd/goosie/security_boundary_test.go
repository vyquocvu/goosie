package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdnet "net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/perms"
	"github.com/vyquocvu/goosie/internal/raster"
)

type securityResponse struct {
	status       int
	headers      map[string]string
	headerValues map[string][]string
	body         string
}

type securityClient struct {
	mu        sync.Mutex
	responses map[string]securityResponse
	redirects map[string]string
	requests  []string
	contexts  map[string]context.Context
}

func (c *securityClient) Get(ctx context.Context, raw string) (*net.Response, error) {
	c.mu.Lock()
	c.requests = append(c.requests, raw)
	if c.contexts == nil {
		c.contexts = make(map[string]context.Context)
	}
	c.contexts[raw] = ctx
	target := c.redirects[raw]
	response, ok := c.responses[raw]
	c.mu.Unlock()
	if target != "" {
		u, err := url.Parse(target)
		if err != nil {
			return nil, err
		}
		if guard, guarded := net.AddressGuardFromContext(ctx); guarded {
			var addresses []stdnet.IP
			if ip := stdnet.ParseIP(u.Hostname()); ip != nil {
				addresses = []stdnet.IP{ip}
			} else if guard.Lookup != nil {
				addresses, err = guard.Lookup(ctx, u.Hostname())
				if err != nil {
					return nil, err
				}
			}
			for _, ip := range addresses {
				if guard.Blocked(ip) {
					return nil, fmt.Errorf("%w: %s", net.ErrBlockedAddress, target)
				}
			}
		}
		return c.Get(ctx, target)
	}
	if !ok {
		return nil, fmt.Errorf("securityClient: no fixture for %q", raw)
	}
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	return &net.Response{
		StatusCode:   status,
		Headers:      response.headers,
		HeaderValues: response.headerValues,
		Body:         []byte(response.body),
		URL:          raw,
	}, nil
}

func (c *securityClient) Post(ctx context.Context, raw, contentType string, body io.Reader) (*net.Response, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.requests = append(c.requests, raw)
	c.mu.Unlock()
	response, ok := c.responses[raw]
	if !ok {
		response, ok = c.responses[raw+"#"+contentType+"#"+string(data)]
	}
	if !ok {
		return nil, fmt.Errorf("securityClient: no POST fixture for %q", raw)
	}
	return &net.Response{StatusCode: http.StatusOK, Headers: response.headers, Body: []byte(response.body), URL: raw}, nil
}

func (c *securityClient) SetTimeout(time.Duration)    {}
func (c *securityClient) Close() error                { return nil }
func (c *securityClient) Jar() *net.CookieJar         { return nil }
func (c *securityClient) NetworkLog() *net.NetworkLog { return nil }

func (c *securityClient) requested(raw string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, request := range c.requests {
		if request == raw {
			return true
		}
	}
	return false
}

func (c *securityClient) requestContext(raw string) (context.Context, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, ok := c.contexts[raw]
	return ctx, ok
}

func TestPublicDocumentRedirectHopsAreAddressGuarded(t *testing.T) {
	const start = "http://93.184.216.34/start"
	const hop = "http://8.8.8.8/hop"
	const target = "http://private.example/private"
	client := &securityClient{
		redirects: map[string]string{start: hop, hop: target},
		responses: map[string]securityResponse{target: {body: "private"}},
	}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	ctx := net.WithAddressGuard(context.Background(), net.AddressGuard{
		Blocked: engine.AddressBlocked,
		Lookup: func(ctx context.Context, host string) ([]stdnet.IP, error) {
			return []stdnet.IP{stdnet.ParseIP("127.0.0.1")}, nil
		},
	})
	_, _, _, _, _, err = loadURLCtx(ctx, client, fonts, start, 800, 600, 1, t.TempDir(), false, false)
	if !errors.Is(err, net.ErrBlockedAddress) {
		t.Fatalf("public-to-private redirect error = %v, want ErrBlockedAddress", err)
	}
	if !client.requested(hop) {
		t.Fatal("the public redirect hop was not evaluated")
	}
	if client.requested(target) {
		t.Fatal("the private redirect target was fetched")
	}
}

func TestLocalDocumentCanRedirectToLocalDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html><body>local final</body></html>")
	}))
	defer server.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	client := net.DefaultClient()
	defer client.Close()
	_, _, _, session, finalURL, err := loadURLCtx(context.Background(), client, fonts, server.URL+"/start", 800, 600, 1, t.TempDir(), false, false)
	if err != nil {
		t.Fatalf("local-to-local redirect: %v", err)
	}
	defer session.Close()
	if finalURL != server.URL+"/final" {
		t.Fatalf("final URL = %q, want %q", finalURL, server.URL+"/final")
	}
}

func TestDocumentLoadPreservesCallerAddressGuardSeam(t *testing.T) {
	const documentURL = "http://public.example/index.html"
	var lookups atomic.Int32
	guard := net.AddressGuard{
		Blocked: engine.AddressBlocked,
		Lookup: func(ctx context.Context, host string) ([]stdnet.IP, error) {
			lookups.Add(1)
			return []stdnet.IP{stdnet.ParseIP("127.0.0.1")}, nil
		},
	}
	ctx := net.WithAddressGuard(context.Background(), guard)
	client := &recordingClient{docs: map[string]string{documentURL: "<html><body>seam</body></html>"}}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, session, _, err := loadURLCtx(ctx, client, fonts, documentURL, 800, 600, 1, t.TempDir(), false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	got, ok := net.AddressGuardFromContext(client.ctxes[documentURL])
	if !ok || got.Lookup == nil {
		t.Fatal("loadURLCtx replaced the caller's address guard and resolver seam")
	}
	addresses, err := got.Lookup(context.Background(), "public.example")
	if err != nil || len(addresses) != 1 || !addresses[0].Equal(stdnet.ParseIP("127.0.0.1")) {
		t.Fatalf("preserved lookup = %v, %v", addresses, err)
	}
	if lookups.Load() != 1 {
		t.Fatalf("preserved lookup calls = %d, want 1", lookups.Load())
	}
}

func TestJSTransportPreservesBaseAddressGuard(t *testing.T) {
	const requestURL = "https://document.example/api"
	client := &securityClient{responses: map[string]securityResponse{
		requestURL: {headers: map[string]string{"Content-Type": "text/plain"}, body: "ok"},
	}}
	guard := net.AddressGuard{
		Blocked: engine.AddressBlocked,
		Lookup: func(ctx context.Context, host string) ([]stdnet.IP, error) {
			return []stdnet.IP{stdnet.ParseIP("127.0.0.1")}, nil
		},
	}
	transport := jsTransport{
		client:      client,
		ctx:         net.WithAddressGuard(context.Background(), guard),
		documentURL: "https://document.example/page",
	}
	if _, err := transport.Fetch(context.Background(), requestURL, "GET", nil, nil); err != nil {
		t.Fatal(err)
	}
	ctx, ok := client.requestContext(requestURL)
	if !ok {
		t.Fatal("fetch context was not recorded")
	}
	got, ok := net.AddressGuardFromContext(ctx)
	if !ok || got.Lookup == nil {
		t.Fatal("fetch dropped the shell transport's address guard")
	}
	addresses, err := got.Lookup(context.Background(), "document.example")
	if err != nil || len(addresses) != 1 || !addresses[0].Equal(stdnet.ParseIP("127.0.0.1")) {
		t.Fatalf("fetch guard lookup = %v, %v", addresses, err)
	}
}

func TestSitePermissionsCookiesCoverTransportAndReset(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{}
	apiRequests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.RequestURI()] = r.Header.Get("Cookie")
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/seed":
			w.Header().Set("Set-Cookie", "seed=one; Path=/")
		case "/api":
			apiRequests <- r.Header.Get("Cookie")
			w.Header().Set("Set-Cookie", "api="+r.URL.Query().Get("phase")+"; Path=/")
			_, _ = io.WriteString(w, "ok")
		case "/page":
			w.Header().Set("Content-Type", "text/html")
			if r.URL.Query().Get("phase") == "blocked" {
				w.Header().Set("Set-Cookie", "seed=; Max-Age=0; Path=/")
			} else {
				w.Header().Set("Set-Cookie", "page="+r.URL.Query().Get("phase")+"; Path=/")
			}
			_, _ = fmt.Fprintf(w, `<html><head><link rel="stylesheet" href="/style.css?phase=%s"></head><body>cookies</body></html>`, r.URL.Query().Get("phase"))
		case "/style.css":
			w.Header().Set("Content-Type", "text/css")
			w.Header().Set("Set-Cookie", "style="+r.URL.Query().Get("phase")+"; Path=/")
			_, _ = io.WriteString(w, "body { color: black }")
		}
	}))
	defer server.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen["other"+r.URL.RequestURI()] = r.Header.Get("Cookie")
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/seed" {
			w.Header().Set("Set-Cookie", "other=one; Path=/")
		} else {
			w.Header().Set("Set-Cookie", "other=page; Path=/")
		}
		_, _ = io.WriteString(w, "<html><body>other</body></html>")
	}))
	defer other.Close()
	otherURL := strings.Replace(other.URL, "127.0.0.1", "localhost", 1)

	client := net.DefaultClient()
	defer client.Close()
	if _, err := client.Get(context.Background(), server.URL+"/seed"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(context.Background(), otherURL+"/seed"); err != nil {
		t.Fatal(err)
	}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	manager := perms.NewManager("")
	manager.Set(server.URL, perms.PermissionCookies, perms.StateBlock)
	load := func(raw string) *engine.Session {
		_, _, _, session, _, err := loadURLCtx(context.Background(), client, fonts, raw, 800, 600, 1, t.TempDir(), false, false, manager)
		if err != nil {
			t.Fatalf("loadURLCtx(%s): %v", raw, err)
		}
		t.Cleanup(session.Close)
		session.PumpJS(8)
		return session
	}

	blocked := load(server.URL + "/page?phase=blocked")
	mu.Lock()
	topCookie := seen["/page?phase=blocked"]
	sheetCookie := seen["/style.css?phase=blocked"]
	mu.Unlock()
	if topCookie != "" || sheetCookie != "" {
		t.Fatalf("blocked cookies reached streaming/buffered requests: document=%q stylesheet=%q", topCookie, sheetCookie)
	}
	if got, err := blocked.EvaluateJS("document.cookie"); err != nil || got != "" {
		t.Fatalf("blocked document.cookie = %q, %v", got, err)
	}
	if _, err := blocked.EvaluateJS(fmt.Sprintf(`var request = new XMLHttpRequest(); request.open("GET", %q); request.onload = function() { document.body.setAttribute("data-xhr", "loaded"); }; request.send();`, server.URL+"/api?phase=blocked")); err != nil {
		t.Fatalf("start blocked XHR: %v", err)
	}
	blocked.PumpJS(8)
	if got := receiveSecurityCookie(t, apiRequests); got != "" {
		t.Fatalf("blocked XHR sent Cookie %q", got)
	}
	waitForSecurityCondition(t, func() bool {
		blocked.PumpJS(8)
		return blocked.Doc.Body.GetAttribute("data-xhr") == "loaded"
	}, "blocked XHR did not finish")
	if got := securityCookieValues(client.Jar().CookiesForHost(securityURLHost(t, server.URL))); got != "seed=one" {
		t.Fatalf("blocked responses changed existing cookies: %q", got)
	}

	load(otherURL + "/page")
	mu.Lock()
	otherCookie := seen["other/page"]
	mu.Unlock()
	if otherCookie != "other=one" {
		t.Fatalf("other origin Cookie = %q, want other=one", otherCookie)
	}
	if got := securityCookieValues(client.Jar().CookiesForHost(securityURLHost(t, otherURL))); !strings.Contains(got, "other=page") {
		t.Fatalf("other origin cookies = %q", got)
	}

	manager.Set(server.URL, perms.PermissionCookies, perms.StateDefault)
	reset := load(server.URL + "/page?phase=reset")
	mu.Lock()
	topCookie = seen["/page?phase=reset"]
	sheetCookie = seen["/style.css?phase=reset"]
	mu.Unlock()
	if topCookie != "seed=one" || !strings.Contains(sheetCookie, "seed=one") {
		t.Fatalf("cookies after reset: document=%q stylesheet=%q", topCookie, sheetCookie)
	}
	if _, err := reset.EvaluateJS(fmt.Sprintf(`var request = new XMLHttpRequest(); request.open("GET", %q); request.onload = function() { document.body.setAttribute("data-xhr", "loaded"); }; request.send();`, server.URL+"/api?phase=reset")); err != nil {
		t.Fatalf("start reset XHR: %v", err)
	}
	reset.PumpJS(8)
	if got := receiveSecurityCookie(t, apiRequests); !strings.Contains(got, "seed=one") {
		t.Fatalf("XHR Cookie after reset = %q", got)
	}
	waitForSecurityCondition(t, func() bool {
		reset.PumpJS(8)
		return reset.Doc.Body.GetAttribute("data-xhr") == "loaded"
	}, "reset XHR did not finish")
	if got := securityCookieValues(client.Jar().CookiesForHost(securityURLHost(t, server.URL))); !strings.Contains(got, "seed=one") || !strings.Contains(got, "page=reset") || !strings.Contains(got, "style=reset") || !strings.Contains(got, "api=reset") {
		t.Fatalf("cookies after reset = %q", got)
	}
}

func receiveSecurityCookie(t *testing.T, requests <-chan string) string {
	t.Helper()
	select {
	case cookie := <-requests:
		return cookie
	case <-time.After(3 * time.Second):
		t.Fatal("XHR request did not reach the server")
		return ""
	}
}

func securityCookieValues(cookies []net.Cookie) string {
	values := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		values = append(values, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(values, "; ")
}

func securityURLHost(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Hostname()
}
