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

func BenchmarkGetTableHeavy(b *testing.B) {
	benchmarkGet(b, "table_heavy")
}

func BenchmarkGetFormHeavy(b *testing.B) {
	benchmarkGet(b, "form_heavy")
}

func BenchmarkGetImageHeavy(b *testing.B) {
	benchmarkGet(b, "image_heavy")
}

func BenchmarkGetJavaScriptLightTodo(b *testing.B) {
	benchmarkGet(b, "javascript_light_todo")
}

func BenchmarkGetScrollingShort(b *testing.B) {
	benchmarkGet(b, "scrolling_short")
}

func BenchmarkGetScrollingLong(b *testing.B) {
	benchmarkGet(b, "scrolling_long")
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

func BenchmarkGetContextTableHeavy(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		page, err := GetContext(ctx, "table_heavy")
		if err != nil {
			b.Fatal(err)
		}
		if page.Name == "" {
			b.Fatal("empty page")
		}
	}
}

func BenchmarkGetContextFormHeavy(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		page, err := GetContext(ctx, "form_heavy")
		if err != nil {
			b.Fatal(err)
		}
		if page.Name == "" {
			b.Fatal("empty page")
		}
	}
}

func BenchmarkGetContextImageHeavy(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		page, err := GetContext(ctx, "image_heavy")
		if err != nil {
			b.Fatal(err)
		}
		if page.Name == "" {
			b.Fatal("empty page")
		}
	}
}

func BenchmarkGetContextJavaScriptLightTodo(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		page, err := GetContext(ctx, "javascript_light_todo")
		if err != nil {
			b.Fatal(err)
		}
		if page.Name == "" {
			b.Fatal("empty page")
		}
	}
}

func BenchmarkGetContextScrollingShort(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		page, err := GetContext(ctx, "scrolling_short")
		if err != nil {
			b.Fatal(err)
		}
		if page.Name == "" {
			b.Fatal("empty page")
		}
	}
}

func BenchmarkGetContextScrollingLong(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		page, err := GetContext(ctx, "scrolling_long")
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
