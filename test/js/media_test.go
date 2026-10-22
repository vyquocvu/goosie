package js_test

import (
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/test/domtest"
)

// fakeController is a scriptable media.Controller. It records what the bindings
// asked for and hands back whatever snapshot the test set, so the JS surface can
// be pinned without a decoder, a network or a display.
type fakeController struct {
	mu      sync.Mutex
	snap    media.Snapshot
	events  []media.Event
	current string
	playErr error
	playOp  uint64
	sources []string
	loads   int
	pauses  int
	seeks   []float64
	mutes   []bool
	closed  int
}

func newFakeController() *fakeController {
	return &fakeController{snap: media.Snapshot{Duration: math.NaN()}, playOp: 42}
}

func (c *fakeController) SetSource(url string) {
	c.mu.Lock()
	c.sources = append(c.sources, url)
	c.mu.Unlock()
}

func (c *fakeController) CurrentSrc() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

func (c *fakeController) Load() {
	c.mu.Lock()
	c.loads++
	c.mu.Unlock()
}

func (c *fakeController) Play() (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.playErr != nil {
		return 0, c.playErr
	}
	return c.playOp, nil
}

func (c *fakeController) Pause() {
	c.mu.Lock()
	c.pauses++
	c.mu.Unlock()
}

func (c *fakeController) Seek(seconds float64) {
	c.mu.Lock()
	c.seeks = append(c.seeks, seconds)
	c.mu.Unlock()
}

func (c *fakeController) SetMuted(muted bool) {
	c.mu.Lock()
	c.mutes = append(c.mutes, muted)
	c.snap.Muted = muted
	c.mu.Unlock()
}

func (c *fakeController) Muted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snap.Muted
}

func (c *fakeController) Snapshot() media.Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snap
}

func (c *fakeController) Events() []media.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.events
	c.events = nil
	return out
}

func (c *fakeController) Close() {
	c.mu.Lock()
	c.closed++
	c.mu.Unlock()
}

func (c *fakeController) set(mut func(*media.Snapshot)) {
	c.mu.Lock()
	mut(&c.snap)
	c.mu.Unlock()
}

func (c *fakeController) queue(ev ...media.Event) {
	c.mu.Lock()
	c.events = append(c.events, ev...)
	c.mu.Unlock()
}

const mediaDoc = `<html><body><video id="v" src="clip.mp4" controls></video></body></html>`

func videoOf(doc *dom.Document) *dom.Node {
	nodes := doc.ElementsByTagName("video")
	if len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

// mediaRuntime builds a runtime with the given controller wired to the document's
// video element. A nil controller models a build with no playback backend.
func mediaRuntime(t *testing.T, html string, c media.Controller) (*js.Runtime, *dom.Document) {
	t.Helper()
	doc := domtest.Parse(html)
	rt, err := js.New(js.Options{
		URL:     "https://example.com/page",
		DOM:     doc,
		Timeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	rt.SetMediaProvider(func(n *dom.Node) media.Controller {
		if n == nil || n.Data != "video" {
			return nil
		}
		return c
	})
	return rt, doc
}

// evalString runs a script and returns its string result.
func evalString(t *testing.T, rt *js.Runtime, src string) string {
	t.Helper()
	v, err := rt.Evaluate(src)
	if err != nil {
		t.Fatalf("Evaluate(%s): %v", src, err)
	}
	return v
}

// TestMediaInertWithoutBackend pins what a page sees when nothing can play video.
// The properties have to exist and answer with the DOM's own "nothing loaded"
// values; a missing property would turn a feature check into a TypeError, and a
// duration of 0 would make every seek clamp to the start.
func TestMediaInertWithoutBackend(t *testing.T) {
	rt, _ := mediaRuntime(t, mediaDoc, nil)
	out := evalString(t, rt, `
		var v = document.getElementById('v');
		JSON.stringify({
			durationNaN: v.duration !== v.duration,
			currentTimeNaN: v.currentTime !== v.currentTime,
			paused: v.paused,
			ended: v.ended,
			seeking: v.seeking,
			readyState: v.readyState,
			error: v.error,
			videoWidth: v.videoWidth,
			videoHeight: v.videoHeight,
			currentSrc: v.currentSrc,
			src: v.src,
			controls: v.controls,
			hasPlay: typeof v.play,
			hasPause: typeof v.pause,
			hasLoad: typeof v.load
		});
	`)
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if got["durationNaN"] != true || got["currentTimeNaN"] != true {
		t.Fatalf("duration/currentTime are not NaN: %v", out)
	}
	if got["paused"] != true {
		t.Fatalf("paused = %v, want true", got["paused"])
	}
	if got["ended"] != false || got["seeking"] != false {
		t.Fatalf("ended/seeking = %v/%v, want false", got["ended"], got["seeking"])
	}
	if got["readyState"] != float64(0) {
		t.Fatalf("readyState = %v, want 0", got["readyState"])
	}
	if got["error"] != nil {
		t.Fatalf("error = %v, want null", got["error"])
	}
	if got["videoWidth"] != float64(0) || got["videoHeight"] != float64(0) {
		t.Fatalf("videoWidth/Height = %v/%v, want 0", got["videoWidth"], got["videoHeight"])
	}
	if got["currentSrc"] != "" {
		t.Fatalf("currentSrc = %v, want empty", got["currentSrc"])
	}
	if got["src"] != "clip.mp4" {
		t.Fatalf("src = %v, want the attribute value", got["src"])
	}
	if got["controls"] != true {
		t.Fatalf("controls = %v, want true", got["controls"])
	}
	for _, m := range []string{"hasPlay", "hasPause", "hasLoad"} {
		if got[m] != "function" {
			t.Fatalf("%s = %v, want function", m, got[m])
		}
	}
}

// TestMediaPropertiesReflectTheController pins that the read-only properties are
// the backend's state rather than a copy the bindings keep for themselves.
func TestMediaPropertiesReflectTheController(t *testing.T) {
	c := newFakeController()
	c.current = "https://cdn.example/clip.mp4"
	c.set(func(s *media.Snapshot) {
		s.State = media.StatePlaying
		s.Duration = 12.5
		s.Position = 3.25
		s.Width, s.Height = 640, 360
		s.Seq = 4
	})
	rt, _ := mediaRuntime(t, mediaDoc, c)

	out := evalString(t, rt, `
		var v = document.getElementById('v');
		JSON.stringify([v.currentSrc, v.duration, v.currentTime, v.paused, v.ended,
		               v.seeking, v.readyState, v.videoWidth, v.videoHeight]);
	`)
	var got []any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	want := []any{"https://cdn.example/clip.mp4", 12.5, 3.25, false, false, false,
		float64(4), float64(640), float64(360)}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("property %d = %v, want %v (full: %v)", i, got[i], want[i], got)
		}
	}

	// A state change is visible on the next read, with no re-wrapping.
	c.set(func(s *media.Snapshot) { s.State = media.StatePaused })
	if got := evalString(t, rt, `document.getElementById('v').paused`); got != "true" {
		t.Fatalf("paused after the backend paused = %q, want true", got)
	}
	if got := evalString(t, rt, `document.getElementById('v').readyState`); got != "4" {
		t.Fatalf("readyState = %q, want 4 (a frame has been decoded)", got)
	}
}

// TestMediaErrorObject pins the MediaError shape, including that the code
// distinguishes a source the browser refused from one that would not decode.
func TestMediaErrorObject(t *testing.T) {
	c := newFakeController()
	rt, _ := mediaRuntime(t, mediaDoc, c)

	if got := evalString(t, rt, `document.getElementById('v').error`); got != "null" {
		t.Fatalf("error before a failure = %q, want null", got)
	}
	c.set(func(s *media.Snapshot) {
		s.State = media.StateFailed
		s.Err = media.ErrDecode
	})
	out := evalString(t, rt, `
		var e = document.getElementById('v').error;
		JSON.stringify([e.code, typeof e.message]);
	`)
	var got []any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if got[0] != float64(3) {
		t.Fatalf("error.code = %v, want 3 (MEDIA_ERR_DECODED)", got[0])
	}
}

// TestMediaSrcSetterRunsTheLoadAlgorithm pins that assigning src is not just an
// attribute write: the DOM says it selects the resource and starts loading it.
func TestMediaSrcSetterRunsTheLoadAlgorithm(t *testing.T) {
	c := newFakeController()
	rt, doc := mediaRuntime(t, mediaDoc, c)

	evalString(t, rt, `document.getElementById('v').src = 'other.mp4'`)
	if got := c.sources; len(got) != 1 || got[0] != "other.mp4" {
		t.Fatalf("SetSource saw %v, want [other.mp4]", got)
	}
	if c.loads != 1 {
		t.Fatalf("assigning src triggered %d loads, want 1", c.loads)
	}
	if got := videoOf(doc).GetAttribute("src"); got != "other.mp4" {
		t.Fatalf("the src attribute is %q, want other.mp4", got)
	}
	if got := evalString(t, rt, `document.getElementById('v').src`); got != "other.mp4" {
		t.Fatalf("reading src back gave %q", got)
	}

	// Clearing it stops rather than loading nothing.
	evalString(t, rt, `document.getElementById('v').src = ''`)
	if c.loads != 1 {
		t.Fatalf("clearing src triggered another load (%d total), want none", c.loads)
	}
}

// TestMediaControlsAndMutedReflection pins the two writable properties that are
// not playback state.
func TestMediaControlsAndMutedReflection(t *testing.T) {
	c := newFakeController()
	rt, doc := mediaRuntime(t, mediaDoc, c)

	evalString(t, rt, `document.getElementById('v').controls = false`)
	if videoOf(doc).HasAttribute("controls") {
		t.Fatal("controls = false left the attribute on the element")
	}
	if got := evalString(t, rt, `document.getElementById('v').controls`); got != "false" {
		t.Fatalf("controls reads back as %q", got)
	}

	evalString(t, rt, `document.getElementById('v').muted = true`)
	if got := c.mutes; len(got) != 1 || !got[0] {
		t.Fatalf("SetMuted saw %v, want [true]", got)
	}
	if got := evalString(t, rt, `document.getElementById('v').muted`); got != "true" {
		t.Fatalf("muted reads back as %q", got)
	}
}

// TestMediaCurrentTimeSetterSeeks pins that writing currentTime seeks, and that a
// value that is not a usable time is ignored rather than turning into a seek to
// zero — which is what Number(undefined) would silently produce.
func TestMediaCurrentTimeSetterSeeks(t *testing.T) {
	c := newFakeController()
	rt, _ := mediaRuntime(t, mediaDoc, c)

	evalString(t, rt, `document.getElementById('v').currentTime = 4.5`)
	evalString(t, rt, `document.getElementById('v').currentTime = 'not a number'`)
	evalString(t, rt, `document.getElementById('v').currentTime = -1`)
	evalString(t, rt, `document.getElementById('v').currentTime = Infinity`)

	if got := c.seeks; len(got) != 1 || got[0] != 4.5 {
		t.Fatalf("Seek saw %v, want exactly [4.5]", got)
	}
}

// TestMediaPauseAndLoadCallThrough pins the two methods that have no promise.
func TestMediaPauseAndLoadCallThrough(t *testing.T) {
	c := newFakeController()
	rt, _ := mediaRuntime(t, mediaDoc, c)

	if got := evalString(t, rt, `typeof document.getElementById('v').pause()`); got != "undefined" {
		t.Fatalf("pause() returned %q, want undefined", got)
	}
	if got := evalString(t, rt, `typeof document.getElementById('v').load()`); got != "undefined" {
		t.Fatalf("load() returned %q, want undefined", got)
	}
	if c.pauses != 1 || c.loads != 1 {
		t.Fatalf("pause/load reached the controller %d/%d times, want 1/1", c.pauses, c.loads)
	}
}

// TestMediaPlayReturnsASettledPromise is the contract the whole JS surface rests
// on: play() hands back a real Promise, and it settles when the backend says
// playback started — not when the request was accepted.
func TestMediaPlayReturnsASettledPromise(t *testing.T) {
	c := newFakeController()
	rt, doc := mediaRuntime(t, mediaDoc, c)

	out := evalString(t, rt, `
		var v = document.getElementById('v');
		var p = v.play();
		var state = 'pending';
		p.then(function(){ state = 'resolved'; }, function(e){ state = 'rejected:' + e.name; });
		JSON.stringify([typeof p, typeof p.then, typeof p.catch, state]);
	`)
	var got []any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	// Thenable, not `instanceof Promise`: this runtime's newPromiseObj builds a
	// plain object with then/catch/finally and no prototype, so instanceof fails
	// for every promise it makes — Promise.resolve and Promise.all included. That
	// is a gap in the shared promise implementation, not in the media binding, and
	// fixing it here would mean touching every promise in the runtime.
	if got[0] != "object" || got[1] != "function" || got[2] != "function" {
		t.Fatalf("play() returned %v, want a thenable object", got)
	}
	if got[3] != "pending" {
		t.Fatalf("the promise settled immediately (%v): play() must wait for playback", got[3])
	}

	// Nothing has played yet, so a tick must not settle it.
	rt.Tick()
	if got := evalString(t, rt, `state`); got != "pending" {
		t.Fatalf("the promise settled without a backend event: %q", got)
	}

	c.queue(media.Event{Kind: media.EvPlaying},
		media.Event{Kind: media.EvPlayResolved, Op: c.playOp})
	rt.DispatchMediaEvents(videoOf(doc), c.Events())
	rt.Tick()
	if got := evalString(t, rt, `state`); got != "resolved" {
		t.Fatalf("after EvPlayResolved the promise is %q, want resolved", got)
	}
}

// TestMediaPlayRejectionNamesTheReason pins that each refusal reaches the page as
// the DOMException name it can branch on. A page that catches NotAllowedError to
// show a "click to play" affordance cannot do that with a generic error.
func TestMediaPlayRejectionNamesTheReason(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"no user gesture", media.ErrNotAllowed, "NotAllowedError"},
		{"element went away", media.ErrClosed, "AbortError"},
		{"no backend", media.ErrUnsupported, "NotSupportedError"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newFakeController()
			c.playErr = tc.err
			rt, _ := mediaRuntime(t, mediaDoc, c)
			evalString(t, rt, `
				var state = 'pending';
				document.getElementById('v').play().then(
					function(){ state = 'resolved'; },
					function(e){ state = 'rejected:' + e.name; });
			`)
			rt.Tick()
			if got := evalString(t, rt, `state`); got != "rejected:"+tc.want {
				t.Fatalf("play() rejection = %q, want rejected:%s", got, tc.want)
			}
		})
	}
}

// TestMediaPlayRejectedByEventSettlesThePromise pins the other half: a request
// that was accepted and then abandoned (a pause during the download, a source
// replaced) rejects rather than leaving the page waiting forever.
func TestMediaPlayRejectedByEventSettlesThePromise(t *testing.T) {
	c := newFakeController()
	rt, doc := mediaRuntime(t, mediaDoc, c)
	evalString(t, rt, `
		var state = 'pending';
		document.getElementById('v').play().then(
			function(){ state = 'resolved'; },
			function(e){ state = 'rejected:' + e.name; });
	`)
	c.queue(media.Event{Kind: media.EvPlayRejected, Op: c.playOp, Code: media.CodeAborted})
	rt.DispatchMediaEvents(videoOf(doc), c.Events())
	rt.Tick()
	if got := evalString(t, rt, `state`); got != "rejected:AbortError" {
		t.Fatalf("an aborted play settled as %q, want rejected:AbortError", got)
	}
}

// TestMediaPlayWithoutBackendRejects pins that a build with no playback support
// says so through the promise instead of leaving it pending, which a page would
// read as "still loading".
func TestMediaPlayWithoutBackendRejects(t *testing.T) {
	rt, _ := mediaRuntime(t, mediaDoc, nil)
	evalString(t, rt, `
		var state = 'pending';
		document.getElementById('v').play().then(
			function(){ state = 'resolved'; },
			function(e){ state = 'rejected:' + e.name; });
	`)
	rt.Tick()
	if got := evalString(t, rt, `state`); got != "rejected:NotSupportedError" {
		t.Fatalf("play() with no backend settled as %q, want rejected:NotSupportedError", got)
	}
}

// TestMediaEventsReachListeners pins the dispatch path and the event names, which
// are the DOM's own rather than an internal spelling.
func TestMediaEventsReachListeners(t *testing.T) {
	c := newFakeController()
	rt, doc := mediaRuntime(t, mediaDoc, c)
	evalString(t, rt, `
		var seen = [];
		var v = document.getElementById('v');
		['loadstart','loadedmetadata','loadeddata','canplay','play','playing',
		 'pause','seeking','seeked','timeupdate','volumechange','ended',
		 'emptied','error'].forEach(function(n){
			v.addEventListener(n, function(e){ seen.push(n + ':' + e.type); });
		});
	`)

	c.queue(
		media.Event{Kind: media.EvLoadStart},
		media.Event{Kind: media.EvLoadedMetadata},
		media.Event{Kind: media.EvLoadedData},
		media.Event{Kind: media.EvCanPlay},
		media.Event{Kind: media.EvPlay},
		media.Event{Kind: media.EvPlaying},
		media.Event{Kind: media.EvPause},
		media.Event{Kind: media.EvSeeking},
		media.Event{Kind: media.EvSeeked},
		media.Event{Kind: media.EvTimeUpdate, Position: 1.5},
		media.Event{Kind: media.EvVolumeChange},
		media.Event{Kind: media.EvEnded},
		media.Event{Kind: media.EvEmptied},
		media.Event{Kind: media.EvError, Code: media.CodeNetwork},
	)
	rt.DispatchMediaEvents(videoOf(doc), c.Events())
	rt.Tick()

	out := evalString(t, rt, `JSON.stringify(seen)`)
	var seen []string
	if err := json.Unmarshal([]byte(out), &seen); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	want := []string{"loadstart", "loadedmetadata", "loadeddata", "canplay", "play",
		"playing", "pause", "seeking", "seeked", "timeupdate", "volumechange",
		"ended", "emptied", "error"}
	if len(seen) != len(want) {
		t.Fatalf("listeners saw %d events (%v), want %d", len(seen), seen, len(want))
	}
	for i, w := range want {
		if seen[i] != w+":"+w {
			t.Fatalf("event %d = %q, want %q", i, seen[i], w+":"+w)
		}
	}
}

// TestMediaEventsDoNotBubble pins that a media event is about one element. A
// bubbling "play" would reach a document-level listener for every video on the
// page, which is not what the DOM specifies and not what a page expects.
func TestMediaEventsDoNotBubble(t *testing.T) {
	c := newFakeController()
	rt, doc := mediaRuntime(t, `<html><body><div id="wrap"><video id="v"></video></div></body></html>`, c)
	evalString(t, rt, `
		var hits = [];
		document.getElementById('wrap').addEventListener('playing', function(){ hits.push('wrap'); });
		document.addEventListener('playing', function(){ hits.push('document'); });
		document.getElementById('v').addEventListener('playing', function(){ hits.push('video'); });
	`)
	c.queue(media.Event{Kind: media.EvPlaying})
	rt.DispatchMediaEvents(videoOf(doc), c.Events())
	rt.Tick()

	out := evalString(t, rt, `JSON.stringify(hits)`)
	if !strings.Contains(out, `"video"`) {
		t.Fatalf("the element's own listener did not fire: %s", out)
	}
	if strings.Contains(out, `"wrap"`) || strings.Contains(out, `"document"`) {
		t.Fatalf("a media event bubbled: %s", out)
	}
}

// TestMediaHostileListenerCannotHangThePump pins the bound on dispatched
// listeners. The media pump runs on the browser's UI thread, so a page that loops
// forever inside a timeupdate handler must cost one timeout rather than the
// window's ability to respond to anything.
func TestMediaHostileListenerCannotHangThePump(t *testing.T) {
	c := newFakeController()
	rt, doc := mediaRuntime(t, mediaDoc, c)
	evalString(t, rt, `
		var reached = 0;
		var v = document.getElementById('v');
		v.addEventListener('timeupdate', function(){ while (true) {} });
		v.addEventListener('pause', function(){ reached++; });
	`)

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.queue(media.Event{Kind: media.EvTimeUpdate}, media.Event{Kind: media.EvPause})
		rt.DispatchMediaEvents(videoOf(doc), c.Events())
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("a looping listener hung the media dispatch")
	}

	// The event after the hostile one still arrives: one bad listener costs its
	// own dispatch, not the rest of the element's events.
	rt.Tick()
	if got := evalString(t, rt, `reached`); got != "1" {
		t.Fatalf("the pause listener ran %s times, want 1", got)
	}
}
