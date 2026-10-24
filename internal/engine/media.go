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

// MediaLoader prepares a media source into a local file a backend can open.
// The browser's own HTTP client is what implements it, so media downloads keep
// the cookies, redirect limits, shields and private-network rules every other
// subresource goes through, and a native media stack never sees a URL.
type MediaLoader interface {
	// Load fetches url, resolved against base, completely, and returns a lease
	// on the local file holding it. It blocks until the file is ready or ctx is
	// cancelled, and runs on its own goroutine rather than the session's.
	Load(ctx context.Context, base, url string) (MediaLease, error)
}

// MediaLease is ownership of one downloaded media file. Release removes it and
// must be called exactly once, after the backend has finished with the path.
type MediaLease interface {
	Path() string
	Release()
}

const (
	// maxMediaControllers bounds how many elements one document can put into
	// playback. Without it a page that inserts a video per timer tick would
	// grow a controller, a download slot and a decoder each time.
	maxMediaControllers = 16
	// maxResidentPlayers bounds how many of those may hold a decoder at once.
	// A decoder is the expensive part of a video: past two, a page that keeps
	// adding elements is spending the machine rather than showing anything.
	maxResidentPlayers = 2
	// maxMediaLoads is the depth of one controller's load-result queue. A
	// worker whose result nobody reads releases its own file rather than
	// blocking forever holding it.
	maxMediaLoads = 16
	// maxMediaEvents bounds one controller's undrained event queue, and
	// hardMaxMediaEvents is where even a terminal event is refused. A page that
	// never drains loses timeupdates first; see emitLocked.
	maxMediaEvents     = 256
	hardMaxMediaEvents = 1024
	// timeUpdateInterval is how often a playing element reports its position.
	// The painted progress bar reads the same cadence, so a playing video
	// damages its control bar four times a second rather than every tick.
	timeUpdateInterval = 250 * time.Millisecond
)

// WithMedia supplies the playback backend and the fetcher that brings a source
// in. base is the document's final URL, which media sources resolve against.
// The engine owns the URL policy, the size caps and the temporary file the
// bytes land in; the host owns the transport, so media downloads keep the same
// cookies, redirect limit and private-network guard as every other subresource.
//
// Without this option a <video> stays the inert placeholder paint already draws,
// and asking one to play reports that no backend exists rather than pretending
// to succeed.
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

// mediaRegistry owns one document's media controllers.
//
// backend, loader and base are written once by the option and read without the
// lock afterwards. activation is an atomic shared with every controller so a
// play request can consult it without taking two locks in an order the pump
// takes them the other way round.
type mediaRegistry struct {
	mu     sync.Mutex
	list   []*mediaController
	closed bool

	backend    media.Backend
	loader     MediaLoader
	base       string
	activation *atomic.Bool
	// players counts the resident decoders this document holds, against
	// maxResidentPlayers. It is atomic for the reason acquirePlayer gives.
	players *atomic.Int32
	// ops hands out operation ids document-wide, so an id names one request
	// rather than one request per element.
	ops *atomic.Uint64
}

// SetMediaActivation records whether a trusted user gesture is in progress.
// The host sets it around its own pointer and key dispatch and clears it
// afterwards: activation is what separates a click on the painted play button
// from a script calling play() on a timer, and only the host can tell those
// apart.
func (s *Session) SetMediaActivation(on bool) {
	if s.media != nil {
		s.media.activation.Store(on)
	}
}

// MediaController returns the controller bound to a media element, creating it
// the first time something asks. It returns nil when the session has no backend
// or the node is not a video element, which is how the JS binding decides
// whether the element gets a media API at all.
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

// MediaView is the presentation state paint will draw for a media element. It
// reports false when the element has no controller.
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
	// The source the markup named is recorded but not fetched. This engine
	// ignores preload hints, so a page that embeds forty videos pays nothing
	// until something asks one to play.
	c.url = mediaSourceURL(n)
	r.list = append(r.list, c)
	return c
}

// mediaSourceURL picks an element's source: the src attribute wins, and failing
// that the first <source> child that names a container this engine can play. A
// child with no type at all is taken at its word, because refusing it would
// refuse the common case of a bare <source src="clip.mp4">.
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

// controllerByID is the reflow-time lookup: the arena walks objects, not nodes,
// so attachment asks by ID.
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

// pump advances every controller one step and returns the document-space
// rectangles whose painting changed. Elements that left the document are closed
// here rather than leaking a decoder for a node nothing can reach any more.
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

// drainEvents takes every queued transition off the controllers, paired with the
// element it belongs to. It is a separate step from pump because the events go to
// the JavaScript runtime, and running page code is not something to do while
// holding the pipeline lock.
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

// mediaDispatch is one element's drained transitions, waiting to be handed to the
// runtime that owns its listeners.
type mediaDispatch struct {
	node   *dom.Node
	events []media.Event
}

// MediaHit is one pointer press that landed on a video element's painted
// controls.
type MediaHit struct {
	// Node is the element whose controls were hit.
	Node *dom.Node
	// Part is which control.
	Part paint.VideoControlPart
	// Fraction is where along the seek track the press landed, 0..1. It is only
	// meaningful for VideoControlTrack.
	Fraction float64
}

// MediaControlAt reports which of a video element's controls covers a
// content-space point, in CSS pixels. It returns false when the point is not on a
// control — including when it is on a video with no controls attribute, whose box
// is then just content, and clicking it belongs to the page.
//
// The geometry comes from the same function the painter uses, at scale 1 so both
// sides are speaking CSS pixels. That sharing is the point: a control whose ink
// and whose click target are computed separately drift apart the first time
// either is edited, and the result is a button a user can see and cannot press.
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

// PauseAllMedia stops every element in the document that is playing. It is what a
// tab switch calls: a background tab that keeps playing is a page making noise the
// user cannot see the source of, and nothing resumes it when they come back, which
// is also what the DOM's visibilitychange behaviour amounts to.
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

// FocusedMediaElement is the focused node when it is a video element this document
// can play, and nil otherwise. Keyboard control of a video belongs to the element
// with focus, which is the same rule form controls follow.
func (s *Session) FocusedMediaElement() *dom.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	n := s.focus
	if n == nil || !n.Element() || n.Data != "video" {
		return nil
	}
	return n
}

// FocusMediaElement gives a video element the focus, so the keyboard controls it
// afterwards. It reports false when the node is not a video this document can
// play. A video is not one of the form controls IsActivatable admits, so clicking
// its painted controls is the only way it can ever hold focus, and without that a
// keyboard user has no route to the space bar pausing it.
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
	// The directory goes only after every controller has released its lease, so
	// nothing is handed a path whose file was just unlinked.
	if owner, ok := loader.(mediaDirOwner); ok {
		owner.removeAll()
	}
}

// mediaDirOwner is a loader that keeps its files in a directory it can drop
// wholesale when the document goes away.
type mediaDirOwner interface{ removeAll() }

// acquirePlayer takes one of the document's resident decoder slots. It is
// lock-free on purpose: the controller calls it while holding its own mutex,
// and the pump takes the registry mutex before the controller's, so a registry
// lock here would be the same two locks in the opposite order.
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

// mediaLoadResult is one download worker's outcome. The gen it carries is the
// controller's load generation when the worker started, which is how a result
// for a source the page has already replaced is recognised and its file
// released instead of opened.
type mediaLoadResult struct {
	gen   uint64
	lease MediaLease
	err   error
}

// mediaController is one video element's playback state.
//
// Every field below is guarded by mu and every method runs on the session
// owner's goroutine. The one other party is the download worker, and it only
// ever sends on loads: no worker touches this state directly, which is what
// keeps the media pump and the JS pump from needing to agree on a lock order.
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
	// resident records that this controller holds one of the document's decoder
	// slots, so releasing it is not a guess about whether it was ever taken.
	resident bool

	// wantPlay and playOp are the play request waiting for the asset. The
	// gesture that authorized it stays attached across the download, so a
	// click on the play button still plays once the bytes arrive.
	wantPlay bool
	playOp   uint64
	// playIssued says the request has been handed to the player, so a backend
	// that takes a few polls to report playback is not asked to start again on
	// every one of them.
	playIssued bool

	lastSeek uint64
	seeking  bool

	// shownPos is the position the painted control bar displays. It advances
	// on the timeupdate cadence rather than every poll, so a playing video
	// damages its controls four times a second instead of every tick.
	shownPos       float64
	lastTimeUpdate time.Time

	view    paint.VideoView
	hasView bool
}

// View is the presentation state paint draws. It is a value, so handing it to
// a display list cannot be undone by the next poll.
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

// SetSource records a new source without fetching it. The browser ignores
// preload hints, so bytes only move on Load or on a play request the user is
// allowed to make; a page that names forty sources in a loop pays nothing until
// it asks to play one.
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

// Load starts fetching the selected source.
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
		// Already on the way; playback starts when the asset is ready.
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
		// Nothing is playing yet. Cancelling rather than remembering the pause
		// is what stops a download the user walked away from completing and
		// starting behind their back.
		c.releaseLocked()
	} else if c.player != nil {
		c.player.Pause()
	}
	// Settled here rather than in setStateLocked: pausing an already-paused
	// element is not a transition, but a play request it strands still has to
	// be rejected rather than left hanging.
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
	// Allocated document-wide rather than per element, so an operation id names
	// one request in the whole document. A page with two videos would otherwise
	// hand the same id to both, and whatever settles a play() promise by id would
	// resolve the wrong one.
	return c.reg.ops.Add(1)
}

// CurrentSrc is the absolute URL the element resolved its source to, or "" while
// nothing is selected. It is the browser's answer rather than the author's
// attribute, which is the distinction the DOM draws between src and currentSrc.
func (c *mediaController) CurrentSrc() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.currentSrc == "" || c.state == media.StateEmpty || c.state == media.StateClosed {
		return ""
	}
	return c.currentSrc
}

// resetLocked puts the snapshot back to "nothing loaded" without touching the
// pending play request, which callers settle with the code that fits their
// reason for resetting.
func (c *mediaController) resetLocked() {
	c.snap = media.Snapshot{State: media.StateEmpty, Duration: math.NaN()}
	c.state = media.StateEmpty
	c.failCode = 0
	c.seeking = false
	c.lastSeek = 0
	c.shownPos = math.NaN()
	c.publishLocked()
}

// releaseLocked cancels in-flight work and frees the player and the downloaded
// file. It also retires the load generation, so a worker that was already on
// its way back has its result discarded and its file released rather than
// opened behind the caller's back.
//
// It deliberately leaves a pending play request alone: only the caller knows
// why the work stopped, and the promise has to be rejected with that reason
// rather than left unsettled forever.
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

// settlePlayLocked finishes a pending play request. A request that already
// settled is not settled twice: a promise has one outcome.
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
			// Nobody is reading: a full queue means the controller is closed or
			// far behind, and either way holding the file would leak it.
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
	// Recorded so a backend that takes a few polls to report playback is not
	// asked to start again on every one of them.
	c.playIssued = true
}

// pump runs one owner-thread step: it applies whatever the download workers
// produced, polls the backend, and reports whether the painted view moved.
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
	// A backend reports StatePreparing until it has something to show. Once
	// metadata and a frame have arrived the element is ready whether or not the
	// backend bothered to say so.
	if next == media.StatePreparing && !math.IsNaN(c.snap.Duration) && c.snap.Pixels != nil {
		next = media.StateReady
	}
	if next == media.StatePreparing || next == media.StateLoading || next == media.StateClosed {
		next = c.state
	}
	if c.seeking && next != media.StateSeeking && s.DoneOp != c.lastSeek {
		// A position report that is not this seek's completion does not end it.
		next = c.state
	}
	if next != c.state {
		c.setStateLocked(next)
	} else {
		c.snap.State = next
	}

	// A play requested before the asset existed is still owed. This is the common
	// case rather than an edge one: the user clicks the painted play button on a
	// video that has not been fetched yet, so the request is recorded against a
	// download and has to be honoured when the decoder finally reports ready.
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
	// Publishing is throttled separately from the position changing: a backend can
	// report the same position twice in a row, and gating on the change would
	// leave the control bar showing a time the player has already moved past.
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

// setStateLocked moves to a state and emits what the transition means to a
// page. The states a backend reports are the truth; this only names them.
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

// publishLocked rebuilds the painted view and reports whether it moved, which
// is the only thing that makes a media tick worth a repaint.
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

// emitLocked appends an event, dropping the oldest coalescable one when the
// queue is full. A queue of nothing but terminal events grows to
// hardMaxMediaEvents before it starts refusing, because losing the settlement
// of a play() promise would leave a page waiting forever.
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
