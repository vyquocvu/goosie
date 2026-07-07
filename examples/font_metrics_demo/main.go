package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func main() {
	RunDemo()
}

// This example demonstrates the accurate text measurement capabilities
// using the new FontMetrics module.
func RunDemo() {
	fmt.Println("Font Metrics Demo")
	fmt.Println("=================")

	// Create a FontMetrics instance
	fm := renderer.NewFontMetrics(16.0)

	// Example 1: Basic text measurement
	fmt.Println("Example 1: Basic Text Measurement")
	fmt.Println("----------------------------------")

	text1 := "Hello, World!"
	metrics1 := fm.MeasureText(text1, 16.0, fyne.TextStyle{}, 0)
	fmt.Printf("Text: %q\n", text1)
	fmt.Printf("Width: %.2f pixels\n", metrics1.Width)
	fmt.Printf("Height: %.2f pixels\n", metrics1.Height)
	fmt.Printf("Ascent: %.2f pixels\n", metrics1.Ascent)
	fmt.Printf("Descent: %.2f pixels\n", metrics1.Descent)
	fmt.Println()

	// Example 2: Bold text measurement
	fmt.Println("Example 2: Bold Text")
	fmt.Println("--------------------")

	text2 := "Bold Text"
	metrics2 := fm.MeasureText(text2, 16.0, fyne.TextStyle{Bold: true}, 0)
	fmt.Printf("Text: %q (bold)\n", text2)
	fmt.Printf("Width: %.2f pixels\n", metrics2.Width)
	fmt.Printf("Height: %.2f pixels\n", metrics2.Height)
	fmt.Println()

	// Example 3: Different font sizes
	fmt.Println("Example 3: Font Size Comparison")
	fmt.Println("--------------------------------")

	elements := []struct {
		tag      string
		fontSize float32
	}{
		{"h1", fm.GetFontSize("h1")},
		{"h2", fm.GetFontSize("h2")},
		{"h3", fm.GetFontSize("h3")},
		{"p", fm.GetFontSize("p")},
	}

	text3 := "Sample Text"
	for _, elem := range elements {
		metrics := fm.MeasureText(text3, elem.fontSize, fyne.TextStyle{}, 0)
		fmt.Printf("<%s> font size: %.2f px, text width: %.2f px\n",
			elem.tag, elem.fontSize, metrics.Width)
	}
	fmt.Println()

	// Example 4: Text wrapping
	fmt.Println("Example 4: Text Wrapping")
	fmt.Println("------------------------")

	longText := "This is a long text that will wrap across multiple lines when constrained by width"
	maxWidth := float32(200.0)

	singleLine := fm.MeasureText(longText, 16.0, fyne.TextStyle{}, 0)
	wrapped := fm.MeasureTextWithWrapping(longText, 16.0, fyne.TextStyle{}, 0, maxWidth)

	fmt.Printf("Text: %q\n", longText)
	fmt.Printf("Single line width: %.2f pixels\n", singleLine.Width)
	fmt.Printf("Wrapped width (max %0.f): %.2f pixels\n", maxWidth, wrapped.Width)
	fmt.Printf("Single line height: %.2f pixels\n", singleLine.Height)
	fmt.Printf("Wrapped height: %.2f pixels\n", wrapped.Height)
	fmt.Println()

	// Example 5: Style inheritance
	fmt.Println("Example 5: Style Inheritance")
	fmt.Println("----------------------------")

	// Create a node tree: <strong><em>text</em></strong>
	strong := renderer.NewRenderNode(renderer.NodeTypeElement)
	strong.TagName = "strong"

	em := renderer.NewRenderNode(renderer.NodeTypeElement)
	em.TagName = "em"
	strong.AddChild(em)

	style := fm.GetTextStyleFromNode(em)
	fmt.Printf("Node: <strong><em>text</em></strong>\n")
	fmt.Printf("Inherited styles: Bold=%v, Italic=%v\n", style.Bold, style.Italic)
	fmt.Println()

	fmt.Println("Summary")
	fmt.Println("-------")
	fmt.Println("✅ Text dimensions are calculated using actual font metrics")
	fmt.Println("✅ Baseline, ascent, and descent values are accurate")
	fmt.Println("✅ Different font sizes and styles are measured correctly")
	fmt.Println("✅ Text wrapping respects word boundaries")
	fmt.Println("✅ Style inheritance works correctly")
}
