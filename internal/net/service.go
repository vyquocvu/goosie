package net

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"
)

const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

type ServiceOptions struct {
	Client    *http.Client
	Cache     *HTTPCache
	UserAgent string
}

type Service struct {
	client    *http.Client
	cache     *HTTPCache
	userAgent string
	log       *RequestLog

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
		client:    client,
		cache:     options.Cache,
		userAgent: userAgent,
		log:       &RequestLog{},
	}
}

func (s *Service) Fetch(rawURL string) (string, error) {
	return s.FetchWithContext(context.Background(), rawURL, nil)
}

func (s *Service) FetchWithContext(ctx context.Context, rawURL string, onProgress ProgressCallback) (string, error) {
	startedAt := time.Now()
	if body, entry, ok := s.cache.Get(rawURL); ok {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		wrapped := fmt.Errorf("failed to create request: %w", err)
		s.log.Add(RequestLogEntry{Method: http.MethodGet, URL: rawURL, Error: wrapped.Error(), StartedAt: startedAt, Duration: time.Since(startedAt)})
		s.setSecurity(SecuritySummaryFromResponse(nil, wrapped))
		return "", wrapped
	}
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.client.Do(req)
	s.setSecurity(SecuritySummaryFromResponse(resp, err))
	if err != nil {
		wrapped := fmt.Errorf("failed to fetch URL: %w", err)
		s.log.Add(RequestLogEntry{Method: http.MethodGet, URL: rawURL, Error: wrapped.Error(), StartedAt: startedAt, Duration: time.Since(startedAt)})
		return "", wrapped
	}
	defer resp.Body.Close()

	body, readErr := readResponseBody(resp, onProgress)
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

	s.cache.Put(rawURL, resp, body)
	entry.Duration = time.Since(startedAt)
	s.log.Add(entry)
	return body, nil
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

func readResponseBody(resp *http.Response, onProgress ProgressCallback) (string, error) {
	var reader io.Reader = resp.Body
	if onProgress != nil && resp.ContentLength > 0 {
		reader = &progressReader{
			Reader:   resp.Body,
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
