# TASK-040 — một custom property name đúng lệ làmทั้ง trang bị từ chối (geometry cap)

**Priority:** P1. **Method:** INT + PARITY (fixture mới trong `testdata/render/`).
**Owner:** chưa gán. **Defect:** `GOOSIE-LIVE-002`.
**Liên quan:** `GOOSIE-LIVE-003` (message của admission guard), TASK-038 (sweep sinh ra defect này),
`internal/engine/limits.go` (nơi sửa), `internal/css/token.go` (nguồn token sai).

Task này **đã có root cause đo được**, khác phần lớn task trong repo — bằng chứng ở §1 là
chain bisection chạy lại được từng bước, không phải suy đoán. Vì vậy plan ở §3 ngắn hơn thường lệ
và trọng tâm dồn vào §2 (việc nào chưa đo được) và §4 (điều kiện nhận).

## 1. Bằng chứng đã đo

Nguồn: `docs/qa-live-parity/2026-10-25T1656Z-4de0fca5.json` (sweep 100 trang, TASK-038) và
các probe chạy offline trên `bin/goosie` cùng build (`4de0fca`), **không cần mạng**.

`https://cafef.vn/` → `ADMISSION_CAPPED`, `goosie_rc=1`, **không có PNG nào**:

```
goosie: goosie: build session: CSS numeric value outside safe geometry range
```

Chuỗi bisection, mỗi bước là một lệnh chạy lại được:

| Bước | Phép thử | Kết quả đo |
| --- | --- | --- |
| 1 | `file:///tmp/cafef.html` (bản curl 217,170 B) | rc 1, đúng một dòng stderr như trên |
| 2 | Xoá cả 9 khối `<style>` | **rc 0** ⇒ thủ phạm nằm trong CSS của document |
| 3 | Xoá mọi `style="…"` attribute | rc 1 ⇒ **không** phải inline style attribute |
| 4 | Tách 9 khối `<style>`, render từng khối một mình | chỉ **khối 8** (7,659 B) cho rc 1 |
| 5 | Tách 36 rule trong khối 8, render từng rule | chỉ rule 29: `.box-ttdn .list .mdn .time { color: var(--8e8e8e, #8e8e8e); … }` |
| 6 | Đổi `--8e8e8e` → `--grey` trong **cả document** | **rc 0, ảnh paint 554,999 px ink** |

Repro tối thiểu (mỗi cái là một file HTML độc lập, đã chạy trên máy này):

```css
:root{--8e8e8e:#8e8e8e}                     /* rc 1 */
.a{color:var(--8e8e8e)}                     /* rc 1 */
.a{color:var(--8e8e8e, #8e8e8e)}            /* rc 1 */
.a{color:#8e8e8e}                           /* rc 0  — Hash token, không liên quan */
```

Máy Chromium đo trên cùng repro tối thiểu: `getComputedStyle(...).color` =
`rgb(171, 205, 239)` — tên custom property bắt đầu bằng chữ số là hợp lệ và **không chứa số nào**,
nên đây là false positive, không phải trang tấn công.

### 1.1 Cơ chế, đo bằng tokenizer (probe `internal/css`, đã xoá sau khi đo)

Tokenizer trên chính các chuỗi đó:

```
--8e8e8e   -> Number    value="-"    unit=""        numval=-0
--8e8e8e   -> Dimension value="-8e8" unit="e8e"     numval=-8e+08     ← thủ phạm
--8e8      -> Number    value="-"    unit=""        numval=-0
--8e8      -> Number    value="-8e8" unit=""        numval=-8e+08     ← plain number, cap bỏ qua
--8e5e5    -> Dimension value="-8e5" unit="e5"      numval=-800000    ← dưới cap
--abc8e8e8 -> Dimension value="-"    unit="abc8e8e8" numval=-0        ← magnitude 0
#8e8e8e    -> Hash      value="8e8e8e"                                ← không phải số
```

`internal/engine/limits.go:239-241` áp `MaxGeometry = 2<<20` (= 2,097,152, khai báo tại `:53`)
cho `TokenDimension`/`TokenPercentage`/`TokenNumber` có đơn vị ⇒ `|−8e8| = 8×10⁸` vượt cap ⇒
**cả document bị từ chối**. Matrix hành vi đo được khớp chính xác từng dòng:

| Input | rc | Lý do theo token đã đo |
| --- | --- | --- |
| `:root{--8e8:1}` | 0 | Number thuần, cap chủ ý bỏ qua (`limits.go:233-238`) |
| `:root{--8e8e8:1}` | 1 | Dimension −8e8 → vượt cap |
| `:root{--1e9e9:1}` | 1 | Dimension −1e9 → vượt cap |
| `:root{--8e5e5:1}` | 0 | Dimension −800,000 → dưới cap |
| `:root{--1e2e2:1}` | 0 | Dimension −100 → dưới cap |
| `:root{--abc8e8e8:1}` | 0 | Dimension numval −0 → magnitude 0 |
| `.a{width:8e8}` | 0 | Number thuần |
| `.a{width:8e8e8px}` | 1 | Dimension 8e8 — **đây mới là thứ cap sinh ra để chặn** |

Hai giả thuyết đã bị chính số liệu bác bỏ (ghi lại để người sau khỏi đoán lại):

* "chắc là số 7+ chữ số đâu đó (z-index khổng lồ)" — thay **mọi** số ≥7 chữ số trong document
  vẫn rc 1; thủ phạm chỉ dài 8 ký tự và **không có dãy chữ số nào**.
* "chắc là stylesheet ngoài" — document không tải stylesheet ngoài nào; toàn bộ CSS nằm trong
  9 khối `<style>`.

## 2. Việc chưa đo được — phải đo trước khi sửa

1. **Token `Dimension` này có đúng theo CSS Syntax không?** `--8e8e8e` là một ident hợp lệ
   (starts-with `--`), không phải số. Nhưng trước khi kết luận "tokenizer sai", phải **đo thêm
   họ lân cận**: `--1e5x`, `--5px`, `-8e8e8`, `1e1e1`, và một custom property name có dấu gạch
   giữa (`--a-8e8e8`). Ghi kết quả vào §1 bằng chính format bảng trên.
2. **Số bất thường thứ hai trong output probe:** token đầu của `--8e8e8e` là `Number` với
   `value="-"`, `numval=-0` — một dấu `-` đứng một mình được phát ra như *số*, và token thứ hai
   `"-8e8"` **lặp lại chính dấu `-` đó** (span chồng lấn). Chưa đo xem `cssPropertyAt`
   (`limits.go:254`) quy thuộc property thế nào khi offset chồng lấn như vậy — có thể cùng một
   bug làm cả hai: cap đọc sai property, và `unicode-range` exemption (`limits.go:214-220`) chỉ
   hoạt động tình cờ. Đo trước, rồi hãy quyết định sửa tokenizer hay chỉ sửa guard.
3. **Có bao nhiêu trang sống nằm trong họ này?** `--8e8e8e` (tên đặt theo màu) là kiểu viết rất
   phổ biến. Chưa đo mẫu, nên chưa thể nói đây là 1 trang hay 30 trang.

## 3. Plan (chỉ làm sau khi §2 xong)

* **Bước 0 — bắt buộc, code trước khi sửa:** thêm oracle cho từng dòng bảng §1.1 và từng repro
  §1 vào `test/engine/limits_test.go` (tag `// qc:BR-…` thích hợp). Chạy **trước khi sửa**:
  các case rc 1 ngày hôm nay phải red theo đúng chiều mong đợi sau fix.
* **Bước 1 — làm cho message tự giải thích được nó** (độc lập với bug, vẫn có giá trị nếu
  decision ở bước 2 đổi): cả ba nhánh `limits.go:226`, `:231`, `:241` in cùng một chuỗi vô dụng;
  cho nó biết *property, value, ngưỡng, và nhánh nào bắn*, theo gương `:249`
  (`CSS declaration limit exceeded (%d)`). Sửa luôn tiền tố đúp `goosie: goosie:` ở `cmd/goosie`.
  Việc này biến mọi lần từ chối trong tương lai thành một bug report tự mô tả.
* **Bước 2 — quyết định sửa ở tầng nào**, dựa trên số đo §2.1/§2.2:
  (a) `internal/css/token.go`: không phát `Dimension` bên trong một ident mở đầu bằng `--`
  (đúng theo cấu trúc tokenizer), hoặc (b) `internal/engine/limits.go`: bỏ qua declaration mà
  property là custom property (`--*`), vì giá trị của nó **không phải** geometry cho tới khi
  substitution xảy ra. Chọn một, ghi lý do vào task doc; không làm cả hai "cho chắc".
* **Bước 3 — chống lan:** `unicode-range` exemption là tiền lệ cho thấy mỗi lần nới cap là thêm
  một họ lỗi mới, nên cap phải **được giữ nguyên giá trị** (`MaxGeometry`, ngưỡng, `rc=1` cho
  payload thật). Chứng minh bằng cách chạy lại các test admission hiện có, và bằng mutation §4.
* **Bước 4 — fixture hoá:** thêm `testdata/render/css/NN-custom-property-ident.png` dựng từ repro
  §1 để họ lỗi này không bao giờ quay lại dưới dạng "chỉ trang sống mới gặp". Fixture phải có
  `reference-events.json`/PNG Chromium đo thật, không tự vẽ.
* **Bước 5 — đo lại trên trang thật:** `make live-parity --only cafef` (flag từ TASK-038 §2.6),
  chép số vào §5. Không sửa `testdata/parity.py`, `PASS_AT`, `MIN_INK`, `MIN_SCORED`.

## 4. Điều kiện nhận (không có cái nào là "test chạy xanh")

1. Bốn repro ở §1 trả rc 0 và paint bình thường; `.a{width:8e8e8px}` **vẫn** rc 1 — chứng minh
   cap thật còn tác dụng, đây là criterion quan trọng nhất.
2. Message từ chối nêu được property + value + ngưỡng + nhánh. Oracle assert chuỗi đó.
3. Bảng §1.1 trở thành test table-driven; mỗi dòng một assertion, không dòng nào rỗng.
4. Mutation proof [R-7], ít nhất 4 cái, mỗi cái giết đúng một check:
   (i) bỏ exemption custom property ⇒ case `--8e8e8e` red;
   (ii) nâng `MaxGeometry` lên `math.MaxFloat64` ⇒ case `width:8e8e8px` red;
   (iii) để cap áp cho `TokenNumber` thuần ⇒ case `--8e8:1` red;
   (iv) message mất property/value ⇒ oracle message red.
   Chạy từng cái trên branch, khôi phục, `git status` sạch, không còn `MUTATION-TEMP`.
5. `go test ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `make parity`,
   `make live-parity-selftest` xanh. `make live-parity` **không** phải gate (cần mạng) nhưng phải
   được chạy lại và chép số thật.
6. `GOOSIE-LIVE-002` chỉ được đóng khi số của §5 chứng minh cafef.vn nằm trong mẫu so sánh được.

## 5. Kết quả

*(điền sau khi chạy — số trang thật so sánh được trước, tỷ lệ sau)*

## 6. Ngoài phạm vi

* Không đổi `25 s` deadline của `-screenshot` (`GOOSIE-LIVE-003`) — đó là task khác.
* Không root-cause họ "trang không vẽ gì" (`GOOSIE-LIVE-001`, TASK-039).
* Không nới bất kỳ ngưỡng admission nào khác (`MaxCSSDeclarations`, image caps).
* Không đụng metric của harness đo (`GOOSIE-LIVE-004`).
