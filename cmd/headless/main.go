package main

import (
	"context"
	"flag"
	"fmt"
	"image/png"
	"io"
	"log"
	"os"

	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/version"
)

func main() {
	htmlPath := flag.String("html", "", "Path to HTML file (default: read from stdin)")
	outputPath := flag.String("output", "output.png", "Path to save the PNG output")
	width := flag.Int("width", 800, "Viewport width in pixels")
	height := flag.Int("height", 600, "Viewport height in pixels")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	var htmlContent string
	if *htmlPath != "" {
		data, err := os.ReadFile(*htmlPath)
		if err != nil {
			log.Fatalf("reading HTML file: %v", err)
		}
		htmlContent = string(data)
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalf("reading stdin: %v", err)
		}
		htmlContent = string(data)
	}

	if htmlContent == "" {
		log.Fatal("no HTML content provided")
	}

	img, err := renderer.RenderHTMLToImage(context.Background(), htmlContent, *width, *height)
	if err != nil {
		log.Fatalf("rendering HTML: %v", err)
	}

	f, err := os.Create(*outputPath)
	if err != nil {
		log.Fatalf("creating output file: %v", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		log.Fatalf("encoding PNG: %v", err)
	}

	fmt.Fprintf(os.Stderr, "Wrote %dx%d PNG to %s\n", img.Bounds().Dx(), img.Bounds().Dy(), *outputPath)
}
