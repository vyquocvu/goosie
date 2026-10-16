package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

// The background shorthand lets position and size ride along with the image and
// colour in one declaration. A layer that only sets them through `background`
// must still reach those fields, or the image lands at its default top-left
// origin instead of where the author asked.
func TestBackgroundShorthandPositionAndSize(t *testing.T) {
	s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
		`body { font: 20px/1.5 sans-serif } .x { background: rgba(0,136,238,.3) url("invU.png") bottom repeat-x }`, "div")
	if s.BackgroundImage != "invU.png" {
		t.Errorf("BackgroundImage = %q, want invU.png", s.BackgroundImage)
	}
	if s.BackgroundRepeat != style.BgRepeatRepeatX {
		t.Errorf("BackgroundRepeat = %v, want repeat-x", s.BackgroundRepeat)
	}
	if s.BackgroundPosYMode != style.BgPosEnd {
		t.Errorf("BackgroundPosYMode = %v, want end (bottom)", s.BackgroundPosYMode)
	}
	if s.BackgroundColor.A != 77 { // round(0.3 * 255)
		t.Errorf("background colour alpha = %d, want 77", s.BackgroundColor.A)
	}
}

func TestBackgroundShorthandPositionSizeSlash(t *testing.T) {
	s := styleFor(t, `<html><body><div class="x">x</div></body></html>`,
		`.x { background: url("a.png") center / cover no-repeat }`, "div")
	if s.BackgroundSize != style.BgSizeCover {
		t.Errorf("BackgroundSize = %v, want cover", s.BackgroundSize)
	}
	if s.BackgroundPosXMode != style.BgPosCenter || s.BackgroundPosYMode != style.BgPosCenter {
		t.Errorf("position = (%v,%v), want center/center", s.BackgroundPosXMode, s.BackgroundPosYMode)
	}
	if s.BackgroundRepeat != style.BgRepeatNoRepeat {
		t.Errorf("BackgroundRepeat = %v, want no-repeat", s.BackgroundRepeat)
	}
}
