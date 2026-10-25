package engine_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine"
)

// refusalMatrix is the behaviour table TASK-040 measured on this host, one row
// per line of its §1.1 matrix plus the four §1 repros and the §2.1 neighbouring
// family. wantRefused is what the admission guard in internal/engine/limits.go
// must do with the document, and it is written from the CSS the author wrote -
// not from the token Goosie happened to produce - so that a guard which refuses
// a legal document cannot pass by agreeing with itself.
//
// The "limit" column is the reason, and it names the cap rather than the token:
// MaxGeometry exists to stop a length that would explode layout arithmetic, and
// every row that is not a length has to stay admitted however large its digits.
// qc:BR-164
func TestDigitLeadingCustomPropertyRefusalMatrix(t *testing.T) {
	cases := []struct {
		name        string
		cssText     string
		wantRefused bool
		why         string
	}{
		// §1 repros. These four are the defect: cafef.vn got no window at all.
		{"repro custom property declaration", `:root{--8e8e8e:#8e8e8e}`, false, "a custom property name is an ident"},
		{"repro var reference", `.a{color:var(--8e8e8e)}`, false, "the name is inside var(), still no length"},
		{"repro var with fallback", `.a{color:var(--8e8e8e, #8e8e8e)}`, false, "the fallback is a hash token"},
		{"repro hash colour", `.a{color:#8e8e8e}`, false, "never was a number"},

		// §1.1 matrix. The four rows the old table recorded as rc 1 because the
		// hyphen dispatched to the numeric scanner.
		{"§1.1 plain number name", `:root{--8e8:1}`, false, "ident, and a plain number is exempt anyway"},
		{"§1.1 digit name read as dimension", `:root{--8e8e8:1}`, false, "ident: -8e8 was a tokenizer artifact"},
		{"§1.1 nine-exponent name", `:root{--1e9e9:1}`, false, "ident: -1e9 was a tokenizer artifact"},
		{"§1.1 under-cap dimension name", `:root{--8e5e5:1}`, false, "ident, and 800000 was under the cap"},
		{"§1.1 small dimension name", `:root{--1e2e2:1}`, false, "ident, and 100 was under the cap"},
		{"§1.1 letter-led name", `:root{--abc8e8e8:1}`, false, "ident: magnitude 0"},
		{"§1.1 scientific width", `.a{width:8e8}`, false, "plain number, cap deliberately skips it"},
		{"§1.1 genuine oversized dimension", `.a{width:8e8e8px}`, true, "8e8 px is what the cap exists for"},

		// §2.1 neighbouring family, measured with the tokenizer before the fix.
		{"family unit after exponent", `:root{--1e5x:1}`, false, "ident, was Dimension(-1e5, x)"},
		{"family pixel-suffix name", `:root{--5px:1}`, false, "ident, was Dimension(-5, px)"},
		{"family hyphen inside name", `:root{--a-8e8e8:1}`, false, "ident, was Dimension(-, a-8e8e8)"},
		{"family colour-named digits", `:root{--666666:#666}`, false, "ident, was Number(-666666)"},
		{"family var of colour name", `.a{color:var(--666666)}`, false, "ident inside var()"},
		{"family vendor prefixed ident", `.a{background:-webkit-linear-gradient(red,blue)}`, false, "ident, was Dimension(-, webkit-linear-gradient)"},
		// The geometry property fed by the same name is the row that rules out
		// exempting by property: the guard blames "width" for a token that lives
		// inside the custom property name, so no property-keyed skip reaches it.
		{"family length fed by var of digit name", `.a{width:var(--8e8e8e)}`, false, "no length is written here"},
		{"family cafef rule verbatim", `.box-ttdn .list .mdn .time{color:var(--8e8e8e, #8e8e8e);font-size:12px}`, false, "the exact rule from the live page"},

		// Spec-consistent hyphens: a hyphen before a digit does start a number
		// (CSS Syntax L3 4.3.11), so these keep being visible to the cap.
		{"negative oversized length", `.a{width:-8e8e8px}`, true, "a real negative dimension"},
		{"oversized scientific length", `.a{width:9999999px}`, true, "9,999,999 px is over the cap"},
		{"legal negative length", `.a{margin-left:-5px}`, false, "a normal negative value"},
		{"legal fractional negative length", `.a{text-indent:-.5em}`, false, "-0.5 em"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The body deliberately carries no element matching `.a`: this table
			// measures the admission guard on its own, the way §1.1 of TASK-040
			// measured it. Give the rule a matching element and a second gate -
			// computed-style validation - joins in, and `.a{width:8e8}` would be
			// refused for being a huge box rather than judged by the token it is.
			// TestOversizedAppliedLengthIsRefused covers that stage separately.
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

// TestDigitLeadingCustomPropertyPaints is the other half of acceptance
// criterion 1: the repros must not merely stop refusing, they must paint what
// the page asked for. Chromium, measured on the same minimal document, resolves
// var(--8e8e8e) to the colour it names, so the substituted colour is asserted
// here and the pixels are scored by the parity fixture.
// qc:BR-164
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

// TestOversizedAppliedLengthIsRefused is the other half of the same table with
// the rule wired to an element: a huge number that really becomes a box size is
// refused even though the admission guard lets a plain number through. The cap
// is a geometry protection, and this is the geometry it protects.
// qc:BR-164
func TestOversizedAppliedLengthIsRefused(t *testing.T) {
	for _, cssText := range []string{
		`.a{width:8e8}`,       // plain number: exempt at admission, huge as a box
		`.a{width:8e8e8px}`,   // dimension: refused at admission
		`.a{width:9999999px}`, // dimension over the cap
		`.a{width:-8e8e8px}`,  // negative dimension over the cap
	} {
		html := "<html><head><style>" + cssText + "</style></head><body><p class=\"a\">hi</p></body></html>"
		if _, err := engine.NewSession(html, nil, 800); err == nil {
			t.Errorf("%s applied to an element was admitted, want refused", cssText)
		}
	}
}

// TestGeometryRefusalNamesEverything is the oracle for the refusal message
// (TASK-040 §3 B1). Three branches of the guard used to emit one identical,
// content-free sentence, so a bug report from a live page could not say which
// declaration was blamed, what it held, or which check fired.
// qc:BR-164
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

// TestMaxGeometryIsUnchanged pins the threshold itself. TASK-040 §3 B3 warns
// that every past loosening of this cap added a new failure family, so the
// release has to come with the number, not with a wider tolerance.
// qc:BR-164
func TestMaxGeometryIsUnchanged(t *testing.T) {
	if engine.MaxGeometry != 2<<20 {
		t.Errorf("MaxGeometry = %d, want 2<<20 = %d: TASK-040 fixes a false positive, not the threshold",
			engine.MaxGeometry, 2<<20)
	}
}
