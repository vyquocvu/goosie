package main

import (
	"bytes"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
)

func deferredImgSession(t *testing.T) *engine.Session {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 640, 292))); err != nil {
		t.Fatal(err)
	}
	pngBytes := buf.Bytes()
	s, err := engine.NewSession(`<html><body><img id="pic" src="/pic.png"></body></html>`, nil, 800,
		engine.WithDeferredImages("https://p/", func(base, url string) ([]byte, error) {
			return pngBytes, nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func layerForSession(t *testing.T, s *engine.Session) *frame.Layer {
	t.Helper()
	list, err := s.PaintChecked(1)
	if err != nil {
		t.Fatal(err)
	}
	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		t.Fatal(err)
	}
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)
	return layer
}

func TestApplyNavResultTriggersDeferredImages(t *testing.T) {
	f := newNavTestPath(t)
	f.config.width, f.config.dpr, f.zoom = 800, 1, 1
	tab := f.tabMgr.Active()
	tab.Nav.Serial = 9
	s := deferredImgSession(t)
	layer := layerForSession(t, s)

	f.applyNavResult(navResult{tabID: tab.ID, serial: 9, url: "https://p/", layer: layer, session: s})

	select {
	case res := <-f.imgResults:
		if res.tabID != tab.ID || res.serial != 9 || res.session != s {
			t.Errorf("imgResult = %+v, want tab %d serial 9 session %p", res, tab.ID, s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("applyNavResult never reported deferred images")
	}
}

func TestImageResultReflowsAndRepaints(t *testing.T) {
	f := newNavTestPath(t)
	f.config.width, f.config.dpr, f.zoom = 800, 1, 1
	tab := f.tabMgr.Active()
	tab.Nav.Serial = 9
	s := deferredImgSession(t)
	tab.Layer = layerForSession(t, s)
	before := tab.Layer

	f.applyNavResult(navResult{tabID: tab.ID, serial: 9, url: "https://p/", layer: tab.Layer, session: s})
	res := <-f.imgResults
	f.applyImageResult(res)

	if tab.Layer == before {
		t.Error("tab.Layer unchanged after images landed; repaint did not run")
	}
	if p := s.DeferredImagesPending(); p != 0 {
		t.Errorf("DeferredImagesPending = %d, want 0", p)
	}
}

func TestImageResultIgnoredWhenNavMovedOn(t *testing.T) {
	f := newNavTestPath(t)
	f.config.width, f.config.dpr, f.zoom = 800, 1, 1
	tab := f.tabMgr.Active()
	tab.Nav.Serial = 9
	s := deferredImgSession(t)
	tab.Layer = layerForSession(t, s)
	before := tab.Layer

	f.applyNavResult(navResult{tabID: tab.ID, serial: 9, url: "https://p/", layer: tab.Layer, session: s})
	res := <-f.imgResults
	// A new navigation superseded this one before the images landed.
	tab.Nav.Serial = 10

	f.applyImageResult(res)

	if tab.Layer != before {
		t.Error("stale image result repainted a superseded navigation")
	}
}
