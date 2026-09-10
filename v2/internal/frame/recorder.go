package frame

import (
	"encoding/json"
	"io"
	"math"
	"sort"
	"time"
)

// FrameMark is one frame's timing and work profile. The five timestamps are the
// stages the frame budget is divided between in the design, so a regression can
// be attributed to a stage instead of to "the frame got slower". The counters
// turn the architectural invariants into assertions: StylePasses and
// LayoutPasses must be zero on a scroll-only frame, and TilesRasterized must be
// zero once the tile cache is warm.
type FrameMark struct {
	Serial          uint64    `json:"serial"`
	VsyncAt         time.Time `json:"vsync_at_ns"`
	PlanAt          time.Time `json:"plan_at_ns"`
	SubmitAt        time.Time `json:"submit_at_ns"`
	ComposedAt      time.Time `json:"composed_at_ns"`
	PresentedAt     time.Time `json:"presented_at_ns"`
	TilesRasterized int32     `json:"tiles_rasterized"`
	TilesReused     int32     `json:"tiles_reused"`
	StylePasses     int32     `json:"style_passes"`
	LayoutPasses    int32     `json:"layout_passes"`
}

// Work returns vsync-to-present, the interval the 60fps gate is measured on.
func (m FrameMark) Work() time.Duration {
	if m.VsyncAt.IsZero() || m.PresentedAt.IsZero() {
		return 0
	}
	return m.PresentedAt.Sub(m.VsyncAt)
}

// Report summarizes a recording window. Latency is reported in milliseconds
// because the consumers are a shell script gate and a text baseline file.
type Report struct {
	Frames          int64   `json:"frames"`
	MeanMS          float64 `json:"mean_ms"`
	P50MS           float64 `json:"p50_ms"`
	P99MS           float64 `json:"p99_ms"`
	MaxMS           float64 `json:"max_ms"`
	TilesRasterized int64   `json:"tiles_rasterized"`
	TilesReused     int64   `json:"tiles_reused"`
	StylePasses     int64   `json:"style_passes"`
	LayoutPasses    int64   `json:"layout_passes"`
	ZeroWorkFrames  int64   `json:"zero_work_frames"`
}

// FrameRecorder keeps the most recent capacity marks in a ring buffer. Record is
// allocation-free after construction, which matters because it is called from
// the frame path the recorder is measuring.
type FrameRecorder struct {
	ring   []FrameMark
	next   int
	filled int
	count  int64
}

// NewFrameRecorder returns a recorder retaining capacity marks.
func NewFrameRecorder(capacity int) *FrameRecorder {
	if capacity <= 0 {
		capacity = 1024
	}
	return &FrameRecorder{ring: make([]FrameMark, capacity)}
}

// Record appends a mark, overwriting the oldest once the window is full.
func (r *FrameRecorder) Record(m FrameMark) {
	r.ring[r.next] = m
	r.next++
	if r.next == len(r.ring) {
		r.next = 0
	}
	if r.filled < len(r.ring) {
		r.filled++
	}
	r.count++
}

// Len returns the number of marks in the retained window.
func (r *FrameRecorder) Len() int { return r.filled }

// Total returns every mark recorded since construction, including those the
// window has evicted.
func (r *FrameRecorder) Total() int64 { return r.count }

// Reset empties the window.
func (r *FrameRecorder) Reset() {
	clear(r.ring)
	r.next, r.filled, r.count = 0, 0, 0
}

// Marks returns the retained window in chronological order. It allocates, so it
// belongs to reporting rather than the frame path.
func (r *FrameRecorder) Marks() []FrameMark {
	out := make([]FrameMark, 0, r.filled)
	start := 0
	if r.filled == len(r.ring) {
		start = r.next // oldest element sits at the write cursor once wrapped
	}
	for i := 0; i < r.filled; i++ {
		out = append(out, r.ring[(start+i)%len(r.ring)])
	}
	return out
}

// Report summarizes the retained window. Percentiles use nearest-rank on the
// sorted work durations, which for a 600-frame gate window is the definition
// that keeps p99 meaningful without inventing an interpolation scheme.
func (r *FrameRecorder) Report() Report {
	var rep Report
	if r.filled == 0 {
		return rep
	}
	durs := make([]float64, 0, r.filled)
	marks := r.Marks()
	for _, m := range marks {
		durs = append(durs, float64(m.Work())/float64(time.Millisecond))
		rep.TilesRasterized += int64(m.TilesRasterized)
		rep.TilesReused += int64(m.TilesReused)
		rep.StylePasses += int64(m.StylePasses)
		rep.LayoutPasses += int64(m.LayoutPasses)
		if m.TilesRasterized == 0 && m.StylePasses == 0 && m.LayoutPasses == 0 {
			rep.ZeroWorkFrames++
		}
	}
	sort.Float64s(durs)
	var sum float64
	for _, d := range durs {
		sum += d
	}
	n := len(durs)
	rank := func(q float64) float64 {
		idx := int(math.Ceil(q*float64(n))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		return durs[idx]
	}
	rep.Frames = int64(n)
	rep.MeanMS = sum / float64(n)
	rep.P50MS = rank(0.50)
	rep.P99MS = rank(0.99)
	rep.MaxMS = durs[n-1]
	return rep
}

// WriteJSON emits the report plus the per-frame marks, which is the artifact the
// gate script reads and the baseline file records.
func (r *FrameRecorder) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(struct {
		Report Report      `json:"report"`
		Frames []FrameMark `json:"frames"`
	}{Report: r.Report(), Frames: r.Marks()})
}
