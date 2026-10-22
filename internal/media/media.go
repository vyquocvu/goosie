// Package media holds the playback contracts shared by the engine, the
// JavaScript bindings and the platform backends.
//
// It is a dependency leaf: standard library only. A backend can therefore
// implement Player without importing the front end, and the front end can drive
// a backend without importing a platform. The engine owns the controllers and
// the downloaded files; this package only names what the two sides promise
// each other.
package media

import (
	"errors"
	"image"
	"math"
)

// State is where one media element is in its lifecycle. A backend reports which
// of these it is in; the controller derives the DOM events from the transitions,
// so no backend has to know what a page is allowed to listen for.
type State uint8

const (
	// StateEmpty is an element with no source selected and nothing loaded.
	StateEmpty State = iota
	// StateLoading is a source whose bytes have not arrived yet.
	StateLoading
	// StatePreparing is a local file handed to a backend that has not reported
	// metadata yet.
	StatePreparing
	// StateReady has metadata and is playable, but is not playing.
	StateReady
	// StatePlaying is advancing on its own clock.
	StatePlaying
	// StatePaused holds a frame and a position.
	StatePaused
	// StateSeeking has a move in flight; the position is not final yet.
	StateSeeking
	// StateEnded reached the end of the asset.
	StateEnded
	// StateFailed could not load or decode; Err says why.
	StateFailed
	// StateClosed released everything and accepts no further commands.
	StateClosed
)

func (s State) String() string {
	switch s {
	case StateEmpty:
		return "empty"
	case StateLoading:
		return "loading"
	case StatePreparing:
		return "preparing"
	case StateReady:
		return "ready"
	case StatePlaying:
		return "playing"
	case StatePaused:
		return "paused"
	case StateSeeking:
		return "seeking"
	case StateEnded:
		return "ended"
	case StateFailed:
		return "failed"
	case StateClosed:
		return "closed"
	}
	return "unknown"
}

// UnknownDuration reports whether d is the "metadata not known yet" duration.
// It is NaN rather than zero because that is what HTMLMediaElement.duration is
// at that point, and a zero duration would clamp every seek to the start.
func UnknownDuration(d float64) bool { return math.IsNaN(d) }

// Snapshot is what one Poll observed. It is a value, so a later poll cannot
// change what an earlier reader is holding.
type Snapshot struct {
	State State
	// Position is the playback head in seconds.
	Position float64
	// Duration is the asset length in seconds, or NaN before metadata is known.
	Duration float64
	// Width and Height are the decoded video dimensions in pixels, zero before
	// metadata is known.
	Width, Height int
	// Muted reports whether the audio track is silenced.
	Muted bool
	// Seq counts the frames the backend produced. It only ever increases, so a
	// consumer can tell a repeat of the last frame from a new one.
	Seq uint64
	// Pixels is the frame this poll delivered, or nil when the poll carries no
	// new frame. The pixels are immutable and belong to the caller once
	// returned: a backend must not reuse or overwrite the buffer, because it
	// goes straight into a frozen display list.
	Pixels image.Image
	// DoneOp is the operation that completed, or 0 when this poll reports no
	// completion. Like Pixels it is delivered once: the next poll repeats 0
	// until another operation finishes.
	DoneOp uint64
	// Err is why the asset failed, when State is StateFailed.
	Err error
}

// Player is one asset's playback handle. Every command is asynchronous: it
// records the intent and returns, and Poll reports when the intent took effect.
// Nothing here may block on decoding or on a native dispatch queue, because the
// caller is the browser's UI thread.
type Player interface {
	// Play starts playback and reports the outcome against op.
	Play(op uint64)
	// Pause stops playback. It carries no op: there is nothing to settle.
	Pause()
	// Seek moves the playback head to seconds and reports the outcome against
	// op, so a completion for a superseded seek can be recognised and dropped.
	Seek(op uint64, seconds float64)
	// SetMuted silences or restores the audio track.
	SetMuted(muted bool)
	// Poll returns the newest state without blocking.
	Poll() Snapshot
	// Close releases the asset. It is idempotent and safe from any goroutine.
	Close()
}

// Backend creates players for media the browser has already downloaded to a
// local file. A backend receives a path and never a URL: network access,
// cookies, redirects and private-network policy all belong to the browser's own
// HTTP client, and handing a URL to a native media stack would bypass every one
// of them.
type Backend interface {
	// Name identifies the backend in diagnostics.
	Name() string
	// Open begins preparing path. It returns as soon as the asset is accepted
	// for preparation; readiness arrives through Poll. A backend that cannot
	// play anything in this build or on this machine returns ErrUnsupported.
	Open(path string) (Player, error)
}

var (
	// ErrUnsupported reports a backend that cannot play media here. It is a
	// value rather than a formatted error because the JS binding maps it to
	// NotSupportedError by identity.
	ErrUnsupported = errors.New("media: no playback backend is available here")
	// ErrNotAllowed reports a play request with no trusted user gesture behind
	// it. The JS binding maps it to NotAllowedError.
	ErrNotAllowed = errors.New("media: playback requires a user gesture")
	// ErrClosed reports a command on a controller whose element went away.
	ErrClosed = errors.New("media: the element has no media any more")
	// ErrDecode reports an asset the backend accepted and could not decode,
	// which a page sees as MEDIA_ERR_DECODED rather than as a bad source.
	ErrDecode = errors.New("media: the asset could not be decoded")
)

// Limits bound what a backend will accept. They live here rather than inside a
// backend so the caps a host reasons about and the caps the decoder enforces
// are the same numbers, and so a test can read them instead of restating them.
type Limits struct {
	// MaxWidth and MaxHeight are per-axis decoded pixel limits.
	MaxWidth, MaxHeight int
	// MaxPixels is the decoded area, which catches a wide-but-short asset that
	// fits each axis and still allocates a frame nobody can display.
	MaxPixels int
	// MaxFrameRate is how many frames a second are worth sampling. Frames above
	// it are dropped rather than refused: a 60fps clip still plays at 30.
	MaxFrameRate float64
	// MaxDuration is the asset length in seconds.
	MaxDuration float64
}

// DefaultLimits are the caps this engine's first playback deliverable applies.
//
// They are deliberately well below what the hardware could take. A video frame
// is copied into a Go-owned buffer and held while a display list references it,
// so the cost of a decoded frame is paid in the browser's heap rather than in
// the decoder's, and 1080p at 30fps is where that stops being free. The
// duration cap follows from the download-first design: the whole file is in
// memory and on disk before the first frame, so a long video is a long wait
// with nothing on screen to show for it.
func DefaultLimits() Limits {
	return Limits{
		MaxWidth:     1920,
		MaxHeight:    1920,
		MaxPixels:    2073600,
		MaxFrameRate: 30,
		MaxDuration:  120,
	}
}

// EventKind names a transition the page can listen for. The names mirror the
// DOM ones so the JS binding is a lookup rather than a translation.
type EventKind uint8

const (
	EvLoadStart EventKind = iota
	EvLoadedMetadata
	EvLoadedData
	EvCanPlay
	EvPlay
	EvPlaying
	EvPause
	EvSeeking
	EvSeeked
	EvTimeUpdate
	EvVolumeChange
	EvEnded
	EvEmptied
	EvError
	// EvPlayResolved settles a play() promise successfully. It is not a DOM
	// event; it exists to carry the operation the promise was created for.
	EvPlayResolved
	// EvPlayRejected settles a play() promise with a failure.
	EvPlayRejected
)

func (k EventKind) String() string {
	switch k {
	case EvLoadStart:
		return "loadstart"
	case EvLoadedMetadata:
		return "loadedmetadata"
	case EvLoadedData:
		return "loadeddata"
	case EvCanPlay:
		return "canplay"
	case EvPlay:
		return "play"
	case EvPlaying:
		return "playing"
	case EvPause:
		return "pause"
	case EvSeeking:
		return "seeking"
	case EvSeeked:
		return "seeked"
	case EvTimeUpdate:
		return "timeupdate"
	case EvVolumeChange:
		return "volumechange"
	case EvEnded:
		return "ended"
	case EvEmptied:
		return "emptied"
	case EvError:
		return "error"
	case EvPlayResolved:
		return "playresolved"
	case EvPlayRejected:
		return "playrejected"
	}
	return "unknown"
}

// Coalescable reports whether losing this event costs a page nothing it could
// not recover from the next one. Only the two kinds that repeat on their own
// qualify; everything else is a transition that happens once.
func (k EventKind) Coalescable() bool {
	return k == EvTimeUpdate || k == EvSeeking
}

// ErrorCode classifies a failure for both the DOM error object and a play()
// rejection.
type ErrorCode uint8

const (
	// CodeUnsupported is an asset or backend that cannot play here.
	CodeUnsupported ErrorCode = iota
	// CodeNotAllowed is a play request with no user gesture behind it.
	CodeNotAllowed
	// CodeAborted is a request superseded by pause, load or a new source.
	CodeAborted
	// CodeDecode is an asset the backend accepted and could not decode.
	CodeDecode
	// CodeNetwork is a source that never arrived.
	CodeNetwork
)

// DOMErrorName is the DOMException name a play() rejection carries.
func (c ErrorCode) DOMErrorName() string {
	switch c {
	case CodeNotAllowed:
		return "NotAllowedError"
	case CodeAborted:
		return "AbortError"
	}
	return "NotSupportedError"
}

// MediaErrorCode is the HTMLMediaElement MediaError code.
func (c ErrorCode) MediaErrorCode() int {
	switch c {
	case CodeAborted:
		return 1
	case CodeNetwork:
		return 2
	case CodeDecode:
		return 3
	}
	return 4
}

// Event is one transition a controller observed.
type Event struct {
	Kind EventKind
	// Op identifies the play or seek request this event settles, when it
	// settles one. It is 0 otherwise.
	Op uint64
	// Code classifies a failure, for EvError and EvPlayRejected.
	Code ErrorCode
	// Position is the playback head, for EvTimeUpdate.
	Position float64
}

// Controller is one media element's playback handle as the rest of the browser
// sees it. The engine implements it; the JS binding and the painted controls
// both drive the same one, so a page script and a click on the play button take
// the same path and are subject to the same rules.
//
// A controller belongs to the session that created it. Its methods run on that
// session's owner goroutine; the download worker it spawns is the only other
// party, and it communicates through a channel rather than through this state.
type Controller interface {
	// SetSource selects url, resolved against the document. It replaces any
	// previous source and cancels its pending work, but does not fetch: the
	// browser ignores preload hints, so bytes move on Load or an accepted Play.
	SetSource(url string)
	// CurrentSrc is the absolute URL the element is actually loading or playing,
	// or "" while nothing is. It differs from the src attribute the way the DOM
	// says it should: the attribute is what the author wrote, this is what the
	// browser resolved it to.
	CurrentSrc() string
	// Load starts fetching the selected source. It is what HTMLMediaElement
	// .load() calls, and what an accepted Play calls when nothing is loaded.
	Load()
	// Play requests playback and returns the operation identifying the request.
	// The request settles through EvPlayResolved or EvPlayRejected, not through
	// the return value; the error is only for a refusal that never started.
	Play() (uint64, error)
	// Pause stops playback and abandons a pending play.
	Pause()
	// Seek moves the playback head to a finite number of seconds.
	Seek(seconds float64)
	// SetMuted silences or restores the audio track.
	SetMuted(muted bool)
	// Muted reports whether the audio track is silenced.
	Muted() bool
	// Snapshot is the newest observed state. It never blocks.
	Snapshot() Snapshot
	// Events drains the transitions queued since the last call. The queue is
	// bounded; see the controller for what it drops when a page never drains.
	Events() []Event
	// Close releases the player and the downloaded file. It is idempotent.
	Close()
}
