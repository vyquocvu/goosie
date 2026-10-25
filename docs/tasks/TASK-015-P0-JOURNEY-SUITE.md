# TASK-015: Journey suite P0 — tự động hoá các case QA manual qua accessibility tree

- **Task ID**: `TASK-015`
- **Type**: Test infrastructure / Release assurance
- **Assigned to**: `agy`
- **Review**: `qoder cli` (integrator)

---

## 1. Bối cảnh & Mục tiêu

Người dùng duyệt web bằng hành trình, không bằng đơn vị function. Catalog QC (`docs/qa-test-catalog.md`) đã định nghĩa 19 case **P0**; phần lớn trong số đó là hành trình mà QA manual kiểm qua CUA trên native app — đường **không được CI compile** (cả 4 job CI là `ubuntu-latest`, nên `internal/platform/darwin` với `//go:build darwin && cgo` bị loại hoàn toàn, 4495 dòng, 25 hàm ở 0% coverage).

Tin tốt: hạ tầng để tự động hoá đã có sẵn và đúng chủ đích.

- `internal/ax/node.go` ghi rõ: *"ID is a stable identifier for automation"*. `ax.Node` có `Role, ID, Label, Value, Href, Selected, Disabled, AddressBar, Focused` và bounds `X0,Y0,X1,Y1` — tức là **cùng kênh quan sát mà QA manual dùng** ("URL khớp", "nút disabled", "focus đúng ô").
- `cmd/goosie/e2e_test.go:39-257` đã có driver hoàn chỉnh chạy qua `platform/headless` nhưng dùng thật `surface.Loop`, `raster.Scheduler`, `Composer` và `Present`.
- 42 file test trong `cmd/goosie/` đã chứng minh pattern này chạy được, và `go test ./cmd/goosie` mất ~10s.

**Mục tiêu**: dựng **journey suite cho các P0 case hành trình**, assertion bằng **ngữ nghĩa quan sát được** (AX tree, URL, trạng thái tab/toolbar), **không** bằng con số pixel hay giá trị geometry gõ tay. Lý do manual test đáng tin hơn auto test ở repo này: người QA đánh giá *kết quả theo hành trình*; test tự động hiện chủ yếu assert `got/want` tự viết nên chỉ đóng băng hành vi hiện tại.

## 2. API đã có — TÁI DÙNG, không viết lại harness

Từ `cmd/goosie/e2e_test.go`:

| Helper | Dòng | Dùng cho |
| --- | --- | --- |
| `newBrowserE2E(t)` | 39 | dựng app thật qua headless window |
| `b.inject(t, surface.Event{...})` | 154 | mọi input (pointer/key/AX set value) |
| `b.pressKey(t, rune)` | 161 | phím |
| `b.click(t, x, y)` / `b.clickNode(t, id)` / `b.clickElementByID(t, id)` | 166/173/182 | click toạ độ hoặc click **theo ID AX** |
| `b.clickToolbarButton(t, toolbar.Button)` | 204 | nút toolbar |
| `b.navigateViaAddressBar(t, url)` | 213 | gửi `EvAXSetValue` + `AXTargetAddressBar` |
| `b.waitFor(t, description, cond)` | 219 | chờ điều kiện thay vì sleep |
| `b.activeURL()` / `b.diagnostics()` | 250/237 | trạng thái và log khi fail |
| `findAXNode(nodes []ax.Node, id string)` | 257 | tra node AX |
| `browser.window.AccessibilityTree()` | — | []ax.Node, kênh assert chính |

`ax.Role`: `RoleDocument, RoleGroup, RoleStaticText, RoleLink, RoleButton, RoleImage, RoleTextField, RoleSearch, RoleProgressBar, RoleMeter, RoleTabGroup, RoleTab`.

Event vocabulary: `surface.Event{Kind: EvKey, Key: rune}`, `{Kind: EvPointer, Action: PointerPress|PointerRelease, Button: ButtonLeft, Pos: frame.Point}`, `{Kind: EvAXSetValue, AXTarget: surface.AXTargetAddressBar, Text: string}`.

## 3. Quyền sở hữu file (BẮT BUỘC TU THỦ)

- **Chỉ tạo file mới** trong `cmd/goosie/`, khuyến nghị `journey_p0_test.go`, `journey_p0_tabs_test.go`, `journey_p0_error_test.go`.
- **CẤM sửa** các test file hiện có trong `cmd/goosie/` (để tránh conflict với TASK-014 đang tag `test/**`), cấm sửa `internal/**`, `test/**`, `docs/qa-*`.
- Được phép **đọc** mọi nơi để audit.
- Nếu một hành trình P0 đòi thay đổi production code (tức bạn tìm thấy bug thật): **đừng sửa engine**. Viết test chứng minh bug, ghi rõ vào Status, và để integrator quyết. Test đó phải để ở chế độ `t.Skip` kèm issue note nếu nó làm đỏ suite — nhưng ưu tiên: tìm cách assert đúng hành vi hiện tại và báo cáo độ lệch.

## 4. Danh sách case mục tiêu

P0 hành trình, nằm trong phạm vi `cmd/goosie` + headless:

`BR-001` (mở URL HTTPS đủ, URL cuối và nội dung cùng nguồn) · `BR-002` (HTTP hợp lệ, scheme không bị đổi ngoài redirect) · `BR-003` (hostname không scheme → chuẩn hoá đúng host) · `BR-007` (scheme sai cú pháp → lỗi phục hồi được, không treo, **không dùng URL cũ như thể thành công**) · `BR-012` (server không chạy → lỗi dễ nhận biết, vẫn nhận URL kế tiếp) · `BR-014` (A chậm rồi B; A về sau B → A không ghi đè B) · `BR-017` (Back/Forward qua ≥5 navigation, mỗi bước đúng entry/URL/tài liệu) · `BR-023` (khởi đầu có một tab dùng được) · `BR-026` (chuyển qua lại 3 tab → URL/title/nội dung đúng tab) · `BR-030` (đóng tab active còn tab khác → tab kế được chọn, không crash) · `BR-125` (`Cmd+L` → address bar nhận focus, nội dung được chọn để thay thế).

Ngoài phạm vi task này (đừng làm): `BR-059`/`BR-180` cần Chromium parity (TASK-016), `BR-162`/`BR-163`/`BR-165`/`BR-166` là SEC limits nằm ở `test/entrypoints` + `test/engine` (TASK-014 tag), `BR-088` thuộc `test/js`, `BR-179` là meta của CI.

## 5. Bước thực hiện

1. **Audit trước khi viết.** Với từng case ở mục 4, grep `cmd/goosie/` xem hành vi đó đã có test chưa (ví dụ `TestStaleNavResultCannotPaintOverTheCurrentOne` nhiều khả năng đã phủ `BR-014`; `tab_lifecycle_test.go` phủ `BR-026`/`BR-030`). Lập bảng `case → test đã có / cần viết mới`. **Không trùng lặp** — case đã phủ thì chỉ cần tag.
2. Viết test cho phần còn thiếu, theo đúng mẫu `browserE2E` hiện có (local fixture HTTP server qua `net/http/httptest` như các test hiện dùng; profile tạm; `b.close(t)` qua `defer`).
3. **Nguyên tắc assertion** (đây là chỗ tạo ra chất lượng "như manual"):
   - Assert **điều kiện post-condition hành trình** mà catalog đã nêu, bằng AX + state: `findAXNode(...)` ra `RoleLink` với `Href` đúng, `ax.Node{AddressBar:true, Focused:true}` sau `Cmd+L`, `RoleTab` có `Selected` đúng tab, `b.activeURL()` khớp URL đích cuối sau redirect, trang lỗi có `RoleStaticText` chứa host.
   - **Không** assert toạ độ/pixel/số đo geometry tự đặt.
   - Dùng `b.waitFor` với mô tả dễ đọc, **không** `time.Sleep` cố định.
   - Khi fail phải in `b.diagnostics()` để người đọc tái hiện được như QA manual ghi bằng chứng.
4. **Tag traceability** — quy ước y hệt TASK-014 (2 agent chạy song song, cùng format):

```go
// qc:BR-017
func TestJourneyBackForwardFiveNavigations(t *testing.T) {
```

5. Chạy `go test -count=1 ./cmd/goosie`, `go test -race -count=1 ./cmd/goosie`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test -count=1 ./...`.

## 6. Tiêu chí nghiệm thu

- **[AC-01]** `go test -count=1 ./cmd/goosie` PASS; **[AC-02]** cùng package sạch với `-race`.
- **[AC-03]** `go build ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test -count=1 ./...` PASS.
- **[AC-04]** 11 case mục tiêu: mỗi case hoặc có test mới, hoặc được chứng minh đã có test phủ (kèm tên test thật, kiểm tra bằng cách chạy `-run`).
- **[AC-05]** Mỗi test mới có ≥1 tag `// qc:BR-xxx`; `gofmt -l` trên file bạn tạo trả về rỗng.
- **[AC-06]** Trong Status, ghi bảng audit: case → test phủ → *đã có từ trước / mới viết* → **hành vi lệch so với kỳ vọng catalog nếu phát hiện bug**.

## 7. Ràng buộc vận hành

- Nhiều agent đang ghi song song vào repo. **Chỉ commit trên branch của bạn**, không `git push`, không merge `main`.
- **Đừng tạo worktree con hay branch phụ** từ trong phiên của bạn; commit thẳng lên branch hiện tại.
- Test phải chạy được trên máy **không có display** (đây là hợp đồng của repo: mọi test pass headless).
- Nếu `cmd/goosie` trở nên quá chậm để lặp, dùng `-run` theo tên test của bạn khi dev, nhưng AC-03 phải chạy full.

## 8. Resolution của integrator (2026-10-25)

Sau merge, `go test -count=1 ./...` trên `main` **FAIL** ở `TestJourneyCloseActiveTabSelectsNext`:

```
journey_p0_tabs_test.go:208: second tab ID = 1 must be distinct from first tab ID = 1
```

Nguyên nhân là bug thật trong test, không phải flake của engine: `waitFor` chỉ chốt `tabMgr.Count() == 2` rồi dòng ngay sau đó đọc `tabMgr.Active().ID()`. Số tab đổi trước khi selection đổi (hai việc rời nhau trong loop), nên dưới tải của full suite, `Active()` vẫn trả về tab 1. Xảy ra ở 4 chỗ (`tab2ID`, `tab3ID`, `secondID`, và `active` sau `Cmd+W`).

**Bài học cho mọi hành trình kế tiếp — điều kiện chờ phải bao gồm đúng thứ dòng kế tiếp đọc.** Integrator đã sửa thành:

```go
browser.waitFor(t, "second tab created and focused", func() bool {
    return browser.f.tabMgr.Count() == 2 && browser.f.tabMgr.Active().ID() != firstID
})
```

Riêng `Cmd+W`, post-condition của BR-030 chính là *tab còn lại được chọn*, nên chờ `Count() == 1 && Active().ID() == firstID` thay vì chỉ đếm.

Nghiệm thu: `go test -count=5 -run TestJourney ./cmd/goosie/` PASS dưới tải (một agent khác đang compile song song), và `go test -count=1 ./...` PASS toàn suite (exit 0, không FAIL/panic). Commit `95c427f`. Quy tắc này chuyển nguyên văn sang TASK-020.
