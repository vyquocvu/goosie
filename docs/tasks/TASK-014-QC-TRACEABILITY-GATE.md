# TASK-014: QC traceability gate — bắt buộc mỗi QC case có test tương ứng

- **Task ID**: `TASK-014`
- **Type**: Test infrastructure / Quality gate
- **Assigned to**: `codex`
- **Review**: `qoder cli` (integrator)

## Status

### Coverage audit

The source catalog parses as 185 rows (BR-001–BR-185), including 19 P0 cases. It contains **18 SEC-method cases**, not the 10 stated in sections 1 and 4. The audit found an existing automated test for 17 of 19 P0 cases and all 18 SEC-method cases. The remaining P0 gaps are:

| Case | Automated coverage | Audit result |
| --- | --- | --- |
| BR-179 | No case-specific automated test found. The build, vet, archtest, and gate commands are release checks, not a test function exercising this case. | Gap |
| BR-180 | No automated test covers the complete release sign-off and native CUA/live smoke described by the case. Individual parity and UI behaviors have separate tests. | Gap |

Existing P0 coverage found during the audit:

| Case | Existing test evidence |
| --- | --- |
| BR-001 | `cmd/goosie/e2e_test.go` address-bar navigation journey |
| BR-002 | `test/net/http_test.go` HTTP response tests; browser navigation is also exercised under `cmd/goosie` |
| BR-003, BR-007 | `cmd/goosie/normalize_url_test.go` |
| BR-012 | `cmd/goosie/main_test.go:TestLoadErrorsOmitQuerySecrets` exercises a refused connection and surfaced navigation error |
| BR-014 | `cmd/goosie/tab_lifecycle_test.go:TestStaleNavResultCannotPaintOverTheCurrentOne` |
| BR-017 | `test/toolbar/toolbar_test.go:TestHistoryBackForward` |
| BR-023 | `test/tabs/manager_test.go:TestNewTabHasCorrectInitialState` |
| BR-026 | `cmd/goosie/tab_lifecycle_test.go` independent and out-of-order tab navigation tests |
| BR-030 | `test/tabs/lifecycle_test.go:TestCloseActiveTabSelectsNearest` |
| BR-059 | `test/engine/layout_parity_test.go:TestLayoutParityWithChromium` |
| BR-088 | `test/engine/script_test.go:TestScriptsRunInDocumentOrder` |
| BR-125 | `cmd/goosie` address-bar shortcut tests |
| BR-162 | `test/entrypoints/limits_test.go:TestGoosieHeadlessRejectsOversizedInput` |
| BR-163 | `test/engine/bounds_test.go` node/depth limit cases |
| BR-165 | `internal/image/decode_test.go:TestProbeRejectsEncodedByteLimit` |
| BR-166 | `test/engine/adversarial_test.go:TestOversizedImageHeaderRejectedBeforeDecode` and `internal/image/decode_test.go` dimension/pixel limit cases |

All 18 SEC-method cases have existing test coverage identified in the repository. The 10 SEC cases selected for this transition allowlist are BR-009, BR-010, BR-011, BR-151, BR-159, BR-161, BR-162, BR-164, BR-166, and BR-167. The remaining SEC evidence is BR-050/051 (`test/bookmarks`, `test/history`, and `cmd/goosie/mgmt_pages_test.go`), BR-119/120 (`test/js` fetch and CORS tests), BR-160 (`cmd/goosie/address_guard_test.go`), BR-163 (`test/engine/bounds_test.go`), BR-165 (`internal/image/decode_test.go`), and BR-168 (`test/net/http_test.go` URL validation). Some of these tests are in `cmd/goosie` or `internal`, where this task's ownership rules prohibit adding tags.

The tag scan currently finds 21 distinct cases with tests (15 in the transition allowlist, BR-163, and the five already-PASS cases BR-181–BR-185). The golden artifact therefore has only 15 enforced cases with non-empty `tests`, below AC-05's requirement of at least 29. Existing test functions in `cmd/goosie/**` cannot be tagged under section 2; the cases they cover remain listed above for the integrator to tag in the owning task.

### Gate constraint and verification

The traceability test correctly rejects tagged `NOT RUN` cases and the five catalog `PASS` cases are tagged. However, all 15 currently enforced cases are still `NOT RUN` in the catalog. Section 2 prohibits editing that source catalog, so AC-01 cannot pass until its owners record evidence and update those statuses. The specification also asks for at least 29 enforced cases while section 4 limits this transition to 19 P0 plus 10 SEC cases and excludes uncovered cases; the catalog's 18 SEC cases and the untaggable `cmd/goosie`/`internal` tests make those counts inconsistent.

The checker and golden artifact are implemented. Verification results:

| Command | Result |
| --- | --- |
| `go build ./...` | Exit 0. Go printed a non-fatal module stat-cache write warning for a path outside the workspace. |
| `go vet -all ./...` | Exit 0, no output. |
| `go test ./internal/archtest/` | PASS (`ok`, 3.104s). |
| `go test ./test/qc/` | FAIL as designed by the source mismatch: all 15 enforced cases are still `NOT RUN`. Trace reports 21 tagged cases, 164 without tags, and 32 CUA/GAP-only without tags. |
| `go test ./test/qc/ -run '^TestCatalogRejectsMalformedCaseRow$' -count=1` | PASS (`ok`, 0.321s); a malformed case row is rejected. |
| Unknown-ID probe with temporary `// qc:BR-999` on `test/tabs/manager_test.go:TestCount` | FAIL detected `unknown QC ID BR-999`; the temporary tag was removed. |
| `go test -count=1 ./...` | FAIL. It includes the QC stale-status failures and environment failures: local TCP listener binding denied, outbound DNS/network unavailable for the WPT checkout, clipboard tests failed, and Darwin AVFoundation media playback is unavailable. Several packages passed, including archtest, gate, gatecheck, and most engine packages. |

AC-01, AC-04's full-suite requirement, and AC-05 are not met in this worktree. AC-02's malformed-row failure path and AC-03's unknown-ID failure path were verified. The catalog/status ownership conflict prevents a green QC gate without a catalog update by its owner.

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
4. **Awaiting sign-off** (report-only, không fail): case trạng thái `NOT RUN` nhưng đã có test tagged → `t.Logf` nêu tên test. Coverage và sign-off là hai thứ khác nhau: có test được tag chưa có nghĩa là con người đã xác nhận testAssert đúng expected result. Buộc đổi trạng thái mới tạo áp lực ghi `PASS` giả.

### 3.4. Chế độ chuyển tiếp (để gate xanh ngay khi merge)
Giai đoạn 1 chỉ enforce các case đã thực sự có oracle tự động đo được (codex xác định 15 case), không enforce theo toàn bộ P0/SEC trên giấy.
- Duy trì một allowlist nhỏ `enforced.go` trong `test/qc` liệt kê ID đã bắt buộc. Rule 1 chỉ áp dụng cho ID trong allowlist; rule 4 là report-only trên toàn catalog.
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
- **[AC-05]** `test/qc/traceability.golden.json` được commit; **toàn bộ** case trong allowlist có `tests` không rỗng, và tổng số case được tag được báo cáo bằng số đo thực tế (không chạy theo chỉ tiêu).
- **[AC-06]** Báo cáo cuối (trong `docs/tasks/TASK-014-*.md`, mục Status) nêu: số P0/SEC case đã có test, và **danh sách P0 case không có test tự động nào**.

## 6. Ràng buộc vận hành

- Repo này đang có nhiều agent ghi song song. **Chỉ commit trên branch của bạn**, không `git push`, không merge vào `main`.
- Trước khi commit chạy `gofmt -l` trên file bạn tạo (archtest kiểm gofmt toàn repo và sẽ fail cả repo vì bạn).
- Nếu phát hiện `cmd/goosie` cần tag cho P0 case: đừng sửa; liệt kê vào Status để integrator làm.

## 7. Resolution của integrator (2026-10-25)

Ba điểm yếu của đặc tắc ban đầu đã được sửa theo đúng phát hiện của codex:

1. **Rule 4 không còn fail build.** `NOT RUN` + đã có test tagged là trạng thái hợp lệ của giai đoạn chuyển tiếp; gate chỉ `t.Logf`. Lý do: buộc đổi trạng thái mới chính là cơ chế tạo ra `PASS` giả mà catalog cấm (dòng 236). Ba rule còn lại vẫn fail cứng: thiếu test cho case enforced, ID lạ, và `PASS` không có test.
2. **`scanTests` bổ sung `internal/`.** Bằng chứng thật cho BR-165/BR-166 nằm ở `internal/image/decode_test.go` (`TestProbeRejectsEncodedByteLimit`, `TestProbeRejectsOversizedDimensions`, `TestProbeRejectsJustOverPixelLimit`, `TestProbeAcceptsAtLimit`) — trước đây scanner không nhìn thấy nên không thể tag.
3. **AC-05 bỏ chỉ tiêu 29.** Con số giờ là số đo: allowlist 24 case, cả 24 đều có test; tổng 29/185 case được tag.

Integrator đã tag phần `cmd/goosie` và `internal/image` mà codex bị ràng buộc ownership cấm chạm: BR-002 `TestLoadURLCtxValidSmallDocument`, BR-003 `TestNormalizeURL` + `TestNormalizeAddressAmbiguousInputs`, BR-007 `TestCertFailureReachesAddressBar`, BR-012 `TestCategorizeError` + `TestLoadErrorsOmitQuerySecrets`, BR-014 `TestStaleNavResultCannotPaintOverTheCurrentOne`, BR-028 `TestOutOfOrderResultsLandOnTheirOwnTabs`, BR-036 `TestIndependentTabNavigation`, BR-165/BR-166 như mục 2.

Khoảng trống còn mở, ghi nhận trung thực:
- **BR-166 yếu ở tầng engine**: `test/engine/adversarial_test.go:TestOversizedImageHeaderRejectedBeforeDecode` chỉ Assert session không chết, không Assert ảnh bị từ chối — nó sẽ vẫn xanh nếu engine giải mã bừa. Oracle thật hiện nằm ở `internal/image`. Cần siết test engine hoặc gỡ tag engine khi có bằng chứng trực tiếp.
- **BR-001, BR-026, BR-125** thuộc `cmd/goosie` journey: chờ TASK-015 (agy) tag hành trình thật, chưa enforce.
- **BR-179, BR-180** không có oracle tự động;BR-180 là release sign-off thủ công, giữ nguyên khoảng trống.

### Verify lại sau khi sửa

| Lệnh | Kết quả |
| --- | --- |
| `go test -count=1 ./test/qc/` | PASS; trace: 29 tagged / 156 chưa tag / 32 CUA-GAP-only / 24 chờ sign-off |
| `go test -count=1 ./internal/archtest/ ./internal/image/ ./cmd/goosie/` | PASS |
| `go build ./...`, `go vet -all ./test/qc ./internal/image ./cmd/goosie`, `gofmt -l` | sạch |
