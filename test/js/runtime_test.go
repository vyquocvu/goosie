package js_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

// Additional JS runtime tests covering the contract edges the core
// tests do not reach: Close idempotency, Global's type mapping for
// every Kind, empty-script handling, and console.warn/error routing.

// TestCloseIsIdempotent verifies that calling Close twice does not
// return an error. The document teardown path must be safe to call
// more than once, because a tab close and a process shutdown can
// both try to tear down the same runtime.
func TestCloseIsIdempotent(t *testing.T) {
	r, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second Close: %v, want nil (idempotent)", err)
	}
}

// TestGlobalReturnsCorrectKinds verifies that Global maps every JS
// value type to the right Kind constant. The host uses Kind to decide
// how to interpret Value, so a wrong mapping silently corrupts every
// consumer.
func TestGlobalReturnsCorrectKinds(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	cases := []struct {
		src      string
		name     string
		wantKind js.Kind
		check    func(js.Value) bool
		desc     string
	}{
		{
			src:      `var n = 42;`,
			name:     "n",
			wantKind: js.KindNumber,
			check:    func(v js.Value) bool { return v.Num == 42 },
			desc:     "number",
		},
		{
			src:      `var s = "hello";`,
			name:     "s",
			wantKind: js.KindString,
			check:    func(v js.Value) bool { return v.Str == "hello" },
			desc:     "string",
		},
		{
			src:      `var b = true;`,
			name:     "b",
			wantKind: js.KindBool,
			check:    func(v js.Value) bool { return v.Bool == true },
			desc:     "boolean true",
		},
		{
			src:      `var bf = false;`,
			name:     "bf",
			wantKind: js.KindBool,
			check:    func(v js.Value) bool { return v.Bool == false },
			desc:     "boolean false",
		},
		{
			src:      `var nl = null;`,
			name:     "nl",
			wantKind: js.KindNull,
			check:    func(v js.Value) bool { return true },
			desc:     "null",
		},
		{
			src:      `var obj = {a: 1};`,
			name:     "obj",
			wantKind: js.KindObject,
			check:    func(v js.Value) bool { return true },
			desc:     "object",
		},
		{
			src:      `var fn = function() {};`,
			name:     "fn",
			wantKind: js.KindFunction,
			check:    func(v js.Value) bool { return true },
			desc:     "function",
		},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			if err := r.Run(tc.src, "inline"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			v, ok := r.Global(tc.name)
			if !ok {
				t.Fatalf("Global(%q) not found", tc.name)
			}
			if v.Kind != tc.wantKind {
				t.Errorf("Kind = %v, want %v", v.Kind, tc.wantKind)
			}
			if !tc.check(v) {
				t.Errorf("value check failed: %+v", v)
			}
		})
	}
}

// TestGlobalReturnsFalseForUndefined verifies that reading a name
// that was never assigned returns false (not found) rather than a
// zero Value with a misleading Kind.
func TestGlobalReturnsFalseForUndefined(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	_, ok := r.Global("neverDefined")
	if ok {
		t.Error("Global returned ok for an undefined name, want false")
	}
}

// TestRunEmptySource verifies that an empty script succeeds without
// error. A page with an empty <script> tag is common and must not be
// treated as a failure.
func TestRunEmptySource(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if err := r.Run("", "inline"); err != nil {
		t.Errorf("Run empty source: %v, want nil", err)
	}
}

// TestConsoleWarnAndErrorRouteToWriter verifies that console.warn and
// console.error both write to the host writer, not just console.log.
// A developer tools integration that only captures log would miss
// warnings and errors.
func TestConsoleWarnAndErrorRouteToWriter(t *testing.T) {
	var buf bytes.Buffer
	r := newRuntime(t, js.Options{URL: "https://example.test/", Console: &buf})

	if err := r.Run(`console.warn("warning");`, "inline"); err != nil {
		t.Fatalf("console.warn: %v", err)
	}
	if err := r.Run(`console.error("error");`, "inline"); err != nil {
		t.Fatalf("console.error: %v", err)
	}

	output := buf.String()
	if !contains(output, "warning") {
		t.Errorf("console.warn output %q missing 'warning'", output)
	}
	if !contains(output, "error") {
		t.Errorf("console.error output %q missing 'error'", output)
	}
}

// TestTitleDefaultsToSeededValue verifies that Title returns the
// seeded value before any script runs, and returns it again after
// a script that does not change the title.
func TestTitleDefaultsToSeededValue(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/", Title: "My Page"})
	if got := r.Title(); got != "My Page" {
		t.Errorf("Title() = %q before any script, want %q", got, "My Page")
	}
	if err := r.Run(`var x = 1;`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := r.Title(); got != "My Page" {
		t.Errorf("Title() = %q after unrelated script, want %q", got, "My Page")
	}
}

// TestTitleEmptyByDefault verifies that a runtime created without a
// Title seeds document.title as empty, not as the literal string
// "undefined" or some other default.
func TestTitleEmptyByDefault(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if got := r.Title(); got != "" {
		t.Errorf("Title() = %q with no seed, want empty", got)
	}
}

// TestMultipleScriptsWithDifferentSourceURLs verifies that each Run
// can carry its own source URL for error attribution, and a later
// error correctly names its own source.
func TestMultipleScriptsWithDifferentSourceURLs(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`var a = 1;`, "https://example.test/first.js"); err != nil {
		t.Fatalf("first script: %v", err)
	}
	if err := r.Run(`var b = 2;`, "https://example.test/second.js"); err != nil {
		t.Fatalf("second script: %v", err)
	}

	// Both globals should be visible in the shared realm.
	if v, ok := r.Global("a"); !ok || v.Num != 1 {
		t.Errorf("a = %+v, want 1", v)
	}
	if v, ok := r.Global("b"); !ok || v.Num != 2 {
		t.Errorf("b = %+v, want 2", v)
	}

	// An error in the third script names the third source.
	err := r.Run(`throw new Error("oops");`, "https://example.test/third.js")
	if err == nil {
		t.Fatal("expected error from throw")
	}
	var jsErr *js.Error
	if !errors.As(err, &jsErr) {
		t.Fatalf("error is %T, want *js.Error", err)
	}
	if jsErr.SourceURL != "https://example.test/third.js" {
		t.Errorf("error source = %q, want third.js", jsErr.SourceURL)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
