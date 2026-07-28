package browsercontrol

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
	"github.com/vyquocvu/goosie/internal/engine/session"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
)

// EngineService creates real browser contexts backed by engine packages.
type EngineService struct {
	mu          sync.Mutex
	maxContexts int
	contexts    map[string]*engineContext
}

var _ Service = (*EngineService)(nil)

// NewEngineService creates a new engine-backed service.
func NewEngineService() *EngineService {
	return &EngineService{
		maxContexts: DefaultMaxContexts,
		contexts:    make(map[string]*engineContext),
	}
}

// SetMaxContexts overrides the default context limit.
func (s *EngineService) SetMaxContexts(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxContexts = n
}

func (s *EngineService) CreateContext(ctx context.Context, opts CreateContextOptions) (ContextInfo, error) {
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
	ec := newEngineContext(id, opts.Viewport)
	s.contexts[id] = ec
	return ec.info(), nil
}

func (s *EngineService) ListContexts(ctx context.Context) ([]ContextInfo, error) {
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

func (s *EngineService) CloseContext(ctx context.Context, id string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.mu.Lock()
	ec, ok := s.contexts[id]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.contexts, id)
	s.mu.Unlock()
	ec.close()
	return nil
}

// Context returns the engine context by ID (for test access).
func (s *EngineService) Context(id string) (*engineContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ec, ok := s.contexts[id]
	if !ok {
		return nil, NewError(ErrContextNotFound, "context not found", true, nil)
	}
	return ec, nil
}

// engineContext is a real browser context backed by engine packages.
// It does not import Fyne or internal/ui.
type engineContext struct {
	id      string
	sess    *session.Session
	fetcher *net.Fetcher
	parser  *dom.Parser

	mu        sync.Mutex
	viewport  Viewport
	pageRev   int
	closed    bool
	lastURL   string
	lastDoc   *html.Node
	lastTitle string
	console   []ConsoleEntry
	network   []NetworkEntry
	navID     navigation.ID
	domStore  *dom.Store // Compact DOM store for mutations

	// JavaScript runtime
	jsSession *js.Session
	jsRuntime *js.Runtime
}

var _ Context = (*engineContext)(nil)

func newEngineContext(id string, vp Viewport) *engineContext {
	if vp.Width == 0 {
		vp.Width = 1280
	}
	if vp.Height == 0 {
		vp.Height = 720
	}
	if vp.Scale == 0 {
		vp.Scale = 1.0
	}
	sess := session.New()
	jsSess := js.NewSession(js.DefaultSessionConfig())
	rt := jsSess.Runtime()
	return &engineContext{
		id:        id,
		sess:      sess,
		fetcher:   net.NewFetcherWithClient(sess.HTTPClient()),
		parser:    dom.NewParser(),
		viewport:  vp,
		jsSession: jsSess,
		jsRuntime: rt,
	}
}

func (ec *engineContext) close() {
	ec.mu.Lock()
	if ec.closed {
		ec.mu.Unlock()
		return
	}
	ec.closed = true
	ec.mu.Unlock()
	// Close JS session first
	if ec.jsSession != nil {
		ec.jsSession.Close()
	}
	ec.sess.Close()
}

func (ec *engineContext) info() ContextInfo {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	state := mapSessionState(ec.sess.State())
	return ContextInfo{
		ID:           ec.id,
		State:        state,
		PageRevision: ec.pageRev,
		Viewport:     ec.viewport,
	}
}

func mapSessionState(s session.State) ContextState {
	switch s {
	case session.StateCreated:
		return ContextCreated
	case session.StateNavigating:
		return ContextNavigating
	case session.StateParsing:
		return ContextParsing
	case session.StateInteractive:
		return ContextInteractive
	case session.StateComplete:
		return ContextComplete
	case session.StateFailed:
		return ContextFailed
	case session.StateCancelled:
		return ContextCancelled
	case session.StateClosed:
		return ContextClosed
	default:
		return ContextCreated
	}
}

func (ec *engineContext) checkAlive(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	ec.mu.Lock()
	defer ec.mu.Unlock()
	if ec.closed {
		return ErrContextNotFoundSentinel
	}
	return nil
}

func (ec *engineContext) ID() string { return ec.id }

func (ec *engineContext) Info(ctx context.Context) (ContextInfo, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return ContextInfo{}, err
	}
	return ec.info(), nil
}

func (ec *engineContext) Navigate(ctx context.Context, url string, waitUntil WaitCondition, timeoutMs int) (NavigationResult, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return NavigationResult{}, err
	}
	if timeoutMs <= 0 {
		timeoutMs = DefaultTimeoutMs
	}
	navCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	load, fetchCtx := ec.sess.Navigate(navCtx, url)
	if !load.ID.IsValid() {
		return NavigationResult{}, NewError(ErrPolicyDenied, "navigation rejected", false, nil)
	}
	ec.mu.Lock()
	ec.navID = load.ID
	ec.mu.Unlock()
	ec.sess.URL(url)

	stream, meta, fetchErr := ec.fetcher.FetchStreamWithContext(fetchCtx, url)
	if !ec.sess.IsActive(load.ID) {
		if stream != nil {
			stream.Close()
		}
		ec.sess.Cancel()
		return NavigationResult{}, ErrCancelledSentinel
	}

	var htmlContent string
	var httpStatus int
	if fetchErr != nil {
		ec.sess.Fail(fetchErr)
		return NavigationResult{}, NewError(ErrInternal, fetchErr.Error(), true, nil)
	}
	httpStatus = meta.Status

	// Detect non-HTML content
	contentType := strings.ToLower(meta.ContentType)
	isHTML := strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml+xml")
	if !isHTML && contentType != "" {
		stream.Close()
		ec.sess.Cancel()
		return NavigationResult{}, NewError(ErrUnsupported,
			fmt.Sprintf("unsupported content type: %s", contentType), false, nil)
	}

	data, readErr := io.ReadAll(stream)
	stream.Close()
	if readErr != nil {
		ec.sess.Fail(readErr)
		return NavigationResult{}, NewError(ErrInternal, readErr.Error(), true, nil)
	}
	htmlContent = string(data)
	if httpStatus >= 400 && strings.TrimSpace(htmlContent) == "" {
		htmlContent = fmt.Sprintf(
			"<html><body><h1>%d</h1><p>The server returned an error.</p></body></html>",
			httpStatus,
		)
	}

	ec.sess.Parsing()
	doc, parseErr := ec.parser.ParseDocument(strings.NewReader(htmlContent))
	if parseErr != nil {
		ec.sess.Fail(parseErr)
		return NavigationResult{}, NewError(ErrInternal, "parse error: "+parseErr.Error(), true, nil)
	}

	ec.sess.Interactive()

	title := extractTitle(doc)
	ec.sess.Title(title)

	ec.mu.Lock()
	ec.pageRev++
	rev := ec.pageRev
	ec.lastDoc = doc
	ec.lastURL = url
	ec.lastTitle = title
	ec.mu.Unlock()

	ec.sess.Complete()

	state := mapSessionState(ec.sess.State())
	waitMet := stateMet(state, waitUntil)

	return NavigationResult{
		ContextID:        ec.id,
		NavigationID:     fmt.Sprintf("nav_%d", rev),
		URL:              url,
		State:            state,
		WaitConditionMet: waitMet,
		PageRevision:     rev,
		HTTPStatus:       httpStatus,
	}, nil
}

func (ec *engineContext) Wait(ctx context.Context, opts WaitOptions) (WaitResult, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return WaitResult{}, err
	}
	select {
	case <-ctx.Done():
		return WaitResult{}, ctx.Err()
	default:
	}
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return WaitResult{
		ContextID:    ec.id,
		PageRevision: ec.pageRev,
		State:        mapSessionState(ec.sess.State()),
		URL:          ec.lastURL,
		ConditionMet: true,
	}, nil
}

func (ec *engineContext) Snapshot(ctx context.Context, opts SnapshotOptions) (PageSnapshot, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return PageSnapshot{}, err
	}
	select {
	case <-ctx.Done():
		return PageSnapshot{}, ctx.Err()
	default:
	}
	ec.mu.Lock()
	defer ec.mu.Unlock()
	if ec.lastDoc == nil {
		return PageSnapshot{
			ContextID: ec.id, PageRevision: ec.pageRev,
		}, nil
	}

	// Apply default limits
	maxDepth := opts.MaxDepth
	if maxDepth <= 0 || maxDepth > MaxSnapshotDepth {
		maxDepth = MaxSnapshotDepth
	}
	maxNodes := opts.MaxNodes
	if maxNodes <= 0 || maxNodes > MaxSnapshotNodes {
		maxNodes = MaxSnapshotNodes
	}

	nodes, truncated := domToSemantic(ec.lastDoc, ec.id, maxDepth, maxNodes)
	return PageSnapshot{
		ContextID:    ec.id,
		PageRevision: ec.pageRev,
		URL:          ec.lastURL,
		Title:        ec.lastTitle,
		Viewport:     ec.viewport,
		Nodes:        nodes,
		Truncated:    truncated,
	}, nil
}

func (ec *engineContext) Screenshot(ctx context.Context, opts ScreenshotOptions) (ScreenshotResult, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return ScreenshotResult{}, err
	}
	select {
	case <-ctx.Done():
		return ScreenshotResult{}, ctx.Err()
	default:
	}
	ec.mu.Lock()
	vp := ec.viewport
	rev := ec.pageRev
	ec.mu.Unlock()

	// TODO: Implement real screenshot using renderer
	// For now, return empty placeholder
	return ScreenshotResult{
		ContextID:    ec.id,
		PageRevision: rev,
		Width:        vp.Width,
		Height:       vp.Height,
		Data:         []byte{},
		MIMEType:     "image/png",
		Truncated:    false,
	}, nil
}

func (ec *engineContext) Query(ctx context.Context, locator Locator) (QueryResult, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return QueryResult{}, err
	}
	select {
	case <-ctx.Done():
		return QueryResult{}, ctx.Err()
	default:
	}
	ec.mu.Lock()
	defer ec.mu.Unlock()
	if ec.lastDoc == nil {
		return QueryResult{ContextID: ec.id, PageRevision: ec.pageRev, Refs: []ElementRef{}}, nil
	}

	nodes, _ := domToSemantic(ec.lastDoc, ec.id, 0, 0)
	refs := findRefs(nodes, ec.id, ec.pageRev)

	// Filter refs based on locator
	if locator.Role != nil {
		refs = filterRefsByRole(refs, nodes, locator.Role.Name, locator.Role.Exact)
	}

	return QueryResult{
		ContextID:    ec.id,
		PageRevision: ec.pageRev,
		Refs:         refs,
	}, nil
}

func (ec *engineContext) Click(ctx context.Context, ref ElementRef, opts ClickOptions) (ActionResult, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return ActionResult{}, err
	}
	if ref.ContextID != ec.id {
		return ActionResult{}, NewError(ErrContextNotFound, "ref from different context", false, nil)
	}
	ec.mu.Lock()
	rev := ec.pageRev
	ec.mu.Unlock()
	if ref.PageRevision != rev {
		return ActionResult{}, ErrPageChangedSentinel
	}

	select {
	case <-ctx.Done():
		return ActionResult{}, ctx.Err()
	default:
	}

	// TODO: Implement real click - dispatch click event to element
	// For now, return success

	return ActionResult{
		ContextID:         ec.id,
		PageRevision:      rev,
		ActionApplied:     true,
		NavigationStarted: false,
	}, nil
}

func (ec *engineContext) Type(ctx context.Context, ref ElementRef, text string, opts TypeOptions) (ActionResult, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return ActionResult{}, err
	}
	if ref.ContextID != ec.id {
		return ActionResult{}, NewError(ErrContextNotFound, "ref from different context", false, nil)
	}
	ec.mu.Lock()
	rev := ec.pageRev
	ec.mu.Unlock()
	if ref.PageRevision != rev {
		return ActionResult{}, ErrPageChangedSentinel
	}

	select {
	case <-ctx.Done():
		return ActionResult{}, ctx.Err()
	default:
	}

	// TODO: Implement real type - dispatch input events to element
	// For now, return success

	return ActionResult{
		ContextID:         ec.id,
		PageRevision:      rev,
		ActionApplied:     true,
		NavigationStarted: false,
	}, nil
}

func (ec *engineContext) PressKey(ctx context.Context, key string, modifiers []string) (ActionResult, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return ActionResult{}, err
	}
	select {
	case <-ctx.Done():
		return ActionResult{}, ctx.Err()
	default:
	}

	ec.mu.Lock()
	rev := ec.pageRev
	ec.mu.Unlock()

	return ActionResult{
		ContextID:         ec.id,
		PageRevision:      rev,
		ActionApplied:     true,
		NavigationStarted: false,
	}, nil
}

func (ec *engineContext) Scroll(ctx context.Context, opts ScrollOptions) (ActionResult, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return ActionResult{}, err
	}
	select {
	case <-ctx.Done():
		return ActionResult{}, ctx.Err()
	default:
	}

	ec.mu.Lock()
	rev := ec.pageRev
	ec.mu.Unlock()

	return ActionResult{
		ContextID:         ec.id,
		PageRevision:      rev,
		ActionApplied:     true,
		NavigationStarted: false,
	}, nil
}

func (ec *engineContext) SetViewport(ctx context.Context, vp Viewport) (Viewport, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return Viewport{}, err
	}
	select {
	case <-ctx.Done():
		return Viewport{}, ctx.Err()
	default:
	}
	ec.mu.Lock()
	ec.viewport = vp
	ec.mu.Unlock()
	return vp, nil
}

func (ec *engineContext) Evaluate(ctx context.Context, source string, opts EvaluateOptions) (EvaluationResult, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return EvaluationResult{}, err
	}
	select {
	case <-ctx.Done():
		return EvaluationResult{}, ctx.Err()
	default:
	}

	ec.mu.Lock()
	rev := ec.pageRev
	jsRuntime := ec.jsRuntime
	ec.mu.Unlock()

	if jsRuntime == nil {
		return EvaluationResult{
			ContextID:    ec.id,
			PageRevision: rev,
			Type:        "string",
			Value:       "JavaScript runtime not available",
			IsError:     true,
			ErrorText:   "JS runtime not initialized",
		}, nil
	}

	// Apply limits
	maxBytes := opts.MaxResultBytes
	if maxBytes <= 0 {
		maxBytes = MaxEvalResultBytes
	}
	if maxBytes > MaxEvalResultBytes {
		maxBytes = MaxEvalResultBytes
	}

	// Check source length
	if len(source) > MaxSourceBytes {
		return EvaluationResult{
			ContextID:    ec.id,
			PageRevision: rev,
			Type:        "string",
			Value:       "",
			IsError:     true,
			ErrorText:   fmt.Sprintf("source exceeds maximum length of %d bytes", MaxSourceBytes),
		}, nil
	}

	// Run the script
	value, err := jsRuntime.RunScript(source)

	// Process result
	if err != nil {
		return EvaluationResult{
			ContextID:    ec.id,
			PageRevision: rev,
			Type:        "string",
			Value:       "",
			IsError:     true,
			ErrorText:   truncateError(err.Error(), maxBytes),
		}, nil
	}

	// Convert value to serializable result
	result := jsRuntime.SerializeValue(value, maxBytes)
	if result.Type == "" {
		// Serialization failed
		return EvaluationResult{
			ContextID:    ec.id,
			PageRevision: rev,
			Type:        "string",
			Value:       value.String(),
			IsError:     false,
		}, nil
	}

	return EvaluationResult{
		ContextID:    ec.id,
		PageRevision: rev,
		Type:        result.Type,
		Value:       result.Value,
		IsError:     false,
	}, nil
}

func (ec *engineContext) Console(ctx context.Context, cursor string, limit int) (ConsolePage, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return ConsolePage{}, err
	}
	select {
	case <-ctx.Done():
		return ConsolePage{}, ctx.Err()
	default:
	}
	ec.mu.Lock()
	defer ec.mu.Unlock()

	entries := ec.console
	if entries == nil {
		entries = []ConsoleEntry{}
	}

	limitVal := limit
	if limitVal <= 0 || limitVal > 100 {
		limitVal = 100
	}

	dropped := 0
	if len(entries) > limitVal {
		dropped = len(entries) - limitVal
		entries = entries[:limitVal]
	}

	return ConsolePage{
		ContextID:    ec.id,
		PageRevision: ec.pageRev,
		Entries:      entries,
		Dropped:      dropped,
		Cursor:       fmt.Sprintf("cursor_%d", ec.pageRev),
	}, nil
}

func (ec *engineContext) Network(ctx context.Context, cursor string, limit int) (NetworkPage, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return NetworkPage{}, err
	}
	select {
	case <-ctx.Done():
		return NetworkPage{}, ctx.Err()
	default:
	}
	ec.mu.Lock()
	defer ec.mu.Unlock()

	entries := ec.network
	if entries == nil {
		entries = []NetworkEntry{}
	}

	limitVal := limit
	if limitVal <= 0 || limitVal > 100 {
		limitVal = 100
	}

	dropped := 0
	if len(entries) > limitVal {
		dropped = len(entries) - limitVal
		entries = entries[:limitVal]
	}

	return NetworkPage{
		ContextID:    ec.id,
		PageRevision: ec.pageRev,
		Entries:      entries,
		Dropped:      dropped,
		Cursor:       fmt.Sprintf("cursor_%d", ec.pageRev),
	}, nil
}

func (ec *engineContext) Security(ctx context.Context) (SecuritySummary, error) {
	if err := ec.checkAlive(ctx); err != nil {
		return SecuritySummary{}, err
	}
	select {
	case <-ctx.Done():
		return SecuritySummary{}, ctx.Err()
	default:
	}
	return SecuritySummary{
		ContextID:    ec.id,
		PageRevision: ec.pageRev,
		Scheme:       "https",
	}, nil
}

// --- DOM helpers ---

func extractTitle(doc *html.Node) string {
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
			title = n.FirstChild.Data
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return title
}

func countRefNodes(nodes []SemanticNode) int {
	count := 0
	for _, n := range nodes {
		if n.Ref != "" {
			count++
		}
		count += countRefNodes(n.Children)
	}
	return count
}

func domToSemantic(doc *html.Node, ctxID string, maxDepth int, maxNodes int) ([]SemanticNode, bool) {
	count := 0
	truncated := false
	nodes := buildSemanticNodes(doc, ctxID, 0, maxDepth, maxNodes, &count, &truncated)
	return nodes, truncated
}

func buildSemanticNodes(n *html.Node, ctxID string, depth int, maxDepth int, maxNodes int, count *int, truncated *bool) []SemanticNode {
	if maxDepth > 0 && depth > maxDepth {
		return nil
	}
	var nodes []SemanticNode
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if maxNodes > 0 && *count >= maxNodes {
			*truncated = true
			break
		}
		switch c.Type {
		case html.ElementNode:
			role := inferDOMRole(c)
			name := computeDOMName(c)
			sn := SemanticNode{
				Role: role,
				Name: name,
			}
			attrs := make(map[string]string)
			for _, a := range c.Attr {
				switch a.Key {
				case "href", "src", "class", "id", "type", "aria-label", "role", "alt", "title":
					attrs[a.Key] = a.Val
				}
			}
			if len(attrs) > 0 {
				sn.Attributes = attrs
			}
			if role == "heading" {
				switch c.Data {
				case "h1":
					sn.Level = 1
				case "h2":
					sn.Level = 2
				case "h3":
					sn.Level = 3
				case "h4":
					sn.Level = 4
				case "h5":
					sn.Level = 5
				case "h6":
					sn.Level = 6
				}
			}
			if role != "presentation" && role != "text" && role != "" && role != "none" && role != "unknown" {
				*count++
				ref := fmt.Sprintf("e_%s_%d", ctxID, *count)
				sn.Ref = ref
			}
			sn.Children = buildSemanticNodes(c, ctxID, depth+1, maxDepth, maxNodes, count, truncated)
			nodes = append(nodes, sn)
		case html.TextNode:
			text := strings.TrimSpace(c.Data)
			if text == "" {
				continue
			}
			sn := SemanticNode{
				Role: "text",
				Name: text,
			}
			if len(text) > 80 {
				sn.Name = text[:80] + "…"
			}
			nodes = append(nodes, sn)
		}
	}
	return nodes
}

func inferDOMRole(n *html.Node) string {
	// Check explicit role first
	for _, a := range n.Attr {
		if a.Key == "role" && a.Val != "" {
			return a.Val
		}
	}
	switch n.Data {
	case "a":
		for _, a := range n.Attr {
			if a.Key == "href" {
				return "link"
			}
		}
		return "anchor"
	case "button":
		return "button"
	case "img":
		return "img"
	case "input":
		for _, a := range n.Attr {
			if a.Key == "type" {
				switch a.Val {
				case "checkbox":
					return "checkbox"
				case "radio":
					return "radio"
				case "submit", "button":
					return "button"
				default:
					return "textbox"
				}
			}
		}
		return "textbox"
	case "textarea":
		return "textbox"
	case "select":
		return "listbox"
	case "nav":
		return "navigation"
	case "main":
		return "main"
	case "header":
		return "banner"
	case "footer":
		return "contentinfo"
	case "aside":
		return "complementary"
	case "form":
		return "form"
	case "table":
		return "table"
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "heading"
	case "ul", "ol":
		return "list"
	case "li":
		return "listitem"
	case "meta", "link", "script", "style", "head":
		return "none"
	default:
		return "presentation"
	}
}

func computeDOMName(n *html.Node) string {
	// Check aria-label first
	for _, a := range n.Attr {
		switch a.Key {
		case "aria-label":
			if a.Val != "" {
				return a.Val
			}
		case "alt":
			if a.Val != "" {
				return a.Val
			}
		case "title":
			if a.Val != "" {
				return a.Val
			}
		case "placeholder":
			if a.Val != "" {
				return a.Val
			}
		case "name":
			if a.Val != "" {
				return a.Val
			}
		}
	}
	// For links, use href
	if n.Data == "a" {
		for _, a := range n.Attr {
			if a.Key == "href" {
				if len(a.Val) < 60 && a.Val != "" {
					return a.Val
				}
			}
		}
	}
	return ""
}

func stateMet(current ContextState, target WaitCondition) bool {
	order := map[ContextState]int{
		ContextCreated:     0,
		ContextNavigating:  1,
		ContextParsing:     2,
		ContextInteractive: 3,
		ContextComplete:     4,
		ContextFailed:      5,
		ContextCancelled:   5,
		ContextClosed:     6,
	}
	cur := order[current]
	switch target {
	case WaitCommit:
		return cur >= order[ContextNavigating]
	case WaitInteractive:
		return cur >= order[ContextInteractive]
	case WaitComplete:
		return cur >= order[ContextComplete]
	}
	return false
}

func findRefs(nodes []SemanticNode, ctxID string, rev int) []ElementRef {
	var refs []ElementRef
	for _, n := range nodes {
		if n.Ref != "" {
			refs = append(refs, ElementRef{
				Ref:          n.Ref,
				ContextID:    ctxID,
				PageRevision: rev,
			})
		}
		refs = append(refs, findRefs(n.Children, ctxID, rev)...)
	}
	return refs
}

func filterRefsByRole(refs []ElementRef, nodes []SemanticNode, roleName string, exact bool) []ElementRef {
	var filtered []ElementRef
	for _, ref := range refs {
		if nodeHasRole(nodes, ref.Ref, roleName, exact) {
			filtered = append(filtered, ref)
		}
	}
	return filtered
}

func nodeHasRole(nodes []SemanticNode, ref string, roleName string, exact bool) bool {
	for _, n := range nodes {
		if n.Ref == ref {
			if exact {
				return n.Role == roleName
			}
			return strings.Contains(n.Role, roleName)
		}
		if nodeHasRole(n.Children, ref, roleName, exact) {
			return true
		}
	}
	return false
}

// truncateError truncates an error message to maxBytes.
func truncateError(s string, maxBytes int) string {
	if len(s) > maxBytes {
		return s[:maxBytes] + "...[truncated]"
	}
	return s
}
