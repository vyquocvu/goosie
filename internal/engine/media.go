package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/internal/paint"
)

type MediaLoader interface {
	Load(ctx context.Context, base, url string) (MediaLease, error)
}

type MediaLease interface {
	Path() string
	Release()
}

const (
	maxMediaControllers = 16
	maxResidentPlayers  = 2
	maxMediaLoads       = 16
	maxMediaEvents      = 256
	hardMaxMediaEvents  = 1024
	timeUpdateInterval  = 250 * time.Millisecond
)

func WithMedia(backend media.Backend, fetch MediaFetcher, base string) Option {
	return func(s *Session) {
		if backend == nil || fetch == nil {
			return
		}
		s.media = &mediaRegistry{
			backend:    backend,
			loader:     &mediaDownloader{fetch: fetch, sub: s.sub},
			base:       base,
			activation: &atomic.Bool{},
			players:    &atomic.Int32{},
			ops:        &atomic.Uint64{},
		}
	}
}

type mediaRegistry struct {
	mu     sync.Mutex
	list   []*mediaController
	closed bool

	backend    media.Backend
	loader     MediaLoader
	base       string
	activation *atomic.Bool
	players    *atomic.Int32
	ops        *atomic.Uint64
}

func (s *Session) SetUserActivation(on bool) {
	if s.media != nil {
		s.media.activation.Store(on)
	}
	if on && s.jsRT != nil {
		s.jsRT.SetTransientUserActivation(true)
	}
}

func (s *Session) SetMediaActivation(on bool) { s.SetUserActivation(on) }

func (s *Session) MediaController(n *dom.Node) media.Controller {
	if s.media == nil || n == nil || !n.Element() || n.Data != "video" {
		return nil
	}
	c := s.media.controllerFor(n)
	if c == nil {
		return nil
	}
	return c
}

func (s *Session) MediaView(n *dom.Node) (paint.VideoView, bool) {
	if s.media == nil || n == nil {
		return paint.VideoView{}, false
	}
	s.media.mu.Lock()
	defer s.media.mu.Unlock()
	for _, c := range s.media.list {
		if c.node == n {
			return c.View(), true
		}
	}
	return paint.VideoView{}, false
}

func (r *mediaRegistry) controllerFor(n *dom.Node) *mediaController {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	for _, c := range r.list {
		if c.node == n {
			return c
		}
	}
	if len(r.list) >= maxMediaControllers {
		return nil
	}
	c := &mediaController{
		reg:   r,
		node:  n,
		loads: make(chan mediaLoadResult, maxMediaLoads),
		snap:  media.Snapshot{Duration: math.NaN()},
		view:  paint.VideoView{Duration: math.NaN(), State: paint.VideoIdle},
	}
	c.shownPos = math.NaN()
	c.url = mediaSourceURL(n)
	r.list = append(r.list, c)
	return c
}

func mediaSourceURL(n *dom.Node) string {
	if src := strings.TrimSpace(n.GetAttribute("src")); src != "" {
		return src
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if !c.Element() || c.Data != "source" {
			continue
		}
		if t := strings.ToLower(strings.TrimSpace(c.GetAttribute("type"))); t != "" &&
			!strings.HasPrefix(t, "video/mp4") {
			continue
		}
		if src := strings.TrimSpace(c.GetAttribute("src")); src != "" {
			return src
		}
	}
	return ""
}

func (r *mediaRegistry) controllerByID(id dom.NodeID) *mediaController {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.list {
		if c.node.ID == id {
			return c
		}
	}
	return nil
}

func (r *mediaRegistry) pump(root *dom.Node, arena *layout.Arena) []frame.RectF {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	list := append([]*mediaController(nil), r.list...)
	r.mu.Unlock()

	var damage []frame.RectF
	keep := list[:0]
	for _, c := range list {
		if root != nil && !nodeConnected(root, c.node) {
			c.Close()
			continue
		}
		if c.pump() && arena != nil {
			if id, ok := arena.ForNode(c.node.ID); ok {
				obj := arena.Get(id)
				obj.Media = c.View()
				x0, y0, x1, y1 := obj.BorderRect()
				damage = append(damage, frame.RectF4(x0, y0, x1, y1))
			}
		}
		keep = append(keep, c)
	}
	r.mu.Lock()
	if !r.closed {
		r.list = keep
	}
	r.mu.Unlock()
	return damage
}

func (r *mediaRegistry) drainEvents() []mediaDispatch {
	r.mu.Lock()
	list := append([]*mediaController(nil), r.list...)
	r.mu.Unlock()

	var out []mediaDispatch
	for _, c := range list {
		if ev := c.Events(); len(ev) > 0 {
			out = append(out, mediaDispatch{node: c.node, events: ev})
		}
	}
	return out
}

type mediaDispatch struct {
	node   *dom.Node
	events []media.Event
}

type MediaHit struct {
	Node     *dom.Node
	Part     paint.VideoControlPart
	Fraction float64
}

func (s *Session) MediaControlAt(x, y float32) (MediaHit, bool) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.Arena == nil {
		return MediaHit{}, false
	}
	pt := frame.Point{X: int32(math.Floor(float64(x))), Y: int32(math.Floor(float64(y)))}
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node == nil || obj.Node.Data != "video" || !obj.Node.HasAttribute("controls") {
			continue
		}
		x0, y0, x1, y1 := obj.ContentRect()
		c := paint.VideoControlLayout(frame.Rect4(int32(x0), int32(y0), int32(x1), int32(y1)), 1)
		if !c.Has {
			continue
		}
		part := c.At(pt)
		if part == paint.VideoControlNone {
			continue
		}
		return MediaHit{Node: obj.Node, Part: part, Fraction: c.TrackFraction(pt.X)}, true
	}
	return MediaHit{}, false
}

func (s *Session) PauseAllMedia() {
	if s.media == nil {
		return
	}
	s.media.mu.Lock()
	list := append([]*mediaController(nil), s.media.list...)
	s.media.mu.Unlock()
	for _, c := range list {
		c.Pause()
	}
}

func (s *Session) FocusedMediaElement() *dom.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	n := s.focus
	if n == nil || !n.Element() || n.Data != "video" {
		return nil
	}
	return n
}

func (s *Session) FocusMediaElement(n *dom.Node) bool {
	if n == nil || !n.Element() || n.Data != "video" || n.Doc != s.Doc {
		return false
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.focus = n
	s.focusState.Store(n)
	return true
}

func (r *mediaRegistry) closeAll() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	list := r.list
	r.list = nil
	loader := r.loader
	r.mu.Unlock()
	for _, c := range list {
		c.Close()
	}
	if owner, ok := loader.(mediaDirOwner); ok {
		owner.removeAll()
	}
}

type mediaDirOwner interface{ removeAll() }

func (r *mediaRegistry) acquirePlayer() bool {
	for {
		cur := r.players.Load()
		if int(cur) >= maxResidentPlayers {
			return false
		}
		if r.players.CompareAndSwap(cur, cur+1) {
			return true
		}
	}
}

func (r *mediaRegistry) releasePlayer() {
	for {
		cur := r.players.Load()
		if cur <= 0 {
			return
		}
		if r.players.CompareAndSwap(cur, cur-1) {
			return
		}
	}
}

func nodeConnected(root, n *dom.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p == root {
			return true
		}
	}
	return false
}

type mediaLoadResult struct {
	gen   uint64
	lease MediaLease
	err   error
}

type mediaController struct {
	reg  *mediaRegistry
	node *dom.Node

	mu sync.Mutex

	url        string
	currentSrc string
	loadGen    uint64
	state      media.State
	snap       media.Snapshot
	failCode   media.ErrorCode
	player     media.Player
	lease      MediaLease
	cancel     context.CancelFunc
	loads      chan mediaLoadResult
	events     []media.Event
	closed     bool
	resident   bool

	wantPlay   bool
	playOp     uint64
	playIssued bool

	lastSeek uint64
	seeking  bool

	shownPos       float64
	lastTimeUpdate time.Time

	view    paint.VideoView
	hasView bool
}

func (c *mediaController) View() paint.VideoView {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.view
}

func (c *mediaController) Snapshot() media.Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snap
}

func (c *mediaController) Events() []media.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.events
	c.events = nil
	return out
}

func (c *mediaController) Muted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snap.Muted
}

func (c *mediaController) SetSource(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		c.snap.State = media.StateClosed
		return
	}
	had := c.url != ""
	if had && url == c.url && c.state != media.StateEmpty {
		return
	}
	c.url = url
	c.currentSrc = ""
	c.releaseLocked()
	c.settlePlayLocked(media.CodeAborted, false)
	c.resetLocked()
	if had {
		c.emitLocked(media.Event{Kind: media.EvEmptied})
	}
}

func (c *mediaController) Load() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.url == "" {
		c.state = media.StateEmpty
		c.snap.State = media.StateEmpty
		return
	}
	if c.state == media.StateLoading || c.state == media.StatePreparing {
		return
	}
	c.releaseLocked()
	c.settlePlayLocked(media.CodeAborted, false)
	c.startLoadLocked()
}

func (c *mediaController) Play() (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, media.ErrClosed
	}
	if c.url == "" {
		c.failLocked(media.CodeUnsupported, errors.New("media: the element has no source"))
		return 0, media.ErrUnsupported
	}
	if !c.reg.activation.Load() {
		c.emitLocked(media.Event{Kind: media.EvPlayRejected, Op: c.nextOpLocked(), Code: media.CodeNotAllowed})
		return 0, media.ErrNotAllowed
	}
	op := c.nextOpLocked()
	c.wantPlay = true
	c.playOp = op
	c.playIssued = false
	c.emitLocked(media.Event{Kind: media.EvPlay, Op: op})
	switch c.state {
	case media.StatePlaying:
		c.settlePlayLocked(0, true)
	case media.StateReady, media.StatePaused, media.StateEnded, media.StateSeeking:
		c.startPlaybackLocked()
	case media.StateLoading, media.StatePreparing:
	default:
		c.startLoadLocked()
	}
	return op, nil
}

func (c *mediaController) Pause() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.state == media.StateClosed {
		return
	}
	if c.state == media.StateLoading || c.state == media.StatePreparing {
		c.releaseLocked()
	} else if c.player != nil {
		c.player.Pause()
	}
	c.settlePlayLocked(media.CodeAborted, false)
	c.setStateLocked(media.StatePaused)
}

func (c *mediaController) Seek(seconds float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.player == nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return
	}
	if d := c.snap.Duration; !math.IsNaN(d) && seconds > d {
		seconds = d
	}
	op := c.nextOpLocked()
	c.lastSeek = op
	c.seeking = true
	c.player.Seek(op, seconds)
	c.setStateLocked(media.StateSeeking)
}

func (c *mediaController) SetMuted(muted bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.snap.Muted == muted {
		return
	}
	c.snap.Muted = muted
	if c.player != nil {
		c.player.SetMuted(muted)
	}
	c.emitLocked(media.Event{Kind: media.EvVolumeChange})
}

func (c *mediaController) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	c.releaseLocked()
	c.settlePlayLocked(media.CodeAborted, false)
	c.state = media.StateClosed
	c.snap = media.Snapshot{State: media.StateClosed, Duration: math.NaN()}
	c.view = paint.VideoView{State: paint.VideoIdle, Duration: math.NaN()}
	c.events = nil
}

func (c *mediaController) nextOpLocked() uint64 {
	return c.reg.ops.Add(1)
}

func (c *mediaController) CurrentSrc() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.currentSrc == "" || c.state == media.StateEmpty || c.state == media.StateClosed {
		return ""
	}
	return c.currentSrc
}

func (c *mediaController) resetLocked() {
	c.snap = media.Snapshot{State: media.StateEmpty, Duration: math.NaN()}
	c.state = media.StateEmpty
	c.failCode = 0
	c.seeking = false
	c.lastSeek = 0
	c.shownPos = math.NaN()
	c.publishLocked()
}

func (c *mediaController) releaseLocked() {
	c.loadGen++
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if c.player != nil {
		c.player.Close()
		c.player = nil
	}
	c.playIssued = false
	if c.resident {
		c.resident = false
		c.reg.releasePlayer()
	}
	if c.lease != nil {
		c.lease.Release()
		c.lease = nil
	}
	c.seeking = false
	c.lastSeek = 0
}

func (c *mediaController) settlePlayLocked(code media.ErrorCode, ok bool) {
	if !c.wantPlay {
		return
	}
	c.wantPlay = false
	c.playIssued = false
	op := c.playOp
	c.playOp = 0
	if ok {
		c.emitLocked(media.Event{Kind: media.EvPlayResolved, Op: op})
		return
	}
	c.emitLocked(media.Event{Kind: media.EvPlayRejected, Op: op, Code: code})
}

func (c *mediaController) failLocked(code media.ErrorCode, err error) {
	c.releaseLocked()
	c.state = media.StateFailed
	c.snap.State = media.StateFailed
	c.snap.Err = err
	c.snap.Pixels = nil
	c.failCode = code
	c.emitLocked(media.Event{Kind: media.EvError, Code: code})
	c.settlePlayLocked(code, false)
	c.publishLocked()
}

func (c *mediaController) startLoadLocked() {
	abs, ok := resolveSubresourceURL(c.reg.base, c.url)
	if !ok {
		c.failLocked(media.CodeUnsupported,
			fmt.Errorf("%w: %s is not a source this document may fetch", errMediaRefused, c.url))
		return
	}
	c.currentSrc = abs
	c.loadGen++
	gen := c.loadGen
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.state = media.StateLoading
	c.snap.State = media.StateLoading
	c.snap.Duration = math.NaN()
	c.snap.Pixels = nil
	c.emitLocked(media.Event{Kind: media.EvLoadStart})
	c.publishLocked()

	loader, base := c.reg.loader, c.reg.base
	loads := c.loads
	go func() {
		lease, err := loader.Load(ctx, base, abs)
		select {
		case loads <- mediaLoadResult{gen: gen, lease: lease, err: err}:
		default:
			if lease != nil {
				lease.Release()
			}
		}
	}()
}

func (c *mediaController) startPlaybackLocked() {
	if c.player == nil {
		return
	}
	if c.state == media.StateEnded {
		c.player.Seek(c.nextOpLocked(), 0)
	}
	c.player.Play(c.playOp)
	c.playIssued = true
}

func (c *mediaController) pump() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	for {
		select {
		case res := <-c.loads:
			c.applyLoadLocked(res)
		default:
			if c.player != nil {
				c.applySnapshotLocked(c.player.Poll())
			}
			return c.publishLocked()
		}
	}
}

func (c *mediaController) applyLoadLocked(res mediaLoadResult) {
	if c.closed || res.gen != c.loadGen {
		if res.lease != nil {
			res.lease.Release()
		}
		return
	}
	if res.err != nil {
		if res.lease != nil {
			res.lease.Release()
		}
		c.failLocked(codeForError(res.err), res.err)
		return
	}
	c.lease = res.lease
	if !c.reg.acquirePlayer() {
		c.failLocked(media.CodeUnsupported,
			errors.New("media: this document is already playing as many videos as it may"))
		return
	}
	c.resident = true
	player, err := c.reg.backend.Open(res.lease.Path())
	if err != nil {
		c.failLocked(codeForError(err), err)
		return
	}
	c.player = player
	if c.snap.Muted {
		player.SetMuted(true)
	}
	c.setStateLocked(media.StatePreparing)
}

func (c *mediaController) applySnapshotLocked(s media.Snapshot) {
	if s.State == media.StateFailed {
		err := s.Err
		if err == nil {
			err = errors.New("media: playback failed")
		}
		c.failLocked(codeForError(err), err)
		return
	}
	if s.Err != nil {
		return
	}

	if math.IsNaN(c.snap.Duration) && !math.IsNaN(s.Duration) {
		c.snap.Duration = s.Duration
		c.emitLocked(media.Event{Kind: media.EvLoadedMetadata})
	}
	if c.snap.Width == 0 && s.Width > 0 {
		c.snap.Width, c.snap.Height = s.Width, s.Height
	}
	if s.Pixels != nil && s.Seq >= c.snap.Seq {
		first := c.snap.Pixels == nil
		c.snap.Seq = s.Seq
		c.snap.Pixels = s.Pixels
		if first {
			c.emitLocked(media.Event{Kind: media.EvLoadedData})
			c.emitLocked(media.Event{Kind: media.EvCanPlay})
		}
	}

	next := s.State
	if next == media.StatePreparing && !math.IsNaN(c.snap.Duration) && c.snap.Pixels != nil {
		next = media.StateReady
	}
	if next == media.StatePreparing || next == media.StateLoading || next == media.StateClosed {
		next = c.state
	}
	if c.seeking && next != media.StateSeeking && s.DoneOp != c.lastSeek {
		next = c.state
	}
	if next != c.state {
		c.setStateLocked(next)
	} else {
		c.snap.State = next
	}

	if c.wantPlay && !c.playIssued && c.state == media.StateReady {
		c.startPlaybackLocked()
	}

	if s.DoneOp != 0 {
		switch {
		case c.wantPlay && s.DoneOp == c.playOp:
			c.settlePlayLocked(media.CodeDecode, c.state == media.StatePlaying)
		case c.seeking && s.DoneOp == c.lastSeek:
			c.seeking = false
			c.lastSeek = 0
			c.snap.Position = s.Position
			c.shownPos = s.Position
			c.emitLocked(media.Event{Kind: media.EvSeeked})
		}
	}

	if s.Position != c.snap.Position {
		c.snap.Position = s.Position
	}
	if c.state == media.StatePlaying {
		if now := time.Now(); now.Sub(c.lastTimeUpdate) >= timeUpdateInterval {
			c.lastTimeUpdate = now
			if c.shownPos != c.snap.Position {
				c.shownPos = c.snap.Position
				c.emitLocked(media.Event{Kind: media.EvTimeUpdate, Position: s.Position})
			}
		}
	} else if !c.seeking && c.shownPos != c.snap.Position {
		c.shownPos = c.snap.Position
	}
}

func (c *mediaController) setStateLocked(next media.State) {
	if next == c.state {
		c.snap.State = next
		return
	}
	c.state = next
	c.snap.State = next
	switch next {
	case media.StatePlaying:
		c.emitLocked(media.Event{Kind: media.EvPlaying})
		c.settlePlayLocked(0, true)
		c.lastTimeUpdate = time.Now()
		c.shownPos = c.snap.Position
	case media.StatePaused:
		c.emitLocked(media.Event{Kind: media.EvPause})
		c.settlePlayLocked(media.CodeAborted, false)
		c.shownPos = c.snap.Position
	case media.StateSeeking:
		c.emitLocked(media.Event{Kind: media.EvSeeking})
	case media.StateEnded:
		c.emitLocked(media.Event{Kind: media.EvPause})
		c.emitLocked(media.Event{Kind: media.EvEnded})
		c.settlePlayLocked(media.CodeAborted, false)
		c.shownPos = c.snap.Position
	case media.StateFailed:
		c.emitLocked(media.Event{Kind: media.EvError, Code: c.failCode})
	}
}

func (c *mediaController) publishLocked() bool {
	v := c.buildViewLocked()
	if c.hasView && videoViewEqual(c.view, v) {
		return false
	}
	c.view = v
	c.hasView = true
	return true
}

func (c *mediaController) buildViewLocked() paint.VideoView {
	v := paint.VideoView{
		Pixels:   c.snap.Pixels,
		Width:    c.snap.Width,
		Height:   c.snap.Height,
		Position: c.shownPos,
		Duration: c.snap.Duration,
		Muted:    c.snap.Muted,
	}
	switch c.state {
	case media.StateLoading, media.StatePreparing, media.StateSeeking:
		v.State = paint.VideoLoading
		v.Message = "Loading…"
	case media.StateFailed:
		v.State = paint.VideoFailed
		v.Message = "This video cannot be played"
	case media.StatePlaying:
		v.State = paint.VideoPlaying
	default:
		v.State = paint.VideoReady
	}
	return v
}

func videoViewEqual(a, b paint.VideoView) bool {
	sameDuration := a.Duration == b.Duration || (math.IsNaN(a.Duration) && math.IsNaN(b.Duration))
	samePosition := a.Position == b.Position || (math.IsNaN(a.Position) && math.IsNaN(b.Position))
	return a.Pixels == b.Pixels && a.State == b.State && samePosition && sameDuration &&
		a.Muted == b.Muted && a.Width == b.Width && a.Height == b.Height && a.Message == b.Message
}

func (c *mediaController) emitLocked(e media.Event) {
	if len(c.events) >= maxMediaEvents {
		for i, old := range c.events {
			if old.Kind.Coalescable() {
				c.events = append(c.events[:i], c.events[i+1:]...)
				break
			}
		}
	}
	if len(c.events) >= hardMaxMediaEvents {
		return
	}
	c.events = append(c.events, e)
}

func codeForError(err error) media.ErrorCode {
	switch {
	case errors.Is(err, media.ErrDecode):
		return media.CodeDecode
	case errors.Is(err, media.ErrUnsupported), errors.Is(err, errMediaRefused):
		return media.CodeUnsupported
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return media.CodeAborted
	}
	return media.CodeNetwork
}
