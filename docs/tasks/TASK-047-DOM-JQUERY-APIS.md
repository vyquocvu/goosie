# TASK-047 — Bộ DOM API mà một content script thật gọi tới (`createDocumentFragment`, `insertBefore`, `cloneNode`, `childNodes`, `getComputedStyle`, `readyState`, `DOMContentLoaded`/`load`)

Defect liên quan: **GOOSIE-LIVE-001** (P1, `status: open`, `docs/qa-open-defects.json`),
nhánh C — "page có script, Goosie dựng cây nhưng script chết sớm nên nội dung hiện
không ra". Task này không đóng defect; nó đo và gỡ từng mảng một, có số ở mỗi bước.

Branch: `worktree-task-047-dom-jquery-apis`, base `fe4928c`, chỉ làm việc trong
`.worktrees/task-047`. Không push, không PR, không merge.

## 0. Phạm vi và quy tắc đã binding

- **Không sửa** file của integrator: `docs/qa-test-catalog.md`, `docs/qa-open-defects.json`,
  `docs/qa-ratchet.json`, `test/qc/catalog.go`, `test/qc/traceability.golden.json`,
  `testdata/render-known-gaps.json`. Không thêm marker `// qc:` vào test.
- **Không chạy** `make live-parity` hay `testdata/live-parity.py`. Chromium chỉ được dùng
  qua chính `make parity` (`python3 testdata/parity.py --render --render-chromium
  --binary ./bin/goosie`) để đo reference cho fixture mới.
- Scope do **trang quyết định**, không do danh sách API trong đề bài. Mục 1 là số đo; mục 6
  liệt kê những tên để ngoài và lý do.
- Một điều chỉnh nhỏ về tên tài liệu: commit `24ee057` (feat(engine) dẫn file này bằng đường
  dẫn `docs/tasks/TASK-047-DOM-JQUERY-BOOT-APIS.md`. File thật là file bạn đang đọc,
  `docs/tasks/TASK-047-DOM-JQUERY-APIS.md`; mọi comment trong code đã trỏ đúng đường dẫn này.
  Message của commit đã tạo thì không viết lại (không amend).
- **Đọc trước khi tin số**: §0–§9 là lượt của tôi, §10–§11 là một lượt đo **độc lập** của một
  agent khác trên cùng worktree này (nghệ thuật của họ: `/tmp/t047-work`, không Chromium, không
  `make parity`, không sửa chữ nào của §0–§9). Hai lượt trùng một file vì file doc bị ghi hai
  lần; không bên nào xoá bên nào. §8 là danh sách BR id cho các commit của tôi; §11 là đề nghị
  của lượt kia (và §11.3.3 đã bắt tôi sửa §5.5 — xem dưới).
- Ba chỗ đã phải **đính chính bằng đo** trong lượt của tôi, ghi lại vì cả ba đều là lỗi của
  chính tôi lúc viết: (1) §3.3 — bản fixture đầu không gate gì cả, một mutant cho render
  **giống từng byte**; đã dựng lại fixture và đo lại bằng hai mutant; (2) §2.6 — accessor của
  property chưa resolve trả `undefined` (key không tồn tại), chỉ `getPropertyValue` mới trả
  `""`; (3) §5.5 — reveal bất đồng bộ có **hai** nguyên nhân, không phải một.

Six commit, theo thứ tự:

| SHA | Loại | Nội dung |
| --- | --- | --- |
| `5b2434f` | `feat(dom)` | primitive: `InsertNode`, `ReplaceNode`, `ChildNodes`, `Contains`, `IsConnected`, `Document.NewDocumentFragment`, `Event.PropagationStopped` |
| `4054bde` | `feat(js)` | binding: `node_api.go`, `window_events.go`, `computed_style.go`, `location.go`, `document_implementation.go` + 6 file test |
| `24ee057` | `feat(engine)` | `computed_style.go` (reader của cascade) + `engine.go` (lắp provider, nổ lifecycle, re-resolve) + 1 file test |
| tiếp theo | `test(engine)` | oracle admission cho `cloneNode(true)` |
| tiếp theo | `docs(js)` | sửa ba khai báo đo Sai trong comment (`nextSibling`, `checked`) |
| tiếp theo | `test(parity)` | fixture `js-dom/02` + nâng `MIN_SCORED_FIXTURES` 87→88 |
| tiếp theo | `docs(tasks)` | tài liệu này |

---

## 1. Bằng chứng đã đo

### 1.1 Artifact (tất cả bản lưu nằm ngoài repo, dưới `/tmp/t047`)

| Artifact | Bytes | sha1 | Là gì |
| --- | --- | --- | --- |
| `jquery-3.6.0.min.js` | 89,501 | `b82d238d4e31fdf618bae8ac11a6c812c03dd0d4` | bundle mà www.gov.vn nạp (`/scripts/jquery-3.6.0.min.js?v=1`) |
| `_b2.html` | 201,234 | `1cd00f0612ca20e726e0b711ce94bacc2ae17aa9` | bản lưu trang chủ vnexpress.net (198,049 ký tự UTF-8) |
| `saved-www_gov_vn_.html` | 192,815 | `170d505f6ac1084d2bfcd7113c94d877ed2ccb43` | bản lưu www.gov.vn (redirect thật: chinhphu.vn) |
| `saved-vietnamnet_vn_.html` | 247,993 | `9c52a204abad09c1767a07a89013e46c9ac512e7` | bản lưu vietnamnet.vn |
| `saved-tuoitre_vn_.html` | 277,242 | `f2d770eba3065c9cb4a81b722120886369037712` | bản lưu tuoitre.vn |
| `saved-nhandan_vn_.html` | 43,549 | `424d6f304662fe7aaaef8811309e55da6f0f4928` | bản lưu nhandan.vn |
| `jquery-sync-reveal.html` | 91,436 | `0cb046e0da614df94c0e76c1ee22b70d8794abcf` | trang tái hiện: bundle 89,501 B **inline**, path đồng bộ |
| `jquery-reveal.html` | 91,702 | `11f98022922d7c9a271578c8505a68c89d520159` | như trên, nhưng reveal đi qua `jQuery.ready` (Deferred) |

Vì sao phải tái hiện bằng trang inline bundle: `cmd/goosie-headless` luôn gắn JS runtime
với **fetcher = nil**, nên `<script src>` ngoài không bao giờ chạy offline. Trên bản lưu
gov.vn, bundle nằm ngoài (`grep` cho 22 thẻ `<script src>`, không có `jQuery v`); vì vậy
`createDocumentFragment` chỉ đo được trên trang inline, còn trên bản lưu thì chỉ đo được
hậu quả (`$ is not defined`).

Log và script của từng thí nghiệm (tất cả ngoài repo, giữ nguyên để đối chiếu dòng nào tới từ đâu):

| Artifact | Là gì | Chứng minh cho |
| --- | --- | --- |
| `base-src/` | bản export của `fe4928c` (không có `.git`) | mọi phép so sánh "trước change"; vì không có git nên ba test `test/gatecheck` RCSweep ở đó đỏ **do môi trường**, khai rõ ở §7.1 |
| `goosie-base`, `goosie-mutant`, `goosie-muc`, `goosie-mud` | binary HEAD / mutant `insertBefore(x,nil)` / mutant bỏ provider / mutant bỏ `DOMContentLoaded` | §3.3 — bốn con số 36.03 / 61.93 / 42.97 / 42.97% |
| `headless-base`, `headless-after`, `probe-base`, `probe-after` | `cmd/goosie-headless` và bản probe hai phía | §1.2, §4.2, §4.3, §4.4 |
| `mutation-recheck.log`, `mutation-recheck2.log` | hai round chạy lại mutation trên cây cuối | §3.2 (M08/M16/M21/MU-A và M02/M05/M06/M11) |
| `fixture-oracle.log`, `fixture-oracle.sh` | lệnh harness + điểm `parity.py` + đếm màu cho từng binary | §3.3 |
| `determinism.sh`, `det/run1..6.png` | sáu render độc lập một fixture, md5 bằng nhau | nghi án "render không xác định" mà thực chất là lẫn artifact mutant |
| `bandcolors.py`, `bandink.py` | đếm px theo dải dòng, import **chính** `ink_mask` của harness | §3.3 (25,359 px xanh của ta / 25,205 của reference) |
| `load-race.sh`, `load-race.log`, `lr-*.log` | thí nghiệm tải branch-vs-base cho `cmd/goosie` | §7.1 |
| `final-gates.log`, `final-gates2.log`, `final-gates3.log`, `gate-*.log`, `race-*.log` | các battery gate, gồm cả những lần đỏ | §7 |
| `evidence-probe-base.log`, `evidence-probe-after.log`, `check-*.txt` | runtime reads của từng trang | §4.3, §4.4, §5 |

Lệnh đếm (chạy lại được):

```bash
python3 - <<'PY'
import re, hashlib
src = open('/tmp/t047/_b2.html', encoding='utf-8', errors='replace').read()
print(len(open('/tmp/t047/_b2.html','rb').read()), hashlib.sha1(open('/tmp/t047/_b2.html','rb').read()).hexdigest())
print("insertBefore total", len(re.findall(r'insertBefore\(', src)))
print("literal null ref   ", len(re.findall(r'insertBefore\([^()]*,\s*(?:null|void 0|undefined)\s*\)', src)))
print("parentNode.ib      ", len(re.findall(r'parentNode\.insertBefore\(', src)))
print("window listeners   ", len(re.findall(r'window\.addEventListener\(', src)))
PY
```

Kết quả trên `_b2.html`: `insertBefore(` 9 site, **0** site truyền `null` tường minh,
cả 9 đều là `parentNode.insertBefore(...)`; hai site truyền `x.nextSibling`
(offset 113,261 và 195,744) — giá trị này **bằng null đúng khi node chèn nằm sau child cuối**,
tức nhánh append vẫn tới từ trang. `window.addEventListener(` 6 site: `DOMContentLoaded` ×4,
`load` ×1, `pageshow` ×1.

### 1.2 Engine TRƯỚC change (`headless-base`/`probe-base`, build từ `fe4928c`) chạy trên từng bản lưu

```
/tmp/t047/probe-base -in <file> -js -width 1280 -height 1024
```

```
##### _b2.html                                     (vnexpress)
script_errors_total=6
err[0] inline:1: TypeError: Object has no member 'insertBefore' at inline:1:8689(41) at inline:1:9007(681)
err[1] inline:1: TypeError: Object has no member 'insertBefore' at inline:1:229(43) at inline:1:236(7)
err[2] inline:1: TypeError: Cannot read property 'indexOf' of undefined or null at validateCachePage (inline:6:53(4)) at inline:7:18(96)
err[3] inline:1: TypeError: Object has no member 'addEventListener' at inline:1:73(13)
err[4] inline:1: TypeError: Object has no member 'addEventListener' at inline:1:1509(8)
err[5] inline:1: TypeError: Object has no member 'addEventListener' at inline:2:24(17)
js_live_nodes=2043
extent=Rect(0,1 6630x723)
cmds text=0 glyphs=0 fill=28 border=2 image=0

##### saved-www_gov_vn_.html
script_errors_total=10
err[0] inline:1: TypeError: Object has no member 'insertBefore' at inline:15:58(50) at inline:16:31(23)
err[1..8] inline:1: ReferenceError: $ is not defined at ...   (8 site)
js_live_nodes=3485
extent=Rect(0,0 0x0)
cmds text=0 glyphs=0 fill=0 border=0 image=0

##### saved-vietnamnet_vn_.html
script_errors_total=5
err[0] inline:1: TypeError: Cannot read property 'protocol' of undefined at inline:7:57(16) at inline:9:15(24)
err[1] inline:1: TypeError: Object has no member 'insertBefore' at inline:1:280(60) at inline:1:289(7)
err[2] inline:1: ReferenceError: VnnModules is not defined at inline:6:13(5)
err[3] inline:1: ReferenceError: $ is not defined at inline:49:6(210)
err[4] inline:1: ReferenceError: vnnidmodule is not defined at inline:2:5(0)
js_live_nodes=2837
extent=Rect(8,-2 1264x7541)
cmds text=2103 glyphs=5119 fill=713 border=0 image=0

##### saved-tuoitre_vn_.html
script_errors_total=4
err[0] inline:1: TypeError: Object has no member 'insertBefore' at inline:9:90(60) at inline:10:11(11)
err[1] inline:1: TypeError: Object has no member 'insertBefore' at inline:8:81(51) at inline:9:11(11)
err[2] inline:1: TypeError: Object has no member 'addEventListener' at inline:22:32(5) at inline:30:7(12)
err[3] inline:1: TypeError: Object has no member 'addEventListener' at inline:3:32(9)
js_live_nodes=4000
extent=Rect(-20,-16 2264x2409)
cmds text=1450 glyphs=3833 fill=1234 border=28 image=0

##### saved-nhandan_vn_.html
script_errors_total=0
js_live_nodes=49
extent=Rect(8,6 3137x8026)
cmds text=1541 glyphs=38136 fill=0 border=0 image=0

##### jquery-sync-reveal.html   (bundle inline, path đồng bộ)
script_errors_total=1
err[0] inline:1: TypeError: Object has no member 'createDocumentFragment' at inline:2:37814(645) at inline:2:221(29) at inline:2:225(11)
js_live_nodes=42
extent=Rect(0,0 1280x56)
cmds text=26 glyphs=71 fill=14 border=0 image=0

##### jquery-reveal.html        (bundle inline, reveal qua jQuery.ready)
script_errors_total=2
err[0] inline:1: TypeError: Object has no member 'createDocumentFragment' at inline:2:37814(645) at inline:2:221(29) at inline:2:225(11)
err[1] inline:1: ReferenceError: jQuery is not defined at inline:5:7(2)
js_live_nodes=42
extent=Rect(0,0 1280x56)
cmds text=28 glyphs=77 fill=16 border=0 image=0
```

Ba kết luận, tất cả đều là lỗi in ra chứ không phải suy luận:

1. `Object has no member 'insertBefore'` là lỗi **đầu tiên** trên 4/5 trang thật, và là
   2/6 lỗi trên vnexpress.
2. `Object has no member 'addEventListener'` (trên `window`) tồn tại ở cả vnexpress và tuoitre.
3. `Cannot read property 'protocol' of undefined` trên vietnamnet là `location` không có parts.
4. Trên trang có bundle inline, lỗi là `createDocumentFragment` — statement DOM đầu tiên
   của chính thư viện, nghĩa là thư viện chết trước khi kịp export `jQuery`.

### 1.3 Đếm trong bundle thật (jquery-3.6.0.min.js, 89,501 B)

| Tên | Số lần xuất hiện | Ghi chú đo |
| --- | --- | --- |
| `createDocumentFragment` | 2 | support block @37,880 là statement DOM đầu tiên của cả thư viện |
| `insertBefore` | 5 | 4 call site (50,065 `prepend`; 50,186 `before`; 50,297 `after`; 80,979 `wrapAll`, **thiếu hẳn argument thứ hai**) + 1 khoá alias `insertBefore:"before"` @51,156 |
| `replaceChild` | 1 | `replaceWith`: `t.replaceChild(e, this)` |
| `cloneNode` | 5 | gồm chain `ce.cloneNode(!0).cloneNode(!0).lastChild.checked` |
| `childNodes` | 11 | gồm `slice.call(document.childNodes)` |
| `firstChild` / `lastChild` | 12 / 5 | `prepend` / `append` / checkClone |
| `nextSibling` / `previousSibling` | 10 / 6 | `dir` walk |
| `ownerDocument` | 24 | Sizzle `setDocument` + `Re(e)` của `.css()` |
| `parentNode` | 54 | |
| `nodeValue` | 1 | `.text()` |
| `contains` | 13 | |
| `defaultView` | 5 | `n = C.defaultView` @8,738 |
| `getComputedStyle` | 3 | chuỗi `Re` @51,467: `(n = n \|\| Re(e)) && ("" !== (a = n.getPropertyValue(t) \|\| n[t]))` |
| `readyState` | 6 | nhánh ready @32,281/@32,322 |
| `createHTMLDocument` | 4 | @84,477 / 84,518 / 84,707 / 84,750 |
| `hasFeature` | 0 | nên không binding |

Chuỗi ready của jQuery, nguyên văn từ bundle:

```js
E.addEventListener("DOMContentLoaded", B), C.addEventListener("load", B)
```

`E` là document, `C` là window — thiếu một trong hai thì `$(document).ready(...)` không bao
giờ chạy. Đó là lý do `window` phải là EventTarget thật, không chỉ document.

### 1.4 Hàng rào mà **live sweep** (artifact của integrator) đã ghi, dùng làm "before"

`docs/qa-live-parity/2026-10-25T2212Z-4302e3cb.json` (build `9754fda…`, viewport 1280×1024,
dpr 1, Chromium/Playwright là reference):

```
https://www.gov.vn/     label LAYOUT_EMPTY   doc_height=0   ink_goosie=0    ink_chromium=502474
  goosie_diag: goosie: 21 uncaught script error(s) in https://chinhphu.vn/; first:
    https://chinhphu.vn/scripts/jquery-3.6.0.min.js?v=1:1: TypeError: Object has no member
    'createDocumentFragment' at https://chinhphu.vn/scripts/jquery-3.…
https://vnexpress.net/  label BLANK_OUTPUT   doc_height=752 ink_goosie=0    ink_chromium=281813
  goosie_diag: goosie: 8 uncaught script error(s) in https://vnexpress.net/; first:
    inline:1: TypeError: Object has no member 'insertBefore' at inline:1:8689(41) at inline:1:9007(681)
https://vietnamnet.vn/  label LOW_COVERAGE   doc_height=4447 ink_goosie=175912 ink_chromium=522337
  goosie_diag: goosie: 6 uncaught script error(s) in https://vietnamnet.vn/; first:
    inline:1: TypeError: Cannot read property 'protocol' of undefined at inline:7:57(16) …
https://tuoitre.vn/     label LOW_COVERAGE   doc_height=1531 ink_goosie=187402 ink_chromium=765157
```

Hai thứ đáng chú ý: live sweep **nạp được bundle** (có mạng) nên nó mới là nơi đo được
`createDocumentFragment` trên gov.vn; còn offline thì nó không hiện diện. Cả hai nguồn đều
khớp với 1.2 — cùng một lỗi, cùng một thứ tự.

---

## 2. Quyết định ngữ nghĩa — nhỏ, nhưng đúng như standard mô tả

Mỗi quyết định dưới đây có một oracle riêng trong `test/js/` hoặc `test/engine/`; mục 3 dán
kết quả mutation để chứng minh oracle đó thật sự đỏ khi mechanism bị xoá.

### 2.1 `document.createDocumentFragment()`

Trả về một **Node thật** (`nodeType 11`), mang prototype methods như mọi node khác. Đây là
điều kiện cần của statement đầu tiên trong bundle:
`var ce = E.createDocumentFragment().appendChild(E.createElement("div"))` — vừa cần
`appendChild` tồn tại trên fragment, vừa cần nó **trả về chính child** để `ce` không phải
`undefined`.

### 2.2 `Node.insertBefore(x, ref)`

- `ref` null / undefined / **thiếu hẳn argument** ⇒ append (DOM §pre-insert step 1). Ba
  chứng cứ đo được: jQuery 50,065 (`t.firstChild` null khi parent rỗng), 50,297
  (`this.nextSibling` null khi là child cuối), 80,979 (`t.insertBefore(this[0])` — chỉ một
  argument); vnexpress tới nhánh này qua `a.nextSibling` (offset 113,261 / 195,744).
- Argument là fragment ⇒ **các child của fragment** đi vào theo thứ tự, fragment rỗng sau đó
  (jQuery `buildFragment` trả đúng một fragment và dùng lại nó).
- `ref` không phải child của `this` ⇒ **throw `NotFoundError`** (catchable), không im lặng
  append: im lặng biến bug của trang thành nội dung đặt sai chỗ.
- `newChild === refChild` ⇒ no-op ở tầng primitive `dom.Node.InsertBefore`, vì splice sẽ
  tạo vòng tự tham chiếu và làm mọi walk (style/layout/serialize) lặp vô hạn.

### 2.3 `Node.cloneNode(deep)`

`false` nhân bản mỗi node; `true` nhân bản cả subtree. `deep` đọc theo chuẩn:
`len(args) > 0 && args[0].ToBoolean()`, nên `cloneNode()` ≡ `cloneNode(false)` và
`cloneNode(!0)` ≡ deep. Chain của jQuery là `ce.cloneNode(!0).cloneNode(!0).lastChild.checked`
— một deep-clone-of-clone rồi đọc child-list; nếu một trong hai là shallow thì `lastChild`
null và probe của thư viện throw.

Clone là **detached** nên không bị tính vào node budget của document — cùng cách xử lý mà
`document.createElement` vẫn dùng; lần append sau đó mới charge toàn bộ subtree (mục 2.10).

Deviation có tên: `internal/dom` giữ trạng thái form control **bằng attribute**
(`js.go` `__getChecked__`/`__setChecked__`/`__getValue__`/`__setValue__`), và `CloneNode` copy
cả slice `Attr`, nên `checked` và một `value` đã bị script gán đều đi sang bản clone — đó là
lý do chain checkClone trả `true`. Giá phải trả: không có "dirty value flag" tách riêng, nên
`defaultValue` không có gì để phân biệt với `value` (mục 6).

### 2.4 `Node.childNodes` — snapshot, không phải live NodeList

`childNodes` trả một **mảng snapshot mới**, không phải live NodeList, và đó là quyết định có
ý thức. Live list cần một hook vô hiệu hoá cho mỗi lần mutation cùng một identity mà standard
hứa; `internal/js` không có cả hai. Hai pattern đã đo vẫn đúng với snapshot:

- `for (var i = 0; i < el.childNodes.length; i++) el.removeChild(el.childNodes[i])` (vòng
  `cleanData` của jQuery) — mỗi lần lặp đọc mảng một lần;
- `while (el.firstChild) el.removeChild(el.firstChild)` — đọc lại từ đầu mỗi bước, snapshot
  không sao cả.

Thứ thật sự cần live list (record `previousSibling` của MutationObserver) đã được phục vụ bởi
bộ ghi mutation, không bởi property này.

### 2.5 Các read còn lại

`firstChild`, `lastChild`, `nextSibling`, `previousSibling`, `isConnected`, `nodeValue`
và `ownerDocument` là **accessor sống** (`DefineAccessorProperty`, read-only), không phải data
property snapshot như `className`/`tagName` (lịch sử của nhóm sau: xem mục 6).

- `ownerDocument` trả **chính object document toàn cục**, không phải một wrapper giống hệt:
  Sizzle `setDocument` so sánh `r != C` rồi trỏ nguyên cache nội bộ của thư viện vào thứ nó
  nhận được, nên một wrapper giống mà không phải sẽ được nhận làm document rồi hỏng ngay ở
  property đầu tiên chỉ document thật có. `document.ownerDocument` là `null`.
- `nodeValue`: trả `DataContent` cho text/comment, `null` cho element/document/fragment theo
  chuẩn; **writable** cho hai loại lá (ghi vào `DataContent`, `MarkQueryDirty`,
  `notifyMutation`).

### 2.6 `window.getComputedStyle(elt, pseudoElt)`

Trả về đúng những gì engine gọi là "engine values": các trường cascade mà session đã resolve
(`s.Styles[n.ID]`) cộng `W`/`H` mà layout arena đã đo. Engine không **suy dẫn** giá trị nào.
"Không có gì để báo" có hai hình thái, và cả hai đều được đo bằng `probe-after -check` trên
node mà session chưa resolve (`document.createElement('div')`):

```
window.getComputedStyle(<detached div>).display                 => "undefined"   // key không tồn tại
window.getComputedStyle(<detached div>).getPropertyValue('display') => ""        // hàm luôn có mặt
```

Node mà session chưa từng resolve ⇒ provider trả `nil` ⇒ object chỉ còn `getPropertyValue` và
0 key cascade — một trang đo node off-tree thấy khoảng trống thật (property ** absent**, đọc ra
`undefined`), thay vì một đáp án hợp lý nhưng bịa. Ngược lại, trên `#panel` của fixture đã
resolve, hai cách đọc cùng trả `block` và object có 69 key.

- `getPropertyValue("font-size")` **và** read camelCase (`styles.fontSize`) phải cùng trả lời:
  jQuery thử camelCase trước, rồi mới `getPropertyValue`; thiếu một trong hai thì trang đọc
  `""` và kết luận là property chưa set — tức một câu trả lời sai **im lặng**, tệ hơn throw.
- Argument thứ hai **được nhận và bỏ qua**: box của pseudo-element không phải node mà layout
  tree mang, nên không có gì trung thực để báo. Ghi thành "chưa làm" ở mục 6.
- Inline declarations được merge **sau** cascade snapshot, nên inline thắng. Chi phí phải nói
  thẳng: `internal/js` không được import `internal/css` (bảng import của archtest), nên màu
  authored trả về **đúng như người viết gõ** (`#ee0000`), không phải dạng đã chuẩn hoá.

### 2.7 `document.readyState` và thứ tự lifecycle

Ba hằng đúng chính tả standard: `loading`, `interactive`, `complete`. Trong lúc body script
chạy, `readyState === "loading"`; `FireDocumentLifecycle()` (một lần mỗi document, khoá bằng
`r.lifecycleFired`) nổ `DOMContentLoaded` trên document, đẩy state lên `interactive`, rồi nổ
`load` trên window và đẩy lên `complete`.

Nổ **sau** lần layout đầu là một lệch-timing có chủ đích, và lý do là số đo: Goosie chạy mọi
script sau khi cả document đã tồn tại, nên một ready handler đọc computed style phải nhìn thấy
cascade của lần load, không phải một style map rỗng. Giá phải trả: writes của handler đến sau
layout, nên nếu mutation stamp đổi thì session chạy `RestyleAndReflow` ngay tại đó — và
**chỉ khi stamp đổi** (chứng minh bằng `engine.WorkCounts`: `StyleResolves`/`LayoutBuilds`
không tăng khi handler không sửa gì).

`window` không phải `*dom.Node`, nên listener của nó không thể nằm trong registry của
`internal/dom` (registry keyed by node pointer). Registry thuộc về runtime
(`r.winListeners` + `r.winMu`), và bước-window của propagation path được gắn thêm bởi
`dispatchWithWindowStep`: window-capture → `dom.DispatchEvent` → window-bubble, và **bỏ**
bước bubble nếu handler gọi `stopPropagation` (`Event.PropagationStopped()` vừa được mở ở
commit `5b2434f` là để reading này nằm ngoài `DispatchEvent` mà vẫn đúng).

### 2.8 `window` EventTarget

`addEventListener`/`removeEventListener`/`dispatchEvent` trên window, dedup theo
`(function value, capture)`, listener không phải function bị bỏ qua **im lặng** (đúng hành vi
browser: `document.addEventListener("load", null)` là no-op hợp lệ). `dispatchEvent` trên
window không lan xuống document — hướng lan là target → window, không ngược lại.

### 2.9 `location`

`href/protocol/host/hostname/port/pathname/search/hash/origin` là các parts **sống**, tính từ
URL của session và đi theo history (khi session đổi URL thì parts đổi theo). Trên `_b2.html`,
`validateCachePage` đọc `location.hostname` 33 lần; statement đầu của nó là
`if (window.location.hostname.indexOf('demo.') !== -1) { return; }` (offset 66,563), tức
toàn bộ hàm chết vì `Cannot read property 'indexOf' of undefined` trước khi binding này tồn tại.
`location` coerce ra `href`, và `document.location === window.location` (cùng một object).

Writes và members điều hướng **cố ý không có** (mục 6).

### 2.10 `document.implementation.createHTMLDocument(title)`

Không phải member mà một *trang* đọc — là member mà **thư viện của mọi trang đó** đọc đầu
tiên. Bundle chết ngay ở probe无条件 `E.implementation.createHTMLDocument("").body`
(@84,477…), và `S.parseHTML` với mỗi fragment không-single-tag lại tới nó lần nữa; hậu quả đo
được: `jQuery` không bao giờ được export.

Document trả về là **detached**: không style, không layout, không paint; skeleton đúng bốn
node standard dựng (doctype, html, head + title, body rỗng); title set qua đường text node
chứ không splice argument vào source, vì parser đọc `<title>` như raw text nên `&lt;` sẽ về
nguyên 4 ký tự và một argument chứa `</title>` sẽ đổi cả cây.

**Fix đi kèm, đo được** (`internal/js/js.go`, `indexDocFor`): query index phải rooted theo
**document của node gốc**, không phải document toàn cục. Thiếu nó thì
`detachedDoc.getElementById(...)` hoặc `querySelector` rooted ở document tạm tìm nhầm cây —
đây chính là nhánh `parseHTML` của jQuery (`t.createElement("base")`, `t.head.appendChild(r)`).

### 2.11 Admission: clone thì free, **cài** clone thì bị kiểm

`internal/js/admission.go` gọi `MutationBudget.Reserve`, và engine trả `nil` cho parent
**non-live**: subtree detached không bị charge và cũng không bị depth-check (đúng với
`createElement`). Hệ quả mà `cloneNode(true)` mở ra: một script có thể build 140 tầng nesting
**trong cây detached**, clone cả cây, rồi `appendChild` vào `body`. Nếu chỉ dựa vào
`measureSubtree`, phép đo ấy throw ở độ sâu 129 và... vẫn chặn, nhưng chặn **sai chỗ**: nó
dùng chung bound với cái mà post-parse pass sẽ áp cho document đang sống, nên không chứng minh
được rằng gate nằm ở lần cài.

Vì vậy cổng có **hai** phần và test tách riêng từng phần:

1. Cài một subtree mà chính nó đã sâu hơn `MaxDocumentDepth` ⇒ `measureSubtree(c)` trả error
   ⇒ `QuotaExceededError` catchable, document còn lại nguyên vẹn.
2. Cha đang sống ở độ sâu `pd`, con cao `height`: `if pd+1+height > MaxDocumentDepth` ⇒ từ chối
   dù subtree hợp lệ một mình (40 tầng sống + clone 99 tầng).

**Không thêm cap mới** trong `internal/engine/limits.go`: cả hai gate dùng `MaxDocumentNodes
= 1000000` và `MaxDocumentDepth = 128` sẵn có (dòng 27-28). Bộ limit của `createHTMLDocument`
(1024 node / depth 32 / 256 attribute / 4096 byte) là literal **trong `internal/js`**, không
phải ở `limits.go`, vì archtest (`internal/archtest/rules.go:40-64`) cho `internal/js` import
`{internal/dom, internal/media}` — muốn đọc cap của engine thì phải import `internal/engine`,
và `internal/engine` import `internal/js` nên vòng import sẽ hỏng build. Con số chọn theo đúng
bộ `internal/js` đã áp cho một lần viết `innerHTML` (js.go, fragment path: 1024 node, depth 32).
Cổng này **chưa** được soi ở tầng `test/js`: `fakeMutationBudget` trong
`test/js/admission_test.go` charge cả insert dưới parent detached, tức không phản ánh
`Reserve` thật — nên oracle của nó nằm ở `test/engine/js_admission_test.go`, chạy qua
`engine.NewSession` thật và `Session.PaintChecked` (untrusted-input binary vẫn giữ hợp đồng
`PaintChecked`).

---

## 3. Oracle

### 3.1 Một cluster, một file test

| Cluster | File | Số case |
| --- | --- | --- |
| Node tree (fragment, insertBefore, replaceChild, cloneNode, reads) | `test/js/node_tree_test.go` | 12 |
| Window EventTarget + readyState + DOMContentLoaded/load | `test/js/window_lifecycle_test.go` | 9 |
| `getComputedStyle` trong realm + `defaultView` | `test/js/computed_style_test.go` | 8 |
| `location` parts | `test/js/location_parts_test.go` | 9 |
| `document.implementation` + detached document | `test/js/document_implementation_test.go` | 8 |
| Biểu thức boot thật của jQuery 3.6.0 | `test/js/jquery_dom_boot_test.go` | 4 |
| Cascade của session + repaint vì lifecycle | `test/engine/computed_style_render_test.go` | 5 |
| Admission của clone sâu | `test/engine/js_admission_test.go` | 1 (mới) |

Cả 6 file `test/js` đều là package `js_test`, chạy `js.New(...)` trực tiếp (không qua engine),
tức chúng gate đúng tầng binding; hai file `test/engine` gate tầng session.

### 3.2 Mutation — 24 lần, 23 bị giết, 1 survivor tương đương

Mỗi lần: sửa một mechanism, chạy `go test -count=1 ./test/js/ ./test/engine/` (và
`./test/js/ -run <anchor>` cho các case lẻ), đọc exit code **trực tiếp** (`>log 2>&1; echo
exit=$?` — không qua pipe), khôi phục từ backup, rồi `gofmt -l` + `git diff --stat` để chắc
không còn xác mutant. Bảng đầy đủ (exit code là của `go test`, không phải của pipeline):

| Mutation | File | Exit | Dòng đỏ đầu tiên (verbatim, cắt 80 ký tự) |
| --- | --- | --- | --- |
| M01 `installNodeTreeMethods()` bị xoá | `internal/js/js.go` | 1 | `document_implementation_test.go:107: Evaluate((function(){` |
| M02 `setupWindowEvents()` bị xoá | `internal/js/js.go` | 1 | `computed_style_test.go:258: defaultView shape = "true\|true\|true\|null\|undefined\|function\|true", want "true\|true\|true\|nu…` |
| M03 location parts không được cài | `internal/js/js.go` | 1 | `location_parts_test.go:101: page pattern result = "THREW:Cannot read property 'hostname' of undefined", want the matched…` |
| M04 `document.implementation` không cài | `internal/js/js.go` | 1 | `document_implementation_test.go:68: Evaluate(typeof document.implementation.createHTMLDocument): eval:1: TypeError: Cann…` |
| M05 `document.defaultView` thiếu | `internal/js/js.go` | 1 | `computed_style_test.go:230: defaultView identity = "false,false", want "true,true"` |
| M06 `window.getComputedStyle` thiếu | `internal/js/js.go` | 1 | `computed_style_test.go:103: inline:1: TypeError: Object has no member 'getComputedStyle'` (4 test chết: `:103`, `:116 Evaluate(`, `:129 …ReferenceError: getComputedStyle is not defined`, `:169 …`) |
| M07 accessor `childNodes` bị xoá | `internal/js/node_api.go` | 1 | `document_implementation_test.go:68: Evaluate((function(){var d=document.implementation.createHTMLDocument("Scratch");ret…` |
| M08 accessor `lastChild` bị xoá | `internal/js/node_api.go` | 1 | `document_implementation_test.go:68: Evaluate((function(){var d=document.implementation.createHTMLDocument("Scratch");return […]})()): eval:1: TypeError: Cannot read property 'nodeType' of undefined` (8 test chết; anchor của chính cluster là `node_tree_test.go:119: insertBefore = "B\|I\|U/NotFoundError/3", want "B\|U\|I/NotFoundError/3"`) |
| M09 accessor `ownerDocument` bị xoá | `internal/js/node_api.go` | 1 | `document_implementation_test.go:69: own documentElement identity: got "false,false" want "true,true"` |
| M10 query index không rooted theo document của node | `internal/js/js.go` | 1 | `document_implementation_test.go:150: parseHTML base branch: got "https://example.test/page,2,false,false,true,BASE" want…` |
| M11 không merge inline vào computed style | `internal/js/computed_style.go` | 1 | `computed_style_test.go:130: inline color = "undefined", want the element's own declaration` |
| M12 engine không lắp provider | `internal/engine/engine.go` | 1 | `computed_style_render_test.go:102: inherited font-size: got "undefined" want "16px"` |
| M13 session không nổ lifecycle | `internal/engine/engine.go` | 1 | `computed_style_render_test.go:213: lifecycle state seen by page handlers: got "///loading/false" want "interactive/1/com…` |
| M14 `childNodes` chỉ trả element | `internal/js/node_api.go` | 1 | `document_implementation_test.go:69: two root children: got "1" want "2"` |
| M15 `document.readyState` accessor thiếu | `internal/js/window_events.go` | 1 | `jquery_dom_boot_test.go:197: jQuery ready branch: got "timeout/dom+win//undefined/true" want "listeners/dom+win/loading/…` |
| M15b `insertBefore(x, null)` không append | `internal/dom/node.go` | 1 | `admission_test.go:269: divCount = {Kind:3 Num:0 Str: Bool:false}, want 3` |
| M16 `cloneNode` bỏ qua flag shallow | `internal/js/node_api.go` | 1 | `node_tree_test.go:167: cloneNode = "2,2,DIV,c,false,2,true", want "0,2,DIV,c,false,2,true"` — và trên cây cuối, oracle admission mới **cũng** chết: `js_admission_test.go:200: shallow clone insert: caught = "QuotaExceededError", want no error` + `:203: body child count after the shallow insert = {Kind:2 Num:0 Str: Bool:false}, want exactly one more` |
| M17 `appendChild(fragment)` cài cả fragment | `internal/dom/node.go` | 1 | `document_implementation_test.go:240: detached constructors: got "3,hi,true,8,c,11,2,1,,undefined" want "3,hi,true,8,c,11…` |
| M18 `load` không bao giờ được dispatch | `internal/js/window_events.go` | 1 | `jquery_dom_boot_test.go:215: ready handlers: got "doc:interactive" want "doc:interactive,win:complete"` |
| M19 `DOMContentLoaded` không bao giờ được dispatch | `internal/js/window_events.go` | 1 | `jquery_dom_boot_test.go:215: ready handlers: got "win:complete" want "doc:interactive,win:complete"` |
| M20 bỏ bước window của bubble path | `internal/js/window_events.go` | 1 | `window_lifecycle_test.go:251: bubbling path = "box,document", want "box,document,window"` |
| M21 `setReadyState` là no-op (state đóng băng ở `loading`) | `internal/js/window_events.go` | 1 | `jquery_dom_boot_test.go:210: FireDocumentLifecycle reported no DOM mutation; that is the expected case here, the handlers only record state` rồi `:215: ready handlers: got "doc:loading,win:loading" want "doc:interactive,win:complete"` (3 test chết, thêm `window_lifecycle_test.go:83: readyState after the lifecycle = "loading", want "complete"`) |
| MU-A tắt so sánh độ sâu lúc cài | `internal/engine/jsadmission.go` | 1 | `js_admission_test.go:183: NewSession failed: HTML tree depth limit exceeded (128) (the cloned subtree must be refused at insertion, not after it lands)` |
| MU-B nâng bound của `measureSubtree` | `internal/engine/jsadmission.go` | **0** | *survivor* — xem dưới |

Hai anchor đầu tiên của M14 và M17 bị script mutate bỏ (`SKIP-ANCHOR … occurrences: 3` /
`occurrences: 2`) — cả hai được chạy lại ở round sau và đều chết (bảng trên là kết quả chạy
lại, không phải lần định chạy).

Tám dòng **M02 / M05 / M06 / M08 / M11 / M16 / M21 / MU-A** được chạy lại trên cây cuối, thành
hai round, vì các edit comment (`internal/js/node_api.go`, `internal/js/computed_style.go`) và
thêm assertion (`test/js/computed_style_test.go`) làm **số dòng trong file test đổi** —
`node_tree_test.go:113 → :119`, `:161 → :167`. Round 1 (`/tmp/t047/mutation-recheck.log`) chạy
M08/M16/M21/MU-A; round 2 (`/tmp/t047/mutation-recheck2.log`) chạy M02/M05/M06/M11. Kết quả
round 2: M02 vẫn là `computed_style_test.go:258`, M05 vẫn là `:230`, M11 vẫn là `:130` — ba dòng
đó đúng nguyên văn như bảng trên — nhưng **M06 đổi chỗ đỏ đầu tiên**: không còn là assertion
`defaultView shape` (dòng cũ tôi dẫn là `:253`), mà là `:103: TypeError: Object has no member
'getComputedStyle'`, vì chính test đó chạy trước trong package. Mỗi round đều có một lần baseline
và một lần sau khi restore, cả hai `exit=0` (`test/js 3.397s`, `test/engine 5.497s`).

MU-A, verbatim (cả hai oracle cùng đỏ):

```
--- FAIL: TestEngineJSDeepNestingStoppedAtBoundary (0.09s)
    js_admission_test.go:49: NewSession failed: HTML tree depth limit exceeded (128) (admission must stop the flood at the boundary, not fail the document)
--- FAIL: TestEngineJSCloneOfDeepDetachedSubtreeIsCheckedAtInsertion (0.09s)
    js_admission_test.go:183: NewSession failed: HTML tree depth limit exceeded (128) (the cloned subtree must be refused at insertion, not after it lands)
FAIL	github.com/vyquocvu/goosie/test/engine	1.000s
MU-A exit=1
```

MU-B (`if e.depth > MaxDocumentDepth` → `MaxDocumentDepth+20`) là **equivalent mutant**, khai
báo thẳng thay vì nhận vơ: với bound của `measureSubtree` nới ra, chiều cao của subtree clone
(139) vẫn đi tới `pd+1+height > MaxDocumentDepth` và vẫn từ chối; hai gate chồng nhau, nên
nới một trong hai không đổi kết quả. Test vẫn xanh (`MU-B exit=0`), và đó là bằng chứng gate
đang chặn **ở chỗ thứ nhất**, không phải chỗ thứ hai.

### 3.3 Fixture render, và floor

`testdata/render/js-dom/02-tree-mutation-and-lifecycle.html` (fixture PARITY thứ 88) sơn, theo
đúng thứ tự từ đầu trang, ba hàng `#panel` mà handler của `DOMContentLoaded` mở ra **sau khi đọc
computed style** (rows 62-97 xanh, 104-139 dương, 146-181 đỏ), một dòng báo cáo do listener
`load` sinh ra (`FRAG 0/11/3 INSERT 18/3 CLONE 2/2/1 READY loading THEN complete OWNER true
HIDDEN true`, rows 190-205), rồi ba hàng fragment, ba hàng `insertBefore` (xanh / xám / đỏ theo
đúng thứ tự mà `insertBefore` quyết định), clone deep + shallow và bản gốc đặt cạnh nhau. Hàng
được cắt 36px + 6px để tất cả xuống tới dòng báo cáo còn nằm trong 600px viewport — đó là điểm
khác biệt sống còn so với bản đầu, xem dưới.

Bốn binary, một reference Chromium (đo bằng `bandcolors.py`/`bandink.py` ngoài repo, import
chính `ink_mask` của harness; `make parity` render cả hai phía ở runtime):

| Binary so với reference | Score | Kết luận | ink | blank regions |
| --- | --- | --- | --- | --- |
| Chromium reference (`--render-chromium`) | — | chuẩn | 333,761 | — |
| HEAD branch `bin/goosie` | **96.79%** | **PASS** | 333,036 | 0/0 |
| `fe4928c` (trước change) | **36.03%** | **FAIL** | 27,360 | 4/0 — `rows=[(190, 205), (222, 236), (254, 541), (549, 599)]` |
| mutant: `insertBefore(x, nil)` cắm xuống đầu (`internal/dom/node.go`) | **61.93%** | **FAIL** | 333,027 | 0/0 (lượng ink không đổi, **thứ tự** đổi) |
| mutant: `computedValues` bỏ provider (`internal/js/computed_style.go`) | **42.97%** | **FAIL** | 279,899 | 1/0 — `rows=[(549, 599)]`, xanh = 0 px |
| mutant: không dispatch `DOMContentLoaded` (`internal/js/window_events.go`) | **42.97%** | **FAIL** | 279,957 | 1/0 — `rows=[(549, 599)]`, xanh = 0 px |

Verbatim từ harness (lần chạy cuối, mục 7):

```
   96.79%  PASS  js-dom/02-tree-mutation-and-lifecycle.png  worst_region_iou=0.9585814476644171  coverage_ratio=0.9977858407662968  blank_regions=0/0  rows=[]  shifted=0
  88/88 passing  (100.0%), 0 failing, 0 errors
  average match: 96.15%  (threshold=30, pass_at=90.0%)
```

**Tiết lộ đo được khi làm fixture, và nó đổi thiết kế.** Bản đầu đọc
`window.getComputedStyle(panel).display` ngay trong body script và đặt `#panel` sau 13 hàng do
script dựng. Hai hậu quả, cả hai đều đo chứ không suy ra:

*_body script đọc sớm hơn cascade*: `probe-after -check` trả `INLINE CS=[undefined]` cho lần đọc
trong body script và `READY CS=[none] NONE=true` cho lần đọc trong handler — tức đặt câu đọc
vào lifecycle không phải phong cách, nó là cách duy nhất để câu đó có giá trị.
*panel nằm sau fold*: render của bản đầu kết thúc ở row 599 với **0 px** màu `#008000`, và khi
tắt hẳn provider thì ảnh render ra **giống từng byte** (md5 `32607afa6303828b7117412b567224e4`,
ink 299,036 cả hai), điểm vẫn 96.70%. Con số 96.70% mà tôi từng ghi là của một mutant render
(mixed artifact trong `/tmp/goosie-renders`), và chính thí nghiệm so md5 đã buộc phải nhìn ra
điều đó. Nói cách khác bản đầu là một oracle **tưởng là gate mà không phải gate**; đã dựng lại
theo hướng block do lifecycle mở ra đặt lên đầu, ba hàng, và đo lại bằng đúng hai mutant ở bảng
trên — giờ mỗi mechanism đều cắt 6 điểm phần trăm trở lên.

`MIN_SCORED_FIXTURES` nâng 87 → 88 **trong cùng commit** với fixture (tăng-only), kèm comment
nêu lý do và con số đo:

```
# 88: js-dom/02-tree-mutation-and-lifecycle.html (TASK-047) - createDocumentFragment,
# insertBefore (both forms), cloneNode shallow/deep, childNodes/lastChild/ownerDocument,
# getComputedStyle, DOMContentLoaded+load. Without the API bindings the script throws
# on its first line and Goosie paints 27,360 ink (36.03%) where Chromium paints
# 333,761; answering getComputedStyle from nothing else scores 42.97%, and inserting
# a null reference at the front instead of the end scores 61.93%.
MIN_SCORED_FIXTURES = 88
```

Average của cả corpus **không giảm** khi thêm fixture: 87/87 average 96.14% (trước, `gate-parity.log`
07:17) → 88/88 average 96.15% (sau). `test/gatecheck/parity_harness_test.go` đếm file HTML thật và
so với đúng dòng floor đó, nên fixture và floor không thể lệch nhau.

Reference của fixture **không** commit ảnh: `make parity` render cả hai phía ở runtime
(`--render` vào `/tmp/goosie-renders`, `--render-chromium` vào `/tmp/playwright-renders`), nên
con số Chromium đến từ Playwright đúng như ràng buộc của task.

---

## 4. Bằng chứng trên trang thật, offline

### 4.1 Lệnh

Hai binary `cmd/goosie-headless`: `headless-base` (build từ `fe4928c`) và `headless-after`
(build từ branch này), render cùng một file ở 1280×1024 dpr 1, đếm ink bằng **chính**
`ink_mask` của harness:

```bash
python3 /tmp/t047/ink.py _b2.html saved-www_gov_vn_.html saved-vietnamnet_vn_.html \
                          saved-tuoitre_vn_.html saved-nhandan_vn_.html \
                          jquery-sync-reveal.html jquery-reveal.html
# mỗi trang: <binary> -in <page> -out <png> -width 1280 -height 1024 -dpr 1
# ink = parity.ink_mask(np.asarray(PIL.Image.open(png).convert("RGB"))).sum(),  INK_CUTOFF=245
```

### 4.2 Ink trước / sau

```
viewport 1280x1024 dpr 1.0; ink via parity.ink_mask (INK_CUTOFF=245)
page                                 before ink    after ink     delta  ink-bbox before -> after
jquery-sync-reveal.html                   54904        65448    +10544  1280x60 -> 1280x237
jquery-reveal.html                        54872        54872        +0  1280x60 -> 1280x60
_b2.html                                      0            0        +0  0x0 -> 0x0
saved-www_gov_vn_.html                        0            0        +0  0x0 -> 0x0
saved-vietnamnet_vn_.html                  7350         7350        +0  257x1018 -> 257x1018
saved-tuoitre_vn_.html                   191172       191172        +0  1280x1024 -> 1280x1024
saved-nhandan_vn_.html                   380645       380645        +0  1272x1018 -> 1272x1018
```

Một mình bảng này sẽ là một báo cáo thất bại nếu không kèm giải thích; §4.4 đo từng nguyên
nhân. Điều bảng này **có** chứng minh: change làm được, trên đúng một trang thật, khi thư viện
chạy hết path đồng bộ — `jquery-sync-reveal.html` +10,544 ink và bbox cao gần gấp bốn
(60 → 237). Trang `jquery-reveal.html` (reveal đi qua `jQuery.ready`) vẫn 0 delta, và lý do
nằm ở timer pump, không ở DOM API (§5.3, §6).

### 4.3 Số lỗi uncaught, trước / sau (cùng lệnh `-js -width 1280 -height 1024`)

| Trang | errors trước | errors sau | Xóa được |
| --- | --- | --- | --- |
| vnexpress `_b2.html` | 6 | 1 | 2× `no member 'insertBefore'`, 3× `no member 'addEventListener'`, 1× `Cannot read property 'indexOf' of undefined` |
| gov.vn | 10 | 9 | 1× `insertBefore`; còn 9× `$ / jQuery is not defined` |
| vietnamnet | 5 | 3 | `protocol` của `location`, `insertBefore` |
| tuoitre | 4 | **0** | 2× `insertBefore`, 2× `addEventListener` |
| nhandan | 0 | 0 | — |
| jquery-sync-reveal | 1 | **0** | `createDocumentFragment` |
| jquery-reveal | 2 | **0** | `createDocumentFragment`, `jQuery is not defined` |

`js_live_nodes` cũng nhích (2043→2049, 3485→3486, 2837→2839, 4000→4002): script đi xa hơn một
chút, tạo thêm node — không phải paint thêm.

### 4.4 Vì sao vnexpress vẫn 0 ink, và ba phần của câu trả lời (đo, không đoán)

1. **`Image` không tồn tại.** Lỗi duy nhất còn lại trên `_b2.html` là
   `ReferenceError: Image is not defined at validateCachePage (inline:6:832(100))` — hàm này
   chết ngay dòng đầu, và nó là hàm dựng nội dung. Binding không có `Image` (§6).
2. **Box có kích thước thật, nhưng display list không có text.** Đọc bằng runtime check:
   ```
   ./probe-after -in _b2.html -js -width 1280 -height 1024 -check '<chain of the first dated link>'
   check="A txt= Thứ hai, 26/10/2026  disp=inline w=135.17188px h=17px | 0:SPAN#-.time-now d=block
   w=135.17188px h=17px vis=visible kids=3 | 1:DIV#-.container_ d=flex w=1100px h=56px vis=visible
   kids=9 | 2:HEADER#-.section_top-header d=block w=1280px h=56px vis=visible kids=3 |
   3:BODY#dark_theme.- d=block w=1280px h=1024px vis=visible kids=100 | 4:HTML#-.- d=block
   w=1280px h=1024px vis=visible kids=12 | 5:#document#-.- d=undefined w=undefined h=undefined
   vis=undefined kids=2"
   ```
   nghĩa là: cascade resolve `inline`, layout đo `135.17188x17`, `visibility` visible, opacity
   1 (đo riêng `h1`/`a`: `d=block w=0px h=0px vis=visible op=1 fs=16px`, `anchors=133
   nonEmptyText=76`) — **và vẫn** `cmds text=0 glyphs=0`. Đây là tầng display-list của chính
   trang, không phải một defect chung của flex: cùng hình dạng đó viết tay, cả hai biến thể
   `display:flex` và `display:block` đều sơn (`cmds text=4 glyphs=18`).
3. **Nút fill duy nhất trong viewport không phải ink.** Ô 0xF7F7F7 — min channel 247 — nên
   `ink_mask` (cutoff 245) không đếm; đó là lý do `fill=28` mà ink vẫn 0.

Trên gov.vn offline: `body h=0px`, `#form1 display:none` cỡ `0pxx0px` với 47 child,
`readyState=complete`, extent `0x0` — không có gì để sơn, và 9 lỗi `$` còn lại là bundle không
nạp được offline, không phải DOM API thiếu.

---

## 5. Còn hỏng trên năm trang đó (khai báo, không giấu)

1. **vnexpress**: còn 1 lỗi (`Image is not defined`) chặn `validateCachePage`; display-list
   không sinh text command nào cho dù box đã resolve và đo cỡ thật; 0 ink không đổi.
2. **gov.vn**: 9 lỗi `$ / jQuery is not defined` — bundle nằm ngoài (`<script
   src="/scripts/jquery-3.6.0.min.js?v=1">`) và offline thì `cmd/goosie-headless` có fetcher
   `nil`. Panel reveal không chạy, `#form1` vẫn `display:none`, extent `0x0`.
3. **vietnamnet**: 3 lỗi còn lại (`VnnModules`, `$`, `vnnidmodule`), ink không đổi 7,350.
4. **tuoitre / nhandan**: 0 lỗi trước và sau (tuoitre giảm 4→0), ink không đổi — nội dung của
   chúng không đi qua các API của task này.
5. **`jquery-reveal.html`**: bundle boot thành công (`jQuery is not defined` biến mất, 0 lỗi)
   nhưng reveal vẫn không sơn. Hai nguyên nhân, cả hai có số đo, và bản đầu của câu này chỉ nêu
   một (đính chính theo §11.3.3):
   - `jQuery.ready` đi qua `Deferred` → `setTimeout`, mà `cmd/goosie-headless` **không pump
     timer** (GOOSIE-LIVE-001 nhánh D). Cùng file reproA dưới harness có pump thì sơn: 0 px →
     **35.943 px** (§10.2).
   - **và**, khi reveal đi qua `$(".sel").show()` với selector bị ẩn bởi *rule stylesheet*,
     vẫn hỏng **dù đã pump**: §10.4 đo `probe-head -js -inv -pump 3` trên reproB và thấy
     `#form1.style.cssText === ""`, `getComputedStyle(#form1).display === "none"`, extent `0x0`
     — `defaultDisplay` của jQuery không lấy được display mặc định của một node vừa cài trong
     cùng task, nên phép thử "đang ẩn?" luôn là yes và `.show()` không ghi gì cả. Đó là
     `GOOSIE-CSSOMTASK-001` (§11.2), không phải defect DOM của task này.
   Trang tái hiện path **đồng bộ** (§4.2, +10,544 ink) vẫn chứng minh phần DOM đã thông.

---

## 6. KHÔNG làm — danh sách có tên và lý do

| Tên | Lý do |
| --- | --- |
| `className` → thuộc tính `class` | Đo được rằng `className` là **data property snapshot** (`js.go:1248`, refresh ở `js.go:1397`); gán `className` không ghi attribute nên không bao giờ select được. `jQuery.addClass` chính là `elem.className = value`. Fix nằm ở tầng wrap-snapshot, không phải ở cluster này; fixture `js-dom/02` dùng `setAttribute` và ghi rõ lý do trong comment. |
| `href` IDL reflection, `defaultValue` | Cùng họ `className`: cần cơ chế reflection attribute↔IDL, chưa có. Vì state form trong Goosie là attribute (`__setChecked__`/`__setValue__`), `defaultValue` cũng không có gì tách khỏi `value`. |
| `Image` constructor | vnexpress gọi (`validateCachePage`). Đây là subresource + loader, không phải DOM tree; đã ghi để task sau. |
| `location.assign/replace/reload` | Engine chưa có navigation. Gọi sẽ TypeError **nêu tên member thiếu** — thất-đoà-loud đúng hợp đồng của package; một no-op im lặng để lại trang tưởng đã đổi trang. |
| `location.searchParams`, `ancestorOrigins` | 0 occurrence trên các trang đo; `new URL(href).searchParams` đã có. |
| Setter của `location.href/protocol/...` | Read-only; browser làm writes navigate. |
| `document.implementation.createDocument`, `hasFeature`, `createCSSStyleSheet` | `hasFeature` 0 occurrence trong bundle; `createDocument` không trang nào gọi; stub chúng chỉ để `typeof` hết là `undefined` sẽ là phỏng đoán. |
| Selector univers `*` trong query index | Không nằm trong cluster này và không phải thứ các trang đo chết; để riêng. |
| Argument thứ hai của `getComputedStyle` (pseudo-element) | Layout tree không mang box pseudo → không có gì trung thực để báo; nhận và bỏ qua, có comment. |
| Shape đầy đủ của `CSSStyleDeclaration` | Đo ở §10.5: accessor của tên engine không mang **không tồn tại** trên object (`typeof` → `undefined`) trong khi `getPropertyValue` trả `""`; `length`/`item()`/`parentRule` đều `undefined` nên `Array.prototype.slice.call(getComputedStyle(el))` ra `[]`. CSSOM chốt cả hai read đều là chuỗi. Không làm ở đây vì nó cần mixin thuộc tính theo danh sách property của engine, không phải thiếu một binding; đã đề nghị `GOOSIE-CSSOMTASK-001` (§11.2). Hành vi hiện tại **đã được neo** bằng test: `test/js/computed_style_test.go` assert `typeof cs.zIndex === "undefined"` bên cạnh `getPropertyValue("line-height") === ""`, nên ai đổi shape sẽ bị đỏ thay vì im lặng. |
| Màu/norm hoá giá trị trả về | `internal/js` không được import `internal/css` (archtest), nên inline colour về đúng dạng authored (`#ee0000`). Đổi chỗ này cần đổi provider sang `internal/engine`. |
| Cross-document graft (WrongDocumentError) | `adopt()` hợp lệ; browser throw. Không trang đo nào graft, throw sẽ đổi lỗi cũ (`insertBefore`) thành lỗi mới trên cùng một trang. |
| Exception từ listener bị nuốt | `fireWindowListeners` gọi `l.fn(...)` và bỏ cả `Value` lẫn `error` (`_, _ =`), nên handler throw không tới bộ đếm script error. Hành vi cũ của browser là report ra console, không phải halt script; task này giữ nguyên để không đổi số `script_errors` của sweep. |
| Pump timer trong `cmd/goosie-headless` | `jQuery.ready` đi Deferred→setTimeout, nên path bất đồng bộ không sơn (§5.5). Thuộc về TASK-017 (fetch drain determinism), không phải DOM API. **Và** đây chỉ là một trong hai nguyên nhân: §10.4 đo cùng path khi **đã** pump vẫn +0, vì `defaultDisplay` cần computed style của một node vừa cài trong cùng task — defect đó là `GOOSIE-CSSOMTASK-001` (§11.2), nằm ở provider `internal/engine/computed_style.go`, ngoài cluster này. |
| Live `NodeList`, `MutationObserver` records | §2.4. |
| `HTMLCollection` vs snapshot `children` | `children` giữ nguyên hành vi cũ; không trang đo nào chạm. |

Và một cap **không** thêm: xem §2.11.

---

## 7. Gates

Chạy một mạch trên **cây cuối** (bốn commit code/test đã đóng gói + fixture + doc), worktree này,
không có job nào khác của tôi chạy song song.  Exit code lấy trực tiếp từ lệnh
(`… > log 2>&1; echo exit=$?`), không bao giờ từ pipeline.  Log: `/tmp/t047/final-gates3.log`
(mở đầu bằng `git status --short` của chính cây đó, nên bảng trên là những gì đã được chấm).

```
### gofmt -l over every file this task touched
gofmt-touched exit=0 output-bytes=0

### gofmt -l .
gofmt-all exit=0 output-lines=0

### go vet -all ./...
vet exit=0 output-bytes=0

### go test -count=1 ./internal/archtest/
archtest exit=0
ok  	github.com/vyquocvu/goosie/internal/archtest	3.190s

### go test -count=1 ./...
test exit=0
ok-packages=42

### go test -race -count=1 ./...  (load at start: { 9.35 21.81 18.44 })
race exit=0
ok-packages=42

### make gate
make-gate exit=0
go run ./cmd/goosie -gate -scene checkerboard -frames 600 -out /tmp/gate.json
goosie: run backend=darwin requested="auto" interactive=true frames=600 stamped=600 elapsed_s=5.01 fps=119.9 scene=checkerboard size=2880x1800 window=2880x1654 dpr=2 startup_ms=144.136
goosie: timings mean_ms=1.515 p50_ms=1.580 p99_ms=2.210 max_ms=10.239 present_mean_ms=0.262 present_p99_ms=0.684 zero_work_frames=362
goosie: counters rasterized=2976 reused=53461 failed=0 refused=0 deferred=0 panics=0 writes=359141632 bytes=780140544 budget=785383424 evictions=0 miss_pct=5.27 presents=600 idle=0 dropped_vsyncs=0 plans_published=1 plans_superseded=0
goosie: env goos=darwin goarch=arm64 go=go1.26.10 cpu=10 workers=4
goosie: present queued=306 dropped=294 committed=305

### make parity
make-parity exit=0
   96.79%  PASS  js-dom/02-tree-mutation-and-lifecycle.png  worst_region_iou=0.9585814476644171
  coverage_ratio=0.9977858407662968  blank_regions=0/0  rows=[]  shifted=0
  88/88 passing  (100.0%), 0 failing, 0 errors
  average match: 96.15%  (threshold=30, pass_at=90.0%)
```

`load at start` của bước race vẫn còn cao (`9.35`) vì đó là đuôi suy giảm của chính bước
`go test ./...` chạy ngay trước nó; lúc đó không còn process nào ngoài battery.  Trung bình match
**tăng** so với trước khi có fixture (96.14% → 96.15%, §3.3), và `MIN_SCORED_FIXTURES` nâng
87 → 88 trong cùng commit với fixture.

### 7.1 Race: hai lần đỏ trong năm lần, và nguyên nhân là tải — đo trên cả hai phía

Lần đầu tôi định viết "đỏ vì một flake không liên quan".  Câu đó đúng kết luận nhưng **sai về
phương pháp**: nó là suy luận, không phải số đo.  Dưới đây là toàn bộ số đo, kể cả những lần đỏ.

Năm lần `go test -race -count=1 ./...` trên branch (mtime của log):

| Lần | Kết quả `cmd/goosie` |
| --- | --- |
| 07:16:34 `gate-race.log` | `ok 28.518s` |
| 07:46:09 `final-gates.log` | **FAIL 33.180s** — `TestJourneySwitchTabRestoresScroll (8.10s)` |
| 07:52:35 `race-full-2.log` | `ok 27.034s` (full-race exit=0) |
| 08:27:13 `final-race2.log` | **FAIL 40.394s** — cùng một assertion, `presents=475 injected=10` |
| 09:01:36 `final-gates3.log` | `ok` (race exit=0, 42 packages) — cây cuối, máy yên |

Cả hai lần đỏ là đúng một dòng, `cmd/goosie/journey_p1_tabs_test.go:102`, và hai trang của journey
đó **không có script nào** — không có đường nào từ change tới test.  Nhưng "không có đường" vẫn
chỉ là lập luận.  Thí nghiệm trực tiếp (`/tmp/t047/load-race.sh`, branch vs bản export của
`fe4928c` đặt ở `/tmp/t047/base-src`, cùng lệnh, cùng `-race`, xen kẽ branch/base để drift của
máy không nghiêng về một phía):

```
### host: 10 cpus, load { 1.93 3.84 4.78 }

### PHASE A - targeted journey test, IDLE machine (branch 5x, base 5x, interleaved)
A-branch-1 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	2.098s
A-base-1 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	2.067s
A-branch-2 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.810s
A-base-2 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.788s
A-branch-3 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.760s
A-base-3 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.854s
A-branch-4 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.828s
A-base-4 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.851s
A-branch-5 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.875s
A-base-5 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.817s

### PHASE B - targeted journey test under 12 burners
burners alive: 0 (tracked 12)
B-branch-1 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	2.322s
B-base-1 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.854s
B-branch-2 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.808s
B-base-2 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.888s
B-branch-3 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.926s
B-base-3 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	1.810s
B-branch-4 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	4.821s
B-base-4 exit=1 journey_fails=1 pkg=FAIL	github.com/vyquocvu/goosie/cmd/goosie	11.027s
B-branch-5 exit=1 journey_fails=1 pkg=FAIL	github.com/vyquocvu/goosie/cmd/goosie	8.683s
B-base-5 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	2.203s

### PHASE C - whole cmd/goosie package under the same 12 burners (3x each)
C-branch-1 exit=1 journey_fails=0 pkg=FAIL	github.com/vyquocvu/goosie/cmd/goosie	35.009s
C-base-1 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	33.329s
C-branch-2 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	32.176s
C-base-2 exit=1 journey_fails=0 pkg=FAIL	github.com/vyquocvu/goosie/cmd/goosie	33.340s
C-branch-3 exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	32.442s
C-base-3 exit=1 journey_fails=0 pkg=FAIL	github.com/vyquocvu/goosie/cmd/goosie	32.113s

### PHASE D - full -race ./... for both trees at once, burners still running
D-branch-full exit=0 journey_fails=0 pkg=ok  	github.com/vyquocvu/goosie/cmd/goosie	33.775s
D-base exit=0 journey_fails=0

### DONE (burners killed; any left die at alarm 1200)
```

`journey_fails` là số lần `--- FAIL: TestJourneySwitchTabRestoresScroll` xuất hiện trong log của
chạy đó; `exit` là exit code thật của `go test`, lấy trực tiếp.  Hai chỗ phải đọc kèm, vì script
của tôi tự ghi sai nhãn: dòng `burners alive: 0` là **lỗi counter của script** (xem dưới), và nhãn
"PHASE D - full -race ./... for both trees at once" **nói quá** những gì nó chạy — Phase D thật ra
chạy `./cmd/goosie/ ./test/gate` trên base đồng thời với `./cmd/goosie/` trên branch, vẫn là hai
cây cùng chịu một tải, nhưng không phải full suite.

Bốn điều đo được, không điều nào đến từ việc đọc code:

1. **Base cũng đỏ, cùng một dòng.** `B-base-4` fail đúng `journey_p1_tabs_test.go:102`, hết
   11.027 s, `presents=473` — `B-branch-5` fail cùng assertion, hết 8.683 s, `presents=470`.
   Tải đủ lớn thì test đó timeout **trên cả hai phía**, nên đỏ không phải bằng chứng cho change.
2. **Cả họ journey đều nhạy tải, cả trước change.** Khi chạy nguyên package dưới 12 burner, base
   fail 2/3 lần và branch fail 1/3 lần, toàn bộ là
   `TestJourneyZoomInGrowsContentAndDisplaysPercentage` — một test journey khác, không liên quan
   DOM API.  (Branch không hề tệ hơn base ở đây.)
3. **Idle thì cả hai phía xanh.** Phase A: 5/5 branch + 5/5 base, 1.76–2.10 s.  Hai full-suite
   control trên base (`race-base-full.log` 08:34, `race-base-full2.log` 08:40) đều
   `ok cmd/goosie` (34.401 s, 26.264 s); overall exit của chúng là 1 vì ba test
   `test/gatecheck` RCSweep cần một repo git thật mà bản export không có — khai rõ để không ai
   đọc nhầm đó là red flag của change.
4. **Hai lần đỏ của tôi trùng lúc máy đang bị chia.** 07:46 và 08:22 là khi chính tôi chạy song
   song `make parity` (Chromium render 88 fixture) và các build mutant; 09:01 là lần duy nhất
   battery chạy mà không có gì khác — và đó là lần được trích ở bảng trên.

Còn hai điều tôi không giấu: (a) con số `burners alive: 0` trong log Phase B là **lỗi của chính
script đo** (`pgrep -P 1 -U … -f 'alarm 1200'` không bắt được process vì chúng không phải con của
PID 1); tôi xác minh lại bằng `pgrep -f "alarm 1200"` → đúng 12 process và `vm.loadavg` tăng, nên
kết luận Phase B/C vẫn dựa trên tải thật, nhưng ai đọc log thô phải biết dòng đó vô nghĩa; (b)
flake này **tôi không sửa**, vì nó nằm trong `cmd/goosie/journey_p1_tabs_test.go` — vừa ngoài
phạm vi TASK-047, vừa là thứ chỉ sửa đúng khi đo lại budget timeout của cả họ journey, và làm vậy
trong cùng commit DOM API sẽ che mất cả hai thay đổi.  Gate `go test -race -count=1 ./...` trên
cây cuối, máy yên: **exit=0**.

---

## 8. Đề nghị BR id + danh sách oracle (integrator tag `// qc:BR-xxx` rồi `-update`)

`test/qc/traceability.golden.json` hiện cao nhất ở **BR-197** (đo: `max BR in golden: 197`,
catalog cũng `BR-197`). Task này đề nghị từ **BR-198** trở đi, và **không** đụng
`test/qc/catalog.go`, `traceability.golden.json`, hay marker `// qc:` trong test.

- **BR-198** — Node child-list API đúng tên standard: `createDocumentFragment` là Node thật
  (nodeType 11, nhận con), `insertBefore` có cả nhánh append khi reference null/thiếu,
  fragment được **flatten theo thứ tự** và rỗng sau khi cài, `replaceChild` thế chỗ và trả con
  cũ, `cloneNode(false|true)` phân biệt shallow/deep, `contains`/`hasChildNodes`/
  `isConnected`.
  - `test/js/node_tree_test.go`: `TestCreateDocumentFragmentIsANodeThatTakesChildren`,
    `TestAppendChildMovesFragmentChildrenAndEmptiesFragment`,
    `TestInsertBeforeWithNullReferenceAppends`,
    `TestInsertBeforePositionsAndRejectsForeignReference`,
    `TestInsertBeforeMovesFragmentChildrenToPosition`, `TestCloneNodeShallowAndDeep`,
    `TestClonedFormControlKeepsCheckedState`, `TestReplaceChildSwapsInPlace`,
    `TestChildListReadsCoverEveryNodeType`, `TestOwnerDocumentAndParentNodeAreTheDocumentObject`,
    `TestIsConnectedFollowsRemoval`, `TestNodeValueIsOnlyForLeaves`.
- **BR-199** — `window` là EventTarget và document có vòng đời: `DOMContentLoaded` tới window
  sau document, `readyState` đi `loading→interactive→complete`, `load` chỉ trên window, dedup
  listener theo (function, capture), bước bubble kết ở window trừ khi `stopPropagation`,
  `FireDocumentLifecycle` nổ đúng một lần và báo có mutation.
  - `test/js/window_lifecycle_test.go`: 9 test, gồm
    `TestDOMContentLoadedReachesWindowAfterDocument`, `TestReadyStateTracksTheLifecycle`,
    `TestReadyStateGuardRunsHandlerAfterLoad`, `TestLoadEventFiresOnWindowOnly`,
    `TestBubblingEventReachesWindowUnlessPropagationStops`,
    `TestFireDocumentLifecycleReportsDomMutation`.
- **BR-200** — `window.getComputedStyle` trong realm: hai cách gọi tên (kebab + camel),
  `getPropertyValue`, inline thắng cascade, **chỉ** engine values, từ chối argument không phải
  element, và `document.defaultView === window`.
  - `test/js/computed_style_test.go`: 8 test, gồm `TestGetComputedStyleMergesInlineOverCascade`,
    `TestGetComputedStyleOnlyReportsEngineValues`, `TestJQueryCssLookupChainResolves`,
    `TestDefaultViewIsTheWindowItself`, `TestGetComputedStyleRejectsNonElementArguments`.
- **BR-201** — Tầng session: provider đọc `s.Styles[n.ID]` của chính session (không bịa
  initial), node chưa resolve trả rỗng, lifecycle nổ **sau** layout và chỉ re-resolve khi DOM
  thực sự đổi, reveal/hide trong handler phải tới được paint.
  - `test/engine/computed_style_render_test.go`: `TestGetComputedStyleReportsTheSessionsOwnResolve`,
    `TestGetComputedStyleForANodeTheSessionNeverResolved`, `TestLifecycleRevealRepaintsTheDocument`,
    `TestLifecycleHideRemovesFromTheRender`, `TestLifecycleReResolvesOnlyWhenTheHandlerMutated`.
- **BR-202** — `location` parts sống theo URL của session/history, `document.location ===
  window.location`, coerce ra `href`, và **không** có members điều hướng.
  - `test/js/location_parts_test.go`: 9 test, gồm `TestLocationHostnameSupportsARealPagePattern`
    (đúng pattern `indexOf('demo.')` của `validateCachePage`),
    `TestLocationNavigationMembersAreAbsent`, `TestLocationWritesAreIgnoredNotNavigating`.
- **BR-203** — `document.implementation.createHTMLDocument(title)`: skeleton bốn node, title
  là text node, document detached cô lập nhưng identity ổn định, constructor của nó tạo node
  thuộc về **chính nó**, và query index rooted theo document của node gốc.
  - `test/js/document_implementation_test.go`: 8 test, gồm
    `TestJQueryCreateHTMLDocumentProbeIsTheExactExpression`,
    `TestParseHTMLBaseHrefBranchRunsAgainstTheDetachedHead`,
    `TestDetachedDocumentIsIsolatedButIdentityStable`,
    `TestDocumentImplementationOmitsUnimplementedMembers`.
- **BR-204** — Biểu thức boot **nguyên văn** của jquery-3.6.0.min.js phải chạy được: support
  block (`createDocumentFragment().appendChild(createElement("div"))`), buildFragment + unwind,
  `slice.call(document.childNodes)`, và nhánh ready (`DOMContentLoaded` trên document + `load`
  trên window, `readyState` nhìn thấy `loading`).
  - `test/js/jquery_dom_boot_test.go`: `TestJQuerySupportBlockRunsAndSetsItsFlags`,
    `TestJQueryBuildFragmentWrapAndUnwindPath`,
    `TestJQuerySlicePushTrickOnDocumentChildNodes`,
    `TestJQueryReadyBranchPicksTheListenerPathAndBothRegistrationsStick`.
- **BR-205** — Admission của clone: subtree detached không bị charge/depth-check lúc dựng,
  nhưng **lúc cài** phải bị từ chối ở `QuotaExceededError` catchable, document còn lại nguyên
  vẹn; hai gate (đo subtree, và `pd+1+height`) tách riêng được.
  - `test/engine/js_admission_test.go`: `TestEngineJSCloneOfDeepDetachedSubtreeIsCheckedAtInsertion`.
- **BR-206** — PARITY (render): fixture sơn toàn bộ cluster trên pixels, với reference do
  Chromium đo ở runtime.
  - `testdata/render/js-dom/02-tree-mutation-and-lifecycle.html`
    (`MIN_SCORED_FIXTURES` 87→88 cùng commit).

---

## 9. Chạy lại

```bash
# 1. Số đo lỗi + runtime reads (probe dựng từ source, đặt ngoài repo)
/tmp/t047/probe-base  -in _b2.html -js -width 1280 -height 1024
/tmp/t047/probe-after -in _b2.html -js -width 1280 -height 1024
/tmp/t047/probe-after -in _b2.html -js -width 1280 -height 1024 -check "$(cat /tmp/t047/check-anch.txt)"

# 2. Ink bằng chính ink_mask của harness, hai binary
python3 /tmp/t047/ink.py _b2.html saved-www_gov_vn_.html saved-vietnamnet_vn_.html \
  saved-tuoitre_vn_.html saved-nhandan_vn_.html jquery-sync-reveal.html jquery-reveal.html

# 3. Oracle + mutation
go test -count=1 ./test/js/ ./test/engine/

# 4. Parity (Chromium render reference tại runtime)
make parity                                   # 88/88
python3 testdata/parity.py --render --binary /tmp/t047/goosie-base     # binary trước change

# 5. Oracle của riêng fixture (§3.3): render cùng một file bằng lệnh của harness
#    với từng binary, rồi chấm + đếm màu.  Script đếm màu ở ngoài repo.
H="file://$PWD/testdata/render/js-dom/02-tree-mutation-and-lifecycle.html"
for b in ./bin/goosie /tmp/t047/goosie-base /tmp/t047/goosie-mutant /tmp/t047/goosie-muc /tmp/t047/goosie-mud; do
  $b -backend headless -width 800 -height 600 -dpr 1 -screenshot \
     -out /tmp/goosie-renders/js-dom/02-tree-mutation-and-lifecycle.png -url "$H"
  python3 testdata/parity.py | grep js-dom/02
done
python3 /tmp/t047/bandcolors.py /tmp/goosie-renders/js-dom/02-tree-mutation-and-lifecycle.png
python3 /tmp/t047/bandink.py  /tmp/goosie-renders/js-dom/02-tree-mutation-and-lifecycle.png \
                              /tmp/playwright-renders/js-dom/02-tree-mutation-and-lifecycle.png
# goosie-mutant  = insertBefore(x, nil) cắm xuống đầu (internal/dom/node.go)
# goosie-muc     = computedValues bỏ provider (internal/js/computed_style.go)
# goosie-mud     = không dispatch DOMContentLoaded (internal/js/window_events.go)
# Cả ba binary mutant dựng ngoài /tmp/t047, source luôn được restore + gofmt -l sạch.
```

---

## 10. Đo độc lập lần hai — artifact `/tmp/t047-work`, ba commit đã đóng gói

Mục 0–9 là lượt làm việc của **một agent khác trên cùng worktree này**; tôi giữ nguyên, không
sửa một chữ nào. Mục 10–11 là số tôi tự đo, từ đúng ba commit `5b2434f` / `4054bde` / `24ee057`,
bộ artifact riêng (`/tmp/t047-work`, không đụng `/tmp/t047`). Ở đâu trùng với §4 thì hai bên
cùng một con số (vnexpress 6→1 lỗi, `Image is not defined` là lỗi duy nhất còn lại, gov.vn
extent `0x0`). Ở đâu khác, tôi ghi thẳng ở §10.4 và §10.9.

Lý do mục này tồn tại: file doc này bị ghi **hai lần** trong cùng worktree. Bản của tôi nộp
trước, bản 747 dòng của agent kia đè lên sau. Xóa một trong hai là phá số đo của người khác, nên
tôi không ghi đè nữa: §0–§9 giữ nguyên của họ, phần của tôi append ở §10–§11 và tự chịu phạm vi
chứng nhận của mình.

**Mọi `file:line` trong §10–§11 tính theo commit `24ee057`**, đã kiểm lại từng dòng bằng
`git show 24ee057:<file>`. Lý do phải neo: worktree này đang có người sửa song song nên số dòng
trong working tree trôi đi trong lúc tôi viết — `internal/js/computed_style.go` ví dụ, comment
block của `computedStyleObject` dài thêm từ 5 lên 21 dòng và bản thân function decl nhảy từ
`119` xuống `135` giữa hai lần tôi đọc file.

### 10.1 Artifact: commit → export → binary

Hai binary cho mỗi mốc, dựng bằng `git archive` để mọi con số truy về đúng một commit có tên:

```bash
cd .worktrees/task-047
git archive fe4928c | tar -x -C /tmp/t047-work/base     # TRƯỚC change
git archive HEAD  | tar -x -C /tmp/t047-work/verify     # 24ee057
(cd /tmp/t047-work/base   && go build -o ../bin/headless-base ./cmd/goosie-headless)
(cd /tmp/t047-work/verify && go build -o ../bin/headless-head ./cmd/goosie-headless)
```

| binary | nguồn | byte | sha256[:16] |
| --- | --- | --- | --- |
| `bin/headless-base` | `fe4928c` | 20.110.178 | `e43aa685f3136de8` |
| `bin/headless-head` | `24ee057` | 20.311.202 | `cc4e2d4d7be64abb` |
| `bin/probe-before` | `fe4928c` + flags `-js -inv -pump -check -png` | 20.163.730 | `2a0fa6d3d5e8ddfd` |
| `bin/probe-head` | `24ee057` + cùng flags | 20.311.826 | `a25eb679101aef31` |

Ink đếm **hai định nghĩa** trên cùng một PNG, để không ai phải đoán số nào là số nào:

```bash
python3 /tmp/t047-work/ink2.py <png> ...
# nonwhite = count(any channel != 255)   ink245 = count(any channel < 245)   # = parity.ink_mask
```

`ink245` thấp hơn `nonwhite` ~3,7% trên các ảnh của task này (39.260 → 37.818), tức đúng mức
chênh antialias mà harness cắt bỏ. Trang tái hiện và trang lưu:

| artifact | byte | sha1 |
| --- | --- | --- |
| `reproA-inline.html` | 90.908 | `34b87e1f` |
| `reproB-cssrule.html` | 90.912 | `6b88bcbe` |
| `reproC-sync-build.html` | 91.196 | `9b85e6e0` |
| `late-insert.html` | 368 | `517b7d0b` |
| `frag-gcs.html` | 1.156 | `938b35bd` (mục 10.4/10.5/10.6) |
| `sel-desc.html` | 734 | `0bff248b` (mục 10.7) |
| `vnexp-saved-201234.html` | 201.234 | `1cd00f06` |
| `jquery-3.6.0.min.js` | 89.501 | `b82d238d` |

### 10.2 Ba trang tái hiện, và một trang phủ nhận chính kết quả của mình

```bash
cd /tmp/t047-work
./bin/headless-base -in reproC-sync-build.html -out rerun-C-base.png -width 1280 -height 800 -dpr 1
./bin/headless-head -in reproC-sync-build.html -out rerun-C-head.png -width 1280 -height 800 -dpr 1
./bin/headless-base -in reproA-inline.html     -out rerun-A-base.png -width 1280 -height 800 -dpr 1
./bin/headless-head -in reproA-inline.html     -out rerun-A-head.png -width 1280 -height 800 -dpr 1
python3 ink2.py rerun-C-base.png rerun-C-head.png rerun-A-base.png rerun-A-head.png
```

| trang | engine | nonwhite | ink245 | extent | `cmds text/glyphs/border` | errors |
| --- | --- | --- | --- | --- | --- | --- |
| reproC | `headless-base` | **0** | 0 | `0x0` | 0/0/0 | 2 |
| reproC | `headless-head` | **39.260** | 37.818 | `Rect(0,10 1280x483)` | 195/498/13 | **0** |
| reproA | `headless-base` | 0 | 0 | `0x0` | 0/0/0 | 2 |
| reproA | `headless-head` | **0** | 0 | `0x0` | 0/0/0 | **0** |
| reproA | `probe-head -js -inv -pump 3` | **35.943** | 34.641 | `Rect(0,10 1280x452)` | 168/449/12 | 0 |

Xuất xứ từng cột, vì hai binary này không cùng loại API: `cmd/goosie-headless` chỉ nhận
`-in -out -width -height -dpr` (đo bằng `--help`: 5 flag, **không có `-js`**) và in đúng một dòng
`rendered <file> → <png> (1280x800, presents=2)` ra stderr, `rc=0`, **không** báo lỗi uncaught — nên
`nonwhite`/`ink245` là của headless, còn `extent`/`cmds`/`errors` là của `probe-*` chạy trên cùng
hai bản dựng (`fe4928c`, `24ee057`). Script có chạy dưới headless thật: reproC cho 0 px trên binary
cũ (nơi script chết ngay statement DOM đầu tiên) và 39.260 px trên binary mới, với cùng một file.

reproC là lần đầu một ảnh `cmd/goosie-headless` của một trang do jQuery dựng hiện lên: 0 →
39.260 px, extent `0x0` → `1280x483`. reproA **cố ý** là số âm: cùng nội dung, cùng API, cùng
hết lỗi, mà vẫn 0 px, vì reveal của nó đi qua `jQuery.ready` và `cmd/goosie-headless` không pump
timer (GOOSIE-LIVE-001 nhánh D). Chạy đúng trang đó dưới harness có pump thì ra 35.943 px. Cặp
này tách hai nguyên nhân mà row `jquery-reveal.html` +0 ở §4.2 không tách được một mình: **DOM
đã thông, pump thì mới sơn** — và cả hai số trên cùng một file, không phải hai trang khác nhau.

### 10.3 Văn bản lỗi ném ra, nguyên văn, và một con số offset tôi từng nói sai

`fe4928c`, `probe-before -in reproC-sync-build.html -js`:

```
script_errors_total=2
err[0] inline:1: TypeError: Object has no member 'createDocumentFragment' at inline:2:37814(645) at inline:2:221(29) at inline:2:225(11)
err[1] inline:1: ReferenceError: jQuery is not defined at inline:5:15(5)
js_live_nodes=26  extent=Rect(0,0 0x0)  cmds text=0 glyphs=0 fill=0 border=0 image=0
```

reproA giống hệt, chỉ khác frame cuối (`inline:2:7(2)`). vnexpress `_b2.html` (bản copy
201.234 B) trước change, 6 lỗi nguyên văn trong `vn-before.log`: 2×
`Object has no member 'insertBefore'`, 3× `Object has no member 'addEventListener'`, 1×
`TypeError: Cannot read property 'indexOf' of undefined or null at validateCachePage
(inline:6:53(4))`; sau change (`vn-after.log`) còn đúng 1×
`ReferenceError: Image is not defined at validateCachePage (inline:6:832(100))`, `js_live_nodes`
2043 → 2049, và **display list y hệt** (`cmds text=0 glyphs=0 fill=28 border=2`) — tức 6→1 là
thắng ở phần boot, chưa phải ở phần sơn.

Sửa lại một câu tôi từng viết: `inline:2:37814` là **cách goja đánh số frame**
(dòng:cột(offset trong file được chạy)), không phải byte offset của statement trong bundle. Byte
offset thật, đo bằng Python trên đúng file 89.501 B: `createDocumentFragment().appendChild` =
**37.880**; `createHTMLDocument` có **4** lần — 84.477 và 84.518 (probe support
`y.createHTMLDocument=((_t=E.implementation.createHTMLDocument("").body).innerHTML="<form></form><form></form>",2===_t.childNodes.length)`)
và 84.707, 84.750 (nhánh không context của `S.parseHTML`, chỗ nó dựng document tạm rồi append
`<base>` với `href=E.location.href`). Hai offset mà §10.4 cần: helper kiểm tra "đang ẩn" bắt đầu ở
**36.479**, và đoạn đo display mặc định (`o=a.body.appendChild(a.createElement(s)),u=S.css(o,"display"),
o.parentNode.removeChild(o),"none"===u&&(u="block"),ue[s]=u`) nằm ở **37.307–37.421**.

### 10.4 `$("#form1").show()`: handler chạy, cây đổi, mà vẫn không sơn — chuỗi nguyên nhân đo được

reproB là trang mà panel bị ẩn bằng **rule stylesheet** (`#form1{display:none}`), reveal nằm
trong `jQuery(function($){ $("#form1").show(); $("h1").css("color","#b00000");
$("p").text(...) })`. Đo trên `probe-head -js -inv -pump 3` (tức **đã** pump 3 lần):

| read | kết quả |
| --- | --- |
| `document.readyState` | `complete` |
| `jQuery.isReady` | `true` |
| `document.querySelector("p").textContent` | `Revealed by jQuery(f…` — handler đã chạy và đã đổi cây |
| `jQuery("h1").css("color")` | `#b00000` |
| `document.getElementById("form1").style.cssText` | `""` — **không có inline style nào được ghi** |
| `getComputedStyle(#form1).display` | `none` |
| extent / `cmds text` | `Rect(0,0 0x0)` / `0` |

Chuỗi nguyên nhân, mỗi mắt xích một số đo, không có mắt xích nào suy luận:

1. Bundle jQuery, offset **36.479** (`var ae=function(e,t){return"none"===…`): điều kiện ẩn
   `"none"===(e=t||e).style.display || ""===e.style.display && <in-doc> && "none"===S.css(e,"display")`
   → `#form1` inline rỗng, ở trong document, và `jQuery.css(#form1,"display")` **đo ra `none`** ⇒
   nhánh "phải khôi phục display" được chọn, đúng như thư viện mong đợi.
2. `showHide` ở offset **37.044**; giá trị mặc định lấy bằng đoạn **37.307–37.421**:
   `o = a.body.appendChild(a.createElement(s)); u = S.css(o,"display"); o.parentNode.removeChild(o);
   "none"===u && (u="block")` — tức **append một node mới rồi đo computed style của nó, ngay
   trong cùng một task**. Bundle này không có chữ `iframe` nào (đếm = 0), nên không phải đường
   iframe.
3. Đo `u`: `jQuery(document.createElement("form")).appendTo(document.body).css("display")` →
   **`undefined`**; `div` và `p` cũng vậy. Lý do ở engine:
   `internal/engine/computed_style.go:26` trả `nil` khi node chưa có trong `s.Styles[n.ID]`, và
   `internal/js/computed_style.go:119`/`:121` (`computedStyleObject`, vòng `Set` chạy trên
   chính map đó) chỉ `Set` những tên có giá trị ⇒ node vừa được cài **không
   có box đã resolve trong chính task đó**.
4. `u === undefined` → phép gán `elemdisplay[nodeName] = undefined` → `values[i]` giữ
   `undefined` → guard `values[i] != null` của jQuery **bỏ qua nốt lệnh ghi style**. Đó là lý do đo
   được `style.cssText === ""`: jQuery không ghi `""`, nó **không ghi gì cả**.
5. node đó vẫn nguyên vẹn ở cuối hàng đợi style. Đọc lại **sau** khi engine resolve
   (`late-insert.html`, cùng probe, sau `-pump 3`) thì
   `getComputedStyle(lateNode).display === "block"` và
   `getPropertyValue("display") === "block"`, trong khi đọc **trong** script trả
   `undefined` / `""`.

Nói cách khác: defect nằm ở **độ hạt resolve trong một task**, không ở CSSOM binding. Chỗ sửa là
provider được inject (`internal/js/computed_style.go:52`), vì archtest đóng bảng import —
`"internal/js": {"internal/dom", "internal/media"}` (`internal/archtest/rules.go:56`) — nên
`internal/js` không được import `internal/style`/`internal/engine`; provider phải tự
resolve-on-demand thay vì trả `nil`.

**Bổ sung cho §5.5/§6**: mục đó quy `jquery-reveal.html` +0 cho "headless không pump timer". Đúng,
nhưng **chưa đủ**: reproB chạy với `-pump 3` (handler được chạy, `jQuery.isReady=true`) mà vẫn +0
vì mắt xích 1–4 ở trên. Hai defect độc lập; sửa pump một mình không làm reveal sơn được.

### 10.5 Hai chữ ký CSSOM mà binding này chưa khớp, đo được bằng chính object trả về

| read trên node **chưa resolve** | read trên node **đã resolve** |
| --- | --- |
| `typeof s.display === "undefined"`, `hasOwnProperty("display") === false` | `typeof s.display === "string"`, `hasOwnProperty === true` |
| `s.getPropertyValue("display") === ""` | `s.getPropertyValue("display") === "none"` |

CSSOM đặt `display` (và mọi tên trong mixin `CSS2Properties`) là **attribute IDL trả chuỗi**;
`getPropertyValue` mới là thứ trả `""`. Ở đây tên property biến mất hẳn trên đường accessor, nên
một trang viết `getComputedStyle(el).display === ""` sẽ ra `false` còn Chromium ra `true`. jQuery
**không** dính: chuỗi tra của nó là `n.getPropertyValue(t) || n[t]` (đo: cả hai dạng đều falsy nên
cùng một đáp án). Trang dùng `in`/`hasOwnProperty` để feature-detect thì dính.

Cũng đo: object không có `length`, `item()`, `parentRule` (cả ba `undefined`), nên
`Array.prototype.slice.call(getComputedStyle(el))` cho `[]`.

Hai điểm trung thực phải nói thêm: (a) comment ở `internal/js/computed_style.go:116-118` và header
của test `TestGetComputedStyleOnlyReportsEngineValues` (`test/js/computed_style_test.go:153`) viết
"property không mô hình đọc ra chuỗi rỗng" — **đúng cho `getPropertyValue`, sai cho accessor**;
(b) chính hai test đó không phân biệt được hai hành vi, vì `[...].join("|")` render `undefined`
thành `""` — nên `cs.zIndex` trong expectation `"|||"` (`:167`) pass dù accessor trả `undefined`.
Test không sai, nhưng nó **không neo** hành vi này; ai sửa CSSOM-shape sẽ không bị hai test đó
chặn. Tôi để nguyên, không nới và không viết thêm assertion giả.

### 10.6 `className` không phản chiếu xuống attribute — đo tới mức cụ thể nhất

```bash
./bin/probe-head -in frag-gcs.html -js -inv -pump 3 -check '<expr>'
```

| expr | kết quả |
| --- | --- |
| `d.className="row2"` sau `d.setAttribute("class","row")` → `getAttribute("class")` \| `className` | `row\|row2` |
| fragment → `d.className="row"` → cài vào `#app` → `d.getAttribute("class")` | `null` |
| số lần trong bundle jQuery 3.6.0 | `className` 7, `innerHTML` 10, `textContent` 9 |

Hàng thứ hai là thứ người dùng nhìn thấy: nội dung do script dựng (fragment + `className`) **không
bao giờ khớp rule class**, nên `.row { padding; border-bottom }` không áp dụng — mất cả padding lẫn
vạch kẻ, dù node tồn tại và sơn text. `innerHTML`/`textContent` là setter ghi được (đo
`innerHTML` là `string`), nhưng cùng họ phản chiếu với `className`.

§6 đã gọi tên defect này ("`className` → thuộc tính `class`", chỉ ra `js.go:1248` và refresh ở
`js.go:1397`). Số ở trên là phần còn thiếu của hồ sơ: một check tái hiện tối giản
(`frag-gcs.html`, 37 dòng) và con số demand thật trong bundle.

Cùng phép đo đó cho ra một defect nữa, **không** có tên ở §6: `Element.outerHTML` không tồn tại
(`typeof` → `undefined`) **và phép gán bị nuốt im lặng**:

```
el.outerHTML = "<p>swapped</p>"  →  own property mới trên wrapper, el.tagName vẫn "DIV",
                                     #app vẫn 3 child, document.querySelectorAll("p").length = 0
```
`fe4928c` và `24ee057` cho **cùng một hành vi**. Trang vnexpress dùng đúng pattern này:
`swapZone(selector, output)` (byte offset 9.642 trong bản copy) poll
`document.querySelector(selector)` rồi gán `.outerHTML = output` để đổi chỗ quảng cáo — tức một
write-path mà engine nhận rồi bỏ qua, còn trang thì tin là đã đổi. Đây là họ lỗi đáng lo nhất trong
task này: **member thiếu thì loud ở đọc và ở gọi, nhưng silent ở ghi.**

### 10.7 Selector engine: mọi combinator và mọi danh sách selector trả 0 — trước và sau y hệt nhau

`sel-desc.html`, đo trên **cả hai** binary (`-js -inv -pump 1`):

| `document.querySelectorAll(...)` | `probe-before` (`fe4928c`) | `probe-head` (`24ee057`) |
| --- | --- | --- |
| `"p"` | 3 | 3 |
| `"h1"` | 1 | 1 |
| `"#app h1"` (descendant) | **0** | **0** |
| `".box>p"` (child) | **0** | **0** |
| `"p+p"` (adjacent) | **0** | **0** |
| `"h1,p"` (danh sách, **không** có combinator) | **0** | **0** |
| `document.querySelector("#app h1") === null` | true | true |

Ba dòng cuối là hai defect khác nhau: engine không hiểu combinator, **và** danh sách selector
(dấu phẩy) cũng trả rỗng. Nguồn: `internal/js/js.go:2270` → `querySelectorAllIndexed` (`:2283`,
fast path chỉ nhận `.class` và tag trụi) → fallback `collectMatching` (`:2313`) → `matchSelector`
(`:2323`), mà doc comment của chính nó chép: *"Supported forms are tag name (`p`), ID (`#main`),
and class (`.active`)"*.

Hệ quả đo được, không phải suy đoán:

- reproC: `jQuery("#app h1").css("color", jQuery("#app h1").length ? "#b00000" : "#000")` chọn
  nhánh `#000`, nên một phần trong 39.260 px của ảnh AFTER là một **câu trả lời sai nhưng nhìn
  được**. Sizzle không che được defect này: bundle rơi xuống `f.querySelectorAll(c)` (đường
  `d.qsa`), tức cùng engine, cùng 0.
- vnexpress: `swapZone` poll `document.querySelector(selector)` cho ô quảng cáo — selector phức
  tạp thì không bao giờ khớp, `setInterval` chạy tới hết 5.000 ms rồi `clearInterval`.
- ngay trong repo: `test/wpt/wpt_report.go:256` lọc bảng bằng
  `document.querySelectorAll('#results tbody tr')` — báo cáo HTML tự filter bằng JS, và filter đó
  im lặng không làm gì.
- `document.title` của `sel-desc.html` ghi lại chính các con số lúc chạy trong body script:
  `inscript desc=0 child=0 adj=0 byTag=1`.

### 10.8 Thứ tự con của `document` và `Document.doctype`

Đo trên `24ee057`, `probe-head -js`:

```
[document.firstChild.nodeType, document.firstChild.nodeName,
 document.lastChild.nodeType,  document.lastChild.nodeName].join(",")  → "1,HTML,10,#doctype"
Array.prototype.map.call(document.childNodes, n => n.nodeType+":"+n.nodeName).join("|")
                                                                        → "1:HTML|10:#doctype"
!!document.doctype                                                      → false
```

Document detached từ `createHTMLDocument` có **cùng thứ tự** (`1,HTML,10,#doctype`). Nguyên nhân ở
parser, không ở binding mới: `internal/dom/parser.go:329` cài `<html>` ngầm định (`insertElement`
cho mode "before html") **trước** khi token `<!DOCTYPE>` được xử lý, và doctype được
`AppendChild` ở `internal/dom/parser.go:942-944` — sau. Theo HTML Standard, mode
"initial insertion mode" tạo và append node `DocumentType` **trước** khi "before html" tạo `<html>`
và append nó, nên ở parser tuân thủ thì doctype là con đầu; DOM Standard cũng định nghĩa getter
`Document.doctype` trả node `DocumentType` hoặc `null`. **Tôi không đo Chromium trên máy này**
(không đụng vào `node_modules` của bản checkout chính), nên câu chuẩn ở đây dẫn theo tên thuật
toán, còn câu đo được ở Goosie thì đã đo thật.

Hai điều phải khai, vì đó là chữ của tôi:

1. Sub-case `"html first, doctype present"` nằm trong test tên là
   `TestCreateHTMLDocumentBuildsTheStandardSkeleton`
   (`test/js/document_implementation_test.go:54`) — **tên test nói "standard" nhưng nó neo tính
   nhất quán của parser**, không neo thứ tự standard. Assertion bổ sung ở `:76`
   (`"1,10,#doctype"` cho document live) càng làm rõ ý đó. Tôi không nới assertion: thu nó lại sẽ
   ẩn đúng lệch mà người sau phải sửa. Nó được đăng ký thành defect ở §11 thay vì thành test.
2. `TestDocumentImplementationOmitsUnimplementedMembers` (`:276`) neo `document.title` của document
   live là `""` trên trang có `<title>Main</title>`, vì đường host-seed
   (`js.Options.Title` cho browser chrome) thắng. DOM Standard buộc `title` phản chiếu text của
   element `title`. Đây cũng là lệch có chủ đích, comment ở `:270-275` đã nói lý do; tôi ghi lại ở
   §11 để nó không bị đọc nhầm là "đã đúng chuẩn".

### 10.9 Mutation proof — một cơ chế mỗi lần, 11 lần

Mỗi lần tắt **một** cơ chế, chạy `go test -count=1 ./test/js/ ./test/engine/ ./internal/archtest/`,
đếm ba thứ rồi restore. Script và log: `/tmp/t047-work/mutate.py`,
`/tmp/t047-work/mutation-table.txt`, `mutations/*.log`.

| # | cơ chế bị tắt | rc | `--- FAIL` | dòng assertion |
| --- | --- | --- | --- | --- |
| M01 | `createHTMLDocument` không được cài | 1 | 8 | 8 |
| M02 | `document.createDocumentFragment` không được cài | 1 | 6 | 6 |
| M03 | `insertBefore`/`replaceChild`/`cloneNode`/`contains`/`hasChildNodes` không được cài | 1 | 9 | 9 |
| M04 | accessor cây node (`childNodes`, sibling, `ownerDocument`, `nodeValue`, `isConnected`) | 1 | 23 | 23 |
| M05 | window EventTarget + `document.readyState` | 1 | 11 | 12 |
| M06 | engine không nổ lifecycle | 1 | 3 | 5 |
| M07 | `location` không được dựng lại với từng phần | 1 | 12 | 12 |
| M08 | `getComputedStyle`/`defaultView` không được cài | 1 | 3 | 4 |
| M09 | engine không trao cascade reader cho runtime | 1 | 2 | **17** |
| M10 | query index bỏ qua document gốc của node | 1 | 2 | 2 |
| M11 | `appendChild` quay về `AppendChild` (fragment thành 1 node) | 1 | 2 | 2 |
| — | restore, chạy lại cả ba suite | **0** | **0** | **0** |

Ba hàng lệch nhau đáng chú ý: M09 giết **2** test nhưng in **17** dòng lỗi, vì một test trong số đó
(`TestLifecycleRevealRepaintsTheDocument`) so cả một bảng giá trị; M06 (3/5) và M08 (3/4) lệch vì
một test in nhiều `t.Errorf`. M05 (11 FAIL) đánh vào **8/9** test của `window_lifecycle_test.go`;
test thứ 9, `TestFireDocumentLifecycleReportsDomMutation`, **sống sót** và đó là tin tốt: handler
của nó đăng trên `document` (con đường đã có từ `fe4928c`) và nó không đọc `readyState`, nên M05
đúng là chỉ tắt phần window chứ không tắt cả máy dispatch. Ngoài 8 test đó, M05 còn giết
`TestDefaultViewIsTheWindowItself`,
`TestJQueryReadyBranchPicksTheListenerPathAndBothRegistrationsStick` và test engine
`TestLifecycleRevealRepaintsTheDocument`: đúng cái liên kết mà §2.7 nói "hai cơ chế phải đi cùng
nhau".

Không có survivor nào trong 11 lần: mọi cơ chế đều có ít nhất một test chết vì **đúng** cơ chế đó.
Không sửa test, không xóa assertion, không nới ngưỡng nào trong suốt quá trình này.

**Ghi lại một hazard tôi gặp thật, người sau sẽ gặp nếu dùng cách này**: script snapshot hash của
từng file trước khi mutate, và 5 lần nó phát hiện `internal/js/js.go` **bản khác** (agent kia đang
sửa giữa chừng) nên restore về bản HEAD. Hôm nay `internal/js/js.go` trong worktree khớp HEAD từng
byte (`sha256:5c9de450ccf439c1`), còn `/tmp/t047-work/drifted` — nơi các bản lạ được lưu — **đã bị
xóa** nên nội dung đó không lấy lại được từ `/tmp`. Kết luận vận hành: vòng mutate/restore và một
agent đang sửa không được dùng chung một worktree.

### 10.10 Gates, chạy lại trên HEAD thuần

`24ee057` được `git archive` ra `/tmp/t047-work/head-check` rồi chạy ở đó (tức không lẫn WIP chưa
commit của agent khác), cùng lúc `go test -count=1 ./...` trong worktree:

| gate | HEAD thuần | worktree (HEAD + WIP của agent khác) |
| --- | --- | --- |
| `gofmt -l .` | rỗng (0 dòng) | rỗng (`gofmt -l` trên 5 file họ đang sửa: 0 dòng) |
| `go vet -all ./...` | rc 0, 1.469 s wall | rc 0 |
| `go test -count=1 ./test/js/ ./test/engine/ ./internal/archtest/` | rc 0, 6.558 s wall — `js 3.248s`, `engine 5.254s`, `archtest 2.342s` | rc 0 — `js 3.566s`, `engine 6.174s`, `archtest 5.649s`; cùng ba package đó khi chạy chung full suite (song song) thì là `7.118s`/`9.076s` |
| `go test -count=1 ./...` | 55.265 s wall, **3 FAIL ở `test/gatecheck`** (giải thích dưới), còn lại ok | rc 0, **62.27 s wall**, 42 dòng `ok`, 0 FAIL (`worktree-full-test-2.log`) |

Ba FAIL đó là **artifact của cách export, không phải defect**: thư mục export không có `.git` nên
`git rev-parse HEAD` exit 128 và RC-sweep tự xử nó thành NO VERDICT —

```
--- FAIL: TestRCSweepChecksSuppliedBuildSHAAgainstHEAD (0.31s)
    rc_sweep_test.go:287: SHA mismatch: exit=2 output="rc-sweep: fingerprint: NO VERDICT - cannot
    read build SHA: Command '['git', 'rev-parse', 'HEAD']' returned non-zero exit status 128.
    rc-sweep: gate: PASS - custom step passed … rc-sweep: NO VERDICT (report: …/rc-sweep.json)"
--- FAIL: TestRCSweepReportContainsSevenEvidenceRowsAndFingerprint (0.28s)
    rc_sweep_test.go:119: incomplete fingerprint: {SHA: OS:macOS-26.6.2-arm64-arm-64bit BinarySHA:}
--- FAIL: TestRCSweepRequiresHumanSignoffForExactBuildSHA (0.40s)
    rc_sweep_test.go:337: exit status 128
FAIL	github.com/vyquocvu/goosie/test/gatecheck	30.915s
```

Trong worktree thật (có `.git`), `go test -count=1 ./...` là `rc=0`, `real 56.11`, 70 dòng package
(`ok test/entrypoints 49.121s`, `ok test/gate 36.221s`, `ok test/gatecheck 33.332s`,
`ok test/wpt 13.293s`) — log `/tmp/t047-work/worktree-full-test.log`. **Phạm vi chứng nhận:** con
số đó đo trên cây có WIP chưa commit của agent khác (`internal/js/node_api.go`,
`test/engine/js_admission_test.go`, `test/js/node_tree_test.go`, `testdata/parity.py`,
`testdata/render/js-dom/02-…`); tôi chỉ chứng nhận ba commit của mình.

`make gate`, `make parity`, `make live-parity`: **tôi không chạy**, theo ràng buộc của task — mọi
thứ liên quan Chromium trong doc này do agent kia đo và agent kia nộp.

### 10.11 Unresolved / limits, nói thẳng

1. **Trong một task, node vừa cài không có computed style.** Đây là thứ chặn `.show()` (§10.4), sửa
   được ở provider `internal/engine/computed_style.go:26` + `internal/js/computed_style.go:52`,
   không sửa được trong `internal/js` vì bảng import.
2. **`cmd/goosie-headless` không pump timer** (GOOSIE-LIVE-001 nhánh D): reproA 0 px dù 0 lỗi và
   DOM đã đổi; chỉ harness probe của tôi (`-js -inv -pump 3`) sơn tới 35.943 px. Số của §10.2 không
   phải số của sản phẩm.
3. **`PumpJS` chỉ re-resolve khi có invalidation**: `engine.go:2254-2293` chỉ resolve lại khi
   `s.focusStylePending || (s.inv != nil && !s.inv.Empty())`. Harness không gọi
   `engine.WithInvalidation(...)` thì pump có chạy mà style/layout không theo. Đây là lý do probe
   của tôi phải có cờ `-inv`.
4. **Selector engine không hiểu combinator và danh sách selector** (§10.7) — pre-existing, trước
   sau như nhau, và jQuery thừa hưởng nguyên.
5. **`className` không phản chiếu `class`, `outerHTML` vừa thiếu vừa nuốt phép gán** (§10.6) —
   content do script dựng ra mất style, và ad-swap của vnexpress im lặng hỏng.
6. **`Document.doctype` không tồn tại, thứ tự con lệch so với standard** (§10.8) — test của tôi neo
   hành vi này thành *nhất quán parser*, không thành *đúng chuẩn*.
7. **`CSSStyleDeclaration` thiếu `length`/`item`/`parentRule`, và accessor trả `undefined` cho tên
   chưa resolve** thay vì `""` (§10.5).
8. **`location.assign/replace/reload` không tồn tại**: đo nguyên văn
   `TypeError: Object has no member 'reload'`, `…'assign'`. Trang vnexpress gọi `location.reload()`
   đúng **1** lần, trong `validateCachePage`, ngay sau `new Image(1,1).src=…` — mà `Image` thì throw
   trước, nên hôm nay nó chưa tới được. Khi binding `Image` ra đời, đây là lỗi kế tiếp, và nó **nên**
   là lỗi loud: engine không có navigation.
9. **`iframe.contentWindow`/`contentDocument` không tồn tại** (đo `typeof` → `undefined`); jQuery
   dùng `contentDocument` 3 lần (bundle offset 27.600, `S.fn.contents`). May là `defaultDisplay`
   của đúng bundle này **không** dùng iframe (đếm chuỗi `iframe` = 0), nên reproB không hỏng vì
   mắt xích đó — nói rõ để không ai sửa nhầm chỗ.
10. **Ink của vnexpress vẫn 0** với display list y hệt trước/sau (`text=0`, `fill=28` màu
    `0xF7F7F7` — min channel 247, `ink245` cutoff 245 nên không đếm). §4.4 đã đo; tôi xác nhận độc
    lập cùng con số.
11. **TÔI không chạy parity/Chromium**; các số Chromium trong file này là của §3.3/§4 (agent kia).
12. `testdata/render/js-dom/02-tree-mutation-and-lifecycle.html` và `MIN_SCORED_FIXTURES` 87→88 vẫn
    **chưa commit** ở thời điểm tôi viết; §3.3 của họ còn một placeholder số đo
    ("Số đo sau khi sửa: (điền ở mục 3.4)"). Không phải phần của tôi, nhưng integrator cần biết.

### 10.12 Ràng buộc mà lượt của tôi đã tuân thủ, khai để kiểm tra lại được

- **Không sửa, không stage** file của integrator/harness: `docs/qa-test-catalog.md`,
  `docs/qa-open-defects.json`, `docs/qa-ratchet.json`, `docs/qa-signoffs/*`, `test/qc/*`,
  `testdata/render-known-gaps.json`, `testdata/parity.py`, `testdata/live-parity.py`.
  `testdata/parity.py` hiện **có** modification trong worktree — của agent kia (`MIN_SCORED_FIXTURES`
  87→88); tôi không stage, không revert, không đụng tới nó.
- **Không** thêm marker `// qc:` vào bất kỳ test nào.
- **Không** chạy `make parity`, **không** chạy `make live-parity`/`testdata/live-parity.py`. §10
  **không có số Chromium nào**, vì tôi không chạy Chromium trên máy này; mọi con số Chromium trong
  file này (§3.3, §4) là của lượt kia.
- Scratch chỉ `/tmp/t047-work/`; không ghi gì vào bản checkout chính, không xoá worktree khác,
  không xoá thư mục `/tmp` của ai.
- Không push, không amend, không merge, không rebase; ba commit `5b2434f`/`4054bde`/`24ee057` là
  commit mới, cộng thêm commit doc này (`git add` từng đường file, không `git add -A`).
- Bảng import của archtest giữ nguyên: `internal/js` vẫn không import `internal/style`/
  `internal/engine` (`internal/archtest/rules.go:40-64`), nên computed style ở lại dạng provider
  được inject.
- `cmd/probe-t047/` **không** còn trong worktree (agent kia đã xóa); probe của tôi là binary dựng
  ngoài repo (`/tmp/t047-work/probe-final.go` → `bin/probe-before`, `bin/probe-head`), nên không có
  code thí nghiệm nào lọt vào commit.

---

## 11. §proposals (bổ sung) — BR id và defect nên đăng ký thay vì test

**Vì sao không đánh số từ BR-200 như brief yêu cầu:** đo trước khi đề nghị —
`test/qc/traceability.golden.json` có đúng 197 id, `max = BR-197` (đếm bằng Python trên file;
`docs/qa-test-catalog.md` cũng dừng ở BR-197). §8 của lượt trước đã phân bổ **BR-198 → BR-206**
cho đúng cluster oracle này. Nếu tôi mint lại từ BR-200 thì cùng một test sẽ có hai id và người tag
không biết chọn id nào, nên phần này **không** mint id mới cho oracle đã có.

### 11.1 Neo BR-198 → BR-206: xác nhận độc lập

Đếm bằng `grep -c '^func Test'` trên từng file, commit `24ee057`:

| BR | file oracle | số test | trạng thái xác minh |
| --- | --- | --- | --- |
| BR-198 | `test/js/node_tree_test.go` | 12 | đủ 12, khớp 12 tên §8 liệt kê |
| BR-199 | `test/js/window_lifecycle_test.go` | 9 | §8 nêu tên 6/9, cả 6 đều tồn tại |
| BR-200 | `test/js/computed_style_test.go` | 8 | §8 nêu tên 5/8, cả 5 tồn tại |
| BR-201 | `test/engine/computed_style_render_test.go` | 5 | §8 nêu tên 5/5 |
| BR-202 | `test/js/location_parts_test.go` | 9 | §8 nêu tên 3/9, cả 3 tồn tại |
| BR-203 | `test/js/document_implementation_test.go` | 8 | §8 nêu tên 4/8, cả 4 tồn tại |
| BR-204 | `test/js/jquery_dom_boot_test.go` | 4 | đủ 4 |
| BR-205 | `test/engine/js_admission_test.go` | (WIP, chưa commit) | chưa phải của `24ee057` |
| BR-206 | `testdata/render/js-dom/02-tree-mutation-and-lifecycle.html` | (WIP, chưa commit) | parity, thuộc lượt kia |

Tổng oracle của ba commit tôi chứng nhận: **55** (12+9+9+8+8+4+5). Kiểm tra "tên ma": lấy mọi
`Test[A-Za-z0-9_]+` được nêu trong doc rồi đối chiếu `func Test…(` trong toàn repo — **42/42 khớp,
0 ma**. 13 test chưa được nêu tên ở §8, và cả 13 đều đã nằm gọn trong các file mà BR-198…BR-204 chỉ
đích: `TestGetComputedStyleAnswersKebabAndCamelNames`,
`TestGetComputedStylePropertyValueAcceptsCamelCase`,
`TestGetComputedStyleWithoutProviderFallsBackToInline`, `TestWindowListenerDedupAndRemoval`,
`TestWindowRemoveEventListenerBeforeDispatch`, `TestLocationPartsOfAFullURL`,
`TestLocationPartsOfAnOriginOnlyURL`, `TestLocationPartsFollowHistory`,
`TestLocationOfAnUnparseableURL`, `TestDetachedDocumentTitleIsTextAndIsWritable`,
`TestCreateHTMLDocumentBuildsTheStandardSkeleton`,
`TestCreateElementOnTheLiveDocumentStillUsesTheLiveTree`,
`TestGetComputedStyleForANodeTheSessionNeverResolved` (tên đầy đủ kèm `file:line` ở §10 cho ba cái
cuối là `document_implementation_test.go:54`, `:294` và
`computed_style_render_test.go:113`). Integrator tag theo file thì 55 test tự đủ, không cần thêm id.

### 11.2 Defect nên **đăng ký** thay vì viết test (tên đề nghị đã đối chiếu 42 id hiện có, không trùng)

| id đề nghị | một dòng, kèm số đo |
| --- | --- |
| `GOOSIE-SELQUERY-001` | `querySelectorAll`/`querySelector` trả **0** cho mọi combinator (`#app h1`, `.box>p`, `p+p`) **và** cho danh sách selector có dấu phẩy (`h1,p`), trong khi selector đơn giản đúng (`p`→3, `h1`→1). Đo `probe-before` = `probe-head` ⇒ pre-existing, không do TASK-047. Nguồn: `internal/js/js.go:2283`/`:2313`/`:2323`. Viết test lúc này là **neo** câu trả lời sai, nên đăng ký. |
| `GOOSIE-CSSOMTASK-001` | `getComputedStyle` trên node vừa được cài **trong cùng task** không có giá trị: accessor `undefined`, `getPropertyValue` `""`; cùng node đó đọc sau một lần re-resolve thì `block`. Chuỗi nhân quả đã đo từng mắt xích ở §10.4 và là lý do thật khiến `$("#form1").show()` không sơn. Kèm luôn phần shape: thiếu `length`/`item`/`parentRule` (§10.5). |
| `GOOSIE-IDLREFLECT-001` | Attribute IDL không phản chiếu xuống content attribute: `el.className="row2"` sau `setAttribute("class","row")` → `getAttribute("class")` vẫn `"row"`; node dựng bằng fragment + `className` có `getAttribute("class") === null` nên rule class không áp dụng. §6 đã gọi tên, hồ sơ đo ở §10.6. (Họ của `GOOSIE-FORMVALUE-001/002`.) |
| `GOOSIE-OUTERHTML-001` | `Element.outerHTML` không tồn tại **và** phép gán bị nuốt im lặng: `el.outerHTML="<p>swapped</p>"` tạo own property trên wrapper, cây không đổi (`tag=DIV`, `#app` 3 child, `querySelectorAll("p")` 0). Trang thật dùng để đổi ô quảng cáo (vnexpress byte 9.642, `swapZone`). Rule chung rút ra từ phép đo này: member thiếu thì loud ở đọc và ở gọi, nhưng **silent ở ghi**. |
| `GOOSIE-DOCTYPE-001` | `Document.doctype` không được cài (`!!document.doctype === false`) và doctype là con **cuối** (`1:HTML|10:#doctype`) do `internal/dom/parser.go:329` cài `<html>` trước khi `:942-944` append doctype — lệch so với "initial insertion mode" → "before html". Hai test ở §10.8 đang neo hành vi này như nhất quán parser; đăng ký để không đọc nhầm thành "đúng chuẩn". |
| `GOOSIE-IFRAME-001` | Không có `contentWindow`/`contentDocument` trên `<iframe>` (đo `typeof` → `undefined`); jQuery chạm `contentDocument` 3 lần (bundle 27.600, `S.fn.contents`). Không chặn reproB (bundle này không có chuỗi `iframe`), nhưng chặn mọi measurement-by-iframe và `contents()` trên iframe. |
| `GOOSIE-IMAGE-001` *(nếu integrator muốn một id riêng; §6 đã nêu tên cùng defect)* | `Image` constructor chưa có; là lỗi **duy nhất** còn lại trên vnexpress (`ReferenceError: Image is not defined at validateCachePage (inline:6:832(100))`) và chặn luôn `location.reload()` phía sau nó (§10.11 mục 8). |
| *(đã có, chỉ bổ sung số đo)* `GOOSIE-LIVE-001` nhánh D | headless không pump timer: reproA 0 px / 0 lỗi, cùng trang đó dưới harness có pump 35.943 px. Con số này nên vào hồ sơ defect vì nó chứng minh phần DOM **đã** thông và phần còn lại đúng là timer. |
| *(đã có, chỉ bổ sung số đo)* `GOOSIE-CSSSELECT-001` | Không liên quan trực tiếp tới measurement mới ở đây; ghi để phân loại: defect combinator ở `GOOSIE-SELQUERY-001` là **query API**, khác với defect matching selector chứa nháy escape. |

### 11.3 Ba phát hiện tôi nộp lại bằng lời, không nộp bằng test

1. §10.5: comment/expectation "đọc ra chuỗi rỗng" chỉ đúng cho `getPropertyValue`; hai test
   `test/js/computed_style_test.go:153` và `:178` **không phân biệt** `undefined` với `""` vì
   `[...].join("|")`. Đây là khoảng trống oracle thật, nhưng lấp nó bây giờ là chốt cứng hành vi
   chưa khớp CSSOM — nên để `GOOSIE-CSSOMTASK-001` quyết định sau.
2. §10.8: tên test `TestCreateHTMLDocumentBuildsTheStandardSkeleton` và sub-case
   `"html first, doctype present"` nên đổi thành "the engine's own skeleton" khi defect được đăng
   ký; giữ nguyên assertion.
3. §10.4: hai câu trong §5.5/§6 quy hết reveal bất đồng bộ cho "headless không pump". Số của reproB
   cho thấy **cả khi pump**, reveal vẫn hỏng vì một defect khác. Đề nghị sửa §5.5 thành hai nguyên
   nhân khi lượt doc kia được cập nhật — tôi không tự sửa chữ của họ.
