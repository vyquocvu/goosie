package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/tabs"
)

func mustParseDoc(t *testing.T, source string) *dom.Node {
	t.Helper()
	limits := dom.ParseLimits{Nodes: 100, Depth: 50, Attributes: 50, AttributeBytes: 1024}
	doc, err := dom.ParseBounded(source, limits)
	if err != nil || doc == nil {
		t.Fatalf("dom.ParseBounded: %v", err)
	}
	return &doc.Node
}

func TestFindFaviconLink(t *testing.T) {
	base := "https://example.com/blog/post/index.html"
	cases := []struct {
		name string
		html string
		want string
	}{
		{
			name: "standard icon resolved",
			html: `<html><head><link rel="icon" href="/fav.png"></head></html>`,
			want: "https://example.com/fav.png",
		},
		{
			name: "shortcut icon",
			html: `<html><head><link rel="shortcut icon" href="icon.png"></head></html>`,
			want: "https://example.com/blog/post/icon.png",
		},
		{
			name: "apple touch icon fallback",
			html: `<html><head><link rel="apple-touch-icon" href="/touch.png"></head></html>`,
			want: "https://example.com/touch.png",
		},
		{
			name: "standard icon beats apple touch",
			html: `<html><head><link rel="apple-touch-icon" href="/touch.png"><link rel="icon" href="/fav.ico"></head></html>`,
			want: "https://example.com/fav.ico",
		},
		{
			name: "multiple rel tokens",
			html: `<html><head><link rel="icon shortcut" href="/both.ico"></head></html>`,
			want: "https://example.com/both.ico",
		},
		{
			name: "no icon",
			html: `<html><head><link rel="stylesheet" href="/a.css"></head></html>`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := findFaviconLink(mustParseDoc(t, tc.html), base); got != tc.want {
				t.Fatalf("findFaviconLink = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFaviconCandidatePrefersDeclaredIcon(t *testing.T) {
	if got := faviconURLFor(mustParseDoc(t, `<html><head><link rel="icon" href="/app.png"></head></html>`), "https://site.example/deep/page"); got != "https://site.example/app.png" {
		t.Fatalf("declared icon = %q, want resolved link", got)
	}
	if got := faviconURLFor(mustParseDoc(t, `<html><head></head></html>`), "https://site.example/deep/page"); got != "" {
		t.Fatalf("undeclared icon = %q, want no request", got)
	}
}

func encodePNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

type stubHTTP struct {
	get      func(ctx context.Context, url string) (*net.Response, error)
	getCalls int
	lastURL  string
}

func (s *stubHTTP) Get(ctx context.Context, url string) (*net.Response, error) {
	s.getCalls++
	s.lastURL = url
	return s.get(ctx, url)
}
func (s *stubHTTP) Post(context.Context, string, string, io.Reader) (*net.Response, error) {
	return nil, nil
}
func (s *stubHTTP) SetTimeout(time.Duration)    {}
func (s *stubHTTP) Close() error                { return nil }
func (s *stubHTTP) Jar() *net.CookieJar         { return nil }
func (s *stubHTTP) NetworkLog() *net.NetworkLog { return nil }

func TestFetchFaviconDecodesImage(t *testing.T) {
	data := encodePNG(t, 16, 16, color.RGBA{R: 10, G: 200, B: 30, A: 255})
	client := &stubHTTP{get: func(context.Context, string) (*net.Response, error) {
		return &net.Response{StatusCode: 200, Body: data}, nil
	}}
	img, err := fetchFavicon(context.Background(), client, "https://example.com/favicon.ico")
	if err != nil || img == nil {
		t.Fatalf("fetchFavicon = %v, %v", img, err)
	}
	if b := img.Bounds(); b.Dx() != 16 || b.Dy() != 16 {
		t.Fatalf("decoded bounds = %v, want 16x16", b)
	}
}

func TestFetchFaviconRejectsBadStatusAndJunk(t *testing.T) {
	client := &stubHTTP{get: func(context.Context, string) (*net.Response, error) {
		return &net.Response{StatusCode: 404, Body: []byte("nope")}, nil
	}}
	if _, err := fetchFavicon(context.Background(), client, "https://example.com/favicon.ico"); err == nil {
		t.Fatal("404 favicon decoded successfully")
	}
	client = &stubHTTP{get: func(context.Context, string) (*net.Response, error) {
		return &net.Response{StatusCode: 200, Body: []byte("not an image")}, nil
	}}
	if _, err := fetchFavicon(context.Background(), client, "https://example.com/favicon.ico"); err == nil {
		t.Fatal("junk favicon decoded successfully")
	}
}

func TestLoadFaviconAppliesAndDropsStale(t *testing.T) {
	data := encodePNG(t, 8, 8, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	client := &stubHTTP{get: func(context.Context, string) (*net.Response, error) {
		return &net.Response{StatusCode: 200, Body: data}, nil
	}}
	mgr := tabs.NewManager(nil)
	tab := mgr.NewTab()
	f := &framePath{tabMgr: mgr, client: client}

	tab.Nav.Serial = 7
	f.loadFavicon(tab, 6, "https://example.com/favicon.ico")
	if tab.Favicon() != nil {
		t.Fatal("stale navigation applied a favicon")
	}

	f.loadFavicon(tab, 7, "https://example.com/favicon.ico")
	if tab.Favicon() == nil {
		t.Fatal("current navigation did not apply its favicon")
	}
}

func TestLoadFaviconUsesPrivateClientForIncognito(t *testing.T) {
	data := encodePNG(t, 8, 8, color.RGBA{R: 9, G: 9, B: 9, A: 255})
	defaultClient := &stubHTTP{get: func(context.Context, string) (*net.Response, error) {
		t.Error("incognito favicon fetched through the default client")
		return nil, nil
	}}
	privateClient := &stubHTTP{get: func(context.Context, string) (*net.Response, error) {
		return &net.Response{StatusCode: 200, Body: data}, nil
	}}

	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	tab := mgr.NewIncognitoTab()
	f := &framePath{tabMgr: mgr, client: defaultClient, privateClient: privateClient}

	tab.Nav.Serial = 1
	f.loadFavicon(tab, 1, "https://private.example/favicon.ico")
	if privateClient.getCalls != 1 {
		t.Fatalf("private client Get calls = %d, want 1", privateClient.getCalls)
	}
	if defaultClient.getCalls != 0 {
		t.Fatalf("default client Get calls = %d, want 0", defaultClient.getCalls)
	}
	if tab.Favicon() == nil {
		t.Fatal("incognito tab has no favicon after fetch")
	}
}
