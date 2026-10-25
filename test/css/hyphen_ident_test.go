package css_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// tokSpec is one expected token. Num and NumVal are both checked: Value is what
// the admission guard re-parses with strconv.ParseFloat and NumVal is the
// tokenizer's own reading, and TASK-040 is exactly the defect where those two
// disagree about what a chunk of text means.
type tokSpec struct {
	Type   css.TokenType
	Value  string
	Unit   string
	NumVal float64
}

func (s tokSpec) name() string { return typeName(s.Type) }

func typeName(t css.TokenType) string {
	switch t {
	case css.TokenIdent:
		return "Ident"
	case css.TokenNumber:
		return "Number"
	case css.TokenPercentage:
		return "Percentage"
	case css.TokenDimension:
		return "Dimension"
	case css.TokenHash:
		return "Hash"
	case css.TokenDelim:
		return "Delim"
	case css.TokenCDC:
		return "CDC"
	case css.TokenFunction:
		return "Function"
	case css.TokenLBrace:
		return "LBrace"
	}
	return "Other"
}

// TestHyphenTokenKind is the tokenizer oracle for TASK-040 (defect
// GOOSIE-LIVE-002). It pins CSS Syntax Level 3 4.3.7, branch U+002D
// HYPHEN-MINUS, which orders the three readings of a hyphen: a number only when
// the hyphen is followed by a digit or by "." and a digit (4.3.11), otherwise an
// ident when it starts an ident sequence - and 4.3.10 counts a second hyphen as
// starting one, which is what makes every "--" custom property name an ident -
// otherwise a delimiter.
//
// Before the fix the hyphen went straight to the numeric scanner, so a custom
// property named after a colour, "--8e8e8e", arrived as Dimension("-8e8", unit
// "e8e", numval -8e8): eight hundred million CSS pixels, which the engine
// refuses as geometry and which blanked an entire news site.
// qc:BR-192
func TestHyphenTokenKind(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []tokSpec
	}{
		// The defect family: "--" starts an ident sequence, digits and all.
		{"digit-leading custom property", "--8e8e8e", []tokSpec{{css.TokenIdent, "--8e8e8e", "", 0}}},
		{"digit-leading short name", "--8e8", []tokSpec{{css.TokenIdent, "--8e8", "", 0}}},
		{"digit-leading colour name", "--666666", []tokSpec{{css.TokenIdent, "--666666", "", 0}}},
		{"hyphen inside the name", "--a-8e8e8", []tokSpec{{css.TokenIdent, "--a-8e8e8", "", 0}}},
		{"non-numeric tail", "--1e9e9", []tokSpec{{css.TokenIdent, "--1e9e9", "", 0}}},
		{"bare double hyphen", "--", []tokSpec{{css.TokenIdent, "--", "", 0}}},
		// Vendor-prefixed idents read the same way: one ident, not a zero
		// Dimension wearing the property name as its unit.
		{"vendor prefix", "-webkit-mask", []tokSpec{{css.TokenIdent, "-webkit-mask", "", 0}}},
		{"vendor prefix with digits", "-moz-opacity2", []tokSpec{{css.TokenIdent, "-moz-opacity2", "", 0}}},
		// A hyphen before a digit really does start a number (4.3.11), so these
		// keep their numeric token: the geometry cap must still see them.
		{"negative length", "-5px", []tokSpec{{css.TokenDimension, "-5", "px", -5}}},
		{"negative fraction", "-.5em", []tokSpec{{css.TokenDimension, "-.5", "em", -0.5}}},
		{"negative scientific", "-8e8e8", []tokSpec{{css.TokenDimension, "-8e8", "e8", -8e8}}},
		{"negative percentage", "-25%", []tokSpec{{css.TokenPercentage, "-25", "", -25}}},
		// A lone hyphen is a delimiter, not the number zero.
		{"lone hyphen", "-", []tokSpec{{css.TokenDelim, "-", "", 0}}},
		{"hyphen before brace", "-{", []tokSpec{{css.TokenDelim, "-", "", 0}, {css.TokenLBrace, "{", "", 0}}},
		// Untouched neighbours, recorded so the fix cannot move them.
		{"hash colour", "#8e8e8e", []tokSpec{{css.TokenHash, "8e8e8e", "", 0}}},
		{"digit-led dimension", "8e8e8px", []tokSpec{{css.TokenDimension, "8e8", "e8px", 8e8}}},
		{"CDC", "-->", []tokSpec{{css.TokenCDC, "-->", "", 0}}},
		{"custom property function", "--a(", []tokSpec{{css.TokenFunction, "--a", "", 0}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := css.NewTokenizer(tc.input)
			var got []tokSpec
			for {
				tk := tok.Next()
				if tk.Type == css.TokenEOF {
					break
				}
				got = append(got, tokSpec{tk.Type, tk.Value, tk.Unit, tk.NumVal})
			}
			if len(got) != len(tc.want) {
				t.Fatalf("token stream for %q = %v (%d tokens), want %v (%d tokens)",
					tc.input, describe(got), len(got), describe(tc.want), len(tc.want))
			}
			for i, w := range tc.want {
				g := got[i]
				if g.Type != w.Type || g.Value != w.Value || g.Unit != w.Unit {
					t.Errorf("token %d for %q = %s value %q unit %q, want %s value %q unit %q",
						i, tc.input, g.name(), g.Value, g.Unit, w.name(), w.Value, w.Unit)
					continue
				}
				if math.Abs(g.NumVal-w.NumVal) > 1e-9 {
					t.Errorf("token %d for %q numval %v, want %v", i, tc.input, g.NumVal, w.NumVal)
				}
			}
		})
	}
}

// TestTokenSpansCoverInputExactly records the answer to the second anomaly the
// task doc flagged: the guard was suspected of being fed *overlapping* token
// spans, a second Dimension re-reading the hyphen the first Number already
// consumed. It is not. Every token's span starts exactly where the previous one
// ended and the last ends at the input length, so the defect is one misdispatched
// hyphen, not a tokenizer that reads the same bytes twice.
// qc:BR-192
func TestTokenSpansCoverInputExactly(t *testing.T) {
	for _, in := range []string{"--8e8e8e", "--1e5x", "--5px", "-8e8e8", "1e1e1", "--a-8e8e8", "-webkit-mask"} {
		tok := css.NewTokenizer(in)
		type span struct {
			kind css.TokenType
			a, b int
		}
		var spans []span
		for {
			tk := tok.Next()
			if tk.Type == css.TokenEOF {
				break
			}
			spans = append(spans, span{tk.Type, tok.Offset(), 0})
			if len(spans) > 1 {
				spans[len(spans)-2].b = tok.Offset()
			}
		}
		if len(spans) == 0 {
			t.Fatalf("tokenizer produced no tokens for %q", in)
		}
		spans[len(spans)-1].b = len(in)
		for i, s := range spans {
			if s.b <= s.a {
				t.Fatalf("token %d of %q has empty or reversed span [%d,%d)", i, in, s.a, s.b)
			}
			if i > 0 && spans[i-1].b != s.a {
				t.Errorf("token %d of %q starts at %d but the previous token ended at %d: spans overlap or leave bytes uncovered",
					i, in, s.a, spans[i-1].b)
			}
		}
	}
}

func describe(ts []tokSpec) string {
	out := "["
	for i, t := range ts {
		if i > 0 {
			out += " "
		}
		out += t.name() + "(value=" + t.Value + " unit=" + t.Unit + ")"
	}
	return out + "]"
}
