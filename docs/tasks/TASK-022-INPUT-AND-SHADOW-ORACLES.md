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


## 5.1 Quyết định của integrator (2026-10-25) — cả hai câu hỏi mục 5 đã có đáp án bằng code, không cần chọn phương án

**Câu 1 — shadow: không cần accessor mới, và cũng không cần đo qua display list.**
`internal/style/style.go:237` là `type ComputedStyle struct` (đã export), và chính nó khai báo:

```go
BoxShadow  []css.BoxShadow   // style.go:507 — nil nghĩa là `none`
TextShadow []css.TextShadow  // style.go:510
```

Thư mục test đã đọc hai field này trực tiếp: `test/style/shadow_test.go:14` (`TestBoxShadowResolved`) assert `len(s.BoxShadow)` rồi `OffsetX/OffsetY/Blur/Spread`, qua helper `styleFor(...)` mà `TestShadowPanicsNoPanic` đang gọi rồi **vứt kết quả** (`shadow_test.go:210`: `_ = styleFor(...)`). Vậy hướng (a)/(b) trong mục 5 là giả sai: API tồn tại, chỉ có assert là chưa viết. `TestShadowPanicsNoPanic` phải đổi thành bảng kỳ vọng per-declaration dùng đúng `styleFor` đang có.

Một phân biệt cần giữ: "shadow" trong file này là **CSS box/text-shadow**, không phải DOM shadow tree (`n.ShadowRoot` ở `internal/style/incremental.go:194`). Phạm vi task là CSS shadow; ai muốn phủ shadow root thì mở task khác, vì oracle của nó là cascade/invalidate chứ không phải resolver.

**Câu 2 — hover: KHÔNG bỏ khỏi phạm vi, vì hover đã quan sát được qua DOM event.**
`Session.hovered` (`internal/engine/engine.go:133`) là field không export và nên giữ nguyên như vậy. Nhưng `SetHover` dispatch **`mouseenter`** tại `internal/engine/focus.go:861` và unhover phần tử cũ qua `mouseleave` ở nhánh prev. Đó là chính thứ người dùng cảm nhận được (link đổi màu, con trỏ đổi shape là hệ quả), nên bất biến phải viết trên event, không trên field:

* `SetHover` trả `true` ⇒ **đúng một** `mouseenter` hoặc `mouseleave` đã fire kể từ lần gọi trước.
* `SetHover` trả `false` ⇒ **không** event nào fire (`focus.go:840`: `if hovered == s.hovered { return false }` — nhánh đó về sớm trước mọi dispatch), và phần tử đang hover không đổi.
* Gọi hai điểm trùng nhau rồi gọi `SetHover` ở điểm đó lần thứ hai ⇒ phải trả `false` (đây là case mà một stub "luôn trả true" chết ngay).

Ràng buộc: test đăng ký listener qua DOM API có thật của engine, không thêm hàm đọc vào `internal/**`. Nếu engine không cho gắn `mouseenter` listener từ script/`dom.AddEventListener` sẵn có, **dừng và báo** kèm bằng chứng, đừng tự mở API.

## 6. Status

### Thay đổi

- `TestShadowPanicsNoPanic` được đổi tên thành `TestShadowDeclarationsResolve` và gộp thành bảng kỳ vọng cho đủ 9 khai báo. Bảng kiểm số layer, thứ tự, offset, blur, spread, màu/alpha, `ColorIsCurrent` và `inset`; các trường không áp dụng cho `text-shadow` không bị bịa thêm.
- Fuzz oracle kiểm tra `ClickAt` false không đổi focus và khớp kết quả activation với listener click; `FocusControl` phản ánh đúng thay đổi focus; `EditRune` kiểm tra text sau thành công/thất bại; hover đăng ký listener `mouseenter`/`mouseleave` qua `dom.AddEventListener`; chuỗi tương tác gọi `PaintChecked(1)` và đòi display list không rỗng.
- Không thêm tag `qc:BR-xxx`; không cập nhật golden traceability.

### Bằng chứng mutation

`FAIL/PASS` dưới đây là số lần chạy mutant và chạy lại sau revert. Với sáu phép mutation chính, sau revert `git status --short` không in dòng nào; test mục tiêu chạy lại ra `ok` như ghi bên dưới. Các output FAIL là nguyên văn:

| Oracle | Assertion được kiểm | Mutation thật | Kết quả |
| --- | --- | --- | --- |
| ClickAt | Kết quả trả về khớp event click trên button đã biết; false giữ nguyên focus | Stub kết quả `ClickAt` thành false tại call site sau click button | 1/1 FAIL, 1/1 PASS |
| FocusControl | True đổi node focus; false giữ nguyên node; focus input xác định và gọi lặp lại | `setFocusedNodeLocked` cập nhật focus nhưng trả false | 1/1 FAIL, 1/1 PASS |
| Edit | Rune hợp lệ được ghi; kết quả false không được che thay đổi text | Bỏ `setControlValue` trong nhánh chèn rune; thêm lần thử stub return false sau ghi | 2/2 FAIL, 2/2 PASS |
| Hover | Chuyển đến cùng target không phát event thừa; điểm target đã nhận `mouseenter` trước lần lặp | Bỏ early return khi target hover không đổi | 1/1 FAIL, 1/1 PASS |
| PaintChecked | Sau chuỗi input trả list có command | `list.Reset()` ngay sau khi build paint list | 1/1 FAIL, 1/1 PASS |
| Shadow | Layer count và thứ tự text-shadow | Cắt layer đầu tiên khỏi `TextShadow` sau parse | 1/1 FAIL, 1/1 PASS |

Output mutation ClickAt (stub tại call site):

```text
--- FAIL: FuzzClickAt (0.00s)
    --- FAIL: FuzzClickAt/seed#0 (0.00s)
        fuzz_input_test.go:80: ClickAt returned false after 1 click events, want return to match activation
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.771s
FAIL
```

Sau revert, `git status --short` rỗng và `go test -count=1 -run 'FuzzClickAt/seed#0' ./test/engine/`:

```text
ok  	github.com/vyquocvu/goosie/test/engine	0.534s
```

Output mutation FocusControl (trả false sau khi đã cập nhật focus):

```text
--- FAIL: FuzzFocusControl (0.00s)
    --- FAIL: FuzzFocusControl/seed#0 (0.00s)
        fuzz_input_test.go:168: FocusControl did not report and establish focus on fixture input
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.762s
FAIL
```

Sau revert, `git status --short` rỗng và test mục tiêu PASS:

```text
ok  	github.com/vyquocvu/goosie/test/engine	0.550s
```

Output mutation Edit (bỏ thao tác ghi):

```text
--- FAIL: FuzzEdit (0.01s)
    --- FAIL: FuzzEdit/seed#0 (0.00s)
        fuzz_input_test.go:183: Edit returned true but FocusedText "hello" does not contain inserted rune 'a'
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.753s
FAIL
```

Output mutation Edit (stub trả false sau khi text đã đổi):

```text
--- FAIL: FuzzEdit (0.00s)
    --- FAIL: FuzzEdit/seed#0 (0.00s)
        fuzz_input_test.go:226: Edit returned false but FocusedText changed from "hello" to "helloa"
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.764s
FAIL
```

Sau revert mutation bỏ thao tác ghi, `git status --short` rỗng và test seed chạy lại:

```text
ok  	github.com/vyquocvu/goosie/test/engine	0.549s
```

Sau revert mutation stub false, output status/test là:

```text
?? test/engine/testdata/
ok  	github.com/vyquocvu/goosie/test/engine	0.545s
```

Corpus fuzz thất bại trước đó làm status không hoàn toàn sạch ở lần proof thứ hai; phần mutation nguồn đã revert. Xem ghi chú giới hạn dọn corpus ở cuối status này.

Output mutation hover (bỏ guard target không đổi):

```text
--- FAIL: FuzzSetHover (0.00s)
    --- FAIL: FuzzSetHover/seed#0 (0.00s)
        fuzz_input_test.go:128: SetHover at the unchanged fixture target returned true
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.756s
FAIL
```

Sau revert, `git status --short` rỗng và test mục tiêu PASS:

```text
ok  	github.com/vyquocvu/goosie/test/engine	0.547s
```

Output mutation PaintChecked (xóa display commands sau build):

```text
--- FAIL: FuzzInteractionSequence (0.00s)
    --- FAIL: FuzzInteractionSequence/seed#0 (0.00s)
        fuzz_input_test.go:265: PaintChecked after interaction sequence returned &{[] {0 0 0 0}} display commands
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.755s
FAIL
```

Sau revert, `git status --short` rỗng và test mục tiêu PASS:

```text
ok  	github.com/vyquocvu/goosie/test/engine	0.587s
```

Output mutation shadow (bỏ layer đầu):

```text
--- FAIL: TestShadowDeclarationsResolve (0.00s)
    --- FAIL: TestShadowDeclarationsResolve/text-shadow:_0_0_red,_1px_1px_blue,_2px_2px_green (0.00s)
        shadow_test.go:235: text-shadow layer count = 2, want 3
FAIL
FAIL	github.com/vyquocvu/goosie/test/style	0.638s
FAIL
```

Sau revert, `git status --short` rỗng và `go test -count=1 -run TestShadowDeclarationsResolve ./test/style/`:

```text
ok  	github.com/vyquocvu/goosie/test/style	0.415s
```

### Assertions điều chỉnh và sai lệch CSS

- Điều kiện `FocusControl == true ⇒ Focused() != nil` không đúng với hợp đồng tại `focus.go:76`: focus vào vùng trống có thể xóa focus và vẫn trả true. Test dùng điều kiện phù hợp: true phải thay đổi node focus (kể cả sang nil), false phải giữ nguyên.
- Điều kiện hover true ⇒ đúng một event không đúng khi chuyển giữa hai element: nhánh engine có thể phát `mouseleave` và `mouseenter`, tổng cộng hai event. Test cho phép một hoặc hai event khi true; lần gọi lặp lại trên target đã nhận `mouseenter` vẫn phải false và không phát event.
- `ClickAt` chỉ kích hoạt control, không đặt focus theo API ở `engine.go:1502`; vì vậy bỏ điều kiện true ⇒ `Focused() != nil`. Thay bằng quan hệ return/event click trên button thật và false ⇒ focus không đổi.
- Rune surrogate không phải Unicode scalar hợp lệ. Oracle Edit chỉ xét `utf8.ValidRune` và ký tự không control.
- Không có assert nào bị bỏ vì không chứng minh được bằng mutation. Các điều kiện nêu trên được thay vì mâu thuẫn với contract/API hoặc Unicode.
- Không tìm thấy sai lệch CSS-spec trong 9 khai báo. Ví dụ tối giản `box-shadow: 1px 1px` cho offset `(1,1)`, blur `0`, spread `0`; màu omitted được biểu diễn bằng `ColorIsCurrent`. Khai báo box-shadow hai layer và text-shadow ba layer giữ đúng count, thứ tự, màu, offset và blur. Alpha rgba được byte-quantize theo rounding (0.3 → 77; 0.5 → 128).
- Tên `TestShadowPanicsNoPanic` được đổi thành `TestShadowDeclarationsResolve`; tính chất no-panic vẫn được bao phủ trong lúc resolve từng khai báo.

### Nghiệm thu

- **AC-01:** `go test -count=1 ./test/engine/ ./test/style/` PASS; `go test -count=1 -run Fuzz -v ./test/engine/` PASS, gồm seed và corpus đã lưu.
- **AC-02:** `go test -race -count=2 ./test/engine/` PASS (`ok ... 47.279s`).
- **AC-03:** các lệnh `-fuzz ... -fuzztime=30s` PASS: `FuzzClickAt` 206,282 execs; `FuzzSetHover` 194,657; `FuzzFocusControl` 195,132; `FuzzSelectAtTo` 191,493; `FuzzEdit` 178,459; `FuzzInteractionSequence` 159,670. Lệnh đầu không nâng quyền bị loopback `httptest` chặn; các lần chạy 30 s hoàn tất khi cấp quyền loopback.
- **AC-05:** `go build ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test ./test/qc/` PASS; `gofmt -l test/engine/fuzz_input_test.go test/style/shadow_test.go` không in file.
- **AC-06:** không đổi tag QC nên không chạy `-update`; golden không đổi.
- **AC-07:** các test cùng dạng còn lại (chỉ liệt kê): `test/engine/control_events_test.go` (FocusControl/Edit, khoảng dòng 28–370); `disclosure_test.go:57`; `focus_test.go` (FocusControl/Edit, dòng 67–396); `ime_test.go` (FocusControl/Edit, dòng 29–195); `events_test.go` (SetHover/FocusControl, dòng 123–247); `unicode_test.go:50–62`; `popover_test.go:83–230`; `parity_interaction_test.go:69–182`; `interaction_e2e_test.go:56–623`. Nhiều test trong nhóm này kiểm tra tác dụng phụ ở assertion phía sau dù bỏ qua bool trả về.

### Giới hạn dọn dẹp

Lần fuzz đầu phát hiện hai oracle kỳ vọng quá hẹp và Go đã tạo hai corpus repro ở `test/engine/testdata/fuzz/FuzzSetHover/0e33ac8c36128d91` và `test/engine/testdata/fuzz/FuzzEdit/767b1cc07fe7a7f1`; các corpus này hiện PASS trong seed run. Tôi thử xóa đúng hai thư mục này để giữ phạm vi file, nhưng auto-review chặn lệnh dọn đệ quy vì đường dẫn thuộc `testdata/**` mà task cấm sửa. Không có corpus nào được đưa vào commit; worktree còn `?? test/engine/testdata/` và cần integrator cho phép dọn để trạng thái Git hoàn toàn sạch.

## 7. Integrator review (2026-10-25) — hai oracle vẫn mù với no-op mutation, đã sửa trong `59e7401`

§6 của agent là trung thực và có sản phẩm thật: 5 fuzz target đã có invariant, `TestShadowDeclarationsResolve` kiểm đủ layer/order/offset/blur/spread/màu, và bảng proof của agent có output nguyên văn. Nhưng integrator chạy **mutation tắt hẳn chức năng** trên một bản detached của `cacd444`: oracle `ClickAt` và `Edit` vẫn xanh, `SetHover` thì đã tự bắt được (ghi ở dòng C để đủ bảng, không phải lỗ hổng):

| Mutation | Sửa gì | Test như `cacd444` | Test sau `59e7401` |
| --- | --- | --- | --- |
| A | `case EditRune:` đổi điều kiện thành `if true { return false }` — không bao giờ chèn rune | `ok ... 0.752s` **(PASS)** | `--- FAIL: FuzzEdit/seed#0 ... returned false; text stayed "hello"` + fail cả corpus `767b1cc07fe7a7f1` |
| A2 | `insert(string(r) + string(r))` — chèn trùng | không chạy ở bản cũ (oracle chỉ cần "chữ có xuất hiện đâu đó") | `--- FAIL: FuzzEdit/seed#0 ... produced "helloaa" from "hello", want "helloa"` |
| B | đầu `Session.ClickAt`: `return false` — không hit-test, không dispatch | `ok ... 0.747s` **(PASS)** | `--- FAIL: FuzzClickAt/seed#0 ... returned false after 0 click events, want true and 1` |
| C | đầu `Session.SetHover`: `return false` — không hover, không event | `--- FAIL: FuzzSetHover/seed#0 ... returned without exactly one enter/leave event` — **đã bị bắt từ bản cũ** (oracle này đã có kỳ vọng dương) | vẫn FAIL, dòng tương ứng nay là `fuzz_input_test.go:109` |

Lỗi chung của A/B: invariant chỉ viết ở dạng `returned X <===> effect Y`. Một hàm bị vô hiệu hoá hoàn toàn thì hai vế **đều sai một cách nhất quán**, nên test vẫn xanh. Đây chính xác là lớp đã giết BR-021 (TASK-020 §6), quay lại ở file khác.

Cả ba oracle vẫn được viết lại cho chặt, trong đó hai oracle phải thêm **kỳ vọng dương**: click vào nút submit đã biết ⇒ `handled == true` và đúng một `click`; `EditRune` trên input đang focus ⇒ text phải bằng `before + string(r)` (không phải "chữ có nằm trong text"); hover ⇒ `handled` đúng khi và chỉ khi có event, và không quá một `mouseenter`/một `mouseleave`.

Hai phát hiện thật từ lần siết này, không phải chuyện test:

* **Quy tắc từ chối của engine rộng hơn dự kiến.** Oracle mới ban đầu liệt kê `{\n, \r, \t, 0}`; fuzz 45s tìm ra `\f` (`U+000C`) cũng bị từ chối — vì `internal/engine/focus.go:360` chặn **mọi** `r < 0x20`. Đã ghi lại đúng hợp đồng đó kèm lý do (không phím nào gửi C0 control vào form field; CR/LF/TAB phải nằm ngoài value một dòng). Rune không hợp lệ/surrogate đơn được engine chèn thành `U+FFFD` — hành vi đúng theo tinh thần WHATWG, và `string(r)` của Go cũng làm đúng phép gấp đó, nên oracle không cần đặc cách `utf8`.
* **Chuỗi hover không phải một event.** Probe trên fixture: từ trống vào `#box` ⇒ 1 `mouseenter`; từ `#box` sang `body` ⇒ 1 `leave` + 1 `enter`; từ `#ta` ra khỏi trang ⇒ 1 `leave`. Oracle `events == 1` của bản đầu là **sai về phía test**, còn `1 hoặc 2` là rộng tuỳ tiện. Dạng đã ghi: `enters <= 1 && leaves <= 1 && handled == (events > 0)`. **Lưu ý cho bug sau:** goosie chỉ bắn `mouseenter`/`mouseleave` trên node bị chạm, không bắn cho tổ tiên mới đi vào — Chromium thì bắn cho cả chuỗi. Chưa đối chứng bằng `testdata/interaction-parity`, nên đây là gap ghi nhận, chưa phải defect đã đóng; oracle hiện **pin hành vi cũ**, và nếu sửa theo spec thì test này sẽ đỏ có chủ đích.

Quy trình fuzz của agent giữ nguyên giá trị: chính `-fuzz` là thứ tìm ra hai oracle quá hẹp. Ba corpus repro ở `test/engine/testdata/fuzz/` được **commit làm regression seed** (agent xin phép dọn vì nằm ngoài phạm vi file — quyết định của integrator: giữ), và input `\f` giờ assert đúng điều engine làm thay vì bị nhảy qua.

**[R-7]** áp cho mọi task oracle còn lại (TASK-026/027/028/029): mutation proof nộp kèm phải có ít nhất một mutation **tắt hẳn chức năng** (`return false` / xoá thao tác ghi), không chỉ mutation trả sai giá trị; và mỗi oracle phải chứa ít nhất một **kỳ vọng dương** cho input tốt đã biết.

