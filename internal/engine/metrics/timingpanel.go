package metrics

import (
	"fmt"
	"math"
	"time"
)

// TimingPanel is a backend-neutral representation of a navigation's
// phase timing surface, intended for the developer-tools "Performance"
// tab and any external consumer (logs, JSON export, golden snapshots).
//
// The panel is a value type with no Fyne, GUI, or platform
// dependencies, so it can be constructed and asserted on in headless
// tests. UI adapters consume TimingPanel and translate the rows into
// their own widget primitives.
//
// Data layout:
//
//   - Summary describes the navigation as a whole: identifier, URL,
//     total wall-clock, top-line counters, and overall status flag.
//   - Rows contains one entry per observed Phase, sorted in a
//     deterministic order chosen by TimingPanelOptions.
//
// Stability:
//
//   - Field ordering and labels are part of the contract. Downstream
//     callers (and golden-snapshot tests for the dev-tools UI) may
//     rely on the string format produced by (*TimingPanel).String.
//     Changes to that format must be visible in the snapshot tests.
type TimingPanel struct {
	// NavID is the navigation identifier from the source Metrics.
	NavID uint64
	// URL is the navigation URL from the source Metrics.
	URL string
	// StartedAt and EndedAt are the navigation's start and end times.
	StartedAt time.Time
	EndedAt   time.Time
	// TotalDuration is wall-clock duration (EndedAt - StartedAt).
	TotalDuration time.Duration
	// Rows is the ordered list of phase rows.
	Rows []TimingPanelRow
	// CounterGroups groups the non-timing counters into human-readable
	// buckets. Each group has a label and a count total for compact
	// rendering. The grouping is intentionally fixed: structure,
	// bytes, cache, and script.
	CounterGroups []CounterGroup
	// OverallStatus is the most severe Status across all rows and
	// counter groups. UI adapters color the panel by this value.
	OverallStatus StatusKind
	// Status counts.
	StatusSummary StatusSummary
}

// TimingPanelRow describes a single phase's contribution to the
// navigation. The DurationMs and Percentage fields are pre-computed so
// UI code does not need to do math.
type TimingPanelRow struct {
	// PhaseID is the source Phase enum value.
	PhaseID Phase
	// PhaseLabel is the human-friendly phase name (e.g. "Layout").
	PhaseLabel string
	// Duration is the cumulative time spent in this phase.
	Duration time.Duration
	// DurationMs is Duration expressed in milliseconds with two
	// decimal places of precision. UI code can format directly.
	DurationMs float64
	// Percentage of TotalDuration. Values may exceed 100 in
	// pathological cases (overlapping intervals). UI code can clamp
	// for visual rendering.
	Percentage float64
	// Status is the per-row severity classification.
	Status StatusKind
	// Intervals records how many Begin/End intervals combined into
	// this row's Duration. A row produced from a single phase pass
	// has IntervalCount == 1; phases re-entered during the same
	// navigation (e.g. layout after scroll-triggered reflow) have
	// higher counts.
	IntervalCount int
}

// CounterGroup is a labeled bucket of Counters, used by the panel to
// show non-timing measurements without inflating the rows slice.
type CounterGroup struct {
	// Label is a short human description of the group.
	Label string
	// Entries are the count pairs. Empty groups are omitted by the
	// constructor.
	Entries []CounterEntry
}

// CounterEntry is one (label, value) pair inside a CounterGroup.
type CounterEntry struct {
	// Name is the human-friendly name (e.g. "Nodes").
	Name string
	// Value is the numeric value of the counter; rendered as a
	// decimal integer for whole counters and a byte-quantity for the
	// bytes fields.
	Value int64
	// Bytes marks this entry as a byte-quantity so renderers can
	// apply a "B/KB/MB" suffix. False means render the integer.
	Bytes bool
}

// StatusKind classifies a TimingPanelRow or the panel as a whole.
// Threshold defaults are exposed via DefaultStatusThresholds so they
// can be tuned without breaking callers.
type StatusKind int

const (
	// StatusNone is the zero value, used for empty phases where no
	// measurement was taken.
	StatusNone StatusKind = iota
	// StatusOK marks a phase whose duration is within the configured
	// healthy range.
	StatusOK
	// StatusWarning marks a phase whose duration is elevated but
	// not yet critical; surfaces a yellow indicator.
	StatusWarning
	// StatusSlow marks a phase whose duration exceeds the critical
	// threshold; surfaces a red indicator.
	StatusSlow
)

// String implements fmt.Stringer for diagnostic output.
func (s StatusKind) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusWarning:
		return "warning"
	case StatusSlow:
		return "slow"
	default:
		return "none"
	}
}

// StatusSummary aggregates the per-status counts so the panel can
// render a concise badge (e.g. "1 slow / 3 ok / 5 none").
type StatusSummary struct {
	OK      int
	Warning int
	Slow    int
	None    int
}

// TimingPanelOptions controls how NewTimingPanel builds the panel.
// The zero value is sensible (sort by document order, default status
// thresholds). Callers can override fields explicitly.
type TimingPanelOptions struct {
	// Sort controls the row ordering. Default: SortByDocumentOrder
	// which keeps PhaseNavigation, PhaseDNSResolve, ..., PhasePresent
	// in pipeline order.
	Sort TimingPanelSort
	// StatusThresholds tunes classification thresholds. A nil value
	// falls back to DefaultStatusThresholds.
	StatusThresholds *StatusThresholds
}

// TimingPanelSort selects the row ordering. Unknown values fall back
// to SortByDocumentOrder.
type TimingPanelSort int

const (
	// SortByDocumentOrder keeps rows in the canonical pipeline order.
	SortByDocumentOrder TimingPanelSort = iota
	// SortByDurationDesc sorts rows by Duration descending, so the
	// slowest phase appears first.
	SortByDurationDesc
	// SortByDurationAsc sorts rows by Duration ascending; useful for
	// surfacing the fastest phases.
	SortByDurationAsc
	// SortByPhaseID sorts rows by phase id ascending. Equivalent to
	// SortByDocumentOrder but explicit.
	SortByPhaseID
)

// StatusThresholds tunes the policy that classifies a phase's
// duration as ok / warning / slow. Zero values use the defaults.
type StatusThresholds struct {
	// WarnAt is the duration at which a row crosses into warning.
	WarnAt time.Duration
	// SlowAt is the duration at which a row crosses into slow.
	SlowAt time.Duration
}

// DefaultStatusThresholds are the engine-default warning/slow
// thresholds applied when the caller does not override them.
//
// The defaults are intentionally conservative: an isolated script
// run on the reference machine clears all phases well under
// WarnAt, while DocumentOrder-class interactive loads typically
// sit between WarnAt and SlowAt. Tuned for the contributor
// workstation (see ARCHITECTURE.md and PERFORMANCE.md).
var DefaultStatusThresholds = StatusThresholds{
	WarnAt: 50 * time.Millisecond,
	SlowAt: 250 * time.Millisecond,
}

// NewTimingPanel constructs a TimingPanel from a Metrics snapshot.
// The function is pure: identical input produces identical output
// (determinism is asserted by the regression tests).
func NewTimingPanel(m Metrics) TimingPanel {
	return NewTimingPanelWith(m, TimingPanelOptions{})
}

// NewTimingPanelWith constructs a TimingPanel from a Metrics snapshot
// using the supplied options.
func NewTimingPanelWith(m Metrics, opts TimingPanelOptions) TimingPanel {
	thresholds := DefaultStatusThresholds
	if opts.StatusThresholds != nil {
		thresholds = *opts.StatusThresholds
	}

	// Aggregate per-phase durations and interval counts in source
	// order so percentages can be computed against the canonical
	// navigation total.
	phaseDurations := make(map[Phase]time.Duration, 11)
	phaseIntervals := make(map[Phase]int, 11)
	for _, t := range m.Timings {
		phaseDurations[t.Phase] += t.Duration()
		phaseIntervals[t.Phase]++
	}

	rows := make([]TimingPanelRow, 0, len(phaseDurations))
	for phase, dur := range phaseDurations {
		rows = append(rows, TimingPanelRow{
			PhaseID:       phase,
			PhaseLabel:    phase.String(),
			Duration:      dur,
			DurationMs:    roundMs(dur),
			Percentage:    percentage(dur, m.TotalDuration()),
			Status:        classify(dur, thresholds),
			IntervalCount: phaseIntervals[phase],
		})
	}

	// Apply ordering option.
	switch opts.Sort {
	case SortByDurationDesc:
		sortRowsByDuration(rows, false)
	case SortByDurationAsc:
		sortRowsByDuration(rows, true)
	case SortByPhaseID, SortByDocumentOrder:
		sortRowsByPhaseID(rows)
	}

	groups := buildCounterGroups(m.Counters)

	overall := StatusNone
	var summary StatusSummary
	for _, r := range rows {
		switch r.Status {
		case StatusOK:
			summary.OK++
		case StatusWarning:
			summary.Warning++
		case StatusSlow:
			summary.Slow++
		default:
			summary.None++
		}
		if rank(r.Status) > rank(overall) {
			overall = r.Status
		}
	}
	// Counter bytes or cache misses that are non-zero quietly raise
	// the overall status to "warning" so a high-latency load with
	// zero measured phases still surfaces useful signal. We only
	// count groups that actually have visible entries — empty
	// CounterGroup shells (zero counts) are not meaningful signal.
	nonEmpty := false
	for _, g := range groups {
		if len(g.Entries) > 0 {
			nonEmpty = true
			break
		}
	}
	if overall == StatusNone && (nonEmpty || len(rows) > 0) {
		overall = StatusOK
		summary.OK++
	}
	if overall == StatusNone {
		summary.None++
	}

	return TimingPanel{
		NavID:         m.NavID,
		URL:           m.URL,
		StartedAt:     m.StartedAt,
		EndedAt:       m.EndedAt,
		TotalDuration: m.TotalDuration(),
		Rows:          rows,
		CounterGroups: groups,
		OverallStatus: overall,
		StatusSummary: summary,
	}
}

// buildCounterGroups converts a Counters value into the panel's
// pre-grouped representation. The grouping logic is intentionally
// hard-coded so the panel surface is stable across releases.
func buildCounterGroups(c Counters) []CounterGroup {
	structure := []CounterEntry{
		{Name: "Nodes", Value: int64(c.NodeCount)},
		{Name: "Rules", Value: int64(c.RuleCount)},
		{Name: "Selectors", Value: int64(c.SelectorCount)},
		{Name: "Boxes", Value: int64(c.BoxCount)},
		{Name: "Fragments", Value: int64(c.FragmentCount)},
		{Name: "Display items", Value: int64(c.DisplayItemCount)},
		{Name: "Tiles", Value: int64(c.TileCount)},
		{Name: "Images", Value: int64(c.ImageCount)},
	}
	bytes := []CounterEntry{
		{Name: "Downloaded", Value: c.BytesDownloaded, Bytes: true},
		{Name: "Decoded image", Value: c.DecodedImageBytes, Bytes: true},
	}
	cache := []CounterEntry{
		{Name: "Hits", Value: int64(c.CacheHits)},
		{Name: "Misses", Value: int64(c.CacheMisses)},
	}
	script := []CounterEntry{
		{Name: "Errors", Value: int64(c.ScriptErrors)},
	}
	return []CounterGroup{
		{Label: "Structure", Entries: dropEmpty(structure)},
		{Label: "Bytes", Entries: dropEmpty(bytes)},
		{Label: "Cache", Entries: dropEmpty(cache)},
		{Label: "Script", Entries: dropEmpty(script)},
	}
}

// dropEmpty omits zero-valued entries so the panel only shows
// measurements actually taken. Empty groups still pass through the
// caller, which filters them out later when rendering.
func dropEmpty(in []CounterEntry) []CounterEntry {
	out := in[:0]
	for _, e := range in {
		if e.Value != 0 {
			out = append(out, e)
		}
	}
	return out
}

// classify maps a duration to a StatusKind using the supplied
// thresholds.
func classify(d time.Duration, t StatusThresholds) StatusKind {
	switch {
	case d <= 0:
		return StatusNone
	case d >= t.SlowAt:
		return StatusSlow
	case d >= t.WarnAt:
		return StatusWarning
	default:
		return StatusOK
	}
}

// rank returns a comparable integer for a StatusKind so the panel
// can pick the most severe observed status.
func rank(s StatusKind) int {
	switch s {
	case StatusSlow:
		return 3
	case StatusWarning:
		return 2
	case StatusOK:
		return 1
	default:
		return 0
	}
}

// percentage returns (partial / total) * 100, or zero when total
// is non-positive. The caller is responsible for clamping visual
// rendering at 100.
func percentage(partial, total time.Duration) float64 {
	if total <= 0 {
		return 0
	}
	return float64(partial) / float64(total) * 100
}

// roundMs rounds d to two decimal places in milliseconds. We avoid
// math.Round to keep allocations minimal.
func roundMs(d time.Duration) float64 {
	ms := float64(d) / float64(time.Millisecond)
	// Multiply by 100, truncate toward zero.
	return math.Round(ms*100) / 100
}

// sortRowsByPhaseID sorts rows by PhaseID ascending (pipeline order).
func sortRowsByPhaseID(rows []TimingPanelRow) {
	// Insertion sort — list is small (<= 11 phases).
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j-1].PhaseID > rows[j].PhaseID; j-- {
			rows[j-1], rows[j] = rows[j], rows[j-1]
		}
	}
}

// sortRowsByDuration sorts rows by Duration in either direction. The
// primary key is Duration; the secondary key is PhaseID so
// equal-duration rows remain in pipeline order.
//
// asc=true sorts smallest first; asc=false sorts largest first.
func sortRowsByDuration(rows []TimingPanelRow, asc bool) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0; j-- {
			outOfOrder := false
			if asc {
				outOfOrder = rows[j-1].Duration > rows[j].Duration ||
					(rows[j-1].Duration == rows[j].Duration && rows[j-1].PhaseID > rows[j].PhaseID)
			} else {
				outOfOrder = rows[j-1].Duration < rows[j].Duration ||
					(rows[j-1].Duration == rows[j].Duration && rows[j-1].PhaseID > rows[j].PhaseID)
			}
			if !outOfOrder {
				break
			}
			rows[j-1], rows[j] = rows[j], rows[j-1]
		}
	}
}

// labelNav is the human-readable navigation label used in the panel's
// textual rendering. UI adapters may translate this in their own
// localization layer.
const labelNav = "Navigation"

// String returns a stable, human-readable rendering of the panel,
// suitable for golden snapshots and structured logs.
//
// The format is:
//
//	Navigation <id>  <url>
//	  total: <ms> ms   status: <status>
//	  <phase>: <ms> ms (<pct>%) [<count>x] <status>
//	  ...
//	  <group>: <name>=<value>(B), ...
//
// Field order and labels are part of the contract: changes must be
// reflected in the regression tests under internal/engine/metrics.
func (p TimingPanel) String() string {
	var b fmtB
	b.f("%s %d  %s\n", labelNav, p.NavID, p.URL)
	b.f("  nav_id=%d\n", p.NavID)
	b.f("  total: %.2f ms  status: %s\n", roundMs(p.TotalDuration), p.OverallStatus)
	for _, r := range p.Rows {
		intervals := ""
		if r.IntervalCount != 1 {
			intervals = fmt.Sprintf(" [%dx]", r.IntervalCount)
		}
		b.f("  %-12s %8.2f ms (%5.1f%%)%s  %s\n",
			r.PhaseLabel, r.DurationMs, r.Percentage, intervals, r.Status)
	}
	for _, g := range p.CounterGroups {
		if len(g.Entries) == 0 {
			continue
		}
		b.f("  %s: ", g.Label)
		for i, e := range g.Entries {
			if i > 0 {
				b.f(", ")
			}
			b.f("%s=%s", e.Name, formatCounterValue(e))
		}
		b.f("\n")
	}
	return b.s
}

// formatCounterValue renders a CounterEntry as a human string. Byte
// values use "B / KB / MB / GB" suffixes; integer counters print
// verbatim.
func formatCounterValue(e CounterEntry) string {
	if !e.Bytes {
		return fmt.Sprintf("%d", e.Value)
	}
	return formatBytes(e.Value)
}

// formatBytes prints the byte count using decimal SI suffixes. The
// goal is readability, not byte-exact formatting. Returns "0 B" for
// zero or negative values.
func formatBytes(b int64) string {
	const step = 1000
	if b <= 0 {
		return "0 B"
	}
	abs := float64(b)
	units := []string{"B", "KB", "MB", "GB"}
	idx := 0
	for abs >= step && idx < len(units)-1 {
		abs /= step
		idx++
	}
	return fmt.Sprintf("%.2f %s", abs, units[idx])
}

// fmtB is a tiny strings.Builder helper kept private to this file.
type fmtB struct{ s string }

func (b *fmtB) f(format string, args ...any) {
	b.s += fmt.Sprintf(format, args...)
}
