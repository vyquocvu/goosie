package main

import (
	"context"
	"math"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/tabs"
)

// mediaFetcher is the browser's HTTP client behind the engine's MediaFetcher
// callback. Media goes through here rather than being handed to AVFoundation as a
// URL for one reason: the transport is where the cookies, the redirect limit, the
// ad filter, the private-client choice and the dial-time address guard live, and a
// native media stack fetching for itself would bypass every one of them.
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

// mediaOption is the session option that gives a document video playback, or nil
// when this run has no way to play video.
//
// The backend comes from the window that was actually opened rather than from the
// platform the process is running on. A headless run on a Mac could ask the
// machine whether AVFoundation exists and be told yes, and a video that "plays"
// into nothing is worse than one that says it cannot: the element would report a
// advancing position that no frame ever followed.
func mediaOption(backend media.Backend, client net.HTTP, docURL string) engine.Option {
	if backend == nil || client == nil {
		return nil
	}
	return engine.WithMedia(backend, mediaFetcher(client, docURL), docURL)
}

// pumpChrome is the per-vsync hook: one event-loop turn for the active tab, then
// one media step. They ride the chrome event thread together so no second thread
// ever touches a session.
func (f *framePath) pumpChrome() {
	f.pumpJS()
	f.pumpMedia()
}

// pumpMedia advances the active tab's video elements and repaints what changed.
// It runs whether or not the document has scripting, because a page with
// <video controls> and no script at all still has to play: the controls are ours,
// not the page's.
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

// effectiveScale is the device-pixel ratio a repaint paints at, including zoom and
// any device emulation. It is the same number repaintTab uses, and a damage rect
// scaled by a different one would miss the pixels it was meant to cover.
func (f *framePath) effectiveScale() float32 {
	dpr := float32(f.config.dpr)
	if f.emulationPreset != nil {
		dpr = f.emulationPreset.DPR
	}
	return dpr * float32(f.zoom)
}

// mediaDamage unions document-space rectangles into one device-pixel rect. Video
// damage is small and contiguous — one element's box — so a union costs nothing
// that per-rectangle repaints would not cost more of.
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

// handleMediaControl turns a left press on a video's painted controls into the
// playback command the user meant, and reports whether it handled the press.
//
// The press is a trusted gesture, and marking the session activated around it is
// what separates this from a script calling play() on a timer: the engine refuses
// the second and accepts the first, and only the host can tell them apart.
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
	// Clicking a control focuses the element, which is what puts the keyboard
	// controls in reach afterwards. A video is not a form control, so this press
	// is the only way it can ever hold focus.
	sess.FocusMediaElement(hit.Node)
	sess.SetUserActivation(true)
	defer sess.SetUserActivation(false)

	switch hit.Part {
	case paint.VideoControlPlay:
		if c.Snapshot().State == media.StatePlaying {
			c.Pause()
		} else {
			// The request settles through an event, not through this return value;
			// a refusal (no source, too many players) shows up in the element's own
			// error state rather than as a click that did nothing silently.
			_, _ = c.Play()
		}
	case paint.VideoControlMute:
		c.SetMuted(!c.Muted())
	case paint.VideoControlTrack:
		if d := c.Snapshot().Duration; !math.IsNaN(d) && d > 0 {
			c.Seek(hit.Fraction * d)
		}
	}
	// Reflect the press now rather than at the next tick: a play button that
	// changes a frame late looks like a control that did not respond.
	f.pumpMedia()
	return true
}

// mediaKey handles the keys a focused video owns. It reports whether it consumed
// the key, so the page and the text cursor do not also see it.
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
	case 0xF702, 0xF703: // left, right arrows
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

// pauseMedia stops the departing tab's video. It does not remember that it was
// playing: coming back to a tab that resumes by itself is a tab that starts
// making noise without being asked, and the DOM agrees — visibilitychange pauses
// and nothing unpauses.
func (f *framePath) pauseMedia(tab *tabs.Tab) {
	if tab == nil {
		return
	}
	if sess := tab.Session(); sess != nil {
		sess.PauseAllMedia()
	}
}

// closeAllSessions releases every document the frame path still holds. Close is
// idempotent, so a tab that was already closed on the way out costs nothing here,
// and a tab that was not is the reason this exists: without it the only thing
// stopping a background tab's audio at exit would be the process dying. A paced
// run keeps its one document outside the tab manager so the driver can pump it;
// the field is not cleared because the pump goroutine may still be reading it,
// and Close is idempotent.
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
