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
