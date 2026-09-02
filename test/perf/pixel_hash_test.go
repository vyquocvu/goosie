package perf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestPixelHashManifest(t *testing.T) {
	fixtures := []string{
		"../../testdata/perf/typography_sample.html",
		"../../testdata/perf/layout_sample.html",
		"../../testdata/perf/large_page.html",
	}

	manifestPath := "../../docs/perf/pixel-manifest.json"
	updateManifest := os.Getenv("UPDATE_MANIFEST") == "true"

	var manifest map[string]string
	if !updateManifest {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatalf("failed to read manifest: %v", err)
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatalf("failed to unmarshal manifest: %v", err)
		}
	} else {
		manifest = make(map[string]string)
	}

	for _, fixture := range fixtures {
		htmlContent, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatalf("failed to read fixture %s: %v", fixture, err)
		}

		img, err := renderer.RenderHTMLToImage(context.Background(), string(htmlContent), 800, 600)
		if err != nil {
			t.Fatalf("failed to render %s: %v", fixture, err)
		}

		// Encode to PNG and compute hash
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatalf("failed to encode PNG: %v", err)
		}
		hash := sha256.Sum256(buf.Bytes())
		hashStr := hex.EncodeToString(hash[:])

		key := filepath.Base(fixture)
		if updateManifest {
			manifest[key] = hashStr
		} else {
			expected, ok := manifest[key]
			if !ok {
				t.Errorf("fixture %s not in manifest", key)
				continue
			}
			if hashStr != expected {
				t.Errorf("pixel hash mismatch for %s: got %s, want %s", key, hashStr, expected)
			}
		}
	}

	if updateManifest {
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			t.Fatalf("failed to marshal manifest: %v", err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(manifestPath, data, 0644); err != nil {
			t.Fatalf("failed to write manifest: %v", err)
		}
		t.Logf("manifest updated at %s", manifestPath)
	}
}
