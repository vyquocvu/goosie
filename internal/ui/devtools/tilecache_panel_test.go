package devtools

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/memory"
)

func TestTileCachePanel_InitialState(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newTileCachePanelContent(func() *TabContext { return &TabContext{} })
	assert.NotNil(t, p)
}

func TestTileCachePanel_WithMemoryManager(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	cfg := memory.Config{
		Limits: map[memory.Component]uint64{
			memory.ComponentTile: 1024 * 1024,
		},
	}
	mgr := memory.NewManager(cfg)
	ctx := &TabContext{Memory: mgr}
	p := newTileCachePanelContent(func() *TabContext { return ctx })
	assert.NotNil(t, p)
}

func TestTileCachePanel_WithRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{
		Renderer: &mockRendererProvider{
			summary:  map[string]int{},
			commands: nil,
		},
	}
	p := newTileCachePanelContent(func() *TabContext { return ctx })
	assert.NotNil(t, p)
}

func TestTileCachePanel_UnlimitedBudget(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mgr := memory.NewManager(memory.Config{})
	ctx := &TabContext{Memory: mgr}
	p := newTileCachePanelContent(func() *TabContext { return ctx })
	assert.NotNil(t, p)
}

// TestTileCachePanel_FormatCount verifies the counter formatter
// used by the per-card metric labels. A negative input means
// "unknown / not available" and renders as "n/a"; otherwise the
// value is rendered with the supplied unit suffix.
func TestTileCachePanel_FormatCount(t *testing.T) {
	cases := []struct {
		value int64
		unit  string
		want  string
	}{
		{-1, "hits", "n/a"},
		{0, "hits", "0 hits"},
		{42, "hits", "42 hits"},
		{12345, "evictions", "12345 evictions"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.want, func(t *testing.T) {
			assert.Equal(t, c.want, formatCount(c.value, c.unit))
		})
	}
}

// TestTileCachePanel_FormatLimit verifies the per-component
// memory limit formatter. A zero limit is rendered as
// "unbounded" so the user can distinguish "no budget" from
// "exactly zero bytes".
func TestTileCachePanel_FormatLimit(t *testing.T) {
	assert.Equal(t, "Image: unbounded", formatLimit("Image", 0))
	assert.Equal(t, "Image: 1.0 KB", formatLimit("Image", 1024))
	assert.Equal(t, "Image: 1.5 MB", formatLimit("Image", 1024*1024+512*1024))
}

// TestTileCachePanel_FormatBudget verifies the full budget
// summary covers every component without panicking on missing
// entries or zero limits.
func TestTileCachePanel_FormatBudget(t *testing.T) {
	stats := memory.Stats{
		Limits: map[memory.Component]uint64{
			memory.ComponentTile:         2 * 1024 * 1024,
			memory.ComponentImage:        4 * 1024 * 1024,
			memory.ComponentGlyph:        1 * 1024 * 1024,
			memory.ComponentPageCache:    0,
			memory.ComponentLayoutIntrinsicSize: 512 * 1024,
		},
	}
	out := formatBudget(stats)
	assert.Contains(t, out, "Tile")
	assert.Contains(t, out, "ImageCache")
	assert.Contains(t, out, "GlyphCache")
	assert.Contains(t, out, "PageCache")
	assert.Contains(t, out, "LayoutIntrinsicSize")
	// PageCache was configured as zero — must show "unbounded".
	assert.Contains(t, out, "PageCache: unbounded")
}

// TestTileCachePanel_SnapshotMetrics verifies the snapshot
// helper returns nil when no MetricsRecorder is wired and a
// non-nil pointer otherwise.
func TestTileCachePanel_SnapshotMetrics(t *testing.T) {
	// Nil recorder path.
	assert.Nil(t, snapshotMetrics(&TabContext{}))

	// With a recorder (we use the mockRecorderProvider wired in
	// the devtools tests; see mock_metrics_provider_test.go).
	ctx := &TabContext{MetricsRecorder: &mockMetricsProvider{}}
	snap := snapshotMetrics(ctx)
	if assert.NotNil(t, snap) {
		// The mock provider returns zero counters, which is
		// fine: we just want to assert the wiring exists.
		assert.Equal(t, 0, snap.Counters.CacheHits)
	}
}