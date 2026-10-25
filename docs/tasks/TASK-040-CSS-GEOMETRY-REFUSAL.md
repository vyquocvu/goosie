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

## 2. Số đo (trước đây là mục "chưa đo được")

Cả ba mục đều đã đo trên máy này, head `a5e1b77`. Probe là một file test tạm
`internal/engine/zz_probe_task040_test.go` (in-package, chạy `NewSession` +
`cssPropertyAt` trực tiếp), **đã xoá sau khi đo** — cùng quy trình §1.1 đã ghi.
Bản "trước sửa" lấy bằng cách tạm thời ghi đè tokenizer của HEAD rồi khôi phục từ
bản đã lưu, không dùng stash/branch. Chỉ `internal/css/token.go` bị thay lại: tại thời
điểm đo, guard đã mang message builder mới, nên chuỗi message trong cột trước/sau trông
giống nhau — đổi tokenizer không di chuyển ngưỡng, exemption, hay nhánh nào.

```
cp internal/css/token.go /tmp/token-after.go
git show HEAD:internal/css/token.go > internal/css/token.go
go test -count=1 -v -run TestProbeTask040 ./internal/engine   # bản trước sửa
cp /tmp/token-after.go internal/css/token.go
diff -q /tmp/token-after.go internal/css/token.go             # TOKEN_RESTORED_IDENTICAL
```

### 2.1 Họ lân cận, theo đúng format bảng §1.1

`span` là `[start,end)` in ra từ probe; `numval` là giá trị tokenizer tự đọc.

| Input | TRƯỚC (HEAD `32e3409`) | SAU (head này) | rc trước | rc sau |
| --- | --- | --- | --- | --- |
| `--8e8e8e` | Number `-` [0,1) numval −0; Dimension `-8e8` unit `e8e` numval −8e+08 | Ident `--8e8e8e` [0,8) | 1 | 0 |
| `--1e5x` | Number `-`; Dimension `-1e5` unit `x` numval −100000 | Ident `--1e5x` [0,6) | 0 | 0 |
| `--5px` | Number `-`; Dimension `-5` unit `px` numval −5 | Ident `--5px` [0,5) | 0 | 0 |
| `--8e8` | Number `-`; Number `-8e8` numval −8e+08 | Ident `--8e8` [0,5) | 0 | 0 |
| `--8e5e5` | Number `-`; Dimension `-8e5` unit `e5` numval −800000 | Ident `--8e5e5` [0,7) | 0 | 0 |
| `--1e9e9` | Number `-`; Dimension `-1e9` unit `e9` numval −1e+09 | Ident `--1e9e9` [0,7) | 1 | 0 |
| `--1e2e2` | Number `-`; Dimension `-1e2` unit `e2` numval −100 | Ident `--1e2e2` [0,7) | 0 | 0 |
| `--abc8e8e8` | Number `-`; Dimension `-` unit `abc8e8e8` numval −0 | Ident `--abc8e8e8` [0,10) | 0 | 0 |
| `--a-8e8e8` | Number `-`; Dimension `-` unit `a-8e8e8` numval −0 | Ident `--a-8e8e8` [0,9) | 0 | 0 |
| `--666666` | Number `-`; Number `-666666` numval −666666 | Ident `--666666` [0,8) | 0 | 0 |
| `-webkit-mask` | Dimension `-` unit `webkit-mask` numval −0 | Ident `-webkit-mask` [0,12) | 0 | 0 |
| `-moz-opacity` | Dimension `-` unit `moz-opacity` numval −0 | Ident `-moz-opacity` [0,12) | 0 | 0 |
| `-8e8e8` | Dimension `-8e8` unit `e8` numval −8e+08 | **không đổi** | 1 | 1 |
| `1e1e1` | Dimension `1e1` unit `e1` numval 10 | **không đổi** | 0 | 0 |
| `#8e8e8e` | Hash `8e8e8e` | **không đổi** | 0 | 0 |

Bốn hàng `-8e8e8`, `1e1e1`, `#8e8e8e` và `.a{width:8e8e8px}` là thứ *không được*
dịch chuyển; chúng green trước khi sửa và vẫn green sau khi sửa
(`test/css/hyphen_ident_test.go:TestHyphenTokenKind`, 18 hàng).

Kết luận §2.1: hypothesis "chỉ `--8e8e8e` bị" là **sai một phần** — mọi name bắt
đầu bằng số đều bị tách đôi, nhưng chỉ name có `e`…số tạo ra Dimension **vượt**
`MaxGeometry`; `--1e5x` (−100.000) và `--5px` (−5) nằm dưới cap nên rc 0 tình cờ.
`-webkit-mask`/`-moz-opacity` cũng từng là Dimension `-` (magnitude 0) — vô hại
nhưng sai kiểu token.

### 2.2 `cssPropertyAt` quy thuộc thế nào — và giả thuyết "span chồng lấn" bị bác

Số đo: **không có chồng lấn**. `--8e8e8e` phát ra `[0,1)` rồi `[1,8)` — kế tiếp
khít, phủ đúng `[0,8)`. Cái trông như trùng lặp là *chuỗi value* của token hai
(`"-8e8"` chứa lại dấu `-`), vì `readNumber` bắt đầu đọc từ chính dấu gạch mà
token một đã tiêu thụ. Bằng chứng tự động:
`test/css/hyphen_ident_test.go:TestTokenSpansCoverInputExactly`.

`cssPropertyAt(src, offset)` (limits.go:291 hiện tại) trên từng document:

| Declaration | token bị nghi | property mà guard quy thuộc |
| --- | --- | --- |
| `:root{--8e8e8e:#8e8e8e}` | Dimension `-8e8` | `""` (không property) |
| `.a{color:var(--8e8e8e)}` | Dimension `-8e8` | `"color"` |
| `.a{color:var(--8e8e8e, #8e8e8e)}` | Dimension `-8e8` | `"color"` |
| `.a{width:var(--8e8e8e)}` | Dimension `-8e8` | `"width"` |
| `.box .time{color:var(--8e8e8e,#8e8e8e);font-size:12px}` | Dimension `-8e8` / `12` | `"color"` / `"font-size"` |
| `@font-face{unicode-range:U+1E80-1EFF}` | Number `1E80` / Dimension `-1` | `"unicode-range"` / `"unicode-range"` |

Hai hệ quả quyết định:

* **Phương án (b) không sửa được defect.** Token sai được quy thuộc cho declaration
  *dùng* biến (`color`, `width`), không phải declaration khai báo `--8e8e8e` — nên
  "bỏ qua declaration có property là `--*`" vẫn để nguyên `.a{color:var(--8e8e8e)}`
  và trang vẫn bị từ chối.
* **`unicode-range` exemption không hoạt động tình cờ.** `cssPropertyAt` trả đúng
  `"unicode-range"` cho cả hai token của range, nên exemption khớp vì tên property,
  không vì offset lệch. Không có bug chung cho cả hai như §2 từng đoán.
* Một token `Number` với `value="-"` được `strconv.ParseFloat` trả
  `invalid syntax` (không phải `out of range`) nên nhánh `continue` bỏ qua — đó là
  lý do dấu `-` đơn lẻ chưa từng tự gây từ chối.

### 2.3 Bao nhiêu trang sống nằm trong họ này

Lệnh (chạy offline, đọc artifact đã commit):

```
python3 - <<'EOF'   # đếm trang bị cap trong hai bản report của sweep 100 trang
import json, glob, re
CAP = re.compile(r"outside safe geometry range|limit exceeded|must be finite|unsafe document")
for path in sorted(glob.glob("docs/qa-live-parity/*.json")):
    d = json.load(open(path)); rows = d["rows"]
    print(path, d["pages_in_corpus"], d["pages_scored"], d["counts"])
    print("  cap rows:", [r["url"] for r in rows if CAP.search(json.dumps(r))])
EOF
```

Số ra:

* `2026-10-25T1656Z-4de0fca5.json`: 100 trang, 94 được chấm,
  `ADMISSION_CAPPED: 1` → **đúng một trang**, `https://cafef.vn/`
  (`goosie_rc=1`, `goosie_diag` mang tiền tố đúp).
* `2026-10-25T1630Z-4de0fca5.json`: 100 trang, 93 được chấm, **0** hàng cap.
* Trong `/tmp/cafef.html` (212.708 B): 2 tên custom property bắt đầu bằng chữ số
  — `--666666` và `--8e8e8e`; chỉ `--8e8e8e` có `e`+số nên chỉ một tên vượt cap,
  `--666666` là Number thuần (đang được exemption bỏ qua).
* Corpus HTML trong repo: **0/114** file có tên digit-leading; sau khi task này
  thêm fixture thì 1/115 và chính file đó là fixture mới.

Số đo đây là họ lỗi "1 trang trong 100", không phải 30 trang — nhưng một trang cũng
đủ để fixture hoá, vì chi phí là một file HTML và họ lỗi này nhân giống bằng
CSS do máy tạo (colour-as-name).

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

### Quyết định tầng sửa: (a) tokenizer, một tầng duy nhất

Chọn **(a) `internal/css/token.go`**. Lý do là bảng quy thuộc §2.2, không phải sở thích:
Dimension sai bị guard quy thuộc cho declaration **dùng** biến — `.a{color:var(--8e8e8e)}`
nằm dưới tên `"color"`, `.a{width:var(--8e8e8e)}` dưới `"width"`, còn chính declaration
`:root{--8e8e8e:…}` thì thuộc tính `""`. Vậy phương án (b) ("bỏ qua declaration có property
`--*`") không chạm tới hai trong bốn repro và vẫn để cafef.vn bị từ chối; muốn nó hoạt động
phải thêm logic nhận diện `var()` vào guard — tức là một exemption mới đúng cái kiểu
`unicode-range` mà Bước 3 cảnh báo.

Sửa ở (a) đúng một chỗ: `Next()` phân nhánh U+002D theo thứ tự CSS Syntax L3 4.3.7
(số → ident → delimiter). Guard **không đổi hành vi**: `MaxGeometry` vẫn `2 << 20`, exemption
plain-Number/border-radius/`unicode-range` giữ nguyên, `rc=1` cho payload oversized thật.
Phần duy nhất khác của `internal/engine/limits.go` là message (Bước 1) — nó không quyết định
document nào được hay bị từ chối, và mutation §4 (ii)+(iii) chứng minh điều đó: hai mutation
này chỉ giết được các hàng "must refuse"/"must admit", không đụng tới tầng sửa.

**TASK-041 không bị đụng:** `readString` (`internal/css/token.go:201` ở HEAD, `:223` sau commit
này vì dispatch mới chen phía trên) giữ nguyên từng ký tự.
TASK-041 để hàm đó nguyên trạng vì độ dài chuỗi đi vào bộ đếm admission của CSS
(`MaxCSSDeclarations`, số byte được `cssBudget.check` đếm); thay đổi lần này chỉ nằm
trong dispatch của `Next()` với ký tự `-`, nên không token string nào, offset nào,
hay bộ đếm nào dịch chuyển.
Không có test nào của TASK-041 (`test/css` bad-string family) red sau fix — số chạy ở §5.

### Kết quả từng bước

* **Bước 0** — `ef1699a` (oracle trước code; 11 hàng tokenizer + 8 hàng engine red, một chiều).
  Đúng hành vi đó được đo lại ở head cuối qua mutation (i) §5.4: 9 hàng tokenizer, 7 hàng matrix
  và `TestDigitLeadingCustomPropertyPaints` red — lệch nhau vì hai expectation đã đo lại ở
  `9516799`.
* **Bước 1** — message `a8266ff`; tiền tố đúp `09d1874`.
* **Bước 2** — tầng (a), `4278104`.
* **Bước 3** — cap giữ nguyên: `TestMaxGeometryIsUnchanged` + mutation (ii)/(iii) ở §5.
* **Bước 4** — `a5e1b77`: `testdata/render/css-features/09-digit-leading-custom-property.html`,
  `MIN_SCORED_FIXTURES` 81→82 cùng commit, `render-known-gaps.json` không thêm entry (0 band đo được).
* **Bước 5** — số thật ở §5; `make live-parity` không nhận arg nên chạy trực tiếp
  `python3 testdata/live-parity.py --binary ./bin/goosie --only cafef`.

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

Head đo: **`a5e1b77`** (branch `worktree-task-040-css-geometry`, base `32e3409`).
Commit theo thứ tự: `ef1699a` oracle, `9516799` sửa 2 expectation đo lại,
`4278104` tokenizer (tầng đã chọn), `a8266ff` message, `09d1874` tiền tố + test
end-to-end, `a5e1b77` fixture + floor.

### 5.1 Tiêu chuẩn 1 — mở trang hợp lệ, vẫn chặn payload thật

Bốn repro §1, chạy qua `NewSession` (probe) và qua binary thật:

| Document | rc trước | rc sau | Ghi chú |
| --- | --- | --- | --- |
| `:root{--8e8e8e:#8e8e8e}` | 1 | **0** | ident, không còn là length |
| `.a{color:var(--8e8e8e)}` | 1 | **0** | |
| `.a{color:var(--8e8e8e, #8e8e8e)}` | 1 | **0** | dạng thật của cafef.vn |
| `.a{color:#8e8e8e}` | 0 | 0 | không đổi |
| `.a{width:var(--8e8e8e)}` | 1 | **0** | length thật không tồn tại ở đây |
| `.box .time{color:var(--8e8e8e,#8e8e8e);font-size:12px}` | 1 | **0** | rule 29 của khối 8 |
| **`.a{width:8e8e8px}`** | 1 | **1** | **vẫn bị từ chối — criterion quan trọng nhất** |
| `.a{width:-8e8e8px}` | 1 | 1 | dimension âm thật |
| `.a{width:9999999px}` | 1 | 1 | 9.999.999 px > 2.097.152 |
| `:root{--8e8:1}`, `.a{width:8e8}` | 0 | 0 | plain Number vẫn exemption |

Cột "trước" đo trực tiếp bằng probe với tokenizer HEAD (`go test -v -run TestProbeTask040`,
đoạn "document matrix") cho mọi hàng trừ hai hàng `width:-8e8e8px` và `width:9999999px`:
hai hàng đó không có trong matrix, giá trị "trước" của chúng suy từ token không đổi
(§2.1: `-8e8e8` và `8e8e8px` vẫn là Dimension vượt cap cả trước lẫn sau) và chỉ chiều
"sau" là được đo bằng document thật.

Dòng stderr thật của binary đã build (`./bin/goosie -screenshot -out … -url
file:///tmp/task040-refuse.html`, `.a{width:8e8e8px}`), `rc=1`, **không có PNG**:

```
goosie: build session: CSS numeric value outside safe geometry range: property "width", value "8e8", unit "e8px", parsed 8e+08, numval 8e+08, limit 2097152 CSS pixels, branch magnitude
```

So với dòng cũ (`goosie: goosie: build session: CSS numeric value outside safe geometry
range`): một tiền tố, và nêu property + value + unit + ngưỡng + nhánh.

### 5.2 cafef.vn đo lại trên máy này

`file:///tmp/cafef.html` (217.170 B, bản curl §1 bước 1), `./bin/goosie -screenshot`,
ink đếm theo cùng quy ước với `parity.py` (kênh min < 245):

| | trước (`4de0fca`) | sau (`a5e1b77`) |
| --- | --- | --- |
| rc | 1 | **0** |
| PNG | không tồn tại | **247.616 B**, 2880×1800 |
| ink | 0 | **1.959.401** px non-white |
| stderr | 1 dòng từ chối | 0 byte |

Sweep thật, một host (`python3 testdata/live-parity.py --binary ./bin/goosie
--only cafef --out-dir /tmp/task040-live-cafef`, report
`2026-10-25T1912Z-a5e1b77e.json`, Chromium 131.0.6778.33):

| field | trước (1656Z, `4de0fca5`) | sau (1912Z, `a5e1b77`) |
| --- | --- | --- |
| label | `ADMISSION_CAPPED` | **`OK`** |
| goosie_rc | 1 | 0 |
| goosie_secs | 0.63 | 2.81 |
| doc_height | null | 5274 |
| ink_goosie / ink_chromium | 0 / — | 542.415 / 796.443 |
| coverage | — | 0.642 |
| pct_exact | — | 46.23 |
| blank bands / reference bands | — | **0** / 3 |
| goosie_diag | `goosie: goosie: build session: CSS numeric value outside safe geometry range` | `""` |

`counts` của run: `{'OK': 1}`; `floor_met: false` và **exit 2** là do chính harness
báo khi mẫu nhỏ hơn 75 trang ("too few pages compared — this run is broken") — chạy
một host thì luôn dưới floor, đó là hành vi fail-closed đã thiết kế ở TASK-036, không
phải lỗi mới. cafef.vn **đã nằm trong mẫu so sánh được với PNG thật của cả hai bên**,
đó là điều tiêu chuẩn 6 đòi.

### 5.3 Oracle: đỏ trước khi sửa, xanh sau khi sửa

`go test -count=1 -v -run TestProbeTask040 ./internal/engine` với tokenizer HEAD
(§2) cho bảng rc "trước" ở 5.1; sau khi sửa, bộ oracle xanh toàn bộ:

| Oracle | số assertion | sau fix |
| --- | --- | --- |
| `test/css/hyphen_ident_test.go:TestHyphenTokenKind` | 18 hàng token | xanh |
| `test/css/hyphen_ident_test.go:TestTokenSpansCoverInputExactly` | 7 input, không subtest | xanh |
| `test/engine/css_geometry_admission_test.go:TestDigitLeadingCustomPropertyRefusalMatrix` | 24 hàng (đếm `--- PASS`) | xanh |
| `…:TestDigitLeadingCustomPropertyPaints` | `var(--8e8e8e)` → `css.Color{142,142,142,255}`, paint non-empty | xanh |
| `…:TestOversizedAppliedLengthIsRefused` | 1 | xanh |
| `…:TestGeometryRefusalNamesEverything` | 2 nhánh + bound 600 B | xanh |
| `…:TestMaxGeometryIsUnchanged` | `2 << 20` | xanh |
| `cmd/goosie/error_prefix_test.go` (2 test) | 6 hàng + chain thật | xanh |
| `test/entrypoints/geometry_refusal_line_test.go` (2 test) | 2 lần build + chạy binary thật | xanh |

Chạy lại nguyên họ của TASK-041 (`test/css`) và admission hiện có (`./internal/engine`,
`test/engine`): **ok**, không red nào còn sót.

### 5.4 Mutation proof [R-7] — 5 cái, mỗi cái giết đúng một check

Mỗi mutation sửa một chỗ, chạy test liên quan, rồi `git checkout --` khôi phục.
`grep -rn MUTATION-TEMP` trên cây code (trừ `docs/`): **no matches**; `git status` sạch
sau từng cái.

| # | Mutation | FAIL đo được |
| --- | --- | --- |
| (i) | bỏ nhánh `hyphenStartsIdent` ("custom property exemption") | `hyphen_ident_test.go:104: token stream for "--666666" = [Delim(value=- unit=) Number(value=-666666 unit=)] (2 tokens), want [Ident(value=--666666 unit=)] (1 tokens)` — 9 hàng tokenizer red; `css_geometry_admission_test.go:81: :root{--8e8e8e:#8e8e8e}: document refused, want admitted (a custom property name is an ident)` — 7 hàng matrix + `TestDigitLeadingCustomPropertyPaints` red |
| (ii) | nâng cap tới `math.MaxFloat64` | **không build được**: 7 lỗi compile, ví dụ `internal/engine/limits.go:107:12: -edge (untyped float constant -1.43815e+309) truncated to int32`, `engine.go:1255:41: MaxGeometry … overflows float32`. Số này tự nó là bằng chứng cap đang gánh ba chỗ khác. Biến thể *cùng ý nghĩa* và compile được — so sánh với `math.MaxFloat64` tại nhánh magnitude — red đúng hàng phải-chặn: `css_geometry_admission_test.go:78: .a{width:8e8e8px}: document admitted, want refused (8e8 px is what the cap exists for)` (+ `width:-8e8e8px`, `width:9999999px`, `TestGeometryRefusalNamesEverything/magnitude_branch`) |
| (iii) | bỏ điều kiện `t.Type != css.TokenNumber` (cap chạm plain Number) | `css_geometry_admission_test.go:81: .a{width:8e8}: document refused, want admitted (plain number, cap deliberately skips it)` và test **có trước** `test/engine/css_numeric_gate_test.go:17: NewSession with huge counter/z-index = … property "counter-reset", value "2000000000" … want admitted` |
| (iv) | message bỏ `property`/`value` | `css_geometry_admission_test.go:194: refusal message "…unit \"e8px\", parsed 8e+08, …" is missing "property \"width\""` (2 nhánh red) và end-to-end `geometry_refusal_line_test.go:68: refusal line omits property "width"` |
| (v) | `main()` in lại `err` thô (tiền tố đúp trở lại) | `geometry_refusal_line_test.go:72: doubled program label on the line an operator reads: "goosie: goosie: build session: CSS numeric value …"` và `:78: goosie: label count = 2, want 1` |

Hai độ lệch so với dự đoán trong §4.4, ghi lại nguyên văn vì chúng là số thật:

* (ii) `math.MaxFloat64` **không compile được** nên không "red một test" theo nghĩa
  đen; đã chạy biến thể compile được ở trên và báo cả hai kết quả.
* (iii) §4 đoán hàng `--8e8:1` sẽ red. Sau fix tầng (a), `--8e8` là ident và số duy
  nhất trong declaration đó là `1` — nên mutation bị bắt bởi hàng `.a{width:8e8}` và
  bởi `TestHugeNonGeometryNumbersAdmitted` thay thế. Đúng check bị giết, khác hàng.
* (v) oracle unit trong `cmd/goosie` **sống sót** mutation này (nó test helper, không
  test chỗ in); chỉ oracle end-to-end giết được. Ghi nhận là giới hạn của test unit.

### 5.5 Verification battery (mỗi lệnh chạy một lần, không `-race`)

| Lệnh | Exit | Số |
| --- | --- | --- |
| `go test -count=1 ./...` | **1** | 41 package ok, **1 FAIL: `test/qc`** — `traceability_test.go:112: traceability.golden.json is stale; run go test ./test/qc -update`. Không package nào khác red. |
| `go vet -all ./...` | 0 | sạch |
| `go test -count=1 ./internal/archtest/` | 0 | ok, import/arch không đổi |
| `make parity` (`python3 testdata/parity.py --render --render-chromium --binary ./bin/goosie`) | 0 | **82/82 passing (100%)**, average match **96.12%**, known gaps 5 fixtures / 10 bands, hàng mới: `98.90% PASS css-features/09-digit-leading-custom-property.png worst_region_iou=0.9918 coverage_ratio=0.9916 blank_regions=0/0` |
| `python3 testdata/live-parity.py --selftest` | 0 | `selftest green` |
| `python3 testdata/live-parity.py --binary ./bin/goosie --only cafef` | 2 | trang duy nhất `OK` + `rerender OK`; exit 2 vì floor 75 trang (xem 5.2) |
| `go test -count=1 ./test/qc/` | 1 | stale golden, xem 5.6 |

`render-known-gaps.json` **không** thêm entry: fixture mới đo 0 blank band, khai 0 =
đo 0. `MIN_SCORED_FIXTURES` 81→82 nằm cùng commit với fixture (`a5e1b77`), và
`test/gatecheck/parity_harness_test.go` đếm 82 file HTML thật nên xanh.

### 5.6 Traceability: golden stale, và dòng registry đề nghị

Diff duy nhất sau `go test ./test/qc -run TestTraceability -args -update` (đã **revert**,
theo luật không sửa artifact): `BR-164` được bổ sung 11 test refs —

```
cmd/goosie/error_prefix_test.go:TestProgramErrorLineCarriesOneLabel
cmd/goosie/error_prefix_test.go:TestRunFailureLineFromTheRealChain
test/css/hyphen_ident_test.go:TestHyphenTokenKind
test/css/hyphen_ident_test.go:TestTokenSpansCoverInputExactly
test/engine/css_geometry_admission_test.go:TestDigitLeadingCustomPropertyPaints
test/engine/css_geometry_admission_test.go:TestDigitLeadingCustomPropertyRefusalMatrix
test/engine/css_geometry_admission_test.go:TestGeometryRefusalNamesEverything
test/engine/css_geometry_admission_test.go:TestMaxGeometryIsUnchanged
test/engine/css_geometry_admission_test.go:TestOversizedAppliedLengthIsRefused
test/entrypoints/geometry_refusal_line_test.go:TestGoosieDigitLeadingCustomPropertyIsAdmitted
test/entrypoints/geometry_refusal_line_test.go:TestGoosieGeometryRefusalLineIsWellFormed
```

Oracle tag bằng `// qc:BR-164` (case hiện có, "CSS/input selector cực lớn hoặc lặp bất
lợi → admission limit áp dụng") vì một id chưa tồn tại trong `docs/qa-test-catalog.md`
sẽ làm `TestTraceability` fail kiểu *unknown QC ID* — catalogue là việc của integrator.
Đề nghị của integrator, theo thứ tự:

1. `go test ./test/qc -run TestTraceability -args -update` để bắt 11 ref trên.
2. Nếu muốn một case riêng cho họ này, thêm vào `docs/qa-test-catalog.md`
   `BR-187 | P1 | SEC, INT | NOT RUN` — "custom property name bắt đầu bằng chữ số là
   ident, không được sinh ra Dimension; admission guard không được từ chối document chỉ
   vì tên biến" — rồi đổi tag ở 5 file test trên sang `BR-187` và `-update` lại golden.
3. `docs/qa-open-defects.json` / `GOOSIE-LIVE-002`, ba field đề nghị:
   * `"status": "closed"`
   * `"closed_by": "a5e1b77"` (chain `4278104` `a8266ff` `09d1874` `a5e1b77`)
   * `"evidence"`, nguyên số đo ở §5:

     Tokenizer HEAD phát `Number("-")` + `Dimension("-8e8", unit "e8e", numval -8e+08)` cho
     `--8e8e8e`; guard đọc Dimension đó và từ chối **cả document**. Sửa ở
     `internal/css/token.go` theo thứ tự U+002D của CSS Syntax L3 4.3.7 (số → ident → delimiter),
     một tầng duy nhất, guard không đổi ngưỡng.
     Measured trước: 1/100 trang sweep bị — cafef.vn, `2026-10-25T1656Z-4de0fca5.json`,
     `ADMISSION_CAPPED`, `goosie_rc=1`, không PNG, diag mang tiền tố đúp.
     Measured sau: `file:///tmp/cafef.html` rc 0, PNG 247.616 B (2880×1800), ink 1.959.401 px;
     live one-host `2026-10-25T1912Z-a5e1b77e.json` label `OK`, rc 0, doc_height 5274,
     ink_goosie 542.415 / ink_chromium 796.443, coverage 0.642, 0 blank band trên 3 reference band.
     Criterion 1: `.a{width:8e8e8px}` vẫn rc 1, message mới nêu `property "width"`, `value "8e8"`,
     `unit "e8px"`, `limit 2097152 CSS pixels`, `branch magnitude`; `MaxGeometry` vẫn `2 << 20`.
     Oracle: 24 hàng matrix + 18 hàng token + 2 test entrypoints end-to-end xanh; parity 82/82
     (fixture mới `css-features/09-digit-leading-custom-property.png` 98.90%), 5 mutation giết
     đúng 5 check.
     Giới hạn còn lại: `:root{-8e8e8:1}` vẫn là Dimension và vẫn bị từ chối; `+`/`.` vẫn được
     gửi thẳng cho numeric scanner; nhánh `non-finite` chưa input nào tới được qua đường token.
4. Task này **không** tự đánh dấu `GOOSIE-LIVE-002` đã đóng ở bất kỳ file nào.

### 5.7 Những gì chưa chứng minh được

* **Chỉ một trang sống trong mẫu 100** thể hiện họ lỗi; không có bằng chứng tỷ lệ thật
  trên corpus lớn hơn. Prevalence đo được là 1/100 (§2.3).
* `:root{-8e8e8:1}` (một dấu `-` rồi tới chữ số) vẫn là Dimension −8e8 và document vẫn
  bị từ chối. Browser thật chỉ bỏ đúng một declaration sai; bỏ luôn cả document là hành
  vi guard, cố ý **không** sửa ở task này (luật một tầng) — đây là việc của guard task
  nếu họ này tái xuất hiện trên trang thật.
* `+` và `.` vẫn được gửi thẳng cho `readNumber` (`+abc` thành Dimension `+`/`abc`):
  chưa đo tác động, chưa sửa, không có oracle.
* Nhánh `non-finite` của guard: không input nào đi qua token parser tới được đó
  (`ParseFloat` trả `out of range` trước), nên nó chỉ được assert một cách hình thức
  qua builder, không phải qua một document thật.
* Mutation (ii) không chạy được nguyên văn (`math.MaxFloat64` không compile); bằng chứng
  "cap còn tác dụng" dựa vào biến thể compile được ở nhánh + `TestMaxGeometryIsUnchanged`
  + hai test end-to-end.
* Oracle unit của tiền tố (`cmd/goosie`) không giết được mutation (v) — chỉ oracle
  end-to-end làm việc đó (§5.4).
* `make live-parity` đầy đủ 100 trang **không** được chạy ở đây (đang có sweep khác trên
  máy, và cần mạng); số cafef là run một host, không phải run toàn corpus.

## 6. Ngoài phạm vi

* Không đổi `25 s` deadline của `-screenshot` (`GOOSIE-LIVE-003`) — đó là task khác.
* Không root-cause họ "trang không vẽ gì" (`GOOSIE-LIVE-001`, TASK-039).
* Không nới bất kỳ ngưỡng admission nào khác (`MaxCSSDeclarations`, image caps).
* Không đụng metric của harness đo (`GOOSIE-LIVE-004`).
* Hai họ lệch còn lại **cố ý không sửa** vì luật một tầng: `:root{-8e8e8:1}` (một dấu `-`
  rồi tới chữ số) vẫn là Dimension và vẫn làm cả document bị từ chối, và `+`/`.` vẫn được
  gửi thẳng cho `readNumber`. Số đo ở §5.7; việc của task guard/tokenizer kế tiếp.
