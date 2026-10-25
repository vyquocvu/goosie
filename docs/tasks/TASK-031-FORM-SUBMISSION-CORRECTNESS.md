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

Hoàn tất phần implementation và verification trên branch `worktree-task-031-form-submission`.

### Thay đổi

- Đổi API thành `SubmitForm(form, submitter)` và `CollectFormData(form, submitter)`, không giữ overload cũ. Click truyền đúng button; Enter truyền submit button đầu tiên theo tree order. Submitter được thu đúng một lần, tại vị trí tree order của nó; button không có `type` được xem là submit.
- Thêm validation subset `required`, `email`, `number`; `novalidate` bỏ qua validation. Invalid event fire theo thứ tự tree trên từng control invalid, focus chuyển tới control invalid đầu tiên, và submission không được xếp hàng. Validation diễn ra trước submit event và `method="dialog"`.
- Bổ sung giá trị mặc định `on` cho checkbox/radio và đọc `button[value]` khi serialize form.
- Có 9 test function mới (15 case/subtest), tagged `// qc:BR-105` và `// qc:BR-107`; cập nhật traceability bằng `go test ./test/qc -update`.
- Các test engine xác nhận không có `FormSubmission` đang chờ khi invalid. Session engine xếp hàng `FormSubmission` cho host; nó không tự phát HTTP request trong `SubmitForm`.

### Hiệu chỉnh bằng chứng §1

Đã xác nhận các dòng evidence còn lại trong bảng trên nhánh này. Hai trích dẫn sau không khớp source hiện tại, ghi lại để không âm thầm dựa vào chúng:

- `internal/engine/focus.go:378` không phải call site Enter; đó là dòng cuối của helper `insert`. Call site `s.submitFormLocked(form)` nằm ở `internal/engine/focus.go:399` trước sửa. Nay truyền `FirstSubmitButton(form)` tại dòng 399.
- `internal/css/selector.go:887,934` không phải `:required` selector. Dòng 887–909 thuộc kiểm tra option `:checked`; 934 thuộc danh sách phần tử `:disabled`. `rg -n required internal/css/selector.go` chỉ tìm thấy comment không liên quan ở dòng 855. Kết luận không có required validation ở submit path vẫn đúng; không tìm thấy checker `required` trong selector file.

### Verification nguyên văn

Mỗi test function mới được chạy riêng với `-count=1`. Tất cả đều PASS:

```text
$ go test -count=1 ./test/engine/ -v -run '^TestFormRequiredValidation$'
=== RUN   TestFormRequiredValidation
--- PASS: TestFormRequiredValidation (0.00s)
PASS
ok  	github.com/vyquocvu/goosie/test/engine	0.757s

$ go test -count=1 ./test/engine/ -v -run '^TestFormCheckboxRequiredValidation$'
=== RUN   TestFormCheckboxRequiredValidation
--- PASS: TestFormCheckboxRequiredValidation (0.00s)
PASS
ok  	github.com/vyquocvu/goosie/test/engine	0.536s

$ go test -count=1 ./test/engine/ -v -run '^TestFormRadioRequiredValidation$'
=== RUN   TestFormRadioRequiredValidation
--- PASS: TestFormRadioRequiredValidation (0.00s)
PASS
ok  	github.com/vyquocvu/goosie/test/engine	0.558s

$ go test -count=1 ./test/engine/ -v -run '^TestFormValidationTypes$'
=== RUN   TestFormValidationTypes
=== RUN   TestFormValidationTypes/email
=== RUN   TestFormValidationTypes/number
--- PASS: TestFormValidationTypes (0.00s)
    --- PASS: TestFormValidationTypes/email (0.00s)
    --- PASS: TestFormValidationTypes/number (0.00s)
PASS
ok  	github.com/vyquocvu/goosie/test/engine	0.552s

$ go test -count=1 ./test/engine/ -v -run '^TestFormNovalidateBypass$'
=== RUN   TestFormNovalidateBypass
--- PASS: TestFormNovalidateBypass (0.00s)
PASS
ok  	github.com/vyquocvu/goosie/test/engine	0.542s

$ go test -count=1 ./test/engine/ -v -run '^TestFormSubmitterClickAndImplicit$'
=== RUN   TestFormSubmitterClickAndImplicit
=== RUN   TestFormSubmitterClickAndImplicit/clicked_submitter
=== RUN   TestFormSubmitterClickAndImplicit/implicit_submitter
--- PASS: TestFormSubmitterClickAndImplicit (0.00s)
    --- PASS: TestFormSubmitterClickAndImplicit/clicked_submitter (0.00s)
    --- PASS: TestFormSubmitterClickAndImplicit/implicit_submitter (0.00s)
PASS
ok  	github.com/vyquocvu/goosie/test/engine	0.553s

$ go test -count=1 ./test/engine/ -v -run '^TestFormSubmitterWithoutNameOmitted$'
=== RUN   TestFormSubmitterWithoutNameOmitted
--- PASS: TestFormSubmitterWithoutNameOmitted (0.00s)
PASS
ok  	github.com/vyquocvu/goosie/test/engine	0.541s

$ go test -count=1 ./test/engine/ -v -run '^TestFormButtonDefaultTypeSubmits$'
=== RUN   TestFormButtonDefaultTypeSubmits
--- PASS: TestFormButtonDefaultTypeSubmits (0.00s)
PASS
ok  	github.com/vyquocvu/goosie/test/engine	0.549s

$ go test -count=1 ./test/engine/ -v -run '^TestFormTextareaSelectAndDialogValidation$'
=== RUN   TestFormTextareaSelectAndDialogValidation
=== RUN   TestFormTextareaSelectAndDialogValidation/textarea_and_select
=== RUN   TestFormTextareaSelectAndDialogValidation/method_dialog
--- PASS: TestFormTextareaSelectAndDialogValidation (0.00s)
    --- PASS: TestFormTextareaSelectAndDialogValidation/textarea_and_select (0.00s)
    --- PASS: TestFormTextareaSelectAndDialogValidation/method_dialog (0.00s)
PASS
ok  	github.com/vyquocvu/goosie/test/engine	0.550s

$ go test -count=5 ./test/engine/ -run 'TestForm(RequiredValidation|CheckboxRequiredValidation|RadioRequiredValidation|ValidationTypes|NovalidateBypass|SubmitterClickAndImplicit|SubmitterWithoutNameOmitted|ButtonDefaultTypeSubmits|TextareaSelectAndDialogValidation)$'
ok  	github.com/vyquocvu/goosie/test/engine	0.545s

$ go test -count=1 -race ./test/engine/ ./cmd/goosie/ ./internal/engine/
ok  	github.com/vyquocvu/goosie/test/engine	24.569s
ok  	github.com/vyquocvu/goosie/cmd/goosie	27.340s
ok  	github.com/vyquocvu/goosie/internal/engine	2.275s

$ go test -count=1 ./...
?   	github.com/vyquocvu/goosie/cmd/debug-layout	[no test files]
?   	github.com/vyquocvu/goosie/cmd/dump-fonts	[no test files]
?   	github.com/vyquocvu/goosie/cmd/dump-layout	[no test files]
?   	github.com/vyquocvu/goosie/cmd/dump-paint	[no test files]
?   	github.com/vyquocvu/goosie/cmd/dump-url	[no test files]
ok  	github.com/vyquocvu/goosie/cmd/goosie	12.501s
?   	github.com/vyquocvu/goosie/cmd/goosie-headless	[no test files]
?   	github.com/vyquocvu/goosie/cmd/test_colors	[no test files]
?   	github.com/vyquocvu/goosie/cmd/wpt-runner	[no test files]
ok  	github.com/vyquocvu/goosie/internal/archtest	3.896s
?   	github.com/vyquocvu/goosie/internal/ax	[no test files]
?   	github.com/vyquocvu/goosie/internal/bookmarks	[no test files]
ok  	github.com/vyquocvu/goosie/internal/css	1.766s
ok  	github.com/vyquocvu/goosie/internal/dom	1.350s
ok  	github.com/vyquocvu/goosie/internal/download	2.783s
ok  	github.com/vyquocvu/goosie/internal/engine	1.000s
?   	github.com/vyquocvu/goosie/internal/flags	[no test files]
?   	github.com/vyquocvu/goosie/internal/frame	[no test files]
?   	github.com/vyquocvu/goosie/internal/history	[no test files]
ok  	github.com/vyquocvu/goosie/internal/image	3.662s
?   	github.com/vyquocvu/goosie/internal/js	[no test files]
?   	github.com/vyquocvu/goosie/internal/layout	[no test files]
?   	github.com/vyquocvu/goosie/internal/manifest	[no test files]
?   	github.com/vyquocvu/goosie/internal/media	[no test files]
?   	github.com/vyquocvu/goosie/internal/net	[no test files]
?   	github.com/vyquocvu/goosie/internal/paint	[no test files]
?   	github.com/vyquocvu/goosie/internal/pdf	[no test files]
?   	github.com/vyquocvu/goosie/internal/perms	[no test files]
?   	github.com/vyquocvu/goosie/internal/platform	[no test files]
ok  	github.com/vyquocvu/goosie/internal/platform/darwin	3.767s
?   	github.com/vyquocvu/goosie/internal/platform/headless	[no test files]
ok  	github.com/vyquocvu/goosie/internal/preferences	1.776s
?   	github.com/vyquocvu/goosie/internal/raster	[no test files]
?   	github.com/vyquocvu/goosie/internal/session	[no test files]
ok  	github.com/vyquocvu/goosie/internal/style	2.617s
?   	github.com/vyquocvu/goosie/internal/surface	[no test files]
ok  	github.com/vyquocvu/goosie/internal/tabs	4.046s
ok  	github.com/vyquocvu/goosie/internal/toolbar	2.259s
?   	github.com/vyquocvu/goosie/internal/userscript	[no test files]
?   	github.com/vyquocvu/goosie/internal/viewsource	[no test files]
ok  	github.com/vyquocvu/goosie/test/bookmarks	3.439s
ok  	github.com/vyquocvu/goosie/test/css	3.431s
ok  	github.com/vyquocvu/goosie/test/dom	3.237s
ok  	github.com/vyquocvu/goosie/test/domtest	3.557s
ok  	github.com/vyquocvu/goosie/test/download	3.672s
ok  	github.com/vyquocvu/goosie/test/engine	9.121s
ok  	github.com/vyquocvu/goosie/test/entrypoints	26.628s
ok  	github.com/vyquocvu/goosie/test/flags	3.513s
ok  	github.com/vyquocvu/goosie/test/frame	3.621s
ok  	github.com/vyquocvu/goosie/test/gate	36.939s
ok  	github.com/vyquocvu/goosie/test/gatecheck	34.753s
ok  	github.com/vyquocvu/goosie/test/history	3.735s
ok  	github.com/vyquocvu/goosie/test/js	6.773s
ok  	github.com/vyquocvu/goosie/test/layout	4.089s
ok  	github.com/vyquocvu/goosie/test/manifest	3.212s
ok  	github.com/vyquocvu/goosie/test/net	5.654s
ok  	github.com/vyquocvu/goosie/test/paint	3.069s
ok  	github.com/vyquocvu/goosie/test/pdf	2.244s
ok  	github.com/vyquocvu/goosie/test/perms	2.345s
ok  	github.com/vyquocvu/goosie/test/platform	2.400s
ok  	github.com/vyquocvu/goosie/test/qc	1.967s
ok  	github.com/vyquocvu/goosie/test/raster	2.514s
ok  	github.com/vyquocvu/goosie/test/session	1.927s
ok  	github.com/vyquocvu/goosie/test/style	2.461s
ok  	github.com/vyquocvu/goosie/test/surface	3.481s
ok  	github.com/vyquocvu/goosie/test/tabs	3.341s
ok  	github.com/vyquocvu/goosie/test/toolbar	3.681s
?   	github.com/vyquocvu/goosie/test/userscript	[no test files]
ok  	github.com/vyquocvu/goosie/test/viewsource	3.571s
ok  	github.com/vyquocvu/goosie/test/wpt	10.423s

$ go vet -all ./...

$ go test -count=1 ./internal/archtest/
ok  	github.com/vyquocvu/goosie/internal/archtest	4.062s

$ go test -count=1 ./test/qc/
ok  	github.com/vyquocvu/goosie/test/qc	0.502s

$ gofmt -l internal cmd test

```

`TestJourneySelectOptionChange` và các test BR-101 trong `test/engine/select_keyboard_test.go` được chạy trong full `go test ./...` và đều xanh. `TestNativeEndToEndPlaysARealVideo` không fail; không phải chạy lại.

### Mutation proof [R-7]

**(i) Tắt hẳn validation — FAIL → revert → PASS**

```text
$ go test -count=1 ./test/engine/ -run '^(TestFormRequiredValidation|TestFormCheckboxRequiredValidation|TestFormRadioRequiredValidation|TestFormValidationTypes|TestFormTextareaSelectAndDialogValidation)$'
--- FAIL: TestFormRequiredValidation (0.01s)
    form_submission_test.go:43: invalid form queued submission: &engine.FormSubmission{Action:"/send", Method:"GET", Data:[]engine.FormData{engine.FormData{Name:"q", Value:""}, engine.FormData{Name:"other", Value:"intact"}}, Encoded:"q=&other=intact"}
--- FAIL: TestFormCheckboxRequiredValidation (0.00s)
    form_submission_test.go:71: invalid form queued submission: &engine.FormSubmission{Action:"", Method:"GET", Data:[]engine.FormData(nil), Encoded:""}
--- FAIL: TestFormRadioRequiredValidation (0.00s)
    form_submission_test.go:90: invalid form queued submission: &engine.FormSubmission{Action:"", Method:"GET", Data:[]engine.FormData(nil), Encoded:""}
--- FAIL: TestFormValidationTypes (0.00s)
    --- FAIL: TestFormValidationTypes/email (0.00s)
        form_submission_test.go:107: invalid form queued submission: &engine.FormSubmission{Action:"", Method:"GET", Data:[]engine.FormData{engine.FormData{Name:"email", Value:"abc"}}, Encoded:"email=abc"}
    --- FAIL: TestFormValidationTypes/number (0.00s)
        form_submission_test.go:121: invalid form queued submission: &engine.FormSubmission{Action:"", Method:"GET", Data:[]engine.FormData{engine.FormData{Name:"n", Value:"not-a-number"}}, Encoded:"n=not-a-number"}
--- FAIL: TestFormTextareaSelectAndDialogValidation (0.00s)
    --- FAIL: TestFormTextareaSelectAndDialogValidation/textarea_and_select (0.00s)
        form_submission_test.go:205: invalid form queued submission: &engine.FormSubmission{Action:"", Method:"GET", Data:[]engine.FormData{engine.FormData{Name:"ta", Value:""}, engine.FormData{Name:"sel", Value:""}}, Encoded:"ta=&sel="}
    --- FAIL: TestFormTextareaSelectAndDialogValidation/method_dialog (0.00s)
        form_submission_test.go:218: dialog open=false close events=1; want open and no close
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	1.154s
FAIL

revert:
$ go test -count=1 ./test/engine/ -run '^(TestFormRequiredValidation|TestFormCheckboxRequiredValidation|TestFormRadioRequiredValidation|TestFormValidationTypes|TestFormTextareaSelectAndDialogValidation)$'
ok  	github.com/vyquocvu/goosie/test/engine	0.546s
```

**(ii) Validate rồi vẫn tiếp tục submit — FAIL → revert → PASS**

```text
$ go test -count=1 ./test/engine/ -run '^TestFormRequiredValidation$'
--- FAIL: TestFormRequiredValidation (0.00s)
    form_submission_test.go:43: invalid form queued submission: &engine.FormSubmission{Action:"/send", Method:"GET", Data:[]engine.FormData{engine.FormData{Name:"q", Value:""}, engine.FormData{Name:"other", Value:"intact"}}, Encoded:"q=&other=intact"}
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.742s
FAIL

revert:
$ go test -count=1 ./test/engine/ -run '^TestFormRequiredValidation$'
ok  	github.com/vyquocvu/goosie/test/engine	0.536s
```

**(iii-a) Bỏ submitter — FAIL → revert → PASS**

```text
$ go test -count=1 ./test/engine/ -run '^TestFormSubmitterClickAndImplicit$'
--- FAIL: TestFormSubmitterClickAndImplicit (0.00s)
    --- FAIL: TestFormSubmitterClickAndImplicit/clicked_submitter (0.00s)
        form_submission_test.go:158: click save payload="q=x&tail=end", want "q=x&action=save&tail=end"
    --- FAIL: TestFormSubmitterClickAndImplicit/implicit_submitter (0.00s)
        form_submission_test.go:169: implicit payload="q=x", want q=x&action=first
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.785s
FAIL

revert:
$ go test -count=1 ./test/engine/ -run '^TestFormSubmitterClickAndImplicit$'
ok  	github.com/vyquocvu/goosie/test/engine	0.557s
```

**(iii-b) Thêm submitter hai lần — FAIL → revert → PASS**

```text
$ go test -count=1 ./test/engine/ -run '^TestFormSubmitterClickAndImplicit$'
--- FAIL: TestFormSubmitterClickAndImplicit (0.00s)
    --- FAIL: TestFormSubmitterClickAndImplicit/clicked_submitter (0.00s)
        form_submission_test.go:158: click save payload="q=x&action=save&action=save&tail=end", want "q=x&action=save&tail=end"
    --- FAIL: TestFormSubmitterClickAndImplicit/implicit_submitter (0.00s)
        form_submission_test.go:169: implicit payload="q=x&action=first&action=first", want q=x&action=first
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.749s
FAIL

revert:
$ go test -count=1 ./test/engine/ -run '^TestFormSubmitterClickAndImplicit$'
ok  	github.com/vyquocvu/goosie/test/engine	0.554s
```

**(iv) Novalidate luôn bỏ qua validation — required FAIL, novalidate PASS → revert → cả hai PASS**

```text
$ go test -count=1 ./test/engine/ -v -run '^(TestFormRequiredValidation|TestFormNovalidateBypass)$'
=== RUN   TestFormRequiredValidation
    form_submission_test.go:43: invalid form queued submission: &engine.FormSubmission{Action:"/send", Method:"GET", Data:[]engine.FormData{engine.FormData{Name:"q", Value:""}, engine.FormData{Name:"other", Value:"intact"}}, Encoded:"q=&other=intact"}
--- FAIL: TestFormRequiredValidation (0.01s)
=== RUN   TestFormNovalidateBypass
--- PASS: TestFormNovalidateBypass (0.00s)
FAIL
FAIL	github.com/vyquocvu/goosie/test/engine	0.750s
FAIL

revert:
$ go test -count=1 ./test/engine/ -v -run '^(TestFormRequiredValidation|TestFormNovalidateBypass)$'
=== RUN   TestFormRequiredValidation
--- PASS: TestFormRequiredValidation (0.00s)
=== RUN   TestFormNovalidateBypass
--- PASS: TestFormNovalidateBypass (0.00s)
PASS
ok  	github.com/vyquocvu/goosie/test/engine	0.554s
```

### Chưa chắc chắn

- Validation có chủ đích chỉ xử lý tập con task yêu cầu; `email` được kiểm tra bằng điều kiện có `@`, không phải kiểm tra đầy đủ cú pháp địa chỉ email.
- Engine ghi submission vào `FormSubmission`; kiểm tra không có mục chờ xác nhận engine sẽ không giao submission cho host. HTTP dispatch thực tế thuộc lớp host.

### Bổ sung journey BR-105

`cmd/goosie/journey_p1_form_validation_test.go:TestJourneyFormValidationBlocksRequestUntilValid` chạy qua browser headless và thao tác pointer/keyboard, với trang được tải từ `httptest.Server`. Sau click submit khi `q` rỗng, server chỉ thấy **1 request** (GET `/form`), không thấy request search; URL vẫn là `/form` và control `q` được focus. Sau khi gõ `ready` rồi click lại, server thấy **1 request mới** (tổng: page 1, search 1), `GET /search`, payload `q=ready&other=intact&intent=send`; submitter `intent=send` có mặt.

**Mutation proof [R-7] cho browser journey — mỗi lần FAIL → revert → PASS:**

1. Bỏ toàn bộ khối `ValidateForm` trong `submitFormLocked`. Journey FAIL: sau click invalid, URL chuyển sang `/search?q=&other=intact&intent=send` thay vì focus `q`; request invalid đã tới host và điều hướng. Hoàn tác mutation, journey PASS.
2. Trong `focusClick`, bỏ điều kiện `ok` của `PopSubmittedForm` và dispatch submission GET tới URL hiện tại kể cả khi engine trả nil. Journey FAIL tại bộ đếm: `requests after invalid submit = page:2 search:0, want page:1 search:0`. Điều này xác nhận phép đo đi qua host dispatch thật. Hoàn tác mutation, journey PASS.

**Verification** (đã chạy; `GOCACHE` trỏ vào thư mục tạm do cache mặc định ngoài vùng ghi được):

```text
$ go test -count=1 -v ./cmd/goosie/ -run 'TestJourneyFormValidationBlocksRequestUntilValid'
=== RUN   TestJourneyFormValidationBlocksRequestUntilValid
--- PASS: TestJourneyFormValidationBlocksRequestUntilValid (0.06s)
PASS
ok  github.com/vyquocvu/goosie/cmd/goosie  0.802s

$ go test -count=2 -race ./cmd/goosie/
ok  github.com/vyquocvu/goosie/cmd/goosie  54.740s

$ go test -count=1 ./test/qc/ ./internal/archtest/ ./test/engine/
ok  github.com/vyquocvu/goosie/test/qc  0.804s
ok  github.com/vyquocvu/goosie/internal/archtest  4.055s
ok  github.com/vyquocvu/goosie/test/engine  5.985s

$ go vet -all ./...
# PASS (exit code 0, no diagnostics)

$ go test -count=1 ./...
# PASS (all packages; cmd/goosie 10.742s, test/gate 40.135s, test/gatecheck 33.727s)
```

QC golden chỉ thêm `cmd/goosie/journey_p1_form_validation_test.go:TestJourneyFormValidationBlocksRequestUntilValid` vào BR-105.

Đã revert tất cả mutation; không còn mutation trong source. Sau khi ghi status này, commit được tạo và `git status` được kiểm tra sạch.
