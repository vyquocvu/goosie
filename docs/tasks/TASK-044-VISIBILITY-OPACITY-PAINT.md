# TASK-044 — `visibility` và `opacity` của box không vào được display list (GOOSIE-VIS-001)

**Priority:** P1. **Method:** INT (oracle ở biên style→paint trong `test/`) + PARITY (fixture mới trong `testdata/render/css-features/`).
**Owner:** agent được giao (nhánh `worktree-task-044-visibility-opacity`). **Defect:** `GOOSIE-VIS-001`.
**Base:** `main` tại `6071b8b` (đã gồm TASK-040: tokenizer U+002D, message từ chối hình học, một nhãn `goosie:` mỗi dòng).

## 0. Phạm vi và quy tắc làm việc trong nhánh này

- **Không sửa** `docs/qa-test-catalog.md`, `docs/qa-open-defects.json`, `docs/qa-ratchet.json`,
  `test/qc/catalog.go`, `test/qc/traceability.golden.json`, `testdata/render-known-gaps.json`.
  Registry QC và known-gaps là của integrator; hãy ghi BR id bạn *đề nghị* cùng danh sách oracle vào §5,
  integrator sẽ tag `// qc:BR-xxx` và chạy `-update`. Nếu điểm số fixture nào đó dịch chuyển, **báo lại số đo**,
  không tự re-baseline known gaps.
- `testdata/parity.py` chỉ được đụng tới dòng `MIN_SCORED_FIXTURES`, và chỉ theo quy tắc: thêm fixture thì +1
  ngay trong commit thêm fixture. Trên base này giá trị là **85** (main có `css-features/11-digit-leading-custom-property.html`).
  Muốn đổi hành vi harness thì mở task khác.
- **Không** `git push`. Mỗi claim trong §5 phải kèm lệnh + exit code + số đo thật trên máy bạn.
- Trước khi sửa, chạy lại §1 và §2 trên máy của bạn; nếu số đo khác, **ghi số của bạn** và giải thích, đừng copy số ở đây.

## 1. Bằng chứng đã đo (integrator, 2026-10-26, head `6071b8b`)

### 1.1 Bốn box, một kết luận

`/tmp/vis-probe2.html` — `.ov{opacity:0;visibility:hidden}`, `.vx{visibility:hidden}`, `.op{opacity:0}`, `.dp{display:none}`,
mỗi box `width:100px;height:50px` (`.ov` là `position:absolute;width:100%;height:200px`), chạy `go run ./cmd/dump-paint`:

```
EXTENT Rect(0,0 800x200)
FILL rect=(0,0,800,200) color=4160223231
FILL rect=(8,8,108,58) color=4278190335
FILL rect=(8,58,108,108) color=16711935
```

Ba `FILL` đầu là `.ov`, `.vx`, `.op`; **chỉ** `.dp` biến mất. Màu `16711935 = 0x00FF00FF` tức green với **alpha 255**,
nên `opacity:0` không những không làm mất lệnh vẽ mà còn vào lệnh vẽ với alpha đầy đủ.

Đo pixel (400×300, `.ov` nền `#f7f7f7`): **79.509 px** `#f7f7f7` ở **cả bốn** biến thể
(`opacity:0`+`visibility:hidden`, chỉ `visibility:hidden`, chỉ `opacity:0`, và không khai báo gì) —
400×200 = 80.000 trừ 4 px bo góc. Ảnh vẽ của box bị "ẩn" giống hệt ảnh của box hiện.

### 1.2 Đọc code: hai tầng hỏng **khác nhau**, đừng sửa một chỗ rồi tưởng xong

| Tầng | Bằng chứng | Hệ quả |
| --- | --- | --- |
| `style` lưu giá trị | `internal/style/style.go:2559` `case "visibility": cs.Visibility = value`; `:2796-2803` `case "opacity": cs.Opacity = float32(parsed.Num)`; mặc định `Visibility: "visible"` (`:679`), `Opacity: 1.0` (`:655`) | Cascade **có** giữ cả hai field; `internal/engine/style_diff.go:20,101` cũng trở `Visibility`/`ContentVisibility` qua boundary |
| `paint` dùng giá trị | `internal/paint/builder_box.go:63` `opacity: obj.Style.Opacity`; `internal/paint/builder_controls.go:466` là **nơi duy nhất** trong tree đọc `s.Visibility` (chỉ cho list marker) | `visibility:hidden` **không có** nhánh nào cho box/text → luôn vẽ. `opacity` **có** nhánh, nhưng display list ở §1.1 vẫn emit alpha 255 → hoặc `cs.Opacity` không phải 0 như cascade promise, hoặc `parsed.Num` cho value `"0"` không phải số, hoặc builder đọc field khác |

Việc đầu tiên của task: **xác định opacity hỏng ở tầng nào** bằng một oracle cấp `test/` (không đoán từ đọc code),
rồi sửa đúng tầng đó. Ghi rõ kết luận vào §5.

### 1.3 Vì sao đây là P1 chứ không phải "thiếu tính năng dễ thương"

Sweep live `docs/qa-live-parity/2026-10-25T1952Z-2ca78897.md`: vnexpress rơi từ `LOW_COVERAGE`
(coverage 0.441, ink 320.348, doc_height 1625) xuống **`BLANK_OUTPUT`** (coverage 0.0, ink 32, doc_height 691).
Thủ phạm là rule `.wrap-all-menu{width:100%;float:left;background:#f7f7f7;position:absolute;opacity:0;visibility:hidden}`
— trước đây cả sheet bị cắt nên rule này không tồn tại; sau GOOSIE-CSSSTRING-003 (`e3b8292`) nó được nhận
và **vẽ một tấm xám phủ toàn trang**. Attributed bằng đo lường trên một bản lưu của trang
(254.146 ký tự, render cả bản có và bản bỏ `<script>`, ba binary `eef92ae` / `72a11f8` / `2ca7889`):
PNG 316.893 / 224.125 / 4.837 byte, và bản bỏ script cho ink 261.030 / 261.030 / 345.600 —
hai cột đầu **giống nhau từng byte**, nên loại trừ TASK-039 (script chạy thật), và 345.600 px chính là tấm `#f7f7f7`.

**Không revert CSS fix**: trên đúng chín sheet vnexpress, parser mới cho **882 rule** chỗ parser cũ cho **174**,
và marker 120×120 nối cuối sheet 94 KB sống trên main (14.400 ink px) nhưng mất ở binary trước fix (0).
Phía user-visible thiếu là semantics ẩn, tức task này.

## 2. Phép đo bắt buộc **trước** khi sửa

1. `go run ./cmd/dump-paint <probe của riêng bạn>` với bốn biến thể như §1.1 → dán output thật.
2. Một oracle chứng minh `style.ComputedStyle.Opacity` bằng bao nhiêu khi stylesheet khai `opacity:0`,
   `opacity:0.5`, `opacity:.5`, `opacity:5` (trần hữu hiệu là 1), `opacity:0%`? Chromium cho giá trị nào
   (`testdata/parity.py` không so style, nên dùng Playwright/`page.evaluate(getComputedStyle(...).opacity)`)
   — **đo**, đừng lấy từ trí nhớ.
3. Chromium hành xử với `visibility:hidden` trên **con**: `.parent{visibility:hidden}.child{visibility:visible}`
   phải hiện lại. Đo cùng document để biết acceptance đúng là gì.
4. Ghi lại điểm số parity hiện tại của toàn corpus (`python3 testdata/parity.py --render --binary ./bin/goosie`)
   **trước** khi sửa, để biết mình vừa dịch chuyển gì.

## 3. Plan

1. **visibility**: box có `Visibility != "visible"` (và `hidden` của `.ov` kế thừa xuống subtree theo đúng quy tắc
   kế thừa `visible → hidden`, con khai `visible` thì hiện lại) phải **không sinh paint command** cho background,
   border, text và marker của chính nó, nhưng **vẫn chiếm chỗ trong layout** (khác `display:none`).
   `collapse` (table) nằm ngoài phạm vi — ghi rõ là chưa làm nếu bạn không làm.
2. **opacity**: sửa đúng tầng tìm được ở §1.2 để `cs.Opacity` xuống tới `boxPaintContext.opacity`,
   và áp cho **cả** background, border, text của element cùng subtree. Nếu đường tile hiện tại chỉ nhân alpha
   per-command (không group-composite), thì khai báo trung thực: overlapping children trong cùng group sẽ
   cho kết quả khác Chromium; ghi giới hạn đó vào §5 và defect, **không** giấu.
3. **ContentVisibility** (`style.go:2561`) đang có field mà không có consumer: để nguyên hoặc ghi rõ chưa dùng;
   đừng nhân tiện "tiện tay" bật nó.

## 4. Điều kiện nhận (không cái nào là "test chạy xanh")

1. Unit/integration oracle cho **cả hai** cơ chế, đặt ở `test/` (không phải test nội bộ package vẽ), với document tối giản
   và kỳ vọng lấy từ phép đo §2 (Chromium là chuẩn). Ít nhất: `visibility:hidden` trên box nền; subtree hidden;
   con tự `visible` hiện lại; `opacity:0` không vẽ; `opacity:0.5` vẽ đúng alpha pha; `display:none` vẫn không chiếm chỗ
   (để chứng minh hai cơ chế không lẫn vào nhau).
2. Fixture parity mới `testdata/render/css-features/12-hidden-and-transparent.html` **hai chiều**: có nội dung thật
   phía trên và phía dưới fold, có một overlay `opacity:0;visibility:hidden` (dạng đã giết vnexpress), có `.child{visibility:visible}`
   trong chain bị ẩn, có `opacity:0.5` chồng chữ. Fixture phải **chấm được** (`--render --render-chromium`, PASS ≥ 90)
   và phải FAIL khi mutation — xem (4).
3. `go test -count=1 ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `make gate` xanh trên máy bạn;
   `python3 testdata/parity.py --render --render-chromium --binary ./bin/goosie` báo **86/86** (floor đã +1) với
   average match **không giảm quá 0,2 điểm** so với con số bạn đo ở §2.4. Nếu known gaps (5 fixtures / 10 bands) dịch,
   **dừng và báo** — đó là file của integrator.
4. **Mutation tự chứng minh, ba phép**, mỗi phép phải làm test tương ứng chết và để lại output verbatim trong §5:
   (i) `Visibility` bị builder bỏ qua (hard-code `"visible"` ở điểm đọc) → oracle visibility chết;
   (ii) `opacity` bị hard-code 1.0 ở `boxPaintContext` → oracle opacity chết;
   (iii) fixture-level: revert một trong hai, render lại fixture mới bằng binary đó, chấm điểm và dán
   `PASS/FAIL`, `match_pct`, `blank_regions`, exit code của parity. Nếu fixture vẫn PASS sau khi revert,
   nó không phải gate — viết lại fixture.
5. Số đo live, không bắt buộc mạng nhưng bắt buộc trung thực: render lại **một bản lưu** của vnexpress
   (`curl -sL --compressed -A 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Goosie/2.0' https://vnexpress.net/ -o /tmp/vn.html`)
   bằng binary trước và sau sửa; báo cáo ink và doc_height của cả hai. Nếu không cải thiện được vì **nhiều** nguyên nhân
   khác chồng lên (script, image, layout), nói rõ phần nào task này sửa được và phần nào còn lại — đừng khẳng định
   "vnexpress đã xanh" nếu sweep tiếp theo nói khác.
6. Commit sạch: code + test + fixture + §5 vào nhánh, **không** đụng registry; pull `main` vào nhánh trước khi báo
   hoàn thành và chạy lại toàn bộ (4)–(5) sau merge.

## 5. Kết quả — agent điền

(chưa chạy; điền theo mục 1–6 của §4, kèm lệnh + exit code + số đo.)

## 6. Ngoài phạm vi

- `mix-blend-mode`, `filter`, stacking context đầy đủ, `will-change`, group compositing thật sự (kể cả
  `isolation`), `contain: paint`, `content-visibility` auto/hidden semantics.
- Selector matching với escape (`GOOSIE-CSSSELECT-001`) và dispatch `+`/`.` trong tokenizer (ghi ở `GOOSIE-LIVE-002`).
- Việc harness dùng `/tmp/*-renders` chung (`GOOSIE-PARITYTMP-001`, gồm hệ quả "PNG cũ còn đó nên render rc 1 vẫn chấm 100%").
