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

## 6. Kết quả

**Kết luận trước, và nó sửa §1:** indicator zoom **không phải gap pixel, mà là gap AX.** Badge
`150%` vẫn được paint (`internal/toolbar/toolbar.go:1466`, trong `drawAddressBar`), nhưng chưa từng
có node AX nào mang nó. Vì thế hai assertion `if _, ok := findAXNode(..., "goosie.zoom"); ok {…}`
trong `cmd/goosie/journey_p1_chrome_zoom_ax_test.go` **không bao giờ chạy nhánh đúng**: product có
xoá luôn badge đi thì test vẫn xanh. Batch này xoá cả hai chỗ vacuous đó và làm hành vi thành thật
— node `goosie.zoom` tồn tại **đúng khi** badge được paint, không nhân bản rule.

### 6.1 Rule duy nhất, một chỗ đọc

| Chỗ | Vai trò |
| --- | --- |
| `internal/toolbar/toolbar.go:302 zoomBadgeRect` | vị từ duy nhất: ẩn khi `FindActive`, khi `Zoom <= 0`, và khi `Zoom == 1.0` sau grace 2.5s. Paint path đọc nó (`:1466`), hit-test đọc nó (`:425`, `:747`). |
| `internal/toolbar/toolbar.go:328 ZoomBadge(toolbarW)` | API mới, **gọi lại chính `zoomBadgeRect`**, trả `label` (đúng `FormatZoomPercent`, `:298`) + rect ở toạ độ logical của hàng toolbar (dựng device rect như `drawAddressBar` rồi chia `sc`); `shown=false` đúng khi paint ẩn badge. Không restating rule. |
| `cmd/goosie/toolbar_window.go:306-308` | node được phát tại chỗ các node chrome khác, dùng `appendTool` (`:276`) — hàm này nay nhận `ax.Role` làm tham số đầu tiên (4 call site cũ truyền `ax.RoleButton`, không đổi hành vi), node zoom là `ax.RoleMeter` + label `"Zoom level"` + value là nhãn percent, rect của badge dịch vào hàng toolbar bằng `translateAxRect(..., toolY)` như mọi nút khác. `RoleMeter` đã có đường map sang `GOOSIE_AX_METER` (`internal/platform/darwin/window.go:452`). |

Không thêm package-level global, không thêm import mới — `go test -count=1 ./internal/archtest/` exit 0 (6.5).

### 6.2 Hai điều §1 bắt "đo trước khi viết assertion" — đo bằng code, không đoán

1. **Indicator có tồn tại ở chỗ user nhìn thấy, và nay có node AX:** `goosie.zoom`, value là đúng
   nhãn whole-percent badge paint (`150%`, `100%`), rect nằm trong hàng toolbar.
2. **Cmd+0 KHÔNG ẩn badge.** §1 và prompt đều giả định "sau reset thì badge biến mất"; đo lại:
   `onZoom(0)` → `framePath.handleZoom(0)` (`cmd/goosie/interaction.go:675-688`) đặt zoom 1.0 rồi gọi
   `toolbar.SetZoom(1.0)` (`internal/toolbar/toolbar.go:275`) — tức **nối lại** grace 2.5s.
   `ResetZoom()` (ẩn ngay, `:283`) chỉ chạy khi bấm vào badge (`:749`, wire tại `cmd/goosie/main.go:409`).
   Nên BR-131 assert vế product thật hứa: **node tồn tại với `100%`**; hai vế ẩn (hết grace, và
   `Zoom <= 0`) được đóng đinh không cần đồng hồ trong `test/toolbar/zoom_badge_ax_test.go`. Nếu QC
   muốn Cmd+0 ẩn indicator ngay, đó là **quyết định product**, không phải việc của batch test này —
   ghi lại để integrator quyết.

### 6.3 Oracle mới trong `test/` (không cần display) — pin rule chung cho paint và AX

`test/toolbar/zoom_badge_ax_test.go`, tag `// qc:BR-129` và `// qc:BR-131`. Mỗi case đo **cả hai phía**:
API trả gì, và `Draw` thật sơn gì (đếm pixel đúng màu pill `frame.RGB(232,240,254)`):

* **125%** ⇒ `shown = true`, `label == ZoomPercent() == "125%"`, rect nằm trong `AddressBarRect`, và
  **bbox của pixel badge bằng đúng rect × scale** — số đo thật: `sc=1` rect `(712,8) 40x24` với
  **800 px** ink; `sc=2` ink `(1424,16) 80x48` với **3268 px**; yêu cầu ink ≥ 1/2 diện tích pill
  (đo được 83–85%). Đây là chỗ AX-vs-pixel không thể lệch mà không đỏ.
* **100% sau grace** (`ResetZoom`) ⇒ `shown = false` **và 0 px** badge.
* **100% trong grace** (`SetZoom(1.0)`) ⇒ `shown = true` **và có px** — vế ngược của rule.
* **Find đang mở ở 125%** ⇒ `shown = false` và 0 px.
* **`Zoom = 0`** ⇒ `shown = false` và 0 px.

### 6.4 Journey: hai assertion đã unconditional

* Helper mới `awaitZoomNode` (`cmd/goosie/journey_p1_chrome_zoom_ax_test.go:71`): poll **giá trị đã
  publish trong cây AX** cho tới khi khớp nhãn, hết hạn thì FAIL theo hai nhánh tách bạch — "không có
  node nào được publish" vs "node publish giá trị khác" — kèm nhãn toolbar đang paint và
  `b.diagnostics()`. Cây AX chỉ dựng lại theo frame được present (`cmd/goosie/presentation.go:185`),
  nên poll giá trị đã publish là thứ chống race; không đếm frame, không `time.Sleep`, không assert duration.
* BR-129 `:314` — unconditional: value `150%`, `role == ax.RoleMeter`, hộp chữ nhật khả kiến nằm
  trong hàng toolbar, và value == `ZoomPercent()`.
* BR-131 `:383` — unconditional: sau Cmd+0 node phải có value `100%` (kèm `ZoomPercent() == "100%"`
  đã có ở `:378`), lý do đo được ghi thẳng trong comment của test.
* `grep -n 'if _, ok := findAXNode.*goosie.zoom' cmd/goosie/journey_p1_chrome_zoom_ax_test.go` →
  **0 kết quả, exit 1** (grep trả exit 1 khi không khớp) — hai guard vacuous đã biến mất. Tìm `findAXNode`
  nói chung còn **12 khớp** (`grep -c findAXNode` → 12), trong đó duy nhất một chỗ dùng `ok` để **bỏ qua** là
  vòng poll của `awaitZoomNode` (`:80`, mọi nhánh thoát bằng `t.Fatalf`); `:49` là helper `toolbarNode`
  `t.Fatalf` khi node vắng; `:219` và `:235` là `if _, ok := ...; !ok { t.Fatal(...) }` của test Stop đã có
  trước — chúng **FAIL khi node vắng mặt**, tức assertion một chiều; 7 chỗ còn lại là vị từ của `waitFor`
  (bản thân `waitFor` `t.Fatalf` khi hết hạn). Không còn guard nào cho phép assertion bị nhảy.

### 6.5 Mutation `[R-7]` — tắt chính hành vi, bốn lần, mỗi lần revert ngay

| # | Mutation (tắt gì) | Kết quả |
| --- | --- | --- |
| (i) | `cmd/goosie/toolbar_window.go`: value của node hard-code `"100%"` thay vì `zoomValue` | BR-129 **FAIL**, exit 1 |
| (ii) | `cmd/goosie/toolbar_window.go`: xoá hẳn khối phát node `goosie.zoom` | BR-129 **và** BR-131 **FAIL**, exit 1 — đây là bằng chứng vacuity đã biến mất (trước batch này, mutation y hệt vẫn xanh) |
| (iii-a) | `cmd/goosie/toolbar_window.go`: đảo rule — `if _, _, shown := ...; !shown { publish }` | BR-129 **và** BR-131 **FAIL**, exit 1 |
| (iii-b) | `internal/toolbar/toolbar.go`: AX **nhân bản** rule kiểu bỏ nhánh grace (`ok = ok && s.Zoom != 1.0`) — AX ẩn node khi pixel vẫn sơn | BR-131 **FAIL**, exit 1, trong khi BR-129 vẫn xanh (oracle cô lập đúng vế reset); oracle `test/toolbar` **FAIL**, exit 1 |

Output nguyên văn:

```
# (i) value hard-code "100%" — exit 1
--- FAIL: TestJourneyZoomInGrowsContentAndDisplaysPercentage (8.18s)
    journey_p1_chrome_zoom_ax_test.go:314: zoom AX node published value "100%", want "150%" (the badge paints "150%"); url="http://127.0.0.1:55089" loading=false error="" history=[http://127.0.0.1:55089] nameAX=(false,{Role:0 ID: Label: Value: Href: Selected:false Disabled:false AddressBar:false Focused:false X0:0 Y0:0 X1:0 Y1:0 Children:[]}) ime=false presents=487 injected=7
FAIL
FAIL	github.com/vyquocvu/goosie/cmd/goosie	8.924s
FAIL
```

```
# (ii) xoá emission của goosie.zoom — exit 1 (proves the vacuity is gone)
--- FAIL: TestJourneyZoomInGrowsContentAndDisplaysPercentage (8.18s)
    journey_p1_chrome_zoom_ax_test.go:314: no goosie.zoom AX node was published within 8s, want value "150%" (the badge paints "150%"); url="http://127.0.0.1:55098" loading=false error="" history=[http://127.0.0.1:55098] nameAX=(false,{Role:0 ID: Label: Value: Href: Selected:false Disabled:false AddressBar:false Focused:false X0:0 Y0:0 X1:0 Y1:0 Children:[]}) ime=false presents=484 injected=7
--- FAIL: TestJourneyCmdZeroResetsZoomAndKeepsAXGeometryValid (8.05s)
    journey_p1_chrome_zoom_ax_test.go:383: no goosie.zoom AX node was published within 8s, want value "100%" (the badge paints "100%"); url="http://127.0.0.1:55106" loading=false error="" history=[http://127.0.0.1:55106] nameAX=(false,{Role:0 ID: Label: Value: Href: Selected:false Disabled:false AddressBar:false Focused:false X0:0 Y0:0 X1:0 Y1:0 Children:[]}) ime=false presents=480 injected=6
FAIL
FAIL	github.com/vyquocvu/goosie/cmd/goosie	16.991s
FAIL
```

```
# (iii-a) đảo rule tại chỗ phát node (!shown) — exit 1
--- FAIL: TestJourneyZoomInGrowsContentAndDisplaysPercentage (8.18s)
    journey_p1_chrome_zoom_ax_test.go:314: no goosie.zoom AX node was published within 8s, want value "150%" (the badge paints "150%"); url="http://127.0.0.1:55118" loading=false error="" history=[http://127.0.0.1:55118] nameAX=(false,{Role:0 ID: Label: Value: Href: Selected:false Disabled:false AddressBar:false Focused:false X0:0 Y0:0 X1:0 Y1:0 Children:[]}) ime=false presents=486 injected=7
--- FAIL: TestJourneyCmdZeroResetsZoomAndKeepsAXGeometryValid (8.04s)
    journey_p1_chrome_zoom_ax_test.go:383: no goosie.zoom AX node was published within 8s, want value "100%" (the badge paints "100%"); url="http://127.0.0.1:55128" loading=false error="" history=[http://127.0.0.1:55128] nameAX=(false,{Role:0 ID: Label: Value: Href: Selected:false Disabled:false AddressBar:false Focused:false X0:0 Y0:0 X1:0 Y1:0 Children:[]}) ime=false presents=478 injected=6
FAIL
FAIL	github.com/vyquocvu/goosie/cmd/goosie	16.975s
FAIL
```

```
# (iii-b) ZoomBadge tự restating rule, bỏ nhánh grace — exit 1
# cmd/goosie (chỉ BR-131 chết; BR-129 vẫn xanh => oracle vế reset là độc lập)
--- FAIL: TestJourneyCmdZeroResetsZoomAndKeepsAXGeometryValid (8.06s)
    journey_p1_chrome_zoom_ax_test.go:383: no goosie.zoom AX node was published within 8s, want value "100%" (the badge paints "100%"); url="http://127.0.0.1:55424" loading=false error="" history=[http://127.0.0.1:55424] nameAX=(false,{Role:0 ID: Label: Value: Href: Selected:false Disabled:false AddressBar:false Focused:false X0:0 Y0:0 X1:0 Y1:0 Children:[]}) ime=false presents=471 injected=6
FAIL
FAIL	github.com/vyquocvu/goosie/cmd/goosie	12.219s
FAIL

# test/toolbar (oracle không cần display cũng chết)
--- FAIL: TestZoomBadgeHiddenExactlyWhenTheBadgeIsNotPainted (0.00s)
    zoom_badge_ax_test.go:126: ZoomBadge shown = false at 100% inside the grace, want exposed; label=""
FAIL
FAIL	github.com/vyquocvu/goosie/test/toolbar	1.267s
FAIL
```

**Một vòng tự sửa trước khi báo cáo:** lần chạy (i) đầu tiên **giết** test nhưng chỉ bằng
`waitFor: timed out waiting for AX publishes the zoom badge as 150%` — oracle lúc đó còn ẩn giá trị
trong điều kiện wait, nên chữ "stale value" không hiện ra. Đã thay bằng `awaitZoomNode` (6.4) để
thiếu node và node sai giá trị cho hai thông báo khác nhau; output (i) ở trên là sau khi củng cố.

### 6.6 Lệnh và exit code, worktree này, sau khi merge `main`

| Lệnh | Exit | Ghi |
| --- | --- | --- |
| `git merge main` | 0 | fast-forward `4de0fca` → `6071b8b` (branch chưa có commit nào; golden được stash rồi regenerate nên không conflict) |
| `go test -count=1 ./test/qc/ -update` | 0 | golden chỉ đổi 6 row (6.7) |
| `go test -count=1 ./...` | 0 | 42 package `ok`, 0 dòng FAIL |
| `go vet -all ./...` | 0 | im lặng, kể cả `internal/platform/darwin` (cgo build) |
| `go test -count=1 ./internal/archtest/` | 0 | import table, cgo, gofmt, globals |
| `go test ./test/gate -run TestGate -v` | 0 | 24.1s; không đổi bộ đếm gate |
| `go test -count=1 -race ./cmd/goosie` | 0 | 22.9s |
| `go test -count=3 ./cmd/goosie -run 'TestJourneyZoom...|TestJourneyCmdZero...'` | 0 | journeys zoom: 0.18s / 0.13s / 0.05s |
| `go test -count=20 ./cmd/goosie -run <hai journey zoom>` | 0 | 5.0s — kiểm vế `100% trong grace` không phụ thuộc đồng hồ (assertion chạy trong ~50ms) |
| `go test -count=1 ./test/toolbar/ -run TestZoomBadge -v` | 0 | 2 oracle mới |
| `gofmt -l <bốn file Go thay đổi>` | 0 | in ra 0 dòng |
| re-check sau khi sửa §6.4 (chỉ sửa doc, code giữ nguyên): `go test -count=1 ./cmd/goosie -run 'TestJourneyZoomInGrowsContentAndDisplaysPercentage'` và `-run 'TestJourneyCmdZeroResetsZoomAndKeepsAXGeometryValid'`, `./test/toolbar -run TestZoomBadge` | 0 / 0 / 0 | 0.18s / 0.05s / 0.31s |

`uptime` lúc chạy suite: load 1-minute 5.19 → 9.83, dưới ngưỡng 30 của `GOOSIE-FLAKE-001`.

### 6.7 `test/qc/traceability.golden.json` (integrator review bắt buộc)

Regenerate bằng `-update` (không tay sửa). `git diff --stat`: **20 insertions(+), 6 deletions(-)**,
và **đúng 6 row** đổi `"tests": null` → danh sách test:

* BR-126 ← `cmd/goosie/journey_p1_chrome_zoom_ax_test.go:TestJourneyBackForwardDisabledStateAndClicks`
* BR-127 ← `...:TestJourneyStopLoadingAndCompletedStopNoOp`
* BR-128 ← `...:TestJourneyReloadRestoresReloadLabelAndRequestsExactlyOnce`
* BR-129 ← `...:TestJourneyZoomInGrowsContentAndDisplaysPercentage` + `test/toolbar/zoom_badge_ax_test.go:TestZoomBadgeExposesTheBadgeTheToolbarPaints`
* BR-130 ← `...:TestJourneyZoomOutClampsAndStillAcceptsPageDown`
* BR-131 ← `...:TestJourneyCmdZeroResetsZoomAndKeepsAXGeometryValid` + `test/toolbar/zoom_badge_ax_test.go:TestZoomBadgeHiddenExactlyWhenTheBadgeIsNotPainted`

`status` vẫn `NOT RUN` cho cả 6 (chưa sign-off), `enforced` không đổi. Không đụng
`docs/qa-test-catalog.md`, `test/qc/catalog.go`, `docs/qa-open-defects.json`, `docs/qa-ratchet.json`,
`testdata/render-known-gaps.json`, `testdata/parity.py`.

### 6.8 Còn mở / chưa gate được (khai báo)

1. **Node chỉ mới bằng tốc độ present.** Cây AX dựng lại trong `publishAccessibility`
   (`cmd/goosie/presentation.go:185`), nên badge hết hạn theo đồng hồ (100% sau grace 2.5s) vẫn có thể
   ngồi trong **tree đã publish cuối** cho tới frame kế tiếp. Hành vi này có từ trước, thuộc cả
   document tree chứ riêng gì zoom. Không có assertion nào cho nó vì sẽ là assert duration;
   `test/toolbar/zoom_badge_ax_test.go` đóng đinh rule ở tầng API+pixel thay thế.
2. **Vế "Cmd+0 ẩn indicator" không tồn tại trong product** (6.2.2) — BR-131 nhân nhượng theo đo lường,
   không theo catalog. Đề xuất cho integrator: hoặc sửa product (`handleZoom(0)` gọi `ResetZoom()` khi
   delta == 0), hoặc viết lại catalog cho BR-131 thành "badge chỉ 100% trong 2.5s sau reset".
3. **Không có pixel thay đổi, nên không chạy parity.** `git diff internal/toolbar/toolbar.go` là
   **thêm thuần** (25 dòng, một method mới) — `drawAddressBar`/`zoomBadgeRect` không đổi một dòng nào,
   rect và nhãn badge paint vẫn như cũ; `test/gate` và corpus không đổi số đo. Cộng với ràng buộc của
   prompt (3 agent khác đang chạy, cấm `testdata/parity.py --render` và `live-parity.py`), không có
   measurement parity mới trong batch này. Oracle hình ảnh của badge: `cmd/goosie/find_and_zoom_test.go:116`
   (đỏ nếu badge không sơn) + bbox-check mới ở 6.3.
4. **BR-126/127/128/130 giữ nguyên như agent trước viết**; batch này chỉ tái-đo sau merge (xanh, 6.6)
   và không nới helper nào của `e2e_test.go`.
5. **`GOOSIE-TRACE-001`:** 6 row giờ có test refs trong golden artifact; việc đóng defect và đổi
   `status` trong catalog vẫn là quyết định của integrator.

