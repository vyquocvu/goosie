package mcpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
	"github.com/vyquocvu/goosie/internal/mcpserver"
)

// --- Fake context for testing ---

type fakeBCContext struct {
	id       string
	state    browsercontrol.ContextState
	pageRev  int
	url      string
	title    string
	nodes    []browsercontrol.SemanticNode
	viewport browsercontrol.Viewport
	closed   bool
	mu       sync.Mutex
}

func (f *fakeBCContext) ID() string                                   { return f.id }
func (f *fakeBCContext) Info(ctx context.Context) (browsercontrol.ContextInfo, error) {
	return browsercontrol.ContextInfo{
		ID:           f.id,
		State:        f.state,
		PageRevision: f.pageRev,
		Viewport:     f.viewport,
	}, nil
}
func (f *fakeBCContext) Navigate(ctx context.Context, url string, wait browsercontrol.WaitCondition, timeoutMs int) (browsercontrol.NavigationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return browsercontrol.NavigationResult{}, browsercontrol.ErrContextNotFoundSentinel
	}
	f.state = browsercontrol.ContextComplete
	f.pageRev++
	f.url = url
	return browsercontrol.NavigationResult{
		ContextID:        f.id,
		NavigationID:     "nav_1",
		URL:              url,
		State:            f.state,
		WaitConditionMet: true,
		PageRevision:     f.pageRev,
		HTTPStatus:       200,
	}, nil
}
func (f *fakeBCContext) Wait(ctx context.Context, opts browsercontrol.WaitOptions) (browsercontrol.WaitResult, error) {
	return browsercontrol.WaitResult{}, nil
}
func (f *fakeBCContext) Snapshot(ctx context.Context, opts browsercontrol.SnapshotOptions) (browsercontrol.PageSnapshot, error) {
	return browsercontrol.PageSnapshot{
		ContextID:    f.id,
		PageRevision: f.pageRev,
		URL:          f.url,
		Title:        f.title,
		Nodes:        f.nodes,
	}, nil
}
func (f *fakeBCContext) Screenshot(ctx context.Context, opts browsercontrol.ScreenshotOptions) (browsercontrol.ScreenshotResult, error) {
	return browsercontrol.ScreenshotResult{
		ContextID:    f.id,
		PageRevision: f.pageRev,
		Width:        f.viewport.Width,
		Height:       f.viewport.Height,
		Data:         []byte("fake-png"),
		MIMEType:     "image/png",
	}, nil
}
func (f *fakeBCContext) Query(ctx context.Context, locator browsercontrol.Locator) (browsercontrol.QueryResult, error) {
	return browsercontrol.QueryResult{
		ContextID:    f.id,
		PageRevision: f.pageRev,
		Refs:         []browsercontrol.ElementRef{},
	}, nil
}
func (f *fakeBCContext) Click(ctx context.Context, ref browsercontrol.ElementRef, opts browsercontrol.ClickOptions) (browsercontrol.ActionResult, error) {
	return browsercontrol.ActionResult{ContextID: f.id, PageRevision: f.pageRev, ActionApplied: true}, nil
}
func (f *fakeBCContext) Type(ctx context.Context, ref browsercontrol.ElementRef, text string, opts browsercontrol.TypeOptions) (browsercontrol.ActionResult, error) {
	return browsercontrol.ActionResult{ContextID: f.id, PageRevision: f.pageRev, ActionApplied: true}, nil
}
func (f *fakeBCContext) PressKey(ctx context.Context, key string, modifiers []string) (browsercontrol.ActionResult, error) {
	return browsercontrol.ActionResult{}, nil
}
func (f *fakeBCContext) Scroll(ctx context.Context, opts browsercontrol.ScrollOptions) (browsercontrol.ActionResult, error) {
	return browsercontrol.ActionResult{}, nil
}
func (f *fakeBCContext) SetViewport(ctx context.Context, vp browsercontrol.Viewport) (browsercontrol.Viewport, error) {
	return vp, nil
}
func (f *fakeBCContext) Evaluate(ctx context.Context, source string, opts browsercontrol.EvaluateOptions) (browsercontrol.EvaluationResult, error) {
	return browsercontrol.EvaluationResult{}, nil
}
func (f *fakeBCContext) Console(ctx context.Context, cursor string, limit int) (browsercontrol.ConsolePage, error) {
	return browsercontrol.ConsolePage{}, nil
}
func (f *fakeBCContext) Network(ctx context.Context, cursor string, limit int) (browsercontrol.NetworkPage, error) {
	return browsercontrol.NetworkPage{}, nil
}
func (f *fakeBCContext) Security(ctx context.Context) (browsercontrol.SecuritySummary, error) {
	return browsercontrol.SecuritySummary{}, nil
}

// --- Fake service for testing ---

type fakeService struct {
	mu        sync.Mutex
	contexts  map[string]*fakeBCContext
	nextID    int
	maxCtx    int
}

func newFakeService() *fakeService {
	return &fakeService{
		contexts: make(map[string]*fakeBCContext),
		maxCtx:   browsercontrol.DefaultMaxContexts,
	}
}

func (s *fakeService) CreateContext(ctx context.Context, opts browsercontrol.CreateContextOptions) (browsercontrol.ContextInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.contexts) >= s.maxCtx {
		return browsercontrol.ContextInfo{}, browsercontrol.ErrLimitExceededSentinel
	}

	s.nextID++
	id := "ctx_test_" + string(rune('0'+s.nextID))
	if s.nextID > 9 {
		id = "ctx_test_" + string(rune('a'+s.nextID-10))
	}

	vp := opts.Viewport
	if vp.Width == 0 {
		vp.Width = 1280
	}
	if vp.Height == 0 {
		vp.Height = 720
	}
	if vp.Scale == 0 {
		vp.Scale = 1.0
	}

	c := &fakeBCContext{
		id:       id,
		state:    browsercontrol.ContextCreated,
		viewport: vp,
	}
	s.contexts[id] = c

	return browsercontrol.ContextInfo{
		ID:           id,
		State:        c.state,
		PageRevision: c.pageRev,
		Viewport:     c.viewport,
	}, nil
}

func (s *fakeService) ListContexts(ctx context.Context) ([]browsercontrol.ContextInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]browsercontrol.ContextInfo, 0, len(s.contexts))
	for _, c := range s.contexts {
		list = append(list, browsercontrol.ContextInfo{
			ID:           c.id,
			State:        c.state,
			PageRevision: c.pageRev,
		})
	}
	return list, nil
}

func (s *fakeService) CloseContext(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c, ok := s.contexts[id]; ok {
		c.closed = true
		delete(s.contexts, id)
	}
	return nil
}

func (s *fakeService) Context(ctx context.Context, id string) (browsercontrol.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c, ok := s.contexts[id]; ok {
		return c, nil
	}
	return nil, browsercontrol.ErrContextNotFoundSentinel
}

// --- Tests ---

func TestServer_NewServer(t *testing.T) {
	svc := newFakeService()
	server, err := mcpserver.NewServer(svc, mcpserver.ServerOptions{
		Name:    "test",
		Version: "0.0.1",
	})
	require.NoError(t, err)
	assert.NotNil(t, server)
}

func TestToolSchemas_GetSchemas(t *testing.T) {
	schemas := mcpserver.GetToolSchemas()
	assert.NotEmpty(t, schemas)
	assert.Len(t, schemas, 10) // We have 10 tools defined

	// Check that all expected tools are present
	names := make(map[string]bool)
	for _, s := range schemas {
		names[s.Name] = true
	}

	assert.True(t, names["browser_context_create"])
	assert.True(t, names["browser_context_list"])
	assert.True(t, names["browser_context_close"])
	assert.True(t, names["browser_navigate"])
	assert.True(t, names["browser_snapshot"])
	assert.True(t, names["browser_screenshot"])
	assert.True(t, names["browser_page_info"])
	assert.True(t, names["browser_query"])
	assert.True(t, names["browser_click"])
	assert.True(t, names["browser_type"])
}

func TestToolSchemas_GetSchema(t *testing.T) {
	schema, ok := mcpserver.GetToolSchema("browser_context_create")
	assert.True(t, ok)
	assert.Equal(t, "browser_context_create", schema.Name)
	assert.NotEmpty(t, schema.Description)
}

func TestToolSchemas_GetSchema_NotFound(t *testing.T) {
	_, ok := mcpserver.GetToolSchema("nonexistent_tool")
	assert.False(t, ok)
}

func TestFakeService_CreateContext(t *testing.T) {
	svc := newFakeService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{
		Viewport: browsercontrol.Viewport{Width: 1920, Height: 1080, Scale: 1},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, info.ID)
	assert.Equal(t, browsercontrol.ContextCreated, info.State)
	assert.Equal(t, 1920, info.Viewport.Width)
	assert.Equal(t, 1080, info.Viewport.Height)
}

func TestFakeService_ListContexts(t *testing.T) {
	svc := newFakeService()
	ctx := context.Background()

	_, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	_, err = svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	list, err := svc.ListContexts(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestFakeService_CloseContext(t *testing.T) {
	svc := newFakeService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	err = svc.CloseContext(ctx, info.ID)
	require.NoError(t, err)

	list, err := svc.ListContexts(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 0)
}

func TestFakeService_CloseContext_Idempotent(t *testing.T) {
	svc := newFakeService()
	ctx := context.Background()

	err := svc.CloseContext(ctx, "nonexistent")
	require.NoError(t, err) // Should not error
}

func TestFakeContext_Navigate(t *testing.T) {
	c := &fakeBCContext{
		id:       "test",
		viewport: browsercontrol.Viewport{Width: 1280, Height: 720},
	}

	result, err := c.Navigate(context.Background(), "https://example.com", browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", result.URL)
	assert.Equal(t, browsercontrol.ContextComplete, result.State)
	assert.Equal(t, 1, result.PageRevision)
}

func TestFakeContext_AfterNavigation(t *testing.T) {
	c := &fakeBCContext{
		id:       "test",
		viewport: browsercontrol.Viewport{Width: 1280, Height: 720},
	}

	c.Navigate(context.Background(), "https://example.com", browsercontrol.WaitComplete, 5000)

	info, err := c.Info(context.Background())
	require.NoError(t, err)
	assert.Equal(t, browsercontrol.ContextComplete, info.State)
}

func TestFakeContext_Snapshot(t *testing.T) {
	c := &fakeBCContext{
		id:       "test",
		url:      "https://example.com",
		title:    "Example",
		nodes:    []browsercontrol.SemanticNode{{Role: "heading", Name: "Test"}},
		viewport: browsercontrol.Viewport{Width: 1280, Height: 720},
	}

	snap, err := c.Snapshot(context.Background(), browsercontrol.SnapshotOptions{})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", snap.URL)
	assert.Equal(t, "Example", snap.Title)
	assert.Len(t, snap.Nodes, 1)
}

func TestFakeContext_Screenshot(t *testing.T) {
	c := &fakeBCContext{
		viewport: browsercontrol.Viewport{Width: 1280, Height: 720},
	}

	shot, err := c.Screenshot(context.Background(), browsercontrol.ScreenshotOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1280, shot.Width)
	assert.Equal(t, 720, shot.Height)
	assert.Equal(t, "image/png", shot.MIMEType)
	assert.NotEmpty(t, shot.Data)
}

func TestFakeContext_Click(t *testing.T) {
	c := &fakeBCContext{id: "test", pageRev: 1}

	result, err := c.Click(context.Background(), browsercontrol.ElementRef{
		Ref:          "e_1_1",
		ContextID:    "test",
		PageRevision: 1,
	}, browsercontrol.ClickOptions{})
	require.NoError(t, err)
	assert.True(t, result.ActionApplied)
}

func TestFakeContext_Type(t *testing.T) {
	c := &fakeBCContext{id: "test", pageRev: 1}

	result, err := c.Type(context.Background(), browsercontrol.ElementRef{
		Ref:          "e_1_1",
		ContextID:    "test",
		PageRevision: 1,
	}, "hello", browsercontrol.TypeOptions{})
	require.NoError(t, err)
	assert.True(t, result.ActionApplied)
}

func TestServerOptions_Defaults(t *testing.T) {
	opts := mcpserver.ServerOptions{}
	assert.Empty(t, opts.Name)
	assert.Empty(t, opts.Version)
}

func TestGetString(t *testing.T) {
	m := map[string]interface{}{
		"key": "value",
	}
	assert.Equal(t, "value", mcpserver.GetString(m, "key"))
	assert.Equal(t, "", mcpserver.GetString(m, "missing"))
}

func TestGetBool(t *testing.T) {
	m := map[string]interface{}{
		"true":  true,
		"false": false,
	}
	assert.True(t, mcpserver.GetBool(m, "true"))
	assert.False(t, mcpserver.GetBool(m, "false"))
	assert.False(t, mcpserver.GetBool(m, "missing"))
}

func TestParseInt(t *testing.T) {
	assert.Equal(t, 42, mcpserver.ParseInt(42.0))
	assert.Equal(t, 42, mcpserver.ParseInt("42"))
	assert.Equal(t, 0, mcpserver.ParseInt("not a number"))
}

func TestTruncateString(t *testing.T) {
	assert.Equal(t, "hello", mcpserver.TruncateString("hello", 10))
	assert.Equal(t, "hello...", mcpserver.TruncateString("hello world", 8))
}

func TestContains(t *testing.T) {
	ss := []string{"a", "b", "c"}
	assert.True(t, mcpserver.Contains(ss, "a"))
	assert.False(t, mcpserver.Contains(ss, "d"))
}

func TestNormalizeURL(t *testing.T) {
	assert.Equal(t, "https://example.com", mcpserver.NormalizeURL("example.com"))
	assert.Equal(t, "https://example.com", mcpserver.NormalizeURL("https://example.com"))
	assert.Equal(t, "http://example.com", mcpserver.NormalizeURL("http://example.com"))
}

// Test integration with httptest server
func TestFakeService_WithTestServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>Test</title></head><body><p>Hello</p></body></html>"))
	}))
	defer srv.Close()

	svc := newFakeService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	c, err := svc.Context(ctx, info.ID)
	require.NoError(t, err)

	result, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	assert.Equal(t, srv.URL, result.URL)
	assert.Equal(t, 200, result.HTTPStatus)
}

// Test JSON serialization of error responses
func TestErrorResponse_JSON(t *testing.T) {
	resp := mcpserver.NewErrorResponse(browsercontrol.ErrPageChanged, "reference outdated", true)
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Contains(t, parsed, "error")
	errObj := parsed["error"].(map[string]interface{})
	assert.Equal(t, "page_changed", errObj["code"])
	assert.Equal(t, "reference outdated", errObj["message"])
	assert.Equal(t, true, errObj["retryable"])
}
