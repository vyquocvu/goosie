# Raster Backend Integration Guide

This guide explains how to add a new raster backend to the Goosie engine.

## Backend Interface

All raster backends implement the `Backend` interface in `internal/renderer/frame/raster/cpu_backend.go`:

```go
type Backend interface {
    BeginFrame(info FrameInfo) error
    Rasterize(list DisplayList, regions []DirtyRegion) (FrameOutput, error)
    EndFrame() error
    Close() error
}
```

## Step-by-Step

### 1. Create the Implementation File

Create a new file, e.g., `metal_backend.go` for a Metal backend:

```go
// Package raster — Metal backend (macOS, CGo)
//
//go:build darwin && cgo
package raster

type metalBackend struct {
    // backend state
}

func (b *metalBackend) BeginFrame(info FrameInfo) error { ... }
func (b *metalBackend) Rasterize(list DisplayList, regions []DirtyRegion) (FrameOutput, error) { ... }
func (b *metalBackend) EndFrame() error { ... }
func (b *metalBackend) Close() error { ... }
```

If the backend is platform-specific, use build tags. Create a no-op stub for other platforms.

### 2. Support the Command Subset

The raster backend receives `DisplayCmd` values (7 kinds). Your backend must handle:

| CmdKind | Description | Required |
|---|---|---|
| `CmdFill` | Solid fill rectangle | Yes |
| `CmdBorder` | Rectangle border | Yes |
| `CmdClip` | Push/pop clip rect | Yes |
| `CmdOpacity` | Push/pop opacity group | Yes |
| `CmdText` | Shaped text run | Yes |
| `CmdImage` | Decoded image | Yes |
| `CmdPath` | Simplified SVG path | No (optional) |

Return an error for unsupported commands rather than silently ignoring them.

### 3. Register in the Factory

Add a `BackendType` constant in `internal/renderer/frame/raster/backend_type.go`:

```go
const (
    BackendCPU          BackendType = "cpu"
    BackendCoreGraphics BackendType = "coregraphics"
    BackendMetal        BackendType = "metal"  // new
)
```

Update `NewBackend()` to try the new backend based on build tags or platform detection:

```go
func selectBackend() BackendType {
    if runtime.GOOS == "darwin" && cgoEnabled {
        return BackendMetal  // prefer Metal over CG
    }
    return BackendCPU
}
```

### 4. Handle Frame Lifecycle

The compositor calls BeginFrame → Rasterize → EndFrame in sequence:

- **BeginFrame**: Reset per-frame state, validate viewport, prepare command buffers.
- **Rasterize**: Convert DisplayCmd list to backend-native draw calls. Rasterize only dirty regions when provided (use `regions` parameter).
- **EndFrame**: Flush command buffers, present to screen, release scratch resources.

### 5. Implement Frame Buffer Reuse

Reuse pixel buffers or GPU textures across frames to avoid per-frame allocation:

```go
type metalBackend struct {
    pools    []*MetalTexture  // recycled textures
    frameBuf *image.RGBA      // optional CPU fallback buffer
}
```

### 6. Add Cross-Backend Equivalence Tests

Add tests in `test/internal/renderer/frame/golden/` that render the same display commands with both the CPU backend and your new backend, then compare output:

```go
func TestMetalGoldenEquivalence(t *testing.T) {
    cmds := LoadTestFixture("solid-fill")
    cpuImg := renderWithBackend(cmds, BackendCPU)
    metalImg := renderWithBackend(cmds, BackendMetal)
    diff := CompareImages(cpuImg, metalImg)
    if diff > tolerance { t.Error("backends differ") }
}
```

### 7. Update Documentation

- Add the new backend to `webview-architecture.md` (backend table).
- Add build instructions to `README.md` if additional dependencies are needed.
- Document platform requirements and known limitations.

## Testing Checklist

- [ ] All 7 `DisplayCmd` kinds handled
- [ ] Dirty-region-only raster works correctly
- [ ] Frame buffer reuse prevents allocation growth
- [ ] Cross-backend equivalence with CPU backend
- [ ] Race detector clean (`go test -race ./test/internal/engine/...`)
- [ ] Memory growth test passes (repeated frames)
- [ ] `Close()` releases all backend resources
- [ ] Build-tag isolation (non-target platforms compile to stub)
