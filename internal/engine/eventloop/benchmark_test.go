package eventloop

import (
	"context"
	"testing"
)

func BenchmarkBurstScrollScheduling(b *testing.B) {
	const burst = 10_000
	loop := New(Config{})
	event := InputEvent{Type: InputScroll, Viewport: Viewport{Height: 800}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < burst; j++ {
			event.Viewport.Y = float32(j)
			_ = loop.PostInput(event)
		}
		events := loop.DrainInput()
		_, _ = loop.ScheduleRender(context.Background(), RenderRequest{
			Generation: Generation{Viewport: uint64(i + 1)},
			Viewport:   events[0].Viewport,
		})
	}
}

func BenchmarkRenderRequestReplacement(b *testing.B) {
	loop := New(Config{})
	req := RenderRequest{Generation: Generation{Navigation: 1}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = loop.ScheduleRender(context.Background(), req)
	}
}
