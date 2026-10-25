# TASK-014: QC traceability gate — bắt buộc mỗi QC case có test tương ứng

- **Task ID**: `TASK-014`
- **Type**: Test infrastructure / Quality gate
- **Assigned to**: `codex`
- **Review**: `qoder cli` (integrator)

---

## 1. Bối cảnh & Mục tiêu

`docs/qa-test-catalog.md` chứa **185 QC case** (`BR-001`–`BR-185`), mỗi case có mã, priority (`P0/P1/P2`), method (`INT`/`PARITY`/`CUA`/`SEC`/`GAP`) và trạng thái (`NOT RUN`/`PASS`/`FAIL`/`BLOCKED`/`DEFERRED`).

Đã đo và xác nhận:

1. **Zero traceability**: không một file `.go/.py/.js/.sh/.yml` nào tham chiếu `BR-xxx`. Toàn bộ ánh xạ QC↔test chỉ tồn tại dưới dạng văn prose trong `docs/qa-test-plan.md` (ví dụ dòng "**Automation hiện có:** `cmd/goosie/tab_lifecycle_test.go:TestStaleNavResultCannotPaintOverTheCurrentOne`"), không có gì kiểm chứng.
2. **Trạng thái stale by construction**: 167/185 là `NOT RUN` dù repo có 2085 test. `BR-179` (P0: "`go test ./...`, `go vet -all ./...`, archtest và gate suite exit 0") chính là điều CI đã làm mỗi push, vẫn `NOT RUN`. `BR-014` (P0) đã có test nhưng vẫn `NOT RUN`.
3. Quy tắc đóng run ở cuối catalog ("Không đổi `NOT RUN` thành `PASS` nếu thiếu expected result hoặc bằng chứng", "Case tự động phải lưu lệnh, exit code và artifact") hiện **không machine-enforced**.

**Mục tiêu**: biến trạng thái QC thành thứ **dẫn xuất được từ code**, không gõ tay. Gate phải *fail khi tài liệu và code lệch nhau*, theo cả hai chiều.

## 2. Phạm vi & quyền sở hữu file (BẮT BUỘC TU THỦ)

Chỉ được tạo/sửa trong:

- `test/qc/**` (package mới; `test/` được phép import bất cứ gì theo quy tắc archtest)
- `docs/tasks/TASK-014-*.md` (cập nhật phần Status khi xong)
- `test/**` — **chỉ** thêm dòng comment `// qc:BR-xxx` vào doc comment của các `func Test*` / `func Fuzz*` đã tồn tại
- `cmd/goosie/**` — **CẤM SỬA**. Package này do TASK-015 sở hữu song song. Gate của bạn được *đọc* `cmd/goosie` để quét tag, nhưng không được sửa file nào ở đó.

Không sửa `docs/qa-test-catalog.md` / `docs/qa-test-plan.md` (dữ liệu đầu vào của gate; sửa nó = tự ghi điểm). Không sửa `internal/**`.

## 3. Yêu cầu thiết kế

### 3.1. Registry (parse catalog, không tạo bản sao)
Đọc thẳng `docs/qa-test-catalog.md` làm nguồn duy nhất. Dòng case có dạng:

```
- **BR-050 | P2 | CUA, INT, SEC | BLOCKED** — mô tả...
```

Parse thành: `ID`, `Priority`, `Methods[]`, `Status`, và phần mô tả sau `—`. Ghi rõ số case parse được phải = 185; nếu regex bỏ sót case nào thì **fail** (đây là check quan trọng, tránh gate "xanh" vì parse thiếu).

### 3.2. Tag trong test
Quy ước: một dòng comment ngay trên khai báo test func.

```go
// qc:BR-014 BR-021
func TestStaleNavResultCannotPaintOverTheCurrentOne(t *testing.T) {
```

Quét bằng `go/parser` + `go/ast` trên `test/**` và `cmd/goosie/**`, lấy `Doc` của `FuncDecl` tên `Test*`/`Fuzz*`. Một test có thể phục vụ nhiều case.

### 3.3. Assert (gate)
1. **Missing-test**: case có method `INT`, `PARITY` hoặc `SEC` mà **không** có test tagged → FAIL.Case chỉ `CUA` và/hoặc `GAP` được miễn.
2. **Unknown-ID**: tag trỏ tới ID không có trong catalog → FAIL.
3. **Fake-PASS**: case trạng thái `PASS` mà không có test tagged → FAIL. (Cơ giới hoá đúng quy tắc dòng 234 của catalog.)
4. **Stale-status**: case trạng thái `NOT RUN` nhưng **đã có** test tagged → FAIL, kèm thông điệp bảo đổi trạng thái. Check này làm tài liệu tự hết stale.

### 3.4. Chế độ chuyển tiếp (để gate xanh ngay khi merge)
Giai đoạn 1 chỉ tag **19 case P0** và **10 case SEC** (tổng 29, có thể giao nhau). Vì vậy:
- Duy trì một allowlist nhỏ `enforced.go` trong `test/qc` liệt kê ID đã bắt buộc. Rule 1 và 4 chỉ áp dụng cho ID trong allowlist.
- Với phần còn lại, in báo cáo tổng hợp (số case đã có test / chưa có test / CUA-only) qua `t.Logf`, không fail.
- Gate phải **tăng dần** được: thêm ID vào allowlist là bước mở rộng duy nhất.

### 3.5. Evidence artifact
Catalog đòi "lệnh, exit code và artifact". Thêm `-race` an toàn và ghi `test/qc/traceability.golden.json` (chế độ `go test ./test/qc -update` regenerate): per-case `{id, priority, methods, status, tests[], enforced bool}`. File này là ma trận truy vết để người và CI đọc.

## 4. Việc cụ thể

1. Điều tra trước: với mỗi một trong 19 P0 case, dùng grep để tìm test *đã tồn tại* phục vụ nó (nhiều khả năng có sẵn trong `cmd/goosie/tab_lifecycle_test.go`, `test/entrypoints/limits_test.go`, `test/engine/*`). Chỉ tag cái đã có; **không viết test mới** ở task này.
2. Nếu một P0 case thực sự không có test nào và method là `INT`/`SEC` → **không** cho vào allowlist, mà ghi thẳng vào báo cáo cuối như khoảng trống thật. Đây là đầu ra có giá trị nhất của task: liệt kê chính xác P0 case trắng.
3. Viết `test/qc/traceability_test.go` + `test/qc/catalog.go` (parser) + `test/qc/enforced.go`.

## 5. Tiêu chí nghiệm thu

- **[AC-01]** `go test ./test/qc/` PASS.
- **[AC-02]** Parser xác nhận đúng 185 case; thử cố tình làm hỏng 1 dòng catalog bằng test tạm → gate fail (chứng minh nó không im lặng bỏ qua).
- **[AC-03]** `go test ./test/qc/ -run TestTraceability` fail với thông điệp rõ nếu thêm tag `// qc:BR-999` lạ vào bất kỳ test nào (tự kiểm chứng rồi revert).
- **[AC-04]** `go build ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test ./...` đều PASS.
- **[AC-05]** `test/qc/traceability.golden.json` được commit và chứa ≥29 case enforced có `tests` không rỗng.
- **[AC-06]** Báo cáo cuối (trong `docs/tasks/TASK-014-*.md`, mục Status) nêu: số P0/SEC case đã có test, và **danh sách P0 case không có test tự động nào**.

## 6. Ràng buộc vận hành

- Repo này đang có nhiều agent ghi song song. **Chỉ commit trên branch của bạn**, không `git push`, không merge vào `main`.
- Trước khi commit chạy `gofmt -l` trên file bạn tạo (archtest kiểm gofmt toàn repo và sẽ fail cả repo vì bạn).
- Nếu phát hiện `cmd/goosie` cần tag cho P0 case: đừng sửa; liệt kê vào Status để integrator làm.
