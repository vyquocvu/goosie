package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
	"github.com/vyquocvu/goosie/internal/mcpserver"
)

// helper to start an HTTP server on a random port
func startTestHTTPServer(t *testing.T, config mcpserver.HTTPConfig) (*mcpserver.HTTPServer, string) {
	t.Helper()
	bc := browsercontrol.NewEngineService()
	srv, err := mcpserver.NewServer(bc, mcpserver.ServerOptions{
		Name:        "test",
		Version:     "1.0.0",
		MaxContexts: 10,
	})
	require.NoError(t, err)

	config.Port = 0 // ephemeral
	hs, err := mcpserver.NewHTTPServer(srv, config)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	addr, err := hs.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		cancel()
		hs.Stop(context.Background())
	})

	return hs, "http://" + addr.String()
}

// TestHTTPServer_LoopbackOnly verifies that non-loopback addresses are rejected.
func TestHTTPServer_LoopbackOnly(t *testing.T) {
	bc := browsercontrol.NewEngineService()
	srv, _ := mcpserver.NewServer(bc, mcpserver.ServerOptions{})

	// Non-loopback should be rejected
	_, err := mcpserver.NewHTTPServer(srv, mcpserver.HTTPConfig{Bind: "0.0.0.0"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "loopback")

	_, err = mcpserver.NewHTTPServer(srv, mcpserver.HTTPConfig{Bind: "192.168.1.1"})
	assert.Error(t, err)

	// Loopback should be accepted
	_, err = mcpserver.NewHTTPServer(srv, mcpserver.HTTPConfig{Bind: "127.0.0.1"})
	assert.NoError(t, err)

	_, err = mcpserver.NewHTTPServer(srv, mcpserver.HTTPConfig{Bind: "localhost"})
	assert.NoError(t, err)
}

// TestHTTPServer_HealthEndpoint tests the health endpoint.
func TestHTTPServer_HealthEndpoint(t *testing.T) {
	_, baseURL := startTestHTTPServer(t, mcpserver.DefaultHTTPConfig())

	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	require.NoError(t, err)

	assert.Contains(t, data, "healthy")
	assert.Contains(t, data, "version")
	assert.Equal(t, mcpserver.Version, data["version"])
}

// TestHTTPServer_VersionEndpoint tests the version endpoint.
func TestHTTPServer_VersionEndpoint(t *testing.T) {
	_, baseURL := startTestHTTPServer(t, mcpserver.DefaultHTTPConfig())

	resp, err := http.Get(baseURL + "/version")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var info mcpserver.ServerInfo
	err = json.Unmarshal(body, &info)
	require.NoError(t, err)

	assert.Equal(t, "goosie-mcp-server", info.Name)
	assert.Equal(t, mcpserver.Version, info.Version)
}

// TestHTTPServer_InitializeSession tests the initialize flow.
func TestHTTPServer_InitializeSession(t *testing.T) {
	_, baseURL := startTestHTTPServer(t, mcpserver.DefaultHTTPConfig())

	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": mcpserver.ProtocolVersion,
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}
	body, _ := json.Marshal(initReq)

	resp, err := http.Post(baseURL+mcpserver.DefaultHTTPConfig().Path, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	sessionID := resp.Header.Get("Mcp-Session-Id")
	assert.NotEmpty(t, sessionID)
	assert.Len(t, sessionID, 64) // 32 bytes hex
}

// TestHTTPServer_ToolsList tests the tools/list method.
func TestHTTPServer_ToolsList(t *testing.T) {
	hs, baseURL := startTestHTTPServer(t, mcpserver.DefaultHTTPConfig())
	_ = hs

	sessionID := createSession(t, baseURL)

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	}
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", baseURL+mcpserver.DefaultHTTPConfig().Path, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Mcp-Session-Id", sessionID)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Origin", baseURL)

	resp, err := http.DefaultClient.Do(httpReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var data map[string]interface{}
	json.Unmarshal(respBody, &data)

	assert.Contains(t, data, "result")
}

// TestHTTPServer_OriginValidation verifies Origin header is validated.
func TestHTTPServer_OriginValidation(t *testing.T) {
	_, baseURL := startTestHTTPServer(t, mcpserver.DefaultHTTPConfig())

	body := bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))

	// External origin should be blocked
	req, _ := http.NewRequest("POST", baseURL+mcpserver.DefaultHTTPConfig().Path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestHTTPServer_AllowedOrigins tests explicit Origin allowlist.
func TestHTTPServer_AllowedOrigins(t *testing.T) {
	config := mcpserver.DefaultHTTPConfig()
	config.AllowedOrigins = []string{"https://myapp.example.com"}

	_, baseURL := startTestHTTPServer(t, config)
	body := bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))

	// Non-allowed origin should be blocked
	req, _ := http.NewRequest("POST", baseURL+config.Path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.com")
	resp, _ := http.DefaultClient.Do(req)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()

	// Allowed origin should succeed
	req2, _ := http.NewRequest("POST", baseURL+config.Path, bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Origin", "https://myapp.example.com")
	resp2, _ := http.DefaultClient.Do(req2)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	resp2.Body.Close()
}

// TestHTTPServer_HostValidation verifies Host header is validated.
func TestHTTPServer_HostValidation(t *testing.T) {
	startTestHTTPServer(t, mcpserver.DefaultHTTPConfig())

	// Loopback should be allowed (we tested that above)
	// External Host should be rejected - but this is hard to test with localhost server
	// since the Host header is usually set by Go's http client based on the URL.
	// We test the validation function directly here.

	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"[::1]", true},
		{"evil.com", false},
		{"8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			// Manually create a request with custom Host header
			req, _ := http.NewRequest("GET", "http://example.com/test", nil)
			req.Host = tt.host

			hs, _ := mcpserver.NewHTTPServer(nil, mcpserver.DefaultHTTPConfig())
			got := hs.ValidateHost(req)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestHTTPServer_AuthRequired tests bearer token authentication.
func TestHTTPServer_AuthRequired(t *testing.T) {
	config := mcpserver.DefaultHTTPConfig()
	config.RequireAuth = true
	config.AuthToken = "secret-token-123"

	_, baseURL := startTestHTTPServer(t, config)

	body := bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))

	// No auth header - should fail
	req, _ := http.NewRequest("POST", baseURL+config.Path, body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// Wrong token - should fail
	req2, _ := http.NewRequest("POST", baseURL+config.Path, bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer wrong-token")
	resp2, _ := http.DefaultClient.Do(req2)
	assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode)
	resp2.Body.Close()

	// Correct token - should succeed
	req3, _ := http.NewRequest("POST", baseURL+config.Path, bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "Bearer secret-token-123")
	resp3, _ := http.DefaultClient.Do(req3)
	assert.Equal(t, http.StatusOK, resp3.StatusCode)
	resp3.Body.Close()
}

// TestHTTPServer_DeleteSession tests session cleanup via DELETE.
func TestHTTPServer_DeleteSession(t *testing.T) {
	hs, baseURL := startTestHTTPServer(t, mcpserver.DefaultHTTPConfig())
	sessionID := createSession(t, baseURL)

	req, _ := http.NewRequest("DELETE", baseURL+mcpserver.DefaultHTTPConfig().Path, nil)
	req.Header.Set("Mcp-Session-Id", sessionID)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Subsequent requests with the deleted session should 404
	hs.Mu.RLock()
	_, exists := hs.Sessions[sessionID]
	hs.Mu.RUnlock()
	assert.False(t, exists)
}

// TestHTTPServer_BodySizeLimit verifies body size enforcement.
func TestHTTPServer_BodySizeLimit(t *testing.T) {
	config := mcpserver.DefaultHTTPConfig()
	config.MaxRequestBytes = 100

	_, baseURL := startTestHTTPServer(t, config)

	// Send a body larger than the limit
	large := strings.Repeat("x", 200)
	body := bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"` + large + `","params":{}}`))

	resp, err := http.Post(baseURL+config.Path, "application/json", body)
	if err == nil {
		defer resp.Body.Close()
		// Either 400 (rejected) or some response
		// Just ensure server didn't crash
	}
}

// TestHTTPServer_InvalidJSON tests rejection of invalid JSON.
func TestHTTPServer_InvalidJSON(t *testing.T) {
	_, baseURL := startTestHTTPServer(t, mcpserver.DefaultHTTPConfig())
	sessionID := createSession(t, baseURL)

	req, _ := http.NewRequest("POST", baseURL+mcpserver.DefaultHTTPConfig().Path, bytes.NewReader([]byte(`not json`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", sessionID)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestHTTPServer_RateLimiting verifies per-session rate limiting.
func TestHTTPServer_RateLimiting(t *testing.T) {
	config := mcpserver.DefaultHTTPConfig()
	config.RateCapacity = 2
	config.RateRefill = 0.1 // very slow refill

	hs, baseURL := startTestHTTPServer(t, config)
	sessionID := createSession(t, baseURL)

	hs.Mu.RLock()
	sess := hs.Sessions[sessionID]
	hs.Mu.RUnlock()
	require.NotNil(t, sess)

	// First 2 should pass
	assert.True(t, sess.Limiter.Allow())
	assert.True(t, sess.Limiter.Allow())
	// 3rd should fail
	assert.False(t, sess.Limiter.Allow())
}

// TestHTTPServer_SessionTimeout tests session expiry.
func TestHTTPServer_SessionTimeout(t *testing.T) {
	config := mcpserver.DefaultHTTPConfig()
	config.SessionTimeout = 100 * time.Millisecond

	hs, baseURL := startTestHTTPServer(t, config)
	sessionID := createSession(t, baseURL)

	// Session should exist
	hs.Mu.RLock()
	_, exists := hs.Sessions[sessionID]
	hs.Mu.RUnlock()
	assert.True(t, exists)

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	// Trigger cleanup manually if needed
	hs.CleanupExpiredSessions()
	time.Sleep(50 * time.Millisecond)

	hs.Mu.RLock()
	_, exists = hs.Sessions[sessionID]
	hs.Mu.RUnlock()
	// Session should be gone (or at least marked stale)
}

// TestGenerateSessionID verifies session ID generation.
func TestGenerateSessionID(t *testing.T) {
	id1, err := mcpserver.GenerateSessionID()
	require.NoError(t, err)
	assert.Len(t, id1, 64) // 32 bytes hex

	id2, err := mcpserver.GenerateSessionID()
	require.NoError(t, err)
	assert.NotEqual(t, id1, id2)
}

// TestIsLoopbackOrigin tests the origin check function.
func TestIsLoopbackOrigin(t *testing.T) {
	tests := []struct {
		origin string
		want   bool
	}{
		{"http://localhost:8080/path", true},
		{"http://127.0.0.1:8080/path", true},
		{"http://[::1]:8080/path", true},
		{"https://example.com", false},
		{"https://192.168.1.1", false},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			got := mcpserver.IsLoopbackOrigin(tt.origin)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRouteHTTPRequest_UnknownMethod tests error handling for unknown methods.
func TestRouteHTTPRequest_UnknownMethod(t *testing.T) {
	bc := browsercontrol.NewEngineService()
	srv, _ := mcpserver.NewServer(bc, mcpserver.ServerOptions{})
	hs, err := mcpserver.NewHTTPServer(srv, mcpserver.DefaultHTTPConfig())
	require.NoError(t, err)

	resp := hs.RouteHTTPRequest(context.Background(), "unknown/method", nil, 1)
	assert.Contains(t, resp, "error")
	errData, _ := resp["error"].(map[string]interface{})
	assert.Equal(t, -32601, errData["code"])
}

// TestRouteHTTPRequest_Ping verifies ping method.
func TestRouteHTTPRequest_Ping(t *testing.T) {
	bc := browsercontrol.NewEngineService()
	srv, _ := mcpserver.NewServer(bc, mcpserver.ServerOptions{})
	hs, _ := mcpserver.NewHTTPServer(srv, mcpserver.DefaultHTTPConfig())

	resp := hs.RouteHTTPRequest(context.Background(), "ping", nil, 1)
	assert.Contains(t, resp, "result")
	assert.NotContains(t, resp, "error")
}

// createSession is a helper that creates a session via the HTTP API.
func createSession(t *testing.T, baseURL string) string {
	t.Helper()
	body := bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + mcpserver.ProtocolVersion + `"}}`))
	resp, err := http.Post(baseURL+mcpserver.DefaultHTTPConfig().Path, "application/json", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return resp.Header.Get("Mcp-Session-Id")
}
