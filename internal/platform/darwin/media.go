//go:build darwin && cgo

package darwin

/*
#include "media.h"
#include <stdlib.h>
#include <CoreFoundation/CoreFoundation.h>
*/
import "C"

import (
	"fmt"
	"image"
	"math"
	"sync"
	"time"
	"unsafe"

	"github.com/vyquocvu/goosie/internal/media"
)

// mediaBackend is AVFoundation seen through the media.Backend contract. It holds
// no state of its own: every player owns its native handle, its queue and its
// frame slot, so a window that is closed cannot leave a backend behind that
// outlives the players it made.
type mediaBackend struct{}

// Name is what diagnostics report, and it names the framework rather than the
// platform: "darwin" would say where the process is running, not what decoded
// the video.
func (mediaBackend) Name() string { return "avfoundation" }

func (mediaBackend) Open(path string) (media.Player, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: no path was given", media.ErrUnsupported)
	}
	lim := media.DefaultLimits()
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var cerr C.int
	cp := C.GoosieMediaPlayerCreate(cpath,
		C.int(lim.MaxWidth), C.int(lim.MaxHeight), C.int(lim.MaxPixels),
		C.double(lim.MaxFrameRate), C.double(lim.MaxDuration), &cerr)
	if cp == nil {
		if cerr == C.GOOSIE_MEDIA_CREATE_BAD_ARG {
			return nil, fmt.Errorf("%w: the path could not be read", media.ErrUnsupported)
		}
		return nil, fmt.Errorf("%w: AVFoundation refused the asset", media.ErrUnsupported)
	}
	return &mediaPlayer{p: cp}, nil
}

// mediaPlayer is one asset's playback handle.
//
// mu is not about the native side, which serializes itself on its own queue: it
// is about the Go side, where Poll and Close can arrive from the session owner
// and from a teardown on another goroutine. Closed is what stops a poll from
// using a handle Destroy already freed.
type mediaPlayer struct {
	mu     sync.Mutex
	p      *C.GoosieMediaPlayer
	closed bool
}

func (pl *mediaPlayer) Play(op uint64) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.closed {
		return
	}
	C.GoosieMediaPlayerPlay(pl.p, C.ulonglong(op))
}

func (pl *mediaPlayer) Pause() {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.closed {
		return
	}
	C.GoosieMediaPlayerPause(pl.p)
}

func (pl *mediaPlayer) Seek(op uint64, seconds float64) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.closed || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return
	}
	C.GoosieMediaPlayerSeek(pl.p, C.ulonglong(op), C.double(seconds))
}

func (pl *mediaPlayer) SetMuted(muted bool) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.closed {
		return
	}
	var m C.int
	if muted {
		m = 1
	}
	C.GoosieMediaPlayerSetMuted(pl.p, m)
}

// Poll reads the newest observation and takes the frame if one is waiting. It
// never blocks on the decoder: the native side queues a sample request and
// reports whatever it already has.
func (pl *mediaPlayer) Poll() media.Snapshot {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.closed {
		return media.Snapshot{State: media.StateClosed, Duration: math.NaN()}
	}

	var st C.GoosieMediaState
	C.GoosieMediaPlayerPoll(pl.p, &st)

	snap := media.Snapshot{
		State:    mediaStateFromC(int(st.state)),
		Position: float64(st.position),
		// The native side reports an unknown duration as negative rather than
		// as a NaN in a C double, which is the one thing a Go reader should not
		// have to reinterpret.
		Duration: math.NaN(),
		Width:    int(st.width),
		Height:   int(st.height),
		Muted:    st.muted != 0,
		Seq:      uint64(st.frameSeq),
		DoneOp:   uint64(st.doneOp),
	}
	if st.duration >= 0 {
		snap.Duration = float64(st.duration)
	}
	if snap.State == media.StateFailed {
		snap.Err = mediaErrorFromC(int(st.errorCode))
	}
	if st.hasFrame != 0 {
		if img := pl.takeFrame(int(st.frameW), int(st.frameH), int(st.frameBytes)); img != nil {
			snap.Pixels = img
		}
	}
	return snap
}

// takeFrame copies the native slot into a freshly allocated image. A new
// allocation per frame rather than a reused buffer is the point: the image goes
// into a frozen display list that raster workers read without a lock, and a
// buffer the next sample overwrites would change pixels a frame is still
// painting.
func (pl *mediaPlayer) takeFrame(w, h, n int) image.Image {
	if w <= 0 || h <= 0 || n <= 0 || n != w*h*4 {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	got := C.GoosieMediaPlayerFrame(pl.p, (*C.uchar)(unsafe.Pointer(&img.Pix[0])), C.int(len(img.Pix)))
	if int(got) != n {
		return nil
	}
	return img
}

func (pl *mediaPlayer) Close() {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.closed {
		return
	}
	pl.closed = true
	C.GoosieMediaPlayerDestroy(pl.p)
	pl.p = nil
}

// PumpMainRunLoopForTest services the calling thread's CFRunLoop for up to d. It
// exists because AVPlayer only prepares its item on the process main thread: the
// browser gets that for free from GoosieAppRun, and a test binary has to supply
// it itself. It is exported so the package's own TestMain can call it while the
// main goroutine holds the main OS thread.
func PumpMainRunLoopForTest(d time.Duration) {
	C.CFRunLoopRunInMode(C.kCFRunLoopDefaultMode, C.double(d.Seconds()), 0)
}

func mediaStateFromC(s int) media.State {
	switch s {
	case C.GOOSIE_MEDIA_READY:
		return media.StateReady
	case C.GOOSIE_MEDIA_PLAYING:
		return media.StatePlaying
	case C.GOOSIE_MEDIA_PAUSED:
		return media.StatePaused
	case C.GOOSIE_MEDIA_ENDED:
		return media.StateEnded
	case C.GOOSIE_MEDIA_FAILED:
		return media.StateFailed
	}
	return media.StatePreparing
}

func mediaErrorFromC(code int) error {
	if code == C.GOOSIE_MEDIA_ERR_DECODE {
		return media.ErrDecode
	}
	return fmt.Errorf("%w: AVFoundation will not play this asset", media.ErrUnsupported)
}

// NewMediaBackend returns the AVFoundation playback backend.
//
// The backend needs no window: AVPlayer owns the audio and the clock, and the
// pixels are handed over rather than drawn, so there is no view to attach one to.
// A browser still asks the window for it — see platform.MediaBackend — because the
// question a run needs answered is "can the surface I opened play video", not
// "does this machine have AVFoundation".
func NewMediaBackend() media.Backend { return mediaBackend{} }

// MediaBackend returns this window's playback backend. It is a method on the
// window rather than a package function so that the answer comes from the
// surface that was actually opened: a headless run gets no backend from
// platform.MediaBackend, and a test that checks "can this play video" is asking
// about the window on screen rather than about the machine it happens to be on.
func (w *Window) MediaBackend() media.Backend { return NewMediaBackend() }
