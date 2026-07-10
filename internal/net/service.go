package net

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"
)

// ErrBodyTooLarge is returned when the response body exceeds the configured
// MaxBodySize. Callers can check it with errors.Is.
var ErrBodyTooLarge = errors.New("response body exceeds size limit")

// limitedContextReader wraps an io.Reader with context cancellation awareness
// and optional byte limit enforcement. A limit of 0 means unlimited.
type limitedContextReader struct {
	ctx    context.Context
	reader io.Reader
	limit  int64 // max bytes to read; 0 = unlimited
	read   int64 // bytes read so far
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
	return n, err
}

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

type ServiceOptions struct {
	Client      *http.Client
	Cache       *HTTPCache
	UserAgent   string
	MaxBodySize int64 // maximum response body size in bytes; 0 = unlimited
}

type Service struct {
	client      *http.Client
	cache       *HTTPCache
	userAgent   string
	log         *RequestLog
	maxBodySize int64

	securityMu sync.Mutex
	security   SecuritySummary
}

func NewService(options ServiceOptions) *Service {
	client := options.Client
	if client == nil {
		jar, _ := cookiejar.New(nil)
		client = &http.Client{Jar: jar, Timeout: 30 * time.Second}
	}
	userAgent := options.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	return &Service{
		client:      client,
		cache:       options.Cache,
		userAgent:   userAgent,
		log:         NewRequestLog(),
		maxBodySize: options.MaxBodySize,
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

	resp, err := s.client.Do(req)
	s.setSecurity(SecuritySummaryFromResponse(resp, err))
	if err != nil {
		wrapped := fmt.Errorf("failed to fetch URL: %w", err)
		s.log.Add(RequestLogEntry{Method: http.MethodGet, URL: rawURL, Error: wrapped.Error(), StartedAt: startedAt, Duration: time.Since(startedAt)})
		return "", ResponseMeta{Header: make(http.Header)}, wrapped
	}
	defer resp.Body.Close()

	// Capture metadata before body is consumed.
	meta := ResponseMetaFromResponse(resp)

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

	resp, err := s.client.Do(req)
	s.setSecurity(SecuritySummaryFromResponse(resp, err))
	if err != nil {
		wrapped := fmt.Errorf("failed to fetch URL: %w", err)
		s.log.Add(RequestLogEntry{Method: http.MethodGet, URL: rawURL, Error: wrapped.Error(), StartedAt: startedAt, Duration: time.Since(startedAt)})
		return "", wrapped
	}
	defer resp.Body.Close()

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

func readResponseBody(ctx context.Context, resp *http.Response, onProgress ProgressCallback, maxBodySize int64) (string, error) {
	reader := io.Reader(&limitedContextReader{
		ctx:    ctx,
		reader: resp.Body,
		limit:  maxBodySize,
	})
	if onProgress != nil && resp.ContentLength > 0 {
		reader = &progressReader{
			Reader:   reader,
			total:    resp.ContentLength,
			callback: onProgress,
		}
	}

	var buf bytes.Buffer
	_, err := io.Copy(&buf, reader)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
