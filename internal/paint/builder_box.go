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

func (b *Builder) paintBox(id layout.ObjectID, inheritedClip *frame.Rect, ordinal int, inheritedOpacity float32) {
	obj := b.arena.Get(id)
	if obj.Style != nil && obj.Style.Display == style.DisplayNone {
		return
	}

	// Group opacity: an element's own opacity multiplies the accumulated opacity
	// of every ancestor, and that product reaches the element's background, border
	// and text as well as its descendants. A fully transparent box (effective 0)
	// paints nothing and hides its whole subtree, because opacity does not inherit
	// but a zero-opacity stacking context clips everything inside it to invisible;
	// we stop the walk rather than emit a command, since DisplayCmd.Opacity 0 is
	// reserved to mean "opaque/unset".
	effective := inheritedOpacity
	if obj.Style != nil {
		o := obj.Style.Opacity
		if o < 0 {
			o = 0
		} else if o > 1 {
			o = 1
		}
		effective *= o
	}
	if effective <= 0 {
		return
	}

	// visibility:hidden removes only the box's own paint; the box still occupies
	// layout space (unlike display:none) and a descendant may set visibility
	// back to visible, so children are walked regardless. "collapse" is treated
	// as hidden here; table-part collapse is out of scope.
	visibilityHidden := obj.Style != nil && obj.Style.Visibility != "visible"

	childClip := inheritedClip
	if obj.Style != nil {
		box := b.paintContextFor(obj, effective)
		commandStart := b.list.Len()
		if !box.hidden && !visibilityHidden {
			b.paintBoxDecoration(obj, box, ordinal)
			b.paintBoxContents(obj, box, inheritedClip)
		}
		b.applyBoxEffects(obj, box, commandStart)
		childClip = b.overflowClip(obj, childClip)
	}
	if obj.Style != nil && obj.Style.ContentVisibility == "hidden" {
		return
	}
	b.paintChildren(obj, childClip, effective)
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
		// Inline boxes span multiple lines; their text fragments own the visible
		// per-line backgrounds, while the union box would fill the gaps.
		if len(computed.BoxShadow) > 0 {
			b.paintBoxShadow(obj, paint.rect, paint.radius, paint.opacity)
		}
		if computed.BackgroundColor.A > 0 && !paint.backgroundClip.Empty() {
			b.appendBgFill(obj, paint.backgroundClip, paint.clipRadius, convertColor(computed.BackgroundColor), paint.opacity)
		}
		if g := gradient(computed.BackgroundGradient, paint.opacity); !g.Empty() {
			b.paintGradientTiles(obj, paint.backgroundClip, paint.clipRadius, g, false, frame.RadialGradient{})
		}
		if rg := radialGradient(computed.BackgroundRadialGradient, paint.opacity); !rg.Empty() {
			b.paintGradientTiles(obj, paint.backgroundClip, paint.clipRadius, frame.LinearGradient{}, true, rg)
		}
		b.paintBackground(obj, paint.opacity)
	}
	if computed.Display != style.DisplayInline && computed.Display != style.DisplayNone {
		b.paintBorders(obj, paint.rect, paint.radius, paint.opacity)
	}
	if computed.Display == style.DisplayListItem {
		b.paintMarker(obj, ordinal)
	}
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
	case "img":
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
	if computed.Overflow != style.OverflowHidden && computed.OverflowX != style.OverflowHidden && computed.OverflowY != style.OverflowHidden {
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
	item := 0
	for kid := obj.FirstKid; kid != 0; kid = b.arena.Get(kid).NextSibling {
		ordinal := 0
		if child := b.arena.Get(kid); child.Style != nil && child.Style.Display == style.DisplayListItem {
			// The counter advances for every item, even ones styled list-style:none.
			item++
			ordinal = item
		}
		b.paintBox(kid, clip, ordinal, opacity)
	}
}
