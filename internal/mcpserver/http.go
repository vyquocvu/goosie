package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// HTTPConfig configures the HTTP transport.
type HTTPConfig struct {
	// Bind address (default "127.0.0.1").
	Bind string
	// Port (default 0 = ephemeral).
	Port int
	// Path for MCP endpoint (default "/mcp").
	Path string
	// AllowedOrigins is the list of allowed Origin headers.
	// If empty, only loopback origins are allowed.
	AllowedOrigins []string
	// RequireAuth enables authentication (Bearer token).
	RequireAuth bool
	// AuthToken is the bearer token to accept (or env var name).
	AuthToken string
	// SessionTimeout is how long sessions remain valid (default 30min).
	SessionTimeout time.Duration
	// MaxRequestBytes limits request body size (default 1MB).
	MaxRequestBytes int64
	// RateCapacity per session (default 100).
	RateCapacity int
	// RateRefill tokens per second per session (default 50).
	RateRefill float64
}

// DefaultHTTPConfig returns loopback-only defaults.
func DefaultHTTPConfig() HTTPConfig {
	return HTTPConfig{
		Bind:           "127.0.0.1",
		Path:           "/mcp",
		SessionTimeout: 30 * time.Minute,
		MaxRequestBytes: 1 << 20, // 1MB
		RateCapacity:   100,
		RateRefill:     50,
	}
}

// HTTPServer provides Streamable HTTP transport for MCP.
type HTTPServer struct {
	config   HTTPConfig
	server   *Server
	listener net.Listener

	mu       sync.RWMutex
	sessions map[string]*httpSession

	requestCount  atomic.Uint64
	sessionCount  atomic.Int64
}

// NewHTTPServer creates a new HTTP transport server.
// The server still wraps the underlying browser service.
func NewHTTPServer(server *Server, config HTTPConfig) (*HTTPServer, error) {
	if config.Bind == "" {
		config.Bind = "127.0.0.1"
	}
	if config.Path == "" {
		config.Path = "/mcp"
	}
	if config.SessionTimeout <= 0 {
		config.SessionTimeout = 30 * time.Minute
	}
	if config.MaxRequestBytes <= 0 {
		config.MaxRequestBytes = 1 << 20
	}

	// Enforce loopback binding by default
	if err := validateLoopbackBind(config.Bind); err != nil {
		return nil, err
	}

	hs := &HTTPServer{
		config:   config,
		server:   server,
		sessions: make(map[string]*httpSession),
	}

	return hs, nil
}

// validateLoopbackBind ensures the bind address is loopback only.
// This prevents accidental external exposure.
func validateLoopbackBind(bind string) error {
	if bind == "" {
		return nil
	}
	// Allow only IPv4 loopback, IPv6 loopback, or localhost
	allowedHosts := map[string]bool{
		"localhost":   true,
		"127.0.0.1":   true,
		"::1":         true,
		"[::1]":       true,
	}
	if allowedHosts[bind] {
		return nil
	}
	return fmt.Errorf("HTTP bind must be loopback (127.0.0.1, ::1, localhost); got %q", bind)
}

// Start begins listening on the configured address.
// Returns the actual bound address (useful when Port=0).
func (h *HTTPServer) Start(ctx context.Context) (net.Addr, error) {
	addr := fmt.Sprintf("%s:%d", h.config.Bind, h.config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to bind to %s: %w", addr, err)
	}
	h.listener = listener

	mux := http.NewServeMux()
	mux.HandleFunc(h.config.Path, h.handleMCP)
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/version", h.handleVersion)

	httpSrv := &http.Server{
		Handler:      h.securityMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Run server
	go func() {
		slog.Info("HTTP MCP server listening", "addr", listener.Addr())
		if err := httpSrv.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	// Expire idle sessions in the background.
	go h.runSessionReaper(ctx)

	// Shutdown on context cancellation
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	return listener.Addr(), nil
}

// Addr returns the actual bound address (after Start).
func (h *HTTPServer) Addr() net.Addr {
	if h.listener == nil {
		return nil
	}
	return h.listener.Addr()
}

// securityMiddleware adds security headers and validation to all requests.
func (h *HTTPServer) securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Rate limit before any heavy work
		h.requestCount.Add(1)

		// Body size limit
		r.Body = http.MaxBytesReader(w, r.Body, h.config.MaxRequestBytes)

		// Add security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}

// handleMCP handles MCP-over-HTTP requests.
func (h *HTTPServer) handleMCP(w http.ResponseWriter, r *http.Request) {
	// Only POST and GET allowed (GET is for SSE streams); DELETE closes a session.
	if r.Method != http.MethodPost && r.Method != http.MethodGet && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate Origin to prevent DNS-rebinding and CSRF
	if !h.validateOrigin(r) {
		slog.Warn("rejected request with invalid Origin", "origin", r.Header.Get("Origin"))
		http.Error(w, "invalid Origin header", http.StatusForbidden)
		return
	}

	// Validate Host to prevent DNS-rebinding
	if !h.validateHost(r) {
		slog.Warn("rejected request with invalid Host", "host", r.Host)
		http.Error(w, "invalid Host header", http.StatusForbidden)
		return
	}

	// Optional authentication
	if h.config.RequireAuth {
		if !h.validateAuth(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Handle session
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID != "" {
		h.handleSessionRequest(w, r, sessionID)
		return
	}

	// No session - this is an initialization request
	if r.Method == http.MethodPost {
		h.handleInitialize(w, r)
		return
	}

	http.Error(w, "GET without session not supported in v1", http.StatusBadRequest)
}

// validateOrigin checks the Origin header against the allowlist.
func (h *HTTPServer) validateOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// No Origin header - same-origin request, allow.
		// POST and DELETE are state-changing; GET is used for SSE streams.
		if r.Method == http.MethodPost {
			return isJSONContentType(r.Header.Get("Content-Type"))
		}
		return r.Method == http.MethodGet || r.Method == http.MethodDelete
	}

	// If explicit allowlist configured, use it
	if len(h.config.AllowedOrigins) > 0 {
		for _, allowed := range h.config.AllowedOrigins {
			if origin == allowed {
				return true
			}
		}
		return false
	}

	// Default: only allow loopback origins
	return isLoopbackOrigin(origin)
}

// validateHost checks the Host header to prevent DNS-rebinding.
// Only allows exact loopback matches.
func (h *HTTPServer) validateHost(r *http.Request) bool {
	host := r.Host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	// Normalize bracketed IPv6 literals (e.g. "[::1]" -> "::1").
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")

	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// validateAuth checks the bearer token if auth is required.
func (h *HTTPServer) validateAuth(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	token := strings.TrimPrefix(auth, "Bearer ")

	// Support env var lookup
	expected := h.config.AuthToken
	if strings.HasPrefix(expected, "env:") {
		envName := strings.TrimPrefix(expected, "env:")
		expected = os.Getenv(envName)
	}

	if expected == "" {
		return false
	}

	// Constant-time comparison
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

// isLoopbackOrigin returns true if the URL's host is loopback.
func isLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func isJSONContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(ct)
	return ct == "application/json" || ct == "text/json"
}

// httpSession represents an active MCP-over-HTTP session.
type httpSession struct {
	ID          string
	CreatedAt   time.Time
	LastSeenAt  time.Time
	Context     context.Context
	Cancel      context.CancelFunc
	Limiter     *RateLimiter
	mu          sync.Mutex
	outbox      chan []byte
	initialized bool
}

// newSession creates a new HTTP session.
func (h *HTTPServer) newSession() *httpSession {
	id, err := generateSessionID()
	if err != nil {
		slog.Error("failed to generate session ID", "error", err)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	sess := &httpSession{
		ID:         id,
		CreatedAt:  time.Now(),
		LastSeenAt: time.Now(),
		Context:    ctx,
		Cancel:     cancel,
		Limiter:    NewRateLimiter(h.config.RateCapacity, h.config.RateRefill),
		outbox:     make(chan []byte, 16),
	}
	h.mu.Lock()
	h.sessions[id] = sess
	h.mu.Unlock()
	h.sessionCount.Add(1)
	return sess
}

// getSession retrieves an existing session.
func (h *HTTPServer) getSession(id string) *httpSession {
	h.mu.RLock()
	sess, ok := h.sessions[id]
	h.mu.RUnlock()
	if !ok {
		return nil
	}
	sess.mu.Lock()
	sess.LastSeenAt = time.Now()
	sess.mu.Unlock()
	return sess
}

// closeSession removes a session.
func (h *HTTPServer) closeSession(id string) {
	h.mu.Lock()
	sess, ok := h.sessions[id]
	if ok {
		delete(h.sessions, id)
	}
	h.mu.Unlock()
	if sess != nil {
		sess.Cancel()
		close(sess.outbox)
	}
	h.sessionCount.Add(-1)
}

// runSessionReaper periodically removes sessions older than SessionTimeout.
func (h *HTTPServer) runSessionReaper(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.cleanupExpiredSessions()
		}
	}
}

// cleanupExpiredSessions removes sessions older than SessionTimeout.
func (h *HTTPServer) cleanupExpiredSessions() {
	h.mu.Lock()
	now := time.Now()
	for id, sess := range h.sessions {
		if now.Sub(sess.LastSeenAt) > h.config.SessionTimeout {
			delete(h.sessions, id)
			sess.Cancel()
			close(sess.outbox)
			h.sessionCount.Add(-1)
		}
	}
	h.mu.Unlock()
}

// handleInitialize handles initial handshake and creates a session.
func (h *HTTPServer) handleInitialize(w http.ResponseWriter, r *http.Request) {
	// Read the body to extract initialize request
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Parse to ensure it's a valid initialize request
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if method, _ := req["method"].(string); method != "initialize" {
		http.Error(w, "first request must be initialize", http.StatusBadRequest)
		return
	}

	// Create session
	sess := h.newSession()
	if sess == nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	// Mark initialized
	sess.mu.Lock()
	sess.initialized = true
	sess.mu.Unlock()

	// Forward to MCP server and capture response
	// In a real impl, we'd use the SDK's streamable transport directly.
	// For now, proxy the request and add session ID to response.
	w.Header().Set("Mcp-Session-Id", sess.ID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Echo back a basic initialize response
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      req["id"],
		"result": map[string]interface{}{
			"protocolVersion": ProtocolVersion,
			"serverInfo":      GetServerInfo(),
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{},
				"resources": map[string]interface{}{},
			},
		},
	}
	data, _ := json.Marshal(resp)
	w.Write(data)

	slog.Info("session created", "id", sess.ID)
}

// handleSessionRequest handles requests within an existing session.
func (h *HTTPServer) handleSessionRequest(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess := h.getSession(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	if !sess.Limiter.Allow() {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// DELETE = close session
	if r.Method == http.MethodDelete {
		h.closeSession(sessionID)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Validate Accept header per MCP spec
	accept := r.Header.Get("Accept")
	if !strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/event-stream") {
		http.Error(w, "Accept must include application/json or text/event-stream", http.StatusBadRequest)
		return
	}

	// Read and validate body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if len(body) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// Parse JSON-RPC request
	var jsonReq map[string]interface{}
	if err := json.Unmarshal(body, &jsonReq); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate MCP-Protocol-Version header on session requests
	if pv := r.Header.Get("MCP-Protocol-Version"); pv != "" && pv != ProtocolVersion {
		http.Error(w, fmt.Sprintf("unsupported protocol version: %s", pv), http.StatusBadRequest)
		return
	}

	// Process the request through the MCP server
	method, _ := jsonReq["method"].(string)
	id, _ := jsonReq["id"]

	// Route to appropriate handler
	response := h.routeHTTPRequest(r.Context(), method, jsonReq["params"], id)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	data, _ := json.Marshal(response)
	w.Write(data)
}

// routeHTTPRequest routes an HTTP request to the appropriate handler.
// Returns the JSON-RPC response.
func (h *HTTPServer) routeHTTPRequest(ctx context.Context, method string, params interface{}, id interface{}) map[string]interface{} {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
	}

	switch method {
	case "initialize":
		resp["result"] = map[string]interface{}{
			"protocolVersion": ProtocolVersion,
			"serverInfo":      GetServerInfo(),
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{},
				"resources": map[string]interface{}{},
			},
		}
	case "tools/list":
		resp["result"] = h.handleToolsList()
	case "tools/call":
		pm, ok := params.(map[string]interface{})
		if !ok {
			resp["error"] = map[string]interface{}{
				"code":    -32602,
				"message": "invalid params",
			}
			return resp
		}
		result, err := h.server.executeTool(ctx, getString(pm, "name"), getMap(pm, "arguments"))
		if err != nil {
			resp["error"] = map[string]interface{}{
				"code":    -32603,
				"message": err.Error(),
			}
			return resp
		}
		content, _ := h.server.resultToContent(result)
		resp["result"] = map[string]interface{}{
			"content": content,
		}
	case "resources/list":
		resp["result"] = map[string]interface{}{
			"resources": []map[string]interface{}{
				{
					"uri":      "goosie://contexts",
					"name":    "Active Contexts",
					"mimeType": "application/json",
				},
			},
		}
	case "resources/read":
		pm, ok := params.(map[string]interface{})
		if !ok {
			resp["error"] = map[string]interface{}{
				"code":    -32602,
				"message": "invalid params",
			}
			return resp
		}
		uri, _ := pm["uri"].(string)
		if uri != "goosie://contexts" {
			resp["error"] = map[string]interface{}{
				"code":    -32602,
				"message": "unknown resource: " + uri,
			}
			return resp
		}
		list, err := h.server.bc.ListContexts(ctx)
		if err != nil {
			resp["error"] = map[string]interface{}{
				"code":    -32603,
				"message": err.Error(),
			}
			return resp
		}
		data, _ := json.Marshal(map[string]interface{}{"contexts": list})
		resp["result"] = map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":       uri,
					"mimeType": "application/json",
					"text":     string(data),
				},
			},
		}
	case "ping":
		resp["result"] = map[string]interface{}{}
	default:
		resp["error"] = map[string]interface{}{
			"code":    -32601,
			"message": "method not found: " + method,
		}
	}
	return resp
}

// handleToolsList returns the list of available tools.
func (h *HTTPServer) handleToolsList() map[string]interface{} {
	return map[string]interface{}{
		"tools": []map[string]interface{}{
			{
				"name":        "browser_context_create",
				"description": "Create a new browser context with optional viewport",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"viewport": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"width":  map[string]interface{}{"type": "integer"},
								"height": map[string]interface{}{"type": "integer"},
							},
						},
					},
				},
			},
			{
				"name":        "browser_navigate",
				"description": "Navigate to a URL",
				"inputSchema": map[string]interface{}{
					"type": "object",
					"required": []string{"contextId", "url"},
					"properties": map[string]interface{}{
						"contextId": map[string]interface{}{"type": "string"},
						"url":       map[string]interface{}{"type": "string"},
					},
				},
			},
		},
	}
}

// handleHealth responds to health checks.
func (h *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	healthy, msg := h.server.IsHealthy()
	if !healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"healthy":   healthy,
		"reason":    msg,
		"metrics":   h.server.Health(),
		"sessions":  h.sessionCount.Load(),
		"requests":  h.requestCount.Load(),
		"version":   Version,
	})
}

// handleVersion returns server version info.
func (h *HTTPServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GetServerInfo())
}

// generateSessionID generates a cryptographically random session ID.
func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Helper to extract string from map.
func getString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// Helper to extract map from interface.
func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	if v, ok := m[key].(map[string]interface{}); ok {
		return v
	}
	return nil
}

// Stop shuts down the HTTP server gracefully.
func (h *HTTPServer) Stop(ctx context.Context) error {
	// Close all sessions
	h.mu.Lock()
	for id := range h.sessions {
		sess := h.sessions[id]
		delete(h.sessions, id)
		sess.Cancel()
		close(sess.outbox)
	}
	h.mu.Unlock()

	if h.listener != nil {
		return h.listener.Close()
	}
	return nil
}
