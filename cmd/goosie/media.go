package main

import (
	"context"
	"math"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/tabs"
)

func mediaFetcher(client net.HTTP, docURL string) engine.MediaFetcher {
	return func(ctx context.Context, url string) (engine.MediaResponse, error) {
		resp, err := client.Get(subresourceCtx(ctx, docURL), url)
		if err != nil {
			return engine.MediaResponse{}, err
		}
		return engine.MediaResponse{
			StatusCode: resp.StatusCode,
			Body:       resp.Body,
			URL:        resp.URL,
		}, nil
	}
}

func mediaOption(backend media.Backend, client net.HTTP, docURL string) engine.Option {
	if backend == nil || client == nil {
		return nil
	}
	return engine.WithMedia(backend, mediaFetcher(client, docURL), docURL)
}

func (f *framePath) pumpChrome() {
	f.pumpJS()
	f.pumpMedia()
	f.refreshFind()
	if f.toolbar != nil {
		f.toolbar.RefreshSuggestions(time.Now())
	}
	f.drainInspectRepaints()
}

func (f *framePath) pumpMedia() {
	if f.tabMgr == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	sess := tab.Session()
	if sess == nil {
		return
	}
	dmg := sess.PumpMedia()
	if len(dmg) == 0 {
		return
	}
	f.repaintTab(tab, mediaDamage(dmg, f.effectiveScale()))
}

func (f *framePath) effectiveScale() float32 {
	dpr := float32(f.config.dpr)
	if f.emulationPreset != nil {
		dpr = f.emulationPreset.DPR
	}
	return dpr * float32(f.zoom)
}

func mediaDamage(rects []frame.RectF, scale float32) frame.Rect {
	var out frame.Rect
	for i, r := range rects {
		d := frame.Rect4(
			int32(math.Floor(float64(r.X0*scale))),
			int32(math.Floor(float64(r.Y0*scale))),
			int32(math.Ceil(float64(r.X1*scale))),
			int32(math.Ceil(float64(r.Y1*scale))),
		)
		if i == 0 {
			out = d
			continue
		}
		out = frame.Rect4(min(out.X0, d.X0), min(out.Y0, d.Y0), max(out.X1, d.X1), max(out.Y1, d.Y1))
	}
	return out
}

func (f *framePath) handleMediaControl(contentX, contentY int32) bool {
	tab := f.tabMgr.Active()
	if tab == nil {
		return false
	}
	sess := tab.Session()
	if sess == nil {
		return false
	}
	dx, dy := f.docPoint(contentX, contentY)
	hit, ok := sess.MediaControlAt(dx, dy)
	if !ok {
		return false
	}
	c := sess.MediaController(hit.Node)
	if c == nil {
		return false
	}
	sess.FocusMediaElement(hit.Node)
	sess.SetUserActivation(true)
	defer sess.SetUserActivation(false)

	switch hit.Part {
	case paint.VideoControlPlay:
		if c.Snapshot().State == media.StatePlaying {
			c.Pause()
		} else {
			_, _ = c.Play()
		}
	case paint.VideoControlMute:
		c.SetMuted(!c.Muted())
	case paint.VideoControlTrack:
		if d := c.Snapshot().Duration; !math.IsNaN(d) && d > 0 {
			c.Seek(hit.Fraction * d)
		}
	}
	f.pumpMedia()
	return true
}

func (f *framePath) mediaKey(tab *tabs.Tab, key rune) bool {
	sess := tab.Session()
	if sess == nil {
		return false
	}
	node := sess.FocusedMediaElement()
	if node == nil {
		return false
	}
	c := sess.MediaController(node)
	if c == nil {
		return false
	}
	var step float64
	switch key {
	case ' ', '\r', '\n':
	case 'm', 'M':
	case 0xF702, 0xF703:
		step = 5
	default:
		return false
	}

	sess.SetUserActivation(true)
	defer sess.SetUserActivation(false)

	snap := c.Snapshot()
	switch key {
	case ' ', '\r', '\n':
		if snap.State == media.StatePlaying {
			c.Pause()
		} else {
			_, _ = c.Play()
		}
	case 'm', 'M':
		c.SetMuted(!c.Muted())
	case 0xF702:
		if !math.IsNaN(snap.Position) {
			c.Seek(math.Max(0, snap.Position-step))
		}
	case 0xF703:
		if !math.IsNaN(snap.Position) {
			c.Seek(snap.Position + step)
		}
	}
	f.pumpMedia()
	return true
}

func (f *framePath) pauseMedia(tab *tabs.Tab) {
	if tab == nil {
		return
	}
	if sess := tab.Session(); sess != nil {
		sess.PauseAllMedia()
	}
}

func (f *framePath) closeAllSessions() {
	if f.pacedSession != nil {
		f.pacedSession.Close()
	}
	if f.tabMgr == nil {
		return
	}
	for _, tab := range f.tabMgr.Tabs() {
		if sess := tab.Session(); sess != nil {
			sess.Close()
		}
	}
}
