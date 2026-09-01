package browsercontrol_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
)

// -- Slice A: Service Lifecycle --

func TestCreateContext_ReturnsUniquePrivateContext(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{
		Viewport: browsercontrol.Viewport{Width: 1280, Height: 720, Scale: 1},
	})

	require.NoError(t, err)
	assert.NotEmpty(t, info.ID)
	assert.Equal(t, browsercontrol.ContextCreated, info.State)
	assert.Equal(t, 0, info.PageRevision)
	assert.Equal(t, 1280, info.Viewport.Width)
	assert.Equal(t, 720, info.Viewport.Height)
	assert.Equal(t, 1.0, info.Viewport.Scale)

	// Second context must have a different ID
	info2, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	assert.NotEqual(t, info.ID, info2.ID)
}

func TestListContexts_IsConnectionScoped(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	_, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	_, err = s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	list, err := s.ListContexts(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestCloseContext_Idempotent(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	// First close succeeds
	err = s.CloseContext(ctx, info.ID)
	assert.NoError(t, err)

	// List should be empty
	list, err := s.ListContexts(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 0)

	// Second close is idempotent — no error
	err = s.CloseContext(ctx, info.ID)
	assert.NoError(t, err)
}

func TestOperationAfterClose_ReturnsContextNotFound(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)

	s.CloseContext(ctx, info.ID)

	// Try to navigate on closed context
	_, err = c.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 5000)
	require.Error(t, err)

	var be *browsercontrol.Error
	assert.ErrorAs(t, err, &be)
	assert.Equal(t, browsercontrol.ErrContextNotFound, be.Code)
}

func TestParentCancellation_ClosesChildWork(t *testing.T) {
	s := browsercontrol.NewFakeService()

	parentCtx, cancel := context.WithCancel(context.Background())
	info, err := s.CreateContext(parentCtx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)

	// Cancel parent
	cancel()

	// Subsequent operations should respect the parent cancellation
	_, err = c.Navigate(parentCtx, "https://example.com", browsercontrol.WaitComplete, 5000)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestMaximumContextQuota_Fails(t *testing.T) {
	s := browsercontrol.NewFakeService()
	s.SetMaxContexts(2)

	ctx := context.Background()
	_, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	_, err = s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	// Third create should fail
	_, err = s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.Error(t, err)
	var be *browsercontrol.Error
	assert.ErrorAs(t, err, &be)
	assert.Equal(t, browsercontrol.ErrLimitExceeded, be.Code)
}

func TestConcurrentCreateClose_RaceClean(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	var wg sync.WaitGroup

	createFn := func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{
				Viewport: browsercontrol.Viewport{Width: 1024, Height: 768, Scale: 1},
			})
			if err == nil {
				s.CloseContext(ctx, info.ID)
			}
		}
	}

	wg.Add(3)
	go createFn()
	go createFn()
	go createFn()
	wg.Wait()

	list, err := s.ListContexts(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 0)
}

// -- Slice A: Context Contract Tests --

func TestNavigate_ReturnsStateAndRevision(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{
		Viewport: browsercontrol.Viewport{Width: 1280, Height: 720},
	})
	require.NoError(t, err)

	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)

	nav, err := c.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	assert.Equal(t, info.ID, nav.ContextID)
	assert.NotEmpty(t, nav.NavigationID)
	assert.Equal(t, "https://example.com", nav.URL)
	assert.Equal(t, browsercontrol.ContextComplete, nav.State)
	assert.True(t, nav.WaitConditionMet)
	assert.Equal(t, 1, nav.PageRevision)
	assert.Equal(t, 200, nav.HTTPStatus)
}

func TestSnapshot_ReturnsSemanticTree(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)

	c.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 5000)

	snap, err := c.Snapshot(ctx, browsercontrol.SnapshotOptions{Format: browsercontrol.SnapshotSemantic})
	require.NoError(t, err)
	assert.Equal(t, info.ID, snap.ContextID)
	assert.Equal(t, 1, snap.PageRevision)
	assert.NotEmpty(t, snap.URL)
	assert.NotEmpty(t, snap.Title)
	assert.NotEmpty(t, snap.Nodes)
}

func TestQuery_LocatorReturnsRefs(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)

	c.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 5000)

	qr, err := c.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "link", Exact: true},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, qr.Refs)
	for _, ref := range qr.Refs {
		assert.Equal(t, info.ID, ref.ContextID)
		assert.Equal(t, 1, ref.PageRevision)
		assert.NotEmpty(t, ref.Ref)
	}
}

func TestEvaluate_ReturnsPrimitiveResult(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)

	c.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 5000)

	result, err := c.Evaluate(ctx, "document.title", browsercontrol.EvaluateOptions{
		AwaitPromise:   true,
		TimeoutMs:      1000,
		MaxResultBytes: 65536,
	})
	require.NoError(t, err)
	assert.Equal(t, info.ID, result.ContextID)
	assert.NotEmpty(t, result.Type)
}

func TestScreenshot_ReturnsImageData(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{
		Viewport: browsercontrol.Viewport{Width: 1280, Height: 720, Scale: 1},
	})
	require.NoError(t, err)

	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)

	c.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 5000)

	shot, err := c.Screenshot(ctx, browsercontrol.ScreenshotOptions{Scope: "viewport"})
	require.NoError(t, err)
	assert.Equal(t, info.ID, shot.ContextID)
	assert.Equal(t, 1280, shot.Width)
	assert.Equal(t, 720, shot.Height)
	assert.Equal(t, "image/png", shot.MIMEType)
	assert.NotEmpty(t, shot.Data)
}

func TestConsole_ReturnsBoundedEntries(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)

	c.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 5000)

	page, err := c.Console(ctx, "", 10)
	require.NoError(t, err)
	assert.Equal(t, info.ID, page.ContextID)
	// Console may or may not have entries, but the call should succeed
}

func TestSecurity_ReturnsSummary(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)

	c.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 5000)

	sec, err := c.Security(ctx)
	require.NoError(t, err)
	assert.Equal(t, info.ID, sec.ContextID)
}

func TestError_TypedCodes(t *testing.T) {
	tests := []struct {
		err       *browsercontrol.Error
		wantCode  browsercontrol.ErrorCode
		wantMsg   string
		retryable bool
	}{
		{browsercontrol.ErrContextNotFoundSentinel, browsercontrol.ErrContextNotFound, "context not found or closed", true},
		{browsercontrol.ErrPageChangedSentinel, browsercontrol.ErrPageChanged, "belongs to an earlier page revision", true},
		{browsercontrol.ErrElementNotFoundSentinel, browsercontrol.ErrElementNotFound, "cannot be resolved", true},
		{browsercontrol.ErrAmbiguousTargetSentinel, browsercontrol.ErrAmbiguousTarget, "matched multiple elements", true},
		{browsercontrol.ErrInvalidStateSentinel, browsercontrol.ErrInvalidState, "current lifecycle state", true},
		{browsercontrol.ErrPolicyDeniedSentinel, browsercontrol.ErrPolicyDenied, "security policy rejected", false},
		{browsercontrol.ErrDeadlineExceededSentinel, browsercontrol.ErrDeadlineExceeded, "timed out", true},
		{browsercontrol.ErrCancelledSentinel, browsercontrol.ErrCancelled, "was cancelled", true},
		{browsercontrol.ErrLimitExceededSentinel, browsercontrol.ErrLimitExceeded, "quota or limit exceeded", false},
		{browsercontrol.ErrUnsupportedSentinel, browsercontrol.ErrUnsupported, "not supported", false},
		{browsercontrol.ErrInternalSentinel, browsercontrol.ErrInternal, "internal server error", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.wantCode), func(t *testing.T) {
			assert.Contains(t, tt.err.Error(), tt.wantMsg)
			assert.Equal(t, tt.retryable, tt.err.Retryable)
		})
	}
}

func TestNewError_WithDetails(t *testing.T) {
	err := browsercontrol.NewError(browsercontrol.ErrPageChanged, "custom message", true, map[string]interface{}{
		"currentPageRevision": 2,
	})
	assert.Equal(t, browsercontrol.ErrPageChanged, err.Code)
	assert.Equal(t, "custom message", err.Message)
	assert.True(t, err.Retryable)
	assert.Equal(t, 2, err.Details["currentPageRevision"])
	assert.Contains(t, err.Error(), "custom message")
}

func TestWait_Timeout(t *testing.T) {
	s := browsercontrol.NewFakeService()

	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)

	c.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 5000)

	result, err := c.Wait(ctx, browsercontrol.WaitOptions{
		Condition: browsercontrol.WaitComplete,
		TimeoutMs: 100,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.URL)
	assert.NotEqual(t, 0, result.PageRevision)
}

func TestCloseContext_UnknownID_NoError(t *testing.T) {
	s := browsercontrol.NewFakeService()

	err := s.CloseContext(context.Background(), "nonexistent-id")
	assert.NoError(t, err)
}

func TestService_ImplementsInterface(t *testing.T) {
	var s browsercontrol.Service = browsercontrol.NewFakeService()
	_, err := s.CreateContext(context.Background(), browsercontrol.CreateContextOptions{})
	assert.NoError(t, err)
	_ = s
}

func TestSnapshot_DefaultOptions(t *testing.T) {
	s := browsercontrol.NewFakeService()
	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)
	c.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 5000)

	// Default options should work
	snap, err := c.Snapshot(ctx, browsercontrol.SnapshotOptions{})
	require.NoError(t, err)
	assert.NotNil(t, snap)
}

func TestContextID_NotEmpty(t *testing.T) {
	s := browsercontrol.NewFakeService()
	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)
	assert.Equal(t, info.ID, c.ID())
	assert.NotEmpty(t, c.ID())
}

func TestClick_ReturnsActionResult(t *testing.T) {
	s := browsercontrol.NewFakeService()
	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	c, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)
	c.Navigate(ctx, "https://example.com", browsercontrol.WaitComplete, 5000)

	qr, err := c.Query(ctx, browsercontrol.Locator{Role: &browsercontrol.RoleLocator{Name: "link", Exact: false}})
	require.NoError(t, err)
	if len(qr.Refs) > 0 {
		result, err := c.Click(ctx, qr.Refs[0], browsercontrol.ClickOptions{Button: "left", TimeoutMs: 5000})
		require.NoError(t, err)
		assert.True(t, result.ActionApplied)
		assert.Equal(t, info.ID, result.ContextID)
	}
}

func TestViewport_ValidRanges(t *testing.T) {
	vp := browsercontrol.Viewport{Width: 1920, Height: 1080, Scale: 1.5}
	assert.Equal(t, 1920, vp.Width)
	assert.Equal(t, 1080, vp.Height)
	assert.Equal(t, 1.5, vp.Scale)
}
