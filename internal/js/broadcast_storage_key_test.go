package js

import (
	"testing"
	"time"
)

func TestBroadcastChannelUsesTopLevelSchemefulSitePartition(t *testing.T) {
	hub := NewBroadcastHub()
	newRuntime := func(rawURL string) *Runtime {
		t.Helper()
		r, err := New(Options{Timeout: 5 * time.Second, URL: rawURL, BroadcastHub: hub})
		if err != nil {
			t.Fatalf("New(%q): %v", rawURL, err)
		}
		t.Cleanup(func() { _ = r.Close() })
		return r
	}
	parentA := newRuntime("https://a.site-one.test/")
	parentB := newRuntime("https://b.site-one.test/")
	parentC := newRuntime("https://a.site-two.test/")
	childA := newRuntime("https://widget.third-party.test/a")
	childB := newRuntime("https://widget.third-party.test/b")
	childC := newRuntime("https://widget.third-party.test/c")
	childA.iframeParent = parentA
	childB.iframeParent = parentB
	childC.iframeParent = parentC
	for _, child := range []*Runtime{childA, childB, childC} {
		if err := child.Run(`
			var received = [];
			var channel = new BroadcastChannel('partition');
			channel.onmessage = function(event) { received.push(event.data); };
		`, "broadcast-storage-key.js"); err != nil {
			t.Fatal(err)
		}
	}
	evaluate := func(runtime *Runtime, expression string) string {
		t.Helper()
		value, err := runtime.Evaluate(expression)
		if err != nil {
			t.Fatalf("Evaluate(%q): %v", expression, err)
		}
		return value
	}
	if gotA, gotB, gotC := runtimeBroadcastTopLevelSite(childA), runtimeBroadcastTopLevelSite(childB), runtimeBroadcastTopLevelSite(childC); gotA != gotB || gotA == gotC {
		t.Fatalf("schemeful top-level sites = %q, %q, %q", gotA, gotB, gotC)
	}
	if err := childA.Run(`channel.postMessage('site-one');`, "broadcast-send.js"); err != nil {
		t.Fatal(err)
	}
	childB.Tick()
	childC.Tick()
	if got := evaluate(childB, `received.join('|')`); got != "site-one" {
		t.Fatalf("same-site third-party recipient = %q, want site-one", got)
	}
	if got := evaluate(childC, `received.length`); got != "0" {
		t.Fatalf("cross-site third-party recipient count = %q, want 0", got)
	}
	if err := childC.Run(`channel.postMessage('site-two');`, "broadcast-send.js"); err != nil {
		t.Fatal(err)
	}
	childA.Tick()
	childB.Tick()
	if got := evaluate(childA, `received.length`); got != "0" {
		t.Fatalf("cross-site sender reached site-one recipient: %q", got)
	}
	if got := evaluate(childB, `received.length`); got != "1" {
		t.Fatalf("cross-site sender reached same-site recipient: %q", got)
	}
}

func TestBroadcastChannelIDNOriginUsesASCIIHostSerialization(t *testing.T) {
	hub := NewBroadcastHub()
	newRuntime := func(rawURL string) *Runtime {
		t.Helper()
		r, err := New(Options{Timeout: 5 * time.Second, URL: rawURL, BroadcastHub: hub})
		if err != nil {
			t.Fatalf("New(%q): %v", rawURL, err)
		}
		t.Cleanup(func() { _ = r.Close() })
		return r
	}
	unicodeHost := newRuntime("https://bücher.example/page")
	punycodeHost := newRuntime("https://xn--bcher-kva.example/other")
	if got := runtimeBroadcastOrigin(unicodeHost); got != "https://xn--bcher-kva.example" {
		t.Fatalf("IDN origin serialization = %q", got)
	}
	if got := runtimeBroadcastTopLevelSite(unicodeHost); got != runtimeBroadcastTopLevelSite(punycodeHost) {
		t.Fatalf("IDN top-level sites differ: %q vs %q", got, runtimeBroadcastTopLevelSite(punycodeHost))
	}
	for _, runtime := range []*Runtime{unicodeHost, punycodeHost} {
		if err := runtime.Run(`
			var received = '';
			var channel = new BroadcastChannel('idn-origin');
			channel.onmessage = function(event) { received = event.origin + '|' + event.data; };
		`, "broadcast-idn.js"); err != nil {
			t.Fatal(err)
		}
	}
	if err := unicodeHost.Run(`new BroadcastChannel('idn-origin').postMessage('hello');`, "broadcast-idn-send.js"); err != nil {
		t.Fatal(err)
	}
	punycodeHost.Tick()
	if got, err := punycodeHost.Evaluate(`received`); err != nil || got != "https://xn--bcher-kva.example|hello" {
		t.Fatalf("IDN cross-runtime message = %q, err=%v", got, err)
	}
}
