# ADR 0003: Raster Backend Boundaries — Interface, Lifecycle, and Selection

**Status:** Accepted (implemented in Milestones 6 and 11)

**Decision Date:** 2024-Q4 (M6) / 2025-Q1 (M11)

**Deciders:** Goosie engine team

---

## Context

The raster backend converts display commands into pixel buffers. In v1, raster was tightly coupled to the Fyne GUI toolkit: the renderer called Fyne canvas APIs directly. This caused three problems:

1. **Engine depended on UI toolkit types.** Core engine packages (`internal/renderer`, `internal/engine`) imported `fyne.io/fyne/v2` and `fyne.io/fyne/v2/canvas`, violating the architectural principle that the engine must be UI-independent.
2. **Only one raster path existed.** Switching between CPU raster and hardware-accelerated rendering required patching the renderer internals.
3. **Testability.** Golden image tests required either mocking Fyne or running with a dummy window. Neither approach was clean.

### Design Constraints

- Core engine packages (`internal/dom`, `internal/css`, `internal/renderer/frame`, `internal/engine`, `internal/js`, `internal/net`) **must not import Fyne types.**
- The interface must support multiple backend types: pure-Go CPU raster (primary), CoreGraphics via CGo (macOS, optional), and potentially future GPU backends.
- Backend selection must work at runtime where possible, with compile-time fallback via build tags.
- The interface must support dirty-region-only rasterization (M5.3) — not full-frame redraw on every frame.
- Frame buffers must be reusable to avoid per-frame allocation.
- The raster lifecyle must match the compositor's frame cycle (begin frame → rasterize → end frame).

---

## Decision

### Interface Design

Define a `Backend` interface with four methods in a dedicated raster package:

```go
type Backend interface {
    BeginFrame(info FrameInfo) error
    Rasterize(list DisplayList, regions []DirtyRegion) (FrameOutput, error)
    EndFrame() error
    Close() error
}
```

**`BeginFrame`** initializes per-frame state (e.g., resetting clip/opacity stacks, validating viewport dimensions). It takes a `FrameInfo` struct containing viewport size, pixel scale, and frame sequence number.

**`Rasterize`** is the core method. It receives a display list and a set of dirty regions (from M5.3) and produces a `FrameOutput` containing the final pixel buffer plus metadata (which tiles changed, frame timing). The backend is expected to rasterize only the dirty regions when possible, falling back to full-frame raster when the dirty set covers most of the viewport.

**`EndFrame`** finalizes the frame — flushes any batched GPU commands (for accelerated backends), signals frame completion, and releases per-frame scratch buffers.

**`Close`** releases all backend-owned resources — GPU textures, font caches, scratch buffers, and native handles.

### Frame Types (M6.1)

All inter-method types are backend-neutral value types:

```
Color       = packed uint32 RGBA (8 bits per channel)
Point       = [2]float32
Rect        = [4]float32
ImageHandle = uint32 index into a session-owned image cache
FontHandle  = uint32 index into a session-owned font cache
Glyph       = glyph ID + position + advance
TextRun     = font handle + size + color + []Glyph + string ID
PixelScale  = float32 (device-pixel ratio, 1.0 = standard)
Viewport    = origin + size (float32)
FrameInfo   = viewport + pixelScale + frameNumber
FrameOutput = *image.RGBA + changed tile list + timing
```

These types are defined in `internal/renderer/frame/frame_types.go` with no backend-specific dependencies.

### Pure-Go CPU Backend (M6.2)

The primary backend is `CPUBackend` in `internal/renderer/frame/raster/cpu_backend.go`:

- Renders directly to an `*image.RGBA` using standard Go image libraries.
- Supports all 7 raster-level command kinds (fill, border, clip, opacity, text, image, path).
- Rasterizes only dirty regions when provided; falls back to full frame when dirty coverage > 80%.
- Reuses a `FrameBuffer` pool to avoid per-frame `*image.RGBA` allocation.
- Uses a flat `DisplayCmd` representation (resolved from `DisplayCommand` — see ADR 0002) for zero-interface hot-path dispatch.

### CoreGraphics Backend (M11.2, optional)

A secondary backend for macOS uses CoreGraphics via CGo:

- Behind build tag `darwin && cgo` — compiled only on macOS with CGo enabled.
- Batches cgo calls (e.g., `fillRectsBatch` fills multiple rects in one call) to minimize cross-language overhead.
- Measures 4.2–10.9× faster than CPU backend for solid fills on macOS hardware.
- Falls back to CPU backend when CoreGraphics initialization fails (configurable via `WithCrashRecover`).

### Backend Selection Policy (M11.3)

Selection uses a factory function with build-tagged auto-detection:

```go
func NewBackend(w, h int, opts ...BackendOption) (Backend, BackendType, error)
```

Selection rules (ordered):
1. If `WithBackend(CoreGraphics)` is specified, try CoreGraphics. On failure, return error (unless `WithCrashRecover` is set, then fall back to CPU).
2. If no backend is forced, auto-detect: CoreGraphics on `darwin && cgo`, CPU everywhere else.
3. If auto-detect fails, fall back to CPU backend.

`BackendOption` is an interface for extensibility. Current options:
- `WithBackend(BackendType)` — force a specific backend.
- `WithCrashRecover()` — panic-safe backend construction with CPU fallback.
- `WithCrashRecoverFunc(func(error))` — custom recovery handler.

The selected `BackendType` is returned as a second value so callers can record it as a metric label via `BackendType.String()`.

### Frame Lifecycle Ownership

A `Compositor` owns the `Backend` instance and drives the frame cycle:

```
Compositor.BeginFrame()
  └─ Backend.BeginFrame(FrameInfo)
Compositor.Rasterize(list, regions)
  └─ Backend.Rasterize(list, regions) → FrameOutput
Compositor.EndFrame()
  └─ Backend.EndFrame()
  └─ Present FrameOutput to Fyne adapter
```

The Fyne adapter (`internal/renderer/fyne_adapter.go`) receives `*image.RGBA` buffers and converts them to Fyne `canvas.Image` widgets. It never calls `Backend` methods directly.

### Backend-Neutrality Enforcement

- The raster package exports `Backend` interface, frame types, and factory — but not Fyne imports.
- The CPU backend imports only `image`, `image/color`, `image/draw`, `golang.org/x/image/font`, `golang.org/x/image/math/fixed`, and `golang.org/x/image/bmp` (for image decoding).
- The CoreGraphics backend lives in platform-specific files (`cg_backend_darwin.go`, `cg_backend_other.go`) with build-tagged empty stubs on non-macOS platforms.
- A CI gate (`go vet` on linux) catches accidental Fyne imports in core packages.

---

## Consequences

### Positive

- **Engine testable without Fyne.** Golden image tests create a `CPUBackend`, render display commands, and compare output against reference images — all without opening a window.
- **Multiple backends without engine changes.** Adding a new backend requires only implementing the `Backend` interface and registering it in `NewBackend`. The renderer and compositor are unchanged.
- **Clean separation of concerns.** The compositor drives the lifecycle, the backend handles pixel output, and the Fyne adapter bridges only the final buffer.
- **Measurable cross-backend equivalence.** Golden tests run on both CPU and CoreGraphics backends with `CompareImages` tolerance checks.
- **Build-tag isolation.** The CoreGraphics backend never compiles on Linux or Windows, preventing accidental CGo imports.

### Negative

- **Dual command representation persists.** The raster backend operates on `DisplayCmd` (7 kinds), while the renderer produces `DisplayCommand` (12 kinds with push/pop). The translation adds ~40ns per command (measured, negligible relative to raster cost).
- **CoreGraphics backend is macOS-only.** Accelerated rendering on other platforms requires future backends (Vulkan, DirectX, Metal).
- **Backend selection is currently trivial** (one auto-detect rule). The option pattern is designed for future complexity but adds indirection today.
- **CGo overhead is non-zero.** Even with batched calls, the CoreGraphics backend pays ~1μs per batch call for the Go-to-C transition. This is acceptable given the 4–10× raw fill speedup.

---

## Alternatives Considered

### 1. No abstract backend — hardcode CPU raster
Simplest path: always use `image.RGBA` raster. Rejected because it locks out hardware acceleration entirely and makes macOS performance suboptimal (CoreGraphics is 4–10× faster on fill-heavy pages).

### 2. `Render(cmds) image.Image` stateless API
Replace the lifecycle with a single `Render(DisplayList) *image.RGBA` call. Rejected because it prevents backends from holding state across frames (texture caches, GPU command buffers, scratch pools) and forces full-frame raster on every call.

### 3. Interface per backend type
Define separate interfaces (e.g., `CPUBackend`, `CGBackend`) and use type switches in the compositor. Rejected because it couples the compositor to specific backend implementations and makes adding a new backend a compositor change.

### 4. CGo-free accelerated backend
Use a pure-Go GPU compute library (e.g., `gonum/gonum` + image manipulation) for acceleration. Investigated but rejected because no mature pure-Go GPU compute library exists that matches CoreGraphics performance on macOS.

### 5. Fyne-native rendering
Continue the v1 approach of using Fyne `canvas.*` objects for all rendering. Rejected because it makes the engine untestable without a window, violates the no-Fyne-in-engine rule, and limits backend selection to whatever Fyne provides.

---

## Performance Evidence

Measured on M11 benchmark fixtures (macOS, M-series):

| Operation | CPU Backend | CoreGraphics Backend | Speedup |
|---|---|---|---|
| Solid fill (full frame) | 320 μs | 30 μs | 10.7× |
| Fills batch (100 rects) | 180 μs | 42 μs | 4.3× |
| Clip + fill | 410 μs | 92 μs | 4.5× |
| Border (complex) | 85 μs | 115 μs | 0.74× |
| Text run (100 glyphs) | 210 μs | 48 μs | 4.4× |

The CPU backend is competitive on small geometries and complex borders but loses on fill-throughput-heavy scenarios. The `NewBackend` factory's `WithBackend` option allows forcing CPU on machines where CoreGraphics is unavailable or unstable.

---

## Related

- `website/docs/backend-integration.md`
- `internal/renderer/frame/raster/backend.go` — `Backend` interface
- `internal/renderer/frame/raster/cpu_backend.go` — CPU implementation
- `internal/renderer/frame/raster/cg_backend_darwin.go` — CoreGraphics implementation
- `internal/renderer/frame/raster/backend_type.go` — factory and selection logic
- `internal/renderer/frame/frame_types.go` — platform-neutral frame types
- `internal/renderer/frame/cache/` — glyph/image caches
- `internal/renderer/fyne_adapter.go` — Fyne presentation bridge
- ADR 0002: Retained Display List Design (display list as raster input)
