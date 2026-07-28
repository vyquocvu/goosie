package mcpserver

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Version is the MCP server version reported to clients.
const Version = "1.0.0-alpha"

// ProtocolVersion is the MCP protocol version this server implements.
const ProtocolVersion = "2025-11-25"

// ServerInfo contains static information about the server.
type ServerInfo struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	ProtocolVersion string `json:"protocolVersion"`
	GoVersion       string `json:"goVersion"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
}

// GetServerInfo returns static server information.
func GetServerInfo() ServerInfo {
	return ServerInfo{
		Name:            "goosie-mcp-server",
		Version:         Version,
		ProtocolVersion: ProtocolVersion,
		GoVersion:       runtime.Version(),
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
	}
}

// HealthMetrics tracks runtime metrics for the server.
type HealthMetrics struct {
	StartedAt     time.Time `json:"startedAt"`
	UptimeSeconds int64     `json:"uptimeSeconds"`

	TotalRequests  uint64 `json:"totalRequests"`
	TotalErrors    uint64 `json:"totalErrors"`
	TotalTimeouts  uint64 `json:"totalTimeouts"`
	TotalDenied    uint64 `json:"totalDenied"`

	ActiveContexts int64 `json:"activeContexts"`
	MaxContexts    int   `json:"maxContexts"`

	MemoryAllocBytes  uint64 `json:"memoryAllocBytes"`
	MemorySysBytes    uint64 `json:"memorySysBytes"`
	MemoryHeapObjects uint64 `json:"memoryHeapObjects"`

	Goroutines int `json:"goroutines"`

	GCRuns uint32 `json:"gcRuns"`
}

// HealthReporter provides health and metrics information.
type HealthReporter struct {
	startedAt     time.Time
	requestCount  atomic.Uint64
	errorCount    atomic.Uint64
	timeoutCount  atomic.Uint64
	deniedCount   atomic.Uint64
	mu            sync.RWMutex
	maxContexts   int
	activeCountFn func() int64
	lastGCCount   uint32
}

// NewHealthReporter creates a new health reporter.
// activeContexts is a function that returns the current count of active contexts.
// maxContexts is the configured maximum.
func NewHealthReporter(maxContexts int, activeContexts func() int64) *HealthReporter {
	return &HealthReporter{
		startedAt:     time.Now(),
		maxContexts:   maxContexts,
		activeCountFn: activeContexts,
	}
}

// RecordRequest increments the request counter.
func (h *HealthReporter) RecordRequest() {
	h.requestCount.Add(1)
}

// RecordError increments the error counter.
func (h *HealthReporter) RecordError() {
	h.errorCount.Add(1)
}

// RecordTimeout increments the timeout counter.
func (h *HealthReporter) RecordTimeout() {
	h.timeoutCount.Add(1)
}

// RecordDenied increments the denied counter (security policy).
func (h *HealthReporter) RecordDenied() {
	h.deniedCount.Add(1)
}

// Metrics returns a snapshot of current health metrics.
func (h *HealthReporter) Metrics() HealthMetrics {
	h.mu.RLock()
	maxCtx := h.maxContexts
	h.mu.RUnlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	var active int64
	if h.activeCountFn != nil {
		active = h.activeCountFn()
	}

	return HealthMetrics{
		StartedAt:         h.startedAt,
		UptimeSeconds:     int64(time.Since(h.startedAt).Seconds()),
		TotalRequests:     h.requestCount.Load(),
		TotalErrors:       h.errorCount.Load(),
		TotalTimeouts:     h.timeoutCount.Load(),
		TotalDenied:       h.deniedCount.Load(),
		ActiveContexts:    active,
		MaxContexts:       maxCtx,
		MemoryAllocBytes:  memStats.Alloc,
		MemorySysBytes:    memStats.Sys,
		MemoryHeapObjects: memStats.HeapObjects,
		Goroutines:        runtime.NumGoroutine(),
		GCRuns:            memStats.NumGC,
	}
}

// Health returns whether the server is healthy.
func (h *HealthReporter) Health() (bool, string) {
	metrics := h.Metrics()

	// Memory check: if allocation exceeds 500MB, report unhealthy
	const memLimit = 500 * 1024 * 1024
	if metrics.MemoryAllocBytes > memLimit {
		return false, fmt.Sprintf("memory pressure: %s allocated", FormatSize(int(metrics.MemoryAllocBytes)))
	}

	// Context limit check
	if metrics.MaxContexts > 0 && metrics.ActiveContexts >= int64(metrics.MaxContexts) {
		return false, fmt.Sprintf("context limit reached: %d/%d", metrics.ActiveContexts, metrics.MaxContexts)
	}

	return true, "ok"
}

// SetMaxContexts updates the maximum context count for reporting.
func (h *HealthReporter) SetMaxContexts(n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.maxContexts = n
}
