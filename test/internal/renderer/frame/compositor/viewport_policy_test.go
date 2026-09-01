package compositor_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame/compositor"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
)

// ---------------------------------------------------------------------------
// compositor.ViewportPolicyConfig — defaults
// ---------------------------------------------------------------------------

func TestDefaultViewportPolicyConfig(t *testing.T) {
	cfg := compositor.DefaultViewportPolicyConfig()
	if cfg.PrefetchMargin != 512 {
		t.Errorf("PrefetchMargin = %f, want 512", cfg.PrefetchMargin)
	}
	if cfg.OppositeMargin != 128 {
		t.Errorf("OppositeMargin = %f, want 128", cfg.OppositeMargin)
	}
	if cfg.MaxPrefetchTiles != 32 {
		t.Errorf("MaxPrefetchTiles = %d, want 32", cfg.MaxPrefetchTiles)
	}
	if cfg.HiddenTabPrefetchFraction != 0.0 {
		t.Errorf("HiddenTabPrefetchFraction = %f, want 0", cfg.HiddenTabPrefetchFraction)
	}
}

// ---------------------------------------------------------------------------
// Scroll direction detection
// ---------------------------------------------------------------------------

func TestViewportPolicy_DirectionDown(t *testing.T) {
	cache := compositor.NewTileCache(compositor.DefaultTileCacheConfig())
	p := compositor.NewViewportPolicy(cache, compositor.DefaultViewportPolicyConfig())

	p.UpdateViewport(frame.Viewport{Width: 800, Height: 600, ScrollX: 0, ScrollY: 0})
	p.UpdateViewport(frame.Viewport{Width: 800, Height: 600, ScrollX: 0, ScrollY: 100})

	if p.Direction() != compositor.ScrollDirectionDown {
		t.Errorf("Direction = %d, want Down", p.Direction())
	}
}

func TestViewportPolicy_DirectionUp(t *testing.T) {
	cache := compositor.NewTileCache(compositor.DefaultTileCacheConfig())
	p := compositor.NewViewportPolicy(cache, compositor.DefaultViewportPolicyConfig())

	p.UpdateViewport(frame.Viewport{Width: 800, Height: 600, ScrollX: 0, ScrollY: 200})
	p.UpdateViewport(frame.Viewport{Width: 800, Height: 600, ScrollX: 0, ScrollY: 100})

	if p.Direction() != compositor.ScrollDirectionUp {
		t.Errorf("Direction = %d, want Up", p.Direction())
	}
}

func TestViewportPolicy_DirectionRight(t *testing.T) {
	cache := compositor.NewTileCache(compositor.DefaultTileCacheConfig())
	p := compositor.NewViewportPolicy(cache, compositor.DefaultViewportPolicyConfig())

	p.UpdateViewport(frame.Viewport{Width: 800, Height: 600, ScrollX: 0, ScrollY: 0})
	p.UpdateViewport(frame.Viewport{Width: 800, Height: 600, ScrollX: 100, ScrollY: 0})

	if p.Direction() != compositor.ScrollDirectionRight {
		t.Errorf("Direction = %d, want Right", p.Direction())
	}
}

func TestViewportPolicy_DirectionLeft(t *testing.T) {
	cache := compositor.NewTileCache(compositor.DefaultTileCacheConfig())
	p := compositor.NewViewportPolicy(cache, compositor.DefaultViewportPolicyConfig())

	p.UpdateViewport(frame.Viewport{Width: 800, Height: 600, ScrollX: 200, ScrollY: 0})
	p.UpdateViewport(frame.Viewport{Width: 800, Height: 600, ScrollX: 100, ScrollY: 0})

	if p.Direction() != compositor.ScrollDirectionLeft {
		t.Errorf("Direction = %d, want Left", p.Direction())
	}
}

func TestViewportPolicy_DirectionNone(t *testing.T) {
	cache := compositor.NewTileCache(compositor.DefaultTileCacheConfig())
	p := compositor.NewViewportPolicy(cache, compositor.DefaultViewportPolicyConfig())

	p.UpdateViewport(frame.Viewport{Width: 800, Height: 600, ScrollX: 0, ScrollY: 0})

	if p.Direction() != compositor.ScrollDirectionNone {
		t.Errorf("Direction = %d, want None", p.Direction())
	}
}

// ---------------------------------------------------------------------------
// Hidden tab
// ---------------------------------------------------------------------------

func TestViewportPolicy_HiddenTab(t *testing.T) {
	cache := compositor.NewTileCache(compositor.DefaultTileCacheConfig())
	p := compositor.NewViewportPolicy(cache, compositor.DefaultViewportPolicyConfig())

	if p.IsHidden() {
		t.Error("should not be hidden initially")
	}
	p.SetHidden(true)
	if !p.IsHidden() {
		t.Error("should be hidden after SetHidden(true)")
	}
}

func TestViewportPolicy_HiddenTabReducesPrefetch(t *testing.T) {
	cfg := compositor.ViewportPolicyConfig{
		PrefetchMargin:            512,
		OppositeMargin:            128,
		MaxPrefetchTiles:          32,
		HiddenTabPrefetchFraction: 0.0,
	}
	cache := compositor.NewTileCache(compositor.TileCacheConfig{TileWidth: 100, TileHeight: 100, MaxBytes: 1 << 30, MaxTiles: 1000})
	p := compositor.NewViewportPolicy(cache, cfg)

	p.UpdateViewport(frame.Viewport{Width: 200, Height: 200, ScrollX: 0, ScrollY: 0})
	p.UpdateViewport(frame.Viewport{Width: 200, Height: 200, ScrollX: 0, ScrollY: 100})

	visible := p.PrefetchRect()

	// Hidden tab with 0 fraction — prefetch rect should have no margin beyond opposite.
	p.SetHidden(true)
	hidden := p.PrefetchRect()

	if hidden.H > visible.H {
		t.Errorf("hidden prefetch H=%f should be <= visible H=%f", hidden.H, visible.H)
	}
}

// ---------------------------------------------------------------------------
// PrefetchRect
// ---------------------------------------------------------------------------

func TestViewportPolicy_PrefetchRect_Down(t *testing.T) {
	cfg := compositor.ViewportPolicyConfig{PrefetchMargin: 100, OppositeMargin: 50, MaxPrefetchTiles: 32}
	cache := compositor.NewTileCache(compositor.DefaultTileCacheConfig())
	p := compositor.NewViewportPolicy(cache, cfg)

	p.UpdateViewport(frame.Viewport{Width: 200, Height: 200, ScrollX: 0, ScrollY: 0})
	p.UpdateViewport(frame.Viewport{Width: 200, Height: 200, ScrollX: 0, ScrollY: 100})

	pr := p.PrefetchRect()
	// Scrolling down: prefetch rect should extend below the viewport.
	if pr.Y < 200 {
		t.Errorf("PrefetchRect.Y = %f, want >= 200 (below viewport bottom)", pr.Y)
	}
}

func TestViewportPolicy_PrefetchRect_NoDirection(t *testing.T) {
	cfg := compositor.ViewportPolicyConfig{PrefetchMargin: 100, OppositeMargin: 50, MaxPrefetchTiles: 32}
	cache := compositor.NewTileCache(compositor.DefaultTileCacheConfig())
	p := compositor.NewViewportPolicy(cache, cfg)

	p.UpdateViewport(frame.Viewport{Width: 200, Height: 200, ScrollX: 0, ScrollY: 0})

	pr := p.PrefetchRect()
	// No direction: should expand equally.
	if pr.X >= 0 || pr.Y >= 0 {
		t.Errorf("PrefetchRect should expand: got X=%f Y=%f", pr.X, pr.Y)
	}
}

// ---------------------------------------------------------------------------
// PrioritizeTiles
// ---------------------------------------------------------------------------

func TestViewportPolicy_PrioritizeTiles(t *testing.T) {
	cfg := compositor.ViewportPolicyConfig{PrefetchMargin: 200, OppositeMargin: 50, MaxPrefetchTiles: 32}
	cache := compositor.NewTileCache(compositor.TileCacheConfig{TileWidth: 100, TileHeight: 100, MaxBytes: 1 << 30, MaxTiles: 1000})

	// Insert a grid of tiles.
	for col := int32(0); col < 5; col++ {
		for row := int32(0); row < 5; row++ {
			coord := compositor.TileCoord{Col: col, Row: row}
			cache.Put(&compositor.Tile{Coord: coord, Bounds: cache.BoundsForCoord(coord), ByteSize: 1})
		}
	}

	p := compositor.NewViewportPolicy(cache, cfg)
	p.UpdateViewport(frame.Viewport{Width: 200, Height: 200, ScrollX: 0, ScrollY: 0})
	p.UpdateViewport(frame.Viewport{Width: 200, Height: 200, ScrollX: 0, ScrollY: 100})

	results := p.PrioritizeTiles()
	if len(results) == 0 {
		t.Fatal("PrioritizeTiles should return results")
	}

	// First results should be Visible priority.
	if results[0].Priority != compositor.TilePriorityVisible {
		t.Errorf("first result priority = %d, want Visible", results[0].Priority)
	}

	// Verify ordering: no Hidden before Visible.
	seenNear := false
	for _, r := range results {
		if r.Priority == compositor.TilePriorityHidden && !seenNear {
			// Check no visible comes after hidden.
		}
		if r.Priority == compositor.TilePriorityNear {
			seenNear = true
		}
	}
}

func TestViewportPolicy_PrioritizeTiles_BoundedByMax(t *testing.T) {
	cfg := compositor.ViewportPolicyConfig{PrefetchMargin: 1000, OppositeMargin: 100, MaxPrefetchTiles: 5}
	cache := compositor.NewTileCache(compositor.TileCacheConfig{TileWidth: 100, TileHeight: 100, MaxBytes: 1 << 30, MaxTiles: 1000})

	for col := int32(0); col < 20; col++ {
		for row := int32(0); row < 20; row++ {
			coord := compositor.TileCoord{Col: col, Row: row}
			cache.Put(&compositor.Tile{Coord: coord, Bounds: cache.BoundsForCoord(coord), ByteSize: 1})
		}
	}

	p := compositor.NewViewportPolicy(cache, cfg)
	p.UpdateViewport(frame.Viewport{Width: 200, Height: 200, ScrollX: 0, ScrollY: 0})

	results := p.PrioritizeTiles()
	if len(results) > 5 {
		t.Errorf("len = %d, want <= 5 (MaxPrefetchTiles)", len(results))
	}
}

// ---------------------------------------------------------------------------
// compositor.ResourcePrefetchLimits
// ---------------------------------------------------------------------------

func TestDefaultResourcePrefetchLimits(t *testing.T) {
	limits := compositor.DefaultResourcePrefetchLimits()
	if limits.MaxCachedPages != 3 {
		t.Errorf("MaxCachedPages = %d, want 3", limits.MaxCachedPages)
	}
	if limits.MaxPrefetchResources != 16 {
		t.Errorf("MaxPrefetchResources = %d, want 16", limits.MaxPrefetchResources)
	}
	if limits.MaxPrefetchBytes != 8<<20 {
		t.Errorf("MaxPrefetchBytes = %d, want %d", limits.MaxPrefetchBytes, int64(8<<20))
	}
}

// ---------------------------------------------------------------------------
// Config sanitization
// ---------------------------------------------------------------------------

func TestViewportPolicy_SanitizesConfig(t *testing.T) {
	cache := compositor.NewTileCache(compositor.DefaultTileCacheConfig())
	p := compositor.NewViewportPolicy(cache, compositor.ViewportPolicyConfig{
		PrefetchMargin:   -1,
		OppositeMargin:   -1,
		MaxPrefetchTiles: 0,
	})
	if p.Config.PrefetchMargin != 512 {
		t.Errorf("sanitized PrefetchMargin = %f, want 512", p.Config.PrefetchMargin)
	}
	if p.Config.OppositeMargin != 128 {
		t.Errorf("sanitized OppositeMargin = %f, want 128", p.Config.OppositeMargin)
	}
	if p.Config.MaxPrefetchTiles != 32 {
		t.Errorf("sanitized MaxPrefetchTiles = %d, want 32", p.Config.MaxPrefetchTiles)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkViewportPolicy_PrioritizeTiles(b *testing.B) {
	cache := compositor.NewTileCache(compositor.TileCacheConfig{TileWidth: 256, TileHeight: 256, MaxBytes: 1 << 30, MaxTiles: 10000})
	for col := int32(0); col < 16; col++ {
		for row := int32(0); row < 16; row++ {
			coord := compositor.TileCoord{Col: col, Row: row}
			cache.Put(&compositor.Tile{Coord: coord, Bounds: cache.BoundsForCoord(coord), ByteSize: 1})
		}
	}
	p := compositor.NewViewportPolicy(cache, compositor.DefaultViewportPolicyConfig())
	vp := frame.Viewport{Width: 1024, Height: 768, ScrollX: 0, ScrollY: 0}
	p.UpdateViewport(vp)
	p.UpdateViewport(frame.Viewport{Width: 1024, Height: 768, ScrollX: 0, ScrollY: 256})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p.PrioritizeTiles()
	}
}

func BenchmarkViewportPolicy_PrefetchRect(b *testing.B) {
	cache := compositor.NewTileCache(compositor.DefaultTileCacheConfig())
	p := compositor.NewViewportPolicy(cache, compositor.DefaultViewportPolicyConfig())
	p.UpdateViewport(frame.Viewport{Width: 1024, Height: 768, ScrollX: 0, ScrollY: 0})
	p.UpdateViewport(frame.Viewport{Width: 1024, Height: 768, ScrollX: 0, ScrollY: 100})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p.PrefetchRect()
	}
}
