package message

import (
	"encoding/json"
	"fmt"
	"time"
)

// Version is the current message schema version. Bump on backward-incompatible
// wire format changes.
const Version = 1

// Message is a versioned, self-describing container for all engine messages.
// Exactly one payload field should be set per message; the rest are nil.
// JSON omitempty ensures the wire format contains only the active payload.
type Message struct {
	Version int       `json:"v"`
	Time    time.Time `json:"ts,omitempty"`

	Navigation *Navigation       `json:"nav,omitempty"`
	Title      *Title            `json:"title,omitempty"`
	URL        *URL              `json:"url,omitempty"`
	Paint      *FirstPaint       `json:"paint,omitempty"`
	Progress   *Progress         `json:"progress,omitempty"`
	Error      *Error            `json:"error,omitempty"`
	Security   *SecuritySummary  `json:"security,omitempty"`
	Download   *Download         `json:"download,omitempty"`
	Log        *Log              `json:"log,omitempty"`
	Input      *Input            `json:"input,omitempty"`
	Viewport   *Viewport         `json:"viewport,omitempty"`
	Resource   *ResourceResponse `json:"resource,omitempty"`
	Frame      *Frame            `json:"frame,omitempty"`
	Crash      *Crash            `json:"crash,omitempty"`
}

// Encode serializes m to indented JSON.
func (m *Message) Encode() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// Decode deserializes data into a Message. Returns an error if the version
// is unrecognised or the JSON is malformed.
func Decode(data []byte) (*Message, error) {
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("message: decode: %w", err)
	}
	if m.Version != Version {
		return nil, fmt.Errorf("message: unsupported version %d", m.Version)
	}
	return &m, nil
}

// ---------------------------------------------------------------------------
// Navigation lifecycle messages
// ---------------------------------------------------------------------------

// NavState mirrors the session lifecycle states.
type NavState int

const (
	StateCreated     NavState = 0
	StateNavigating  NavState = 1
	StateParsing     NavState = 2
	StateInteractive NavState = 3
	StateComplete    NavState = 4
	StateCancelled   NavState = 5
	StateFailed      NavState = 6
	StateClosed      NavState = 7
)

// Navigation is emitted whenever the session transitions between lifecycle
// states (e.g. Navigating -> Parsing -> Interactive -> Complete).
type Navigation struct {
	NavID uint64   `json:"navID"`
	URL   string   `json:"url,omitempty"`
	State NavState `json:"state"`
}

// Title is emitted when the page title is discovered or changes.
type Title struct {
	NavID uint64 `json:"navID"`
	Title string `json:"title"`
}

// URL is emitted when the active URL changes.
type URL struct {
	NavID uint64 `json:"navID"`
	URL   string `json:"url"`
}

// FirstPaint is emitted when the first contentful paint occurs.
type FirstPaint struct {
	NavID uint64 `json:"navID"`
}

// Progress is emitted to report load progress.
type Progress struct {
	NavID    uint64  `json:"navID"`
	Progress float64 `json:"progress"`
}

// Error is emitted when a navigation error occurs.
type Error struct {
	NavID   uint64 `json:"navID"`
	Message string `json:"message"`
}

// SecuritySummary carries TLS/security status for the active navigation.
type SecuritySummary struct {
	URL    string `json:"url,omitempty"`
	Scheme string `json:"scheme,omitempty"`
	Secure bool   `json:"secure"`
	Error  string `json:"error,omitempty"`
}

// Download is emitted when a download starts or completes.
type Download struct {
	URL          string `json:"url"`
	Status       string `json:"status"` // "running", "complete", "failed"
	BytesWritten int64  `json:"bytesWritten"`
	Error        string `json:"error,omitempty"`
}

// Log carries a console log entry from the JS runtime.
type Log struct {
	Level   string `json:"level"` // "log", "info", "warn", "error"
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// Input, viewport, resource, frame, and crash messages
// ---------------------------------------------------------------------------

// InputKind identifies the type of input event.
type InputKind int

const (
	InputMouseDown InputKind = iota
	InputMouseUp
	InputMouseMove
	InputClick
	InputKeyDown
	InputKeyUp
	InputKeyPress
	InputScroll
)

// Input carries a user-input event at the IPC boundary.
type Input struct {
	Kind   InputKind `json:"kind"`
	X      float64   `json:"x,omitempty"`
	Y      float64   `json:"y,omitempty"`
	Key    string    `json:"key,omitempty"`
	Mod    string    `json:"mod,omitempty"`
	Scroll float64   `json:"scroll,omitempty"`
}

// Viewport describes the current visible area for a navigation.
type Viewport struct {
	NavID   uint64  `json:"navID"`
	Width   float64 `json:"width"`
	Height  float64 `json:"height"`
	ScrollX float64 `json:"scrollX,omitempty"`
	ScrollY float64 `json:"scrollY,omitempty"`
	Scale   float64 `json:"scale,omitempty"`
}

// ResourceResponse carries HTTP response metadata for a sub-resource load.
type ResourceResponse struct {
	NavID         uint64 `json:"navID"`
	URL           string `json:"url"`
	Status        int    `json:"status"`
	ContentType   string `json:"contentType,omitempty"`
	ContentLength int64  `json:"contentLength"`
	CacheHit      bool   `json:"cacheHit,omitempty"`
	Error         string `json:"error,omitempty"`
}

// FrameKind identifies the type of frame event.
type FrameKind int

const (
	FrameBegin  FrameKind = iota // start of a new render frame
	FrameCommit                  // frame fully composed and ready
	FrameDrop                    // frame was skipped/dropped
)

// Frame reports render-frame lifecycle events.
type Frame struct {
	NavID    uint64    `json:"navID"`
	Kind     FrameKind `json:"kind"`
	Duration int64     `json:"durationNs,omitempty"` // frame wall time in ns
}

// CrashInfoKind identifies the subsystem that crashed.
type CrashInfoKind int

const (
	CrashEngine   CrashInfoKind = iota // engine crash (panic/recover)
	CrashRenderer                      // renderer crash
	CrashJS                            // JavaScript runtime crash
)

// Crash reports an engine subsystem failure.
type Crash struct {
	Kind    CrashInfoKind `json:"kind"`
	Message string        `json:"message"`
	Stack   string        `json:"stack,omitempty"`
}
