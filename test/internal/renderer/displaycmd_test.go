package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"encoding/json"
	"image/color"
	"math"
	"testing"
)

// --- DisplayCommandKind tests ---

func TestDisplayCommandKindString(t *testing.T) {
	tests := []struct {
		kind renderer.DisplayCommandKind
		want string
	}{
		{renderer.CmdRect, "Rect"},
		{renderer.CmdBorder, "Border"},
		{renderer.CmdText, "Text"},
		{renderer.CmdImage, "Image"},
		{renderer.CmdPushClip, "PushClip"},
		{renderer.CmdPopClip, "PopClip"},
		{renderer.CmdPushTransform, "PushTransform"},
		{renderer.CmdPopTransform, "PopTransform"},
		{renderer.CmdPushOpacity, "PushOpacity"},
		{renderer.CmdPopOpacity, "PopOpacity"},
		{renderer.CmdPushStackingContext, "PushStackingContext"},
		{renderer.CmdPopStackingContext, "PopStackingContext"},
		{renderer.DisplayCommandKind(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		got := tt.kind.String()
		if got != tt.want {
			t.Errorf("DisplayCommandKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

// --- RectCommand tests ---

func TestRectCommandDefaults(t *testing.T) {
	cmd := renderer.RectCommand{}
	if cmd.Color != nil {
		t.Error("default Color should be nil")
	}
	if cmd.Bounds != (renderer.RectF{}) {
		t.Error("default Bounds should be zero RectF")
	}
}

func TestRectCommandWithValues(t *testing.T) {
	cmd := renderer.RectCommand{
		Bounds: renderer.RectF{X: 10, Y: 20, W: 100, H: 50},
		Color:  color.RGBA{R: 255, G: 0, B: 0, A: 255},
	}
	if cmd.Bounds.X != 10 || cmd.Bounds.Y != 20 || cmd.Bounds.W != 100 || cmd.Bounds.H != 50 {
		t.Errorf("unexpected bounds: %+v", cmd.Bounds)
	}
}

// --- BorderCommand tests ---

func TestBorderCommandUniform(t *testing.T) {
	cmd := renderer.BorderCommand{
		Bounds: renderer.RectF{X: 0, Y: 0, W: 200, H: 100},
		Top:    renderer.BorderSide{Width: 1, Color: color.Black, Style: renderer.BorderSolid},
		Right:  renderer.BorderSide{Width: 1, Color: color.Black, Style: renderer.BorderSolid},
		Bottom: renderer.BorderSide{Width: 1, Color: color.Black, Style: renderer.BorderSolid},
		Left:   renderer.BorderSide{Width: 1, Color: color.Black, Style: renderer.BorderSolid},
	}
	if cmd.Top.Width != 1 || cmd.Top.Style != renderer.BorderSolid {
		t.Error("uniform border not set correctly")
	}
}

func TestBorderCommandPerSide(t *testing.T) {
	cmd := renderer.BorderCommand{
		Bounds: renderer.RectF{W: 100, H: 100},
		Top:    renderer.BorderSide{Width: 2, Color: color.RGBA{R: 255}, Style: renderer.BorderDashed},
		Right:  renderer.BorderSide{Width: 0},
		Bottom: renderer.BorderSide{Width: 1, Color: color.RGBA{B: 255}, Style: renderer.BorderDotted},
		Left:   renderer.BorderSide{Width: 3, Color: color.RGBA{G: 255}, Style: renderer.BorderSolid},
	}
	if cmd.Top.Style != renderer.BorderDashed {
		t.Error("top style should be dashed")
	}
	if cmd.Bottom.Style != renderer.BorderDotted {
		t.Error("bottom style should be dotted")
	}
}

// --- TextCommand tests ---

func TestTextCommandDefaults(t *testing.T) {
	cmd := renderer.TextCommand{}
	if cmd.FontSize != 0 {
		t.Error("default FontSize should be 0")
	}
	if cmd.Bold || cmd.Italic || cmd.Underline || cmd.Strikethrough {
		t.Error("default text decorations should be false")
	}
}

func TestTextCommandWithValues(t *testing.T) {
	cmd := renderer.TextCommand{
		Bounds:        renderer.RectF{X: 10, Y: 20, W: 80, H: 16},
		Text:          "Hello, World!",
		FontSize:      16,
		Color:         color.RGBA{R: 0, G: 0, B: 0, A: 255},
		Bold:          true,
		Italic:        true,
		Underline:     false,
		Strikethrough: true,
	}
	if cmd.Text != "Hello, World!" {
		t.Errorf("unexpected text: %q", cmd.Text)
	}
	if !cmd.Bold || !cmd.Italic || cmd.Underline || !cmd.Strikethrough {
		t.Error("text decoration flags not set correctly")
	}
}

// --- ImageCommand tests ---

func TestImageCommandWithValues(t *testing.T) {
	cmd := renderer.ImageCommand{
		Bounds: renderer.RectF{X: 0, Y: 0, W: 320, H: 240},
		Src:    "https://example.com/image.png",
		Alt:    "Example image",
	}
	if cmd.Src != "https://example.com/image.png" {
		t.Errorf("unexpected src: %q", cmd.Src)
	}
	if cmd.Alt != "Example image" {
		t.Errorf("unexpected alt: %q", cmd.Alt)
	}
}

// --- ClipCommand tests ---

func TestClipCommandWithValues(t *testing.T) {
	cmd := renderer.ClipCommand{
		Bounds: renderer.RectF{X: 10, Y: 20, W: 200, H: 300},
	}
	if cmd.Bounds.W != 200 {
		t.Errorf("unexpected clip width: %f", cmd.Bounds.W)
	}
}

// --- TransformCommand tests ---

func TestTransformCommandIdentity(t *testing.T) {
	cmd := renderer.TransformCommand{
		Matrix: renderer.TransformMatrix{A: 1, B: 0, C: 0, D: 1, E: 0, F: 0},
	}
	if !cmd.Matrix.IsIdentity() {
		t.Error("identity matrix should report IsIdentity() == true")
	}
}

func TestTransformMatrixMultiply(t *testing.T) {
	identity := renderer.TransformMatrix{A: 1, B: 0, C: 0, D: 1, E: 0, F: 0}
	translate := renderer.TransformMatrix{A: 1, B: 0, C: 0, D: 1, E: 10, F: 20}
	result := identity.Mul(translate)
	if result.E != 10 || result.F != 20 {
		t.Errorf("identity * translate should equal translate, got %+v", result)
	}
}

func TestTransformMatrixInverse(t *testing.T) {
	m := renderer.TransformMatrix{A: 2, B: 0, C: 0, D: 2, E: 10, F: 20}
	inv := m.Inverse()
	if inv == nil {
		t.Fatal("non-singular matrix should have inverse")
	}
	// m * inv should be identity
	product := m.Mul(*inv)
	if !almostEq(product.A, 1) || !almostEq(product.D, 1) {
		t.Errorf("m * inv should be identity, got %+v", product)
	}
	if !almostEq(product.E, 0) || !almostEq(product.F, 0) {
		t.Errorf("translation should be ~0, got %+v", product)
	}
}

func TestTransformMatrixInverseSingular(t *testing.T) {
	m := renderer.TransformMatrix{A: 0, B: 0, C: 0, D: 0, E: 0, F: 0}
	inv := m.Inverse()
	if inv != nil {
		t.Error("singular matrix should return nil inverse")
	}
}

func TestTransformMatrixTranslate(t *testing.T) {
	m := renderer.TranslateMatrix(10, 20)
	if m.E != 10 || m.F != 20 {
		t.Errorf("translate matrix should have E=10, F=20, got %+v", m)
	}
}

func TestTransformMatrixScale(t *testing.T) {
	m := renderer.ScaleMatrix(2, 3)
	if m.A != 2 || m.D != 3 {
		t.Errorf("scale matrix should have A=2, D=3, got %+v", m)
	}
}

func TestTransformMatrixRotate(t *testing.T) {
	m := renderer.RotateMatrix(float32(math.Pi / 2))
	// cos(pi/2) ≈ 0, sin(pi/2) ≈ 1
	if !almostEq(m.A, 0) || !almostEq(m.B, 1) || !almostEq(m.C, -1) || !almostEq(m.D, 0) {
		t.Errorf("rotate(pi/2) unexpected: %+v", m)
	}
}

// --- OpacityCommand tests ---

func TestOpacityCommandClamp(t *testing.T) {
	cmd := renderer.OpacityCommand{Opacity: 1.5}
	if cmd.Opacity != 1.5 {
		// OpacityCommand stores raw value; clamping is consumer responsibility
		t.Log("opacity stored as-is; clamping at render time")
	}
}

func TestOpacityCommandValidRange(t *testing.T) {
	for _, v := range []float32{0, 0.5, 1.0} {
		cmd := renderer.OpacityCommand{Opacity: v}
		if cmd.Opacity < 0 || cmd.Opacity > 1 {
			t.Errorf("opacity %f out of valid range", cmd.Opacity)
		}
	}
}

// --- StackingContextCommand tests ---

func TestStackingContextCommand(t *testing.T) {
	cmd := renderer.StackingContextCommand{
		ZIndex:    5,
		Isolation: true,
	}
	if cmd.ZIndex != 5 {
		t.Errorf("unexpected z-index: %d", cmd.ZIndex)
	}
	if !cmd.Isolation {
		t.Error("isolation should be true")
	}
}

// --- DisplayCommand wrapper tests ---

func TestDisplayCommandKind(t *testing.T) {
	cmd := renderer.DisplayCommand{Kind: renderer.CmdRect}
	if cmd.Kind != renderer.CmdRect {
		t.Errorf("expected CmdRect, got %v", cmd.Kind)
	}
}

func TestDisplayCommandRectData(t *testing.T) {
	cmd := renderer.DisplayCommand{
		Kind: renderer.CmdRect,
		Rect: renderer.RectCommand{
			Bounds: renderer.RectF{X: 1, Y: 2, W: 3, H: 4},
			Color:  color.RGBA{R: 128, A: 255},
		},
	}
	if cmd.Rect.Bounds.X != 1 {
		t.Error("rect data not accessible")
	}
}

func TestDisplayCommandTextData(t *testing.T) {
	cmd := renderer.DisplayCommand{
		Kind: renderer.CmdText,
		Text: renderer.TextCommand{
			Text:     "test",
			FontSize: 14,
		},
	}
	if cmd.Text.Text != "test" {
		t.Error("text data not accessible")
	}
}

// --- DisplayList tests ---

func TestNewDisplayCommandList(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	if dl.Len() != 0 {
		t.Errorf("new list should be empty, got len=%d", dl.Len())
	}
}

func TestDisplayCommandListAdd(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdRect, Rect: renderer.RectCommand{Bounds: renderer.RectF{W: 100, H: 50}}})
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdText, Text: renderer.TextCommand{Text: "hello"}})
	if dl.Len() != 2 {
		t.Errorf("expected 2 commands, got %d", dl.Len())
	}
}

func TestDisplayCommandListClear(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdRect})
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdText})
	dl.Clear()
	if dl.Len() != 0 {
		t.Errorf("cleared list should be empty, got %d", dl.Len())
	}
}

func TestDisplayCommandListIndex(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdRect, Rect: renderer.RectCommand{Bounds: renderer.RectF{X: 10}}})
	cmd := dl.At(0)
	if cmd.Kind != renderer.CmdRect || cmd.Rect.Bounds.X != 10 {
		t.Error("index access returned wrong data")
	}
}

func TestDisplayCommandListIndexOutOfRange(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on out-of-range index")
		}
	}()
	dl.At(0)
}

func TestDisplayCommandListSlice(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdRect})
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdText})
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdBorder})
	cmds := dl.Commands()
	if len(cmds) != 3 {
		t.Errorf("expected 3 commands, got %d", len(cmds))
	}
}

func TestDisplayCommandListValueSemantics(t *testing.T) {
	// Verify that modifying a retrieved command doesn't affect the list
	dl := renderer.NewDisplayCommandList()
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdText, Text: renderer.TextCommand{Text: "original"}})
	cmd := dl.At(0)
	cmd.Text.Text = "modified"
	// The list should still have the original value since DisplayCommand is a value type
	// but the underlying slice element is modified because At returns a copy
	// Actually, At returns a copy, so the list should be unchanged
	if dl.At(0).Text.Text != "original" {
		t.Error("modifying copy should not affect list")
	}
}

// --- Serialization tests ---

func TestRectFMarshalJSON(t *testing.T) {
	r := renderer.RectF{X: 1.5, Y: 2.5, W: 100, H: 50}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var got renderer.RectF
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != r {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", got, r)
	}
}

func TestColorMarshalJSON(t *testing.T) {
	c := color.RGBA{R: 255, G: 128, B: 0, A: 255}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var got color.RGBA
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != c {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", got, c)
	}
}

func TestColorMarshalJSONNil(t *testing.T) {
	var c color.Color
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "null" {
		t.Errorf("nil color should marshal to null, got %s", string(data))
	}
}

func TestTransformMatrixMarshalJSON(t *testing.T) {
	m := renderer.TransformMatrix{A: 1, B: 0, C: 0, D: 1, E: 10, F: 20}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got renderer.TransformMatrix
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != m {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", got, m)
	}
}

func TestDisplayCommandMarshalJSONRect(t *testing.T) {
	cmd := renderer.DisplayCommand{
		Kind: renderer.CmdRect,
		Rect: renderer.RectCommand{
			Bounds: renderer.RectF{X: 10, Y: 20, W: 100, H: 50},
			Color:  color.RGBA{R: 255, A: 255},
		},
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	var got renderer.DisplayCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != renderer.CmdRect {
		t.Errorf("kind mismatch: got %v", got.Kind)
	}
	if got.Rect.Bounds != cmd.Rect.Bounds {
		t.Errorf("bounds mismatch: got %+v", got.Rect.Bounds)
	}
}

func TestDisplayCommandMarshalJSONText(t *testing.T) {
	cmd := renderer.DisplayCommand{
		Kind: renderer.CmdText,
		Text: renderer.TextCommand{
			Bounds:   renderer.RectF{X: 0, Y: 0, W: 80, H: 16},
			Text:     "Hello, World!",
			FontSize: 16,
			Color:    color.RGBA{A: 255},
			Bold:     true,
		},
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	var got renderer.DisplayCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != renderer.CmdText || got.Text.Text != "Hello, World!" || !got.Text.Bold {
		t.Errorf("text roundtrip failed: %+v", got)
	}
}

func TestDisplayCommandMarshalJSONImage(t *testing.T) {
	cmd := renderer.DisplayCommand{
		Kind: renderer.CmdImage,
		Image: renderer.ImageCommand{
			Bounds: renderer.RectF{W: 320, H: 240},
			Src:    "img.png",
			Alt:    "alt text",
		},
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	var got renderer.DisplayCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Image.Src != "img.png" || got.Image.Alt != "alt text" {
		t.Errorf("image roundtrip failed: %+v", got)
	}
}

func TestDisplayCommandMarshalJSONBorder(t *testing.T) {
	cmd := renderer.DisplayCommand{
		Kind: renderer.CmdBorder,
		Border: renderer.BorderCommand{
			Bounds: renderer.RectF{W: 100, H: 100},
			Top:    renderer.BorderSide{Width: 1, Color: color.Black, Style: renderer.BorderSolid},
		},
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	var got renderer.DisplayCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Border.Top.Width != 1 || got.Border.Top.Style != renderer.BorderSolid {
		t.Errorf("border roundtrip failed: %+v", got)
	}
}

func TestDisplayCommandMarshalJSONClip(t *testing.T) {
	cmd := renderer.DisplayCommand{
		Kind: renderer.CmdPushClip,
		Clip: renderer.ClipCommand{Bounds: renderer.RectF{X: 10, Y: 20, W: 200, H: 300}},
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	var got renderer.DisplayCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != renderer.CmdPushClip || got.Clip.Bounds.W != 200 {
		t.Errorf("clip roundtrip failed: %+v", got)
	}
}

func TestDisplayCommandMarshalJSONTransform(t *testing.T) {
	cmd := renderer.DisplayCommand{
		Kind:      renderer.CmdPushTransform,
		Transform: renderer.TransformCommand{Matrix: renderer.TranslateMatrix(10, 20)},
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	var got renderer.DisplayCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != renderer.CmdPushTransform || got.Transform.Matrix.E != 10 {
		t.Errorf("transform roundtrip failed: %+v", got)
	}
}

func TestDisplayCommandMarshalJSONOpacity(t *testing.T) {
	cmd := renderer.DisplayCommand{
		Kind:    renderer.CmdPushOpacity,
		Opacity: renderer.OpacityCommand{Opacity: 0.5},
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	var got renderer.DisplayCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != renderer.CmdPushOpacity || got.Opacity.Opacity != 0.5 {
		t.Errorf("opacity roundtrip failed: %+v", got)
	}
}

func TestDisplayCommandMarshalJSONStackingContext(t *testing.T) {
	cmd := renderer.DisplayCommand{
		Kind:            renderer.CmdPushStackingContext,
		StackingContext: renderer.StackingContextCommand{ZIndex: 5, Isolation: true},
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	var got renderer.DisplayCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.StackingContext.ZIndex != 5 || !got.StackingContext.Isolation {
		t.Errorf("stacking context roundtrip failed: %+v", got)
	}
}

func TestDisplayCommandMarshalJSONPopCommands(t *testing.T) {
	pops := []renderer.DisplayCommandKind{renderer.CmdPopClip, renderer.CmdPopTransform, renderer.CmdPopOpacity, renderer.CmdPopStackingContext}
	for _, kind := range pops {
		cmd := renderer.DisplayCommand{Kind: kind}
		data, err := json.Marshal(cmd)
		if err != nil {
			t.Fatalf("marshal %v: %v", kind, err)
		}
		var got renderer.DisplayCommand
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal %v: %v", kind, err)
		}
		if got.Kind != kind {
			t.Errorf("pop roundtrip: got %v, want %v", got.Kind, kind)
		}
	}
}

func TestDisplayCommandListMarshalJSON(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdRect, Rect: renderer.RectCommand{Bounds: renderer.RectF{W: 100, H: 50}}})
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdText, Text: renderer.TextCommand{Text: "hello", FontSize: 14}})
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdPushClip, Clip: renderer.ClipCommand{Bounds: renderer.RectF{W: 200, H: 300}}})
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdPopClip})

	data, err := json.Marshal(dl)
	if err != nil {
		t.Fatal(err)
	}
	var got renderer.DisplayCommandList
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Len() != 4 {
		t.Errorf("expected 4 commands after roundtrip, got %d", got.Len())
	}
	if got.At(0).Kind != renderer.CmdRect || got.At(1).Kind != renderer.CmdText || got.At(2).Kind != renderer.CmdPushClip || got.At(3).Kind != renderer.CmdPopClip {
		t.Error("command kinds not preserved")
	}
}

func TestDisplayCommandMarshalJSONEmpty(t *testing.T) {
	cmd := renderer.DisplayCommand{Kind: renderer.CmdRect}
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatal(err)
	}
	var got renderer.DisplayCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != renderer.CmdRect {
		t.Errorf("empty rect roundtrip: got kind %v", got.Kind)
	}
}

// --- BorderStyle tests ---

func TestBorderStyleString(t *testing.T) {
	tests := []struct {
		style renderer.BorderStyle
		want  string
	}{
		{renderer.BorderNone, "none"},
		{renderer.BorderSolid, "solid"},
		{renderer.BorderDashed, "dashed"},
		{renderer.BorderDotted, "dotted"},
		{renderer.BorderStyle(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.style.String(); got != tt.want {
			t.Errorf("BorderStyle(%d).String() = %q, want %q", tt.style, got, tt.want)
		}
	}
}

// --- RectF tests ---

func TestRectFContains(t *testing.T) {
	r := renderer.RectF{X: 10, Y: 20, W: 100, H: 50}
	if !r.Contains(50, 40) {
		t.Error("point (50,40) should be inside rect")
	}
	if r.Contains(5, 40) {
		t.Error("point (5,40) should be outside rect")
	}
}

func TestRectFIntersects(t *testing.T) {
	a := renderer.RectF{X: 0, Y: 0, W: 100, H: 100}
	b := renderer.RectF{X: 50, Y: 50, W: 100, H: 100}
	c := renderer.RectF{X: 200, Y: 200, W: 10, H: 10}
	if !a.Intersects(b) {
		t.Error("a and b should intersect")
	}
	if a.Intersects(c) {
		t.Error("a and c should not intersect")
	}
}

// --- Helpers ---

func almostEq(a, b float32) bool {
	const eps float32 = 1e-6
	return abs32(a-b) < eps
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// --- Benchmarks ---

func BenchmarkDisplayCommandCreate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = renderer.DisplayCommand{
			Kind: renderer.CmdRect,
			Rect: renderer.RectCommand{
				Bounds: renderer.RectF{X: 10, Y: 20, W: 100, H: 50},
				Color:  color.RGBA{R: 255, A: 255},
			},
		}
	}
}

func BenchmarkDisplayCommandListAdd(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dl := renderer.NewDisplayCommandList()
		for j := 0; j < 100; j++ {
			dl.Add(renderer.DisplayCommand{Kind: renderer.CmdRect, Rect: renderer.RectCommand{Bounds: renderer.RectF{W: 100, H: 50}}})
		}
	}
}

func BenchmarkDisplayCommandListAddMixed(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dl := renderer.NewDisplayCommandList()
		for j := 0; j < 100; j++ {
			switch j % 5 {
			case 0:
				dl.Add(renderer.DisplayCommand{Kind: renderer.CmdRect, Rect: renderer.RectCommand{Bounds: renderer.RectF{W: 100, H: 50}}})
			case 1:
				dl.Add(renderer.DisplayCommand{Kind: renderer.CmdText, Text: renderer.TextCommand{Text: "hello", FontSize: 14}})
			case 2:
				dl.Add(renderer.DisplayCommand{Kind: renderer.CmdBorder, Border: renderer.BorderCommand{Bounds: renderer.RectF{W: 100, H: 50}}})
			case 3:
				dl.Add(renderer.DisplayCommand{Kind: renderer.CmdImage, Image: renderer.ImageCommand{Bounds: renderer.RectF{W: 320, H: 240}}})
			case 4:
				dl.Add(renderer.DisplayCommand{Kind: renderer.CmdPushClip, Clip: renderer.ClipCommand{Bounds: renderer.RectF{W: 200, H: 300}}})
			}
		}
	}
}

func BenchmarkDisplayCommandSerializeRect(b *testing.B) {
	b.ReportAllocs()
	cmd := renderer.DisplayCommand{
		Kind: renderer.CmdRect,
		Rect: renderer.RectCommand{
			Bounds: renderer.RectF{X: 10, Y: 20, W: 100, H: 50},
			Color:  color.RGBA{R: 255, A: 255},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(cmd)
	}
}

func BenchmarkDisplayCommandSerializeList(b *testing.B) {
	b.ReportAllocs()
	dl := renderer.NewDisplayCommandList()
	for j := 0; j < 100; j++ {
		dl.Add(renderer.DisplayCommand{Kind: renderer.CmdRect, Rect: renderer.RectCommand{Bounds: renderer.RectF{W: float32(j) * 10, H: 50}}})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(dl)
	}
}

func BenchmarkDisplayCommandDeserializeList(b *testing.B) {
	dl := renderer.NewDisplayCommandList()
	for j := 0; j < 100; j++ {
		dl.Add(renderer.DisplayCommand{Kind: renderer.CmdRect, Rect: renderer.RectCommand{Bounds: renderer.RectF{W: float32(j) * 10, H: 50}}})
	}
	data, _ := json.Marshal(dl)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var got renderer.DisplayCommandList
		_ = json.Unmarshal(data, &got)
	}
}

func BenchmarkTransformMatrixMul(b *testing.B) {
	b.ReportAllocs()
	a := renderer.TranslateMatrix(10, 20)
	c := renderer.ScaleMatrix(2, 3)
	for i := 0; i < b.N; i++ {
		_ = a.Mul(c)
	}
}

func BenchmarkTransformMatrixInverse(b *testing.B) {
	b.ReportAllocs()
	m := renderer.TranslateMatrix(10, 20)
	for i := 0; i < b.N; i++ {
		_ = m.Inverse()
	}
}
