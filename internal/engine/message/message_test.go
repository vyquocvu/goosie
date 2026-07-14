package message

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Round-trip encode / decode
// ---------------------------------------------------------------------------

func TestRoundTrip_Navigation(t *testing.T) {
	m := &Message{
		Version: 1,
		Time:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Navigation: &Navigation{
			NavID: 42,
			URL:   "https://example.com",
			State: StateNavigating,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if got.Navigation == nil {
		t.Fatal("Navigation is nil")
	}
	if got.Navigation.NavID != 42 {
		t.Errorf("NavID = %d, want 42", got.Navigation.NavID)
	}
	if got.Navigation.URL != "https://example.com" {
		t.Errorf("URL = %q, want https://example.com", got.Navigation.URL)
	}
	if got.Navigation.State != StateNavigating {
		t.Errorf("State = %d, want %d", got.Navigation.State, StateNavigating)
	}
}

func TestRoundTrip_Title(t *testing.T) {
	m := &Message{
		Version: 1,
		Time:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Title: &Title{
			NavID: 7,
			Title: "Hello, World!",
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title == nil {
		t.Fatal("Title is nil")
	}
	if got.Title.NavID != 7 {
		t.Errorf("NavID = %d, want 7", got.Title.NavID)
	}
	if got.Title.Title != "Hello, World!" {
		t.Errorf("Title = %q, want Hello, World!", got.Title.Title)
	}
}

func TestRoundTrip_URL(t *testing.T) {
	m := &Message{
		Version: 1,
		URL: &URL{
			NavID: 3,
			URL:   "https://example.com/page2",
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.URL == nil {
		t.Fatal("URL is nil")
	}
	if got.URL.NavID != 3 {
		t.Errorf("NavID = %d, want 3", got.URL.NavID)
	}
	if got.URL.URL != "https://example.com/page2" {
		t.Errorf("URL = %q", got.URL.URL)
	}
}

func TestRoundTrip_Paint(t *testing.T) {
	m := &Message{
		Version: 1,
		Paint: &FirstPaint{
			NavID: 1,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Paint == nil {
		t.Fatal("Paint is nil")
	}
	if got.Paint.NavID != 1 {
		t.Errorf("NavID = %d, want 1", got.Paint.NavID)
	}
}

func TestRoundTrip_Progress(t *testing.T) {
	m := &Message{
		Version: 1,
		Progress: &Progress{
			NavID:    1,
			Progress: 0.75,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Progress == nil {
		t.Fatal("Progress is nil")
	}
	if got.Progress.Progress != 0.75 {
		t.Errorf("Progress = %f, want 0.75", got.Progress.Progress)
	}
}

func TestRoundTrip_Error(t *testing.T) {
	m := &Message{
		Version: 1,
		Error: &Error{
			NavID:   5,
			Message: "connection refused",
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Error == nil {
		t.Fatal("Error is nil")
	}
	if got.Error.Message != "connection refused" {
		t.Errorf("Message = %q", got.Error.Message)
	}
}

func TestRoundTrip_Security(t *testing.T) {
	m := &Message{
		Version: 1,
		Security: &SecuritySummary{
			URL:    "https://example.com",
			Scheme: "https",
			Secure: true,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Security == nil {
		t.Fatal("Security is nil")
	}
	if !got.Security.Secure {
		t.Error("Secure should be true")
	}
}

func TestRoundTrip_Download(t *testing.T) {
	m := &Message{
		Version: 1,
		Download: &Download{
			URL:          "https://example.com/file.zip",
			Status:       "complete",
			BytesWritten: 1024,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Download == nil {
		t.Fatal("Download is nil")
	}
	if got.Download.BytesWritten != 1024 {
		t.Errorf("BytesWritten = %d, want 1024", got.Download.BytesWritten)
	}
}

func TestRoundTrip_Log(t *testing.T) {
	m := &Message{
		Version: 1,
		Log: &Log{
			Level:   "error",
			Message: "Uncaught ReferenceError: foo is not defined",
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Log == nil {
		t.Fatal("Log is nil")
	}
	if got.Log.Level != "error" {
		t.Errorf("Level = %q, want error", got.Log.Level)
	}
}

// ---------------------------------------------------------------------------
// Zero value
// ---------------------------------------------------------------------------

func TestZeroValue_EncodesAndDecodes(t *testing.T) {
	m := &Message{Version: 1}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if got.Navigation != nil {
		t.Error("Navigation should be nil")
	}
}

// ---------------------------------------------------------------------------
// Version compatibility
// ---------------------------------------------------------------------------

func TestDecode_WrongVersion(t *testing.T) {
	data := `{"v":2,"nav":{"navID":1,"state":1}}`
	_, err := Decode([]byte(data))
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !strings.Contains(err.Error(), "unsupported version") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecode_MalformedJSON(t *testing.T) {
	_, err := Decode([]byte(`{invalid`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// ---------------------------------------------------------------------------
// Wire format structure
// ---------------------------------------------------------------------------

func TestWireFormat_ContainsVersion(t *testing.T) {
	m := &Message{
		Version: 1,
		Navigation: &Navigation{
			NavID: 1,
			URL:   "https://example.com",
			State: StateNavigating,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wire), `"v": 1`) {
		t.Errorf("wire format missing version:\n%s", wire)
	}
	if !strings.Contains(string(wire), `"nav"`) {
		t.Errorf("wire format missing nav field:\n%s", wire)
	}
	// Unset payloads should be absent (omitempty).
	if strings.Contains(string(wire), `"title"`) {
		t.Errorf("wire format should not contain title:\n%s", wire)
	}
}

// ---------------------------------------------------------------------------
// All NavState values
// ---------------------------------------------------------------------------

func TestAllNavStatesRoundTrip(t *testing.T) {
	for _, s := range []NavState{
		StateCreated, StateNavigating, StateParsing,
		StateInteractive, StateComplete, StateCancelled,
		StateFailed, StateClosed,
	} {
		m := &Message{
			Version: 1,
			Navigation: &Navigation{
				NavID: 1,
				State: s,
			},
		}
		wire, err := m.Encode()
		if err != nil {
			t.Fatalf("state %d encode: %v", s, err)
		}
		got, err := Decode(wire)
		if err != nil {
			t.Fatalf("state %d decode: %v", s, err)
		}
		if got.Navigation.State != s {
			t.Errorf("state %d round-trip got %d", s, got.Navigation.State)
		}
	}
}

func TestRoundTrip_Input(t *testing.T) {
	m := &Message{
		Version: 1,
		Input: &Input{
			Kind: InputClick,
			X:    100,
			Y:    200,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Input == nil {
		t.Fatal("Input is nil")
	}
	if got.Input.Kind != InputClick {
		t.Errorf("Kind = %d, want %d", got.Input.Kind, InputClick)
	}
	if got.Input.X != 100 {
		t.Errorf("X = %f, want 100", got.Input.X)
	}
}

func TestRoundTrip_Viewport(t *testing.T) {
	m := &Message{
		Version: 1,
		Viewport: &Viewport{
			NavID:   1,
			Width:   1920,
			Height:  1080,
			ScrollY: 500,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Viewport == nil {
		t.Fatal("Viewport is nil")
	}
	if got.Viewport.Width != 1920 {
		t.Errorf("Width = %f, want 1920", got.Viewport.Width)
	}
	if got.Viewport.ScrollY != 500 {
		t.Errorf("ScrollY = %f, want 500", got.Viewport.ScrollY)
	}
}

func TestRoundTrip_Resource(t *testing.T) {
	m := &Message{
		Version: 1,
		Resource: &ResourceResponse{
			NavID:         1,
			URL:           "https://example.com/style.css",
			Status:        200,
			ContentType:   "text/css",
			ContentLength: 4096,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Resource == nil {
		t.Fatal("Resource is nil")
	}
	if got.Resource.Status != 200 {
		t.Errorf("Status = %d, want 200", got.Resource.Status)
	}
	if got.Resource.ContentType != "text/css" {
		t.Errorf("ContentType = %q", got.Resource.ContentType)
	}
}

func TestRoundTrip_Frame(t *testing.T) {
	m := &Message{
		Version: 1,
		Frame: &Frame{
			NavID:    1,
			Kind:     FrameCommit,
			Duration: 16_000_000,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Frame == nil {
		t.Fatal("Frame is nil")
	}
	if got.Frame.Kind != FrameCommit {
		t.Errorf("Kind = %d, want %d", got.Frame.Kind, FrameCommit)
	}
	if got.Frame.Duration != 16_000_000 {
		t.Errorf("Duration = %d", got.Frame.Duration)
	}
}

func TestRoundTrip_Crash(t *testing.T) {
	m := &Message{
		Version: 1,
		Crash: &Crash{
			Kind:    CrashJS,
			Message: "runtime: out of memory",
			Stack:   "goroutine 1 [running]:\nmain.foo()",
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Crash == nil {
		t.Fatal("Crash is nil")
	}
	if got.Crash.Kind != CrashJS {
		t.Errorf("Kind = %d, want %d", got.Crash.Kind, CrashJS)
	}
	if got.Crash.Message != "runtime: out of memory" {
		t.Errorf("Message = %q", got.Crash.Message)
	}
}

// ---------------------------------------------------------------------------
// InputKind, FrameKind, CrashInfoKind round-trip
// ---------------------------------------------------------------------------

func TestAllInputKindsRoundTrip(t *testing.T) {
	for _, k := range []InputKind{
		InputMouseDown, InputMouseUp, InputMouseMove, InputClick,
		InputKeyDown, InputKeyUp, InputKeyPress, InputScroll,
	} {
		m := &Message{Version: 1, Input: &Input{Kind: k}}
		wire, err := m.Encode()
		if err != nil {
			t.Fatalf("kind %d encode: %v", k, err)
		}
		got, err := Decode(wire)
		if err != nil {
			t.Fatalf("kind %d decode: %v", k, err)
		}
		if got.Input.Kind != k {
			t.Errorf("kind %d round-trip got %d", k, got.Input.Kind)
		}
	}
}

func TestAllFrameKindsRoundTrip(t *testing.T) {
	for _, k := range []FrameKind{FrameBegin, FrameCommit, FrameDrop} {
		m := &Message{Version: 1, Frame: &Frame{Kind: k}}
		wire, err := m.Encode()
		if err != nil {
			t.Fatalf("kind %d encode: %v", k, err)
		}
		got, err := Decode(wire)
		if err != nil {
			t.Fatalf("kind %d decode: %v", k, err)
		}
		if got.Frame.Kind != k {
			t.Errorf("kind %d round-trip got %d", k, got.Frame.Kind)
		}
	}
}

func TestAllCrashKindsRoundTrip(t *testing.T) {
	for _, k := range []CrashInfoKind{CrashEngine, CrashRenderer, CrashJS} {
		m := &Message{Version: 1, Crash: &Crash{Kind: k}}
		wire, err := m.Encode()
		if err != nil {
			t.Fatalf("kind %d encode: %v", k, err)
		}
		got, err := Decode(wire)
		if err != nil {
			t.Fatalf("kind %d decode: %v", k, err)
		}
		if got.Crash.Kind != k {
			t.Errorf("kind %d round-trip got %d", k, got.Crash.Kind)
		}
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkEncode_Navigation(b *testing.B) {
	m := &Message{
		Version: 1,
		Navigation: &Navigation{
			NavID: 42,
			URL:   "https://example.com/long/path/to/page.html",
			State: StateParsing,
		},
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = m.Encode()
	}
}

func BenchmarkDecode_Navigation(b *testing.B) {
	data := []byte(`{"v":1,"nav":{"navID":42,"url":"https://example.com/long/path/to/page.html","state":2}}`)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Decode(data)
	}
}

func BenchmarkRoundTrip_Navigation(b *testing.B) {
	m := &Message{
		Version: 1,
		Navigation: &Navigation{
			NavID: 42,
			URL:   "https://example.com/long/path/to/page.html",
			State: StateParsing,
		},
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		wire, _ := m.Encode()
		_, _ = Decode(wire)
	}
}

func BenchmarkEncode_Log(b *testing.B) {
	m := &Message{
		Version: 1,
		Log: &Log{
			Level:   "error",
			Message: "Uncaught ReferenceError: foo is not defined",
		},
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = m.Encode()
	}
}
