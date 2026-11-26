package layout_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

// qc:BR-328
func TestFitContentWidthUsesIntrinsicContent(t *testing.T) {
	arena := session(t, `<html><body style="margin:0;">
<div id="host" style="width:600px;">
  <div id="pill" style="width:fit-content;background:#fff;">
    <div id="child" style="width:120px;height:20px;"></div>
  </div>
</div>
</body></html>`, 800)
	pill := findByID(arena, "pill")
	child := findByID(arena, "child")
	if pill == nil || child == nil {
		t.Fatal("missing layout objects")
	}
	if !almostEqual(pill.W, 120) {
		t.Errorf("fit-content width = %v, want the 120px content width", pill.W)
	}
}

// qc:BR-328
func TestContentBoxHeightKeepsItsPadding(t *testing.T) {
	arena := session(t, `<html><body style="margin:0;">
<div id="banner" style="box-sizing:content-box;height:48px;padding:36px 0;background:#fff;"></div>
</body></html>`, 800)
	banner := findByID(arena, "banner")
	if banner == nil {
		t.Fatal("missing banner")
	}
	if !almostEqual(banner.H, 48) {
		t.Errorf("content-box height = %v, want 48", banner.H)
	}
	if !almostEqual(banner.H+banner.PaddingTop+banner.PaddingBottom, 120) {
		t.Errorf("banner border-box height = %v, want 120", banner.H+banner.PaddingTop+banner.PaddingBottom)
	}
}

// qc:BR-328
func TestStickyTopDoesNotMoveAnElementBelowItsThreshold(t *testing.T) {
	arena := session(t, `<html><body style="margin:0;">
<div id="parent" style="position:relative;height:300px;">
  <div style="height:100px;"></div>
  <div id="sticky" style="position:sticky;top:16px;height:20px;"></div>
</div>
</body></html>`, 800)
	sticky := findByID(arena, "sticky")
	if sticky == nil {
		t.Fatal("missing sticky element")
	}
	if !almostEqual(sticky.Y, 100) {
		t.Errorf("sticky Y = %v, want 100 with no scroll offset", sticky.Y)
	}
}

// qc:BR-328
func TestOutOfFlowInlineDoesNotCreateAnAnonymousLineBox(t *testing.T) {
	arena := session(t, `<html><body style="margin:0;">
  <svg id="sprite" style="position:absolute;width:0;height:0;overflow:hidden;"></svg>
  <div id="child" style="height:20px;background:#fff;"></div>
</body></html>`, 800)
	child := findByID(arena, "child")
	if child == nil {
		t.Fatal("missing child")
	}
	if !almostEqual(child.Y, 0) {
		t.Errorf("child Y = %v, want 0 without a hidden sprite line box", child.Y)
	}
}

// qc:BR-328
func TestGoogleJumpLinksFixturePaintsBanner(t *testing.T) {
	root := repoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "testdata/render/layout-features/13-google-jumplinks.html"))
	if err != nil {
		t.Fatal(err)
	}
	sess, err := engine.NewSession(string(src), nil, 1280)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatal(err)
	}
	pill := false
	for _, cmd := range list.All() {
		if cmd.Kind == paint.CmdFill && cmd.Color.Opaque() && cmd.Rect.Y0 >= 470 && cmd.Rect.Y0 <= 540 &&
			cmd.Rect.X0 >= 0 && cmd.Rect.X1 <= 1700 && cmd.Rect.X1-cmd.Rect.X0 >= 550 {
			pill = true
			break
		}
	}
	if !pill {
		t.Fatal("no wide white fill in the jump-links band")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
