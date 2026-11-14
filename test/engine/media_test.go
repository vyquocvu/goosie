package engine_test

import (
	"context"
	"errors"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/media"
)

const timeUpdateInterval = 250 * time.Millisecond

var mp4Header = []byte{
	0x00, 0x00, 0x00, 0x14,
	'f', 't', 'y', 'p',
	'i', 's', 'o', 'm',
	0x00, 0x00, 0x02, 0x00,
	'i', 's', 'o', 'm',
}

type fakeFetcher struct {
	mu       sync.Mutex
	urls     []string
	started  int
	gate     chan struct{}
	fail     error
	body     []byte
	status   int
	finalURL string
}

func newFakeFetcher() *fakeFetcher { return &fakeFetcher{body: mp4Header} }

func (f *fakeFetcher) Fetch(ctx context.Context, url string) (engine.MediaResponse, error) {
	f.mu.Lock()
	f.urls = append(f.urls, url)
	f.started++
	gate, fail, body, status, final := f.gate, f.fail, f.body, f.status, f.finalURL
	f.mu.Unlock()

	if fail != nil {
		return engine.MediaResponse{}, fail
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return engine.MediaResponse{}, ctx.Err()
		}
	}
	if body == nil {
		body = mp4Header
	}
	if status == 0 {
		status = 200
	}
	if final == "" {
		final = url
	}
	return engine.MediaResponse{StatusCode: status, Body: body, URL: final}, nil
}

func (f *fakeFetcher) requested() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.urls...)
}

func (f *fakeFetcher) starts() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.started
}

type seekReq struct {
	op      uint64
	seconds float64
}

type fakePlayer struct {
	mu     sync.Mutex
	snap   media.Snapshot
	plays  []uint64
	pauses int
	seeks  []seekReq
	closes int
}

func (p *fakePlayer) Play(op uint64) {
	p.mu.Lock()
	p.plays = append(p.plays, op)
	p.mu.Unlock()
}

func (p *fakePlayer) Pause() {
	p.mu.Lock()
	p.pauses++
	p.mu.Unlock()
}

func (p *fakePlayer) Seek(op uint64, seconds float64) {
	p.mu.Lock()
	p.seeks = append(p.seeks, seekReq{op, seconds})
	p.mu.Unlock()
}

func (p *fakePlayer) SetMuted(muted bool) {
	p.mu.Lock()
	p.snap.Muted = muted
	p.mu.Unlock()
}

func (p *fakePlayer) Poll() media.Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.snap
	p.snap.DoneOp = 0
	p.snap.Pixels = nil
	return out
}

func (p *fakePlayer) Close() {
	p.mu.Lock()
	p.closes++
	p.mu.Unlock()
}

func (p *fakePlayer) set(mut func(*media.Snapshot)) {
	p.mu.Lock()
	mut(&p.snap)
	p.mu.Unlock()
}

func (p *fakePlayer) playOps() []uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]uint64(nil), p.plays...)
}

func (p *fakePlayer) seekOps() []seekReq {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]seekReq(nil), p.seeks...)
}

func (p *fakePlayer) closeCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closes
}

type fakeBackend struct {
	mu      sync.Mutex
	players []*fakePlayer
	paths   []string
	fail    error
}

func newFakeBackend() *fakeBackend { return &fakeBackend{} }

func (b *fakeBackend) Name() string { return "fake" }

func (b *fakeBackend) Open(path string) (media.Player, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.paths = append(b.paths, path)
	if b.fail != nil {
		return nil, b.fail
	}
	p := &fakePlayer{snap: media.Snapshot{State: media.StatePreparing, Duration: math.NaN()}}
	b.players = append(b.players, p)
	return p, nil
}

func (b *fakeBackend) nth(i int) *fakePlayer {
	b.mu.Lock()
	defer b.mu.Unlock()
	if i < 0 || i >= len(b.players) {
		return nil
	}
	return b.players[i]
}

func (b *fakeBackend) path(i int) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if i < 0 || i >= len(b.paths) {
		return ""
	}
	return b.paths[i]
}

func (b *fakeBackend) openCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.players)
}

func solid(c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < 4*4; i++ {
		img.Pix[i*4+0] = c.R
		img.Pix[i*4+1] = c.G
		img.Pix[i*4+2] = c.B
		img.Pix[i*4+3] = c.A
	}
	return img
}

func exists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

const videoHTML = `<html><body><video id="v" controls width="320" height="180"></video></body></html>`

func videoNode(t *testing.T, doc *dom.Document) *dom.Node {
	t.Helper()
	nodes := doc.ElementsByTagName("video")
	if len(nodes) == 0 {
		t.Fatal("the fixture has no <video> element")
	}
	return nodes[0]
}

func newMediaSession(t *testing.T, html string, b *fakeBackend, f *fakeFetcher) *engine.Session {
	t.Helper()
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithMedia(b, f.Fetch, "https://example.com/page"),
	)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)
	return sess
}

func kinds(ev []media.Event) []media.EventKind {
	out := make([]media.EventKind, len(ev))
	for i, e := range ev {
		out[i] = e.Kind
	}
	return out
}

func hasKind(ev []media.Event, k media.EventKind) bool {
	for _, e := range ev {
		if e.Kind == k {
			return true
		}
	}
	return false
}

func pumpUntil(t *testing.T, sess *engine.Session, want func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		sess.PumpMedia()
		if want() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the controller never reached the expected state")
}

func TestMediaWithoutBackendIsInert(t *testing.T) {
	sess, err := engine.NewSession(videoHTML, nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	if c := sess.MediaController(videoNode(t, sess.Doc)); c != nil {
		t.Fatal("a session with no backend handed out a controller")
	}
	if got := sess.PumpMedia(); len(got) != 0 {
		t.Fatalf("PumpMedia with no media reported damage %v", got)
	}
}

func TestMediaPlayRequiresActivation(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))
	if c == nil {
		t.Fatal("no controller for the video element")
	}
	c.SetSource("clip.mp4")

	if _, err := c.Play(); !errors.Is(err, media.ErrNotAllowed) {
		t.Fatalf("Play without activation = %v, want ErrNotAllowed", err)
	}
	if got := f.starts(); got != 0 {
		t.Fatalf("a refused play started %d downloads", got)
	}
	var rejected *media.Event
	for _, e := range c.Events() {
		if e.Kind == media.EvPlayRejected {
			ev := e
			rejected = &ev
		}
	}
	if rejected == nil {
		t.Fatal("a refused play produced no EvPlayRejected")
	}
	if rejected.Code != media.CodeNotAllowed {
		t.Fatalf("rejection code = %v, want CodeNotAllowed", rejected.Code)
	}
}

func TestTrustedGestureRetainsDOMTransientActivationAfterDispatch(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := engine.NewSession(`<!doctype html><html><body><input id="picker" type="date"></body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	sess.SetUserActivation(true)
	sess.SetUserActivation(false)
	got, err := sess.EvaluateJS(`navigator.userActivation.isActive+'|'+navigator.userActivation.hasBeenActive`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "true|true" {
		t.Fatalf("activation after host event dispatch = %q, want true|true", got)
	}
	got, err = sess.EvaluateJS(`document.getElementById('picker').showPicker();navigator.userActivation.isActive+'|'+navigator.userActivation.hasBeenActive`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "false|true" {
		t.Fatalf("activation after showPicker = %q, want false|true", got)
	}
}

func TestMediaPlayAfterDownload(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))
	c.SetSource("clip.mp4")
	if got := c.Snapshot().State; got != media.StateEmpty {
		t.Fatalf("state right after SetSource = %v, want StateEmpty: naming a source must not fetch it", got)
	}
	if got := f.starts(); got != 0 {
		t.Fatalf("SetSource started %d downloads", got)
	}
	c.Load()
	if got := c.Snapshot().State; got != media.StateLoading {
		t.Fatalf("state right after Load = %v, want StateLoading", got)
	}

	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	if got := f.requested(); len(got) != 1 || got[0] != "https://example.com/clip.mp4" {
		t.Fatalf("fetcher saw %v, want the source resolved against the document", got)
	}
	p := b.nth(0)
	if p == nil {
		t.Fatal("the backend was never asked to open the downloaded file")
	}
	if !exists(b.path(0)) {
		t.Fatal("the downloaded file is not on disk where the backend was pointed")
	}
	if !hasKind(c.Events(), media.EvLoadStart) {
		t.Fatal("selecting a source produced no EvLoadStart")
	}

	p.set(func(s *media.Snapshot) {
		s.State = media.StateReady
		s.Duration = 12.5
		s.Width, s.Height = 320, 180
		s.Pixels = solid(color.RGBA{R: 255, A: 255})
		s.Seq = 1
	})
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StateReady })

	snap := c.Snapshot()
	if snap.Duration != 12.5 || snap.Width != 320 || snap.Height != 180 {
		t.Fatalf("metadata = %+v, want 12.5s at 320x180", snap)
	}
	ev := c.Events()
	for _, want := range []media.EventKind{media.EvLoadedMetadata, media.EvCanPlay} {
		if !hasKind(ev, want) {
			t.Fatalf("events %v are missing %v", kinds(ev), want)
		}
	}

	sess.SetMediaActivation(true)
	op, err := c.Play()
	if err != nil {
		t.Fatalf("Play with activation: %v", err)
	}
	sess.SetMediaActivation(false)
	if got := p.playOps(); len(got) != 1 || got[0] != op {
		t.Fatalf("player saw play ops %v, want exactly [%d]", got, op)
	}
	if got := c.Events(); hasKind(got, media.EvPlayResolved) {
		t.Fatal("play settled before the backend reported playback")
	}

	p.set(func(s *media.Snapshot) { s.State = media.StatePlaying; s.DoneOp = op })
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePlaying })
	ev = c.Events()
	if !hasKind(ev, media.EvPlaying) || !hasKind(ev, media.EvPlayResolved) {
		t.Fatalf("events %v are missing EvPlaying/EvPlayResolved", kinds(ev))
	}
	for _, e := range ev {
		if e.Kind == media.EvPlayResolved && e.Op != op {
			t.Fatalf("EvPlayResolved carried op %d, want %d", e.Op, op)
		}
	}
}

func TestMediaPauseDuringDownload(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	f.gate = make(chan struct{})
	defer close(f.gate)
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))
	c.SetSource("clip.mp4")

	sess.SetMediaActivation(true)
	op, err := c.Play()
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	sess.SetMediaActivation(false)
	pumpUntil(t, sess, func() bool { return f.starts() == 1 })

	c.Pause()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePaused })

	var aborted bool
	for _, e := range c.Events() {
		if e.Kind == media.EvPlayRejected && e.Op == op {
			aborted = e.Code == media.CodeAborted
		}
	}
	if !aborted {
		t.Fatal("the pending play was not rejected as aborted")
	}
	if got := b.openCount(); got != 0 {
		t.Fatalf("a paused download still opened %d players", got)
	}
}

func TestMediaLatestSourceWins(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))

	c.SetSource("first.mp4")
	c.Load()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	first := b.nth(0)
	firstPath := b.path(0)
	if first == nil {
		t.Fatal("the first source never opened")
	}

	c.SetSource("second.mp4")
	c.Load()
	pumpUntil(t, sess, func() bool { return b.openCount() == 2 })

	if got := first.closeCount(); got != 1 {
		t.Fatalf("the superseded player was closed %d times, want 1", got)
	}
	if exists(firstPath) {
		t.Fatalf("the superseded source's file is still on disk at %s", firstPath)
	}
	if !exists(b.path(1)) {
		t.Fatal("the live source's file went missing")
	}
	if !hasKind(c.Events(), media.EvEmptied) {
		t.Fatal("replacing a source produced no EvEmptied")
	}
	if got := f.requested(); len(got) != 2 ||
		got[0] != "https://example.com/first.mp4" || got[1] != "https://example.com/second.mp4" {
		t.Fatalf("fetcher saw %v, want first then second", got)
	}
}

func TestMediaSeekSupersession(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))
	c.SetSource("clip.mp4")
	c.Load()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	p := b.nth(0)
	p.set(func(s *media.Snapshot) {
		s.State = media.StateReady
		s.Duration = 10
		s.Width, s.Height = 320, 180
	})
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StateReady })
	_ = c.Events()

	c.Seek(2)
	c.Seek(7)
	ops := p.seekOps()
	if len(ops) != 2 {
		t.Fatalf("player saw %d seeks, want 2", len(ops))
	}
	if got := c.Snapshot().State; got != media.StateSeeking {
		t.Fatalf("state after seeking = %v, want StateSeeking", got)
	}

	p.set(func(s *media.Snapshot) { s.State = media.StatePaused; s.Position = 2; s.DoneOp = ops[0].op })
	sess.PumpMedia()
	if ev := c.Events(); hasKind(ev, media.EvSeeked) {
		t.Fatalf("a superseded seek settled: %v", kinds(ev))
	}

	p.set(func(s *media.Snapshot) { s.State = media.StatePaused; s.Position = 7; s.DoneOp = ops[1].op })
	pumpUntil(t, sess, func() bool { return hasKind(c.Events(), media.EvSeeked) })
	if got := c.Snapshot().Position; got != 7 {
		t.Fatalf("position after the newest seek = %v, want 7", got)
	}
}

func TestMediaLoadFailureIsReported(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	f.fail = errors.New("net: fetch failed")
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))
	c.SetSource("clip.mp4")

	sess.SetMediaActivation(true)
	op, err := c.Play()
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	sess.SetMediaActivation(false)

	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StateFailed })
	ev := c.Events()
	if !hasKind(ev, media.EvError) {
		t.Fatalf("a failed load produced %v, want EvError", kinds(ev))
	}
	var rejected bool
	for _, e := range ev {
		if e.Kind == media.EvPlayRejected && e.Op == op && e.Code == media.CodeNetwork {
			rejected = true
		}
	}
	if !rejected {
		t.Fatalf("the pending play was not rejected as a network error: %v", kinds(ev))
	}
	if got := b.openCount(); got != 0 {
		t.Fatal("a failed download still opened a player")
	}
}

func TestMediaUnsupportedBackend(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	b.fail = media.ErrUnsupported
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))
	c.SetSource("clip.mp4")
	c.Load()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StateFailed })

	if got := f.starts(); got != 1 {
		t.Fatalf("fetcher ran %d times, want 1", got)
	}
	if got := b.openCount(); got != 0 {
		t.Fatalf("a refused open still produced %d players", got)
	}
	if exists(b.path(0)) {
		t.Fatalf("an unsupported asset left its file at %s", b.path(0))
	}
}

func TestMediaCloseIsIdempotent(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))
	c.SetSource("clip.mp4")
	c.Load()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	p := b.nth(0)
	path := b.path(0)

	c.Close()
	c.Close()
	if got := p.closeCount(); got != 1 {
		t.Fatalf("player closed %d times, want 1", got)
	}
	if exists(path) {
		t.Fatalf("a closed controller left its file at %s", path)
	}
	if _, err := c.Play(); err == nil {
		t.Fatal("a closed controller accepted Play")
	}
	c.SetSource("other.mp4")
	if got := c.Snapshot().State; got != media.StateClosed {
		t.Fatalf("state after closing = %v, want StateClosed", got)
	}
	sess.PumpMedia()
}

func TestMediaSessionCloseReleasesEverything(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))
	c.SetSource("clip.mp4")
	c.Load()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	p := b.nth(0)
	dir := filepath.Dir(b.path(0))

	sess.Close()
	if got := p.closeCount(); got != 1 {
		t.Fatalf("Session.Close closed the player %d times, want 1", got)
	}
	if got := c.Snapshot().State; got != media.StateClosed {
		t.Fatalf("state after Session.Close = %v, want StateClosed", got)
	}
	if exists(dir) {
		t.Fatalf("Session.Close left the media directory at %s", dir)
	}
}

func TestMediaPumpWithoutJS(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	if sess.PumpJS(800) {
		t.Fatal("PumpJS reported work with no runtime")
	}
	c := sess.MediaController(videoNode(t, sess.Doc))
	c.SetSource("clip.mp4")
	c.Load()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	p := b.nth(0)
	p.set(func(s *media.Snapshot) {
		s.State = media.StatePlaying
		s.Duration = 4
		s.Width, s.Height = 320, 180
	})
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePlaying })
}

func TestMediaPumpReportsDamageOnlyOnChange(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))
	c.SetSource("clip.mp4")
	c.Load()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	p := b.nth(0)

	p.set(func(s *media.Snapshot) {
		s.State = media.StatePlaying
		s.Duration = 4
		s.Width, s.Height = 320, 180
		s.Pixels = solid(color.RGBA{G: 255, A: 255})
		s.Seq = 1
	})
	var dmg []frame.RectF
	for i := 0; i < 400 && len(dmg) == 0; i++ {
		dmg = sess.PumpMedia()
		time.Sleep(time.Millisecond)
	}
	if len(dmg) == 0 {
		t.Fatal("a new frame reported no damage")
	}
	r := dmg[0]
	if r.X1-r.X0 < 300 || r.Y1-r.Y0 < 150 {
		t.Fatalf("damage %v does not cover the video box", r)
	}

	for i := 0; i < 3; i++ {
		sess.PumpMedia()
	}
	if got := sess.PumpMedia(); len(got) != 0 {
		t.Fatalf("an idle video reported damage %v", got)
	}

	p.set(func(s *media.Snapshot) { s.Position = 0.1 })
	if got := sess.PumpMedia(); len(got) != 0 {
		t.Fatalf("a sub-interval position change reported damage %v", got)
	}
	time.Sleep(timeUpdateInterval + 20*time.Millisecond)
	p.set(func(s *media.Snapshot) { s.Position = 1.5 })
	if got := sess.PumpMedia(); len(got) == 0 {
		t.Fatal("a position change past the timeupdate interval reported no damage")
	}
}

func TestMediaFrameAttachedForPaint(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	node := videoNode(t, sess.Doc)
	c := sess.MediaController(node)
	c.SetSource("clip.mp4")
	c.Load()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	p := b.nth(0)
	want := solid(color.RGBA{B: 255, A: 255})
	p.set(func(s *media.Snapshot) {
		s.State = media.StatePlaying
		s.Duration = 4
		s.Width, s.Height = 320, 180
		s.Pixels = want
		s.Seq = 1
	})
	pumpUntil(t, sess, func() bool { return c.Snapshot().Seq == 1 })

	arena := sess.Arena
	dmg := sess.PumpMedia()
	if sess.Arena != arena {
		t.Fatal("PumpMedia rebuilt the layout arena")
	}
	view, ok := sess.MediaView(node)
	if !ok {
		t.Fatal("no presentation view was attached for the video element")
	}
	if view.Pixels != want {
		t.Fatal("the attached view does not carry the decoded frame")
	}
	if view.Position != 0 || view.Duration != 4 {
		t.Fatalf("view timing = %v/%v, want 0/4", view.Position, view.Duration)
	}
	if len(dmg) != 0 {
		t.Fatalf("re-pumping an unchanged frame reported damage %v", dmg)
	}
}

func TestMediaPlayBeforeTheSourceIsLoaded(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	c := sess.MediaController(videoNode(t, sess.Doc))
	c.SetSource("clip.mp4")

	sess.SetMediaActivation(true)
	op, err := c.Play()
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	sess.SetMediaActivation(false)
	if got := c.Snapshot().State; got != media.StateLoading {
		t.Fatalf("play on an unloaded element left it in %v, want StateLoading", got)
	}

	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	p := b.nth(0)
	if p == nil {
		t.Fatal("the backend was never asked to open the downloaded file")
	}
	if got := p.playOps(); len(got) != 0 {
		t.Fatalf("the player was told to play %v before the asset was ready", got)
	}

	p.set(func(s *media.Snapshot) {
		s.State = media.StateReady
		s.Duration = 6
		s.Width, s.Height = 320, 180
	})
	pumpUntil(t, sess, func() bool {
		got := p.playOps()
		return len(got) == 1 && got[0] == op
	})

	if hasKind(c.Events(), media.EvPlayResolved) {
		t.Fatal("play settled when the asset became ready rather than when playback started")
	}
	p.set(func(s *media.Snapshot) { s.State = media.StatePlaying; s.DoneOp = op })
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePlaying })
	if ev := c.Events(); !hasKind(ev, media.EvPlayResolved) {
		t.Fatalf("playback started but the request never settled: %v", kinds(ev))
	}

	p.set(func(s *media.Snapshot) { s.State = media.StateReady })
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StateReady })
	for i := 0; i < 5; i++ {
		sess.PumpMedia()
	}
	if got := p.playOps(); len(got) != 1 {
		t.Fatalf("the player was told to play %d times, want exactly the one request", len(got))
	}
}

func TestMediaRemovedElementIsClosed(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	node := videoNode(t, sess.Doc)
	c := sess.MediaController(node)
	c.SetSource("clip.mp4")
	c.Load()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	p := b.nth(0)
	path := b.path(0)

	node.Parent.RemoveChild(node)
	sess.PumpMedia()

	if got := p.closeCount(); got != 1 {
		t.Fatalf("a removed element closed its player %d times, want 1", got)
	}
	if exists(path) {
		t.Fatalf("a removed element left its file at %s", path)
	}
	if got := c.Snapshot().State; got != media.StateClosed {
		t.Fatalf("state after removal = %v, want StateClosed", got)
	}
	if got := sess.PumpMedia(); len(got) != 0 {
		t.Fatalf("a removed element still reported damage %v", got)
	}
}
