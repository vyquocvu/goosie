package main

import (
	"strings"
	"testing"
	"time"
)

func TestSequenceUnpaintedTailKeepsDocumentScrollRange(t *testing.T) {
	plain := strings.Replace(seqTallPage, ";background:rgb(250,250,250)", "", 1)
	for _, twin := range []struct {
		name string
		html string
	}{
		{name: "PLAIN", html: plain},
		{name: "PAINTED", html: seqTallPage},
	} {
		t.Run(twin.name, func(t *testing.T) {
			url := writeSeqPage(t, "tall.html", twin.html)
			_, rec, shots := captureSeqArgs(t, url, 2, time.Duration(seqIntervalMS)*time.Millisecond, "-seq-scroll-screens", "5")
			if len(shots) != 2 {
				t.Fatalf("wrote %d captures, want 2", len(shots))
			}
			if rec.ScrollScreens != 5 {
				t.Errorf("scroll_screens = %d, want 5", rec.ScrollScreens)
			}
			if rec.ScrollLanded == nil || !*rec.ScrollLanded {
				t.Fatalf("scroll_landed = %s, want true", landedText(rec.ScrollLanded))
			}
			for i, frame := range rec.Frames {
				if frame.ScrollY != 920 {
					t.Errorf("frame %d scroll_y = %d, want 920", i, frame.ScrollY)
				}
			}

			_, markerRec, markerShots := captureSeqArgs(t, url, 1, time.Duration(seqIntervalMS)*time.Millisecond, "-seq-scroll-screens", "1")
			if markerRec.ScrollLanded == nil || !*markerRec.ScrollLanded {
				t.Fatalf("marker scroll_landed = %s, want true", landedText(markerRec.ScrollLanded))
			}
			if len(markerRec.Frames) != 1 || markerRec.Frames[0].ScrollY != 360 {
				t.Fatalf("marker frames = %+v, want one at scroll_y 360", markerRec.Frames)
			}
			if len(markerShots) != 1 || inked(markerShots[0], seqBlue) == 0 {
				t.Fatal("the marker was not inked at its scroll position")
			}
		})
	}
}
