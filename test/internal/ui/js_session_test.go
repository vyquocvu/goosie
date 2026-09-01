package ui_test

import (
	"sync/atomic"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/ui"
)

// TestTabJSSessionLifecycle covers the tab's single-owner JS session:
// SetJSRuntime wraps the runtime in a session, RunScriptOnOwner /
// SubmitOnOwner execute on the session owner, and CloseJSSession
// rejects further work with ErrSessionClosed.
func TestTabJSSessionLifecycle(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	tab := browser.NewTab()
	defer tab.CloseJSSession()

	// No session attached yet — owner helpers reject.
	_, err := tab.RunScriptOnOwner("1 + 1")
	assert.ErrorIs(t, err, js.ErrSessionClosed)
	assert.ErrorIs(t, tab.SubmitOnOwner(func(r *js.Runtime) {}), js.ErrSessionClosed)

	rt := js.NewRuntime()
	tab.SetJSRuntime(rt)
	assert.Same(t, rt, tab.GetJSRuntime())

	// Eval routes through the owner goroutine and returns its value.
	v, err := tab.RunScriptOnOwner("6 * 7")
	require.NoError(t, err)
	assert.Equal(t, "42", v.String())

	// SubmitOnOwner runs configuration on the owner goroutine.
	var ran atomic.Bool
	require.NoError(t, tab.SubmitOnOwner(func(r *js.Runtime) {
		ran.Store(true)
	}))
	assert.True(t, ran.Load())

	// Closing the session rejects queued work predictably.
	tab.CloseJSSession()
	_, err = tab.RunScriptOnOwner("1")
	assert.ErrorIs(t, err, js.ErrSessionClosed)
	assert.ErrorIs(t, tab.SubmitOnOwner(func(r *js.Runtime) {}), js.ErrSessionClosed)
}

// TestTabSetJSRuntimeNilDetaches closes an active session when a nil
// runtime is attached, mirroring navigation teardown.
func TestTabSetJSRuntimeNilDetaches(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w)

	tab := browser.NewTab()
	tab.SetJSRuntime(js.NewRuntime())
	require.NotNil(t, tab.GetJSRuntime())

	tab.SetJSRuntime(nil)
	assert.Nil(t, tab.GetJSRuntime())
	_, err := tab.RunScriptOnOwner("1")
	assert.ErrorIs(t, err, js.ErrSessionClosed)
}
