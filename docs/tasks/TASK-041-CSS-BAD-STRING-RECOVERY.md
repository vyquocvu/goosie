# TASK-041 — chuỗi CSS chưa đóng bị vẽ thành nội dung trang (bad-string recovery)

**Priority:** P1. **Method:** INT (unit oracle trong `test/`) + PARITY (fixture mới `testdata/render/css/`).
**Owner:** chưa gán. **Defect:** `GOOSIE-CSSSTRING-001`.
**Liên quan:** `internal/css/token.go` (nơi sửa), `internal/css/parser.go` (đường khai báo bị loại),
`testdata/render/selectors/03-pseudo-elements.html` (fixture đang chứa bug), `GOOSIE-BLANKMETRIC-001` /
`TASK-036` (vì sao harness không nhìn thấy bug này — **không** sửa trong task này).

Bug này có repro tối thiểu, số đo hai chiều và vị trí code chính xác ngay từ đầu (§1). Toàn bộ
phần đo đều chạy offline, không cần mạng. §2 là những việc **phải đo thêm trước khi sửa**, vì sửa
sai tầng sẽ biến "mất một declaration" thành "mất cả stylesheet".

## 1. Bằng chứng đã đo

### 1.1 Cơ chế

`internal/css/token.go:201-240` — `readString(quote byte)`:

```go
for t.pos < len(t.input) {
    c := t.input[t.pos]
    if c == quote { t.pos++; return Token{Type: TokenString, ...} }
    ...
    sb.WriteByte(c)   // newline cũng đi qua đây
    t.pos++
}
return Token{Type: TokenString, Value: sb.String()}   // EOF: vẫn trả về <string>
```

Hàm chỉ thoát vòng lặp khi gặp dấu đóng. Gặp `\n` nó **ghi ký tự đó vào value** và đi tiếp tới EOF,
rồi trả về `TokenString`. CSS Syntax Level 3 §4.3.7 yêu cầu gặp newline là phải phát
`<bad-string>`; parser khi thấy `<bad-string>` thì loại khai báo chứa nó. Goosie không có khái niệm
đó: `grep -rn "bad-string" internal/` → **0 kết quả**.

Hệ quả dây chuyền: value của `content` trở thành **toàn bộ CSS còn lại**, và nó được vẽ lên trang.

### 1.2 Repro tối thiểu (đã chạy trên máy này, 2026-10-26)

`/tmp/bs-repro.html` — một rule duy nhất, chép nguyên văn (dòng **xuống dòng ngay sau chuỗi hỏng**
là phần thiết yếu của repro, đừng nén một dòng):

```css
.q::before {
  content: """;
  font-size: 48px;
  color: #3498db;
}
.q { font: 16px monospace; color: #000; }
```

| Bên | Canvas | Ink px | Ghi chú |
| --- | --- | --- | --- |
| Goosie | 1280×1024 | **2,845** | vẽ dòng `"""; font-size: 48px; color: #3498db; } .q { font: 16px monospace; color: #000; }` |
| Chromium | 1280×1024 | **523** | chỉ "Hello world"; `getComputedStyle('.q','::before').content` = **`none`** |
| Goosie | 2880×1800 (cửa sổ `-screenshot` mặc định) | 8,424 | cùng bug, font lớn hơn |

Hai chi tiết chứng minh mức nghiêm trọng vượt "một declaration sai":

1. Rule `.q` **nằm sau** chuỗi hỏng cũng không áp dụng — "Hello world" render bằng serif mặc định
   chứ không phải monospace ⇒ toàn bộ phần còn lại của stylesheet bị nuốt.
2. Ảnh chụp cho thấy CSS hiện thành text 48px màu xanh ngay trước nội dung trang. Người dùng đọc
   được mã nguồn CSS của site trên chính trang đó.

### 1.3 Fixture trong corpus đã nhiễm, và gate mù với nó

`testdata/render/selectors/03-pseudo-elements.html:47` và `:53` đều là `content: """;` — có từ
`2bea5cc` ("add the fixture set the parity harness scores"), tức là **chưa bao giờ** được đo đúng.

Đo trên fixture thật, canvas 800×2400:

| Bản | Ink px |
| --- | --- |
| Goosie, chuỗi hỏng (nguyên bản) | 68,830 |
| Goosie, thay bằng `content: "\201C";` | 49,295 |
| Chromium | 59,068 |

⇒ ~9.8k px là mã nguồn CSS vẽ nhầm. Ảnh crop vùng `::before and ::after together` cho thấy nguyên
văn `"""; font-size: 48px; color: #3498db; vertical-align: -20px; } .quote::after { content: """`.

Gate không thấy, vì **hai lý do độc lập** — cả hai đều là defect khác, không nằm trong phạm vi task này:

* Ở canvas 800×600 mà `make parity` dùng, bản hỏng và bản sửa có ink **bằng nhau từng byte**
  (28,719 px mỗi bản): chỗ rò nằm dưới fold.
* `score_regions` (testdata/parity.py:253-279) dựng band từ mask **Chromium**; ink của Goosie ở nơi
  Chromium không có ink thì không band nào được tạo ⇒ ink thừa không bao giờ bị phạt.
  → `GOOSIE-BLANKMETRIC-001`, `TASK-036`.

### 1.4 Không phải regression của TASK-033

`git diff 4de0fca HEAD -- internal/css/token.go` → rỗng. `readString` không đổi kể từ
`0134aac`. Bug tồn tại độc lập với work gần đây.

## 2. Phải đo trước khi sửa

1. **Tách hai hành vi, đừng gộp.** Newline-trong-string (bad-string) và EOF-trong-string là hai
   nhánh khác nhau của §4.3.7. Đo Chromium trên **cả hai**: (a) `content: """;` rồi còn rule khác
   phía sau; (b) `content: """;` rồi **hết file**; (c) dấu nháy đơn `'`. Ghi `content` computed và
   ink của từng case vào §1. Chỉ sau khi có số mới quyết định EOF trả `<string>` hay `<bad-string>`.
2. **Vòng pre-pass `stripComments` có ăn theo không?** `internal/css/parser.go:123-150` tự theo dõi
   dấu nháy để không cắt nhầm `/*` trong `url("…")`. Một chuỗi chưa đóng có thể làm nó coi **cả
   phần còn lại của sheet là bên trong string** và bỏ qua mọi comment sau đó — một lỗi thứ hai, độc
   lập với tokenizer. Đo bằng cách thêm `/* x */` vào sau chuỗi hỏng rồi so sánh kết quả render.
3. **Parser loại khai báo ở tầng nào?** Sau khi tokenizer phát `<bad-string>`, cần biết
   `internal/css/parser.go` có đường nào để bỏ đúng một declaration rồi tiếp tục từ `;`/`}` kế tiếp
   hay không. Nếu không có, sửa tokenizer một mình sẽ **không** khỏi bug (hoặc khỏi sai chiều: mất
   cả rule). Đọc và đo trước, rồi mới chọn tầng sửa.
4. **Bao nhiêu trang thật dính?** Chưa đo. Chạy grep `content:\s*["'][^"']*$` trên corpus
   `docs/qa-live-parity/` (HTML đã tải) để biết đây là 1 fixture hay cả họ trang.

## 3. Plan (chỉ làm sau khi §2 xong)

* **Bước 0 — oracle trước, code sau.** Thêm vào `test/css/` (hoặc package tương đương đang giữ
  tokenizer test) một test table-driven cho §2.1: mỗi input một kỳ vọng *kiểu token*
  (`TokenBadString` / `TokenString`) **và** một kỳ vọng hành vi mức document (declaration bị loại,
  các rule sau vẫn áp dụng). Tag `// qc:BR-…` thích hợp. Chạy trước khi sửa để thấy red đúng chiều.
* **Bước 1 — phát `<bad-string>` đúng spec**, chỉ ở `readString`: gặp `\n` thì dừng, reconsume,
  trả token hỏng. Thêm `TokenBadString` vào token type nếu chưa có. Không "tiện tay" đổi EOF.
* **Bước 2 — cho parser bỏ đúng phạm vi cần bỏ**: một declaration, không phải cả rule, không phải
  cả sheet. Oracle bắt buộc: sau khi sửa, `.q { font: 16px monospace }` trong §1.2 **phải** áp dụng
  (đo font thật, không đo "không panic").
* **Bước 3 — fixture hoá**, vì đây là bug harness mù: thêm `testdata/render/css/NN-bad-string.html`
  dựng từ §1.2, kèm PNG Chromium đo thật. Fixture phải **có nội dung nằm dưới fold 600px** để
  chứng minh nó không qua mặt gate theo kiểu §1.3 — nếu cần, dùng fixture cao hơn và khai báo
  band tương ứng.
* **Bước 4 — sửa fixture đang nhiễm**: đổi `content: """;` → `content: "\201C";` (đã đo ở §1.3)
  trong `selectors/03-pseudo-elements.html`, rồi **đo lại** `blank_regions` của nó. Con số có thể
  đổi; cập nhật `testdata/render-known-gaps.json` là hợp lệ **chỉ** khi có số đo mới, và phải ghi
  rõ là đổi do fixture sửa, không do nới ngưỡng.
* **Bước 5 — không sửa metric.** Mọi vấn đề "band rỗng do lệch dòng" ở §1.3 thuộc TASK-036.

## 4. Điều kiện nhận (không có cái nào là "test chạy xanh")

1. Repro §1.2: Goosie compute `content: none` cho `.q::before` **và** `.q` vẫn là monospace;
   ink về mức tương đương Chromium (đo, ghi số, không ước lượng).
2. Bảng §2.1 thành test table-driven; mỗi dòng một assertion; không dòng nào rỗng.
3. `.a{content:"unclosed}` (EOF) xử lý theo **đúng** số đo Chromium ở §2.1 — không phải theo cảm
   giác "nên khoan dung".
4. Mutation proof [R-7], tối thiểu 3 cái, mỗi cái giết đúng một check:
   (i) `readString` bỏ kiểm tra newline ⇒ case bad-string red;
   (ii) parser nuốt cả rule thay vì một declaration ⇒ case "rule sau vẫn áp dụng" red;
   (iii) EOF trả `<string>` trong khi §2.1 nói `<bad-string>` ⇒ case EOF red.
   Chạy từng cái, khôi phục, `git status` sạch, không còn file `MUTATION-TEMP`.
5. `go test ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `make parity`,
   `make live-parity-selftest` xanh; `make parity` chạy **sau** bước 4 và số của
   `selectors/03` (match %, blank_regions, ink) được chép vào §5.
6. `GOOSIE-CSSSTRING-001` chỉ đóng khi §5 có: số ink hai chiều của repro, số của fixture trước/sau
   sửa, và kết quả grep §2.4.

## 5. Kết quả

*(điền sau khi chạy)*

## 6. Ngoài phạm vi

* Không sửa `score_regions`, `blank_regions`, `PASS_AT`, `THRESHOLD`, `MIN_INK` — `TASK-036`.
* Không làm cho harness render full-page (fold 600px là quyết định kiến trúc, không phải bug ở đây).
* Không sửa `GOOSIE-PSEUDO-002` / `GOOSIE-FLOAT-001` (hai defect ghi danh TASK-033), dù cùng vùng
  pseudo-element.
* Không đụng `internal/engine/limits.go` (`TASK-040`).
