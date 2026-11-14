package paint

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
)

type FindMark struct {
	Key    layout.BoxKey
	Rect   frame.RectF
	Active bool
}

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
