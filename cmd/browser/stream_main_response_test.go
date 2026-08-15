package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	fynetest "fyne.io/fyne/v2/test"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine/session"
	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/ui"
)

// stagedBodyReader delivers a head chunk, then blocks until released
// before delivering the body tail. It simulates a slow response whose
// <head> (with resource links) arrives well before the body tail.
type stagedBodyReader struct {
	mu      sync.Mutex
	head    string
	rest    string
	release <-chan struct{}
	phase   int // 0=head, 1=blocked, 2=rest, 3=EOF
}

func (r *stagedBodyReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	switch r.phase {
	case 0:
		r.phase = 1
		n := copy(p, r.head)
		r.mu.Unlock()
		return n, nil
	case 1:
		r.mu.Unlock()
		<-r.release
		r.mu.Lock()
		r.phase = 2
		n := copy(p, r.rest)
		r.mu.Unlock()
		return n, nil
	case 2:
		r.phase = 3
		return 0, io.EOF
	default:
		r.mu.Unlock()
		return 0, io.EOF
	}
}

// errAfterReader returns a prefix, then a non-EOF error (a truncated body).
type errAfterReader struct {
	prefix string
	err    error
	sent   bool
}

func (r *errAfterReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, r.prefix), nil
	}
	return 0, r.err
}

// newStreamTestBrowser builds the headless browser + session harness used
// by the coordinator streaming tests.
func newStreamTestBrowser(t *testing.T) (*ui.Browser, *session.Session, *goosienet.Service, *goosienet.Fetcher, *dom.Parser) {
	t.Helper()
	app := fynetest.NewApp()
	t.Cleanup(app.Quit)
	window := app.NewWindow("stream test")
	navSession := session.New()
	t.Cleanup(navSession.Close)
	networkService := goosienet.NewService(goosienet.ServiceOptions{Client: navSession.HTTPClient()})
	t.Cleanup(func() { _ = networkService.Close() })
	fetcher := goosienet.NewFetcherWithService(networkService)
	parser := dom.NewParser()
	browser := ui.NewBrowserWithDependencies(ui.BrowserDependencies{
		App:        app,
		Window:     window,
		Headless:   true,
		NavSession: navSession,
		Network:    networkService,
	})
	browser.RendererFactory = func() ui.HTMLRenderer {
		return renderer.NewRenderer(1000, 600)
	}
	return browser, navSession, networkService, fetcher, parser
}

// TestCoordinatorStreamDiscoversResourcesWhileBodyStreams is the PR12
// guard for streaming the main response: the discovery parse consumes the
// body as it downloads, so a <link rel=stylesheet> in the head is
// discovered — and its fetch starts — before the rest of the body has
// been delivered. (The previous path io.ReadAll'd the whole body before
// any discovery.)
func TestCoordinatorStreamDiscoversResourcesWhileBodyStreams(t *testing.T) {
	cssHit := make(chan struct{}, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/theme.css", func(w http.ResponseWriter, _ *http.Request) {
		select {
		case cssHit <- struct{}{}:
		default:
		}
		_, _ = w.Write([]byte("body { color: red; }"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	browser, navSession, networkService, fetcher, parser := newStreamTestBrowser(t)

	load, ctx := navSession.Navigate(context.Background(), srv.URL+"/")
	head := `<html><head><link rel="stylesheet" href="` + srv.URL + `/theme.css"></head><body>`
	rest := `<p>streamed body tail</p></body></html>`
	release := make(chan struct{})
	body := &stagedBodyReader{head: head, rest: rest, release: release}

	done := make(chan struct{})
	go func() {
		defer close(done)
		updateUIWithCoordinatorStream(ctx, browser, fetcher, navSession, load.ID,
			body, nil, goosienet.ResponseMeta{Status: 200}, false, load.URL,
			networkService, parser)
	}()

	// The stylesheet must be discovered — and fetched — while the body is
	// still staged: the CSS request arrives before we release the tail.
	select {
	case <-cssHit:
	case <-time.After(5 * time.Second):
		t.Fatal("stylesheet was not discovered while the body was still streaming")
	}
	close(release)

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("coordinator stream never completed")
	}
}

// TestCoordinatorStreamPropagatesBodyReadError is the PR12 guard for the
// read-error contract: a truncated body (non-EOF read error mid-stream)
// surfaces as a failed navigation instead of silently rendering a
// partial page.
func TestCoordinatorStreamPropagatesBodyReadError(t *testing.T) {
	browser, navSession, networkService, fetcher, parser := newStreamTestBrowser(t)

	load, ctx := navSession.Navigate(context.Background(), "https://example.com/")
	wantErr := errors.New("simulated body read failure")
	body := &errAfterReader{
		prefix: `<html><head><title>x</title></head><body>`,
		err:    wantErr,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		updateUIWithCoordinatorStream(ctx, browser, fetcher, navSession, load.ID,
			body, nil, goosienet.ResponseMeta{Status: 200}, false, load.URL,
			networkService, parser)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("coordinator stream never returned")
	}
	if got := navSession.State(); got != session.StateFailed {
		t.Fatalf("session state = %v, want failed after a body read error", got)
	}
	if !errors.Is(navSession.NavigationErr(), wantErr) {
		t.Fatalf("navigation err = %v, want %v", navSession.NavigationErr(), wantErr)
	}
}
