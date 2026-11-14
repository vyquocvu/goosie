package js

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func base64Runtime(t *testing.T) *Runtime {
	t.Helper()
	doc, err := dom.ParseBounded(`<html><body></body></html>`, dom.ParseLimits{Nodes: 16, Depth: 8, Attributes: 8, AttributeBytes: 128})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(Options{DOM: doc, URL: "https://example.test/base64.html"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

// qc:BR-290
func TestBtoaAtobRoundTripKnownVectors(t *testing.T) {
	runtime := base64Runtime(t)
	if err := runtime.Run(`
		window.encoded = btoa("Hello, world");
		window.decoded = atob("SGVsbG8sIHdvcmxk");
		window.emptyEnc = btoa("");
		window.emptyDec = atob("");
		window.loop = btoa(atob(btoa("Goosie engine")));
		window.orig = atob(window.loop);
	`, "base64-vectors"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, want string }{
		{"encoded", "SGVsbG8sIHdvcmxk"},
		{"decoded", "Hello, world"},
		{"emptyEnc", ""},
		{"emptyDec", ""},
		{"loop", atobEncode(t, "Goosie engine")},
		{"orig", "Goosie engine"},
	} {
		got, ok := runtime.Global(tc.name)
		if !ok || got.Str != tc.want {
			t.Errorf("%s = %q (present=%v), want %q", tc.name, got.Str, ok, tc.want)
		}
	}
}

// qc:BR-290
func TestAtobIgnoresAsciiWhitespaceAndOptionalPadding(t *testing.T) {
	runtime := base64Runtime(t)
	if err := runtime.Run(`
		window.spaced = atob("SGVs bG8s \tIHdv cmxk");
		window.unpadded = atob("SGVsbG8");
	`, "base64-forgiving"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, want string }{
		{"spaced", "Hello, world"},
		{"unpadded", "Hello"},
	} {
		got, ok := runtime.Global(tc.name)
		if !ok || got.Str != tc.want {
			t.Errorf("%s = %q (present=%v), want %q", tc.name, got.Str, ok, tc.want)
		}
	}
}

// qc:BR-290
func TestBtoaRejectsAboveLatin1AndAtobRejectsBadLength(t *testing.T) {
	runtime := base64Runtime(t)
	if err := runtime.Run(`
		window.encErr = "";
		try { btoa("snow\u2603"); } catch (e) { window.encErr = e.name; }
		window.latin1Ok = btoa("caf\u00e9");
		window.lenErr = "";
		try { atob("ABCDE"); } catch (e) { window.lenErr = e.name; }
		window.charsetErr = "";
		try { atob("!!!!"); } catch (e) { window.charsetErr = e.name; }
	`, "base64-rejections"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, want string }{
		{"encErr", "InvalidCharacterError"},
		{"latin1Ok", "Y2Fm6Q=="},
		{"lenErr", "InvalidCharacterError"},
		{"charsetErr", "InvalidCharacterError"},
	} {
		got, ok := runtime.Global(tc.name)
		if !ok || got.Str != tc.want {
			t.Errorf("%s = %q (present=%v), want %q", tc.name, got.Str, ok, tc.want)
		}
	}
}

func atobEncode(t *testing.T, s string) string {
	t.Helper()
	runtime := base64Runtime(t)
	if err := runtime.Run("window.out = btoa("+gojaString(s)+");", "helper"); err != nil {
		t.Fatal(err)
	}
	got, _ := runtime.Global("out")
	return got.Str
}

func gojaString(s string) string {
	out := "\""
	for _, r := range s {
		switch r {
		case '"':
			out += "\\\""
		case '\\':
			out += "\\\\"
		default:
			out += string(r)
		}
	}
	return out + "\""
}
