package image

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	_ "golang.org/x/image/webp"
)

// LoadState represents the state of an image load operation
type LoadState int

const (
	// StateLoading indicates the image is being loaded
	StateLoading LoadState = iota
	// StateLoaded indicates the image was successfully loaded
	StateLoaded
	// StateError indicates an error occurred during loading
	StateError
)

// ImageData represents a loaded image with metadata
type ImageData struct {
	Image  image.Image
	Width  int
	Height int
	Format string
	State  LoadState
	Error  error
}

// OnLoadCallback is a callback function for when an image is loaded
type OnLoadCallback func(source string)

// loader handles loading images from various sources
type loader struct {
	httpClient *http.Client
	cache      *Cache
	mu         sync.RWMutex
	// Track in-progress loads to avoid duplicate requests
	inProgress map[string]*sync.WaitGroup
	// OnLoad is called when an image is successfully loaded
	OnLoad OnLoadCallback

	// Per-domain rate limiting to avoid 429 responses
	domainSem   map[string]chan struct{}
	domainSemMu sync.Mutex
}

const (
	maxConcurrentPerDomain = 2
	maxRetries             = 3
	retryBaseDelay         = 500 * time.Millisecond
)

// NewLoader creates a new image loader with a cache
func NewLoader(cacheSize int) Loader {
	return &loader{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache:      NewCache(cacheSize),
		inProgress: make(map[string]*sync.WaitGroup),
		domainSem:  make(map[string]chan struct{}),
	}
}

// acquireDomainSem acquires the per-domain semaphore for the given domain,
// blocking if the maximum number of concurrent requests is already in flight.
func (l *loader) acquireDomainSem(domain string) {
	l.domainSemMu.Lock()
	sem, ok := l.domainSem[domain]
	if !ok {
		sem = make(chan struct{}, maxConcurrentPerDomain)
		l.domainSem[domain] = sem
	}
	l.domainSemMu.Unlock()
	sem <- struct{}{}
}

// releaseDomainSem releases the per-domain semaphore.
func (l *loader) releaseDomainSem(domain string) {
	l.domainSemMu.Lock()
	sem := l.domainSem[domain]
	l.domainSemMu.Unlock()
	<-sem
}

// SetOnLoadCallback sets the callback for when an image is loaded
func (l *loader) SetOnLoadCallback(callback OnLoadCallback) {
	l.mu.Lock()
	l.OnLoad = callback
	l.mu.Unlock()
}

// Load loads an image from a URL or file path
// Returns cached image if available, otherwise loads asynchronously
func (l *loader) Load(source string) (*ImageData, error) {
	// Check cache first
	if cached := l.cache.Get(source); cached != nil {
		return cached, nil
	}

	// Check if already loading this image
	l.mu.Lock()
	if _, exists := l.inProgress[source]; exists {
		l.mu.Unlock()
		// Already loading - return loading state immediately
		// Do not wait here to avoid blocking the UI thread
		return &ImageData{State: StateLoading}, nil
	}

	// Mark as in-progress
	wg := &sync.WaitGroup{}
	wg.Add(1)
	l.inProgress[source] = wg
	l.mu.Unlock()

	// Return loading state immediately and load in background
	go l.loadAsync(source, wg)

	return &ImageData{State: StateLoading}, nil
}

// LoadSync loads an image synchronously
func (l *loader) LoadSync(source string) (*ImageData, error) {
	// Check cache first
	if cached := l.cache.Get(source); cached != nil {
		return cached, nil
	}

	// Load the image
	data, err := l.loadImage(source)
	if err != nil {
		data = &ImageData{
			State: StateError,
			Error: err,
		}
	}

	// Cache the result (even errors)
	l.cache.Put(source, data)

	return data, err
}

// loadAsync loads an image asynchronously
func (l *loader) loadAsync(source string, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		l.mu.Lock()
		delete(l.inProgress, source)
		l.mu.Unlock()
	}()

	data, err := l.loadImage(source)
	if err != nil {
		log.Printf("Failed to load image %s: %v", source, err)
		data = &ImageData{
			State: StateError,
			Error: err,
		}
	} else {
		log.Printf("Successfully loaded image %s", source)
	}

	// Cache the result
	l.cache.Put(source, data)

	// Trigger callback if loaded successfully
	l.mu.RLock()
	cb := l.OnLoad
	l.mu.RUnlock()
	if cb != nil && data.State == StateLoaded {
		cb(source)
	}
}

// loadImage loads an image from a source (URL or file path)
func (l *loader) loadImage(source string) (*ImageData, error) {
	// Determine if it's a URL or file path
	if strings.HasPrefix(source, "data:") {
		return l.loadFromDataURI(source)
	}
	if isURL(source) {
		return l.loadFromURL(source)
	}
	return l.loadFromFile(source)
}

// isSVGSource returns true if the source URL or path ends in .svg (case-insensitive).
func isSVGSource(source string) bool {
	lower := strings.ToLower(source)
	// Strip query string for URL-based sources
	if idx := strings.Index(lower, "?"); idx != -1 {
		lower = lower[:idx]
	}
	return strings.HasSuffix(lower, ".svg")
}

// decodeSVG rasterizes SVG bytes into an ImageData using oksvg/rasterx.
func decodeSVG(data []byte) (*ImageData, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data), oksvg.WarnErrorMode)
	if err != nil {
		return nil, fmt.Errorf("svg parse: %w", err)
	}

	w := int(icon.ViewBox.W)
	h := int(icon.ViewBox.H)
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 100
	}

	icon.SetTarget(0, 0, float64(w), float64(h))

	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), image.White, image.Point{}, draw.Src)

	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	raster := rasterx.NewDasher(w, h, scanner)
	icon.Draw(raster, 1.0)

	return &ImageData{
		Image:  rgba,
		Width:  w,
		Height: h,
		Format: "svg",
		State:  StateLoaded,
	}, nil
}

// loadFromDataURI loads an image from a data URI
func (l *loader) loadFromDataURI(dataURI string) (*ImageData, error) {
	// Format: data:[<mediatype>][;base64],<data>
	parts := strings.SplitN(dataURI, ",", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid data URI format")
	}

	meta := parts[0]
	data := parts[1]

	isBase64 := strings.Contains(meta, ";base64")

	if isBase64 {
		decoded, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 data: %w", err)
		}
		return l.decodeImage(bytes.NewReader(decoded))
	}

	// URL encoded
	decoded, err := neturl.QueryUnescape(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unescape data URI: %w", err)
	}
	return l.decodeImage(strings.NewReader(decoded))
}

// loadFromURL loads an image from a remote URL with per-domain rate limiting
// and automatic retry on 429 (Too Many Requests) with exponential backoff.
func (l *loader) loadFromURL(url string) (*ImageData, error) {
	parsed, err := neturl.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image URL: %w", err)
	}

	// Acquire per-domain semaphore to limit concurrent requests
	l.acquireDomainSem(parsed.Host)
	defer l.releaseDomainSem(parsed.Host)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create image request: %w", err)
	}
	req.Header.Set("User-Agent", "goosie/1.0 (https://github.com/vyquocvu/goosie; like Gecko)")

	var resp *http.Response
	delay := retryBaseDelay
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(delay)
			delay *= 2
		}

		resp, err = l.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch image: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			// Use Retry-After header if present, otherwise exponential backoff
			retryAfter := resp.Header.Get("Retry-After")
			if retryAfter != "" {
				if seconds, err := strconv.Atoi(retryAfter); err == nil {
					time.Sleep(time.Duration(seconds) * time.Second)
				}
			}
			continue
		}

		defer resp.Body.Close()
		break
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	// Read the response body
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	// Detect SVG by URL suffix or Content-Type header.
	ct := resp.Header.Get("Content-Type")
	if isSVGSource(url) || strings.Contains(ct, "image/svg") {
		return decodeSVG(data)
	}

	return l.decodeImage(bytes.NewReader(data))
}

// loadFromFile loads an image from a local file
func (l *loader) loadFromFile(path string) (*ImageData, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open image file: %w", err)
	}
	defer file.Close()

	// Detect SVG by file extension
	if isSVGSource(path) {
		data, err := io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read svg file: %w", err)
		}
		return decodeSVG(data)
	}

	return l.decodeImage(file)
}

// decodeImage decodes an image from a reader
func (l *loader) decodeImage(r io.Reader) (*ImageData, error) {
	img, format, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	bounds := img.Bounds()
	return &ImageData{
		Image:  img,
		Width:  bounds.Dx(),
		Height: bounds.Dy(),
		Format: format,
		State:  StateLoaded,
	}, nil
}

// GetCache returns the cache instance
func (l *loader) GetCache() *Cache {
	return l.cache
}

// isURL checks if a string is a URL
func isURL(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || s[:8] == "https://")
}
