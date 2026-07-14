package renderer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/message"
)

// helper: connect a Child and Parent over in-memory pipes.
func connect(t *testing.T) (*Child, *Parent, func()) {
	t.Helper()

	childInR, childInW := io.Pipe()
	childOutR, childOutW := io.Pipe()

	child := NewChild(childInR, childOutW)
	parent := NewParent(childInW, childOutR)

	cleanup := func() {
		childInR.Close()
		childInW.Close()
		childOutR.Close()
		childOutW.Close()
	}

	return child, parent, cleanup
}

func TestProtocolRoundTrip(t *testing.T) {
	cmds := []*Command{
		NewNavigateCommand("https://example.com"),
		NewSetViewportCommand(800, 600),
		NewInputCommand(message.Input{
			Kind: message.InputClick,
			X:    100,
			Y:    200,
		}),
		NewPingCommand(),
		NewCloseCommand(),
	}

	for _, cmd := range cmds {
		data, err := cmd.Encode()
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		got, err := DecodeCommand(data)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if got.Version != ProtocolVersion {
			t.Errorf("Version = %d, want %d", got.Version, ProtocolVersion)
		}
	}
}

func TestNavigateProducesEvents(t *testing.T) {
	child, parent, cleanup := connect(t)
	defer cleanup()

	var mu sync.Mutex
	var events []*message.Message
	parent.OnEvent = func(msg *message.Message) {
		mu.Lock()
		events = append(events, msg)
		mu.Unlock()
	}

	parent.Start()

	// Start the child's command loop in background
	childCtx, childCancel := context.WithCancel(context.Background())
	defer childCancel()
	go child.Run(childCtx)

	// Send navigate command
	if err := parent.Send(NewNavigateCommand("https://example.com")); err != nil {
		t.Fatal(err)
	}

	// Wait for events to flow
	time.Sleep(300 * time.Millisecond)

	// Send close
	if err := parent.Send(NewCloseCommand()); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(events) == 0 {
		t.Fatal("no events received")
	}

	found := false
	for _, ev := range events {
		if ev.Navigation != nil {
			if ev.Navigation.State == message.StateNavigating {
				found = true
				if ev.Navigation.URL != "https://example.com" {
					t.Errorf("URL = %q, want %q", ev.Navigation.URL, "https://example.com")
				}
			}
		}
	}
	if !found {
		t.Error("no Navigation event with StateNavigating found")
	}
}

func TestPingPong(t *testing.T) {
	child, parent, cleanup := connect(t)
	defer cleanup()

	var mu sync.Mutex
	var pongReceived bool
	parent.OnEvent = func(msg *message.Message) {
		mu.Lock()
		pongReceived = true
		mu.Unlock()
	}

	parent.Start()

	childCtx, childCancel := context.WithCancel(context.Background())
	defer childCancel()
	go child.Run(childCtx)

	if err := parent.Send(NewPingCommand()); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !pongReceived {
		t.Error("no pong received")
	}
}

func TestChildCloseExitsRun(t *testing.T) {
	child, parent, cleanup := connect(t)
	defer cleanup()

	parent.Start()

	errCh := make(chan error, 1)
	go func() {
		errCh <- child.Run(context.Background())
	}()

	if err := parent.Send(NewCloseCommand()); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("child.Run returned: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("child did not exit after Close command")
	}
}

func TestParentCloseExitsChild(t *testing.T) {
	child, parent, cleanup := connect(t)
	defer cleanup()

	parent.Start()

	errCh := make(chan error, 1)
	go func() {
		errCh <- child.Run(context.Background())
	}()

	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("child.Run returned: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("child did not exit after parent close")
	}
}

func TestInvalidCommandReturnsError(t *testing.T) {
	var buf bytes.Buffer
	c := NewChild(strings.NewReader("{invalid json\n"), &buf)

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.Run(context.Background())
	}()

	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("child did not exit after invalid input")
	}

	output := buf.String()
	if !strings.Contains(output, "decode error") {
		t.Errorf("expected decode error event, got: %s", output)
	}
}

func TestVersionMismatchRejectsCommand(t *testing.T) {
	cmd := &Command{
		Version: 999,
		Navigate: &NavigateCmd{URL: "https://example.com"},
	}
	data, err := cmd.Encode()
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecodeCommand(data)
	if err == nil {
		t.Fatal("expected error for mismatched version")
	}
	if !strings.Contains(err.Error(), "unsupported protocol version") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMessageWireFormat(t *testing.T) {
	msg := &message.Message{
		Version: message.Version,
		Time:    time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
		Navigation: &message.Navigation{
			NavID: 1,
			URL:   "https://example.com",
			State: message.StateComplete,
		},
	}

	data, err := msg.Encode()
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if v, ok := parsed["v"].(float64); !ok || v != 1 {
		t.Errorf("v = %v, want 1", parsed["v"])
	}

	if _, ok := parsed["nav"]; !ok {
		t.Error("missing nav field in wire format")
	}
}

func TestChildWritesNewlineDelimited(t *testing.T) {
	var buf bytes.Buffer
	c := NewChild(strings.NewReader(""), &buf)

	c.writeEvent(&message.Message{
		Version: message.Version,
	})

	output := buf.String()
	if !strings.HasSuffix(output, "\n") {
		t.Error("event does not end with newline")
	}

	trimmed := strings.TrimSuffix(output, "\n")
	var parsed message.Message
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestParentSendConcurrent(t *testing.T) {
	child, parent, cleanup := connect(t)
	defer cleanup()

	parent.Start()

	childCtx, childCancel := context.WithCancel(context.Background())
	defer childCancel()
	go child.Run(childCtx)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			parent.Send(NewPingCommand())
		}()
	}
	wg.Wait()

	time.Sleep(100 * time.Millisecond)
}

func TestViewportCommandOverIPC(t *testing.T) {
	child, parent, cleanup := connect(t)
	defer cleanup()

	var mu sync.Mutex
	var events []*message.Message
	parent.OnEvent = func(msg *message.Message) {
		mu.Lock()
		events = append(events, msg)
		mu.Unlock()
	}

	parent.Start()
	childCtx, childCancel := context.WithCancel(context.Background())
	defer childCancel()
	go child.Run(childCtx)

	if err := parent.Send(NewSetViewportCommand(1024, 768)); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	if err := parent.Send(NewCloseCommand()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, ev := range events {
		if ev.Viewport != nil {
			found = true
			if ev.Viewport.Width != 1024 {
				t.Errorf("Width = %f, want 1024", ev.Viewport.Width)
			}
			if ev.Viewport.Height != 768 {
				t.Errorf("Height = %f, want 768", ev.Viewport.Height)
			}
		}
	}
	if !found {
		t.Error("no Viewport event received")
	}
}

func TestInputCommandOverIPC(t *testing.T) {
	child, parent, cleanup := connect(t)
	defer cleanup()

	var mu sync.Mutex
	var events []*message.Message
	parent.OnEvent = func(msg *message.Message) {
		mu.Lock()
		events = append(events, msg)
		mu.Unlock()
	}

	parent.Start()
	childCtx, childCancel := context.WithCancel(context.Background())
	defer childCancel()
	go child.Run(childCtx)

	if err := parent.Send(NewInputCommand(message.Input{
		Kind: message.InputClick,
		X:    50,
		Y:    100,
	})); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	if err := parent.Send(NewCloseCommand()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, ev := range events {
		if ev.Log != nil && strings.Contains(ev.Log.Message, "input") {
			found = true
		}
	}
	if !found {
		t.Error("no input log event received")
	}
}

func TestNavigateAndViewportSequence(t *testing.T) {
	child, parent, cleanup := connect(t)
	defer cleanup()

	var mu sync.Mutex
	var events []*message.Message
	parent.OnEvent = func(msg *message.Message) {
		mu.Lock()
		events = append(events, msg)
		mu.Unlock()
	}

	parent.Start()
	childCtx, childCancel := context.WithCancel(context.Background())
	defer childCancel()
	go child.Run(childCtx)

	// Send navigate, then viewport
	if err := parent.Send(NewNavigateCommand("https://example.com")); err != nil {
		t.Fatal(err)
	}
	if err := parent.Send(NewSetViewportCommand(1280, 720)); err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	if err := parent.Send(NewCloseCommand()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	hasNav := false
	hasVP := false
	for _, ev := range events {
		if ev.Navigation != nil {
			hasNav = true
		}
		if ev.Viewport != nil {
			hasVP = true
		}
	}
	if !hasNav {
		t.Error("no Navigation event in sequence")
	}
	if !hasVP {
		t.Error("no Viewport event in sequence")
	}
}

func TestFrameOutputOverIPC(t *testing.T) {
	child, parent, cleanup := connect(t)
	defer cleanup()

	var mu sync.Mutex
	var events []*message.Message
	parent.OnEvent = func(msg *message.Message) {
		mu.Lock()
		events = append(events, msg)
		mu.Unlock()
	}

	parent.Start()
	childCtx, childCancel := context.WithCancel(context.Background())
	defer childCancel()
	go child.Run(childCtx)

	if err := parent.Send(NewNavigateCommand("https://example.com")); err != nil {
		t.Fatal(err)
	}

	// Wait for navigation + frame events + completion
	time.Sleep(300 * time.Millisecond)

	if err := parent.Send(NewCloseCommand()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	var frameEvents []*message.Frame
	for _, ev := range events {
		if ev.Frame != nil {
			frameEvents = append(frameEvents, ev.Frame)
		}
	}

	if len(frameEvents) < 2 {
		t.Fatalf("expected at least 2 frame events (begin + commit), got %d", len(frameEvents))
	}

	if frameEvents[0].Kind != message.FrameBegin {
		t.Errorf("first frame event Kind = %d, want FrameBegin", frameEvents[0].Kind)
	}
	if frameEvents[1].Kind != message.FrameCommit {
		t.Errorf("second frame event Kind = %d, want FrameCommit", frameEvents[1].Kind)
	}
	if frameEvents[1].Duration <= 0 {
		t.Error("FrameCommit Duration should be positive")
	}
}
