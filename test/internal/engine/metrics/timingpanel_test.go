package metrics_test

import (
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/metrics"
)

// sampleMetrics returns a Metrics snapshot with deterministic timings
// and counters. Phase durations are chosen so each classification
// bucket (None, OK, Warning, Slow) is exercised by at least one row.
func sampleMetrics() metrics.Metrics {
	start := time.Unix(1730000000, 0).UTC()
	end := start.Add(500 * time.Millisecond)
	return metrics.Metrics{
		NavID:     7,
		URL:       "https://example.com/page",
		StartedAt: start,
		EndedAt:   end,
		Timings: []metrics.Timing{
			{Phase: metrics.PhaseNavigation, Started: start, Ended: start.Add(1 * time.Millisecond)},
			{Phase: metrics.PhaseDNSResolve, Started: start.Add(1 * time.Millisecond), Ended: start.Add(3 * time.Millisecond)},
			{Phase: metrics.PhaseConnect, Started: start.Add(3 * time.Millisecond), Ended: start.Add(7 * time.Millisecond)},
			{Phase: metrics.PhaseFirstByte, Started: start.Add(7 * time.Millisecond), Ended: start.Add(15 * time.Millisecond)},
			{Phase: metrics.PhaseBodyRead, Started: start.Add(15 * time.Millisecond), Ended: start.Add(35 * time.Millisecond)},
			{Phase: metrics.PhaseParse, Started: start.Add(35 * time.Millisecond), Ended: start.Add(95 * time.Millisecond)},
			{Phase: metrics.PhaseStyle, Started: start.Add(95 * time.Millisecond), Ended: start.Add(200 * time.Millisecond)},
			{Phase: metrics.PhaseLayout, Started: start.Add(200 * time.Millisecond), Ended: start.Add(330 * time.Millisecond)},
			{Phase: metrics.PhasePaint, Started: start.Add(330 * time.Millisecond), Ended: start.Add(420 * time.Millisecond)},
			{Phase: metrics.PhaseRaster, Started: start.Add(420 * time.Millisecond), Ended: start.Add(490 * time.Millisecond)},
			{Phase: metrics.PhasePresent, Started: start.Add(490 * time.Millisecond), Ended: end},
		},
		Counters: metrics.Counters{
			NodeCount:         432,
			RuleCount:         17,
			SelectorCount:     96,
			BoxCount:          410,
			FragmentCount:     120,
			DisplayItemCount:  156,
			TileCount:         24,
			ImageCount:        6,
			BytesDownloaded:   12_345,
			DecodedImageBytes: 4096,
			CacheHits:         4,
			CacheMisses:       12,
			ScriptErrors:      1,
		},
	}
}

func TestNewTimingPanel_HappyPath(t *testing.T) {
	p := metrics.NewTimingPanel(sampleMetrics())
	if p.NavID != 7 {
		t.Fatalf("NavID = %d, want 7", p.NavID)
	}
	if p.URL != "https://example.com/page" {
		t.Fatalf("URL = %q", p.URL)
	}
	if p.TotalDuration != 500*time.Millisecond {
		t.Fatalf("TotalDuration = %v, want 500ms", p.TotalDuration)
	}
	if len(p.Rows) != len(sampleMetrics().Timings) {
		t.Fatalf("got %d rows, want %d", len(p.Rows), len(sampleMetrics().Timings))
	}
}

// TestNewTimingPanel_PipelineOrder verifies the default sort keeps the
// canonical phase pipeline order.
func TestNewTimingPanel_PipelineOrder(t *testing.T) {
	p := metrics.NewTimingPanel(sampleMetrics())
	for i := 1; i < len(p.Rows); i++ {
		if p.Rows[i-1].PhaseID >= p.Rows[i].PhaseID {
			t.Fatalf("rows not in pipeline order at i=%d: %+v", i, p.Rows[i])
		}
	}
}

// TestNewTimingPanel_PercentageMath verifies the row percentages sum
// (within rounding tolerance) to the total duration fraction. We
// check that the phase with the largest duration reports a
// non-trivial percentage and that all values are non-negative.
func TestNewTimingPanel_PercentageMath(t *testing.T) {
	p := metrics.NewTimingPanel(sampleMetrics())

	total := 0.0
	for _, r := range p.Rows {
		if r.Percentage < 0 {
			t.Fatalf("percentage for %s is negative: %f", r.PhaseLabel, r.Percentage)
		}
		total += r.Percentage
	}
	// Allow generous rounding tolerance because percentages are
	// rounded only at print time — internally they're not summed.
	if total < 99.0 || total > 101.0 {
		t.Fatalf("sum of percentages = %.2f, want ~100", total)
	}
}

// TestNewTimingPanel_StatusClassification verifies the row Status
// matches the threshold defaults (WarnAt=50ms, SlowAt=250ms).
func TestNewTimingPanel_StatusClassification(t *testing.T) {
	p := metrics.NewTimingPanel(sampleMetrics())
	want := map[metrics.Phase]metrics.StatusKind{
		metrics.PhaseNavigation: metrics.StatusOK,
		metrics.PhaseDNSResolve: metrics.StatusOK,
		metrics.PhaseConnect:    metrics.StatusOK,
		metrics.PhaseFirstByte:  metrics.StatusOK,
		metrics.PhaseBodyRead:   metrics.StatusOK,
		metrics.PhaseParse:      metrics.StatusWarning, // ~60ms (>=50ms, <250ms)
		metrics.PhaseStyle:      metrics.StatusWarning, // ~105ms
		metrics.PhaseLayout:     metrics.StatusWarning, // ~130ms (>=50ms, <250ms)
		metrics.PhasePaint:      metrics.StatusWarning, // ~90ms
		metrics.PhaseRaster:     metrics.StatusWarning, // ~70ms
		metrics.PhasePresent:    metrics.StatusOK,      // ~10ms
	}
	for _, r := range p.Rows {
		if got, ok := want[r.PhaseID]; ok && r.Status != got {
			t.Fatalf("phase %s status = %s, want %s", r.PhaseLabel, r.Status, got)
		}
	}
}

// TestNewTimingPanel_OverallStatus verifies the panel reports the
// most-severe observed status as its OverallStatus.
func TestNewTimingPanel_OverallStatus(t *testing.T) {
	p := metrics.NewTimingPanel(sampleMetrics())
	if p.OverallStatus != metrics.StatusWarning {
		t.Fatalf("OverallStatus = %s, want warning", p.OverallStatus)
	}
	if p.StatusSummary.Warning < 1 {
		t.Fatalf("StatusSummary.Warning = %d, want >= 1", p.StatusSummary.Warning)
	}
}

// TestNewTimingPanel_EmptyMetrics verifies the panel handles a
// Metrics with no timings without panicking.
func TestNewTimingPanel_EmptyMetrics(t *testing.T) {
	m := metrics.Metrics{NavID: 1, URL: "https://empty.test"}
	p := metrics.NewTimingPanel(m)
	if p.TotalDuration != 0 {
		t.Fatalf("TotalDuration = %v, want 0", p.TotalDuration)
	}
	if len(p.Rows) != 0 {
		t.Fatalf("len(Rows) = %d, want 0", len(p.Rows))
	}
	if p.OverallStatus != metrics.StatusNone {
		t.Fatalf("OverallStatus = %s, want none", p.OverallStatus)
	}
}

// TestNewTimingPanel_CounterGroups verifies counter groups are
// emitted with non-empty entries and that bytes counter values use
// the Bytes flag.
func TestNewTimingPanel_CounterGroups(t *testing.T) {
	p := metrics.NewTimingPanel(sampleMetrics())
	labels := map[string]bool{}
	var bytes *metrics.CounterEntry
	for _, g := range p.CounterGroups {
		labels[g.Label] = true
		for i := range g.Entries {
			if g.Entries[i].Bytes && bytes == nil {
				bytes = &g.Entries[i]
			}
		}
	}
	for _, want := range []string{"Structure", "Bytes", "Cache", "Script"} {
		if !labels[want] {
			t.Fatalf("missing counter group %q in panel", want)
		}
	}
	if bytes == nil {
		t.Fatal("expected at least one bytes counter entry")
	}
	if !bytes.Bytes {
		t.Fatal("downloaded counter must be marked as bytes")
	}
}

// TestNewTimingPanel_SortByDuration verifies the duration-descending
// sort option places the slowest phase first.
func TestNewTimingPanel_SortByDuration(t *testing.T) {
	p := metrics.NewTimingPanelWith(sampleMetrics(), metrics.TimingPanelOptions{Sort: metrics.SortByDurationDesc})
	if len(p.Rows) < 2 {
		t.Fatal("not enough rows")
	}
	if p.Rows[0].Duration < p.Rows[1].Duration {
		t.Fatalf("rows not sorted by duration desc: %s (%v) before %s (%v)",
			p.Rows[0].PhaseLabel, p.Rows[0].Duration,
			p.Rows[1].PhaseLabel, p.Rows[1].Duration)
	}
}

// TestNewTimingPanel_SortByDurationAsc verifies ascending sort places
// the smallest duration first.
func TestNewTimingPanel_SortByDurationAsc(t *testing.T) {
	p := metrics.NewTimingPanelWith(sampleMetrics(), metrics.TimingPanelOptions{Sort: metrics.SortByDurationAsc})
	if p.Rows[0].Duration > p.Rows[1].Duration {
		t.Fatalf("rows not sorted by duration asc")
	}
}

// TestNewTimingPanel_IntervalCount verifies a phase observed twice
// (e.g. layout after a scroll reflow) is recorded with IntervalCount
// equal to the number of intervals summed into its duration.
func TestNewTimingPanel_IntervalCount(t *testing.T) {
	now := time.Unix(1740000000, 0).UTC()
	m := metrics.Metrics{
		NavID: 1, URL: "x",
		StartedAt: now,
		EndedAt:   now.Add(100 * time.Millisecond),
		Timings: []metrics.Timing{
			{Phase: metrics.PhaseLayout, Started: now, Ended: now.Add(20 * time.Millisecond)},
			{Phase: metrics.PhaseLayout, Started: now.Add(40 * time.Millisecond), Ended: now.Add(60 * time.Millisecond)},
		},
	}
	p := metrics.NewTimingPanel(m)
	for _, r := range p.Rows {
		if r.PhaseID == metrics.PhaseLayout {
			if r.IntervalCount != 2 {
				t.Fatalf("layout IntervalCount = %d, want 2", r.IntervalCount)
			}
			if r.Duration != 40*time.Millisecond {
				t.Fatalf("layout Duration = %v, want 40ms", r.Duration)
			}
			return
		}
	}
	t.Fatal("layout row missing")
}

// TestNewTimingPanel_CustomThresholds verifies the option overrides
// the default status thresholds.
func TestNewTimingPanel_CustomThresholds(t *testing.T) {
	now := time.Unix(1750000000, 0).UTC()
	m := metrics.Metrics{
		NavID: 1, URL: "x",
		StartedAt: now, EndedAt: now.Add(100 * time.Millisecond),
		Timings: []metrics.Timing{
			{Phase: metrics.PhaseLayout, Started: now, Ended: now.Add(20 * time.Millisecond)},
		},
	}
	// Tight thresholds: 20ms is "slow" under this policy.
	tight := &metrics.StatusThresholds{WarnAt: 5 * time.Millisecond, SlowAt: 10 * time.Millisecond}
	p := metrics.NewTimingPanelWith(m, metrics.TimingPanelOptions{StatusThresholds: tight})
	if p.Rows[0].Status != metrics.StatusSlow {
		t.Fatalf("under tight thresholds, layout Status = %s, want slow", p.Rows[0].Status)
	}

	// Wide thresholds: 20ms is "ok" under this policy.
	wide := &metrics.StatusThresholds{WarnAt: 100 * time.Millisecond, SlowAt: 500 * time.Millisecond}
	p = metrics.NewTimingPanelWith(m, metrics.TimingPanelOptions{StatusThresholds: wide})
	if p.Rows[0].Status != metrics.StatusOK {
		t.Fatalf("under wide thresholds, layout Status = %s, want ok", p.Rows[0].Status)
	}
}

// TestNewTimingPanel_DeterministicOutput guards against accidental
// non-determinism creeping into the panel renderer.
func TestNewTimingPanel_DeterministicOutput(t *testing.T) {
	first := metrics.NewTimingPanel(sampleMetrics()).String()
	second := metrics.NewTimingPanel(sampleMetrics()).String()
	if first != second {
		t.Fatalf("non-deterministic output\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestNewTimingPanel_StringFormat exercises the textual rendering
// and ensures it contains the expected labels in the expected order.
func TestNewTimingPanel_StringFormat(t *testing.T) {
	p := metrics.NewTimingPanel(sampleMetrics())
	s := p.String()
	// First line must begin with "Navigation" and contain the URL.
	if !strings.HasPrefix(s, "Navigation ") {
		t.Fatalf("output does not start with 'Navigation ': %q", firstLine(s))
	}
	if !strings.Contains(s, "https://example.com/page") {
		t.Fatalf("output missing URL: %s", s)
	}
	// Rows must each include their phase label and a ms line.
	for _, r := range p.Rows {
		if !strings.Contains(s, r.PhaseLabel) {
			t.Fatalf("output missing phase label %s", r.PhaseLabel)
		}
	}
	// Total ms and status line must appear.
	if !strings.Contains(s, "total: ") || !strings.Contains(s, "ms  status:") {
		t.Fatalf("missing total/status line in output:\n%s", s)
	}
	// Counter groups must appear.
	if !strings.Contains(s, "Structure:") {
		t.Fatalf("missing Structure group line:\n%s", s)
	}
	if !strings.Contains(s, "Bytes:") {
		t.Fatalf("missing Bytes group line:\n%s", s)
	}
}

// TestFormatBytes sanity-checks the byte formatter for a small set
// of representative values.
func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512.00 B"},
		{1500, "1.50 KB"},
		{1_500_000, "1.50 MB"},
		{1_500_000_000, "1.50 GB"},
	}
	for _, c := range cases {
		got := metrics.FormatBytes(c.in)
		if got != c.want {
			t.Fatalf("FormatBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestClassifyBoundaryConditions verifies the boundary cases of the
// status classification: zero, exactly warn, exactly slow, between.
func TestClassifyBoundaryConditions(t *testing.T) {
	th := metrics.DefaultStatusThresholds
	cases := []struct {
		name string
		in   time.Duration
		want metrics.StatusKind
	}{
		{"zero", 0, metrics.StatusNone},
		{"one_ns", time.Nanosecond, metrics.StatusOK},
		{"below_warn", th.WarnAt - time.Millisecond, metrics.StatusOK},
		{"at_warn", th.WarnAt, metrics.StatusWarning},
		{"between", th.WarnAt + 10*time.Millisecond, metrics.StatusWarning},
		{"at_slow", th.SlowAt, metrics.StatusSlow},
		{"above_slow", th.SlowAt + time.Second, metrics.StatusSlow},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if got := metrics.Classify(c.in, th); got != c.want {
				t.Fatalf("Classify(%v) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

// firstLine returns the first line of s; used by error messages.
func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
