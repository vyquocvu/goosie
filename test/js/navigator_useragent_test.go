package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

// qc:BR-330
func TestNavigatorUserAgentMatchesNetworkCompatibilityUA(t *testing.T) {
	const want = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Goosie/2.0"
	r, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`navigatorUserAgent = navigator.userAgent;`, ""); err != nil {
		t.Fatal(err)
	}
	got, ok := r.Global("navigatorUserAgent")
	if !ok || got.Str != want {
		t.Fatalf("navigator.userAgent = %q, want %q", got.Str, want)
	}
}
