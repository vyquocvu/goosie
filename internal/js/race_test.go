package js

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// raceMockFetcher mock for fetch testing
type raceMockFetcher struct {
	delay time.Duration
	body  string
	err   error
}

func (f *raceMockFetcher) Fetch(url string) (string, error) {
	time.Sleep(f.delay)
	return f.body, f.err
}

// M8.5: Repeated navigation during timers
func TestRace_RepeatedNavigationDuringTimers(t *testing.T) {
	session := NewSession(DefaultSessionConfig())
	go func() {
		_ = session.Run()
	}()
	defer session.Close()

	// Start timers
	err := session.Submit(func(rt *Runtime) {
		_, _ = rt.RunScript(`
			for (var i = 0; i < 20; i++) {
				setTimeout(function() {}, 10);
				setInterval(function() {}, 10);
			}
		`)
	})
	if err != nil {
		t.Fatalf("failed to submit script: %v", err)
	}

	// Repeatedly navigate to trigger runtime resets and timer cleanup
	for i := 0; i < 5; i++ {
		time.Sleep(5 * time.Millisecond)
		err = session.Submit(func(rt *Runtime) {
			session.Navigate()
		})
		if err != nil && !errors.Is(err, ErrSessionClosed) {
			t.Errorf("error during navigation: %v", err)
		}
	}
}

// M8.5: Fetch completion after navigation cancellation
func TestRace_FetchCompletionAfterNavigationCancellation(t *testing.T) {
	session := NewSession(DefaultSessionConfig())
	fetcher := &raceMockFetcher{delay: 20 * time.Millisecond, body: `{"status":"ok"}`}

	err := session.Submit(func(rt *Runtime) {
		rt.SetFetcher(fetcher)
	})
	if err != nil {
		t.Fatalf("failed to set fetcher: %v", err)
	}

	go func() {
		_ = session.Run()
	}()
	defer session.Close()

	// Submit fetch call
	err = session.Submit(func(rt *Runtime) {
		_, _ = rt.RunScript(`
			var success = false;
			fetch("http://example.com").then(function(res) {
				success = true;
			});
		`)
	})
	if err != nil {
		t.Fatalf("failed to submit fetch: %v", err)
	}

	// Cancel and navigate away immediately
	time.Sleep(5 * time.Millisecond)
	err = session.Submit(func(rt *Runtime) {
		session.Navigate()
	})
	if err != nil {
		t.Fatalf("failed to navigate: %v", err)
	}

	// Wait for the background fetch to complete
	time.Sleep(30 * time.Millisecond)
}

// M8.5: DOM mutation bursts
func TestRace_DOMMutationBursts(t *testing.T) {
	session := NewSession(DefaultSessionConfig())

	var mutationCount atomic.Int32
	err := session.Submit(func(rt *Runtime) {
		rt.SetDOMMutationCallback(func(mutation string) {
			mutationCount.Add(1)
		})
	})
	if err != nil {
		t.Fatalf("failed to set mutation handler: %v", err)
	}

	go func() {
		_ = session.Run()
	}()
	defer session.Close()

	// Execute massive burst of DOM mutations in JS
	err = session.Submit(func(rt *Runtime) {
		_, err := rt.RunScript(`
			// Create document structure
			document.body.innerHTML = '<div id="container"></div>';
			var container = document.getElementById("container");
			
			// Append many elements in a burst
			for (var i = 0; i < 100; i++) {
				var p = document.createElement("p");
				p.innerHTML = "item " + i;
				container.appendChild(p);
			}
		`)
		if err != nil {
			t.Errorf("script failed: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("failed to submit script: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	if mutationCount.Load() == 0 {
		t.Error("expected DOM mutations to fire")
	}
}

// M8.5: Tab close while script tasks are pending
func TestRace_TabCloseWithPendingTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSessionWithContext(ctx, DefaultSessionConfig())

	go func() {
		_ = session.Run()
	}()

	// Submit some slow/pending tasks
	for i := 0; i < 50; i++ {
		_ = session.Submit(func(rt *Runtime) {
			time.Sleep(time.Millisecond)
		})
	}

	// Simulate tab close by cancelling the context
	cancel()
	session.Close()

	time.Sleep(10 * time.Millisecond)
}

// M8.5: Script exceptions during event dispatch
func TestRace_ScriptExceptionsDuringEventDispatch(t *testing.T) {
	runtime := NewRuntime()
	runtime.SetHTMLContent(`<html><body><button id="btn">Click</button></body></html>`)

	// Add event listener that throws an exception, then execute it directly
	_, err := runtime.RunScript(`
		var btn = document.getElementById("btn");
		btn.addEventListener("click", function() {
			throw new Error("test click error");
		});
		
		var listeners = btn._listeners["click"];
		if (listeners && listeners.length > 0) {
			listeners[0]();
		}
	`)
	if err == nil {
		t.Fatal("expected script exception during event dispatch, got nil")
	}

	if !strings.Contains(err.Error(), "test click error") {
		t.Errorf("expected click error, got %v", err)
	}
}
