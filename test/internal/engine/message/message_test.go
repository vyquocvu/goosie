package message_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/message"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
	"github.com/vyquocvu/goosie/internal/engine/session"
	engineNet "github.com/vyquocvu/goosie/internal/net"
)

// ---------------------------------------------------------------------------
// Round-trip encode / decode
// ---------------------------------------------------------------------------

func TestRoundTrip_Navigation(t *testing.T) {
	m := &message.Message{
		Version: 1,
		Time:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Navigation: &message.Navigation{
			NavID: 42,
			URL:   "https://example.com",
			State: message.StateNavigating,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
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
	if got.Navigation.State != message.StateNavigating {
		t.Errorf("State = %d, want %d", got.Navigation.State, message.StateNavigating)
	}
}

func TestRoundTrip_Title(t *testing.T) {
	m := &message.Message{
		Version: 1,
		Time:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Title: &message.Title{
			NavID: 7,
			Title: "Hello, World!",
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
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
	m := &message.Message{
		Version: 1,
		URL: &message.URL{
			NavID: 3,
			URL:   "https://example.com/page2",
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
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
	m := &message.Message{
		Version: 1,
		Paint: &message.FirstPaint{
			NavID: 1,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
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
	m := &message.Message{
		Version: 1,
		Progress: &message.Progress{
			NavID:    1,
			Progress: 0.75,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
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
	m := &message.Message{
		Version: 1,
		Error: &message.Error{
			NavID:   5,
			Message: "connection refused",
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
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
	m := &message.Message{
		Version: 1,
		Security: &message.SecuritySummary{
			URL:    "https://example.com",
			Scheme: "https",
			Secure: true,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
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
	m := &message.Message{
		Version: 1,
		Download: &message.Download{
			URL:          "https://example.com/file.zip",
			Status:       "complete",
			BytesWritten: 1024,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
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
	m := &message.Message{
		Version: 1,
		Log: &message.Log{
			Level:   "error",
			Message: "Uncaught ReferenceError: foo is not defined",
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
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
	m := &message.Message{Version: 1}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
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
	_, err := message.Decode([]byte(data))
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !strings.Contains(err.Error(), "unsupported version") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecode_MalformedJSON(t *testing.T) {
	_, err := message.Decode([]byte(`{invalid`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// ---------------------------------------------------------------------------
// Wire format structure
// ---------------------------------------------------------------------------

func TestWireFormat_ContainsVersion(t *testing.T) {
	m := &message.Message{
		Version: 1,
		Navigation: &message.Navigation{
			NavID: 1,
			URL:   "https://example.com",
			State: message.StateNavigating,
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
	for _, s := range []message.NavState{
		message.StateCreated, message.StateNavigating, message.StateParsing,
		message.StateInteractive, message.StateComplete, message.StateCancelled,
		message.StateFailed, message.StateClosed,
	} {
		m := &message.Message{
			Version: 1,
			Navigation: &message.Navigation{
				NavID: 1,
				State: s,
			},
		}
		wire, err := m.Encode()
		if err != nil {
			t.Fatalf("state %d encode: %v", s, err)
		}
		got, err := message.Decode(wire)
		if err != nil {
			t.Fatalf("state %d decode: %v", s, err)
		}
		if got.Navigation.State != s {
			t.Errorf("state %d round-trip got %d", s, got.Navigation.State)
		}
	}
}

func TestRoundTrip_Input(t *testing.T) {
	m := &message.Message{
		Version: 1,
		Input: &message.Input{
			Kind: message.InputClick,
			X:    100,
			Y:    200,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Input == nil {
		t.Fatal("Input is nil")
	}
	if got.Input.Kind != message.InputClick {
		t.Errorf("Kind = %d, want %d", got.Input.Kind, message.InputClick)
	}
	if got.Input.X != 100 {
		t.Errorf("X = %f, want 100", got.Input.X)
	}
}

func TestRoundTrip_Viewport(t *testing.T) {
	m := &message.Message{
		Version: 1,
		Viewport: &message.Viewport{
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
	got, err := message.Decode(wire)
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
	m := &message.Message{
		Version: 1,
		Resource: &message.ResourceResponse{
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
	got, err := message.Decode(wire)
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
	m := &message.Message{
		Version: 1,
		Frame: &message.Frame{
			NavID:    1,
			Kind:     message.FrameCommit,
			Duration: 16_000_000,
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Frame == nil {
		t.Fatal("Frame is nil")
	}
	if got.Frame.Kind != message.FrameCommit {
		t.Errorf("Kind = %d, want %d", got.Frame.Kind, message.FrameCommit)
	}
	if got.Frame.Duration != 16_000_000 {
		t.Errorf("Duration = %d", got.Frame.Duration)
	}
}

func TestRoundTrip_Crash(t *testing.T) {
	m := &message.Message{
		Version: 1,
		Crash: &message.Crash{
			Kind:    message.CrashJS,
			Message: "runtime: out of memory",
			Stack:   "goroutine 1 [running]:\nmain.foo()",
		},
	}
	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got.Crash == nil {
		t.Fatal("Crash is nil")
	}
	if got.Crash.Kind != message.CrashJS {
		t.Errorf("Kind = %d, want %d", got.Crash.Kind, message.CrashJS)
	}
	if got.Crash.Message != "runtime: out of memory" {
		t.Errorf("Message = %q", got.Crash.Message)
	}
}

// ---------------------------------------------------------------------------
// InputKind, FrameKind, CrashInfoKind round-trip
// ---------------------------------------------------------------------------

func TestAllInputKindsRoundTrip(t *testing.T) {
	for _, k := range []message.InputKind{
		message.InputMouseDown, message.InputMouseUp, message.InputMouseMove, message.InputClick,
		message.InputKeyDown, message.InputKeyUp, message.InputKeyPress, message.InputScroll,
	} {
		m := &message.Message{Version: 1, Input: &message.Input{Kind: k}}
		wire, err := m.Encode()
		if err != nil {
			t.Fatalf("kind %d encode: %v", k, err)
		}
		got, err := message.Decode(wire)
		if err != nil {
			t.Fatalf("kind %d decode: %v", k, err)
		}
		if got.Input.Kind != k {
			t.Errorf("kind %d round-trip got %d", k, got.Input.Kind)
		}
	}
}

func TestAllFrameKindsRoundTrip(t *testing.T) {
	for _, k := range []message.FrameKind{message.FrameBegin, message.FrameCommit, message.FrameDrop} {
		m := &message.Message{Version: 1, Frame: &message.Frame{Kind: k}}
		wire, err := m.Encode()
		if err != nil {
			t.Fatalf("kind %d encode: %v", k, err)
		}
		got, err := message.Decode(wire)
		if err != nil {
			t.Fatalf("kind %d decode: %v", k, err)
		}
		if got.Frame.Kind != k {
			t.Errorf("kind %d round-trip got %d", k, got.Frame.Kind)
		}
	}
}

func TestAllCrashKindsRoundTrip(t *testing.T) {
	for _, k := range []message.CrashInfoKind{message.CrashEngine, message.CrashRenderer, message.CrashJS} {
		m := &message.Message{Version: 1, Crash: &message.Crash{Kind: k}}
		wire, err := m.Encode()
		if err != nil {
			t.Fatalf("kind %d encode: %v", k, err)
		}
		got, err := message.Decode(wire)
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
	m := &message.Message{
		Version: 1,
		Navigation: &message.Navigation{
			NavID: 42,
			URL:   "https://example.com/long/path/to/page.html",
			State: message.StateParsing,
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
		_, _ = message.Decode(data)
	}
}

func BenchmarkRoundTrip_Navigation(b *testing.B) {
	m := &message.Message{
		Version: 1,
		Navigation: &message.Navigation{
			NavID: 42,
			URL:   "https://example.com/long/path/to/page.html",
			State: message.StateParsing,
		},
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = m.Encode()
	}
}

// ---------------------------------------------------------------------------
// Event → Message converter
// ---------------------------------------------------------------------------

func TestConvertEvent_StateChange(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:  session.EventStateChange,
		NavID: navigation.ID(1),
		State: session.StateNavigating,
		URL:   "https://example.com",
	}
	m := message.Event(ev, ts)

	if m.Version != message.Version {
		t.Fatalf("version = %d, want %d", m.Version, message.Version)
	}
	if m.Navigation == nil {
		t.Fatal("Navigation payload is nil")
	}
	if m.Navigation.NavID != 1 {
		t.Errorf("NavID = %d, want 1", m.Navigation.NavID)
	}
	if m.Navigation.State != message.StateNavigating {
		t.Errorf("State = %d, want %d", m.Navigation.State, message.StateNavigating)
	}
	if m.Navigation.URL != "https://example.com" {
		t.Errorf("URL = %q, want %q", m.Navigation.URL, "https://example.com")
	}
}

func TestConvertEvent_TitleChange(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:  session.EventTitleChange,
		NavID: navigation.ID(5),
		Title: "Hello World",
	}
	m := message.Event(ev, ts)

	if m.Title == nil {
		t.Fatal("Title payload is nil")
	}
	if m.Title.Title != "Hello World" {
		t.Errorf("Title = %q, want %q", m.Title.Title, "Hello World")
	}
	if m.Title.NavID != 5 {
		t.Errorf("NavID = %d, want 5", m.Title.NavID)
	}
}

func TestConvertEvent_URLChange(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:  session.EventURLChange,
		NavID: navigation.ID(2),
		URL:   "https://go.dev",
	}
	m := message.Event(ev, ts)

	if m.URL == nil {
		t.Fatal("URL payload is nil")
	}
	if m.URL.URL != "https://go.dev" {
		t.Errorf("URL = %q, want %q", m.URL.URL, "https://go.dev")
	}
}

func TestConvertEvent_FirstPaint(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:  session.EventFirstPaint,
		NavID: navigation.ID(3),
	}
	m := message.Event(ev, ts)

	if m.Paint == nil {
		t.Fatal("Paint payload is nil")
	}
	if m.Paint.NavID != 3 {
		t.Errorf("NavID = %d, want 3", m.Paint.NavID)
	}
}

func TestConvertEvent_Progress(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:     session.EventProgress,
		NavID:    navigation.ID(4),
		Progress: 0.75,
	}
	m := message.Event(ev, ts)

	if m.Progress == nil {
		t.Fatal("Progress payload is nil")
	}
	if m.Progress.Progress != 0.75 {
		t.Errorf("Progress = %f, want 0.75", m.Progress.Progress)
	}
}

func TestConvertEvent_Error(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:  session.EventError,
		NavID: navigation.ID(6),
		Err:   errors.New("connection refused"),
	}
	m := message.Event(ev, ts)

	if m.Error == nil {
		t.Fatal("Error payload is nil")
	}
	if m.Error.Message != "connection refused" {
		t.Errorf("Message = %q, want %q", m.Error.Message, "connection refused")
	}
}

func TestConvertEvent_ErrorNil(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:  session.EventError,
		NavID: navigation.ID(7),
	}
	m := message.Event(ev, ts)

	if m.Error == nil {
		t.Fatal("Error payload is nil")
	}
	if m.Error.Message != "" {
		t.Errorf("Message = %q, want empty", m.Error.Message)
	}
}

func TestConvertEvent_Security(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:  session.EventSecuritySummary,
		NavID: navigation.ID(8),
		SecuritySummary: engineNet.SecuritySummary{
			URL:    "https://secure.example.com",
			Scheme: "https",
			Secure: true,
		},
	}
	m := message.Event(ev, ts)

	if m.Security == nil {
		t.Fatal("Security payload is nil")
	}
	if !m.Security.Secure {
		t.Error("Secure = false, want true")
	}
	if m.Security.Scheme != "https" {
		t.Errorf("Scheme = %q, want %q", m.Security.Scheme, "https")
	}
}

func TestConvertEvent_Download(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:  session.EventDownload,
		NavID: navigation.ID(9),
		Download: engineNet.DownloadRecord{
			URL:          "https://example.com/file.zip",
			Status:       engineNet.DownloadComplete,
			BytesWritten: 1024,
		},
	}
	m := message.Event(ev, ts)

	if m.Download == nil {
		t.Fatal("Download payload is nil")
	}
	if m.Download.URL != "https://example.com/file.zip" {
		t.Errorf("URL = %q, want %q", m.Download.URL, "https://example.com/file.zip")
	}
	if m.Download.Status != "complete" {
		t.Errorf("Status = %q, want %q", m.Download.Status, "complete")
	}
	if m.Download.BytesWritten != 1024 {
		t.Errorf("BytesWritten = %d, want 1024", m.Download.BytesWritten)
	}
}

func TestConvertEvent_WireRoundTrip(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:  session.EventStateChange,
		NavID: navigation.ID(42),
		State: session.StateComplete,
		URL:   "https://example.com",
	}
	m := message.Event(ev, ts)

	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := message.Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Navigation == nil {
		t.Fatal("decoded Navigation is nil")
	}
	if decoded.Navigation.NavID != 42 {
		t.Errorf("NavID = %d, want 42", decoded.Navigation.NavID)
	}
	if decoded.Navigation.State != message.StateComplete {
		t.Errorf("State = %d, want %d", decoded.Navigation.State, message.StateComplete)
	}
}

func TestConvertEvent_OtherEventsIgnored(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:  session.EventStateChange,
		NavID: navigation.ID(1),
		State: session.StateCreated,
	}
	m := message.Event(ev, ts)

	if m.Navigation == nil {
		t.Fatal("Navigation payload is nil")
	}
	if m.Title != nil {
		t.Error("Title should be nil for StateChange event")
	}
	if m.Error != nil {
		t.Error("Error should be nil for StateChange event")
	}
}

func TestConvertEvent_StateMapping(t *testing.T) {
	cases := []struct {
		sessionState session.State
		wantState    message.NavState
	}{
		{session.StateCreated, message.StateCreated},
		{session.StateNavigating, message.StateNavigating},
		{session.StateParsing, message.StateParsing},
		{session.StateInteractive, message.StateInteractive},
		{session.StateComplete, message.StateComplete},
		{session.StateCancelled, message.StateCancelled},
		{session.StateFailed, message.StateFailed},
		{session.StateClosed, message.StateClosed},
	}
	for _, tc := range cases {
		t.Run(tc.sessionState.String(), func(t *testing.T) {
			got := message.ConvertState(tc.sessionState)
			if got != tc.wantState {
				t.Errorf("ConvertState(%v) = %d, want %d", tc.sessionState, got, tc.wantState)
			}
		})
	}
}

func TestConvertEvent_Immutability(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:  session.EventTitleChange,
		NavID: navigation.ID(1),
		Title: "Original",
	}
	m1 := message.Event(ev, ts)

	ev.Title = "Modified"
	m2 := message.Event(ev, ts)

	if m1.Title.Title != "Original" {
		t.Errorf("first message mutated: Title = %q", m1.Title.Title)
	}
	if m2.Title.Title != "Modified" {
		t.Errorf("second message Title = %q, want %q", m2.Title.Title, "Modified")
	}
}

func TestConvertEvent_EncodeJSON(t *testing.T) {
	ts := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	ev := session.Event{
		Type:  session.EventStateChange,
		NavID: navigation.ID(10),
		State: session.StateParsing,
		URL:   "https://example.com/page",
	}
	m := message.Event(ev, ts)

	wire, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	s := string(wire)
	if !strings.Contains(s, `"navID": 10`) {
		t.Errorf("wire missing navID: %s", s)
	}
	if !strings.Contains(s, `"state": 2`) {
		t.Errorf("wire missing state=2 (Parsing): %s", s)
	}
}

func BenchmarkEncode_Log(b *testing.B) {
	m := &message.Message{
		Version: 1,
		Log: &message.Log{
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
