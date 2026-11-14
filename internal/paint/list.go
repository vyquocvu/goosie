package paint

import (
	"image"
	"reflect"
	"slices"

	"github.com/vyquocvu/goosie/internal/frame"
)

type List struct {
	cmds    []DisplayCmd
	bounds  frame.Rect
	pending []pendingGroup
}

type Group struct {
	OwnOpacity float32
	Bounds     frame.Rect
	Start      int
	End        int
}

type pendingGroup struct {
	own         float32
	first, last int32
}

func NewList(n int) *List {
	if n < 0 {
		n = 0
	}
	return &List{cmds: make([]DisplayCmd, 0, n)}
}

func (l *List) Reset() {
	for i := range l.cmds {
		l.cmds[i] = DisplayCmd{}
	}
	l.cmds = l.cmds[:0]
	l.bounds = frame.Rect{}
	l.pending = l.pending[:0]
}

func (l *List) Append(c DisplayCmd) {
	c.Seq = int32(len(l.cmds))
	l.cmds = append(l.cmds, c)
	if b, ok := c.Bounds(); ok {
		if len(l.cmds) == 1 || l.bounds.Empty() {
			l.bounds = b
		} else {
			l.bounds = l.bounds.Union(b)
		}
	}
}

func (l *List) AddGroup(own float32, count int) {
	if count <= 0 || own <= 0 || own >= 1 {
		return
	}
	last := int32(len(l.cmds))
	l.pending = append(l.pending, pendingGroup{own: own, first: last - int32(count), last: last})
}

func (l *List) Len() int { return len(l.cmds) }

func (l *List) At(i int) DisplayCmd { return l.cmds[i] }

func (l *List) All() []DisplayCmd { return l.cmds }

func (l *List) Bounds() frame.Rect { return l.bounds }

func (l *List) SortStable() {
	slices.SortStableFunc(l.cmds, func(a, b DisplayCmd) int {
		switch {
		case a.Z < b.Z:
			return -1
		case a.Z > b.Z:
			return 1
		}
		return 0
	})
}

func (l *List) Build(version uint64) *LayerDL {
	dl := &LayerDL{cmds: l.cmds, version: version, bounds: l.bounds}
	var cb []cmdBounds
	if n := len(l.cmds); n > 0 {
		cb = make([]cmdBounds, n)
		for i := range l.cmds {
			r, ok := l.cmds[i].Bounds()
			cb[i] = cmdBounds{r: r, ok: ok}
		}
		dl.cb = cb
	}
	if len(l.pending) > 0 {
		dl.groups, dl.firstGroupAt = reconstructGroups(l.cmds, cb, l.pending)
	}
	l.cmds = nil
	l.bounds = frame.Rect{}
	l.pending = l.pending[:0]
	return dl
}

func reconstructGroups(cmds []DisplayCmd, cb []cmdBounds, pending []pendingGroup) ([]Group, []int32) {
	firstGroupAt := make([]int32, len(cmds))
	for i := range firstGroupAt {
		firstGroupAt[i] = -1
	}
	posOf := make([]int32, len(cmds))
	for i := range cmds {
		posOf[cmds[i].Seq] = int32(i)
	}
	mapped := make([]Group, 0, len(pending))
	for _, p := range pending {
		start := int(posOf[p.first])
		end := start + int(p.last-p.first)
		var b frame.Rect
		for i := start; i < end && i < len(cb); i++ {
			if cb[i].ok {
				b = b.Union(cb[i].r)
			}
		}
		mapped = append(mapped, Group{OwnOpacity: p.own, Bounds: b, Start: start, End: end})
	}
	slices.SortStableFunc(mapped, func(a, b Group) int {
		switch {
		case a.Start < b.Start:
			return -1
		case a.Start > b.Start:
			return 1
		case a.End > b.End:
			return -1
		case a.End < b.End:
			return 1
		}
		return 0
	})
	groups := make([]Group, 0, len(mapped))
	for idx := range mapped {
		groups = append(groups, mapped[idx])
		firstGroupAt[mapped[idx].Start] = int32(len(groups) - 1)
	}
	return groups, firstGroupAt
}

type LayerDL struct {
	cmds         []DisplayCmd
	version      uint64
	bounds       frame.Rect
	published    bool
	cb           []cmdBounds
	groups       []Group
	firstGroupAt []int32
}

type cmdBounds struct {
	r  frame.Rect
	ok bool
}

func (d *LayerDL) Publish() *LayerDL {
	if d.published {
		panic("paint: LayerDL published twice; a display list is frozen after publication")
	}
	d.published = true
	return d
}

func (d *LayerDL) Frozen() bool { return d.published }

func (d *LayerDL) Version() uint64 { return d.version }

func (d *LayerDL) Extent() frame.Rect { return d.bounds }

func (d *LayerDL) Bounds() frame.Rect { return d.bounds }

func (d *LayerDL) Len() int { return len(d.cmds) }

func (d *LayerDL) At(i int) DisplayCmd { return d.cmds[i] }

func (d *LayerDL) All() []DisplayCmd { return d.cmds }

func (d *LayerDL) Groups() []Group { return d.groups }

func (d *LayerDL) GroupAt(i int) (Group, bool) {
	if i < 0 || i >= len(d.firstGroupAt) {
		return Group{}, false
	}
	gi := d.firstGroupAt[i]
	if gi < 0 {
		return Group{}, false
	}
	return d.groups[gi], true
}

func ContentDamage(before, after frame.Content) frame.Rect {
	old, oldOK := before.(*LayerDL)
	next, nextOK := after.(*LayerDL)
	if !oldOK || !nextOK || old.Version() != next.Version() {
		var dirty frame.Rect
		if before != nil {
			dirty = before.Extent()
		}
		if after != nil {
			dirty = dirty.Union(after.Extent())
		}
		return dirty
	}
	var dirty frame.Rect
	add := func(cmd DisplayCmd) {
		if bounds, ok := cmd.Bounds(); ok {
			dirty = dirty.Union(bounds)
		}
	}
	if len(old.cmds) == len(next.cmds) {
		for i, cmd := range old.cmds {
			if !sameCommand(cmd, next.cmds[i]) {
				add(cmd)
				add(next.cmds[i])
			}
		}
		return dirty.Union(groupDamage(old, next))
	}
	start, oldEnd, newEnd := 0, len(old.cmds), len(next.cmds)
	for start < oldEnd && start < newEnd && sameCommand(old.cmds[start], next.cmds[start]) {
		start++
	}
	for oldEnd > start && newEnd > start && sameCommand(old.cmds[oldEnd-1], next.cmds[newEnd-1]) {
		oldEnd--
		newEnd--
	}
	for _, cmd := range old.cmds[start:oldEnd] {
		add(cmd)
	}
	for _, cmd := range next.cmds[start:newEnd] {
		add(cmd)
	}
	return dirty.Union(groupDamage(old, next))
}

func groupDamage(a, b *LayerDL) frame.Rect {
	var dirty frame.Rect
	if len(a.groups) != len(b.groups) {
		for _, g := range a.groups {
			dirty = dirty.Union(g.Bounds)
		}
		for _, g := range b.groups {
			dirty = dirty.Union(g.Bounds)
		}
		return dirty
	}
	for k := range a.groups {
		if a.groups[k] != b.groups[k] {
			dirty = dirty.Union(a.groups[k].Bounds)
			dirty = dirty.Union(b.groups[k].Bounds)
		}
	}
	return dirty
}

func sameCommand(a, b DisplayCmd) bool {
	return a.Kind == b.Kind && a.Rect == b.Rect && a.Color == b.Color &&
		a.Border == b.Border && a.Radius == b.Radius && a.Opacity == b.Opacity &&
		a.Clip == b.Clip && a.Z == b.Z && a.Filter == b.Filter && a.BackdropFilter == b.BackdropFilter &&
		a.Text.Color == b.Text.Color && slices.Equal(a.Text.Glyphs, b.Text.Glyphs) &&
		a.Image.SrcBox == b.Image.SrcBox && sameImage(a.Image.Src, b.Image.Src) && sameImage(a.Mask, b.Mask) &&
		a.Gradient.Angle == b.Gradient.Angle && slices.Equal(a.Gradient.Stops, b.Gradient.Stops) &&
		a.RadialGradient.Cx == b.RadialGradient.Cx && a.RadialGradient.Cy == b.RadialGradient.Cy &&
		a.RadialGradient.Rx == b.RadialGradient.Rx && a.RadialGradient.Ry == b.RadialGradient.Ry &&
		slices.Equal(a.RadialGradient.Stops, b.RadialGradient.Stops)
}

func sameImage(a, b image.Image) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	typ := reflect.TypeOf(a)
	return typ == reflect.TypeOf(b) && typ.Comparable() && a == b
}

func (d *LayerDL) CachedIntersects(i int, r frame.Rect) bool {
	if len(d.cb) == len(d.cmds) && i >= 0 && i < len(d.cmds) {
		return d.cb[i].ok && d.cb[i].r.Intersects(r)
	}
	return d.cmds[i].Intersects(r)
}

func (d *LayerDL) Intersecting(r frame.Rect) []DisplayCmd {
	lo, hi := -1, -1
	if len(d.cb) == len(d.cmds) {
		for i := range d.cmds {
			if !d.cb[i].ok || !d.cb[i].r.Intersects(r) {
				continue
			}
			if lo < 0 {
				lo = i
			}
			hi = i
		}
	} else {
		for i := range d.cmds {
			if !d.cmds[i].Intersects(r) {
				continue
			}
			if lo < 0 {
				lo = i
			}
			hi = i
		}
	}
	if lo < 0 {
		return nil
	}
	return d.cmds[lo : hi+1 : hi+1]
}
