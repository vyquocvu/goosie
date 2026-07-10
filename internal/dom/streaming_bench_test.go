package dom

import (
	"context"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine/testpages"
)

// ---------------------------------------------------------------------------
// Streaming parse benchmarks (size-graduated)
// ---------------------------------------------------------------------------

func BenchmarkStreamParseSmall(b *testing.B) {
	p := NewParser()
	ctx := context.Background()
	cfg := ParseConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(smallHTML), cfg)
	}
}

func BenchmarkStreamParseMedium(b *testing.B) {
	p := NewParser()
	ctx := context.Background()
	cfg := ParseConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(mediumHTML), cfg)
	}
}

func BenchmarkStreamParseLarge(b *testing.B) {
	p := NewParser()
	ctx := context.Background()
	cfg := ParseConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(longFormHTML), cfg)
	}
}

// ---------------------------------------------------------------------------
// Streaming parse benchmarks (testpages corpus)
// ---------------------------------------------------------------------------

func BenchmarkStreamParseTableHeavy(b *testing.B) {
	page, ok := testpages.Get("table_heavy")
	if !ok {
		b.Fatal("table_heavy page not found")
	}
	p := NewParser()
	ctx := context.Background()
	cfg := ParseConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(page.HTML), cfg)
	}
}

func BenchmarkStreamParseFormHeavy(b *testing.B) {
	page, ok := testpages.Get("form_heavy")
	if !ok {
		b.Fatal("form_heavy page not found")
	}
	p := NewParser()
	ctx := context.Background()
	cfg := ParseConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(page.HTML), cfg)
	}
}

func BenchmarkStreamParseImageHeavy(b *testing.B) {
	page, ok := testpages.Get("image_heavy")
	if !ok {
		b.Fatal("image_heavy page not found")
	}
	p := NewParser()
	ctx := context.Background()
	cfg := ParseConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(page.HTML), cfg)
	}
}

func BenchmarkStreamParseScrollingShort(b *testing.B) {
	page, ok := testpages.Get("scrolling_short")
	if !ok {
		b.Fatal("scrolling_short page not found")
	}
	p := NewParser()
	ctx := context.Background()
	cfg := ParseConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(page.HTML), cfg)
	}
}

func BenchmarkStreamParseScrollingLong(b *testing.B) {
	page, ok := testpages.Get("scrolling_long")
	if !ok {
		b.Fatal("scrolling_long page not found")
	}
	p := NewParser()
	ctx := context.Background()
	cfg := ParseConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(page.HTML), cfg)
	}
}

// ---------------------------------------------------------------------------
// Comparison benchmarks: old ParseDocument vs new ParseDocumentCtx
// ---------------------------------------------------------------------------

func BenchmarkParseDocumentVsStreamSmall(b *testing.B) {
	b.Run("Old_ParseDocument", func(b *testing.B) {
		p := NewParser()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocument(strings.NewReader(smallHTML))
		}
	})
	b.Run("Stream_ParseDocumentCtx", func(b *testing.B) {
		p := NewParser()
		ctx := context.Background()
		cfg := ParseConfig{}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(smallHTML), cfg)
		}
	})
}

func BenchmarkParseDocumentVsStreamMedium(b *testing.B) {
	b.Run("Old_ParseDocument", func(b *testing.B) {
		p := NewParser()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocument(strings.NewReader(mediumHTML))
		}
	})
	b.Run("Stream_ParseDocumentCtx", func(b *testing.B) {
		p := NewParser()
		ctx := context.Background()
		cfg := ParseConfig{}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(mediumHTML), cfg)
		}
	})
}

func BenchmarkParseDocumentVsStreamLarge(b *testing.B) {
	b.Run("Old_ParseDocument", func(b *testing.B) {
		p := NewParser()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocument(strings.NewReader(longFormHTML))
		}
	})
	b.Run("Stream_ParseDocumentCtx", func(b *testing.B) {
		p := NewParser()
		ctx := context.Background()
		cfg := ParseConfig{}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(longFormHTML), cfg)
		}
	})
}

func BenchmarkParseDocumentVsStreamTableHeavy(b *testing.B) {
	page, ok := testpages.Get("table_heavy")
	if !ok {
		b.Fatal("table_heavy page not found")
	}
	b.Run("Old_ParseDocument", func(b *testing.B) {
		p := NewParser()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocument(strings.NewReader(page.HTML))
		}
	})
	b.Run("Stream_ParseDocumentCtx", func(b *testing.B) {
		p := NewParser()
		ctx := context.Background()
		cfg := ParseConfig{}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(page.HTML), cfg)
		}
	})
}

func BenchmarkParseDocumentVsStreamFormHeavy(b *testing.B) {
	page, ok := testpages.Get("form_heavy")
	if !ok {
		b.Fatal("form_heavy page not found")
	}
	b.Run("Old_ParseDocument", func(b *testing.B) {
		p := NewParser()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocument(strings.NewReader(page.HTML))
		}
	})
	b.Run("Stream_ParseDocumentCtx", func(b *testing.B) {
		p := NewParser()
		ctx := context.Background()
		cfg := ParseConfig{}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(page.HTML), cfg)
		}
	})
}

// ---------------------------------------------------------------------------
// Node throughput benchmark
// ---------------------------------------------------------------------------

// BenchmarkStreamParseNodeThroughput measures nodes/second for a large document.
func BenchmarkStreamParseNodeThroughput(b *testing.B) {
	p := NewParser()
	ctx := context.Background()
	cfg := ParseConfig{}

	// Do one parse to get the actual node count.
	doc, err := p.ParseDocumentCtx(ctx, strings.NewReader(longFormHTML), cfg)
	if err != nil {
		b.Fatal(err)
	}
	nodeCount := 0
	for it := doc.Store.Subtree(doc.Root); it.Next(); {
		nodeCount++
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d, err := p.ParseDocumentCtx(ctx, strings.NewReader(longFormHTML), cfg)
		if err != nil {
			b.Fatal(err)
		}
		_ = d
	}
	b.StopTimer()
	b.ReportMetric(float64(nodeCount), "nodes/op")
}
