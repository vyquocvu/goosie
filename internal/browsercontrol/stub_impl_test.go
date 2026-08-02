package browsercontrol

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/net/html"
)

// TestScreenshot_BasicRender tests that Screenshot produces valid PNG bytes
// even with no navigation. The renderer fallback should still produce an image.
func TestScreenshot_BasicRender(t *testing.T) {
	svc := NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, CreateContextOptions{
		Viewport: Viewport{Width: 320, Height: 240},
	})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Screenshot(ctx, ScreenshotOptions{})
	require.NoError(t, err)

	assert.Equal(t, info.ID, result.ContextID)
	assert.Equal(t, "image/png", result.MIMEType)
	assert.NotEmpty(t, result.Data, "screenshot should produce non-empty PNG bytes")
	assert.True(t, result.Width > 0)
	assert.True(t, result.Height > 0)
	assert.LessOrEqual(t, len(result.Data), MaxScreenshotEncoded)
}

// TestScreenshot_HugeViewport tests that large viewports get scaled down.
func TestScreenshot_HugeViewport(t *testing.T) {
	svc := NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, CreateContextOptions{
		Viewport: Viewport{Width: 10000, Height: 10000}, // 100MP
	})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	result, err := ec.Screenshot(ctx, ScreenshotOptions{})
	require.NoError(t, err)

	// Should be scaled down below MaxScreenshotPixels (16MP)
	assert.Less(t, result.Width*result.Height, MaxScreenshotPixels+1)
}

// TestScreenshot_AfterNavigate tests that screenshot uses the loaded page DOM.
func TestScreenshot_AfterNavigate(t *testing.T) {
	svc := NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, CreateContextOptions{
		Viewport: Viewport{Width: 800, Height: 600},
	})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	// Inject a parsed document (simulating a successful navigation).
	// The test exercises the real engine's lastDoc, which lives
	// on the unexported *engineContext. Cast through the public
	// Context interface to the concrete type so we can drive
	// the internal state.
	doc, err := html.Parse(strings.NewReader("<html><body><h1>Hello</h1></body></html>"))
	require.NoError(t, err)
	ecImpl, ok := ec.(*engineContext)
	require.True(t, ok, "expected *engineContext, got %T", ec)
	ecImpl.mu.Lock()
	ecImpl.lastDoc = doc
	ecImpl.mu.Unlock()

	result, err := ec.Screenshot(ctx, ScreenshotOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Data)
}

// TestClick_NotFoundWithoutDoc tests click when no ref matches.
func TestClick_NotFoundWithoutDoc(t *testing.T) {
	svc := NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	ref := ElementRef{Ref: "no-such-ref", ContextID: info.ID, PageRevision: 0}
	_, err = ec.Click(ctx, ref, ClickOptions{})
	assert.Error(t, err)
}

// TestStubClick_WrongContext verifies ref from another context is rejected.
func TestStubClick_WrongContext(t *testing.T) {
	svc := NewEngineService()
	ctx := context.Background()

	infoA, _ := svc.CreateContext(ctx, CreateContextOptions{})
	infoB, _ := svc.CreateContext(ctx, CreateContextOptions{})

	ecA, _ := svc.Context(context.Background(), infoA.ID)
	ref := ElementRef{Ref: "e1", ContextID: infoB.ID, PageRevision: 0}

	_, err := ecA.Click(ctx, ref, ClickOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different context")
}

// TestStubClick_RevisionMismatch verifies stale refs are rejected.
func TestStubClick_RevisionMismatch(t *testing.T) {
	svc := NewEngineService()
	ctx := context.Background()

	info, _ := svc.CreateContext(ctx, CreateContextOptions{})
	ec, _ := svc.Context(context.Background(), info.ID)

	ref := ElementRef{Ref: "e1", ContextID: info.ID, PageRevision: 999}
	_, err := ec.Click(ctx, ref, ClickOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "page_changed")
}

// TestClick_Cancelled verifies context cancellation propagates.
func TestClick_Cancelled(t *testing.T) {
	svc := NewEngineService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// A cancelled context means CreateContext fails — verify that
	// and confirm the error is observable.
	_, err := svc.CreateContext(ctx, CreateContextOptions{})
	require.Error(t, err, "CreateContext with a cancelled parent must fail")

	// For the second half of the test (operation on a live
	// context with a cancelled call), use a fresh context.
	fresh, freshCancel := context.WithCancel(context.Background())
	defer freshCancel()
	info, err := svc.CreateContext(fresh, CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)

	// Now cancel the per-call context and verify Click returns an
	// error rather than panicking.
	callCtx, callCancel := context.WithCancel(context.Background())
	callCancel()
	ref := ElementRef{Ref: "e1", ContextID: info.ID, PageRevision: 0}
	_, err = ec.Click(callCtx, ref, ClickOptions{})
	require.Error(t, err)
}

// TestType_WrongContext verifies Type ref must be from same context.
func TestType_WrongContext(t *testing.T) {
	svc := NewEngineService()
	ctx := context.Background()

	infoA, _ := svc.CreateContext(ctx, CreateContextOptions{})
	infoB, _ := svc.CreateContext(ctx, CreateContextOptions{})

	ecA, _ := svc.Context(context.Background(), infoA.ID)
	ref := ElementRef{Ref: "i1", ContextID: infoB.ID, PageRevision: 0}
	_, err := ecA.Type(ctx, ref, "hello", TypeOptions{})
	require.Error(t, err)
}

// TestStubType_RevisionMismatch verifies stale refs are rejected for Type.
func TestStubType_RevisionMismatch(t *testing.T) {
	svc := NewEngineService()
	ctx := context.Background()
	info, _ := svc.CreateContext(ctx, CreateContextOptions{})
	ec, _ := svc.Context(context.Background(), info.ID)

	ref := ElementRef{Ref: "i1", ContextID: info.ID, PageRevision: 999}
	_, err := ec.Type(ctx, ref, "hello", TypeOptions{})
	require.Error(t, err)
}

// TestType_Cancelled verifies cancellation.
func TestType_Cancelled(t *testing.T) {
	svc := NewEngineService()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// CreateContext with a cancelled parent must fail.
	_, err := svc.CreateContext(ctx, CreateContextOptions{})
	require.Error(t, err)

	// For the per-call cancellation, use a fresh context to create
	// the live context, then cancel the call's context.
	fresh, freshCancel := context.WithCancel(context.Background())
	defer freshCancel()
	info, err := svc.CreateContext(fresh, CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(context.Background(), info.ID)
	require.NoError(t, err)
	callCtx, callCancel := context.WithCancel(context.Background())
	callCancel()
	ref := ElementRef{Ref: "i1", ContextID: info.ID, PageRevision: 0}
	_, err = ec.Type(callCtx, ref, "x", TypeOptions{})
	require.Error(t, err)
}
