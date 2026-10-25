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

Tất cả số đo dưới đây lấy trên máy này (arm64, go1.26.10), HEAD base `6071b8b`,
nhánh `worktree-task-044-visibility-opacity`. Mỗi mục có lệnh + exit code + output thật.

### 5.0 Opacity hỏng ở tầng nào — xác định bằng đo, không đoán từ đọc code

Đo tại seam style→paint (dựng document thật, chạy `Session.PaintChecked(1.0)` →
`dl.Build(1)` → đọc `paint.DisplayCmd.Opacity/Color`, và `style.ResolveViewport` →
`ComputedStyle.Opacity`). Kết luận: **cả hai tầng đều hỏng, và hỏng khác nhau**.

- Tầng `style`: `opacity:5` được lưu nguyên vẹn thành `5.0` (không clamp), và
  giá trị parse lỗi rơi về `0` — sau khi tầng paint bỏ hộp khi opacity 0, một typo
  sẽ làm biến mất cả phần tử. (Oracle `TestOpacityClampsToUnitRange` chốt lại.)
- Tầng `paint`/`raster`: `opacity:0` đi vào display list với `DisplayCmd.Opacity==0`,
  mà raster đọc `0` là sentinel "unset → opaque" (`cmd.go`, `raster/tile.go:applyOpacity`),
  nên hộp "trong suốt" vẽ nguyên đậm (alpha 255, thấy ở §5.1); và opacity của cha
  không nhân xuống con nên subtree không bị mờ.

Sửa đúng cả hai tầng: clamp + giữ nguyên giá trị lỗi ở `style`; tích luỹ opacity hiệu
hiệu theo cây, bỏ hộp khi hiệu ≤ 0, và bỏ vẽ chính hộp khi `Visibility != "visible"`
(vẫn đi tiếp con) ở `paint`.

### 5.1 Bốn hộp — trước và sau, bằng `cmd/dump-paint` trên probe của riêng tôi

Probe `/tmp/vis-probe044.html`: `.ov{opacity:0;visibility:hidden;background:#f7f7f7;
position:absolute;width:100%;height:200px}`, `.vx{visibility:hidden;background:blue}`,
`.op{opacity:0;background:green}`, `.dp{display:none;background:red}` (màu named:
`green` = `#008000`).

TRƯỚC (binary HEAD pristine, dựng bằng `git stash push -u` rồi `go build`):

```
$ /tmp/dumppaint-before /tmp/vis-probe044.html   # exit 0
EXTENT Rect(8,8 800x200)
FILL rect=(8,8,808,208) color=4160223231
FILL rect=(8,8,108,58) color=65535
FILL rect=(8,58,108,108) color=8388863
```

Định dạng màu là `0xRRGGBBAA`: `4160223231=0xF7F7F7FF` (xám opacity 0, alpha 255),
`65535=0x0000FFFF` (xanh dương visibility:hidden, alpha 255), `8388863=0x008000FF`
(xanh lá opacity:0, alpha 255). Ba hộp `.ov/.vx/.op` **đều vẽ**, chỉ `.dp` (display:none)
biến mất — khớp kết luận §1.1 (số nguyên màu khác bản doc vì tôi dùng named color và
viewport của tôi có lề 8px, nhưng hiện tượng giống hệt: opacity:0 vào lệnh vẽ với alpha đầy đủ).

SAU (binary đã sửa):

```
$ /tmp/dumppaint-after /tmp/vis-probe044.html    # exit 0
EXTENT Rect(0,0 0x0)
```

Không còn FILL nào — cả bốn hộp đúng: `.ov`/`.op` bị bỏ vì opacity hiệu dụng 0, `.vx`
bị bỏ vì visibility hidden, `.dp` vẫn collapse.

### 5.2 Oracle opacity cấp `style` — Chromium là chuẩn (đo, không nhớ)

`node ./tmp-chromium-oracle.js` (Playwright `getComputedStyle(...).opacity`) exit 0:

```
a(opacity:0)=0  b(0.5)=0.5  c(.5)=0.5  d(5)=1  e(-3)=0
f(0%)=0  g(50%)=0.5  h(100%)=1  bad(opacity:oops)=1
```

Goosie sau sửa khớp từng giá trị một (`TestOpacityClampsToUnitRange` xanh): `5`→`1`,
`-3`→`0`, `0%`→`0`, `50%`→`0.5`, và **giá trị lỗi phải ra `1`** chứ không được rơi về `0`
(vì `0` giờ bỏ cả subtree).

### 5.3 Oracle visibility kế thừa/trả lại — cùng document với §5.2

Cũng từ `tmp-chromium-oracle.js` exit 0:

```
.p{visibility:hidden} -> p=hidden  silent(inherit)=hidden
.shown{visibility:visible} -> shown=visible  grand=visible
```

Goosie sau sửa khớp (`TestVisibilityInheritsAndRestores` xanh): hidden kế xuống con,
con khai `visible` hiện lại và truyền `visible` xuống cháu.

### 5.4 Điểm parity TRƯỚC khi sửa (§2.4) và SAU khi sửa (§4.3)

- TRƯỚC: `python3 testdata/parity.py --render --binary /tmp/goosie-before` (exit 1 vì
  chính fixture mới chưa qua), toàn corpus 86 fixture avg **96.03%**. Trong 85 fixture
  cũ (không tính fixture mới của tôi) mức trung bình ≈ **96.17%**.
- SAU: `python3 testdata/parity.py --render --render-chromium --binary ./bin/goosie`
  exit 0:

```
  86/86 passing  (100.0%), 0 failing, 0 errors
  average match: 96.19%  (threshold=30, pass_at=90.0%)
  known gaps: 5 fixtures / 10 bands
   98.07%  PASS  css-features/12-hidden-and-transparent.png  worst_region_iou=0.99796...  coverage_ratio=0.98954...  blank_regions=0/0  shifted=0
```

Diff điểm **từng fixture** giữ nguyên so với binary trước (85 fixture chung chỉ 3 cái
dịch, tất cả vẫn PASS, biên độ ≤0.18đ): `colors/03-opacity.png 99.29→99.47` (chính
fix opacity làm fixture opacity sẵn có **tốt lên**), `layout-features/04-positioning
97.76→97.83`, `semantic/02-article-section 95.8→95.76`. Average toàn corpus **tăng**
96.03→96.19 (nhờ fixture mới 84.42→98.07); **không giảm quá 0.2đ** so với baseline —
giảm 0,00 trên tập 85 cũ. **known gaps không dịch: vẫn 5 fixtures / 10 bands** (file
integrator; tôi không đụng và không re-baseline).

### 5.5 Ba phép mutation tự chứng minh (output verbatim)

(i) Builder bỏ qua `Visibility` (hard-code `visibilityHidden := false`):

```
$ go test -count=1 ./test/paint/ -run Visibility   # exit 1
--- FAIL: TestVisibilityHiddenDropsBoxPaint (0.00s)
    visibility_paint_test.go:60: visibility:hidden background painted 4 fill(s); want 0
    visibility_paint_test.go:64: visibility:hidden box emitted a border command at Rect(0,0 128x68); want none
    visibility_paint_test.go:69: visibility:hidden box painted its text; want none
--- FAIL: TestVisibilityHiddenSubtreeInheritsHidden (0.00s)
    visibility_paint_test.go:96: hidden parent background painted 1 fill(s); want 0
    visibility_paint_test.go:99: child that inherits hidden painted 1 fill(s); want 0
--- FAIL: TestVisibilityHiddenKeepsLayout (0.00s)
    visibility_paint_test.go:124: hidden box emitted 1 fill(s); want 0
FAIL
```

(ii) Hard-code `opacity: 1` trong `boxPaintContext`:

```
$ go test -count=1 ./test/paint/ -run Opacity   # exit 1
--- FAIL: TestOpacityFractionReachesCommand (0.00s)
    opacity_paint_test.go:65: opacity:0.5 carried 1 in the display command, want 0.5
    opacity_paint_test.go:68: opacity:.5 carried 1 in the display command, want 0.5
--- FAIL: TestGroupOpacityAppliesToSubtree (0.00s)
    opacity_paint_test.go:109: opacity:0.5 parent carried 1, want 0.5
    opacity_paint_test.go:112: opaque child of an opacity:0.5 parent carried 1, want 0.5 (group opacity applies to the subtree)
FAIL
```

(iii) Fixture-level, revert **một** cơ chế (opacity), render lại fixture bằng binary đó và chấm:

```
$ /tmp/goosie-mut3 -backend headless -url file:///.../12-hidden-and-transparent.html -screenshot ...   # exit 0
$ python3 testdata/parity.py --binary ./bin/goosie   # exit 1
   84.42%  FAIL  css-features/12-hidden-and-transparent.png  worst_region_iou=0.0  coverage_ratio=0.0  blank_regions=8/0  rows=[(13, 32), (43, 56), (69, 108), (119, 154), (193, 260), (307, 320), (333, 372), (387, 400)]  shifted=0
  85/86 passing  (98.8%), 1 failing, 0 errors
```

Revert opacity làm tấm `opacity:0` phủ toàn viewport, coverage_ratio=0.0 (mất hết ink),
exit 1. Revert visibility cũng vậy (tấm `visibility:hidden`). **Mỗi cơ chế một mình là
gate thật** — nếu fixture vẫn PASS sau revert thì nó không chặn được gì, và ở đây nó FAIL.

### 5.6 vnexpress trước/sau trên một bản lưu (không cải thiện được vì nhiều nguyên nhân)

Bản lưu giữ ngoài repo: `/tmp/vn-task044.html` (`curl -sL --compressed -A 'Mozilla/5.0
… Goosie/2.0' https://vnexpress.net/ -o /tmp/vn.html`, snapshot offline). Render bằng
`/tmp/goosie-before` (HEAD) và `bin/goosie` (sau sửa):

```
$ goosie-before -bench -frames 3 -url file:///tmp/vn-task044.html -width 800 -height 600 -dpr 1 -out /tmp/vn-before-rep.json   # exit 0 → report.doc_height
$ goosie        -bench -frames 3 -url file:///tmp/vn-task044.html -width 800 -height 600 -dpr 1 -out /tmp/vn-after-rep.json    # exit 0
before doc_height=706
after  doc_height=526
```

Đếm ink trên PNG 800×1600 (`-screenshot`, PIL, ink = kênh < 245):

```
before: 800x1600 ink=0 #f7f7f7=216000 white=1064000
after : 800x1600 ink=0 #f7f7f7=216000 white=1064000
```

**Trung thực:** trên snapshot offline này, task này **không** làm vnexpress hết trắng.
- `doc_height` giảm 706→526 vì tấm overlay `opacity:0` (position:absolute, cao) vốn bị
  vẽ trước đây giờ không còn góp vào paint extent — đúng cơ chế, nhưng không đủ để hiện nội dung.
- `ink` vẫn 0 ở **cả hai**: vùng `#f7f7f7` 216.000px = 800×270 đúng bằng header hợp lệ
  `min-height:270px` (không phải overlay opacity:0), còn lại trắng. Nội dung bài viết
  không vẽ vì **nguyên nhân khác ngoài visibility/opacity** (ảnh không có mạng, script
  offline, layout phức tạp) — đúng như cảnh báo "nhiều nguyên nhân chồng lên" ở §4.5.
- Vì vậy **không khẳng định** vnexpress đã xanh; sweep live tiếp theo mới kết luận, và
  sweep đó do tiến trình khác chạy (tôi **không** chạy `testdata/live-parity.py`).

### 5.7 Bộ kiểm định xanh (trên máy này, SAU khi `git merge main`)

```
go build -o bin/goosie ./cmd/goosie          # exit 0
gofmt -l <touch packages>                    # exit 0 (rỗng)
go vet -all ./...                            # exit 0
go test -count=1 ./...                       # exit 0
go test -count=1 ./internal/archtest/        # exit 0
make gate                                    # exit 0  (failed=0 refused=0 deferred=0 panics=0 evictions=0)
python3 testdata/parity.py --render --render-chromium --binary ./bin/goosie   # exit 0  (86/86, avg 96.19%)
```

`git merge main` (đầu vào là các commit docs/QA + TASK-037 option-normalization) không
xung đột, không đụng `testdata/parity.py` (`MIN_SCORED_FIXTURES` vẫn 86 của tôi) và không
thêm fixture scored nào; gate + parity chạy lại cho kết quả y như trước merge (§5.4/§5.7).

### 5.8 Đề nghị BR id + danh sách oracle (integrator tag `// qc:BR-xxx` rồi `-update`)

BR cao nhất hiện có trong `test/qc/traceability.golden.json` là **BR-194**; tôi đề nghị
tiếp quản **BR-195/196/197**:

- **BR-195** — `visibility:hidden` bỏ vẽ chính hộp (bg/border/text), vẫn giữ layout,
  kế thừa xuống subtree, con khai `visible` hiện lại; phân biệt với `display:none`.
  Oracles (PARITY-adjacent, biên style→paint):
  - `test/paint/visibility_paint_test.go`: `TestVisibilityHiddenDropsBoxPaint`,
    `TestVisibilityHiddenSubtreeInheritsHidden`, `TestVisibilityHiddenKeepsLayout`,
    `TestDisplayNoneStillCollapses`.
- **BR-196** — `opacity` xuống tới display list: `0` bỏ cả subtree, phân số thật đi tới
  lệnh vẽ, opacity của tổ tích luỹ xuống cả nhóm (group opacity), và **một khai báo chỉ bị
  tính một lần** dù layout chèn anonymous block/text run (pass 2, §5.10).
  - `test/paint/opacity_paint_test.go`: `TestOpacityZeroDropsBox`,
    `TestOpacityFractionReachesCommand`, `TestOpacityAboveOnePaintsOpaque`,
    `TestGroupOpacityAppliesToSubtree`, `TestGroupOpacityCountedOncePerElement`.
- **BR-197** — tầng `style`: `opacity` clamp `[0,1]`, `<percentage>` = fraction/100,
  giá trị lỗi để nguyên (→1); `visibility` cascade.
  - `test/style/opacity_visibility_computed_test.go`: `TestOpacityClampsToUnitRange`,
    `TestVisibilityInheritsAndRestores`.
- Fixture PARITY cho cả ba: `testdata/render/css-features/12-hidden-and-transparent.html`
  (`MIN_SCORED_FIXTURES` 85→86 cùng commit thêm fixture). Pass 2 thêm
  `testdata/render/css-features/13-anonymous-group-opacity.html` cho rule "một khai báo, một
  lần nhân" (`MIN_SCORED_FIXTURES` 86→87 cùng commit).

### 5.9 Còn chưa làm / giới hạn phải khai báo

- `visibility:collapse` (table) **không** xử lý riêng — tôi gộp collapse như `hidden`
  (bỏ vẽ), chưa làm semantics "row tự sập khi mọi cell collapse". Ghi rõ ngoài phạm vi.
- **Group compositing thật**: tile raster nhân alpha **per-command**, không offscreen
  composite cả nhóm. Vì vậy các **sibling chồng nhau trong cùng một `opacity` group** sẽ
  **sẫm kép** (double-darken) khác Chromium (nhóm chỉ mờ một lần). Với các hộp không chồng
  nhau (đa số fixture và vnexpress) thì giống hệt; khác biệt chỉ lộ khi hai phần tử opaque
  đè nhau **trong** một nhóm mờ. Đây là giới hạn đã đo, không giấu.
- `ContentVisibility` (`style.go`, field có mà chưa có consumer): **để nguyên**, không bật
  "tiện tay"; vẫn giữ nhánh `ContentVisibility=="hidden"` đã tồn tại trong builder, chưa mở rộng.
- vnexpress offline: **0 ink → 0 ink**; chỉ `doc_height` đổi (706→526). Task này sửa phần
  overlay `opacity:0;visibility:hidden` lọt vào paint, **không** dựng lại được bài viết —
  còn script/ảnh/layout (đa nguyên nhân), phải đo bằng sweep live.

### 5.10 Pass 2 — một khai báo bị nhân hai và ba lần: chính pass 1 đã regression shape phổ biến nhất

Integrator đo trên nhánh và báo số liệu. Tôi **đo lại trước khi tin** — probe scratch
`test/paint/scratch_groupopacity_probe_test.go` in `Opacity` của **mọi** `CmdText`/`CmdFill`
sau `Session.PaintChecked(1.0)` → `dl.Build(1)`; probe đã bị xoá trước khi commit, nên số
liệu chốt ở đây được giữ sống bằng oracle vĩnh cửu dưới. Kết luận: đúng, và phạm vi rộng
hơn báo cáo một chút — **bốn** shape, không phải ba.

**Cơ chế.** `style.AnonymousBlockStyle` và `style.AnonymousTableStyle` copy nguyên
`ComputedStyle` của phần tử sinh (`s = *parent`, `internal/style/style.go:593-611`, `:619+`)
và **không** xoá `Opacity`; `style.inheritStyle` cũng copy cho mọi text node
(`cs := *parent`, `:1613-1625`). Walk của pass 1 (`internal/paint/builder_box.go:29-42` tại
`8eb00bc`) nhân `obj.Style.Opacity` vào `effective` cho **mọi** box, vô danh hay không.
Chromium tính một khai báo đúng **một lần trên phần tử khai báo nó**, vô luận layout chèn
bao nhiêu box.

**Bốn trạng thái trên cùng một probe** — opacity mà lệnh vẽ của **raw text** mang:

| shape | markup | `6071b8b` (trước nhánh) | `8eb00bc` (pass 1) | pass 2 | Chromium |
| --- | --- | --- | --- | --- | --- |
| `span` | `<div style=opacity:.5><span>text</span></div>` | **1** (mất fade) | 0.5 | 0.5 | 0.506 |
| `anon` | `<div style=opacity:.5>text</div>` | 0.5 (đúng do may) | **0.25** | 0.5 | 0.506 |
| `anondiv` | raw text + một `<div>` block sibling | 0.5, nhưng nền **và** text của con = **1** | **0.125** | 0.5 | 0.506 |
| `quarter` | `<div style=opacity:.25>text</div>` | 0.25 | **0.0625** | 0.25 | 0.255 |
| `nested` | cha `.5`, con khai `opacity:1`, raw text | **1** | 0.5 | 0.5 | 0.506 |
| `tableAnon` | `<td style=opacity:.5>` + raw text + block | không đo | **0.25** | 0.5 | 0.506 |
| `pseudoOwn` | `#t::before{content:"gen";opacity:.5}` | 0.5 | 0.5 | 0.5 | 0.506 |

Cột `6071b8b` đo thật: `git checkout 6071b8b -- internal/paint/`, chạy lại probe (exit 0),
rồi restore. Đọc cả bảng: pass 1 sửa được `span`/`nested` nhưng **làm hỏng
`anon`/`anondiv`/`quarter` — raw text nằm trong hộp mờ, tức shape có mặt ở mọi trang**,
kể cả cái mà trước nhánh vẫn đúng. `pseudoOwn` đứng ở bảng để phạt mọi sửa quá tay:
`::before` tự khai opacity thì phải được tính.

**Anchor Chromium, đo trên máy này.** `scripts/verify-task044-group-opacity.js` (Playwright,
viewport 400×200, DPR 1, `#t{font:16px 'Times New Roman';color:#000}` trên nền trắng;
worktree không có `node_modules` nên phải trỏ lên repo cha):

```
$ NODE_PATH=../../node_modules node scripts/verify-task044-group-opacity.js   # exit 0
SHOT span /tmp/t044-span.png … SHOT nested /tmp/t044-nested.png
$ python3 (PIL, min luminance trên /tmp/t044-*.png, alpha = 1 - lum/255)
span     min_lum=126.0  glyph alpha=0.506  ink_px=543
anon     min_lum=126.0  glyph alpha=0.506  ink_px=543
anondiv  min_lum=126.0  glyph alpha=0.506  ink_px=7743
quarter  min_lum=190.0  glyph alpha=0.255  ink_px=489
nested   min_lum=126.0  glyph alpha=0.506  ink_px=543
```

Bốn shape 0.5 cho **cùng một pixel tối nhất**: anonymous box và text run không tăng thêm bất
kỳ độ mờ nào. `anondiv` có `ink_px` lớn hơn vì nó vẽ thêm nền `rgb(10,10,120)` của đứa con.

**Điểm tôi làm khác brief — có đo, không phải ý kiến.** Brief ghi guard là
`obj.Style != nil && obj.Node != nil`. Lắp nguyên văn biến thể đó, chạy oracle:

```
$ go test -count=1 ./test/paint/ -run TestGroupOpacityCountedOncePerElement   # exit 1
--- FAIL: TestGroupOpacityCountedOncePerElement/anon       ... carried opacity 0.25,   want 0.5
--- FAIL: TestGroupOpacityCountedOncePerElement/anondiv    ... carried opacity 0.25,   want 0.5
--- FAIL: TestGroupOpacityCountedOncePerElement/quarter    ... carried opacity 0.0625, want 0.25
--- FAIL: TestGroupOpacityCountedOncePerElement/tableAnon  ... carried opacity 0.25,   want 0.5
```

Lý do: **multiplier thứ hai không phải anonymous block, mà là text run — và text run có
`Node`** (node text thật, `style.inheritStyle`). Guard kiểu đó bỏ được một trong hai lần
nhân: `anondiv` đi từ 0.125 lên 0.25, vẫn sai. Hai biến thể khác cũng đã đo thật:
`obj.Node.Element()` làm xanh cả bảy shape nhưng **giết** `::before{opacity:.5}` (0.5 → 1) —
`layout.buildPseudoElement`/`buildFirstLetterPseudoElement` (`internal/layout/arena.go:604-633`)
cho hộp pseudo một `doc.NewText` **không cha**, còn `style.computePseudoStyle`
(`internal/style/pseudo.go:124-159`) là kết quả cascade riêng, không copy `Opacity` của
parent; và biến thể nhận diện pseudo bằng `obj.Key.Kind()` hỏng vì tách word ở
`internal/layout/inline.go:724-768` dùng `nodeCopy := *k.Node`, để `Key` bằng 0 và **set
`Node` của hộp nguồn về nil**.

Luật chốt — `boxDeclaresOpacity`, `internal/paint/builder_box.go:22-37`: một box chỉ mang
opacity của riêng nó khi node của nó **là phần tử** hoặc **không thuộc phần tử nào**
(generated content). Clamp `[0,1]`, nhánh bỏ hộp khi `effective <= 0`, và cách xử lý
`visibility` giữ nguyên như pass 1. **Không** sửa `AnonymousBlockStyle`/`AnonymousTableStyle`:
nhiều consumer khác đọc bản copy đó (font, nền, border), và "mượn style của phần tử sinh" là
contract đúng — chỗ sai duy nhất là paint đem bản copy đó nhân thêm.

**Oracle vĩnh cửu.** `test/paint/opacity_paint_test.go::TestGroupOpacityCountedOncePerElement`
— bảy shape đúng bằng bảng trên; mỗi subtest so `Opacity` của **mọi** `CmdText`/`CmdFill` với
con số Chromium đã đo, và message trích luôn `min_lum` để sau này không ai phải đoán lại neo.

**Ba chứng minh.** (a) đỏ trên `8eb00bc`:

```
$ git stash push -- internal/paint/builder_box.go
$ go test -count=1 ./test/paint/          # exit 1
--- FAIL: TestGroupOpacityCountedOncePerElement (0.00s)
    --- FAIL: TestGroupOpacityCountedOncePerElement/anon (0.00s)
    --- FAIL: TestGroupOpacityCountedOncePerElement/anondiv (0.00s)
    --- FAIL: TestGroupOpacityCountedOncePerElement/quarter (0.00s)
    --- FAIL: TestGroupOpacityCountedOncePerElement/tableAnon (0.00s)
FAIL	github.com/vyquocvu/goosie/test/paint	0.652s
```

Bốn shape đỏ khớp đúng cột `8eb00bc`; `span`/`nested`/`pseudoOwn` xanh. (b) behaviour-off —
xoá guard, tức dựng lại chính walk của pass 1:

```
$ # boxDeclaresOpacity() -> `return obj.Style != nil`
$ go test -count=1 ./test/paint/ -run TestGroupOpacityCountedOncePerElement   # exit 1
    --- FAIL: TestGroupOpacityCountedOncePerElement/anon        (0.25)
    --- FAIL: TestGroupOpacityCountedOncePerElement/anondiv     (0.125)
    --- FAIL: TestGroupOpacityCountedOncePerElement/quarter     (0.0625)
    --- FAIL: TestGroupOpacityCountedOncePerElement/tableAnon   (0.25)
```

(c) restore → cả package xanh. Tập oracle đỏ trên commit đã hỏng, đỏ khi behaviour tắt, xanh
khi behaviour bật: nó gate **rule**, không gate trùng một bản sửa.

**Fixture pixel.** `testdata/render/css-features/13-anonymous-group-opacity.html`
(`MIN_SCORED_FIXTURES` 86→87 **cùng commit** thêm fixture): bốn dòng raw text 52px (shape
`anon`; dòng cuối nằm sát đáy viewport 600px), một khối 46px gồm raw text + block child + raw text
(shape `anondiv`, ba dòng), một dòng `quarter` 0.25, một control `nested opacity:1`,
cộng heading/paragraph/coloured bar để vẫn là một trang thật. Cố ý **không** đặt nền nào
trong vùng mờ: probe (`bgRawText`) in ra một `FILL rgba=200,0,0` **riêng cho từng word box**
— text run vẽ lại nền của phần tử sinh, một defect khác đã có từ trước và không liên quan;
nền translucent sẽ buộc fixture so sánh hai lỗi cùng lúc.

```
$ python3 testdata/parity.py --render --render-chromium --binary ./bin/goosie \
    --json /tmp/parity-t044-baseline.json          # exit 1 — binary pass 1 (guard đang stash)
   85.45%  FAIL  css-features/13-anonymous-group-opacity.png  blank_regions=0/0
  86/87 passing  (98.9%), 1 failing, 0 errors
  average match: 96.06%

$ make parity                                      # exit 0 — pass 2
  87/87 passing  (100.0%), 0 failing, 0 errors
  average match: 96.14%
```

Diff **từng** fixture giữa hai binary: chỉ fixture mới dịch quá 0,2đ — `+7,22đ`
(85.45 → 92.67), đúng là khoảng cách "một lần vs hai-ba lần" trên ảnh 800×600. Ba fixture cũ
dịch ≤0,18đ và **chính là ba fixture đã ghi ở §5.4**, theo chiều ngược lại:
`colors/03-opacity` 99.29 (trước nhánh) → 99.47 (pass 1) → **99.29** (pass 2);
`layout-features/04-positioning` 97.76 → 97.83 → **97.76**;
`semantic/02-article-section` 95.8 → 95.76 → **95.79**. Tức pass 2 không regression mà còn
trả ba fixture về đúng điểm trước khi nhánh tồn tại. **Known gaps không đổi: vẫn 5 fixtures /
10 bands**, và parity **không** đòi sửa `testdata/render-known-gaps.json` (fixture mới
`blank_regions=0/0`, exit 0) — file đó vẫn không bị đụng tới.

**Còn nguyên, không giấu.**

- Group compositing của §5.9 vẫn đúng: raster nhân alpha per-command, không offscreen, nên
  sibling chồng nhau **trong** một nhóm mờ vẫn sẫm kép khác Chromium. Pass 2 sửa thừa số của
  walk, không sửa cách hợp nhất — không tốt hơn, không tệ hơn.
- `::first-letter` mờ: rule mới áp dụng cho nó bằng cùng nhánh "node không cha", nhưng tôi
  **chưa đo pixel** cho shape đó và không có oracle riêng — ghi là chưa chứng minh.
- Text run vẽ lại nền của phần tử sinh (`bgRawText` ở trên) là defect **khác, có trước**,
  liên quan tới `paintsInlineFragments` ở `internal/paint/builder_box.go:112-131`; để nguyên,
  khai báo ở đây vì nó là lý do fixture 13 tránh nền translucent.

## 6. Ngoài phạm vi

- `mix-blend-mode`, `filter`, stacking context đầy đủ, `will-change`, group compositing thật sự (kể cả
  `isolation`), `contain: paint`, `content-visibility` auto/hidden semantics.
- Selector matching với escape (`GOOSIE-CSSSELECT-001`) và dispatch `+`/`.` trong tokenizer (ghi ở `GOOSIE-LIVE-002`).
- Việc harness dùng `/tmp/*-renders` chung (`GOOSIE-PARITYTMP-001`, gồm hệ quả "PNG cũ còn đó nên render rc 1 vẫn chấm 100%").
