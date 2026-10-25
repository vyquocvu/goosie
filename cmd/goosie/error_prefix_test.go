package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestProgramErrorLineCarriesOneLabel is the oracle for the doubled program
// label on stderr. TASK-040 measured a refused document printing
//
//	goosie: goosie: build session: CSS numeric value outside safe geometry range
//
// because the wrappers in this command already begin with "goosie: " and main
// added it again. Each row is the error string a call chain can hand back; the
// wanted line is what one reader should see.
// qc:BR-190
func TestProgramErrorLineCarriesOneLabel(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "unlabelled error gains the label once",
			in:   "CSS numeric value outside safe geometry range",
			want: "goosie: CSS numeric value outside safe geometry range",
		},
		{
			name: "single label is not duplicated",
			in:   "goosie: build session: viewport: width must be positive",
			want: "goosie: build session: viewport: width must be positive",
		},
		{
			name: "doubled label collapses to one",
			in:   "goosie: goosie: build session: CSS numeric value outside safe geometry range",
			want: "goosie: build session: CSS numeric value outside safe geometry range",
		},
		{
			name: "thrice-wrapped label collapses to one",
			in:   "goosie: goosie: goosie: paint document: tile cache: out of memory",
			want: "goosie: paint document: tile cache: out of memory",
		},
		{
			name: "label after the first is another stage's context and stays",
			in:   "goosie: reflow: goosie: build session: fetch failed",
			want: "goosie: reflow: goosie: build session: fetch failed",
		},
		{
			name: "the measured geometry refusal keeps every field it names",
			in: `goosie: goosie: build session: CSS numeric value outside safe geometry range: ` +
				`property "width", value "8e8", unit "e8px", parsed 8e+08, numval 8e+08, ` +
				`limit 2097152 CSS pixels, branch magnitude`,
			want: `goosie: build session: CSS numeric value outside safe geometry range: ` +
				`property "width", value "8e8", unit "e8px", parsed 8e+08, numval 8e+08, ` +
				`limit 2097152 CSS pixels, branch magnitude`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := programErrorLine(errors.New(tc.in))
			if got != tc.want {
				t.Errorf("programErrorLine() = %q, want %q", got, tc.want)
			}
			if strings.HasPrefix(got, "goosie: goosie:") {
				t.Errorf("line still carries the doubled label: %q", got)
			}
		})
	}
}

// TestRunFailureLineFromTheRealChain runs the code path that produced the
// doubling: a loader-stage wrapper around a guard error, printed the way main
// prints it. The chain is built here rather than through a live render because
// what is being proved is the shape of the line, not the document.
// qc:BR-190
func TestRunFailureLineFromTheRealChain(t *testing.T) {
	guard := fmt.Errorf("%w", errors.New("CSS numeric value outside safe geometry range: "+
		`property "width", value "8e8", unit "e8px", parsed 8e+08, numval 8e+08, `+
		"limit 2097152 CSS pixels, branch magnitude"))
	loader := fmt.Errorf("goosie: build session: %w", guard)
	line := programErrorLine(loader)
	if strings.Contains(line, "goosie: goosie:") {
		t.Errorf("doubled label survived: %q", line)
	}
	for _, want := range []string{"goosie: build session:", `property "width"`, "limit 2097152", "branch magnitude"} {
		if !strings.Contains(line, want) {
			t.Errorf("line lost %q: %q", want, line)
		}
	}
	if n := strings.Count(line, "goosie: "); n != 1 {
		t.Errorf("goosie: label count = %d, want 1 in %q", n, line)
	}
}
