package main

import (
	"context"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/raster"
)

func TestFramePathBroadcastHubSeparatesPrivateBrowsing(t *testing.T) {
	normalClient := &recordingClient{}
	privateClient := &recordingClient{}
	normalHub := js.NewBroadcastHub()
	privateHub := js.NewBroadcastHub()
	f := &framePath{
		client:                normalClient,
		privateClient:         privateClient,
		broadcastHub:          normalHub,
		incognitoBroadcastHub: privateHub,
	}

	if got := f.broadcastHubFor(normalClient); got != normalHub {
		t.Fatal("normal browsing context did not use the normal BroadcastChannel hub")
	}
	if got := f.broadcastHubFor(privateClient); got != privateHub {
		t.Fatal("private browsing context did not use the isolated BroadcastChannel hub")
	}
}

func TestLoadedDocumentsKeepPrivateBroadcastMessagesSeparate(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	page := "<!doctype html><html><body>broadcast partition</body></html>"
	normalClient := &recordingClient{docs: map[string]string{
		"https://partition.example/normal-a": page,
		"https://partition.example/normal-b": page,
	}}
	privateClient := &recordingClient{docs: map[string]string{
		"https://partition.example/private": page,
	}}
	f := &framePath{
		client:                normalClient,
		privateClient:         privateClient,
		fonts:                 fonts,
		storage:               js.NewOriginStorageRegistry("", false),
		incognitoStorage:      js.NewOriginStorageRegistry("", true),
		broadcastHub:          js.NewBroadcastHub(),
		incognitoBroadcastHub: js.NewBroadcastHub(),
	}
	load := func(client net.HTTP, url string) *engine.Session {
		t.Helper()
		_, _, _, session, _, err := f.loadDocumentCtx(context.Background(), client, url, 256, 256, 1, t.TempDir(), false, false)
		if err != nil {
			t.Fatalf("loadDocumentCtx(%s): %v", url, err)
		}
		t.Cleanup(func() { session.Close() })
		return session
	}
	normalSender := load(normalClient, "https://partition.example/normal-a")
	normalReceiver := load(normalClient, "https://partition.example/normal-b")
	privateReceiver := load(privateClient, "https://partition.example/private")
	for _, session := range []*engine.Session{normalReceiver, privateReceiver} {
		if _, err := session.EvaluateJS(`var partitionReceived = ''; var partitionChannel = new BroadcastChannel('same-name'); partitionChannel.onmessage = function(e) { partitionReceived = e.data; };`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := normalSender.EvaluateJS(`new BroadcastChannel('same-name').postMessage('normal-only');`); err != nil {
		t.Fatal(err)
	}
	normalReceiver.PumpJS(256)
	privateReceiver.PumpJS(256)
	if got, err := normalReceiver.EvaluateJS(`partitionReceived`); err != nil || got != "normal-only" {
		t.Fatalf("normal context received %q, err=%v; want normal-only", got, err)
	}
	if got, err := privateReceiver.EvaluateJS(`partitionReceived`); err != nil || got != "" {
		t.Fatalf("private context received %q, err=%v; want no message", got, err)
	}
}
