package eventloop

import "sync/atomic"

// Metrics is an immutable snapshot of event-loop counters.
type Metrics struct {
	InputEventsReceived   uint64
	InputSignalsDropped   uint64
	CoalescedScrollEvents uint64
	CoalescedMouseMoves   uint64
	CoalescedResizeEvents uint64

	RenderRequestsCreated uint64
	RenderRequestsDropped uint64
	RenderErrors          uint64
	StaleFramesDropped    uint64
	FramesPresented       uint64
}

type metricState struct {
	inputEventsReceived   atomic.Uint64
	inputSignalsDropped   atomic.Uint64
	coalescedScrollEvents atomic.Uint64
	coalescedMouseMoves   atomic.Uint64
	coalescedResizeEvents atomic.Uint64
	renderRequestsCreated atomic.Uint64
	renderRequestsDropped atomic.Uint64
	renderErrors          atomic.Uint64
	staleFramesDropped    atomic.Uint64
	framesPresented       atomic.Uint64
}

func (m *metricState) snapshot() Metrics {
	return Metrics{
		InputEventsReceived:   m.inputEventsReceived.Load(),
		InputSignalsDropped:   m.inputSignalsDropped.Load(),
		CoalescedScrollEvents: m.coalescedScrollEvents.Load(),
		CoalescedMouseMoves:   m.coalescedMouseMoves.Load(),
		CoalescedResizeEvents: m.coalescedResizeEvents.Load(),
		RenderRequestsCreated: m.renderRequestsCreated.Load(),
		RenderRequestsDropped: m.renderRequestsDropped.Load(),
		RenderErrors:          m.renderErrors.Load(),
		StaleFramesDropped:    m.staleFramesDropped.Load(),
		FramesPresented:       m.framesPresented.Load(),
	}
}
