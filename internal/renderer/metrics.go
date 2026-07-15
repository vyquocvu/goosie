package renderer

import (
	"math"
	"sort"
	"sync"
	"time"
)

type RenderMetrics struct {
	mu                      sync.Mutex
	renderHTMLTimes         []time.Duration
	computeLayoutTimes      []time.Duration
	renderWithViewportTimes []time.Duration
}

func NewRenderMetrics() *RenderMetrics {
	return &RenderMetrics{}
}

func (m *RenderMetrics) RecordRenderHTML(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.renderHTMLTimes = append(m.renderHTMLTimes, d)
}

func (m *RenderMetrics) RecordComputeLayout(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.computeLayoutTimes = append(m.computeLayoutTimes, d)
}

func (m *RenderMetrics) RecordRenderWithViewport(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.renderWithViewportTimes = append(m.renderWithViewportTimes, d)
}

func renderPercentile(latencies []time.Duration, p float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(math.Ceil((p / 100.0) * float64(len(sorted))))
	if idx == 0 {
		return sorted[0]
	}
	return sorted[idx-1]
}

func (m *RenderMetrics) Stats() map[string]time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()

	return map[string]time.Duration{
		"RenderHTML_p50":           renderPercentile(m.renderHTMLTimes, 50),
		"RenderHTML_p95":           renderPercentile(m.renderHTMLTimes, 95),
		"RenderHTML_p99":           renderPercentile(m.renderHTMLTimes, 99),
		"ComputeLayout_p50":        renderPercentile(m.computeLayoutTimes, 50),
		"ComputeLayout_p95":        renderPercentile(m.computeLayoutTimes, 95),
		"ComputeLayout_p99":        renderPercentile(m.computeLayoutTimes, 99),
		"RenderWithViewport_p50":   renderPercentile(m.renderWithViewportTimes, 50),
		"RenderWithViewport_p95":   renderPercentile(m.renderWithViewportTimes, 95),
		"RenderWithViewport_p99":   renderPercentile(m.renderWithViewportTimes, 99),
	}
}
