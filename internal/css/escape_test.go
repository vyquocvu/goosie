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
