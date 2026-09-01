package mcpserver_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
	"github.com/vyquocvu/goosie/internal/mcpserver"
)

// TestServer_WithHardening tests that the server integrates with hardening components.
func TestServer_WithHardening(t *testing.T) {
	bc := browsercontrol.NewEngineService()
	bc.SetMaxContexts(5)

	server, err := mcpserver.NewServer(bc, mcpserver.ServerOptions{
		Name:          "test",
		Version:       "1.0.0",
		MaxContexts:   5,
		RateCapacity:  10,
		RateRefill:    5,
		Quota:         mcpserver.DefaultQuotaLimits(),
	})
	require.NoError(t, err)
	require.NotNil(t, server)

	// Verify hardening components are wired up
	assert.NotNil(t, server.Audit())
	assert.NotNil(t, server.Quota())

	// Verify health check works
	healthy, msg := server.IsHealthy()
	assert.True(t, healthy, msg)

	// Verify metrics collect
	metrics := server.Health()
	assert.Equal(t, 5, metrics.MaxContexts)
}

// TestServer_RateLimiting verifies rate limiting is enforced.
func TestServer_RateLimiting(t *testing.T) {
	bc := browsercontrol.NewEngineService()
	server, err := mcpserver.NewServer(bc, mcpserver.ServerOptions{
		Name:         "test",
		Version:      "1.0.0",
		RateCapacity: 2, // Very small capacity
		RateRefill:   0.1,
	})
	require.NoError(t, err)

	// Get rate limiter directly to verify configuration
	limiter := server.Limiter
	require.NotNil(t, limiter)

	// First 2 should pass immediately
	assert.True(t, limiter.Allow())
	assert.True(t, limiter.Allow())
	// 3rd should fail
	assert.False(t, limiter.Allow())
}

// TestServer_QuotaEnforcement tests per-context quotas.
func TestServer_QuotaEnforcement(t *testing.T) {
	bc := browsercontrol.NewEngineService()
	server, err := mcpserver.NewServer(bc, mcpserver.ServerOptions{
		Name:    "test",
		Version: "1.0.0",
		Quota: mcpserver.QuotaLimits{
			MaxRequestsPerContext: 3,
		},
	})
	require.NoError(t, err)

	tracker := server.Quota()
	ctxID := "ctx_test"

	// Record 3 requests
	for i := 0; i < 3; i++ {
		ok, _ := tracker.CheckRequest(ctxID)
		require.True(t, ok)
		tracker.RecordRequest(ctxID)
	}

	// 4th should fail
	ok, msg := tracker.CheckRequest(ctxID)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)
}

// TestServer_AuditLoggedToStderr verifies audit events are formatted properly.
func TestServer_AuditLoggedToStderr(t *testing.T) {
	var buf bytes.Buffer
	audit := mcpserver.NewAuditLoggerTo(&buf)

	audit.LogServerEvent("test_start", "success", map[string]string{
		"version": "1.0.0",
	})

	output := buf.String()
	assert.Contains(t, output, "test_start")
	assert.Contains(t, output, "success")
	assert.Contains(t, output, "1.0.0")
}

// TestServer_ShutdownIsClean verifies graceful shutdown.
func TestServer_ShutdownIsClean(t *testing.T) {
	bc := browsercontrol.NewEngineService()
	server, err := mcpserver.NewServer(bc, mcpserver.ServerOptions{
		Name:    "test-shutdown",
		Version: "1.0.0",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	require.NoError(t, err)
}
