package main

import (
	"context"
	"testing"

	"github.com/vyquocvu/goosie/internal/raster"
)

// qc:BR-297
func TestDocumentReadsPlumbedDevicePixelRatio(t *testing.T) {
	const pageURL = "https://dpr.example.test/"
	client := &recordingClient{docs: map[string]string{
		pageURL: `<!doctype html><html><body><script>
			document.body.setAttribute('data-dpr', String(window.devicePixelRatio));
		</script></body></html>`,
	}}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("create fonts: %v", err)
	}
	for _, scale := range []float32{1, 2} {
		_, _, _, sess, _, err := loadURLCtx(context.Background(), client, fonts, pageURL, 800, 600, scale, t.TempDir(), false, false)
		if err != nil {
			t.Fatalf("loadURLCtx at scale %v: %v", scale, err)
		}
		got, err := sess.EvaluateJS(`document.body.getAttribute('data-dpr')`)
		sess.Close()
		if err != nil {
			t.Fatalf("read data-dpr at scale %v: %v", scale, err)
		}
		want := "1"
		if scale == 2 {
			want = "2"
		}
		if got != want {
			t.Fatalf("page read window.devicePixelRatio = %q at -dpr %v, want %q", got, scale, want)
		}
	}
}
