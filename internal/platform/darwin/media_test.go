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

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", "testdata", "media", name)
	if _, err := os.Stat(p); err != nil {
		t.Skipf("the fixture %s is missing (%v)", name, err)
	}
	return p
}

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

func TestMediaBackendDecodesRealFrames(t *testing.T) {
	p, err := mediaBackend{}.Open(fixturePath(t, "clip.mp4"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer p.Close()

	snap := waitFor(t, "metadata", p, func(s media.Snapshot) bool {
		return !media.UnknownDuration(s.Duration) && s.Width > 0
	})
	if snap.Width != 320 || snap.Height != 180 {
		t.Fatalf("decoded size = %dx%d, want 320x180", snap.Width, snap.Height)
	}
	if snap.Duration < 1.5 || snap.Duration > 2.5 {
		t.Fatalf("duration = %v, want about 2s", snap.Duration)
	}

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

	held := make([]byte, len(img.Pix))
	copy(held, img.Pix)

	const playOp uint64 = 7
	p.Play(playOp)
	playing := waitFor(t, "playback", p, func(s media.Snapshot) bool {
		return s.State == media.StatePlaying && s.DoneOp == playOp
	})
	if playing.DoneOp != playOp {
		t.Fatalf("playback settled op %d, want %d", playing.DoneOp, playOp)
	}

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

	p.SetMuted(true)
	if got := waitFor(t, "mute", p, func(s media.Snapshot) bool { return s.Muted }).Muted; !got {
		t.Fatal("SetMuted(true) did not reach the state")
	}
	p.SetMuted(false)
	waitFor(t, "unmute", p, func(s media.Snapshot) bool { return !s.Muted })

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
