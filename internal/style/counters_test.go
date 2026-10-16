package style

import (
	"reflect"
	"testing"
)

func TestParseCounterDecls(t *testing.T) {
	cases := []struct {
		in    string
		def   int
		want  map[string]int
		valid bool
	}{
		{"c", 0, map[string]int{"c": 0}, true},
		{"c", 1, map[string]int{"c": 1}, true},
		{"c 19", 0, map[string]int{"c": 19}, true},
		{"c -2", 1, map[string]int{"c": -2}, true},
		{"a 1 b 2", 0, map[string]int{"a": 1, "b": 2}, true},
		{"a b", 1, map[string]int{"a": 1, "b": 1}, true},
		{"none", 0, nil, true},
		{"NONE", 1, nil, true},
		{"", 0, nil, true},
		{"5", 0, nil, false},
		{"c 1 2", 0, nil, false},
	}
	for _, tc := range cases {
		got := ParseCounterDecls(tc.in, tc.def)
		if !tc.valid {
			if got != nil {
				t.Errorf("ParseCounterDecls(%q) = %v, want nil (malformed)", tc.in, got)
			}
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseCounterDecls(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestFormatCounter(t *testing.T) {
	cases := []struct {
		v     int
		style string
		want  string
	}{
		{5, "decimal", "5"},
		{0, "decimal", "0"},
		{-3, "decimal", "-3"},
		{5, "", "5"},
		{5, "bogus-style", "5"},
		{5, "decimal-leading-zero", "05"},
		{12, "decimal-leading-zero", "12"},
		{0, "decimal-leading-zero", "00"},
		{1, "upper-roman", "I"},
		{4, "upper-roman", "IV"},
		{9, "upper-roman", "IX"},
		{49, "upper-roman", "XLIX"},
		{389, "upper-roman", "CCCLXXXIX"},
		{3489, "upper-roman", "MMMCDLXXXIX"},
		{3999, "upper-roman", "MMMCMXCIX"},
		{4000, "upper-roman", "4000"},
		{0, "upper-roman", "0"},
		{14, "lower-roman", "xiv"},
		{1, "upper-alpha", "A"},
		{26, "upper-alpha", "Z"},
		{27, "upper-alpha", "AA"},
		{28, "lower-alpha", "ab"},
		{0, "upper-alpha", "0"},
		{3, "lower-latin", "c"},
		{10, "cjk-decimal", "一〇"},
		{0, "cjk-decimal", "〇"},
		{-5, "cjk-decimal", "-五"},
		{5, "thai", "๕"},
		{12, "arabic-indic", "١٢"},
		{7, "devanagari", "७"},
		// Georgian spot checks from the css-lists WPT references.
		{1, "georgian", "ა"},
		{8, "georgian", "ჱ"},
		{10, "georgian", "ი"},
		{11, "georgian", "ია"},
		{20, "georgian", "კ"},
		{60, "georgian", "ჲ"},
		{100, "georgian", "რ"},
		{400, "georgian", "ჳ"},
		{1000, "georgian", "ჩ"},
		{10000, "georgian", "ჵ"},
		{19999, "georgian", "ჵჰშჟთ"},
		{20000, "georgian", "20000"},
		{0, "georgian", "0"},
		{1, "armenian", "Ա"},
		{10, "armenian", "Ժ"},
		{49, "armenian", "ԽԹ"},
		{100, "armenian", "Ճ"},
		{3489, "armenian", "ՎՆՁԹ"},
		{10000, "armenian", "10000"},
	}
	for _, tc := range cases {
		if got := FormatCounter(tc.v, tc.style); got != tc.want {
			t.Errorf("FormatCounter(%d, %q) = %q, want %q", tc.v, tc.style, got, tc.want)
		}
	}
}

func TestParseContentParts(t *testing.T) {
	parts, ok := ParseContentParts(`"Section " counter(c, upper-roman) ":"`)
	if !ok || len(parts) != 3 {
		t.Fatalf("mixed content parsed ok=%v parts=%v, want 3 parts", ok, parts)
	}
	if parts[0].Kind != ContentText || parts[0].Text != "Section " {
		t.Errorf("part 0 = %+v, want text", parts[0])
	}
	if parts[1].Kind != ContentCounter || parts[1].Name != "c" || parts[1].Style != "upper-roman" {
		t.Errorf("part 1 = %+v, want counter c upper-roman", parts[1])
	}
	parts, ok = ParseContentParts(`counters(c, ".", georgian)`)
	if !ok || len(parts) != 1 || parts[0].Kind != ContentCounters {
		t.Fatalf("counters() parsed ok=%v parts=%v", ok, parts)
	}
	if parts[0].Sep != "." || parts[0].Style != "georgian" {
		t.Errorf("counters part = %+v, want sep . style georgian", parts[0])
	}
	parts, ok = ParseContentParts(`counter(c)`)
	if !ok || len(parts) != 1 || parts[0].Style != "decimal" {
		t.Errorf("bare counter() = %v %+v, want decimal default", ok, parts)
	}
	for _, bad := range []string{
		`"just a string"`,
		`attr(title)`,
		`counter(`,
		`counter(1)`,
		`counters(c)`,
		`counters(c, .)`,
		`counter(c, upper-roman, extra)`,
		`"unclosed`,
		`counter(c) garbage!`,
	} {
		if _, ok := ParseContentParts(bad); ok {
			t.Errorf("ParseContentParts(%q) succeeded, want raw fallback", bad)
		}
	}
}

func TestCounterScopeNesting(t *testing.T) {
	// Mirrors counters-003: body resets c, p increments it, first span nests.
	body := (&CounterScope{}).ApplyReset(map[string]int{"c": 0})
	body = body.ApplyIncrement(map[string]int{"c": 1}) // <p>
	if v, _ := body.Lookup("c"); v != 1 {
		t.Fatalf("body c = %d, want 1", v)
	}
	span1 := body.ApplyReset(map[string]int{"c": 0}).ApplyIncrement(map[string]int{"c": 1})
	if v, _ := span1.Lookup("c"); v != 1 {
		t.Fatalf("span1 c = %d, want own 1", v)
	}
	if levels := span1.Levels("c"); !reflect.DeepEqual(levels, []int{1, 1}) {
		t.Fatalf("span1 levels = %v, want [1 1] (body, own)", levels)
	}
	// A sibling without reset increments the body's counter.
	body2 := body.ApplyIncrement(map[string]int{"c": 1})
	if v, _ := body2.Lookup("c"); v != 2 {
		t.Fatalf("body c after sibling = %d, want 2", v)
	}
	// Set overwrites in place, creating nothing new.
	set := body2.ApplySet(map[string]int{"c": 48})
	if v, _ := set.Lookup("c"); v != 48 {
		t.Fatalf("set c = %d, want 48", v)
	}
	if levels := set.Levels("c"); !reflect.DeepEqual(levels, []int{48}) {
		t.Fatalf("set levels = %v, want single [48]", levels)
	}
	// Increment with no counter anywhere creates it locally, invisible to
	// the parent scope.
	fresh := (&CounterScope{}).ApplyIncrement(map[string]int{"c": 3})
	if v, _ := fresh.Lookup("c"); v != 3 {
		t.Fatalf("fresh c = %d, want 3", v)
	}
	if _, ok := (&CounterScope{}).Lookup("c"); ok {
		t.Fatal("empty scope reports a counter")
	}
}

func TestEvaluateContent(t *testing.T) {
	body := (&CounterScope{}).ApplyReset(map[string]int{"c": 0}).ApplyIncrement(map[string]int{"c": 12})
	span := body.ApplyReset(map[string]int{"c": 48}).ApplyIncrement(map[string]int{"c": 1})
	parts, ok := ParseContentParts(`counters(c, ".", upper-roman)`)
	if !ok {
		t.Fatal("parse failed")
	}
	if got := EvaluateContent(parts, span); got != "XII.XLIX" {
		t.Fatalf("EvaluateContent = %q, want XII.XLIX", got)
	}
	missing, _ := ParseContentParts(`counter(nope)`)
	if got := EvaluateContent(missing, span); got != "0" {
		t.Fatalf("missing counter = %q, want 0", got)
	}
}
