package js_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

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

func TestGlobalReturnsFalseForUndefined(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	_, ok := r.Global("neverDefined")
	if ok {
		t.Error("Global returned ok for an undefined name, want false")
	}
}

func TestRunEmptySource(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if err := r.Run("", "inline"); err != nil {
		t.Errorf("Run empty source: %v, want nil", err)
	}
}

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

func TestTitleEmptyByDefault(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})
	if got := r.Title(); got != "" {
		t.Errorf("Title() = %q with no seed, want empty", got)
	}
}

func TestMultipleScriptsWithDifferentSourceURLs(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`var a = 1;`, "https://example.test/first.js"); err != nil {
		t.Fatalf("first script: %v", err)
	}
	if err := r.Run(`var b = 2;`, "https://example.test/second.js"); err != nil {
		t.Fatalf("second script: %v", err)
	}

	if v, ok := r.Global("a"); !ok || v.Num != 1 {
		t.Errorf("a = %+v, want 1", v)
	}
	if v, ok := r.Global("b"); !ok || v.Num != 2 {
		t.Errorf("b = %+v, want 2", v)
	}

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
