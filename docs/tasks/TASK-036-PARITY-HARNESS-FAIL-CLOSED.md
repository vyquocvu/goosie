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
