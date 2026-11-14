package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// A screenshot answers one question - what does this page look like once it is
// done drawing - and QA-002 asks the next one: does it stay that way? The
// evidence for that is a run that opens the page once and records the same
// viewport again and again, so the answer cannot be an artefact of navigating
// twice. Five captures a second apart is what docs/qa-test-plan.md §QA-002
// specifies, and docs/qa-test-plan.md QA-002 forbids closing the flicker question
// on PNG byte hashes, so every capture also records the frame-path counters
// that say how the page looked at that moment, and the comparison itself is
// left to pixels.
//
// frameSequence is that recorder. It is driven entirely by the settle law it is
// attached to: the driver asks it on frames it would otherwise end the run on,
// which is the only moment the engine can claim the PNG it writes is complete.
// Captures therefore land at least -seq-interval apart and never on a frame the
// run would not have written by itself.

// frameShot is one recorded capture: where its pixels are, when the run took
// them, and what the frame path said about itself at that instant. The counters
// are the part that separates "the page moved" from "the harness sampled at a
// different moment": a frame that still wanted tiles or held work on order was
// not complete, and the pixels that differ between two of those prove nothing.
type frameShot struct {
	PNG           string `json:"png"`
	MS            int64  `json:"ms"`
	Idle          bool   `json:"idle"`
	TilesNeeded   int    `json:"tiles_needed"`
	PaintingBytes int64  `json:"painting_bytes"`
	Presents      int64  `json:"presents"`
	Frames        int64  `json:"frames"`
}

// sequenceReport is frames.json: the conditions a live case has to be recorded
// with (docs/qa-test-catalog.md reading rules: time, OS, window size, zoom) and
// the captures in order. Build SHA and network state belong to whoever ran it,
// which is why scripts/live-frame-stability.py records them next to this file.
type sequenceReport struct {
	URL        string      `json:"url"`
	Started    time.Time   `json:"started"`
	OS         string      `json:"os"`
	Arch       string      `json:"arch"`
	Backend    string      `json:"backend"`
	ViewportW  int         `json:"viewport_width"`
	ViewportH  int         `json:"viewport_height"`
	DPR        float64     `json:"dpr"`
	Zoom       float64     `json:"zoom"`
	IntervalMS int64       `json:"interval_ms"`
	Frames     []frameShot `json:"frames"`
	Error      string      `json:"error,omitempty"`
}

// frameSequence writes the captures a -screenshot-seq run owes and the record
// that describes them.
type frameSequence struct {
	dir      string
	count    int
	interval time.Duration
	writePNG func(string) error
	stats    func() (idle bool, needed int, painting int64, presents, frames int64)
	started  time.Time
	report   sequenceReport
	next     time.Time
	err      error
}

func newFrameSequence(c config, dir string, count int, interval time.Duration, writePNG func(string) error, stats func() (bool, int, int64, int64, int64), zoom float64, backend string) *frameSequence {
	return &frameSequence{
		dir:      dir,
		count:    count,
		interval: interval,
		writePNG: writePNG,
		stats:    stats,
		started:  time.Now(),
		report: sequenceReport{
			URL:        c.url,
			Started:    time.Now(),
			OS:         runtime.GOOS,
			Arch:       runtime.GOARCH,
			Backend:    backend,
			ViewportW:  c.width,
			ViewportH:  c.height,
			DPR:        c.dpr,
			Zoom:       zoom,
			IntervalMS: interval.Milliseconds(),
		},
	}
}

// horizon is the wall time a full sequence needs, which is what the driver adds
// to its deadline: the interval between every pair of captures, with the run's
// own settling already inside the time the screenshot deadline gives it.
func (s *frameSequence) horizon() time.Duration {
	return time.Duration(s.count) * s.interval
}

// capture is the hold the driver consults on a frame it would have ended the run
// on. It returns whether the sequence still owes captures, so the run keeps
// pacing until the last one is written and then finishes like any screenshot.
func (s *frameSequence) capture(now time.Time) bool {
	if len(s.report.Frames) >= s.count {
		return false
	}
	if len(s.report.Frames) > 0 && now.Before(s.next) {
		return true
	}
	name := fmt.Sprintf("frame-%03d.png", len(s.report.Frames))
	idle, needed, painting, presents, frames := s.stats()
	shot := frameShot{
		PNG:           name,
		MS:            now.Sub(s.started).Milliseconds(),
		Idle:          idle,
		TilesNeeded:   needed,
		PaintingBytes: painting,
		Presents:      presents,
		Frames:        frames,
	}
	if err := s.writePNG(filepath.Join(s.dir, name)); err != nil {
		s.err = err
		return false
	}
	s.report.Frames = append(s.report.Frames, shot)
	s.next = now.Add(s.interval)
	return len(s.report.Frames) < s.count
}

// write records frames.json and returns the run's first capture error, if the
// sequence never got its pixels there is no evidence to grade.
func (s *frameSequence) write() error {
	if s.err != nil {
		return s.err
	}
	s.report.Error = ""
	body, err := json.MarshalIndent(s.report, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(filepath.Join(s.dir, "frames.json"), body, 0o644)
}
