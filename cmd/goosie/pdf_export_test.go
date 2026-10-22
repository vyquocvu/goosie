package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/raster"
)

func TestExportSessionToPDF(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("no fonts: %v", err)
	}

	html := `<!DOCTYPE html>
<html>
<head>
<style>
  body { font-family: sans-serif; margin: 20px; background-color: #fdfdfd; color: #222; }
  h1 { color: #1a73e8; }
  p { line-height: 1.6; }
  .box { height: 600px; background: #e8f0fe; margin-bottom: 20px; padding: 15px; border-radius: 8px; }
  .box2 { height: 800px; background: #fef7e0; padding: 15px; }
</style>
</head>
<body>
  <h1>Goosie PDF Export Test</h1>
  <p>This is a multi-page document rendered by Goosie's pure Go PDF exporter engine.</p>
  <div class="box">Section 1 content spanning page 1.</div>
  <div class="box2">Section 2 content flowing onto page 2.</div>
</body>
</html>`

	sess, err := engine.NewSession(html, nil, 800, engine.WithMetrics(fonts))
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer sess.Close()

	outPath := filepath.Join(t.TempDir(), "test_output.pdf")
	if err := ExportSessionToPDF(sess, fonts, 1.5, outPath); err != nil {
		t.Fatalf("ExportSessionToPDF failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read exported PDF: %v", err)
	}

	// 1. Verify standard PDF 1.4 header
	if !bytes.HasPrefix(data, []byte("%PDF-1.4")) {
		t.Fatalf("PDF missing %%PDF-1.4 header: %s", string(data[:20]))
	}

	// 2. Verify standard trailer and EOF
	if !strings.HasSuffix(strings.TrimSpace(string(data)), "%%EOF") {
		t.Fatalf("PDF missing %%%%EOF trailer")
	}

	// 3. Verify PDF structure elements
	contentStr := string(data)
	if !strings.Contains(contentStr, "/Type /Catalog") {
		t.Fatal("PDF missing Catalog object")
	}
	if !strings.Contains(contentStr, "/Type /Pages") {
		t.Fatal("PDF missing Pages object")
	}
	if !strings.Contains(contentStr, "/Type /Page") {
		t.Fatal("PDF missing Page object")
	}
	if !strings.Contains(contentStr, "/Type /XObject") || !strings.Contains(contentStr, "/Subtype /Image") {
		t.Fatal("PDF missing Image XObject")
	}
	if !strings.Contains(contentStr, "xref") {
		t.Fatal("PDF missing cross-reference table")
	}

	// 4. Verify pagination: height of document is > 1400px so it spans at least 2 pages
	pageMatches := strings.Count(contentStr, "/Type /Page\n")
	if pageMatches < 2 {
		t.Fatalf("expected at least 2 pages for 1400px+ document, got %d", pageMatches)
	}
}

func TestExportSessionToPDF_NilSession(t *testing.T) {
	err := ExportSessionToPDF(nil, nil, 1, "/tmp/should_fail.pdf")
	if err == nil {
		t.Fatal("expected error on nil session")
	}
}

func TestPrintPDFCommandFlag(t *testing.T) {
	outPDF := filepath.Join(t.TempDir(), "cli_export.pdf")
	testHTML := "data:text/html,<html><body><h1>CLI PDF Test</h1><p>Exported directly via command flag</p></body></html>"

	err := run([]string{"-print-pdf", outPDF, "-url", testHTML, "-backend", "headless"})
	if err != nil {
		t.Fatalf("run with -print-pdf failed: %v", err)
	}

	info, err := os.Stat(outPDF)
	if err != nil {
		t.Fatalf("PDF file was not created: %v", err)
	}
	if info.Size() < 100 {
		t.Fatalf("PDF file suspiciously small: %d bytes", info.Size())
	}
}
