# TASK-035 — Trạng thái nút chrome và zoom phải soi được qua AX (BR-126/127/128/129/130/131)

**Priority:** P1. **Method:** CUA → INT (oracle tự động). **Owner:** chưa gán.
**Blocked cases:** BR-126, BR-127, BR-128, BR-129, BR-130, BR-131 trong
`docs/qa-test-catalog.md` — cả 6 đang `NOT RUN` và `tests: 0`
(đo từ `test/qc/traceability.golden.json` tại `d24ec99`). Catalog gắn nhãn 6 case này là
`CUA`, nhưng **toàn bộ hành vi chúng mô tả đã nằm trong app path** (bảng §1) và repo này đã
có hạ tầng journey đọc AX tree (`clickNode`, `pressKey`, `window.AccessibilityTree()`), nên
chúng tự động hoá được. Task này là bằng chứng cho nhận định đó.
**Defect liên quan:** `GOOSIE-TRACE-001`.

## 1. Hành vi đã đo trên `main`

| Hành vi | Chỗ trong code | Rule đọc từ source |
| --- | --- | --- |
| Back/Forward là node AX có cờ disabled | `cmd/goosie/toolbar_window.go:286-287` | `appendTool("goosie.back", …, !canBack, false)`, `appendTool("goosie.forward", …, !canForward, false)` |
| Trail rỗng ⇒ không đi được | `internal/toolbar/history.go:13-15,121-131` | `NewHistory()` khởi tạo `index: -1`; `CanBack/CanForward` lấy từ lock |
| Nút Reload biến thành Stop khi đang tải | `cmd/goosie/toolbar_window.go:290`, `internal/toolbar/toolbar.go:1099,1179` | `reloadLabel = "Stop loading"`; có `drawStop` riêng |
| Callback Stop | `internal/toolbar/toolbar.go:105,706-708`, `cmd/goosie/main.go:382` | `OnStop func()` được wire ở app; invoke lấy `cb := s.OnStop` |
| Cờ loading của tab | `cmd/goosie/navigation.go:481-485` (`tab.Nav.Loading = true`, `Serial++`, `Nav.Cancel`) | loading là state thật, không suy ra từ timing |
| Zoom bị chặn biên | `cmd/goosie/interaction.go:648-657` | `minZoom = 0.25`, `maxZoom = 5.0`, `clampZoom` |
| Phím zoom và reset | `cmd/goosie/toolbar_window.go:1128-1139` | `onZoom(zoomStep)` / `onZoom(-zoomStep)` / `onZoom(0)` cho Cmd+0 |

**Phải đo trước khi viết assertion (không được giả định):**
* Indicator zoom hiển thị ở đâu và qua node AX nào (id/label/value). Nếu **không tồn tại**
  bất kỳ chỗ nào user nhìn thấy để phân biệt 100% với 125%, thì BR-129 chỉ assert được phần
  *tỷ lệ nội dung* — phần indicator là **gap product**, phải báo cáo chứ không được assertion
  lách qua.
* Stop sau khi tải xong có phải no-op thật không (`OnStop` khi `Nav.Loading == false`).

## 2. Test phải có — mỗi case một oracle

Đặt tại `cmd/goosie/` (journey) vì 6 case đều là hành vi chrome-level. Dùng helper đã có
(`newBrowserE2E`, `pressKey`, `inject`, `clickNode`, `waitFor`); **không** dùng `.Arena` từ
goroutine test — đó là `GOOSIE-TEST-001`, đừng làm nặng thêm.

1. **BR-126** — tab mới (trail rỗng) ⇒ `goosie.back`/`goosie.forward` `Disabled == true`;
   click vào node disabled ⇒ URL không đổi, không có navigation nào được phát (đếm bằng
   httptest server). Mở A→B ⇒ `goosie.back` enabled, `goosie.forward` vẫn disabled.
2. **BR-127** — server delay ~300ms; trong lúc tải, node reload mang nhãn Stop và callback
   Stop có hiệu lực: request bị hủy, tab hết loading, tài liệu cũ còn nguyên. Sau khi tải
   xong, gọi Stop ⇒ không được xóa/hỏng tài liệu, không được đổi URL.
3. **BR-128** — reload một trang đã tải xong ⇒ label trở lại Reload (không kẹt ở Stop), nội
   dung re-render, và đúng một request mới tới server; `Nav.Serial` tăng — assertion phải dựa
   trên state đã công bố, không dựa vào `time.Sleep`.
4. **BR-129** — Cmd+Plus nhiều lần ⇒ layout đổi theo hướng phóng to và giá trị zoom tăng đơn
   điệu tới trần; số pixel/glyph measure (nếu có indicator thì assert qua AX).
5. **BR-130** — Cmd+Minus tới biên ⇒ zoom không xuống dưới `minZoom`, không văng khỏi trần
   `maxZoom`, cửa sổ vẫn nhận input (assert: sau khi chạm biên, một cú PageDown vẫn scroll).
6. **BR-131** — Cmd+0 sau khi zoom ⇒ zoom về đúng 1.0, layout được tính lại (viewport offset
   hợp lệ), và AX/indicator (nếu tồn tại) báo 100%.

Gắn `// qc:BR-126` … `// qc:BR-131` (mỗi case vào đúng test của nó), rồi
`go test -count=1 ./test/qc/ -update` và kiểm diff golden **chỉ đổi 6 row**.

## 3. Nghiệm thu `[R-7]`

Proof là **tắt hành vi**; dán nguyên văn `FAIL → revert → PASS` kèm bảng mutation:

* (i) cho `appendTool` back/forward luôn truyền `false` cho disabled ⇒ BR-126 **FAIL**.
* (ii) bỏ `!canBack` trong `traverseTab` (delta luôn áp dụng) ⇒ BR-126 **FAIL** ở vế click.
* (iii) để `reloadLabel` không đổi khi `Nav.Loading` ⇒ BR-127 **FAIL**.
* (iv) cho `OnStop` chạy `Cancel()` ngay cả khi không loading và xóa session ⇒ BR-127 vế
  no-op **FAIL**; nếu không fail, disclose.
* (v) bỏ `clampZoom` ⇒ BR-130 **FAIL** (zoom vượt 0.25).
* (vi) cho `onZoom(0)` (Cmd+0) trở thành no-op ⇒ BR-131 **FAIL**.

Không được chứng minh bằng cách trả giá trị sai: phải tắt chính tính năng đó.

## 4. Ràng buộc vận hành

* Branch từ **HEAD của `main`** lúc nhận prompt; làm trong worktree riêng; không `git push`,
  không merge `main`, không tạo worktree con.
* Không sửa `docs/qa-test-catalog.md`; `docs/qa-open-defects.json` do integrator sửa — chỉ
  **đề xuất** dòng mới (ví dụ gap "không có indicator zoom người dùng nhìn thấy được") trong
  báo cáo cuối, theo schema `id, priority, status, owner, summary, evidence, closed_by`.
* Cấm nới `PASS_AT`/`MIN_INK`/`gateFootprintSlack`/`testdata/render-known-gaps.json`.
* Trước khi chạy suite full hoặc gate footprint, kiểm `uptime`; load 1-minute > 30 thì chờ
  (`GOOSIE-FLAKE-001`: gate at-rest đỏ giả dưới tải). Không tạo tải bằng `while :`.
* Verification dán output: `go test -count=1 ./cmd/goosie/`,
  `go test -count=1 -race ./cmd/goosie/`, `go test -count=1 ./test/qc/ ./internal/archtest/`,
  `go vet -all ./...`, và `go test -count=1 ./...` full ở cuối.
* Test phải xanh trên máy không có display. Không assert duration; `waitFor` theo state, không
  theo `time.Sleep`.

## 5. Ngoài phạm vi

BR-132 (zoom per-tab khi chuyển tab) — cần quyết định policy scope trước; BR-133..137
(find-in-page: `runFind` `interaction.go:731`, `findStep` `:758`, `closeFind` `:797`,
`clearFindHighlights` `:816` — đã tồn tại nhưng cần batch riêng vì số lượng 5 case và cần
highlight oracle); BR-032/BR-033 (drag tab, overflow) cần pointer-level fixture;
BR-141 (theme) thuộc họ parity pixel.
