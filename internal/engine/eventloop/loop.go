package eventloop

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	defaultInputQueueSize = 128
	renderQueueSize       = 1
)

var (
	// ErrClosed is returned when work is posted after the loop closes.
	ErrClosed = errors.New("eventloop: closed")
	// ErrInputQueueFull is returned when the bounded ordered-input queue is full.
	ErrInputQueueFull = errors.New("eventloop: input queue full")
)

// Config configures a Loop.
type Config struct {
	InputQueueSize int
	FrameBudget    FrameBudget
	Present        func(RenderResult)
}

// Loop owns bounded input scheduling, render replacement, generation checks,
// and presentation eligibility. It does not start worker goroutines.
type Loop struct {
	Mu sync.Mutex

	ordered    []InputEvent
	inputHead  int
	inputCount int

	latestScroll InputEvent
	hasScroll    bool
	latestMouse  InputEvent
	hasMouse     bool
	latestResize InputEvent
	hasResize    bool

	generation Generation

	renderRequests chan RenderRequest
	renderResults  chan RenderResult
	wake           chan struct{}
	done           chan struct{}
	closeOnce      sync.Once
	closed         bool

	cancelRender context.CancelFunc
	budget       FrameBudget
	present      func(RenderResult)
	metrics      metricState
	RunCtx       context.Context
}

// New creates a stopped event loop. Call Run to process render results.
func New(cfg Config) *Loop {
	if cfg.InputQueueSize <= 0 {
		cfg.InputQueueSize = defaultInputQueueSize
	}
	if cfg.FrameBudget.Duration <= 0 {
		cfg.FrameBudget = NewFrameBudget(0)
	}
	return &Loop{
		ordered:        make([]InputEvent, cfg.InputQueueSize),
		renderRequests: make(chan RenderRequest, renderQueueSize),
		renderResults:  make(chan RenderResult, renderQueueSize),
		wake:           make(chan struct{}, 1),
		done:           make(chan struct{}),
		budget:         cfg.FrameBudget,
		present:        cfg.Present,
	}
}

// PostInput schedules an input event. Scroll, mouse move, and resize use
// latest-wins slots; click and key events use the bounded FIFO.
func (l *Loop) PostInput(event InputEvent) error {
	l.Mu.Lock()
	defer l.Mu.Unlock()
	if l.closed {
		return ErrClosed
	}

	l.metrics.inputEventsReceived.Add(1)
	switch event.Type {
	case InputScroll:
		if l.hasScroll {
			l.metrics.coalescedScrollEvents.Add(1)
		}
		l.latestScroll, l.hasScroll = event, true
	case InputMouseMove:
		if l.hasMouse {
			l.metrics.coalescedMouseMoves.Add(1)
		}
		l.latestMouse, l.hasMouse = event, true
	case InputResize:
		if l.hasResize {
			l.metrics.coalescedResizeEvents.Add(1)
		}
		l.latestResize, l.hasResize = event, true
	default:
		if l.inputCount == len(l.ordered) {
			l.metrics.inputSignalsDropped.Add(1)
			return ErrInputQueueFull
		}
		idx := (l.inputHead + l.inputCount) % len(l.ordered)
		l.ordered[idx] = event
		l.inputCount++
	}
	l.signal()
	return nil
}

// DrainInput returns pending input in deterministic policy order: ordered user
// intent first, then latest resize, scroll, and mouse-move state.
func (l *Loop) DrainInput() []InputEvent {
	l.Mu.Lock()
	defer l.Mu.Unlock()

	capacity := l.inputCount
	if l.hasResize {
		capacity++
	}
	if l.hasScroll {
		capacity++
	}
	if l.hasMouse {
		capacity++
	}
	out := make([]InputEvent, 0, capacity)
	for l.inputCount > 0 {
		out = append(out, l.ordered[l.inputHead])
		l.ordered[l.inputHead] = InputEvent{}
		l.inputHead = (l.inputHead + 1) % len(l.ordered)
		l.inputCount--
	}
	if l.hasResize {
		out = append(out, l.latestResize)
		l.hasResize = false
	}
	if l.hasScroll {
		out = append(out, l.latestScroll)
		l.hasScroll = false
	}
	if l.hasMouse {
		out = append(out, l.latestMouse)
		l.hasMouse = false
	}
	return out
}

// SetGeneration replaces the current engine generation and cancels any render
// scheduled under the prior generation.
func (l *Loop) SetGeneration(g Generation) {
	l.Mu.Lock()
	if !l.generation.Matches(g) && l.cancelRender != nil {
		l.cancelRender()
		l.cancelRender = nil
	}
	l.generation = g
	l.Mu.Unlock()
}

// Generation returns the current generation.
func (l *Loop) Generation() Generation {
	l.Mu.Lock()
	defer l.Mu.Unlock()
	return l.generation
}

// ScheduleRender creates a child context and replaces any queued render
// request. The returned request is the value visible to a worker.
func (l *Loop) ScheduleRender(parent context.Context, req RenderRequest) (RenderRequest, error) {
	if parent == nil {
		parent = context.Background()
	}
	l.Mu.Lock()
	defer l.Mu.Unlock()
	if l.closed {
		return RenderRequest{}, ErrClosed
	}
	if l.cancelRender != nil {
		l.cancelRender()
	}
	ctx, cancel := context.WithCancel(parent)
	l.cancelRender = cancel
	req.Context = ctx
	if req.Created.IsZero() {
		req.Created = now()
	}
	l.metrics.renderRequestsCreated.Add(1)

	select {
	case <-l.renderRequests:
		l.metrics.renderRequestsDropped.Add(1)
	default:
	}
	l.renderRequests <- req
	l.signal()
	return req, nil
}

// RenderRequests returns the bounded worker-facing render queue.
func (l *Loop) RenderRequests() <-chan RenderRequest { return l.renderRequests }

// SubmitRenderResult queues a completed result for generation and cancellation
// checks. A newer completion replaces an older unprocessed completion.
func (l *Loop) SubmitRenderResult(result RenderResult) error {
	l.Mu.Lock()
	defer l.Mu.Unlock()
	if l.closed {
		return ErrClosed
	}
	select {
	case <-l.renderResults:
		l.metrics.staleFramesDropped.Add(1)
	default:
	}
	l.renderResults <- result
	l.signal()
	return nil
}

// HandleRenderResult processes a single render result, applying generation and
// cancellation checks before presenting.
func (l *Loop) HandleRenderResult(result RenderResult) {
	if result.Err != nil {
		l.metrics.renderErrors.Add(1)
		return
	}
	if result.Request.Context != nil && result.Request.Context.Err() != nil {
		l.metrics.staleFramesDropped.Add(1)
		return
	}
	l.Mu.Lock()
	current := l.generation
	present := l.present
	closed := l.closed
	runCtx := l.RunCtx
	l.Mu.Unlock()
	if closed || (runCtx != nil && runCtx.Err() != nil) || !result.Request.Generation.Matches(current) {
		l.metrics.staleFramesDropped.Add(1)
		return
	}
	if present != nil {
		present(result)
	}
	l.metrics.framesPresented.Add(1)
}

// ProcessPendingResults processes any completed render results already
// queued via SubmitRenderResult without blocking, applying the same
// generation and cancellation checks as Run. It returns the number of
// results processed. Integrations that present synchronously on their own
// thread (e.g. a UI thread without a dedicated worker goroutine) call
// this after submitting a result so presentation runs on that thread.
func (l *Loop) ProcessPendingResults() int {
	processed := 0
	for {
		select {
		case result := <-l.renderResults:
			l.HandleRenderResult(result)
			processed++
		default:
			return processed
		}
	}
}

// Run processes completed render results until ctx is cancelled or Close is
// called. It is safe to run on a caller-owned goroutine.
func (l *Loop) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	l.Mu.Lock()
	if l.closed {
		l.Mu.Unlock()
		return nil
	}
	l.RunCtx = ctx
	l.Mu.Unlock()
	defer l.Close()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-l.done:
			return nil
		case result := <-l.renderResults:
			l.HandleRenderResult(result)
		case <-l.wake:
		}
	}
}

// Close cancels pending render work and shuts the loop down idempotently.
func (l *Loop) Close() {
	l.closeOnce.Do(func() {
		l.Mu.Lock()
		l.closed = true
		if l.cancelRender != nil {
			l.cancelRender()
			l.cancelRender = nil
		}
		l.Mu.Unlock()
		close(l.done)
	})
}

// Done is closed after Close or Run shutdown.
func (l *Loop) Done() <-chan struct{} { return l.done }

// Metrics returns an immutable counter snapshot.
func (l *Loop) Metrics() Metrics { return l.metrics.snapshot() }

// FrameBudget returns the configured scheduling budget.
func (l *Loop) FrameBudget() FrameBudget { return l.budget }

func (l *Loop) signal() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

var now = func() time.Time { return time.Now() }
