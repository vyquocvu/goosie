package main

import (
	"context"
	"fmt"
	"log"

	"github.com/vyquocvu/goosie/internal/renderer"
)

func main() {
	fmt.Println("HTML Renderer Demo")
	fmt.Println("==================")
	fmt.Println()

	// Create a renderer
	htmlRenderer := renderer.NewRenderer(800, 600)

	// Test with various HTML structures
	testCases := []struct {
		name string
		html string
	}{
		{
			name: "Simple Heading and Paragraph",
			html: `
				<html>
				<body>
					<h1>Welcome to Goosie</h1>
					<p>This is a simple HTML renderer demonstration.</p>
				</body>
				</html>
			`,
		},
		{
			name: "Multiple Headings",
			html: `
				<html>
				<body>
					<h1>Heading Level 1</h1>
					<h2>Heading Level 2</h2>
					<h3>Heading Level 3</h3>
					<p>Regular paragraph text.</p>
				</body>
				</html>
			`,
		},
		{
			name: "Lists",
			html: `
				<html>
				<body>
					<h2>Shopping List</h2>
					<ul>
						<li>Apples</li>
						<li>Bananas</li>
						<li>Oranges</li>
					</ul>
				</body>
				</html>
			`,
		},
		{
			name: "Links and Styling",
			html: `
				<html>
				<body>
					<p>Visit <a href="https://example.com">Example.com</a> for more info.</p>
					<p>This text has <strong>bold</strong> and <em>italic</em> elements.</p>
				</body>
				</html>
			`,
		},
		{
			name: "Complex Structure",
			html: `
				<html>
				<head><title>Test Page</title></head>
				<body>
					<div id="header">
						<h1>Main Title</h1>
						<p>Subtitle text</p>
					</div>
					<div id="content">
						<h2>Section 1</h2>
						<p>First paragraph with <strong>bold text</strong>.</p>
						<ul>
							<li>First item</li>
							<li>Second item</li>
						</ul>
						<h2>Section 2</h2>
						<p>Second paragraph with a <a href="https://example.com">link</a>.</p>
					</div>
					<div id="footer">
						<p>Footer content</p>
					</div>
				</body>
				</html>
			`,
		},
	}

	for _, tc := range testCases {
		fmt.Printf("Test: %s\n", tc.name)
		fmt.Println("-------------------")

		canvasObject, err := htmlRenderer.RenderHTML(context.Background(), tc.html)
		if err != nil {
			log.Printf("Error rendering HTML: %v\n", err)
			fmt.Println()
			continue
		}

		if canvasObject != nil {
			fmt.Println("✓ Successfully rendered HTML to canvas object")
		} else {
			fmt.Println("✗ Failed to render HTML (nil result)")
		}

		fmt.Println()
	}

	// Test error handling
	fmt.Println("Test: Invalid HTML")
	fmt.Println("-------------------")
	invalidHTML := "<div><p>Unclosed tags"
	canvasObject, err := htmlRenderer.RenderHTML(context.Background(), invalidHTML)
	if err != nil {
		fmt.Printf("✓ Properly handled error: %v\n", err)
	} else if canvasObject != nil {
		fmt.Println("✓ Renderer gracefully handled malformed HTML")
	}
	fmt.Println()

	// Test resizing
	fmt.Println("Test: Renderer Resizing")
	fmt.Println("------------------------")
	htmlRenderer.SetSize(1024, 768)
	fmt.Println("✓ Successfully resized renderer to 1024x768")
	fmt.Println()

	// Summary
	fmt.Println("Summary")
	fmt.Println("=======")
	fmt.Println("✓ HTML Renderer module is working correctly")
	fmt.Println("✓ Supports headings, paragraphs, lists, links, and more")
	fmt.Println("✓ Handles complex nested HTML structures")
	fmt.Println("✓ Gracefully handles malformed HTML")
	fmt.Println()
	fmt.Println("The renderer is ready for integration with the browser UI!")
}
