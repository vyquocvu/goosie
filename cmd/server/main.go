package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
)

func main() {
	// Custom handler to serve HTML files with proper content type
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Default to index.html if no specific file is requested
		path := r.URL.Path
		if path == "/" || path == "" {
			path = "/console_demo.html"
		}

		// Remove leading slash for file path
		filePath := strings.TrimPrefix(path, "/")

		// Set content type based on file extension
		ext := strings.ToLower(filepath.Ext(filePath))
		switch ext {
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case ".css":
			w.Header().Set("Content-Type", "text/css")
		case ".js":
			w.Header().Set("Content-Type", "application/javascript")
		case ".png":
			w.Header().Set("Content-Type", "image/png")
		case ".jpg", ".jpeg":
			w.Header().Set("Content-Type", "image/jpeg")
		case ".gif":
			w.Header().Set("Content-Type", "image/gif")
		case ".svg":
			w.Header().Set("Content-Type", "image/svg+xml")
		}

		// Serve the file
		http.ServeFile(w, r, "./examples/"+filePath)
	})

	// Create a simple directory listing page
	http.HandleFunc("/files", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Example Files</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        h1 { color: #333; }
        ul { list-style-type: none; padding: 0; }
        li { margin: 10px 0; }
        a { color: #0066cc; text-decoration: none; }
        a:hover { text-decoration: underline; }
        .section { margin: 20px 0; }
        .section h2 { color: #666; border-bottom: 1px solid #ddd; }
    </style>
</head>
<body>
    <h1>Goosie Example Files</h1>
    <p>Click on any file to view it in your browser:</p>
    
    <div class="section">
        <h2>Demo Files</h2>
        <ul>
            <li><a href="/console_demo.html">Console Demo</a> - Demonstrates JavaScript console functionality</li>
            <li><a href="/enhanced_html_demo.html">Enhanced HTML Demo</a> - Shows enhanced HTML support</li>
            <li><a href="/long_page.html">Long Page</a> - Test page for scroll performance</li>
        </ul>
    </div>
    
    <div class="section">
        <h2>HTML Examples</h2>
        <ul>
            <li><a href="/html/css_demo.html">CSS Demo</a> - CSS styling examples</li>
            <li><a href="/html/forms.html">Forms</a> - HTML form elements</li>
            <li><a href="/html/full_css_demo.html">Full CSS Demo</a> - Comprehensive CSS examples</li>
            <li><a href="/html/tables.html">Tables</a> - HTML table examples</li>
        </ul>
    </div>
    
    <div class="section">
        <h2>Go Files (Source)</h2>
        <ul>
            <li><a href="/console_demo.go">console_demo.go</a> - Console demo source</li>
            <li><a href="/dom_api_demo.go">dom_api_demo.go</a> - DOM API demo source</li>
            <li><a href="/font_metrics_demo.go">font_metrics_demo.go</a> - Font metrics demo source</li>
            <li><a href="/image_loading_demo.go">image_loading_demo.go</a> - Image loading demo source</li>
            <li><a href="/scroll_perf_demo.go">scroll_perf_demo.go</a> - Scroll performance demo source</li>
        </ul>
    </div>
</body>
</html>`)
	})

	// Handle static files
	http.Handle("/", handler)

	port := ":8080"
	fmt.Printf("Server starting on http://localhost%s\n", port)
	fmt.Println("Available endpoints:")
	fmt.Println("  /          - Serves console_demo.html (default)")
	fmt.Println("  /files     - Directory listing of all available files")
	fmt.Println("  /*.html    - Serves HTML files from examples directory")
	fmt.Println("  /html/*    - Serves files from examples/html subdirectory")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop the server")

	log.Fatal(http.ListenAndServe(port, nil))
}
