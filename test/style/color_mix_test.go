package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestStyle_ColorMix(t *testing.T) {
	html := `<!DOCTYPE html><html><body><div id="target" class="box">Text</div></body></html>`
	doc := domtest.Parse(html)
	cssText := `
    .box {
      color: color-mix(in srgb, red 40%, blue);
      background-color: color-mix(in srgb, white, black);
      border-top-color: color-mix(in srgb, red, transparent);
    }
	`
	sheet := css.Parse(cssText)
	vp := style.Viewport{W: 800, H: 600}
	styles := style.ResolveViewport(doc, []*css.Stylesheet{sheet}, vp, nil)

	el := doc.ElementByID("target")
	if el == nil {
		t.Fatal("target element not found")
	}

	cs, ok := styles[el.ID]
	if !ok {
		t.Fatal("computed style for target not found")
	}

	// 40% red + 60% blue in srgb -> (102, 0, 153, 255)
	if cs.Color.R != 102 || cs.Color.G != 0 || cs.Color.B != 153 || cs.Color.A != 255 {
		t.Errorf("computed Color = %+v, want (102, 0, 153, 255)", cs.Color)
	}

	// 50% white + 50% black in srgb -> (128, 128, 128, 255)
	if cs.BackgroundColor.R != 128 || cs.BackgroundColor.G != 128 || cs.BackgroundColor.B != 128 || cs.BackgroundColor.A != 255 {
		t.Errorf("computed BackgroundColor = %+v, want (128, 128, 128, 255)", cs.BackgroundColor)
	}

	// 50% red + 50% transparent -> (255, 0, 0, 128)
	if cs.BorderTopColor.R != 255 || cs.BorderTopColor.G != 0 || cs.BorderTopColor.B != 0 || cs.BorderTopColor.A != 128 {
		t.Errorf("computed BorderTopColor = %+v, want (255, 0, 0, 128)", cs.BorderTopColor)
	}
}

// Suppress unused css import warning
var _ = css.Color{}
