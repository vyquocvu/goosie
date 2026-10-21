package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

type capWin struct {
	got    *frame.Bitmap
	damage []frame.Rect
}

func (c *capWin) Present(buf *frame.Bitmap, damage []frame.Rect) error {
	cp := frame.NewBitmap(buf.W, buf.H)
	copy(cp.RGBA, buf.RGBA)
	c.got = cp
	c.damage = append([]frame.Rect(nil), damage...)
	return nil
}
func (c *capWin) Events() <-chan surface.Event { return nil }
func (c *capWin) SetCursor(surface.Cursor)     {}
func (c *capWin) SetIME(bool)                  {}
func (c *capWin) ScaleFactor() float32         { return 2 }
func (c *capWin) Close() error                 { return nil }

func TestPresentComposesWindowSize(t *testing.T) {
	cw2 := &capWin{}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	tb := toolbar.NewState(1440, fonts)
	tb.SetScale(2)
	cw := &chromeWindow{Window: cw2, toolbar: tb, tabMgr: mgr, scale: 2, fonts: fonts}

	content := frame.NewBitmap(1440, 1800)
	green := frame.RGB(10, 200, 10)
	content.FillRect(frame.Rect4(0, 0, 1440, 1800), green, nil)

	chromeH := cw.chromeHeight()
	if err := cw.Present(content, []frame.Rect{{X0: 0, Y0: 0, X1: 1440, Y1: 1800}}); err != nil {
		t.Fatal(err)
	}
	if int32(cw2.got.H) != 1800+chromeH {
		t.Fatalf("out height = %d, want %d", cw2.got.H, 1800+chromeH)
	}
	if g := cw2.got.At(700, int(chromeH)+900); g != green {
		t.Fatalf("content pixel below chrome = %v, want green", g)
	}
	wantFull := frame.Rect4(0, 0, 1440, 1800+chromeH)
	if len(cw2.damage) != 1 || cw2.damage[0] != wantFull {
		t.Fatalf("first-frame damage = %v, want one full-window rect %v", cw2.damage, wantFull)
	}
}

func TestPresentMarksOnlyChangedChrome(t *testing.T) {
	cw2 := &capWin{}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	tb := toolbar.NewState(800, fonts)
	cw := &chromeWindow{Window: cw2, toolbar: tb, tabMgr: mgr, scale: 1, fonts: fonts}
	content := frame.NewBitmap(800, 600)
	full := []frame.Rect{content.Bounds()}

	if err := cw.Present(content, full); err != nil {
		t.Fatal(err)
	}
	if len(cw2.damage) == 0 {
		t.Fatal("first frame did not damage the new window surface")
	}

	if err := cw.Present(content, nil); err != nil {
		t.Fatal(err)
	}
	if len(cw2.damage) != 0 {
		t.Fatalf("unchanged chrome damage = %v, want none", cw2.damage)
	}

	tb.SyncFromTab("https://example.test/", false, "", nil)
	if err := cw.Present(content, nil); err != nil {
		t.Fatal(err)
	}
	if len(cw2.damage) == 0 || cw2.damage[0].Y0 != 0 || cw2.damage[0].Y1 == 0 {
		t.Fatalf("changed address bar damage = %v, want a chrome rect", cw2.damage)
	}
}

func TestPresentCopiesOnlyDamagedContent(t *testing.T) {
	cw2 := &capWin{}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	tb := toolbar.NewState(800, fonts)
	cw := &chromeWindow{Window: cw2, toolbar: tb, tabMgr: mgr, scale: 1, fonts: fonts}
	content := frame.NewBitmap(800, 600)
	green, red := frame.RGB(10, 200, 10), frame.RGB(220, 20, 20)
	content.FillRect(content.Bounds(), green, nil)
	if err := cw.Present(content, []frame.Rect{content.Bounds()}); err != nil {
		t.Fatal(err)
	}

	changed := frame.Rect4(20, 30, 25, 35)
	content.FillRect(changed, red, nil)
	if err := cw.Present(content, []frame.Rect{changed}); err != nil {
		t.Fatal(err)
	}

	chromeH := cw.chromeHeight()
	if got := cw2.got.At(22, int(chromeH)+32); got != red {
		t.Fatalf("damaged content pixel = %v, want red %v", got, red)
	}
	if got := cw2.got.At(100, int(chromeH)+100); got != green {
		t.Fatalf("retained content pixel = %v, want green %v", got, green)
	}
	want := frame.Rect4(changed.X0, changed.Y0+chromeH, changed.X1, changed.Y1+chromeH)
	if len(cw2.damage) != 1 || cw2.damage[0] != want {
		t.Fatalf("partial content damage = %v, want %v", cw2.damage, want)
	}
}

func TestPresentFullDamagesWhenChromeHeightChanges(t *testing.T) {
	cw2 := &capWin{}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	tb := toolbar.NewState(800, fonts)
	cw := &chromeWindow{Window: cw2, toolbar: tb, tabMgr: mgr, scale: 1, fonts: fonts}
	first := frame.NewBitmap(800, 600)
	first.FillRect(first.Bounds(), frame.RGB(10, 200, 10), nil)
	if err := cw.Present(first, []frame.Rect{first.Bounds()}); err != nil {
		t.Fatal(err)
	}

	cw.showBookmarksBar = true
	second := frame.NewBitmap(800, 600-int(bookmarksBarHeight))
	second.FillRect(second.Bounds(), frame.RGB(20, 20, 200), nil)
	if int32(second.H)+cw.chromeHeight() != int32(cw2.got.H) {
		t.Fatalf("test setup changed output height: content=%d chrome=%d prior output=%d", second.H, cw.chromeHeight(), cw2.got.H)
	}
	if err := cw.Present(second, []frame.Rect{second.Bounds()}); err != nil {
		t.Fatal(err)
	}
	if len(cw2.damage) != 1 || cw2.damage[0] != cw.outBitmap.Bounds() {
		t.Fatalf("chrome-height transition damage = %v, want full frame %v", cw2.damage, cw.outBitmap.Bounds())
	}
}
