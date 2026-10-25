# TASK-023: `scripts/release-gate.sh` — biến QC case BR-179 thành thứ chạy được thật

- **Task ID**: `TASK-023`
- **Type**: Test infrastructure / Release assurance
- **Assigned to**: `qoder cli` (integrator tự làm, vì nó nối trực tiếp TASK-018)
- **Review**: integrator

## 1. Vấn đề

BR-179 (P0, INT) trong `docs/qa-test-catalog.md:215`: *"Chạy `go test ./...`, `go vet -all ./...`, archtest và gate suite trên commit ứng viên → tất cả lệnh exit 0; lưu log/artifact."*

Cho tới trước task này, đó là **một câu mô tả quy trình**, không phải một thứ chạy được. Không có script nào chứa bốn lệnh đó, không có artifact nào được lưu, và vì vậy không có oracle nào để fail. Đây là P0 case cuối cùng (cùng BR-180) của catalog chưa có automation dù repo có tới 6 job CI.

Hai lỗi cụ thể mà "chạy tay" mắc phải, và script này chặn:
- Quên `-count=1`: `go test ./...` tái dùng kết quả cache từ **một tree khác** rồi in `ok` — release được "chứng minh" bởi code không phải đang ship.
- Chạy thiếu archtest: toàn bộ quy tắc import-graph (`internal/archtest/rules.go:40-64`) không được kiểm trên commit ứng viên.

## 2. Thiết kế

`scripts/release-gate.sh` — danh sách gate dựng sẵn, `bash -c` từng lệnh, tee stdout+stderr của mỗi lệnh vào log riêng dưới `-out`, rồi ghi `release-gate.json` (status, số lệnh, exit từng lệnh, số giây, đường dẫn log).

- Hợp đồng exit-status **giống hệt** `v2-gate-check.sh` và `release-smoke.sh` mà repo đã có: `0` pass, `1` có lệnh fail, `2` **không có verdict**.
- `127` (lệnh không tồn tại) → `2`, không phải `1`. Đây là chỗ gate tự-run bằng người hay sai nhất: `go vet -all` đánh nhầm thành `go vet -alls` thì shell báo "command not found", người bấm release thấy cửa sổ scroll qua rồi tưởng suite sạch.
- **Chạy tiếp sau khi có lệnh fail**: "vet sạch trên commit này" vẫn là thông tin đáng giá, và bắt người dùng chạy lại cả gate để biết điều đó là cách nhanh nhất khiến họ bỏ qua gate.
- `-commands FILE` thay danh sách lệnh: tồn tại chỉ để test chứng minh fail-closed mà không phải chạy cả suite bốn lần. `-list` in danh sách dựng sẵn dạng JSON.

Danh sách dựng sẵn (thứ tự tín hiệu rẻ trước):

```
go test -count=1 ./...
go vet -all ./...
go test -count=1 ./internal/archtest/
go test -count=1 -run TestGate ./test/gate/
```

`test/gatecheck/release_gate_test.go` (8 test) là hợp đồng của file script — cùng khuôn với `release_smoke_test.go` đã có. Phần quan trọng nhất: `TestReleaseGateDefaultListIsTheQCGate` assert chính xác bốn lệnh trên **và** assert chuỗi `-count=1` và `-run TestGate` còn nguyên, nên rút ngắn danh sách hoặc bỏ `-count=1` đều làm CI đỏ.

## 3. Nghiệm thu

| AC | Kết quả |
| --- | --- |
| BR-179 có automation, enforced | `test/qc/enforced.go` thêm `BR-179`; golden ghi `tests: [TestReleaseGateDefaultListIsTheQCGate, TestReleaseGateFailsWhenOneCommandFails]` |
| 8 test hợp đồng PASS | `go test -count=1 -run TestReleaseGate ./test/gatecheck/` → `ok ... 2.112s` |
| **Mutation proof** | Tạm xoá `"go vet -all ./..."` khỏi `GATE_COMMANDS` → `FAIL: built-in gate list is missing "go vet -all ./..."`; revert → PASS |
| fail-closed | `exit 3` từ một lệnh → script exit `1`, report `status: fail`, log lệnh đó vẫn còn trên đĩa |
| no-verdict | lệnh không tồn tại → exit `2`, report `no_verdict: 1` |
| invocation sai | `-commands` file không có / `-out` thiếu argument / cờ lạ → exit `2`; list chỉ có comment → exit `2` **và không để lại report** |
| Guards | `go test ./test/qc/`, `./internal/archtest/`, `./test/gatecheck/` PASS; `gofmt -l` sạch |
| Gate chạy thật trên `main` | `make release-gate` — xem mục 5 |

## 4. Cái task này **không** làm

- Không nối vào `.github/workflows/ci.yml`: CI đã chạy cả bốn lệnh đó ở các job riêng, chạy thêm lần nữa trong cùng một pipeline là trả gấp đôi thời gian cho cùng một thông tin. Script này dành cho **commit ứng viên** (trước khi tag `v*`), còn CI vẫn là lưới dự phòng.
- Không đổi catalog (`docs/qa-*`), không tự sign-off BR-179. Catalog vẫn `NOT RUN`: script chứng minh *gate tồn tại và chạy được*, còn "QC đã xem log của release candidate" là việc của người. Gate này báo cáo, không ký thay ai.
- Không chứa parity (`testdata/parity.py`) hay WPT: BR-179 không yêu cầu; nhúng vào sẽ làm gate mất vài tiếng và khiến nó bị bỏ qua.

## 5. Kết quả chạy thật đầu tiên

`make release-gate` trên `main` (tree của commit `36993cf`, gồm cả test gatecheck mới):

```
release-gate: FAILED (1) go test -count=1 ./... - see /tmp/goosie-release-gate/01-go-test-count-1.log
release-gate: ok (2s)  go vet -all ./...
release-gate: ok (7s)  go test -count=1 ./internal/archtest/
release-gate: ok (28s) go test -count=1 -run TestGate ./test/gate/
release-gate: FAIL (report: /tmp/goosie-release-gate/release-gate.json)
```

Report: `status: fail`, 4 lệnh, một lệnh exit 1 sau 51s. **Lượt chạy đầu tiên của gate đã bắt được một bug thật**: `test/js TestFetchCatchOnError` fail, và log chỉ đúng dòng `fetch_test.go:374: gotCatch = {Kind:4 ...}`. Đó chính là ordering bug mà TASK-017 để lọt — giờ nó là `TASK-025`. Không có gate thì bug này chỉ lộ ra khi ai đó tình cờ chạy đúng seq command dưới tải.

Lưu ý cho người đọc log: khi recipe fail, **make** trả `2` (quy ước của GNU make), không phải exit của script. Script gọi trực tiếp (`scripts/release-gate.sh`) trả đúng `0/1/2` như hợp đồng; gatecheck test gọi script nên không bị lớp `make` che mất.

Artifact: `/tmp/goosie-release-gate/release-gate.json` (per-command exit/seconds/log path) + log từng lệnh. Đây là phần "lưu log/artifact" mà BR-179 yêu cầu và chưa từng tồn tại trước task này.
