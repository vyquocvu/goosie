# internal/image — Agent Constraints & Architecture

## Core Responsibilities

The `internal/image` package manages asynchronous image fetching, decoding, format conversion, SVG vector rasterization, and memory caching for Goosie.

## Supported Formats & Decoders

- Standard raster formats supported out of the box: PNG, JPEG, GIF, and WebP (`golang.org/x/image/webp`).
- Vector graphics: Scalable Vector Graphics (SVG) parsed via `oksvg` (`svg.go`) and rasterized directly to standard `image.Image` / `*image.RGBA` surfaces.
- Data URLs: `data:image/...;base64,...` inline payloads parsed and decoded synchronously.

## Asynchronous Image Loader (`ImageLoader`)

- `ImageLoader` in `loader.go` orchestrates non-blocking image loading:
  - In-flight request deduplication: `inProgress map[string]*sync.WaitGroup` ensures duplicate requests for the same URL share a single network fetch.
  - Per-domain rate limiting: Throttles consecutive image requests per host to prevent HTTP 429 Too Many Requests errors.
  - Asynchronous notifications: Fires `OnLoad(source string)` callbacks on worker goroutines upon decode completion to trigger targeted renderer invalidations.

## In-Memory Image Cache (`Cache`)

- Decoded images are cached in `Cache` (`cache.go`) to avoid repeated network fetches and CPU decoding overhead.
- Cached items track byte sizes and participate in memory management via `internal/memory` (`ComponentImage`).

## Thread Safety

- `ImageLoader` and `Cache` are safe for concurrent access across goroutines.
- Image decoding runs entirely on background worker pools, never on the Fyne UI main thread.

## Testing & Verification

All image subsystem tests reside in `test/internal/image/...`.

Run the image test suite with the race detector:
```bash
go test -race -short ./test/internal/image/...
```
