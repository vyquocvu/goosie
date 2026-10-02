package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// mockWorkerRunner is a mock worker runner for testing.
type mockWorkerRunner struct {
	messages []string
	term     bool
}

func (m *mockWorkerRunner) Run(scriptURL string, onData func(string), onError func(error)) error {
	return nil
}

func (m *mockWorkerRunner) PostMessage(data string) error {
	m.messages = append(m.messages, data)
	return nil
}

func (m *mockWorkerRunner) Terminate() error {
	m.term = true
	return nil
}

// mockWorkerDialer is a mock worker dialer for testing.
type mockWorkerDialer struct {
	runner *mockWorkerRunner
}

func (d *mockWorkerDialer) CreateWorker(scriptURL string) (js.WorkerRunner, error) {
	return d.runner, nil
}

func TestWorkerConstructor(t *testing.T) {
	runner := &mockWorkerRunner{}
	dialer := &mockWorkerDialer{runner: runner}

	r, err := js.New(js.Options{
		Timeout:      5 * time.Second,
		URL:          "https://example.com",
		WorkerDialer: dialer,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var worker = new Worker('worker.js');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestWorkerPostMessage(t *testing.T) {
	runner := &mockWorkerRunner{}
	dialer := &mockWorkerDialer{runner: runner}

	r, err := js.New(js.Options{
		Timeout:      5 * time.Second,
		URL:          "https://example.com",
		WorkerDialer: dialer,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var worker = new Worker('worker.js');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	err = r.Run(`
		worker.postMessage('hello');
		worker.postMessage('world');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(runner.messages) != 2 {
		t.Fatalf("sent %d messages, want 2", len(runner.messages))
	}
	if runner.messages[0] != "hello" {
		t.Errorf("messages[0] = %q, want %q", runner.messages[0], "hello")
	}
}

func TestWorkerTerminate(t *testing.T) {
	runner := &mockWorkerRunner{}
	dialer := &mockWorkerDialer{runner: runner}

	r, err := js.New(js.Options{
		Timeout:      5 * time.Second,
		URL:          "https://example.com",
		WorkerDialer: dialer,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var worker = new Worker('worker.js');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	err = r.Run(`
		worker.terminate();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !runner.term {
		t.Error("worker not terminated")
	}
}

func TestWorkerOnWindow(t *testing.T) {
	runner := &mockWorkerRunner{}
	dialer := &mockWorkerDialer{runner: runner}

	r, err := js.New(js.Options{
		Timeout:      5 * time.Second,
		URL:          "https://example.com",
		WorkerDialer: dialer,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var worker = new window.Worker('worker.js');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// Service Worker tests

func TestServiceWorkerRegister(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var reg = navigator.serviceWorker.register('/sw.js');
		var scope = reg.scope;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("scope")
	if !ok {
		t.Fatal("scope not found")
	}
	if val.Str != "/" {
		t.Errorf("scope = %q, want %q", val.Str, "/")
	}
}

func TestServiceWorkerRegisterWithScope(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var reg = navigator.serviceWorker.register('/sw.js', { scope: '/app/' });
		var scope = reg.scope;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("scope")
	if !ok {
		t.Fatal("scope not found")
	}
	if val.Str != "/app/" {
		t.Errorf("scope = %q, want %q", val.Str, "/app/")
	}
}

func TestServiceWorkerGetRegistration(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		navigator.serviceWorker.register('/sw.js');
		var reg = navigator.serviceWorker.getRegistration('/');
		var hasReg = reg !== null;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("hasReg")
	if !ok {
		t.Fatal("hasReg not found")
	}
	if !val.Bool {
		t.Error("expected registration to be found")
	}
}

func TestServiceWorkerUnregister(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var reg = navigator.serviceWorker.register('/sw.js');
		var result = reg.unregister();
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestServiceWorkerOnWindow(t *testing.T) {
	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var hasSW = window.navigator.serviceWorker !== undefined;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, ok := r.Global("hasSW")
	if !ok {
		t.Fatal("hasSW not found")
	}
	if !val.Bool {
		t.Error("expected serviceWorker to be available on window.navigator")
	}
}
