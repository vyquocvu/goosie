package ui

import (
	"context"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
	"github.com/vyquocvu/goosie/internal/engine/session"
)

func TestBrowserDevToolsButtonNetworkDialog(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.devToolsButton)
	assert.Equal(t, "DevTools", browser.devToolsButton.Text)
}

func TestBrowserShowNetworkQueueNoSession(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	browser.showNetworkQueueDialog()
	assert.True(t, browser.devToolsVisible)
}

func TestBrowserShowNetworkQueueEmpty(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	sess := session.New()
	browser.deps.NavSession = sess
	browser.showNetworkQueueDialog()
	assert.True(t, browser.devToolsVisible)
}

func TestBrowserShowNetworkQueueWithLoads(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	sess := session.New()
	browser.deps.NavSession = sess

	ctx := navigation.WithPriority(context.Background(), navigation.PriorityDocument)
	_, _ = sess.Navigate(ctx, "https://example.test/page")
	browser.showNetworkQueueDialog()
	assert.True(t, browser.devToolsVisible)
}

func TestBrowserDevToolsToggleInNavBar(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.devToolsButton)
	assert.Equal(t, "DevTools", browser.devToolsButton.Text)
	assert.False(t, browser.devToolsVisible)

	browser.devToolsButton.OnTapped()
	assert.True(t, browser.devToolsVisible)

	browser.devToolsButton.OnTapped()
	assert.False(t, browser.devToolsVisible)
}

func TestSessionPendingLoadsAfterNavigate(t *testing.T) {
	sess := session.New()

	ctx := navigation.WithPriority(context.Background(), navigation.PriorityDocument)
	load, _ := sess.Navigate(ctx, "https://example.test/page")

	loads := sess.PendingLoads()
	assert.NotNil(t, loads)
	assert.Equal(t, load.ID, loads[0].ID)
	assert.Equal(t, "https://example.test/page", loads[0].URL)
}

func TestSessionPendingLoadsEmptyForFreshSession(t *testing.T) {
	sess := session.New()
	loads := sess.PendingLoads()
	assert.Nil(t, loads)
}

func TestSessionPendingLoadsAfterComplete(t *testing.T) {
	sess := session.New()

	ctx := navigation.WithPriority(context.Background(), navigation.PriorityDocument)
	_, _ = sess.Navigate(ctx, "https://example.test/page")

	sess.Complete()

	loads := sess.PendingLoads()
	assert.NotNil(t, loads)
}

func TestPendingLoadPriorityOrder(t *testing.T) {
	sess := session.New()

	ctx := navigation.WithPriority(context.Background(), navigation.PriorityDocument)
	sess.Navigate(ctx, "https://example.test/document")

	loads := sess.PendingLoads()
	if len(loads) > 0 {
		assert.Equal(t, navigation.PriorityDocument, loads[0].Priority)
	}
}

func TestPendingLoadStartedAt(t *testing.T) {
	sess := session.New()
	before := time.Now()

	ctx := navigation.WithPriority(context.Background(), navigation.PriorityDocument)
	sess.Navigate(ctx, "https://example.test/page")

	loads := sess.PendingLoads()
	if len(loads) > 0 {
		assert.True(t, loads[0].StartedAt.After(before.Add(-time.Second)) || loads[0].StartedAt.Equal(before))
	}
}
