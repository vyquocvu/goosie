package memory_test

import (
	"sync/atomic"
	"testing"

	mem "github.com/vyquocvu/goosie/internal/memory"
)

func TestNewManager(t *testing.T) {
	cfg := mem.Config{
		Limits: map[mem.Component]uint64{
			mem.ComponentDOM:   1000,
			mem.ComponentImage: 2000,
		},
		GlobalLimit: 5000,
	}
	m := mem.NewManager(cfg)

	limits := m.Limits()
	if limits[mem.ComponentDOM] != 1000 {
		t.Errorf("expected DOM limit 1000, got %d", limits[mem.ComponentDOM])
	}
	if limits[mem.ComponentImage] != 2000 {
		t.Errorf("expected Image limit 2000, got %d", limits[mem.ComponentImage])
	}
	if limits[mem.ComponentStyle] != 0 {
		t.Errorf("expected Style limit 0, got %d", limits[mem.ComponentStyle])
	}

	stats := m.Stats()
	if stats.GlobalLimit != 5000 {
		t.Errorf("expected global limit 5000, got %d", stats.GlobalLimit)
	}
}

func TestUsageTracking(t *testing.T) {
	m := mem.NewManager(mem.Config{})

	m.UpdateUsage(mem.ComponentDOM, 500)
	m.UpdateUsage(mem.ComponentStyle, 300)

	if m.Usage(mem.ComponentDOM) != 500 {
		t.Errorf("expected DOM usage 500, got %d", m.Usage(mem.ComponentDOM))
	}
	if m.Usage(mem.ComponentStyle) != 300 {
		t.Errorf("expected Style usage 300, got %d", m.Usage(mem.ComponentStyle))
	}
	if m.TotalUsage() != 800 {
		t.Errorf("expected total usage 800, got %d", m.TotalUsage())
	}
}

func TestComponentEviction(t *testing.T) {
	m := mem.NewManager(mem.Config{
		Limits: map[mem.Component]uint64{
			mem.ComponentImage: 1000,
		},
	})

	var evictedBytes atomic.Uint64
	m.RegisterEvictor(mem.ComponentImage, func(target uint64) uint64 {
		evictedBytes.Store(target)
		return target // Simulate full eviction of requested target
	})

	// Over limit by 500 bytes
	m.UpdateUsage(mem.ComponentImage, 1500)

	if evictedBytes.Load() != 500 {
		t.Errorf("expected evictor to be called with target 500, got %d", evictedBytes.Load())
	}

	// Internal usage should be updated down to limit
	if m.Usage(mem.ComponentImage) != 1000 {
		t.Errorf("expected Image usage to be updated to 1000, got %d", m.Usage(mem.ComponentImage))
	}
}

func TestGlobalEviction(t *testing.T) {
	m := mem.NewManager(mem.Config{
		GlobalLimit: 2000,
	})

	// Eviction order: ComponentNetworkCache (1st), ComponentStyle (2nd)
	m.SetEvictionOrder([]mem.Component{mem.ComponentNetworkCache, mem.ComponentStyle})

	var netEvicted atomic.Uint64
	var styleEvicted atomic.Uint64

	m.RegisterEvictor(mem.ComponentNetworkCache, func(target uint64) uint64 {
		netEvicted.Store(target)
		return target
	})

	m.RegisterEvictor(mem.ComponentStyle, func(target uint64) uint64 {
		styleEvicted.Store(target)
		return target
	})

	// Populate usage: NetworkCache=800, Style=1500 (total=2300, 300 over global limit)
	m.UpdateUsage(mem.ComponentNetworkCache, 800)
	m.UpdateUsage(mem.ComponentStyle, 1500)

	// Since NetworkCache is first in eviction order and has 800 bytes,
	// it should be evicted for the entire 300 excess bytes.
	if netEvicted.Load() != 300 {
		t.Errorf("expected NetworkCache to evict 300, got %d", netEvicted.Load())
	}
	if styleEvicted.Load() != 0 {
		t.Errorf("expected Style to evict 0, got %d", styleEvicted.Load())
	}

	if m.Usage(mem.ComponentNetworkCache) != 500 {
		t.Errorf("expected NetworkCache usage to be 500, got %d", m.Usage(mem.ComponentNetworkCache))
	}
	if m.Usage(mem.ComponentStyle) != 1500 {
		t.Errorf("expected Style usage to be 1500, got %d", m.Usage(mem.ComponentStyle))
	}
}

func TestGlobalEvictionCascade(t *testing.T) {
	m := mem.NewManager(mem.Config{
		GlobalLimit: 1000,
	})

	m.SetEvictionOrder([]mem.Component{mem.ComponentNetworkCache, mem.ComponentStyle})

	var netEvictable int64 = 300
	m.RegisterEvictor(mem.ComponentNetworkCache, func(target uint64) uint64 {
		freed := target
		if freed > uint64(netEvictable) {
			freed = uint64(netEvictable)
		}
		netEvictable -= int64(freed)
		return freed
	})

	var styleTarget atomic.Uint64
	m.RegisterEvictor(mem.ComponentStyle, func(target uint64) uint64 {
		styleTarget.Store(target)
		return target
	})

	// Populate usage: NetworkCache=500, Style=1000 (total=1500, 500 over global limit)
	m.UpdateUsage(mem.ComponentNetworkCache, 500)
	m.UpdateUsage(mem.ComponentStyle, 1000)

	// NetworkCache evicts 300 bytes. Remaining excess is 200, which cascades to Style.
	if styleTarget.Load() != 200 {
		t.Errorf("expected Style to be called with target 200, got %d", styleTarget.Load())
	}
}
