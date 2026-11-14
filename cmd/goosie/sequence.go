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
	ScrollY       int32  `json:"scroll_y"`
}

type seqScroll struct {
	Screens int
	Screen  func() int32
	Scroll  func(int32)
	Offset  func() int32
}

type sequenceReport struct {
	URL           string      `json:"url"`
	Started       time.Time   `json:"started"`
	OS            string      `json:"os"`
	Arch          string      `json:"arch"`
	Backend       string      `json:"backend"`
	ViewportW     int         `json:"viewport_width"`
	ViewportH     int         `json:"viewport_height"`
	DPR           float64     `json:"dpr"`
	Zoom          float64     `json:"zoom"`
	IntervalMS    int64       `json:"interval_ms"`
	ScrollScreens int         `json:"scroll_screens"`
	ScrollPresses int         `json:"scroll_presses"`
	ScrollLanded  *bool       `json:"scroll_landed,omitempty"`
	Frames        []frameShot `json:"frames"`
	Error         string      `json:"error,omitempty"`
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
	scroll   seqScroll
	presses  int
	awaiting bool
	moved    bool
	from     int32
	drawn    int64
	finished bool
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

func (s *frameSequence) setScroll(sc seqScroll) {
	s.scroll = sc
	s.report.ScrollScreens = sc.Screens
}

func (s *frameSequence) horizon() time.Duration {
	return time.Duration(s.count+s.scroll.Screens) * s.interval
}

func (s *frameSequence) finish(landed bool) {
	s.report.ScrollLanded = &landed
	s.finished = true
}

func (s *frameSequence) press(drawn int64) {
	s.from = s.scroll.Offset()
	s.drawn = drawn
	s.presses++
	s.report.ScrollPresses = s.presses
	s.scroll.Scroll(s.scroll.Screen())
	s.awaiting = true
}

func (s *frameSequence) rolling() bool {
	if s.scroll.Screens <= 0 || s.finished {
		return false
	}
	_, _, _, _, drawn := s.stats()
	if !s.awaiting {
		s.press(drawn)
		return true
	}
	if drawn < s.drawn+2 {
		return true
	}
	s.awaiting = false
	if s.scroll.Offset() == s.from {
		s.finish(s.moved)
		return false
	}
	s.moved = true
	if s.presses >= s.scroll.Screens {
		s.finish(true)
		return false
	}
	s.press(drawn)
	return true
}

func (s *frameSequence) capture(now time.Time) bool {
	if len(s.report.Frames) >= s.count {
		return false
	}
	if len(s.report.Frames) > 0 && now.Before(s.next) {
		return true
	}
	if s.rolling() {
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
		ScrollY:       s.scroll.Offset(),
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
