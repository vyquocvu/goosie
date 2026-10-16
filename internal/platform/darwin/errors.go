package darwin

import "errors"

// These are untagged on purpose. A caller on Linux gets ErrPlatformUnavailable and a
// caller on macOS off the main thread gets ErrNotMainThread, and both have to be able
// to errors.Is their way out of it; an error value that only exists in one build is an
// error value a portable caller cannot name.

var (
	// ErrPlatformUnavailable reports a build or a machine with no window shim. It is
	// the error stub.go returns, and the one platform_darwin.go maps to
	// platform.ErrBackendUnavailable.
	ErrPlatformUnavailable = errors.New("platform/darwin: no window shim in this build")
	// ErrNotMainThread reports a window opened from a thread AppKit will not accept.
	// It is worth its own error because the fix is in the caller's main function -
	// runtime.LockOSThread before anything can be opened - and not in the arguments.
	ErrNotMainThread = errors.New("platform/darwin: a window must be opened on the process's main thread")
	// ErrClosed reports a window that has been closed. The loop's event channel closes
	// before this can be seen from a present, so a caller that does see it is racing a
	// close it started itself.
	ErrClosed = errors.New("platform/darwin: window is closed")
	// ErrNoPixels reports a present with nothing to show.
	ErrNoPixels = errors.New("platform/darwin: present with a buffer that has no pixels")
	// ErrNoDisplay reports a machine this process cannot reach a window server from -
	// an ssh session, a headless CI macOS runner, or a detached daemon.
	ErrNoDisplay = errors.New("platform/darwin: no window server is reachable")
	// ErrNoDisplayLink reports a display but no way to be paced by it, which is a
	// window that would never redraw.
	ErrNoDisplayLink = errors.New("platform/darwin: the display cannot be paced")
)
