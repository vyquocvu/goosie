package testpages

import (
	"context"
	"testing"
)

func BenchmarkList(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pages := List()
		if len(pages) == 0 {
			b.Fatal("empty corpus")
		}
	}
}

func BenchmarkGetLongArticle(b *testing.B) {
	benchmarkGet(b, "long_article")
}

func BenchmarkGetDocumentation(b *testing.B) {
	benchmarkGet(b, "documentation")
}

func BenchmarkGetContextDocumentation(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		page, err := GetContext(ctx, "documentation")
		if err != nil {
			b.Fatal(err)
		}
		if page.Name == "" {
			b.Fatal("empty page")
		}
	}
}

func benchmarkGet(b *testing.B, name string) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		page, ok := Get(name)
		if !ok {
			b.Fatalf("missing page %q", name)
		}
		if page.Name == "" {
			b.Fatal("empty page")
		}
	}
}
