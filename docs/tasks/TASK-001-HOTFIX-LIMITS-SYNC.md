# TASK-001: Khôi phục kiểm tra lỗi TileCacheBudget & Đồng bộ ngưỡng tài nguyên 16 MiB

- **Task ID**: `TASK-001`
- **Type**: Blocker Bug Fix / Tech Debt / Baseline Stabilization
- **Assigned to**: `qoder cli` (Developer)
- **Reviewed by**: PM & QA Lead

---

## 1. Bối cảnh & Mục tiêu (Context & Objective)
PO đã quyết định giữ nguyên các hằng số giới hạn tài nguyên mới trong `internal/engine/limits.go` (16 MiB Document Bytes, 1,000,000 Nodes, 524,288 Document Tiles, 2 MiB Geometry) để hỗ trợ các website hiện đại nặng tải.

Tuy nhiên:
1. `cmd/goosie/main.go` bị mất đoạn kiểm tra `if err != nil` sau `engine.TileCacheBudget(extent)`, làm vô hiệu hóa hàng rào an ninh kích thước tile cache đối với tài liệu không tin cậy.
2. Các test suite an ninh và bounds (`test/domtest`, `test/engine`, `test/entrypoints`, `test/net`, `cmd/goosie`) vẫn kiểm tra theo mốc 8 MiB / 65,536 tiles cũ, dẫn đến 5 packages test bị FAIL.
3. Tài liệu kiến trúc `PROJECT.md` chưa được cập nhật đồng bộ các hằng số này.

**Mục tiêu**: Đưa toàn bộ test suite (`go test ./...`) về trạng thái **100% GREEN** và đồng bộ hóa tài liệu kiến trúc.

---

## 2. Danh sách tệp cần cập nhật (Files to Modify)

### 2.1. `cmd/goosie/main.go` (Dòng ~1637)
Khôi phục kiểm tra lỗi trả về từ `engine.TileCacheBudget`:
```go
	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: size document cache: %w", err)
	}
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
```

### 2.2. `cmd/goosie/main_test.go` (`TestLoadURLCtxRejectsOversizedDocument`)
Kích thước tài liệu quá khổ trong test phải vượt ngưỡng mới (`MaxDocumentTiles = 524288` tiles).
- Với `frame.TileSize = 256`: Một tài liệu `width: 131073px; height: 262144px;` sẽ tạo ra `513 * 1024 = 525,312` tiles (> 524,288 tiles).
- Thay thế:
```go
html := `<!DOCTYPE html><html><body><div style="width:131073px;height:262144px;background:red"></div></body></html>`
```

### 2.3. `test/domtest/domtest.go` (`fixtureLimits()`)
Cập nhật các literal tương ứng với `limits.go`:
```go
func fixtureLimits() dom.ParseLimits {
	return dom.ParseLimits{
		Nodes:          1000000,
		Depth:          128,
		Attributes:     64,
		AttributeBytes: 1048576,
	}
}
```

### 2.4. `test/engine/bounds_test.go` (`TestSessionRejectsUnboundedInput`)
Cập nhật các test case kích thước:
- "HTML bytes": `strings.Repeat(" ", (16<<20)+1)`
- "geometry": `<p style="width:2097153px">x` (vượt `MaxGeometry = 2 << 20`)
- "aggregate geometry": `<div style="height:1200000px"></div><div style="height:1200000px"></div>` (tổng 2.4M > 2,097,152)

### 2.5. `test/engine/viewport_test.go` (`TestValidateExtentCountsSignedTileSpans`)
Cập nhật các trường hợp test số tile vượt ngưỡng `MaxDocumentTiles = 524288`:
- `{"aligned limit", frame.Rect4(0, 0, 131072, 262144), true}` (512 * 1024 = 524,288 tiles)
- `{"one extra column", frame.Rect4(0, 0, 131073, 262144), false}` (513 * 1024 = 525,312 tiles)
- `{"negative partial tiles", frame.Rect4(-1, -1, 131071, 262143), false}` (513 * 1025 = 525,825 tiles)

### 2.6. `test/entrypoints/limits_test.go`
- Trong `TestGoosieHeadlessRejectsOversizedDocument`:
  Đổi kích thước HTML sang `width:131073px;height:262144px;background:red`.
- Trong `TestGoosieHeadlessRejectsOversizedInput`:
  Đổi kích thước sang 16 MiB: `hugeContent := strings.Repeat("x", (16<<20)+1)`.

### 2.7. `test/net/http_test.go`
- `TestGetRejectsOversizedBody`: `bytes.Repeat([]byte("x"), (16<<20)+1)`
- `TestHTTPResponseByteLimit`: `if transport.MaxResponseBytes != 16<<20`

### 2.8. `PROJECT.md`
Cập nhật mục **Resource Limits** (dòng 129-135):
- `MaxDocumentBytes` 16 MiB
- `MaxDocumentNodes` 1,000,000
- `MaxAttributeBytes` 1 MiB
- `MaxGeometry` 2 MiB
- `MaxLayoutObjects` 4,194,304
- `MaxTextRunes` 8,388,608
- `MaxDocumentTiles` 524,288
- `MaxTileCacheBytes` 64 MiB

---

## 3. Tiêu chí Nghiệm thu (Acceptance Criteria - AC)
1. **[AC-01]**: `go test -v ./cmd/goosie -run TestLoadURLCtxRejectsOversizedDocument` PASS.
2. **[AC-02]**: `go test -v ./test/domtest` PASS.
3. **[AC-03]**: `go test -v ./test/engine -run "TestSessionRejectsUnboundedInput|TestValidateExtentCountsSignedTileSpans"` PASS.
4. **[AC-04]**: `go test -v ./test/entrypoints` PASS.
5. **[AC-05]**: `go test -v ./test/net -run "TestGetRejectsOversizedBody|TestHTTPResponseByteLimit"` PASS.
6. **[AC-06]**: `go test ./internal/archtest/` PASS sạch.
7. **[AC-07]**: `go test ./...` toàn bộ repo PASS 100%, không còn bất kỳ package nào lỗi.
8. **[AC-08]**: `go vet -all ./...` không có cảnh báo/lỗi lint nào.

---

## Status: already resolved (verified 2026-10-24)

Verified by TASK-003 developer during Go toolchain bump audit:

1. **TileCacheBudget err checks**: all 8 call sites in `cmd/goosie/*.go` check the error return:
   - `document_loader.go:117-119` (view-source path)
   - `document_loader.go:194-196` (PDF path)
   - `document_loader.go:447-449` (main document path)
   - `error_page.go:168-170`
   - `internal_pages.go:1839-1841`
   - `presentation.go:130-133`
   - `render_work_test.go:669-671`
   - `progressive_images_test.go:39-41`

2. **Test constants match `internal/engine/limits.go`** (16 MiB, 1,000,000 nodes, 524,288 tiles, 2 MiB geometry):
   - `test/domtest/domtest.go:23` — Nodes: 1000000; line 26 — AttributeBytes: 1048576 (1 MiB)
   - `test/engine/bounds_test.go:18-29` — uses `engine.MaxDocumentBytes+1`, `engine.MaxDocumentNodes+1`, `2097153px` (> MaxGeometry), aggregate 2.4M px (> 2 MiB)
   - `test/engine/viewport_test.go:37-39` — 131072×262144 = 524,288 tiles (aligned limit); 131073×262144 = 525,312 tiles (rejected)
   - `test/entrypoints/limits_test.go:73` — 131073px×262144px; line 180 — 16<<20+1
   - `cmd/goosie/main_test.go:209` — 131073px×262144px
   - `test/net/http_test.go:48` — (16<<20)+1; line 228 — 16<<20

All acceptance criteria from the original task are satisfied. No further action needed.
