# TASK-020: Hành trình P1 cho navigation và tab (tiếp nối TASK-015)

- **Task ID**: `TASK-020`
- **Type**: Test coverage / User journey automation
- **Assigned to**: `agy`
- **Review**: `qoder cli` (integrator)

## 1. Bối cảnh

TASK-015 của bạn đã merge: 11 hành trình P0 trong `cmd/goosie/journey_p0*.go`, assert qua accessibility tree, đã tag `// qc:BR-xxx` và đã được QC traceability gate (`test/qc/`) ghi nhận — hiện 32/185 case có automation, 17/19 P0 case đã phủ.

Gate đó đang báo 27 case "có automation nhưng chờ sign-off". Việc của task này là **tăng số case có hành trình thật**, không phải đổi trạng thái catalog. Quy tắc cũ vẫn còn hiệu lực: không sửa engine, không sửa catalog, không pixel test.

## 2. Việc cụ thể

### 2.1. Audit trước khi viết (bắt buộc, kết quả ghi vào Status)

Với **mỗi** case trong mục 2.2, tìm test đã tồn tại bằng `go test -run` + grep trong `cmd/goosie/*_test.go` và `test/**`. Nếu đã có test assert đúng expected result của case: **chỉ thêm tag `// qc:BR-xxx`** lên function đó (thêm dòng, không sửa logic), không viết test trùng. Ghi vào Status dạng bảng: `case | test đã có | action (tag mới / test mới / không có)`.

### 2.2. Danh sách case (12)

Domain A — navigation:

| Case | Hành trình cần chứng minh | Post-condition phải assert |
| --- | --- | --- |
| BR-004 | Nhập URL có path + query + fragment | Request gửi đi giữ nguyên path/query; tài liệu giữ fragment |
| BR-008 | Trang trả redirect 1 bước | Address bar hiển thị URL đích cuối; Back quay về trang trước, không loop |
| BR-013 | Trang chậm, bấm Stop | Request hiện tại dừng; tài liệu đang hiển thị không bị xoá ngoài policy |
| BR-015 | Bấm link thường trong cùng tab | URL, tài liệu và history đồng bộ một lần, không entry giả |
| BR-016 | Anchor cùng trang và anchor từ trang khác | Fragment đúng và viewport nhảy tới đúng mục tiêu |
| BR-020 | Reload trang có query/fragment | Tải lại URL hiện hành, history không thêm visit ngoài policy |
| BR-021 | Refresh khi response cũ còn pending | Chỉ kết quả navigation mới nhất được áp dụng |
| BR-022 | Focus address bar, chọn toàn bộ, gõ URL mới | Không lọt ký tự vào document phía sau |

Domain B — tab:

| Case | Hành trình cần chứng minh | Post-condition phải assert |
| --- | --- | --- |
| BR-024 | Mở tab mới bằng nút và bằng shortcut | Tab mới được focus và nhận nhập liệu |
| BR-027 | Tài liệu hoàn tất ở background tab | Nội dung tab foreground không đổi |
| BR-034 | Đổi tab khi mỗi tab có scroll offset riêng | Quay lại: đúng tài liệu và đúng vị trí cuộn |
| BR-035 | Mỗi tab có Back/Forward riêng | Lịch sử không dùng chung giữa hai tab |

### 2.3. Cách viết

Dựng trên harness có sẵn trong `cmd/goosie/e2e_test.go` — **tái sử dụng, không viết lại**: `newBrowserE2E`, `close`, `inject`, `pressKey`, `click`, `clickNode`, `clickToolbarButton`, `navigateViaAddressBar`, `waitFor`, `diagnostics`, `activeURL`, `findAXNode`, `axContainsLabel`.

Mỗi test: tạo fixture qua `httptest.NewServer`, thao tác như người dùng, rồi assert **post-condition ngữ nghĩa qua accessibility tree / tab state / history**, không assert ảnh. Khi cần chờ điều kiện bất định, dùng `waitFor` (đã fail rõ khi timeout) chứ không `time.Sleep`.

Đặt tên file `cmd/goosie/journey_p1_nav_test.go` và `cmd/goosie/journey_p1_tabs_test.go`. **Chỉ tạo file mới trong `cmd/goosie/`**; sửa file có sẵn chỉ theo nghĩa thêm dòng tag.

**Quy tắc bắt buộc về `waitFor` (rút từ failure thật sau merge của TASK-015, xem `docs/tasks/TASK-015-P0-JOURNEY-SUITE.md` §8):** điều kiện chờ phải bao gồm **đúng thứ dòng code kế tiếp đọc**. Sai là kiểu: chờ `tabMgr.Count() == 2` rồi ngay sau đó đọc `tabMgr.Active().ID()` — số tab và selection đổi ở hai iteration khác nhau, nên test xanh khi chạy một mình và fail dưới tải full suite (`second tab ID = 1 must be distinct from first tab ID = 1`). Đúng là đưa cả hai về một predicate:

```go
browser.waitFor(t, "second tab created and focused", func() bool {
    return browser.f.tabMgr.Count() == 2 && browser.f.tabMgr.Active().ID() != firstID
})
```

Vì vậy `[AC-01]` yêu cầu `-count=5` chứ không phải `-count=1`: một predicate thiếu điều kiện thường vẫn qua ở lượt đầu.

Đầu mỗi test function, thêm dòng tag trên cùng của doc comment: `// qc:BR-0xx` (một function có thể tag nhiều case).

## 3. Tiêu chí nghiệm thu

- **[AC-01]** `go test -count=1 -race -run 'TestJourney' ./cmd/goosie/` PASS, và chạy lặp `go test -count=5 -run 'TestJourney' ./cmd/goosie/` PASS (không flake).
- **[AC-02]** `go test -count=1 ./cmd/goosie/` và `go test -count=1 ./test/qc/` PASS. Nếu bạn thêm tag mới, regenerate artifact: `go test -count=1 -run TestTraceability ./test/qc/ -update` rồi commit `test/qc/traceability.golden.json`.
- **[AC-03]** `go build ./...`, `go vet -all ./...`, `go test ./internal/archtest/` PASS; `gofmt -l cmd/goosie` sạch.
- **[AC-04]** Mỗi case trong mục 2.2 hoặc có test mới, hoặc có dòng tag, hoặc được ghi là "không automatable" kèm lý do trong Status. Không được im lặng bỏ qua.
- **[AC-05]** Self-review: với mỗi test mới, mô tả 1 câu "test này sẽ fail nếu tính năng X hỏng" — đưa vào Status. Test nào không trả lời được thì viết lại hoặc bỏ.
- **[AC-06]** Chạy được không cần display (mọi test goosie hiện tại chạy trên headless surface).

## 4. Ràng buộc vận hành

- **Chỉ tạo/ sửa file mới trong `cmd/goosie/`**, cộng tag trên function đã tồn tại ở đó, cộng `test/qc/traceability.golden.json` khi tag đổi.
- **Cấm sửa**: `docs/qa-*` (catalog là source của QC, integrator giữ), `internal/**`, `test/**` (agent khác đang sở hữu), `testdata/**`, `.github/**`, `docs/tasks/**` khác TASK-020.
- Không sửa engine để test pass. Nếu phát hiện bug engine cản hành trình, ghi vào Status kèm reproducer nhỏ, **dừng ở đó**, không vá.
- Chỉ commit trên branch `worktree-task-020-p1-journeys`. **Không `git push`, không merge vào `main`.**
