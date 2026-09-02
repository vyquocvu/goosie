package image

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
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

// ImageLoader handles loading images from various sources
type ImageLoader struct {
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

// NewLoader creates a new image ImageLoader with a cache
func NewLoader(cacheSize int) Loader {
	return &ImageLoader{
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
func (l *ImageLoader) acquireDomainSem(domain string) {
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
func (l *ImageLoader) releaseDomainSem(domain string) {
	l.domainSemMu.Lock()
	sem := l.domainSem[domain]
	l.domainSemMu.Unlock()
	<-sem
}

// SetOnLoadCallback sets the callback for when an image is loaded
func (l *ImageLoader) SetOnLoadCallback(callback OnLoadCallback) {
	l.mu.Lock()
	l.OnLoad = callback
	l.mu.Unlock()
}

// Load loads an image from a URL or file path
// Returns cached image if available, otherwise loads asynchronously
func (l *ImageLoader) Load(source string) (*ImageData, error) {
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
func (l *ImageLoader) LoadSync(source string) (*ImageData, error) {
	// Check cache first
	if cached := l.cache.Get(source); cached != nil {
		return cached, nil
	}

	// Load the image
	data, err := l.LoadImage(source)
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
func (l *ImageLoader) loadAsync(source string, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		l.mu.Lock()
		delete(l.inProgress, source)
		l.mu.Unlock()
	}()

	data, err := l.LoadImage(source)
	if err != nil {
		log.Printf("Failed to load image %s: %v", source, err)
		data = &ImageData{
			State: StateError,
			Error: err,
		}
	} else {
		// log.Printf("Successfully loaded image %s", source)
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

// LoadImage loads an image from a source (URL or file path).
func (l *ImageLoader) LoadImage(source string) (*ImageData, error) {
	// Determine if it's a URL or file path
	if strings.HasPrefix(source, "data:") {
		return l.loadFromDataURI(source)
	}
	if IsURL(source) {
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

	// Prefer the SVG's width/height attributes (matching browsers) over the
	// viewBox when they are present and valid.
	w := int(icon.ViewBox.W)
	h := int(icon.ViewBox.H)
	if aw, ah, ok := ParseSVGIntrinsicSize(data); ok {
		w, h = aw, ah
	}
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 100
	}

	rgba := rasterizeIcon(icon, w, h)

	return &ImageData{
		Image:  rgba,
		Width:  w,
		Height: h,
		Format: "svg",
		State:  StateLoaded,
	}, nil
}

// parseSVGIntrinsicSize extracts the width and height attributes from an SVG
// document's root <svg> element. Length units other than px are ignored
// (viewBox is used as the fallback in that case). Returns false when either
// attribute is missing or invalid.
func ParseSVGIntrinsicSize(data []byte) (int, int, bool) {
	open := bytes.Index(data, []byte("<svg"))
	if open < 0 {
		return 0, 0, false
	}
	tag := data[open:]
	close := bytes.IndexByte(tag, '>')
	if close < 0 {
		return 0, 0, false
	}
	tag = tag[:close]

	wStr := extractSVGAttribute(tag, "width")
	hStr := extractSVGAttribute(tag, "height")
	if wStr == "" || hStr == "" {
		return 0, 0, false
	}
	w := parseSVGLength(wStr)
	h := parseSVGLength(hStr)
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// extractSVGAttribute returns the value of a quoted attribute within an SVG
// element tag, or "" if absent. Attribute names are matched case-insensitively
// (SVG attribute names are case-sensitive, but real-world documents
// occasionally uppercase them, and browsers treat XML attributes case-sensitively
// while tolerating these variants).
func extractSVGAttribute(tag []byte, name string) string {
	lowerTag := bytes.ToLower(tag)
	lowerName := strings.ToLower(name)
	idx := 0
	for {
		pos := bytes.Index(lowerTag[idx:], []byte(lowerName))
		if pos < 0 {
			return ""
		}
		pos += idx
		// Ensure it's a full attribute name (followed by optional whitespace/=)
		if pos+len(lowerName) < len(tag) {
			next := tag[pos+len(lowerName)]
			if next != '=' && next != ' ' && next != '\t' && next != '\n' {
				idx = pos + len(lowerName)
				continue
			}
		}
		eq := bytes.IndexByte(tag[pos:], '=')
		if eq < 0 {
			return ""
		}
		valStart := pos + eq + 1
		for valStart < len(tag) && (tag[valStart] == ' ' || tag[valStart] == '\t') {
			valStart++
		}
		if valStart >= len(tag) {
			return ""
		}
		var val []byte
		if tag[valStart] == '"' || tag[valStart] == '\'' {
			q := tag[valStart]
			end := bytes.IndexByte(tag[valStart+1:], q)
			if end < 0 {
				return ""
			}
			val = tag[valStart+1 : valStart+1+end]
		} else {
			end := valStart
			for end < len(tag) && tag[end] != ' ' && tag[end] != '\t' && tag[end] != '>' && tag[end] != '/' {
				end++
			}
			val = tag[valStart:end]
		}
		return string(val)
	}
}

// parseSVGLength parses a CSS-like length. Returns the value in pixels when it
// is unitless or has a px unit; returns <= 0 otherwise.
func parseSVGLength(s string) int {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	num := s
	scale := float64(1)
	switch {
	case strings.HasSuffix(lower, "px"):
		num = strings.TrimSpace(lower[:len(lower)-2])
	case strings.HasSuffix(lower, "pt"):
		num = strings.TrimSpace(lower[:len(lower)-2])
		scale = 96.0 / 72.0
	case strings.HasSuffix(lower, "pc"):
		num = strings.TrimSpace(lower[:len(lower)-2])
		scale = 16
	case strings.HasSuffix(lower, "mm"):
		num = strings.TrimSpace(lower[:len(lower)-2])
		scale = 96.0 / 25.4
	case strings.HasSuffix(lower, "cm"):
		num = strings.TrimSpace(lower[:len(lower)-2])
		scale = 96.0 / 2.54
	case strings.HasSuffix(lower, "in"):
		num = strings.TrimSpace(lower[:len(lower)-2])
		scale = 96
	default:
		if strings.HasSuffix(lower, "%") || strings.HasSuffix(lower, "em") || strings.HasSuffix(lower, "rem") || strings.HasSuffix(lower, "vw") || strings.HasSuffix(lower, "vh") {
			return -1
		}
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil || v < 0 {
		return -1
	}
	return int(v * scale)
}

// loadFromDataURI loads an image from a data URI
func (l *ImageLoader) loadFromDataURI(dataURI string) (*ImageData, error) {
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
func (l *ImageLoader) loadFromURL(url string) (*ImageData, error) {
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
func (l *ImageLoader) loadFromFile(path string) (*ImageData, error) {
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
func (l *ImageLoader) decodeImage(r io.Reader) (*ImageData, error) {
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
func (l *ImageLoader) GetCache() *Cache {
	return l.cache
}

// GetHTTPClient returns the HTTP client. Exported for testing.
func (l *ImageLoader) GetHTTPClient() *http.Client {
	return l.httpClient
}

// IsURL checks if a string is a URL.
func IsURL(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || s[:8] == "https://")
}
