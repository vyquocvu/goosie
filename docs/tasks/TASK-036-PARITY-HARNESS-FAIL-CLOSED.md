# TASK-036 — oracle của parity phải không thể bị gian lận (BR-176, BR-177)

**Priority:** P1. **Method:** INT + PARITY. **Owner:** chưa gán.
**Blocked cases:** BR-176 ("Chạy render parity corpus → mọi fixture có reference cho kết quả;
diff được xuất để xem và không có fixture nào bị bỏ qua âm thầm") và BR-177 ("Thiếu
Chromium/reference artifact rồi chạy parity → lệnh fail-closed hoặc báo BLOCKED, không báo
PASS"), cả hai hiện `NOT RUN` và **không có test nào** (`docs/qa-test-catalog.md`).
**Defect mới:** `GOOSIE-PARITYGATE-001`.

Task này không sửa engine. Nó sửa **công cụ đo** mà mọi task PARITY khác (TASK-027/033/035)
đang dựa vào. Nếu công cụ đo gian lận được thì toàn bộ bằng chứng render của repo này vô nghĩa.

## 1. Bằng chứng đã đo, không đoán

Đo bằng cách import `testdata/parity.py` (`importlib`) và ghi đè `FIXTURES`/`GOOSIE`/`CHROMIUM`
sang corpus tạm 1 fixture trong `/tmp`, không đụng thư mục reference thật. Output nguyên văn:

| # | Tình huống | Kết quả đo |
| --- | --- | --- |
| A | cặp render khớp, `known_gaps={}` | `('a.png', 'PASS', 100.0, [], 0, None)` |
| B | **baseline khai báo 5 band trong khi đo được 0** | `('a.png', 'PASS', 100.0, [], 5, None)` — **vẫn PASS** |
| C | thiếu `chromium/a.png`, strict (mặc định) | `('a.png', 'MISSING', 0.0, [], 0, 'chromium render missing')` |
| D | thiếu `chromium/a.png`, **`strict=False`** | `[]` — fixture **biến mất khỏi báo cáo**, không có dòng nào |
| E | `load_known_gaps(<file không tồn tại>)` | `FileNotFoundError: [Errno 2] ...` (traceback, rc≠0) |

Nguồn của từng dòng:

| Sự thật | Chỗ trong code |
| --- | --- |
| `status = "PASS" if pct >= pass_at and len(blank_rows) <= allowed_blanks` — quan hệ là `<=`, nên nới baseline luôn PASS | `testdata/parity.py:353` |
| `--strict` có `action="store_true", default=True` nên nó **là no-op**; `--no-strict` mới là công tắc thật | `testdata/parity.py:695-699` |
| Ở chế độ non-strict, fixture thiếu pair bị `continue` **không append** → mẫu số nhỏ lại, `print_summary` in "N/N (100%)", `main` return 0 | `testdata/parity.py:312-331`, `778-779` |
| `if not results: return 2` chỉ cứu được trường hợp **tất cả** đều thiếu; thiếu một nửa thì không | `testdata/parity.py:741-744` |
| Không có chỗ nào khoá **số fixture tối thiểu**: xoá bớt `testdata/render/**/*.html` thì gate vẫn xanh | không tồn tại trong `testdata/parity.py` |
| Call site: `scripts/run-parity.sh` truyền `--strict`; `scripts/rc-sweep.sh:284` chạy parity **không** truyền flag nào (strict mặc định) | `scripts/run-parity.sh` (mục scoring), `scripts/rc-sweep.sh:284` |
| `--no-strict` **không** được bất kỳ script/CI/Makefile nào dùng | `grep -rn "no-strict" scripts/ Makefile .github/ docs/` → rỗng |
| Baseline thật **hôm nay đã bằng**, không thừa: forms/04 2/2, html-features/01 1/1, selectors/02 3/3, unicode/02 1/1, selectors/03 3/3 | `make parity` trên `4437f65`, dòng "known gaps: 5 fixtures / 10 bands" |

Hệ quả: **mọi** claim "parity xanh" trong repo hiện có hai lỗ im lặng — nới baseline, và làm
co mẫu số. cả hai đều cho rc 0.

## 2. Thiết kế tối thiểu

1. **Bỏ hẳn escape hatch.** Xoá `--no-strict` và tham số `strict` của `score_fixtures`;
   missing pair và dimension mismatch luôn tạo `FixtureResult`. Giữ `--strict` trong argparse
   (`action="store_true"`) để `scripts/run-parity.sh` không vỡ, nhưng nó không còn bật/tắt gì;
   chú thích trong `--help` phải ghi rõ "accepted for compatibility, strictness is not
   optional". Không viết shim tương thích nào khác.
2. **Ratchet band chính xác.** Thay `<=` bằng đúng bằng: nếu `len(blank_rows) != allowed_blanks`
   thì FAIL với `error="known-gap baseline is stale: declared N, measured M"`. Đã đo ở §1: hôm
   nay 5/5 entry **bằng nhau**, nên gate mới xanh ngay — ai nới baseline sẽ đỏ, ai sửa được một
   band mà không thu file sẽ đỏ. Đây chính là quy tắc "chỉ được giảm" mà các task trước ghi bằng
   chữ, giờ thành máy.
3. **Sàn số fixture.** Thêm `MIN_SCORED_FIXTURES` (đặt cạnh `THRESHOLD`/`PASS_AT`, giá trị = số
   fixture đo được lúc làm task) và `+1` khi thêm fixture mới. Nếu `len(results)` nhỏ hơn sàn
   → in lý do và `return 2`. Song song, test Go đếm `testdata/render/**/*.html` và so với đúng
   con số đó, để không ai sửa một bên.
4. **Test harness:** `testdata/parity_selftest.py` dùng `importlib` (mẫu ở §1) + corpus tạm
   trong `t.TempDir()`, không ghi vào `/tmp/goosie-renders` hay `/tmp/playwright-renders`;
   và `test/gatecheck/parity_harness_test.go` chạy nó qua `python3`, skip khi thiếu
   `python3`/`numpy`/`PIL` (CI đã cài: `.github/workflows/ci.yml:131`).

Không đổi `THRESHOLD`, `PASS_AT`, `MIN_INK`, `BAND_GAP`, `INK_CUTOFF` — task này cấm chạm
ngưỡng, chỉ cấm *ăn gian* ngưỡng.

## 3. Test phải có

* `testdata/parity_selftest.py` (case exit code ≠ 0 là đủ, không cần ảnh thật):
  1. thiếu chromium render → có `FixtureResult` status `MISSING` **kể cả khi** caller muốn
     non-strict (tham số không còn tồn tại) ⇒ BR-177.
  2. baseline 5, đo 0 band → FAIL + thông báo "stale" ⇒ BR-176.
  3. baseline khớp band đo được → PASS ⇒ BR-176 (oracle dương, tránh "sửa thành luôn FAIL").
  4. `len(results) < MIN_SCORED_FIXTURES` → `main` return 2 ⇒ BR-176.
  5. thiếu `testdata/render-known-gaps.json` → không bao giờ rc 0 (dù traceback hay error
     sạch) ⇒ BR-177.
* `test/gatecheck/parity_harness_test.go`:
  6. chạy selftest bằng `python3`, assert rc 0; tag `// qc:BR-177`.
  7. đếm fixture thật, assert `== MIN_SCORED_FIXTURES` (đọc số từ `testdata/parity.py` bằng
     regex, không hard-code hai nơi theo kiểu có thể lệch); tag `// qc:BR-176`.
  8. `python3 testdata/parity.py --no-strict` → rc ≠ 0 và stderr chứa `unrecognized` — chứng
     minh escape hatch đã chết thay vì bị ẩn.
* Chạy `make parity` thật một lần: **đòi hỏi** vẫn 80/80, average 96.07%, và 5 entry known-gaps
  khớp band đo được (không sửa file baseline).

## 4. Nghiệm thu `[R-7]`

Mỗi proof là **tắt hành vi**, dán `FAIL → revert → PASS`:

* (i) phục hồi `<=` thành `len(blank_rows) <= allowed_blanks` ⇒ test 2 **FAIL** (case "dư
  baseline" không còn bị bắt). Nếu test vẫn xanh thì nó không đo gì — viết lại.
* (ii) phục hồi nhánh `if not strict: continue` (không append) ⇒ test 1 và 8 **FAIL**.
* (iii) đặt `MIN_SCORED_FIXTURES = 0` ⇒ test 4 và 7 **FAIL**.
* (iv) nâng một entry trong `testdata/render-known-gaps.json` lên `+1` ⇒ `make parity` và test 3
  **FAIL**; đây là mutation mà **không** test nào hiện nay bắt được.
* (v) xoá `--strict` khỏi argparse ⇒ test 8 vẫn phải fail-closed khi gọi `scripts/run-parity.sh`
  (measure, đừng đoán): nếu `run-parity.sh` vỡ với rc 2 thì ghi rõ là call site đã được cập nhật
  trong cùng commit.

Proof nào không giết được test nào thì **khai** trong §Status, không im lặng.

## 5. Ràng buộc vận hành

Không `git push`, không merge `main`, không worktree con. Test chạy không cần display và
**không** cần Chromium: selftest chỉ dùng corpus tạm. `go test ./internal/archtest/`,
`go vet -all ./...` xanh. Không sửa `docs/qa-test-catalog.md`; đề xuất dòng registry
(`GOOSIE-PARITYGATE-001`) trong báo cáo, integrator mới là người ghi.
`testdata/render-known-gaps.json` **không được đổi** bởi task này — nếu nó phải đổi thì thiết
kế sai.

## 6. Ngoài phạm vi

`BR-178` (interaction parity: thứ tự event so với `reference-events.json` — harness khác,
task riêng); `--inspect` output; việc fixture nào *đạt* gì (đó là đất của TASK-027/033/035);
mọi thay đổi trong `internal/`.

## 7. Kết quả (branch `worktree-task-036-parity-fail-closed`)

Máy đo: macOS, `python3` 3.9.6 + numpy 1.26.4 + PIL 11.3.0. Uptime trước khi làm
`0:48 up 5 days, load 5.37`; sau khi chạy `go test ./internal/archtest/` +
`go vet -all ./...`: `1:09 up 5 days, load 51.96` (hai job khác đang chạy song song —
độ trễ do họ, không phải do lệnh này).

### 7.1 Đã đổi gì (chỉ chạm công cụ đo, không đụng ngưỡng)

`testdata/parity.py` (55+, 27-):
- `MIN_SCORED_FIXTURES = 80` đặt cạnh `THRESHOLD`/`PASS_AT` (dòng 56) — sàn số fixture;
  Go test đọc đúng dòng này bằng regex nên không lệch được.
- `score_fixtures(threshold, pass_at, known_gaps=None)` (dòng 314): **bỏ tham số
  `strict`**; ba nhánh missing luôn `append` `FixtureResult`.
- Điều kiện pass (dòng 372): `elif len(blank_rows) != allowed_blanks:` → FAIL với
  `error="known-gap baseline is stale: declared N, measured M"` (đệm `!=`, không còn `<=`).
- argparse (dòng 723): **xoá `--no-strict`**; giữ `--strict` (`action="store_true"`) như
  no-op với help "accepted for compatibility, strictness is not optional".
- main (dòng 767): `if len(results) < MIN_SCORED_FIXTURES: return 2` thay cho `if not results`.

Không đổi `THRESHOLD`, `PASS_AT`, `MIN_INK`, `BAND_GAP`, `INK_CUTOFF`.
Không đổi `testdata/render-known-gaps.json` (chỉ sửa tạm cho proof (iv) rồi `git checkout`).

Test mới: `testdata/parity_selftest.py` (5 case, corpus tạm trong `tempfile.mkdtemp`,
không mạng/không browser, không ghi `/tmp/{goosie,playwright}-renders`) và
`test/gatecheck/parity_harness_test.go` (3 test, skip khi thiếu python3/numpy/PIL).

### 7.2 Lệnh và output nguyên văn (trạng thái sau fix)

```
$ python3 testdata/parity.py            # corpus thật, đã có render ở /tmp
  80/80 passing  (100.0%), 0 failing, 0 errors
  average match: 96.08%  (threshold=30, pass_at=90.0%)
  known gaps: 5 fixtures / 10 bands
  (forms/04 2/2, html-features/01 1/1, selectors/02 3/3, unicode/02 1/1, selectors/03 3/3)
  rc=0
```
Spec §3 dự kiến 96.07%; đo thực tế **96.08%** (làm tròn, 80 fixture vẫn PASS, baseline vẫn
bằng nhau — không đổi ngưỡng). Selftest và Go gate:

```
$ python3 testdata/parity_selftest.py    -> "selftest green", rc=0   (5/5 case PASS)
$ go test ./test/gatecheck/ -run TestParity
--- PASS: TestParitySelftestIsGreen      (BR-177)
--- PASS: TestParityFixtureFloorMatchesCorpus (BR-176)
--- PASS: TestParityRejectsNoStrictFlag  (BR-177)
$ python3 testdata/parity.py --no-strict  -> rc=2, "parity.py: error: unrecognized arguments: --no-strict"
$ python3 testdata/parity.py --strict     -> rc=0 (accepted, no-op)
$ go test ./internal/archtest/            -> ok (gofmt + import + cgo + globals rules)
$ go vet -all ./...                        -> rc=0 (silent)
```

### 7.3 Năm mutation proof (§4) — mỗi cái: tắt hành vi → FAIL → revert → PASS

Backup `parity.py` đã-fix ra `/tmp` rồi `cp` lại khi revert; `render-known-gaps.json` revert
bằng `git checkout`. Sau tất cả, `diff` bản revert = bản fix (không còn residue) và xoá backup.

**(i) `!=` → `>` (phục hồi `<=`).** test 2 FAIL, reproducing đúng row B của §1:
```
case 2: declared 5 bands against 0 measured is FAIL, not PASS (BR-176)
    measured: ('a.png', 'PASS', 100.0, [], 5, None)
  FAIL  status is FAIL (was PASS before the fix)  -- PASS
  FAIL  error names a stale baseline  -- None
selftest rc=1 ; go TestParitySelftestIsGreen FAIL (case 3 vẫn PASS vì measured==declared)
```
Revert (`!=`): `('a.png', 'FAIL', 100.0, [], 5, 'known-gap baseline is stale: declared 5, measured 0')`,
selftest rc=0, go rc=0.

**(ii) phục hồi `strict` param + `if strict:` branch + `--no-strict` + main `strict=`.** test 1 và
test 8 FAIL:
```
case 1:
  FAIL  score_fixtures no longer takes a 'strict' parameter -- params=['threshold','pass_at','strict','known_gaps']
  FAIL  calling score_fixtures(strict=False) raises TypeError ...
$ python3 testdata/parity.py --no-strict  -> rc=0 (hatch sống lại, âm thầm chạy)
go TestParityRejectsNoStrictFlag FAIL (chấp nhận --no-strict, in "known gaps" đầy đủ)
```
Revert: `--no-strict` rc=2 unrecognized; selftest rc=0, go rc=0.

**(iii) `MIN_SCORED_FIXTURES = 0`.** test 4 và test 7 FAIL:
```
case 4: main() với 1 fixture in "1/1 passing (100.0%)" và rc=0 (đáng lẽ 2)
  FAIL  main() returns 2 for 1 fixture (floor 0)  -- rc=0
go TestParityFixtureFloorMatchesCorpus FAIL:
  "testdata/render has 80 HTML fixtures but MIN_SCORED_FIXTURES is 0; ..."
```
Revert (`= 80`): selftest rc=0, go rc=0.

**(iv) `render-known-gaps.json` forms/04 `2 → 3`** (đây là mutation §4 ghi là *không test nào
hiện nay bắt được* — giờ đã bị bắt). `python3 testdata/parity.py` (corpus thật) và test 3 FAIL:
```
$ python3 testdata/parity.py
   96.63%  FAIL  forms/04-advanced-inputs.png ... blank_regions=2/3 rows=[(376,388),(443,454)]
  79/80 passing  (98.8%), 1 failing, 0 errors
  rc=1
case 3: FAIL  forms/04-advanced-inputs.png: measured 2 == declared 3 -> PASS -- FAIL measured=2 declared=3
```
Revert (`git checkout`): forms/04 về `blank_regions: 2`; real rc=0, selftest rc=0, go rc=0.
`git status --porcelain testdata/render-known-gaps.json` sạch.

**(v) xoá `--strict` khỏi argparse.** ĐO, không đoán:
```
$ python3 testdata/parity.py --strict  -> rc=2, "parity.py: error: unrecognized arguments: --strict"
```
`scripts/run-parity.sh:146` truyền `--strict`, và `:188 exit $PARITY_EXIT` → run-parity.sh sẽ rc 2
(fail-closed ầm ĩ, không im lặng). **KHAI BÁO: proof (v) KHÔNG giết test Go/selftest nào** —
chúng không bao giờ truyền `--strict` (selftest rc=0, go rc=0 vẫn xanh dưới mutation này). Nó
được phòng thủ bởi chính call site: vì thế thiết kế **giữ** `--strict`; không call-site nào cần
sửa trong commit này.

### 7.4 Hệ quả phụ traceability (integrator xử lý, mình không đụng golden)

`go test ./test/qc/` sau khi gắn tag:
```
traceability_test.go:112: traceability.golden.json is stale; run go test ./test/qc -update
  (BR-176/BR-177 now point at test/gatecheck/parity_harness_test.go, golden still tests:null)
```
Đây là **bước ghi registry của integrator**: `go test ./test/qc -update` (re-sync golden) +
chuyển BR-176/BR-177 sang RUN trong `docs/qa-test-catalog.md`. Mình bị cấm sửa
`traceability.golden.json`/catalog nên để nguyên; `archtest` và `vet` không liên quan, vẫn xanh.
rc-sweep (BR-180) đọc bản golden đã commit (chưa đổi) → không bị ảnh hưởng.

### 7.5 Đề xuất dòng registry (integrator ghi, không phải mình)

- `GOOSIE-PARITYGATE-001` → `closed_by`:
  `test/gatecheck/parity_harness_test.go:TestParitySelftestIsGreen`,
  `:TestParityFixtureFloorMatchesCorpus`, `:TestParityRejectsNoStrictFlag`
  (backing Python: `testdata/parity_selftest.py` cases 1–5).
- BR-177 (thiếu artifact → fail-closed, không báo PASS): selftest case 1 + case 5;
  Go `TestParitySelftestIsGreen`, `TestParityRejectsNoStrictFlag` (tag `// qc:BR-177`).
- BR-176 (mọi fixture có reference cho kết quả, không bỏ qua âm thầm, baseline trung thực):
  selftest case 2 + case 3 + case 4; Go `TestParityFixtureFloorMatchesCorpus` (tag `// qc:BR-176`).

### 7.6 Chưa chứng minh được / còn mở

- Không chạy `make parity --render-chromium` (cần browser, agent khác đang dùng). Đã dùng
  `python3 testdata/parity.py` trên render thật có sẵn ở `/tmp` cho proof (iv) — đủ để bắt
  inflation. Nếu `/tmp` trống, proof (iv) vẫn chạy qua selftest case 3 (corpus tạm).
- `go test ./test/qc/` đỏ do golden stale — **có chủ đích**, integrator `-update`.
- `--inspect` output: ngoài phạm vi (§6), không đổi.
