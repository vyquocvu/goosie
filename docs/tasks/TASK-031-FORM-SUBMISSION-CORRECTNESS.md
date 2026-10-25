# TASK-031 — Form submission đúng: validation + submitter (BR-105, BR-107)

**Priority:** P1. **Method:** INT. **Owner chưa gán.** **Blocked cases:** BR-105
("Submit form thiếu required/value sai type → validation ngăn submit và giữ nội dung
nhập"), BR-107 ("Form có button submit phụ với name/value riêng → chỉ submitter kích
hoạt được gửi") — cả hai `NOT RUN`.

Hai gap này đi chung một task vì chúng cùng nằm trên một đường code
(`submitFormLocked` → `CollectFormData`) và cùng một loại lỗi: **engine gửi đi thứ mà
server không mong nhận**.

## 1. Bằng chứng đã đo lại trên `main`

| Sự thật | Chỗ trong code |
| --- | --- |
| Submit dispatch `submit` rồi **collect và gửi ngay**, không có dòng validation nào | `internal/engine/engine.go:1619 submitFormLocked` (`ev := dom.NewEvent("submit"...)` → `data := CollectFormData(form)`) |
| Không có chỗ nào đọc thuộc tính `required` khi submit (chỉ CSS `:required` selector) | `internal/css/selector.go:887,934`; không có `required` check nào khác |
| `method="dialog"` đã được xử **trước** khi tạo submission — validation phải chèn trước cả nhánh này | `internal/engine/engine.go:1631-1641` |
| Button luôn bị loại khỏi form data với chú thích "handled separately", nhưng **không có chỗ nào handle separately** | `internal/engine/forms.go:70-72` (`input` type submit/reset/button) và `:88` (`case "button": return false`) |
| Click submit button gọi `SubmitForm(form)` **không truyền node button** ⇒ mất submitter | `internal/engine/engine.go:1560-1568 runClickDefault` |
| Enter trong input cũng submit mà không có submitter | `internal/engine/focus.go:378` (`submitFormLocked(form)`) |
| API public hiện tại chỉ nhận form | `internal/engine/engine.go:1613 SubmitForm(form *dom.Node)`; call sites: `focus.go:378`, `engine.go:1535`, `engine.go:1564`, `test/engine/control_events_test.go:232`, `test/engine/dialog_test.go:194` |

## 2. Thiết kế

### 2.1 Submitter

`SubmitForm(form *dom.Node, submitter *dom.Node)` — đổi signature, không giữ overload
tương thích (call sites đều trong repo). `submitter == nil` nghĩa là implicit submission.

* `runClickDefault` truyền chính node button vừa click.
* Enter trong text input: submitter là **button submit đầu tiên theo thứ tự tree** của
  form (đúng Chromium), `nil` nếu form không có button submit nào.
* `CollectFormData(form, submitter)`: nếu submitter là `input[type=submit]` hoặc
  `button[type=submit]` **có** `name`, thêm `name=value` của nó **một lần**, ở vị trí
  của nó trong tree order. Submitter không `name` ⇒ không thêm gì. `button` không
  `type` ⇒ mặc định là submit theo HTML, nhưng engine hiện coi `type==""` là submit ở
  `runClickDefault:1561` — giữ đúng quán tính đó và test nó.

### 2.2 Validation (chặn submit, giữ nội dung)

Trước khi dispatch `submit`, chạy bước "interactive validation" trên các control thành
công của form:

1. Form có thuộc tính `novalidate` ⇒ bỏ qua toàn bộ bước này.
2. Với mỗi control không `disabled` và có `name`:
   * `required` + value rỗng ⇒ invalid. Checkbox `required` mà không `checked` ⇒ invalid.
     Radio `required` mà cả nhóm không có cái nào `checked` ⇒ invalid (đánh dấu control
     **đầu tiên** của nhóm). `select required` mà không option nào chọn ⇒ invalid.
   * `type=email` value khác rỗng mà không có `@` ⇒ invalid. `type=number` value khác
     rỗng mà không parse được số ⇒ invalid. **Không** làm URL/regex/pattern/datetime —
     ngoài phạm vi (§5).
3. Nếu có control invalid: dispatch `invalid` trên **từng** control invalid (theo thứ tự
   tree), focus control invalid **đầu tiên**, và **return không gửi request**. Nội dung đã
   nhập phải còn nguyên (không có operation reset nào ở đây).
4. Validation chạy **trước** dispatch `submit`, nên `submit` listener không bao giờ thấy
   một form invalid; và `method="dialog"` cũng bị chặn (đúng Chromium).

## 3. Test phải có

1. `required` text rỗng + click submit ⇒ **0 HTTP request**; control nhận `invalid`;
   focus nằm ở nó; nội dung các field khác giữ nguyên.
2. Điền đủ rồi submit ⇒ 1 request, payload đúng.
3. `required` checkbox chưa tick ⇒ chặn; tick rồi ⇒ qua.
4. Radio group `required` ⇒ chặn khi chưa chọn, qua khi chọn, và `invalid` chỉ fire trên
   member đầu tiên (đếm 1 lần).
5. `novalidate` ⇒ `required` bị bỏ qua, request gửi.
6. `type=email` value `"abc"` ⇒ chặn; `""` mà không `required` ⇒ **qua** (empty không
   phải là lỗi type).
7. Hai nút `name="action" value="save"` / `value="publish"` ⇒ payload chứa đúng cặp của
   nút vừa bấm, và **không** chứa cặp của nút kia.
8. Nút submit không `name` ⇒ payload không có mẩu nào từ nó.
9. Enter trong text input với form có button submit có `name` ⇒ payload mang cặp của nút
   đó (implicit submitter).
10. `<textarea>`/`<select>` `required` rỗng ⇒ chặn; `method="dialog"` form invalid ⇒
    `open` attribute **không** bị xoá và không có `close` event.

## 4. Nghiệm thu `[R-7]`

Bốn proof **tắt hành vi**, dán `FAIL → revert → PASS` nguyên văn:

* (i) cho `validateForm` trả `nil` list (tắt hẳn validation) ⇒ test 1, 3, 4, 6, 10 **FAIL**.
* (ii) validation vẫn chạy nhưng vẫn tiếp tục gửi request ⇒ test 1 **FAIL** (đây là mutation
  mà oracle "không request" kiểu cũ bắt được, chứng minh oracle không mù).
* (iii) `CollectFormData` bỏ hẳn submitter ⇒ test 7 **FAIL**; thêm submitter **hai lần**
  ⇒ test 7 **FAIL** (bắt lỗi trùng khoá).
* (iv) `novalidate` bị đọc sai thành "luôn bỏ qua validation" ⇒ test 1 **FAIL** nhưng test
  5 **PASS**: chứng bộ điều kiện thật sự là hai nhánh phân biệt, không phải một hằng số.

BR-105 và BR-107 phải có tag `// qc:` trong `test/qc/traceability.golden.json` và test
xanh với `-count=5`. **Không sửa `docs/qa-test-catalog.md`** — status là của
`scripts/rc-sweep.sh` (TASK-029).

## 5. Ngoài phạm vi

`pattern`, URL/date/time validation, `minlength`/`maxlength`, `step`, custom validity
(`setCustomValidity`), popup tooltip validation UI, và `formmethod`/`formenctype`/`formaction`
override trên button.

## 6. Ràng buộc vận hành

Không `git push`, không merge `main`, không tạo worktree con. Test chạy trên máy **không
có display**. `go vet -all ./...`, `go test ./internal/archtest/`, `go test -race ./...`
phải xanh. Không nới limit trong `internal/engine/limits.go` để sửa bug.

## 7. Status

Chưa bắt đầu.
