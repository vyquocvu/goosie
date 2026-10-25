# TASK-041 — chuỗi CSS chưa đóng bị vẽ thành nội dung trang (bad-string recovery)

**Priority:** P1. **Method:** INT (unit oracle trong `test/`) + PARITY (fixture mới trong `testdata/render/css-features/`).
**Owner:** Qoder (nhánh `worktree-task-041-css-bad-string`). **Defect:** `GOOSIE-CSSSTRING-001`.
**Liên quan:** `internal/css/parser.go` (nơi sửa — ba scanner theo dõi dấu nháy),
`internal/css/token.go` (cùng thiếu check newline nhưng **bất khả trình** từ đường parse sheet, xem §1.1),
`testdata/render/selectors/03-pseudo-elements.html` (fixture đang chứa bug), `GOOSIE-BLANKMETRIC-001` /
`TASK-036` (vì sao harness không nhìn thấy bug này — **không** sửa trong task này).

Bug này có repro tối thiểu và số đo hai chiều ngay từ đầu, nhưng **vị trí code trong §1.1 bản đầu là
sai** và đã bị rút lại bằng đo lường (§1.1). §2 là bốn phép đo **bắt buộc trước khi sửa**, vì sửa sai
tầng sẽ biến "mất một declaration" thành "mất cả stylesheet"; cả bốn đã chạy trên máy này ngày
2026-10-26, không cần mạng (Chromium chạy local qua Playwright, mọi URL là `file://`).

## 1. Bằng chứng đã đo

### 1.1 Cơ chế

`internal/css/parser.go` — ba vòng scanner tự theo dõi dấu nháy, mỗi vòng đều đóng `quote` **chỉ khi
gặp đúng ký tự nháy**, không có nhánh nào cho newline:

| Vị trí (trên `main`) | Vòng lặp | Hậu quả khi chuỗi chưa đóng |
| --- | --- | --- |
| `parseRule` :1224-1236 | quét selector tới `{` | selector nuốt tới EOF |
| `parseRule` :1248-1265 | quét thân rule tới `}` | `declText` = **toàn bộ sheet còn lại** |
| `parseBlockWithRules` :1375-1379 | tách từng statement | một statement khổng lồ thay vì nhiều khai báo |

`declText` sau đó đi vào `parseBlockWithRules`, nên value của `content` nhận nguyên CSS chưa parse.
Đo bằng probe trên `main` (bỏ probe sau khi đo), input
`.q::before { content: """;\n font-size: 48px;\n color: #3498db;\n}\n.q { font: 16px monospace; color: #000; }`:

```
RULE [.q::before] decls=1
   content = "\"\"\";\n font-size: 48px;\n color: #3498db;\n}\n.q { font: 16px monospace; color: #000; }"  (83 bytes)
```

một rule duy nhất, và rule `.q` **không tồn tại trong stylesheet**. CSS Syntax Level 3 §4.3.7 quy
định chuỗi chưa đóng phải thành `<bad-string>` **ngay tại newline**, và §5.4.4 yêu cầu loại đúng
declaration chứa nó. `grep -rn "bad-string" internal/` → **0 kết quả**.

**Attribution rút lại:** bản §1.1 đầu tiên đổ lỗi cho `internal/css/token.go:201 readString`. Sai.
`grep -rn "NewTokenizer" internal/` chỉ trả về `css/value.go:114` (lex **một value**) và
`engine/limits.go:181` (đếm token để áp trần admission) — **không** có đường nào từ `Parse` đi qua
tokenizer, nên `readString` không thể nuốt cả sheet. Probe ở trên chạy nguyên văn trên `main` và giá
trị 83 byte xuất hiện từ scanner chứ không từ tokenizer.

Cùng một thiếu sót (đóng `quote` chỉ khi gặp ký tự nháy) còn nằm ở `stripComments` (`parser.go:124`,
đo ở §2.2), `splitDeclarations`/`SplitDeclarations` (`:1768`, `:1776`) và các scanner
at-rule/nested (`:1314-1355`, `:1408-1420`).
Chúng tôi **không** sửa trong task này, có chủ đích: `readString` nuôi trần admission mà `TASK-040`
đang sở hữu (đổi số token đếm được là đổi hành vi từ chối sheet), còn splitter ở tầng value là thứ
giữ cho data URI base64 xuống dòng không bị cắt. §2.2 đo tác dụng thật của `stripComments` để việc
"để lại" này không phải là phỏng đoán.

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

`git diff 4de0fca HEAD -- internal/css/token.go internal/css/parser.go` → **cả hai rỗng**.
`readString` không đổi kể từ `0134aac` (2026-10-06); `parser.go` lần cuối đổi ở `99b09a6`
(2026-10-18), trước work TASK-033. Bug tồn tại độc lập với work gần đây.

## 2. Bốn phép đo trước khi sửa — **đã đo xong 2026-10-26**, trừ §2.4 một phần

### 2.1 Newline và EOF là hai hành vi khác nhau (Chromium 131.0.6778.33 qua Playwright, viewport 1280×1024, dpr 1)

Bốn file cố định trong `/tmp/bs-meas/`, đo cả `getComputedStyle` lẫn ink px (ngưỡng <250):

| Case | Input | Chromium `content` | Chromium font-family của `.q` | Kết luận |
| --- | --- | --- | --- | --- |
| a | `content: """;` xuống dòng, **còn rule `.q` phía sau** | `none` | `monospace` | newline = bad-string ⇒ mất **một** declaration, sheet sống |
| c | như a nhưng dấu nháy **đơn** `content: ''';` | `none` | `monospace` | giống hệt a ⇒ không được xử lý riêng cho `"` |
| b | `content: """` rồi **hết file** | `""` | `Times` | EOF ⇒ Chromium **vẫn trả `<string>`** (§4.3.7 "return a string token"), không phải bad-string |
| d | `body { font-family: "x; }` rồi `p { color: green; }`, hết file | — | `Times`, `p` vẫn đen `rgb(0,0,0)` | chuỗi chạy tới EOF nên **rule sau nó nằm trong value** ⇒ bị loại, không cứu được |

⇒ **EOF phải giữ hành vi khoan dung.** Sửa newline thành bad-string mà lỡ tay coi EOF cũng
bad-string là đổi hành vi của case b/d, tức là làm khác Chromium. Đây là lý do
`TestUnterminatedStringAtEOFStaysLenient` tồn tại trong oracle.

### 2.2 `stripComments` có ăn theo: **có, nhưng không phải cơ chế chính**

Input `.a { content: """;\n}\n/* keep */ .b { color: red; }`, đo trên `main` (probe, đã xoá):

```
StripComments(...) = ".a { content: \"\"\";\n}\n/* keep */ .b { color: red; }"   // comment KHÔNG bị cắt
rule [.a] decls=1  content = "\"\"\";\n}\n/* keep */ .b { color: red; }"          // .b không thành rule
```

Xác nhận đúng như §2.2 nghi ngờ: `stripComments` (`parser.go:124`) tự theo dõi dấu nháy nên coi phần
còn lại là **bên trong chuỗi** và bỏ qua mọi comment sau đó — lỗi thứ hai, độc lập. Nhưng hậu quả
nhìn thấy được vẫn do scanner §1.1 gây ra: sau khi chỉ sửa scanner, cùng input cho
`rule [.a] decls=0`, `rule [.b] decls=1 {color red}`. `stripComments` được để lại **có chủ đích**
(nó chỉ ảnh hưởng văn bản comment, không ảnh hưởng nội dung vẽ); nếu muốn sửa thì phải là defect
riêng, không phải "tiện tay" trong task này.

### 2.3 Parser loại khai báo ở tầng nào: **không có tầng nào cả**

`Parse` không đi qua tokenizer, còn `parseBlockWithRules` thì chỉ *tách* statement rồi append —
không có đường nào "bỏ một declaration và tiếp tục từ `;`/`}` kế tiếp". Nên sửa một mình
tokenizer (nếu có `<bad-string>`) sẽ **không** khỏi bug, đúng như lo ngại ban đầu. Cách sửa buộc
phải là hai bước trong cùng parser: (1) đóng `quote` tại newline (`closesStringAt`), (2) đánh dấu
statement chứa bad-string và **không append** nó (`stmtBadString`). Cả hai đều có oracle riêng.

### 2.4 Bao nhiêu trang dính: **corpus cục bộ = 2 dòng; trang thật chưa đo**

Quét bằng CSS-tokenizer thật (chỉ xét bên trong `<style>` và file `.css`, đã cắt comment, xử lý
escape) trên `testdata/**`: **112 file, 101 khối CSS → đúng 2 dòng** chứa chuỗi chưa đóng, cả hai ở
`testdata/render/selectors/03-pseudo-elements.html:47` và `:53`. Không fixture nào khác nhiễm.

*Chưa đo:* mức nhiễm trên **trang thật**. `docs/qa-live-parity/` chỉ chứa báo cáo JSON/MD —
`live-parity.py:340-374` chỉ ghi `write_reports`, không giữ HTML đã tải — nên phép đo này cần mạng
và thuộc Track A. Nó **không** chặn TASK-041: repro cục bộ đã đủ nghiêm trọng để sửa (P1), và fixture
duy nhất nhiễm sẽ được sửa ở §3 Bước 4. Ghi rõ là **còn nợ** ở §6.

## 3. Plan (chỉ làm sau khi §2 xong)

* **Bước 0 — oracle trước, code sau. ✅** `test/css/bad_string_test.go`, 5 test, mỗi test một kỳ
  vọng mức **stylesheet** (số declaration trên từng selector) chứ không phải mức token — vì §2.3
  chứng minh `Parse` không đi qua tokenizer, nên kỳ vọng `TokenBadString` sẽ không đo gì cả. Chạy
  trước khi sửa: **3/5 red**, 2 test còn lại là control (chuỗi đóng đúng, và EOF) phải green ngay từ
  đầu để ca sửa không đổi hành vi ngoài phạm vi.
* **Bước 1 — đóng chuỗi tại newline, đúng §4.3.7. ✅ (sửa lại so với bản đầu).** Không sửa
  `readString` (§1.1: bất khả trình từ đường parse sheet). Thay vào đó `closesStringAt(quote, c)`
  được dùng ở **cả hai scanner của `parseRule`** (selector và thân rule). **Không đụng EOF**: tới
  EOF vẫn là chuỗi hợp lệ, đúng case b/d của §2.1.
* **Bước 2 — bỏ đúng một declaration. ✅.** `parseBlockWithRules` thêm cờ `stmtBadString`; statement
  nào chứa bad-string thì **không append**, kể cả `trailing`. Oracle bắt buộc từ §1.2 đã green ở
  **hai tầng**: `test/css` thấy `.q` có 2 declaration (`font: 16px monospace`, `color`), và ảnh
  render 1280×1024 của Goosie khớp Chromium trên cùng crop — "Hello world" vẽ bằng monospace.
* **Bước 3 — fixture hoá. ⏳ còn làm.** Thêm `testdata/render/css/NN-bad-string.html` dựng từ §1.2,
  kèm PNG Chromium đo thật. Fixture phải **có nội dung nằm dưới fold 600px** để chứng minh nó không
  qua mặt gate theo kiểu §1.3.
* **Bước 4 — sửa fixture đang nhiễm. ⏳ còn làm.** Đổi `content: """;` → `content: "\201C";` (đã đo
  ở §1.3) trong `selectors/03-pseudo-elements.html`, rồi **đo lại** `blank_regions` của nó. Con số có
  thể đổi; cập nhật `testdata/render-known-gaps.json` là hợp lệ **chỉ** khi có số đo mới, và phải ghi
  rõ là đổi do fixture sửa, không do nới ngưỡng.
* **Bước 5 — không sửa metric. ✅ (không đụng `testdata/parity.py`).** Mọi vấn đề "band rỗng do lệch
  dòng" ở §1.3 thuộc TASK-036.

## 4. Điều kiện nhận (không có cái nào là "test chạy xanh")

1. Repro §1.2: Goosie compute `content: none` cho `.q::before` **và** `.q` vẫn là monospace;
   ink về mức tương đương Chromium (đo, ghi số, không ước lượng).
2. Bảng §2.1 thành test table-driven; mỗi dòng một assertion; không dòng nào rỗng.
   **Lệch có chủ đích (ghi ở §5):** oracle viết thành 5 test đặt tên thay vì một bảng, vì mỗi dòng
   của §2.1 cần một kỳ vọng *khác nhau về cấu trúc* (số declaration trên selector nào còn sống) —
   ép bảng sẽ phải nhét logic rẽ nhánh vào ô kỳ vọng. Số assertion mỗi dòng vẫn là một, và
   `TestEmptyStringStillGeneratesABox` là control để chứng minh red trước khi sửa là red đúng chỗ.
3. `.a{content:"unclosed}` (EOF) xử lý theo **đúng** số đo Chromium ở §2.1 — không phải theo cảm
   giác "nên khoan dung".
4. Mutation proof [R-7], tối thiểu 4 cái, mỗi cái giết **đúng một** check:
   (i) `closesStringAt` bỏ nhánh `c == '\n'` (quay về hành vi `main`) ⇒
       `TestBadStringEndsAtNewline` + `TestBadStringSingleQuoteMatchesDoubleQuote` red;
   (ii) giữ `closesStringAt` nhưng bỏ cờ `stmtBadString` ở chỗ append `stmt` ⇒ chuỗi hỏng vẫn được
        *parse* mà không bị *loại*, `TestBadStringDropsOneDeclarationNotTheSheet` red;
   (iii) bỏ `stmtBadString` ở chỗ append `trailing` ⇒ red cùng một check nhưng ở nhánh trailing,
        chứng minh hai điểm loại là hai check riêng chứ không dư;
   (iv) cho EOF cũng thành bad-string (`quote` bị đóng vô điều kiện khi hết input) ⇒
        `TestUnterminatedStringAtEOFStaysLenient` red — đây là mutation giữ cho ca sửa không vượt quá
        §2.1.
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
* **Nợ đo, không đóng được trong task này:** mức nhiễm trên **trang thật** (§2.4) — cần chạy mạng,
  và `live-parity.py` không giữ HTML đã tải, nên phép đo phải tải lại và grep trên bộ HTML đó.
* **Nợ sửa, cố ý để riêng:** cùng thiếu sót "đóng `quote` chỉ khi gặp ký tự nháy" vẫn còn ở
  `stripComments` (`parser.go:124`) và `readString` (`token.go:201`, nuôi trần admission của
  `TASK-040`). §2.2 đã đo để chứng minh chúng **không** phải cơ chế gây rò; nếu muốn siết thì phải
  là defect riêng với oracle riêng, không phải sửa kèm ở đây.
