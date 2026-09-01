package css_test

import "github.com/vyquocvu/goosie/internal/css"

import "testing"

// BenchmarkM3.1_PropertyInterning benchmarks property name interning
func BenchmarkPropertyInterning(b *testing.B) {
	cssText := `p { color: red; font-size: 16px; margin: 10px; padding: 5px; }`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := css.NewParser(cssText)
		_, err := p.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkM3.1_HotColdClassification benchmarks hot/cold classification
func BenchmarkHotColdClassification(b *testing.B) {
	cssText := `
		div { display: block; color: red; font-size: 16px; margin: 10px; }
		p { padding: 5px; background: blue; border: 1px solid black; }
		span { width: 100px; height: 50px; -webkit-transform: rotate(45deg); }
	`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := css.NewParser(cssText)
		_, err := p.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkM3.1_SourceOrderTracking benchmarks source order tracking
func BenchmarkSourceOrderTracking(b *testing.B) {
	cssText := `
		h1 { color: red; }
		h2 { color: blue; }
		h3 { color: green; }
		p { font-size: 16px; }
		div { margin: 10px; }
	`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := css.NewParser(cssText)
		_, err := p.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkM3.1_ParseWithConfig benchmarks parsing with config
func BenchmarkParseWithConfig(b *testing.B) {
	cssText := mediumCSS
	config := css.ParseConfig{
		MaxBytes:       0,
		MaxImportDepth: 0,
		Origin:         css.OriginAuthor,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := css.NewParserWithConfig(cssText, config)
		_, err := p.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkM3.1_PropertyTableIntern benchmarks the property table interning
func BenchmarkPropertyTableIntern(b *testing.B) {
	props := []string{
		"display", "color", "font-size", "margin", "padding",
		"width", "height", "background", "border", "flex",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, prop := range props {
			css.InternPropertyName(prop)
		}
	}
}
