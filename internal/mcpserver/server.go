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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp/tool"

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
}

type ServerOptions struct {
	Name    string
	Version string
}

// NewServer creates a new MCP server backed by a browser-control service.
func NewServer(bc browsercontrol.Service, opts ServerOptions) (*Server, error) {
	if opts.Name == "" {
		opts.Name = "goosie"
	}
	if opts.Version == "" {
		opts.Version = "0.1.0"
	}

	s := &Server{
		bc:       bc,
		opts:     opts,
		requests: make(map[string]context.CancelFunc),
	}

	// Create MCP server with stdio transport
	mcpServer := mcp.NewServer(opts.Name, opts.Version,
		mcp.WithToolHandler(s.handleToolCall),
		mcp.WithResourceHandler(s.handleResourceRequest),
	)
	s.mcpServer = mcpServer

	return s, nil
}

// Run starts the MCP server using stdio transport.
func (s *Server) Run(ctx context.Context) error {
	slog.Info("starting MCP server", "name", s.opts.Name, "version", s.opts.Version)

	// Use stdio transport
	transport := mcp.NewStdioTransport(os.Stdin, os.Stdout, os.Stderr)

	// Run the server
	return s.mcpServer.Serve(ctx, transport)
}

// handleToolCall handles incoming MCP tool calls.
func (s *Server) handleToolCall(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slog.Debug("handling tool call", "name", req.Params.Name)

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

	result, err := s.executeTool(ctx, req.Params.Name, req.Params.Arguments)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: err.Error(),
				},
			},
		}, nil
	}

	// Convert result to MCP content
	content, err := s.resultToContent(result)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
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

	// Cancel all in-progress requests
	s.mu.Lock()
	for id, cancel := range s.requests {
		cancel()
		delete(s.requests, id)
	}
	s.mu.Unlock()

	return nil
}
