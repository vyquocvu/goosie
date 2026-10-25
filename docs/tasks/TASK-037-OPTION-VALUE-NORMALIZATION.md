# TASK-037 — giá trị `<option>` phải normalize đúng như Chromium (BR-101, BR-104, BR-106, BR-139)

**Priority:** P1. **Method:** INT. **Owner:** chưa gán.
**Blocked cases:** BR-101 (chọn option ⇒ value đúng), BR-104/BR-106 (submit gửi đúng
name/value của control, và encoding của chúng) và BR-139 (AX tree phản ánh value của DOM).
**Defect:** `GOOSIE-FORMVALUE-001`.

## 1. Bằng chứng đã đo, không suy diễn

Probe chạy trên `main` sau unify `optionValue` (file probe đã xoá ngay sau khi đo, không commit):

```
GOOSIE  SUBMITTED name="c" value="\n  Charcoal\n"
GOOSIE  ENCODED "c=%0A++Charcoal%0A"
```

markup tương ứng — chính là kiểu HTML người ta viết tay mọi ngày, option một dòng/thụt lề:

```html
<select id=s name=c>
<option selected>
  Charcoal
</option>
<option value=b>B</option>
</select>
```

Cùng markup, đo Chromium qua Playwright trên cùng máy:

```
option.textContent = '  Hi\n  there\n'  ->  option.value === "Hi there"
                                         ->  option.label === "Hi there"
```

| Sự thật | Chỗ trong code |
| --- | --- |
| Giá trị option = `TextContent()` nguyên bản, không strip không collapse | `internal/engine/focus.go` `optionValue` (một định nghĩa duy nhất, dùng bởi `forms.go:controlValue`, `Session.controlValue`, `accessibility.go:162-193`) |
| `Label` của option trong AX **đã** `strings.TrimSpace` trong khi `Value` thì không ⇒ cùng một node, hai câu trả lời lệch nhau | `internal/engine/accessibility.go:171-177` |
| `<select>` có **hai** option cùng `selected`: Chromium chọn cái **cuối** (`selectedIndex 1, value "b", FormData "b"`); `SelectedOption` trả về ngay khi thấy `selected` đầu tiên ⇒ Goosie gửi `"a"` | `internal/engine/focus.go:634` `SelectedOption` |
| Hành vi first-wins này **không** có test nào khoá: `grep selected test/engine/` không có case hai option cùng `selected` | `test/engine/` |

Vì sao đáng làm: form `<GET>` nhận `?c=%0A++Charcoal%0A` thì server không khớp được giá trị nào;
đây là lỗi "browser dùng được thật" chứ không phải lỗi conformance trừu tượng.

## 2. Thiết kế tối thiểu

0. **Đã xong, không làm lại:** một helper `optionValue` duy nhất cho cả submit, bàn phím và AX,
   cùng oracle chéo `TestAccessibilitySelectValueMatchesSubmittedFormValue` (`41446b2`). Phần còn
   lại của task là *normalize* giá trị và *tie-break* `selected`. Oracle chéo phải xanh trong suốt
   quá trình sửa — nó là thứ phát hiện ngay nếu hai đường đi lệch nhau.
1. **Đo trước khi sửa.** Viết một script Playwright nhỏ (mẫu: `scripts/verify-phase*.js`, hoặc
   `python3 -c` như trên) trả về, cho từng case: option text có whitespace đầu/cuối, có chuỗi
   whitespace bên trong, và `value="  x  "` (attribute có space). **Chấp nhận kết quả đo được,
   không chấp nhận suy luận từ spec.** Dán output vào §Status.
2. Sửa `optionValue` theo đúng measurements ở (1): nhiều khả năng là "strip ASCII whitespace hai
   đầu + collapse chuỗi whitespace bên trong thành một U+0020" cho *text content*, còn *value
   attribute* thì xử lý theo measurement chứ không theo trí nhớ.
3. `SelectedOption`: đổi tie-break thành **option `selected` cuối cùng** thắng, vẫn fallback
   option đầu tiên. Phải kiểm tra bàn phím (TASK-030, `MoveSelect` ở
   `internal/engine/select.go`) không đổi hành vi trên markup một option selected — đó là ca thật.
4. `accessibility.go`: label và value của cùng một option phải dùng **chung** một helper normalize,
   để không còn tình trạng `Label` đã sạch còn `Value` thì chưa.
5. Không đụng paint, không đụng layout, không đụng parser. `internal/dom` giữ nguyên text node —
   normalize là chuyện của *giá trị*, không phải của nội dung hiển thị.

## 3. Test phải có (tất cả ở `test/engine/`, không cần display)

1. Option text `\n  Charcoal\n` selected ⇒ `engine.CollectFormData` trả `c` = `Charcoal`, và
   `EncodeFormURLEncoded` **không** chứa `%0A`.
2. Option text `"  Hi   there "` ⇒ giá trị `"Hi there"` (collapse, không chỉ strip).
3. `value` attribute có whitespace bao quanh ⇒ đúng bằng kết quả đo ở §2.1, và test phải ghi rõ
   nó đo từ Chromium chỗ nào.
4. Hai option cùng `selected` ⇒ giá trị là option **cuối** (theo Chromium), có comment dẫn chứng
   đo được ở §1.
5. Oracle chéo đã tồn tại: `TestAccessibilitySelectValueMatchesSubmittedFormValue` — giá trị AX và
   giá trị submit phải một. Test này là lý do sửa ở (2) và (4) phải làm **cùng lúc**; giữ nó xanh.
6. Keyboard: `FocusNode(sel)` + `MoveSelect(EditDown)` trên markup thụt lề ⇒ value đổi đúng option,
   AX theo đúng DOM (nối BR-101 với BR-139 như TASK-032 đã làm).
7. Tag: `// qc:BR-104` cho test 1, 2, 4 (submit phải mang đúng value), `// qc:BR-106` cho test 1
   vì nó kiểm luôn `EncodeFormURLEncoded`, `// qc:BR-101` cho test 6 (bàn phím đổi selection),
   `// qc:BR-139` cho test 5 đã có sẵn. Chạy `go test -count=1 ./test/qc/ -update` và kiểm tra
   golden chỉ đổi các row tương ứng, rồi nâng sàn trong `docs/qa-ratchet.json` nếu số
   P0/P1 được tag tăng lên.

## 4. Nghiệm thu `[R-7]`

Mỗi proof là **tắt hành vi**, dán `FAIL → revert → PASS`:

* (i) `optionValue` trả `n.TextContent()` thô (bỏ normalize) ⇒ test 1 và 2 **FAIL**.
* (ii) chỉ strip hai đầu, không collapse ⇒ test 2 **FAIL** (mutation dễ lọt nhất vì ca "Charcoal"
  vẫn xanh).
* (iii) `SelectedOption` quay về first-wins ⇒ test 4 **FAIL**.
* (iv) normalize cả nhánh `value` attribute (nếu §2.1 đo được Chromium **không** làm thế) ⇒ test 3
  **FAIL**. Nếu measurement cho thấy Chromium cũng strip attribute, thì chứng minh theo chiều
  ngược lại: bỏ strip attribute ⇒ test 3 **FAIL**.
* (v) sửa `forms.go` nhưng để `accessibility.go` tự tính giá trị riêng ⇒ test 5 **FAIL**. Đây là
  mutation quan trọng nhất của task này.

Proof nào không giết được test nào thì khai trong §Status.

## 5. Ràng buộc vận hành

Không `git push`, không merge `main`, không worktree con. `go test ./internal/archtest/` và
`go vet -all ./...` xanh. Test không cần display. `make parity` phải **không đổi** kết quả
(80/80, average 96.07%, known gaps 5 fixtures/10 bands) — task này không được động vào ảnh; nếu
fixture `forms/*` đổi band thì phải giải thích được bằng số dòng, không được sửa baseline.
Không sửa `docs/qa-test-catalog.md`; đề xuất dòng registry `GOOSIE-FORMVALUE-001` trong báo cáo.

## 6. Ngoài phạm vi

`<optgroup>` — engine đã ghi nhận chưa hỗ trợ ngay trong code
(`internal/engine/select.go:46-48`: option lồng trong optgroup không tới được bằng bàn phím), nên
đó là task riêng; `<select multiple>`, `<datalist>`, `size>1`, và việc textarea/input có cần
normalize tương tự hay không.
