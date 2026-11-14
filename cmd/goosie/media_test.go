package main

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

type stubBackend struct {
	mu      sync.Mutex
	players []*stubPlayer
}

func (b *stubBackend) Name() string { return "stub" }

func (b *stubBackend) Open(path string) (media.Player, error) {
	p := &stubPlayer{state: media.StateReady, dur: 10, w: 320, h: 180}
	b.mu.Lock()
	b.players = append(b.players, p)
	b.mu.Unlock()
	return p, nil
}

func (b *stubBackend) first() *stubPlayer {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.players) == 0 {
		return nil
	}
	return b.players[0]
}

type stubPlayer struct {
	mu     sync.Mutex
	state  media.State
	pos    float64
	dur    float64
	w, h   int
	done   uint64
	plays  int
	pauses int
	seeks  []float64
	mutes  []bool
	closes int
}

func (p *stubPlayer) Play(op uint64) {
	p.mu.Lock()
	p.plays++
	p.state = media.StatePlaying
	p.done = op
	p.mu.Unlock()
}

func (p *stubPlayer) Pause() {
	p.mu.Lock()
	p.pauses++
	if p.state == media.StatePlaying {
		p.state = media.StatePaused
	}
	p.mu.Unlock()
}

func (p *stubPlayer) Seek(op uint64, seconds float64) {
	p.mu.Lock()
	p.seeks = append(p.seeks, seconds)
	p.pos = seconds
	p.done = op
	p.mu.Unlock()
}

func (p *stubPlayer) SetMuted(muted bool) {
	p.mu.Lock()
	p.mutes = append(p.mutes, muted)
	p.mu.Unlock()
}

func (p *stubPlayer) Poll() media.Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := media.Snapshot{
		State: p.state, Position: p.pos, Duration: p.dur,
		Width: p.w, Height: p.h, DoneOp: p.done,
	}
	if len(p.mutes) > 0 {
		s.Muted = p.mutes[len(p.mutes)-1]
	}
	p.done = 0
	return s
}

func (p *stubPlayer) Close() {
	p.mu.Lock()
	p.closes++
	p.mu.Unlock()
}

func (p *stubPlayer) counts() (plays, pauses int, seeks []float64, mutes []bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.plays, p.pauses, append([]float64(nil), p.seeks...), append([]bool(nil), p.mutes...)
}

const stubMP4Body = "\x00\x00\x00\x14ftypisom\x00\x00\x02\x00isom"

func mediaTestPath(t *testing.T, b *stubBackend) *framePath {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><video id="v" src="clip.mp4" controls width="320" height="180"></video></body></html>`))
	})
	mux.HandleFunc("/clip.mp4", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte(stubMP4Body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := net.DefaultClient()
	t.Cleanup(func() { _ = client.Close() })
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("fonts: %v", err)
	}

	f := newNavTestPath(t)
	f.config.width = 800
	f.config.dpr = 1
	f.zoom = 1
	f.mediaBackend = b
	f.client = client

	_, _, _, sess, _, err := loadURLCtx(t.Context(), client, fonts, srv.URL, 800, 600, 1, t.TempDir(), false, false, b)
	if err != nil {
		t.Fatalf("loadURLCtx: %v", err)
	}
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	t.Cleanup(sess.Close)

	c := sess.MediaController(videoElement(t, sess))
	if c == nil {
		t.Fatal("the loaded document produced no media controller")
	}
	c.Load()
	for i := 0; i < 400 && b.first() == nil; i++ {
		f.pumpMedia()
		time.Sleep(time.Millisecond)
	}
	if b.first() == nil {
		t.Fatalf("the source never reached the backend (state %v)", c.Snapshot().State)
	}
	f.pumpMedia()
	return f
}

func videoElement(t *testing.T, sess *engine.Session) *dom.Node {
	t.Helper()
	nodes := sess.Doc.ElementsByTagName("video")
	if len(nodes) == 0 {
		t.Fatal("the fixture has no <video> element")
	}
	return nodes[0]
}

func videoBox(t *testing.T, sess *engine.Session) frame.Rect {
	t.Helper()
	for i := range sess.Arena.Objects {
		o := &sess.Arena.Objects[i]
		if o.Node != nil && o.Node.Data == "video" {
			x0, y0, x1, y1 := o.ContentRect()
			return frame.Rect4(int32(x0), int32(y0), int32(x1), int32(y1))
		}
	}
	t.Fatal("the arena has no video box")
	return frame.Rect{}
}

func centre(r frame.Rect) (int32, int32) {
	return (r.X0 + r.X1) / 2, (r.Y0 + r.Y1) / 2
}

func activeSession(t *testing.T, f *framePath) *engine.Session {
	t.Helper()
	tab := f.tabMgr.Active()
	if tab == nil {
		t.Fatal("no active tab")
	}
	sess := tab.Session()
	if sess == nil {
		t.Fatal("the active tab has no session")
	}
	return sess
}

func TestMediaOptionWithoutBackendIsNoOption(t *testing.T) {
	if got := mediaOption(nil, net.DefaultClient(), "https://example.com/"); got != nil {
		t.Fatal("mediaOption produced an option with no backend")
	}
	if got := mediaOption(&stubBackend{}, nil, "https://example.com/"); got != nil {
		t.Fatal("mediaOption produced an option with no HTTP client")
	}
	if got := mediaOption(&stubBackend{}, net.DefaultClient(), "https://example.com/"); got == nil {
		t.Fatal("mediaOption produced nothing for a backend and a client")
	}
}

func TestMediaDamageUnionsAndScales(t *testing.T) {
	rects := []frame.RectF{
		{X0: 10, Y0: 20, X1: 110, Y1: 120},
		{X0: 200, Y0: 5, X1: 260, Y1: 40},
	}
	got := mediaDamage(rects, 2)
	want := frame.Rect4(20, 10, 520, 240)
	if got != want {
		t.Fatalf("mediaDamage at 2x = %v, want %v", got, want)
	}
	if got := mediaDamage(nil, 2); !got.Empty() {
		t.Fatalf("mediaDamage of nothing = %v, want empty", got)
	}
	got = mediaDamage([]frame.RectF{{X0: 0.25, Y0: 0.25, X1: 10.5, Y1: 10.5}}, 1)
	if got.X0 > 0 || got.Y0 > 0 || got.X1 < 11 || got.Y1 < 11 {
		t.Fatalf("mediaDamage at 1x rounded inwards: %v", got)
	}
}

func TestMediaControlPressPlaysAndPauses(t *testing.T) {
	b := &stubBackend{}
	f := mediaTestPath(t, b)
	sess := activeSession(t, f)
	p := b.first()

	c := paint.VideoControlLayout(videoBox(t, sess), 1)
	if !c.Has {
		t.Fatal("the element painted no control bar")
	}
	x, y := centre(c.Play)

	if !f.handleMediaControl(x, y) {
		t.Fatal("a press on the play button was not handled")
	}
	if plays, _, _, _ := p.counts(); plays != 1 {
		t.Fatalf("the play button issued %d play commands, want 1", plays)
	}
	f.pumpMedia()

	if !f.handleMediaControl(x, y) {
		t.Fatal("a second press on the play button was not handled")
	}
	if _, pauses, _, _ := p.counts(); pauses != 1 {
		t.Fatalf("pressing the button while playing issued %d pauses, want 1", pauses)
	}
}

func TestMediaControlPressIsTrusted(t *testing.T) {
	b := &stubBackend{}
	f := mediaTestPath(t, b)
	sess := activeSession(t, f)
	c := paint.VideoControlLayout(videoBox(t, sess), 1)
	x, y := centre(c.Play)

	f.handleMediaControl(x, y)
	if plays, _, _, _ := b.first().counts(); plays == 0 {
		t.Fatal("a real press did not authorize playback: activation was not set around it")
	}
	ctrl := sess.MediaController(videoElement(t, sess))
	if _, err := ctrl.Play(); err != media.ErrNotAllowed {
		t.Fatalf("play() outside a gesture = %v, want ErrNotAllowed", err)
	}
}

func TestMediaControlTrackSeeks(t *testing.T) {
	b := &stubBackend{}
	f := mediaTestPath(t, b)
	sess := activeSession(t, f)
	c := paint.VideoControlLayout(videoBox(t, sess), 1)
	if c.Track.Empty() {
		t.Fatal("the control bar has no track to click")
	}
	p := b.first()

	x := c.Track.X0 + (c.Track.X1-c.Track.X0)/4
	y := (c.Track.Y0 + c.Track.Y1) / 2
	if !f.handleMediaControl(x, y) {
		t.Fatal("a press on the track was not handled")
	}
	_, _, seeks, _ := p.counts()
	if len(seeks) != 1 {
		t.Fatalf("the track press issued %d seeks, want 1", len(seeks))
	}
	if seeks[0] < 2.0 || seeks[0] > 3.0 {
		t.Fatalf("a press a quarter along a 10s track sought to %v, want about 2.5", seeks[0])
	}
}

func TestMediaControlMuteToggles(t *testing.T) {
	b := &stubBackend{}
	f := mediaTestPath(t, b)
	sess := activeSession(t, f)
	c := paint.VideoControlLayout(videoBox(t, sess), 1)
	if c.Mute.Empty() {
		t.Fatal("the control bar has no mute button")
	}
	x, y := centre(c.Mute)
	p := b.first()

	f.handleMediaControl(x, y)
	f.handleMediaControl(x, y)
	_, _, _, mutes := p.counts()
	if len(mutes) != 2 || mutes[0] != true || mutes[1] != false {
		t.Fatalf("two presses on mute sent %v, want [true false]", mutes)
	}
}

func TestMediaControlMissFallsThrough(t *testing.T) {
	b := &stubBackend{}
	f := mediaTestPath(t, b)
	sess := activeSession(t, f)
	box := videoBox(t, sess)
	c := paint.VideoControlLayout(box, 1)

	midPicture := frame.Point{X: (box.X0 + box.X1) / 2, Y: box.Y0 + (c.Bar.Y0-box.Y0)/2}
	if f.handleMediaControl(midPicture.X, midPicture.Y) {
		t.Fatal("a press on the picture was handled as a control press")
	}
	offVideo := frame.Point{X: box.X1 + 50, Y: box.Y0 + 5}
	if f.handleMediaControl(offVideo.X, offVideo.Y) {
		t.Fatal("a press beside the video was handled as a control press")
	}
	if plays, pauses, _, _ := b.first().counts(); plays != 0 || pauses != 0 {
		t.Fatalf("a missed press still moved playback: %d plays, %d pauses", plays, pauses)
	}
}

func TestMediaKeySpaceTogglesAFocusedVideo(t *testing.T) {
	b := &stubBackend{}
	f := mediaTestPath(t, b)
	sess := activeSession(t, f)
	tab := f.tabMgr.Active()
	p := b.first()

	if f.mediaKey(tab, ' ') {
		t.Fatal("space was consumed by a video that does not have focus")
	}

	if f.mediaKey(tab, ' ') {
		t.Fatal("space was consumed by a video that does not have focus")
	}

	c := paint.VideoControlLayout(videoBox(t, sess), 1)
	x, y := centre(c.Play)
	f.handleMediaControl(x, y)
	if sess.FocusedMediaElement() == nil {
		t.Fatal("pressing a control did not focus the video, so the keyboard can never reach it")
	}

	if !f.mediaKey(tab, ' ') {
		t.Fatal("space was not consumed by the focused video")
	}
	if _, pauses, _, _ := p.counts(); pauses == 0 {
		t.Fatal("space on a playing video did not pause it")
	}
	if !f.mediaKey(tab, 'm') {
		t.Fatal("m was not consumed by the focused video")
	}
	if _, _, _, mutes := p.counts(); len(mutes) == 0 || !mutes[len(mutes)-1] {
		t.Fatalf("m did not mute: %v", mutes)
	}
}

func TestCloseAllSessionsReleasesMedia(t *testing.T) {
	b := &stubBackend{}
	f := mediaTestPath(t, b)
	p := b.first()
	c := paint.VideoControlLayout(videoBox(t, activeSession(t, f)), 1)
	x, y := centre(c.Play)
	f.handleMediaControl(x, y)
	f.pumpMedia()

	f.closeAllSessions()
	p.mu.Lock()
	closes := p.closes
	p.mu.Unlock()
	if closes != 1 {
		t.Fatalf("closeAllSessions closed the player %d times, want 1", closes)
	}
}

func TestPauseMediaStopsTheDepartingTab(t *testing.T) {
	b := &stubBackend{}
	f := mediaTestPath(t, b)
	sess := activeSession(t, f)
	c := paint.VideoControlLayout(videoBox(t, sess), 1)
	x, y := centre(c.Play)
	f.handleMediaControl(x, y)
	f.pumpMedia()
	p := b.first()

	f.pauseMedia(f.tabMgr.Active())
	if _, pauses, _, _ := p.counts(); pauses == 0 {
		t.Fatal("switching away from a tab left its video playing")
	}
	if got := sess.MediaController(videoElement(t, sess)).Snapshot().State; got == media.StatePlaying {
		t.Fatal("the controller still reports playing after the tab was paused")
	}
}

func TestMediaFetcherGoesThroughTheBrowserClient(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte(stubMP4Body))
	}))
	defer srv.Close()

	client := net.DefaultClient()
	defer client.Close()
	resp, err := mediaFetcher(client, srv.URL)(t.Context(), srv.URL+"/clip.mp4")
	if err != nil {
		t.Fatalf("mediaFetcher: %v", err)
	}
	if gotPath != "/clip.mp4" {
		t.Fatalf("the client fetched %q, want /clip.mp4", gotPath)
	}
	if resp.StatusCode != 200 || len(resp.Body) == 0 || resp.URL == "" {
		t.Fatalf("the response came back as %+v", resp)
	}
}
