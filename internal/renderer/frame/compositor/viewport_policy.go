package compositor

import (
	"github.com/vyquocvu/goosie/internal/renderer/frame"
)

// ---------------------------------------------------------------------------
// ViewportPolicy — scroll-direction-aware tile prioritization
// ---------------------------------------------------------------------------

// ScrollDirection represents the estimated scroll direction.
type ScrollDirection uint8

const (
	ScrollDirectionNone  ScrollDirection = iota
	ScrollDirectionUp                    // scrolling toward lower Y
	ScrollDirectionDown                  // scrolling toward higher Y
	ScrollDirectionLeft                  // scrolling toward lower X
	ScrollDirectionRight                 // scrolling toward higher X
)

// ViewportPolicyConfig configures the viewport prefetch policy.
type ViewportPolicyConfig struct {
	// PrefetchMargin is the extra pixels to prefetch beyond the visible
	// viewport in the scroll direction.
	PrefetchMargin float32

	// OppositeMargin is the smaller margin to keep behind the scroll
	// direction (tiles that just scrolled off).
	OppositeMargin float32

	// MaxPrefetchTiles is the maximum number of tiles to prefetch
	// beyond the visible set.
	MaxPrefetchTiles int

	// HiddenTabPrefetchFraction is the fraction of normal prefetch
	// margin to use when the tab is hidden (0.0 = no prefetch).
	HiddenTabPrefetchFraction float32
}

// DefaultViewportPolicyConfig returns sensible defaults.
func DefaultViewportPolicyConfig() ViewportPolicyConfig {
	return ViewportPolicyConfig{
		PrefetchMargin:            512,
		OppositeMargin:            128,
		MaxPrefetchTiles:          32,
		HiddenTabPrefetchFraction: 0.0,
	}
}

// ViewportPolicy manages tile prioritization based on viewport state
// and scroll direction. It determines which tiles to rasterize first
// and how far to prefetch in the scroll direction.
type ViewportPolicy struct {
	config     ViewportPolicyConfig
	cache      *TileCache
	visible    frame.Viewport
	prevScroll frame.Point
	direction  ScrollDirection
	hidden     bool
}

// NewViewportPolicy creates a viewport policy backed by the given tile cache.
func NewViewportPolicy(cache *TileCache, cfg ViewportPolicyConfig) *ViewportPolicy {
	if cfg.PrefetchMargin <= 0 {
		cfg.PrefetchMargin = 512
	}
	if cfg.OppositeMargin <= 0 {
		cfg.OppositeMargin = 128
	}
	if cfg.MaxPrefetchTiles <= 0 {
		cfg.MaxPrefetchTiles = 32
	}
	return &ViewportPolicy{
		config: cfg,
		cache:  cache,
	}
}

// UpdateViewport updates the viewport state and estimates scroll direction.
func (p *ViewportPolicy) UpdateViewport(vp frame.Viewport) {
	cur := frame.Point{X: vp.ScrollX, Y: vp.ScrollY}

	dx := cur.X - p.prevScroll.X
	dy := cur.Y - p.prevScroll.Y

	switch {
	case dy > 0:
		p.direction = ScrollDirectionDown
	case dy < 0:
		p.direction = ScrollDirectionUp
	case dx > 0:
		p.direction = ScrollDirectionRight
	case dx < 0:
		p.direction = ScrollDirectionLeft
	}

	p.prevScroll = cur
	p.visible = vp
}

// SetHidden marks the tab as hidden or visible. Hidden tabs get reduced
// prefetch and deprioritized raster work.
func (p *ViewportPolicy) SetHidden(hidden bool) {
	p.hidden = hidden
}

// IsHidden reports whether the tab is hidden.
func (p *ViewportPolicy) IsHidden() bool {
	return p.hidden
}

// Direction returns the current estimated scroll direction.
func (p *ViewportPolicy) Direction() ScrollDirection {
	return p.direction
}

// PrefetchRect returns the expanded rect to prefetch based on scroll
// direction and policy. For hidden tabs, the margin is reduced.
func (p *ViewportPolicy) PrefetchRect() frame.Rect {
	vr := p.visible.VisibleRect()
	margin := p.config.PrefetchMargin
	if p.hidden {
		margin *= p.config.HiddenTabPrefetchFraction
	}

	switch p.direction {
	case ScrollDirectionDown:
		vr.Y = vr.Y + vr.H - p.config.OppositeMargin
		vr.H = margin + p.config.OppositeMargin
	case ScrollDirectionUp:
		vr.H = margin + p.config.OppositeMargin
		vr.Y = vr.Y - margin
	case ScrollDirectionRight:
		vr.X = vr.X + vr.W - p.config.OppositeMargin
		vr.W = margin + p.config.OppositeMargin
	case ScrollDirectionLeft:
		vr.W = margin + p.config.OppositeMargin
		vr.X = vr.X - margin
	default:
		// No direction — prefetch equally in all directions.
		vr = vr.Expand(margin / 2)
	}
	return vr
}

// TilePriorityResult pairs a tile coordinate with its computed priority.
type TilePriorityResult struct {
	Coord    TileCoord
	Priority TilePriority
}

// PrioritizeTiles returns tile coordinates within the prefetch rect,
// sorted by priority: visible first, then near, then hidden.
// The result is bounded by MaxPrefetchTiles.
func (p *ViewportPolicy) PrioritizeTiles() []TilePriorityResult {
	prefetchRect := p.PrefetchRect()
	visibleRect := p.visible.VisibleRect()
	coords := p.cache.CoordsInRect(prefetchRect)

	results := make([]TilePriorityResult, 0, len(coords))
	for _, c := range coords {
		bounds := p.cache.BoundsForCoord(c)
		var pri TilePriority
		if bounds.Intersects(visibleRect) {
			pri = TilePriorityVisible
		} else if bounds.Intersects(prefetchRect) {
			pri = TilePriorityNear
		} else {
			pri = TilePriorityHidden
		}
		results = append(results, TilePriorityResult{Coord: c, Priority: pri})
	}

	// Sort: Visible < Near < Hidden (stable by coord for determinism).
	sortResults(results)

	// Bound the result.
	if len(results) > p.config.MaxPrefetchTiles {
		results = results[:p.config.MaxPrefetchTiles]
	}
	return results
}

// sortResults sorts by priority (Visible < Near < Hidden), then by coord.
func sortResults(results []TilePriorityResult) {
	// Simple insertion sort — bounded by MaxPrefetchTiles (32).
	for i := 1; i < len(results); i++ {
		key := results[i]
		j := i - 1
		for j >= 0 && compareResults(results[j], key) > 0 {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}
}

func compareResults(a, b TilePriorityResult) int {
	if a.Priority != b.Priority {
		return int(a.Priority) - int(b.Priority)
	}
	if a.Coord.Row != b.Coord.Row {
		return int(a.Coord.Row - b.Coord.Row)
	}
	return int(a.Coord.Col - b.Coord.Col)
}

// ---------------------------------------------------------------------------
// ResourcePrefetchLimits — document-level resource limits
// ---------------------------------------------------------------------------

// ResourcePrefetchLimits defines bounds for page cache and resource prefetch.
type ResourcePrefetchLimits struct {
	// MaxCachedPages is the maximum number of fully-rendered pages to
	// keep in memory for back/forward navigation.
	MaxCachedPages int

	// MaxPrefetchResources is the maximum number of resources (images,
	// stylesheets) to prefetch beyond the current document.
	MaxPrefetchResources int

	// MaxPrefetchBytes is the byte budget for prefetched resources.
	MaxPrefetchBytes int64
}

// DefaultResourcePrefetchLimits returns conservative defaults.
func DefaultResourcePrefetchLimits() ResourcePrefetchLimits {
	return ResourcePrefetchLimits{
		MaxCachedPages:       3,
		MaxPrefetchResources: 16,
		MaxPrefetchBytes:     8 << 20, // 8 MB
	}
}
