package paint

import (
	"image"
	"math"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

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
	if s := obj.Style; s != nil {
		switch s.ObjectFit {
		case "contain", "scale-down":
			if r, ok := fitContain(box, x1-x0, y1-y0); ok {
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

func (b *Builder) paintImageInput(obj *layout.Object, opacity float32, clip *frame.Rect) {
	src, ok := obj.Image.(image.Image)
	if ok && src != nil {
		box := src.Bounds()
		if box.Dx() > 0 && box.Dy() > 0 {
			b.paintImage(obj, opacity, clip)
			return
		}
	}
	b.paintImageInputFallback(obj, opacity, clip)
}

func (b *Builder) paintImageInputFallback(obj *layout.Object, opacity float32, clip *frame.Rect) {
	label := layout.ImageInputFallbackLabel(obj.Node)
	if label == "" || obj.Style == nil {
		return
	}
	x0, y0, x1, y1 := obj.ContentRect()
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
	iconSize := int32(math.Round(float64(16 * b.scale)))
	if iconSize < 1 {
		iconSize = 1
	}
	iconY := vis.Y0 + (vis.Y1-vis.Y0-iconSize)/2
	icon := frame.Rect4(vis.X0, iconY, vis.X0+min(iconSize, vis.W()), iconY+iconSize).Intersection(vis)
	if !icon.Empty() {
		outline := frame.RGB(96, 96, 96)
		broken := frame.RGB(176, 72, 64)
		fill := func(ix0, iy0, ix1, iy1 int32, color frame.Color) {
			rx0 := icon.X0 + int32(float64(ix0)*float64(iconSize)/16)
			ry0 := icon.Y0 + int32(float64(iy0)*float64(iconSize)/16)
			rx1 := icon.X0 + int32(float64(ix1)*float64(iconSize)/16)
			ry1 := icon.Y0 + int32(float64(iy1)*float64(iconSize)/16)
			if rx1 <= rx0 {
				rx1 = rx0 + 1
			}
			if ry1 <= ry0 {
				ry1 = ry0 + 1
			}
			rect := frame.Rect4(rx0, ry0, rx1, ry1).Intersection(vis)
			if rect.Empty() {
				return
			}
			b.list.Append(DisplayCmd{Kind: CmdFill, Rect: rect, Color: color, Opacity: opacity, Clip: vis})
		}
		fill(0, 0, 16, 1, outline)
		fill(0, 15, 16, 16, outline)
		fill(0, 0, 1, 16, outline)
		fill(15, 0, 16, 16, outline)
		fill(3, 3, 5, 5, outline)
		fill(2, 12, 6, 13, outline)
		fill(5, 9, 8, 10, outline)
		fill(7, 7, 10, 9, outline)
		fill(9, 5, 12, 8, outline)
		fill(11, 10, 14, 13, outline)
		fill(7, 2, 8, 14, broken)
		fill(8, 8, 9, 15, broken)
	}
	textRect := vis
	if textRect.W() <= iconSize {
		return
	}
	textRect.X0 += iconSize
	b.appendRun(label, textRect, obj.Style, convertColor(obj.Style.Color), opacity)
}

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
	if clip != nil {
		cmd.Clip = *clip
	}
	b.list.Append(cmd)
}

func (b *Builder) glyphAdvance(size int32, r rune, slot frame.FontSlot) int32 {
	if b.metrics == nil {
		return size * 3 / 5
	}
	return b.metrics.GlyphAdvance(size, r, slot)
}

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
		Kind:    CmdText,
		Rect:    vis,
		Text:    TextRun{Glyphs: glyphs, Color: color},
		Clip:    vis,
		Opacity: opacity,
	})
}

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
