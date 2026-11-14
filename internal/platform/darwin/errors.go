package darwin

import "errors"

var (
	ErrPlatformUnavailable = errors.New("platform/darwin: no window shim in this build")
	ErrNotMainThread       = errors.New("platform/darwin: a window must be opened on the process's main thread")
	ErrClosed              = errors.New("platform/darwin: window is closed")
	ErrNoPixels            = errors.New("platform/darwin: present with a buffer that has no pixels")
	ErrNoDisplay           = errors.New("platform/darwin: no window server is reachable")
	ErrNoDisplayLink       = errors.New("platform/darwin: the display cannot be paced")
	ErrAppQuitting         = errors.New("platform/darwin: the app is quitting; no new window can be opened")
)
