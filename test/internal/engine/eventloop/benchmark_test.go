package eventloop_test

import (
	"context"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine/eventloop"
)

func BenchmarkBurstScrollScheduling(b *testing.B) {
	const burst = 10_000
	loop := eventloop.New(eventloop.Config{})
	event := eventloop.InputEvent{Type: eventloop.InputScroll, Viewport: eventloop.Viewport{Height: 800}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < burst; j++ {
			event.Viewport.Y = float32(j)
			_ = loop.PostInput(event)
		}
		events := loop.DrainInput()
		_, _ = loop.ScheduleRender(context.Background(), eventloop.RenderRequest{
			Generation: eventloop.Generation{Viewport: uint64(i + 1)},
			Viewport:   events[0].Viewport,
		})
	}
}

func BenchmarkRenderRequestReplacement(b *testing.B) {
	loop := eventloop.New(eventloop.Config{})
	req := eventloop.RenderRequest{Generation: eventloop.Generation{Navigation: 1}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loop.ScheduleRender(context.Background(), req)
	}
}
