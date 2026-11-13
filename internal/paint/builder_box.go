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

// boxDeclaresOpacity reports whether a box brings an opacity of its own into the
// group-opacity walk. Layout also hands out boxes that own no element and fills
// them with a verbatim copy of the generating element's style: anonymous blocks
// and table wrappers (style.AnonymousBlockStyle: `s = *parent`) and every text
// run (style.inheritStyle: `cs := *parent`). That element has already
// contributed, so counting its copy again would fade one declaration once per
// generated box rather than once per element, which is what Chromium does.
// Generated content is the exception: a ::before/::after/::first-letter box's
// style is its own cascade result, and the synthetic text node standing in for it
// is attached to no parent element.
func boxDeclaresOpacity(obj *layout.Object) bool {
	if obj.Style == nil || obj.Node == nil {
		return false
	}
	return obj.Node.Element() || obj.Node.Parent == nil
}

// fragmentOwnsBackground reports whether a box may emit the background its
// computed style carries.
//
// The same verbatim style copies that made group opacity double-count own this
// defect (GOOSIE-INLINEBG-001). Layout splices a box per word for every text
// node and fills it with the generating element's computed style, background
// included (style.inheritStyle: `cs := *parent`). When that element is inline the
// copy is the whole story: the element's own box paints no background at all -
// paintBoxDecoration's paintsInlineFragments test leaves it to the fragments -
// and each fragment carries one piece of it, which is how an inline background is
// meant to fragment across lines.
//
// When the generating element is not inline, its own box already fills that
// background across the whole padding area, so a fragment that fills it again
// composites the element's alpha a second time wherever a word sits: measured on
// a 300x60 block with `background:rgba(200,0,0,.5)` holding `one two three four`,
// the run fills rendered (213,63,63) against the block's own (227,127,127), where
// Chromium renders (227,127,127) uniformly.
//
// The generating element is looked up by node rather than by walking up the arena
// parents, because layout reparents runs into the anonymous blocks and line boxes
// it splices: the box a run hangs off is frequently anonymous, while the element
// that owns the copied style is the text node's DOM parent. Generated content is
// the exception, exactly as it is for group opacity above: a
// ::before/::after/::first-letter box's synthetic text node has no parent
// element, that box *is* the pseudo-element's box, and it owns its background.
func (b *Builder) fragmentOwnsBackground(obj *layout.Object) bool {
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

	// Group opacity: an element's own opacity multiplies the accumulated opacity
	// of every ancestor. When this box is a *clean* faded group we withhold its
	// own fraction from the threaded value and record it instead, so the group is
	// faded once offscreen at composite time rather than per command (which would
	// double-darken overlapping children). A non-clean faded box keeps threading
	// own × inherited into every command exactly as before. A fully transparent
	// box (effective 0) still paints nothing and hides its subtree, so the walk
	// stops rather than emit a command, since DisplayCmd.Opacity 0 is reserved to
	// mean "opaque/unset".
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

	// visibility:hidden removes only the box's own paint; the box still occupies
	// layout space (unlike display:none) and a descendant may set visibility
	// back to visible, so children are walked regardless. "collapse" is treated
	// as hidden here; table-part collapse is out of scope.
	visibilityHidden := obj.Style != nil && obj.Style.Visibility != "visible"

	childClip := inheritedClip
	commandStart := b.list.Len()
	// A nested group that would open on the same command as an enclosing
	// recorded group is not recorded: its fade threads per-command inside the
	// outer buffer instead. Safe (never loses a fade); the only case it touches
	// is a decoration-less faded container whose first command is an already-
	// grouped faded child.
	headConflict := len(b.groupHeadStack) > 0 && b.groupHeadStack[len(b.groupHeadStack)-1] == int32(commandStart)
	recordThis := groupOpacity > 0 && !headConflict
	// A clean faded box we decide not to record must still fade: its own fraction
	// was withheld from effective above, so fold it back in here. It then paints
	// per-command inside the enclosing group's buffer rather than vanishing (the
	// enclosing group fades the whole buffer once by its own opacity on top).
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

// zIndexOf maps a positioned box's non-negative z-index onto the display list's
// ordering field. The engine publishes stacking support for positioned boxes
// only: a positive value lifts the box and its subtree above the Z=0 flow, and
// later commands at the same value keep document order through the stable sort.
func zIndexOf(s *style.ComputedStyle) (uint32, bool) {
	if s == nil || !s.HasZIndex || s.ZIndex <= 0 || s.Position == style.PositionStatic {
		return 0, false
	}
	return uint32(s.ZIndex), true
}

// isCleanFadedGroup reports whether obj's subtree contains no descendant with a
// lifting z-index. It is the precondition for recording a faded group: a clean
// group's every command carries the box's single Z, so List.SortStable keeps the
// whole range contiguous and the rasterizer can render [Start,End) as one buffer.
// The walk is bounded by the same arena depth the paint walk already allows, so
// it adds no unbounded recursion. A false read is always safe — the box simply
// falls back to the per-command path.
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

// clipCommands intersects an ancestor's overflow clip into every command a box
// emitted. Only text and media consulted the inherited clip before, so a
// descendant's background and borders escaped an overflow:hidden scrollport
// even though the text inside it was cut.
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
		// Inline boxes span multiple lines; their text fragments own the visible
		// per-line backgrounds, while the union box would fill the gaps.
		//
		// The shadow is left gated only on paintsInlineFragments: text runs
		// duplicate it the same way they duplicated the background, but that is
		// measured and registered as its own defect, not silently folded into
		// this one.
		if len(computed.BoxShadow) > 0 {
			b.paintBoxShadow(obj, paint.rect, paint.radius, paint.opacity)
		}
		if b.fragmentOwnsBackground(obj) {
			b.paintBoxBackground(obj, paint)
			// Inset layers are the other half of the same declaration and cannot
			// join the shadow call above: they paint over the background just
			// emitted and under the borders below (CSS Backgrounds §4.4), so they
			// belong on the background's own ownership test rather than the outer
			// shadow's.
			b.paintInsetBoxShadow(obj, paint)
		}
	}
	switch {
	case computed.Display == style.DisplayInline:
		// An inline element fragments its border per line; the union box would
		// draw a single rectangle across the line gaps. Outline on inline
		// fragments stays deferred (GOOSIE-INLINEBORDER-001 §8).
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

// paintBoxBackground emits a box's four background layers in paint order: the
// colour, then the linear and radial gradient layers, then any background-image.
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
	// Clip for hidden, scroll, and auto overflow — all three establish a scroll
	// container whose children must not paint outside the content box.
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
			// The counter advances for every item, even ones styled list-style:none.
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
