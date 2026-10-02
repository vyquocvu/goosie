package net

import (
	"crypto/tls"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// HTTP2Config holds HTTP/2 configuration options.
type HTTP2Config struct {
	// Enabled controls whether HTTP/2 is used.
	Enabled bool
	// MaxConcurrentStreams limits concurrent streams per connection.
	MaxConcurrentStreams uint32
	// MaxHeaderListSize limits header list size.
	MaxHeaderListSize uint32
	// AllowHTTP allows HTTP/2 over plain TCP (h2c).
	AllowHTTP bool
}

// DefaultHTTP2Config returns sensible HTTP/2 defaults.
func DefaultHTTP2Config() HTTP2Config {
	return HTTP2Config{
		Enabled:              true,
		MaxConcurrentStreams: 100,
		MaxHeaderListSize:    1 << 20, // 1 MiB
		AllowHTTP:            false,
	}
}

// configureHTTP2 applies HTTP/2 settings to a transport.
func configureHTTP2(t *http.Transport, cfg HTTP2Config) error {
	if !cfg.Enabled {
		return nil
	}

	if t.TLSClientConfig == nil {
		t.TLSClientConfig = &tls.Config{}
	}

	hasH2 := false
	for _, p := range t.TLSClientConfig.NextProtos {
		if p == "h2" {
			hasH2 = true
			break
		}
	}
	if !hasH2 {
		t.TLSClientConfig.NextProtos = append(t.TLSClientConfig.NextProtos, "h2", "http/1.1")
	}

	return http2.ConfigureTransport(t)
}

// DefaultClientWithHTTP2 returns an HTTP client with HTTP/2 support enabled.
func DefaultClientWithHTTP2() HTTP {
	cfg := DefaultHTTP2Config()
	return DefaultClientWithHTTP2Config(cfg)
}

// DefaultClientWithHTTP2Config returns an HTTP client with custom HTTP/2 config.
func DefaultClientWithHTTP2Config(cfg HTTP2Config) HTTP {
	t := clientTransport()
	_ = configureHTTP2(t, cfg)

	return &httpClient{
		client: &http.Client{
			Timeout:       30 * time.Second,
			Transport:     t,
			CheckRedirect: checkRedirect,
		},
		jar:   NewCookieJar(),
		cache: newResponseCache(),
	}
}
