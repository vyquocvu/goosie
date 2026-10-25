# TASK-045 — Nhãn cuối của thang live-parity đang tuyên bố một điều không được đo

**Priority:** P2 (GOOSIE-LIVE-004) + P3 (GOOSIE-LIVE-008), làm chung một task vì cả hai sửa
**ý nghĩa của báo cáo** trong cùng một file. **Method:** INT (selftest offline của harness) +
REAL (sweep 8 URL vào `/tmp`) + MUTATION (tắt từng hành vi, tựtest phải đỏ).
**Owner:** chưa gán. **Defect:** `GOOSIE-LIVE-004`, `GOOSIE-LIVE-008`.
**Liên quan:** `docs/tasks/TASK-038-LIVE-WBSITE-PARITY.md` (thang nhãn §2.2, ngưỡng đóng băng
§2.4, bằng chứng §6.3/§6.5), `docs/tasks/TASK-042-BLANK-BAND-DRIFT.md` (`BLANK_OUTPUT` đã bị
làm yếu trước đó), `testdata/live-parity.py` (nơi duy nhất sửa logic), `Makefile` (chỉ comment
của target `live-parity`), `docs/qa-live-parity/2026-10-25T2045Z-ec0f7fb9.{md,json}` (bằng chứng
đọc nguyên trạng, **không sửa**).

Task này **không sửa engine** và **không thêm bất kỳ ngưỡng nào**. Nó lấy lại hai tuyên bố sai:
một cái tên đọc như một phán quyết, và một tên file đọc như chứng cứ về build.

## 0. Hai lỗi, phát biểu chính xác

* **GOOSIE-LIVE-004** — nhãn cuối `OK` assert một điều không phép đo nào kiểm. `classify()` rơi
  xuống nhánh cuối sau khi đã xét ink, blank band và `coverage`; **không một rung nào ở trên so
  sánh vị trí điểm ảnh**. `worst_iou` được tính rồi ghi vào mọi row và không ai đọc. `coverage`
  là tỉ lệ **khối lượng mực** `goosie_total / chromium_total`, nên nó vượt 1 khi Goosie in *nhiều
  hơn* trang gốc chứ không phải in *đúng*.
* **GOOSIE-LIVE-008** — `run_id` (và vì thế tên file báo cáo) được ghép từ SHA của HEAD **lúc ghi
  file**, không phải lúc đo. Một sweep dài hơn một lần merge sẽ mang tên một commit chưa từng chạy.

## 1. Bằng chứng đã đo

### 1.1 `OK` in ra trên những trang gần như không khớp một điểm ảnh nào

Đo trực tiếp trên `docs/qa-live-parity/2026-10-25T2045Z-ec0f7fb9.json` (100 row, 93 được chấm,
44 row mang nhãn cuối), bằng code của chính `parity.py`/`live-parity.py`, chỉ đọc:

| Con số | Giá trị đo được |
| --- | --- |
| row mang nhãn cuối (`OK`) | **44** |
| trong đó `pct_exact` < 50 | **17** |
| row có `coverage` > 1.0 (trên toàn bộ 100 row) | **28**, trong đó 25 row mang nhãn cuối |
| row nhãn cuối có `worst_iou` < 0.5 / < 0.1 | **24** / **13** |

Sáu trang tệ nhất về khớp điểm ảnh mà vẫn được in là `OK`:

| URL | pct_exact | coverage (khối lượng mực) | worst_iou |
| --- | --- | --- | --- |
| `https://www.target.com/` | 0.24 | 0.61 | 0.584 |
| `https://www.w3.org/Style/CSS/` | 0.63 | **2.834** | 0.594 |
| `https://tuoitre.vn/` | 0.84 | 2.005 | 0.627 |
| `https://plo.vn/` | 1.96 | 1.494 | 0.628 |
| `https://nld.com.vn/` | 3.6 | 1.48 | 0.628 |
| `https://www.ebay.com/` | 6.4 | 2.572 | 0.578 |

`w3.org/Style/CSS/` là vụ điển hình: **0.63%** điểm ảnh khớp, `coverage` 2.834 — con số cao nhất
nhóm đó lại là con số vô nghĩa nhất, vì nó đo lượng mực in thừa.

Không có gì ở đây là lỗi của ngưỡng: `TASK-038` §6.3 đã ghi rõ `OK` nghĩa là "đã paint, không
band trống, đủ khối lượng mực, đúng kích thước cửa sổ", **không** phải "đúng vị trí". Lỗi là ở
chỗ cái tên nói một điều khác với định nghĩa đã công bố.

### 1.2 Tên file không chỉ ra build nào đã tạo ra các điểm ảnh trong file

* `Makefile:68` tại base `6a44637` (nay là `:72` vì comment của task này dài thêm 4 dòng) —
  `live-parity: build`: binary được build **trước**, rồi sweep mới chạy.
* `run_id` sinh lúc ghi báo cáo (`main()`), tức là HEAD *sau* khi sweep kết thúc.
* Chứng cứ thực: sweep `2026-10-25T2045Z-ec0f7fb9` dùng `bin/goosie` có mtime
  `2026-10-25T20:31:52Z`, trong khi main đi qua `0cefc1d`, `bcfab51` rồi `ec0f7fb` trước 20:45Z.
  Toàn bộ 100 row của file đó mô tả engine của `6071b8b`, tức build mang tên sweep
  *liền trước nó* (`2017Z-6071b8b9`).

Hệ quả đo được: một người đọc hai file liên tiếp để tìm hồi quy sẽ quy đúng/sai cho một commit
chưa từng được chạy.

## 2. Thiết kế bản sửa — sửa ngữ nghĩa, không sáng tạo ra con số

### 2.1 Nhãn cuối đổi thành `UNFLAGGED`, khai báo một lần duy nhất

`testdata/live-parity.py:124` `TERMINAL_LABEL = "UNFLAGGED"`; `LADDER` (`:125-141`) và
`SCORED_LABELS` (`:145-148`) đều tham chiếu **hằng số đó**, không có chuỗi "OK" thứ hai nào.
Bài học "một quy tắc, một chỗ" từ `TASK-037`: một lần đổi tên phải đổi toàn bộ, và tựtest phải
đỏ nếu ai đó thêm lại tên cũ ở một chỗ khác.

* 14 rung phía trên **giữ nguyên văn và nguyên thứ tự**; chỉ rung cuối đổi tên.
* `classify()` (`:266-272`) trả `UNFLAGGED` kèm **ba dòng note** nói rõ thang đã *không* kiểm gì:
  `pct_exact … match within 30/channel`, `worst_iou … is the weakest reference-band overlap`, và
  `coverage … is ink mass (goosie/chromium), not position` + số band trống.
* Không có ngưỡng mới. `LIVE_COVERAGE_OK = 0.60`, `LIVE_MIN_INK = parity.MIN_INK` (200),
  `LIVE_MAX_BLANK_BANDS = 0`, `MIN_SCORED = 75` giữ nguyên giá trị đã công bố — chỉnh lại ngưỡng
  sau khi thấy dữ liệu là điều `TASK-038` §2.4/§6.5 cấm.

### 2.2 Hai con số bị bỏ quên giờ hiện ra cạnh nhãn

Bảng "Per page" có thêm cột `worst_iou` (`:640-655`), và một helper `cell()` (`:644`) in `—` cho
giá trị `null` để row không đo được vẫn hiển thị đúng thay vì in chữ `None`.

### 2.3 Một section **distribution**, không phải gate

`pct_exact_decades()` (`:430-458`) xếp các trang nhãn cuối vào 10 khoảng đều 10 điểm
(`0-9 … 80-89`, `90-100`), mỗi khoảng ghi số trang và trang thấp nhất về `pct_exact`,
`worst_iou`, `coverage`. Heading của nó tự khai: **"Terminal-labelled pages by pct_exact decade —
a distribution, not a gate"** (`DECADE_SECTION_TITLE`, `:162-163`), trong body in kèm câu
"**This is a distribution, not a gate**" và liệt kê đủ 10 khoảng kể cả khi rỗng.

Điểm cắt 10-điểm là hình dạng của một tỉ lệ phần trăm, chọn trước khi nhìn xem trang nào rơi vào
đâu. **Không một bucket nào được `classify()`, `exit_code()` hay `MIN_SCORED` đọc tới.**

### 2.4 `exit_code()` không đổi số

`:681-690` vẫn là công thức `TASK-038` shipped: `scored < 75 → 2`; `1` nếu chưa phải mọi trang
được chấm đều tới rung cuối hoặc có nhãn khác bật lên; `0` chỉ khi toàn bộ trang được chấm tới
rung cuối. Đổi tên nhãn không làm đổi bất kỳ kết quả nào — tựtest chốt lại bằng cặp
`exit_code({UNFLAGGED: 75}, 75) == 0` và `exit_code({UNFLAGGED: 74}, 74) == 2`
(không phát minh dấu "pass" nào: một sweep mà mọi trang khớp 0.0% điểm ảnh vẫn ra 0).

### 2.5 Danh tính build: chụp một lần, lúc bắt đầu run

`capture_build()` (`:398-421`) ghi **một lần, trước trang đầu tiên**: `source_sha`,
`source_sha8`, `binary_path` (đã resolve qua PATH nếu cần) và `binary_sha256` — hash của đúng
byte sẽ sinh ra mọi PNG. Hash là cái gánh toàn bộ chứng cứ: nó chỉ ra artifact thật khi cây nguồn
đã đi tiếp.

* JSON có thêm key `build` (`:503`) gồm bốn field trên + `report_sha8` (SHA-8 mà *tên file* claim,
  parse bằng `run_id_sha8()` `:424-427`) + `report_names_measured_build` (bool).
* `build_header_lines()` (`:533-562`) đặt dòng "**Pixels measured from:** …" lên **trước mọi
  bảng**, và đúng một trong ba cảnh báo: thiếu field → "**WARNING — this run cannot identify its
  own build**"; `report_sha8 != source_sha8` → "**WARNING — the filename names a different commit
  than these pixels**"; tên file không chứa SHA → ghi chú nói thẳng tên file không mang build nào.
* Tên file **vẫn giữ SHA lúc ghi** để tương thích với những gì integrator đang diff. Cái đổi là
  báo cáo giờ tự khai build nào đã tạo ra điểm ảnh, và cảnh báo khi hai cái khác nhau.
* `_sha8()` (`:1114`) được docstring ghi rõ là *write-time, filename-only*; fallback `"unknown"`
  chỉ dành cho tree không có git, và tựtest bây giờ từ chối nó khi `_git_rev()` trả được SHA.

### 2.6 Key JSON cũ ổn định, chỉ thêm

`url`, `label`, `notes`, `doc_height`, `pct_exact`, `coverage`, `doc_height`… giữ nguyên tên và
nguyên nghĩa. Thêm `MEASURED_KEYS`/`STABLE_ROW_KEYS` (`:155-158`) và `row.setdefault(k, None)`
(`:315-319`) để **mọi row mang cùng bộ key**, kể cả row không đo được (7/100 row `FETCH_FAILED`
của sweep 2045Z từng thiếu hẳn `pct_exact`/`coverage`/`worst_iou` — một cú diff liên-sweep mất
trang vì thiếu field). Thêm key là hợp lệ; đổi tên/xoá là breaking change, và comment ở `:150-154`
nói đúng điều đó.

## 3. Selftest: case mới phải bắt được lỗi, không được rỗng

Case `solid_overlay` (`:815`) dựng cố ý một trang **đạt rung cuối với ~0% điểm ảnh khớp**,
dựa trên dạng của `w3.org/Style/CSS/`: reference có hai band mảnh (ink 640 px ≥ `MIN_INK` 200,
nên `REFERENCE_EMPTY` không thể bật), Goosie paint nguyên một khối đặc chiếm hết cửa sổ
(`coverage` cao, `n_blank_bands` 0 nên `MISSING_REGIONS` không bật). Đo được:
`pct_exact 0.0`, `worst_iou 0.004`, `coverage 24.0`, `blank []` → nhãn `UNFLAGGED`.

Ba nhóm kiểm tra mới (tổng **49 PASS**, offline, không network, không Chromium):

* `terminal rung: a name, not a verdict (GOOSIE-LIVE-004)` — không rung nào của ladder là một
  từ phán quyết tốt (`goodness ∩ set(LADDER) == ∅`, `:857`), rung cuối đúng là `TERMINAL_LABEL`
  và vẫn được tính là scored; một trang khớp 0.0% vẫn tới rung cuối; note của row mang đủ
  `pct_exact` và `worst_iou`; 10 bucket đều nhau và tổng số trang đúng bằng số trang nhãn cuối;
  0% và 100% nằm ở hai đầu ngược nhau của **cùng một nhãn**; và không có dấu "pass" nào được
  sáng tạo — sweep mà mọi trang khớp 0.0% vẫn ra 0 khi đủ floor.
* `row keys: stable … (GOOSIE-LIVE-008)` — một bộ key duy nhất cho mọi row; row không đo được
  báo `null` chứ không thiếu field.
* `build identity: … (GOOSIE-LIVE-008)` — JSON nêu cả ba field; markdown nêu build ở dòng đầu,
  trước mọi bảng; tên file không SHA được xử lý thẳng thắn; `run_id_sha8()` parse `…-6071b8b9`
  và từ chối `nota-sha`; **warning mismatch xuất hiện đúng một lần và nêu cả hai SHA**; không có
  warning khi tên file trùng build; run không hash được binary thì tự nhận "unidentified"; section
  decade có mặt và tự gọi mình là distribution.

Ba kiểm tra cuối của nhóm thứ ba **không chạy trên fixture**: `capture_build()` được cho một file
thật trong tmp và phải trả đúng `hashlib.sha256(bytes)` của file đó, một path không đọc được phải
cho `binary_sha256 == ""` chứ không được đoán, và `_git_rev(8)` phải là 8 ký hex và là prefix thật
của `_git_rev()` (tree không có git thì cả ba phải rỗng). Lý do có chúng: ba kiểm tra đầu của nhóm
này đọc một dict literal, nên chúng **không thể** thấy lỗi trong chính hàm điền dict — và lỗi đó
đã xảy ra thật, xem §4.1.

## 4. Bằng chứng mutation (hành vi tắt, tựtest phải đỏ)

Mọi mutation chạy trên `testdata/live-parity.py` rồi restore bằng bản copy
`/tmp/t045-live-parity.BACKUP.py` (sha256 `d7a8468a…8a97`, bản trước khi sửa `_git_rev`) hoặc
`/tmp/t045-live-parity.FIXED.py` (sha256 `8a4a2904…5bdd`, bản cuối cùng); hash được xác nhận lại
sau mỗi lần restore.

| # | Mutation | Lệnh | rc | Case đỏ |
| --- | --- | --- | --- | --- |
| A | `TERMINAL_LABEL = "OK"` (đổi tên nhãn bị revert) | `python3 testdata/live-parity.py --selftest` | **1** | `FAIL no rung of the ladder is a goodness word, and the terminal rung is the last one and still counted as scored — vocabulary=['OK'] last=OK` |
| B | thêm lại `"OK"` vào `LADDER` trong khi nhãn cuối vẫn là `UNFLAGGED` | như trên | **1** | cùng case, `vocabulary=['OK'] last=UNFLAGGED` |
| C | bỏ `"build": build` khỏi payload JSON | như trên | **1** | `FAIL the JSON names the source SHA, the binary path and the binary SHA-256 — null` + `FAIL the markdown names the measured build in its first lines, above any table` (in ra `source UNKNOWN, binary UNKNOWN`), rồi `KeyError: 'build'` tại `selftest` `:970` |
| D | `_git_rev(8)` bỏ `HEAD` khỏi lệnh (`rev-parse --short=8` một mình) | như trên | **1** | `FAIL a run in a git tree names the build's source SHA, and the abbreviated form is a real prefix of the full one — full='6a44637…' short='' _sha8='unknown'` + `FAIL capture_build() hashes the bytes it is given: … "source_sha8": ""` |

Sau khi restore: `selftest` rc=0 và `git diff --numstat` là
`testdata/live-parity.py  448  45`, `Makefile  5  1` (tính cả sửa đổi ở §4.1).

Ghi chú trung thực về mutation C: kiểm tra *determinism* ("two runs into different artifact dirs
produce identical JSON") **vẫn xanh**, vì cả hai lần viết đều thiếu `build` như nhau. Cái đỏ là
kiểm tra danh tính build — đúng như mong đợi: mutation này lấy đi thông tin, không lấy đi tính
ổn định.

### 4.1 Chính sweep thật bắt được một lỗi do task này tạo ra

Sweep 8 URL đầu tiên (`/tmp/t045-sweep/2026-10-25T2130Z-unknown.md`) in ra
`measured build: source UNKNOWN` và tên file mang đuôi `-unknown`. Nguyên nhân: khi gom
`_sha8()` vào `_git_rev(abbrev)`, nhánh rút ngắn sinh lệnh `git rev-parse --short=8` **thiếu
revision** — git thoát 128 và in `fatal: Needed a single revision` ra stderr, stdout rỗng, nên
`source_sha8` trống và `_sha8()` rơi vào fallback `"unknown"`.

Ba kiểm tra đầu của nhóm build identity **không thể** thấy lỗi này vì chúng đọc một dict literal
(`build = {"source_sha": "0"*40, …}`) chứ không gọi `capture_build()`. Fix: `_git_rev` luôn truyền
`HEAD` (`:389`) và trả `""` khi `returncode != 0`; ba kiểm tra phi-fixture ở §3 được thêm để
mutation D trở thành đỏ. Bài học ghi thẳng: selftest bằng fixture chỉ chứng minh được phần *đọc*
dữ liệu, không chứng minh được phần *thu thập* dữ liệu — một thay đổi về danh tính build bắt buộc
phải có ít nhất một run thật.

## 5. Cổng đã chạy, exit code thật

| Lệnh | rc |
| --- | --- |
| `python3 testdata/live-parity.py --selftest` | **0** ("selftest green", 49 PASS) |
| `make live-parity-selftest` | **0** (cùng một lệnh, qua Makefile) |
| `python3 -m py_compile testdata/live-parity.py` | **0** |
| `go test ./...` | **0** |
| `go test ./internal/archtest/` | **0** |
| `make -n live-parity` | **0** (target vẫn parse, vẫn build trước rồi sweep) |
| `git diff --check` | **0** |
| `gofmt -l` trên file Go đã sửa | không áp dụng — **không sửa file Go nào** |
| 4 mutation A/B/C/D, mỗi cái một lần `--selftest` | **1** mỗi cái (xem §4) |
| `python3 testdata/live-parity.py --binary ./bin/goosie --corpus /tmp/t045-corpus.json --out-dir /tmp/t045-sweep` (lần 1, phát hiện §4.1) | **2** |
| `python3 testdata/live-parity.py --binary ./bin/goosie --corpus /tmp/t045-corpus.json --out-dir /tmp/t045-sweep2` (lần 2, sau fix) | **2** |

`rc=2` ở hai sweep là **đúng như công bố**, không phải kết quả của bản sửa: 8 trang được chấm
không thể vượt floor `MIN_SCORED = 75`, và tool báo "too few pages compared — this run is broken".
Đó chính là hành vi fail-closed của `TASK-036`/`TASK-038` còn nguyên sau khi đổi nhãn.

## 6. Sweep thật (8 URL, ghi vào /tmp, không đụng docs/qa-live-parity)

Corpus: 8 site đầu của `testdata/live-sites.json`, copy ra `/tmp/t045-corpus.json`
(**file corpus trong repo không bị sửa**). Lệnh:

```bash
python3 testdata/live-parity.py --binary ./bin/goosie \
  --corpus /tmp/t045-corpus.json --out-dir /tmp/t045-sweep2
# rc = 2  (floor 75, 8 trang -> "this run is broken", đúng như §5)
```

15 dòng đầu của `/tmp/t045-sweep2/2026-10-25T2136Z-6a446372.md`, nguyên văn:

```markdown
# Live render parity — 2026-10-25T2136Z-6a446372

**Pixels measured from:** source `6a446372` (`6a44637285b64c61112a79a12ba3cc2366522b9c`), binary `bin/goosie`, SHA-256 `84ddace5731e7e7cef41b7ed7d9e772b85134a7911972a14040c45ba5b6d7482` — captured at the start of the run, before the first page was fetched.

Corpus `t045-corpus.json`: **8 pages**, of which **8 produced a two-sided comparison** (floor 75, NOT met — the run is broken).

Viewport-only on both sides; Goosie's screenshot driver never scrolls. This is a report, not a gate — see docs/tasks/TASK-038-LIVE-WBSITE-PARITY.md.

`UNFLAGGED` is the ladder's terminal rung, not a verdict: it means no rung above it fired, which the ink/blank-band/coverage checks decide. Coverage is ink MASS and may exceed 1; pixel agreement (`pct_exact`) and band position (`worst_iou`) are printed beside the label below and sorted into decades in “Terminal-labelled pages by pct_exact decade — a distribution, not a gate” (GOOSIE-LIVE-004).

## Noise floor (same pages rendered twice)

- 8 sites, 7 comparable: max |Δpct_exact| = 23.61, max |Δcoverage| = 0.562, 2 label flips.
No conclusion below may be stated unless it clears this spread.

## Labels
```

Bằng chứng đo từ chính file đó (`…-6a446372.json`):

* `build` là key **duy nhất** được thêm vào top-level; **không key nào bị đổi tên hoặc xoá**
  (diff tập key với `2026-10-25T2045Z-ec0f7fb9.json`: added `['build']`, removed `[]`).
* `{ "source_sha": "6a446372…", "source_sha8": "6a446372", "binary_path": "bin/goosie",
  "binary_sha256": "84ddace5731e…", "report_sha8": "6a446372",
  "report_names_measured_build": true }` — run này tên file và build trùng nhau nên **không** có
  warning, đúng hành vi mong đợi; nhánh mismatch được chốt ở §3.
* Mọi row mang **cùng một bộ key** (1 key set cho 8 row).
* `https://tuoitre.vn/` → `UNFLAGGED` với `pct_exact 1.23`, `coverage 1.443`, `worst_iou 0.627`,
  `0/4 band trống`. **Đây chính là hình dạng của GOOSIE-LIVE-004 đã đo trong báo cáo cũ**
  (trang này từng là `OK` với `pct_exact 0.84` ở sweep 2045Z): một trang khớp 1.23% điểm ảnh giờ
  mang một cái tên không tuyên bố gì, kèm note:
  `no rung above fired; this is not a claim that the render agrees | pct_exact 1.23% of pixels
  match within 30/channel, worst_iou 0.627 is the weakest reference-band overlap | coverage 1.443
  is ink mass (goosie/chromium), not position, and 0 of 4 reference bands are empty`.
* Section decade trên dữ liệu thật: `0-9 | 1 | tuoitre.vn`, `30-39 | 1 | dantri.com.vn`, 8 khoảng
  còn bằng 0, và dòng kết "**2 page(s) carry `UNFLAGGED`; 2 of them agree on fewer than 50% of
  their pixels**" — không kèm phán quyết.
* Noise floor của chính run này ghi `2 label flips` giữa hai lần render cùng 8 trang: một lời nhắc
  rằng `pct_exact` đơn lẻ không thể thành gate mà không so với độ lệch này.

## 7. Giới hạn — những gì task này **không** thể chốt bằng cổng

* **Không có cổng nào cho "pixel agreement".** Task cố ý không thêm ngưỡng. Phần distribution
  cho người đọc thấy 17/44 trang dưới 50% khớp điểm ảnh, nhưng exit code vẫn không đổi. Phán
  quyết "bao nhiêu phần trăm là đủ" vẫn là việc của người đọc + defect riêng, và bất kỳ con số
  nào đưa vào lúc này đều là post-hoc tuning (`TASK-038` §6.5 cấm).
* **Sweep 100 trang không chạy lại ở đây** (~20 phút, và sẽ ghi vào `docs/qa-live-parity/` — vùng
  bị cấm sửa). Vì vậy *báo cáo mới nhất có nhãn `UNFLAGGED`* chưa tồn tại trong repo; các file đã
  commit vẫn mang `OK`, và người diff liên-sweep sẽ thấy từ vựng nhãn đổi từ `OK` sang
  `UNFLAGGED` tại commit này. Đây là hệ quả có chủ đích của việc đổi tên, không phải hồi quy đo
  lường; những key mà integrator diff (`url`, `label`, `pct_exact`, `coverage`, `doc_height`) vẫn
  nguyên tên.
* **Nhánh "warning mismatch" chỉ được chứng minh offline.** Để có nó trong một báo cáo thật, một
  lần merge phải rơi vào giữa một sweep — không thể sắp đặt một cách trung thực. Tựtest dựng cả
  hai phía (match và mismatch) nên hành vi không thể âm thầm mất.
* **`binary_sha256` chỉ ổn định trên chính artifact, không phải "rebuild ra cùng hash".**
  `make build` dùng `-trimpath` nhưng hash vẫn phụ thuộc toolchain/OS. Ta không tuyên bố gì về
  tính tái lập của build, chỉ tuyên bố: file nói đúng *từng byte* đã paint.
* **`capture_build()` gọi `git`**, nên một cây không phải git sẽ cho `source_sha` rỗng → báo cáo
  tự nhận "cannot identify its own build" thay vì im lặng. Không có cổng nào chứng minh điều đó
  trên một tree không có git; nó được chốt bằng fixture thiếu field trong tựtest.
* **Phần mềm tham chiếu vẫn đổi.** Chromium trên máy này là `chromium-1148`; `pct_exact` của một
  sweep chỉ so được với sweep khác nếu cùng binary Chromium. `build` block hiện tại chỉ danh tính
  Goosie, không phải của reference browser — đó là giới hạn còn nguyên của task này.
* **`docs/qa-open-defects.json` không được sửa** (ngoài phạm vi cho phép). Hai defect vẫn `open`.

## 8. Đề xuất dòng registry cho integrator

```json
{ "id": "GOOSIE-LIVE-004", "status": "fixed-unverified",
  "note": "TASK-045: nhãn cuối đổi thành UNFLAGGED, note per-row + cột worst_iou carry pct_exact/worst_iou, thêm section distribution theo decade (không gate). MIN_SCORED/LIVE_COVERAGE_OK/LIVE_MIN_INK/LIVE_MAX_BLANK_BANDS và exit code không đổi. Cần một sweep 100 trang để xác nhận từ vựng mới." }
{ "id": "GOOSIE-LIVE-008", "status": "fixed-unverified",
  "note": "TASK-045: build block (source SHA + binary path + SHA-256) chụp lúc bắt đầu run, vào JSON và header markdown; báo cáo cảnh báo khi tên file chỉ một commit khác. Tên file giữ SHA lúc ghi. Cần sweep thật kế tiếp để xác nhận header." }
```

`fixed-unverified` là trạng thái thật: hành vi được chốt bằng tựtest offline và một sweep 8 URL,
chưa bằng một sweep 100 trang đầy đủ.
