package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// mockWSConn is a mock WebSocket connection for testing.
type mockWSConn struct {
	sent    []string
	closed  bool
	msgs    []mockMsg
	readIdx int
}

type mockMsg struct {
	typ  int
	data []byte
}

func (m *mockWSConn) Send(data string) error {
	m.sent = append(m.sent, data)
	return nil
}

func (m *mockWSConn) SendBytes(data []byte) error {
	m.sent = append(m.sent, string(data))
	return nil
}

func (m *mockWSConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockWSConn) ReadMessage() (int, []byte, error) {
	if m.readIdx >= len(m.msgs) {
		time.Sleep(100 * time.Millisecond)
		return 0, nil, &mockClosedError{}
	}
	msg := m.msgs[m.readIdx]
	m.readIdx++
	return msg.typ, msg.data, nil
}

type mockClosedError struct{}

func (e *mockClosedError) Error() string { return "connection closed" }

// mockWSDialer is a mock WebSocket dialer for testing.
type mockWSDialer struct {
	conn      *mockWSConn
	protocol  string
	dialErr   error
	dialCount int
}

func (d *mockWSDialer) Dial(url string, protocols []string) (js.WebSocketConn, string, error) {
	d.dialCount++
	if d.dialErr != nil {
		return nil, "", d.dialErr
	}
	return d.conn, d.protocol, nil
}

func TestWebSocketConstructor(t *testing.T) {
	conn := &mockWSConn{}
	dialer := &mockWSDialer{conn: conn, protocol: "chat"}

	r, err := js.New(js.Options{
		Timeout:         5 * time.Second,
		URL:             "https://example.com",
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var ws = new WebSocket('wss://echo.example.com');
		var url = ws.url;
		var readyState = ws.readyState;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("url")
	if !ok {
		t.Fatal("url not found")
	}
	if val.Str != "wss://echo.example.com" {
		t.Errorf("url = %q, want %q", val.Str, "wss://echo.example.com")
	}

	val, ok = r.Global("readyState")
	if !ok {
		t.Fatal("readyState not found")
	}
	if val.Num != 0 { // CONNECTING
		t.Errorf("readyState = %v, want 0 (CONNECTING)", val.Num)
	}
}

func TestWebSocketConstants(t *testing.T) {
	conn := &mockWSConn{}
	dialer := &mockWSDialer{conn: conn}

	r, err := js.New(js.Options{
		Timeout:         5 * time.Second,
		URL:             "https://example.com",
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var c = WebSocket.CONNECTING;
		var o = WebSocket.OPEN;
		var cl = WebSocket.CLOSING;
		var cd = WebSocket.CLOSED;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	tests := []struct {
		name string
		want float64
	}{
		{"c", 0},
		{"o", 1},
		{"cl", 2},
		{"cd", 3},
	}

	for _, tt := range tests {
		val, ok := r.Global(tt.name)
		if !ok {
			t.Fatalf("%s not found", tt.name)
		}
		if val.Num != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, val.Num, tt.want)
		}
	}
}

func TestWebSocketSend(t *testing.T) {
	conn := &mockWSConn{}
	dialer := &mockWSDialer{conn: conn, protocol: ""}

	r, err := js.New(js.Options{
		Timeout:         5 * time.Second,
		URL:             "https://example.com",
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var ws = new WebSocket('wss://echo.example.com');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Wait for connection to open.
	time.Sleep(50 * time.Millisecond)

	err = r.Run(`
		ws.send('hello');
		ws.send('world');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(conn.sent) != 2 {
		t.Fatalf("sent %d messages, want 2", len(conn.sent))
	}
	if conn.sent[0] != "hello" {
		t.Errorf("sent[0] = %q, want %q", conn.sent[0], "hello")
	}
	if conn.sent[1] != "world" {
		t.Errorf("sent[1] = %q, want %q", conn.sent[1], "world")
	}
}

func TestWebSocketClose(t *testing.T) {
	conn := &mockWSConn{}
	dialer := &mockWSDialer{conn: conn}

	r, err := js.New(js.Options{
		Timeout:         5 * time.Second,
		URL:             "https://example.com",
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var ws = new WebSocket('wss://echo.example.com');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	err = r.Run(`
		ws.close(1000, 'normal closure');
		var state = ws.readyState;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("state")
	if !ok {
		t.Fatal("state not found")
	}
	if val.Num != 3 { // CLOSED
		t.Errorf("readyState = %v, want 3 (CLOSED)", val.Num)
	}

	if !conn.closed {
		t.Error("connection not closed")
	}
}

func TestWebSocketOnWindow(t *testing.T) {
	conn := &mockWSConn{}
	dialer := &mockWSDialer{conn: conn}

	r, err := js.New(js.Options{
		Timeout:         5 * time.Second,
		URL:             "https://example.com",
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var ws = new window.WebSocket('wss://echo.example.com');
		var url = ws.url;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("url")
	if !ok {
		t.Fatal("url not found")
	}
	if val.Str != "wss://echo.example.com" {
		t.Errorf("url = %q, want %q", val.Str, "wss://echo.example.com")
	}
}

// CORS tests

func TestCORSIsCORSRequest(t *testing.T) {
	tests := []struct {
		docURL   string
		reqURL   string
		wantCORS bool
	}{
		{"https://example.com", "https://example.com/api", false},
		{"https://example.com", "https://api.example.com", true},
		{"https://example.com", "http://example.com", true},
		{"https://example.com", "https://other.com", true},
	}

	for _, tt := range tests {
		got := js.IsCORSRequest(tt.docURL, tt.reqURL)
		if got != tt.wantCORS {
			t.Errorf("isCORSRequest(%q, %q) = %v, want %v",
				tt.docURL, tt.reqURL, got, tt.wantCORS)
		}
	}
}

func TestCORSIsSimpleMethod(t *testing.T) {
	tests := []struct {
		method     string
		wantSimple bool
	}{
		{"GET", true},
		{"HEAD", true},
		{"POST", true},
		{"PUT", false},
		{"DELETE", false},
		{"PATCH", false},
	}

	for _, tt := range tests {
		got := js.IsSimpleMethod(tt.method)
		if got != tt.wantSimple {
			t.Errorf("isSimpleMethod(%q) = %v, want %v",
				tt.method, got, tt.wantSimple)
		}
	}
}

func TestCORSNeedsPreflight(t *testing.T) {
	tests := []struct {
		method        string
		headers       map[string]string
		wantPreflight bool
	}{
		{"GET", nil, false},
		{"POST", map[string]string{"Content-Type": "text/plain"}, false},
		{"PUT", nil, true},
		{"POST", map[string]string{"X-Custom-Header": "value"}, true},
		{"POST", map[string]string{"Content-Type": "application/json"}, true},
	}

	for _, tt := range tests {
		got := js.NeedsPreflight(tt.method, tt.headers)
		if got != tt.wantPreflight {
			t.Errorf("needsPreflight(%q, %v) = %v, want %v",
				tt.method, tt.headers, got, tt.wantPreflight)
		}
	}
}

// CSP tests

func TestCSPParse(t *testing.T) {
	header := "default-src 'self'; script-src 'self' https://cdn.example.com; img-src *; style-src 'unsafe-inline'"
	policy := js.ParseCSP(header)

	if policy == nil {
		t.Fatal("ParseCSP returned nil")
	}

	if len(policy.Directives) != 4 {
		t.Errorf("got %d directives, want 4", len(policy.Directives))
	}

	if _, ok := policy.Directives["default-src"]; !ok {
		t.Error("default-src directive not found")
	}
	if _, ok := policy.Directives["script-src"]; !ok {
		t.Error("script-src directive not found")
	}
}

func TestCSPAllowsScript(t *testing.T) {
	policy := js.ParseCSP("script-src 'self' https://cdn.example.com")

	tests := []struct {
		src       string
		docURL    string
		wantAllow bool
	}{
		{"https://example.com/app.js", "https://example.com", true},
		{"https://cdn.example.com/lib.js", "https://example.com", true},
		{"https://evil.com/malware.js", "https://example.com", false},
		{"http://example.com/app.js", "https://example.com", false},
	}

	for _, tt := range tests {
		got := policy.AllowsScript(tt.src, tt.docURL)
		if got != tt.wantAllow {
			t.Errorf("AllowsScript(%q, %q) = %v, want %v",
				tt.src, tt.docURL, got, tt.wantAllow)
		}
	}
}

func TestCSPAllowsInline(t *testing.T) {
	policy1 := js.ParseCSP("script-src 'self'")
	if policy1.AllowsInlineScript() {
		t.Error("inline script should not be allowed without 'unsafe-inline'")
	}

	policy2 := js.ParseCSP("script-src 'self' 'unsafe-inline'")
	if !policy2.AllowsInlineScript() {
		t.Error("inline script should be allowed with 'unsafe-inline'")
	}
}

func TestCSPAllowsEval(t *testing.T) {
	policy1 := js.ParseCSP("script-src 'self'")
	if policy1.AllowsEval() {
		t.Error("eval should not be allowed without 'unsafe-eval'")
	}

	policy2 := js.ParseCSP("script-src 'self' 'unsafe-eval'")
	if !policy2.AllowsEval() {
		t.Error("eval should be allowed with 'unsafe-eval'")
	}
}

func TestCSPDefaultSrc(t *testing.T) {
	policy := js.ParseCSP("default-src 'self'")

	if !policy.AllowsScript("https://example.com/app.js", "https://example.com") {
		t.Error("same-origin script should be allowed by default-src 'self'")
	}
	if policy.AllowsScript("https://cdn.example.com/lib.js", "https://example.com") {
		t.Error("cross-origin script should not be allowed by default-src 'self'")
	}
	if !policy.AllowsImage("https://example.com/img.png", "https://example.com") {
		t.Error("same-origin image should be allowed by default-src 'self'")
	}
}

func TestCSPNone(t *testing.T) {
	policy := js.ParseCSP("script-src 'none'")

	if policy.AllowsScript("https://example.com/app.js", "https://example.com") {
		t.Error("'none' should block all sources")
	}
}

func TestCSPWildcard(t *testing.T) {
	policy := js.ParseCSP("img-src *")

	if !policy.AllowsImage("https://example.com/img.png", "https://example.com") {
		t.Error("wildcard should allow all sources")
	}
	if !policy.AllowsImage("https://cdn.other.com/img.png", "https://example.com") {
		t.Error("wildcard should allow all sources")
	}
}

func TestCSPNilPolicy(t *testing.T) {
	var policy *js.CSPPolicy

	if !policy.AllowsScript("https://example.com/app.js", "https://example.com") {
		t.Error("nil policy should allow everything")
	}
	if !policy.AllowsInlineScript() {
		t.Error("nil policy should allow inline scripts")
	}
	if !policy.AllowsEval() {
		t.Error("nil policy should allow eval")
	}
}
