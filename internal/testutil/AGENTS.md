# internal/testutil — Agent Constraints & Architecture

## Core Responsibilities

The `internal/testutil` package provides shared testing utilities for Goosie packages and E2E suites. It is a test-support package only; no production code imports it.

## Components

- Screenshot helpers (`testutil.go`): `RenderToImage` renders a `fyne.CanvasObject` to an `image.Image` through a Fyne test app with a light theme, unpadded window (`SetPadded(false)`, so pixels map 1:1 to Goosie layout coordinates), and a white background rectangle; the capture is clipped to the requested dimensions. `SaveRenderedScreenshot` and `SaveImageAsPNG` persist captures as PNG.
- Environment-gated saving: `GOOSIE_SCREENSHOT_DIR` (constant `ScreenshotDir`) controls saving via `GetScreenshotDir` / `ShouldSaveScreenshots`; `SaveTestScreenshot` is a no-op returning an empty path when the variable is unset.
- Filename safety: `SanitizeFilename` keeps `[A-Za-z0-9_.-]` and maps spaces/slashes to underscores.
- HTTP mocking (`mock.go`): `MockRoundTripper` implements `http.RoundTripper` returning a canned `*http.Response` and error; `NewMockClient(responseBody, statusCode, err)` wraps it in an `*http.Client` for network-layer tests.
- `export.go` contains no code (kept for package consistency).

## Import Rules

- May import `fyne.io/fyne/v2` (including the `test` package) — this is the one internal package where Fyne test infrastructure is a first-class dependency, because its whole purpose is Fyne canvas capture.
- Consumers include `test/internal/renderer`, `test/internal/test_suite/integration`, and `test/e2e` (screenshot capture in live-URL tests).

## Testing & Verification

Tests reside in `test/internal/testutil/`.

```bash
go test ./test/internal/testutil/...
```
