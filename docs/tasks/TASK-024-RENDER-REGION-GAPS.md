# TASK-024: Năm defect nội-dung-trống mà region oracle vừa lộ ra (intake)

- **Task ID**: `TASK-024`
- **Type**: Engine defect intake
- **Assigned to**: chưa chia (mỗi dòng một task engine riêng khi có người)
- **Review**: `qoder cli` (integrator)
- **Nguồn**: `testdata/parity.py` sau TASK-021. Baseline: `testdata/render-known-gaps.json`.

## 1. Đây là cái gì

TASK-021 thêm một oracle mới vào render parity: so **khối mực** theo từng dải nội dung, thay vì chỉ đếm pixel khớp trên toàn viewport. Oracle cũ báo **80/80 fixture PASS** trong khi goosie không vẽ gì ở 18 dải thuộc 5 fixture. Một heading 200x40 bị tô trắng hoàn toàn chỉ làm điểm toàn viewport giảm 0.15–1.7 trên thang 100 — nghĩa là loại lỗi "mất hẳn nội dung" vô hình với gate cũ.

18 dải này **không phải giả thuyết**. Mỗi dòng dưới đây là một khoảng dòng mà Chromium có ≥200 pixel mực và goosie có **0**. `--render` lại không đổi kết quả: đây là output của engine, không phải của ảnh cache.

| Fixture | Dải trống (rows) | Nghi vấn từ fixture HTML | Ưu tiên đề xuất |
| --- | --- | --- | --- |
| `unicode/02-emoji-symbols.png` | 113–132, 155–174, 240–254, 282–301, 368–385, 409–428, 451–470, 496–509, 579–598 (9) | Emoji faces/gestures/objects/flags không có font fallback → ô vuông rỗng hoặc không gì cả | P1: văn bản quốc tế là nội dung người dùng đọc |
| `selectors/03-pseudo-elements.png` | 457–475, 539–554, 579–599 (3) | Nội dung sinh bởi `::before`/`::after` (dấu sao, chấm, quotation, marker) không xuất hiện | P1: `content` là tính năng CSS có thật trong catalog |
| `selectors/02-pseudo-classes.png` | 471–487, 506–526, 551–569 (3) | Các ví dụ selector cấu trúc ở phần cuối và `:empty` | P2 |
| `forms/04-advanced-inputs.png` | 376–388, 443–454 (2) | Control native type mới (date/time, color, range, file) không được vẽ | P1: form là đường cùng của người dùng |
| `html-features/01-forms-complete.png` | 593–599 (1) | Dòng form cuối sát mép viewport: thiếu control hoặc nhãn bị clip | P2 |

Nghi vấn chỉ là **đoán từ HTML fixture**, chưa phải root cause. Task engine thật phải bắt đầu bằng việc xác nhận nguyên nhân, không phải vá theo bảng này.

## 2. Cách tái hiện từng dòng

```bash
make build
python3 testdata/parity.py --render --binary ./bin/goosie
# in ra: "WARNING known gap: <fixture>: N blank band(s), baseline N; rows=[...]"
```

Muốn nhìn tận mắt: `python3 testdata/parity.py --inspect <fixture> --band 32` sinh ảnh diff side-by-side (goosie | chromium | diff), hoặc mở trực tiếp `/tmp/goosie-renders/<fixture>` cạnh `/tmp/playwright-renders/<fixture>`.

## 3. Quy tắc cho baseline (phần dễ bị gian lận nhất)

`testdata/render-known-gaps.json` ghi `{fixture, blank_regions, follow_up_task}` cho **5** fixture. `parity.py` fail khi số dải trống **vượt** con số đã ghi, và fail với mọi fixture **không** có trong baseline. Nghĩa là:

- **Được phép**: số trong baseline **giảm** khi engine được sửa (kèm commit sửa engine, và `follow_up_task` đổi thành "đã sửa" hoặc xoá dòng đó).
- **Không được phép**: tăng số trong baseline; thêm fixture mới vào baseline như cách "qua gate"; hạ `MIN_INK` hay nâng `BAND_GAP`/`MIN_BAND_HEIGHT` để dải trống biến mất về mặt đo lường; sửa hoặc xoá fixture HTML để nội dung không còn được yêu cầu. Bốn cách cuối đều làm oracle mù đi đúng chỗ nó vừa mở mắt.
- Nếu một defect là **hành vi đúng** (ví dụ: fixture mong đợi emoji mà máy chủ CI không cài font emoji), thì phải chứng minh bằng cơ chế khác (kịch bản render có font chỉ định), không phải ghi nó vào baseline như một sự thật vĩnh viễn.

## 4. Việc phái sinh khi nhận từng dòng

Mỗi defect nên là một task riêng, tự tái hiện bằng `--inspect`, và kết thúc bằng việc baseline **giảm** — đó là nghiệm thu, không phải "đã điều tra xong". Ba ứng viên nối vào catalog QC: emoji/font fallback, `::before`/`::after` generated content, và form controls type mới. Khi task engine landed, tag test của nó vào case tương ứng rồi `go test -count=1 -run TestTraceability ./test/qc/ -update` để dấu vết QC đi theo bằng chứng thật.

## 5. Trạng thái

Intake này được tạo khi TASK-021 landed (`bdbddb4`). Chưa có defect nào ở trên được sửa. Chưa có dòng nào được sign-off CUA: người xem vẫn cần mở ảnh diff — oracle mới chỉ chứng minh "có nội dung biến mất", chưa chứng minh "vì sao".


## 6. Phân loại đã xác nhận bằng probe (integrator, 2026-10-25)

Cột "nghi vấn" ở §1 là **đoán từ HTML**, và có một dòng đoán sai. Tất cả đã được kiểm lại bằng render tối giản (`/tmp/probe/`) và ảnh crop side-by-side (`/tmp/blankbands/`). 18 band quy về **3 nguyên nhân + 1 artifact**:

| Nguyên nhân | Band | Probe chứng minh | Task |
| --- | --- | --- | --- |
| **A. Glyph coverage thiếu, và glyph mất bị biến thành khoảng trắng** | **12** (9 `unicode/02` + 3 `selectors/03`) | `/tmp/probe/b.png`: `→` (U+2192) và `∑` (U+2211) vẽ bình thường, nhưng `✔` (U+2714), `✓` (U+2713) và emoji U+1F000+ **mất hẳn**, để lại đúng một khoảng trắng. Doc comment `internal/raster/fonts.go:406-409` nhận: *"no face has such a glyph, which the text path treats as whitespace rather than an error"*. Chuỗi fallback (`fonts.go:357`) chỉ gồm các slot Times/Arial/Courier/Georgia/Verdana/Helvetica — không face nào phủ Dingbats hay emoji. | `TASK-027-GLYPH-COVERAGE.md` |
| **B. Form control UA appearance** | **5** (2 `forms/04` + 3 `selectors/02`, trong đó 1 band chưa chắc chắn) | `/tmp/probe/d.png`: `color`, `range`, `date`, `month`, `week` **không vẽ gì cả**; `text`/`number` vẽ giá trị nhưng không có border; checkbox/radio có đường vẽ riêng. Chỗ chặn: `internal/paint/builder_controls.go:87` special-case 5 type, `builder_box.go:119` chỉ vẽ checkbox/radio — các type còn lại rơi vào cả hai nhánh đều không ai vẽ. Lệch nghiêm trọng: `internal/engine/accessibility.go:149-150` **có** biết đủ các type, tức AX tree báo "có control" khi màn hình trống. | `TASK-028-FORM-CONTROL-APPEARANCE.md` |
| **C. Artifact của fold** | **1** (`html-features/01`, rows 593-599, cao 7px sát mép viewport 600) | Chromium để lộ đúng đường viền trên của một box khi hết khung hình; không có nội dung nào biến mất. | **Không mở task engine.** Giữ nguyên trong baseline, kèm lý do. |

**Sai lầm trong §1 cần ghi lại để đừng lặp lại**: `selectors/03-pseudo-elements.png` bị nghi là `::before`/`::after` không hoạt động. Probe `/tmp/probe/a.png` chứng minh generated content vẽ đúng (`BEFORE MIDDLE AFTER`, `PLAIN AFTER`). Ba band của fixture đó thực chất là `content:"✔"` — **cùng nguyên nhân A**, không phải cascade. Nếu không probe mà đã đi viết task cho `::before`, sẽ có một engine task hoàn toàn vô nghĩa và một font bug vẫn còn nguyên.

Hệ quả cho baseline: sau khi TASK-027 landed, `unicode/02` và `selectors/03` phải **xoá khỏi** `render-known-gaps.json` (12 band); sau TASK-028, `forms/04` xoá, `selectors/02` giảm còn tối đa 2; `html-features/01` ở lại với 1 band và lý do ở dòng C.
