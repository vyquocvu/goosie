package dom_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine/testpages"
	"github.com/vyquocvu/goosie/internal/dom"
)

// ---------------------------------------------------------------------------
// Streaming parse benchmarks (size-graduated)
// ---------------------------------------------------------------------------

func BenchmarkStreamParseSmall(b *testing.B) {
	p := dom.NewParser()
	ctx := context.Background()
	cfg := dom.ParseConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(smallHTML), cfg)
	}
}

func BenchmarkStreamParseMedium(b *testing.B) {
	p := dom.NewParser()
	ctx := context.Background()
	cfg := dom.ParseConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(mediumHTML), cfg)
	}
}

func BenchmarkStreamParseLarge(b *testing.B) {
	p := dom.NewParser()
	ctx := context.Background()
	cfg := dom.ParseConfig{}
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
	p := dom.NewParser()
	ctx := context.Background()
	cfg := dom.ParseConfig{}
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
	p := dom.NewParser()
	ctx := context.Background()
	cfg := dom.ParseConfig{}
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
	p := dom.NewParser()
	ctx := context.Background()
	cfg := dom.ParseConfig{}
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
	p := dom.NewParser()
	ctx := context.Background()
	cfg := dom.ParseConfig{}
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
	p := dom.NewParser()
	ctx := context.Background()
	cfg := dom.ParseConfig{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(page.HTML), cfg)
	}
}

// ---------------------------------------------------------------------------
// Unsupported feature detection benchmarks
// ---------------------------------------------------------------------------

func BenchmarkStreamParseWithUnsupportedDetection(b *testing.B) {
	input := `<html><body>
		<p>Normal paragraph of text for testing purposes.</p>
		<canvas id="game" width="800" height="600"></canvas>
		<p>Another paragraph with more content here.</p>
		<video src="video.mp4" controls></video>
		<p>Yet another paragraph to provide page structure.</p>
		<iframe src="embed.html" width="300" height="200"></iframe>
		<p>Final paragraph closing out the page body.</p>
	</body></html>`

	b.Run("no_callback", func(b *testing.B) {
		parser := dom.NewParser()
		ctx := context.Background()
		cfg := dom.ParseConfig{}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = parser.ParseDocumentCtx(ctx, strings.NewReader(input), cfg)
		}
	})

	b.Run("with_callback", func(b *testing.B) {
		parser := dom.NewParser()
		ctx := context.Background()
		var count int
		cfg := dom.ParseConfig{
			OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
				count++
			},
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = parser.ParseDocumentCtx(ctx, strings.NewReader(input), cfg)
		}
		_ = count
	})
}

func BenchmarkStreamParseUnsupportedHeavy(b *testing.B) {
	// Page with many canvas elements to stress detection.
	var sb strings.Builder
	sb.WriteString("<html><body>")
	for i := 0; i < 100; i++ {
		sb.WriteString("<canvas id=\"c")
		sb.WriteString(string(rune('0' + i%10)))
		sb.WriteString("\"></canvas>\n")
	}
	sb.WriteString("</body></html>")
	input := sb.String()

	b.Run("no_callback", func(b *testing.B) {
		parser := dom.NewParser()
		ctx := context.Background()
		cfg := dom.ParseConfig{}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = parser.ParseDocumentCtx(ctx, strings.NewReader(input), cfg)
		}
	})

	b.Run("with_callback", func(b *testing.B) {
		parser := dom.NewParser()
		ctx := context.Background()
		var count int
		cfg := dom.ParseConfig{
			OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
				count++
			},
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = parser.ParseDocumentCtx(ctx, strings.NewReader(input), cfg)
		}
		_ = count
	})
}

// ---------------------------------------------------------------------------
// Comparison benchmarks: old ParseDocument vs new ParseDocumentCtx
// ---------------------------------------------------------------------------

func BenchmarkParseDocumentVsStreamSmall(b *testing.B) {
	b.Run("Old_ParseDocument", func(b *testing.B) {
		p := dom.NewParser()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocument(strings.NewReader(smallHTML))
		}
	})
	b.Run("Stream_ParseDocumentCtx", func(b *testing.B) {
		p := dom.NewParser()
		ctx := context.Background()
		cfg := dom.ParseConfig{}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(smallHTML), cfg)
		}
	})
}

func BenchmarkParseDocumentVsStreamMedium(b *testing.B) {
	b.Run("Old_ParseDocument", func(b *testing.B) {
		p := dom.NewParser()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocument(strings.NewReader(mediumHTML))
		}
	})
	b.Run("Stream_ParseDocumentCtx", func(b *testing.B) {
		p := dom.NewParser()
		ctx := context.Background()
		cfg := dom.ParseConfig{}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocumentCtx(ctx, strings.NewReader(mediumHTML), cfg)
		}
	})
}

func BenchmarkParseDocumentVsStreamLarge(b *testing.B) {
	b.Run("Old_ParseDocument", func(b *testing.B) {
		p := dom.NewParser()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocument(strings.NewReader(longFormHTML))
		}
	})
	b.Run("Stream_ParseDocumentCtx", func(b *testing.B) {
		p := dom.NewParser()
		ctx := context.Background()
		cfg := dom.ParseConfig{}
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
		p := dom.NewParser()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocument(strings.NewReader(page.HTML))
		}
	})
	b.Run("Stream_ParseDocumentCtx", func(b *testing.B) {
		p := dom.NewParser()
		ctx := context.Background()
		cfg := dom.ParseConfig{}
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
		p := dom.NewParser()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = p.ParseDocument(strings.NewReader(page.HTML))
		}
	})
	b.Run("Stream_ParseDocumentCtx", func(b *testing.B) {
		p := dom.NewParser()
		ctx := context.Background()
		cfg := dom.ParseConfig{}
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
	p := dom.NewParser()
	ctx := context.Background()
	cfg := dom.ParseConfig{}

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
