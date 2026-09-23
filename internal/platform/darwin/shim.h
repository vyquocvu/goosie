// shim.h is the whole macOS surface of v2's platform layer: a C API that a Go
// package wraps into a surface.Window. Objective-C lives in shim_darwin.m and
// never crosses this boundary, which is why the rest of v2 can stay pure Go and
// buildable on a machine with no display.
//
// Two rules hold everywhere here:
//
//   - No Go pointer is ever stored in C memory. C keeps its own pixel buffers and
//     its own event queue; a Go buffer is read only during the call that names it.
//   - No call here blocks on the display except GoosieNextEvent, which is the pump
//     a goroutine exists to block in. Present hands its pixels to a queue and
//     returns, because the frame path's "never wait on the platform" rule is the
//     same rule it has for rasterization.
#ifndef GOOSIE_SHIM_H
#define GOOSIE_SHIM_H

#ifdef __cplusplus
extern "C" {
#endif

// GoosieWindow is one on-screen window and everything hanging off it: the event
// queue, the pixel ring Core Animation reads from, and the display link.
typedef struct GoosieWindow GoosieWindow;

// GoosieEventKind is a discriminator for GoosieEvent. The values are the shim's
// own and are mapped to surface.EventKind in Go, deliberately not by numeric
// equality: an enum that a Go iota happens to match is an enum that breaks
// silently when either side reorders it.
enum {
	GOOSIE_EV_VSYNC = 0,
	GOOSIE_EV_SCROLL = 1,
	GOOSIE_EV_POINTER = 2,
	GOOSIE_EV_KEY = 3,
	GOOSIE_EV_RESIZE = 4,
	GOOSIE_EV_IME = 5,
	// GOOSIE_EV_NONE is what a zeroed event reads as, so a struct that was never
	// filled cannot be mistaken for a tick.
	GOOSIE_EV_NONE = 255
};

// GoosieIMEAction separates the two things an EvIME carries.
enum {
	GOOSIE_IME_MARKED = 0,
	GOOSIE_IME_COMMIT = 1
};

// GoosiePointerAction separates the three things an EvPointer carries.
enum {
	GOOSIE_POINTER_PRESS = 0,
	GOOSIE_POINTER_RELEASE = 1,
	GOOSIE_POINTER_MOTION = 2
};

// GoosieCursor is the pointer shape, mirroring surface.Cursor's four cases.
enum {
	GOOSIE_CURSOR_DEFAULT = 0,
	GOOSIE_CURSOR_POINTER = 1,
	GOOSIE_CURSOR_TEXT = 2,
	GOOSIE_CURSOR_GRAB = 3
};

// GoosiePresentStatus is GoosiePresent's result.
enum {
	GOOSIE_OK = 0,
	// GOOSIE_DROPPED means every pixel surface is still owed to Core Animation, so
	// this frame was not queued. The next frame re-uploads its own damage; a dropped
	// present is a skipped frame, never a stalled UI thread.
	GOOSIE_DROPPED = 1,
	// GOOSIE_CLOSED means the window is gone and the caller should stop drawing.
	GOOSIE_CLOSED = 2
};

// Modifier flags for key events, matching NSEventModifierFlag masks.
enum {
	GOOSIE_MOD_SHIFT   = 1 << 17,
	GOOSIE_MOD_OPTION  = 1 << 19,
	GOOSIE_MOD_CONTROL = 1 << 18,
	GOOSIE_MOD_COMMAND = 1 << 20,
};

// Special key codes mapped from NSEvent keyCode values.
// These are in the Unicode Private Use Area so they never collide with a real rune.
enum {
	GOOSIE_KEY_UP    = 0xF700,
	GOOSIE_KEY_DOWN  = 0xF701,
	GOOSIE_KEY_LEFT  = 0xF702,
	GOOSIE_KEY_RIGHT = 0xF703,
	GOOSIE_KEY_HOME  = 0xF704,
	GOOSIE_KEY_END   = 0xF705,
};

// GoosieEvent is one platform input, in device pixels. at_ns is the shim's clock
// at the moment the input happened, which is where a vsync-to-present measurement
// starts.
typedef struct GoosieEvent {
	int kind;
	int dx, dy;     // GOOSIE_EV_SCROLL, device pixels
	int px, py;     // GOOSIE_EV_POINTER
	int action;     // GOOSIE_POINTER_*
	int button;     // 1 left, 2 right, 3 middle, 0 none
	int key;        // GOOSIE_EV_KEY: the typed rune, 0 for a control key
	int mods;       // GOOSIE_EV_KEY: GOOSIE_MOD_* modifier flags
	int ime;        // GOOSIE_EV_IME: GOOSIE_IME_MARKED or GOOSIE_IME_COMMIT
	char text[256]; // GOOSIE_EV_IME: UTF-8 composition text, NUL-terminated
	int w, h;       // GOOSIE_EV_RESIZE: the new surface size, device pixels
	double scale;   // the device pixel ratio this event was produced under
	long long at_ns;
} GoosieEvent;

// GoosieWindowCreate builds the application object, the window, and its display
// link, and shows the window. It must be called on the process's main thread -
// AppKit requires it and says so by returning NULL with *err set rather than by
// letting the call abort the process. w and h are device pixels; the window's
// frame is derived from them under the scale the backing store reports.
GoosieWindow *GoosieWindowCreate(const char *title, int w, int h, int *err);

// GoosieIsMainThread reports whether the calling thread is the process's main
// thread, which is the only thread AppKit will work with.
int GoosieIsMainThread(void);

// GoosieHasDisplay reports whether this process can reach a window server at all.
// It is the question platform.Available asks, and it is answered from CoreGraphics
// rather than from AppKit on purpose: a capability probe that needed the main thread
// could only be made by a program already committed to opening a window, and a test
// binary on a machine with a display has to be able to ask without deciding anything.
int GoosieHasDisplay(void);

// GoosieAppRun turns the main event loop until the window is closed, by GoosieClose or
// by the close button. Call it on the main thread, from the goroutine that created the
// window and stays pinned to that thread. It returns on its own once the quit flag is
// set; nothing here needs a pending event to notice one.
void GoosieAppRun(GoosieWindow *gw);

// GoosieNextEvent blocks until an event is queued and writes it to *out,
// returning 1, or returns 0 once the window is closed. It is the only blocking
// call here, and it is meant for one goroutine of its own.
int GoosieNextEvent(GoosieWindow *gw, GoosieEvent *out);

// GoosiePresent uploads the rects that changed into the front pixel surface and
// queues a Core Animation commit. damage is 4 ints per rect - x0, y0, x1, y1 in
// surface coordinates, which is frame.Rect's own layout - and may be NULL with n
// 0, meaning "the whole surface". It never waits for the commit.
int GoosiePresent(GoosieWindow *gw, const unsigned char *pixels, int w, int h,
	int stride, const int *damage, int n);

// GoosieSetCursor asks for a pointer shape. Like every other AppKit call it is
// carried to the main thread rather than made there.
void GoosieSetCursor(GoosieWindow *gw, int cursor);

// GoosieSetIME turns the window's text input context on or off. Enabled, key
// events flow through NSTextInputClient so an input method can compose; the
// composition lands here as GOOSIE_EV_IME events. Disabling drops any marked
// text. Like every other AppKit call it is carried to the main thread.
void GoosieSetIME(GoosieWindow *gw, int enabled);

// GoosieScaleFactor is the window's current device pixel ratio. It is readable
// from any thread, which is what makes a scroll event's delta convertible after
// the fact.
double GoosieScaleFactor(GoosieWindow *gw);

// GoosieSizeDevice reports the surface size in device pixels, the size a
// GoosiePresent buffer has to match.
void GoosieSizeDevice(GoosieWindow *gw, int *w, int *h);

// GoosieCounters reads the shim's tallies for a run: presents queued, presents dropped
// for want of a free pixel surface, ticks dropped for want of a reader, commits that
// reached the main thread, and vsync ticks produced. Any argument may be NULL.
//
// The two drops are reported apart because they mean different things: a dropped
// present is the display still holding every surface, while a dropped tick is the frame
// path not coming back for one. A single number would let either hide the other.
void GoosieCounters(GoosieWindow *gw, int *queued, int *dropped, int *shed_vsyncs,
	int *committed, int *vsyncs);

// GoosieClose stops the display link, asks the run loop to end, and wakes a
// GoosieNextEvent that is waiting. It is safe from any thread and safe twice.
//
// There is no GoosieWindowFree beside it, and that is a decision rather than an
// omission. Everything a window owns - the NSWindow, its layer, the pixel ring, the
// commit blocks already queued to the main queue - outlives any teardown a caller
// could order, because a block queued to the main queue may never run once [NSApp run]
// has returned, and freeing the memory it holds a pointer to would be a use-after-free
// with no thread left to report it on. A v2 process that has closed its window is
// minutes from exiting; the shim holds one window's worth of C memory until then.
void GoosieClose(GoosieWindow *gw);

// GoosieClipboardRead returns the clipboard text as a C string the caller must
// free, or NULL if the clipboard is empty or has no string content. The call is
// safe from any thread; it dispatches to the main queue internally.
char *GoosieClipboardRead(void);

// GoosieClipboardWrite replaces the clipboard contents with text. A NULL text
// clears the clipboard. The call is safe from any thread.
void GoosieClipboardWrite(const char *text);

// GoosieAxRole is a node's kind, mapped onto the AX tree's roles in Go rather
// than shared numerically, for the same reason the event kinds are.
enum {
	GOOSIE_AX_DOCUMENT = 0,
	GOOSIE_AX_GROUP = 1,
	GOOSIE_AX_STATIC_TEXT = 2,
	GOOSIE_AX_LINK = 3,
	GOOSIE_AX_BUTTON = 4,
	GOOSIE_AX_IMAGE = 5,
	GOOSIE_AX_TEXT_FIELD = 6
};

// GoosieAxNode is one node of a flattened accessibility tree. The tree arrives
// flat - first_child and next_sibling are indices into the same array, -1 for
// none - because a C walk over a linked Go tree would either hold a Go pointer
// across calls or re-flatten on every frame. Coordinates are window content
// coordinates in CSS pixels - document pixels minus the scroll offset - and the
// shim converts those to screen space at query time.
typedef struct GoosieAxNode {
	int role;
	float x0, y0, x1, y1;
	const char *label;
	const char *value;
	const char *href;
	int first_child;
	int next_sibling;
} GoosieAxNode;

// GoosieSetAccessibility replaces the window's accessibility tree with n nodes.
//
// The strings are copied during this call: the shim strdups them into its own
// snapshot before returning, so the caller frees its CStrings as soon as the
// call returns and nothing of the Go tree is retained. The snapshot itself is
// installed and the previous one freed on the main thread, then
// NSAccessibilityLayoutChangedNotification is posted so VoiceOver re-reads.
// The call is safe from any thread.
void GoosieSetAccessibility(GoosieWindow *gw, const GoosieAxNode *nodes, int n);

#ifdef __cplusplus
}
#endif

#endif  // GOOSIE_SHIM_H
