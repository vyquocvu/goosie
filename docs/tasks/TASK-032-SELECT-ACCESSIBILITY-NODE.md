# TASK-032 — `<select>` phải có node trong accessibility tree (BR-139)

**Priority:** P1. **Method:** INT. **Owner:** chưa gán. **Blocked case:** BR-139
(`docs/qa-test-catalog.md:169` — "Accessibility tree trên trang có heading/link/button/
input → role, name, value, href phản ánh DOM", hiện `NOT RUN`).
**Defect:** `GOOSIE-AX-001` trong `docs/qa-open-defects.json`.

## 1. Vấn đề, đo chứ không đoán

TASK-030 làm `<select>` đổi được option bằng bàn phím, nhưng control này **vẫn vô hình với
người dùng assistive**: nó không xuất hiện trong AX tree ở bất kỳ dạng nào.

Bằng chứng đã lấy trên `main` (`7ecdc04`, sau khi merge TASK-030):

| Sự thật | Chỗ trong code |
| --- | --- |
| AX builder switch trên `n.DataAtom` có `meter`/`a`/`img`/`button`/`input`/`textarea` — **không có `dom.AtomSelect`** | `internal/engine/accessibility.go:120-162` |
| Vì thế select rơi vào nhánh generic: `hasKids` sai hoặc `len(kids) == 0` ⇒ trả `nil` | `internal/engine/accessibility.go:164-180` |
| Probe thật: layout arena có `obj tag="select" id="sel"`, nhưng `Session.AccessibilityTree()` dưới form chỉ trả về node `role=4 id="send"` (button) | ghi ở `docs/tasks/TASK-030-SELECT-INTERACTION.md` §9.4 |
| Hệ quả thực dụng: helper click theo toạ độ AX (`clickNode`) không định vị được select; journey BR-101 buộc phải dùng `clickElementByID` | `cmd/goosie/journey_p1_forms_test.go` (comment ngay trước `browser.clickElementByID(t, "sel")`) |
| `ax.Node` **đã có** đủ trường cần thiết: `Value`, `Selected`, `Disabled`, `Focused`, `Children` | `internal/ax/ax.go:29-45` |
| AX **không có** role nào cho combo box / option | `internal/ax/ax.go:11-22` (`RoleDocument`…`RoleTab`) |
| Role lạ trên macOS âm thầm biến thành GROUP, không lỗi | `internal/platform/darwin/window.go:458-459` (`default: return C.GOOSIE_AX_GROUP`) |
| Hằng role C nằm ở `shim.h` và map sang `NSAccessibility…Role` trong `shim_darwin.m` | `internal/platform/darwin/shim.h:213-223`, `shim_darwin.m:100-112` |

## 2. Thiết kế tối thiểu

1. `internal/ax/ax.go`: thêm **cuối** enum `RoleComboBox` và `RoleOption` (không chèn giữa —
   giá trị enum đang được dùng trong fixture và assertion theo số, ví dụ `role=4`).
2. `internal/engine/accessibility.go`: `case dom.AtomSelect:` emit
   `RoleComboBox{ID, Label: <label chữ>, Value: controlValue(sel), Disabled, Focused}` với
   `Children` là một `RoleOption` mỗi `<option>`: `Label` = text của option (fallback
   `value`), `Value` = `controlValue(option)`, `Selected` = option mang `selected` (hoặc
   option đầu khi chưa ai selected — **giống hệt** quy tắc `SelectedOption`
   `internal/engine/focus.go:616`, để AX không bao giờ nói khác DOM). Option `disabled` vẫn
   có node nhưng `Disabled: true`. Không có option ⇒ ComboBox không children.
   `select` phải **dừng** ở nhánh của nó, không rơi xuống generic path.
3. `internal/platform/darwin`: thêm `GOOSIE_AX_COMBO_BOX` / `GOOSIE_AX_OPTION` vào
   `shim.h`, map trong `shim_darwin.m` (`NSAccessibilityPopUpButtonRole` /
   `NSAccessibilityStaticTextRole`… — chọn role Apple thật sự có, **đo bằng cách chạy**, không
   đoán), và thêm `case` trong `axRoleCode`. Nếu không map thì `default` GROUP là một bug
   chết: test nào cũng xanh mà VoiceOver vẫn không đọc được.
4. Không sửa paint, không sửa layout, không đụng `internal/engine/select.go`.

## 3. Test phải có

* `test/engine/accessibility_test.go` **đã tồn tại** và đã có helper `axFind(tree, role,
  label)` (dùng ở `:55`, `:69`) — viết test mới trong file đó, tái dụng `axFind`, không
  sinh helper song song.
  1. select 3 option, option 2 `selected` ⇒ đúng **một** node `RoleComboBox` id=`sel`,
     `Value` = value option 2, `Label` không rỗng, 3 con `RoleOption`, `Selected` chỉ một.
  2. chưa option nào `selected` ⇒ `Value` = option đầu (bất biến `SelectedOption`).
  3. `disabled` ⇒ node vẫn có, `Disabled == true`.
  4. đổi lựa chọn bằng `MoveSelect` (TASK-030) rồi build lại AX ⇒ `Value` và `Selected`
     theo đúng DOM. Đây là test nối BR-139 với BR-101, bắt buộc có.
  5. `select` rỗng ⇒ ComboBox không children, không panic.
* `test/qc`: gắn `// qc:BR-139` cho test 1 và 4; chạy `go test ./test/qc/ -update` rồi
  **kiểm tra golden chỉ đổi row BR-139**.
* Journey: `clickNode(t, "sel")` phải dùng được thay cho `clickElementByID` — sửa journey
  BR-101 (`cmd/goosie/journey_p1_forms_test.go`) để nó đi qua AX path, vì đó chính là bằng
  chứng người dùng assistive định vị được control.

## 4. Nghiệm thu `[R-7]`

Proof là **tắt hành vi**, mỗi cái dán `FAIL → revert → PASS`:

* (i) bỏ `case dom.AtomSelect` (select rơi lại generic path) ⇒ test 1 **FAIL**.
* (ii) set `Selected` cho **mọi** option thay vì option đang chọn ⇒ test 1 **FAIL**
    (đây là kiểu sai im lặng nguy hiểm nhất với AX client).
* (iii) `Value` lấy từ `GetAttribute("value")` của `<select>` (thuộc tính mà HTML không dùng
    cho việc này) thay vì `controlValue` ⇒ test 1 và 4 **FAIL**.
* (iv) `Selected` tính lại theo index chứ không theo attribute, rồi `MoveSelect` đổi attribute
    ⇒ test 4 **FAIL** (AX và DOM lệch nhau, đúng loại bug BR-139 mô tả).
* (v) xoá `case` mới trong `axRoleCode` để role rơi vào `default` ⇒ test darwin-side (nếu
    có harness) hoặc **phải ghi nhận là không có test nào giết được** và disclose trong
    §Status — không được im lặng.

Baseline **phải tiến**: BR-139 có tag trong golden. Không sửa `docs/qa-test-catalog.md`;
`GOOSIE-AX-001` chỉ được đóng khi test ở §3 xanh và sweep đọc ra nó `closed`.

## 5. Ràng buộc vận hành

Không `git push`, không merge `main`, không tạo worktree con. Test chạy trên máy **không có
display** (`platform/headless`); phần cgo chỉ được kiểm bằng `go vet`/build, không assert
kết quả VoiceOver. `go test ./internal/archtest/` và `go vet -all ./...` phải xanh; thêm role
vào enum không được làm đổi số thứ tự role cũ. Không đổi threshold parity.
