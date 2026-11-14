package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

func gmtStringRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	return r
}

// qc:BR-294
func TestDateToGMTStringIsAFunctionOnThePrototype(t *testing.T) {
	r := gmtStringRuntime(t)
	got, err := r.Evaluate(`typeof Date.prototype.toGMTString`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got != "function" {
		t.Fatalf("typeof Date.prototype.toGMTString = %q, want function", got)
	}
	held, err := r.Evaluate(`Object.getPrototypeOf(new Date()).hasOwnProperty('toGMTString')`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if held != "true" {
		t.Fatalf("toGMTString own on Date.prototype = %q, want true", held)
	}
}

// qc:BR-294
func TestDateToGMTStringMatchesToUTCString(t *testing.T) {
	r := gmtStringRuntime(t)
	cases := []struct {
		expr string
		want string
	}{
		{`(new Date(0)).toGMTString()`, "Thu, 01 Jan 1970 00:00:00 GMT"},
		{`(new Date(Date.UTC(2001, 8, 9, 10, 11, 12))).toGMTString()`, "Sun, 09 Sep 2001 10:11:12 GMT"},
	}
	for _, c := range cases {
		got, err := r.Evaluate(c.expr)
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if got != c.want {
			t.Fatalf("%s = %q, want %q", c.expr, got, c.want)
		}
	}
	identical, err := r.Evaluate(`(new Date(1234567890000)).toGMTString() === (new Date(1234567890000)).toUTCString()`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if identical != "true" {
		t.Fatalf("toGMTString is not byte-identical to toUTCString: %q", identical)
	}
}

// qc:BR-294
func TestDateToGMTStringRejectsNonDateReceiver(t *testing.T) {
	r := gmtStringRuntime(t)
	got, err := r.Evaluate(`(function () { try { Date.prototype.toGMTString.call(42); return 'no-throw'; } catch (e) { return e.name; } })()`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got != "TypeError" {
		t.Fatalf("toGMTString.call(42) = %q, want TypeError", got)
	}
}
