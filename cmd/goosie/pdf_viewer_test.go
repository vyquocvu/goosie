package main

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/tabs"
)

func newPDFTestPath(t *testing.T) *framePath {
	t.Helper()
	spec := paint.SceneSpec{DocHeight: 256}
	_, layer := paint.BuildLayer(spec)
	pool := raster.New(1, 4, func(j raster.Job) error { return nil })
	pool.Start(context.Background())
	t.Cleanup(func() { _ = pool.Close() })
	sched := raster.NewScheduler(layer, pool, frame.Viewport{Size: frame.Size{W: 256, H: 256}}, 1, raster.Pref{})

	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	f := &framePath{
		sched:      sched,
		pool:       pool,
		navResults: make(chan navResult, 16),
		imgResults: make(chan imgResult, 16),
		zoom:       1.0,
		fonts:      fonts,
		client:     net.DefaultClient(),
	}
	f.tabMgr = tabs.NewManager(nil)
	f.tabMgr.NewTab().SetLayer(layer)
	return f
}

func buildViewerTestPDF(title, content string) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>\nendobj\n")

	streamContent := fmt.Sprintf("BT\n/F1 12 Tf\n(%s) Tj\nET\n", content)
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	_, _ = zw.Write([]byte(streamContent))
	_ = zw.Close()
	compressed := zbuf.Bytes()

	buf.WriteString(fmt.Sprintf("4 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n", len(compressed)))
	buf.Write(compressed)
	buf.WriteString("\nendstream\nendobj\n")

	buf.WriteString(fmt.Sprintf("5 0 obj\n<< /Title (%s) >>\nendobj\n", title))
	buf.WriteString("xref\n0 6\ntrailer\n<< /Root 1 0 R /Info 5 0 R >>\nstartxref\n500\n%%EOF\n")
	return buf.Bytes()
}

// TestPDFViewer_NavigationRendersViewer tests that navigating to a PDF URL
// renders the PDF document directly inside the browser using the built-in viewer.
func TestPDFViewer_NavigationRendersViewer(t *testing.T) {
	pdfBytes := buildViewerTestPDF("Research Paper", "Engine Conformance Report")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfBytes)
	}))
	defer ts.Close()

	f := newPDFTestPath(t)
	layer, _, _, sess, _, err := f.loadDocumentCtx(context.Background(), f.client, ts.URL+"/report.pdf", 800, 600, 1.0, t.TempDir(), false, false)
	if err != nil {
		t.Fatalf("loadDocumentCtx: %v", err)
	}
	if layer == nil || sess == nil {
		t.Fatal("expected non-nil layer and session for PDF document")
	}
	defer sess.Close()

	// Verify rendered session represents the viewer
	h1Text, err := sess.EvaluateJS(`document.querySelector(".pdf-title").textContent`)
	if err != nil {
		t.Fatalf("EvaluateJS title: %v", err)
	}
	if h1Text != "Research Paper" {
		t.Errorf("PDF title = %q, want 'Research Paper'", h1Text)
	}

	metaText, err := sess.EvaluateJS(`document.querySelector(".pdf-meta").textContent`)
	if err != nil {
		t.Fatalf("EvaluateJS meta: %v", err)
	}
	if !strings.Contains(metaText, "1 Pages") {
		t.Errorf("PDF meta = %q, want '1 Pages'", metaText)
	}
}

// TestPDFViewer_AttachmentDispositionDownloads tests that Content-Disposition: attachment
// forces a file download instead of opening the viewer.
func TestPDFViewer_AttachmentDispositionDownloads(t *testing.T) {
	pdfBytes := buildViewerTestPDF("Invoice", "Total Due: $0")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="invoice.pdf"`)
		_, _ = w.Write(pdfBytes)
	}))
	defer ts.Close()

	f := newPDFTestPath(t)
	downloadDir := t.TempDir()
	_, _, _, _, _, err := f.loadDocumentCtx(context.Background(), f.client, ts.URL+"/invoice.pdf", 800, 600, 1.0, downloadDir, false, false)

	var dd downloadDone
	if !errors.As(err, &dd) {
		t.Fatalf("expected downloadDone error for attachment disposition, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(downloadDir, "invoice.pdf")); err != nil {
		t.Errorf("expected downloaded invoice.pdf to exist: %v", err)
	}
}
