package net

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	stdnet "net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"

	htmlcharset "golang.org/x/net/html/charset"
	"golang.org/x/net/idna"
)

const MaxResponseBytes = 16 << 20

const MaxResponseHeaderBytes = 256 << 10

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Goosie/2.0"

var ErrResponseTooLarge = errors.New("net: response exceeds 16 MiB limit")

type HTTP interface {
	Get(ctx context.Context, url string) (*Response, error)
	Post(ctx context.Context, url, contentType string, body io.Reader) (*Response, error)
	SetTimeout(d time.Duration)
	Close() error
	Jar() *CookieJar
	NetworkLog() *NetworkLog
}

type StreamingHTTP interface {
	GetStream(ctx context.Context, url string) (*StreamResponse, error)
}

type RangeStreamingHTTP interface {
	GetStreamRange(ctx context.Context, url string, start int64) (*StreamResponse, error)
}

type CacheClearingHTTP interface {
	ClearCache()
}

type StreamResponse struct {
	StatusCode    int
	Headers       map[string]string
	HeaderValues  map[string][]string
	URL           string
	ContentLength int64
	Body          io.ReadCloser
	cancel        context.CancelFunc
	closeOnce     sync.Once
	closeErr      error
}

func (r *StreamResponse) Cancel() {
	if r != nil && r.cancel != nil {
		r.cancel()
	}
}

func (r *StreamResponse) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.Cancel()
		if r.Body != nil {
			r.closeErr = r.Body.Close()
		}
	})
	return r.closeErr
}

func (r *StreamResponse) ReadBounded(maxBytes int64) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, errors.New("net: response stream is closed")
	}
	defer r.Close()
	if maxBytes < 0 {
		return nil, errors.New("net: invalid response limit")
	}
	if r.ContentLength > maxBytes {
		return nil, ErrResponseTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if len(data) > int(maxBytes) {
		return nil, ErrResponseTooLarge
	}
	if err != nil {
		return nil, fmt.Errorf("net: reading body: %w", err)
	}
	return data, nil
}

type RequestTrace struct {
	URL           string        `json:"url"`
	Method        string        `json:"method"`
	StatusCode    int           `json:"statusCode"`
	ContentType   string        `json:"contentType"`
	Bytes         int           `json:"bytes"`
	Duration      time.Duration `json:"duration"`
	Timestamp     time.Time     `json:"timestamp"`
	Blocked       bool          `json:"blocked,omitempty"`
	BlockedReason string        `json:"blockedReason,omitempty"`
}

type NetworkLog struct {
	mu      sync.RWMutex
	traces  []RequestTrace
	maxSize int
}

func NewNetworkLog(maxSize int) *NetworkLog {
	if maxSize <= 0 {
		maxSize = 200
	}
	return &NetworkLog{maxSize: maxSize}
}

func (nl *NetworkLog) Record(t RequestTrace) {
	nl.mu.Lock()
	defer nl.mu.Unlock()
	if len(nl.traces) >= nl.maxSize {
		nl.traces = nl.traces[1:]
	}
	nl.traces = append(nl.traces, t)
}

func (nl *NetworkLog) List() []RequestTrace {
	nl.mu.RLock()
	defer nl.mu.RUnlock()
	out := make([]RequestTrace, len(nl.traces))
	copy(out, nl.traces)
	return out
}

func (nl *NetworkLog) Clear() {
	nl.mu.Lock()
	defer nl.mu.Unlock()
	nl.traces = nil
}

type Response struct {
	StatusCode   int
	Headers      map[string]string
	HeaderValues map[string][]string
	Body         []byte
	URL          string
}

func headerMaps(header http.Header) (map[string]string, map[string][]string) {
	headers := make(map[string]string, len(header))
	valuesByKey := make(map[string][]string, len(header))
	for k, values := range header {
		valuesByKey[k] = append([]string(nil), values...)
		if strings.EqualFold(k, "Set-Cookie") {
			headers[k] = strings.Join(values, "\n")
		} else {
			headers[k] = strings.Join(values, ", ")
		}
	}
	return headers, valuesByKey
}

func StyleSheetRefused(contentType string) bool {
	mt := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if mt == "" || mt == "text/css" {
		return false
	}
	for _, prefix := range []string{"text/html", "application/xhtml", "text/xml", "application/xml",
		"image/", "font/", "audio/", "video/"} {
		if strings.HasPrefix(mt, prefix) {
			return true
		}
	}
	for _, script := range []string{"javascript", "ecmascript", "json"} {
		if strings.HasSuffix(mt, "/"+script) {
			return true
		}
	}
	return false
}

func (r *Response) Text() string {
	text, _, _ := r.TextWithEncoding()
	return text
}

func (r *Response) TextWithEncoding() (text, label string, certain bool) {
	contentType := ""
	if r.Headers != nil {
		contentType = r.Headers["Content-Type"]
	}
	return DecodeHTML(r.Body, contentType)
}

func DecodeHTML(body []byte, contentType string) (text, label string, certain bool) {
	_, label, certain = htmlcharset.DetermineEncoding(body, contentType)
	reader, err := htmlcharset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return string(body), label, certain
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return string(body), label, certain
	}
	return strings.TrimPrefix(string(decoded), "\uFEFF"), label, certain
}

func NormalizeURL(raw string) (string, error) {
	if hasControl(raw) {
		return "", errors.New("net: URL contains control characters")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "/") {
		return "", errors.New("net: expected a URL or hostname")
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil || hasControl(decoded) {
		return "", errors.New("net: invalid URL escaping or control characters")
	}

	lower := strings.ToLower(raw)
	if !strings.HasPrefix(lower, "http:") && !strings.HasPrefix(lower, "https:") && !strings.HasPrefix(lower, "file:") {
		if strings.Contains(raw, "://") {
			return "", errors.New("net: unsupported URL scheme")
		}
		authority := strings.FieldsFunc(raw, func(r rune) bool { return r == '/' || r == '?' || r == '#' })[0]
		if colon := strings.IndexByte(authority, ':'); colon >= 0 {
			host := authority[:colon]
			if !strings.EqualFold(host, "localhost") && !strings.Contains(host, ".") && !strings.HasPrefix(authority, "[") {
				return "", errors.New("net: unsupported URL scheme")
			}
		}
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("net: parsing URL: %w", err)
	}
	if u.User != nil || u.Opaque != "" {
		return "", errors.New("net: credentials and opaque URLs are not supported")
	}
	if u.Scheme == "http" || u.Scheme == "https" {
		asciiHost, err := idnaHost(u.Hostname())
		if err != nil {
			return "", err
		}
		if asciiHost != u.Hostname() {
			if port := u.Port(); port != "" {
				u.Host = stdnet.JoinHostPort(asciiHost, port)
			} else {
				u.Host = asciiHost
			}
		}
	}
	switch u.Scheme {
	case "http", "https":
		if err := validateHTTPHost(u); err != nil {
			return "", err
		}
	case "file":
		if (u.Host != "" && !strings.EqualFold(u.Host, "localhost")) || !filepath.IsAbs(u.Path) || u.RawQuery != "" || u.ForceQuery {
			return "", errors.New("net: file URL requires a local authority, absolute path, and no query")
		}
		u.Path = filepath.Clean(u.Path)
	default:
		return "", errors.New("net: unsupported URL scheme")
	}
	return u.String(), nil
}

func scrubURLError(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		uerr.URL = RedactURL(uerr.URL)
	}
	return err
}

func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" && u.Host == "" && u.Path == "" && u.Opaque == "" {
		return "(unparseable URL)"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	u.RawFragment = ""
	return u.String()
}

func hasControl(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

func idnaHost(host string) (string, error) {
	if host == "" || isASCII(host) {
		return host, nil
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return "", fmt.Errorf("net: invalid international hostname: %w", err)
	}
	return ascii, nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func validateHTTPHost(u *url.URL) error {
	host := u.Hostname()
	if host == "" {
		return errors.New("net: URL has no hostname")
	}
	if strings.HasPrefix(u.Host, "[") {
		if !strings.Contains(host, ":") || stdnet.ParseIP(host) == nil {
			return errors.New("net: invalid IPv6 hostname")
		}
	} else if strings.Contains(host, ":") {
		return errors.New("net: IPv6 hostname must be bracketed")
	} else if stdnet.ParseIP(host) == nil {
		name := strings.TrimSuffix(host, ".")
		if len(name) == 0 || len(name) > 253 {
			return errors.New("net: invalid hostname length")
		}
		numeric := true
		for _, label := range strings.Split(name, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return errors.New("net: invalid hostname label")
			}
			for _, c := range label {
				if c < '0' || c > '9' {
					numeric = false
				}
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
					return errors.New("net: invalid hostname character")
				}
			}
		}
		if numeric && strings.Contains(name, ".") {
			return errors.New("net: invalid IPv4 hostname")
		}
	}
	if strings.HasSuffix(u.Host, ":") {
		return errors.New("net: empty port")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return errors.New("net: invalid port")
		}
	}
	return nil
}

func clientTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxResponseHeaderBytes = MaxResponseHeaderBytes
	t.DialContext = dialAddressGuarded
	return t
}

func DefaultClient() HTTP {
	c := &httpClient{
		jar:    NewCookieJar(),
		cache:  newResponseCache(),
		filter: NewFilterEngine(),
		netLog: NewNetworkLog(200),
	}
	c.client = &http.Client{
		Timeout:       30 * time.Second,
		Transport:     clientTransport(),
		CheckRedirect: c.checkRedirect,
	}
	return c
}

func DefaultPrivateClient() HTTP {
	c := &httpClient{
		jar:     NewCookieJar(),
		cache:   newResponseCache(),
		filter:  NewFilterEngine(),
		netLog:  NewNetworkLog(200),
		private: true,
	}
	c.client = &http.Client{
		Timeout:       30 * time.Second,
		Transport:     clientTransport(),
		CheckRedirect: c.checkRedirect,
	}
	return c
}

func DefaultClientWithTLS(tlsConfig *tls.Config) HTTP {
	t := clientTransport()
	t.TLSClientConfig = tlsConfig
	c := &httpClient{
		jar:    NewCookieJar(),
		cache:  newResponseCache(),
		filter: NewFilterEngine(),
		netLog: NewNetworkLog(200),
	}
	c.client = &http.Client{
		Timeout:       30 * time.Second,
		Transport:     t,
		CheckRedirect: c.checkRedirect,
	}
	return c
}

func DefaultClientWithCookies(cookiesPath string) (HTTP, error) {
	c := DefaultClient().(*httpClient)
	if err := c.jar.Load(cookiesPath); err != nil {
		return nil, err
	}
	c.persistPath = cookiesPath
	return c, nil
}

func (c *httpClient) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 10 {
		return errors.New("net: too many redirects")
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return errors.New("net: redirect requires HTTP(S)")
	}
	if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme == "http" {
		return errors.New("net: refusing HTTPS-to-HTTP downgrade redirect")
	}
	if req.Response != nil && req.Response.Request != nil && c.jar != nil && cookiesAllowedForURL(req.Context(), req.Response.Request.URL) {
		if values := req.Response.Header.Values("Set-Cookie"); len(values) > 0 {
			c.jar.Store(req.Response.Request.URL, values)
		}
	}
	if cookiesAllowedForURL(req.Context(), req.URL) {
		if c.jar != nil && req.Header != nil {
			if cookie := c.jar.HeaderFor(req.URL); cookie != "" {
				req.Header.Set("Cookie", cookie)
			} else {
				req.Header.Del("Cookie")
			}
		}
	} else if req.Header != nil {
		req.Header.Del("Cookie")
	}
	_, err := NormalizeURL(req.URL.String())
	return err
}

type httpClient struct {
	client      *http.Client
	jar         *CookieJar
	cache       *responseCache
	filter      *FilterEngine
	netLog      *NetworkLog
	private     bool
	persistPath string
	hstsMu      sync.Mutex
	hsts        map[string]time.Time
}

func (c *httpClient) NetworkLog() *NetworkLog {
	return c.netLog
}

func (c *httpClient) Jar() *CookieJar {
	return c.jar
}

func (c *httpClient) ClearCache() {
	if c.cache != nil {
		c.cache.clear()
	}
}

func (c *httpClient) Filter() *FilterEngine {
	return c.filter
}

func (c *httpClient) SetFilter(f *FilterEngine) {
	c.filter = f
}

func (c *httpClient) noteHSTS(reqURL *url.URL, header http.Header) {
	if reqURL == nil || reqURL.Scheme != "https" {
		return
	}
	host := strings.ToLower(reqURL.Hostname())
	if host == "" {
		return
	}
	maxAge := int64(-1)
	for _, part := range strings.Split(header.Get("Strict-Transport-Security"), ";") {
		part = strings.TrimSpace(part)
		if len(part) > 8 && strings.EqualFold(part[:8], "max-age=") {
			if n, err := strconv.ParseInt(strings.TrimSpace(part[8:]), 10, 64); err == nil && n >= 0 {
				maxAge = n
			}
		}
	}
	if maxAge < 0 {
		return
	}
	c.hstsMu.Lock()
	defer c.hstsMu.Unlock()
	if maxAge == 0 {
		delete(c.hsts, host)
		return
	}
	if c.hsts == nil {
		c.hsts = make(map[string]time.Time)
	}
	c.hsts[host] = time.Now().Add(time.Duration(maxAge) * time.Second)
}

func (c *httpClient) upgradeHSTS(u *url.URL) {
	if u == nil || u.Scheme != "http" {
		return
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return
	}
	c.hstsMu.Lock()
	defer c.hstsMu.Unlock()
	exp, ok := c.hsts[host]
	if !ok {
		return
	}
	if time.Now().After(exp) {
		delete(c.hsts, host)
		return
	}
	u.Scheme = "https"
}

func (c *httpClient) Get(ctx context.Context, raw string) (*Response, error) {
	return c.request(ctx, http.MethodGet, raw, "", nil)
}

func (c *httpClient) GetStream(ctx context.Context, raw string) (*StreamResponse, error) {
	return c.getStream(ctx, raw, 0)
}

func (c *httpClient) GetStreamRange(ctx context.Context, raw string, start int64) (*StreamResponse, error) {
	if start < 0 {
		return nil, errors.New("net: invalid range offset")
	}
	return c.getStream(ctx, raw, start)
}

func (c *httpClient) getStream(ctx context.Context, raw string, start int64) (*StreamResponse, error) {
	startedAt := time.Now()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalized, err := NormalizeURL(raw)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(normalized)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("net: streaming supports HTTP(S) URLs only")
	}
	if c.filter != nil && c.filter.IsEnabled() && !IsShieldsDisabled(ctx) {
		if blocked, reason := c.filter.ShouldBlock(normalized); blocked {
			if c.netLog != nil {
				c.netLog.Record(RequestTrace{URL: normalized, Method: http.MethodGet, Blocked: true, BlockedReason: reason, Timestamp: startedAt})
			}
			return nil, fmt.Errorf("%w: %s (%s)", ErrBlockedByFilter, RedactURL(normalized), reason)
		}
	}
	if start == 0 && c.cache != nil && !hasCookiePolicy(ctx) {
		if cached, ok := c.cache.get(normalized, time.Now()); ok {
			if c.netLog != nil {
				c.netLog.Record(RequestTrace{
					URL:         normalized,
					Method:      http.MethodGet,
					StatusCode:  304,
					ContentType: cached.Headers["Content-Type"],
					Bytes:       len(cached.Body),
					Timestamp:   startedAt,
				})
			}
			return &StreamResponse{
				StatusCode:    cached.StatusCode,
				Headers:       cached.Headers,
				HeaderValues:  cached.HeaderValues,
				URL:           cached.URL,
				ContentLength: int64(len(cached.Body)),
				Body:          io.NopCloser(bytes.NewReader(cached.Body)),
			}, nil
		}
	}
	streamCtx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, normalized, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("net: request %s: %w", RedactURL(normalized), scrubURLError(err))
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if cookiesAllowedForURL(ctx, u) {
		if cookie := c.jar.HeaderFor(u); cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
	}
	if start > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	}
	resp, err := c.client.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("net: fetch %s: %w", RedactURL(normalized), scrubURLError(err))
	}
	headers, headerValues := headerMaps(resp.Header)
	if c.jar != nil && cookiesAllowedForURL(ctx, resp.Request.URL) {
		if values := resp.Header.Values("Set-Cookie"); len(values) > 0 {
			c.jar.Store(resp.Request.URL, values)
		}
	}
	body := &streamLogBody{
		ReadCloser: resp.Body,
		log:        c.netLog,
		trace: RequestTrace{
			URL:         normalized,
			Method:      http.MethodGet,
			StatusCode:  resp.StatusCode,
			ContentType: resp.Header.Get("Content-Type"),
			Timestamp:   startedAt,
		},
		started: startedAt,
	}
	return &StreamResponse{
		StatusCode:    resp.StatusCode,
		Headers:       headers,
		HeaderValues:  headerValues,
		URL:           resp.Request.URL.String(),
		ContentLength: resp.ContentLength,
		Body:          body,
		cancel:        cancel,
	}, nil
}

type streamLogBody struct {
	io.ReadCloser
	log     *NetworkLog
	trace   RequestTrace
	started time.Time
	bytes   atomic.Int64
	once    sync.Once
}

func (b *streamLogBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.bytes.Add(int64(n))
	if err != nil {
		b.record()
	}
	return n, err
}

func (b *streamLogBody) Close() error {
	b.record()
	return b.ReadCloser.Close()
}

func (b *streamLogBody) record() {
	b.once.Do(func() {
		if b.log != nil {
			trace := b.trace
			trace.Bytes = int(b.bytes.Load())
			trace.Duration = time.Since(b.started)
			b.log.Record(trace)
		}
	})
}

func (c *httpClient) Post(ctx context.Context, raw, contentType string, body io.Reader) (*Response, error) {
	return c.request(ctx, http.MethodPost, raw, contentType, body)
}

func (c *httpClient) request(ctx context.Context, method, raw, contentType string, body io.Reader) (*Response, error) {
	start := time.Now()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalized, err := NormalizeURL(raw)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(normalized)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "file" {
		if method != http.MethodGet {
			return nil, errors.New("net: only GET supports file URLs")
		}
		res, err := readFileURL(ctx, u)
		if c.netLog != nil && err == nil {
			c.netLog.Record(RequestTrace{
				URL:         normalized,
				Method:      method,
				StatusCode:  200,
				ContentType: res.Headers["Content-Type"],
				Bytes:       len(res.Body),
				Duration:    time.Since(start),
				Timestamp:   start,
			})
		}
		return res, err
	}
	if c.filter != nil && c.filter.IsEnabled() && !IsShieldsDisabled(ctx) {
		if blocked, reason := c.filter.ShouldBlock(normalized); blocked {

			if c.netLog != nil {
				c.netLog.Record(RequestTrace{
					URL:           normalized,
					Method:        method,
					Blocked:       true,
					BlockedReason: reason,
					Timestamp:     start,
				})
			}
			return nil, fmt.Errorf("%w: %s (%s)", ErrBlockedByFilter, RedactURL(normalized), reason)
		}
	}
	if method == http.MethodGet && c.cache != nil && !hasCookiePolicy(ctx) {
		if resp, ok := c.cache.get(normalized, time.Now()); ok {
			if c.netLog != nil {
				c.netLog.Record(RequestTrace{
					URL:         normalized,
					Method:      method,
					StatusCode:  304,
					ContentType: resp.Headers["Content-Type"],
					Bytes:       len(resp.Body),
					Duration:    time.Since(start),
					Timestamp:   start,
				})
			}
			return resp, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, normalized, body)
	if err != nil {
		return nil, fmt.Errorf("net: request %s: %w", RedactURL(normalized), scrubURLError(err))
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if cookiesAllowedForURL(ctx, u) {
		if cookie := c.jar.HeaderFor(u); cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("net: fetch %s: %w", RedactURL(normalized), scrubURLError(err))
	}
	defer resp.Body.Close()
	if resp.ContentLength > MaxResponseBytes {
		return nil, ErrResponseTooLarge
	}
	data, err := readBounded(ctx, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("net: reading body: %w", err)
	}
	headers, headerValues := headerMaps(resp.Header)
	if c.jar != nil && cookiesAllowedForURL(ctx, resp.Request.URL) {
		if values := resp.Header.Values("Set-Cookie"); len(values) > 0 {
			c.jar.Store(resp.Request.URL, values)
		}
	}
	out := &Response{
		StatusCode:   resp.StatusCode,
		Headers:      headers,
		HeaderValues: headerValues,
		Body:         data,
		URL:          resp.Request.URL.String(),
	}
	if method == http.MethodGet && c.cache != nil && !hasCookiePolicy(ctx) && len(resp.Header.Values("Set-Cookie")) == 0 {
		c.cache.store(normalized, out, time.Now())
	}
	if c.netLog != nil {
		c.netLog.Record(RequestTrace{
			URL:         normalized,
			Method:      method,
			StatusCode:  resp.StatusCode,
			ContentType: resp.Header.Get("Content-Type"),
			Bytes:       len(data),
			Duration:    time.Since(start),
			Timestamp:   start,
		})
	}
	return out, nil
}

func (c *httpClient) SetTimeout(d time.Duration) {
	c.client.Timeout = d
}

func (c *httpClient) Close() error {
	c.client.CloseIdleConnections()
	if c.persistPath != "" {
		if err := c.jar.Save(c.persistPath); err != nil {
			return err
		}
	}
	if c.private {
		c.jar.Clear()
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if canceled := r.ctx.Err(); canceled != nil {
		return n, canceled
	}
	return n, err
}

func readBounded(ctx context.Context, r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, r: r}, MaxResponseBytes+1))
	if canceled := ctx.Err(); canceled != nil {
		return nil, canceled
	}
	if len(data) > MaxResponseBytes {
		return nil, ErrResponseTooLarge
	}
	return data, err
}

func readFileURL(ctx context.Context, u *url.URL) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := filepath.Clean(u.Path)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("net: stat file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("net: file URL requires a regular file")
	}
	if info.Size() > MaxResponseBytes {
		return nil, ErrResponseTooLarge
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("net: opening file: %w", err)
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return nil, fmt.Errorf("net: stat open file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("net: file URL requires a regular file")
	}
	if info.Size() > MaxResponseBytes {
		return nil, ErrResponseTooLarge
	}
	data, err := readBounded(ctx, file)
	if err != nil {
		return nil, fmt.Errorf("net: reading file: %w", err)
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return &Response{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": contentType},
		Body:       data,
		URL:        u.String(),
	}, nil
}
