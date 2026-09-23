package main

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/raster"
)

func TestParsePrivateFlag(t *testing.T) {
	c, err := parse([]string{"-private"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.private {
		t.Fatal("-private did not set config.private")
	}
	c, err = parse([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if c.private {
		t.Fatal("default run set config.private")
	}
}

func TestParseRejectsInvalidViewport(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"zero width", []string{"-width", "0"}},
		{"negative height", []string{"-height", "-1"}},
		{"dpr zero", []string{"-dpr", "0"}},
		{"dpr NaN", []string{"-dpr", "NaN"}},
		{"dpr Inf", []string{"-dpr", "Inf"}},
		{"dpr too large", []string{"-dpr", "9"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parse(tc.args)
			if err == nil {
				t.Fatalf("parse(%v) succeeded, want error", tc.args)
			}
		})
	}
}

func TestLoadURLCtxRejectsInvalidViewport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html></html>"))
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		w, h  int
		scale float32
	}{
		{"zero width", 0, 600, 1},
		{"NaN scale", 800, 600, float32(math.NaN())},
		{"Inf scale", 800, 600, float32(math.Inf(1))},
		{"round to zero", 1, 1, 0.001},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			layer, _, _, _, err := loadURLCtx(context.Background(), client, fonts, u.String(), tc.w, tc.h, tc.scale, t.TempDir())
			if err == nil {
				t.Fatalf("loadURLCtx(%d,%d,%v) succeeded, want error", tc.w, tc.h, tc.scale)
			}
			if layer != nil {
				t.Fatalf("loadURLCtx returned non-nil layer on error")
			}
		})
	}
}

func TestLoadURLCtxValidSmallDocument(t *testing.T) {
	html := `<!DOCTYPE html><html><body><div style="width:64px;height:64px;background:red"></div></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	layer, _, _, _, err := loadURLCtx(context.Background(), client, fonts, u.String(), 800, 600, 1, t.TempDir())
	if err != nil {
		t.Fatalf("loadURLCtx failed: %v", err)
	}
	if layer == nil {
		t.Fatal("loadURLCtx returned nil layer")
	}
	if layer.Bounds.Empty() {
		t.Fatal("loadURLCtx returned empty layer")
	}
}

func TestLoadURLCtxRejectsOversizedDocument(t *testing.T) {
	html := `<!DOCTYPE html><html><body><div style="width:65537px;height:65536px;background:red"></div></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(html))
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	layer, _, _, _, err := loadURLCtx(context.Background(), client, fonts, u.String(), 800, 600, 1, t.TempDir())
	if err == nil {
		t.Fatal("loadURLCtx succeeded on oversized document")
	}
	if layer != nil {
		t.Fatal("loadURLCtx returned non-nil layer on oversized document")
	}
}

func TestDevSizeRounding(t *testing.T) {
	c := config{width: 800, height: 600, dpr: 1.5}
	size := c.devSize()
	if size.W != 1200 || size.H != 900 {
		t.Errorf("devSize() = %v, want {1200 900}", size)
	}
}

func TestConfigDevSize(t *testing.T) {
	c := config{width: 100, height: 100, dpr: 2}
	got := c.devSize()
	want := frame.Size{W: 200, H: 200}
	if got != want {
		t.Errorf("devSize() = %v, want %v", got, want)
	}
}
