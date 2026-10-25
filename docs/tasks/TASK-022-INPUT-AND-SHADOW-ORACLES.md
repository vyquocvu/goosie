# TASK-022: Biến các test "chỉ không crash" thành test có oracle (fuzz input + shadow)

- **Task ID**: `TASK-022`
- **Type**: Test quality / Oracle strength
- **Assigned to**: `codex` (sau khi TASK-021 nghiệm thu)
- **Review**: `qoder cli` (integrator)

## 1. Bằng chứng: 5 fuzz target và 1 unit test không kiểm tra gì cả

`codex` lập danh sách này ở AC-05 của TASK-019; integrator đã kiểm lại bằng mắt. `test/engine/fuzz_input_test.go` có **đúng một** assertion trong cả file — `t.Fatalf("NewSession: %v", err)` ở dòng 38. Năm target còn lại chỉ kết luận "không panic":

| Vị trí | Hiện tại làm gì | Thứ bị vứt đi |
| --- | --- | --- |
| `fuzz_input_test.go:44` `FuzzClickAt` | `sess.ClickAt(x, y)` | giá trị `bool` trả về |
| `fuzz_input_test.go:59` `FuzzSetHover` | `sess.SetHover(x, y)` | giá trị `bool` |
| `fuzz_input_test.go:~70` `FuzzFocusControl` | `sess.FocusControl(10, 10)` | giá trị `bool` |
| `fuzz_input_test.go:100` `FuzzEdit` | `sess.Edit(action, r)` | giá trị `bool` + nội dung text |
| `fuzz_input_test.go:~118` `FuzzInteractionSequence` | chạy một chuỗi thao tác | mọi trạng thái sau đó |
| `test/style/shadow_test.go:195` `TestShadowPanicsNoPanic` | parse 9 khai báo shadow qua resolver | kết quả computed style |

Đây đúng là loại assurance rỗng mà chương trình QC này đang triệt tiêu: một resolver mà **lặng lẽ bỏ hết box-shadow** vẫn làm `TestShadowPanicsNoPanic` xanh; một `ClickAt` trả `false` cho mọi điểm vẫn xanh. "Không crash" là điều kiện cần, không phải kết quả kiểm thử.

Điểm thuận lợi: cả năm hàm input **đã trả `bool`** (`engine.go:1502 ClickAt`, `focus.go:76 FocusControl`, `focus.go:318 Edit`, `focus.go:818 SetHover`) và session đã expose `focus.go:220 Focused()`, `focus.go:461 FocusedText()`, `focus.go:471 FocusedEditable()`, `engine.go:1849 SelectionBounds()`, `selection.go:252 SelectionText()`. Không cần API mới cho phần lớn việc.

## 2. Yêu cầu

### 2.1 Fuzz input: thêm **bất biến**, không thêm quyền

Với mỗi target, giữ nguyên phần sinh input rồi thêm assert thoả mãn cả ba tính chất: (a) đúng với mọi input hợp lệ, (b) **fail khi hành vi hỏng**, (c) không phụ thuộc thời gian/địa chỉ bộ nhớ.

Bất biến tối thiểu phải có:

1. **Tính nhất quán trả về ↔ trạng thái.** `ClickAt`/`FocusControl` trả `true` ⇒ `Focused() != nil`. Trả `false` ⇒ `Focused()` **không đổi** so với snapshot trước đó. Đây là thứ mà `go test` hiện tại không hề đo.
2. **`Edit` có hậu quả thật.** Trên một editable đang focus: khi `Edit` trả `true` với action chèn ký tự `r` (in-range, không điều khiển), `FocusedText()` phải chứa `r` tại vị trí phù hợp; khi `Edit` trả `false`, `FocusedText()` phải **nguyên văn** như trước.
3. **Giới hạn không đổi.** Sau chuỗi thao tác random, session vẫn phải **paint được**: `sess.PaintChecked(...)` trả về không error và số layer > 0 (dùng đúng hợp đồng untrusted-input của repo). Đây là oracle thật cho "browser còn vẽ được sau khi bị bắn input", thứ QA manual hay thử bằng cách click loạn.
4. **Không âm thầm nới giới hạn admission.** Task này **cấm** sửa `internal/engine/limits.go`, `internal/image/decode.go`, hoặc bất kỳ hằng số nào.

### 2.2 Shadow: assert computed style, không assert "không panic"

`TestShadowPanicsNoPanic` đổi tên/ thêm test mới sao cho mỗi khai báo trong danh sách được kiểm **số layer shadow và các thành phần offset/blur/color** mà resolver tạo ra. Ví dụ `box-shadow: 1px 1px` ⇒ đúng 1 shadow, offset (1,1), blur 0, spread 0; `box-shadow: none` ⇒ 0 shadow; chuỗi 2 shadow ⇒ 2 layer theo đúng thứ tự. Nếu computed style chưa expose shadow ra ngoài `internal/style`, **dừng và báo cáo** thay vì tự thêm accessor — integrator sẽ quyết định hướng đó (xem mục 5).

### 2.3 Proof cho **từng** oracle mới (không phải một proof chung)

Với mỗi assert thêm vào, phải có một lần làm hỏng hành vi tương ứng và thấy **FAIL**:
- `ClickAt` luôn trả `false` (stub ở call site test, không sửa engine vĩnh viễn) ⇒ assert 1 fail.
- `Edit` không ghi ký tự ⇒ assert 2 fail.
- Resolver bỏ `text-shadow` đầu tiên ⇒ assert shadow fail.

Ghi nguyên văn output FAIL + PASS sau revert vào Status. Một assert không chứng minh được thì **bỏ assert đó**, không giữ làm cảnh.

## 3. Tiêu chí nghiệm thu

- **[AC-01]** `go test -count=1 ./test/engine/ ./test/style/` PASS; `go test -count=1 -run Fuzz -v ./test/engine/` PASS ở chế độ seed (không cần corpus).
- **[AC-02]** `go test -race -count=2 ./test/engine/` PASS.
- **[AC-03]** Chạy được fuzz thực thụ trong 30s cho mỗi target: `go test -fuzz FuzzClickAt -fuzztime=30s ./test/engine/` (và các target khác). Nếu sandbox không chạy được `-fuzz`, ghi rõ lệnh nào không chạy và vì sao — **không** báo PASS cho phần không chạy.
- **[AC-04]** Proof mục 2.3 cho **3** oracle (input-consistency, edit-effect, shadow-count), mỗi proof có output fail nguyên văn và `git status` sạch sau revert.
- **[AC-05]** `go build ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test ./test/qc/` PASS; `gofmt -l` sạch.
- **[AC-06]** Nếu thêm/di chuyển tag `// qc:BR-xxx`: `go test -count=1 -run TestTraceability ./test/qc/ -update` và commit golden. Các case ứng viên tự nhiên: BR-059 (input/undo), và các case fuzz/robustness hiện đang enforced mà chưa có oracle thật.
- **[AC-07]** Status liệt kê **các test khác còn lại** có cùng dạng "gọi hàm rồi bỏ kết quả" mà bạn thấy khi đọc `test/**` (grep `_, _ =` / `sess\.[A-Z]` không kèm assert). Chỉ liệt kê, không sửa.

## 4. Ràng buộc vận hành

- Phạm vi sửa: `test/engine/**`, `test/style/**`, `test/qc/traceability.golden.json` khi tag đổi, mục Status của file task này.
- **Cấm**: `internal/**` (kể cả thêm accessor — nếu cần thì ghi nhu cầu vào Status và dừng), `cmd/**` (agent khác sở hữu), `testdata/**` (agent khác sở hữu), `docs/qa-*`, `Makefile`, `.github/**`.
- Không sửa engine để test pass. Không dùng `time.Sleep` để đồng bộ.
- Chỉ commit trên branch `worktree-task-022-input-oracles`. Không `git push`, không merge `main`.

## 5. Câu hỏi cần integrator quyết (ghi vào Status nếu gặp)

1. `internal/style` đã expose danh sách shadow cho computed style chưa? Nếu chưa, chọn (a) accessor read-only duy nhất (tiền lệ `js.Runtime.PendingFetches()` ở TASK-017) hay (b) đo qua `PaintChecked` display list (số shadow layer vẽ ra)? Hướng (b) không cần API mới nhưng chứng minh cả đường paint, hướng (a) sắc hơn.
2. `SetHover` có đường đọc trạng thái hover nào không? Nếu không, assert cho hover chỉ có thể là "hover không làm hỏng layout" — chấp nhận mức đó hay bỏ hover khỏi phạm vi?
