package memory

import (
	"sync/atomic"
	"testing"
)

func TestNewManager(t *testing.T) {
	cfg := Config{
		Limits: map[Component]uint64{
			ComponentDOM:   1000,
			ComponentImage: 2000,
		},
		GlobalLimit: 5000,
	}
	m := NewManager(cfg)

	limits := m.Limits()
	if limits[ComponentDOM] != 1000 {
		t.Errorf("expected DOM limit 1000, got %d", limits[ComponentDOM])
	}
	if limits[ComponentImage] != 2000 {
		t.Errorf("expected Image limit 2000, got %d", limits[ComponentImage])
	}
	if limits[ComponentStyle] != 0 {
		t.Errorf("expected Style limit 0, got %d", limits[ComponentStyle])
	}

	stats := m.Stats()
	if stats.GlobalLimit != 5000 {
		t.Errorf("expected global limit 5000, got %d", stats.GlobalLimit)
	}
}

func TestUsageTracking(t *testing.T) {
	m := NewManager(Config{})

	m.UpdateUsage(ComponentDOM, 500)
	m.UpdateUsage(ComponentStyle, 300)

	if m.Usage(ComponentDOM) != 500 {
		t.Errorf("expected DOM usage 500, got %d", m.Usage(ComponentDOM))
	}
	if m.Usage(ComponentStyle) != 300 {
		t.Errorf("expected Style usage 300, got %d", m.Usage(ComponentStyle))
	}
	if m.TotalUsage() != 800 {
		t.Errorf("expected total usage 800, got %d", m.TotalUsage())
	}
}

func TestComponentEviction(t *testing.T) {
	m := NewManager(Config{
		Limits: map[Component]uint64{
			ComponentImage: 1000,
		},
	})

	var evictedBytes atomic.Uint64
	m.RegisterEvictor(ComponentImage, func(target uint64) uint64 {
		evictedBytes.Store(target)
		return target // Simulate full eviction of requested target
	})

	// Over limit by 500 bytes
	m.UpdateUsage(ComponentImage, 1500)

	if evictedBytes.Load() != 500 {
		t.Errorf("expected evictor to be called with target 500, got %d", evictedBytes.Load())
	}

	// Internal usage should be updated down to limit
	if m.Usage(ComponentImage) != 1000 {
		t.Errorf("expected Image usage to be updated to 1000, got %d", m.Usage(ComponentImage))
	}
}

func TestGlobalEviction(t *testing.T) {
	m := NewManager(Config{
		GlobalLimit: 2000,
	})

	// Eviction order: ComponentNetworkCache (1st), ComponentStyle (2nd)
	m.SetEvictionOrder([]Component{ComponentNetworkCache, ComponentStyle})

	var netEvicted atomic.Uint64
	var styleEvicted atomic.Uint64

	m.RegisterEvictor(ComponentNetworkCache, func(target uint64) uint64 {
		netEvicted.Store(target)
		return target
	})

	m.RegisterEvictor(ComponentStyle, func(target uint64) uint64 {
		styleEvicted.Store(target)
		return target
	})

	// Populate usage: NetworkCache=800, Style=1500 (total=2300, 300 over global limit)
	m.UpdateUsage(ComponentNetworkCache, 800)
	m.UpdateUsage(ComponentStyle, 1500)

	// Since NetworkCache is first in eviction order and has 800 bytes,
	// it should be evicted for the entire 300 excess bytes.
	if netEvicted.Load() != 300 {
		t.Errorf("expected NetworkCache to evict 300, got %d", netEvicted.Load())
	}
	if styleEvicted.Load() != 0 {
		t.Errorf("expected Style to evict 0, got %d", styleEvicted.Load())
	}

	if m.Usage(ComponentNetworkCache) != 500 {
		t.Errorf("expected NetworkCache usage to be 500, got %d", m.Usage(ComponentNetworkCache))
	}
	if m.Usage(ComponentStyle) != 1500 {
		t.Errorf("expected Style usage to be 1500, got %d", m.Usage(ComponentStyle))
	}
}

func TestGlobalEvictionCascade(t *testing.T) {
	m := NewManager(Config{
		GlobalLimit: 1000,
	})

	m.SetEvictionOrder([]Component{ComponentNetworkCache, ComponentStyle})

	var netEvictable int64 = 300
	m.RegisterEvictor(ComponentNetworkCache, func(target uint64) uint64 {
		freed := target
		if freed > uint64(netEvictable) {
			freed = uint64(netEvictable)
		}
		netEvictable -= int64(freed)
		return freed
	})

	var styleTarget atomic.Uint64
	m.RegisterEvictor(ComponentStyle, func(target uint64) uint64 {
		styleTarget.Store(target)
		return target
	})

	// Populate usage: NetworkCache=500, Style=1000 (total=1500, 500 over global limit)
	m.UpdateUsage(ComponentNetworkCache, 500)
	m.UpdateUsage(ComponentStyle, 1000)

	// NetworkCache evicts 300 bytes. Remaining excess is 200, which cascades to Style.
	if styleTarget.Load() != 200 {
		t.Errorf("expected Style to be called with target 200, got %d", styleTarget.Load())
	}
}
