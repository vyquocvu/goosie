package css

import "testing"

func TestDecodeEscapes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"\\25b3", "△"},
		{"\\25B3", "△"},
		{"\\25b3 ", "△"},
		{"hello\\25b3world", "hello△world"},
		{"\\000041", "A"},
		{"no escapes", "no escapes"},
		{"\\n", "n"},
	}
	for _, tt := range tests {
		got := DecodeEscapes(tt.input)
		if got != tt.want {
			t.Errorf("DecodeEscapes(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseValueStringWithEscape(t *testing.T) {
	v := ParseValue(`"\25b3"`)
	if v.Type != ValueString {
		t.Fatalf("expected ValueString, got %v", v.Type)
	}
	if v.Str != "△" {
		t.Errorf("expected △, got %q", v.Str)
	}
}

// TestStripCDATAMarkers pins XHTML style handling: reference pages wrap
// <style> in CDATA, and the markers must not fuse onto the first selector
// (which silently unmatched every rule in the sheet).
func TestStripCDATAMarkers(t *testing.T) {
	sh := Parse("<![CDATA[\n  img\n  {\n  height: 50px;\n  width: 50px;\n  }\n  ]]>")
	if len(sh.Rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(sh.Rules))
	}
	if len(sh.Rules[0].SelectorStrs) != 1 || sh.Rules[0].SelectorStrs[0] != "img" {
		t.Errorf("selector = %q, want [img]", sh.Rules[0].SelectorStrs)
	}
	plain := Parse("img { width: 50px; }")
	if len(plain.Rules) != 1 || plain.Rules[0].SelectorStrs[0] != "img" {
		t.Errorf("plain sheet broke: %+v", plain.Rules)
	}
}
