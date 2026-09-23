// shim_darwin.m is the whole Objective-C half of v2: an NSWindow whose content
// layer shows a CGImage built from the frame path's composed buffer, a CVDisplayLink
// that turns the display's ticks into vsync events, and the AppKit input handlers
// that turn a trackpad, a mouse, a key, and a resize into the same event queue.
//
// The file is named _darwin rather than only tagged because a .m file is compiled
// by cgo on every platform that has cgo: on Linux CI, where clang will happily
// compile Objective-C but has no Cocoa headers, the name is what keeps it out of
// the build.
//
// Two things here are load-bearing for the frame path and are worth saying before
// the code says them again:
//
//   AppKit is called on the main thread only. Everything reachable from a goroutine
//   either touches plain C state under a mutex or dispatches a block to the main
//   queue, which is what lets the UI thread present without waiting for the display.
//
//   The pixels are copied once, into the shim's own surface, and only the rects that
//   changed are copied. That is not timidity about the copy: the frame path hands
//   Present the same backing store every frame and shifts it in place, so a CGImage
//   that referenced the Go buffer directly would have Core Animation reading page
//   content while the next frame was rewriting it. The damage-only upload costs the
//   exposed band - one 100px scroll at 1440x900@2 is 1.2 MB - and makes the racing
//   write impossible, because the memory CA reads is never written by anybody else.

#import "shim.h"

#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreVideo/CoreVideo.h>
#import <QuartzCore/QuartzCore.h>
#include <stdatomic.h>
#import <pthread.h>
#import <stdlib.h>
#import <string.h>

// The two sizes below are enum constants rather than static consts because they are
// array bounds in a struct: in C a const int is not a constant expression, and clang
// accepts the array anyway as a folded variable-length array - an extension, and one
// that would quietly make GoosieWindow's own layout depend on a compiler's patience.
enum {
	// kGoosieSurfaceSlots is how many pixel surfaces a window owns. Two would be a
	// double buffer; three is what keeps Present non-blocking when the display is
	// still committing the frame before last while the UI thread is already uploading
	// the next one. The extra surface costs one more frame of latency in memory and
	// buys back the rule that no call the UI thread makes waits on the display.
	kGoosieSurfaceSlots = 3,

	// kGoosieEventQueue is the event ring's depth. Vsyncs beyond it are dropped and
	// counted rather than queued, for the same reason the headless window drops them:
	// a tick nobody has taken yet is a frame the UI thread has already missed, and
	// keeping it would turn one slow frame into a backlog that skews every later
	// timing measurement.
	kGoosieEventQueue = 512
};

// kGoosieIdlePoll is how long the main loop waits for input before looking at the quit
// flag again. It is a latency ceiling on shutdown, not on drawing: a frame begins at a
// display-link tick and the display link does not answer to this loop.
static const double kGoosieIdlePoll = 0.02;

// ---------------------------------------------------------------------------
// accessibility
// ---------------------------------------------------------------------------

// The published tree is the one AppKit object graph the frame path hands over: a
// GoosieAxElement per node, built from a snapshot when Go pushes one and swapped
// out whole on the next publish. Strings are copied into NSStrings at build time
// and the flat C snapshot freed right after, so nothing here points at Go memory.
// Like the window itself, everything built here lives until the swap that replaces
// it - an assistive client may still be holding an element when the next publish
// lands, and per-node lifetimes would turn every query into a race.

static char *goosieAxCopy(const char *s) { return s ? strdup(s) : NULL; }

typedef struct GoosieAxSnapshot {
	GoosieAxNode *nodes;
	int n;
} GoosieAxSnapshot;

// GoosieAxSnapshotFree frees a snapshot and the strings inside it. Safe with NULL.
static void GoosieAxSnapshotFree(GoosieAxSnapshot *snap) {
	if (!snap) return;
	for (int i = 0; i < snap->n; i++) {
		free((void *)snap->nodes[i].label);
		free((void *)snap->nodes[i].value);
		free((void *)snap->nodes[i].href);
	}
	free(snap->nodes);
	free(snap);
}

// axRoleName maps a GoosieAxRole onto the role string AppKit reports. The web area
// is spelled out because AppKit exports no constant for it; it is the role both
// WebKit and Chromium report for a document's content.
static NSString *axRoleName(int role) {
	switch (role) {
		case GOOSIE_AX_DOCUMENT: return @"AXWebArea";
		case GOOSIE_AX_GROUP: return NSAccessibilityGroupRole;
		case GOOSIE_AX_STATIC_TEXT: return NSAccessibilityStaticTextRole;
		case GOOSIE_AX_LINK: return NSAccessibilityLinkRole;
		case GOOSIE_AX_BUTTON: return NSAccessibilityButtonRole;
		case GOOSIE_AX_IMAGE: return NSAccessibilityImageRole;
		case GOOSIE_AX_TEXT_FIELD: return NSAccessibilityTextFieldRole;
		default: return NSAccessibilityUnknownRole;
	}
}

// GoosieAxElement is one published node as an assistive client sees it. It keeps
// its rect in the content view's own flipped, point-based space and converts on
// every query, so a window that has moved or changed displays reports the frame it
// is on now rather than the one it was built on - the reason this is a subclass
// rather than the frame-at-creation element AppKit's constructor hands out.
@interface GoosieAxElement : NSAccessibilityElement
- (instancetype)initWithRole:(int)role
                      label:(NSString *)label
                      value:(NSString *)value
                       href:(NSString *)href
                      frame:(NSRect)frameInView
                       view:(NSView *)view
                    children:(NSArray *)children;
@end

@implementation GoosieAxElement {
	int _role;
	NSString *_label;
	NSString *_value;
	NSString *_href;
	NSRect _frame;
	// The view outlives every element - the window's own retain is what keeps it
	// alive - so the reference is deliberately not retained: a retained view here
	// is a cycle, and the unretained one is always valid.
	NSView *_view;
	NSArray *_children;
}

- (instancetype)initWithRole:(int)role
                      label:(NSString *)label
                      value:(NSString *)value
                       href:(NSString *)href
                      frame:(NSRect)frameInView
                       view:(NSView *)view
                    children:(NSArray *)children {
	if ((self = [super init])) {
		_role = role;
		_label = [label copy];
		_value = [value copy];
		_href = [href copy];
		_frame = frameInView;
		_view = view;
		_children = [children copy];
	}
	return self;
}

- (void)dealloc {
	[_label release];
	[_value release];
	[_href release];
	[_children release];
	[super dealloc];
}

// accessibilityFrame converts through the view into screen space. The accessibility
// APIs report frames with the origin at the top-left of the primary screen, which
// AppKit's convertRectToScreen does not produce directly - it speaks Cocoa's
// bottom-left convention - so the y is flipped here, the way every AX bridge does.
- (NSRect)accessibilityFrame {
	if (!_view) return NSZeroRect;
	NSRect win = [_view convertRect:_frame toView:nil];
	NSRect screen = [[_view window] convertRectToScreen:win];
	NSScreen *primary = [[NSScreen screens] firstObject];
	CGFloat h = primary ? primary.frame.size.height : 0;
	return NSMakeRect(screen.origin.x, h - screen.origin.y - screen.size.height,
			  screen.size.width, screen.size.height);
}

- (NSAccessibilityRole)accessibilityRole { return axRoleName(_role); }

- (NSString *)accessibilityLabel { return _label; }

- (NSString *)accessibilityValue {
	return _value.length > 0 ? _value : [super accessibilityValue];
}

- (NSURL *)accessibilityURL {
	return _href.length > 0 ? [NSURL URLWithString:_href] : nil;
}

- (BOOL)isAccessibilityElement { return YES; }

- (NSArray *)accessibilityChildren {
	return _children ? _children : @[];
}

@end

// GoosieContentView is the window's only view and the only thing in the shim that
// sees input. It is layer-backed rather than drawRect: - because the frame path hands
// over finished pixels, and a view that redrew them through CoreGraphics would be a
// second rasterizer on the machine.
@interface GoosieContentView : NSView <NSTextInputClient>
// The owner is C memory the view does not own and must not retain: it is the window
// the shim handed out, reachable only so an input handler can push an event. See
// GoosieClose for why nothing here is ever freed.
- (void *)goosieOwner;
- (void)setGoosieOwner:(void *)owner;
- (void)setIMEEnabled:(BOOL)on;
- (void)setAccessibilityTree:(GoosieAxSnapshot *)snap;
@end

@interface GoosieWindowDelegate : NSObject <NSWindowDelegate>
@end

@interface GoosieAppDelegate : NSObject <NSApplicationDelegate>
@end

struct GoosieWindow {
	// The AppKit objects. Retained for the process: releasing an NSWindow needs the
	// main thread and a run loop that is still turning, and a Go program that has
	// stopped [NSApp run] has neither. See GoosieClose.
	NSWindow *window;
	GoosieContentView *view;
	GoosieWindowDelegate *winDel;
	GoosieAppDelegate *appDel;
	CVDisplayLinkRef link;

	pthread_mutex_t mu;
	pthread_cond_t cv;
	GoosieEvent queue[kGoosieEventQueue];
	int head;
	int count;
	// quit is set by a close or a window-closed callback and is what releases a
	// blocked GoosieNextEvent. Every commit block checks it before touching gw.
	int quit;

	// The pixel surfaces. surf[i] is C memory, sw/sh[i] its dimensions, ss[i] its stride,
	// cap[i] its allocation, busy[i] whether Core Animation still owes it a release callback.
	unsigned char *surf[kGoosieSurfaceSlots];
	int sw[kGoosieSurfaceSlots];
	int sh[kGoosieSurfaceSlots];
	int ss[kGoosieSurfaceSlots];
	size_t cap[kGoosieSurfaceSlots];
	int busy[kGoosieSurfaceSlots];
	// need_full says the next successful present has to upload the whole surface rather
	// than its own damage list, because what a slot holds no longer matches what the
	// frame path composed. It starts set - a freshly malloc'd surface is garbage - and is
	// set by a dropped present, whose pixels never reached the screen.
	int need_full;
	int front;

	// The device-pixel geometry and scale, readable from any thread.
	_Atomic int devW;
	_Atomic int devH;
	_Atomic double scale;

	_Atomic int queued;
	_Atomic int dropped;
	_Atomic int commits;
	_Atomic int vsyncs;
	_Atomic int shedVsyncs;
};

// ---------------------------------------------------------------------------
// time and the event queue
// ---------------------------------------------------------------------------

static long long nowNs(void) {
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC_RAW, &ts);
	return (long long)ts.tv_sec * 1000000000LL + ts.tv_nsec;
}

// push queues one event and wakes the pump. It runs on the CVDisplayLink thread, on
// AppKit's main thread, and on neither: none of those may block on a Go goroutine
// that has stopped reading, so a full ring sheds a tick rather than waiting and
// reports the shed through the counters.
//
// Real input is never shed while a tick is available to shed instead. A dropped vsync
// costs one skipped frame; a dropped scroll delta leaves the document at an offset the
// user did not scroll to, which is a wrong answer rather than a late one. So when the
// ring is full and a tick is the oldest thing in it, the tick gives up its slot.
static void push(GoosieWindow *gw, GoosieEvent ev) {
	pthread_mutex_lock(&gw->mu);
	if (gw->quit) {
		pthread_mutex_unlock(&gw->mu);
		return;
	}
	if (gw->count >= kGoosieEventQueue) {
		// queue[head] is the oldest event, and a ring this deep has been filling for at
		// least eight frames of nobody reading. Evicting its oldest tick is the only
		// eviction that keeps input intact; a ring full of input is one that deserves to
		// lose this event instead.
		if (ev.kind != GOOSIE_EV_VSYNC && gw->queue[gw->head].kind == GOOSIE_EV_VSYNC) {
			gw->head = (gw->head + 1) % kGoosieEventQueue;
			gw->count--;
			atomic_fetch_add(&gw->shedVsyncs, 1);
		} else {
			pthread_mutex_unlock(&gw->mu);
			if (ev.kind == GOOSIE_EV_VSYNC) atomic_fetch_add(&gw->shedVsyncs, 1);
			return;
		}
	}
	int tail = (gw->head + gw->count) % kGoosieEventQueue;
	gw->queue[tail] = ev;
	gw->count++;
	pthread_cond_signal(&gw->cv);
	pthread_mutex_unlock(&gw->mu);
}

static void pushQuit(GoosieWindow *gw) {
	pthread_mutex_lock(&gw->mu);
	gw->quit = 1;
	pthread_cond_broadcast(&gw->cv);
	pthread_mutex_unlock(&gw->mu);
}

// ---------------------------------------------------------------------------
// the display link
// ---------------------------------------------------------------------------

// onVsync runs on a CoreVideo thread at the panel's refresh rate. It does nothing
// but stamp a tick and hand it to the queue, which is the whole reason vsync is an
// event rather than a callback: the UI thread's one input source stays the channel.
static CVReturn onVsync(CVDisplayLinkRef link, const CVTimeStamp *now,
			const CVTimeStamp *outputTime, CVOptionFlags flagsIn,
			CVOptionFlags *flagsOut, void *info) {
	GoosieWindow *gw = (GoosieWindow *)info;
	GoosieEvent ev;
	memset(&ev, 0, sizeof(ev));
	ev.kind = GOOSIE_EV_VSYNC;
	ev.scale = atomic_load(&gw->scale);
	ev.at_ns = nowNs();
	atomic_fetch_add(&gw->vsyncs, 1);
	(void)link;
	(void)now;
	(void)outputTime;
	(void)flagsIn;
	(void)flagsOut;
	push(gw, ev);
	return kCVReturnSuccess;
}

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

static int startDisplayLink(GoosieWindow *gw) {
	CVDisplayLinkRef link = NULL;
	if (CVDisplayLinkCreateWithCGDisplay(CGMainDisplayID(), &link) != kCVReturnSuccess || link == NULL) {
		// No display link means no ticks, which means a window that never redraws. That
		// is a machine property a caller has to be able to see, so it is reported through
		// GoosieWindowCreate's failure rather than by a window that quietly sits there.
		return 0;
	}
	if (CVDisplayLinkSetOutputCallback(link, onVsync, gw) != kCVReturnSuccess) {
		CVDisplayLinkRelease(link);
		return 0;
	}
	if (CVDisplayLinkStart(link) != kCVReturnSuccess) {
		CVDisplayLinkRelease(link);
		return 0;
	}
	gw->link = link;
	return 1;
}

static void stopDisplayLink(GoosieWindow *gw) {
	if (gw->link == NULL) return;
	CVDisplayLinkStop(gw->link);
	CVDisplayLinkRelease(gw->link);
	gw->link = NULL;
}

#pragma clang diagnostic pop

// ---------------------------------------------------------------------------
// the view and the delegates
// ---------------------------------------------------------------------------

// deviceScale is the ratio the view's geometry has to be multiplied by. It is read
// from the window rather than from a cached value because a window can move to a
// display with another ratio between two events.
static double viewScale(GoosieContentView *v) {
	NSWindow *w = v.window;
	return w ? w.backingScaleFactor : 1.0;
}

static int deviceWidth(GoosieContentView *v) {
	return (int)(v.bounds.size.width * viewScale(v) + 0.5);
}

static int deviceHeight(GoosieContentView *v) {
	return (int)(v.bounds.size.height * viewScale(v) + 0.5);
}

static void sendResize(GoosieContentView *v) {
	GoosieWindow *gw = (GoosieWindow *)[v goosieOwner];
	if (!gw) return;
	GoosieEvent ev;
	memset(&ev, 0, sizeof(ev));
	ev.kind = GOOSIE_EV_RESIZE;
	ev.w = deviceWidth(v);
	ev.h = deviceHeight(v);
	ev.scale = viewScale(v);
	ev.at_ns = nowNs();
	atomic_store(&gw->devW, ev.w);
	atomic_store(&gw->devH, ev.h);
	atomic_store(&gw->scale, ev.scale);
	push(gw, ev);
}

// pushIME queues one composition event. The text rides a fixed 256-byte buffer,
// so a longer composition is cut at a UTF-8 boundary rather than handed to Go as
// a partial rune.
static void pushIME(GoosieWindow *gw, int action, NSString *text) {
	GoosieEvent ev;
	memset(&ev, 0, sizeof(ev));
	ev.kind = GOOSIE_EV_IME;
	ev.ime = action;
	const char *utf8 = [text UTF8String];
	size_t n = strlen(utf8);
	if (n > sizeof(ev.text) - 1) n = sizeof(ev.text) - 1;
	while (n > 0) {
		unsigned char b = (unsigned char)utf8[n - 1];
		if ((b & 0xC0) == 0x80) { n--; continue; }
		if ((b & 0x80) != 0) n--;  // lead byte whose continuation bytes were cut
		break;
	}
	memcpy(ev.text, utf8, n);
	ev.text[n] = 0;
	ev.at_ns = nowNs();
	push(gw, ev);
}

@implementation GoosieContentView {
	GoosieWindow *_ownerRef;
	// _imeEnabled routes printable key events through NSTextInputClient so an
	// input method can compose. _imeMarked mirrors the client-side marked state
	// the input context queries back.
	BOOL _imeEnabled;
	BOOL _imeMarked;
	// _axElements is the tree the accessibility snapshot built, or nil for none.
	NSArray *_axElements;
}

// isFlipped puts the view's y axis top-down like the frame path's, so an event's
// position is a device-pixel offset from the surface's top-left with no flip in the
// Go code that reads it.
- (BOOL)isFlipped { return YES; }
- (BOOL)wantsUpdateLayer { return YES; }
- (BOOL)acceptsFirstResponder { return YES; }
- (BOOL)isOpaque { return YES; }

- (void *)goosieOwner { return (void *)_ownerRef; }
- (void)setGoosieOwner:(void *)owner { _ownerRef = (GoosieWindow *)owner; }

- (void)setIMEEnabled:(BOOL)on {
	if (_imeEnabled == on) return;
	_imeEnabled = on;
	// Turning the context off mid-composition cancels it: the Go side's focus is
	// gone, so a preview that kept composing would land in a control nobody sees.
	if (!on && _imeMarked) {
		_imeMarked = NO;
		GoosieWindow *gw = (GoosieWindow *)[self goosieOwner];
		if (gw) pushIME(gw, GOOSIE_IME_MARKED, @"");
	}
}

- (void)setFrameSize:(NSSize)newSize {
	[super setFrameSize:newSize];
	sendResize(self);
}

- (void)viewDidChangeBackingProperties {
	[super viewDidChangeBackingProperties];
	// A window moved to a display with another ratio changes the meaning of every
	// delta and every rect, so the frame path has to be told in the same breath.
	sendResize(self);
}

// scrollWheel: delivers device-pixel deltas. A precise (trackpad) scroll is already
// in points and scales 1:1; a legacy wheel delta is in lines, and multiplying by a
// fixed pitch is the same thing every other toolkit does rather than a guess at
// macOS's own acceleration curve.
//
// momentumPhase is deliberately not read. AppKit keeps reporting decaying deltas in
// scrollingDeltaY through a fling, and M1 applies a delta straight to the scroll offset
// without its own kinetic model, so a flick already travels the distance the gesture
// carried. What the phase would buy is the ability to tell a finger on the glass from a
// released fling - which matters as soon as something wants to interrupt momentum, and
// belongs on surface.Event rather than being half-expressed here and dropped.
- (void)scrollWheel:(NSEvent *)e {
	GoosieWindow *gw = (GoosieWindow *)[self goosieOwner];
	if (!gw) return;
	double s = viewScale(self);
	double factor = e.hasPreciseScrollingDeltas ? s : s * 10.0;
	GoosieEvent ev;
	memset(&ev, 0, sizeof(ev));
	ev.kind = GOOSIE_EV_SCROLL;
	ev.dx = (int)(e.scrollingDeltaX * factor);
	ev.dy = (int)(e.scrollingDeltaY * factor);
	ev.scale = s;
	ev.at_ns = nowNs();
	if (ev.dx == 0 && ev.dy == 0) return;
	push(gw, ev);
}

- (void)sendPointer:(NSEvent *)e action:(int)action {
	GoosieWindow *gw = (GoosieWindow *)[self goosieOwner];
	if (!gw) return;
	NSPoint p = [self convertPoint:e.locationInWindow fromView:nil];
	double s = viewScale(self);
	GoosieEvent ev;
	memset(&ev, 0, sizeof(ev));
	ev.kind = GOOSIE_EV_POINTER;
	ev.px = (int)(p.x * s);
	ev.py = (int)(p.y * s);
	ev.action = action;
	ev.scale = s;
	ev.at_ns = nowNs();
	switch (e.buttonNumber) {
		case 0: ev.button = 1; break;  // left
		case 1: ev.button = 3; break;  // right
		case 2: ev.button = 2; break;  // middle
		default: ev.button = 0; break;
	}
	push(gw, ev);
}

- (void)mouseDown:(NSEvent *)e { [self sendPointer:e action:GOOSIE_POINTER_PRESS]; }
- (void)mouseUp:(NSEvent *)e { [self sendPointer:e action:GOOSIE_POINTER_RELEASE]; }
- (void)mouseDragged:(NSEvent *)e { [self sendPointer:e action:GOOSIE_POINTER_MOTION]; }
- (void)rightMouseDown:(NSEvent *)e { [self sendPointer:e action:GOOSIE_POINTER_PRESS]; }
- (void)rightMouseUp:(NSEvent *)e { [self sendPointer:e action:GOOSIE_POINTER_RELEASE]; }
- (void)otherMouseDown:(NSEvent *)e { [self sendPointer:e action:GOOSIE_POINTER_PRESS]; }
- (void)otherMouseUp:(NSEvent *)e { [self sendPointer:e action:GOOSIE_POINTER_RELEASE]; }
- (void)mouseMoved:(NSEvent *)e { [self sendPointer:e action:GOOSIE_POINTER_MOTION]; }

- (void)keyDown:(NSEvent *)e {
	GoosieWindow *gw = (GoosieWindow *)[self goosieOwner];
	if (!gw) return;
	GoosieEvent ev;
	memset(&ev, 0, sizeof(ev));
	ev.kind = GOOSIE_EV_KEY;
	ev.scale = viewScale(self);
	ev.at_ns = nowNs();
	ev.mods = (int)e.modifierFlags;
	// Map special keys by keyCode first; these produce no useful character.
	switch (e.keyCode) {
		case 126: ev.key = GOOSIE_KEY_UP; push(gw, ev); return;
		case 125: ev.key = GOOSIE_KEY_DOWN; push(gw, ev); return;
		case 123: ev.key = GOOSIE_KEY_LEFT; push(gw, ev); return;
		case 124: ev.key = GOOSIE_KEY_RIGHT; push(gw, ev); return;
		case 115: ev.key = GOOSIE_KEY_HOME; push(gw, ev); return;
		case 119: ev.key = GOOSIE_KEY_END; push(gw, ev); return;
	}
	// With the input context on, printable keys go through NSTextInputClient first so
	// an IME can compose them. handleEvent returns NO for keys it does not want -
	// those fall through to the raw path below, unchanged.
	if (_imeEnabled && !(e.modifierFlags & (NSEventModifierFlagCommand | NSEventModifierFlagControl))) {
		if ([[self inputContext] handleEvent:e]) return;
	}
	// charactersIgnoringModifiers strips Cmd/Ctrl but keeps Option (dead keys,
	// compose sequences). Control+letter yields a control code (1-26); convert
	// back to the base lowercase letter so the Go handler sees the letter with
	// ModControl rather than a bare control code.
	NSString *chars = [e charactersIgnoringModifiers];
	if (chars.length > 0) {
		unichar c = [chars characterAtIndex:0];
		if ((ev.mods & GOOSIE_MOD_CONTROL) && c < 0x20 && c >= 1) {
			c = 'a' + (c - 1) % 26;
		}
		// Take the first whole UTF-16 scalar, never a lone surrogate: Go's rune
		// conversion would turn one into U+FFFD and corrupt the edit. A lone
		// surrogate leaves key 0, which the Go side treats as a control key.
		if (c >= 0xD800 && c <= 0xDBFF && chars.length > 1) {
			unichar lo = [chars characterAtIndex:1];
			if (lo >= 0xDC00 && lo <= 0xDFFF) {
				ev.key = (int)(0x10000 + ((c - 0xD800) << 10) + (lo - 0xDC00));
			}
		} else if (c < 0xD800 || c > 0xDFFF) {
			ev.key = (int)c;
		}
	}
	push(gw, ev);
}

// NSTextInputClient is the input method's view of this window. It carries no text
// state of its own: insertions and marked-text updates are pushed to Go as EvIME
// events and the engine holds the composing string, because the caret and the value
// live in the document, not in AppKit.
- (void)insertText:(id)string replacementRange:(NSRange)replacementRange {
	NSString *text = [string isKindOfClass:[NSAttributedString class]] ? [string string] : (NSString *)string;
	GoosieWindow *gw = (GoosieWindow *)[self goosieOwner];
	_imeMarked = NO;
	if (gw) pushIME(gw, GOOSIE_IME_COMMIT, text);
}

- (void)setMarkedText:(id)string selectedRange:(NSRange)selRange replacementRange:(NSRange)replacementRange {
	NSString *text = [string isKindOfClass:[NSAttributedString class]] ? [string string] : (NSString *)string;
	if (!text) text = @"";
	GoosieWindow *gw = (GoosieWindow *)[self goosieOwner];
	_imeMarked = text.length > 0;
	if (gw) pushIME(gw, GOOSIE_IME_MARKED, text);
}

- (void)unmarkText {
	GoosieWindow *gw = (GoosieWindow *)[self goosieOwner];
	_imeMarked = NO;
	if (gw) pushIME(gw, GOOSIE_IME_MARKED, @"");
}

- (BOOL)hasMarkedText { return _imeMarked; }

- (NSRange)markedRange { return _imeMarked ? NSMakeRange(0, 1) : NSMakeRange(NSNotFound, 0); }

- (NSRange)selectedRange { return NSMakeRange(0, 0); }

- (NSArray *)validAttributesForMarkedText { return @[]; }

- (NSAttributedString *)attributedSubstringForProposedRange:(NSRange)range actualRange:(NSRangePointer)actualRange {
	return nil;
}

- (NSUInteger)characterIndexForPoint:(NSPoint)point { return 0; }

- (NSRect)firstRectForCharacterRange:(NSRange)range actualRange:(NSRangePointer)actualRange {
	// The candidate window anchors at the view, not at the caret: reporting the
	// view's frame keeps the panel near the input, which is the part a v2 without a
	// text-run-to-caret mapping can honestly answer.
	NSRect r = [self convertRect:self.bounds toView:nil];
	return [[self window] convertRectToScreen:r];
}

// accessibilityChildren is how the published tree hangs off the window: the view
// has no subviews of its own, so the override is the whole story - what AX sees
// under this view is the elements built from the last snapshot, in document order.
- (NSArray *)accessibilityChildren { return _axElements; }

- (BOOL)isAccessibilityElement { return NO; }

// axElementsFromSnapshot builds one element per node, depth first. The walk reads
// only the snapshot's own array - children and siblings are indices - and is
// bounded by the depth cap and by the sibling count against n, so a malformed
// snapshot (a cycle among the indices, a child pointing outside the array) costs a
// short walk instead of a hang. Strings become NSStrings here, which is what lets
// the snapshot itself be freed the moment this method returns.
- (NSArray *)axElementsFromSnapshot:(GoosieAxSnapshot *)snap index:(int)i depth:(int)depth {
	NSMutableArray *out = [NSMutableArray array];
	int count = 0;
	for (int c = i; c >= 0 && c < snap->n && count < snap->n; c = snap->nodes[c].next_sibling, count++) {
		GoosieAxNode *n = &snap->nodes[c];
		NSArray *kids = nil;
		if (depth < 64 && n->first_child >= 0 && n->first_child < snap->n) {
			kids = [self axElementsFromSnapshot:snap index:n->first_child depth:depth + 1];
		}
		NSString *href = (n->href && n->href[0]) ? [NSString stringWithUTF8String:n->href] : nil;
		GoosieAxElement *el = [[GoosieAxElement alloc]
		    initWithRole:n->role
		    label:(n->label ? [NSString stringWithUTF8String:n->label] : @"")
		    value:(n->value ? [NSString stringWithUTF8String:n->value] : @"")
		    href:href
		    frame:NSMakeRect(n->x0, n->y0, n->x1 - n->x0, n->y1 - n->y0)
		    view:self
		    children:kids];
		[out addObject:el];
		[el release];
	}
	return out;
}

// setAccessibilityTree replaces the published tree and tells an assistive client
// the layout changed. The new elements are built before the swap, so a query
// landing mid-publish sees the previous tree, never a half-built one; the snapshot
// is freed after building because every string it carried has been copied.
- (void)setAccessibilityTree:(GoosieAxSnapshot *)snap {
	NSArray *fresh = snap && snap->n > 0 ? [self axElementsFromSnapshot:snap index:0 depth:0] : nil;
	NSArray *old = _axElements;
	_axElements = [fresh retain];
	[old release];
	GoosieAxSnapshotFree(snap);
	if (fresh) {
		NSAccessibilityPostNotification(self, NSAccessibilityLayoutChangedNotification);
	}
}

@end

@implementation GoosieWindowDelegate {
	GoosieWindow *_gw;
}

- (instancetype)initWithWindow:(GoosieWindow *)gw {
	if ((self = [super init])) _gw = gw;
	return self;
}

- (BOOL)windowShouldClose:(id)sender {
	// The close button is a request to stop drawing, not to free the window: the pump
	// learns from pushQuit that there will be no more events.
	pushQuit(_gw);
	return YES;
}

- (void)windowWillClose:(NSNotification *)note { pushQuit(_gw); }

@end

@implementation GoosieAppDelegate {
	GoosieWindow *_gw;
}

- (instancetype)initWithWindow:(GoosieWindow *)gw {
	if ((self = [super init])) _gw = gw;
	return self;
}

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender { return YES; }

- (void)applicationWillFinishLaunching:(NSNotification *)notification {
	// A menu bar with a working Quit item, because ⌘Q is how a person stops a GUI
	// program and an app with no menu has no key equivalent to answer it with.
	NSMenu *bar = [[NSMenu alloc] init];
	NSMenuItem *item = [bar addItemWithTitle:@"Goosie" action:NULL keyEquivalent:@""];
	NSMenu *appMenu = [[NSMenu alloc] init];
	[appMenu addItemWithTitle:@"Quit Goosie"
                       action:@selector(terminate:)
                keyEquivalent:@"q"];
	[item setSubmenu:appMenu];
	[NSApp setMainMenu:bar];
	[appMenu release];
	[bar release];
}

- (void)applicationWillTerminate:(NSNotification *)notification {
	if (_gw) pushQuit(_gw);
}

@end

// ---------------------------------------------------------------------------
// presentation
// ---------------------------------------------------------------------------

// GoosieRetire is what ties one Core Animation image back to the surface it came
// from. It is C memory holding a C pointer, which is the whole point: the release
// callback runs on a CoreGraphics thread, and nothing reachable from it may be a Go
// object.
typedef struct {
	GoosieWindow *gw;
	int slot;
} GoosieRetire;

static void retireProvider(void *info, const void *data, size_t size) {
	GoosieRetire *r = (GoosieRetire *)info;
	(void)data;
	(void)size;
	if (!r) return;
	GoosieWindow *gw = r->gw;
	pthread_mutex_lock(&gw->mu);
	if (r->slot >= 0 && r->slot < kGoosieSurfaceSlots) gw->busy[r->slot] = 0;
	pthread_mutex_unlock(&gw->mu);
	free(r);
}

static NSCursor *cursorFor(int shape) {
	switch (shape) {
		case GOOSIE_CURSOR_POINTER: return [NSCursor pointingHandCursor];
		case GOOSIE_CURSOR_TEXT: return [NSCursor IBeamCursor];
		case GOOSIE_CURSOR_GRAB: return [NSCursor openHandCursor];
		default: return [NSCursor arrowCursor];
	}
}

// commitSurface is the only place this file touches Core Animation, and it runs on
// the main thread. The transaction disables implicit actions for one reason: a layer
// whose contents animate between two frames interpolates the browser's scroll, which
// looks like smooth motion and measures like a display that is always one frame
// behind.
static void commitSurface(GoosieWindow *gw, int slot, int w, int h, int stride) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gw->quit) {
			pthread_mutex_lock(&gw->mu);
			gw->busy[slot] = 0;
			pthread_mutex_unlock(&gw->mu);
			return;
		}
		size_t bytes = (size_t)stride * (size_t)h;
		GoosieRetire *r = (GoosieRetire *)malloc(sizeof(GoosieRetire));
		r->gw = gw;
		r->slot = slot;
		CGDataProviderRef provider =
			CGDataProviderCreateWithData(r, gw->surf[slot], bytes, retireProvider);
		if (!provider) {
			free(r);
			pthread_mutex_lock(&gw->mu);
			gw->busy[slot] = 0;
			pthread_mutex_unlock(&gw->mu);
			return;
		}
		CGColorSpaceRef space = CGColorSpaceCreateDeviceRGB();
		// Byte order 32Big with premultiplied-last is the frame path's own in-memory
		// order: frame.Bitmap writes R, G, B, A at ascending addresses, and Color packs
		// R into the high byte. Nothing is swizzled here.
		CGImageRef image = CGImageCreate(w, h, 8, 32, stride, space,
			kCGBitmapByteOrder32Big | kCGImageAlphaPremultipliedLast, provider,
			NULL, /*shouldInterpolate=*/false, kCGRenderingIntentDefault);
		CGColorSpaceRelease(space);
		CGDataProviderRelease(provider);  // the image retained the provider
		if (!image) {
			pthread_mutex_lock(&gw->mu);
			gw->busy[slot] = 0;
			pthread_mutex_unlock(&gw->mu);
			return;
		}
		[CATransaction begin];
		[CATransaction setDisableActions:YES];
		gw->view.layer.contents = (__bridge id)image;
		[CATransaction commit];
		CGImageRelease(image);
		atomic_fetch_add(&gw->commits, 1);
	});
}

// ---------------------------------------------------------------------------
// the C API
// ---------------------------------------------------------------------------

int GoosieIsMainThread(void) { return pthread_main_np(); }

int GoosieHasDisplay(void) { return CGMainDisplayID() != 0; }

GoosieWindow *GoosieWindowCreate(const char *title, int w, int h, int *err) {
	if (err) *err = 0;
	if (!pthread_main_np()) {
		if (err) *err = 1;  // not the main thread; AppKit would abort the process
		return NULL;
	}
	if (w <= 0 || h <= 0) {
		if (err) *err = 2;
		return NULL;
	}
	if (!GoosieHasDisplay()) {
		// Checked before AppKit is touched rather than after: a process with no window
		// server reaches this from a goroutine that was asked for a window, and the
		// answer it needs is an error code, not a half-built NSApplication.
		if (err) *err = 4;
		return NULL;
	}
	GoosieWindow *gw = (GoosieWindow *)calloc(1, sizeof(GoosieWindow));
	if (!gw) {
		if (err) *err = 3;
		return NULL;
	}
	pthread_mutex_init(&gw->mu, NULL);
	pthread_cond_init(&gw->cv, NULL);

	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		[app setActivationPolicy:NSApplicationActivationPolicyRegular];
		gw->appDel = [[GoosieAppDelegate alloc] initWithWindow:gw];
		[app setDelegate:gw->appDel];

		// w and h are device pixels and a content rect is in points, so the window is
		// built under the ratio its screen reports and corrected once the one it actually
		// landed on is known.
		CGFloat scale = [NSScreen mainScreen] ? [[NSScreen mainScreen] backingScaleFactor] : 1.0;
		if (scale < 1.0) scale = 1.0;
		NSRect content = NSMakeRect(0, 0, (CGFloat)w / scale, (CGFloat)h / scale);
		NSWindow *win = [[NSWindow alloc] initWithContentRect:content
												   styleMask:(NSWindowStyleMaskTitled |
													   NSWindowStyleMaskClosable |
													   NSWindowStyleMaskMiniaturizable |
													   NSWindowStyleMaskResizable)
						     backing:NSBackingStoreBuffered
						       defer:NO];
		if (!win) {
			free(gw);
			if (err) *err = 4;  // no window server for this session
			return NULL;
		}
		[win setReleasedWhenClosed:NO];
		gw->winDel = [[GoosieWindowDelegate alloc] initWithWindow:gw];
		[win setDelegate:gw->winDel];
		if (title) [win setTitle:[NSString stringWithUTF8String:title]];

		GoosieContentView *view = [[GoosieContentView alloc] initWithFrame:content];
		[view setGoosieOwner:(void *)gw];
		[view setWantsLayer:YES];
		[win setContentView:view];
		gw->view = view;
		[view release];  // the window's retain, not ours, is what keeps it alive
		[win setAcceptsMouseMovedEvents:YES];

		// A window's real ratio is the one the screen it ends up on reports, and that is
		// not necessarily the main screen's. Correct the frame once for it, then publish
		// the geometry that follows - and let the frame path learn it the way it learns
		// any other resize, because a buffer sized from what was asked for rather than
		// from what was handed out would be presented as a stretched image.
		CGFloat shown = [win backingScaleFactor];
		if (shown < 1.0) shown = 1.0;
		if (shown != scale) {
			[win setContentSize:NSMakeSize((CGFloat)w / shown, (CGFloat)h / shown)];
		}
		[win center];
		[win makeKeyAndOrderFront:nil];
		[app activateIgnoringOtherApps:YES];

		atomic_store(&gw->scale, shown);
		atomic_store(&gw->devW, deviceWidth(view));
		atomic_store(&gw->devH, deviceHeight(view));

		if (!startDisplayLink(gw)) {
			// The window is on screen and its view holds a pointer to gw, so gw is left
			// allocated rather than freed here: freeing it would be a use-after-free the
			// next click could reach. A create that fails is rare, fatal to the run, and
			// costs one struct and the objects above, which is what GoosieClose's note about
			// teardown already accepts for a window that closes.
			[win close];
			if (err) *err = 5;  // a display, but no display link to pace it with
			return NULL;
		}
	}
	return gw;
}

void GoosieAppRun(GoosieWindow *gw) {
	if (!gw) return;
	@autoreleasepool { [NSApp finishLaunching]; }
	// The loop is turned by hand rather than by -[NSApplication run], for one reason:
	// -run notices -stop: only at the top of an iteration it decides to take, and an app
	// with no pending input sits in a kernel message wait indefinitely. A run that can
	// end when the user moves the mouse is not a run a scripted gate can finish, so the
	// quit flag is checked here on a bounded wait instead. The cost is one wake-up per
	// kGoosieIdlePoll while idle; frame pacing comes from the display link, not from
	// this loop, so nothing about a drawn frame changes.
	for (;;) {
		@autoreleasepool {
			pthread_mutex_lock(&gw->mu);
			int quit = gw->quit;
			pthread_mutex_unlock(&gw->mu);
			if (quit) break;
			NSEvent *e = [NSApp nextEventMatchingMask:NSEventMaskAny
									untilDate:[NSDate dateWithTimeIntervalSinceNow:kGoosieIdlePoll]
										inMode:NSDefaultRunLoopMode
									   dequeue:YES];
			if (e) [NSApp sendEvent:e];
		}
	}
	// Nothing is coming after this. The loop is how a queued commit reaches the layer
	// and how a resize is noticed, and it has stopped turning, so a pump still waiting
	// in GoosieNextEvent has to be woken rather than left in a condwait nobody will
	// signal.
	pushQuit(gw);
}

int GoosieNextEvent(GoosieWindow *gw, GoosieEvent *out) {
	if (!gw || !out) return 0;
	pthread_mutex_lock(&gw->mu);
	while (gw->count == 0 && !gw->quit) pthread_cond_wait(&gw->cv, &gw->mu);
	if (gw->count == 0) {
		pthread_mutex_unlock(&gw->mu);
		return 0;
	}
	// Events queued before a close are still delivered: the queue's order is the input's
	// order, and a resize the loop never sees is a buffer the next frame is the wrong
	// size for.
	*out = gw->queue[gw->head];
	gw->head = (gw->head + 1) % kGoosieEventQueue;
	gw->count--;
	pthread_mutex_unlock(&gw->mu);
	return 1;
}

// upload copies the named rects, in surface pixels, out of the caller's buffer and into
// one of the shim's. With no rects it copies the whole surface, which is what a first
// frame or a damage list the composer widened says.
//
// The copy is the point rather than an overhead to apologise for: the frame path hands
// over the same backing store every frame and shifts it in place, so this is the one
// step that makes it impossible for Core Animation to be reading a page while the next
// frame rewrites it.
static void upload(unsigned char *dst, const unsigned char *src, int w, int h, int stride,
			   const int *damage, int n) {
	if (!damage || n <= 0) {
		for (int y = 0; y < h; y++) {
			memcpy(dst + (size_t)y * (size_t)stride, src + (size_t)y * (size_t)stride,
				(size_t)stride);
		}
		return;
	}
	for (int i = 0; i < n; i++) {
		int x0 = damage[4 * i + 0], y0 = damage[4 * i + 1];
		int x1 = damage[4 * i + 2], y1 = damage[4 * i + 3];
		if (x0 < 0) x0 = 0;
		if (y0 < 0) y0 = 0;
		if (x1 > w) x1 = w;
		if (y1 > h) y1 = h;
		if (x1 <= x0 || y1 <= y0) continue;
		size_t bytes = (size_t)(x1 - x0) * 4;
		for (int y = y0; y < y1; y++) {
			size_t off = (size_t)y * (size_t)stride + (size_t)x0 * 4;
			memcpy(dst + off, src + off, bytes);
		}
	}
}

// claimSurface returns the index of a surface this present can write into, resizing the
// slot when it does not match, or -1 when every surface is still owed to Core Animation.
// Call it with gw->mu held; it leaves the chosen slot busy, which is what makes copying
// into it outside the lock safe: nothing else writes a busy surface, and a busy
// surface's allocation cannot change while it is busy.
//
// *fresh comes back 1 when the chosen slot is not the one the last commit used at the
// same size - a new allocation, or a surface that was resized - which is the case where
// its contents cannot be trusted as a base for a partial upload.
static int claimSurface(GoosieWindow *gw, int w, int h, int stride, int *fresh) {
	size_t need = (size_t)stride * (size_t)h;
	for (int i = 0; i < kGoosieSurfaceSlots; i++) {
		if (!gw->busy[i] && gw->surf[i] && gw->sw[i] == w && gw->sh[i] == h && gw->ss[i] == stride) {
			gw->busy[i] = 1;
			*fresh = 0;
			return i;
		}
	}
	for (int i = 0; i < kGoosieSurfaceSlots; i++) {
		if (gw->busy[i]) continue;
		if (gw->cap[i] < need) {
			free(gw->surf[i]);
			gw->surf[i] = (unsigned char *)malloc(need);
			gw->cap[i] = gw->surf[i] ? need : 0;
		}
		if (!gw->surf[i]) continue;
		gw->sw[i] = w;
		gw->sh[i] = h;
		gw->ss[i] = stride;
		gw->busy[i] = 1;
		*fresh = 1;
		return i;
	}
	*fresh = 0;
	return -1;
}

int GoosiePresent(GoosieWindow *gw, const unsigned char *pixels, int w, int h,
			  int stride, const int *damage, int n) {
	if (!gw || !pixels || w <= 0 || h <= 0 || stride < w * 4) return GOOSIE_CLOSED;

	pthread_mutex_lock(&gw->mu);
	if (gw->quit) {
		pthread_mutex_unlock(&gw->mu);
		return GOOSIE_CLOSED;
	}
	int fresh = 0;
	int slot = claimSurface(gw, w, h, stride, &fresh);
	if (slot < 0) {
		// Every surface is still being read by the display. Dropping this frame is the
		// only answer that does not block the UI thread - and the pixels it carried are
		// now missing from the screen, so the next present that gets a surface has to
		// upload everything rather than its own damage list.
		atomic_fetch_add(&gw->dropped, 1);
		gw->need_full = 1;
		pthread_mutex_unlock(&gw->mu);
		return GOOSIE_DROPPED;
	}
	int whole = fresh || gw->need_full;
	if (whole) gw->need_full = 0; // this present carried every pixel the surface will show
	unsigned char *dst = gw->surf[slot];
	pthread_mutex_unlock(&gw->mu);

	if (whole) {
		upload(dst, pixels, w, h, stride, NULL, 0);
	} else {
		upload(dst, pixels, w, h, stride, damage, n);
	}
	atomic_fetch_add(&gw->queued, 1);
	commitSurface(gw, slot, w, h, stride);
	return GOOSIE_OK;
}

void GoosieSetCursor(GoosieWindow *gw, int cursor) {
	if (!gw) return;
	// -[NSCursor set] is an AppKit call, so it goes to the main thread like every other
	// one here rather than being made wherever the frame path happened to be standing.
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gw->quit) return;
		[cursorFor(cursor) set];
	});
}

void GoosieSetIME(GoosieWindow *gw, int enabled) {
	if (!gw) return;
	// setIMEEnabled touches the view's input context, an AppKit object: main thread.
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gw->quit) return;
		[gw->view setIMEEnabled:enabled ? YES : NO];
	});
}

void GoosieSetAccessibility(GoosieWindow *gw, const GoosieAxNode *nodes, int n) {
	if (!gw) return;
	// The strings are copied on the calling thread, while the caller's memory is
	// guaranteed to still be there: when this returns, the caller frees its own
	// copies and nothing of the Go tree is retained.
	GoosieAxSnapshot *snap = NULL;
	if (nodes && n > 0) {
		snap = malloc(sizeof(*snap));
		if (snap) {
			snap->nodes = malloc(sizeof(GoosieAxNode) * (size_t)n);
			if (!snap->nodes) {
				free(snap);
				snap = NULL;
			} else {
				memcpy(snap->nodes, nodes, sizeof(GoosieAxNode) * (size_t)n);
				snap->n = n;
				for (int i = 0; i < n; i++) {
					snap->nodes[i].label = goosieAxCopy(snap->nodes[i].label);
					snap->nodes[i].value = goosieAxCopy(snap->nodes[i].value);
					snap->nodes[i].href = goosieAxCopy(snap->nodes[i].href);
				}
			}
		}
	}
	// Building elements touches AppKit objects, so the install happens on the main
	// thread like every other one here. The snapshot's ownership moves with the
	// block: setAccessibilityTree: frees it after copying the strings into elements,
	// and a window that quit before the block ran frees it here.
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gw->quit) {
			GoosieAxSnapshotFree(snap);
			return;
		}
		[gw->view setAccessibilityTree:snap];
	});
}

double GoosieScaleFactor(GoosieWindow *gw) {
	if (!gw) return 1.0;
	double s = atomic_load(&gw->scale);
	return s > 0 ? s : 1.0;
}

void GoosieSizeDevice(GoosieWindow *gw, int *w, int *h) {
	if (w) *w = gw ? atomic_load(&gw->devW) : 0;
	if (h) *h = gw ? atomic_load(&gw->devH) : 0;
}

void GoosieCounters(GoosieWindow *gw, int *queued, int *dropped, int *shed_vsyncs,
			    int *committed, int *vsyncs) {
	if (!gw) return;
	if (queued) *queued = atomic_load(&gw->queued);
	// The two kinds of loss are reported apart: a present dropped for want of a free
	// surface says the display is still holding every buffer, and a tick dropped for
	// want of a reader says the frame path did not come back. Added together, either
	// one can hide the other.
	if (dropped) *dropped = atomic_load(&gw->dropped);
	if (shed_vsyncs) *shed_vsyncs = atomic_load(&gw->shedVsyncs);
	if (committed) *committed = atomic_load(&gw->commits);
	if (vsyncs) *vsyncs = atomic_load(&gw->vsyncs);
}

void GoosieClose(GoosieWindow *gw) {
	if (!gw) return;
	stopDisplayLink(gw);
	pushQuit(gw);
}

char *GoosieClipboardRead(void) {
	__block char *result = NULL;
	void (^block)(void) = ^{
		@autoreleasepool {
			NSPasteboard *pb = [NSPasteboard generalPasteboard];
			NSString *s = [pb stringForType:NSPasteboardTypeString];
			if (s) {
				const char *utf8 = [s UTF8String];
				if (utf8) result = strdup(utf8);
			}
		}
	};
	if ([NSThread isMainThread]) { block(); } else { dispatch_sync(dispatch_get_main_queue(), block); }
	return result;
}

void GoosieClipboardWrite(const char *text) {
	void (^block)(void) = ^{
		@autoreleasepool {
			NSPasteboard *pb = [NSPasteboard generalPasteboard];
			[pb clearContents];
			if (text) {
				[pb setString:[NSString stringWithUTF8String:text]
				       forType:NSPasteboardTypeString];
			}
		}
	};
	if ([NSThread isMainThread]) { block(); } else { dispatch_sync(dispatch_get_main_queue(), block); }
}
