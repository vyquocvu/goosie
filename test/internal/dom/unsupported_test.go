package dom_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/dom/atom"
)

// ---------------------------------------------------------------------------
// TestStreamParseDetectsCanvas
// ---------------------------------------------------------------------------

func TestStreamParseDetectsCanvas(t *testing.T) {
	const input = `<html><body><canvas id="game" width="800" height="600"></canvas></body></html>`

	var detected bool
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			if f.Kind == dom.FeatureCanvas {
				detected = true
			}
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.True(t, detected, "should have detected <canvas> as unsupported")
}

// ---------------------------------------------------------------------------
// TestStreamParseDetectsAllUnsupportedFeatures
// ---------------------------------------------------------------------------

func TestStreamParseDetectsAllUnsupportedFeatures(t *testing.T) {
	const input = `<html><body>
		<canvas id="c"></canvas>
		<video src="vid.mp4"></video>
		<audio src="aud.mp3"></audio>
		<iframe src="page.html"></iframe>
		<object data="plugin.dat"></object>
		<embed src="plugin.swf">
	</body></html>`

	var mu sync.Mutex
	detected := make(map[dom.UnsupportedFeatureKind]int)

	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			mu.Lock()
			detected[f.Kind]++
			mu.Unlock()
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, 1, detected[dom.FeatureCanvas], "should detect canvas")
	assert.Equal(t, 1, detected[dom.FeatureVideo], "should detect video")
	assert.Equal(t, 1, detected[dom.FeatureAudio], "should detect audio")
	assert.Equal(t, 1, detected[dom.FeatureIframe], "should detect iframe")
	assert.Equal(t, 1, detected[dom.FeatureObject], "should detect object")
	assert.Equal(t, 1, detected[dom.FeatureEmbed], "should detect embed")
	assert.Equal(t, 6, len(detected), "should detect exactly 6 unsupported features")
}

// ---------------------------------------------------------------------------
// TestStreamParseNoUnsupportedFeatureCallback
// ---------------------------------------------------------------------------

func TestStreamParseNoUnsupportedFeatureCallback(t *testing.T) {
	const input = `<html><body>
		<canvas id="c"></canvas>
		<video src="vid.mp4"></video>
	</body></html>`

	// No OnUnsupportedFeature in config — should not panic.
	cfg := dom.ParseConfig{}
	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err, "should not panic or error without OnUnsupportedFeature callback")
	require.NotNil(t, doc)

	assert.Equal(t, dom.NodeKindDocument, doc.Store.Kind(doc.Root))
	assert.Greater(t, doc.Store.NodeCount(), 3, "should have parsed some nodes")
}

// ---------------------------------------------------------------------------
// TestStreamParseUnsupportedFeatureMixedWithNormal
// ---------------------------------------------------------------------------

func TestStreamParseUnsupportedFeatureMixedWithNormal(t *testing.T) {
	const input = `<html><body>
		<h1>Welcome</h1>
		<p>Normal content.</p>
		<canvas id="game"></canvas>
		<video src="vid.mp4"></video>
		<p>More content.</p>
		<img src="photo.jpg">
		<iframe src="embed.html"></iframe>
	</body></html>`

	var detected int
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			detected++
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, 3, detected, "should detect canvas, video, iframe (not img)")

	// Verify normal elements are still parsed correctly.
	h1ID := findElement(t, doc, atom.AtomH1)
	assert.NotEqual(t, dom.NodeNone, h1ID, "should find h1 element")

	imgID := findElement(t, doc, atom.AtomImg)
	assert.NotEqual(t, dom.NodeNone, imgID, "should find img element (supported)")
}

// ---------------------------------------------------------------------------
// TestStreamParseUnsupportedFeatureCancellation
// ---------------------------------------------------------------------------

func TestStreamParseUnsupportedFeatureCancellation(t *testing.T) {
	const input = `<html><body>
		<p>Start</p>
		<canvas id="game"></canvas>
		<p>End</p>
	</body></html>`

	// Pre-cancelled context should prevent parsing entirely.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var detected int
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			detected++
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(ctx, strings.NewReader(input), cfg)
	require.Error(t, err, "cancelled context should produce error")
	assert.Nil(t, doc, "should not return document on cancelled context")
	assert.Equal(t, 0, detected, "should not fire callback on cancelled context")
}

// ---------------------------------------------------------------------------
// TestStreamParseUnsupportedFeatureInHead
// ---------------------------------------------------------------------------

func TestStreamParseUnsupportedFeatureInHead(t *testing.T) {
	// <video> or <canvas> inside <head> is non-standard but should still be detected.
	const input = `<html><head>
		<title>Test</title>
		<video src="bg.mp4"></video>
	</head><body><p>Content</p></body></html>`

	var detected bool
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			if f.Kind == dom.FeatureVideo {
				detected = true
			}
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.True(t, detected, "should detect <video> even inside <head>")
}

// ---------------------------------------------------------------------------
// TestUnsupportedFeatureKindString
// ---------------------------------------------------------------------------

func TestUnsupportedFeatureKindString(t *testing.T) {
	tests := []struct {
		kind dom.UnsupportedFeatureKind
		want string
	}{
		{dom.FeatureCanvas, "canvas"},
		{dom.FeatureVideo, "video"},
		{dom.FeatureAudio, "audio"},
		{dom.FeatureIframe, "iframe"},
		{dom.FeatureESModule, "es-module"},
		{dom.FeatureObject, "object"},
		{dom.FeatureEmbed, "embed"},
		{dom.FeaturePWAManifest, "pwa-manifest"},
		{dom.FeatureWebSocket, "websocket"},
		{dom.FeatureWebWorker, "web-worker"},
		{dom.FeatureServiceWorker, "service-worker"},
		{dom.UnsupportedFeatureKind(0), "UnsupportedFeatureKind(0)"},
		{dom.UnsupportedFeatureKind(99), "UnsupportedFeatureKind(99)"},
	}
	for _, tc := range tests {
		got := tc.kind.String()
		assert.Equal(t, tc.want, got, "String() for kind %d", tc.kind)
	}
}

// ---------------------------------------------------------------------------
// TestStreamParseUnsupportedFeatureWithResourceCallback
// ---------------------------------------------------------------------------

func TestStreamParseUnsupportedFeatureWithResourceCallback(t *testing.T) {
	// Verify that both callbacks work together without interference.
	const input = `<html><head>
		<link rel="stylesheet" href="style.css">
	</head><body>
		<img src="photo.jpg">
		<canvas id="game"></canvas>
		<video src="vid.mp4"></video>
	</body></html>`

	var resources []dom.Resource
	var unsupported []dom.UnsupportedFeature
	var mu sync.Mutex

	cfg := dom.ParseConfig{
		OnResource: func(r dom.Resource) {
			mu.Lock()
			resources = append(resources, r)
			mu.Unlock()
		},
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			mu.Lock()
			unsupported = append(unsupported, f)
			mu.Unlock()
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)

	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, resources, 2, "should find CSS + image resource")
	assert.Len(t, unsupported, 2, "should detect canvas + video")

	// Verify correct types were found.
	hasCSS := false
	hasImg := false
	for _, r := range resources {
		switch r.Kind {
		case dom.ResourceCSS:
			hasCSS = true
		case dom.ResourceImage:
			hasImg = true
		}
	}
	assert.True(t, hasCSS, "should detect stylesheet")
	assert.True(t, hasImg, "should detect image")

	hasCanvas := false
	hasVideo := false
	for _, f := range unsupported {
		switch f.Kind {
		case dom.FeatureCanvas:
			hasCanvas = true
		case dom.FeatureVideo:
			hasVideo = true
		}
	}
	assert.True(t, hasCanvas, "should detect canvas")
	assert.True(t, hasVideo, "should detect video")
}

// ---------------------------------------------------------------------------
// TestStreamParseUnsupportedFeatureMultipleInstances
// ---------------------------------------------------------------------------

func TestStreamParseUnsupportedFeatureMultipleInstances(t *testing.T) {
	const input = `<html><body>
		<canvas id="c1"></canvas>
		<canvas id="c2"></canvas>
		<canvas id="c3"></canvas>
	</body></html>`

	var count int
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			if f.Kind == dom.FeatureCanvas {
				count++
			}
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, 3, count, "should detect each <canvas> instance")
}

// ---------------------------------------------------------------------------
// TestStreamParseDetectsESModule
// ---------------------------------------------------------------------------

func TestStreamParseDetectsESModule(t *testing.T) {
	const input = `<html><head>
		<script type="module" src="app.mjs"></script>
	</head><body><p>Content</p></body></html>`

	var detected bool
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			if f.Kind == dom.FeatureESModule {
				detected = true
			}
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.True(t, detected, "should detect <script type=module> as unsupported")
}

// ---------------------------------------------------------------------------
// TestStreamParseDetectsESModuleInline
// ---------------------------------------------------------------------------

func TestStreamParseDetectsESModuleInline(t *testing.T) {
	const input = `<html><body>
		<script type="module">import { foo } from "bar";</script>
	</body></html>`

	var detected bool
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			if f.Kind == dom.FeatureESModule {
				detected = true
			}
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.True(t, detected, "should detect inline <script type=module>")
}

// ---------------------------------------------------------------------------
// TestStreamParseDetectsPWAManifest
// ---------------------------------------------------------------------------

func TestStreamParseDetectsPWAManifest(t *testing.T) {
	const input = `<html><head>
		<link rel="manifest" href="/manifest.json">
	</head><body><p>Content</p></body></html>`

	var detected bool
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			if f.Kind == dom.FeaturePWAManifest {
				detected = true
			}
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.True(t, detected, "should detect <link rel=manifest> as unsupported")
}

// ---------------------------------------------------------------------------
// TestStreamParseDetectsObject
// ---------------------------------------------------------------------------

func TestStreamParseDetectsObject(t *testing.T) {
	const input = `<html><body>
		<object data="plugin.dat" type="application/x-someplugin"></object>
	</body></html>`

	var detected bool
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			if f.Kind == dom.FeatureObject {
				detected = true
			}
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.True(t, detected, "should detect <object> as unsupported")
}

// ---------------------------------------------------------------------------
// TestStreamParseDetectsEmbed
// ---------------------------------------------------------------------------

func TestStreamParseDetectsEmbed(t *testing.T) {
	const input = `<html><body>
		<embed src="plugin.swf" type="application/x-shockwave-flash">
	</body></html>`

	var detected bool
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			if f.Kind == dom.FeatureEmbed {
				detected = true
			}
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.True(t, detected, "should detect <embed> as unsupported")
}

// ---------------------------------------------------------------------------
// TestStreamParseDetectsPlainScriptNotModule
// ---------------------------------------------------------------------------

func TestStreamParseDetectsPlainScriptNotModule(t *testing.T) {
	const input = `<html><head>
		<script src="app.js"></script>
		<script>var x = 1;</script>
	</head><body><p>Content</p></body></html>`

	var detectedESModule bool
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			if f.Kind == dom.FeatureESModule {
				detectedESModule = true
			}
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.False(t, detectedESModule, "should NOT detect plain <script> as module")
}

// ---------------------------------------------------------------------------
// TestStreamParseDetectsPWAManifestNotStylesheet
// ---------------------------------------------------------------------------

func TestStreamParseDetectsPWAManifestNotStylesheet(t *testing.T) {
	const input = `<html><head>
		<link rel="stylesheet" href="style.css">
	</head><body><p>Content</p></body></html>`

	var detectedPWA bool
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			if f.Kind == dom.FeaturePWAManifest {
				detectedPWA = true
			}
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.False(t, detectedPWA, "should NOT detect <link rel=stylesheet> as PWA manifest")
}

// ---------------------------------------------------------------------------
// TestStreamParseDetectsCombinedScriptTypes
// ---------------------------------------------------------------------------

func TestStreamParseDetectsCombinedScriptTypes(t *testing.T) {
	const input = `<html><head>
		<script src="regular.js"></script>
		<script type="module" src="app.mjs"></script>
		<script>var x = 1;</script>
	</head><body><p>Content</p></body></html>`

	var count int
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			if f.Kind == dom.FeatureESModule {
				count++
			}
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, 1, count, "should detect exactly one module script, not plain scripts")
}

// ---------------------------------------------------------------------------
// TestStreamParseDetectsScriptTypeMismatch
// ---------------------------------------------------------------------------

func TestStreamParseDetectsScriptTypeMismatch(t *testing.T) {
	const input = `<html><head>
		<script type="text/javascript" src="app.js"></script>
		<script type="application/javascript" src="lib.js"></script>
	</head><body><p>Content</p></body></html>`

	var detected bool
	cfg := dom.ParseConfig{
		OnUnsupportedFeature: func(f dom.UnsupportedFeature) {
			if f.Kind == dom.FeatureESModule {
				detected = true
			}
		},
	}

	parser := dom.NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.False(t, detected, "should not detect text/javascript as module")
}
