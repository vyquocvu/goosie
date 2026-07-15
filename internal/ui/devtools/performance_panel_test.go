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
