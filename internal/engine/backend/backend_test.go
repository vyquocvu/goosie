package backend

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// DefaultBackend — compile-time interface check
// ---------------------------------------------------------------------------

func TestDefaultBackendImplementsBackend(t *testing.T) {
	var b Backend = NewDefaultBackend()
	_ = b
}

// ---------------------------------------------------------------------------
// DefaultBackend — navigation always fails
// ---------------------------------------------------------------------------

func TestDefaultBackendNavigateFails(t *testing.T) {
	d := NewDefaultBackend()
	assert.ErrorIs(t, d.Navigate("https://example.com"), ErrNotSupported)
	assert.ErrorIs(t, d.Reload(), ErrNotSupported)
	assert.ErrorIs(t, d.GoBack(), ErrNotSupported)
	assert.ErrorIs(t, d.GoForward(), ErrNotSupported)
}

func TestDefaultBackendStopSucceeds(t *testing.T) {
	d := NewDefaultBackend()
	assert.NoError(t, d.Stop())
}

func TestDefaultBackendCanGoReturnsFalse(t *testing.T) {
	d := NewDefaultBackend()
	assert.False(t, d.CanGoBack())
	assert.False(t, d.CanGoForward())
}

// ---------------------------------------------------------------------------
// DefaultBackend — content methods
// ---------------------------------------------------------------------------

func TestDefaultBackendLoadHTMLFails(t *testing.T) {
	d := NewDefaultBackend()
	assert.ErrorIs(t, d.LoadHTML("<html></html>", "https://base"), ErrNotSupported)
}

func TestDefaultBackendEvaluateJSFails(t *testing.T) {
	d := NewDefaultBackend()
	result, err := d.EvaluateJS("1+1")
	assert.Empty(t, result)
	assert.ErrorIs(t, err, ErrNotSupported)
}

// ---------------------------------------------------------------------------
// DefaultBackend — profile
// ---------------------------------------------------------------------------

func TestDefaultBackendPrivateMode(t *testing.T) {
	d := NewDefaultBackend()
	assert.False(t, d.IsPrivateMode(), "should default to false")

	d.SetPrivateMode(true)
	assert.True(t, d.IsPrivateMode())

	d.SetPrivateMode(false)
	assert.False(t, d.IsPrivateMode())
}

// ---------------------------------------------------------------------------
// DefaultBackend — devtools
// ---------------------------------------------------------------------------

func TestDefaultBackendDevTools(t *testing.T) {
	d := NewDefaultBackend()
	assert.ErrorIs(t, d.ShowDevTools(), ErrNotSupported)
	assert.Empty(t, d.DevToolsURL())
}

// ---------------------------------------------------------------------------
// DefaultBackend — lifecycle
// ---------------------------------------------------------------------------

func TestDefaultBackendCloseSucceeds(t *testing.T) {
	d := NewDefaultBackend()
	assert.NoError(t, d.Close())
}

// ---------------------------------------------------------------------------
// DefaultBackend — callbacks
// ---------------------------------------------------------------------------

func TestDefaultBackendSetCallbacks(t *testing.T) {
	d := NewDefaultBackend()

	var called bool
	cb := Callbacks{
		OnNavigation: func(event NavEvent, url string) {
			called = true
		},
	}
	d.SetCallbacks(cb)
	assert.NotNil(t, d.cb.OnNavigation)

	// Calling the stored callback should set the flag.
	d.cb.OnNavigation(NavStarted, "https://example.com")
	assert.True(t, called)
}

// ---------------------------------------------------------------------------
// NavEvent String
// ---------------------------------------------------------------------------

func TestNavEventString(t *testing.T) {
	tests := []struct {
		e    NavEvent
		want string
	}{
		{NavStarted, "started"},
		{NavSucceeded, "succeeded"},
		{NavFailed, "failed"},
		{NavRedirected, "redirected"},
		{NavEvent(99), "unknown"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.e.String())
	}
}

// ---------------------------------------------------------------------------
// PermissionKind String
// ---------------------------------------------------------------------------

func TestPermissionKindString(t *testing.T) {
	tests := []struct {
		k    PermissionKind
		want string
	}{
		{PermissionUnknown, "unknown"},
		{PermissionGeolocation, "geolocation"},
		{PermissionNotifications, "notifications"},
		{PermissionMicrophone, "microphone"},
		{PermissionCamera, "camera"},
		{PermissionClipboard, "clipboard"},
		{PermissionDownloads, "downloads"},
		{PermissionAutoplay, "autoplay"},
		{PermissionPopups, "popups"},
		{PermissionFileSystem, "file-system"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.k.String())
	}
}

// ---------------------------------------------------------------------------
// ErrNotSupported
// ---------------------------------------------------------------------------

func TestErrNotSupported(t *testing.T) {
	assert.Equal(t, "backend not supported on this platform", ErrNotSupported.Error())
	assert.True(t, errors.Is(ErrNotSupported, ErrNotSupported))
	assert.True(t, errors.Is(errNotSupported{}, ErrNotSupported))

	err := someFuncThatReturnsNotSupported()
	assert.ErrorIs(t, err, ErrNotSupported)
}

func someFuncThatReturnsNotSupported() error {
	return ErrNotSupported
}

// ---------------------------------------------------------------------------
// DownloadInfo — struct sanity
// ---------------------------------------------------------------------------

func TestDownloadInfoDefaultValues(t *testing.T) {
	d := DownloadInfo{}
	assert.Empty(t, d.URL)
	assert.Empty(t, d.MIMEType)
	assert.Empty(t, d.SuggestedName)
	assert.Equal(t, int64(0), d.TotalBytes)
}

// ---------------------------------------------------------------------------
// PermissionRequest — struct sanity
// ---------------------------------------------------------------------------

func TestPermissionRequestDefaultValues(t *testing.T) {
	r := PermissionRequest{}
	assert.Equal(t, PermissionUnknown, r.Kind)
	assert.Empty(t, r.Origin)
}

// ---------------------------------------------------------------------------
// PermissionResponse — ensures int enum compiles without panics
// ---------------------------------------------------------------------------

func TestPermissionResponseValues(t *testing.T) {
	assert.Equal(t, 0, int(PermissionDeny))
	assert.Equal(t, 1, int(PermissionAllow))
	assert.Equal(t, 2, int(PermissionAllowAlways))
}
