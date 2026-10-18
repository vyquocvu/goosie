package main

import (
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestBookmarksBarToggle(t *testing.T) {
	cw2 := &capWin{}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	tb := toolbar.NewState(800, fonts)
	cw := newChromeWindow(cw2, tb, mgr,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fonts, 1)

	initialH := cw.chromeHeight()

	// Toggle with Cmd+Shift+B
	toggleEv := surface.Event{
		Kind: surface.EvKey,
		Key:  'b',
		Mods: surface.ModCommand | surface.ModShift,
	}
	consumed := cw.intercept(toggleEv)
	if !consumed || !cw.showBookmarksBar {
		t.Fatalf("Cmd+Shift+B did not show bookmarks bar: consumed=%v show=%v", consumed, cw.showBookmarksBar)
	}

	toggledH := cw.chromeHeight()
	if toggledH != initialH+bookmarksBarHeight {
		t.Fatalf("chrome height want %d, got %d", initialH+bookmarksBarHeight, toggledH)
	}

	// Toggle off
	consumed2 := cw.intercept(toggleEv)
	if !consumed2 || cw.showBookmarksBar {
		t.Fatalf("Cmd+Shift+B did not hide bookmarks bar: consumed=%v show=%v", consumed2, cw.showBookmarksBar)
	}
	if cw.chromeHeight() != initialH {
		t.Fatalf("chrome height after hiding want %d, got %d", initialH, cw.chromeHeight())
	}
}

func TestBookmarksBarRenderingAndClick(t *testing.T) {
	cw2 := &capWin{}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	tb := toolbar.NewState(800, fonts)

	tmp := filepath.Join(t.TempDir(), "bookmarks.json")
	bmStore, err := bookmarks.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	bmStore.Toggle("https://go.dev", "Go Website")
	bmStore.Toggle("https://news.ycombinator.com", "Hacker News")

	var clickedURL string
	cw := newChromeWindow(cw2, tb, mgr,
		nil, nil, nil, nil, nil, nil,
		func(href string) { clickedURL = href },
		nil, nil, nil, nil, nil, nil, nil, nil, fonts, 1)
	cw.bookmarks = bmStore
	cw.showBookmarksBar = true

	// Check pills
	pills := cw.bookmarkPills()
	if len(pills) != 2 {
		t.Fatalf("want 2 pills, got %d", len(pills))
	}
	if pills[0].entry.URL != "https://go.dev" {
		t.Fatalf("pill 0 URL mismatch: %s", pills[0].entry.URL)
	}

	// Present to verify rendering
	content := frame.NewBitmap(800, 600)
	if err := cw.Present(content, nil); err != nil {
		t.Fatal(err)
	}

	// Hit test clicking on the first pill
	clickPos := frame.Point{X: pills[0].rect.X0 + 5, Y: pills[0].rect.Y0 + 5}
	clickEv := surface.Event{
		Kind:   surface.EvPointer,
		Action: surface.PointerPress,
		Button: surface.ButtonLeft,
		Pos:    clickPos,
	}
	consumed := cw.intercept(clickEv)
	if !consumed || clickedURL != "https://go.dev" {
		t.Fatalf("click on bookmark pill failed: consumed=%v clickedURL=%s", consumed, clickedURL)
	}

	// Hover test
	motionEv := surface.Event{
		Kind:   surface.EvPointer,
		Action: surface.PointerMotion,
		Button: surface.ButtonNone,
		Pos:    clickPos,
	}
	cw.intercept(motionEv)
	if cw.hoverBookmark != 0 {
		t.Fatalf("hover bookmark want 0, got %d", cw.hoverBookmark)
	}
}
