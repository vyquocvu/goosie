package session

import (
	"context"
	"testing"
)

func BenchmarkSessionCreate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = New()
	}
}

func BenchmarkSessionNavigate(b *testing.B) {
	s := New()
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		load, navCtx := s.Navigate(ctx, "https://example.com")
		_ = load
		_ = navCtx
	}
}

func BenchmarkSessionNavigateComplete(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := New()
		s.Navigate(ctx, "https://example.com")
		s.Complete()
	}
}

func BenchmarkSessionFullLifecycle(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := New()
		_, navCtx := s.Navigate(ctx, "https://example.com")
		_ = navCtx
		s.Parsing()
		s.Interactive()
		s.Complete()
	}
}

func BenchmarkSessionConcurrentNavigate(b *testing.B) {
	s := New()
	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			load, navCtx := s.Navigate(ctx, "https://example.com")
			_ = load
			_ = navCtx
		}
	})
}

func BenchmarkSessionStateAccess(b *testing.B) {
	s := New()
	s.Navigate(context.Background(), "https://example.com")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.State()
		_ = s.NavID()
		_ = s.ActiveURL()
		_ = s.IsActive(s.NavID())
	}
}

func BenchmarkSessionClose(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := New()
		s.Navigate(context.Background(), "https://example.com")
		s.Close()
	}
}

func BenchmarkSessionTransportCreation(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = defaultTransport()
	}
}

func BenchmarkSessionHTTPClient(b *testing.B) {
	s := New()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.HTTPClient()
	}
}

func BenchmarkSessionTransportAccess(b *testing.B) {
	s := New()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.Transport()
	}
}
