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

## 2. Phải đo trước khi sửa

1. **Phân loại lại đủ 10 band đang khai báo**, không chỉ 3 band của `selectors/03`. Với mỗi band:
   ghi `(số px trong band, số px trong cửa ±band_height, dòng gần nhất có ink)`. Kết quả phải vào §1
   dưới dạng bảng, đo bằng code chứ không bằng mắt.
2. **Bao nhiêu `BLANK_OUTPUT` của sweep sống là drift?** `docs/qa-live-parity/2026-10-25T1656Z-4de0fca5.json`
   có sẵn ink mask; chạy lại classifier cũ trên dữ liệu cũ, chỉ thêm cột drift. Con số này quyết
   định mức độ khẩn cấp của task — hiện chưa biết.
3. **Ngưỡng nào là nguyên tắc, không phải chỉnh theo số?** Cửa trôi phải đến từ một đại lượng có
   nghĩa sẵn (chiều cao band, hoặc line-height của chính band đó), **không** phải một hằng số mới
   được chọn cho tới khi báo cáo đẹp. Viết rõ công thức và lý do chọn trước khi chạy trên corpus.
4. **Có trường hợp "mất thật" mà cửa trôi cũng che được không?** Tìm một mutation chủ ý (xoá hẳn
   một phần tử khỏi fixture) và chứng minh nó vẫn bị báo `absent`, không bị tha thành `shifted`.
   Không có phép thử này thì mọi nới lỏng đều vô hại hoá metric.

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
