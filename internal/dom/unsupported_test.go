package dom

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/dom/atom"
)

// ---------------------------------------------------------------------------
// TestStreamParseDetectsCanvas
// ---------------------------------------------------------------------------

func TestStreamParseDetectsCanvas(t *testing.T) {
	const input = `<html><body><canvas id="game" width="800" height="600"></canvas></body></html>`

	var detected bool
	cfg := ParseConfig{
		OnUnsupportedFeature: func(f UnsupportedFeature) {
			if f.Kind == FeatureCanvas {
				detected = true
			}
		},
	}

	parser := NewParser()
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
	</body></html>`

	var mu sync.Mutex
	detected := make(map[UnsupportedFeatureKind]int)

	cfg := ParseConfig{
		OnUnsupportedFeature: func(f UnsupportedFeature) {
			mu.Lock()
			detected[f.Kind]++
			mu.Unlock()
		},
	}

	parser := NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)

	mu.Lock()
	defer mu.Unlock()

	assert.Equal(t, 1, detected[FeatureCanvas], "should detect canvas")
	assert.Equal(t, 1, detected[FeatureVideo], "should detect video")
	assert.Equal(t, 1, detected[FeatureAudio], "should detect audio")
	assert.Equal(t, 1, detected[FeatureIframe], "should detect iframe")
	assert.Equal(t, 4, len(detected), "should detect exactly 4 unsupported features")
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
	cfg := ParseConfig{}
	parser := NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err, "should not panic or error without OnUnsupportedFeature callback")
	require.NotNil(t, doc)

	assert.Equal(t, NodeKindDocument, doc.Store.Kind(doc.Root))
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
	cfg := ParseConfig{
		OnUnsupportedFeature: func(f UnsupportedFeature) {
			detected++
		},
	}

	parser := NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, 3, detected, "should detect canvas, video, iframe (not img)")

	// Verify normal elements are still parsed correctly.
	h1ID := findElement(t, doc, atom.AtomH1)
	assert.NotEqual(t, NodeNone, h1ID, "should find h1 element")

	imgID := findElement(t, doc, atom.AtomImg)
	assert.NotEqual(t, NodeNone, imgID, "should find img element (supported)")
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
	cfg := ParseConfig{
		OnUnsupportedFeature: func(f UnsupportedFeature) {
			detected++
		},
	}

	parser := NewParser()
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
	cfg := ParseConfig{
		OnUnsupportedFeature: func(f UnsupportedFeature) {
			if f.Kind == FeatureVideo {
				detected = true
			}
		},
	}

	parser := NewParser()
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
		kind UnsupportedFeatureKind
		want string
	}{
		{FeatureCanvas, "canvas"},
		{FeatureVideo, "video"},
		{FeatureAudio, "audio"},
		{FeatureIframe, "iframe"},
		{UnsupportedFeatureKind(0), "UnsupportedFeatureKind(0)"},
		{UnsupportedFeatureKind(99), "UnsupportedFeatureKind(99)"},
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

	var resources []Resource
	var unsupported []UnsupportedFeature
	var mu sync.Mutex

	cfg := ParseConfig{
		OnResource: func(r Resource) {
			mu.Lock()
			resources = append(resources, r)
			mu.Unlock()
		},
		OnUnsupportedFeature: func(f UnsupportedFeature) {
			mu.Lock()
			unsupported = append(unsupported, f)
			mu.Unlock()
		},
	}

	parser := NewParser()
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
		case ResourceCSS:
			hasCSS = true
		case ResourceImage:
			hasImg = true
		}
	}
	assert.True(t, hasCSS, "should detect stylesheet")
	assert.True(t, hasImg, "should detect image")

	hasCanvas := false
	hasVideo := false
	for _, f := range unsupported {
		switch f.Kind {
		case FeatureCanvas:
			hasCanvas = true
		case FeatureVideo:
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
	cfg := ParseConfig{
		OnUnsupportedFeature: func(f UnsupportedFeature) {
			if f.Kind == FeatureCanvas {
				count++
			}
		},
	}

	parser := NewParser()
	doc, err := parser.ParseDocumentCtx(context.Background(), strings.NewReader(input), cfg)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, 3, count, "should detect each <canvas> instance")
}
