package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestDatalistAssociationAndOptions(t *testing.T) {
	doc := domtest.Parse(`<html><body>
<input id="browser-input" list="browsers" name="browser">
<datalist id="browsers">
  <option value="Edge">
  <option value="Firefox">
  <option value="Chrome">
  <option value="Goosie">
  <option>Safari</option>
</datalist>
</body></html>`)

	input := doc.ElementByID("browser-input")
	if input == nil {
		t.Fatal("browser-input not found")
	}

	opts := input.DatalistOptions()
	if len(opts) != 5 {
		t.Fatalf("expected 5 datalist options, got %d: %v", len(opts), opts)
	}

	expected := []string{"Edge", "Firefox", "Chrome", "Goosie", "Safari"}
	for i, want := range expected {
		if opts[i] != want {
			t.Errorf("opts[%d] = %q, want %q", i, opts[i], want)
		}
	}
}

func TestDatalistSuggestionsExcludeDisabledAndEmptyOptions(t *testing.T) {
	doc := domtest.Parse(`<input id="range" type="range" list="choices">
<datalist id="choices">
	<option value="10">
	<option disabled value="20">
	<option value="">
	<option> 30 </option>
	<optgroup disabled><option value="40"></option></optgroup>
</datalist>`)
	input := doc.ElementByID("range")
	if input == nil {
		t.Fatal("range input not found")
	}
	options := input.DatalistOptions()
	want := []string{"10", "30"}
	if len(options) != len(want) {
		t.Fatalf("datalist suggestions = %v, want %v", options, want)
	}
	for i, value := range want {
		if options[i] != value {
			t.Errorf("suggestion[%d] = %q, want %q", i, options[i], value)
		}
	}
}
