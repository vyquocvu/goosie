# TASK-038 — đo render 100 trang web thật, lấy Chromium/Playwright làm chuẩn

**Priority:** P1. **Method:** PARITY (live corpus). **Owner:** integrator (hạ tầng + đo).
**Liên quan:** `GOOSIE-LIVE-001..004` (bốn defect task này mở ra từ số đo),
`GOOSIE-PARITYGATE-001` (TASK-036 — cùng kỷ luật fail-closed),
TASK-039 (root-cause họ trang không vẽ gì), TASK-040 (từ chối CSS trên trang thật),
`docs/roadmap-v2.md` (gate "browser dùng được").

## 0. Vì sao cần batch này

Corpus hiện tại (`testdata/render/`, 80 fixture) chứng minh engine vẽ đúng **những gì tác giả
viết ra để kiểm soát biến số**. Nó không trả lời câu hỏi người dùng thực gặp phải: *một trang
web thật ngoài internet có mở được không*. Đo trên 5 trang sống cho thấy khoảng trống đó là
thật, không phải suy đoán — xem §1.

## 1. Bằng chứng đã đo trên `main` (`4de0fca`), viewport 1280x1024, DPR 1

Lệnh Goosie: `goosie -backend headless -width 1280 -height 1024 -dpr 1 -screenshot -url <U> -out <png>`.
Lệnh Chromium: Playwright sync, `new_page(viewport 1280x1024, device_scale_factor=1)`,
`goto(wait_until="load", timeout=30000)`, `screenshot(animations="disabled")` — **viewport,
không full_page**, vì `steadyDriver` không scroll (`cmd/goosie/drive.go:134`).
Điểm ảnh tính bằng hàm có sẵn của `testdata/parity.py` (import, không viết lại lần hai).

| Trang | Chromium | Goosie | Kết quả đo |
| --- | --- | --- | --- |
| `example.com` | 95,967 B | 57,978 B | rc 0, ~2 s |
| `news.ycombinator.com` | 304,754 B | 209,181 B | rc 0 |
| `vnexpress.net` | 580,564 B, **ink 282,682 px** | 6,153 B, **ink 32 px** | rc 0, **stderr rỗng**, 6 band tham chiếu đều trống |
| `en.wikipedia.org/wiki/Goat` | 451,633 B | 387,477 B | pct_exact 71.05, coverage 0.872, 0 band trống |
| `developer.mozilla.org/en-US/docs/Learn` | 272,921 B | 132,847 B | pct_exact 86.41, coverage 0.218, worst_iou 0.117, 0 band trống |

Ba kết luận đo được, không suy diễn:

1. **Thất bại im lặng là có thật.** `vnexpress.net` render ra trang gần như trắng (32 px ink
   trên tổng 282,682 px tham chiếu) với **rc 0 và không một dòng stderr**. Không một test nào
   trong repo, và không một cổng nào trong `scripts/rc-sweep.sh`, nhìn thấy điều này.
2. **Không phải lỗi "không layout".** Chạy `goosie -gate -frames 120 -backend headless -url
   https://vnexpress.net/ -out json` trên cùng trang cho `doc_height=1694`,
   `tiles_rasterized=72`, `tiles_reused=8548`. Nghĩa là hộp đã được dựng và tile đã được nặn,
   nhưng ảnh cuối không có nội dung. Tách được `LAYOUT_EMPTY` khỏi `PAINT_EMPTY` là lý do
   tool phải chạy **hai** chế độ trên mỗi trang.
3. **`pct_exact` không phải thước đo đúng cho trang sống.** Wikipedia đạt coverage 0.872 nhưng
   chỉ 71% số pixel khớp từng kênh — lệch position/font là bình thường giữa hai trình render.
   Vì vậy phân loại phải dựa trên **band trống** và **coverage**, còn `pct_exact` chỉ là số ghi
   để so sánh giữa các lần chạy.

Ràng buộc kỹ thuật tìm được khi đo: deadline của chế độ screenshot **hard-code 25 s**
(`cmd/goosie/drive.go:170`, floor 8 frame ở `drive.go:129`), không có flag, và khi nó bắn thì
rc vẫn 0. Với trang sống chậm, đây là nguồn "ảnh cắt dở" — tool không được phép coi đó là
divergence của engine.

## 2. Thiết kế

### 2.1 Corpus — `testdata/live-sites.json`

100 URL, mỗi entry: `url`, `category`, `why` (một dòng: hành vi engine nó thử thách),
`expect` (`static` | `js-revealed` | `media-heavy` | `forms` | `tables`). Quy tắc chọn, ghi
thành chữ để người sau không đoán:

* Một trang/URL, không crawl; tôn trọng robots và ToS; tuần tự, nghỉ 150 ms giữa trang.
* Không trang bắt đăng nhập, không trang stream vô hạn.
* Hạn ngạch (tổng đúng 100): 18 tin tức VN, 14 tin tức quốc tế, 18 tài liệu/kỹ thuật,
  14 wiki/bách khoa, 10 commerce, 10 chuẩn-web/demo CSS, 10 trang chính phủ/tổ chức,
  6 trang tối giản hoặc cổ (HTML table layout, trang những năm 90).
* URLs nào chết theo thời gian thì **giữ nguyên** và để tool phân loại `FETCH_FAILED`; xoá
  khỏi corpus là hành động phải giải trình trong commit, vì nó làm sample nhỏ lại.

### 2.2 Tool — `testdata/live-parity.py`

* **Import** `parity.py` (`ink_mask`, `content_bands`, `score_regions`, `THRESHOLD`) — một
  định nghĩa duy nhất cho "thế nào là band trống", đúng bài học TASK-037 (một rule, một chỗ).
* Mỗi trang: (a) Chromium PNG, (b) Goosie PNG, (c) Goosie `-gate -frames 120` JSON.
  `doc_height` lấy từ (c).
* Thang phân loại, **bắt đúng một nhãn theo thứ tự ưu tiên**:
  `FETCH_FAILED` → `GOOSIE_CRASH` (rc != 0) → `ADMISSION_CAPPED` (stderr khớp
  `limit exceeded|must be finite|unsafe document extent`) → `DIMENSION_MISMATCH` →
  `LAYOUT_EMPTY` (`doc_height <= 0`) → `BLANK_OUTPUT` (ink Goosie < `MIN_INK` khi Chromium
  >= `MIN_INK`) → `MISSING_REGIONS` (band trống > 0) → `LOW_COVERAGE` (coverage < ngưỡng
  §2.4) → `OK`.
* `NOT_SETTLED_TODIMENSION` — nếu Goosie chạy đúng 25 s thì gắn cờ `maybe_partial`; trang
  đó không được tính vào `OK`.
* Noise floor: 12 trang được render **hai lần**; báo cáo `max |Δpct|` và `max |Δcoverage|`.
  Mọi kết luận phải lớn hơn dải nhiễu này mới được nêu.
* Đầu ra: `docs/qa-live-parity/<YYYY-MM-DD>T<HHMM>Z-<sha8>.json` (machine-readable, để lần
  sau so delta) và `.md` kèm bảng theo category.

### 2.3 Kỷ luật fail-closed (học từ GOOSIE-PARITYGATE-001)

* Không có chế độ "bỏ qua trang lỗi". Mỗi URL trong corpus phải có **đúng một** dòng trong
  báo cáo; thiếu artifact là một nhãn, không phải là skip.
* `MIN_SCORED` (số trang đã so sánh được cả hai bên) là **sàn cứng**: không đạt thì exit 2,
  tức "tool hỏng", chứ không in một tỷ lệ % gây hiểu lầm.
* Không được nâng `expect` của một trang để biến FAIL thành PASS.

### 2.4 Ngưỡng của riêng tool sống

Khai trong file và chứng minh bằng noise floor, **không** sửa `parity.py`:
`LIVE_COVERAGE_OK = 0.60` (dựa trên MDN 0.218 = hỏng rõ, Wikipedia 0.872 = ổn rõ),
`LIVE_MIN_INK = parity.MIN_INK` (200), band trống: 0 mới là `OK`.

### 2.5 Bốn nhãn phải thêm khi chạy thật (đo, không đoán)

Thang ở §2.2 viết trước khi có số liệu. Chạy thật buộc bổ sung — mỗi nhãn thêm có một
case trong `--selftest` và một mutation chứng minh nó giết được test:

* `GOOSIE_TIMEOUT` — `GOOSIE_TIMEOUT_S = 120`: một trang treo phải là một nhãn, không phải
  một exception làm hỏng cả sweep.
* `ADMISSION_CAPPED` đặt **trước** `GOOSIE_CRASH`. Đo trên `cafef.vn`: tầng CSS từ chối
  (`goosie: goosie: build session: CSS numeric value outside safe geometry range`) và
  **rc = 1**. Xét rc trước sẽ gộp trang này vào họ "crash", tức gộp "engine từ chối tài
  liệu" với "engine chạy hỏng" — hai họ sửa khác nhau. `CAP_RE` lấy theo lời phản hồi thật
  của `internal/engine/limits.go` (limit exceeded / must be finite / unsafe document /
  outside safe geometry range).
* `REFERENCE_EMPTY` — Chromium tự nó không vẽ gì (`ink < LIVE_MIN_INK`). Trang đó không
  kiểm được engine, nên vừa là nhãn riêng vừa **không** tính vào `scored`; tính vào là để
  tỷ lệ % đếm cả những trang chưa từng là một phép so sánh.
* `STRUCTURE_DISAGREES` — `doc_height = 0` mà ảnh Goosie có 268,190 px ink (đo trên
  `nytimes.com`, `usatoday.com`). Hai chế độ là hai lần chạy khác nhau của một trang sống;
  gán `LAYOUT_EMPTY` cho nó là quy kết cho layout điều dữ liệu không nâng đỡ.

Thêm `goosie_diag`: một dòng giải thích (dòng từ chối, hoặc dòng fatal), **chỉ** khi có dòng
như vậy. Không bao giờ lấy dòng counter/timing của `-screenshot`: nó đổi theo từng lần chạy
và sẽ phá đúng yêu cầu "hai lần chạy cho ra cùng một báo cáo" (§3.7).

### 2.6 Điều khiển cần để sửa một họ lỗi

`--only <substring>` (lặp được) và `--artifacts <dir>`: sau khi phân loại, đo lại đúng nhóm
trang mang một nhãn và xem PNG bằng mắt, mà không phải gọi lại 100 lần lên server của người
khác. `--limit`/`--only-category` dùng cho smoke run; cả hai đều không đổi nhãn, chỉ đổi
tập trang — nên báo cáo nhỏ sẽ đúng theo §2.3 bị exit 2 vì dưới `MIN_SCORED`.

## 3. Test của chính tool (nghiệm thu `[R-7]` áp lên hạ tầng kiểm thử)

`testdata/live-parity.py --selftest` chạy trên corpus tạm (12 trang tổng hợp), **không cần
mạng, không cần Chromium**: renderer thật được thay bằng stub trả đúng hình/PNG/rc/stderr
mà thang phân loại sẽ nhìn thấy. Bảy case ban đầu, cùng năm case do §2.5 thêm:

1. `blank` — Goosie PNG trắng toàn bộ, Chromium có nội dung ⇒ `BLANK_OUTPUT`.
2. `cropped` — Goosie PNG cắt mất nửa dưới ⇒ `DIMENSION_MISMATCH`, không phải `OK`.
3. `layout_empty` — `doc_height=0` **và** không vẽ gì ⇒ `LAYOUT_EMPTY` (khác `BLANK_OUTPUT`).
4. `missing` — rc 0 nhưng không có PNG ⇒ `GOOSIE_RENDER_MISSING`, và `scored` giảm tương ứng.
5. `crash` — rc 1, stderr fatal không thuộc họ từ chối ⇒ `GOOSIE_CRASH`, không rơi vào
   nhánh coverage.
6. `regions` — 3/4 band có nội dung ⇒ `MISSING_REGIONS`; chạy nhiều nhãn ⇒ exit != 0 **và**
   báo cáo vẫn đủ dòng cho mọi URL.
7. `determinism` — hai lần chạy cùng corpus, khác thư mục artifact ⇒ JSON **giống hệt**.
8. `dead` — Chromium không tải được ⇒ `FETCH_FAILED`, không phải exception.
9. `ref_empty` — Chromium không vẽ gì ⇒ `REFERENCE_EMPTY`, và không tính vào `scored`.
10. `slow` — đúng 25 s ⇒ `NOT_SETTLED` + `maybe_partial`, không được tính là `OK`.
11. `capped` — từ chối ở CSS kèm rc 1 ⇒ `ADMISSION_CAPPED` (thắng cả `GOOSIE_CRASH` lẫn
    `BLANK_OUTPUT` mà nó cũng thỏa), và dòng từ chối phải xuất hiện trong báo cáo.
12. `disagrees` — `doc_height=0` kèm ảnh có ink ⇒ `STRUCTURE_DISAGREES`, không đổ cho layout.

Kiểm tra bổ sung: `goosie_diag` của một chạy sạch phải rỗng (chứng minh bằng case 7 — nếu
tool rò rỉ dòng timing thì hai lần chạy không còn giống hệt); `exit_code` ưu tiên sàn
`MIN_SCORED` hơn "sạch" (6 trang `OK` vẫn là exit 2, vì đó là một lần đo không đủ mẫu).

Tám mutation đã chạy, mỗi cái giết ít nhất một check — xem §6.

## 4. Ràng buộc vận hành

* **Không phải gate CI**: cần mạng, và trang sống thay đổi từng ngày. Chỉ chạy qua
  `make live-parity` (target mới, opt-in). `make parity` và corpus fixture giữ nguyên kết quả.
* **Cấm** sửa `testdata/parity.py`, `testdata/render-known-gaps.json`, `PASS_AT`, `MIN_INK`
  của fixture harness. Tool sống có hằng số riêng, đặt trong file của nó.
* Không đổi code engine trong task này — kể cả deadline 25 s. Nếu cần cờ để phân biệt
  "chưa settle", ghi thành defect riêng (`GOOSIE-LIVE-003`), task khác làm.
* Chạy một mình, không song song với sweep khác (`GOOSIE-FLAKE-001`); kiểm `uptime` trước.
* Không `git push`, không merge `main`, làm trong worktree riêng.
* Báo cáo phải nêu **số trang thật so sánh được** trước khi nêu bất kỳ tỷ lệ %.

## 5. Ngoài phạm vi

So sánh **toàn bộ** trang (full-page screenshot) — cần scroll/viewport pipeline của engine
trước đã; đo hiệu năng và footprint khi mở 100 trang; các họ lỗi root-cause mà sweep tìm ra
(JS reveal, web font, grid/flex từ CSS thực tế, cookie overlay) — mỗi họ một defect và một
task riêng sau khi có số liệu.

## 6. Kết quả đo (integrator chạy trực tiếp, worktree `worktree-task-038`)

Binary: `go build -o /tmp/liveprobe/goosie ./cmd/goosie` tại `4de0fca`. Chromium: Playwright
sync, đúng contract §1. Hai lần quét toàn corpus, cách nhau 26 phút:

| Lệnh | Load lúc chạy | Artifact |
| --- | --- | --- |
| `python3 testdata/live-parity.py --binary /tmp/liveprobe/goosie` (classifier **trước** §2.5) | 3.24 | `docs/qa-live-parity/2026-10-25T1630Z-4de0fca5` |
| `python3 testdata/live-parity.py --binary /tmp/liveprobe/goosie --artifacts /tmp/live-artifacts` (classifier **sau** §2.5) | 7.67 | `docs/qa-live-parity/2026-10-25T1656Z-4de0fca5` |

`make live-parity-selftest`: green. `make -n live-parity live-parity-selftest`: đúng recipe.
Báo cáo **authoritative là lần 2** (do code đang ship); lần 1 giữ lại vì nó là chứng cứ cho
§2.5 và cho phép tính độ ổn định liên lần.

### 6.1 Con số

Lần 2: **94/100 trang so sánh được hai bên** (sàn `MIN_SCORED = 75`, không đổi sau khi đo);
exit 1 vì có trang không `OK`.

| label | lần 1 | lần 2 |
| --- | --- | --- |
| `OK` | 47 | **46** |
| `MISSING_REGIONS` | 21 | **22** |
| `LOW_COVERAGE` | 14 | **17** |
| `LAYOUT_EMPTY` | 9 | **8** |
| `BLANK_OUTPUT` | 2 | **1** |
| `ADMISSION_CAPPED` | — | **1** (`GOOSIE_CRASH` 1 ở lần 1, xem §2.5) |
| `FETCH_FAILED` (lỗi mạng, không đổ cho engine) | 6 | **5** |

Theo category (OK/n, lần 2): wiki 11/14, minimal-legacy 4/6, web-standards 6/10,
commerce 5/10, docs 8/18, news-vn 6/18, gov-org 3/10, news-intl 3/14.
Theo `expect`: **static 33/59**, **js-revealed 12/39**, media-heavy 1/1, tables 0/1.
Khoảng cách static ↔ js-revealed (56% so với 31%) là khác biệt lớn hơn mọi noise quan sát
được, nên kết luận "trang dựa vào JS là họ yếu nhất" có cơ sở; còn thứ tự *giữa* các category
thì chưa, vì dải nhiễu liên lần (mục dưới) rộng hơn chênh lệch giữa hai category gần nhau.

### 6.2 Noise floor và độ ổn định

* Double-render 12 trang, **cả hai phía**: lần 2 max |Δpct_exact| = 23.21 (vnexpress),
  max |Δcoverage| = **0.084**, 0 nhãn đổi. Lần 1: 24.49 / 0.469 / 1 nhãn đổi (laodong.vn).
* 5/12 trang đo lại có Δ **chính xác bằng 0.0** cho cả pct lẫn coverage (guardian,
  docs.python.org, php.net, htmldog, example.com; uniqlo 0.02): với trang tĩnh, hạ tầng này
  tái lập từng điểm ảnh. Nhiễu tập trung ở nhóm JS-revealed, không phải ở phép đo.
* Toàn corpus: **94/100 nhãn giống hệt** giữa hai lần quét. 6 nhãn đổi, trong đó 1 là do
  code phân loại đổi (cafef.vn `GOOSIE_CRASH` → `ADMISSION_CAPPED`), 5 là do trang đổi
  trạng thái: laodong.vn `BLANK_OUTPUT`→`LOW_COVERAGE`, usatoday
  `LAYOUT_EMPTY`→`LOW_COVERAGE`, vneconomy `OK`→`LOW_COVERAGE`, fr.wikipedia
  `OK`→`MISSING_REGIONS`, bestbuy `FETCH_FAILED`→`OK`.
* Hệ quả bắt buộc với người đọc báo cáo: so **họ nhãn** giữa hai lần quét; không so
  `coverage`/`pct_exact` của một trang JS nào đó giữa hai lần.

### 6.3 Bốn họ đã đo được, chép nguyên số vào defect registry

* **`LAYOUT_EMPTY` + rc 0 + stderr rỗng — 8 trang**: apnews, nytimes, japantimes, reuters,
  etsy, newegg, zara, gov.vn. Mỗi trang `doc_height=0` **và** `ink_goosie=0`, trong khi
  Chromium cùng viewport vẽ 8,846–501,735 px ink (gov.vn: 501,735). Không một dòng stderr.
  Probe offline sau lần quét cho thấy **đây không phải một lỗi**: `www.gov.vn` tái hiện không cần
  mạng (bản curl 192,815 B với **11,859 ký tự text nhìn thấy được** → rc 0, ink 0,
  `-gate -frames 120` báo `doc_height=0`, `tiles_rasterized=0`, `zero_work_frames=120`), còn
  `apnews.com` **không** tái hiện (cùng binary, cùng markup lưu về 2,610,822 B: ink **763,855**,
  `doc_height=34,867`) — nghĩa là ít nhất một nhánh nằm ở đường fetch/response, không phải layout,
  và `expect: js-revealed` không giải thích được apnews vì HTML của nó có 70,685 ký tự nội dung
  tĩnh. Bẫy đo: `style_passes`/`layout_passes` đọc 0 trên **cả** trang đã layout thành công, nên
  không được dùng làm bằng chứng "engine chưa style". → `GOOSIE-LIVE-001`, đo tiếp ở TASK-039.
* **`BLANK_OUTPUT` — vnexpress.net**: `doc_height=691`, `ink_goosie=32` trên
  `ink_chromium=301,440`; rc 0, stderr rỗng. Tái lập y hệt lần 1 (`691→847`, `32/627,498`)
  và đúng như đo thủ công ở §1. → cùng `GOOSIE-LIVE-001` (một họ, hai mức: không layout, và
  có layout mà không paint).
* **`ADMISSION_CAPPED` — cafef.vn**: `goosie: goosie: build session: CSS numeric value
  outside safe geometry range`, rc 1. Tái lập **ngoài mạng** từ bản HTML lưu sẵn
  (`-url file:///tmp/cafef.html`, 217,170 B). Chuỗi bisection sau lần quét đã tìm ra thủ phạm:
  **tên** một custom property bắt đầu bằng chữ số — `.box-ttdn .list .mdn .time {
  color: var(--8e8e8e, #8e8e8e) }` — tokenizer phát `Dimension value="-8e8" unit="e8e"`
  (`numval=-8e+08`), `internal/engine/limits.go:239-241` áp `MaxGeometry = 2<<20` (= 2,097,152)
  cho dimension ⇒ **cả document bị từ chối**. Repro một dòng `:root{--8e8e8e:#8e8e8e}`;
  Chromium đo trên máy này chấp nhận (`var(--8e8e8e)` → `rgb(171, 205, 239)`); đổi đúng ident đó
  thành `--grey` thì cafef.vn paint **554,999 px ink** với rc 0. Đã bác bằng thực nghiệm: "số ≥7
  chữ số ở đâu đó trong document (kể cả `z-index:999999999`)" — thay hết vẫn rc 1, vì thủ phạm dài 8 ký tự và
  không chứa dãy chữ số nào; và "giá trị sinh ra sau khi script chạy" — nó nằm nguyên trong khối
  `<style>` thứ 8 của tài liệu tĩnh, bỏ 9 khối `<style>` là rc 0 trong khi bỏ `style=` attribute
  thì không. Message vẫn không nêu property, không nêu value, không nêu ngưỡng
  (`internal/engine/limits.go:53,226-241`), và bị in đúp tiền tố `goosie: goosie:`.
  → `GOOSIE-LIVE-002`, spec ở TASK-040.
* **Họ "nội dung ở chỗ khác" — và nó làm mềm số 46 `OK`**: `coverage` là `goosie_total /
  chromium_total` (`testdata/parity.py:279`), tức **tỷ lệ khối ink**, nên nó vượt 1 khi Goosie in
  nhiều hơn reference: **30/94** trang so sánh được có coverage > 1.0, cao nhất 2.83
  (`w3.org/Style/CSS/` — 1,306,325 px ink Goosie so với 15,209 của Chromium), 2.57 (`ebay.com`),
  2.32 (`validator.w3.org`). `w3.org/TR/css-syntax-3` bị `LOW_COVERAGE` (0.174) dù Goosie in
  nhiều hơn reference; `go.dev/ref/spec` cho 2.268. Tệ hơn, **nhãn không dùng số vị trí**:
  trong 46 trang `OK` có **26 trang `worst_iou < 0.5`** và **16 trang `< 0.1`**, ví dụ
  `en.wikipedia.org/wiki/Vietnam` (coverage 1.547, `worst_iou` **0.001**, 0/4 band trống) và
  `wiki/Cuneiform` (1.972, 0.014) — tức ink của Goosie gần như **không chồng** lên band tương ứng
  của Chromium mà vẫn được chấm `OK`. `worst_iou` được ghi ở `testdata/live-parity.py:162` và
  **không** tham gia `classify`. Hệ quả cho cách đọc §6: `OK` nghĩa là "có vẽ, không band nào
  trống, đủ khối ink, đúng kích thước cửa sổ", **không** nghĩa là "đúng vị trí". Các nhãn
  `LAYOUT_EMPTY`, `BLANK_OUTPUT`, `MISSING_REGIONS`, `DIMENSION_MISMATCH` vẫn đáng tin vì chúng
  so theo toạ độ band. → `GOOSIE-LIVE-004` (defect của chính hạ tầng đo, không phải của engine).

### 6.4 Mutation proof trên hạ tầng (8/8 giết được check)

Mỗi dòng: sửa một quyết định phân loại trong `testdata/live-parity.py`, chạy `--selftest`,
rồi khôi phục. Kết thúc: `diff` với bản gốc — **giống từng byte**.

| Mutation | Check bị giết (FAIL) |
| --- | --- |
| Xét rc trước họ từ chối | `capped -> ADMISSION_CAPPED` (thành `GOOSIE_CRASH`), `a refusal that exits 1 outranks both crash and blank-paint` |
| Bỏ nhánh `REFERENCE_EMPTY` | `ref_empty -> REFERENCE_EMPTY` (thành `BLANK_OUTPUT`), `scored counts only two-sided comparisons — scored=7` |
| `doc_height<=0` luôn là layout-empty | `disagrees -> STRUCTURE_DISAGREES`, `doc_height=0 against a painted page is not blamed on layout` |
| Bỏ sàn `MIN_SCORED` | `the floor outranks cleanliness…`, `below MIN_SCORED the tool calls itself broken (exit 2)…` |
| Bỏ nhánh rc != 0 | `crash -> GOOSIE_CRASH` (thành `GOOSIE_RENDER_MISSING`) |
| Bỏ kiểm tra deadline 25 s | `slow -> NOT_SETTLED` (thành `OK`), `deadline page is flagged maybe_partial` |
| Resize ảnh thay vì báo mismatch | `cropped -> DIMENSION_MISMATCH` (thành `MISSING_REGIONS`), `scored counts…`, `…nonzero exit, all 3 rows reported` |
| `_diag` rớt về dòng stderr cuối | `a clean run records no diagnostic (timings would break reproducibility)` — lộ `goosie: timings mean_ms=1.234 p50_ms=1.100 max_ms=3.400` |

### 6.5 Giới hạn, ghi ra chứ không giấu

* **Chỉ viewport** (1280×1024, không scroll, hai phía như nhau): một trang có nội dung dưới
  fold hoàn toàn không được đo; `MISSING_REGIONS` chỉ kết luận được về phần trên của trang.
* `wait_until: "load"` + `animations: disabled`: nội dung cần cuộn/click mới hiện sẽ tính là
  hỏng. Điều đó đúng với trải nghiệm, nhưng nó là giới hạn của phép đo chứ không phải bằng
  chứng "engine render sai CSS".
* Lần 2 chạy với load 7.67/10 core, trong lúc một agent khác đang làm việc ở worktree của
  nó. Tải cao chỉ có thể gây **đỏ giả** (`NOT_SETTLED`, timeout), không tạo xanh giả. Kiểm
  bằng số thay vì yên trí: render Goosie chậm nhất trong lần quét đó là 8.93 s
  (straitstimes.com), còn cách ngưỡng 25 s hơn 16 s, và không trang nào bị `NOT_SETTLED` hay
  `GOOSIE_TIMEOUT` — tức phân bố nhãn không bị điều kiện máy ảnh hưởng theo hướng có lợi.
* 100 URL chọn bằng tay theo hạn ngạch §2.1 — **không** phải mẫu ngẫu nhiên của internet;
  tỷ lệ OK chỉ có nghĩa trong phạm vi tập này.
* Sweep đặt **tên cho họ lỗi**, không tìm nguyên nhân. `MIN_SCORED = 75` và
  `LIVE_COVERAGE_OK = 0.60` viết trước khi quét và không đổi sau khi có số.
* `expect: tables` chỉ 1 trang (0 OK) — quá nhỏ để kết luận gì về table layout.
* **Phía chuẩn cũng dao động**, không chỉ phía Goosie: vnexpress giữa hai lần quét cho
  `ink_chromium=627,498 → 301,440` và `doc_height=847 → 691`. Một phần noise của trang
  `js-revealed` nằm ở chuẩn, nên `Δcoverage`/`Δpct_exact` của nhóm đó không quy hết cho engine.
* **Hai chế độ của Goosie có thể mâu thuẫn trên cùng một URL, cùng một lần chạy**: run 1 của
  `nytimes.com` cho `-gate` `doc_height=0` trong lúc `-screenshot` paint **268,190 px ink**.
  Nhãn `STRUCTURE_DISAGREES` (§2.5) sinh ra đúng vì dòng này, và nó là lý do không được kết luận
  "layout rỗng" từ một chế độ duy nhất.
* Artifact `.json` ghi `run_id` mang timestamp; ngoài nó ra hai lần chạy cho báo cáo giống
  hệt (selftest case 7).

