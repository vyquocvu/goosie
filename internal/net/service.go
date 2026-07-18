package net

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"
)

// DefaultMaxBodySize is the default maximum decompressed response body size
// (100 MB). Services created without an explicit MaxBodySize use this limit.
// Set MaxBodySize to a negative value to disable the limit entirely.
const DefaultMaxBodySize int64 = 100 * 1024 * 1024

// MaxDecompressionRatio is the maximum allowed ratio of decompressed bytes
// read to compressed bytes received (Content-Length). A ratio above this
// threshold indicates a decompression bomb. The check only applies when
// the server provides a non-negative Content-Length header.
const MaxDecompressionRatio int64 = 100

// ErrBodyTooLarge is returned when the response body exceeds the configured
// MaxBodySize. Callers can check it with errors.Is.
var ErrBodyTooLarge = errors.New("response body exceeds size limit")

// ErrDecompressedTooLarge is returned when the decompressed response body
// exceeds MaxDecompressionRatio times the compressed Content-Length,
// indicating a potential decompression bomb. Callers can check it with errors.Is.
var ErrDecompressedTooLarge = errors.New("response decompression ratio exceeded")

// limitedContextReader wraps an io.Reader with context cancellation awareness
// and optional byte limit enforcement. A limit of 0 means unlimited. When
// compressedSize is positive, it tracks the decompression ratio and returns
// ErrDecompressedTooLarge if decompressedBytes > compressedSize * ratio.
type limitedContextReader struct {
	ctx            context.Context
	reader         io.Reader
	limit          int64 // max bytes to read; 0 = unlimited
	read           int64 // bytes read so far
	compressedSize int64 // Content-Length (compressed); 0 or -1 = unknown
}

func (r *limitedContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.limit > 0 {
		remaining := r.limit - r.read
		if remaining <= 0 {
			return 0, ErrBodyTooLarge
		}
		if int64(len(p)) > remaining {
			p = p[:remaining]
		}
	}
	n, err := r.reader.Read(p)
	r.read += int64(n)
	// Decompression bomb check: if we know the compressed size and the
	// decompressed output exceeds the allowed ratio, abort immediately.
	if r.compressedSize > 0 && r.read > r.compressedSize*MaxDecompressionRatio {
		return 0, ErrDecompressedTooLarge
	}
	return n, err
}

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// maxRedirects is the maximum number of HTTP redirects the engine will
// follow for a single request, matching Chromium's and Edge's limit.
const maxRedirects = 20

type ServiceOptions struct {
	Client      *http.Client
	Cache       *HTTPCache
	UserAgent   string
	MaxBodySize int64 // maximum decompressed response body size in bytes; 0 = DefaultMaxBodySize, negative = unlimited
	// ExpectedContentType is a list of allowed Content-Type media types for
	// responses. When non-empty, each fetch method validates the response
	// Content-Type against this list and returns ErrUnsupportedMediaType on
	// mismatch. Empty means accept all types.
	ExpectedContentType []string
	TLSConfig           *tls.Config // TLS configuration for advanced scenarios
}

type Service struct {
	client              *http.Client
	cache               *HTTPCache
	userAgent           string
	log                 *RequestLog
	maxBodySize         int64
	expectedContentType []string

	securityMu sync.Mutex
	security   SecuritySummary

	// cspMu protects csp (read-only after FetchWithMeta sets it).
	cspMu sync.RWMutex
	csp   *CSPPolicy

	downloadsMu sync.Mutex
	downloads   []DownloadRecord
}

func NewService(options ServiceOptions) *Service {
	client := options.Client
	if client == nil {
		jar, _ := cookiejar.New(nil)
		transport := http.DefaultTransport.(*http.Transport).Clone()
		if options.TLSConfig != nil {
			transport.TLSClientConfig = options.TLSConfig
		}
		client = &http.Client{Jar: jar, Timeout: 30 * time.Second, Transport: transport}
	} else if options.TLSConfig != nil {
		if tr, ok := client.Transport.(*http.Transport); ok {
			tr.TLSClientConfig = options.TLSConfig
		} else if client.Transport == nil {
			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.TLSClientConfig = options.TLSConfig
			client.Transport = transport
		}
	}
	userAgent := options.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	maxBodySize := options.MaxBodySize
	if maxBodySize == 0 {
		maxBodySize = DefaultMaxBodySize
	}
	return &Service{
		client:              client,
		cache:               options.Cache,
		userAgent:           userAgent,
		log:                 NewRequestLog(),
		maxBodySize:         maxBodySize,
		expectedContentType: options.ExpectedContentType,
	}
}

func (s *Service) Fetch(rawURL string) (string, error) {
	return s.FetchWithContext(context.Background(), rawURL, nil)
}

// FetchWithMeta retrieves the content and captures immutable response metadata.
// The returned ResponseMeta is valid even after the response body is closed and
// includes status, headers, protocol, and cache-hit information.
func (s *Service) FetchWithMeta(ctx context.Context, rawURL string, onProgress ProgressCallback) (string, ResponseMeta, error) {
	startedAt := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		wrapped := fmt.Errorf("failed to create request: %w", err)
		s.log.Add(RequestLogEntry{Method: http.MethodGet, URL: rawURL, Error: wrapped.Error(), StartedAt: startedAt, Duration: time.Since(startedAt)})
		s.setSecurity(SecuritySummaryFromResponse(nil, wrapped))
		return "", ResponseMeta{Header: make(http.Header)}, wrapped
	}
	req.Header.Set("User-Agent", s.userAgent)
	hasCookies := s.client.Jar != nil && len(s.client.Jar.Cookies(req.URL)) > 0
	if !hasCookies {
		if body, entry, ok := s.cache.Get(rawURL); ok {
			s.setSecurity(securitySummaryFromURL(req.URL))
			s.log.Add(RequestLogEntry{
				Method:      http.MethodGet,
				URL:         rawURL,
				Status:      entry.Status,
				ContentType: entry.ContentType,
				Bytes:       int64(len(body)),
				CacheHit:    true,
				StartedAt:   startedAt,
				Duration:    time.Since(startedAt),
			})
			meta := ResponseMetaFromCacheEntry(entry)
			return body, meta, nil
		}
	}

	resp, redirectCount, err := s.doRequest(req)
	s.setSecurity(SecuritySummaryFromResponse(resp, err))
	if err != nil {
		wrapped := fmt.Errorf("failed to fetch URL: %w", err)
		s.log.Add(RequestLogEntry{Method: http.MethodGet, URL: rawURL, Error: wrapped.Error(), StartedAt: startedAt, Duration: time.Since(startedAt)})
		return "", ResponseMeta{Header: make(http.Header)}, wrapped
	}
	defer resp.Body.Close()

	// Capture metadata before body is consumed.
	meta := ResponseMetaFromResponse(resp)
	meta.RedirectCount = redirectCount
	if resp.Request != nil && resp.Request.URL != nil {
		meta.FinalURL = resp.Request.URL.String()
	}

	// Parse Content-Security-Policy from response headers.
	if cspHeader := resp.Header.Get("Content-Security-Policy"); cspHeader != "" {
		s.setCSP(ParseCSPHeader(cspHeader))
	} else {
		s.setCSP(nil)
	}

	// Validate Content-Type against expected types before reading the body.
	if err := s.validateContentType(meta.ContentType); err != nil {
		resp.Body.Close()
		entry := RequestLogEntry{
			Method:      http.MethodGet,
			URL:         rawURL,
			Status:      resp.StatusCode,
			ContentType: meta.ContentType,
			Error:       err.Error(),
			StartedAt:   startedAt,
			Duration:    time.Since(startedAt),
		}
		s.log.Add(entry)
		return "", meta, err
	}

	body, readErr := readResponseBody(ctx, resp, onProgress, s.maxBodySize)
	entry := RequestLogEntry{
		Method:      http.MethodGet,
		URL:         rawURL,
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Bytes:       int64(len(body)),
		StartedAt:   startedAt,
	}
	if readErr != nil {
		wrapped := fmt.Errorf("failed to read response body: %w", readErr)
		entry.Error = wrapped.Error()
		entry.Duration = time.Since(startedAt)
		s.log.Add(entry)
		return "", meta, wrapped
	}

	if resp.StatusCode >= 400 {
		if strings.TrimSpace(body) == "" {
			body = fmt.Sprintf(
				"<html><body><h1>%d %s</h1><p>The server returned an error.</p></body></html>",
				resp.StatusCode, http.StatusText(resp.StatusCode),
			)
			entry.Bytes = int64(len(body))
		}
		entry.Duration = time.Since(startedAt)
		s.log.Add(entry)
		return body, meta, nil
	}

	if !hasCookies && responseMatchesOriginalURL(rawURL, resp) {
		s.cache.Put(rawURL, resp, body)
	}
	entry.Duration = time.Since(startedAt)
	s.log.Add(entry)
	return body, meta, nil
}

func (s *Service) FetchWithContext(ctx context.Context, rawURL string, onProgress ProgressCallback) (string, error) {
	startedAt := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		wrapped := fmt.Errorf("failed to create request: %w", err)
		s.log.Add(RequestLogEntry{Method: http.MethodGet, URL: rawURL, Error: wrapped.Error(), StartedAt: startedAt, Duration: time.Since(startedAt)})
		s.setSecurity(SecuritySummaryFromResponse(nil, wrapped))
		return "", wrapped
	}
	req.Header.Set("User-Agent", s.userAgent)
	hasCookies := s.client.Jar != nil && len(s.client.Jar.Cookies(req.URL)) > 0
	if !hasCookies {
		if body, entry, ok := s.cache.Get(rawURL); ok {
			s.setSecurity(securitySummaryFromURL(req.URL))
			s.log.Add(RequestLogEntry{
				Method:      http.MethodGet,
				URL:         rawURL,
				Status:      entry.Status,
				ContentType: entry.ContentType,
				Bytes:       int64(len(body)),
				CacheHit:    true,
				StartedAt:   startedAt,
				Duration:    time.Since(startedAt),
			})
			return body, nil
		}
	}

	resp, _, err := s.doRequest(req)
	s.setSecurity(SecuritySummaryFromResponse(resp, err))
	if err != nil {
		wrapped := fmt.Errorf("failed to fetch URL: %w", err)
		s.log.Add(RequestLogEntry{Method: http.MethodGet, URL: rawURL, Error: wrapped.Error(), StartedAt: startedAt, Duration: time.Since(startedAt)})
		return "", wrapped
	}
	defer resp.Body.Close()

	// Parse Content-Security-Policy from response headers.
	if cspHeader := resp.Header.Get("Content-Security-Policy"); cspHeader != "" {
		s.setCSP(ParseCSPHeader(cspHeader))
	} else {
		s.setCSP(nil)
	}

	// Validate Content-Type against expected types before reading the body.
	respContentType := resp.Header.Get("Content-Type")
	if err := s.validateContentType(respContentType); err != nil {
		entry := RequestLogEntry{
			Method:      http.MethodGet,
			URL:         rawURL,
			Status:      resp.StatusCode,
			ContentType: respContentType,
			Error:       err.Error(),
			StartedAt:   startedAt,
			Duration:    time.Since(startedAt),
		}
		s.log.Add(entry)
		return "", err
	}

	body, readErr := readResponseBody(ctx, resp, onProgress, s.maxBodySize)
	entry := RequestLogEntry{
		Method:      http.MethodGet,
		URL:         rawURL,
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Bytes:       int64(len(body)),
		StartedAt:   startedAt,
	}
	if readErr != nil {
		wrapped := fmt.Errorf("failed to read response body: %w", readErr)
		entry.Error = wrapped.Error()
		entry.Duration = time.Since(startedAt)
		s.log.Add(entry)
		return "", wrapped
	}

	if resp.StatusCode >= 400 {
		if strings.TrimSpace(body) == "" {
			body = fmt.Sprintf(
				"<html><body><h1>%d %s</h1><p>The server returned an error.</p></body></html>",
				resp.StatusCode, http.StatusText(resp.StatusCode),
			)
			entry.Bytes = int64(len(body))
		}
		entry.Duration = time.Since(startedAt)
		s.log.Add(entry)
		return body, nil
	}

	if !hasCookies && responseMatchesOriginalURL(rawURL, resp) {
		s.cache.Put(rawURL, resp, body)
	}
	entry.Duration = time.Since(startedAt)
	s.log.Add(entry)
	return body, nil
}

func responseMatchesOriginalURL(rawURL string, resp *http.Response) bool {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return false
	}
	return normalizeCacheURL(resp.Request.URL.String()) == normalizeCacheURL(rawURL)
}

func (s *Service) Log() *RequestLog {
	return s.log
}

// CachedBody returns the cached response body for rawURL when a fresh cache
// entry exists. It lets developer tools (e.g. the Sources panel) display
// sub-resource content without issuing new network requests.
func (s *Service) CachedBody(rawURL string) (string, bool) {
	if s == nil || s.cache == nil {
		return "", false
	}
	body, _, ok := s.cache.Get(rawURL)
	return body, ok
}

// doRequest wraps http.Client.Do with a redirect policy that limits the
// number of redirects to maxRedirects and tracks how many were followed.
func (s *Service) doRequest(req *http.Request) (*http.Response, int, error) {
	var redirectCount int
	client := &http.Client{
		Transport: s.client.Transport,
		Jar:       s.client.Jar,
		Timeout:   s.client.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				redirectCount = maxRedirects
				return http.ErrUseLastResponse
			}
			redirectCount = len(via)
			return nil
		},
	}
	resp, err := client.Do(req)
	return resp, redirectCount, err
}

// FetchStream retrieves the response body as an io.ReadCloser without buffering
// the entire body into memory. The caller must close the returned body when done.
// Response metadata is captured before the body is returned. This eliminates the
// intermediate bytes.Buffer copy used by FetchWithMeta, enabling the HTML tokenizer
// to consume the response stream directly (M1.3).
//
// Unlike FetchWithMeta, FetchStream does not populate the HTTP cache because
// caching requires reading the full body. Error responses (status >= 400) return
// the raw body without generating fallback HTML — callers should check
// ResponseMeta.StatusCode.
func (s *Service) FetchStream(ctx context.Context, rawURL string) (io.ReadCloser, ResponseMeta, error) {
	startedAt := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		wrapped := fmt.Errorf("failed to create request: %w", err)
		s.log.Add(RequestLogEntry{Method: http.MethodGet, URL: rawURL, Error: wrapped.Error(), StartedAt: startedAt, Duration: time.Since(startedAt)})
		s.setSecurity(SecuritySummaryFromResponse(nil, wrapped))
		return nil, ResponseMeta{Header: make(http.Header)}, wrapped
	}
	req.Header.Set("User-Agent", s.userAgent)

	resp, redirectCount, err := s.doRequest(req)
	s.setSecurity(SecuritySummaryFromResponse(resp, err))
	if err != nil {
		wrapped := fmt.Errorf("failed to fetch URL: %w", err)
		s.log.Add(RequestLogEntry{Method: http.MethodGet, URL: rawURL, Error: wrapped.Error(), StartedAt: startedAt, Duration: time.Since(startedAt)})
		return nil, ResponseMeta{Header: make(http.Header)}, wrapped
	}

	// Capture metadata before body is consumed.
	meta := ResponseMetaFromResponse(resp)
	meta.RedirectCount = redirectCount
	if resp.Request != nil && resp.Request.URL != nil {
		meta.FinalURL = resp.Request.URL.String()
	}

	// Parse Content-Security-Policy from response headers.
	if cspHeader := resp.Header.Get("Content-Security-Policy"); cspHeader != "" {
		s.setCSP(ParseCSPHeader(cspHeader))
	} else {
		s.setCSP(nil)
	}

	// Validate Content-Type against expected types before returning the stream.
	if err := s.validateContentType(meta.ContentType); err != nil {
		resp.Body.Close()
		entry := RequestLogEntry{
			Method:      http.MethodGet,
			URL:         rawURL,
			Status:      resp.StatusCode,
			ContentType: meta.ContentType,
			Error:       err.Error(),
			StartedAt:   startedAt,
			Duration:    time.Since(startedAt),
		}
		s.log.Add(entry)
		return nil, meta, err
	}

	// Content-Length pre-check: reject before returning the stream.
	if err := checkContentLength(resp, s.maxBodySize); err != nil {
		resp.Body.Close()
		return nil, meta, err
	}

	// Wrap body with context cancellation and optional size limit.
	reader := io.Reader(&limitedContextReader{
		ctx:            ctx,
		reader:         resp.Body,
		limit:          s.maxBodySize,
		compressedSize: resp.ContentLength,
	})
	stream := io.NopCloser(reader)

	s.log.Add(RequestLogEntry{
		Method:      http.MethodGet,
		URL:         rawURL,
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		StartedAt:   startedAt,
	})

	return stream, meta, nil
}

func (s *Service) Security() SecuritySummary {
	s.securityMu.Lock()
	defer s.securityMu.Unlock()
	return s.security
}

func (s *Service) setSecurity(summary SecuritySummary) {
	s.securityMu.Lock()
	defer s.securityMu.Unlock()
	s.security = summary
}

// CSP returns the most recently parsed Content-Security-Policy. Returns nil
// when no CSP header was present in the last fetch.
func (s *Service) CSP() *CSPPolicy {
	s.cspMu.RLock()
	defer s.cspMu.RUnlock()
	return s.csp
}

func (s *Service) setCSP(p *CSPPolicy) {
	s.cspMu.Lock()
	defer s.cspMu.Unlock()
	s.csp = p
}

// checkContentLength returns ErrBodyTooLarge if the response's Content-Length
// header is known and exceeds the configured maxBodySize. A maxBodySize <= 0
// disables the check. Returns nil if the check passes or Content-Length is unknown.
func checkContentLength(resp *http.Response, maxBodySize int64) error {
	if maxBodySize <= 0 {
		return nil
	}
	if resp.ContentLength > maxBodySize {
		return ErrBodyTooLarge
	}
	return nil
}

func readResponseBody(ctx context.Context, resp *http.Response, onProgress ProgressCallback, maxBodySize int64) (string, error) {
	if err := checkContentLength(resp, maxBodySize); err != nil {
		return "", err
	}
	reader := io.Reader(&limitedContextReader{
		ctx:            ctx,
		reader:         resp.Body,
		limit:          maxBodySize,
		compressedSize: resp.ContentLength,
	})
	if onProgress != nil && resp.ContentLength > 0 {
		reader = &progressReader{
			Reader:   reader,
			total:    resp.ContentLength,
			callback: onProgress,
		}
	}

	// Use io.ReadAll instead of bytes.Buffer to avoid the intermediate buffer
	// growth allocations. This performs a single allocation for the body bytes
	// plus one string conversion, instead of Buffer's repeated doubling.
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Service) Close() error {
	if s.cache != nil {
		return s.cache.Close()
	}
	return nil
}

// validateContentType checks the response Content-Type against the configured
// expected types. Returns nil if no expected types are configured (accept all)
// or if the Content-Type matches. Returns ErrUnsupportedMediaType otherwise.
func (s *Service) validateContentType(contentType string) error {
	return ValidateContentType(contentType, s.expectedContentType)
}

func (s *Service) Downloads() []DownloadRecord {
	s.downloadsMu.Lock()
	defer s.downloadsMu.Unlock()
	out := make([]DownloadRecord, len(s.downloads))
	copy(out, s.downloads)
	return out
}

func (s *Service) AddDownload(d DownloadRecord) {
	s.downloadsMu.Lock()
	defer s.downloadsMu.Unlock()
	s.downloads = append(s.downloads, d)
}

func (s *Service) UpdateDownload(d DownloadRecord) {
	s.downloadsMu.Lock()
	defer s.downloadsMu.Unlock()
	for i, record := range s.downloads {
		if record.URL == d.URL && record.TargetPath == d.TargetPath && record.StartedAt.Equal(d.StartedAt) {
			s.downloads[i] = d
			break
		}
	}
}

func (s *Service) StartDownload(ctx context.Context, rawURL, targetPath string) (DownloadRecord, error) {
	m := NewDownloadManager(s.client)
	record := DownloadRecord{
		URL:        rawURL,
		TargetPath: targetPath,
		Status:     DownloadRunning,
		StartedAt:  time.Now(),
	}
	s.AddDownload(record)

	go func() {
		res, _ := m.DownloadWithContext(ctx, rawURL, targetPath)
		s.UpdateDownload(res)
	}()
	return record, nil
}
