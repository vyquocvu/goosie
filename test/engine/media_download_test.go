package engine_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/media"
)

// failCode pumps until the controller reports failure and returns the code it
// failed with, which is the part a page can act on.
func failCode(t *testing.T, sess *engine.Session, c media.Controller) media.ErrorCode {
	t.Helper()
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StateFailed })
	var code media.ErrorCode
	var found bool
	for _, e := range c.Events() {
		if e.Kind == media.EvError {
			code, found = e.Code, true
		}
	}
	if !found {
		t.Fatal("the controller failed without emitting EvError")
	}
	return code
}

// loadOne selects a source and waits for the element to either reach a
// non-loading state or fail, returning the controller.
func loadOne(t *testing.T, sess *engine.Session, b *fakeBackend, f *fakeFetcher, href string) media.Controller {
	t.Helper()
	c := sess.MediaController(videoNode(t, sess.Doc))
	if c == nil {
		t.Fatal("no controller for the video element")
	}
	c.SetSource(href)
	c.Load()
	return c
}

// TestMediaSourceURLPolicy pins that a media source is judged by the same rule
// as every other subresource. A video is the one resource the browser hands to
// a native stack, so a URL that would have been refused for a stylesheet must
// not slip through here.
func TestMediaSourceURLPolicy(t *testing.T) {
	for _, tc := range []struct {
		name string
		href string
	}{
		{"a non-web scheme", "file:///etc/passwd"},
		{"a data URL", "data:video/mp4;base64,AAAA"},
		{"a downgrade out of a secure document", "http://example.com/clip.mp4"},
		{"a public page reaching a private address", "http://127.0.0.1/clip.mp4"},
		{"a link-local metadata endpoint", "http://169.254.169.254/clip.mp4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, f := newFakeBackend(), newFakeFetcher()
			sess := newMediaSession(t, videoHTML, b, f)
			c := loadOne(t, sess, b, f, tc.href)

			if got := failCode(t, sess, c); got != media.CodeUnsupported {
				t.Fatalf("failure code = %v, want CodeUnsupported", got)
			}
			if got := f.starts(); got != 0 {
				t.Fatalf("a refused source still reached the network %d times", got)
			}
			if got := b.openCount(); got != 0 {
				t.Fatalf("a refused source still opened %d players", got)
			}
		})
	}
}

// TestMediaResponseAdmission pins what a body has to look like to be handed to a
// decoder. Each case is a way a server can answer a media request with
// something that is not a playable file.
func TestMediaResponseAdmission(t *testing.T) {
	htmlPage := []byte("<!doctype html><title>404</title>not found")
	for _, tc := range []struct {
		name   string
		mutate func(*fakeFetcher)
	}{
		{"a partial answer", func(f *fakeFetcher) { f.status = 206 }},
		{"an error status", func(f *fakeFetcher) { f.status = 404 }},
		{"an empty body", func(f *fakeFetcher) { f.body = []byte{} }},
		{"an HTML error page", func(f *fakeFetcher) { f.body = htmlPage }},
		{"a truncated box header", func(f *fakeFetcher) { f.body = []byte{0, 0, 0} }},
		{"a box longer than the file", func(f *fakeFetcher) {
			f.body = []byte{0x00, 0x00, 0xff, 0xff, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
		}},
		{"a redirect to a private address", func(f *fakeFetcher) { f.finalURL = "http://127.0.0.1/clip.mp4" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, f := newFakeBackend(), newFakeFetcher()
			tc.mutate(f)
			sess := newMediaSession(t, videoHTML, b, f)
			c := loadOne(t, sess, b, f, "clip.mp4")

			if got := failCode(t, sess, c); got != media.CodeUnsupported {
				t.Fatalf("failure code = %v, want CodeUnsupported", got)
			}
			if got := b.openCount(); got != 0 {
				t.Fatalf("an inadmissible body still opened %d players", got)
			}
			if got := f.starts(); got != 1 {
				t.Fatalf("fetcher ran %d times, want 1", got)
			}
		})
	}
}

// TestMediaOversizedBodyIsRefused is kept out of the table above because it
// allocates the limit rather than a fixture.
func TestMediaOversizedBodyIsRefused(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	f.body = make([]byte, engine.MaxMediaBytes+1)
	copy(f.body, mp4Header)
	f.body[0], f.body[1], f.body[2], f.body[3] = 0, 0, 0, 20
	sess := newMediaSession(t, videoHTML, b, f)
	c := loadOne(t, sess, b, f, "clip.mp4")

	if got := failCode(t, sess, c); got != media.CodeUnsupported {
		t.Fatalf("failure code = %v, want CodeUnsupported", got)
	}
	if got := b.openCount(); got != 0 {
		t.Fatalf("an oversized body still opened %d players", got)
	}
}

// TestMediaTempFilePermissions pins that the handoff file is readable only by
// this process. A downloaded video can be anything the user was entitled to
// see, and a world-readable temporary directory makes it everyone's.
func TestMediaTempFilePermissions(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	c := loadOne(t, sess, b, f, "clip.mp4")
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })

	path := b.path(0)
	if path == "" {
		t.Fatal("the backend was never handed a path")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("the media file is mode %o, want 0600", got)
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat the media directory: %v", err)
	}
	if got := dir.Mode().Perm(); got&0o077 != 0 {
		t.Fatalf("the media directory is mode %o, which is readable by someone else", got)
	}
	if !dir.IsDir() {
		t.Fatal("the media file was not written into its own directory")
	}
}

// TestMediaOpensOnlyAfterTheWholeBodyArrives pins the ordering the download-first
// design rests on: the decoder is not pointed at a file until the fetch has
// completed and the file is closed.
func TestMediaOpensOnlyAfterTheWholeBodyArrives(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	f.gate = make(chan struct{})
	sess := newMediaSession(t, videoHTML, b, f)
	c := loadOne(t, sess, b, f, "clip.mp4")

	pumpUntil(t, sess, func() bool { return f.starts() == 1 })
	for i := 0; i < 20; i++ {
		sess.PumpMedia()
		time.Sleep(time.Millisecond)
	}
	if got := b.openCount(); got != 0 {
		t.Fatalf("the backend was handed a file while %d bytes were still arriving", got)
	}
	if got := c.Snapshot().State; got != media.StateLoading {
		t.Fatalf("state during the download = %v, want StateLoading", got)
	}

	close(f.gate)
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	if got := b.openCount(); got != 1 {
		t.Fatalf("after the body arrived the backend opened %d players, want 1", got)
	}
	if !exists(b.path(0)) {
		t.Fatal("the file the backend was pointed at does not exist")
	}
}

// TestMediaDownloadConcurrencyIsBounded pins that a page naming many sources
// cannot open many sockets at once. The third request fails fast rather than
// queueing behind the display, which is what keeps a hostile page from turning
// a video element into a connection amplifier.
func TestMediaDownloadConcurrencyIsBounded(t *testing.T) {
	const html = `<html><body><video id="a" controls></video><video id="b" controls></video><video id="c" controls></video></body></html>`
	b, f := newFakeBackend(), newFakeFetcher()
	f.gate = make(chan struct{})
	sess := newMediaSession(t, html, b, f)
	nodes := sess.Doc.ElementsByTagName("video")
	if len(nodes) != 3 {
		t.Fatalf("the fixture has %d video elements, want 3", len(nodes))
	}
	var controllers []media.Controller
	for _, n := range nodes {
		c := sess.MediaController(n)
		if c == nil {
			t.Fatal("no controller for a video element")
		}
		c.SetSource("clip.mp4")
		c.Load()
		controllers = append(controllers, c)
	}

	pumpUntil(t, sess, func() bool {
		failed := 0
		for _, c := range controllers {
			if c.Snapshot().State == media.StateFailed {
				failed++
			}
		}
		return failed == 1
	})
	loading := 0
	for _, c := range controllers {
		if got := c.Snapshot().State; got == media.StateLoading {
			loading++
		}
	}
	if loading != 2 {
		t.Fatalf("%d elements are still loading, want exactly the 2 the cap allows", loading)
	}

	close(f.gate)
	pumpUntil(t, sess, func() bool { return b.openCount() == 2 })
	if got := b.openCount(); got != 2 {
		t.Fatalf("after the gate opened, %d players exist, want 2", got)
	}
}

// TestMediaFilesAreGoneAfterClose pins the whole-directory cleanup: a session
// that goes away leaves nothing behind, including files whose leases were never
// individually released.
func TestMediaFilesAreGoneAfterClose(t *testing.T) {
	b, f := newFakeBackend(), newFakeFetcher()
	sess := newMediaSession(t, videoHTML, b, f)
	c := loadOne(t, sess, b, f, "clip.mp4")
	pumpUntil(t, sess, func() bool { return c.Snapshot().State == media.StatePreparing })
	dir := filepath.Dir(b.path(0))
	if !exists(dir) {
		t.Fatal("the media directory does not exist while the video is loaded")
	}

	sess.Close()
	if exists(dir) {
		left, _ := os.ReadDir(dir)
		t.Fatalf("Session.Close left the media directory at %s with %d entries", dir, len(left))
	}
}
