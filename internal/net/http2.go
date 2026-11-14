package net

import (
	"crypto/tls"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

type HTTP2Config struct {
	Enabled              bool
	MaxConcurrentStreams uint32
	MaxHeaderListSize    uint32
	AllowHTTP            bool
}

func DefaultHTTP2Config() HTTP2Config {
	return HTTP2Config{
		Enabled:              true,
		MaxConcurrentStreams: 100,
		MaxHeaderListSize:    1 << 20,
		AllowHTTP:            false,
	}
}

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

func DefaultClientWithHTTP2() HTTP {
	cfg := DefaultHTTP2Config()
	return DefaultClientWithHTTP2Config(cfg)
}

func DefaultClientWithHTTP2Config(cfg HTTP2Config) HTTP {
	t := clientTransport()
	_ = configureHTTP2(t, cfg)

	c := &httpClient{
		jar:   NewCookieJar(),
		cache: newResponseCache(),
	}
	c.client = &http.Client{
		Timeout:       30 * time.Second,
		Transport:     t,
		CheckRedirect: c.checkRedirect,
	}
	return c
}
