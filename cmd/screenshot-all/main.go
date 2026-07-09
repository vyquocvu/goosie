package main

import (
	"context"
	"flag"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/testutil"
)

func main() {
	inputDir := flag.String("input", "testdata", "Input directory containing HTML files")
	outputDir := flag.String("output", "testdata/screenshots", "Output directory for screenshots")
	width := flag.Int("width", 1280, "Viewport width")
	height := flag.Int("height", 800, "Viewport height")
	fullPage := flag.Bool("fullpage", true, "Capture full page")
	flag.Parse()

	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("output dir error: %v", err)
	}

	files := collectHTMLFiles(*inputDir)
	if len(files) == 0 {
		log.Fatalf("no html files found in %s", *inputDir)
	}

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			log.Printf("read error for %s: %v", f, err)
			continue
		}

		r := renderer.NewRenderer(float32(*width), float32(*height))

		abs, err := filepath.Abs(f)
		if err == nil {
			r.SetCurrentURL("file://" + abs)
		}

		obj, err := r.RenderHTML(context.Background(), string(content))
		if err != nil {
			log.Printf("render error for %s: %v", f, err)
			continue
		}

		finalHeight := *height
		if *fullPage {
			h := int(r.GetContentHeight())
			if h > 0 {
				finalHeight = h
			}
		}

		base := filepath.Base(f)
		out := filepath.Join(*outputDir, base[:len(base)-len(filepath.Ext(base))]+".png")

		if err := testutil.SaveRenderedScreenshot(obj, out, *width, finalHeight); err != nil {
			log.Printf("screenshot error for %s: %v", f, err)
		} else {
			log.Printf("saved %s (%sx%s fullpage=%v)", out, strconv.Itoa(*width), strconv.Itoa(finalHeight), *fullPage)
		}
	}
}

func collectHTMLFiles(root string) []string {
	var files []string
	pattern := filepath.Join(root, "test_*.html")
	globbed, _ := filepath.Glob(pattern)
	files = append(files, globbed...)

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".html" && !contains(files, path) {
			files = append(files, path)
		}
		return nil
	})
	return files
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
