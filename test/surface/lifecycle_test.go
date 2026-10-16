package surface_test

import (
	"context"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
)

type inputScheduler struct {
	surface.Scheduler
	keys       []rune
	frameEvent surface.Event
}

func (s *inputScheduler) HandleEvent(ev surface.Event) bool {
	if ev.Kind == surface.EvKey {
		s.keys = append(s.keys, ev.Key)
		return true
	}
	return false
}

func (s *inputScheduler) BeginFrame(ev surface.Event) (*frame.FramePlan, []frame.TileCoord) {
	s.frameEvent = ev
	return nil, nil
}

func (s *inputScheduler) Submit([]frame.TileCoord) surface.FrameWork {
	return surface.FrameWork{}
}

func TestLoopRoutesDiscreteInputBeforeCoalescing(t *testing.T) {
	w := newFakeWindow(4)
	w.events <- surface.Event{Kind: surface.EvKey, Key: 'a'}
	w.events <- surface.Event{Kind: surface.EvKey, Key: 'b'}
	w.events <- surface.Event{Kind: surface.EvResize, Size: frame.Size{W: 640, H: 480}}
	w.events <- surface.Event{Kind: surface.EvVsync}
	close(w.events)
	s := &inputScheduler{}
	if err := surface.NewLoop(w, s, nil, nil).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if string(s.keys) != "ab" {
		t.Fatalf("discrete input = %q, want ab", string(s.keys))
	}
	if s.frameEvent.Key != 0 {
		t.Fatal("consumed key reached the frame scheduler")
	}
	if s.frameEvent.Size != (frame.Size{W: 640, H: 480}) {
		t.Fatalf("resize lost: %+v", s.frameEvent)
	}
}
