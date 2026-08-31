package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
)

// --- Tool handlers ---

func (s *Server) toolCreateContext(ctx context.Context, bc createContextService, args map[string]interface{}) (*browsercontrol.ContextInfo, error) {
	opts := browsercontrol.CreateContextOptions{
		JavaScriptEnabled: true,
	}

	// Parse viewport if provided
	if vp, ok := args["viewport"].(map[string]interface{}); ok {
		if w, ok := vp["width"].(float64); ok {
			opts.Viewport.Width = int(w)
		}
		if h, ok := vp["height"].(float64); ok {
			opts.Viewport.Height = int(h)
		}
		if sc, ok := vp["scale"].(float64); ok {
			opts.Viewport.Scale = sc
		}
	}

	// Parse javascript_enabled if provided
	if js, ok := args["javascriptEnabled"].(bool); ok {
		opts.JavaScriptEnabled = js
	}

	slog.Debug("creating context", "viewport", opts.Viewport)
	info, err := bc.CreateContext(ctx, opts)
	if err != nil {
		return nil, mapError(err)
	}

	return &info, nil
}

func (s *Server) toolListContexts(ctx context.Context, bc createContextService, args map[string]interface{}) ([]browsercontrol.ContextInfo, error) {
	list, err := bc.ListContexts(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	return list, nil
}

func (s *Server) toolCloseContext(ctx context.Context, bc createContextService, args map[string]interface{}) (map[string]interface{}, error) {
	contextID, ok := args["contextId"].(string)
	if !ok || contextID == "" {
		return nil, fmt.Errorf("contextId is required")
	}

	slog.Debug("closing context", "contextId", contextID)
	if err := bc.CloseContext(ctx, contextID); err != nil {
		return nil, mapError(err)
	}

	return map[string]interface{}{
		"contextId": contextID,
		"closed":   true,
	}, nil
}

func (s *Server) toolNavigate(ctx context.Context, args map[string]interface{}) (*browsercontrol.NavigationResult, error) {
	contextID, ok := args["contextId"].(string)
	if !ok || contextID == "" {
		return nil, fmt.Errorf("contextId is required")
	}

	url, ok := args["url"].(string)
	if !ok || url == "" {
		return nil, fmt.Errorf("url is required")
	}

	// Get context
	ec, err := s.getContext(contextID)
	if err != nil {
		return nil, err
	}

	// Parse options
	waitUntil := browsercontrol.WaitComplete
	if w, ok := args["waitUntil"].(string); ok {
		waitUntil = browsercontrol.WaitCondition(w)
	}

	timeoutMs := browsercontrol.DefaultTimeoutMs
	if t, ok := args["timeoutMs"].(float64); ok {
		timeoutMs = int(t)
	}

	slog.Debug("navigating", "contextId", contextID, "url", url, "waitUntil", waitUntil)
	result, err := ec.Navigate(ctx, url, waitUntil, timeoutMs)
	if err != nil {
		return nil, mapError(err)
	}

	return &result, nil
}

func (s *Server) toolSnapshot(ctx context.Context, args map[string]interface{}) (*browsercontrol.PageSnapshot, error) {
	contextID, ok := args["contextId"].(string)
	if !ok || contextID == "" {
		return nil, fmt.Errorf("contextId is required")
	}

	ec, err := s.getContext(contextID)
	if err != nil {
		return nil, err
	}

	opts := browsercontrol.SnapshotOptions{
		Format: browsercontrol.SnapshotSemantic,
	}

	if md, ok := args["maxDepth"].(float64); ok {
		opts.MaxDepth = int(md)
	}
	if mn, ok := args["maxNodes"].(float64); ok {
		opts.MaxNodes = int(mn)
	}
	if ih, ok := args["includeHidden"].(bool); ok {
		opts.IncludeHidden = ih
	}

	snap, err := ec.Snapshot(ctx, opts)
	if err != nil {
		return nil, mapError(err)
	}

	return &snap, nil
}

func (s *Server) toolScreenshot(ctx context.Context, args map[string]interface{}) (*browsercontrol.ScreenshotResult, error) {
	contextID, ok := args["contextId"].(string)
	if !ok || contextID == "" {
		return nil, fmt.Errorf("contextId is required")
	}

	ec, err := s.getContext(contextID)
	if err != nil {
		return nil, err
	}

	opts := browsercontrol.ScreenshotOptions{
		Scope: "viewport",
	}

	if ob, ok := args["omitBackground"].(bool); ok {
		opts.OmitBackground = ob
	}

	shot, err := ec.Screenshot(ctx, opts)
	if err != nil {
		return nil, mapError(err)
	}

	return &shot, nil
}

func (s *Server) toolPageInfo(ctx context.Context, args map[string]interface{}) (*browsercontrol.ContextInfo, error) {
	contextID, ok := args["contextId"].(string)
	if !ok || contextID == "" {
		return nil, fmt.Errorf("contextId is required")
	}

	ec, err := s.getContext(contextID)
	if err != nil {
		return nil, err
	}

	info, err := ec.Info(ctx)
	if err != nil {
		return nil, mapError(err)
	}

	return &info, nil
}

func (s *Server) toolQuery(ctx context.Context, args map[string]interface{}) (*browsercontrol.QueryResult, error) {
	contextID, ok := args["contextId"].(string)
	if !ok || contextID == "" {
		return nil, fmt.Errorf("contextId is required")
	}

	ec, err := s.getContext(contextID)
	if err != nil {
		return nil, err
	}

	// Parse locator
	var locator browsercontrol.Locator
	if role, ok := args["role"].(map[string]interface{}); ok {
		locator.Role = &browsercontrol.RoleLocator{
			Name:  GetString(role, "name"),
			Exact: GetBool(role, "exact"),
		}
	} else if css, ok := args["css"].(map[string]interface{}); ok {
		locator.CSS = &browsercontrol.CSSLocator{
			Selector: GetString(css, "selector"),
		}
	} else if text, ok := args["text"].(map[string]interface{}); ok {
		locator.Text = &browsercontrol.TextLocator{
			Value: GetString(text, "value"),
			Exact: GetBool(text, "exact"),
		}
	}

	result, err := ec.Query(ctx, locator)
	if err != nil {
		return nil, mapError(err)
	}

	return &result, nil
}

func (s *Server) toolClick(ctx context.Context, args map[string]interface{}) (*browsercontrol.ActionResult, error) {
	contextID, ok := args["contextId"].(string)
	if !ok || contextID == "" {
		return nil, fmt.Errorf("contextId is required")
	}

	ec, err := s.getContext(contextID)
	if err != nil {
		return nil, err
	}

	ref := browsercontrol.ElementRef{}
	if r, ok := args["ref"].(string); ok {
		ref.Ref = r
	}
	ref.ContextID = contextID

	// Parse pageRevision from context info
	info, _ := ec.Info(ctx)
	ref.PageRevision = info.PageRevision

	opts := browsercontrol.ClickOptions{
		Button: "left",
	}
	if btn, ok := args["button"].(string); ok {
		opts.Button = btn
	}
	if t, ok := args["timeoutMs"].(float64); ok {
		opts.TimeoutMs = int(t)
	}

	result, err := ec.Click(ctx, ref, opts)
	if err != nil {
		return nil, mapError(err)
	}

	return &result, nil
}

func (s *Server) toolType(ctx context.Context, args map[string]interface{}) (*browsercontrol.ActionResult, error) {
	contextID, ok := args["contextId"].(string)
	if !ok || contextID == "" {
		return nil, fmt.Errorf("contextId is required")
	}

	ec, err := s.getContext(contextID)
	if err != nil {
		return nil, err
	}

	ref := browsercontrol.ElementRef{}
	if r, ok := args["ref"].(string); ok {
		ref.Ref = r
	}
	ref.ContextID = contextID

	info, _ := ec.Info(ctx)
	ref.PageRevision = info.PageRevision

	text, _ := args["text"].(string)

	opts := browsercontrol.TypeOptions{
		Replace: true,
		Submit:  false,
	}
	if rep, ok := args["replace"].(bool); ok {
		opts.Replace = rep
	}
	if sub, ok := args["submit"].(bool); ok {
		opts.Submit = sub
	}

	result, err := ec.Type(ctx, ref, text, opts)
	if err != nil {
		return nil, mapError(err)
	}

	return &result, nil
}

// --- Helpers ---

type createContextService interface {
	CreateContext(ctx context.Context, opts browsercontrol.CreateContextOptions) (browsercontrol.ContextInfo, error)
	ListContexts(ctx context.Context) ([]browsercontrol.ContextInfo, error)
	CloseContext(ctx context.Context, id string) error
}

type contextGetter interface {
	Context(ctx context.Context, id string) (browsercontrol.Context, error)
}

func (s *Server) getContext(id string) (browsercontrol.Context, error) {
	// Try to get from EngineService
	if eg, ok := s.bc.(interface {
		Context(ctx context.Context, id string) (browsercontrol.Context, error)
	}); ok {
		return eg.Context(context.Background(), id)
	}

	// For FakeService, we need a different approach
	// List and find the context
	list, err := s.bc.ListContexts(context.Background())
	if err != nil {
		return nil, err
	}

	for _, info := range list {
		if info.ID == id {
			// We need a way to get the context from FakeService
			// For now, return an error asking for EngineService
			return nil, fmt.Errorf("context %s found but cannot be retrieved: use EngineService", id)
		}
	}

	return nil, browsercontrol.ErrContextNotFoundSentinel
}

func mapError(err error) error {
	if err == nil {
		return nil
	}

	if bcErr, ok := err.(*browsercontrol.Error); ok {
		return fmt.Errorf("%s: %s", bcErr.Code, bcErr.Message)
	}

	return err
}

func GetBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func ParseInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}

// MarshalJSON implements json.Marshaler for error responses.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    browsercontrol.ErrorCode `json:"code"`
	Message string                  `json:"message"`
	Retryable bool                  `json:"retryable"`
}

func NewErrorResponse(code browsercontrol.ErrorCode, message string, retryable bool) ErrorResponse {
	return ErrorResponse{
		Error: ErrorDetail{
			Code:       code,
			Message:    message,
			Retryable: retryable,
		},
	}
}

func parseLocator(args map[string]interface{}) browsercontrol.Locator {
	var locator browsercontrol.Locator

	// Check for role locator
	if role, ok := args["role"].(map[string]interface{}); ok {
		locator.Role = &browsercontrol.RoleLocator{
			Name:  GetString(role, "name"),
			Exact: GetBool(role, "exact"),
		}
		return locator
	}

	// Check for CSS locator
	if css, ok := args["css"].(map[string]interface{}); ok {
		locator.CSS = &browsercontrol.CSSLocator{
			Selector: GetString(css, "selector"),
		}
		return locator
	}

	// Check for text locator
	if text, ok := args["text"].(map[string]interface{}); ok {
		locator.Text = &browsercontrol.TextLocator{
			Value: GetString(text, "value"),
			Exact: GetBool(text, "exact"),
		}
		return locator
	}

	// Default: try to find any role by name
	if name, ok := args["name"].(string); ok {
		locator.Role = &browsercontrol.RoleLocator{
			Name:  name,
			Exact: false,
		}
	}

	return locator
}

// formatResult formats a result for MCP response.
func formatResult(v interface{}) string {
	if v == nil {
		return "{}"
	}

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to marshal: %v"}`, err)
	}

	return string(data)
}

// truncateString truncates a string to maxLen characters.
func TruncateString(s string, maxLen int) string {
	return Truncate(s, maxLen)
}

// parseJSONArgs parses raw JSON arguments into a map.
func parseJSONArgs(rawArgs json.RawMessage) (map[string]interface{}, error) {
	if len(rawArgs) == 0 {
		return nil, nil
	}

	var args map[string]interface{}
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return nil, fmt.Errorf("invalid JSON arguments: %w", err)
	}
	return args, nil
}

// contains checks if a string slice contains a value.
func Contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// normalizeURL normalizes a URL for comparison.
func NormalizeURL(url string) string {
	url = strings.TrimSpace(url)
	url = strings.ToLower(url)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	return url
}
