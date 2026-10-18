package pdf_test

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/pdf"
)

func TestIsPDF(t *testing.T) {
	if !pdf.IsPDF([]byte("%PDF-1.4\n..."), "", "") {
		t.Error("expected IsPDF=true for %PDF- prefix")
	}
	if !pdf.IsPDF([]byte("not-header"), "application/pdf", "") {
		t.Error("expected IsPDF=true for application/pdf")
	}
	if !pdf.IsPDF([]byte("not-header"), "", "https://example.com/doc.pdf") {
		t.Error("expected IsPDF=true for .pdf extension")
	}
	if pdf.IsPDF([]byte("<!DOCTYPE html>"), "text/html", "https://example.com/index.html") {
		t.Error("expected IsPDF=false for HTML")
	}
}

func buildTestPDF(title, text string, compressed bool) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>\nendobj\n")

	content := fmt.Sprintf("BT\n/F1 12 Tf\n(%s) Tj\nET\n", text)
	if compressed {
		var zbuf bytes.Buffer
		zw := zlib.NewWriter(&zbuf)
		_, _ = zw.Write([]byte(content))
		_ = zw.Close()
		compressedBytes := zbuf.Bytes()

		buf.WriteString(fmt.Sprintf("4 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n", len(compressedBytes)))
		buf.Write(compressedBytes)
		buf.WriteString("\nendstream\nendobj\n")
	} else {
		buf.WriteString(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", len(content), content))
	}

	buf.WriteString(fmt.Sprintf("5 0 obj\n<< /Title (%s) >>\nendobj\n", title))
	buf.WriteString("xref\n0 6\ntrailer\n<< /Root 1 0 R /Info 5 0 R >>\nstartxref\n500\n%%EOF\n")
	return buf.Bytes()
}

func TestParse_CompressedStream(t *testing.T) {
	data := buildTestPDF("Goosie Spec", "Pure Go Web Browser Engine", true)
	doc, err := pdf.Parse(data, "Untitled")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if doc.Title != "Goosie Spec" {
		t.Errorf("Title = %q, want 'Goosie Spec'", doc.Title)
	}
	if len(doc.Pages) == 0 {
		t.Fatal("expected at least 1 page")
	}

	found := false
	for _, l := range doc.Pages[0].Lines {
		if strings.Contains(l, "Pure Go Web Browser Engine") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected extracted line 'Pure Go Web Browser Engine', got lines: %v", doc.Pages[0].Lines)
	}
}

func TestParse_UncompressedText(t *testing.T) {
	data := buildTestPDF("Uncompressed Report", "Direct Text Stream", false)
	doc, err := pdf.Parse(data, "Fallback")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if doc.Title != "Uncompressed Report" {
		t.Errorf("Title = %q, want 'Uncompressed Report'", doc.Title)
	}

	found := false
	for _, l := range doc.Pages[0].Lines {
		if strings.Contains(l, "Direct Text Stream") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected extracted line 'Direct Text Stream', got lines: %v", doc.Pages[0].Lines)
	}
}

func TestRenderHTML(t *testing.T) {
	doc := &pdf.Document{
		Title:   "Annual Report",
		Version: "1.4",
		Pages: []pdf.Page{
			{Number: 1, Lines: []string{"Executive Summary", "Revenue is up 20%."}},
			{Number: 2, Lines: []string{"Conclusion"}},
		},
	}

	htmlContent := pdf.RenderHTML(doc, "https://example.com/report.pdf")
	if !strings.Contains(htmlContent, "Annual Report") {
		t.Error("expected document title in HTML")
	}
	if !strings.Contains(htmlContent, "2 Pages") {
		t.Error("expected page count in toolbar")
	}
	if !strings.Contains(htmlContent, "Executive Summary") {
		t.Error("expected page 1 text in HTML")
	}
	if !strings.Contains(htmlContent, "Conclusion") {
		t.Error("expected page 2 text in HTML")
	}
}
