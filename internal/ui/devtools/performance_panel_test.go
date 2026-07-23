package devtools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/engine/metrics"
)

type mockMetricsProvider struct {
	m metrics.Metrics
}

func (m *mockMetricsProvider) Snapshot() metrics.Metrics { return m.m }

func TestPerformancePanelEmpty(t *testing.T) {
	panel := newPerformancePanel(nil)
	assert.NotNil(t, panel)
}

func TestPerformancePanelRefresh(t *testing.T) {
	panel := newPerformancePanel(nil).(*performancePanel)
	ctx := &TabContext{
		MetricsRecorder: &mockMetricsProvider{
			m: metrics.Metrics{
				NavID: 1,
				URL:   "https://example.com",
				Timings: []metrics.Timing{
					{Phase: metrics.PhaseDNSResolve, Started: time.Now(), Ended: time.Now().Add(10 * time.Millisecond)},
					{Phase: metrics.PhaseConnect, Started: time.Now(), Ended: time.Now().Add(50 * time.Millisecond)},
				},
				Counters: metrics.Counters{
					NodeCount:       100,
					CacheHits:       50,
					CacheMisses:     10,
					BytesDownloaded: 1024,
				},
			},
		},
	}
	panel.RefreshFrom(ctx)
	assert.NotNil(t, panel.label)
}

func TestPerformancePanel_Counters(t *testing.T) {
	panel := newPerformancePanel(nil).(*performancePanel)
	ctx := &TabContext{
		MetricsRecorder: &mockMetricsProvider{
			m: metrics.Metrics{
				NavID: 2,
				Counters: metrics.Counters{
					NodeCount:        200,
					RuleCount:        50,
					SelectorCount:    30,
					BoxCount:         80,
					DisplayItemCount: 120,
					ImageCount:       10,
					TileCount:        15,
					BytesDownloaded:  50000,
					DecodedImageBytes: 200000,
					CacheHits:        100,
					CacheMisses:      5,
					ScriptErrors:     3,
				},
			},
		},
	}
	panel.RefreshFrom(ctx)
	assert.Contains(t, panel.label.Text, "200")
	assert.Contains(t, panel.label.Text, "50")
	assert.Contains(t, panel.label.Text, "100")
	assert.Contains(t, panel.label.Text, "5")
	assert.Contains(t, panel.label.Text, "48.8 KB")
}

func TestPerformancePanel_NilMetrics(t *testing.T) {
	panel := newPerformancePanel(nil).(*performancePanel)
	ctx := &TabContext{MetricsRecorder: nil}
	panel.RefreshFrom(ctx)
	assert.Contains(t, panel.label.Text, "No performance data")
}

func TestPerformancePanel_NilTabContext(t *testing.T) {
	panel := newPerformancePanel(nil).(*performancePanel)
	panel.RefreshFrom(nil)
	assert.Contains(t, panel.label.Text, "No performance data")
}

func TestPerformancePanel_RenderStats(t *testing.T) {
	panel := newPerformancePanel(nil).(*performancePanel)
	ctx := &TabContext{
		MetricsRecorder: &mockMetricsProvider{
			m: metrics.Metrics{
				NavID: 3,
				Counters: metrics.Counters{
					NodeCount: 50,
				},
			},
		},
		RenderStats: map[string]time.Duration{
			"RenderHTML_p50":         5 * time.Millisecond,
			"RenderHTML_p95":         10 * time.Millisecond,
			"RenderWithViewport_p50": 8 * time.Millisecond,
			"RenderWithViewport_p99": 30 * time.Millisecond,
		},
	}
	panel.RefreshFrom(ctx)
	assert.Contains(t, panel.label.Text, "RenderHTML p50")
	assert.Contains(t, panel.label.Text, "5ms")
	assert.Contains(t, panel.label.Text, "RenderWithViewport p99")
	assert.Contains(t, panel.label.Text, "30ms")
	assert.Contains(t, panel.label.Text, "Render Timing")
}

func TestPerformancePanel_HumanPhaseLabel(t *testing.T) {
	assert.Equal(t, "DNS Resolve", humanPhaseLabel("dns_resolve"))
	assert.Equal(t, "First Byte", humanPhaseLabel("first_byte"))
	assert.Equal(t, "Body Read", humanPhaseLabel("body_read"))
	assert.Equal(t, "Connect", humanPhaseLabel("connect"))
}
