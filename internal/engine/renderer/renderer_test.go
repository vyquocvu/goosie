package renderer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestChildPanicSendsCrashEvent(t *testing.T) {
	// Create a child that panics when it receives a navigate command.
	childInR, childInW := io.Pipe()
	childOutR, childOutW := io.Pipe()
	defer childInR.Close()
	defer childInW.Close()
	defer childOutR.Close()
	defer childOutW.Close()

	c := NewChild(childInR, childOutW)
	panickingChild := &panicOnNavigate{Child: c}

	var mu sync.Mutex
	var events []*message.Message

	parent := NewParent(childInW, childOutR)
	parent.OnEvent = func(msg *message.Message) {
		mu.Lock()
		events = append(events, msg)
		mu.Unlock()
	}
	parent.Start()

	go panickingChild.Run(context.Background())

	parent.Send(NewNavigateCommand("https://crash.example.com"))
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	foundCrash := false
	for _, ev := range events {
		if ev.Crash != nil {
			foundCrash = true
			if ev.Crash.Kind != message.CrashEngine {
				t.Errorf("Crash.Kind = %d, want CrashEngine", ev.Crash.Kind)
			}
		}
	}
	if !foundCrash {
		t.Error("no Crash event received from panicking child")
	}
}

func TestUnexpectedChildExitDetected(t *testing.T) {
	childInR, childInW := io.Pipe()
	childOutR, childOutW := io.Pipe()

	parent := NewParent(childInW, childOutR)

	var mu sync.Mutex
	var exitErr error
	parent.OnExit = func(err error) {
		mu.Lock()
		exitErr = err
		mu.Unlock()
	}
	parent.Start()

	// Simulate child crash by closing pipes abruptly (no Close command sent).
	childOutW.Close()
	childInR.Close()

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if exitErr == nil {
		t.Error("expected non-nil error for unexpected child exit")
	}
}

func TestGracefulCloseNoCrash(t *testing.T) {
	child, parent, cleanup := connect(t)
	defer cleanup()

	var mu sync.Mutex
	var exitErr error
	parent.OnExit = func(err error) {
		mu.Lock()
		exitErr = err
		mu.Unlock()
	}

	parent.Start()
	childCtx, childCancel := context.WithCancel(context.Background())
	defer childCancel()
	go child.Run(childCtx)

	parent.Close()
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if exitErr != nil {
		t.Errorf("expected nil error for graceful close, got: %v", exitErr)
	}
}

// panicOnNavigate wraps a Child and panics when it receives a Navigate command.
type panicOnNavigate struct {
	*Child
}

func (p *panicOnNavigate) Run(ctx context.Context) error {
	defer func() {
		if r := recover(); r != nil {
			p.writeEvent(&message.Message{
				Version: message.Version,
				Time:    time.Now(),
				Crash: &message.Crash{
					Kind:    message.CrashEngine,
					Message: fmt.Sprintf("child panic: %v", r),
				},
			})
		}
	}()

	// Read one command, panic on navigate, then delegate to normal Run.
	scanner := bufio.NewScanner(p.r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if scanner.Scan() {
		line := scanner.Bytes()
		cmd, err := DecodeCommand(line)
		if err == nil && cmd.Navigate != nil {
			panic("test: forced engine crash on navigate")
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Tab (crash recovery) tests
// ---------------------------------------------------------------------------

func TestTabCrashDetected(t *testing.T) {
	childInR, childInW := io.Pipe()
	childOutR, childOutW := io.Pipe()

	child := NewChild(childInR, childOutW)
	parent := NewParent(childInW, childOutR)

	var mu sync.Mutex
	var crashErr error
	tab := NewTab(child, parent)
	tab.OnCrash = func(err error) {
		mu.Lock()
		crashErr = err
		mu.Unlock()
	}

	go child.Run(context.Background())

	// Simulate crash by closing child's output pipe abruptly.
	childOutW.Close()
	childInR.Close()

	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if crashErr == nil {
		t.Error("OnCrash was not called after pipe close")
	}
}

func TestTabRestartAfterCrash(t *testing.T) {
	// Phase 1: set up tab, crash it, detect crash
	childInR1, childInW1 := io.Pipe()
	childOutR1, childOutW1 := io.Pipe()

	child1 := NewChild(childInR1, childOutW1)
	parent1 := NewParent(childInW1, childOutR1)

	var mu sync.Mutex
	var crashDetected bool
	tab := NewTab(child1, parent1)
	tab.OnCrash = func(err error) {
		mu.Lock()
		crashDetected = true
		mu.Unlock()
	}

	go child1.Run(context.Background())

	// Navigate first
	tab.Navigate("https://example.com")
	time.Sleep(100 * time.Millisecond)

	// Crash the child
	childOutW1.Close()
	childInR1.Close()
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	if !crashDetected {
		mu.Unlock()
		t.Fatal("crash not detected")
	}
	mu.Unlock()

	// Phase 2: restart with new pipes
	childInR2, childInW2 := io.Pipe()
	childOutR2, childOutW2 := io.Pipe()
	child2 := NewChild(childInR2, childOutW2)

	var recovered bool
	var events []*message.Message
	tab.OnEvent = func(msg *message.Message) {
		mu.Lock()
		events = append(events, msg)
		mu.Unlock()
	}
	tab.OnRecover = func() {
		mu.Lock()
		recovered = true
		mu.Unlock()
	}

	tab.Restart(childInR2, childOutW2, childInW2, childOutR2)

	// Start the new child first, then replay navigation
	go child2.Run(context.Background())

	// Replay the last navigation
	tab.mu.Lock()
	lastURL := tab.nextURL
	tab.mu.Unlock()
	if lastURL != "" {
		tab.Navigate(lastURL)
	}

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if !recovered {
		t.Error("OnRecover was not called")
	}

	// Should have navigation event from the replayed URL
	foundNav := false
	for _, ev := range events {
		if ev.Navigation != nil {
			foundNav = true
			if ev.Navigation.URL != "https://example.com" {
				t.Errorf("replayed URL = %q, want %q", ev.Navigation.URL, "https://example.com")
			}
		}
	}
	if !foundNav {
		t.Error("no Navigation event after restart")
	}
}

func TestTabGracefulClose(t *testing.T) {
	child, parent, cleanup := connect(t)
	defer cleanup()

	tab := NewTab(child, parent)
	go child.Run(context.Background())

	if err := tab.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
}
