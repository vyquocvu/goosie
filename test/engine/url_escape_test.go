package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

const escSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="rgb(0,0,255)"/></svg>`

func escDataURL(svg string) string { return `data:image/svg+xml,` + svg }

// qc:BR-269
func TestEscapedQuoteDataURLBackgroundAttachesToTheArena(t *testing.T) {
	singleQuoted := strings.ReplaceAll(escSVG, `"`, `'`)
	cases := []struct{ name, decl string }{
		{"escaped double quotes", `url("` + escDataURL(strings.ReplaceAll(escSVG, `"`, `\"`)) + `")`},
		{"escaped single quotes", `url('` + escDataURL(strings.ReplaceAll(singleQuoted, `'`, `\'`)) + `')`},
		{"hex-escaped double quotes", `url("` + escDataURL(strings.ReplaceAll(escSVG, `"`, `\22 `)) + `")`},
		{"control, percent-encoded", `url("` + escDataURL(strings.ReplaceAll(escSVG, `"`, `%22`)) + `")`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := `<html><head><style>div{background-image: ` + tc.decl + `;background-repeat:no-repeat;width:20px;height:20px}</style></head><body style="margin:0;"><div>x</div></body></html>`
			s, err := engine.NewSession(html, nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7}))
			if err != nil {
				t.Fatal(err)
			}
			var attached bool
			for i := range s.Arena.Objects {
				o := &s.Arena.Objects[i]
				if o.Node != nil && o.Node.Data == "div" {
					attached = o.BgImage != nil
				}
			}
			if !attached {
				t.Errorf("no decoded background attached for %s; the data URI the escapes denote never reached the decoder", tc.decl)
			}
		})
	}
}
