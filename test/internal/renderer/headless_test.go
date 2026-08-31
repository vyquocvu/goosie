package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"context"
	"image"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

func TestRenderHTMLToImage_SimpleHTML(t *testing.T) {
	html := `<!DOCTYPE html><html><body><h1>Hello</h1><p>World</p></body></html>`
	img, err := renderer.RenderHTMLToImage(context.Background(), html, 800, 600)
	if err != nil {
		t.Fatalf("RenderHTMLToImage: %v", err)
	}
	if img == nil {
		t.Fatal("result is nil")
	}
	b := img.Bounds()
	if w := b.Dx(); w != 800 {
		t.Errorf("width = %d, want 800", w)
	}
	if h := b.Dy(); h != 600 {
		t.Errorf("height = %d, want 600", h)
	}
}

func TestRenderHTMLToImage_EmptyHTML(t *testing.T) {
	img, err := renderer.RenderHTMLToImage(context.Background(), "", 800, 600)
	if err != nil {
		t.Fatalf("RenderHTMLToImage: %v", err)
	}
	if img == nil {
		t.Fatal("result is nil")
	}
}

func TestRenderHTMLToImage_MinimalHTML(t *testing.T) {
	html := `<p>Minimal content</p>`
	img, err := renderer.RenderHTMLToImage(context.Background(), html, 400, 300)
	if err != nil {
		t.Fatalf("RenderHTMLToImage: %v", err)
	}
	if img == nil {
		t.Fatal("result is nil")
	}
	b := img.Bounds()
	if b.Dx() != 400 || b.Dy() != 300 {
		t.Errorf("expected 400x300, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestRenderHTMLToImage_StyledHTML(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
		h1 { color: red; font-size: 24px; }
		p { color: blue; }
	</style></head><body><h1>Title</h1><p>Body text</p></body></html>`
	img, err := renderer.RenderHTMLToImage(context.Background(), html, 800, 600)
	if err != nil {
		t.Fatalf("RenderHTMLToImage: %v", err)
	}
	if img == nil {
		t.Fatal("result is nil")
	}
}

func TestRenderHTMLToImage_List(t *testing.T) {
	html := `<!DOCTYPE html><html><body><ul><li>Item A</li><li>Item B</li></ul></body></html>`
	img, err := renderer.RenderHTMLToImage(context.Background(), html, 800, 600)
	if err != nil {
		t.Fatalf("RenderHTMLToImage: %v", err)
	}
	if img == nil {
		t.Fatal("result is nil")
	}
}

func TestRenderHTMLToImage_Image(t *testing.T) {
	html := `<!DOCTYPE html><html><body><img src="nonexistent.png" alt="test"></body></html>`
	img, err := renderer.RenderHTMLToImage(context.Background(), html, 800, 600)
	if err != nil {
		t.Fatalf("RenderHTMLToImage: %v", err)
	}
	if img == nil {
		t.Fatal("result is nil")
	}
}

func TestRenderHTMLToImage_Table(t *testing.T) {
	html := `<!DOCTYPE html><html><body><table>
		<tr><td>A</td><td>B</td></tr>
		<tr><td>C</td><td>D</td></tr>
	</table></body></html>`
	img, err := renderer.RenderHTMLToImage(context.Background(), html, 800, 600)
	if err != nil {
		t.Fatalf("RenderHTMLToImage: %v", err)
	}
	if img == nil {
		t.Fatal("result is nil")
	}
}

func TestRenderHTMLToImage_MalformedHTML(t *testing.T) {
	html := `<div><p>Unclosed tags<span>nested<strong>more`
	img, err := renderer.RenderHTMLToImage(context.Background(), html, 800, 600)
	if err != nil {
		t.Fatalf("RenderHTMLToImage: %v", err)
	}
	if img == nil {
		t.Fatal("result is nil")
	}
}

func TestRenderHTMLToImage_ZeroDimensions(t *testing.T) {
	html := `<!DOCTYPE html><html><body><p>Hello</p></body></html>`
	img, err := renderer.RenderHTMLToImage(context.Background(), html, 0, 0)
	if err != nil {
		t.Fatalf("RenderHTMLToImage: %v", err)
	}
	if img == nil {
		t.Fatal("result is nil")
	}
}

func TestRenderHTMLToImage_NegativeDimensions(t *testing.T) {
	html := `<!DOCTYPE html><html><body><p>Hello</p></body></html>`
	img, err := renderer.RenderHTMLToImage(context.Background(), html, -100, -50)
	if err != nil {
		t.Fatalf("RenderHTMLToImage: %v", err)
	}
	if img == nil {
		t.Fatal("result is nil")
	}
}

func TestRenderHTMLToImage_LargeDimensions(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<h1>Large Canvas Test</h1>
		<p>This tests rendering at a larger resolution.</p>
	</body></html>`
	img, err := renderer.RenderHTMLToImage(context.Background(), html, 1920, 1080)
	if err != nil {
		t.Fatalf("RenderHTMLToImage: %v", err)
	}
	if img == nil {
		t.Fatal("result is nil")
	}
	b := img.Bounds()
	if b.Dx() != 1920 || b.Dy() != 1080 {
		t.Errorf("expected 1920x1080, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestRenderHTMLToImage_FormBody(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<form>
			<input type="text" placeholder="Enter name">
			<button type="submit">Submit</button>
			<textarea placeholder="Comments"></textarea>
		</form>
	</body></html>`
	img, err := renderer.RenderHTMLToImage(context.Background(), html, 800, 600)
	if err != nil {
		t.Fatalf("RenderHTMLToImage: %v", err)
	}
	if img == nil {
		t.Fatal("result is nil")
	}
}

func TestRenderHTMLToImage_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	html := `<!DOCTYPE html><html><body><p>Hello</p></body></html>`
	_, err := renderer.RenderHTMLToImage(ctx, html, 800, 600)
	if err != nil {
		t.Logf("cancelled context error: %v", err)
	}
}

func TestConvertPaintCommands_NilInput(t *testing.T) {
	result := renderer.ConvertPaintCommands(nil)
	if result == nil {
		t.Fatal("renderer.ConvertPaintCommands(nil) should return empty slice, not nil")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 commands, got %d", len(result))
	}
}

func TestConvertPaintCommands_EmptyInput(t *testing.T) {
	result := renderer.ConvertPaintCommands([]*renderer.PaintCommand{})
	if result == nil {
		t.Fatal("renderer.ConvertPaintCommands(empty) should return empty slice, not nil")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 commands, got %d", len(result))
	}
}

func TestConvertPaintCommands_SkipsNil(t *testing.T) {
	cmds := []*renderer.PaintCommand{
		nil,
		{Type: renderer.PaintRect, FillColor: image.Black, Box: renderer.Rect{X: 10, Y: 20, Width: 30, Height: 40}},
		nil,
	}
	result := renderer.ConvertPaintCommands(cmds)
	if len(result) != 1 {
		t.Fatalf("expected 1 command, got %d", len(result))
	}
	if result[0].Kind != raster.CmdFill {
		t.Errorf("expected CmdFill, got %v", result[0].Kind)
	}
}

func TestConvertPaintCommands_EmptyText(t *testing.T) {
	cmds := []*renderer.PaintCommand{
		{Type: renderer.PaintText, Text: "  ", Box: renderer.Rect{Width: 100, Height: 20}},
		{Type: renderer.PaintText, Text: "hello", Box: renderer.Rect{Width: 100, Height: 20}},
	}
	result := renderer.ConvertPaintCommands(cmds)
	if len(result) != 1 {
		t.Fatalf("expected 1 command (empty text skipped), got %d", len(result))
	}
	if result[0].Kind != raster.CmdText {
		t.Errorf("expected CmdText, got %v", result[0].Kind)
	}
}

func BenchmarkRenderHTMLToImage_Small(b *testing.B) {
	html := `<!DOCTYPE html><html><body>
		<h1>Small Document</h1>
		<p>This is a small test document for benchmarking.</p>
		<ul><li>Item 1</li><li>Item 2</li><li>Item 3</li></ul>
	</body></html>`
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := renderer.RenderHTMLToImage(ctx, html, 800, 600)
		if err != nil {
			b.Fatalf("RenderHTMLToImage: %v", err)
		}
	}
}

func BenchmarkRenderHTMLToImage_Medium(b *testing.B) {
	html := `<!DOCTYPE html><html><body>` +
		`<h1>Medium Document</h1>` +
		`<p>` + strings.Repeat("Lorem ipsum dolor sit amet. ", 50) + `</p>` +
		`<table>` +
		strings.Repeat("<tr><td>Cell A</td><td>Cell B</td><td>Cell C</td></tr>", 20) +
		`</table>` +
		`</body></html>`
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := renderer.RenderHTMLToImage(ctx, html, 800, 600)
		if err != nil {
			b.Fatalf("RenderHTMLToImage: %v", err)
		}
	}
}

func BenchmarkRenderHTMLToImage_Styled(b *testing.B) {
	html := `<!DOCTYPE html><html><head><style>` +
		strings.Repeat("p { color: blue; font-size: 16px; }\n", 10) +
		strings.Repeat(".highlight { background: yellow; }\n", 10) +
		`</style></head><body>` +
		strings.Repeat("<p class=\"highlight\">Styled paragraph text here.</p>", 30) +
		`</body></html>`
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := renderer.RenderHTMLToImage(ctx, html, 800, 600)
		if err != nil {
			b.Fatalf("RenderHTMLToImage: %v", err)
		}
	}
}
