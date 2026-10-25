# TASK-021: Region scoring cho render parity — phát hiện nội dung trống mà oracle toàn-cửa-sổ bỏ qua

- **Task ID**: `TASK-021`
- **Type**: Test quality / Visual oracle
- **Assigned to**: `codex` (sau khi TASK-019 nghiệm thu)
- **Review**: `qoder cli` (integrator)

## 1. Vấn đề đo được, không phải suy đoán

`testdata/parity.py` hiện chấm **một tỷ lệ trên toàn viewport** (`score_fixtures`, dòng 235-243: `np.abs(g-p) <= threshold` rồi `np.mean`) và gate ở `PASS_AT = 90.0`. Trang thái hiện tại trên `main`: **80/80 fixture PASS**, `average match: 96.09%`, thấp nhất `90.12%`.

Ba defect nhân tạo dựng trên chính ảnh Chromium tham chiếu (viewport 800x600), đo bằng oracle hiện tại:

| Defect dựng | Ý nghĩa với QA manual | Điểm toàn viewport | Phán xử với PASS_AT=90 |
| --- | --- | --- | --- |
| Lệch toàn nội dung dưới `y=120` lên 8px (một dòng sai chiều cao) | "dòng bị chồng/lệch" | 89.2 / 90.3 / 92.6 / 98.7 (4 fixture) | 3/4 **PASS** |
| Xoá hẳn một heading 200x40 (tô trắng) | "mất tiêu đề" | 98.33 – 99.85 | **PASS** cả 4 |
| Mất dải 150px cuối | "mất nội dung cuối trang" | 92.2 / 98.1 / 99.75 / 100.0 | **PASS** cả 4 |

Kết luận: **một heading biến mất chỉ làm giảm 0.15–1.7 điểm.** Oracle hiện tại không phân biệt được "chữ render khác đi một chút" và "nội dung không tồn tại".

Và đây không phải lỗi lý thuyết. Dùng thang đo khác — so **khối mực ink** theo từng dải nội dung (band), không so từng pixel — trên đúng ảnh goosie hiện tại:

```
bands total 566 ; blank-in-goosie bands 18 (3.2%) ; fixtures có >=1 vùng trống: 5
  unicode/02-emoji-symbols.png        9 dải trống
  selectors/02-pseudo-classes.png     3
  selectors/03-pseudo-elements.png    3
  forms/04-advanced-inputs.png        2
  html-features/01-forms-complete.png 1
```

Ví dụ cụ thể: `unicode/02-emoji-symbols.png` dòng 113-132, Chromium 3036 pixel mực, goosie **0**; `selectors/03-pseudo-elements.png` dòng 408-429, Chromium 1493, goosie 18. Cả hai fixture này đang được báo **PASS 94.4% và 90.3%**. Tăng dung sai dilate lên 3px không cứu được (IoU vẫn 0.000) → đây là **mất nội dung**, không phải lệch vị trí.

QA manual nhìn vào trang sẽ báo ngay: emoji không hiển thị, `::before/::after` không sinh nội dung, một số form control không vẽ. Parity gate nói "100% passing".

## 2. Thang đo và những gì **không** được làm gate

Đã đo trên dữ liệu thật (n=80 fixture, band = nhóm dòng liên tiếp có mực, gap ≤ 6px, cao ≥ 6px, cắt theo extent mực của Chromium trong band; `ink_iou` = `(a∩dilate(b) + b∩dilate(a)) / (a+b)`, dilate 1px):

- Nhạy defect: clean `1.000` → lệch 8px `0.221–0.439` → mất nửa dưới `0.000`.
- **NHƯNG** trên goosie thật: median của min-band IoU chỉ `0.906`, pct-10 là `0.150`, và `39/80` fixture có min-band IoU < 0.9 — do rasterizer/hinting/khoảng cách glyph khác Chromium, **không phải bug**.

Vì vậy, quy tắc cứng cho task này:

1. **Gate mới = blank-region**: band mà Chromium có ink (`ink_px >= MIN_INK`, đề xuất 200) mà goosie **không có gì** (`ink_px == 0`) → FAIL. Tiêu chí này sắc, ít nhiễu, và hôm nay chỉ chạm 5 fixture / 18 band.
2. `worst_region_iou` và `coverage_ratio` chỉ được **báo cáo** (JSON/JUnit/summary), chưa dùng để fail. Muốn nâng lên gate phải có số liệu calibration riêng, ghi trong Status.
3. `PASS_AT` toàn viewport giữ nguyên — không nhân bản thêm một oracle yếu thứ hai, cũng không âm thầm siết để rồi phải nới.

## 3. Việc cụ thể

1. `testdata/parity.py`: thêm hàm tách band + `ink_iou`/`ink_px` như định nghĩa ở mục 2 (viết gọn, comment giải thích *tại sao* so khối mực chứ không so pixel). `FixtureResult` thêm `worst_region_iou`, `blank_regions` (số lượng) và danh sách dải dòng bị trống; `to_dict`/`write_junit_report`/`print_summary` xuất tương ứng.
2. Điểm quyết định PASS/FAIL: giữ `pct >= pass_at` **và** thêm `blank_regions == 0` (trừ khi khớp baseline ở bước 3).
3. **Baseline có chủ đích, không phải checkbox**: file `testdata/render-known-gaps.json` liệt kê 5 fixture + số band trống đang có, kèm đường dẫn tới task sẽ sửa engine. Fixture **mới** có band trống thì CI đỏ ngay; fixture trong baseline vẫn in cảnh báo và số band thực đo, để không cho phép số band trống **tăng** (so với con số đã ghi).
4. In bảng 5 fixture đang bị trống vào Status, mỗi dòng một ghi chú ngắn "nội dung nào biến mất" (đọc fixture HTML để đoán: emoji font fallback? `::before`? `<input>` type mới?). **Không sửa engine trong task này.**
5. Test cho chính scorer: `testdata/` là Python, nên thêm `testdata/test_parity_regions.py` (pytest hoặc `unittest`, chạy bằng `python3 -m unittest` nếu repo chưa có pytest) với fixture numpy tự dựng chứng minh: (a) band trống → phát hiện; (b) glyph khác màu nền một chút trong band → không phát hiện; (c) band < `MIN_INK` → bỏ qua. Không có test thì scorer mới chỉ là một con mắt chưa được kiểm thị lực.
6. Không đổi `Makefile`/`ci.yml` ngoại trừ khi `--junit`/`--json` cần thêm field (vẫn tương thích consumer cũ).

## 4. Tiêu chí nghiệm thu

- **[AC-01] Mutation proof của oracle mới.** Với một fixture trong baseline, tạm làm goosie không vẽ một vùng có nội dung (hoặc vá trực tiếp ảnh render trong `/tmp/goosie-renders` để repro): scorer **phải FAIL**; khôi phục → PASS. Dán cả hai lượt output.
- **[AC-02]** Chạy `python3 testdata/parity.py --render --binary ./bin/goosie` (hoặc không `--render` nếu đã có ảnh) trên `main` hiện tại: **80/80 vẫn PASS** nhờ baseline, và summary in rõ "known gaps: 5 fixtures / 18 bands".
- **[AC-03]** Thêm một fixture giả có band trống **không** nằm trong baseline → scorer FAIL, exit != 0.
- **[AC-04]** `python3 -m unittest testdata/test_parity_regions.py` (hoặc lệnh test bạn chọn) PASS.
- **[AC-05]** `go test -count=1 ./...` không đổi kết quả so với `main` (task này không đụng Go); `go vet -all ./...` sạch.
- **[AC-06]** Status nêu **giới hạn còn lại** của thang đo: ví dụ "band trống bắt được nội dung mất hẳn, nhưng nội dung sai vị trí dưới 6px hoặc sai chữ thì vẫn chỉ có pixel-score toàn viewport — chưa đủ để sign-off CUA case BR-180".

## 5. Ràng buộc vận hành

- Chỉ sửa `testdata/parity.py`, `testdata/render-known-gaps.json` (mới), `testdata/test_parity_regions.py` (mới). **Cấm** `internal/**`, `cmd/**`, `test/**`, `docs/qa-*`, `Makefile`, `.github/**`.
- Không sửa engine để band trống biến mất. Mỗi vùng trống là một bug **phải được ghi thành task riêng**, không phải thứ để che.
- Chỉ commit trên branch `worktree-task-021-parity-regions`. Không `git push`, không merge `main`.
- sandbox codex: Playwright/Chromium có thể không chạy được; task này chỉ cần ảnh render đã có trong `/tmp/goosie-renders` và `/tmp/playwright-renders` (integrator đã tạo). Nếu thiếu, ghi rõ lệnh nào không chạy được.

## 6. Việc phái sinh (integrator giao sau, không nằm trong task này)

5 vùng trống thật cần engine fix, và mỗi cái là một QC case thật:
- `unicode/02-emoji-symbols.png` (9 band): font fallback cho emoji.
- `selectors/02-pseudo-classes.png`, `selectors/03-pseudo-elements.png` (6 band): nội dung `::before`/`::after`.
- `forms/04-advanced-inputs.png`, `html-features/01-forms-complete.png` (3 band): form controls.

## Status

Implemented in `testdata/parity.py`: row bands use Chromium ink runs (gap ≤6 rows, height ≥6), horizontal bounds are taken from the reference ink, and `MIN_INK=200`. Ink pixels are RGB values with a channel below 245. One-pixel dilation is used for the reported tolerant ink IoU. Only a completely empty Goosie band is a new gate; `worst_region_iou` and `coverage_ratio` are emitted as diagnostics in console, JSON, and JUnit. `PASS_AT=90` is unchanged. The baseline allows each existing fixture up to its recorded count and makes any increase or any blank region on a new fixture fail.

The Chromium and Goosie images already present under `/tmp/playwright-renders` and `/tmp/goosie-renders` were used. No Playwright rerender was needed; `--render-chromium` was not run.

### Existing blank regions and fixture-based hypotheses

| Fixture | Blank bands | Observed row ranges | Hypothesis from fixture HTML |
| --- | ---: | --- | --- |
| `unicode/02-emoji-symbols.png` | 9 | 113–132, 155–174, 240–254, 282–301, 368–385, 409–428, 451–470, 496–509, 579–598 | Emoji sequences (faces, gestures, objects, flags and symbols) likely lack a usable emoji font fallback. |
| `selectors/02-pseudo-classes.png` | 3 | 471–487, 506–526, 551–569 | Later structural selector examples and the `:empty` examples may be missing or not styled/generated as expected. |
| `selectors/03-pseudo-elements.png` | 3 | 457–475, 539–554, 579–599 | The star/checkmark, quotation mark, generated-content, and list marker examples suggest missing `::before`/`::after` content in later sections. |
| `forms/04-advanced-inputs.png` | 2 | 376–388, 443–454 | Some advanced native controls (date/time, color, range or file) may be absent. |
| `html-features/01-forms-complete.png` | 1 | 593–599 | The final form row is at the viewport edge; a submit/reset/control or its clipped label may be missing. |

These are visual hypotheses from the fixture source, not root-cause findings. The baseline points to the proposed follow-up `docs/tasks/TASK-023-RENDER-REGION-GAPS.md`; that follow-up task is outside this change.

### Acceptance evidence

| AC | Command / method | Result |
| --- | --- | --- |
| AC-01 | Mutation of existing `unicode/02-emoji-symbols.png` Goosie render at rows 197–216; rerun scorer; restore original bytes and rerun | Mutation FAIL (10/9 bands, exit 1); restored PASS (9/9, exit 0). Exact output below. |
| AC-02 | `python3 testdata/parity.py` using the pre-rendered images | 80/80 PASS; summary reports 5 fixture / 18 band known gaps. Output below. |
| AC-03 | Add a temporary synthetic HTML fixture and 800×600 PNG pair with one blank 10×25 reference band; run `python3 testdata/parity.py`; remove the temporary files | Synthetic fixture FAIL at 99.95% pixel match, 1/0 blank bands; process exit 1. Exact scorer lines below. |
| AC-04 | `python3 -m unittest testdata/test_parity_regions.py` | PASS, 3 tests. |
| AC-05 | `go test -count=1 ./...`; `go vet -all ./...` | PASS, exit 0 for both. Initial sandbox run of `go test` hit blocked localhost/network operations; rerun with the approved unsandboxed permission completed successfully. No Go files changed. |
| AC-06 | Limit of the added gate | It detects substantial completely blank content bands; small positional shifts and incorrect glyphs can still pass the full-viewport pixel score. This does not sign off CUA case BR-180. |

#### AC-01 exact output

```text
mutation blank band: unicode/02-emoji-symbols.png FAIL; blank_regions=10/9; rows=[(113, 132), (155, 174), (197, 216), (240, 254), (282, 301), (368, 385), (409, 428), (451, 470), (496, 509), (579, 598)]; exit=1
restored image: unicode/02-emoji-symbols.png PASS; blank_regions=9/9; rows=[(113, 132), (155, 174), (240, 254), (282, 301), (368, 385), (409, 428), (451, 470), (496, 509), (579, 598)]; exit=0
```

#### AC-02 summary output

```text
  80/80 passing  (100.0%), 0 failing, 0 errors
  average match: 96.09%  (threshold=30, pass_at=90.0%)
  known gaps: 5 fixtures / 18 bands
  WARNING known gap: forms/04-advanced-inputs.png: 2 blank band(s), baseline 2; rows=[(376, 388), (443, 454)]
  WARNING known gap: html-features/01-forms-complete.png: 1 blank band(s), baseline 1; rows=[(593, 599)]
  WARNING known gap: selectors/02-pseudo-classes.png: 3 blank band(s), baseline 3; rows=[(471, 487), (506, 526), (551, 569)]
  WARNING known gap: unicode/02-emoji-symbols.png: 9 blank band(s), baseline 9; rows=[(113, 132), (155, 174), (240, 254), (282, 301), (368, 385), (409, 428), (451, 470), (496, 509), (579, 598)]
  WARNING known gap: selectors/03-pseudo-elements.png: 3 blank band(s), baseline 3; rows=[(457, 475), (539, 554), (579, 599)]
```

#### AC-03 exact output

```text
   99.95%  FAIL  .task021-synthetic/blank.png  worst_region_iou=0.0  coverage_ratio=0.0  blank_regions=1/0  rows=[(100, 109)]
  80/81 passing  (98.8%), 1 failing, 0 errors
    .task021-synthetic/blank.png  (99.95%)
exit=1
```

#### AC-04 exact output

```text
...
----------------------------------------------------------------------
Ran 3 tests in 0.001s

OK
```

#### AC-05 output

`go test -count=1 ./...` completed with exit 0 after permission was granted for the suite's localhost and network tests. `go vet -all ./...` completed with exit 0 and no diagnostics.
