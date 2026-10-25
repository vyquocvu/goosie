# TASK-030 — `<select>` tương tác được bằng bàn phím (BR-101)

**Priority:** P1. **Method:** INT + CUA. **Owner:** integrator (đã làm, xem §9). **Blocked case:** BR-101
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

**Đã implement + chạy được trong worktree, chờ merge.** Người thực hiện: integrator (không
gán agent, vì đây là chỗ thiết kế mình đã đo và sai một lần ở §6).

### 9.1 Đã làm

| File | Thay đổi |
| --- | --- |
| `internal/engine/select.go` (mới) | `Session.FocusedSelect()`, `Session.MoveSelect(EditAction)`, `selectOptionNodes`, `next/prev/first/lastEnabledOption`, `setSelectedOption`. |
| `internal/engine/focus.go` | `clickFocusable(n)` (editable **hoặc** `select`), tách `controlAt` thành `controlAtMatching(x, y, admit)`; `FocusControl` dùng `clickFocusable`, `controlAt` (hover/I-beam) vẫn chỉ editable. |
| `cmd/goosie/interaction.go` | `scrollContentKey` bỏ qua phím khi select đang focus (`FocusedSelect() == nil`); trước `Edit`, nếu `FocusedSelect() != nil && MoveSelect(action)` thì reflow + repaint và **return** (không xuống đường edit). |
| `test/engine/select_keyboard_test.go` (mới) | 10 unit test, 2 cái tag `// qc:BR-101`. |
| `cmd/goosie/journey_p1_forms_test.go` | stub `t.Skip` BR-101 thay bằng journey thật (`// qc:BR-101 TestJourneySelectOptionChange`). |
| `test/qc/traceability.golden.json` | `-update` thêm mục BR-101. |

**Lệch so với §3 — có lý do, ghi lại để người sau không đoán:** thay `SelectOptionDelta` /
`SelectOptionEdge` bằng **một** API `MoveSelect(EditAction)`. Lý do: phím mà shell nhận
(`0xF700`–`0xF705`) đã được `cmd/goosie/interaction.go:381-405` map sẵn sang
`engine.EditAction`; hai API riêng sẽ bắt shell phải tự dịch lại lần hai và tạo thêm một
chỗ lệch. §3 cũng định sửa `keyDefaultAction` — **đã không sửa**, vì §2 chỉ ra (và test
chứng minh) phím mũi tên **không bao giờ tới** `keyDefaultAction`: nó vào `Session.Edit`,
không vào `Session.PressKey`. Proof (i) vì vậy được thay bằng proof M6 dưới đây (tắt guard
ở shell), mới là hành vi thật.

### 9.2 Test

`go test -count=1 ./test/engine/ -run 'TestSelect|TestClickFocusesSelect'` → `ok 0.54s`
(10 test). `go test -count=3 ./cmd/goosie/ -run TestJourneySelectOptionChange` → `ok`.
Journey dùng `clickElementByID` chứ không `clickNode` (select không có node AX, §9.4), chờ
theo **snapshot có khoá** `Session.FocusedSelectValue()` chứ không cầm node DOM (§9.6), và
assert paint ở mức cả chuỗi chứ không per-key (§9.5).

### 9.3 `[R-7]` — six mutations, mỗi cái là **tắt hành vi**

Chạy bằng runner; mỗi lần mutate chỉ hai package đó được chạy, sau đó restore tức thì.
`rc` là exit code thật của `go test`.

| # | Mutation (tắt gì) | `test/engine` | `cmd/goosie` journey |
| --- | --- | --- | --- |
| — | Baseline | `rc=0 ok` | `rc=0 ok` |
| M1 | `setSelectedOption` không được gọi ⇒ DOM không bao giờ đổi | `rc=1` FAIL ×4 (`TestSelectArrowKeysMoveSelection`, `...ConsumedAtTheEnds`, `...SkipsDisabledOptions`, `...HomeEndGoToTheEdges`) | `rc=1` FAIL `TestJourneySelectOptionChange` |
| M2 | Bỏ dispatch `input` | `rc=1` FAIL `TestSelectArrowKeysMoveSelection` | `rc=0 ok` (journey không đếm `input`) |
| M3 | Không bỏ `selected` khỏi option cũ | `rc=1` FAIL ×4 (gồm `...CollapsesMultipleAuthoredSelections`) | `rc=1` FAIL |
| M4 | `nextEnabledOption` trả option kế tiếp kể cả `disabled` | `rc=1` FAIL `TestSelectSkipsDisabledOptions` | `rc=0 ok` |
| M5 | `clickFocusable`: select không focus được bằng chuột | `rc=1` FAIL `TestClickFocusesSelectButNotDisabledControls` | `rc=1` FAIL |
| M6 | Bỏ guard `FocusedSelect() == nil` ở shell ⇒ Home/End cuộn trang | `rc=0 ok` (unit không chạy shell) | `rc=1` FAIL |
| — | Restore | `rc=0 ok` | `rc=0 ok` |

**Không proof nào vô hiệu.** M2/M4 chỉ unit giết, M6 chỉ journey giết — đúng kỳ vọng: M6
là hành vi shell, M2/M4 là ngữ nghĩa engine mà trang test không nhìn thấy. Ghi rõ để
người sau không tưởng M4 được e2e che.

### 9.4 Hai gap engine mới, phát hiện lúc chạy chứ không lúc đọc

1. **`<select>` không có node trong AX tree.** Probe: layout arena có
   `obj tag="select" id="sel"`, nhưng `Session.AccessibilityTree()` dưới form chỉ trả
   `role=4 id="send"`. Hệ quả: `clickNode` (tọa độ lấy từ AX) không dùng được với select,
   và người dùng screen reader không thấy control nào. **Tách ra defect/task riêng** —
   BR-101 không đòi AX, nên không gắn vào đây.
2. **`disabled` input trước đây vẫn nhận click focus.** `setFocusedNodeLocked` không chặn
   disabled; chỉ `controlEditable` quyết định ai được gọi. `clickFocusable` giữ nguyên
   guard `n.HasAttribute("disabled")`, nên hành vi **đổi có chủ đích** với mọi control
   disabled khi click — đã pin bằng `TestClickFocusesSelectButNotDisabledControls`
   (`#txt` disabled: click không focus gì).

### 9.5 Phát hiện về tile cache (sửa lại cách nghiệm thu paint)

`surface.LoopStats.Rasterize` đếm tile **mới** được rasterize; nội dung từng thấy rồi sẽ
tính là `Reused`. Vì vậy `Rasterize` **không** phải oracle per-change: chuyển option
`Home` về option đầu thường trùng nội dung tile đã cache. Journey assertions đổi thành
DOM-state per-key + **một** assertion trình độ cả chuỗi:

```go
if got := browser.f.loop.Stats().Rasterize; got == rasterAtFocus {
    t.Fatalf("no tile was rasterized across three selection changes ...")
}
```

### 9.6 `go test -race` bắt được một bug của **chính test**, không phải của engine

Journey BR-101 đọc lựa chọn bằng `engine.SelectedOption(sel)` + `GetAttribute("value")`
**trên goroutine test**, trong lúc goroutine pump của shell gọi `MoveSelect` →
`setSelectedOption` → `dom.Node.SetAttribute` (`internal/dom/node.go:329`). `-race` báo:

```
Write at 0x00c0154c9060 by goroutine 605:
  internal/dom.(*Node).SetAttribute()                 node.go:329
  internal/engine.setSelectedOption()                 select.go:160
  internal/engine.(*Session).MoveSelect()             select.go:109
  cmd/goosie.(*framePath).contentKey()                interaction.go:408
Previous read at 0x00c0154c9060 by goroutine 596:
  internal/dom.(*Node).HasAttribute()                 node.go:354
  internal/engine.SelectedOption()                    focus.go:626
  cmd/goosie.TestJourneySelectOptionChange.func4()    journey_p1_forms_test.go:416
```

**Đường sản phẩm không race.** Mọi chỗ đọc DOM trong tiến trình vẽ đi qua
`Session.PaintChecked` (`internal/engine/paint.go:15` lấy `opMu`), còn input/toggle/
`MoveSelect` cũng lấy `opMu`. Chỉ cái node pointer mà test tự giữ (`sel`) là ngoài vòng
khoá. Bài học cho các journey sau: **đọc trạng thái DOM từ goroutine khác phải qua
accessor có khoá**, không cầm node rồi tự đi.

Sửa: thêm `Session.FocusedSelectValue()` (`internal/engine/select.go`) — snapshot đúng
giá trị form sẽ submit, đọc dưới `opMu`, dùng `controlValue` (forms.go:94) nên trùng khớp
với payload; `TestSelectMoveChangesSubmittedValue` pin bất biến đó
(`FocusedSelectValue reports %q but the form submits %q`). Journey bỏ luôn
`ElementByID("sel")` khỏi goroutine test.

Chạy lại: `-race -count=2` cả hai package → `ok`. **Battery §9.3 chạy lại sau khi sửa,
từng dòng kết quả giống hệt** (M1–M6 vẫn bị giết, baseline và restore vẫn xanh).

### 9.7 Gate trên nguyên worktree (2026-10-25)

| Lệnh | Exit | Ghi chú |
| --- | --- | --- |
| `go test -count=1 -race -timeout 1800s ./...` | 0 | không dòng `FAIL`, không `WARNING: DATA RACE` |
| `go test -count=1 ./internal/archtest/` | 0 | không thêm package mới, import table giữ nguyên |
| `go vet -all ./...` | 0 | |
| `go test -count=5 ./cmd/goosie/ -run TestJourneySelectOptionChange` | 0 | BR-101 chạy 5 lần liên tiếp, không flake |
| `go test ./test/qc/` | 0 | golden đã có BR-101; `gofmt -l internal cmd test` sạch |

BR-101 đi từ `NOT RUN` không test sang có test unit + journey thật. **Không sửa status
trong `docs/qa-test-catalog.md`** — đúng ràng buộc §6: status là chỗ của
`scripts/rc-sweep.sh` (TASK-029). Merge `worktree-task-030-select` vào `main`, không
`git push`.

