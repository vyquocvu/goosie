package js

import (
	"testing"
)

func BenchmarkRuntimeCreate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rt := NewRuntime()
		rt.Cleanup()
	}
}

func BenchmarkRuntimeRunScriptSimple(b *testing.B) {
	rt := NewRuntime()
	defer rt.Cleanup()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := rt.RunScript("1 + 1")
		if err != nil {
			b.Fatalf("RunScript failed: %v", err)
		}
	}
}

func BenchmarkRuntimeRunScriptDOM(b *testing.B) {
	rt := NewRuntime()
	defer rt.Cleanup()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := rt.RunScript("document.createElement('div');")
		if err != nil {
			b.Fatalf("RunScript failed: %v", err)
		}
	}
}
func BenchmarkDOMMutationNotification(b *testing.B) {
	const page = `<html><body><main><section><p>alpha</p><p>beta</p><p>gamma</p><p>delta</p></section></main></body></html>`
	b.Run("LegacySerialize", func(b *testing.B) {
		rt := NewRuntime()
		defer rt.Cleanup()
		rt.SetHTMLContent(page)
		rt.SetDOMMutationCallback(func(string) {})
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := rt.RunScript(`document.body.firstChild.firstChild.firstChild.textContent = "updated"`)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("TypedBatch", func(b *testing.B) {
		rt := NewRuntime()
		defer rt.Cleanup()
		rt.SetHTMLContent(page)
		rt.SetDOMMutationBatchCallback(func([]DOMMutation) {})
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := rt.RunScript(`document.body.firstChild.firstChild.firstChild.textContent = "updated"`)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkMutationBatchFlush(b *testing.B) {
	el := NewEventLoop(DefaultEventLoopConfig())
	el.SetMutationBatchFlush(func([]DOMMutation) {})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		el.QueueTask(func() {
			for j := 0; j < 100; j++ {
				el.RecordMutation()
			}
		})
		el.RunOnce()
	}
}
