// Package mcpserver provides an MCP (Model Context Protocol) server adapter
// for the browser-control package.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
)

// Server wraps a browser-control Service and exposes it via MCP protocol.
type Server struct {
	bc   browsercontrol.Service
	opts ServerOptions
	mu   sync.RWMutex

	// MCP server
	mcpServer *mcp.Server

	// Active requests for cancellation tracking
	requests map[string]context.CancelFunc

	// Hardening components
	audit   *AuditLogger
	health  *HealthReporter
	quota   *QuotaTracker
	limiter *RateLimiter
}

type ServerOptions struct {
	Name    string
	Version string

	// Hardening configuration
	Quota         QuotaLimits
	RateCapacity  int
	RateRefill    float64 // tokens per second
	MaxContexts   int
}

// NewServer creates a new MCP server backed by a browser-control service.
func NewServer(bc browsercontrol.Service, opts ServerOptions) (*Server, error) {
	if opts.Name == "" {
		opts.Name = "goosie"
	}
	if opts.Version == "" {
		opts.Version = "0.1.0"
	}
	if opts.MaxContexts <= 0 {
		opts.MaxContexts = 100
	}
	if opts.RateCapacity <= 0 {
		opts.RateCapacity = 100
	}
	if opts.RateRefill <= 0 {
		opts.RateRefill = 50
	}

	s := &Server{
		bc:       bc,
		opts:     opts,
		requests: make(map[string]context.CancelFunc),
		audit:    NewAuditLogger(),
		quota:    NewQuotaTracker(opts.Quota),
		limiter:  NewRateLimiter(opts.RateCapacity, opts.RateRefill),
	}
	s.health = NewHealthReporter(opts.MaxContexts, s.activeContextCount)

	// Create MCP server with stdio transport
	mcpServer := mcp.NewServer(opts.Name, opts.Version,
		mcp.WithToolHandler(s.handleToolCall),
		mcp.WithResourceHandler(s.handleResourceRequest),
	)
	s.mcpServer = mcpServer

	s.audit.LogServerEvent("startup", "success", map[string]string{
		"name":    opts.Name,
		"version": opts.Version,
	})

	return s, nil
}

// activeContextCount returns the count of active contexts via the service.
func (s *Server) activeContextCount() int64 {
	list, err := s.bc.ListContexts(context.Background())
	if err != nil {
		return 0
	}
	return int64(len(list))
}

// Health returns the current health metrics.
func (s *Server) Health() HealthMetrics {
	return s.health.Metrics()
}

// IsHealthy returns whether the server is healthy.
func (s *Server) IsHealthy() (bool, string) {
	return s.health.Health()
}

// Audit returns the audit logger for external use.
func (s *Server) Audit() *AuditLogger {
	return s.audit
}

// Quota returns the quota tracker for external use.
func (s *Server) Quota() *QuotaTracker {
	return s.quota
}

// LimiterTokens returns the current rate-limiter token count.
// Useful for diagnostics and tool integrations.
func (s *Server) LimiterTokens() float64 {
	return s.limiter.Tokens()
}

// LimiterAllow checks if a single request would be allowed under the rate limit.
func (s *Server) LimiterAllow() bool {
	return s.limiter.Allow()
}

// LimiterCapacity returns the configured burst capacity.
func (s *Server) LimiterCapacity() int {
	return s.limiter.capacity
}

// LimiterRefillRate returns the configured refill rate (tokens/sec).
func (s *Server) LimiterRefillRate() float64 {
	return s.limiter.refillRate
}

// RecordNavigation increments the per-context navigation counter.
func (s *Server) RecordNavigation(ctxID string) {
	s.quota.RecordNavigation(ctxID)
}

// RecordScreenshot increments the per-context screenshot counter.
func (s *Server) RecordScreenshot(ctxID string) {
	s.quota.RecordScreenshot(ctxID)
}

// RecordRequestGlobal records a server-wide request for health metrics.
func (s *Server) RecordRequestGlobal() {
	s.health.RecordRequest()
}

// RecordErrorGlobal records a server-wide error for health metrics.
func (s *Server) RecordErrorGlobal() {
	s.health.RecordError()
}

// Run starts the MCP server using stdio transport.
func (s *Server) Run(ctx context.Context) error {
	slog.Info("starting MCP server", "name", s.opts.Name, "version", s.opts.Version)
	s.audit.LogServerEvent("run", "start", nil)

	// Use stdio transport
	transport := mcp.NewStdioTransport(os.Stdin, os.Stdout, os.Stderr)

	// Run the server
	err := s.mcpServer.Serve(ctx, transport)
	if err != nil {
		s.audit.LogServerEvent("run", "error", map[string]string{"error": SafeError(err)})
		return err
	}
	return nil
}

// handleToolCall handles incoming MCP tool calls.
func (s *Server) handleToolCall(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slog.Debug("handling tool call", "name", req.Params.Name)

	start := time.Now()
	toolName := req.Params.Name

	// Rate limit check
	if !s.limiter.Allow() {
		s.health.RecordDenied()
		s.health.RecordError()
		s.audit.LogToolCall("", toolName, "denied", "rate_limited", time.Since(start), nil)
		return s.errorResult("rate limit exceeded; please slow down"), nil
	}
	s.health.RecordRequest()

	// Extract correlation ID for logging
	correlationID := req.JSONRPCRequest.ID
	if id, ok := correlationID.(string); ok {
		reqCtx, cancel := context.WithCancel(ctx)
		s.mu.Lock()
		s.requests[id] = cancel
		s.mu.Unlock()

		defer func() {
			s.mu.Lock()
			delete(s.requests, id)
			s.mu.Unlock()
		}()

		_ = reqCtx // Used via cancellation propagation
	}

	// Per-context quota check
	if ctxID := s.extractContextID(req.Params.Arguments); ctxID != "" {
		if ok, msg := s.quota.CheckRequest(ctxID); !ok {
			s.health.RecordDenied()
			s.audit.LogToolCall(ctxID, toolName, "denied", "quota_exceeded", time.Since(start),
				map[string]string{"reason": msg})
			return s.errorResult(msg), nil
		}
		s.quota.RecordRequest(ctxID)
	}

	result, err := s.executeTool(ctx, toolName, req.Params.Arguments)
	if err != nil {
		s.health.RecordError()
		s.audit.LogToolCall(s.extractContextID(req.Params.Arguments), toolName, "error",
			s.extractErrorCode(err), time.Since(start), nil)
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				{
					Type: "text",
					Text: SafeError(err),
				},
			},
		}, nil
	}

	s.audit.LogToolCall(s.extractContextID(req.Params.Arguments), toolName, "success", "",
		time.Since(start), nil)

	// Convert result to MCP content
	content, err := s.resultToContent(result)
	if err != nil {
		s.health.RecordError()
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("failed to serialize result: %v", err),
				},
			},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: content,
	}, nil
}

// extractContextID extracts context ID from tool arguments.
func (s *Server) extractContextID(args map[string]interface{}) string {
	if args == nil {
		return ""
	}
	if id, ok := args["contextId"].(string); ok {
		return id
	}
	return ""
}

// extractErrorCode attempts to extract an error code from an error.
func (s *Server) extractErrorCode(err error) string {
	if be, ok := err.(*browsercontrol.Error); ok {
		return be.Code
	}
	return "internal_error"
}

// errorResult creates an error tool result.
func (s *Server) errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			{
				Type: "text",
				Text: msg,
			},
		},
	}
}

// executeTool dispatches to the appropriate browser-control method.
func (s *Server) executeTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	bc, ok := s.bc.(interface {
		CreateContext(ctx context.Context, opts browsercontrol.CreateContextOptions) (browsercontrol.ContextInfo, error)
		ListContexts(ctx context.Context) ([]browsercontrol.ContextInfo, error)
		CloseContext(ctx context.Context, id string) error
	})
	if !ok {
		return nil, fmt.Errorf("service does not implement expected interface")
	}

	switch name {
	case "browser_context_create":
		return s.toolCreateContext(ctx, bc, args)
	case "browser_context_list":
		return s.toolListContexts(ctx, bc, args)
	case "browser_context_close":
		return s.toolCloseContext(ctx, bc, args)
	case "browser_navigate":
		return s.toolNavigate(ctx, args)
	case "browser_snapshot":
		return s.toolSnapshot(ctx, args)
	case "browser_screenshot":
		return s.toolScreenshot(ctx, args)
	case "browser_page_info":
		return s.toolPageInfo(ctx, args)
	case "browser_query":
		return s.toolQuery(ctx, args)
	case "browser_click":
		return s.toolClick(ctx, args)
	case "browser_type":
		return s.toolType(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// resultToContent converts a browser-control result to MCP content.
func (s *Server) resultToContent(result interface{}) ([]mcp.Content, error) {
	if result == nil {
		return []mcp.Content{
			{
				Type: "text",
				Text: "{}",
			},
		}, nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	return []mcp.Content{
		{
			Type: "text",
			Text: string(data),
		},
	}, nil
}

// handleResourceRequest handles MCP resource requests.
func (s *Server) handleResourceRequest(ctx context.Context, req *mcp.GetResourceRequest) (*mcp.GetResourceResult, error) {
	slog.Debug("handling resource request", "uri", req.Params.URI)

	// Parse resource URI
	switch req.Params.URI {
	case "goosie://contexts":
		return s.getContextsResource(ctx)
	default:
		return nil, fmt.Errorf("unknown resource: %s", req.Params.URI)
	}
}

func (s *Server) getContextsResource(ctx context.Context) (*mcp.GetResourceResult, error) {
	bc := s.bc
	list, err := bc.ListContexts(ctx)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(map[string]interface{}{
		"contexts": list,
	})
	if err != nil {
		return nil, err
	}

	return &mcp.GetResourceResult{
		Contents: []mcp.ResourceContents{
			{
				URI: "goosie://contexts",
				MIMEType: "application/json",
				Blob: string(data),
			},
		},
	}, nil
}

// CancelRequest cancels an in-progress request.
func (s *Server) CancelRequest(id string) {
	s.mu.RLock()
	cancel, ok := s.requests[id]
	s.mu.RUnlock()

	if ok {
		cancel()
	}
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("shutting down MCP server")
	s.audit.LogServerEvent("shutdown", "start", nil)

	// Cancel all in-progress requests
	cancelled := 0
	s.mu.Lock()
	for id, cancel := range s.requests {
		cancel()
		delete(s.requests, id)
		cancelled++
	}
	s.mu.Unlock()

	// Close audit logger last so we capture this event
	defer s.audit.LogServerEvent("shutdown", "complete", map[string]string{
		"cancelled_requests": fmt.Sprintf("%d", cancelled),
	})
	defer s.audit.Close()

	return nil
}
