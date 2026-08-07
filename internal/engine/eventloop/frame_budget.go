package eventloop

import "time"

// FrameBudget defines the maximum time available for a scheduling tick.
type FrameBudget struct {
	Duration time.Duration
}

// NewFrameBudget creates a budget. Non-positive durations use a 60 Hz frame.
func NewFrameBudget(duration time.Duration) FrameBudget {
	if duration <= 0 {
		duration = time.Second / 60
	}
	return FrameBudget{Duration: duration}
}

// Remaining returns the unused budget at now for work started at start.
func (b FrameBudget) Remaining(start, now time.Time) time.Duration {
	remaining := b.Duration - now.Sub(start)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Exhausted reports whether work has consumed the frame budget.
func (b FrameBudget) Exhausted(start, now time.Time) bool {
	return b.Remaining(start, now) == 0
}

// Slice returns a bounded share of the remaining budget.
func (b FrameBudget) Slice(start, now time.Time, maximum time.Duration) time.Duration {
	remaining := b.Remaining(start, now)
	if maximum > 0 && remaining > maximum {
		return maximum
	}
	return remaining
}
