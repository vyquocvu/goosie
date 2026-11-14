package engine_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine"
)

// qc:BR-193
func TestDigitLeadingCustomPropertyRefusalMatrix(t *testing.T) {
	cases := []struct {
		name        string
		cssText     string
		wantRefused bool
		why         string
	}{
		{"repro custom property declaration", `:root{--8e8e8e:#8e8e8e}`, false, "a custom property name is an ident"},
		{"repro var reference", `.a{color:var(--8e8e8e)}`, false, "the name is inside var(), still no length"},
		{"repro var with fallback", `.a{color:var(--8e8e8e, #8e8e8e)}`, false, "the fallback is a hash token"},
		{"repro hash colour", `.a{color:#8e8e8e}`, false, "never was a number"},

		{"§1.1 plain number name", `:root{--8e8:1}`, false, "ident, and a plain number is exempt anyway"},
		{"§1.1 digit name read as dimension", `:root{--8e8e8:1}`, false, "ident: -8e8 was a tokenizer artifact"},
		{"§1.1 nine-exponent name", `:root{--1e9e9:1}`, false, "ident: -1e9 was a tokenizer artifact"},
		{"§1.1 under-cap dimension name", `:root{--8e5e5:1}`, false, "ident, and 800000 was under the cap"},
		{"§1.1 small dimension name", `:root{--1e2e2:1}`, false, "ident, and 100 was under the cap"},
		{"§1.1 letter-led name", `:root{--abc8e8e8:1}`, false, "ident: magnitude 0"},
		{"§1.1 scientific width", `.a{width:8e8}`, false, "plain number, cap deliberately skips it"},
		{"§1.1 genuine oversized dimension", `.a{width:8e8e8px}`, true, "8e8 px is what the cap exists for"},

		{"family unit after exponent", `:root{--1e5x:1}`, false, "ident, was Dimension(-1e5, x)"},
		{"family pixel-suffix name", `:root{--5px:1}`, false, "ident, was Dimension(-5, px)"},
		{"family hyphen inside name", `:root{--a-8e8e8:1}`, false, "ident, was Dimension(-, a-8e8e8)"},
		{"family colour-named digits", `:root{--666666:#666}`, false, "ident, was Number(-666666)"},
		{"family var of colour name", `.a{color:var(--666666)}`, false, "ident inside var()"},
		{"family vendor prefixed ident", `.a{background:-webkit-linear-gradient(red,blue)}`, false, "ident, was Dimension(-, webkit-linear-gradient)"},
		{"family length fed by var of digit name", `.a{width:var(--8e8e8e)}`, false, "no length is written here"},
		{"family cafef rule verbatim", `.box-ttdn .list .mdn .time{color:var(--8e8e8e, #8e8e8e);font-size:12px}`, false, "the exact rule from the live page"},

		{"negative oversized length", `.a{width:-8e8e8px}`, true, "a real negative dimension"},
		{"oversized scientific length", `.a{width:9999999px}`, true, "9,999,999 px is over the cap"},
		{"legal negative length", `.a{margin-left:-5px}`, false, "a normal negative value"},
		{"legal fractional negative length", `.a{text-indent:-.5em}`, false, "-0.5 em"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := "<html><head><style>" + tc.cssText + "</style></head><body><p>hi</p></body></html>"
			_, err := engine.NewSession(html, nil, 800)
			if tc.wantRefused && err == nil {
				t.Errorf("%s: document admitted, want refused (%s)", tc.cssText, tc.why)
			}
			if !tc.wantRefused && err != nil {
				t.Errorf("%s: document refused, want admitted (%s): %v", tc.cssText, tc.why, err)
			}
			if err != nil && !tc.wantRefused {
				return
			}
			if err != nil && !strings.Contains(err.Error(), "outside safe geometry range") {
				t.Errorf("%s: refused for the wrong reason: %v", tc.cssText, err)
			}
		})
	}
}

// qc:BR-193
func TestDigitLeadingCustomPropertyPaints(t *testing.T) {
	const wantColor = "#8e8e8e"
	html := `<html><head><style>:root{--8e8e8e:` + wantColor +
		`}.t{color:var(--8e8e8e, #000);font-size:16px}</style></head><body><p class="t">ink</p></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatalf("NewSession = %v, want the document admitted", err)
	}
	var got css.Color
	found := false
	for _, st := range s.Styles {
		if st == nil {
			continue
		}
		if st.Color == (css.Color{R: 142, G: 142, B: 142, A: 255}) {
			got, found = st.Color, true
		}
	}
	if !found {
		t.Fatalf("no computed style resolves to %s, the var() substitution lost the colour", wantColor)
	}
	list, err := s.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked = %v", err)
	}
	cmds := list.All()
	if len(cmds) == 0 {
		t.Fatal("display list is empty, the page painted nothing")
	}
	t.Logf("substituted colour %d,%d,%d,%d over %d display commands", got.R, got.G, got.B, got.A, len(cmds))
}

// qc:BR-193
func TestOversizedAppliedLengthIsRefused(t *testing.T) {
	for _, cssText := range []string{
		`.a{width:8e8}`,
		`.a{width:8e8e8px}`,
		`.a{width:9999999px}`,
		`.a{width:-8e8e8px}`,
	} {
		html := "<html><head><style>" + cssText + "</style></head><body><p class=\"a\">hi</p></body></html>"
		if _, err := engine.NewSession(html, nil, 800); err == nil {
			t.Errorf("%s applied to an element was admitted, want refused", cssText)
		}
	}
}

// qc:BR-193
func TestGeometryRefusalNamesEverything(t *testing.T) {
	cases := []struct {
		name    string
		cssText string
		want    []string
	}{
		{
			name:    "magnitude branch",
			cssText: `.a{width:8e8e8px}`,
			want: []string{
				`outside safe geometry range`,
				`property "width"`,
				`value "8e8"`,
				`unit "e8px"`,
				fmt.Sprintf("limit %d", engine.MaxGeometry),
				`branch magnitude`,
			},
		},
		{
			name:    "float64 overflow branch",
			cssText: `.a{width:` + strings.Repeat("9", 400) + `px}`,
			want: []string{
				`outside safe geometry range`,
				`property "width"`,
				fmt.Sprintf("limit %d", engine.MaxGeometry),
				`branch float64-overflow`,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := "<html><head><style>" + tc.cssText + "</style></head><body><p>hi</p></body></html>"
			_, err := engine.NewSession(html, nil, 800)
			if err == nil {
				t.Fatalf("%s: document admitted, want refused", tc.cssText)
			}
			msg := err.Error()
			for _, w := range tc.want {
				if !strings.Contains(msg, w) {
					t.Errorf("refusal message %q is missing %q", msg, w)
				}
			}
			if len(msg) > 600 {
				t.Errorf("refusal message is %d bytes, the value it quotes must be clipped: %q", len(msg), msg)
			}
		})
	}
}

// qc:BR-193
func TestMaxGeometryIsUnchanged(t *testing.T) {
	if engine.MaxGeometry != 2<<20 {
		t.Errorf("MaxGeometry = %d, want 2<<20 = %d: TASK-040 fixes a false positive, not the threshold",
			engine.MaxGeometry, 2<<20)
	}
}
