package paint

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

type boxPaintContext struct {
	x0, y0, x1, y1 float32
	rect           frame.Rect
	radius         frame.Corners
	backgroundClip frame.Rect
	clipRadius     frame.Corners
	opacity        float32
	hidden         bool
}

func boxDeclaresOpacity(obj *layout.Object) bool {
	if obj.Style == nil || obj.Node == nil {
		return false
	}
	return obj.Node.Element() || obj.Node.Parent == nil
}

func (b *Builder) fragmentOwnsDecorations(obj *layout.Object) bool {
	if obj.Node == nil || !obj.Node.Text() || obj.Node.Parent == nil {
		return true
	}
	generator, ok := b.arena.ForNode(obj.Node.Parent.ID)
	if !ok {
		return true
	}
	st := b.arena.Get(generator).Style
	return st == nil || st.Display == style.DisplayInline
}

func (b *Builder) paintBox(id layout.ObjectID, inheritedClip *frame.Rect, ordinal int, inheritedOpacity float32) {
	obj := b.arena.Get(id)
	if obj.Style != nil && (obj.Style.Display == style.DisplayNone || obj.Style.ContentVisibility == "hidden") {
		return
	}

	effective := inheritedOpacity
	groupOpacity := float32(0)
	if boxDeclaresOpacity(obj) {
		o := obj.Style.Opacity
		if o < 0 {
			o = 0
		} else if o > 1 {
			o = 1
		}
		if o > 0 && o < 1 && b.isCleanFadedGroup(obj) {
			groupOpacity = o
		} else {
			effective *= o
		}
	}
	if effective <= 0 {
		return
	}

	visibilityHidden := obj.Style != nil && obj.Style.Visibility != "visible"

	childClip := inheritedClip
	commandStart := b.list.Len()
	headConflict := len(b.groupHeadStack) > 0 && b.groupHeadStack[len(b.groupHeadStack)-1] == int32(commandStart)
	recordThis := groupOpacity > 0 && !headConflict
	if groupOpacity > 0 && headConflict {
		effective *= groupOpacity
	}
	if obj.Style != nil {
		box := b.paintContextFor(obj, effective)
		start := b.list.Len()
		if !box.hidden && !visibilityHidden {
			b.paintBoxDecoration(obj, box, ordinal)
			b.paintBoxContents(obj, box, inheritedClip)
		}
		b.applyBoxEffects(obj, box, start)
		if inheritedClip != nil {
			clipCommands(b.list, start, *inheritedClip)
		}
		childClip = b.overflowClip(obj, childClip)
	}
	if recordThis {
		b.groupHeadStack = append(b.groupHeadStack, int32(commandStart))
	}
	b.paintChildren(obj, childClip, effective)
	if recordThis {
		b.groupHeadStack = b.groupHeadStack[:len(b.groupHeadStack)-1]
		b.list.AddGroup(groupOpacity, b.list.Len()-commandStart)
	}
	if z, ok := zIndexOf(obj.Style); ok {
		commands := b.list.All()
		for i := commandStart; i < len(commands); i++ {
			if commands[i].Z == 0 {
				commands[i].Z = z
			}
		}
	}
}

func zIndexOf(s *style.ComputedStyle) (uint32, bool) {
	if s == nil || !s.HasZIndex || s.ZIndex <= 0 || s.Position == style.PositionStatic {
		return 0, false
	}
	return uint32(s.ZIndex), true
}

func (b *Builder) isCleanFadedGroup(obj *layout.Object) bool {
	for kid := obj.FirstKid; kid != 0; kid = b.arena.Get(kid).NextSibling {
		k := b.arena.Get(kid)
		if _, ok := zIndexOf(k.Style); ok {
			return false
		}
		if !b.isCleanFadedGroup(k) {
			return false
		}
	}
	return true
}

func clipCommands(list *List, start int, clip frame.Rect) {
	commands := list.All()
	for i := start; i < len(commands); i++ {
		if commands[i].Clip.Empty() {
			commands[i].Clip = clip
		} else {
			commands[i].Clip = commands[i].Clip.Intersection(clip)
		}
	}
}

func (b *Builder) paintContextFor(obj *layout.Object, opacity float32) boxPaintContext {
	x0, y0, x1, y1 := obj.BorderRect()
	rect := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
	radius := corners(obj.Style, rect, b.scale)
	cx0, cy0, cx1, cy1 := obj.BgAreaRect(obj.Style.BackgroundClip)
	backgroundClip := rect.Intersection(frame.RectF4(cx0, cy0, cx1, cy1).ToDevice(b.scale))

	return boxPaintContext{
		x0:             x0,
		y0:             y0,
		x1:             x1,
		y1:             y1,
		rect:           rect,
		radius:         radius,
		backgroundClip: backgroundClip,
		clipRadius: radius.Shrunk(
			backgroundClip.Y0-rect.Y0, rect.X1-backgroundClip.X1,
			rect.Y1-backgroundClip.Y1, backgroundClip.X0-rect.X0),
		opacity: opacity,
		hidden:  obj.Style.HasClip && (obj.Style.ClipTop >= obj.Style.ClipBottom || obj.Style.ClipLeft >= obj.Style.ClipRight),
	}
}

func (b *Builder) paintBoxDecoration(obj *layout.Object, paint boxPaintContext, ordinal int) {
	computed := obj.Style
	paintsInlineFragments := computed.Display != style.DisplayInline || (obj.Node != nil && obj.Node.Text())
	if paintsInlineFragments {
		owns := b.fragmentOwnsDecorations(obj)
		if owns && len(computed.BoxShadow) > 0 {
			b.paintBoxShadow(obj, paint.rect, paint.radius, paint.opacity)
		}
		if owns {
			b.paintBoxBackground(obj, paint)
			b.paintInsetBoxShadow(obj, paint)
		}
	}
	switch {
	case computed.Display == style.DisplayInline:
		if len(obj.InlineFragments) > 0 {
			b.paintInlineBorderFragments(obj, paint.opacity)
		}
	case computed.Display != style.DisplayNone:
		b.paintBorders(obj, paint.rect, paint.radius, paint.opacity)
		b.paintOutline(computed, paint.rect, paint.opacity)
	}
	if computed.Display == style.DisplayListItem {
		b.paintMarker(obj, ordinal)
	}
}

func (b *Builder) paintBoxBackground(obj *layout.Object, paint boxPaintContext) {
	computed := obj.Style
	if computed.BackgroundColor.A > 0 && !paint.backgroundClip.Empty() {
		b.appendBgFill(obj, paint.backgroundClip, paint.clipRadius, convertColor(computed.BackgroundColor), paint.opacity)
	}
	if !computed.BackgroundGradient.Empty() {
		b.paintGradientTiles(obj, paint.backgroundClip, paint.clipRadius, &computed.BackgroundGradient, nil, paint.opacity)
	}
	if !computed.BackgroundRadialGradient.Empty() {
		b.paintGradientTiles(obj, paint.backgroundClip, paint.clipRadius, nil, &computed.BackgroundRadialGradient, paint.opacity)
	}
	b.paintBackground(obj, paint.opacity)
}

func (b *Builder) paintBoxContents(obj *layout.Object, box boxPaintContext, clip *frame.Rect) {
	if obj.Node != nil && obj.Node.Text() && (obj.Node.DataContent != "" || b.composingTextareaText(obj)) {
		b.paintText(obj, box.rect, box.opacity, clip)
	}
	b.paintInput(obj, box, clip)
	if obj.Node != nil && obj.Node.Data == "select" {
		b.paintSelect(obj, box.opacity, clip)
	}
	b.paintReplacedElement(obj, box.opacity, clip)
	b.paintProgressMeter(obj, box.rect, box.opacity)
	if obj.Node != nil && obj.Node.Namespace == svgNamespace && isSVGShape(obj.Node.Data) {
		b.paintSVGShape(obj, box.opacity)
	}
	if b.focus.object != nil && obj == b.focus.object {
		b.paintFocus(obj, clip)
	}
}

func (b *Builder) paintInput(obj *layout.Object, box boxPaintContext, clip *frame.Rect) {
	if obj.Node == nil || obj.Node.Data != "input" {
		return
	}
	switch strings.ToLower(obj.Node.GetAttribute("type")) {
	case "checkbox":
		b.paintCheckbox(obj, box.rect, box.opacity)
	case "radio":
		b.paintRadio(obj, box.rect, box.opacity)
	case "range":
		b.paintRange(obj, box.opacity, clip)
	case "submit", "reset", "button":
		b.paintButtonLabel(obj, box.opacity, clip)
	default:
		b.paintControlValue(obj, box.opacity, clip)
		b.paintPlaceholder(obj, box.opacity, clip)
	}
}

func (b *Builder) paintReplacedElement(obj *layout.Object, opacity float32, clip *frame.Rect) {
	if obj.Node == nil {
		return
	}
	switch obj.Node.Data {
	case "img", "canvas":
		b.paintImage(obj, opacity, clip)
	case "video":
		b.paintVideo(obj, opacity, clip)
	case "audio":
		b.paintAudio(obj, opacity, clip)
	}
}

func (b *Builder) paintProgressMeter(obj *layout.Object, rect frame.Rect, opacity float32) {
	if obj.Node == nil {
		return
	}
	switch obj.Node.Data {
	case "progress":
		b.paintProgress(obj, rect, opacity)
	case "meter":
		b.paintMeter(obj, rect, opacity)
	}
}

func (b *Builder) applyBoxEffects(obj *layout.Object, box boxPaintContext, commandStart int) {
	if len(obj.Style.Transform) > 0 {
		b.applyTransform(obj, commandStart, box.x0, box.y0, box.x1, box.y1)
	}
	hasFilter := len(obj.Style.Filter) > 0
	hasBackdropFilter := len(obj.Style.BackdropFilter) > 0
	if !hasFilter && !hasBackdropFilter {
		return
	}

	filter := css.FormatFilter(obj.Style.Filter)
	backdropFilter := css.FormatFilter(obj.Style.BackdropFilter)
	commands := b.list.All()
	for i := commandStart; i < len(commands); i++ {
		if hasFilter {
			commands[i].Filter = filter
		}
		if hasBackdropFilter {
			commands[i].BackdropFilter = backdropFilter
		}
	}
}

func (b *Builder) overflowClip(obj *layout.Object, inherited *frame.Rect) *frame.Rect {
	computed := obj.Style
	hasClip := computed.Overflow == style.OverflowHidden ||
		computed.Overflow == style.OverflowScroll ||
		computed.Overflow == style.OverflowAuto ||
		computed.OverflowX == style.OverflowHidden ||
		computed.OverflowX == style.OverflowScroll ||
		computed.OverflowX == style.OverflowAuto ||
		computed.OverflowY == style.OverflowHidden ||
		computed.OverflowY == style.OverflowScroll ||
		computed.OverflowY == style.OverflowAuto
	if !hasClip {
		return inherited
	}
	x0, y0, x1, y1 := obj.ContentRect()
	clip := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
	if inherited == nil {
		return &clip
	}
	intersected := inherited.Intersection(clip)
	return &intersected
}

func (b *Builder) paintChildren(obj *layout.Object, clip *frame.Rect, opacity float32) {
	var scrollDX, scrollDY int32
	if b.scrollOffset != nil && obj.Style != nil {
		s := obj.Style
		isScrollContainer := s.Overflow == style.OverflowScroll || s.Overflow == style.OverflowAuto ||
			s.OverflowX == style.OverflowScroll || s.OverflowX == style.OverflowAuto ||
			s.OverflowY == style.OverflowScroll || s.OverflowY == style.OverflowAuto
		if isScrollContainer {
			scrollX := b.scrollOffset(obj, "scrollLeft")
			scrollY := b.scrollOffset(obj, "scrollTop")
			scrollDX = int32(-scrollX * float64(b.scale))
			scrollDY = int32(-scrollY * float64(b.scale))
		}
	}
	cmdStart := b.list.Len()
	item := 0
	for kid := obj.FirstKid; kid != 0; kid = b.arena.Get(kid).NextSibling {
		ordinal := 0
		if child := b.arena.Get(kid); child.Style != nil && child.Style.Display == style.DisplayListItem {
			item++
			ordinal = item
		}
		b.paintBox(kid, clip, ordinal, opacity)
	}
	if scrollDX != 0 || scrollDY != 0 {
		cmds := b.list.All()
		for i := cmdStart; i < len(cmds); i++ {
			cmds[i].Rect.X0 += scrollDX
			cmds[i].Rect.X1 += scrollDX
			cmds[i].Rect.Y0 += scrollDY
			cmds[i].Rect.Y1 += scrollDY
		}
	}
}
