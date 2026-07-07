package main

import (
	"fmt"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/testutil"
	"log"
	"os"
	"path/filepath"
)

var (
	passedCount int
	failedCount int
)

func main() {
	fmt.Println("=== Goosie Roadmap Feature Verification ===")

	// Ensure output directory exists
	outputDir := "roadmap_test_output"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	testPhase1()
	testPhase2()
	testPhase3()
	testPhase4(outputDir)

	fmt.Println("\n=== Verification Summary ===")
	fmt.Printf("Total Tests: %d\n", passedCount+failedCount)
	fmt.Printf("Passed:      %d\n", passedCount)
	fmt.Printf("Failed:      %d\n", failedCount)

	if failedCount > 0 {
		os.Exit(1)
	}
}

func report(name string, success bool, details string) {
	if success {
		fmt.Printf("✅ %-40s: %s\n", name, details)
		passedCount++
	} else {
		fmt.Printf("❌ %-40s: %s\n", name, details)
		failedCount++
	}
}

// Phase 1: Essential Browser Features
func testPhase1() {
	fmt.Println("\n--- Phase 1: Essential Browser Features ---")

	// 1. HTTP Fetch
	fetcher := net.NewFetcher()
	htmlContent, err := fetcher.Fetch("https://example.com")
	if err != nil {
		report("HTTP Fetching", false, fmt.Sprintf("Failed: %v", err))
		// Use mock content if fetch fails to allow other tests to proceed
		htmlContent = `<html><body><h1>Mock Content</h1></body></html>`
	} else {
		report("HTTP Fetching", true, fmt.Sprintf("Fetched %d bytes", len(htmlContent)))
	}

	// 2. HTML Parsing
	parser := dom.NewParser()
	bodyText, err := parser.ParseBodyText(htmlContent)
	if err != nil {
		report("HTML Parsing", false, fmt.Sprintf("Failed: %v", err))
	} else {
		report("HTML Parsing", true, fmt.Sprintf("Parsed body text length: %d", len(bodyText)))
	}

	// 3. JS Execution
	runtime := js.NewRuntime()
	// runtime.SetHTMLContent(htmlContent) // Optional: Set content context for JS
	val, err := runtime.RunScript("1 + 1")
	if err != nil {
		report("JS Execution", false, fmt.Sprintf("Failed: %v", err))
	} else {
		result := val.String()
		if result == "2" {
			report("JS Execution", true, "1 + 1 = 2")
		} else {
			report("JS Execution", false, fmt.Sprintf("Expected '2', got '%s'", result))
		}
	}
}

// Phase 2: Enhanced JavaScript Support
func testPhase2() {
	fmt.Println("\n--- Phase 2: Enhanced JavaScript Support ---")

	runtime := js.NewRuntime()

	// 1. Console Support
	// Capture console output usually involves more complex redirection or checking internal buffer
	// For this test, we verify that calling these functions doesn't crash the program
	script := `
		console.log("Log test");
		console.info("Info test");
		console.warn("Warn test");
		console.error("Error test");
		console.table([{a:1, b:2}, {a:3, b:4}]);
	`
	_, err := runtime.RunScript(script)
	report("Console API", err == nil, "Console methods executed without error")

	// 2. Error Reporting
	_, err = runtime.RunScript("syntax error ???")
	if err != nil {
		report("JS Error Reporting", true, fmt.Sprintf("Correctly caught error: %v", err))
	} else {
		report("JS Error Reporting", false, "Failed to catch syntax error")
	}
}

// Phase 3: Advanced Features (CSS, Layout)
func testPhase3() {
	fmt.Println("\n--- Phase 3: Advanced Features ---")

	// 1. CSS Parser
	cssContent := `
		body { margin: 0; padding: 0; }
		.container { display: flex; flex-direction: row; }
		.item { width: 100px; height: 100px; background-color: red; }
		@media (max-width: 600px) { .item { background-color: blue; } }
	`
	parser := css.NewParser(cssContent)
	stylesheet, err := parser.Parse()
	if err != nil {
		report("CSS Parser", false, fmt.Sprintf("Failed: %v", err))
	} else {
		report("CSS Parser", true, fmt.Sprintf("Parsed %d rules", len(stylesheet.Rules)))
	}

	// 2. Box Model & Flexbox (Indirectly via Layout Tree)
	// Create a simple DOM structure
	html := `
		<html>
		<head>
			<style>
				.flex-container { display: flex; width: 500px; }
				.flex-item { flex: 1; height: 100px; }
			</style>
		</head>
		<body>
			<div class="flex-container">
				<div class="flex-item">Item 1</div>
				<div class="flex-item">Item 2</div>
			</div>
		</body>
		</html>
	`

	// Create renderer and build layout
	r := renderer.NewRenderer(800, 600)
	layoutRoot, err := r.RenderHTML(html)
	if err != nil {
		report("Layout Engine", false, fmt.Sprintf("Failed to render: %v", err))
		return
	}

	// Verify Flexbox layout application
	// We need to traverse the layout tree to find the flex items and check their widths
	// This is a simplified check assuming the renderer correctly laid out the items
	if layoutRoot != nil {
		report("Layout Engine", true, "Layout tree generated successfully")

		// In a real deep verify, we would check:
		// - Container width is 500
		// - Item widths are 250 (since flex: 1 and count is 2)
		// For now, checks that we have a valid layout tree structure is a good step.
	} else {
		report("Layout Engine", false, "Layout root is nil")
	}
}

// Phase 4: Developer Tools
func testPhase4(outputDir string) {
	fmt.Println("\n--- Phase 4: Developer Tools ---")

	// 1. Screenshot Capability
	html := `
		<div style="background-color: #f0f0f0; padding: 20px; font-family: sans-serif;">
			<h1 style="color: #333;">Roadmap Test</h1>
			<p>This is a test of the screenshot capability.</p>
			<div style="display: flex; gap: 10px;">
				<div style="background: red; width: 50px; height: 50px;"></div>
				<div style="background: green; width: 50px; height: 50px;"></div>
				<div style="background: blue; width: 50px; height: 50px;"></div>
			</div>
		</div>
	`

	r := renderer.NewRenderer(800, 600)
	obj, err := r.RenderHTML(html)
	if err != nil {
		report("Renderer", false, fmt.Sprintf("Failed to render HTML for screenshot: %v", err))
		return
	}

	filename := filepath.Join(outputDir, "test_screenshot.png")
	err = testutil.SaveRenderedScreenshot(obj, filename, 800, 600)
	if err != nil {
		report("Screenshot", false, fmt.Sprintf("Failed to save screenshot: %v", err))
	} else {
		report("Screenshot", true, fmt.Sprintf("Saved to %s", filename))
	}
}
