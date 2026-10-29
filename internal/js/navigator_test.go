package js

import "testing"

func TestNavigatorPlatformMatchesUserAgentPlatform(t *testing.T) {
	runtime, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(`window.navigatorPlatform = navigator.platform;`, "navigator-platform"); err != nil {
		t.Fatal(err)
	}
	if got, _ := runtime.Global("navigatorPlatform"); got.Str != "MacIntel" {
		t.Fatalf("navigator.platform = %q, want MacIntel", got.Str)
	}
}
