package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestMaximumUnicodeCodePointEscapeDoesNotPanicAndPreservesEscapedBackslash(t *testing.T) {
	r := newDOMRuntime(t, domtest.Parse(`<html><body></body></html>`))
	if err := r.Run(`var scalar = "a\u{10FFFF}"; var patternText = "a\\u{10FFFF}";`, "unicode-escape.js"); err != nil {
		t.Fatalf("run maximum code point escape: %v", err)
	}
	got, err := r.Evaluate(`scalar.length === 3 && scalar.codePointAt(1) === 0x10FFFF && patternText === "a\\u{10FFFF}"`)
	if err != nil || got != "true" {
		t.Fatalf("maximum code point and escaped backslash = %q, err %v", got, err)
	}
	got, err = r.Evaluate(`"\u{10FFFF}".codePointAt(0) === 0x10FFFF`)
	if err != nil || got != "true" {
		t.Fatalf("evaluate maximum code point escape = %q, err %v", got, err)
	}
}
