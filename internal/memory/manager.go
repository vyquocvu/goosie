package memory

import (
	"sync"
)

// Component represents a tracked component of the browser engine.
type Component string

const (
	ComponentDOM                 Component = "dom"
	ComponentStyle               Component = "style"
	ComponentLayout              Component = "layout"
	ComponentDisplayList         Component = "display-list"
	ComponentTile                Component = "tile"
	ComponentImage               Component = "image"
	ComponentGlyph               Component = "glyph"
	ComponentScript              Component = "script"
	ComponentNetworkCache        Component = "network-cache"
	ComponentLayoutIntrinsicSize Component = "layout-intrinsic-size"
	ComponentPageCache           Component = "page-cache"
)

// Evictor is a callback function registered by a component.
// It evicts up to targetBytes from the component and returns the actual bytes freed.
type Evictor func(targetBytes uint64) uint64

// Config configures the soft memory limits.
type Config struct {
	Limits      map[Component]uint64
	GlobalLimit uint64
}

// Stats captures a snapshot of current memory budgets and usages.
type Stats struct {
	Limits      map[Component]uint64
	Usage       map[Component]uint64
	GlobalLimit uint64
	TotalUsage  uint64
}

// Manager tracks memory estimates per component, sets soft limits,
// and schedules ordered eviction when limits are exceeded.
type Manager struct {
	mu            sync.RWMutex
	limits        map[Component]uint64
	globalLimit   uint64
	usage         map[Component]uint64
	evictors      map[Component]Evictor
	evictionOrder []Component
}

// NewManager creates a memory manager with the given config.
func NewManager(cfg Config) *Manager {
	limits := make(map[Component]uint64)
	for k, v := range cfg.Limits {
		limits[k] = v
	}

	// Default eviction order (less critical/pure cached items first)
	order := []Component{
		ComponentNetworkCache,
		ComponentPageCache,
		ComponentStyle,
		ComponentGlyph,
		ComponentImage,
		ComponentTile,
		ComponentDisplayList,
		ComponentLayoutIntrinsicSize,
		ComponentLayout,
		ComponentDOM,
		ComponentScript,
	}

	return &Manager{
		limits:        limits,
		globalLimit:   cfg.GlobalLimit,
		usage:         make(map[Component]uint64),
		evictors:      make(map[Component]Evictor),
		evictionOrder: order,
	}
}

// SetEvictionOrder overrides the default eviction order.
func (m *Manager) SetEvictionOrder(order []Component) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.evictionOrder = make([]Component, len(order))
	copy(m.evictionOrder, order)
}

// RegisterEvictor registers an eviction callback for a component.
func (m *Manager) RegisterEvictor(comp Component, evictor Evictor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.evictors[comp] = evictor
}

// SetLimit updates the limit for a specific component.
func (m *Manager) SetLimit(comp Component, limit uint64) {
	m.mu.Lock()
	m.limits[comp] = limit
	m.mu.Unlock()
}

// SetGlobalLimit updates the global soft memory limit.
func (m *Manager) SetGlobalLimit(limit uint64) {
	m.mu.Lock()
	m.globalLimit = limit
	m.mu.Unlock()
}

// UpdateUsage sets the current memory estimate for a component
// and triggers eviction if limits are exceeded.
func (m *Manager) UpdateUsage(comp Component, bytes uint64) {
	m.mu.Lock()
	m.usage[comp] = bytes
	m.mu.Unlock()

	m.checkLimitsAndEvict()
}

// checkLimitsAndEvict iteratively runs eviction.
func (m *Manager) checkLimitsAndEvict() {
	// 1. Evict individual component limits
	for {
		var nextJob *evictJob
		m.mu.Lock()
		for comp, usage := range m.usage {
			limit, ok := m.limits[comp]
			if ok && limit > 0 && usage > limit {
				nextJob = &evictJob{
					comp:   comp,
					target: usage - limit,
				}
				break
			}
		}
		m.mu.Unlock()

		if nextJob == nil {
			break
		}

		m.mu.RLock()
		evictor, ok := m.evictors[nextJob.comp]
		m.mu.RUnlock()

		if ok && evictor != nil {
			freed := evictor(nextJob.target)
			if freed > 0 {
				m.mu.Lock()
				if m.usage[nextJob.comp] >= freed {
					m.usage[nextJob.comp] -= freed
				} else {
					m.usage[nextJob.comp] = 0
				}
				m.mu.Unlock()
			} else {
				break
			}
		} else {
			break
		}
	}

	// 2. Evict global limit sequentially
	skipped := make(map[Component]bool)
	for {
		var nextJob *evictJob
		m.mu.Lock()
		if m.globalLimit > 0 {
			var totalUsage uint64
			for _, usage := range m.usage {
				totalUsage += usage
			}
			if totalUsage > m.globalLimit {
				needed := totalUsage - m.globalLimit
				for _, comp := range m.evictionOrder {
					if skipped[comp] {
						continue
					}
					usage := m.usage[comp]
					if usage > 0 {
						target := needed
						if target > usage {
							target = usage
						}
						nextJob = &evictJob{
							comp:   comp,
							target: target,
						}
						break
					}
				}
			}
		}
		m.mu.Unlock()

		if nextJob == nil {
			break
		}

		m.mu.RLock()
		evictor, ok := m.evictors[nextJob.comp]
		m.mu.RUnlock()

		if ok && evictor != nil {
			freed := evictor(nextJob.target)
			if freed > 0 {
				m.mu.Lock()
				if m.usage[nextJob.comp] >= freed {
					m.usage[nextJob.comp] -= freed
				} else {
					m.usage[nextJob.comp] = 0
				}
				usage := m.usage[nextJob.comp]
				m.mu.Unlock()
				if usage == 0 {
					skipped[nextJob.comp] = true
				}
			} else {
				skipped[nextJob.comp] = true
			}
		} else {
			skipped[nextJob.comp] = true
		}
	}
}

// Usage returns the current memory usage of a component.
func (m *Manager) Usage(comp Component) uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.usage[comp]
}

// TotalUsage returns the sum of all components' memory usage.
func (m *Manager) TotalUsage() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var total uint64
	for _, u := range m.usage {
		total += u
	}
	return total
}

// Limits returns a copy of the component soft limits.
func (m *Manager) Limits() map[Component]uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	limits := make(map[Component]uint64)
	for k, v := range m.limits {
		limits[k] = v
	}
	return limits
}

// Stats returns a copy of current limits and usage.
func (m *Manager) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	limits := make(map[Component]uint64)
	for k, v := range m.limits {
		limits[k] = v
	}
	usage := make(map[Component]uint64)
	var totalUsage uint64
	for k, v := range m.usage {
		usage[k] = v
		totalUsage += v
	}

	return Stats{
		Limits:      limits,
		Usage:       usage,
		GlobalLimit: m.globalLimit,
		TotalUsage:  totalUsage,
	}
}

type evictJob struct {
	comp   Component
	target uint64
}
