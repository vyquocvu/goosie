package frame

import "sync"

// FramePlan is the only object that crosses from the thread producing content to
// the thread presenting it. It is a value: the viewport, scale, layer pointers,
// and background travel together, so a frame cannot be presented with a viewport
// that does not match its layers.
//
// The layers inside it point at frozen content. Together those two properties
// are why no lock is taken during rasterization.
type FramePlan struct {
	Serial     uint64
	Viewport   Viewport
	Scale      float32
	Layers     []*Layer
	Background Color
}

// Layer returns the plan's layer by id, or nil.
func (p FramePlan) Layer(id LayerID) *Layer {
	for _, l := range p.Layers {
		if l != nil && l.ID == id {
			return l
		}
	}
	return nil
}

// Plan is a depth-1 slot holding the newest published frame plan.
//
// A channel would express the same handoff but either blocks the producer or
// buffers superseded plans, and the design rule is that a newer plan always
// replaces an unpublished older one. A mutex-protected single slot is that rule,
// with no queue to drain and nothing to leak.
type Plan struct {
	mu        sync.Mutex
	latest    FramePlan
	ok        bool
	consumed  bool
	published int64
	dropped   int64
}

// Publish replaces the pending plan. A plan superseded before the presenter
// consumed it is counted, because a sustained drop rate means content is being
// produced faster than the display can consume it.
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

// Latest returns the newest published plan without consuming it.
func (p *Plan) Latest() (FramePlan, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.latest, p.ok
}

// Take returns the newest plan and marks it consumed, so the next Publish can
// tell whether this one was actually presented. Re-taking the same plan without
// an intervening Publish is idempotent: presenting an unchanged plan across
// vsyncs is normal and must stay cheap.
func (p *Plan) Take() (FramePlan, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consumed = true
	return p.latest, p.ok
}

// PlanStats counts publishes and superseded drops.
type PlanStats struct {
	Published int64
	Dropped   int64
}

// Stats returns the publish counters.
func (p *Plan) Stats() PlanStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PlanStats{Published: p.published, Dropped: p.dropped}
}
