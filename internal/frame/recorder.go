package frame

import (
	"encoding/json"
	"io"
	"math"
	"sort"
	"time"
)

type FrameMark struct {
	Serial          uint64    `json:"serial"`
	VsyncAt         time.Time `json:"vsync_at"`
	PlanAt          time.Time `json:"plan_at"`
	SubmitAt        time.Time `json:"submit_at"`
	ComposedAt      time.Time `json:"composed_at"`
	PresentedAt     time.Time `json:"presented_at"`
	TilesRasterized int32     `json:"tiles_rasterized"`
	TilesReused     int32     `json:"tiles_reused"`
	StylePasses     int32     `json:"style_passes"`
	LayoutPasses    int32     `json:"layout_passes"`
}

func (m FrameMark) Work() time.Duration {
	if m.VsyncAt.IsZero() || m.PresentedAt.IsZero() {
		return 0
	}
	return m.PresentedAt.Sub(m.VsyncAt)
}

func (m FrameMark) Present() time.Duration {
	if m.ComposedAt.IsZero() || m.PresentedAt.IsZero() {
		return 0
	}
	return m.PresentedAt.Sub(m.ComposedAt)
}

type Report struct {
	Frames          int64    `json:"frames"`
	MeanMS          float64  `json:"mean_ms"`
	P50MS           float64  `json:"p50_ms"`
	P99MS           float64  `json:"p99_ms"`
	MaxMS           float64  `json:"max_ms"`
	PresentMeanMS   float64  `json:"present_mean_ms"`
	PresentP99MS    float64  `json:"present_p99_ms"`
	TilesRasterized int64    `json:"tiles_rasterized"`
	TilesReused     int64    `json:"tiles_reused"`
	StylePasses     int64    `json:"style_passes"`
	LayoutPasses    int64    `json:"layout_passes"`
	ZeroWorkFrames  int64    `json:"zero_work_frames"`
	StartupMS       *float64 `json:"startup_ms"`
	TileBytes       *int64   `json:"tile_bytes"`
	TileBudget      *int64   `json:"tile_budget"`
	TileEvictions   *int64   `json:"tile_evictions"`
	Document        string   `json:"document"`
	DocHeight       int64    `json:"doc_height"`
}

type FrameRecorder struct {
	ring   []FrameMark
	next   int
	filled int
	count  int64

	startupRef     time.Time
	firstPresented time.Time

	tiles *GridStats

	content   string
	docHeight int64
}

func NewFrameRecorder(capacity int) *FrameRecorder {
	if capacity <= 0 {
		capacity = 1024
	}
	return &FrameRecorder{ring: make([]FrameMark, capacity)}
}

func (r *FrameRecorder) SetStartupReference(t time.Time) { r.startupRef = t }

func (r *FrameRecorder) SetTileLedger(g GridStats) { r.tiles = &g }

func (r *FrameRecorder) SetContent(document string, docHeight int64) {
	r.content, r.docHeight = document, docHeight
}

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
	if r.firstPresented.IsZero() && !m.PresentedAt.IsZero() {
		r.firstPresented = m.PresentedAt
	}
}

func (r *FrameRecorder) Len() int { return r.filled }

func (r *FrameRecorder) Total() int64 { return r.count }

func (r *FrameRecorder) Reset() {
	clear(r.ring)
	r.next, r.filled, r.count = 0, 0, 0
	r.firstPresented = time.Time{}
}

func (r *FrameRecorder) Marks() []FrameMark {
	out := make([]FrameMark, 0, r.filled)
	start := 0
	if r.filled == len(r.ring) {
		start = r.next
	}
	for i := 0; i < r.filled; i++ {
		out = append(out, r.ring[(start+i)%len(r.ring)])
	}
	return out
}

func (r *FrameRecorder) Report() Report {
	var rep Report
	rep.Document, rep.DocHeight = r.content, r.docHeight
	if r.filled == 0 {
		return rep
	}
	durs := make([]float64, 0, r.filled)
	presents := make([]float64, 0, r.filled)
	marks := r.Marks()
	for _, m := range marks {
		durs = append(durs, float64(m.Work())/float64(time.Millisecond))
		if !m.ComposedAt.IsZero() && !m.PresentedAt.IsZero() {
			presents = append(presents, float64(m.Present())/float64(time.Millisecond))
		}
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
	rank := func(sorted []float64, q float64) float64 {
		m := len(sorted)
		idx := int(math.Ceil(q*float64(m))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= m {
			idx = m - 1
		}
		return sorted[idx]
	}
	rep.Frames = int64(n)
	rep.MeanMS = sum / float64(n)
	rep.P50MS = rank(durs, 0.50)
	rep.P99MS = rank(durs, 0.99)
	rep.MaxMS = durs[n-1]
	if len(presents) > 0 {
		sort.Float64s(presents)
		var psum float64
		for _, d := range presents {
			psum += d
		}
		rep.PresentMeanMS = psum / float64(len(presents))
		rep.PresentP99MS = rank(presents, 0.99)
	}
	if !r.startupRef.IsZero() && !r.firstPresented.IsZero() {
		startup := float64(r.firstPresented.Sub(r.startupRef)) / float64(time.Millisecond)
		rep.StartupMS = &startup
	}
	if r.tiles != nil {
		held, budget, evictions := r.tiles.Bytes, r.tiles.Budget, r.tiles.Evictions
		rep.TileBytes, rep.TileBudget, rep.TileEvictions = &held, &budget, &evictions
	}
	return rep
}

func (r *FrameRecorder) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(struct {
		Report Report      `json:"report"`
		Frames []FrameMark `json:"frames"`
	}{Report: r.Report(), Frames: r.Marks()})
}
