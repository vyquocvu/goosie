package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vyquocvu/goosie/internal/mcpserver"
)

// TestAuditLogger_BasicEvent tests that audit events are written correctly.
func TestAuditLogger_BasicEvent(t *testing.T) {
	var buf bytes.Buffer
	logger := mcpserver.NewAuditLoggerTo(&buf)

	logger.LogServerEvent("test_event", "success", map[string]string{"key": "value"})

	output := buf.String()
	assert.Contains(t, output, "test_event")
	assert.Contains(t, output, "success")
	assert.Contains(t, output, "key")
	assert.Contains(t, output, "value")
	// Verify it's single-line JSON
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Equal(t, 1, len(lines))
}

// TestAuditLogger_ToolCall tests tool call audit logging.
func TestAuditLogger_ToolCall(t *testing.T) {
	var buf bytes.Buffer
	logger := mcpserver.NewAuditLoggerTo(&buf)

	logger.LogToolCall("ctx_123", "browser_navigate", "success", "", 100*time.Millisecond, nil)

	output := buf.String()
	assert.Contains(t, output, "tool_call")
	assert.Contains(t, output, "ctx_123")
	assert.Contains(t, output, "browser_navigate")
	assert.Contains(t, output, "100") // duration ms
}

// TestAuditLogger_SensitiveRedaction tests that sensitive keys are redacted.
func TestAuditLogger_SensitiveRedaction(t *testing.T) {
	sensitive := map[string]string{
		"password":      "secret123",
		"secret":        "abc",
		"regular_field": "ok",
	}
	sanitized := mcpserver.SanitizeMetadata(sensitive)

	assert.Equal(t, "[REDACTED]", sanitized["password"])
	assert.Equal(t, "[REDACTED]", sanitized["secret"])
	assert.Equal(t, "ok", sanitized["regular_field"])
}

// TestSafeError verifies safe error formatting.
func TestSafeError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"simple", simpleErr("simple error"), "simple error"},
		{"multiline", simpleErr("line1\nstack trace here"), "line1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mcpserver.SafeError(tt.err)
			if tt.want == "" {
				assert.Equal(t, "", got)
			} else {
				assert.Contains(t, got, tt.want)
			}
		})
	}
}

type simpleErr string

func (e simpleErr) Error() string { return string(e) }

// TestRedactURL verifies URL credential stripping.
func TestRedactURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/page", "https://example.com/page"},
		{"https://user:pass@example.com/page", "https://example.com/page"},
		{"http://user@host.com/", "http://host.com/"},
		{"no-scheme/path", "no-scheme/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mcpserver.RedactURL(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRateLimiter_Allows tests basic rate limiting.
func TestRateLimiter_Allows(t *testing.T) {
	limiter := mcpserver.NewRateLimiter(10, 1.0) // 10 burst, 1 token/sec

	// Should allow 10 immediately
	for i := 0; i < 10; i++ {
		assert.True(t, limiter.Allow(), "token %d should be allowed", i)
	}
	// 11th should be denied
	assert.False(t, limiter.Allow(), "11th request should be denied")
}

// TestRateLimiter_Refills tests that tokens refill over time.
func TestRateLimiter_Refills(t *testing.T) {
	limiter := mcpserver.NewRateLimiter(2, 10.0) // 2 burst, fast refill

	// Use both tokens
	limiter.Allow()
	limiter.Allow()
	assert.False(t, limiter.Allow())

	// Wait for refill
	time.Sleep(200 * time.Millisecond)
	assert.True(t, limiter.Allow())
}

// TestRateLimiter_Concurrent tests thread safety.
func TestRateLimiter_Concurrent(t *testing.T) {
	limiter := mcpserver.NewRateLimiter(100, 1000.0)
	var wg sync.WaitGroup
	var allowed int64
	var mu sync.Mutex

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.Allow() {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	// At least burst should be allowed; never more than burst + small refill
	assert.GreaterOrEqual(t, allowed, int64(100))
	assert.LessOrEqual(t, allowed, int64(110))
}

// TestQuotaTracker_RequestLimit tests request quota.
func TestQuotaTracker_RequestLimit(t *testing.T) {
	tracker := mcpserver.NewQuotaTracker(mcpserver.QuotaLimits{MaxRequestsPerContext: 3})

	ctxID := "ctx_test"

	// First 3 should pass
	for i := 0; i < 3; i++ {
		ok, msg := tracker.CheckRequest(ctxID)
		require.True(t, ok, "request %d should be allowed: %s", i, msg)
		tracker.RecordRequest(ctxID)
	}

	// 4th should fail
	ok, msg := tracker.CheckRequest(ctxID)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)
}

// TestQuotaTracker_NavigationLimit tests navigation quota.
func TestQuotaTracker_NavigationLimit(t *testing.T) {
	tracker := mcpserver.NewQuotaTracker(mcpserver.QuotaLimits{MaxNavigationsPerContext: 2})

	ctxID := "ctx_nav"

	ok, _ := tracker.CheckNavigation(ctxID)
	require.True(t, ok)
	tracker.RecordNavigation(ctxID)

	ok, _ = tracker.CheckNavigation(ctxID)
	require.True(t, ok)
	tracker.RecordNavigation(ctxID)

	ok, msg := tracker.CheckNavigation(ctxID)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)
}

// TestQuotaTracker_MemoryLimit tests memory quota.
func TestQuotaTracker_MemoryLimit(t *testing.T) {
	tracker := mcpserver.NewQuotaTracker(mcpserver.QuotaLimits{MaxMemoryPerContext: 100})

	ctxID := "ctx_mem"

	// First allocation should pass
	ok, _ := tracker.AddMemory(ctxID, 50)
	assert.True(t, ok)

	// Second allocation that pushes over limit should fail
	ok, msg := tracker.AddMemory(ctxID, 60)
	assert.False(t, ok)
	assert.NotEmpty(t, msg)
}

// TestQuotaTracker_Release verifies release clears counters.
func TestQuotaTracker_Release(t *testing.T) {
	tracker := mcpserver.NewQuotaTracker(mcpserver.QuotaLimits{MaxRequestsPerContext: 5})

	ctxID := "ctx_release"
	tracker.RecordRequest(ctxID)
	tracker.RecordRequest(ctxID)

	usage := tracker.Usage(ctxID)
	assert.Equal(t, 2, usage.RequestCount)

	tracker.ReleaseContext(ctxID)

	usage = tracker.Usage(ctxID)
	assert.Equal(t, 0, usage.RequestCount)
}

// TestHealthReporter_BasicMetrics tests health metrics collection.
func TestHealthReporter_BasicMetrics(t *testing.T) {
	reporter := mcpserver.NewHealthReporter(10, func() int64 { return 3 })

	reporter.RecordRequest()
	reporter.RecordRequest()
	reporter.RecordError()
	reporter.RecordTimeout()

	metrics := reporter.Metrics()
	assert.Equal(t, uint64(2), metrics.TotalRequests)
	assert.Equal(t, uint64(1), metrics.TotalErrors)
	assert.Equal(t, uint64(1), metrics.TotalTimeouts)
	assert.Equal(t, int64(3), metrics.ActiveContexts)
	assert.Equal(t, 10, metrics.MaxContexts)
	assert.InDelta(t, float64(metrics.UptimeSeconds), time.Since(metrics.StartedAt).Seconds(), 1.0)
}

// TestHealthReporter_Health tests health check.
func TestHealthReporter_Health(t *testing.T) {
	reporter := mcpserver.NewHealthReporter(100, func() int64 { return 5 })

	healthy, msg := reporter.Health()
	assert.True(t, healthy)
	assert.Equal(t, "ok", msg)
}

// TestHealthReporter_UnhealthyOnMaxContexts tests unhealthy state.
func TestHealthReporter_UnhealthyOnMaxContexts(t *testing.T) {
	reporter := mcpserver.NewHealthReporter(5, func() int64 { return 10 })

	healthy, msg := reporter.Health()
	assert.False(t, healthy)
	assert.Contains(t, msg, "context limit")
}

// TestShutdownHandler_Trigger tests shutdown triggering.
func TestShutdownHandler_Trigger(t *testing.T) {
	handler := mcpserver.NewShutdownHandler(100*time.Millisecond, nil)

	assert.Equal(t, mcpserver.ShutdownNone, handler.Signal())

	handler.Trigger(mcpserver.ShutdownGraceful)
	assert.Equal(t, mcpserver.ShutdownGraceful, handler.Signal())

	// Duplicate trigger should not panic
	handler.Trigger(mcpserver.ShutdownStdin)
	assert.Equal(t, mcpserver.ShutdownGraceful, handler.Signal()) // first signal wins
}

// TestShutdownHandler_Wait tests wait behavior.
func TestShutdownHandler_Wait(t *testing.T) {
	handler := mcpserver.NewShutdownHandler(50*time.Millisecond, nil)

	// Wait should block until triggered
	done := make(chan mcpserver.ShutdownSignal)
	go func() {
		sig := handler.Wait(context.Background())
		done <- sig
	}()

	time.Sleep(20 * time.Millisecond)
	handler.Trigger(mcpserver.ShutdownGraceful)

	select {
	case sig := <-done:
		assert.Equal(t, mcpserver.ShutdownGraceful, sig)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("wait did not return after trigger")
	}
}

// TestShutdownHandler_Execute tests shutdown handler execution.
func TestShutdownHandler_Execute(t *testing.T) {
	called := false
	handler := mcpserver.NewShutdownHandler(100*time.Millisecond, func(ctx context.Context) error {
		called = true
		return nil
	})

	err := handler.Execute()
	require.NoError(t, err)
	assert.True(t, called)
}

// TestServerInfo_Static tests server info.
func TestServerInfo_Static(t *testing.T) {
	info := mcpserver.GetServerInfo()
	assert.Equal(t, "goosie-mcp-server", info.Name)
	assert.NotEmpty(t, info.Version)
	assert.NotEmpty(t, info.ProtocolVersion)
	assert.NotEmpty(t, info.GoVersion)
}

// TestFormatSize tests size formatting helper.
func TestFormatSize(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{100, "100 B"},
		{2048, "2.00 KB"},
		{5 * 1024 * 1024, "5.00 MB"},
		{2 * 1024 * 1024 * 1024, "2.00 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := mcpserver.FormatSize(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestAuditLogger_Concurrent tests concurrent audit logging.
func TestAuditLogger_Concurrent(t *testing.T) {
	var buf bytes.Buffer
	logger := mcpserver.NewAuditLoggerTo(&buf)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.LogServerEvent("concurrent", "ok", nil)
		}()
	}
	wg.Wait()

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Equal(t, 100, len(lines))
}

// TestTruncate tests truncation helper.
func TestTruncate(t *testing.T) {
	assert.Equal(t, "abc", mcpserver.Truncate("abc", 10))
	assert.Equal(t, "abcdef...", mcpserver.Truncate("abcdefghij", 9))
	assert.Equal(t, "a...", mcpserver.Truncate("abcdef", 4))
	assert.Equal(t, "abc", mcpserver.Truncate("abcdef", 3)) // too small for ellipsis
}

// TestQuotaUsage_JSON tests QuotaUsage JSON marshalling.
func TestQuotaUsage_JSON(t *testing.T) {
	usage := mcpserver.QuotaUsage{
		MemoryBytes:     1024,
		RequestCount:    5,
		ScreenshotCount: 2,
		NavigationCount: 3,
	}

	data, err := json.Marshal(usage)
	require.NoError(t, err)
	parsed := make(map[string]interface{})
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.Equal(t, float64(1024), parsed["memoryBytes"])
	assert.Equal(t, float64(5), parsed["requestCount"])
	assert.Equal(t, float64(2), parsed["screenshotCount"])
	assert.Equal(t, float64(3), parsed["navigationCount"])
}
