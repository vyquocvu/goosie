# TASK-042 — "band trống" trong parity có thể chỉ là một dòng bị lệch ~20px

**Priority:** P2. **Method:** INT (selftest của harness) + PARITY (đo lại toàn bộ corpus).
**Owner:** chưa gán. **Defect:** `GOOSIE-BLANKMETRIC-001`.
**Liên quan:** `testdata/parity.py` (nơi sửa), `testdata/live-parity.py` (import `score_regions`
nguyên trạng nên ăn cùng lỗi), `testdata/render-known-gaps.json` (baseline 5 fixture / 10 band),
`docs/tasks/TASK-027-GLYPH-COVERAGE.md` §6.2 (bản phân loại 18 band), `TASK-036` (cùng file,
**phải phối hợp** — xem §5).

Task này không sửa engine. Nó sửa **ý nghĩa của một con số** mà toàn bộ bằng chứng render của repo
dựa vào: `blank_regions`. Hiện nay con số đó trả lời sai câu hỏi mà mọi người đang đọc nó như thể
nó trả lời.

Vì sao ghi P2 chứ không P1: chiều sai của metric này là **báo oan** (nội dung có thật mà báo là
mất), không phải **bỏ lọt**. Một phép đo báo nhiều vấn đề hơn thực tế vẫn còn an toàn hơn nhiều so
với một phép đo bỏ qua vấn đề — đó chính là khác biệt mà `GOOSIE-PARITYGATE-001` (P1) đang chặn.
Nhưng chi phí của nó là có thật và đã đo: `TASK-033` đã được giao một phần để diệt 3 band ở
`selectors/03`, và cả 3 band còn nguyên sau khi `::first-letter` chạy đúng.

## 1. Bằng chứng đã đo (2026-10-26, tại `452b6c2`, offline)

### 1.1 Cơ chế trong code

`testdata/parity.py:253-279` `score_regions()`:

* band được dựng từ mask của **Chromium**: `bands = content_bands(cm)`.
* một band bị báo trống khi `goosie_count == 0` **trong đúng các dòng của Chromium**.
* nhánh IoU có `_dilate_one()` (giãn 1 dòng) trước khi so sánh; nhánh blank **không giãn gì cả**.

⇒ mọi đường của Goosie lệch khỏi band Chromium nhiều hơn khoảng trống giữa hai dòng đều bị đếm là
"nội dung mất", dù nó đang nằm ngay sát đó.

### 1.2 Hai trường hợp đo được, nội dung có thật

| Fixture | Band Chromium | Ink Goosie trong band | Ink Goosie quanh band | Nội dung có trên ảnh? |
| --- | --- | --- | --- | --- |
| `selectors/03-pseudo-elements.png` | `(457,475)` | **0** | dòng 437–455 và 483–498 | **có** — "This paragraph has a checkmark after it" kèm cả dấu ✓ |
| `layout-features/05-floats.png` (canvas 800×2400) | `(1055,1071)`, cols 20–460, 2,775 px | **0** (bản hiện tại) | bản bỏ float-skip: 2,289 px, lệch ~15px | **có** — "Content after the overflow:hidden container (should be below)." |

Lệnh chạy lại được (cả hai đều đã chạy trên máy này):

```bash
python3 testdata/parity.py --render --binary ./bin/goosie   # rồi đọc WARNING known gap ...
python3 - <<'PY'
import sys; sys.path.insert(0,'testdata')
from PIL import Image; import parity
g=parity.ink_mask(Image.open('/tmp/goosie-renders/selectors/03-pseudo-elements.png').convert('RGB'))
print([ (y0,y1) for y0,y1,_,_,_ in parity.content_bands(g) if 420<=y0<=510 ])
PY
```

### 1.3 Hệ quả đã xảy ra, không phải hệ quả giả định

* `testdata/render-known-gaps.json` chứa **5 fixture / 10 band** và là baseline của release gate.
  Cho tới nay các con số đó được đọc là "10 chỗ nội dung chưa render được".
* `docs/tasks/TASK-027-GLYPH-COVERAGE.md` §6.2 phân loại nguyên nhân từng band. Ít nhất 3 band
  trong số đó (`selectors/03`) nay **được chứng minh là artifact**; chính §6.2 đã mô tả đúng cơ chế
  ("các band đối chiếu của Chromium rơi vào khoảng margin trống giữa các đoạn văn của Goosie") rồi
  gán nhầm nguyên nhân cho `::first-letter`. Đã đính chính tại đó, nhưng **8 band còn lại chưa hề
  được đo lại** — xem §2.1.
* `testdata/live-parity.py` import `score_regions` và `content_bands` **bằng reference**, nên nhãn
  `BLANK_OUTPUT` của sweep 100 trang sống mang cùng ý nghĩa sai.

## 1.4 Phân loại lại đủ 10 band đang khai báo (kết quả §2.1 — đo bằng code, 2026-10-26, `worktree-task-042`, offline)

Đo bằng `ink_mask`/`content_bands` của chính `parity.py` trên ảnh đã render sẵn ở
`/tmp/goosie-renders` và `/tmp/playwright-renders` (chỉ đọc, không render lại). Cột:
`số px trong đúng dòng band`, `số px trong cửa ±band_height (cùng cột x0:x1)`,
`dòng ink Goosie gần nhất trong cột band + khoảng cách tới mép band`. `bh = y1-y0+1`.

| Fixture | Band | bh | ink trong band | ink trong ±bh | dòng ink gần nhất (dist) | phân loại |
| --- | --- | --- | --- | --- | --- | --- |
| `forms/04-advanced-inputs.png` | (376,388) | 13 | 0 | 392 | 372 (4) | **shifted** |
| `forms/04-advanced-inputs.png` | (443,454) | 12 | 0 | 493 | 437 (6) | **shifted** |
| `html-features/01-forms-complete.png` | (593,599) | 7 | 0 | 0 | 585 (8) | **blank (thật)** |
| `selectors/02-pseudo-classes.png` | (471,487) | 17 | 0 | 1472 | 488 (1) | **shifted** |
| `selectors/02-pseudo-classes.png` | (506,526) | 21 | 0 | 1723 | 528 (2) | **shifted** |
| `selectors/02-pseudo-classes.png` | (551,569) | 19 | 0 | 3633 | 573 (4) | **shifted** |
| `unicode/02-emoji-symbols.png` | (451,470) | 20 | 0 | 898 | 449 (2) | **shifted** |
| `selectors/03-pseudo-elements.png` | (457,475) | 19 | 0 | 934 | 455 (2) | **shifted** |
| `selectors/03-pseudo-elements.png` | (539,554) | 16 | 0 | 2725 | 556 (2) | **shifted** |
| `selectors/03-pseudo-elements.png` | (579,599) | 21 | 0 | 2680 | 578 (1) | **shifted** |

Kết luận đo được: **9/10 band là drift** (có ≥ `MIN_INK` ink ngay cạnh band, chỉ lệch
1–6 dòng ≤ bh), **1/10 band thật sự mất** (`html-features/01` (593,599): 0 ink trong
cửa ±7, ink gần nhất cách 8 dòng > bh). 3 band của `selectors/03` mà TASK-033 đã nhận
là artifact nay được xác nhận bằng code: cả 3 đều shifted.

## 2. Phải đo trước khi sửa

1. **Phân loại lại đủ 10 band đang khai báo**, không chỉ 3 band của `selectors/03`. Với mỗi band:
   ghi `(số px trong band, số px trong cửa ±band_height, dòng gần nhất có ink)`. Kết quả phải vào §1
   dưới dạng bảng, đo bằng code chứ không bằng mắt.
   → **ĐÃ ĐO, xem §1.4.** 9/10 shifted, 1/10 (`html-features/01`) blank thật. Không dòng nào "chưa đo".

2. **Bao nhiêu `BLANK_OUTPUT` của sweep sống là drift?** `docs/qa-live-parity/2026-10-25T1656Z-4de0fca5.json`
   có sẵn ink mask; chạy lại classifier cũ trên dữ liệu cũ, chỉ thêm cột drift. Con số này quyết
   định mức độ khẩn cấp của task — hiện chưa biết.
   → **ĐÃ ĐO (2026-10-26): 0/1.** Sweep có đúng 1 nhãn `BLANK_OUTPUT` (`https://vnexpress.net/`,
   `ink_goosie=32`, `ink_chromium=301440`, `coverage=0.0`). `BLANK_OUTPUT` do `classify` gán bằng
   điều kiện **toàn trang** `ink_goosie < LIVE_MIN_INK (200)` (live-parity.py:203-204), không phải
   theo band. Vì cả trang chỉ 32 ink px < 200, không band nào có thể đạt ngưỡng `MIN_INK` trong cửa
   ±bh ⇒ **cả 7 band của nó vẫn `blank`, 0 band thành `shifted`.** Kết luận: `BLANK_OUTPUT` về mặt
   cấu trúc **không** phải drift — drift nằm ở tín hiệu theo band (`blank_regions`/`MISSING_REGIONS`),
   đúng chỗ `parity.py` và 22 dòng `MISSING_REGIONS` của sweep bị ảnh hưởng.
   → **Chưa đo được mức drift của 22 dòng `MISSING_REGIONS`** (cần ảnh goosie để đếm ink quanh từng
   band): file JSON chỉ lưu `ink_goosie`, `blank_bands` (tọa độ dòng) và các aggregate, **không** lưu
   mask/ảnh; PNG của sweep ghi vào temp dir và bị `shutil.rmtree` khi thoát (live-parity.py:679,688).
   Render lại bị cấm trên máy này (rule 9: `/tmp/*-renders` là bằng chứng sống, sweep 100 trang đang
   chạy, và live URL cần mạng). Ghi rõ là **chưa đo**, không ước số.

3. **Ngưỡng nào là nguyên tắc, không phải chỉnh theo số?** Cửa trôi phải đến từ một đại lượng có
   nghĩa sẵn (chiều cao band, hoặc line-height của chính band đó), **không** phải một hằng số mới
   được chọn cho tới khi báo cáo đẹp. Viết rõ công thức và lý do chọn trước khi chạy trên corpus.
   → **ĐÃ CHỐN TRƯỚC KHI CHẠY CORPUS.** Công thức: cửa trôi của band `(y0,y1,x0,x1)` là
   `[y0 - bh, y1 + bh] × [x0, x1)` với `bh = y1 - y0 + 1` (chính là chiều cao band mà
   `content_bands` đã trả về). Band `shifted` ⇔ `ink_trong_band == 0` **và** `ink_trong_cửa >= MIN_INK`;
   ngược lại `blank`. `MIN_INK = 200` là ngưỡng "đây là nội dung thật" **đã có sẵn** của harness
   (docstring `ink_mask`), không phải hằng số mới. Vì sao nguyên tắc: `bh` là cỡ đúng của một vùng
   nội dung — độ lệch cỡ một dòng/box (line-height, margin rounding) vẫn là cùng nội dung, còn độ lệch
   lớn hơn chính chiều cao vùng thì không còn là "một dòng bị nudged". Chọn `bh` (tự chứa trong band)
   chứ không chọn line-spacing (cần band kề bên, không tồn tại khi band đơn độc). Không đụng
   `PASS_AT`/`THRESHOLD`/`MIN_INK`/`MIN_SCORED_FIXTURES`/`LIVE_COVERAGE_OK`.

4. **Có trường hợp "mất thật" mà cửa trôi cũng che được không?** Tìm một mutation chủ ý (xoá hẳn
   một phần tử khỏi fixture) và chứng minh nó vẫn bị báo `absent`, không bị tha thành `shifted`.
   Không có phép thử này thì mọi nới lỏng đều vô hại hoá metric.
   → **KHÔNG. Đã chứng minh bằng cả dữ liệu thật lẫn mutation.** (a) Thật: `html-features/01`
   (593,599), bh=7, cửa [586,606] có 0 ink, ink gần nhất cách 8 dòng (> bh) ⇒ vẫn `blank`.
   (b) Synthetic `case D4` trong `parity_selftest.py --drift`: xoá hẳn phần tử (goosie không vẽ gì)
   ⇒ `truly_blank`, `shifted` rỗng. (c) Mutation (ii) §4.5: nới thanh `>= MIN_INK` thành `>= 0`
   (tha mọi pixel lạc) thì `case D4` **FAIL** — oracle thật sự bắt được việc tha mất nội dung.

## 3. Plan (chỉ làm sau khi §2 xong)

* **Bước 0 — oracle trước.** Thêm case vào selftest của harness (kiểu `live-parity.py --selftest`:
  ảnh tổng hợp, không cần browser) cho từng hình thái: khớp, lệch 1 dòng, lệch hơn một band, và
  **mất hẳn**. Mỗi case một kỳ vọng dương `[R-7]`.
* **Bước 1 — tách hai tín hiệu trong `score_regions`**, trả về thêm một kết quả phụ thay vì đổi
  nghĩa kết quả cũ: `blank` (không có ink trong cửa trôi) và `shifted` (band trống nhưng có ink
  trong cửa). **Giữ nguyên** tập giá trị trả về hiện có để mọi call site cũ còn chạy đúng; call
  site nào muốn dùng `shifted` phải gọi tường minh.
* **Bước 2 — báo cáo, không ẩn đi.** `make parity` in cả hai loại, ví dụ
  `blank=1 shifted=2 rows=[...]`. `match %` **không đổi** ở bước này.
* **Bước 3 — phân loại lại corpus và live report** bằng cột mới, chép bảng vào §1.2 (mở rộng) và
  cập nhật `docs/tasks/TASK-027-GLYPH-COVERAGE.md` §6.2 cho từng band thật sự là artifact.
* **Bước 4 — re-baseline `testdata/render-known-gaps.json` chỉ khi có số đo mới**, trong **một
  commit riêng** nêu rõ band nào chuyển từ `blank` sang `shifted` và vì sao. Không nới baseline
  trong commit sửa code — đó đúng là chiêu mà `GOOSIE-PARITYGATE-001` đang chặn.
* **Bước 5 — không sửa ngưỡng nào:** `PASS_AT`, `THRESHOLD`, `MIN_INK`, `MIN_SCORED`,
  `LIVE_COVERAGE_OK` đứng nguyên. Task này **thêm** tín hiệu, không nới tín hiệu cũ.

## 4. Nghiệm thu `[R-7]`

1. Bảng §2.1 tồn tại trong task doc, đủ 10 dòng, mỗi dòng có số đo thật. Không dòng nào "chưa đo".
2. Có ít nhất một fixture mà band vẫn `blank` sau khi thêm cửa trôi — chứng minh metric mới không
   tha cho mất nội dung thật (điều kiện cần của §2.4).
3. `make parity` vẫn xanh và **không đổi `match %`** so với trước khi sửa; nếu % đổi thì thiết kế
   sai (đã đổi nghĩa tín hiệu cũ).
4. `make live-parity-selftest` xanh; `python3 testdata/parity.py` trên corpus thật xanh.
5. Mutation proof, tối thiểu 3 cái, mỗi cái giết đúng một check:
   (i) đặt cửa trôi = 0 ⇒ case "lệch 1 dòng" **FAIL**;
   (ii) bỏ hẳn phần tử khỏi fixture với cửa trôi rộng ⇒ vẫn phải `blank`, nếu thành `shifted` thì
   **FAIL** (đây là mutation quan trọng nhất);
   (iii) đổi nghĩa giá trị trả về cũ thay vì thêm kết quả phụ ⇒ call site cũ **FAIL**.
   Chạy từng cái, khôi phục, `git status` sạch.
6. `GOOSIE-BLANKMETRIC-001` chỉ đóng khi §1.2 có đủ 10 band và mục 2 ở trên có số.

## 5. Ràng buộc vận hành

Không `git push`, không merge `main`, không worktree con. Test không cần display và **không cần
Chromium** (case tổng hợp bằng numpy/PIL như selftest hiện tại).
`go test ./internal/archtest/`, `go vet -all ./...` phải xanh (task này không đụng Go, nhưng gate
vẫn chạy). Không sửa `docs/qa-open-defects.json`, `docs/qa-test-catalog.md`, `docs/qa-ratchet.json` —
đề xuất dòng registry ghi trong báo cáo, integrator là người ghi.

**Cùng file với `TASK-036`** (`testdata/parity.py`). Hai task chạm nhau ở `score_regions` và ở
`render-known-gaps.json`. Không chạy song song; nếu buộc phải song song thì TASK-042 làm sau và
rebase lên kết quả của TASK-036, vì TASK-036 thêm equality gate vào chính baseline mà task này sẽ
re-baseline.

## 6. Ngoài phạm vi

* `GOOSIE-PARITYGATE-001` / `TASK-036` (fail-closed, `--no-strict`, equality của baseline).
* `GOOSIE-LIVE-004` (coverage > 1 và `worst_iou` không được `classify` đọc) — cùng họ "metric nói
  sai" nhưng là số khác; đừng gộp một lúc hai sửa đổi vào `classify`.
* Mọi thay đổi trong `internal/` — kể cả việc "tranh thủ" sửa line-height cho hết lệch.
* Việc full-page screenshot (fold 600px) — đó là quyết định kiến trúc của harness, ghi ở
  `TASK-041` §1.3.

## 7. Đã thực hiện + nghiệm thu đo được (2026-10-26, `worktree-task-042-blank-drift`, offline)

Không sửa engine. Không sửa `score_regions`. Thêm một tín hiệu phụ và báo cáo nó.

**Code đã đổi (3 file, không đụng `internal/`, không đụng ngưỡng nào):**
* `testdata/parity.py` — thêm `score_regions_drift()` (cửa ±bh, thanh `MIN_INK`); `score_regions`
  giữ nguyên từng byte. `FixtureResult` thêm `truly_blank_region_rows`/`shifted_region_rows`
  (report-only). `score_fixtures` gọi thêm `score_regions_drift` nhưng gate/match% vẫn dùng
  `blank_rows` cũ. Console + JSON + JUnit in thêm `shifted`/`truly_blank`.
* `testdata/parity_selftest.py` — thêm `case D1..D4` + `--drift` mode (ảnh tổng hợp numpy/PIL,
  không browser/không mạng).
* `test/gatecheck/parity_harness_test.go` — thêm `TestParityDriftMetricIsGreen` (tag `qc:BR-187`).

**Bước 1 (giữ nguyên call site):** `live-parity.py:161` vẫn `worst, coverage, blanks =
parity.score_regions(g, c)`. Vì `score_regions` không đổi, nhãn `BLANK_OUTPUT` (global-ink test) và
`MISSING_REGIONS` (đếm `blank_bands`) **giữ nguyên 100% ý nghĩa cũ**. `live-parity.py --selftest`
xanh (exit 0) sau khi sửa. Không sửa file đó.

**Nghiệm thu 3 — `make parity` xanh và KHÔNG đổi `match %`:**
Dùng `python3 testdata/parity.py` (KHÔNG chạy `make parity` vì target đó `--render --render-chromium`
sẽ ghi đè `/tmp/goosie-renders` + `/tmp/playwright-renders` là bằng chứng sống của việc khác — rule 9).
| | before | after |
| --- | --- | --- |
| exit code | 0 | 0 |
| passing | 81/81 | 81/81 |
| average match | 96.09% | 96.09% |
| known gaps | 5 fixtures / 10 bands | 5 fixtures / 10 bands |
| per-fixture (match%, status, fixture) | — | **diff rỗng: byte-identical cả 81 dòng** |
| per-fixture `blank_regions=X/Y` | — | **identical** |
Dòng mới: `drift-adjusted (GOOSIE-BLANKMETRIC-001): of those 10 blank bands, 9 have ink in a
+/-band-height window (shifted) and 1 stay blank (truly missing)`.

**Nghiệm thu 5 — mutation proofs (mỗi cái giết đúng một check, khôi phục, sha về bản tốt):**
* (i) `band_height = 0` ⇒ `case D2` "drift signal lists it as shifted" **FAIL**
  (`shifted=[] truly=[(500,511)]`), exit 1.
* (ii) `window_ink >= 0` (tha mọi pixel) ⇒ `case D4` "a deleted band is never labeled shifted" **FAIL**
  (`shifted=[(500,511)]`) + `case D3` far-away **FAIL**, exit 1. (Mutation quan trọng nhất.)
* (iii) `score_regions` trả `truly` thay vì `blanks` (đổi nghĩa giá trị cũ) ⇒ `python3 testdata/parity.py`
  **4 fixture FAIL** (`known-gap baseline is stale`: forms/04, selectors/02, unicode/02, selectors/03 —
  measured < declared) trong khi `average match` vẫn 96.09% ⇒ đúng là call site gate phụ thuộc ý nghĩa cũ.
Mỗi lần chạy xong `cp /tmp/parity.good.py` khôi phục; `grep MUTATION testdata/*.py` = none; `git status`
chỉ còn 3 file thay đổi thật.

**Lệnh kiểm tra:** `python3 testdata/parity.py` (rc 0) · `python3 testdata/parity_selftest.py`
(rc 0, "selftest green") · `python3 testdata/parity_selftest.py --drift` (rc 0, "drift selftest green") ·
`python3 testdata/live-parity.py --selftest` (rc 0) · `go test ./test/gatecheck -run TestParity` (4/4 ok,
gồm `TestParityDriftMetricIsGreen`) · `go vet -all ./...` (rc 0) · `go test ./internal/archtest/` (rc 0).

**Re-baseline `render-known-gaps.json` — KHÔNG cần, và CỐ Ý không làm:** gate vẫn dùng
`blank_regions` cũ (đếm exact-rows = 10), nên baseline 5 fixture/10 band vẫn khớp đúng → parity vẫn
xanh mà không sửa file đó. Số 9/10 drifted chỉ là **thông tin báo cáo**, chưa được đưa vào gate.
Quyết định "gate đếm theo tín hiệu drift-adjusted (⇒ baseline tụt từ 10 xuống 1)" là một **thay đổi
ý nghĩa** của release gate, nằm ngoài phạm vi task này (task chỉ THÊM tín hiệu, không nới tín hiệu cũ)
và cần một commit/quyết định riêng của integrator. Không threshold nào nhúc nhích.

**Còn chứng minh chưa đủ (thành thật):**
* Mức drift thật của 22 dòng `MISSING_REGIONS` trong sweep sống: **chưa đo** — JSON không lưu mask/ảnh,
  render lại bị cấm trên máy này. Chỉ đo được `BLANK_OUTPUT` (0/1) bằng bound aggregate.
* Cửa ±bh chỉ là heuristic hình học, chưa được đối chiếu với line-height tính từ DOM (không đụng engine).
* `html-features/01` (593,599) là `blank` thật theo metric, nhưng chưa xác minh độc lập rằng đó là mất
  render engine chứ không phải chính reference band cắt sát một dòng — cần ảnh/fixture riêng để chốt.

**Dòng registry đề xuất (integrator ghi vào `docs/qa-test-catalog.md`, rồi `go test ./test/qc -update`):**
`- **BR-187 | P2 | PARITY, INT | NOT RUN** — Đo lại tín hiệu blank-band theo cửa trôi ±band-height: một band lệch 1 dòng phải được báo `shifted` chứ không mất hẳn, band mất thật/xoá phần tử vẫn phải `blank`; `TestParityDriftMetricIsGreen` (case D1..D4, `parity_selftest.py --drift`) chứng minh `score_regions` cũ không đổi nghĩa. Defect `GOOSIE-BLANKMETRIC-001`.`
Hiện `go test ./...` có đúng MỘT lỗi thật: `test/qc TestTraceability: unknown QC ID BR-187 referenced by
test/gatecheck/parity_harness_test.go:TestParityDriftMetricIsGreen` — hệ quả tất yếu của rule 7 (không
sửa catalog). Golden `traceability.golden.json` chưa stale (BR-187 chưa là dòng catalog); nó sẽ stale sau
khi integrator thêm dòng trên, đó là lúc `-update`.
