# TASK-018: CI build và test đúng đường native macOS (darwin + cgo)

- **Task ID**: `TASK-018`
- **Type**: Test infrastructure / Release safety
- **Assigned to**: `qoder cli`
- **Reviewed by**: PM & QA Lead

## 1. Vấn đề đo được, không phải suy đoán

Trước task này, `.github/workflows/ci.yml` có 5 job và **cả 5 chạy `ubuntu-latest`**. Trong khi đó:

- `internal/platform/darwin` là nơi duy nhất có cgo trong repo (`internal/archtest/rules.go` ghi rõ vậy), và toàn bộ cửa sổ thật, clipboard, `NSSavePanel`, backend AVFoundation đều nằm sau `//go:build darwin && cgo`.
- `internal/platform/darwin/stub.go` mang `//go:build !darwin || !cgo`, tức trên Linux CI, **mọi call site của native path đều được compile vào stub rỗng**.
- Hệ quả: một commit làm hỏng `window.go` vẫn xanh CI. Nơi đầu tiên phát hiện ra là job `build` của `.github/workflows/release.yml`, vốn chỉ chạy khi push tag `v*`. Lần build đầu tiên của binary macOS chính là lần build phát hành.

Đây đúng là loại khoảng trống mà catalog QC đã cảnh báo: `BR-179`/`BR-180` tới giờ vẫn `NOT RUN` vì không có gate nào chứng minh "cái máy macOS thật chạy được".

## 2. Đã làm

Thêm job `darwin` vào `.github/workflows/ci.yml`, chạy trên `macos-latest` (arm64), có `timeout-minutes: 20`, gồm 4 bước:

1. **Guard chống green-giả**: `go list -f '{{join .CgoFiles " "}} {{join .MFiles " "}}' ./internal/platform/darwin/` rồi yêu cầu đủ `window.go media.go clipboard.go dialog.go shim_darwin.m media_darwin.m`.
   Lý do bước này tồn tại: nếu một lần refactor tag build khiến package rơi về stub, job vẫn pass vì nó… đang test stub. `.GoFiles` **không** chứa file cgo (chúng nằm ở `.CgoFiles`), nên kiểm tra sai field sẽ cho danh sách chỉ có `doc.go errors.go` và báo missing — bẫy này đã gặp thật khi viết task này.
2. `go build ./...` — lần đầu tiên đường cgo được build ở mọi PR chứ không chỉ ở release.
3. `go vet -all ./...` — vet cả code cgo (Linux vet chưa từng thấy các file này).
4. `go test -race -count=1 ./internal/platform/darwin/ ./test/platform/ ./internal/archtest/ ./cmd/goosie/`.
   `internal/platform/darwin` chứa bộ test **duy nhất trong repo chứng minh có giải mã thật xảy ra** (`TestMediaBackendDecodesRealFrames`, `...PlaysAVideoWithNoAudioTrack`, `...ClosesWhilePlaying`), fixtured bằng `testdata/media/*.mp4` đã commit. Header của `media_test.go` nói thẳng chúng không cần display, nên runner không có ai login vẫn chạy được.

## 3. Bằng chứng nghiệm thu

Toàn bộ được chạy trên máy thật `darwin/arm64` (Apple Silicon, macOS 25.6.0) — đúng platform CI sẽ dùng, không phải đoán:

| Lệnh | Kết quả |
| --- | --- |
| `go list -f '{{join .CgoFiles " "}} {{join .MFiles " "}}' ./internal/platform/darwin/` | `clipboard.go dialog.go media.go window.go media_darwin.m shim_darwin.m` → guard OK |
| Cùng lệnh với `GOOS=linux CGO_ENABLED=0` | rỗng → guard **fail**, chứng minh nó nhạy thật, không phải bước trang trí |
| `go build ./...` | exit 0 |
| `go vet -all ./...` | exit 0 |
| `go test -race -count=1 ./internal/platform/darwin/ ./test/platform/ ./internal/archtest/ ./cmd/goosie/` | ok / ok / ok / ok (2.0s, 2.1s, 9.1s, 19.6s) |
| `go test -count=1 -v ./internal/platform/darwin/` | 8 test native PASS, không skip |
| `python3 -c "yaml.safe_load(ci.yml)"` | jobs: checks, race, darwin, audit, wpt-pilot, parity |

## 4. Khoảng trống còn mở — ghi lại chứ không giả vờ đã xong

- **`scripts/v2-gate-check.sh` vẫn chưa được nối vào CI.** `docs/qa-test-plan.md:30` và `:245` nói ngưỡng thời lượng thuộc "nightly macOS", và `docs/engine-compatibility-review.md:373` đã tự ghi nhận "wire it to the macOS job; today it is referenced only by tests and docs". Task này **không** nối, vì có lý do kỹ thuật thật: script đọc artifact `go run ./cmd/goosie -gate -out FILE` và cố ý **fail (exit 2) khi một metric vắng mặt** thay vì đọc nó là 0. Trên runner macOS không có ai login thật, `CVDisplayLink`/`startup_ms` có thể không đo được, tức nightly sẽ đỏ vĩnh viễn và bị tắt trong một tháng — chính cái cách mà header script cấm. Hướng đúng: self-hosted runner có display, hoặc một job `workflow_dispatch` ban đêm trên máy được chỉ định. Việc này cần quyết định hạ tầng, giao PM.
- **Chi phí**: macOS minutes tính hệ số cao hơn Linux. Job này build + vet + 4 package test (`~40s` test + build), chấp nhận được; nếu về sau đắt, thu hẹp bước `-race` còn `./internal/platform/darwin/` rồi mở rộng dần, **không** bỏ bước guard.
- **`BR-050`/`BR-051` và toàn bộ nhãn `BLOCKED` vì CUA "không có display"** vẫn chờ cùng một hạ tầng ở ý đầu tiên.
