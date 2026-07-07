// Package metrics provides phase-level instrumentation for the browser engine.
//
// Each navigation records timing and counters through a Recorder that is safe
// for concurrent use. Metrics are exported without importing UI packages.
//
// Optional structured debug logging is available via Recorder.SetDebugLog. When
// enabled, a structured record (navigation id, url, total duration, per-phase
// durations, and a counters group) is emitted through log/slog on Finalize and
// on explicit LogStructured calls. Debug logging is a no-op when disabled and
// after Finalize, so it never affects hot paths in production builds.
package metrics

import "fmt"

// Phase identifies a rendering pipeline stage for timing.
type Phase int

const (
	PhaseNavigation Phase = iota
	PhaseDNSResolve
	PhaseConnect
	PhaseFirstByte
	PhaseBodyRead
	PhaseParse
	PhaseStyle
	PhaseLayout
	PhasePaint
	PhaseRaster
	PhasePresent
)

var phaseStrings = map[Phase]string{
	PhaseNavigation: "navigation",
	PhaseDNSResolve: "dns_resolve",
	PhaseConnect:    "connect",
	PhaseFirstByte:  "first_byte",
	PhaseBodyRead:   "body_read",
	PhaseParse:      "parse",
	PhaseStyle:      "style",
	PhaseLayout:     "layout",
	PhasePaint:      "paint",
	PhaseRaster:     "raster",
	PhasePresent:    "present",
}

func (p Phase) String() string {
	if s, ok := phaseStrings[p]; ok {
		return s
	}
	return fmt.Sprintf("phase_%d", int(p))
}
