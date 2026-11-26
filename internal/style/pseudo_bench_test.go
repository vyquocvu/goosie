package style

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/test/domtest"
)

func BenchmarkPseudoStylePass(b *testing.B) {
	doc := domtest.Parse(strings.Repeat(`<div class="row"><span>Row</span></div>`, 1000))
	for _, tc := range []struct {
		name string
		css  string
		want int
	}{
		{name: "no_pseudo_rules", css: `.row { color: black }`, want: 0},
		{name: "before_rules", css: `.row { color: black } .row::before { content: "x" }`, want: 1000},
	} {
		b.Run(tc.name, func(b *testing.B) {
			resolver := NewResolver([]*css.Stylesheet{css.Parse(tc.css)})
			styles := resolver.ResolveViewport(doc, Viewport{W: 1440, H: 900}, nil)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pseudos := resolver.ResolvePseudoElements(doc, styles, nil)
				if len(pseudos) != tc.want {
					b.Fatalf("pseudo styles = %d, want %d", len(pseudos), tc.want)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(tc.want), "pseudos/op")
		})
	}
}
