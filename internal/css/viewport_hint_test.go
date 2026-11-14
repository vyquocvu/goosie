package css

import (
	"strings"
	"testing"
)

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

func TestViewportTermHintsCoverEveryPatternUnit(t *testing.T) {
	for _, u := range patternUnits(t) {
		if !hasHint(u) {
			t.Errorf("viewport unit %q contains none of the hints %v; ResolveViewportUnits would skip it and never rewrite it", u, viewportTermHints)
		}
	}
}

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

var noViewportTermCorpus = []string{
	"none", "auto", "bold", "center", "hidden", "uppercase", "1.5", "0 auto",
	"#f0f0f2", "2px solid #ccc", "rgba(0, 0, 0, 0.5)", "600 12px/1.4 Helvetica",
	"translate(3px, 4px)", "-1px", "100%", "solid", "repeat-x", "pointer",
	"canvas", "10VH", "5 VW", "", " ",
}
