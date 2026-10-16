package css

import (
	"strings"
	"testing"
)

// The cascade rewrites viewport units by scanning a declaration's text with
// viewportTerm, and it does so for every declaration of every element. That is why
// ResolveViewportUnits answers the no-viewport case from viewportTermHints instead:
// the substitution is skipped whenever the scan finds none of its four bigrams.
//
// Skipping is only the same answer as "the regexp found nothing" if the hint set is
// complete, so this derives the unit list from the compiled pattern itself rather
// than restating it. Add a unit to viewportTerm - `cqw`, or a new prefix - without
// extending viewportTermHints, and the assertions below fail instead of the rewrite
// quietly stopping applying to that unit.

// patternUnits pulls the unit alternation out of the pattern source: it is the last
// group, and it is the only text the pattern can match there. Every derivation below
// is checked against the regexp so that a pattern whose shape this reader no longer
// understands fails loudly rather than silently narrowing the corpus to nothing.
func patternUnits(t *testing.T) []string {
	t.Helper()
	src := viewportTerm.String()
	open := strings.LastIndex(src, "(")
	if open < 0 {
		t.Fatalf("viewportTerm has no capture group to read units from: %q", src)
	}
	list := src[open+1:]
	end := strings.Index(list, ")")
	if end < 0 {
		t.Fatalf("viewportTerm's last capture group is unterminated: %q", src)
	}
	units := strings.Split(list[:end], "|")
	for _, u := range units {
		if u == "" || !viewportTerm.MatchString("10"+u) {
			t.Fatalf("read unit %q out of %q but the pattern does not match it; this reader no longer understands the pattern", u, src)
		}
	}
	if len(units) < 10 {
		t.Fatalf("derived %d units from the pattern, want at least the 10 viewport units: %q", len(units), units)
	}
	return units
}

func hasHint(s string) bool {
	for _, h := range viewportTermHints {
		if strings.Contains(s, h) {
			return true
		}
	}
	return false
}

// The whole lemma: a match's text ends with one of the pattern's units, so if every
// unit carries a hint, any string the hint scan rejects could not have matched.
func TestViewportTermHintsCoverEveryPatternUnit(t *testing.T) {
	for _, u := range patternUnits(t) {
		if !hasHint(u) {
			t.Errorf("viewport unit %q contains none of the hints %v; ResolveViewportUnits would skip it and never rewrite it", u, viewportTermHints)
		}
	}
}

// The other half of the same argument, measured against the regexp rather than
// against its source text: for a corpus of values with no hint in them, the pattern
// must find nothing. A mismatch here would mean the hints and the pattern disagree
// somewhere the unit-list argument does not reach.
func TestNoHintValueIsNeverAPatternMatch(t *testing.T) {
	for _, v := range noViewportTermCorpus {
		if hasHint(v) {
			t.Fatalf("%q carries a hint, so it does not belong in the no-hint corpus; move it to hintedLookalikes", v)
		}
		if viewportTerm.MatchString(v) {
			t.Errorf("%q carries no viewport hint yet matches viewportTerm; the hint scan is not equivalent to running the pattern", v)
		}
	}
}

// hintedLookalikes are the values the hint set exists to let through: the "ma" of a
// colour name, the "mi" of a keyword. They reach the regexp rather than the skip, so
// they are the path where an over-eager rewrite would still be caught - and where the
// pattern's own answer, "nothing to rewrite", has to survive.
var hintedLookalikes = []string{
	"maroon", "max-content", "middle", "min-width", "Mama",
}

func TestHintedLookalikesStillGetThePatternsAnswer(t *testing.T) {
	for _, v := range hintedLookalikes {
		if !hasHint(v) {
			t.Fatalf("%q carries no hint, so it is testing the skip rather than the fall-through", v)
		}
		out, found := ResolveViewportUnits(v, 1440, 900)
		if found || out != v {
			t.Errorf("ResolveViewportUnits(%q) = (%q, %v), want it unchanged", v, out, found)
		}
	}
}

// End to end, in the direction that matters for correctness rather than speed: every
// unit the pattern knows about must still be rewritten when it appears as a length.
// A hint set that stopped covering a unit fails here as a wrong computation, which is
// what a page would see, rather than as a source-text mismatch.
func TestEveryPatternUnitIsStillRewritten(t *testing.T) {
	for _, u := range patternUnits(t) {
		in := "10" + u
		out, found := ResolveViewportUnits(in, 1440, 900)
		if !found {
			t.Errorf("ResolveViewportUnits(%q) found no viewport term; the rewrite stopped applying to unit %q", in, u)
			continue
		}
		if !strings.HasSuffix(out, "px") {
			t.Errorf("ResolveViewportUnits(%q) = %q, want a px length", in, out)
		}
	}
}

// noViewportTermCorpus is declaration text of the kind a real page is mostly made of,
// none of which holds one of the four bigrams.
var noViewportTermCorpus = []string{
	"none", "auto", "bold", "center", "hidden", "uppercase", "1.5", "0 auto",
	"#f0f0f2", "2px solid #ccc", "rgba(0, 0, 0, 0.5)", "600 12px/1.4 Helvetica",
	"translate(3px, 4px)", "-1px", "100%", "solid", "repeat-x", "pointer",
	"canvas", "10VH", "5 VW", "", " ",
}
