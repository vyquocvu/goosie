package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/raster"
)

func BenchmarkMutationRestyle(b *testing.B) {
	for _, tc := range []struct {
		name string
		html string
	}{
		{name: "gate_scroll", html: loadFixture(b)},
		{name: "plain_rows", html: strings.Repeat(`<div class="row"><span>Row</span></div>`, 1000)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			fonts, err := raster.NewFonts()
			if err != nil {
				b.Fatal(err)
			}
			authorCSS := `.row { height: 40px; line-height: 40px; } .hot { color: rgb(255, 0, 0); }`
			sess, err := engine.NewSession(tc.html, []string{authorCSS}, loadViewportW,
				engine.WithMetrics(fonts), engine.WithViewportH(loadViewportH))
			if err != nil {
				b.Fatal(err)
			}
			targets := sess.Doc.ElementsByClassName("row")
			if len(targets) == 0 {
				b.Fatal("mutation fixture has no row target")
			}
			target := targets[0]
			before := sess.WorkCounts()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				class := "row"
				if i%2 == 0 {
					class = "row hot"
				}
				target.SetAttribute("class", class)
				if err := sess.RestyleAndReflow(loadViewportW); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			after := sess.WorkCounts()
			b.ReportMetric(float64(after.LayoutBuilds-before.LayoutBuilds)/float64(b.N), "layouts/op")
			b.ReportMetric(float64(after.StyleNodeVisits-before.StyleNodeVisits)/float64(b.N), "style-nodes/op")
			target.SetAttribute("class", "row hot")
			if err := sess.RestyleAndReflow(loadViewportW); err != nil {
				b.Fatal(err)
			}
			if got, want := sess.Styles[target.ID].Color, (css.Color{R: 255, A: 255}); got != want {
				b.Fatalf("mutation did not reach computed style: got %v, want %v", got, want)
			}
		})
	}
}
