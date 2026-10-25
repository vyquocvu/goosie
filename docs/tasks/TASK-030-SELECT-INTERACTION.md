# TASK-030 — `<select>` tương tác được bằng bàn phím (BR-101)

**Priority:** P1. **Method:** INT + CUA. **Owner chưa gán.** **Blocked case:** BR-101
(`docs/qa-test-catalog.md`: "Chọn option trong select bằng chuột/bàn phím → value,
focus và paint khớp lựa chọn" — hiện `NOT RUN`).

## 1. Vấn đề

`<select>` là control **focus được nhưng không đổi được giá trị**. Người dùng tab tới
nó, thấy mũi tên dropdown, gõ phím thì trang **cuộn** thay vì đổi option. Mọi form có
select (quốc gia, số lượng, vai trò) đều không dùng được thật. Đây là P1 bị chặn, không
phải P2.

## 2. Bằng chứng đã đo lại trên `main` (đừng điều tra lại, chỉ xác nhận từng dòng)

| Sự thật | Chỗ trong code |
| --- | --- |
| `IsActivatable` có `button`, `summary`, `a[href]`, `input[submit/reset/button/image/checkbox/radio]` — **không có `select`** | `internal/engine/focus.go:573-590` |
| `select` **được** coi là focusable | `internal/engine/focus.go:193` (`case "textarea", "select", "button"`) và `:807` |
| Engine đã có hàm đọc lựa chọn hiện tại, ưu tiên `selected`, fallback option đầu tiên | `internal/engine/focus.go:595 SelectedOption` |
| `keyDefaultAction` chỉ xử lý `Tab` / `Enter` / `Space`; không có `ArrowUp`/`ArrowDown`/`Home`/`End` | `internal/engine/events.go:75` (switch), `PressKey` ở `:46` |
| Phím mũi tên vì thế rơi về shell và bị ăn làm cuộn trang | `cmd/goosie/interaction.go:298` → `:425 scrollContentKey` |
| Paint chỉ vẽ nhãn option hiện tại + icon mũi tên; không có dropdown nào được rasterize | `internal/paint/builder_controls.go:269 paintSelect`, gọi từ `internal/paint/builder_box.go:101-102` |
| Thuộc tính DOM là nguồn sự thật cho value khi collect form | `internal/engine/forms.go:85` (`case "select": return true`) |

Node API có sẵn để sửa attribute: `internal/dom/node.go:316 SetAttribute`, `:363 RemoveAttribute`.

## 3. Thiết kế tối thiểu

**Bàn phím, không popup.** Popup listbox rasterize là việc khác (xem §7). Giá trị của
engine là: chọn option bằng phím phải được, và kết quả phải giống Chromium ở DOM + event
+ value — tức là form submit đi lên server mang option đúng.

1. `Session.SelectOptionDelta(delta int)` và `Session.SelectOptionEdge(first bool)`:
   nhận node `<select>` đang focus, tính index option hiện tại theo `SelectedOption`,
   dời trong phạm vi số option **không disabled**; tới đầu/cuối thì **dừng** (không
   wrap — Chromium không wrap với single-select).
2. Đổi lựa chọn = **bỏ `selected` khỏi option cũ, đặt `selected` lên option mới**, rồi
   dispatch `input` và `change` trên `<select>` (thứ tự đó, cả hai đều bubles). Nếu một
   listener `change` gọi `preventDefault`, lựa chọn **vẫn đổi** — đúng theo spec: với
   select, preventDefault không huỷ action người dùng đã thực hiện.
3. `keyDefaultAction` thêm nhánh: khi `keyboardActivationTarget`/focused node là
   `<select>`, `ArrowUp`→-1, `ArrowDown`→+1, `Home`→first, `End`→last, và trả `true` để
   shell **không** cuộn. Không đổi hành vi khi focus ở nơi khác.
4. Shell: sau khi engine tiêu thụ phím select, `repaintTab` + reflow vì nhãn có thể dài
   hơn option cũ (làm layout box đổi bề rộng).
5. `paintSelect` không cần sửa: nó đã đọc `SelectedOption`. **Nếu sau khi đổi attribute
   mà nhãn vẫn cũ, đó là bug cache display-list, phải sửa ở tầng publish, không được vẽ
   lại bằng cách đoán.**

## 4. Việc phải làm

* Engine: `internal/engine/focus.go` (helper đổi lựa chọn), `internal/engine/events.go`
  (`keyDefaultAction`), `internal/engine/engine.go` (nếu cần expose API).
* Chrome: `cmd/goosie/interaction.go` để phím select không thành cuộn.
* Test unit: `test/engine/select_interaction_test.go` — DOM/event là mặt trận chính.
* Journey: `cmd/goosie/` — journey BR-101 thật (focus bằng tab, ArrowDown, submit, đọc
  payload HTTP) thay cho stub đang skip. **Chỉ gắn `// qc:BR-101` khi test có assertion
  thật**: `test/qc/traceability_test.go` giờ fail nếu tag đặt lên test mà dòng đầu là
  `t.Skip`.

## 5. Test phải có (mỗi cái một lần chạy, dán output nguyên văn)

1. ba option, focus select, `ArrowDown` ×2 ⇒ option 3 có `selected`, option 1 không còn;
   `change` fire đúng 2 lần; `input` đúng 2 lần.
2. `ArrowDown` ở option cuối ⇒ không đổi, không event, `PressKey` trả `true` (chứng tỏ
   engine tiêu thụ chứ không để shell cuộn).
3. Option giữa có `disabled` ⇒ bị nhảy qua.
4. Submit form sau khi đổi ⇒ payload HTTP mang `name=value` của option **mới**.
5. `Home`/`End` về đúng biên.
6. Không có option nào `selected` ⇒ điểm khởi đầu là option đầu (theo `SelectedOption`).

## 6. Nghiệm thu `[R-7]`

Mỗi proof là **tắt hành vi**, không phải trả sai giá trị; dán `FAIL → revert → PASS`:

* (i) bỏ nhánh Arrow trong `keyDefaultAction` ⇒ test 1 và 4 **FAIL**.
* (ii) để `SelectOptionDelta` đổi attribute nhưng **không** dispatch `change` ⇒ test 1
  **FAIL**.
* (iii) xoá bước bỏ `selected` khỏi option cũ ⇒ test 1 **FAIL** (hai option cùng selected,
  `SelectedOption` trả về option đầu và value submit sai im lặng).
* (iv) cho `disabled` lọt qua bộ lọc ⇒ test 3 **FAIL**.

Baseline phải **tiến**: BR-101 có tag trong `test/qc/traceability.golden.json` và test
tương ứng xanh với `-count=5`; không được sửa `docs/qa-test-catalog.md` (status do
`scripts/rc-sweep.sh` của TASK-029 sở hữu).

## 7. Ngoài phạm vi (ghi rõ để người sau không đoán)

Popup dropdown rasterize (cần menu layer + hit-test riêng), `multiple`/`size>1` listbox,
type-ahead gõ chữ để nhảy option, `<optgroup disabled>` (engine chưa hỗ trợ), và mở
bằng chuột trái. Single-select keyboard là thứ chặn BR-101; những cái trên là case khác.

## 8. Ràng buộc vận hành

Không `git push`, không merge `main`, không tạo worktree con. Mọi test phải chạy trên máy
**không có display** (`platform/headless`). `go vet -all ./...` và
`go test ./internal/archtest/` phải xanh. Không đổi priority font family hay threshold
parity để "qua gate".

## 9. Status

Chưa bắt đầu.
