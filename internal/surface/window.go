package surface

import (
	"time"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/frame"
)

type EventKind uint8

const (
	EvVsync EventKind = iota
	EvScroll
	EvPointer
	EvKey
	EvResize
	EvIME
	EvAXSetValue
)

func (k EventKind) String() string {
	switch k {
	case EvVsync:
		return "vsync"
	case EvScroll:
		return "scroll"
	case EvPointer:
		return "pointer"
	case EvKey:
		return "key"
	case EvResize:
		return "resize"
	case EvIME:
		return "ime"
	case EvAXSetValue:
		return "accessibility-value"
	}
	return "unknown"
}

type IMEAction uint8

const (
	IMEMarked IMEAction = iota
	IMECommit
)

type AXTarget uint8

const (
	AXTargetContent AXTarget = iota
	AXTargetAddressBar
)

type Button uint8

const (
	ButtonNone Button = iota
	ButtonLeft
	ButtonMiddle
	ButtonRight
)

type PointerAction uint8

const (
	PointerPress PointerAction = iota
	PointerRelease
	PointerMotion
)

type KeyAction uint8

const (
	KeyPress KeyAction = iota
	KeyRelease
)

type Cursor uint8

const (
	CursorDefault Cursor = iota
	CursorPointer
	CursorText
	CursorGrab
)

type KeyMod uint32

const (
	ModShift   KeyMod = 1 << 17
	ModControl KeyMod = 1 << 18
	ModOption  KeyMod = 1 << 19
	ModCommand KeyMod = 1 << 20
)

type Event struct {
	Kind      EventKind
	Delta     frame.Point
	Pos       frame.Point
	Button    Button
	Action    PointerAction
	Key       rune
	Mods      KeyMod
	KeyAction KeyAction
	Text      string
	IME       IMEAction
	AXTarget  AXTarget
	Size      frame.Size
	Scale     float32
	At        time.Time
}

type Window interface {
	Events() <-chan Event
	Present(buf *frame.Bitmap, damage []frame.Rect) error
	SetCursor(Cursor)
	SetIME(bool)
	ScaleFactor() float32
	Close() error
}

type AccessibilityWindow interface {
	Window
	SetAccessibility(nodes []ax.Node)
}
