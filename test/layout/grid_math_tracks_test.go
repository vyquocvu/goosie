package layout_test

import "testing"

func TestGridTracksResolveViewportMathAndNamedLines(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div id="grid" style="display: grid; width: 1280px; grid-template-columns: [extended-full-start] max(1rem, calc(50vw - 720px + 1rem)) [full-start] 1fr [content-start] minmax(0, 48rem) [content-end] 1fr [full-end] max(1rem, calc(50vw - 720px + 1rem)) [extended-full-end];">
  <div id="main" style="grid-column: content-start / extended-full-end; height: 40px;"></div>
</div>
</body></html>`, 1280)

	main := findByID(arena, "main")
	if main == nil {
		t.Fatal("missing grid item")
	}
	if !almostEqual(main.X, 256) || !almostEqual(main.W, 1024) {
		t.Errorf("main = {X:%v W:%v}, want {X:256 W:1024} from viewport-math grid tracks", main.X, main.W)
	}
}
