package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"image"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	imageloader "github.com/vyquocvu/goosie/internal/image"
	"golang.org/x/net/html"
)

// mockImageLoader simulates the async loading of an image.
type mockImageLoader struct {
	loadChan chan struct{}
	callback imageloader.OnLoadCallback
	mu       sync.Mutex
	loaded   bool
	cache    *imageloader.Cache
}

// newMockImageLoader creates a new mock image loader.
func newMockImageLoader() *mockImageLoader {
	return &mockImageLoader{
		loadChan: make(chan struct{}),
		cache:    imageloader.NewCache(10),
	}
}

func (m *mockImageLoader) Load(source string) (*imageloader.ImageData, error) {
	m.mu.Lock()
	if m.loaded {
		m.mu.Unlock()
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		return &imageloader.ImageData{State: imageloader.StateLoaded, Image: img, Width: 1, Height: 1}, nil
	}
	m.mu.Unlock()

	go func() {
		<-m.loadChan // Wait for the trigger
		m.mu.Lock()
		m.loaded = true
		m.mu.Unlock()
		if m.callback != nil {
			m.callback(source)
		}
	}()
	return &imageloader.ImageData{State: imageloader.StateLoading}, nil
}

func (m *mockImageLoader) SetOnLoadCallback(callback imageloader.OnLoadCallback) {
	m.callback = callback
}

func (m *mockImageLoader) GetCache() *imageloader.Cache {
	return m.cache
}

func TestImageRefreshAfterLoad(t *testing.T) {
	// Setup
	loader := newMockImageLoader()
	cr := renderer.NewCanvasRenderer(100, 100)
	cr.SetImageLoader(loader)

	w := test.NewWindow(nil)
	cr.SetWindow(w)
	w.Resize(fyne.NewSize(200, 200))

	htmlContent := `<img src="test.png" alt="alt text">`
	body := parseHTML(htmlContent)

	// Initial Render
	renderTree := renderer.BuildRenderTree(body)
	canvasObject := cr.Render(renderTree)
	w.SetContent(canvasObject)

	// Allow some time for the async load to start
	time.Sleep(100 * time.Millisecond)

	// 1. Verify the "Loading..." state before the image is loaded
	foundLoadingLabel := findWidget(w.Content(), func(obj fyne.CanvasObject) bool {
		if lbl, ok := obj.(*widget.Label); ok {
			return strings.Contains(lbl.Text, "Loading Image")
		}
		return false
	})
	assert.NotNil(t, foundLoadingLabel, "Expected to find 'Loading Image' label")

	// 2. Set up the refresh hook and trigger the image load
	refreshChan := make(chan struct{})
	cr.OnRefresh = func() {
		close(refreshChan)
	}
	close(loader.loadChan)

	// 3. Wait for the refresh to be triggered
	select {
	case <-refreshChan:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnRefresh callback")
	}

	// 4. Re-render and verify the final state
	canvasObjectAfterLoad := cr.Render(renderTree)
	w.SetContent(canvasObjectAfterLoad)

	foundImage := findWidget(w.Content(), func(obj fyne.CanvasObject) bool {
		_, ok := obj.(*canvas.Image)
		return ok
	})
	assert.NotNil(t, foundImage, "Expected to find an image widget after load")
}

// findWidget finds a widget in a canvas object tree that satisfies the condition.
func findWidget(obj fyne.CanvasObject, condition func(fyne.CanvasObject) bool) fyne.CanvasObject {
	if condition(obj) {
		return obj
	}

	if c, ok := obj.(*container.Scroll); ok {
		return findWidget(c.Content, condition)
	}

	if c, ok := obj.(*fyne.Container); ok {
		for _, child := range c.Objects {
			if found := findWidget(child, condition); found != nil {
				return found
			}
		}
	}
	return nil
}

func parseHTML(htmlContent string) *html.Node {
	doc, _ := html.Parse(strings.NewReader(htmlContent))
	return findBodyNode(doc)
}
