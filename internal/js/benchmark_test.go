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
