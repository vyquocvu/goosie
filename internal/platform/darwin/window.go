//go:build darwin && cgo

package darwin

/*
#cgo LDFLAGS: -framework AppKit -framework Foundation -framework CoreGraphics -framework CoreVideo -framework QuartzCore -framework AVFoundation -framework CoreMedia -framework UniformTypeIdentifiers
#cgo CFLAGS: -I${SRCDIR} -Wno-deprecated-declarations
#include "shim.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
)

const Name = "darwin"

type Config struct {
	Title string
	Size  frame.Size
	Scale float32
}

type Window struct {
	gw       *C.GoosieWindow
	events   chan surface.Event
	detach   chan struct{}
	pumpDone chan struct{}
	closing  sync.Once

	dmg [][4]C.int
}

func Available() bool { return C.GoosieHasDisplay() != 0 }

func Open(cfg Config) (*Window, error) {
	if cfg.Size.W <= 0 || cfg.Size.H <= 0 {
		return nil, fmt.Errorf("platform/darwin: a window needs a positive size, got %dx%d",
			cfg.Size.W, cfg.Size.H)
	}
	title := C.CString(cfg.Title)
	defer C.free(unsafe.Pointer(title))

	var cerr C.int
	gw := C.GoosieWindowCreateOnMain(title, C.int(cfg.Size.W), C.int(cfg.Size.H), &cerr)
	if gw == nil {
		return nil, createErr(int(cerr), cfg)
	}
	w := &Window{
		gw:       gw,
		events:   make(chan surface.Event),
		detach:   make(chan struct{}),
		pumpDone: make(chan struct{}),
	}
	go w.pump()
	return w, nil
}

func createErr(code int, cfg Config) error {
	switch code {
	case 1:
		return ErrNotMainThread
	case 2:
		return fmt.Errorf("platform/darwin: a window needs a positive size, got %dx%d",
			cfg.Size.W, cfg.Size.H)
	case 3:
		return fmt.Errorf("platform/darwin: out of memory allocating a %dx%d window",
			cfg.Size.W, cfg.Size.H)
	case 4:
		return ErrNoDisplay
	case 5:
		return ErrNoDisplayLink
	case 6:
		return ErrAppQuitting
	}
	return fmt.Errorf("platform/darwin: window create failed with code %d", code)
}

func (w *Window) Events() <-chan surface.Event { return w.events }

func (w *Window) pump() {
	defer close(w.events)
	defer close(w.pumpDone)

	var ce C.GoosieEvent
	for {
		select {
		case <-w.detach:
			return
		default:
		}
		if C.GoosieNextEvent(w.gw, &ce) == 0 {
			return
		}
		if ev, ok := translate(ce); ok {
			select {
			case w.events <- ev:
			case <-w.detach:
				return
			}
		}
	}
}

func translate(ce C.GoosieEvent) (surface.Event, bool) {
	ev := surface.Event{
		Scale: float32(ce.scale),
		At:    time.Now(),
	}
	switch ce.kind {
	case C.GOOSIE_EV_VSYNC:
		ev.Kind = surface.EvVsync
	case C.GOOSIE_EV_SCROLL:
		ev.Kind = surface.EvScroll
		ev.Delta = frame.Point{X: -int32(ce.dx), Y: -int32(ce.dy)}
		ev.Pos = frame.Point{X: int32(ce.px), Y: int32(ce.py)}
	case C.GOOSIE_EV_POINTER:
		ev.Kind = surface.EvPointer
		ev.Pos = frame.Point{X: int32(ce.px), Y: int32(ce.py)}
		switch ce.action {
		case C.GOOSIE_POINTER_PRESS:
			ev.Action = surface.PointerPress
			ev.Button = button(ce.button)
		case C.GOOSIE_POINTER_RELEASE:
			ev.Action = surface.PointerRelease
		case C.GOOSIE_POINTER_MOTION:
			ev.Action = surface.PointerMotion
		default:
			ev.Action = surface.PointerMotion
		}
	case C.GOOSIE_EV_KEY:
		ev.Kind = surface.EvKey
		ev.Key = rune(ce.key)
		ev.Mods = surface.KeyMod(ce.mods)
		if ce.action == C.GOOSIE_KEY_RELEASE {
			ev.KeyAction = surface.KeyRelease
		}
	case C.GOOSIE_EV_RESIZE:
		ev.Kind = surface.EvResize
		ev.Size = frame.Size{W: int32(ce.w), H: int32(ce.h)}
	case C.GOOSIE_EV_IME:
		ev.Kind = surface.EvIME
		ev.Text = C.GoString(&ce.text[0])
		if ce.ime == C.GOOSIE_IME_COMMIT {
			ev.IME = surface.IMECommit
		} else {
			ev.IME = surface.IMEMarked
		}
	case C.GOOSIE_EV_AX_SET_VALUE:
		ev.Kind = surface.EvAXSetValue
		ev.Pos = frame.Point{X: int32(ce.px), Y: int32(ce.py)}
		if ce.ax_target == C.GOOSIE_AX_TARGET_ADDRESS {
			ev.AXTarget = surface.AXTargetAddressBar
		}
		ev.Text = C.GoString(&ce.text[0])
	default:
		return ev, false
	}
	return ev, true
}

func button(n C.int) surface.Button {
	switch n {
	case 1:
		return surface.ButtonLeft
	case 2:
		return surface.ButtonMiddle
	case 3:
		return surface.ButtonRight
	}
	return surface.ButtonNone
}

func (w *Window) Present(buf *frame.Bitmap, damage []frame.Rect) error {
	if buf == nil || buf.Empty() {
		return ErrNoPixels
	}
	if buf.Stride < buf.W*4 {
		return fmt.Errorf("platform/darwin: buffer stride %d is narrower than a %d px row", buf.Stride, buf.W)
	}
	if n := int64(buf.Stride) * int64(buf.H); int64(len(buf.RGBA)) < n {
		return fmt.Errorf("platform/darwin: buffer has %d bytes but stride*height requires %d", len(buf.RGBA), n)
	}
	if len(w.dmg) < len(damage) {
		w.dmg = make([][4]C.int, len(damage))
	}
	list := w.dmg[:0]
	for _, r := range damage {
		if r.Empty() {
			continue
		}
		list = append(list, [4]C.int{C.int(r.X0), C.int(r.Y0), C.int(r.X1), C.int(r.Y1)})
	}

	var rects *C.int
	if len(list) > 0 {
		rects = &list[0][0]
	}
	rc := C.GoosiePresent(w.gw,
		(*C.uchar)(unsafe.Pointer(&buf.RGBA[0])),
		C.int(buf.W), C.int(buf.H), C.int(buf.Stride),
		rects, C.int(len(list)))
	switch rc {
	case C.GOOSIE_OK, C.GOOSIE_DROPPED:
		return nil
	case C.GOOSIE_CLOSED:
		return ErrClosed
	}
	return fmt.Errorf("%w: present returned %d", ErrClosed, int(rc))
}

func (w *Window) SetCursor(c surface.Cursor) {
	var shape C.int
	switch c {
	case surface.CursorPointer:
		shape = C.GOOSIE_CURSOR_POINTER
	case surface.CursorText:
		shape = C.GOOSIE_CURSOR_TEXT
	case surface.CursorGrab:
		shape = C.GOOSIE_CURSOR_GRAB
	default:
		shape = C.GOOSIE_CURSOR_DEFAULT
	}
	C.GoosieSetCursor(w.gw, shape)
}

func (w *Window) SetIME(on bool) {
	v := C.int(0)
	if on {
		v = 1
	}
	C.GoosieSetIME(w.gw, v)
}

func (w *Window) ScaleFactor() float32 { return float32(C.GoosieScaleFactor(w.gw)) }

func (w *Window) Size() frame.Size {
	var cw, ch C.int
	C.GoosieSizeDevice(w.gw, &cw, &ch)
	return frame.Size{W: int32(cw), H: int32(ch)}
}

func (w *Window) Name() string { return Name }

func (w *Window) Presents() (queued, dropped, committed int64) {
	var q, d, c C.int
	C.GoosieCounters(w.gw, &q, &d, nil, &c, nil)
	return int64(q), int64(d), int64(c)
}

func (w *Window) DroppedVsyncs() int64 {
	var shed C.int
	C.GoosieCounters(w.gw, nil, nil, &shed, nil, nil)
	return int64(shed)
}

func (w *Window) Run() {
	C.GoosieAppRun(w.gw)
}

func (w *Window) Close() error {
	w.closing.Do(func() {
		close(w.detach)
		C.GoosieClose(w.gw)
	})
	<-w.pumpDone
	return nil
}

type cgoAx struct {
	nodes []C.GoosieAxNode
	strs  []*C.char
}

func (f *cgoAx) str(s string) *C.char {
	if s == "" {
		return nil
	}
	c := C.CString(s)
	f.strs = append(f.strs, c)
	return c
}

func (f *cgoAx) emit(siblings []ax.Node) {
	for i, n := range siblings {
		cur := len(f.nodes)
		f.nodes = append(f.nodes, C.GoosieAxNode{
			role:         axRoleCode(n.Role),
			x0:           C.float(n.X0),
			y0:           C.float(n.Y0),
			x1:           C.float(n.X1),
			y1:           C.float(n.Y1),
			identifier:   f.str(n.ID),
			label:        f.str(n.Label),
			value:        f.str(n.Value),
			href:         f.str(n.Href),
			flags:        axFlags(n),
			first_child:  -1,
			next_sibling: -1,
		})
		if len(n.Children) > 0 {
			f.nodes[cur].first_child = C.int(len(f.nodes))
			f.emit(n.Children)
		}
		if i+1 < len(siblings) {
			f.nodes[cur].next_sibling = C.int(len(f.nodes))
		}
	}
}

func axRoleCode(role ax.Role) C.int {
	switch role {
	case ax.RoleDocument:
		return C.GOOSIE_AX_DOCUMENT
	case ax.RoleGroup:
		return C.GOOSIE_AX_GROUP
	case ax.RoleStaticText:
		return C.GOOSIE_AX_STATIC_TEXT
	case ax.RoleLink:
		return C.GOOSIE_AX_LINK
	case ax.RoleButton:
		return C.GOOSIE_AX_BUTTON
	case ax.RoleImage:
		return C.GOOSIE_AX_IMAGE
	case ax.RoleTextField:
		return C.GOOSIE_AX_TEXT_FIELD
	case ax.RoleSearch:
		return C.GOOSIE_AX_SEARCH
	case ax.RoleProgressBar:
		return C.GOOSIE_AX_PROGRESS
	case ax.RoleMeter:
		return C.GOOSIE_AX_METER
	case ax.RoleTabGroup:
		return C.GOOSIE_AX_TAB_GROUP
	case ax.RoleTab:
		return C.GOOSIE_AX_TAB
	case ax.RoleComboBox:
		return C.GOOSIE_AX_COMBO_BOX
	case ax.RoleOption:
		return C.GOOSIE_AX_OPTION
	case ax.RoleHeading:
		return C.GOOSIE_AX_HEADING
	case ax.RoleListBox:
		return C.GOOSIE_AX_LIST_BOX
	default:
		return C.GOOSIE_AX_GROUP
	}
}

func axFlags(n ax.Node) C.int {
	var flags C.int
	if n.Selected {
		flags |= C.GOOSIE_AX_SELECTED
	}
	if n.Disabled {
		flags |= C.GOOSIE_AX_DISABLED
	}
	if n.AddressBar {
		flags |= C.GOOSIE_AX_ADDRESS_BAR
	}
	if n.Focused {
		flags |= C.GOOSIE_AX_FOCUSED
	}
	return flags
}

func (w *Window) SetAccessibility(nodes []ax.Node) {
	f := &cgoAx{}
	f.emit(nodes)
	if len(f.nodes) > 0 {
		C.GoosieSetAccessibility(w.gw, &f.nodes[0], C.int(len(f.nodes)))
	} else {
		C.GoosieSetAccessibility(w.gw, nil, 0)
	}
	for _, c := range f.strs {
		C.free(unsafe.Pointer(c))
	}
}

var _ surface.Window = (*Window)(nil)
var _ surface.AccessibilityWindow = (*Window)(nil)
