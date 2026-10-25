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

**Kết luận trước tiên, và nó sửa §1.2:** đây không phải một họ lỗi layout. §2 tách ra được
**sáu nhánh root cause**, và **sáu trên tám trang `LAYOUT_EMPTY` chưa từng đến engine dưới dạng
một trang**: nguồn trả 401/403 kèm một trang chặn, engine dựng trang chặn đó thành 0 px ink, rc 0,
stderr rỗng. Câu "ít nhất **một lỗi thuộc đường fetch/response** (không phải layout)" ở §1.2 là
đúng hướng nhưng **sai chứng cứ**: nó dựa trên bảng so "bản curl 2,610,822 B" của apnews, mà phép
đo lại hôm nay cho thấy con số đó **không tái lập được** và offline probe của TASK-038 đã render
**một tài liệu mà Goosie không bao giờ nhận** (chi tiết ở 5.1).

Số trang thật so sánh được: **trước = 94/100** (run 2 của TASK-038, `2026-10-25T1656Z-4de0fca5.json`,
trong đó 8 `LAYOUT_EMPTY` + 1 `BLANK_OUTPUT` chính là họ này). **Sau = không đổi.** Ba commit của
task này không thay đổi một pixel nào của trang thật: gov.vn render ra **PNG 2,792 B cả trước lẫn
sau** khi sửa `nodeType`, apnews vẫn 0 px ink. Điều kiện §4.2 (đóng defect khi số trang so sánh
được tăng) **không đạt**, nên `GOOSIE-LIVE-001` **ở lại trạng thái mở**; ba nhánh còn mở được ghi
ở 5.11 và `docs/roadmap-v2.md`.

### 5.1 §2.1 — trên đường wire: gov.vn giống curl từng byte; apnews thì khác, nhưng không phải vì Goosie dựng request sai

Dùng lại chính client mà browser dùng (`internal/net.DefaultClient`) qua một probe tạm trong
`cmd/probe-t039/` (in `status/final/bytes/sha256` rồi bị xoá trước khi commit), so với curl cùng
**ba header y hệt** (UA của Goosie là `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)
AppleWebKit/537.36 (KHTML, like Gecko) Goosie/2.0` — `internal/net/http.go:35`; Goosie không tự đặt
`Accept-Encoding`):

```
GOOSIE  https://www.gov.vn/  status=200 final=https://chinhphu.vn/ bytes=192815
                              sha256=97544fae888ddebce91bb3644f52f9369e3d6d34372c6fbcf83dc16c0fae4e45
CURL -L https://www.gov.vn/  200 192815 redirects=1 final=https://chinhphu.vn/
                              sha256=97544fae888ddebce91bb3644f52f9369e3d6d34372c6fbcf83dc16c0fae4e45
```

⇒ **gov.vn: khác biệt wire = 0 byte.** Không có lỗi fetch cho nhánh này. (§1.1 gọi doc là
`www.gov.vn/`; cả hai phía đều kết thúc ở `https://chinhphu.vn/`.)

```
GOOSIE  https://apnews.com/  status=403 final=https://apnews.com/ bytes=5581   (lặp 1/3)
                              sha256=a636d1ebddc8c3e3e91af304504e84e5a66b0fb6385dd2c1fa03ed764d1f33b9
GOOSIE  https://apnews.com/  status=403 bytes=5581                              (lặp 2/3, 3/3)
CURL    https://apnews.com/  200 2607049 redirects=0 https://apnews.com/       (cả 3 lặp)
```

⇒ **apnews: khác biệt wire là thật và ổn định** — curl cùng header nhận trang 2,607,049 B,
Goosie nhận 403 + 5,581 B ba lần liền. Nhưng nguyên nhân **không nằm ở cách Goosie dựng request**:
`net/http` thuần, không có mã Goosie, đúng ba header đó, cũng bị chặn:

```
rawhttp: status=403 proto=HTTP/2.0 bytes=5581 resp_content_encoding=""
```

Tức Cloudflare (và các hệ chặn bot khác) phân biệt **stack TLS/HTTP2 của Go** với curl. Hệ quả đo
được cho toàn họ, bằng client của chính browser:

| Trang | status Goosie | bytes | Kết luận |
| --- | --- | --- | --- |
| `apnews.com/` | 403 | 5,581 | trang chặn |
| `www.nytimes.com/` | 403 | 771 | trang chặn |
| `www.japantimes.co.jp/` | 403 | 5,613 | trang chặn |
| `www.reuters.com/` | 401 | 774 | từ chối |
| `www.etsy.com/` | 403 | 779 | trang chặn |
| `www.newegg.com/` | 403 | 5,607 | trang chặn |
| `www.zara.com/` | 200 | 2,123 | shell rỗng, nội dung chỉ có sau JS |
| `www.gov.vn/` | 200 | 192,815 | trang thật, 11,859 ký tự nhìn thấy được |
| `vnexpress.net/` | 200 | 257,412 | trang thật |

**Giả thuyết bị falsify (1/3) — chính §1.2 của TASK-038.** Dòng "apnews: 2,610,822 B … KHÔNG tái
hiện offline. Cùng markup, cùng binary: offline thì vẽ, online thì không" so sánh **hai tài liệu
khác nhau**: bản mà offline probe render là body do Chromium tải (`ap-chrome.body`,
2,609,508 B, sha256 `a3d5bec8def63bd090f599215b7e357090b66ff08cd85989e37da6ddbebcb254`), còn tài
liệu Goosie thật sự nhận hôm nay là trang 403 5,581 B. Phép đo cũ của chính tôi (trước khi sửa)
từng kết luận "Goosie == curl, cả hai đều nhận trang challenge" — kết luận đó **bị falsify bởi
chính phép đo lặp ở trên**: curl nhận trang thật, Goosie nhận challenge.

### 5.2 §2.2 — ba số tách "fetch sai" / "parse rỗng" / "JS làm sập session"

Apnews online, `bin/goosie -backend headless -width 800 -height 600 -dpr 1 -screenshot`:

| | rc | PNG | `parity.ink_mask` | stderr | doc_height |
| --- | --- | --- | --- | --- | --- |
| trước sửa (1280×1024) | 0 | 6,109 B | **0 px** | **0 byte** | 0 |
| sau sửa (800×600) | 0 | 2,792 B | **0 px** | 1 dòng refusal | 0 |

Hai PNG khác nhau về số byte **vì khác viewport, không vì nội dung**: cả hai đều 0 px ink theo
chính hàm chấm của `testdata/parity.py`.

Tài liệu challenge có **58 ký tự nhìn thấy được** (`Just a moment... Enable JavaScript and cookies
to continue`) và **1 `<script>`**; phần text đó nằm trong khối chỉ hiện sau khi script của nó chạy
xong. Vậy apnews **không phải** "parse ra document rỗng" và **không phải** "JS làm sập session":
engine không nhận được trang, và cái nó nhận thì đúng là không có gì để vẽ trước JS. **Nhánh fetch
là nhánh thật**, chỉ là nó nằm ở phía "bị chặn", không phải phía "Goosie dựng request sai".

### 5.3 §2.3 — JS có ném không: có, và "một exception làm sập session" là sai

Probe tạm in mọi exception (`/tmp/t039/t039-instr.patch`, đã gỡ trước khi commit), chạy trên bản
lưu của gov.vn và vnexpress:

```
www.gov.vn      grep -c '[js-throw]' js-govvn.err  = 21     goosie: [js-summary] slots=35 ran=34
vnexpress.net   grep -c '[js-throw]' js.err       = 8      goosie: [js-summary] slots=33 ran=32
```

Lỗi đầu tiên trên gov.vn **chỉ thẳng bundle jQuery**:
`https://chinhphu.vn/scripts/jquery-3.6.0.min.js?v=1:1: TypeError: Cannot read property
'createElement' of undefined` (kèm stack `at ce (...):2:8857`).

**Giả thuyết bị falsify (2/3):** "một exception không bắt được có thể kết thúc script trước khi
nội dung hiện ra" (§2.3 nêu như một khả năng). Đo: `runScripts` gọi `_ = s.jsRT.Run(it.text, ...)` —
error bị **vứt bỏ**, không đổi control flow; script đứng sau vẫn chạy, và trên gov.vn **34/35 slot
đã chạy**. Exception không làm sập gì cả; chúng **không được ghi lại ở bất kỳ đâu**, kể cả khi
chính chúng là lý do. Oracle cho hành vi cô lập này: `TestUncaughtScriptErrorsAreRecordedAndIsolated`
assert `survived == 1` trước khi assert số lỗi.

### 5.4 §2.4 — gov.vn: vì sao layout rỗng khi markup có 11,859 ký tự

Toàn bộ body nằm trong một form bị ẩn, chỉ mở bằng jQuery:

```
<form method="post" action="/" id="form1" style="display: none">
$('#form1').fadeIn(200)      (lệnh reveal duy nhất trong tài liệu)
```

`display: none` → không box → `doc_height=0`, `tiles_rasterized=0`, `zero_work_frames=120` (§1.2,
vẫn đúng). **Layout hành xử đúng**; thứ không chạy là reveal. Chuỗi nhân quả đo được, từng bước:

1. Sizzle (trong jQuery 3.x) mở đầu bằng gate `9 === doc.nodeType && doc.documentElement`.
   Go binding **không có `nodeType`** → `undefined` → gate fail → library không tự cấu hình.
   Ordinal phải là **ordinal của DOM Standard**, không phải ordinal nội bộ: `internal/dom` đánh số
   từ 0 (document 0, element 1, text 2) nên binding phải dịch — truyền thẳng là sửa được document
   mà làm hỏng vòng duyệt node `1 === e.nodeType` của jQuery ngay commit đó.
2. Sau khi sửa (1), jQuery boot xa hơn rồi **chết ở bước tiếp theo**: lỗi đầu tiên trên gov.vn trở
   thành `TypeError: Object has no member 'createDocumentFragment'`; trên bản file:// offline là
   `Object has no member 'insertBefore'`. Đo lại cùng lệnh: PNG **2,792 B như trước**, ink vẫn 0.
3. Kể cả khi (2) xong, `fadeIn` là animation theo thời gian: cần event loop. Chế độ paced
   (`-screenshot`/`-gate`/`-bench`) **không cài `chromeWindow.setTick`**, nên `PumpJS` không bao giờ
   chạy và session đã load bị bỏ — mọi trang `js-revealed` trong sweep vì thế không có cơ hội hiện
   nội dung.

**Giả thuyết bị falsify (3/3):** "`expect: js-revealed` trong corpus **không phải** lời giải thích
cho apnews — HTML của nó chứa 70,685 ký tự nội dung tĩnh" (§1.2). Con số 70,685 ký tự thuộc tài
liệu Chromium tải, không thuộc tài liệu Goosie nhận (58 ký tự). Với **gov.vn** thì `static` là nhãn
đúng và chuỗi nhân quả ở trên là lời giải; với **apnews** nhãn `js-revealed` cũng không phải lời
giải thích — nhánh của nó là refusal ở bước 5.1.

### 5.5 §2.5 — vnexpress: 32 px ink là vẽ cái gì

Đo lại PNG Goosie bằng **chính hàm của `testdata/parity.py`**:

```
/tmp/t039/vne-offline.png  parity.ink_mask = 20 px   rows [0,1,2,3,4]  cols [0,1,2,3]
                           màu mẫu: (104,104,104) (163,163,163) (101,101,101)
                           parity.content_bands = 1     MIN_INK = 200
```

Đó là một **vệt xám 4×5 px ở góc trên-trái**, không phải glyph: 20 px offline / 32 px trong sweep
(§1.1), toàn bộ nằm trong rows 0–4 × cols 0–4. Kết luận §2.5: `BLANK_OUTPUT` của vnexpress **không
cùng họ** với `LAYOUT_EMPTY` — pipeline có đi tới paint, nhưng thứ nó paint là một artefact góc,
không phải nội dung; và vì `content_bands` của nó = 1, harness tính vệt đó như một dải nội dung.
Nhánh này để lại thành defect riêng (5.11), **không** sửa trong task này vì chưa có fixture +
oracle cho phía raster/tile.

### 5.6 §2.6 — bao nhiêu trang đứng sau họ này

`make live-parity --only <substring>` không chạy được: make parses `--only` thành target của riêng
nó. Chạy trực tiếp `python3 testdata/live-parity.py --binary ./bin/goosie --only <substring>
--out-dir /tmp/…` (out-dir ngoài repo để không ghi artifact mới vào `docs/qa-live-parity/`):

| Họ | chọn | so sánh 2 phía | LAYOUT_EMPTY | nhãn khác |
| --- | --- | --- | --- | --- |
| `.vn` | 18 | 16 | **1** (chỉ `www.gov.vn/`) | OK 5, LOW_COVERAGE 9, MISSING_REGIONS 1, ADMISSION_CAPPED 1, FETCH_FAILED 1 |
| commerce | 10 | 10 | **3** (etsy, newegg, zara) | OK 4, MISSING_REGIONS 3 |
| news-intl | 14 | 11 | **4** (apnews, nytimes, japantimes, reuters) | OK 4, LOW_COVERAGE 2, MISSING_REGIONS 1, FETCH_FAILED 3 |
| **tổng** | **42** | **37** | **8** | 29 trang vẫn vẽ được |

Đúng **8** trang bị lặp, trùng khớp danh sách của run 100 trang — không trang mới rơi vào họ này,
và **29 trang cùng danh mục vẫn paint được**, nên đây không phải sập toàn danh mục. Trong họ `.vn`
chỉ **1/18** trang thuộc defect. Theo corpus: `expect=js-revealed` có **39** trang; ở run 2 của
TASK-038, **12/39** trang đó có ink Goosie bằng 0/gần 0 (7 `LAYOUT_EMPTY` + 1 `BLANK_OUTPUT` + 4
`FETCH_FAILED`). Cộng gov.vn (`static`) = đúng chín trang của §1.1 — **không còn trang nào khác
đứng sau họ này** trong corpus 100 trang.

### 5.7 Sáu nhánh, ba commit

| Nhánh | Root cause | Trang | Xử lý |
| --- | --- | --- | --- |
| A | Nguồn trả 4xx; engine render trang chặn và **status không được ghi ở đâu** (`loadURLCtx` chưa từng đọc `resp.StatusCode`; media path thì đọc — `internal/engine/media_download.go:120`) | 6/8 | **đã sửa** — `7fd486b` |
| B | `nodeType` không tồn tại trên document và trên node → gate `9 === doc.nodeType` của Sizzle/jQuery fail | gov.vn + toàn họ js-revealed | **đã sửa** — `1f6684c` |
| C | Node/DOM API jQuery 3.6 cần mà binding chưa có: `createDocumentFragment`, `insertBefore`, `cloneNode`, `childNodes`/`lastChild`, `ownerDocument`, `defaultView`, `getComputedStyle`, `readyState` + dispatch `DOMContentLoaded`/`load` | gov.vn (vẫn blank), zara | **mở** → `docs/roadmap-v2.md` |
| D | Chế độ paced không pump event loop JS (thiết kế nhất quán của các cổng đo) | mọi trang js-revealed trong sweep | **mở** → `docs/roadmap-v2.md`, theo Bước 2 §3: ghi roadmap, **để defect mở**, không hạ severity |
| E | Engine vứt bỏ exception (`_ = s.jsRT.Run`), nên lý do không tồn tại ở bất kỳ đâu | 8/9 trang có script | **đã sửa** — `88a3bd4` |
| F | vnexpress: vệt grey 20–32 px ở góc (0,0) là artefact, không phải nội dung | 1 | **mở** — defect riêng, chưa có fixture/oracle phía raster |

Ba commit đều **không đổi output render** (đo: PNG 2,792 B trước = sau cho gov.vn; rc vẫn 0).
Dòng mới chỉ ghi lý do, sau paint, trên cùng đường input không tin cậy đã giữ
`Session.PaintChecked`; không thêm đường nhận input nào.

### 5.8 Mutation proof [R-7]

| # | Tắt gì | Test có tên nào CHẾT | Verbatim |
| --- | --- | --- | --- |
| M1 | cả hai `Set("nodeType", …)` trong `internal/js/js.go` | `TestNodeTypeUsesDOMStandardOrdinals`, `TestLibrarySetupGatePassesTheDocument` | `nodeType ordinals = ",,,,", want "9,1,1,1,3"`; `shown = 0, want 1` |
| M2 | `recordScriptError` trong `runScripts` | `TestUncaughtScriptErrorsAreRecordedAndIsolated`, `TestScriptErrorWindowAndMessageAreBounded` | `ScriptErrorCount() = 0, want 2 (the bundle and the inline script)`; `= 0, want the full 40` |
| M3 | call site `reportScriptErrors` trong `loadURLCtx` | `TestGoosieScreenshotReportsUncaughtScriptErrors` | `a document whose script threw produced no reason on stderr; stderr was ""` |
| M4 | call site `reportRefusedDocument` | `TestGoosieScreenshotReportsARefusedDocument` | `a 403 document reported no reason on stderr; stderr was ""` |
| M5 | ngưỡng `>= 400` → `>= 0` (báo cả trang 200) | `TestGoosieScreenshotStaysSilentOnAnAcceptedDocument` | `a 200 answer reported a refusal: "goosie: the source refused http://127.0.0.1:50147/ with HTTP 200; …"` |
| M6 | recording cho userscript | **sống sót lần đầu** — `./test/engine` và `./cmd/goosie` đều `ok` | nên đã thêm `TestUserScriptErrorsAreRecordedAndLaterScriptsStillRun`; chạy lại M6: `ScriptErrorCount() = 0, want 1 (the broken userscript)` → **đã bị giết** |

Không có mutation nào còn sống ở thời điểm commit.

### 5.9 Fixture và oracle đã đo

* `testdata/render/js-dom/01-library-document-gate.html` (nhánh B): một khối chỉ hiện khi gate
  Sizzle-shaped `nodeType !== 9` đi qua. **Trước** sửa: 2 blank band; **sau**: 98.29% PASS, 0 blank
  band. Chromium PNG của fixture là ảnh đo thật, không tự vẽ.
  Lưu ý thiết kế: fixture nào có output trắng phía Goosie sẽ bắt `make parity` đỏ và buộc phải ghi
  vào `testdata/render-known-gaps.json` — tệp **cấm sửa** trong task này — nên nhánh E/A được neo
  bằng Go tests + stderr đo thật thay vì bằng fixture render.
* `// qc:BR-…`: **không gắn được tag nào mới.** `test/qc/traceability_test.go` fail với mọi
  `// qc:<ID>` không có trong `docs/qa-test-catalog.md`, và golden
  `test/qc/traceability.golden.json` chỉ đổi được bằng `-update`; cả hai đều nằm trong danh sách cấm.
  Ba BR id đề nghị ở 5.13.

### 5.10 Điều kiện nhận §4.3 (một sweep duy nhất, không song song)

```
make parity                    → 81/81 passing (100.0%), 0 failing, 0 errors, average match 96.10%
                                  known gaps: 5 fixtures / 10 bands (PARITY_RC=0)
make live-parity-selftest      → selftest green (gồm "a clean run records no diagnostic")
go test ./...                  → không một dòng FAIL; các package chạm vào:
                                  internal/engine 2.011s, test/engine 9.211s,
                                  test/entrypoints 42.042s, test/js 7.125s
go test -count=1 ./cmd/...     → ok cmd/goosie 11.699s   (dòng này bị tail -60 cắt ở sweep)
go vet -all ./...              → rc 0
go test -count=1 ./internal/archtest/ → ok internal/archtest 3.693s
uptime trước: 1:53 up 5 days, 3:15, load averages: 6.95 7.98 10.73  (Sun Oct 25 18:53:05 UTC 2026)
uptime sau:  1:53 up 5 days, 3:16, load averages: 10.06 8.73 10.87  (Sun Oct 25 18:53:50 UTC 2026)
```

Không có hai sweep song song; sweep live chạy riêng từng họ với `--out-dir` ở `/tmp`.

### 5.11 Còn mở, chưa biết

1. **Nhánh C** — chưa đo được bao nhiêu trang chỉ cần `createDocumentFragment` + `insertBefore` là
   mở khoá; cần một task riêng với fixture cho từng API, vì mỗi API thêm một đường mutation DOM mới.
2. **Nhánh D** — chưa đo blast radius của việc pump JS trong chế độ paced: mọi con số gate/bench/
   screenshot hiện tại đều là "một lượt render đồng bộ"; bật pump sẽ đổi **toàn bộ** đường chuẩn,
   nên nó không được làm ở đây.
3. **Nhánh A** — engine không vượt được bot management vì fingerprint TLS/H2 của Go (đo ở 5.1).
   Chưa biết chi phí/độ an toàn của việc bắt chước Chrome TLS (`crypto/tls` vs `utls`), và đây là
   quyết định chính sách chứ không phải bug sửa một dòng.
4. **Nhánh F** — chưa biết vệt 20 px góc (0,0) sinh ra từ tile nào; cần probe phía `raster`/`frame`
   với fixture tối thiểu, chưa có trong task này.
5. `goosie_diag` của sweep giờ **đầy** với trang 4xx và trang throw exception; chưa chạy lại sweep
   100 trang để xác nhận nhãn không đổi ngoài selftest (task này chạy ba họ, 42 trang).
6. Chưa kiểm chứng trên native macOS — mọi số ở đây là headless.

### 5.12 Chạy lại

```
go build -trimpath -ldflags "-s -w" -o bin/goosie ./cmd/goosie
go test -count=1 ./test/js/ -run 'TestNodeType|TestLibrarySetupGate'
go test -count=1 ./test/engine/ -run ScriptError
go test -count=1 ./cmd/goosie/ -run 'ReportScriptErrors|ReportRefusedDocument'
go test -count=1 ./test/entrypoints/ -run 'ScriptError|RefusedDocument|SilentForAClean|SilentOnAnAccepted'
make parity && make live-parity-selftest

# nhánh A: dòng lý do trên chính trang sống
bin/goosie -backend headless -width 800 -height 600 -dpr 1 -screenshot -url https://apnews.com/ -out /tmp/ap.png 2>&1 >/dev/null | grep refused
# nhánh B/E trên bản lưu gov.vn (không cần mạng cho quyết định nodeType)
bin/goosie -backend headless -width 800 -height 600 -dpr 1 -screenshot -url file:///tmp/t039/goosie/https_chinhphu_vn_.html -out /tmp/gov.png 2>/tmp/gov.err; cat /tmp/gov.err
# §2.6, từng họ (make không truyền được --only)
python3 testdata/live-parity.py --binary ./bin/goosie --only .vn       --out-dir /tmp/t039/live
python3 testdata/live-parity.py --binary ./bin/goosie --only-category commerce   --out-dir /tmp/t039/live2
python3 testdata/live-parity.py --binary ./bin/goosie --only-category news-intl  --out-dir /tmp/t039/live2
```

### 5.13 Dòng đề nghị cho registry (chỉ đề nghị — task này cấm sửa các tệp QA)

* `GOOSIE-LIVE-001` (P1, **mở**): "Tám trang thật không vẽ gì. §2 tách thành sáu nhánh; ba nhánh
  đã có fix (refusal status, `nodeType`, exception recording); ba nhánh còn mở: Node/DOM API
  completion, paced mode không pump event loop, artefact góc của vnexpress. Số trang so sánh được
  **chưa tăng** nên defect chưa đóng."
* `GOOSIE-LIVE-005` (P2, đề nghị mở mới): "một tài liệu bị nguồn từ chối (4xx) được render như một
  trang bình thường và được gắn nhãn `LAYOUT_EMPTY`; status nay đã có trên stderr, nhưng nhãn của
  harness vẫn gộp 'bị chặn' với 'engine không dựng ra gì'."
* `GOOSIE-LIVE-006` (P3, đề nghị mở mới): "vệt ink 20–32 px ở góc (0,0)–(4,3) trên PNG vnexpress;
  `content_bands` tính nó như một dải nội dung."
* BR id mới cho catalog: **BR-186** cho "JS binding phải phơi ordinal của DOM Standard (`nodeType`)
  và dịch từ ordinal nội bộ" (oracle: `test/js/node_type_test.go`, fixture
  `testdata/render/js-dom/01-library-document-gate.html`); **BR-187** cho "trang trắng phải nói
  được lý do: exception được ghi trong cửa sổ chặn và một dòng refusal cho 4xx" (oracle:
  `test/engine/script_errors_test.go`, `cmd/goosie/script_errors_test.go`,
  `test/entrypoints/script_error_report_test.go`, `test/entrypoints/refused_document_report_test.go`).
  Cả hai chỉ là đề nghị: gắn `// qc:` và nâng sàn ratchet là việc của task QA, không phải task này.

## 6. Ngoài phạm vi

* `cafef.vn` / admission guard → TASK-040.
* Flag phân biệt "chưa settle" → `GOOSIE-LIVE-003`.
* Metric vị trí (worst_iou chưa được dùng để chấm) → `GOOSIE-LIVE-004`.
* Full-page screenshot, scroll pipeline, web fonts, cookie overlay — mỗi họ một task riêng sau
  khi có số (đúng như TASK-038 §5 đã hẹn).
