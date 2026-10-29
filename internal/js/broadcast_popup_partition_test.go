package js

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestBroadcastChannelPartitionsOpenedTopLevelContexts(t *testing.T) {
	hub := NewBroadcastHub()
	newDocumentRuntime := func(rawURL string) (*Runtime, *dom.Document) {
		t.Helper()
		doc := dom.NewDocument()
		html := doc.NewElement("html")
		head := doc.NewElement("head")
		body := doc.NewElement("body")
		doc.Node.AppendChild(html)
		doc.HTML = html
		doc.Head = head
		doc.Body = body
		html.AppendChild(head)
		html.AppendChild(body)
		runtime, err := New(Options{Timeout: 5 * time.Second, URL: rawURL, DOM: doc, BroadcastHub: hub})
		if err != nil {
			t.Fatalf("New(%q): %v", rawURL, err)
		}
		t.Cleanup(func() { _ = runtime.Close() })
		return runtime, doc
	}
	attachFrame := func(doc *dom.Document) *dom.Node {
		doc.Body.AppendChild(doc.NewElement("iframe"))
		return doc.Body.LastChild
	}
	attachOpenedContext := func(owner *Runtime, frame *dom.Node, child *Runtime) {
		if owner.iframeOpenedContexts == nil {
			owner.iframeOpenedContexts = make(map[*dom.Node]bool)
		}
		owner.iframeOpenedContexts[frame] = true
		child.iframeParent = owner
		child.iframeParentFrame = frame
	}

	top, topDoc := newDocumentRuntime("https://a.site-one.test/page")
	crossPopup, crossPopupDoc := newDocumentRuntime("https://b.site-two.test/popup")
	attachOpenedContext(top, attachFrame(topDoc), crossPopup)
	crossChild, _ := newDocumentRuntime("https://a.site-one.test/iframe")
	crossPopup.iframeChildRuntimes = map[*dom.Node]*Runtime{}
	crossFrame := attachFrame(crossPopupDoc)
	crossPopup.iframeChildRuntimes[crossFrame] = crossChild
	crossChild.iframeParent = crossPopup
	crossChild.iframeParentFrame = crossFrame

	samePopup, samePopupDoc := newDocumentRuntime("https://b.site-one.test/popup")
	attachOpenedContext(top, attachFrame(topDoc), samePopup)
	sameChild, _ := newDocumentRuntime("https://a.site-one.test/iframe")
	sameFrame := attachFrame(samePopupDoc)
	samePopup.iframeChildRuntimes = map[*dom.Node]*Runtime{sameFrame: sameChild}
	sameChild.iframeParent = samePopup
	sameChild.iframeParentFrame = sameFrame

	if got, want := runtimeBroadcastTopLevelSite(crossChild), "https://site-two.test"; got != want {
		t.Fatalf("opened cross-site context's top-level site = %q, want %q", got, want)
	}
	if got, want := broadcastContextRoot(crossChild), crossPopup; got != want {
		t.Fatalf("opened context's delivery root = %p, want popup runtime %p", got, want)
	}

	for _, runtime := range []*Runtime{top, crossChild, sameChild} {
		if err := runtime.Run(`
			var received = [];
			var channel = new BroadcastChannel('opened-context-partition');
			channel.onmessage = function(event) { received.push(event.data); };
		`, "opened-context-partition.js"); err != nil {
			t.Fatal(err)
		}
	}
	if err := top.Run(`channel.postMessage('same-site-only');`, "opened-context-send.js"); err != nil {
		t.Fatal(err)
	}
	crossPopup.Tick()
	samePopup.Tick()
	for _, tc := range []struct {
		runtime *Runtime
		want    string
	}{
		{crossChild, "0"},
		{sameChild, "same-site-only"},
	} {
		got, err := tc.runtime.Evaluate(`received.length === 0 ? '0' : received.join('|')`)
		if err != nil || got != tc.want {
			t.Errorf("popup child received = %q, err=%v, want %q", got, err, tc.want)
		}
	}
}
