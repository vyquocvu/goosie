package paint

import (
	"image"
	"math"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

// paintImage draws a decoded replaced image into its content box. A box whose
// image never decoded (or was not fetched) carries no pixels, so it contributes
// no command and simply leaves the gap the layout reserved for it.
func (b *Builder) paintImage(obj *layout.Object, opacity float32, clip *frame.Rect) {
	src, ok := obj.Image.(image.Image)
	if !ok || src == nil {
		return
	}
	box := src.Bounds()
	if box.Dx() <= 0 || box.Dy() <= 0 {
		return
	}
	x0, y0, x1, y1 := obj.ContentRect()
	if x1 <= x0 || y1 <= y0 {
		return
	}
	dest := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
	srcBox := box
	// object-fit decides how the intrinsic pixels sit in the content box. The
	// default (fill) and any unrecognised keyword stretch to the whole box, which
	// is what the two rects already say. contain shrinks the destination so the
	// whole image shows, centered; cover keeps the destination on the box and
	// crops the source to the centre so the box fills edge to edge.
	if s := obj.Style; s != nil {
		switch s.ObjectFit {
		case "contain", "scale-down":
			if r, ok := fitContain(box, x1-x0, y1-y0); ok {
				// fitContain centres inside `w x h`, so its edges are relative to
				// the content box origin. Treating them as page coordinates left
				// the picture letterboxed near the top-left of the document while
				// its own box sat empty further down the page.
				dest = frame.RectF4(r.X0+x0, r.Y0+y0, r.X1+x0, r.Y1+y0).ToDevice(b.scale)
			}
		case "cover":
			if sb, ok := fitCoverSrcBox(box, x1-x0, y1-y0); ok {
				srcBox = sb
			}
		}
	}
	b.list.Append(DisplayCmd{
		Kind:    CmdImage,
		Rect:    dest,
		Image:   ImageSpec{Src: src, SrcBox: srcBox},
		Opacity: opacity,
	})
}

// paintVideo draws a video element: the decoded frame when there is one, the
// element's own status message when there is not, and the control bar over the
// bottom when the author asked for controls.
//
// The element's box is measured uncut and the ancestor clip is applied per
// command. Deriving geometry from an already-intersected rectangle is what made
// the controls slide around as the element scrolled out of view: the bar belongs
// to the element, and only its ink is clipped.
func (b *Builder) paintVideo(obj *layout.Object, opacity float32, clip *frame.Rect) {
	s := obj.Style
	if s == nil || obj.Node == nil {
		return
	}
	x0, y0, x1, y1 := obj.ContentRect()
	if x1 <= x0 || y1 <= y0 {
		return
	}
	box := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
	if box.Empty() {
		return
	}
	vis := box
	if clip != nil {
		vis = box.Intersection(*clip)
		if vis.Empty() {
			return
		}
	}
	view, _ := obj.Media.(VideoView)

	// A dark viewport behind everything, so an element with no frame yet is a
	// letterbox rather than a hole showing whatever was painted underneath.
	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    vis,
		Color:   frame.RGB(20, 20, 24),
		Opacity: opacity,
	})

	if view.HasFrame() {
		b.paintVideoFrame(obj, view, x0, y0, x1-x0, y1-y0, clip, opacity)
	} else {
		b.paintVideoMessage(view, vis, opacity, s)
	}
	if obj.Node.HasAttribute("controls") && !view.Unsupported {
		b.paintVideoControls(view, box, vis, opacity, s)
	}
}

// paintVideoFrame draws the decoded picture, fitted the same way an <img> is.
func (b *Builder) paintVideoFrame(obj *layout.Object, view VideoView, x0, y0, cw, ch float32, clip *frame.Rect, opacity float32) {
	src := view.Pixels.Bounds()
	if src.Dx() <= 0 || src.Dy() <= 0 {
		return
	}
	x1, y1 := x0+cw, y0+ch
	dest := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
	srcBox := src
	if s := obj.Style; s != nil {
		switch s.ObjectFit {
		case "contain", "scale-down":
			if r, ok := fitContain(src, cw, ch); ok {
				dest = frame.RectF4(r.X0+x0, r.Y0+y0, r.X1+x0, r.Y1+y0).ToDevice(b.scale)
			}
		case "cover":
			if sb, ok := fitCoverSrcBox(src, cw, ch); ok {
				srcBox = sb
			}
		}
	}
	cmd := DisplayCmd{
		Kind:    CmdImage,
		Rect:    dest,
		Image:   ImageSpec{Src: view.Pixels, SrcBox: srcBox},
		Opacity: opacity,
	}
	// The frame is the part of the element that must not spill outside an
	// ancestor's overflow. Clipping the destination instead would squeeze the
	// whole picture into the visible part, which is the artefact this field
	// exists to avoid.
	if clip != nil {
		cmd.Clip = *clip
	}
	b.list.Append(cmd)
}

// glyphAdvance is the width r occupies at size, or the half-em estimate when the
// builder was constructed without font metrics to ask. Measuring is a luxury the
// display list can do without: the runs carry their size and slot, and the
// rasterizer resolves the real advances when it draws them.
func (b *Builder) glyphAdvance(size int32, r rune, slot frame.FontSlot) int32 {
	if b.metrics == nil {
		return size * 3 / 5
	}
	return b.metrics.GlyphAdvance(size, r, slot)
}

// paintVideoMessage is the loading and error text shown in place of a frame.
func (b *Builder) paintVideoMessage(view VideoView, vis frame.Rect, opacity float32, s *style.ComputedStyle) {
	if view.Message == "" {
		return
	}
	w, h := vis.X1-vis.X0, vis.Y1-vis.Y0
	size := int32(12 * b.scale)
	if size < 8 {
		size = 8
	}
	if size > h/2 {
		size = h / 2
	}
	if size < 6 {
		return
	}
	color := frame.RGB(170, 170, 176)
	if view.State == VideoFailed {
		color = frame.RGB(224, 130, 122)
	}
	slot := s.FontSlot()
	var total int32
	for _, r := range view.Message {
		total += b.glyphAdvance(size, r, slot)
	}
	px := vis.X0 + (w-total)/2
	py := vis.Y0 + (h+size)/3
	var glyphs []GlyphRun
	for _, r := range view.Message {
		glyphs = append(glyphs, GlyphRun{Rune: r, X: px, Y: py, Size: size, Slot: slot})
		px += b.glyphAdvance(size, r, slot)
	}
	if len(glyphs) == 0 {
		return
	}
	b.list.Append(DisplayCmd{
		Kind: CmdText,
		Rect: vis,
		Text: TextRun{Glyphs: glyphs, Color: color},
		// The message is drawn inside the viewport but must not escape the clip
		// that produced that viewport.
		Clip:    vis,
		Opacity: opacity,
	})
}

// paintVideoControls draws the bar over the bottom of the element, from geometry
// the hit tester shares so the two cannot disagree about where a button is.
func (b *Builder) paintVideoControls(view VideoView, box, vis frame.Rect, opacity float32, s *style.ComputedStyle) {
	c := VideoControlLayout(box, b.scale)
	if !c.Has {
		return
	}
	barRect := c.Bar.Intersection(vis)
	if barRect.Empty() {
		return
	}
	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    barRect,
		Color:   frame.RGBA(28, 28, 30, 220),
		Opacity: opacity,
		Clip:    vis,
	})

	ctrlY := c.Bar.Y1 - (c.Bar.Y1-c.Bar.Y0)/2
	btnSize := int32(14 * b.scale)
	if btnSize < 8 {
		btnSize = 8
	}

	// Play, or two bars while it is running. The pause mark is drawn rather than
	// taken from a glyph, because a font that has ▶ is not a font that has ⏸.
	playRect := c.Play.Intersection(vis)
	if !playRect.Empty() {
		if view.State == VideoPlaying {
			gap := btnSize / 4
			half := (c.Play.X1 - c.Play.X0 - gap) / 2
			for _, x := range []int32{c.Play.X0 + gap/2, c.Play.X0 + gap/2 + half + gap} {
				b.list.Append(DisplayCmd{
					Kind:    CmdFill,
					Rect:    frame.Rect4(x, ctrlY-btnSize/2, x+half, ctrlY+btnSize/2).Intersection(vis),
					Color:   frame.RGB(230, 230, 230),
					Opacity: opacity,
					Clip:    vis,
				})
			}
		} else {
			b.list.Append(DisplayCmd{
				Kind: CmdText,
				Rect: playRect,
				Text: TextRun{
					Glyphs: []GlyphRun{{
						// U+25BA, not U+25B6: the font this builds with has the
						// pointer and not the black triangle, and a glyph the
						// font lacks paints nothing at all - an invisible play
						// button on every video.
						Rune: '►',
						X:    c.Play.X0 + (c.Play.X1-c.Play.X0)/2 - btnSize/4,
						Y:    ctrlY + btnSize/3,
						Size: btnSize,
						Slot: s.FontSlot(),
					}},
					Color: frame.RGB(230, 230, 230),
				},
				Clip:    vis,
				Opacity: opacity,
			})
		}
	}

	if !c.Track.Empty() {
		trackH := int32(4 * b.scale)
		if trackH < 2 {
			trackH = 2
		}
		trackY0 := ctrlY - trackH/2
		trackY1 := trackY0 + trackH
		trackRect := frame.Rect4(c.Track.X0, trackY0, c.Track.X1, trackY1)
		if r := trackRect.Intersection(vis); !r.Empty() {
			b.list.Append(DisplayCmd{
				Kind:    CmdFill,
				Rect:    r,
				Color:   frame.RGB(80, 80, 80),
				Opacity: opacity,
				Clip:    vis,
			})
			// Progress from the real position. A duration that is not known yet
			// paints an empty track: guessing a quarter of the way along, which is
			// what a placeholder does, is a claim about the video.
			if frac := videoProgress(view); frac > 0 {
				played := int32(float64(c.Track.X1-c.Track.X0) * frac)
				if played > 0 {
					if r := frame.Rect4(c.Track.X0, trackY0, c.Track.X0+played, trackY1).Intersection(vis); !r.Empty() {
						b.list.Append(DisplayCmd{
							Kind:    CmdFill,
							Rect:    r,
							Color:   frame.RGB(66, 133, 244),
							Opacity: opacity,
							Clip:    vis,
						})
					}
				}
				scrubR := int32(5 * b.scale)
				if scrubR < 2 {
					scrubR = 2
				}
				kx := c.Track.X0 + played
				if r := frame.Rect4(kx-scrubR, ctrlY-scrubR, kx+scrubR, ctrlY+scrubR).Intersection(vis); !r.Empty() {
					b.list.Append(DisplayCmd{
						Kind:    CmdFill,
						Rect:    r,
						Color:   frame.RGB(255, 255, 255),
						Radius:  uniformCorners(float32(scrubR)),
						Opacity: opacity,
						Clip:    vis,
					})
				}
			}
		}
	}

	if !c.Mute.Empty() {
		if r := c.Mute.Intersection(vis); !r.Empty() {
			b.paintVideoMute(view, c.Mute, ctrlY, vis, opacity, s.FontSlot())
		}
	}

	// Elapsed over duration, or just elapsed while the length is unknown.
	label := VideoTimeLabel(view.Position)
	if !math.IsNaN(view.Duration) && view.Duration > 0 {
		label += " / " + VideoTimeLabel(view.Duration)
	}
	timeX := box.X1 - int32(videoTimeWidth*b.scale)
	if r := frame.Rect4(timeX, c.Bar.Y0, box.X1, c.Bar.Y1).Intersection(vis); !r.Empty() && timeX > c.Track.X0 {
		tsize := int32(11 * b.scale)
		if tsize < 7 {
			tsize = 7
		}
		var glyphs []GlyphRun
		px := timeX + int32(4*b.scale)
		for _, run := range label {
			glyphs = append(glyphs, GlyphRun{Rune: run, X: px, Y: ctrlY + tsize/3, Size: tsize, Slot: s.FontSlot()})
			px += b.glyphAdvance(tsize, run, s.FontSlot())
		}
		if len(glyphs) > 0 {
			b.list.Append(DisplayCmd{
				Kind:    CmdText,
				Rect:    r,
				Text:    TextRun{Glyphs: glyphs, Color: frame.RGB(200, 200, 200)},
				Clip:    vis,
				Opacity: opacity,
			})
		}
	}
}

// paintVideoMute draws the speaker out of rects because the font this builds
// with has no speaker glyph at all, and a glyph the font lacks paints nothing:
// an invisible mute button. Unmuted adds two sound waves; muted puts a
// multiplication sign where they were.
func (b *Builder) paintVideoMute(view VideoView, m frame.Rect, ctrlY int32, vis frame.Rect, opacity float32, slot frame.FontSlot) {
	w, h := m.X1-m.X0, m.Y1-m.Y0
	if w < 8 || h < 8 {
		return
	}
	col := frame.RGB(230, 230, 230)
	fill := func(x0, y0, x1, y1 int32) {
		if r := frame.Rect4(x0, y0, x1, y1).Intersection(vis); !r.Empty() {
			b.list.Append(DisplayCmd{Kind: CmdFill, Rect: r, Color: col, Opacity: opacity, Clip: vis})
		}
	}
	fill(m.X0+w/12, ctrlY-h/7, m.X0+w*4/12, ctrlY+h/7)
	fill(m.X0+w*4/12, ctrlY-h/4, m.X0+w*6/12, ctrlY+h/4)
	fill(m.X0+w*6/12, ctrlY-h/3, m.X0+w*8/12, ctrlY+h/3)
	if view.Muted {
		size := h * 3 / 5
		// Right-aligned to the button's own edge: the mark's advance would
		// otherwise run into the time label that sits against it.
		x := m.X1 - b.glyphAdvance(size, '×', slot)
		if r := frame.Rect4(x, m.Y0, m.X1, m.Y1).Intersection(vis); !r.Empty() {
			b.list.Append(DisplayCmd{
				Kind: CmdText,
				Rect: r,
				Text: TextRun{
					Glyphs: []GlyphRun{{Rune: '×', X: x, Y: ctrlY + size/3, Size: size, Slot: slot}},
					Color:  col,
				},
				Clip:    vis,
				Opacity: opacity,
			})
		}
		return
	}
	fill(m.X0+w*9/12, ctrlY-h/5, m.X0+w*10/12, ctrlY+h/5)
	fill(m.X0+w*10/12, ctrlY-h/3, m.X0+w*11/12, ctrlY+h/3)
}

// videoProgress is how much of the asset has played, in 0..1. An unknown duration
// reports 0 rather than a guess: the bar is the one control that makes a claim
// about the video, and a wrong claim there is worse than an empty one.
func videoProgress(view VideoView) float64 {
	if math.IsNaN(view.Duration) || view.Duration <= 0 || math.IsNaN(view.Position) || view.Position < 0 {
		return 0
	}
	f := view.Position / view.Duration
	if f > 1 {
		return 1
	}
	return f
}

// paintAudio draws an audio element control bar placeholder.
func (b *Builder) paintAudio(obj *layout.Object, opacity float32, clip *frame.Rect) {
	s := obj.Style
	if s == nil || obj.Node == nil {
		return
	}
	if !obj.Node.HasAttribute("controls") && (obj.W <= 0 || obj.H <= 0) {
		return
	}
	x0, y0, x1, y1 := obj.ContentRect()
	if x1 <= x0 || y1 <= y0 {
		return
	}
	rect := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
	if rect.Empty() {
		return
	}
	if clip != nil {
		rect = rect.Intersection(*clip)
		if rect.Empty() {
			return
		}
	}

	cornerRadius := int32(8 * b.scale)
	barBg := frame.RGB(241, 243, 244)
	barBorder := frame.RGB(218, 220, 224)

	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    rect,
		Color:   barBg,
		Radius:  uniformCorners(float32(cornerRadius)),
		Opacity: opacity,
	})
	b.list.Append(DisplayCmd{
		Kind: CmdBorder,
		Rect: rect,
		Border: BorderSpec{
			Top:    SideSpec{Width: 1, Color: barBorder},
			Right:  SideSpec{Width: 1, Color: barBorder},
			Bottom: SideSpec{Width: 1, Color: barBorder},
			Left:   SideSpec{Width: 1, Color: barBorder},
		},
		Radius:  uniformCorners(float32(cornerRadius)),
		Opacity: opacity,
	})

	h := rect.Y1 - rect.Y0
	cy := rect.Y0 + h/2

	// Play button circle
	playBtnR := int32(11 * b.scale)
	if playBtnR > h/2-2 {
		playBtnR = h/2 - 2
	}
	playBtnX := rect.X0 + int32(16*b.scale)
	if playBtnX+playBtnR < rect.X1 {
		b.list.Append(DisplayCmd{
			Kind:    CmdFill,
			Rect:    frame.Rect4(playBtnX-playBtnR, cy-playBtnR, playBtnX+playBtnR, cy+playBtnR),
			Color:   frame.RGB(32, 33, 36),
			Radius:  uniformCorners(float32(playBtnR)),
			Opacity: opacity,
		})
		iconSize := playBtnR * 4 / 5
		b.list.Append(DisplayCmd{
			Kind: CmdText,
			Rect: frame.Rect4(playBtnX-playBtnR, cy-playBtnR, playBtnX+playBtnR, cy+playBtnR),
			Text: TextRun{
				Glyphs: []GlyphRun{{
					Rune: '▶',
					X:    playBtnX - iconSize/4,
					Y:    cy + iconSize/3,
					Size: iconSize,
					Slot: s.FontSlot(),
				}},
				Color: frame.RGB(255, 255, 255),
			},
			Opacity: opacity,
		})
	}

	// Elapsed time
	timeX := playBtnX + playBtnR + int32(10*b.scale)
	timeSize := int32(11 * b.scale)
	timeStr := "0:00 / 0:00"
	timeW := int32(len(timeStr)) * timeSize * 3 / 5
	if timeX+timeW < rect.X1-int32(20*b.scale) {
		var gr []GlyphRun
		px := timeX
		for _, r := range timeStr {
			gr = append(gr, GlyphRun{Rune: r, X: px, Y: cy + timeSize/3, Size: timeSize, Slot: s.FontSlot()})
			px += timeSize * 3 / 5
		}
		b.list.Append(DisplayCmd{
			Kind: CmdText,
			Rect: frame.Rect4(timeX, cy-timeSize, timeX+timeW, cy+timeSize),
			Text: TextRun{
				Glyphs: gr,
				Color:  frame.RGB(95, 99, 104),
			},
			Opacity: opacity,
		})
	}

	// Progress track
	trackLeft := timeX + timeW + int32(12*b.scale)
	trackRight := rect.X1 - int32(16*b.scale)
	if trackRight > trackLeft+int32(20*b.scale) {
		trackH := int32(4 * b.scale)
		if trackH < 2 {
			trackH = 2
		}
		trackY0 := cy - trackH/2
		trackY1 := trackY0 + trackH
		b.list.Append(DisplayCmd{
			Kind:    CmdFill,
			Rect:    frame.Rect4(trackLeft, trackY0, trackRight, trackY1),
			Color:   frame.RGB(218, 220, 224),
			Opacity: opacity,
		})
		thumbR := int32(4 * b.scale)
		b.list.Append(DisplayCmd{
			Kind:    CmdFill,
			Rect:    frame.Rect4(trackLeft-thumbR, cy-thumbR, trackLeft+thumbR, cy+thumbR),
			Color:   frame.RGB(26, 115, 232),
			Radius:  uniformCorners(float32(thumbR)),
			Opacity: opacity,
		})
	}
}

// fitContain returns the centered destination rect, in content px, that shows the
// whole source box scaled up or down to fit inside w x h without cropping.
func fitContain(srcBox image.Rectangle, w, h float32) (frame.RectF, bool) {
	iw, ih := float32(srcBox.Dx()), float32(srcBox.Dy())
	if iw <= 0 || ih <= 0 || w <= 0 || h <= 0 {
		return frame.RectF{}, false
	}
	scale := w / iw
	if r := h / ih; r < scale {
		scale = r
	}
	dw, dh := iw*scale, ih*scale
	x := (w - dw) / 2
	y := (h - dh) / 2
	return frame.RectF4(x, y, x+dw, y+dh), true
}

// fitCoverSrcBox returns the centred source sub-rect that fills a w x h box when
// the image is scaled by the larger ratio, cropping the overflow off the edges.
func fitCoverSrcBox(srcBox image.Rectangle, w, h float32) (image.Rectangle, bool) {
	iw, ih := float32(srcBox.Dx()), float32(srcBox.Dy())
	if iw <= 0 || ih <= 0 || w <= 0 || h <= 0 {
		return image.Rectangle{}, false
	}
	scale := w / iw
	if r := h / ih; r > scale {
		scale = r
	}
	cropW := w / scale
	cropH := h / scale
	x0 := srcBox.Min.X + int((iw-cropW)/2)
	y0 := srcBox.Min.Y + int((ih-cropH)/2)
	r := image.Rect(x0, y0, x0+int(cropW+0.5), y0+int(cropH+0.5))
	return r, !r.Empty()
}
