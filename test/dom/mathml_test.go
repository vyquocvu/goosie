package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestMathMLElements(t *testing.T) {
	tests := []struct {
		tag      string
		expected dom.Atom
	}{
		{"math", dom.AtomMath},
		{"mrow", dom.AtomMrow},
		{"mfrac", dom.AtomMfrac},
		{"msup", dom.AtomMsup},
		{"msub", dom.AtomMsub},
		{"msqrt", dom.AtomMsqrt},
		{"mo", dom.AtomMo},
		{"mi", dom.AtomMi},
		{"mn", dom.AtomMn},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			got := dom.Lookup(tt.tag)
			if got != tt.expected {
				t.Errorf("Lookup(%q) = %v, want %v", tt.tag, got, tt.expected)
			}
		})
	}
}

func TestMathMLElementString(t *testing.T) {
	tests := []struct {
		atom     dom.Atom
		expected string
	}{
		{dom.AtomMath, "math"},
		{dom.AtomMrow, "mrow"},
		{dom.AtomMfrac, "mfrac"},
		{dom.AtomMsup, "msup"},
		{dom.AtomMsub, "msub"},
		{dom.AtomMsqrt, "msqrt"},
		{dom.AtomMo, "mo"},
		{dom.AtomMi, "mi"},
		{dom.AtomMn, "mn"},
	}

	for _, tt := range tests {
		t.Run(tt.atom.String(), func(t *testing.T) {
			got := tt.atom.String()
			if got != tt.expected {
				t.Errorf("%v.String() = %q, want %q", tt.atom, got, tt.expected)
			}
		})
	}
}
