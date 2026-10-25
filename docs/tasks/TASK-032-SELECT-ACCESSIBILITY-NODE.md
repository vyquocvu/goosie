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

## 6. Status

Implemented on `worktree-task-032-select-ax` from `960b722`, merged into `main` at `910c445`.

- Added `RoleComboBox` (12) and `RoleOption` (13) at the end of the role enum; existing
  role numbers remain unchanged.
- `<select>` now emits a combo box with form value, associated accessible label, disabled
  and focused state, and one option node per direct option. Option values use form-value
  semantics, including text fallback when the `value` attribute is absent. Selected state
  follows `SelectedOption` and therefore the `selected` attribute, with first-option fallback.
- Added the AppKit mappings `AXPopUpButton` and `AXMenuItem`. A running Swift/AppKit probe
  printed `popup=AXPopUpButton, menuItem=AXMenuItem, comboBox=AXComboBox,
  staticText=AXStaticText`. The Darwin role-code oracle ran on this macOS host and kills the
  missing Go mapping mutation; native VoiceOver interaction was not exercised.
- BR-101 now focuses `sel` through `clickNode`. QC golden changes only the `BR-139` test list.
- No new engine gap was found during this task. `docs/qa-open-defects.json` was not edited;
  the suggested closing row is in the completion report.

### Mutation proof [R-7]

| Mutation | Target test | Mutated rc → reverted rc |
| --- | --- | --- |
| Remove the `AtomSelect` accessibility emitter | `TestAccessibilitySelectComboBoxAndOptions` | 1 → 0 |
| Mark every option selected | `TestAccessibilitySelectComboBoxAndOptions` | 1 → 0 |
| Read combo value from the `<select value>` attribute | `TestAccessibilitySelectComboBoxAndOptions` | 1 → 0 |
| Report the first option index as selected after `MoveSelect` | `TestAccessibilitySelectTracksMoveSelect` | 1 → 0 |
| Remove `RoleComboBox` from `axRoleCode` (Darwin) | `TestAXRoleCodeSelectRoles` | 1 → 0 |

### Required verification output

`go test -count=1 ./...`:

```text
?   	github.com/vyquocvu/goosie/cmd/debug-layout	[no test files]
?   	github.com/vyquocvu/goosie/cmd/dump-fonts	[no test files]
?   	github.com/vyquocvu/goosie/cmd/dump-layout	[no test files]
?   	github.com/vyquocvu/goosie/cmd/dump-paint	[no test files]
?   	github.com/vyquocvu/goosie/cmd/dump-url	[no test files]
ok  	github.com/vyquocvu/goosie/cmd/goosie	12.573s
?   	github.com/vyquocvu/goosie/cmd/goosie-headless	[no test files]
?   	github.com/vyquocvu/goosie/cmd/test_colors	[no test files]
?   	github.com/vyquocvu/goosie/cmd/wpt-runner	[no test files]
ok  	github.com/vyquocvu/goosie/internal/archtest	3.972s
?   	github.com/vyquocvu/goosie/internal/ax	[no test files]
?   	github.com/vyquocvu/goosie/internal/bookmarks	[no test files]
ok  	github.com/vyquocvu/goosie/internal/css	1.744s
ok  	github.com/vyquocvu/goosie/internal/dom	0.456s
ok  	github.com/vyquocvu/goosie/internal/download	1.390s
ok  	github.com/vyquocvu/goosie/internal/engine	1.012s
?   	github.com/vyquocvu/goosie/internal/flags	[no test files]
?   	github.com/vyquocvu/goosie/internal/frame	[no test files]
?   	github.com/vyquocvu/goosie/internal/history	[no test files]
ok  	github.com/vyquocvu/goosie/internal/image	0.720s
?   	github.com/vyquocvu/goosie/internal/js	[no test files]
?   	github.com/vyquocvu/goosie/internal/layout	[no test files]
?   	github.com/vyquocvu/goosie/internal/manifest	[no test files]
?   	github.com/vyquocvu/goosie/internal/media	[no test files]
?   	github.com/vyquocvu/goosie/internal/net	[no test files]
?   	github.com/vyquocvu/goosie/internal/paint	[no test files]
?   	github.com/vyquocvu/goosie/internal/pdf	[no test files]
?   	github.com/vyquocvu/goosie/internal/perms	[no test files]
?   	github.com/vyquocvu/goosie/internal/platform	[no test files]
ok  	github.com/vyquocvu/goosie/internal/platform/darwin	2.947s
?   	github.com/vyquocvu/goosie/internal/platform/headless	[no test files]
ok  	github.com/vyquocvu/goosie/internal/preferences	1.553s
?   	github.com/vyquocvu/goosie/internal/raster	[no test files]
?   	github.com/vyquocvu/goosie/internal/session	[no test files]
ok  	github.com/vyquocvu/goosie/internal/style	1.990s
?   	github.com/vyquocvu/goosie/internal/surface	[no test files]
ok  	github.com/vyquocvu/goosie/internal/tabs	2.921s
ok  	github.com/vyquocvu/goosie/internal/toolbar	2.822s
?   	github.com/vyquocvu/goosie/internal/userscript	[no test files]
?   	github.com/vyquocvu/goosie/internal/viewsource	[no test files]
ok  	github.com/vyquocvu/goosie/test/bookmarks	2.715s
ok  	github.com/vyquocvu/goosie/test/css	2.537s
ok  	github.com/vyquocvu/goosie/test/dom	2.733s
ok  	github.com/vyquocvu/goosie/test/domtest	2.990s
ok  	github.com/vyquocvu/goosie/test/download	2.878s
ok  	github.com/vyquocvu/goosie/test/engine	6.928s
ok  	github.com/vyquocvu/goosie/test/entrypoints	20.394s
ok  	github.com/vyquocvu/goosie/test/flags	2.106s
ok  	github.com/vyquocvu/goosie/test/frame	2.134s
ok  	github.com/vyquocvu/goosie/test/gate	31.189s
ok  	github.com/vyquocvu/goosie/test/gatecheck	23.050s
ok  	github.com/vyquocvu/goosie/test/history	2.536s
ok  	github.com/vyquocvu/goosie/test/js	5.135s
ok  	github.com/vyquocvu/goosie/test/layout	2.840s
ok  	github.com/vyquocvu/goosie/test/manifest	2.176s
ok  	github.com/vyquocvu/goosie/test/net	4.532s
ok  	github.com/vyquocvu/goosie/test/paint	2.484s
ok  	github.com/vyquocvu/goosie/test/pdf	1.696s
ok  	github.com/vyquocvu/goosie/test/perms	1.790s
ok  	github.com/vyquocvu/goosie/test/platform	0.644s
ok  	github.com/vyquocvu/goosie/test/qc	1.112s
ok  	github.com/vyquocvu/goosie/test/raster	1.971s
ok  	github.com/vyquocvu/goosie/test/session	1.273s
ok  	github.com/vyquocvu/goosie/test/style	2.102s
ok  	github.com/vyquocvu/goosie/test/surface	2.200s
ok  	github.com/vyquocvu/goosie/test/tabs	2.951s
ok  	github.com/vyquocvu/goosie/test/toolbar	3.011s
ok  	github.com/vyquocvu/goosie/test/userscript	2.607s
ok  	github.com/vyquocvu/goosie/test/viewsource	2.421s
ok  	github.com/vyquocvu/goosie/test/wpt	9.447s
```

`go test -count=1 -race ./test/engine/ ./cmd/goosie/ ./internal/platform/darwin/`:

```text
ok  	github.com/vyquocvu/goosie/test/engine	20.938s
ok  	github.com/vyquocvu/goosie/cmd/goosie	22.743s
ok  	github.com/vyquocvu/goosie/internal/platform/darwin	2.573s
```

`go test -count=1 ./internal/archtest/`:

```text
ok  	github.com/vyquocvu/goosie/internal/archtest	2.956s
```

`go test -count=1 ./test/qc/`:

```text
ok  	github.com/vyquocvu/goosie/test/qc	0.505s
```

`go vet -all ./...`: no output; exit code 0.

`python3 testdata/parity.py`: `80/80 passing (100.0%), 0 failing, 0 errors`; average match
`96.07%` (threshold 30, pass_at 90.0%). AX additions do not alter pixels; parity was run as
requested for the VoiceOver-visible output and confirmed the render baseline remains green.

The integrator re-ran all five mutations above on the branch and each produced the quoted
FAIL, then PASS on revert (verdict recorded in `docs/qa-open-defects.json`, `GOOSIE-AX-001`).
Two disclosures the agent did not volunteer, found while reviewing the diff:

* Option nodes carry the **select's** bounds, not their own, because a collapsed `<select>`
  lays its options out nowhere. An AX client that hit-tests option rects will get the combo
  rect for every option. Not wrong for a collapsed control, but it is a limitation, not a
  measurement of option geometry.
* The option value rule (value attribute, else text content) now exists in **three** places:
  `forms.go:213` `controlValue`, the `Session.controlValue` method (`focus.go:537`) and inline
  in `accessibility.go`. They agree today because all three delegate selectedness to
  `SelectedOption`, and `SelectedOption` and `forms.go` both pick the *first* `selected`
  option while HTML picks the *last*. Neither the duplication nor the first/last difference
  is covered by a test; unify them (or pin the tie-break with a test) before writing another
  consumer of this value.
