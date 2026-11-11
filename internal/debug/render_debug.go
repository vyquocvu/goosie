// Package debug provides rendering pipeline debugging and visualization tools.
//
// Debug tracks frame-by-frame rendering work: style resolutions, layout builds,
// display list rebuilds, tile invalidations, and damage rects. It logs
// interactions (hover, click, select) and what pipeline stages they trigger.
// Visual overlays draw damage rects, tile boundaries, and hover areas on screen.
package debug

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
)

// Frame records one frame's worth of pipeline work.
type Frame struct {
	Serial          uint64
	Timestamp       time.Time
	Duration        time.Duration
	StylePasses     int
	LayoutPasses    int
	PaintCalls      int
	TilesReused     int
	TilesRasterized int
	DamageRects     []frame.Rect
	Reason          string // "scroll", "hover", "click", "select", "resize", "js"
}

// Interaction records one user interaction and its pipeline effects.
type Interaction struct {
	Type            string // "hover", "click", "select", "scroll", "key"
	Timestamp       time.Time
	X, Y            float32
	TargetNode      string
	TriggeredStyle  bool
	TriggeredLayout bool
	TriggeredPaint  bool
	DamageRect      frame.Rect
}

// Stats holds cumulative rendering statistics.
type Stats struct {
	TotalFrames          uint64
	TotalStylePasses     uint64
	TotalLayoutPasses    uint64
	TotalPaintCalls      uint64
	TotalTilesReused     uint64
	TotalTilesRasterized uint64
	TotalInteractions    uint64

	StylePerFrame  float64
	LayoutPerFrame float64
	PaintPerFrame  float64

	AvgFrameDuration time.Duration
	MaxFrameDuration time.Duration

	mu sync.Mutex
}

// Renderer is the debug visualizer and logger.
type Renderer struct {
	Enabled     bool
	LogToFile   bool
	LogPath     string
	ShowOverlay bool
	ShowTiles   bool
	ShowDamage  bool
	ShowHover   bool

	stats        Stats
	frames       []Frame
	interactions []Interaction
	frameIdx     int

	logFile *os.File
	mu      sync.Mutex
}

// NewRenderer returns a debug renderer with the given settings.
func NewRenderer(enabled bool) *Renderer {
	r := &Renderer{
		Enabled:      enabled,
		ShowOverlay:  enabled,
		ShowTiles:    enabled,
		ShowDamage:   enabled,
		ShowHover:    enabled,
		frames:       make([]Frame, 0, 1000),
		interactions: make([]Interaction, 0, 100),
	}
	if enabled && r.LogToFile {
		r.openLog()
	}
	return r
}

// openLog opens the log file for writing.
func (r *Renderer) openLog() {
	if r.LogPath == "" {
		r.LogPath = "/tmp/goosie-debug.log"
	}
	f, err := os.Create(r.LogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "debug: cannot open log %s: %v\n", r.LogPath, err)
		return
	}
	r.logFile = f
	fmt.Fprintf(f, "# Goosie Debug Log\n")
	fmt.Fprintf(f, "# Started: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "#\n")
}

// log writes a message to the log file if enabled.
func (r *Renderer) log(format string, args ...interface{}) {
	if !r.LogToFile || r.logFile == nil {
		return
	}
	fmt.Fprintf(r.logFile, format+"\n", args...)
}

// BeginFrame starts timing a new frame.
func (r *Renderer) BeginFrame(serial uint64, reason string) {
	if !r.Enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.frameIdx >= len(r.frames) {
		r.frames = append(r.frames, Frame{})
	}
	f := &r.frames[r.frameIdx]
	f.Serial = serial
	f.Timestamp = time.Now()
	f.Reason = reason
	f.DamageRects = f.DamageRects[:0]
}

// EndFrame finishes timing the current frame and records stats.
func (r *Renderer) EndFrame(stylePasses, layoutPasses, paintCalls, tilesReused, tilesRasterized int, damage []frame.Rect) {
	if !r.Enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.frameIdx >= len(r.frames) {
		return
	}
	f := &r.frames[r.frameIdx]
	f.Duration = time.Since(f.Timestamp)
	f.StylePasses = stylePasses
	f.LayoutPasses = layoutPasses
	f.PaintCalls = paintCalls
	f.TilesReused = tilesReused
	f.TilesRasterized = tilesRasterized
	f.DamageRects = append(f.DamageRects[:0], damage...)

	// Update cumulative stats
	r.stats.TotalFrames++
	r.stats.TotalStylePasses += uint64(stylePasses)
	r.stats.TotalLayoutPasses += uint64(layoutPasses)
	r.stats.TotalPaintCalls += uint64(paintCalls)
	r.stats.TotalTilesReused += uint64(tilesReused)
	r.stats.TotalTilesRasterized += uint64(tilesRasterized)

	if f.Duration > r.stats.MaxFrameDuration {
		r.stats.MaxFrameDuration = f.Duration
	}

	// Log frame
	r.log("FRAME %d: reason=%s dur=%v style=%d layout=%d paint=%d tiles_reused=%d tiles_raster=%d damage_rects=%d",
		f.Serial, f.Reason, f.Duration, stylePasses, layoutPasses, paintCalls,
		tilesReused, tilesRasterized, len(damage))

	r.frameIdx++
}

// LogInteraction records a user interaction.
func (r *Renderer) LogInteraction(typ string, x, y float32, target string, style, layout, paint bool, damage frame.Rect) {
	if !r.Enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stats.TotalInteractions++

	interaction := Interaction{
		Type:            typ,
		Timestamp:       time.Now(),
		X:               x,
		Y:               y,
		TargetNode:      target,
		TriggeredStyle:  style,
		TriggeredLayout: layout,
		TriggeredPaint:  paint,
		DamageRect:      damage,
	}

	if len(r.interactions) >= cap(r.interactions) {
		// Drop oldest
		r.interactions = r.interactions[1:]
	}
	r.interactions = append(r.interactions, interaction)

	r.log("INTERACTION: type=%s target=%s pos=(%.0f,%.0f) style=%v layout=%v paint=%v damage=%v",
		typ, target, x, y, style, layout, paint, damage)
}

// Stats returns a snapshot of cumulative statistics.
func (r *Renderer) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()

	s := r.stats
	if s.TotalFrames > 0 {
		s.StylePerFrame = float64(s.TotalStylePasses) / float64(s.TotalFrames)
		s.LayoutPerFrame = float64(s.TotalLayoutPasses) / float64(s.TotalFrames)
		s.PaintPerFrame = float64(s.TotalPaintCalls) / float64(s.TotalFrames)

		var total time.Duration
		for i := 0; i < r.frameIdx && i < len(r.frames); i++ {
			total += r.frames[i].Duration
		}
		s.AvgFrameDuration = total / time.Duration(r.frameIdx)
	}
	return s
}

// RecentFrames returns the last N frames.
func (r *Renderer) RecentFrames(n int) []Frame {
	r.mu.Lock()
	defer r.mu.Unlock()

	if n > r.frameIdx {
		n = r.frameIdx
	}
	start := r.frameIdx - n
	if start < 0 {
		start = 0
	}
	return r.frames[start:r.frameIdx]
}

// RecentInteractions returns the last N interactions.
func (r *Renderer) RecentInteractions(n int) []Interaction {
	r.mu.Lock()
	defer r.mu.Unlock()

	if n > len(r.interactions) {
		n = len(r.interactions)
	}
	return r.interactions[len(r.interactions)-n:]
}

// Close cleans up resources.
func (r *Renderer) Close() {
	if r.logFile != nil {
		r.logFile.Close()
	}
}

// PrintStats prints statistics to stderr.
func (r *Renderer) PrintStats() {
	s := r.Stats()
	fmt.Fprintf(os.Stderr, "\n=== Goosie Debug Stats ===\n")
	fmt.Fprintf(os.Stderr, "Frames: %d\n", s.TotalFrames)
	fmt.Fprintf(os.Stderr, "Interactions: %d\n", s.TotalInteractions)
	fmt.Fprintf(os.Stderr, "Style passes: %d (%.2f/frame)\n", s.TotalStylePasses, s.StylePerFrame)
	fmt.Fprintf(os.Stderr, "Layout passes: %d (%.2f/frame)\n", s.TotalLayoutPasses, s.LayoutPerFrame)
	fmt.Fprintf(os.Stderr, "Paint calls: %d (%.2f/frame)\n", s.TotalPaintCalls, s.PaintPerFrame)
	fmt.Fprintf(os.Stderr, "Tiles reused: %d\n", s.TotalTilesReused)
	fmt.Fprintf(os.Stderr, "Tiles rasterized: %d\n", s.TotalTilesRasterized)
	fmt.Fprintf(os.Stderr, "Avg frame duration: %v\n", s.AvgFrameDuration)
	fmt.Fprintf(os.Stderr, "Max frame duration: %v\n", s.MaxFrameDuration)
	fmt.Fprintf(os.Stderr, "========================\n\n")
}
