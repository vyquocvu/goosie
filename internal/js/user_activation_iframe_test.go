package js

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

type activationClipboard struct{ text string }

func (c *activationClipboard) Read() string      { return c.text }
func (c *activationClipboard) Write(text string) { c.text = text }

func TestTransientActivationPropagatesAcrossIframeOrigins(t *testing.T) {
	newRuntime := func(rawURL string, clipboard ClipboardProvider) *Runtime {
		r, err := New(Options{URL: rawURL, ClipboardProvider: clipboard})
		if err != nil {
			t.Fatalf("New(%q): %v", rawURL, err)
		}
		t.Cleanup(func() { _ = r.Close() })
		return r
	}
	parent := newRuntime("https://top.example/page", &activationClipboard{})
	sameOriginClipboard := &activationClipboard{}
	crossOriginClipboard := &activationClipboard{}
	sameOrigin := newRuntime("https://top.example/frame", sameOriginClipboard)
	crossOrigin := newRuntime("https://other.example/frame", crossOriginClipboard)
	inheritedOrigin := newRuntime("about:srcdoc", &activationClipboard{})
	for _, child := range []*Runtime{sameOrigin, crossOrigin, inheritedOrigin} {
		child.iframeParent = parent
	}
	parent.iframeChildRuntimes = map[*dom.Node]*Runtime{
		{Data: "same"}:   sameOrigin,
		{Data: "cross"}:  crossOrigin,
		{Data: "srcdoc"}: inheritedOrigin,
	}

	parent.SetTransientUserActivation(true)
	for name, child := range map[string]*Runtime{"same-origin": sameOrigin, "inherited-origin": inheritedOrigin} {
		if !child.HasTransientUserActivation() {
			t.Errorf("%s child did not receive parent activation", name)
		}
	}
	if crossOrigin.HasTransientUserActivation() {
		t.Fatal("cross-origin child received activation from its parent")
	}
	if err := sameOrigin.Run(`navigator.clipboard.writeText('allowed').then(function(){clipboardWrite='resolved'}, function(e){clipboardWrite=e.name})`, "same-origin-clipboard.js"); err != nil {
		t.Fatalf("run same-origin clipboard write: %v", err)
	}
	if sameOriginClipboard.text != "allowed" {
		t.Fatalf("same-origin child clipboard write = %q, want allowed", sameOriginClipboard.text)
	}
	if err := crossOrigin.Run(`navigator.clipboard.writeText('blocked').then(function(){clipboardWrite='resolved'}, function(e){clipboardWrite=e.name})`, "cross-origin-clipboard.js"); err != nil {
		t.Fatalf("run cross-origin clipboard write: %v", err)
	}
	if crossOriginClipboard.text != "" {
		t.Fatalf("cross-origin child clipboard write changed host clipboard to %q", crossOriginClipboard.text)
	}

	parent.SetTransientUserActivation(false)
	crossOrigin.SetTransientUserActivation(true)
	if !parent.HasTransientUserActivation() {
		t.Fatal("child activation did not propagate to its ancestor")
	}
	if sameOrigin.HasTransientUserActivation() || inheritedOrigin.HasTransientUserActivation() {
		t.Fatal("cross-origin child activation propagated to unrelated same-origin siblings")
	}
	if !crossOrigin.HasTransientUserActivation() {
		t.Fatal("cross-origin child lost its own activation")
	}

	// Activation-consuming APIs clear transient state across the whole page.
	crossOrigin.SetTransientUserActivation(false)
	for name, runtime := range map[string]*Runtime{
		"parent": parent, "same-origin child": sameOrigin,
		"cross-origin child": crossOrigin, "srcdoc child": inheritedOrigin,
	} {
		if runtime.HasTransientUserActivation() {
			t.Errorf("%s retained activation after page-wide consumption", name)
		}
	}
}
