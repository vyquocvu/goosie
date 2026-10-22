package js

import (
	"errors"
	"math"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/media"
)

// MediaProvider returns the playback controller for a node, or nil when the
// element has no media: the document was opened without a backend, the node is
// not a video, or the document has already reached its element cap. A nil answer
// is why the accessors below fall back to the inert values the DOM specifies
// rather than throwing — a page that asks a video for its duration on a build
// that cannot play video should get NaN, not an exception.
type MediaProvider func(*dom.Node) media.Controller

// SetMediaProvider installs the bridge between wrapped elements and the engine's
// media controllers. It has to happen before any page script runs, because a
// script that reads video.readyState during parsing must see the same element a
// later script sees.
func (r *Runtime) SetMediaProvider(p MediaProvider) {
	r.mu.Lock()
	r.mediaProvider = p
	r.mediaCtrls = nil
	r.mu.Unlock()
}

// mediaController resolves and caches the controller for a node. Caching matters
// because the engine creates a controller the first time one is asked for and
// caps how many a document may have: asking again per property read would spend
// the cap on an element a page merely inspected.
func (r *Runtime) mediaController(node *dom.Node) media.Controller {
	if node == nil {
		return nil
	}
	r.mu.Lock()
	provider := r.mediaProvider
	if r.mediaCtrls == nil {
		r.mediaCtrls = make(map[*dom.Node]media.Controller)
	}
	c, ok := r.mediaCtrls[node]
	r.mu.Unlock()
	if ok {
		return c
	}
	if provider == nil {
		return nil
	}
	c = provider(node)
	if c == nil {
		return nil
	}
	r.mu.Lock()
	r.mediaCtrls[node] = c
	r.mu.Unlock()
	return c
}

// installMediaBridge puts the three functions the per-element accessors call on
// the document object, which is where every other reflected-property bridge in
// this runtime lives. They are on document rather than captured in a closure
// because an accessor's getter runs long after the element was wrapped, and a
// global set for the duration of one RunString is gone by then.
func (r *Runtime) installMediaBridge(doc *goja.Object) {
	_ = doc.Set("__mediaGet__", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		return r.mediaGet(int(call.Arguments[0].ToInteger()), call.Arguments[1].String())
	})
	_ = doc.Set("__mediaSet__", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			return goja.Undefined()
		}
		var v goja.Value
		if call.Arguments[2] != nil {
			v = call.Arguments[2]
		}
		r.mediaSet(int(call.Arguments[0].ToInteger()), call.Arguments[1].String(), v)
		return goja.Undefined()
	})
	_ = doc.Set("__mediaCall__", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		return r.mediaCall(int(call.Arguments[0].ToInteger()), call.Arguments[1].String())
	})
}

// mediaAccessorJS defines the element's media properties. Read-only ones are the
// playback state, which the page must not be able to write; src, currentTime,
// muted and controls are the reflected ones. play, pause and load are methods.
const mediaAccessorJS = `
(function() {
	var t = __mediaTarget__;
	var ro = ['currentSrc','duration','paused','ended','seeking','readyState','error','videoWidth','videoHeight'];
	for (var i = 0; i < ro.length; i++) {
		(function(name) {
			Object.defineProperty(t, name, {
				get: function() {
					var nid = this.__nid__;
					if (nid === undefined) return undefined;
					return document.__mediaGet__(nid, name);
				},
				configurable: true,
				enumerable: true
			});
		})(ro[i]);
	}
	var rw = ['src','currentTime','muted','controls'];
	for (var j = 0; j < rw.length; j++) {
		(function(name) {
			Object.defineProperty(t, name, {
				get: function() {
					var nid = this.__nid__;
					if (nid === undefined) return undefined;
					return document.__mediaGet__(nid, name);
				},
				set: function(v) {
					var nid = this.__nid__;
					if (nid === undefined) return;
					document.__mediaSet__(nid, name, v);
				},
				configurable: true,
				enumerable: true
			});
		})(rw[j]);
	}
	var methods = ['play','pause','load'];
	for (var k = 0; k < methods.length; k++) {
		(function(name) {
			t[name] = function() {
				var nid = this.__nid__;
				if (nid === undefined) return undefined;
				return document.__mediaCall__(nid, name);
			};
		})(methods[k]);
	}
})();
`

// defineMediaAccessors installs the media surface on one wrapped video element.
func (r *Runtime) defineMediaAccessors(obj *goja.Object) {
	_ = r.vm.Set("__mediaTarget__", obj)
	_, _ = r.vm.RunString(mediaAccessorJS)
	_ = r.vm.Set("__mediaTarget__", nil)
}

func (r *Runtime) mediaGet(nid int, name string) goja.Value {
	node := r.nodeRegistry[nid]
	if node == nil {
		return goja.Undefined()
	}
	// Two properties are plain attribute reflections and need no controller, so
	// they work on a build with no backend at all.
	switch name {
	case "src":
		return r.vm.ToValue(node.GetAttribute("src"))
	case "controls":
		return r.vm.ToValue(node.HasAttribute("controls"))
	}
	c := r.mediaController(node)
	if c == nil {
		return r.mediaInertGet(name)
	}
	snap := c.Snapshot()
	switch name {
	case "currentSrc":
		return r.vm.ToValue(c.CurrentSrc())
	case "duration":
		return r.vm.ToValue(snap.Duration)
	case "currentTime":
		return r.vm.ToValue(snap.Position)
	case "paused":
		return r.vm.ToValue(snap.State != media.StatePlaying)
	case "ended":
		return r.vm.ToValue(snap.State == media.StateEnded)
	case "seeking":
		return r.vm.ToValue(snap.State == media.StateSeeking)
	case "muted":
		return r.vm.ToValue(snap.Muted)
	case "readyState":
		return r.vm.ToValue(mediaReadyState(snap))
	case "videoWidth":
		return r.vm.ToValue(snap.Width)
	case "videoHeight":
		return r.vm.ToValue(snap.Height)
	case "error":
		if snap.State != media.StateFailed || snap.Err == nil {
			return goja.Null()
		}
		return r.mediaErrorObject(snap.Err)
	}
	return goja.Undefined()
}

// mediaInertGet is what a media property reads as when the element has no
// controller. These are the DOM's own "nothing is playing" values: a duration of
// NaN rather than 0, because 0 would make every seek clamp to the start.
func (r *Runtime) mediaInertGet(name string) goja.Value {
	switch name {
	case "currentSrc":
		return r.vm.ToValue("")
	case "duration", "currentTime":
		return r.vm.ToValue(math.NaN())
	case "error":
		return goja.Null()
	case "paused":
		return r.vm.ToValue(true)
	case "videoWidth", "videoHeight", "readyState":
		return r.vm.ToValue(0)
	}
	return r.vm.ToValue(false)
}

func (r *Runtime) mediaSet(nid int, name string, v goja.Value) {
	node := r.nodeRegistry[nid]
	if node == nil {
		return
	}
	switch name {
	case "controls":
		if v != nil && v.ToBoolean() {
			node.SetAttribute("controls", "")
		} else {
			node.RemoveAttribute("controls")
		}
		r.notifyMutation()
		return
	case "src":
		url := ""
		if v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			url = v.String()
		}
		node.SetAttribute("src", url)
		r.notifyMutation()
		// Assigning src runs the load algorithm, so the fetch starts here rather
		// than waiting for play(). Clearing it does the opposite: it stops
		// whatever was loading.
		if c := r.mediaController(node); c != nil {
			c.SetSource(url)
			if url != "" {
				c.Load()
			}
		}
		return
	}
	c := r.mediaController(node)
	if c == nil || v == nil {
		return
	}
	switch name {
	case "currentTime":
		// The controller rejects NaN, infinities and negatives itself; reading the
		// number here is only so a non-numeric assignment does not become 0 and
		// silently seek to the start.
		if f := v.ToFloat(); !math.IsNaN(f) && !math.IsInf(f, 0) && f >= 0 {
			c.Seek(f)
		}
	case "muted":
		c.SetMuted(v.ToBoolean())
	}
}

func (r *Runtime) mediaCall(nid int, name string) goja.Value {
	node := r.nodeRegistry[nid]
	if node == nil {
		return goja.Undefined()
	}
	c := r.mediaController(node)
	switch name {
	case "pause":
		if c != nil {
			c.Pause()
		}
	case "load":
		if c != nil {
			c.Load()
		}
	case "play":
		return r.mediaPlay(c)
	}
	return goja.Undefined()
}

// mediaPlay is the whole of the play() contract that is visible from here: a real
// Promise, rejected immediately when the request never started and settled later
// by the event the backend produces when playback actually begins.
//
// The distinction is the reason this is not a resolved promise handed back on
// success. A page that awaits play() is asking to be told when it can rely on
// frames and audio, and "the request was accepted" is not that answer — the
// download may still be running, or the asset may turn out to be undecodable.
func (r *Runtime) mediaPlay(c media.Controller) goja.Value {
	p := r.newPromiseObj()
	if c == nil {
		r.rejectPromise(p, namedError(r, "NotSupportedError", "This element has no media backend"))
		return p.obj
	}
	op, err := c.Play()
	if err != nil {
		r.rejectPromise(p, namedError(r, playErrorName(err), err.Error()))
		return p.obj
	}
	r.mu.Lock()
	if r.pendingPlays == nil {
		r.pendingPlays = make(map[uint64]*promiseObj)
	}
	r.pendingPlays[op] = p
	r.mu.Unlock()
	return p.obj
}

// settleMediaPlay finishes one pending play() promise. An op that is not pending
// is not an error: the promise may already have been settled by an earlier event
// for the same request.
func (r *Runtime) settleMediaPlay(op uint64, rejected bool, code media.ErrorCode) {
	r.mu.Lock()
	p, ok := r.pendingPlays[op]
	if ok {
		delete(r.pendingPlays, op)
	}
	r.mu.Unlock()
	if !ok {
		return
	}
	if !rejected {
		r.resolvePromise(p, goja.Undefined())
		return
	}
	name := code.DOMErrorName()
	r.rejectPromise(p, namedError(r, name, "play() failed: "+name))
}

// DispatchMediaEvents hands one element's transitions to its listeners. The engine
// calls it from the media pump, on the runtime's own goroutine and outside the
// pipeline lock.
//
// Each dispatch runs bounded. A listener is page code, and the media pump is on
// the browser's UI thread, so an unbounded listener would be a page that can hang
// the window by listening for timeupdate — which is why this does not go through
// the same unwrapped path a synthetic dispatchEvent takes.
func (r *Runtime) DispatchMediaEvents(node *dom.Node, events []media.Event) {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed || node == nil {
		return
	}
	for _, e := range events {
		switch e.Kind {
		case media.EvPlayResolved:
			r.settleMediaPlay(e.Op, false, 0)
			continue
		case media.EvPlayRejected:
			r.settleMediaPlay(e.Op, true, e.Code)
			continue
		}
		name := e.Kind.String()
		if name == "" || name == "unknown" {
			continue
		}
		// Media events do not bubble: they are about one element's own state, and
		// a document-level listener for "play" would otherwise fire for every
		// video on the page.
		ev := dom.NewEvent(name, false, false)
		_, _ = r.callBounded(func() (goja.Value, error) {
			dom.DispatchEvent(node, ev)
			return goja.Undefined(), nil
		})
	}
}

// mediaReadyState maps a playback state onto HTMLMediaElement's HAVE_* ladder.
// The top rung means "there is a decoded frame at the current position", which is
// the honest reading here: this engine has no buffer model, so it cannot claim to
// know whether playback would stall, and it should not invent a number that says
// it does.
func mediaReadyState(s media.Snapshot) int {
	switch s.State {
	case media.StateEmpty, media.StateLoading, media.StateFailed, media.StateClosed:
		return 0
	case media.StatePreparing:
		if media.UnknownDuration(s.Duration) {
			return 0
		}
		return 1
	}
	if s.Seq > 0 {
		return 4
	}
	if !media.UnknownDuration(s.Duration) {
		return 1
	}
	return 0
}

func (r *Runtime) mediaErrorObject(err error) goja.Value {
	// MEDIA_ERR_NETWORK by default: a source that did not arrive is the common
	// case, and the two specific codes are the ones a page can act on differently.
	code := 2
	switch {
	case errors.Is(err, media.ErrDecode):
		code = 3
	case errors.Is(err, media.ErrUnsupported):
		code = 4
	}
	e := r.vm.NewObject()
	_ = e.Set("code", code)
	_ = e.Set("message", err.Error())
	return e
}

func playErrorName(err error) string {
	switch {
	case errors.Is(err, media.ErrNotAllowed):
		return "NotAllowedError"
	case errors.Is(err, media.ErrClosed):
		return "AbortError"
	}
	return "NotSupportedError"
}
