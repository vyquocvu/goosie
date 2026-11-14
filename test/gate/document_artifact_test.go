package gate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

const (
	gateDocWidth        = 800
	gateDocHeight       = 600
	gateDocDPR          = 2
	gateFixtureBlocks   = 40
	gateFixtureBlockPx  = 200
	gateFixtureCSSPx    = gateFixtureBlocks * gateFixtureBlockPx
	gateViewportDeviceP = gateDocHeight * gateDocDPR
)

func tallFixture(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("<!doctype html><html><head><style>")
	b.WriteString(fmt.Sprintf("div{height:%dpx;background:#abcdef}", gateFixtureBlockPx))
	b.WriteString("</style></head><body>")
	for i := 0; i < gateFixtureBlocks; i++ {
		fmt.Fprintf(&b, "<div>block %d</div>", i)
	}
	b.WriteString("</body></html>")
	path := filepath.Join(t.TempDir(), "tall.html")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return "file://" + filepath.ToSlash(path)
}

func reportString(t *testing.T, report map[string]any, key string) string {
	t.Helper()
	v, ok := report[key]
	if !ok {
		t.Fatalf("the artifact has no %q key", key)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("%q holds %v (%T), want a string", key, v, v)
	}
	return s
}

func reportInt(t *testing.T, report map[string]any, key string) int64 {
	t.Helper()
	v, ok := report[key]
	if !ok {
		t.Fatalf("the artifact has no %q key", key)
	}
	n, ok := v.(float64)
	if !ok {
		t.Fatalf("%q holds %v (%T), want a number", key, v, v)
	}
	return int64(n)
}

func TestGateArtifactNamesTheDocumentItDrew(t *testing.T) {
	url := tallFixture(t)
	report := runArtifact(t, "-bench", "-backend", "headless", "-url", url,
		"-width", fmt.Sprint(gateDocWidth), "-height", fmt.Sprint(gateDocHeight),
		"-dpr", fmt.Sprint(gateDocDPR), "-frames", "12")

	if got := reportString(t, report, "document"); got != url {
		t.Errorf("document = %q, want the -url the run was handed (%q): an artifact that names "+
			"no document cannot be attributed to the page its timings came from", got, url)
	}
	if got, want := reportInt(t, report, "doc_height"), int64(gateFixtureCSSPx*gateDocDPR); got < want/2 {
		t.Errorf("doc_height = %d device px, want at least half the fixture's %d: the run measured a "+
			"blank layer rather than the document", got, want)
	}
	if got := reportInt(t, report, "tile_budget"); got > engine.MaxTileCacheBytes {
		t.Errorf("tile_budget = %d bytes, over the %d-byte cap a real document's cache is held to",
			got, int64(engine.MaxTileCacheBytes))
	}
	if held, budget := reportInt(t, report, "tile_bytes"), reportInt(t, report, "tile_budget"); held > budget {
		t.Errorf("tile_bytes %d exceeds tile_budget %d on a document run", held, budget)
	}
	t.Logf("document run: %q doc_height=%v tile_bytes=%v of %v, rasterized=%v",
		url, report["doc_height"], report["tile_bytes"], report["tile_budget"], report["tiles_rasterized"])
}

func TestGateSceneArtifactReportsNoDocumentAndItsOwnHeight(t *testing.T) {
	report := runBench(t)
	if got := reportString(t, report, "document"); got != "" {
		t.Errorf("document = %q on a -scene run, want empty: only a -url run drew a document", got)
	}
	if got, want := reportInt(t, report, "doc_height"), int64(paint.DefaultDocHeight); got != want {
		t.Errorf("doc_height = %d, want the scene's %d", got, want)
	}
	if got := reportInt(t, report, "tile_budget"); got <= engine.MaxTileCacheBytes {
		t.Errorf("tile_budget = %d on a -scene run: the gate scene's cache is sized to its whole %d px document, above the %d-byte document cap",
			got, frame.TileSize, int64(engine.MaxTileCacheBytes))
	}
}
