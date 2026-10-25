# TASK-028: Form control UA appearance — 5 control type không vẽ gì cả

- **Task ID**: `TASK-028`
- **Type**: Engine bug / paint + UA stylesheet
- **Priority**: P1
- **Assigned to**: chưa chia
- **Review**: `qoder cli` (integrator)
- **Tách ra từ**: `docs/tasks/TASK-024-RENDER-REGION-GAPS.md`

## 1. Bằng chứng

Probe `/tmp/probe/c.html` — 8 control trong một trang, render ở 300x600 (`/tmp/probe/d.png`):

| Markup | Goosie vẽ | Chromium vẽ |
| --- | --- | --- |
| `<input type="text" value="hi">` | chữ `hi`, **không có border/box** | ô có viền |
| `<input type="color" value="#ff0000">` | **không có gì** | swatch màu |
| `<input type="range">` | **không có gì** | rãnh + knob |
| `<input type="date">` | **không có gì** | `dd/mm/yyyy` + icon |
| `<input type="month">` | **không có gì** | `------ --` + icon lịch |
| `<input type="week">` | **không có gì** | ô week picker |
| `<input type="number" value="3">` | chữ `3`, không có border | ô có viền + spinner |
| `<input type="checkbox" checked>` |hộp nhỏ có dấu tích, **không có outline của `:checked`** | ô xanh trắng |

Trong code, chỗ chặn là `internal/paint/builder_controls.go:87`:

```go
switch typ {
case "checkbox", "radio", "submit", "reset", "button":
    return
}
```

Năm type được special-case thì có đường vẽ riêng (`internal/paint/builder_box.go:119` cho checkbox/radio), còn `color`, `range`, `date`, `month`, `week`, `time`, `datetime-local`, `file` **không có ở cả hai nhánh**: layout cấp box cho chúng (`internal/layout/block.go:2133` chỉ biết checkbox/radio), paint không có đường nào, kết quả là khoảng trắng. Đối chiếu `internal/engine/accessibility.go:149-150` thì thấy AX **có** biết đủ các type này — tức tree nói "có control" trong khi màn hình nói "không có gì". Đó là loại lệch mà QA manual phát hiện ngay bằng mắt, và automation hiện tại không phát hiện được.

## 2. Band parity liên quan

`python3 testdata/parity.py --inspect <fixture>` trên 18 band của TASK-024:

* `forms/04-advanced-inputs.png` — 2 band, rows **376-388** và **443-454**: month/week picker và color picker. **Đã xác nhận** là nguyên nhân ở §1.
* `selectors/02-pseudo-classes.png` — 3 band, rows **471-487**, **506-526**, **551-569**. Xem ảnh crop nhóm band thì khác biệt nhìn thấy được là: checkbox `:checked` (Chromium: ô xanh có viền `outline: 2px solid #2ecc71` từ fixture; Goosie: hộp nhỏ không outline) và `<button>` (Chromium: nút có viền/nền; Goosie: chỉ có chữ). **Band thứ ba (551-569) chưa phân loại chắc chắn** — người nhận task phải xác nhận trước khi sửa, và nếu nó hoá ra là một nguyên nhân khác thì ghi lại, đừng nhét cưỡng ép vào task này.
* `html-features/01-forms-complete.png` — 1 band, rows **593-599**, cao 7px, nằm sát mép viewport 600px. Đây là **artifact của fold** (Chromium để lộ đường viền trên của một box đúng lúc hết khung hình), không phải nội dung thiếu. Việc đúng cho band này là **giữ nguyên trong `render-known-gaps.json`** và ghi lý do, không phải đi sửa engine.

## 3. Yêu cầu

1. **Mỗi input type được hỗ trợ phải vẽ ra thứ gì đó có ink**, kể cả khi chỉ là hộp viền + giá trị. Bất biến được kiểm tự động: với mỗi type trong danh sách ở §1, số ink trong vùng box của control **> 0**. Một control chiếm chỗ trong layout mà không đóng góp pixel nào là bug, bất kể nó "chưa implement đầy đủ".
2. **Không được vẽ sai thành cái trông như đã hỗ trợ.** Nếu engine chưa có date picker thật, vẽ ô viền + text value (dạng `2026-10-25`) là đủ và trung thực; đừng vẽ icon lịch giả. Ghi rõ mức hỗ trợ vào `docs/roadmap-v2.md`.
3. **`:checked` / `:focus` outline** (`testdata/render/selectors/02-pseudo-classes.html:14,19`) phải có đường vẽ, vì đây là tín hiệu phản hồi duy nhất cho người dùng bàn phím.
4. **Text/number/password phải có border theo UA stylesheet**, hiện đang thiếu hoàn toàn (xem bảng §1, hai dòng đầu).

## 4. Tiêu chí nghiệm thu

* **[AC-01]** `go test -count=1 ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test ./test/qc/` PASS; `gofmt -l` sạch.
* **[AC-02]** Probe `/tmp/probe/c.html` render lại phải cho ink **> 0** ở vùng của cả 8 control. Dán số đo ink per-control vào Status (con số, không phải "đã hiển thị").
* **[AC-03]** `forms/04-advanced-inputs.png` không còn band trống → **xoá** entry khỏi `testdata/render-known-gaps.json`. Baseline chỉ được thu nhỏ. Band 593-599 của `html-features/01` vẫn phải còn, kèm ghi chú lý do.
* **[AC-04]** Với `selectors/02-pseudo-classes.png`: hoặc band giảm về 0 và xoá entry, hoặc Status giải thích được band còn lại là nguyên nhân nào khác và mở task mới. Không được nới `MIN_INK`/`BAND_GAP_ROWS` để cho qua.
* **[AC-05]** Mutation proof cho **từng** assert mới (xem `docs/tasks/TASK-020-P1-JOURNEYS.md` §6): bỏ đường vẽ color picker → test phải FAIL; bỏ border của text input → test phải FAIL. Output FAIL nguyên văn + `git status` sạch sau revert. Assert không chứng minh được thì xoá.
* **[AC-06]** Không sửa fixture HTML, không đổi `PASS_AT`/`THRESHOLD`.

## 5. Ràng buộc

* Sở hữu: `internal/paint/builder_controls.go`, `internal/paint/builder_box.go`, test của chúng, `testdata/render-known-gaps.json`, `docs/tasks/TASK-028-FORM-CONTROL-APPEARANCE.md`, và `docs/roadmap-v2.md` (chỉ thêm dòng ghi mức hỗ trợ).
* **Cấm** đổi `internal/layout/**` mà không mô tả trước trong Status: box metric của control là hợp đồng với hit-testing và focus, liên quan trực tiếp tới TASK-026 (form journeys) đang chạy.
* **Cấm** đổi `internal/engine/limits.go` và mọi hằng số admission.
* Không `git push`, không merge `main`, không tạo branch phụ. Test phải chạy trên máy không display.
* Xung đột tiềm tàng với TASK-027 (font coverage) ở đúng các fixture `forms/*`: nếu cả hai cùng chạy, integrator xếp lịch trước/sau, không tự ý rebase lên nhau.
