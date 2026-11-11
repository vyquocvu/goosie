package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

func TestFCRegistryCountsPerKind(t *testing.T) {
	cases := []struct {
		name string
		html string
		kind layout.FCKind
	}{
		{"flex-row", `<div style="display:flex"><i>a</i><i>b</i></div>`, layout.FCFlexRow},
		{"flex-column", `<div style="display:flex;flex-direction:column"><i>a</i><i>b</i></div>`, layout.FCFlexColumn},
		{"grid", `<div style="display:grid"><i>a</i></div>`, layout.FCGrid},
		{"table", `<table><tr><td>a</td></tr></table>`, layout.FCTable},
		{"block", `<div><p>a</p></div>`, layout.FCBlockFlow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := buildBijectionArena(t, c.html, ``)
			if got := a.FCCount(c.kind); got == 0 {
				t.Fatalf("%s: FCCount(%v)=0, want >0", c.name, c.kind)
			}
		})
	}
}
