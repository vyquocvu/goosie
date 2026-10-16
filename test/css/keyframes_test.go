package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// TestParseKeyframesFromTo verifies that from/to selectors parse to 0.0 and 1.0.
func TestParseKeyframesFromTo(t *testing.T) {
	stops, err := css.ParseKeyframes("from { opacity: 0 } to { opacity: 1 }")
	if err != nil {
		t.Fatalf("ParseKeyframes: %v", err)
	}
	if len(stops) != 2 {
		t.Fatalf("got %d stops, want 2", len(stops))
	}
	if stops[0].Offset != 0.0 {
		t.Errorf("stops[0].Offset = %v, want 0.0", stops[0].Offset)
	}
	if stops[1].Offset != 1.0 {
		t.Errorf("stops[1].Offset = %v, want 1.0", stops[1].Offset)
	}
	if stops[0].Declarations["opacity"] != "0" {
		t.Errorf("stops[0].Declarations[opacity] = %q, want %q", stops[0].Declarations["opacity"], "0")
	}
	if stops[1].Declarations["opacity"] != "1" {
		t.Errorf("stops[1].Declarations[opacity] = %q, want %q", stops[1].Declarations["opacity"], "1")
	}
}

// TestParseKeyframesPercentages verifies that percentage selectors parse correctly.
func TestParseKeyframesPercentages(t *testing.T) {
	stops, err := css.ParseKeyframes("0% { opacity: 0 } 50% { opacity: 0.5 } 100% { opacity: 1 }")
	if err != nil {
		t.Fatalf("ParseKeyframes: %v", err)
	}
	if len(stops) != 3 {
		t.Fatalf("got %d stops, want 3", len(stops))
	}
	if stops[0].Offset != 0.0 {
		t.Errorf("stops[0].Offset = %v, want 0.0", stops[0].Offset)
	}
	if stops[1].Offset != 0.5 {
		t.Errorf("stops[1].Offset = %v, want 0.5", stops[1].Offset)
	}
	if stops[2].Offset != 1.0 {
		t.Errorf("stops[2].Offset = %v, want 1.0", stops[2].Offset)
	}
	if stops[1].Declarations["opacity"] != "0.5" {
		t.Errorf("stops[1].Declarations[opacity] = %q, want %q", stops[1].Declarations["opacity"], "0.5")
	}
}

// TestParseKeyframesMultipleSelectors verifies that multiple selectors sharing
// a block produce stops at each offset.
func TestParseKeyframesMultipleSelectors(t *testing.T) {
	stops, err := css.ParseKeyframes("0%, 100% { opacity: 0 } 50% { opacity: 1 }")
	if err != nil {
		t.Fatalf("ParseKeyframes: %v", err)
	}
	if len(stops) != 3 {
		t.Fatalf("got %d stops, want 3", len(stops))
	}
	// Should be sorted: 0%, 50%, 100%.
	if stops[0].Offset != 0.0 {
		t.Errorf("stops[0].Offset = %v, want 0.0", stops[0].Offset)
	}
	if stops[0].Declarations["opacity"] != "0" {
		t.Errorf("stops[0].Declarations[opacity] = %q, want %q", stops[0].Declarations["opacity"], "0")
	}
	if stops[1].Offset != 0.5 {
		t.Errorf("stops[1].Offset = %v, want 0.5", stops[1].Offset)
	}
	if stops[2].Offset != 1.0 {
		t.Errorf("stops[2].Offset = %v, want 1.0", stops[2].Offset)
	}
	if stops[2].Declarations["opacity"] != "0" {
		t.Errorf("stops[2].Declarations[opacity] = %q, want %q", stops[2].Declarations["opacity"], "0")
	}
}

// TestParseKeyframesMultipleProperties verifies that a keyframe block with
// multiple declarations captures all of them.
func TestParseKeyframesMultipleProperties(t *testing.T) {
	stops, err := css.ParseKeyframes("from { opacity: 0; background-color: red } to { opacity: 1; background-color: blue }")
	if err != nil {
		t.Fatalf("ParseKeyframes: %v", err)
	}
	if len(stops) != 2 {
		t.Fatalf("got %d stops, want 2", len(stops))
	}
	if stops[0].Declarations["opacity"] != "0" {
		t.Errorf("stops[0].Declarations[opacity] = %q, want %q", stops[0].Declarations["opacity"], "0")
	}
	if stops[0].Declarations["background-color"] != "red" {
		t.Errorf("stops[0].Declarations[background-color] = %q, want %q", stops[0].Declarations["background-color"], "red")
	}
	if stops[1].Declarations["opacity"] != "1" {
		t.Errorf("stops[1].Declarations[opacity] = %q, want %q", stops[1].Declarations["opacity"], "1")
	}
	if stops[1].Declarations["background-color"] != "blue" {
		t.Errorf("stops[1].Declarations[background-color] = %q, want %q", stops[1].Declarations["background-color"], "blue")
	}
}

// TestParseKeyframesUnsorted verifies that stops given out of order are sorted
// by offset.
func TestParseKeyframesUnsorted(t *testing.T) {
	stops, err := css.ParseKeyframes("100% { opacity: 1 } 0% { opacity: 0 } 50% { opacity: 0.5 }")
	if err != nil {
		t.Fatalf("ParseKeyframes: %v", err)
	}
	if len(stops) != 3 {
		t.Fatalf("got %d stops, want 3", len(stops))
	}
	if stops[0].Offset != 0.0 {
		t.Errorf("stops[0].Offset = %v, want 0.0", stops[0].Offset)
	}
	if stops[1].Offset != 0.5 {
		t.Errorf("stops[1].Offset = %v, want 0.5", stops[1].Offset)
	}
	if stops[2].Offset != 1.0 {
		t.Errorf("stops[2].Offset = %v, want 1.0", stops[2].Offset)
	}
}

// TestParseKeyframesInvalidOffset verifies that a bad percentage returns an error.
func TestParseKeyframesInvalidOffset(t *testing.T) {
	_, err := css.ParseKeyframes("abc { opacity: 0 }")
	if err == nil {
		t.Error("expected error for invalid selector, got nil")
	}
}

// TestParseKeyframesInStylesheet verifies that @keyframes rules parse correctly
// when embedded in a full stylesheet.
func TestParseKeyframesInStylesheet(t *testing.T) {
	sheet := css.Parse(`
		@keyframes fadeIn {
			from { opacity: 0 }
			to { opacity: 1 }
		}
		.box { color: red }
	`)
	if len(sheet.Keyframes) != 1 {
		t.Fatalf("got %d keyframes rules, want 1", len(sheet.Keyframes))
	}
	kf := sheet.Keyframes[0]
	if kf.Name != "fadeIn" {
		t.Errorf("Name = %q, want %q", kf.Name, "fadeIn")
	}
	if len(kf.Stops) != 2 {
		t.Fatalf("got %d stops, want 2", len(kf.Stops))
	}
	if kf.Stops[0].Offset != 0.0 {
		t.Errorf("Stops[0].Offset = %v, want 0.0", kf.Stops[0].Offset)
	}
	if kf.Stops[1].Offset != 1.0 {
		t.Errorf("Stops[1].Offset = %v, want 1.0", kf.Stops[1].Offset)
	}
	// Regular rules should still parse.
	if len(sheet.Rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(sheet.Rules))
	}
}

// TestParseKeyframesWebkitPrefix verifies that -webkit-keyframes is also parsed.
func TestParseKeyframesWebkitPrefix(t *testing.T) {
	sheet := css.Parse(`
		@-webkit-keyframes slide {
			0% { margin-left: 0 }
			100% { margin-left: 100px }
		}
	`)
	if len(sheet.Keyframes) != 1 {
		t.Fatalf("got %d keyframes rules, want 1", len(sheet.Keyframes))
	}
	if sheet.Keyframes[0].Name != "slide" {
		t.Errorf("Name = %q, want %q", sheet.Keyframes[0].Name, "slide")
	}
}

// TestParseKeyframesEmpty verifies that an empty body returns no stops.
func TestParseKeyframesEmpty(t *testing.T) {
	stops, err := css.ParseKeyframes("")
	if err != nil {
		t.Fatalf("ParseKeyframes: %v", err)
	}
	if len(stops) != 0 {
		t.Errorf("got %d stops, want 0", len(stops))
	}
}
