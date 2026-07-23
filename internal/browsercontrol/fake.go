package browsercontrol

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// FakeService is a programmable fake implementation of Service for testing.
// It records calls, blocks until cancellation, emits typed errors, and
// returns configurable fixtures.
type FakeService struct {
	mu           sync.Mutex
	maxContexts  int
	contexts     map[string]*fakeContext
	nextID       int64
	nextNavID    int64
	closed       chan struct{}
}

type fakeContext struct {
	id          string
	state       ContextState
	pageRev     int
	viewport    Viewport
	url         string
	title       string
	createdAt   time.Time
	nodes       []SemanticNode
	console     []ConsoleEntry
	network     []NetworkEntry
	closed      bool
	mu          sync.Mutex
}

var _ Service = (*FakeService)(nil)

// NewFakeService creates a new programmable fake service.
func NewFakeService() *FakeService {
	return &FakeService{
		maxContexts: DefaultMaxContexts,
		contexts:    make(map[string]*fakeContext),
		closed:      make(chan struct{}),
	}
}

// SetMaxContexts overrides the default context limit.
func (s *FakeService) SetMaxContexts(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxContexts = n
}

// Context returns the fake context by ID (for test assertions).
func (s *FakeService) Context(id string) (*fakeContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.contexts[id]
	if !ok {
		return nil, NewError(ErrContextNotFound, "context not found", true, nil)
	}
	return c, nil
}

func generateID(prefix string) string {
	b := make([]byte, 16)
	rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func (s *FakeService) CreateContext(ctx context.Context, opts CreateContextOptions) (ContextInfo, error) {
	select {
	case <-ctx.Done():
		return ContextInfo{}, ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.contexts) >= s.maxContexts {
		return ContextInfo{}, NewError(ErrLimitExceeded,
			fmt.Sprintf("maximum of %d contexts reached", s.maxContexts), false, nil)
	}

	id := generateID("ctx")
	c := &fakeContext{
		id:        id,
		state:     ContextCreated,
		viewport:  opts.Viewport,
		createdAt: time.Now(),
	}
	if c.viewport.Width == 0 {
		c.viewport.Width = 1280
	}
	if c.viewport.Height == 0 {
		c.viewport.Height = 720
	}
	if c.viewport.Scale == 0 {
		c.viewport.Scale = 1.0
	}
	s.contexts[id] = c

	return ContextInfo{
		ID:           id,
		State:        ContextCreated,
		PageRevision: 0,
		Viewport:     c.viewport,
		CreatedAt:    c.createdAt,
	}, nil
}

func (s *FakeService) ListContexts(ctx context.Context) ([]ContextInfo, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]ContextInfo, 0, len(s.contexts))
	for _, c := range s.contexts {
		list = append(list, c.info())
	}
	return list, nil
}

func (s *FakeService) CloseContext(ctx context.Context, id string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.contexts[id]
	if !ok {
		return nil // idempotent
	}
	c.mu.Lock()
	c.closed = true
	c.state = ContextClosed
	c.mu.Unlock()
	delete(s.contexts, id)
	return nil
}

// fakeContext implements the Context interface.
var _ Context = (*fakeContext)(nil)

func (c *fakeContext) ID() string {
	return c.id
}

func (c *fakeContext) Info(ctx context.Context) (ContextInfo, error) {
	if err := c.checkAlive(ctx); err != nil {
		return ContextInfo{}, err
	}
	return c.info(), nil
}

func (c *fakeContext) checkAlive(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrContextNotFoundSentinel
	}
	return nil
}

func (c *fakeContext) info() ContextInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return ContextInfo{
		ID:           c.id,
		State:        c.state,
		PageRevision: c.pageRev,
		Viewport:     c.viewport,
		CreatedAt:    c.createdAt,
	}
}

func (c *fakeContext) Navigate(ctx context.Context, url string, waitUntil WaitCondition, timeoutMs int) (NavigationResult, error) {
	if err := c.checkAlive(ctx); err != nil {
		return NavigationResult{}, err
	}

	select {
	case <-ctx.Done():
		return NavigationResult{}, ctx.Err()
	default:
	}

	c.mu.Lock()
	c.state = ContextNavigating
	c.url = url
	c.pageRev++
	rev := c.pageRev
	navID := fmt.Sprintf("nav_%d", rev)
	httpStatus := 200

	// Simulate state progression
	switch waitUntil {
	case WaitCommit:
		c.state = ContextComplete
	case WaitInteractive:
		c.state = ContextInteractive
	case WaitComplete:
		c.state = ContextComplete
	}

	c.title = fakeTitle(url)
	c.nodes = fakeNodes(url)

	state := c.state
	c.mu.Unlock()

	return NavigationResult{
		ContextID:        c.id,
		NavigationID:     navID,
		URL:              url,
		State:            state,
		WaitConditionMet: true,
		PageRevision:     rev,
		HTTPStatus:       httpStatus,
	}, nil
}

func (c *fakeContext) Wait(ctx context.Context, opts WaitOptions) (WaitResult, error) {
	if err := c.checkAlive(ctx); err != nil {
		return WaitResult{}, err
	}

	select {
	case <-ctx.Done():
		return WaitResult{}, ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return WaitResult{
		ContextID:    c.id,
		PageRevision: c.pageRev,
		State:        c.state,
		URL:          c.url,
		ConditionMet: true,
	}, nil
}

func (c *fakeContext) Snapshot(ctx context.Context, opts SnapshotOptions) (PageSnapshot, error) {
	if err := c.checkAlive(ctx); err != nil {
		return PageSnapshot{}, err
	}

	select {
	case <-ctx.Done():
		return PageSnapshot{}, ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	nodes := c.nodes
	if nodes == nil {
		nodes = []SemanticNode{}
	}

	truncated := false
	if opts.MaxNodes > 0 && len(nodes) > opts.MaxNodes {
		nodes = nodes[:opts.MaxNodes]
		truncated = true
	}
	if opts.MaxDepth > 0 {
		truncateDepth(&nodes, opts.MaxDepth)
	}

	return PageSnapshot{
		ContextID:    c.id,
		PageRevision: c.pageRev,
		URL:          c.url,
		Title:        c.title,
		Viewport:     c.viewport,
		Nodes:        nodes,
		Truncated:    truncated,
	}, nil
}

func (c *fakeContext) Screenshot(ctx context.Context, opts ScreenshotOptions) (ScreenshotResult, error) {
	if err := c.checkAlive(ctx); err != nil {
		return ScreenshotResult{}, err
	}

	select {
	case <-ctx.Done():
		return ScreenshotResult{}, ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return ScreenshotResult{
		ContextID:    c.id,
		PageRevision: c.pageRev,
		Width:        c.viewport.Width,
		Height:       c.viewport.Height,
		Data:         []byte("fake-png-data"),
		MIMEType:     "image/png",
		Truncated:    false,
	}, nil
}

func (c *fakeContext) Query(ctx context.Context, locator Locator) (QueryResult, error) {
	if err := c.checkAlive(ctx); err != nil {
		return QueryResult{}, err
	}

	select {
	case <-ctx.Done():
		return QueryResult{}, ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	refs := findRefs(c.nodes, c.id, c.pageRev)
	return QueryResult{
		ContextID:    c.id,
		PageRevision: c.pageRev,
		Refs:         refs,
	}, nil
}

func (c *fakeContext) Click(ctx context.Context, ref ElementRef, opts ClickOptions) (ActionResult, error) {
	if err := c.checkAlive(ctx); err != nil {
		return ActionResult{}, err
	}

	select {
	case <-ctx.Done():
		return ActionResult{}, ctx.Err()
	case <-time.After(time.Millisecond):
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if ref.ContextID != c.id {
		return ActionResult{}, NewError(ErrContextNotFound, "ref from different context", false, nil)
	}
	if ref.PageRevision != c.pageRev {
		return ActionResult{}, ErrPageChangedSentinel
	}

	return ActionResult{
		ContextID:         c.id,
		PageRevision:      c.pageRev,
		ActionApplied:     true,
		NavigationStarted: false,
	}, nil
}

func (c *fakeContext) Type(ctx context.Context, ref ElementRef, text string, opts TypeOptions) (ActionResult, error) {
	if err := c.checkAlive(ctx); err != nil {
		return ActionResult{}, err
	}

	select {
	case <-ctx.Done():
		return ActionResult{}, ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if ref.ContextID != c.id {
		return ActionResult{}, NewError(ErrContextNotFound, "ref from different context", false, nil)
	}
	if ref.PageRevision != c.pageRev {
		return ActionResult{}, ErrPageChangedSentinel
	}

	return ActionResult{
		ContextID:         c.id,
		PageRevision:      c.pageRev,
		ActionApplied:     true,
		NavigationStarted: false,
	}, nil
}

func (c *fakeContext) PressKey(ctx context.Context, key string, modifiers []string) (ActionResult, error) {
	if err := c.checkAlive(ctx); err != nil {
		return ActionResult{}, err
	}
	select {
	case <-ctx.Done():
		return ActionResult{}, ctx.Err()
	default:
	}
	return ActionResult{
		ContextID:         c.id,
		PageRevision:      c.pageRev,
		ActionApplied:     true,
		NavigationStarted: false,
	}, nil
}

func (c *fakeContext) Scroll(ctx context.Context, opts ScrollOptions) (ActionResult, error) {
	if err := c.checkAlive(ctx); err != nil {
		return ActionResult{}, err
	}
	select {
	case <-ctx.Done():
		return ActionResult{}, ctx.Err()
	default:
	}
	return ActionResult{
		ContextID:         c.id,
		PageRevision:      c.pageRev,
		ActionApplied:     true,
		NavigationStarted: false,
	}, nil
}

func (c *fakeContext) SetViewport(ctx context.Context, vp Viewport) (Viewport, error) {
	if err := c.checkAlive(ctx); err != nil {
		return Viewport{}, err
	}
	select {
	case <-ctx.Done():
		return Viewport{}, ctx.Err()
	default:
	}
	c.mu.Lock()
	c.viewport = vp
	c.mu.Unlock()
	return vp, nil
}

func (c *fakeContext) Evaluate(ctx context.Context, source string, opts EvaluateOptions) (EvaluationResult, error) {
	if err := c.checkAlive(ctx); err != nil {
		return EvaluationResult{}, err
	}
	select {
	case <-ctx.Done():
		return EvaluationResult{}, ctx.Err()
	default:
	}
	return EvaluationResult{
		ContextID:    c.id,
		PageRevision: c.pageRev,
		Type:         "string",
		Value:        "fake-result",
		IsError:      false,
	}, nil
}

func (c *fakeContext) Console(ctx context.Context, cursor string, limit int) (ConsolePage, error) {
	if err := c.checkAlive(ctx); err != nil {
		return ConsolePage{}, err
	}
	select {
	case <-ctx.Done():
		return ConsolePage{}, ctx.Err()
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entries := c.console
	if entries == nil {
		entries = []ConsoleEntry{}
	}
	limitVal := limit
	if limitVal <= 0 || limitVal > 100 {
		limitVal = 100
	}
	if len(entries) > limitVal {
		entries = entries[:limitVal]
	}
	return ConsolePage{
		ContextID:    c.id,
		PageRevision: c.pageRev,
		Entries:      entries,
		Dropped:      0,
		Cursor:       fmt.Sprintf("cursor_%d", c.pageRev),
	}, nil
}

func (c *fakeContext) Network(ctx context.Context, cursor string, limit int) (NetworkPage, error) {
	if err := c.checkAlive(ctx); err != nil {
		return NetworkPage{}, err
	}
	select {
	case <-ctx.Done():
		return NetworkPage{}, ctx.Err()
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entries := c.network
	if entries == nil {
		entries = []NetworkEntry{}
	}
	limitVal := limit
	if limitVal <= 0 || limitVal > 100 {
		limitVal = 100
	}
	if len(entries) > limitVal {
		entries = entries[:limitVal]
	}
	return NetworkPage{
		ContextID:    c.id,
		PageRevision: c.pageRev,
		Entries:      entries,
		Dropped:      0,
		Cursor:       fmt.Sprintf("cursor_%d", c.pageRev),
	}, nil
}

func (c *fakeContext) Security(ctx context.Context) (SecuritySummary, error) {
	if err := c.checkAlive(ctx); err != nil {
		return SecuritySummary{}, err
	}
	select {
	case <-ctx.Done():
		return SecuritySummary{}, ctx.Err()
	default:
	}
	return SecuritySummary{
		ContextID:    c.id,
		PageRevision: c.pageRev,
		Scheme:       "https",
		Subject:      "CN=example.com",
		Issuer:       "CN=Fake CA",
		NotBefore:    "2026-01-01",
		NotAfter:     "2027-01-01",
		CSPEnabled:   true,
	}, nil
}

// --- helpers ---

func fakeTitle(url string) string {
	return "Fake Page: " + url
}

func fakeNodes(url string) []SemanticNode {
	return []SemanticNode{
		{Role: "navigation", Children: []SemanticNode{
			{Role: "link", Name: "Home"},
			{Role: "link", Name: "About"},
		}},
		{Role: "main", Children: []SemanticNode{
			{Role: "heading", Name: "Welcome", Level: 1},
			{Role: "presentation", Children: []SemanticNode{
				{Role: "link", Name: "Learn more"},
			}},
		}},
	}
}

func findRefs(nodes []SemanticNode, ctxID string, rev int) []ElementRef {
	var refs []ElementRef
	for _, n := range nodes {
		if n.Role != "presentation" && n.Role != "" && n.Role != "text" {
			ref := generateID("e")
			n.Ref = ref
			refs = append(refs, ElementRef{
				Ref:          ref,
				ContextID:    ctxID,
				PageRevision: rev,
			})
		}
		refs = append(refs, findRefs(n.Children, ctxID, rev)...)
	}
	return refs
}

func truncateDepth(nodes *[]SemanticNode, maxDepth int) {
	if maxDepth <= 0 {
		*nodes = nil
		return
	}
	for i := range *nodes {
		if maxDepth <= 1 {
			(*nodes)[i].Children = nil
		} else {
			truncateDepth(&(*nodes)[i].Children, maxDepth-1)
		}
	}
}
