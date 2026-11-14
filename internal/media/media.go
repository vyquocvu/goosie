package media

import (
	"errors"
	"image"
	"math"
)

type State uint8

const (
	StateEmpty State = iota
	StateLoading
	StatePreparing
	StateReady
	StatePlaying
	StatePaused
	StateSeeking
	StateEnded
	StateFailed
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

func UnknownDuration(d float64) bool { return math.IsNaN(d) }

type Snapshot struct {
	State         State
	Position      float64
	Duration      float64
	Width, Height int
	Muted         bool
	Seq           uint64
	Pixels        image.Image
	DoneOp        uint64
	Err           error
}

type Player interface {
	Play(op uint64)
	Pause()
	Seek(op uint64, seconds float64)
	SetMuted(muted bool)
	Poll() Snapshot
	Close()
}

type Backend interface {
	Name() string
	Open(path string) (Player, error)
}

var (
	ErrUnsupported = errors.New("media: no playback backend is available here")
	ErrNotAllowed  = errors.New("media: playback requires a user gesture")
	ErrClosed      = errors.New("media: the element has no media any more")
	ErrDecode      = errors.New("media: the asset could not be decoded")
)

type Limits struct {
	MaxWidth, MaxHeight int
	MaxPixels           int
	MaxFrameRate        float64
	MaxDuration         float64
}

func DefaultLimits() Limits {
	return Limits{
		MaxWidth:     1920,
		MaxHeight:    1920,
		MaxPixels:    2073600,
		MaxFrameRate: 30,
		MaxDuration:  120,
	}
}

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
	EvPlayResolved
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

func (k EventKind) Coalescable() bool {
	return k == EvTimeUpdate || k == EvSeeking
}

type ErrorCode uint8

const (
	CodeUnsupported ErrorCode = iota
	CodeNotAllowed
	CodeAborted
	CodeDecode
	CodeNetwork
)

func (c ErrorCode) DOMErrorName() string {
	switch c {
	case CodeNotAllowed:
		return "NotAllowedError"
	case CodeAborted:
		return "AbortError"
	}
	return "NotSupportedError"
}

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

type Event struct {
	Kind     EventKind
	Op       uint64
	Code     ErrorCode
	Position float64
}

type Controller interface {
	SetSource(url string)
	CurrentSrc() string
	Load()
	Play() (uint64, error)
	Pause()
	Seek(seconds float64)
	SetMuted(muted bool)
	Muted() bool
	Snapshot() Snapshot
	Events() []Event
	Close()
}
