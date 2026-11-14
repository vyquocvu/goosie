package download_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/download"
)

// qc:BR-296
func TestShouldDownloadAcceptsCommaSeparatedContentType(t *testing.T) {
	cases := []struct {
		name string
		ct   string
		want bool
	}{
		{"plain", "text/html", false},
		{"semicolon-attached-param", "text/html;charset=utf-8", false},
		{"semicolon-spaced-param", "text/html; charset=utf-8", false},
		{"comma-mistaken-param", "text/html, charset=UTF-8", false},
		{"xhtml", "application/xhtml+xml", false},
		{"pdf-with-binary-charset", "application/pdf; charset=binary", true},
		{"upper-comma-form", "TEXT/HTML, charset=UTF-8", false},
		{"comma-list-first-downloadable", "application/pdf, text/html", true},
		{"inline-disposition-with-comma", "text/html, charset=UTF-8", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			disp := "inline"
			if got := download.ShouldDownload(c.ct, disp); got != c.want {
				t.Fatalf("ShouldDownload(%q, %q) = %v, want %v", c.ct, disp, got, c.want)
			}
		})
	}
}

// qc:BR-296
func TestShouldDownloadAttachmentStillOverridesContentType(t *testing.T) {
	if !download.ShouldDownload("text/html, charset=UTF-8", "attachment; filename=x") {
		t.Fatalf("Content-Disposition attachment must still force a download despite a displayable comma content type")
	}
}
