package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// TestBackgroundDataURLParenAttaches is the §2.3 row-1 end-to-end oracle: the
// payload that reaches Style.BackgroundImage must survive the rest of the
// pipeline (decodeDataImageURL → SVG decode → arena attach). Before the fix the
// two rgb() forms were cut at the first ')' by extractBackgroundURL, so the
// truncated payload failed to decode and the box attached no background at all.
// The parenthesis-free forms are in the same table because a regression there is
// a blocker, not a footnote.
// qc:BR-212
func TestBackgroundDataURLParenAttaches(t *testing.T) {
	const svgRGB = `data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='20' height='20'%3E%3Crect width='20' height='20' fill='rgb(0,0,255)'/%3E%3C/svg%3E`
	const svgHash = `data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='20' height='20'%3E%3Crect width='20' height='20' fill='%230000ff'/%3E%3C/svg%3E`
	cases := []struct {
		name string
		decl string
		want bool
	}{
		{"quoted, payload has ')'", `url("` + svgRGB + `")`, true},
		{"single-quoted, payload has ')'", `url('` + svgRGB + `')`, true},
		{"unquoted, payload has ')'", `url(` + svgRGB + `)`, true},
		{"quoted, parenthesis-free", `url("` + svgHash + `")`, true},
		{"unquoted, parenthesis-free", `url(` + svgHash + `)`, true},
		// A malformed token must attach nothing rather than half a URI.
		{"unterminated quote attaches nothing", `url("` + svgRGB, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := `<html><head><style>div{background-image: ` + tc.decl + `;width:100px;height:20px;}</style></head>` +
				`<body style="margin:0;"><div id="b">x</div></body></html>`
			s, err := engine.NewSession(html, nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7}))
			if err != nil {
				t.Fatal(err)
			}
			n := s.Doc.ElementByID("b")
			if n == nil {
				t.Fatal("div not parsed")
			}
			oid, ok := s.Arena.ForNode(n.ID)
			if !ok {
				t.Fatal("div not laid out")
			}
			obj := s.Arena.Get(oid)
			if tc.want && obj.BgImage == nil {
				t.Errorf("background not attached for %q", first80(tc.decl))
			}
			if !tc.want && obj.BgImage != nil {
				t.Errorf("a malformed url() attached a background: %q", first80(tc.decl))
			}
		})
	}
}

// first80 shortens a declaration for error text without hiding its shape.
func first80(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

// TestStyleSheetImportDataURLParenAttaches drives §2.3 row 3 through a real
// <link> stylesheet body: the fetched sheet's quoted data URI keeps its payload
// through absolutizeCSSURLs and the background still attaches. The sheet is fed
// as an author stylesheet so the test needs no network fetcher.
// qc:BR-212
func TestStyleSheetImportDataURLParenAttaches(t *testing.T) {
	const svgRGB = `data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='20' height='20'%3E%3Crect width='20' height='20' fill='rgb(0,0,255)'/%3E%3C/svg%3E`
	sheet := `.box{background-image:url("` + svgRGB + `");width:100px;height:20px;}`
	if !strings.Contains(sheet, "rgb(0,0,255)") {
		t.Fatal("fixture lost its payload")
	}
	html := `<html><body style="margin:0;"><div class="box" id="b">x</div></body></html>`
	s, err := engine.NewSession(html, []string{sheet}, 800, engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatal(err)
	}
	n := s.Doc.ElementByID("b")
	if n == nil {
		t.Fatal("div not parsed")
	}
	oid, ok := s.Arena.ForNode(n.ID)
	if !ok {
		t.Fatal("div not laid out")
	}
	if s.Arena.Get(oid).BgImage == nil {
		t.Error("author-sheet background with a ')' payload did not attach")
	}
}
