package paint

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
)

// FindMark is one find-in-page highlight rectangle in document CSS pixels.
// Obj is the text box whose painting emits the fill: emitting it there puts
// the highlight underneath that run's glyphs, before they are painted.
type FindMark struct {
	Obj    *layout.Object
	Rect   frame.RectF
	Active bool
}

// paintFindHighlights fills the match rects that belong to obj's run. The
// inactive color is the browser-standard yellow background; the active match
// uses a darker orange so the focused hit stands out.
func (b *Builder) paintFindHighlights(marks []FindMark, opacity float32, clip *frame.Rect) {
	for _, m := range marks {
		r := m.Rect.ToDevice(b.scale)
		if clip != nil {
			r = r.Intersection(*clip)
		}
		if r.Empty() {
			continue
		}
		color := frame.RGB(255, 235, 59)
		if m.Active {
			color = frame.RGB(255, 152, 0)
		}
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: r, Color: color, Opacity: opacity})
	}
}
