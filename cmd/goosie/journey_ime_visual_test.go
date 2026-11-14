package main

import (
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/surface"
)

type inkRegion struct {
	dark, columns int
	x0, x1        int
}

func frameInk(t *testing.T, path string, x0, y0, x1, y1 int) inkRegion {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open presented frame: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode presented frame: %v", err)
	}
	out := inkRegion{x0: 1 << 30, x1: -1}
	seen := map[int]bool{}
	for y := y0; y < y1 && y < img.Bounds().Max.Y; y++ {
		for x := x0; x < x1 && x < img.Bounds().Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 < 110 && g>>8 < 110 && b>>8 < 110 {
				out.dark++
				seen[x] = true
				if x < out.x0 {
					out.x0 = x
				}
				if x > out.x1 {
					out.x1 = x
				}
			}
		}
	}
	out.columns = len(seen)
	if out.columns == 0 {
		out.x0, out.x1 = 0, 0
	}
	return out
}

const imeFixtureFile = "testdata/br111-ime-preview.html"

// qc:BR-111
func TestJourneyIMECompositionInksThePreviewOnce(t *testing.T) {
	fixture, err := os.ReadFile(imeFixtureFile)
	if err != nil {
		t.Fatalf("read the shared BR-111 fixture: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, string(fixture))
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)
	pageURL := server.URL + "/ime"
	browser.navigateViaAddressBar(t, pageURL)
	browser.waitFor(t, "the fixture document is loaded", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "i")
		return browser.activeURL() == pageURL && ok
	})

	tree := browser.window.AccessibilityTree()
	input, okI := findAXNode(tree, "i")
	if !okI {
		t.Fatalf("no accessibility node for #i; %s", browser.diagnostics())
	}
	editor := ax.Node{
		X0: input.X0, Y0: input.Y0 + (70 - 20),
		X1: input.X0 + 220, Y1: input.Y0 + (70 - 20) + 44,
	}
	snap := filepath.Join(t.TempDir(), "frame.png")
	shoot := func() {
		t.Helper()
		browser.waitFor(t, "a presented frame with tiles idle", func() bool {
			last := browser.f.loop.Stats().Last
			return browser.window.Stats().Presents > 0 && last.Needed == 0 && last.PaintingBytes == 0
		})
		if err := browser.window.WritePNG(snap); err != nil {
			t.Fatalf("write presented frame: %v", err)
		}
	}
	measure := func(x0, y0, x1, y1 int) inkRegion {
		return frameInk(t, snap, x0+2, y0+2, x1-1, y1-1)
	}

	shoot()
	if got := measure(int(input.X0), int(input.Y0), int(input.X1), int(input.Y1)); got.dark != 0 {
		t.Fatalf("an untouched input inks %d dark pixels, want 0", got.dark)
	}
	if got := measure(int(editor.X0), int(editor.Y0), int(editor.X1), int(editor.Y1)); got.dark != 0 {
		t.Fatalf("an untouched contenteditable inks %d dark pixels, want 0", got.dark)
	}

	drive := func(box ax.Node, steps []string, commit string) (marked, committed inkRegion) {
		browser.click(t, int32((box.X0+box.X1)/2), int32((box.Y0+box.Y1)/2))
		ink := func() inkRegion {
			return measure(int(box.X0), int(box.Y0), int(box.X1), int(box.Y1))
		}
		for _, st := range steps {
			browser.inject(t, surface.Event{Kind: surface.EvIME, IME: surface.IMEMarked, Text: st})
		}
		browser.waitFor(t, "the marked passage reaches the control", func() bool {
			sess := browser.f.tabMgr.Active().Session()
			return sess != nil && sess.Marked() == steps[len(steps)-1]
		})
		browser.waitFor(t, "the open composition is inked", func() bool {
			if err := browser.window.WritePNG(snap); err != nil {
				return false
			}
			return ink().dark > 0
		})
		shoot()
		marked = ink()
		browser.inject(t, surface.Event{Kind: surface.EvIME, IME: surface.IMECommit, Text: commit})
		browser.waitFor(t, "the commit leaves no marked passage", func() bool {
			sess := browser.f.tabMgr.Active().Session()
			return sess != nil && sess.Marked() == ""
		})
		shoot()
		committed = ink()
		return marked, committed
	}

	steps := []string{"ti", "tie", "tiế", "tiếng"}
	iMarked, iCommitted := drive(input, steps, "tiếng")
	cMarked, cCommitted := drive(editor, steps, "tiếng")
	t.Logf("measured goosie ink  #i marked %+v committed %+v | #c marked %+v committed %+v",
		iMarked, iCommitted, cMarked, cCommitted)

	for _, c := range []struct {
		what             string
		marked, commited inkRegion
		chromiumColumns  int
		maxSpan          int
	}{
		{"the text control", iMarked, iCommitted, 27, 45},
		{"the contenteditable", cMarked, cCommitted, 26, 45},
	} {
		if c.marked.dark == 0 {
			t.Errorf("%s inked nothing while the composition was open", c.what)
			continue
		}
		if c.marked.x0 != c.commited.x0 || c.marked.x1 != c.commited.x1 {
			t.Errorf("%s inks x=%d..%d while composing but x=%d..%d after the commit: the preview is not the same word the commit writes",
				c.what, c.marked.x0, c.marked.x1, c.commited.x0, c.commited.x1)
		}
		if c.marked.columns != c.commited.columns {
			t.Errorf("%s inks %d columns while composing and %d after the commit, want the same word once",
				c.what, c.marked.columns, c.commited.columns)
		}
		if span := c.marked.x1 - c.marked.x0 + 1; span > c.maxSpan {
			t.Errorf("%s composition ink spans %d px, more than one copy of %q can reach (%d ink columns in Chromium, %d here)",
				c.what, span, "tiếng", c.chromiumColumns, c.marked.columns)
		}
		if c.marked.columns < c.chromiumColumns/2 {
			t.Errorf("%s composition inked %d columns, want a whole word (Chromium inked %d)",
				c.what, c.marked.columns, c.chromiumColumns)
		}
	}
}
