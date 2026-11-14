package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type frameShot struct {
	PNG           string `json:"png"`
	MS            int64  `json:"ms"`
	Idle          bool   `json:"idle"`
	TilesNeeded   int    `json:"tiles_needed"`
	PaintingBytes int64  `json:"painting_bytes"`
	Presents      int64  `json:"presents"`
	Frames        int64  `json:"frames"`
}

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

func (s *frameSequence) horizon() time.Duration {
	return time.Duration(s.count) * s.interval
}

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
