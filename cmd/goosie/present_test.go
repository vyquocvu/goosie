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
	found := false
	for _, r := range cw2.damage {
		if r.Y0 == chromeH && r.Y1 == chromeH+1800 {
			found = true
		}
	}
	if !found {
		t.Fatalf("content damage not shifted by chrome: %v", cw2.damage)
	}
	if cw2.damage[0].Y1 != chromeH {
		t.Fatalf("chrome band damage = %v, want top band ending at %d", cw2.damage[0], chromeH)
	}
}
