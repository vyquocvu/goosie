package js_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// mockWorkerRunner is a mock worker runner for testing.
type mockWorkerRunner struct {
	messages []any
	term     bool
}

func (m *mockWorkerRunner) Run(scriptURL string, onData func(any), onError func(error)) error {
	return nil
}

func (m *mockWorkerRunner) PostMessage(data any) error {
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

func TestWorkerFetchesClassicScriptsAndSharesBroadcastChannel(t *testing.T) {
	sources := map[string]string{
		"https://example.com/workers/worker.js": `importScripts('dependency.js');
onmessage = function() {
  if (!self.dependencyLoaded) throw new Error('importScripts did not run');
  var channel = new BroadcastChannel('worker-fetch');
  channel.postMessage('worker-ready');
};`,
		"https://example.com/workers/dependency.js": `self.dependencyLoaded = true;`,
	}
	r, err := js.New(js.Options{
		URL: "https://example.com/page.html",
		WorkerScriptFetcher: func(scriptURL string) ([]byte, error) {
			source, ok := sources[scriptURL]
			if !ok {
				return nil, fmt.Errorf("unexpected worker script URL %q", scriptURL)
			}
			return []byte(source), nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.Run(`
		var received = '';
		var channel = new BroadcastChannel('worker-fetch');
		channel.onmessage = function(event) { received = event.data; };
		var worker = new Worker('workers/worker.js');
		worker.postMessage('start');
	`, "page.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.Tick()
		if value, ok := r.Global("received"); ok && value.Str == "worker-ready" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	value, _ := r.Global("received")
	t.Fatalf("worker BroadcastChannel message not received; received=%v", value)
}

func TestBlobWorkerBroadcastChannelInheritsCreatorOrigin(t *testing.T) {
	r, err := js.New(js.Options{URL: "https://example.com/page.html"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var received = '';
		var channel = new BroadcastChannel('blob-worker-origin');
		channel.onmessage = function(event) { received = event.data; };
		var source = "onmessage = function() { new BroadcastChannel('blob-worker-origin').postMessage('from blob worker'); };";
		var worker = new Worker(URL.createObjectURL(new Blob([source], {type: 'application/javascript'})));
		worker.postMessage('start');
	`, "page.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.Tick()
		if value, ok := r.Global("received"); ok && value.Str == "from blob worker" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	value, _ := r.Global("received")
	t.Fatalf("Blob worker BroadcastChannel message not received; received=%v", value)
}

func TestBlobWorkerBroadcastChannelInheritsOpaqueCreatorOrigin(t *testing.T) {
	r, err := js.New(js.Options{URL: "data:text/html,opaque-document"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var received = '';
		var channel = new BroadcastChannel('opaque-blob-worker-origin');
		channel.onmessage = function(event) { received = event.data; };
		var source = "onmessage = function() { new BroadcastChannel('opaque-blob-worker-origin').postMessage('from opaque blob worker'); };";
		var worker = new Worker(URL.createObjectURL(new Blob([source], {type: 'application/javascript'})));
		worker.postMessage('start');
	`, "opaque-page.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.Tick()
		if value, ok := r.Global("received"); ok && value.Str == "from opaque blob worker" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	value, _ := r.Global("received")
	t.Fatalf("opaque Blob worker BroadcastChannel message not received; received=%v", value)
}

func TestDataURLWorkerRunsWithOpaqueOrigin(t *testing.T) {
	r, err := js.New(js.Options{URL: "https://example.com/page.html"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var workerResult = '';
		var worker = new Worker('data:,onmessage%3Dfunction()%7BpostMessage(typeof%20BroadcastChannel)%7D');
		worker.onmessage = function(event) { workerResult = event.data; };
		worker.postMessage('start');
	`, "data-worker-page.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.Tick()
		if value, ok := r.Global("workerResult"); ok && value.Str == "function" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	value, _ := r.Global("workerResult")
	t.Fatalf("data URL worker did not run or expose BroadcastChannel; result=%v", value)
}

func TestSharedWorkerConnectsMessagePortsAndReusesNamedWorker(t *testing.T) {
	r, err := js.New(js.Options{URL: "https://example.com/page.html"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var firstMessages = [];
		var secondConnection = '';
		var source = "data:,self.addEventListener%28%27connect%27%2Ce%3D%3E%7Blet%20p%3De.ports%5B0%5D%3Bp.postMessage%28%27connected-%27%2B%28++self.connections%7C%7C%28self.connections%3D1%29%29%29%3Bp.onmessage%3Devt%3D%3Ep.postMessage%28%27echo%3A%27%2Bevt.data%29%3Bp.start%28%29%7D%29";
		var worker1 = new SharedWorker(source, {name: 'named-shared-worker'});
		var worker2 = new SharedWorker(source, {name: 'named-shared-worker'});
		worker1.port.onmessage = function(event) { firstMessages.push(event.data); };
		worker2.port.addEventListener('message', function(event) { secondConnection = event.data; });
		worker1.port.start();
		worker2.port.start();
		worker1.port.postMessage('hello');
	`, "shared-worker-page.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.Tick()
		first, _ := r.Evaluate("firstMessages.join(',')")
		second, _ := r.Global("secondConnection")
		if first == "connected-1,echo:hello" && second.Str == "connected-2" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	first, _ := r.Evaluate("firstMessages.join(',')")
	second, _ := r.Global("secondConnection")
	state, _ := r.Evaluate("[typeof SharedWorker, typeof worker1, worker1.port.onmessage === null, firstMessages.join(','), secondConnection].join('|')")
	t.Fatalf("SharedWorker connect results = %q, %v (%s); want connected-1,echo:hello and connected-2", first, second, state)
}

func TestSharedWorkerIsSharedAcrossSameOriginRuntimes(t *testing.T) {
	hub := js.NewBroadcastHub()
	first, err := js.New(js.Options{URL: "https://example.com/first.html", BroadcastHub: hub})
	if err != nil {
		t.Fatalf("New first runtime: %v", err)
	}
	second, err := js.New(js.Options{URL: "https://example.com/second.html", BroadcastHub: hub})
	if err != nil {
		_ = first.Close()
		t.Fatalf("New second runtime: %v", err)
	}
	defer second.Close()
	sharedSource := "data:,self.addEventListener%28%27connect%27%2Ce%3D%3E%7Blet%20p%3De.ports%5B0%5D%3Bp.onmessage%3Dev%3D%3Ep.postMessage%28%27reply%3A%27%2Bev.data%29%3Bp.start%28%29%7D%29"
	for _, tc := range []struct {
		runtime *js.Runtime
		page    string
	}{
		{first, "first"},
		{second, "second"},
	} {
		source := "var shared = new SharedWorker('" + sharedSource + "', 'cross-runtime-shared'); shared.port.onmessage = e => { window.sharedReply = e.data; }; shared.port.start();"
		if err := tc.runtime.Run(source, tc.page+".js"); err != nil {
			t.Fatalf("Run %s page: %v", tc.page, err)
		}
	}
	_ = first.Close()
	if err := second.Run("shared.port.postMessage('from second')", "second-send.js"); err != nil {
		t.Fatalf("send from second runtime: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		second.Tick()
		if got, _ := second.Global("sharedReply"); got.Str == "reply:from second" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	got, _ := second.Global("sharedReply")
	t.Fatalf("cross-runtime SharedWorker reply = %v; want reply:from second", got)
}

func TestClosedWorkerCannotPostBroadcastChannelMessages(t *testing.T) {
	r, err := js.New(js.Options{
		URL: "https://example.com/page.html",
		WorkerScriptFetcher: func(string) ([]byte, error) {
			return []byte(`close();
new BroadcastChannel('closed-worker').postMessage('must not arrive');
postMessage(true);`), nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var broadcastReceived = false;
		var workerFinished = false;
		var channel = new BroadcastChannel('closed-worker');
		channel.onmessage = function() { broadcastReceived = true; };
		var worker = new Worker('worker.js');
		worker.onmessage = function() { workerFinished = true; };
	`, "page.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.Tick()
		if value, ok := r.Global("workerFinished"); ok && value.Bool {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if value, ok := r.Global("workerFinished"); !ok || !value.Bool {
		t.Fatal("closed worker did not report completion")
	}
	r.Tick()
	if value, ok := r.Global("broadcastReceived"); !ok || value.Bool {
		t.Fatal("BroadcastChannel message from closed worker was delivered")
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
