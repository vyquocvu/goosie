package net

import (
	"context"
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
	"syscall"
	"time"
	"unicode"
)

// MaxResponseBytes bounds each decoded HTTP response or local file.
const MaxResponseBytes = 8 << 20

// ErrResponseTooLarge reports a response that exceeds MaxResponseBytes.
var ErrResponseTooLarge = errors.New("net: response exceeds 8 MiB limit")

// HTTP is the 4-method interface the engine depends on. Declaring it here
// rather than using *http.Client directly keeps navigation mockable in tests.
type HTTP interface {
	Get(ctx context.Context, url string) (*Response, error)
	Post(ctx context.Context, url, contentType string, body io.Reader) (*Response, error)
	SetTimeout(d time.Duration)
	Close() error
}

// Response is a simplified HTTP response. Repeated header values are joined
// with a comma and space, retaining all values in the string-valued map.
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	URL        string
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
	default:
		return "", errors.New("net: unsupported URL scheme")
	}
	return u.String(), nil
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

// DefaultClient returns an HTTP client backed by net/http with standard TLS
// verification and at most ten validated HTTP(S) redirects.
func DefaultClient() HTTP {
	return &httpClient{client: &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 10 {
				return errors.New("net: too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("net: redirect requires HTTP(S)")
			}
			_, err := NormalizeURL(req.URL.String())
			return err
		},
	}}
}

type httpClient struct {
	client *http.Client
}

func (c *httpClient) Get(ctx context.Context, raw string) (*Response, error) {
	return c.request(ctx, http.MethodGet, raw, "", nil)
}

func (c *httpClient) Post(ctx context.Context, raw, contentType string, body io.Reader) (*Response, error) {
	return c.request(ctx, http.MethodPost, raw, contentType, body)
}

func (c *httpClient) request(ctx context.Context, method, raw, contentType string, body io.Reader) (*Response, error) {
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
		return readFileURL(ctx, u)
	}
	req, err := http.NewRequestWithContext(ctx, method, normalized, body)
	if err != nil {
		return nil, err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
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
		headers[k] = strings.Join(values, ", ")
	}
	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       data,
		URL:        resp.Request.URL.String(),
	}, nil
}

func (c *httpClient) SetTimeout(d time.Duration) {
	c.client.Timeout = d
}

func (c *httpClient) Close() error {
	c.client.CloseIdleConnections()
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
	// Check before opening so devices and FIFOs are never deliberately opened.
	info, err := os.Stat(u.Path)
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
	file, err := os.OpenFile(u.Path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
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
	contentType := mime.TypeByExtension(filepath.Ext(u.Path))
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
