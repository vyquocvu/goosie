package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	goosienet "github.com/vyquocvu/goosie/internal/net"
)

type blankDocumentClient struct{}

const blankDocumentHTML = "<!doctype html><html><head><title></title></head><body></body></html>"

func isAboutBlankURL(rawURL string) bool {
	lower := strings.ToLower(strings.TrimSpace(rawURL))
	if !strings.HasPrefix(lower, "about:") {
		return false
	}
	rest := lower[len("about:"):]
	if index := strings.IndexAny(rest, "?#"); index >= 0 {
		rest = rest[:index]
	}
	return rest == "blank"
}

func (blankDocumentClient) Get(ctx context.Context, rawURL string) (*goosienet.Response, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !isAboutBlankURL(rawURL) {
		return nil, errors.New("blank document client: unsupported URL")
	}
	return &goosienet.Response{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "text/html; charset=utf-8",
		},
		HeaderValues: map[string][]string{
			"Content-Type": {"text/html; charset=utf-8"},
		},
		Body: []byte(blankDocumentHTML),
		URL:  rawURL,
	}, nil
}

func (blankDocumentClient) Post(context.Context, string, string, io.Reader) (*goosienet.Response, error) {
	return nil, errors.New("blank document client: POST unsupported")
}

func (blankDocumentClient) SetTimeout(time.Duration) {}

func (blankDocumentClient) Close() error { return nil }

func (blankDocumentClient) Jar() *goosienet.CookieJar { return nil }

func (blankDocumentClient) NetworkLog() *goosienet.NetworkLog { return nil }
