//go:build darwin && cgo

package darwin

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/media"
)

// These are the only tests in the repository that prove anything was actually
// decoded. Everything above this package runs against a scripted fake, which
// pins the state machine and the ownership rules but would pass just as happily
// against a backend that returned a black rectangle forever.
//
// They need no display and no window: AVPlayer owns the audio and the clock,
// the video output hands pixels over instead of drawing them, and the player's
// work happens on its own dispatch queue. A headless CI runner can therefore
// run them, and a machine with no AVFoundation at all does not compile this
// file.

// fixturePath resolves a generated asset under testdata/media. The assets are
// produced by the ffmpeg step documented in that directory rather than checked
// in as opaque binaries, so a missing one is a skip with the command to run
// rather than a failure that says nothing about the backend.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", "testdata", "media", name)
	if _, err := os.Stat(p); err != nil {
		t.Skipf("the fixture %s is missing (%v)", name, err)
	}
	return p
}

// waitFor polls until want holds or the deadline passes, and fails rather than
// hanging. A native decode is asynchronous, so every assertion here is a wait
// rather than a read.
func waitFor(t *testing.T, what string, p media.Player, want func(media.Snapshot) bool) media.Snapshot {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var last media.Snapshot
	for time.Now().Before(deadline) {
		last = p.Poll()
		if want(last) {
			return last
		}
		if last.State == media.StateFailed {
			t.Fatalf("%s: the asset failed: %v", what, last.Err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s: never happened; last state %+v", what, last)
	return last
}

// pixelDiff is the largest per-channel difference between two same-sized RGBA
// frames. A moving test pattern is far above zero between two distinct frames
// and exactly zero for a frame compared with itself, which is what makes
// "the video advanced" an assertion rather than a hope.
func pixelDiff(a, b image.Image) int {
	ar, ok1 := a.(*image.RGBA)
	br, ok2 := b.(*image.RGBA)
	if !ok1 || !ok2 || ar.Rect != br.Rect {
		return -1
	}
	max := 0
	for i := 0; i < len(ar.Pix) && i < len(br.Pix); i++ {
		d := int(ar.Pix[i]) - int(br.Pix[i])
		if d < 0 {
			d = -d
		}
		if d > max {
			max = d
		}
	}
	return max
}

func TestMediaBackendReportsItsOwnName(t *testing.T) {
	if got := (mediaBackend{}).Name(); got != "avfoundation" {
		t.Fatalf("Name() = %q, want the framework rather than the platform", got)
	}
}

func TestMediaBackendRefusesAMissingPath(t *testing.T) {
	_, err := mediaBackend{}.Open("")
	if err == nil {
		t.Fatal("Open(\"\") succeeded")
	}
	if !isUnsupported(err) {
		t.Fatalf("Open(\"\") = %v, want ErrUnsupported", err)
	}
}

// TestMediaBackendRefusesAMissingFile pins that an absent asset is reported
// rather than played as silence. Creating an AVURLAsset does not touch the file,
// so Open succeeding is correct: the refusal arrives when the metadata load
// fails, which is the moment anything actually tried to read it.
func TestMediaBackendRefusesAMissingFile(t *testing.T) {
	p, err := mediaBackend{}.Open(filepath.Join(t.TempDir(), "absent.mp4"))
	if err != nil {
		if !isUnsupported(err) {
			t.Fatalf("Open(absent) = %v, want ErrUnsupported", err)
		}
		return
	}
	defer p.Close()
	snap := waitFor(t, "the absent file was refused", p, func(s media.Snapshot) bool {
		return s.State == media.StateFailed
	})
	if !isUnsupported(snap.Err) {
		t.Fatalf("the absent file failed with %v, want ErrUnsupported", snap.Err)
	}
}

// TestMediaBackendRefusesGarbageThatLooksLikeMP4 pins the container check. An
// ftyp box followed by nonsense is what a truncated download or a server error
// page saved with the wrong extension looks like, and the honest answer is
// "unsupported" rather than a player that never produces a frame.
func TestMediaBackendRefusesGarbageThatLooksLikeMP4(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garbage.mp4")
	body := make([]byte, 512)
	copy(body, []byte{0x00, 0x00, 0x00, 0x14, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm',
		0x00, 0x00, 0x02, 0x00, 'i', 's', 'o', 'm'})
	for i := 20; i < len(body); i++ {
		body[i] = byte(i * 7)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write the fixture: %v", err)
	}
	p, err := mediaBackend{}.Open(path)
	if err != nil {
		// Refusing at open is as good as refusing at prepare.
		if !isUnsupported(err) {
			t.Fatalf("Open(garbage) = %v, want ErrUnsupported", err)
		}
		return
	}
	defer p.Close()
	waitFor(t, "the garbage asset was refused", p, func(s media.Snapshot) bool {
		return s.State == media.StateFailed
	})
}

// TestMediaBackendRefusesAnAssetOverTheLimits pins that the caps are enforced
// where the pixels are measured rather than trusted from the file name. The
// fixture is 1922x1080, which is one pixel over the width cap and over the area
// cap as well.
func TestMediaBackendRefusesAnAssetOverTheLimits(t *testing.T) {
	p, err := mediaBackend{}.Open(fixturePath(t, "toobig.mp4"))
	if err != nil {
		if !isUnsupported(err) {
			t.Fatalf("Open(toobig) = %v, want ErrUnsupported", err)
		}
		return
	}
	defer p.Close()
	snap := waitFor(t, "the oversize asset was refused", p, func(s media.Snapshot) bool {
		return s.State == media.StateFailed
	})
	if !isUnsupported(snap.Err) {
		t.Fatalf("the oversize asset failed with %v, want ErrUnsupported", snap.Err)
	}
}

// TestMediaBackendDecodesRealFrames is the end-to-end proof: metadata, a frame
// before playback, playback that advances, a seek that lands where it was asked,
// mute, and a teardown that can be called twice.
func TestMediaBackendDecodesRealFrames(t *testing.T) {
	p, err := mediaBackend{}.Open(fixturePath(t, "clip.mp4"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p.Close()

	// Metadata arrives asynchronously; nothing about the asset is known until it
	// does, and a duration of NaN before that point is the contract the engine
	// and the JS binding both rely on.
	snap := waitFor(t, "metadata", p, func(s media.Snapshot) bool {
		return !media.UnknownDuration(s.Duration) && s.Width > 0
	})
	if snap.Width != 320 || snap.Height != 180 {
		t.Fatalf("decoded size = %dx%d, want 320x180", snap.Width, snap.Height)
	}
	if snap.Duration < 1.5 || snap.Duration > 2.5 {
		t.Fatalf("duration = %v, want about 2s", snap.Duration)
	}

	// A frame must be available before play: a paused video with nothing drawn
	// is a black box the user cannot tell from a broken one.
	first := waitFor(t, "a frame before playback", p, func(s media.Snapshot) bool {
		return s.Pixels != nil
	})
	img, ok := first.Pixels.(*image.RGBA)
	if !ok {
		t.Fatalf("the frame is a %T, want *image.RGBA", first.Pixels)
	}
	if img.Bounds().Dx() != 320 || img.Bounds().Dy() != 180 {
		t.Fatalf("the frame is %v, want 320x180", img.Bounds())
	}
	if !opaqueEnough(img) {
		t.Fatal("the decoded frame is transparent: the premultiplied RGBA conversion dropped the alpha")
	}

	// Frames are immutable and independently owned, because they go straight
	// into a frozen display list.
	held := make([]byte, len(img.Pix))
	copy(held, img.Pix)

	// Playback. The op is what ties the completion back to the request, so a
	// stale play cannot settle a newer one.
	const playOp uint64 = 7
	p.Play(playOp)
	playing := waitFor(t, "playback", p, func(s media.Snapshot) bool {
		return s.State == media.StatePlaying && s.DoneOp == playOp
	})
	if playing.DoneOp != playOp {
		t.Fatalf("playback settled op %d, want %d", playing.DoneOp, playOp)
	}

	// The video has to actually move, and the frame already handed over must not
	// change underneath the caller.
	var moved bool
	var lastPlaying image.Image
	lastSeq := first.Seq
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !moved {
		s := p.Poll()
		if s.Pixels != nil {
			lastPlaying, lastSeq = s.Pixels, s.Seq
			if s.Seq > first.Seq {
				moved = pixelDiff(first.Pixels, s.Pixels) > 0
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !moved {
		t.Fatal("playback produced no frame that differed from the first one")
	}
	for i := range held {
		if held[i] != img.Pix[i] {
			t.Fatalf("byte %d of an already-delivered frame changed: the buffer was reused", i)
		}
	}
	if got := waitFor(t, "the position to advance", p, func(s media.Snapshot) bool {
		return s.Position > 0.05
	}).Position; got <= 0.05 {
		t.Fatalf("position = %v after playback, want it to have advanced", got)
	}

	// Pause. A paused output is not expected to keep serving frames — nothing is
	// advancing, so there is no new frame to serve, and the one already delivered
	// is what stays on screen. What a paused user does next is seek, and that has
	// to produce the frame at the new position rather than the old one.
	p.Pause()
	waitFor(t, "the pause", p, func(s media.Snapshot) bool { return s.State == media.StatePaused })

	const seekOp uint64 = 11
	p.Seek(seekOp, 1.5)
	seeked := waitFor(t, "the seek to settle", p, func(s media.Snapshot) bool {
		return s.DoneOp == seekOp
	})
	if seeked.Position < 1.3 || seeked.Position > 1.7 {
		t.Fatalf("position after seeking to 1.5 = %v", seeked.Position)
	}
	after := waitFor(t, "a frame at the seek target", p, func(s media.Snapshot) bool {
		return s.Pixels != nil && s.Seq > lastSeq
	})
	if lastPlaying != nil && pixelDiff(lastPlaying, after.Pixels) <= 0 {
		t.Fatal("the seek produced the frame that was already showing: a paused seek did not decode its new position")
	}

	// A stale seek must not settle a newer one.
	p.Seek(12, 0.2)
	p.Seek(13, 1.0)
	settled := map[uint64]bool{}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(settled) == 0 {
		if s := p.Poll(); s.DoneOp != 0 {
			settled[s.DoneOp] = true
		}
		time.Sleep(10 * time.Millisecond)
	}
	if settled[12] {
		t.Fatal("a superseded seek reported completion")
	}

	// Mute is observable in the state, because the JS binding reads it from
	// there rather than keeping its own idea.
	p.SetMuted(true)
	if got := waitFor(t, "mute", p, func(s media.Snapshot) bool { return s.Muted }).Muted; !got {
		t.Fatal("SetMuted(true) did not reach the state")
	}
	p.SetMuted(false)
	waitFor(t, "unmute", p, func(s media.Snapshot) bool { return !s.Muted })

	// Replaying from the end is a seek to zero, which is what the DOM says
	// happens and what the engine does.
	p.Seek(14, 0)
	waitFor(t, "the rewind", p, func(s media.Snapshot) bool { return s.DoneOp == 14 })
	p.Play(15)
	waitFor(t, "replay", p, func(s media.Snapshot) bool {
		return s.State == media.StatePlaying && s.DoneOp == 15
	})

	p.Close()
	p.Close()
	if got := p.Poll(); got.State != media.StateClosed {
		t.Fatalf("Poll after Close = %v, want StateClosed", got.State)
	}
	p.Play(99)
	p.Seek(99, 1)
	p.SetMuted(true)
}

// TestMediaBackendPlaysAVideoWithNoAudioTrack pins that the audio track is
// optional: a video-only file is the common case for generated assets and for
// muted autoplay, and refusing it would refuse most of the web's short clips.
func TestMediaBackendPlaysAVideoWithNoAudioTrack(t *testing.T) {
	p, err := mediaBackend{}.Open(fixturePath(t, "silent.mp4"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p.Close()

	waitFor(t, "metadata", p, func(s media.Snapshot) bool { return !media.UnknownDuration(s.Duration) })
	p.Play(1)
	waitFor(t, "playback of a silent asset", p, func(s media.Snapshot) bool {
		return s.State == media.StatePlaying && s.DoneOp == 1
	})
	waitFor(t, "a frame from the silent asset", p, func(s media.Snapshot) bool { return s.Pixels != nil })
}

// TestMediaBackendClosesWhilePlaying pins the teardown a user actually performs:
// closing a tab mid-playback. Audio has to stop, and the close must not hang
// waiting on a decoder that is mid-frame.
func TestMediaBackendClosesWhilePlaying(t *testing.T) {
	p, err := mediaBackend{}.Open(fixturePath(t, "clip.mp4"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	waitFor(t, "metadata", p, func(s media.Snapshot) bool { return !media.UnknownDuration(s.Duration) })
	p.Play(1)
	waitFor(t, "playback", p, func(s media.Snapshot) bool { return s.State == media.StatePlaying })

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Close()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Close blocked on a playing asset")
	}
}

// opaqueEnough reports whether a decoded frame has visible content. The test
// pattern is bright and busy, so a frame that came through the BGRA conversion
// with its alpha zeroed would fail here — which is the mistake worth catching,
// because a fully transparent frame composites to nothing and looks exactly like
// a video that never started.
func opaqueEnough(img *image.RGBA) bool {
	opaque := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 0 {
			opaque++
		}
	}
	return opaque*4 >= len(img.Pix)/4
}

func isUnsupported(err error) bool { return errors.Is(err, media.ErrUnsupported) }

// TestMain pins the main goroutine to the process main thread and services that
// thread's run loop for as long as the tests run.
//
// This is not a convenience. AVPlayer prepares its item only on the main thread:
// a worker thread with its own CFRunLoop turns the loop faithfully and the item's
// status still never leaves Unknown, with no KVO notification and no error to
// explain it. The browser gets the main run loop from GoosieAppRun, which services
// AppKit with bounded waits; a test binary has nothing of the kind, so without
// this every playback test below would be exercising a backend that cannot play.
func TestMain(m *testing.M) {
	runtime.LockOSThread()
	code := 1
	done := make(chan struct{})
	go func() {
		defer close(done)
		code = m.Run()
	}()
	for {
		select {
		case <-done:
			os.Exit(code)
		default:
		}
		PumpMainRunLoopForTest(20 * time.Millisecond)
	}
}
