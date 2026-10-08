package engine_test

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// TestCSSImportBreadthCapped pins the breadth cap on @import expansion: a
// sheet naming a hundred imports must not fetch them all. The document still
// builds, with the imports past the budget silently absent.
func TestCSSImportBreadthCapped(t *testing.T) {
	var fetched int64
	linker := func(base, href string) (string, error) {
		atomic.AddInt64(&fetched, 1)
		if strings.HasSuffix(href, "/root.css") {
			var b strings.Builder
			for i := 0; i < 100; i++ {
				fmt.Fprintf(&b, `@import url("i%d.css");`, i)
			}
			b.WriteString("p { color: green; }")
			return b.String(), nil
		}
		return "p { color: red; }", nil
	}
	html := `<html><head><link rel="stylesheet" href="/root.css"></head><body><p>x</p></body></html>`
	if _, err := engine.NewSession(html, nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithLinkedCSS("https://example.com/", linker)); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt64(&fetched); got > 64 {
		t.Errorf("fetched %d stylesheets for a 100-import sheet, want <= 64 (breadth cap)", got)
	}
	if got := atomic.LoadInt64(&fetched); got < 2 {
		t.Errorf("fetched %d stylesheets, want the root plus at least one import", got)
	}
}
