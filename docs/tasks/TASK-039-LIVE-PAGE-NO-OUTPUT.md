# TASK-039 — họ trang thật "không vẽ gì" mà engine không nói lý do

**Priority:** P1. **Method:** PARITY (live corpus) + INT. **Owner:** chưa gán.
**Defect:** `GOOSIE-LIVE-001`.
**Liên quan:** TASK-038 (sweep sinh ra số liệu), `GOOSIE-LIVE-002` (đã có root cause riêng ở
TASK-040 — **không** gộp vào đây), `GOOSIE-LIVE-003` (không phân biệt được "chưa settle"),
`GOOSIE-LIVE-004` (metric không tách "mất nội dung" khỏi "lệch vị trí").

Đây là task **mở để đo**, không phải task để sửa. TASK-038 cố ý không đụng engine (§4 của nó),
nên toàn bộ họ lỗi này chưa có root cause. Ai nhận task này chỉ được viết code sau khi §2 trả lời
được "một lỗi hay nhiều lỗi, và lỗi nào đứng trước".

## 1. Bằng chứng đã đo

Hai lần chạy độc lập, cùng build `4de0fca`, viewport 1280×1024, DPR 1, `wait_until=load`,
cả hai phía đều screenshot **viewport** (Goosie không scroll):
`docs/qa-live-parity/2026-10-25T1630Z-4de0fca5.json` (run 1) và `…T1656Z-4de0fca5.json` (run 2,
94/100 trang so sánh được).

### 1.1 Chín trang, im lặng tuyệt đối

`goosie_rc=0`, `goosie_diag` rỗng ở **cả hai run**, không một dòng stderr:

| Trang | expect | doc_height | ink Goosie | ink Chromium | Nhãn (cả 2 run) |
| --- | --- | --- | --- | --- | --- |
| `apnews.com/` | js-revealed | 0 | 0 | 15,805 | `LAYOUT_EMPTY` |
| `www.nytimes.com/` | js-revealed | 0 | 0 | 68,129 | `LAYOUT_EMPTY` |
| `www.japantimes.co.jp/` | js-revealed | 0 | 0 | 17,749 | `LAYOUT_EMPTY` |
| `www.reuters.com/` | js-revealed | 0 | 0 | 13,145 | `LAYOUT_EMPTY` |
| `www.etsy.com/` | js-revealed | 0 | 0 | 16,539 | `LAYOUT_EMPTY` |
| `www.newegg.com/` | js-revealed | 0 | 0 | 17,467 | `LAYOUT_EMPTY` |
| `www.zara.com/` | js-revealed | 0 | 0 | 8,846 | `LAYOUT_EMPTY` |
| **`www.gov.vn/`** | **static** | 0 | 0 | **501,735** | `LAYOUT_EMPTY` |
| `vnexpress.net/` | js-revealed | 691 | **32** | 301,440 | `BLANK_OUTPUT` |

`MIN_INK = 200` (kế thừa từ `testdata/parity.py`), nên 32 px ink của vnexpress là "có vẽ, gần như
không phải trang" — một họ khác với "không vẽ gì".

### 1.2 Probe offline — họ này **không phải một lỗi**

Bản HTML do curl tải về, render bằng `file://`, không cần mạng:

| Trang | Bản curl | Render offline | Kết luận đo được |
| --- | --- | --- | --- |
| `www.gov.vn/` | 192,815 B, **11,859 ký tự text nhìn thấy được** trên 177,201 ký tự body, 26 `<script>` | **rc 0, ink 0**, `-gate -frames 120`: `doc_height=0`, `tiles_rasterized=0`, `zero_work_frames=120` | **Tái hiện được offline.** Document có nội dung tĩnh thật, engine vẫn dựng ra không gì cả. |
| `apnews.com/` | 2,610,822 B, **70,685 ký tự text**, 25 `<script>` | **rc 0, ink 763,855**, `doc_height=34,867`, `tiles_rasterized=63` | **KHÔNG tái hiện offline.** Cùng markup, cùng binary: offline thì vẽ, online thì không. |

Phân nhánh này là phát hiện quan trọng nhất của task: ít nhất **một lỗi thuộc đường fetch/response**
(không phải layout), và **một lỗi thuộc đường parse/layout** (gov.vn). `expect: js-revealed` trong
corpus **không phải** lời giải thích cho apnews — HTML của nó chứa 70,685 ký tự nội dung tĩnh.

### 1.3 Bẫy đo đã ghi nhận, người sau không được lặp lại

* **`style_passes` và `layout_passes` đọc 0 trên cả hai case**, kể cả apnews nơi layout rõ ràng
  đã chạy (`doc_height=34,867`). Đó là counter cửa sổ frame, **không được** dùng làm bằng chứng
  "engine chưa style". (Xem `internal/frame/recorder.go` — các trường này window-scoped.)
* **Hai chế độ có thể mâu thuẫn nhau trên cùng một URL, cùng một run.** Run 1 của `nytimes.com`:
  `-gate` báo `doc_height=0` trong lúc `-screenshot` paint ra **268,190 px ink**. Nhãn
  `STRUCTURE_DISAGREES` của TASK-038 sinh ra đúng vì dòng này. Không được kết luận "layout rỗng"
  từ một chế độ duy nhất.
* **Chromium reference cũng dao động trên trang JS-revealed.** vnexpress: run 1
  `ink_chromium=627,498`, `doc_height=847`; run 2 `ink_chromium=301,440`, `doc_height=691`.
  Nghĩa là một phần "noise" nằm ở phía chuẩn, không ở engine. Bất kỳ kết luận nào cũng phải nêu
  số của **cả hai** run, không chọn một.

## 2. Câu hỏi phải trả lời trước khi sửa code

Mỗi câu là một phép đo, không phải một suy đoán; thứ tự ưu tiên đã xếp theo chi phí/thông tin.

1. **Goosie nhận được gì trên đường wire?** So tài liệu Goosie fetch (thêm log hoặc dùng
   `internal/net` trong một probe nhỏ) với 2,610,822 B của curl cho apnews: cùng độ dài? cùng
   redirect chain? cùng encoding (br/gzip)? cùng `Set-Cookie`? — Nếu khác, đây là lỗi fetch, và
   mọi giả thuyết layout bên dưới đều vô nghĩa.
2. **Chuyện gì xảy ra với document Goosie đã parse?** Với apnews online: `doc_height` của chế độ
   `-gate` là bao nhiêu, có PNG không, có dòng `goosie_diag` nào? Ba số đó tách "fetch sai" khỏi
   "parse ra document rỗng" khỏi "JS làm sập session".
3. **JS có ném gì không?** `internal/js` là runtime goja per-document; một exception không bắt được
   có thể kết thúc script trước khi nội dung hiện ra. Đo: bật log exception, chạy lại 8 trang
   js-revealed, đếm trang nào có throw. Chưa đo thì chưa được nói "tại JS".
4. **gov.vn — vì sao layout rỗng khi markup có 11,859 ký tự?** Đây là repro offline **duy nhất**
   hiện có, và cũng là case đáng giá nhất vì nó không cần mạng, không cần JS, không phụ thuộc
   trang sống. Bước đầu: in số node DOM, số box arena, và `Session` dựng có lỗi ở bước nào
   (parse / scripts / stylesheet collection / cascade / layout).
5. **vnexpress — vẽ 32 px là vẽ cái gì?** Crop vùng có ink và so với band tham chiếu; 32 px có thể
   là một glyph duy nhất, tức pipeline chạy tới paint rồi mới chết, tức họ khác hẳn `LAYOUT_EMPTY`.
6. **Bao nhiêu trang nằm sau họ này?** `make live-parity --only <substring>` (TASK-038 §2.6) để
   đo lại từng họ mà không gọi lại 100 máy chủ.

## 3. Plan (chỉ điền sau khi §2 xong)

* **Bước 0:** dựng fixture tái hiện được cho **từng** nhánh mà §2 tách ra được, đặt trong
  `testdata/render/` — trang sống là nơi *phát hiện* lỗi, không phải nơi *neo* lỗi. Fixture phải
  có PNG Chromium đo thật, không tự vẽ, và theo đúng kỹ thuật `testdata/live-sites.json` đã dùng.
* **Bước 1:** một defect cho mỗi nhánh root cause, mỗi cái một commit, mỗi commit có mutation
  proof [R-7] riêng. Không "sửa luôn" những chỗ chưa có oracle.
* **Bước 2:** nếu một nhánh hoá ra là "đúng theo thiết kế" (ví dụ engine chưa hỗ trợ tính năng mà
  các trang này dùng), thì **ghi vào `docs/roadmap-v2.md` và để defect mở**, không hạ mức nghiêm
  trọng của nó xuống.
* **Cấm trong task này:** sửa `testdata/parity.py`, `PASS_AT`, `MIN_INK`, `MIN_SCORED`, ngưỡng
  admission (việc của TASK-040), deadline 25 s (việc của `GOOSIE-LIVE-003`).

## 4. Điều kiện nhận

1. Mỗi nhánh đã tách ở §2 có: một fixture offline, một oracle, một mutation proof giết đúng check
   của nó, và một dòng trong `docs/qa-open-defects.json` nói rõ nhánh nào còn mở.
2. `GOOSIE-LIVE-001` chỉ được đóng khi **số trang thật so sánh được** tăng lên và số liệu hai run
   được chép vào §5 — không đóng bằng "unit test xanh".
3. `make live-parity-selftest`, `make parity`, `go test ./...`, `go vet -all ./...`,
   `go test ./internal/archtest/` xanh; `make live-parity` chạy một mình (không song song sweep
   khác — `GOOSIE-FLAKE-001`), `uptime` kiểm trước.
4. Nếu task này làm thay đổi phân loại của harness, phải cập nhật `testdata/live-parity.py`
   **và** selftest tương ứng, không để nhãn cũ mô tả sai hành vi mới.

## 5. Kết quả

*(điền sau khi chạy — số trang thật so sánh được trước, tỷ lệ sau)*

## 6. Ngoài phạm vi

* `cafef.vn` / admission guard → TASK-040.
* Flag phân biệt "chưa settle" → `GOOSIE-LIVE-003`.
* Metric vị trí (worst_iou chưa được dùng để chấm) → `GOOSIE-LIVE-004`.
* Full-page screenshot, scroll pipeline, web fonts, cookie overlay — mỗi họ một task riêng sau
  khi có số (đúng như TASK-038 §5 đã hẹn).
