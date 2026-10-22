package net

import (
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
	"syscall"
	"time"
	"unicode"
)

// MaxResponseBytes bounds each decoded HTTP response or local file.
const MaxResponseBytes = 16 << 20

// MaxResponseHeaderBytes bounds one response's header block, which the transport
// would otherwise buffer up to its own 10 MiB default before any body byte.
const MaxResponseHeaderBytes = 256 << 10

// userAgent is sent with every request. We identify as Goosie rather than
// spoofing Chrome, because Cloudflare and other CDNs detect TLS fingerprint
// mismatches (Go crypto/tls vs Chrome JA3/JA4) and trigger bot challenges.
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Goosie/2.0"

// ErrResponseTooLarge reports a response that exceeds MaxResponseBytes.
var ErrResponseTooLarge = errors.New("net: response exceeds 16 MiB limit")

// HTTP is the 4-method interface the engine depends on. Declaring it here
// rather than using *http.Client directly keeps navigation mockable in tests.
type HTTP interface {
	Get(ctx context.Context, url string) (*Response, error)
	Post(ctx context.Context, url, contentType string, body io.Reader) (*Response, error)
	SetTimeout(d time.Duration)
	Close() error
	Jar() *CookieJar
	NetworkLog() *NetworkLog
}

// RequestTrace represents one recorded HTTP network transaction.
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

// NetworkLog records recent network transactions in a thread-safe circular buffer.
type NetworkLog struct {
	mu      sync.RWMutex
	traces  []RequestTrace
	maxSize int
}

// NewNetworkLog creates a new network log capped at maxSize entries.
func NewNetworkLog(maxSize int) *NetworkLog {
	if maxSize <= 0 {
		maxSize = 200
	}
	return &NetworkLog{maxSize: maxSize}
}

// Record appends a request trace to the log.
func (nl *NetworkLog) Record(t RequestTrace) {
	nl.mu.Lock()
	defer nl.mu.Unlock()
	if len(nl.traces) >= nl.maxSize {
		nl.traces = nl.traces[1:]
	}
	nl.traces = append(nl.traces, t)
}

// List returns a copy of recorded traces in chronological order.
func (nl *NetworkLog) List() []RequestTrace {
	nl.mu.RLock()
	defer nl.mu.RUnlock()
	out := make([]RequestTrace, len(nl.traces))
	copy(out, nl.traces)
	return out
}

// Clear resets the recorded traces.
func (nl *NetworkLog) Clear() {
	nl.mu.Lock()
	defer nl.mu.Unlock()
	nl.traces = nil
}

// Response is a simplified HTTP response. Repeated header values are joined
// with a comma and space, retaining all values in the string-valued map.
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	URL        string
}

// StyleSheetRefused reports whether a response with this Content-Type must be
// kept out of the cascade. A broken CDN path answers a stylesheet request with
// an HTML error page, and that page's own rules parse as valid CSS: applied
// silently, they restyle the real document. Chromium refuses a sheet whose type
// it can classify as something else, inferring CSS only from a type it does not
// recognise, so this follows the same split.
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

// Text returns the body as UTF-8 text, decoded from whatever single-byte
// charset the response declares. The parser downstream assumes UTF-8, and the
// web's remaining non-UTF-8 pages are nearly all Latin-1/Windows-1252;
// anything else passes through byte-for-byte, which is what UTF-8 source
// wants and what unknown legacy bytes degrade to.
func (r *Response) Text() string {
	charset := ""
	if ct := r.Headers["Content-Type"]; ct != "" {
		if _, params, err := mime.ParseMediaType(ct); err == nil {
			charset = normalizeCharset(params["charset"])
		}
	}
	if !isLegacy8Bit(charset) {
		charset = sniffCharset(r.Body)
	}
	if isLegacy8Bit(charset) {
		return decodeCP1252(r.Body)
	}
	return string(r.Body)
}

func normalizeCharset(s string) string {
	s = strings.ToLower(strings.Trim(strings.TrimSpace(s), `"'`))
	s = strings.ReplaceAll(s, "_", "-")
	switch s {
	case "latin1", "iso8859-1", "iso-8859", "8859-1":
		return "iso-8859-1"
	case "cp1252", "windows1252", "1252":
		return "windows-1252"
	case "ascii", "us":
		return "us-ascii"
	}
	return s
}

func isLegacy8Bit(charset string) bool {
	switch charset {
	case "iso-8859-1", "windows-1252", "us-ascii":
		return true
	}
	return false
}

// sniffCharset implements the practical half of the HTML encoding sniffing:
// a charset= declaration in the first 2 KiB of markup.
func sniffCharset(body []byte) string {
	head := body
	if len(head) > 2048 {
		head = head[:2048]
	}
	lower := strings.ToLower(string(head))
	i := strings.Index(lower, "charset=")
	if i < 0 {
		return ""
	}
	rest := lower[i+len("charset="):]
	end := len(rest)
	for j, c := range rest {
		if c == '"' || c == '\'' || c == ' ' || c == ';' || c == '>' || c == '/' {
			end = j
			break
		}
	}
	return normalizeCharset(rest[:end])
}

// cp1252High maps the 0x80-0x9F range, where Windows-1252 diverges from
// Latin-1 with printables. Latin-1's identity mapping covers 0xA0 and above,
// so one table serves both.
var cp1252High = [32]rune{
	0x20AC, 0x0081, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021,
	0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0x008D, 0x017D, 0x008F,
	0x0090, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0x009D, 0x017E, 0x0178,
}

func decodeCP1252(body []byte) string {
	var b strings.Builder
	b.Grow(len(body) + len(body)/8)
	for _, c := range body {
		switch {
		case c < 0x80:
			b.WriteByte(c)
		case c < 0xA0:
			b.WriteRune(cp1252High[c-0x80])
		default:
			b.WriteRune(rune(c))
		}
	}
	return b.String()
}

// NormalizeURL validates explicit HTTP(S) and local file URLs, and gives bare
// domains and localhost addresses an HTTPS scheme. It does not access the URL.
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

// scrubURLError rewrites *url.Error values to carry the redacted URL. The
// stdlib formats request errors with the full request URL inline
// (Get "https://host/path?secret=...": ...), so wrapping is not enough: the
// inner text must be scrubbed where the error is made. Values are fresh per
// call, so mutating in place is safe.
func scrubURLError(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		uerr.URL = RedactURL(uerr.URL)
	}
	return err
}

// RedactURL strips what diagnostics must never carry: userinfo credentials
// and query/fragment secrets. Scheme, host and path stay for diagnosis.
// Unparseable input reports a fixed token rather than echoing (which could
// itself be the secret). Callers formatting URLs into errors and logs use
// this, not the raw string.
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

// clientTransport is the shared transport's own proxy, dialing, pooling and
// HTTP/2 behaviour with this browser's response-header bound set on top. It is
// a Clone rather than a literal so that the inherited settings stay inherited:
// a hand-built Transport silently drops HTTP_PROXY support.
func clientTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxResponseHeaderBytes = MaxResponseHeaderBytes
	t.DialContext = dialAddressGuarded
	return t
}

// DefaultClient returns an HTTP client backed by net/http with standard TLS
// verification and at most ten validated HTTP(S) redirects. Its cookie jar
// lives in memory for the life of the client.
func DefaultClient() HTTP {
	return &httpClient{
		client: &http.Client{
			Timeout:       30 * time.Second,
			Transport:     clientTransport(),
			CheckRedirect: checkRedirect,
		},
		jar:    NewCookieJar(),
		cache:  newResponseCache(),
		filter: NewFilterEngine(),
		netLog: NewNetworkLog(200),
	}
}

// DefaultPrivateClient is DefaultClient with an ephemeral cookie jar that is
// wiped on Close: nothing a private-mode session learns survives it.
func DefaultPrivateClient() HTTP {
	return &httpClient{
		client: &http.Client{
			Timeout:       30 * time.Second,
			Transport:     clientTransport(),
			CheckRedirect: checkRedirect,
		},
		jar:     NewCookieJar(),
		cache:   newResponseCache(),
		filter:  NewFilterEngine(),
		netLog:  NewNetworkLog(200),
		private: true,
	}
}

// DefaultClientWithTLS returns an HTTP client identical to DefaultClient but
// using a caller-supplied TLS configuration. Tests use it to trust the
// self-signed certificates httptest.NewTLSServer generates.
func DefaultClientWithTLS(tlsConfig *tls.Config) HTTP {
	t := clientTransport()
	t.TLSClientConfig = tlsConfig
	return &httpClient{
		client: &http.Client{
			Timeout:       30 * time.Second,
			Transport:     t,
			CheckRedirect: checkRedirect,
		},
		jar:    NewCookieJar(),
		cache:  newResponseCache(),
		filter: NewFilterEngine(),
		netLog: NewNetworkLog(200),
	}
}

// DefaultClientWithCookies returns DefaultClient whose jar persists to
// cookiesPath: cookies saved by an earlier run load at construction, and
// persistent cookies save back on Close. A missing file is the normal first
// run. A file that exists but cannot be read or parsed is an error, and no
// save happens on Close — a broken store must not be silently truncated.
func DefaultClientWithCookies(cookiesPath string) (HTTP, error) {
	c := DefaultClient().(*httpClient)
	if err := c.jar.Load(cookiesPath); err != nil {
		return nil, err
	}
	c.persistPath = cookiesPath
	return c, nil
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 10 {
		return errors.New("net: too many redirects")
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return errors.New("net: redirect requires HTTP(S)")
	}
	// A secure origin never downgrades mid-navigation: an https page that
	// bounces through http exposes the session to the network it just left.
	if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme == "http" {
		return errors.New("net: refusing HTTPS-to-HTTP downgrade redirect")
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
	// hsts remembers Strict-Transport-Security max-age per exact host for
	// the client's lifetime (memory only; persistence rides with profiles
	// in gate 5). includeSubDomains is not honored: subdomains keep their
	// own entries rather than inheriting trust they never earned.
	hstsMu sync.Mutex
	hsts   map[string]time.Time
}

// NetworkLog returns the client's network transaction log.
func (c *httpClient) NetworkLog() *NetworkLog {
	return c.netLog
}

// Jar returns the client's cookie jar.
func (c *httpClient) Jar() *CookieJar {
	return c.jar
}

// Filter returns the client's ad/tracker filter engine.
func (c *httpClient) Filter() *FilterEngine {
	return c.filter
}

// SetFilter replaces the client's ad/tracker filter engine.
func (c *httpClient) SetFilter(f *FilterEngine) {
	c.filter = f
}

// noteHSTS records one response's transport-security policy: an https
// response bearing max-age pins its exact host until the age lapses, and
// max-age=0 unpins it. Anything else (http delivery, unparsable age)
// changes nothing.
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

// upgradeHSTS rewrites an http URL to https when its exact host is pinned.
// Lapsed pins are swept on sight rather than on a timer.
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
	if method == http.MethodGet && c.cache != nil {
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
	// A plain Go client identifies itself as Go-http-client and real sites
	// (Wikipedia's robots policy among them) answer it with 403. Announcing a
	// browser-like UA and content acceptance is what gets the same bytes a
	// browser would render.
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if cookie := c.jar.HeaderFor(u); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		// The stdlib error embeds the request URL verbatim; redact before
		// it joins our chain, or query secrets ride along into diagnostics.
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
	headers := make(map[string]string, len(resp.Header))
	for k, values := range resp.Header {
		if strings.EqualFold(k, "Set-Cookie") {
			headers[k] = strings.Join(values, "\n")
			c.jar.Store(resp.Request.URL, values)
		} else {
			headers[k] = strings.Join(values, ", ")
		}
	}
	out := &Response{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       data,
		URL:        resp.Request.URL.String(),
	}
	if method == http.MethodGet && c.cache != nil {
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

// contextReader checks cancellation between reads as well as after a read.
// net/http also interrupts blocked network reads through the request context.
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
	// Check before opening so devices and FIFOs are never deliberately opened.
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
	// Nonblocking open prevents a replacement FIFO from hanging between Stat
	// and Open. Validate the opened descriptor again before reading any bytes.
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
