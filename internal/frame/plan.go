package frame

import "sync"

type FramePlan struct {
	Serial     uint64
	Viewport   Viewport
	Scale      float32
	Layers     []*Layer
	Background Color
}

func (p FramePlan) Layer(id LayerID) *Layer {
	for _, l := range p.Layers {
		if l != nil && l.ID == id {
			return l
		}
	}
	return nil
}

type Plan struct {
	mu        sync.Mutex
	latest    FramePlan
	ok        bool
	consumed  bool
	published int64
	dropped   int64
}

func (p *Plan) Publish(next FramePlan) {
	p.mu.Lock()
	if p.ok && !p.consumed {
		p.dropped++
	}
	p.latest = next
	p.ok = true
	p.consumed = false
	p.published++
	p.mu.Unlock()
}

func (p *Plan) Latest() (FramePlan, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.latest, p.ok
}

func (p *Plan) Take() (FramePlan, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consumed = true
	return p.latest, p.ok
}

type PlanStats struct {
	Published int64
	Dropped   int64
}

func (p *Plan) Stats() PlanStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PlanStats{Published: p.published, Dropped: p.dropped}
}
