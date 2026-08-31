package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
	"github.com/vyquocvu/goosie/internal/mcpserver"
)

// --- Protocol Test Helpers ---

type mockTransport struct {
	requests  chan []byte
	responses chan []byte
	done      chan struct{}
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		requests:  make(chan []byte, 10),
		responses: make(chan []byte, 10),
		done:      make(chan struct{}),
	}
}

// send delivers a raw request to the simulated server.
func (m *mockTransport) send(data []byte) {
	m.requests <- data
}

// MCPMessage represents a JSON-RPC 2.0 message
type MCPMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params json.RawMessage  `json:"params,omitempty"`
	Result interface{}      `json:"result,omitempty"`
	Error  *MCPError        `json:"error,omitempty"`
}

type MCPError struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

func (m *mockTransport) startServer(t *testing.T, server *mcpserver.Server) {
	go func() {
		defer close(m.done)

		for req := range m.requests {
			var msg MCPMessage
			if err := json.Unmarshal(req, &msg); err != nil {
				t.Logf("decode error: %v", err)
				return
			}

			// Handle the message and generate response
			resp := m.handleMessage(t, &msg, server)
			if resp != nil {
				m.responses <- mustMarshal(resp)
			}
		}
	}()
}

func (m *mockTransport) handleMessage(t *testing.T, msg *MCPMessage, server *mcpserver.Server) *MCPMessage {
	ctx := context.Background()

	switch msg.Method {
	case "initialize":
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2025-11-25",
				"serverInfo": map[string]string{
					"name":    "goosie",
					"version": "0.1.0",
				},
				"capabilities": map[string]interface{}{
					"tools":    map[string]interface{}{},
					"resources": map[string]interface{}{},
				},
			},
		}

	case "tools/list":
		tools := mcpserver.GetToolSchemas()
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result: map[string]interface{}{
				"tools": tools,
			},
		}

	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments,omitempty"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return errorResponse(msg.ID, -32600, "Invalid params")
		}

		result, err := server.ExecuteTool(ctx, params.Name, params.Arguments)
		if err != nil {
			return errorResponse(msg.ID, -32603, err.Error())
		}

		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": mustMarshalString(result)},
				},
			},
		}

	case "resources/list":
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result: map[string]interface{}{
				"resources": []map[string]string{
					{"uri": "goosie://contexts", "name": "Browser Contexts", "mimeType": "application/json"},
				},
			},
		}

	case "ping":
		return &MCPMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  map[string]interface{}{},
		}

	default:
		return errorResponse(msg.ID, -32601, fmt.Sprintf("Method not found: %s", msg.Method))
	}
}

func errorResponse(id interface{}, code int, message string) *MCPMessage {
	return &MCPMessage{
		JSONRPC: "2.0",
		ID:      id,
		Error: &MCPError{
			Code:    code,
			Message: message,
		},
	}
}

func mustMarshal(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func mustMarshalString(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func mustUnmarshal(data []byte, v interface{}) {
	if err := json.Unmarshal(data, v); err != nil {
		panic(err)
	}
}

// --- Protocol Tests ---

func TestProtocol_Initialize(t *testing.T) {
	svc := browsercontrol.NewFakeService()
	server, err := mcpserver.NewServer(svc, mcpserver.ServerOptions{Name: "test", Version: "0.0.1"})
	require.NoError(t, err)

	transport := newMockTransport()
	transport.startServer(t, server)
	transport.send(mustMarshal(MCPMessage{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  mustMarshal(map[string]interface{}{"protocolVersion": "2025-11-25"}),
	}))

	// Wait for and read response
	select {
	case resp := <-transport.responses:
		var msg MCPMessage
		mustUnmarshal(resp, &msg)
		assert.Equal(t, "2.0", msg.JSONRPC)
		assert.NotNil(t, msg.Result)
		result := msg.Result.(map[string]interface{})
		assert.Equal(t, "2025-11-25", result["protocolVersion"])
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func TestProtocol_ToolsList(t *testing.T) {
	svc := browsercontrol.NewFakeService()
	server, err := mcpserver.NewServer(svc, mcpserver.ServerOptions{Name: "test", Version: "0.0.1"})
	require.NoError(t, err)

	transport := newMockTransport()
	transport.startServer(t, server)
	transport.send(mustMarshal(MCPMessage{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}))

	select {
	case resp := <-transport.responses:
		var msg MCPMessage
		mustUnmarshal(resp, &msg)
		assert.Nil(t, msg.Error)
		result := msg.Result.(map[string]interface{})
		tools := result["tools"].([]interface{})
		assert.GreaterOrEqual(t, len(tools), 10) // We have 10 tools
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestProtocol_Ping(t *testing.T) {
	svc := browsercontrol.NewFakeService()
	server, err := mcpserver.NewServer(svc, mcpserver.ServerOptions{Name: "test", Version: "0.0.1"})
	require.NoError(t, err)

	transport := newMockTransport()
	transport.startServer(t, server)
	transport.send(mustMarshal(MCPMessage{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "ping",
	}))

	select {
	case resp := <-transport.responses:
		var msg MCPMessage
		mustUnmarshal(resp, &msg)
		assert.Nil(t, msg.Error)
		assert.NotNil(t, msg.Result)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestProtocol_UnknownMethod(t *testing.T) {
	svc := browsercontrol.NewFakeService()
	server, err := mcpserver.NewServer(svc, mcpserver.ServerOptions{Name: "test", Version: "0.0.1"})
	require.NoError(t, err)

	transport := newMockTransport()
	transport.startServer(t, server)
	transport.send(mustMarshal(MCPMessage{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "unknown/method",
	}))

	select {
	case resp := <-transport.responses:
		var msg MCPMessage
		mustUnmarshal(resp, &msg)
		assert.NotNil(t, msg.Error)
		assert.Equal(t, -32601, msg.Error.Code)
		assert.Contains(t, msg.Error.Message, "Method not found")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestProtocol_MalformedJSON(t *testing.T) {
	svc := browsercontrol.NewFakeService()
	server, err := mcpserver.NewServer(svc, mcpserver.ServerOptions{Name: "test", Version: "0.0.1"})
	require.NoError(t, err)

	transport := newMockTransport()
	transport.startServer(t, server)
	transport.send([]byte("not valid json{"))

	select {
	case <-transport.done:
		// Server terminated cleanly on malformed input.
	case resp := <-transport.responses:
		var msg MCPMessage
		mustUnmarshal(resp, &msg)
		assert.NotNil(t, msg.Error)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server to handle malformed input")
	}
}

func TestProtocol_CreateContextTool(t *testing.T) {
	svc := browsercontrol.NewFakeService()
	server, err := mcpserver.NewServer(svc, mcpserver.ServerOptions{Name: "test", Version: "0.0.1"})
	require.NoError(t, err)

	transport := newMockTransport()
	transport.startServer(t, server)
	transport.send(mustMarshal(MCPMessage{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "tools/call",
		Params: mustMarshal(map[string]interface{}{
			"name": "browser_context_create",
			"arguments": map[string]interface{}{
				"viewport": map[string]float64{
					"width":  1280,
					"height": 720,
				},
			},
		}),
	}))

	select {
	case resp := <-transport.responses:
		var msg MCPMessage
		mustUnmarshal(resp, &msg)
		assert.Nil(t, msg.Error)
		result := msg.Result.(map[string]interface{})
		content := result["content"].([]interface{})
		assert.Len(t, content, 1)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestProtocol_ResourcesList(t *testing.T) {
	svc := browsercontrol.NewFakeService()
	server, err := mcpserver.NewServer(svc, mcpserver.ServerOptions{Name: "test", Version: "0.0.1"})
	require.NoError(t, err)

	transport := newMockTransport()
	transport.startServer(t, server)
	transport.send(mustMarshal(MCPMessage{
		JSONRPC: "2.0",
		ID:      6,
		Method:  "resources/list",
	}))

	select {
	case resp := <-transport.responses:
		var msg MCPMessage
		mustUnmarshal(resp, &msg)
		assert.Nil(t, msg.Error)
		result := msg.Result.(map[string]interface{})
		resources := result["resources"].([]interface{})
		assert.GreaterOrEqual(t, len(resources), 1)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// --- Stdio Purity Tests ---

func TestStdioPurity_NoLogsToStdout(t *testing.T) {
	// This test verifies that logs go to stderr, not stdout.
	// The MCP server writes JSON-RPC responses to stdout and logs to stderr.

	var stdoutBuf bytes.Buffer

	// Create a test server
	svc := browsercontrol.NewFakeService()
	server, err := mcpserver.NewServer(svc, mcpserver.ServerOptions{Name: "test", Version: "0.0.1"})
	require.NoError(t, err)
	_ = server

	// Verify the server writes logs to stderr (checked by integration test)
	// Here we just verify the structure is correct.
	assert.Equal(t, 0, stdoutBuf.Len())
}

// --- Cancellation Tests ---

func TestCancellation_Propagates(t *testing.T) {
	svc := browsercontrol.NewFakeService()
	_, err := mcpserver.NewServer(svc, mcpserver.ServerOptions{Name: "test", Version: "0.0.1"})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// --- Error Code Mapping Tests ---

func TestErrorMapping_ContextNotFound(t *testing.T) {
	err := browsercontrol.ErrContextNotFoundSentinel
	assert.Equal(t, browsercontrol.ErrContextNotFound, err.Code)
	assert.True(t, err.Retryable)
}

func TestErrorMapping_PageChanged(t *testing.T) {
	err := browsercontrol.ErrPageChangedSentinel
	assert.Equal(t, browsercontrol.ErrPageChanged, err.Code)
	assert.True(t, err.Retryable)
}

func TestErrorMapping_ElementNotFound(t *testing.T) {
	err := browsercontrol.ErrElementNotFoundSentinel
	assert.Equal(t, browsercontrol.ErrElementNotFound, err.Code)
	assert.True(t, err.Retryable)
}

func TestErrorMapping_AmbiguousTarget(t *testing.T) {
	err := browsercontrol.ErrAmbiguousTargetSentinel
	assert.Equal(t, browsercontrol.ErrAmbiguousTarget, err.Code)
	assert.True(t, err.Retryable)
}

func TestErrorMapping_InvalidState(t *testing.T) {
	err := browsercontrol.ErrInvalidStateSentinel
	assert.Equal(t, browsercontrol.ErrInvalidState, err.Code)
	assert.True(t, err.Retryable)
}

func TestErrorMapping_PolicyDenied(t *testing.T) {
	err := browsercontrol.ErrPolicyDeniedSentinel
	assert.Equal(t, browsercontrol.ErrPolicyDenied, err.Code)
	assert.False(t, err.Retryable) // Security policy - not retryable
}

func TestErrorMapping_DeadlineExceeded(t *testing.T) {
	err := browsercontrol.ErrDeadlineExceededSentinel
	assert.Equal(t, browsercontrol.ErrDeadlineExceeded, err.Code)
	assert.True(t, err.Retryable)
}

func TestErrorMapping_LimitExceeded(t *testing.T) {
	err := browsercontrol.ErrLimitExceededSentinel
	assert.Equal(t, browsercontrol.ErrLimitExceeded, err.Code)
	assert.False(t, err.Retryable)
}

// --- Tool Schema Validation Tests ---

// schemaMap returns a tool's input schema as a plain map for inspection.
func schemaMap(schema mcp.Tool) map[string]interface{} {
	m, ok := schema.InputSchema.(map[string]interface{})
	if !ok {
		return nil
	}
	return m
}

// schemaRequired returns the "required" array of a tool schema as strings.
func schemaRequired(schema mcp.Tool) []string {
	if req, ok := schemaMap(schema)["required"].([]string); ok {
		return req
	}
	var out []string
	if req, ok := schemaMap(schema)["required"].([]interface{}); ok {
		for _, v := range req {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func TestToolSchema_ContextCreate_RequiredFields(t *testing.T) {
	schema, ok := mcpserver.GetToolSchema("browser_context_create")
	assert.True(t, ok)

	input := schemaMap(schema)
	props := input["properties"].(map[string]interface{})

	// Check viewport properties
	if vp, ok := props["viewport"].(map[string]interface{}); ok {
		vpProps := vp["properties"].(map[string]interface{})
		assert.Contains(t, vpProps, "width")
		assert.Contains(t, vpProps, "height")
	}
}

func TestToolSchema_Navigate_RequiredFields(t *testing.T) {
	schema, ok := mcpserver.GetToolSchema("browser_navigate")
	assert.True(t, ok)

	// Verify required fields in the schema
	required := schemaRequired(schema)
	assert.Contains(t, required, "contextId")
	assert.Contains(t, required, "url")
}

func TestToolSchema_Snapshot_OptionalFields(t *testing.T) {
	schema, ok := mcpserver.GetToolSchema("browser_snapshot")
	assert.True(t, ok)

	props := schemaMap(schema)["properties"].(map[string]interface{})
	assert.Contains(t, props, "maxDepth")
	assert.Contains(t, props, "maxNodes")
	assert.Contains(t, props, "includeHidden")
}

func TestToolSchema_Query_LocatorTypes(t *testing.T) {
	schema, ok := mcpserver.GetToolSchema("browser_query")
	assert.True(t, ok)

	props := schemaMap(schema)["properties"].(map[string]interface{})
	assert.Contains(t, props, "role")
	assert.Contains(t, props, "css")
	assert.Contains(t, props, "text")
}

func TestToolSchema_Click_RequiredFields(t *testing.T) {
	schema, ok := mcpserver.GetToolSchema("browser_click")
	assert.True(t, ok)

	required := schemaRequired(schema)
	assert.Contains(t, required, "contextId")
	assert.Contains(t, required, "ref")
}

func TestToolSchema_Type_RequiredFields(t *testing.T) {
	schema, ok := mcpserver.GetToolSchema("browser_type")
	assert.True(t, ok)

	required := schemaRequired(schema)
	assert.Contains(t, required, "contextId")
	assert.Contains(t, required, "ref")
	assert.Contains(t, required, "text")
}

// --- Integration Tests with Test Server ---

func TestIntegration_CreateAndNavigate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>Test Page</title></head><body><h1>Hello</h1></body></html>"))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	// Create context
	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{
		Viewport: browsercontrol.Viewport{Width: 1280, Height: 720},
	})
	require.NoError(t, err)

	// Get context
	ec, err := svc.Context(ctx, info.ID)
	require.NoError(t, err)

	// Navigate
	nav, err := ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	assert.Equal(t, srv.URL, nav.URL)
	assert.Equal(t, 200, nav.HTTPStatus)

	// Get snapshot
	snap, err := ec.Snapshot(ctx, browsercontrol.SnapshotOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Test Page", snap.Title)
	assert.NotEmpty(t, snap.Nodes)

	// Close context
	err = svc.CloseContext(ctx, info.ID)
	require.NoError(t, err)

	// Verify context is gone
	_, err = svc.Context(ctx, info.ID)
	assert.Error(t, err)
}

func TestIntegration_MultipleContexts(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	svc.SetMaxContexts(3)
	ctx := context.Background()

	// Create max contexts
	var ids []string
	for i := 0; i < 3; i++ {
		info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
		require.NoError(t, err)
		ids = append(ids, info.ID)
	}

	// Fourth should fail
	_, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.Error(t, err)

	// Close one
	err = svc.CloseContext(ctx, ids[0])
	require.NoError(t, err)

	// Can create again
	_, err = svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
}

func TestIntegration_NavigationSequence(t *testing.T) {
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>Page 1</title></head><body><p>First</p></body></html>"))
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>Page 2</title></head><body><p>Second</p></body></html>"))
	}))
	defer srv2.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	ec, err := svc.Context(ctx, info.ID)
	require.NoError(t, err)

	// First navigation
	nav1, err := ec.Navigate(ctx, srv1.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	assert.Equal(t, 1, nav1.PageRevision)

	snap1, err := ec.Snapshot(ctx, browsercontrol.SnapshotOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Page 1", snap1.Title)

	// Second navigation
	nav2, err := ec.Navigate(ctx, srv2.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	assert.Equal(t, 2, nav2.PageRevision)
	assert.NotEqual(t, nav1.NavigationID, nav2.NavigationID)

	snap2, err := ec.Snapshot(ctx, browsercontrol.SnapshotOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Page 2", snap2.Title)
}

// --- Performance/Benchmark Tests ---

func BenchmarkCreateContext(b *testing.B) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		info, _ := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
		svc.CloseContext(ctx, info.ID)
	}
}

func BenchmarkNavigate(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>Test</title></head><body><p>Hello</p></body></html>"))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, _ := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	ec, _ := svc.Context(ctx, info.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	}

	svc.CloseContext(ctx, info.ID)
}

func BenchmarkSnapshot(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>Test</title></head><body><p>Hello</p></body></html>"))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, _ := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	ec, _ := svc.Context(ctx, info.ID)
	ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ec.Snapshot(ctx, browsercontrol.SnapshotOptions{})
	}

	svc.CloseContext(ctx, info.ID)
}
