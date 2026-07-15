package renderer

import (
	"context"
	"image"
	"image/color"
	"strings"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
	"golang.org/x/net/html"
)

// RenderHTMLToImage renders HTML content to an *image.RGBA using the pure-Go
// CPU raster backend, without opening any window or depending on Fyne at
// call site. The function runs the full engine pipeline:
//
//	HTML Parse → CSS Extract → Render Tree → Style → Layout → Display List → Raster
//
// width and height are in layout pixels. The returned image uses device-pixel
// resolution (1x scale). For HiDPI output, scale the dimensions accordingly.
func RenderHTMLToImage(ctx context.Context, htmlContent string, width, height int) (*image.RGBA, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	stylesheet := extractAndParseCSS(doc)
	w := float32(width)
	h := float32(height)

	bodyNode := findBodyNode(doc)
	if bodyNode == nil {
		bodyNode = doc
	}

	renderTree := BuildRenderTree(bodyNode)
	if renderTree == nil {
		return image.NewRGBA(image.Rect(0, 0, width, height)), nil
	}

	renderTreeCopy := renderTree.Clone()
	if stylesheet != nil && len(stylesheet.Rules) > 0 {
		styleManager := NewStyleManagerWithViewport(stylesheet, w, h)
		styleManager.ApplyStyles(renderTreeCopy)
	}

	layoutEngine := NewLayoutEngine(w, h)
	layoutTree := layoutEngine.ComputeLayout(renderTreeCopy)

	dlb := NewDisplayListBuilder()
	displayList := dlb.Build(layoutTree, renderTreeCopy)
	SortByZIndex(displayList)

	cmds := convertPaintCommands(displayList.Commands)

	vp := frame.NewViewport(float32(width), float32(height), frame.PixelScaleDefault)
	backend := raster.NewCPUBackend(width, height)
	defer backend.Close()

	if err := backend.BeginFrame(vp); err != nil {
		return nil, err
	}

	img, err := backend.Rasterize(cmds, nil)
	if err != nil {
		return nil, err
	}

	if err := backend.EndFrame(); err != nil {
		return nil, err
	}

	return toRGBA(img), nil
}

// convertPaintCommands converts the old PaintCommand slice to raster.DisplayCmd
// commands for the backend-neutral raster pipeline.
func convertPaintCommands(cmds []*PaintCommand) []raster.DisplayCmd {
	out := make([]raster.DisplayCmd, 0, len(cmds))
	for _, cmd := range cmds {
		if cmd == nil {
			continue
		}
		switch cmd.Type {
		case PaintText:
			text := strings.TrimSpace(cmd.Text)
			if text == "" {
				continue
			}
			out = append(out, raster.DisplayCmd{
				Kind:    raster.CmdText,
				Rect:    toFrameRect(cmd.Box),
				Color:   frame.FromStdColor(textPaintColor(cmd)),
				TextRun: buildTextRun(text, effectiveFontSize(cmd), cmd),
			})

		case PaintRect:
			out = append(out, raster.DisplayCmd{
				Kind:  raster.CmdFill,
				Rect:  toFrameRect(cmd.Box),
				Color: frame.FromStdColor(cmd.FillColor),
			})

		case PaintBorder:
			out = append(out, raster.DisplayCmd{
				Kind:   raster.CmdBorder,
				Rect:   toFrameRect(cmd.Box),
				Border: toBorderSpec(cmd),
			})

		case PaintImage:
			out = append(out, raster.DisplayCmd{
				Kind:  raster.CmdImage,
				Rect:  toFrameRect(cmd.Box),
				Image: toImageSpec(cmd),
			})

		case PushClip:
			out = append(out, raster.DisplayCmd{
				Kind: raster.CmdClipPush,
				Rect: toFrameRect(cmd.Box),
			})

		case PopClip:
			out = append(out, raster.DisplayCmd{
				Kind: raster.CmdClipPop,
			})

		case PaintLink:
			text := strings.TrimSpace(cmd.LinkText)
			if text == "" {
				continue
			}
			out = append(out, raster.DisplayCmd{
				Kind:    raster.CmdText,
				Rect:    toFrameRect(cmd.Box),
				Color:   frame.FromStdColor(textPaintColor(cmd)),
				TextRun: buildTextRun(text, effectiveFontSize(cmd), cmd),
			})

		case PaintButton, PaintInput, PaintTextarea:
			if dc := toInputDisplayCmd(cmd); dc.Kind != raster.CmdFill || dc.Rect.W > 0 {
				out = append(out, dc)
			}
		}
	}
	return out
}

// textPaintColor returns the effective text color from a PaintCommand.
func textPaintColor(cmd *PaintCommand) color.Color {
	if cmd.Color != nil {
		return cmd.Color
	}
	if cmd.Node != nil && cmd.Node.ComputedStyle != nil && cmd.Node.ComputedStyle.Color != nil {
		return cmd.Node.ComputedStyle.Color
	}
	return color.Black
}

// effectiveFontSize returns the font size from the command, falling back to 16.
func effectiveFontSize(cmd *PaintCommand) float32 {
	if cmd.FontSize > 0 {
		return cmd.FontSize
	}
	if cmd.Node != nil && cmd.Node.ComputedStyle != nil && cmd.Node.ComputedStyle.FontSize > 0 {
		return cmd.Node.ComputedStyle.FontSize
	}
	return 16
}

// buildTextRun creates a frame.TextRun from text with per-rune glyph layout.
// Uses basic per-rune glyph layout since we don't have full HarfBuzz shaping
// in the headless path. The raster backend renders via basicfont.Face7x13
// scaled by fontSize.
func buildTextRun(text string, fontSize float32, cmd *PaintCommand) frame.TextRun {
	if fontSize <= 0 {
		fontSize = 16
	}
	runes := []rune(text)
	glyphs := make([]frame.Glyph, len(runes))
	charWidth := fontSize * 0.6
	var x float32
	for i, r := range runes {
		glyphs[i] = frame.Glyph{
			ID:      uint32(r),
			Advance: charWidth,
			XOffset: x,
			YOffset: 0,
		}
		x += charWidth
	}
	return frame.TextRun{
		FontSize: fontSize,
		Color:    frame.FromStdColor(textPaintColor(cmd)),
		Glyphs:   glyphs,
	}
}

// toFrameRect converts a renderer.Rect to frame.Rect.
func toFrameRect(r Rect) frame.Rect {
	return frame.Rect{X: r.X, Y: r.Y, W: r.Width, H: r.Height}
}

// toBorderSpec converts a PaintCommand's border fields to raster.BorderSpec.
func toBorderSpec(cmd *PaintCommand) raster.BorderSpec {
	return raster.BorderSpec{
		Top:    raster.SideSpec{Width: cmd.BorderTopWidth, Color: frame.FromStdColor(cmd.BorderTopColor)},
		Right:  raster.SideSpec{Width: cmd.BorderRightWidth, Color: frame.FromStdColor(cmd.BorderRightColor)},
		Bottom: raster.SideSpec{Width: cmd.BorderBottomWidth, Color: frame.FromStdColor(cmd.BorderBottomColor)},
		Left:   raster.SideSpec{Width: cmd.BorderLeftWidth, Color: frame.FromStdColor(cmd.BorderLeftColor)},
	}
}

// toImageSpec converts a PaintCommand to an ImageSpec.
func toImageSpec(cmd *PaintCommand) raster.ImageSpec {
	var img image.Image
	if cmd.Node != nil && cmd.Node.ImageData != nil && cmd.Node.ImageData.Image != nil {
		img = cmd.Node.ImageData.Image
	}
	return raster.ImageSpec{
		Img: img,
	}
}

// toInputDisplayCmd converts form-element paint commands into visual
// equivalents (fill + text) for headless rendering.
func toInputDisplayCmd(cmd *PaintCommand) raster.DisplayCmd {
	buttonText := cmd.ButtonText
	if buttonText == "" {
		buttonText = cmd.InputValue
	}
	if buttonText == "" {
		buttonText = cmd.Placeholder
	}
	if buttonText == "" {
		return raster.DisplayCmd{
			Kind:  raster.CmdFill,
			Rect:  toFrameRect(cmd.Box),
			Color: frame.White,
		}
	}
	return raster.DisplayCmd{
		Kind:    raster.CmdText,
		Rect:    toFrameRect(cmd.Box),
		Color:   frame.Black,
		TextRun: buildTextRun(buttonText, 14, cmd),
	}
}

// toRGBA converts an image.Image to *image.RGBA, returning it directly
// if already *image.RGBA to avoid a copy.
func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	return rgba
}
