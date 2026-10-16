package gate

// What a run drew, as a figure in the artifact rather than a line on stderr.
//
// Until now the JSON carried timings and counters with no statement of what they were
// taken on, and `cmd/goosie/main.go` skipped `-url` for every paced run: a `-bench -url X`
// invocation exited 0 with an artifact full of plausible numbers measured on the synthetic
// checkerboard, and only its stderr `scene=` said so. That is the shape of a memory gate
// that can be satisfied by the wrong page, so the artifact names its content and reports
// the height that content was laid out at.

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

// gateDocViewport is the geometry the document case runs at: small enough that a real
// document's height is many times the viewport, and the difference cannot be a rounding
// artefact of a page that happens to fill one screen.
const (
	gateDocWidth  = 800
	gateDocHeight = 600
	gateDocDPR    = 2
	// gateFixtureBlocks x gateFixtureBlockPx is the fixture's CSS height.
	gateFixtureBlocks   = 40
	gateFixtureBlockPx  = 200
	gateFixtureCSSPx    = gateFixtureBlocks * gateFixtureBlockPx
	gateViewportDeviceP = gateDocHeight * gateDocDPR
)

// tallFixture writes a document ten viewport-heights tall and returns its file:// URL.
// Height rather than content is what makes the assertions below about this page: a blank
// layer's document height is exactly the viewport's, so a run that ignored the URL cannot
// produce the number the fixture implies.
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

// reportString reads a JSON string field, failing rather than treating absence as "".
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

// reportInt reads a JSON number field, failing rather than treating null as 0. A null here
// is a build that stopped measuring, and 0 is a figure that can pass a comparison.
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
	// The claim behind the name: the fixture's CSS height at DPR 2, against a viewport
	// that is a tenth of it. A paced run that dropped the URL draws a page exactly one
	// viewport tall, so this is the assertion that fails for it.
	if got, want := reportInt(t, report, "doc_height"), int64(gateFixtureCSSPx*gateDocDPR); got < want/2 {
		t.Errorf("doc_height = %d device px, want at least half the fixture's %d: the run measured a "+
			"blank layer rather than the document", got, want)
	}
	// The capped path is the point of running a document at all: its cache is authorised
	// by engine.TileCacheBudget, not by the synthetic scene's document-wide budget, so this
	// is the artifact a "lightweight" fence can be set against.
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

// The other direction: a synthetic scene run says so by leaving `document` empty and
// reporting the scene's own height, so the field distinguishes the two content sources
// rather than merely being present.
func TestGateSceneArtifactReportsNoDocumentAndItsOwnHeight(t *testing.T) {
	report := runBench(t)
	if got := reportString(t, report, "document"); got != "" {
		t.Errorf("document = %q on a -scene run, want empty: only a -url run drew a document", got)
	}
	if got, want := reportInt(t, report, "doc_height"), int64(paint.DefaultDocHeight); got != want {
		t.Errorf("doc_height = %d, want the scene's %d", got, want)
	}
	// The scene's budget is deliberately document-wide, so it is the one figure that must
	// NOT look like the capped path; pinning it here is what keeps the assertion above
	// honest about which of the two budgets a fence should read.
	if got := reportInt(t, report, "tile_budget"); got <= engine.MaxTileCacheBytes {
		t.Errorf("tile_budget = %d on a -scene run: the gate scene's cache is sized to its whole %d px document, above the %d-byte document cap",
			got, frame.TileSize, int64(engine.MaxTileCacheBytes))
	}
}
