package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// escSVG is the document every spelling below denotes: an SVG whose attribute
// quotes are plain, because that is what a data URI has to look like for the
// decoder to read it.
const escSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="rgb(0,0,255)"/></svg>`

// escDataURL writes an SVG as the non-base64 data URI form sites use, where the
// markup itself is the payload.
func escDataURL(svg string) string { return `data:image/svg+xml,` + svg }

// TestEscapedQuoteDataURLBackgroundAttachesToTheArena is the session-level
// oracle for GOOSIE-CSSURLESCAPE-001: the escaped-quote forms must reach the
// arena as a decoded image, not as a background that silently never attached.
// A payload that keeps its backslashes is not a valid data URI, so the failing
// shape is BgImage == nil, which is exactly what Chromium renders as nothing
// and Goosie rendered as nothing while the url() scanner failed the token
// closed on the backslash. The percent-encoded control proves the attachment
// path itself was never the problem.
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
