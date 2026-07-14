// Package backend defines the platform WebView interface for compatibility
// fallback. When the pure-Go engine cannot handle a page (detected by
// fallback.Policy), the engine delegates navigation, rendering, and user
// interaction to a platform-specific Backend implementation.
//
// M12.2: Each platform (macOS WKWebView, Windows WebView2, etc.)
// provides its own Backend. The interface is designed to map closely
// to both WKWebView and WebView2 APIs.
package backend

// ---------------------------------------------------------------------------
// Navigation types
// ---------------------------------------------------------------------------

// NavEvent describes the current state of a navigation.
type NavEvent int

const (
	NavStarted    NavEvent = iota // page load has begun
	NavSucceeded                  // page load completed successfully
	NavFailed                     // page load failed with an error
	NavRedirected                 // navigation was redirected
)

func (e NavEvent) String() string {
	switch e {
	case NavStarted:
		return "started"
	case NavSucceeded:
		return "succeeded"
	case NavFailed:
		return "failed"
	case NavRedirected:
		return "redirected"
	default:
		return "unknown"
	}
}

// DownloadInfo describes a file download initiated by the page.
type DownloadInfo struct {
	URL           string // download source URL
	MIMEType      string // content MIME type
	SuggestedName string // filename suggested by the server
	TotalBytes    int64  // expected size, or -1 if unknown
}

// PermissionKind identifies the type of permission being requested.
type PermissionKind int

const (
	PermissionUnknown PermissionKind = iota
	PermissionGeolocation
	PermissionNotifications
	PermissionMicrophone
	PermissionCamera
	PermissionClipboard
	PermissionDownloads
	PermissionAutoplay
	PermissionPopups
	PermissionFileSystem
)

func (k PermissionKind) String() string {
	switch k {
	case PermissionGeolocation:
		return "geolocation"
	case PermissionNotifications:
		return "notifications"
	case PermissionMicrophone:
		return "microphone"
	case PermissionCamera:
		return "camera"
	case PermissionClipboard:
		return "clipboard"
	case PermissionDownloads:
		return "downloads"
	case PermissionAutoplay:
		return "autoplay"
	case PermissionPopups:
		return "popups"
	case PermissionFileSystem:
		return "file-system"
	default:
		return "unknown"
	}
}

// PermissionRequest represents a page-originated permission prompt.
type PermissionRequest struct {
	Kind   PermissionKind
	Origin string // requesting origin, e.g. "https://example.com"
}

// PermissionResponse is the user's decision for a permission request.
type PermissionResponse int

const (
	PermissionDeny PermissionResponse = iota
	PermissionAllow
	PermissionAllowAlways
)

// ---------------------------------------------------------------------------
// Callbacks
// ---------------------------------------------------------------------------

// Callbacks groups the observer functions a Backend implementation calls
// to report page state changes back to the engine.
type Callbacks struct {
	// OnNavigation is called when a navigation starts, succeeds, fails, or
	// is redirected. The URL may be a final or intermediate target.
	OnNavigation func(NavEvent, string)

	// OnTitleChanged is called when the page title changes.
	OnTitleChanged func(title string)

	// OnURLChanged is called when the page URL changes (e.g. after a
	// client-side pushState or hash change).
	OnURLChanged func(url string)

	// OnDownload is called when the page initiates a file download.
	// The caller should use the returned chan to signal the decision:
	// send a file path to accept, or close the channel to cancel.
	OnDownload func(DownloadInfo) (accept <-chan string)

	// OnPermissionRequested is called when the page requests a permission.
	// The caller should send a PermissionResponse on the returned channel.
	OnPermissionRequested func(PermissionRequest) (decision <-chan PermissionResponse)

	// OnLoadingChanged is called when the page loading state changes.
	OnLoadingChanged func(loading bool)

	// OnPageCrashed is called when the WebView process crashes or becomes
	// unresponsive. Returns true if the caller wants to reload the page.
	OnPageCrashed func() (reload bool)
}

// ---------------------------------------------------------------------------
// Backend interface
// ---------------------------------------------------------------------------

// Backend is the interface that platform-specific WebViews must implement.
// It replaces the pure-Go render engine for pages that trigger the
// compatibility fallback policy.
type Backend interface {
	// -- Navigation --------------------------------------------------------
	Navigate(url string) error
	Stop() error
	Reload() error
	GoBack() error
	GoForward() error
	CanGoBack() bool
	CanGoForward() bool

	// -- Content ----------------------------------------------------------
	// LoadHTML replaces the current page content with the given HTML string
	// and base URL. Used for local pages or error pages.
	LoadHTML(html string, baseURL string) error

	// EvaluateJS executes JavaScript in the page context and returns the
	// result as a string (JSON-encoded for non-primitive values).
	EvaluateJS(script string) (string, error)

	// -- Profile / Privacy ------------------------------------------------
	SetPrivateMode(private bool)
	IsPrivateMode() bool

	// -- Developer Tools --------------------------------------------------
	ShowDevTools() error
	DevToolsURL() string

	// -- Lifecycle --------------------------------------------------------
	Close() error

	// -- Observability ----------------------------------------------------
	// SetCallbacks registers the observer functions the backend should call
	// for page state changes. Must be called before Navigate.
	SetCallbacks(Callbacks)
}

// ---------------------------------------------------------------------------
// DefaultBackend (no-op stub)
// ---------------------------------------------------------------------------

// DefaultBackend is a no-op stub for platforms without WebView support.
// All navigation methods return ErrNotSupported. It implements Backend
// so callers can safely check for fallback availability at compile time.
type DefaultBackend struct {
	private bool
	cb      Callbacks
}

// Ensure DefaultBackend implements Backend.
var _ Backend = (*DefaultBackend)(nil)

// ErrNotSupported is returned when the platform does not provide a WebView.
var ErrNotSupported = errNotSupported{}

type errNotSupported struct{}

func (errNotSupported) Error() string { return "backend not supported on this platform" }

func (e errNotSupported) Is(target error) bool {
	_, ok := target.(errNotSupported)
	return ok
}

// NewDefaultBackend returns a no-op DefaultBackend.
func NewDefaultBackend() *DefaultBackend {
	return &DefaultBackend{}
}

// Navigate returns ErrNotSupported.
func (d *DefaultBackend) Navigate(string) error { return ErrNotSupported }

// Stop is a no-op.
func (d *DefaultBackend) Stop() error { return nil }

// Reload returns ErrNotSupported.
func (d *DefaultBackend) Reload() error { return ErrNotSupported }

// GoBack returns ErrNotSupported.
func (d *DefaultBackend) GoBack() error { return ErrNotSupported }

// GoForward returns ErrNotSupported.
func (d *DefaultBackend) GoForward() error { return ErrNotSupported }

// CanGoBack returns false.
func (d *DefaultBackend) CanGoBack() bool { return false }

// CanGoForward returns false.
func (d *DefaultBackend) CanGoForward() bool { return false }

// LoadHTML returns ErrNotSupported.
func (d *DefaultBackend) LoadHTML(string, string) error { return ErrNotSupported }

// EvaluateJS returns an empty string.
func (d *DefaultBackend) EvaluateJS(string) (string, error) { return "", ErrNotSupported }

// SetPrivateMode stores the private flag.
func (d *DefaultBackend) SetPrivateMode(v bool) { d.private = v }

// IsPrivateMode returns the stored private flag.
func (d *DefaultBackend) IsPrivateMode() bool { return d.private }

// ShowDevTools returns ErrNotSupported.
func (d *DefaultBackend) ShowDevTools() error { return ErrNotSupported }

// DevToolsURL returns an empty string.
func (d *DefaultBackend) DevToolsURL() string { return "" }

// Close is a no-op.
func (d *DefaultBackend) Close() error { return nil }

// SetCallbacks stores the callbacks for later invocation.
func (d *DefaultBackend) SetCallbacks(cb Callbacks) { d.cb = cb }
