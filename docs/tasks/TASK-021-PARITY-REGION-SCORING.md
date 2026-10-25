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
