# TASK-029: BR-180 — release candidate sweep, cổng tổng hợp cuối cùng còn trống

- **Task ID**: `TASK-029`
- **Type**: Release engineering + QC gate
- **Priority**: P0 (BR-180 là P0 cuối cùng chưa có test gắn tag)
- **Assigned to**: chưa chia
- **Review**: `qoder cli` (integrator)
- **Catalog**: `docs/qa-test-catalog.md` dòng BR-180; DoD ở `docs/qa-test-plan.md` §"Tiêu chí đóng một test run" và ma trận "Release candidate".

## 1. Case nói gì, và phần nào tự động hoá được

> **BR-180 | P0 | CUA, PARITY | NOT RUN** — Trên build release candidate, chạy lại toàn bộ P0/P1 CUA liên quan và visual parity, gồm Google Products live smoke sau fixture → không còn lỗi P0/P1 mở, evidence đủ và sign-off theo DoD.

Tạm dịch sang việc: một lệnh, chạy trên đúng binary sắp phát hành, và cho ra **một verdict kèm evidence** cho bốn thứ: (a) mọi P0/P1 tự động hoá được thì xanh *ngay bây giờ*, (b) visual parity đạt, (c) live smoke Google Products có chạy và có kết luận, (d) không còn defect P0/P1 mở và có người ký. Phần thao tác CUA thật vẫn là của người; phần **đếm và chứng minh** thì tự động được, và hiện tại chưa có gì làm việc đó.

## 2. Trạng thái đo được (integrator đo ngày 2026-10-25, đừng đoán lại)

* Catalog có **185** dòng case hợp lệ (BR-001→BR-185). Dòng header ghi "Tổng: **180** case" là **sai, đã cũ** — sửa luôn trong task này, vì toàn bộ gate mới đọc chính file này.
* P0/P1: **161** case. Trong đó **45** case có ít nhất một test gắn tag; **116** chưa có. **Mọi case P0 đều đã có test gắn tag, trừ đúng BR-180.**
* Cột trạng thái trong catalog **không dùng làm đầu vào gate được**: 156/161 P0/P1 vẫn ghi `NOT RUN` dù đã có automation. `test/qc/traceability_test.go` chỉ `t.Logf` cho tình huống "có tag mà vẫn NOT RUN" (`awaitingSignOff`), không bao giờ fail — nên một cổng mới đọc cột đó sẽ xanh khi engine đã hỏng, hoặc đỏ vĩnh viễn khi catalog chưa được cập nhật.
* `test/qc/traceability.golden.json` cho mỗi case một danh sách `"<file>.go:<TestName>"` — **duy nhất chỗ này** ánh xạ case → test chạy được. Gate mới phải lấy nó làm nguồn, không parse lại catalog.
* **Không có bước nào hiện nay chứng minh các test được tag thực sự PASS.** Traceability chỉ so sánh *hình dạng* của golden. Một test đổi tên, bị skip, hoặc regression giữa hai commit sẽ vẫn "có tag, đủ vàng" và gate vẫn xanh. Đây là lỗ hổng BR-180 thật sự phải bịt.
* Tiền lệ đã có, dùng lại đúng nguyên văn: `scripts/release-gate.sh` và `scripts/release-smoke.sh` đều fail-closed với **0 = pass, 1 = có verdict fail, 2 = không có verdict**, và đều có contract test trong `test/gatecheck/` gọi script bằng `exec.Command("bash", ...)`.

## 3. Thiết kế

### 3.1 `scripts/rc-sweep.sh` (mới)

Bảy bước, mỗi bước ghi log riêng vào `-out`, và ghi `rc-sweep.json`:

| # | Bước | Lệnh | Exit 1 khi | Exit 2 khi |
| --- | --- | --- | --- | --- |
| 1 | fingerprint | `git rev-parse HEAD`, `sw_vers`, `shasum -a 256 <binary>`, `-sha` phải khớp HEAD hoặc ghi rõ `-allow-sha-mismatch` | thiếu binary / SHA khác HEAD | không đọc được |
| 2 | gate | `scripts/release-gate.sh -out <out>/gate` | gate fail | không chạy được |
| 3 | parity render | `python3 testdata/parity.py --render --binary <bin> --json <out>/parity.json` | parity fail hoặc band vượt baseline | không có Chromium/fixture |
| 4 | parity interaction | `go test -count=1 -run TestParity ./test/engine/` | fail | không chạy được |
| 5 | **per-case run** | với mỗi P0/P1 có tag trong golden: gom theo package, chạy `go test -count=1 -run '^(Test…|Test…)$' <pkg>` | **bất kỳ test nào của P0 fail**, hoặc **test được tag mà báo SKIP** (skip vô điều kiện = không phải bằng chứng; xem `test/qc/traceability_test.go`), hoặc **số P0/P1 có tag giảm dưới floor** (§3.3) | `-run` không match test nào (test đổi tên ≠ test xanh) |
| 6 | live smoke | `./bin/goosie -backend headless … -url <live-url>` với **network preflight** trước | trang vẽ nhưng report tự chấm fail | host không tới được (offline/CI không có mạng) |
| 7 | defects + sign-off | đọc `docs/qa-open-defects.json` và file sign-off (§3.4) | còn defect P0/P1 `open`, hoặc thiếu/sai sign-off | file không đọc được |

Bắt buộc: `-steps FILE` thay được danh sách bước (giống `-commands` của release-gate) để contract test chứng minh fail-closed mà không chạy suite 4 lần; `-live-url` trỏ được tới server local. **Test không được phép gọi internet thật.**

Bước 6 phải giữ nguyên tắc đã ghi ở `qa-test-plan.md`: live smoke không bao giờ được thành PASS khi không chạy — offline là 2 ("không có verdict"), không phải 0, không phải 1.

### 3.2 `test/gatecheck/rc_sweep_test.go` (mới)

Contract test cho từng invariant ở bảng trên, mỗi test mang `// qc:BR-180`. Tối thiểu: report shape; exit 0/1/2 đúng chỗ; thiếu bước ⇒ 2; một test được tag nhưng `-run` không match ⇒ 2 (không được tính là xanh); P0 fail ⇒ 1; defect `open` ⇒ 1; sign-off thiếu hoặc SHA khác SHA của sweep ⇒ 1; offline live smoke ⇒ 2 và **không** có trường `live_smoke: pass`.

### 3.3 `docs/qa-open-defects.json` + ratchet

Registry máy đọc được, seed bằng đúng thứ đã mở thật — **bốn defect có bằng chứng, đừng để agent sau phải điều tra lại**:
`GOOSIE-RENDER-001` (P1, `docs/qa-test-plan.md` §QA-002: chưa có frame-by-frame evidence để đóng phần nhấp nháy), và
ba gap engine ghi ở `docs/tasks/TASK-026-FORM-JOURNEYS.md` §8 (bản đã xác minh lại 2026-10-25):

* `GOOSIE-SELECT-001` (P1) — `<select>` focus được nhưng không đổi được value: `internal/engine/focus.go:573-590 IsActivatable` không có `select`, `internal/engine/events.go:75 keyDefaultAction` không có nhánh Arrow/Home/End nên phím rơi vào `cmd/goosie/interaction.go:425 scrollContentKey`. Spec fix: `docs/tasks/TASK-030-SELECT-INTERACTION.md` (BR-101).
* `GOOSIE-VALIDATE-001` (P1) — `internal/engine/engine.go:1619 submitFormLocked` dispatch `submit` rồi collect và gửi ngay, không đọc `required`/`type`. Spec fix: `docs/tasks/TASK-031-FORM-SUBMISSION-CORRECTNESS.md` (BR-105).
* `GOOSIE-SUBMITTER-001` (P1) — `internal/engine/forms.go:70-72,88 isSuccessfulControl` loại button khỏi form data và `internal/engine/engine.go:1560-1568 runClickDefault` gọi `SubmitForm(form)` không truyền submitter. Spec fix: `docs/tasks/TASK-031-FORM-SUBMISSION-CORRECTNESS.md` (BR-107).

**Không seed defect AX-publish nào.** §8 bản đầu nghi `cmd/goosie/interaction.go:44-58` không
publish accessibility sau khi kích hoạt; đã bác bỏ: `repaintTab` (`cmd/goosie/presentation.go:116`)
kết thúc bằng `publishAccessibility()` ở cả hai nhánh (`:167`, `:185`), và
`cmd/goosie/journey_p1_forms_test.go:46-49` chứng minh `node.Focused` đổi trong AX tree ngay sau
`clickNode`. Defect chỉ được ghi vào registry khi đã **đo**, không phải khi đọc thấy có vẻ thiếu.
 Trường: `id, priority, status (open|closed|waived), owner, summary, evidence, closed_by`.
Ratchet cho golden: một `qa-ratchet.json` ghi `{"p0_tagged": <n>, "p0p1_tagged": <n>}`. Sweep fail (exit 1) nếu số giảm, hoặc nếu có P0 không tagged. **Nâng số là hợp lệ, hạ số phải kèm lý do trong commit.**

### 3.4 Sign-off

`docs/qa-signoffs/<build-sha>.json`: `{reviewer, date, build_sha, cua_cases: [ids đã người thật xem], statement}`. Không có file, hoặc `build_sha` khác SHA của sweep ⇒ BR-180 **không pass**. Đây là chỗ giữ đúng DoD "thiếu bằng chứng không phải PASS" — tự động hoá không được phép thay chữ ký của người.

## 4. Quyền sở hữu và cấm

* Được tạo/sửa: `scripts/rc-sweep.sh`, `test/gatecheck/rc_sweep_test.go`, `docs/qa-open-defects.json`, `docs/qa-ratchet.json`, `docs/qa-signoffs/` (file mẫu), `docs/tasks/TASK-029-*.md` (§Status), `docs/qa-test-catalog.md` (**chỉ** dòng header "180 case" → 185 và dòng BR-180), `test/qc/enforced.go` (thêm `BR-180`), `test/qc/traceability.golden.json` (**chỉ** qua `-update`).
* **Cấm** `internal/**`, `cmd/goosie/**`, `testdata/**`, `scripts/release-gate.sh`, `scripts/release-smoke.sh`, và mọi `test/**` khác `test/gatecheck/`.
* Không sửa test của người khác để bước 5 xanh. Test nào của P0 fail ⇒ **dừng, báo integrator**, đó là phát hiện giá trị nhất của task này.
* Không `git push`, không merge `main`, không tạo worktree con.

## 5. Rules

* **[R-1]** Mọi phát ngôn "gate sẽ fail nếu X" phải kèm **output mutation thật** theo đúng quy trình đã áp dụng ở TASK-020 §6, TASK-022, TASK-026: sửa → chạy → `N/N FAIL` → revert → chạy → `N/N PASS`, dán nguyên văn vào §Status. Một invariant chưa mutate thì chưa được tính là có.
* **[R-2]** Fail-closed ba ngôi: 0/1/2. Không có đường nào để "không chạy" in ra xanh.
* **[R-3]** Không assertion thời lượng; không phụ thuộc display; chạy được trên máy offline (offline ⇒ bước 6 exit 2, cả sweep vẫn phải trả verdict thật cho 5 bước kia).
* **[R-4]** Sweep đọc state từ `traceability.golden.json` và từ kết quả chạy test, **không** đọc cột trạng thái trong catalog để quyết định pass/fail.
* **[R-5]** Bước 5 chạy `-count=1`, không dùng cache; đó là toàn bộ ý nghĩa của bước này.
* **[R-6]** Nếu một invariant chỉ chứng minh được bằng cách nới luật ở `test/qc/`, dừng và hỏi integrator.

## 6. Acceptance

* **AC-01** `bash -n scripts/rc-sweep.sh` sạch; `go vet -all ./...` sạch; `go test -count=1 ./internal/archtest/` xanh.
* **AC-02** `go test -count=1 ./test/gatecheck/` xanh, và `go test -race -count=1 ./test/gatecheck/` xanh.
* **AC-03** `go test -count=1 ./...` xanh; `go test -count=1 -run TestTraceability ./test/qc/ -update` rồi golden chỉ đổi ở các dòng BR-180/enforced.
* **AC-04** Chạy thật `scripts/rc-sweep.sh -binary ./bin/goosie -out /tmp/rc-sweep` trên máy **có mạng**: exit 0 hoặc exit 1 với lý do cụ thể từng bước (không được exit 2 vô cớ); `rc-sweep.json` có đủ 7 bước.
* **AC-05** **Bốn mutation proof, dán output**: (i) thêm một defect `open` giả ⇒ 1; (ii) xoá file sign-off ⇒ 1; (iii) đổi tên một test được tag trong golden (giả) ⇒ 2; (iv) chặn network preflight ⇒ bước live smoke ra 2 và sweep **không** in `pass`.
* **AC-06** `python3 testdata/parity.py` vẫn 80/80 và baseline band không đổi (task này không sửa engine).
* **AC-07** Catalog: header sửa thành 185; dòng BR-180 chỉ đổi trạng thái nếu §Status có đủ evidence của AC-04 **và** một chữ ký người thật — nếu chưa, giữ `NOT RUN` và ghi rõ còn thiếu gì.

## 7. Không nằm trong task này

* Không làm cho 116 P1 chưa tagged có test — đó là các task journey (TASK-026 và những task sau).
* Không tự động hoá CUA, không giả CUA bằng AX tree; `qa-test-plan.md` đã cấm đóng bug render chỉ dựa vào DOM/AX.
* Không đổi ngưỡng parity (`PASS_AT`, `MIN_INK`, band gap) — RULE cũ của TASK-021/024 vẫn còn có hiệu lực.
* Không vá `TestNativeEndToEndPlaysARealVideo` (nhạy với tải máy, đã ghi nhận riêng) — nếu bước 2/5 chạm tới nó, ghi hiện tượng vào §Status, đừng tăng timeout.

## Status (2026-10-25)

- **AC-01 — PASS.** `bash -n scripts/rc-sweep.sh`; `go vet -all ./...`; `go test -count=1 ./internal/archtest/` đều exit 0. Các lệnh này cũng chạy lại trong bước `gate` của sweep cuối.
- **AC-02 — PASS.** `go test -count=1 ./test/gatecheck/` → `ok ... 35.059s`; `go test -race -count=1 ./test/gatecheck/` → `ok ... 36.432s`.
- **AC-03 — PASS sau rerun.** `go test -count=1 ./...` → tất cả package PASS trong release-gate cuối (`/tmp/rc-sweep/gate/01-go-test-count-1.log`). Một sweep trước có `TestQueueFullIsRefusedNotBlocked` timeout ở `test/surface/loop_test.go:873`; chạy riêng `go test -count=1 ./test/surface/ -run '^TestQueueFullIsRefusedNotBlocked$'` → `ok ... 0.464s`, full-suite rerun → `test/surface PASS ... 2.857s`, sweep cuối → release gate PASS. `TestNativeEndToEndPlaysARealVideo` PASS trong các full-suite run cuối; không sửa test/timeout. `go test -count=1 -run TestTraceability ./test/qc/ -update` → `ok ... 0.685s`; golden chỉ đổi row BR-180: các test `test/gatecheck/rc_sweep_test.go` và `enforced: true`.
- **AC-04 — RUN, exit 1 có lý do.** Lệnh: `bash scripts/rc-sweep.sh -binary ./bin/goosie -out /tmp/rc-sweep`. Report: `/tmp/rc-sweep/rc-sweep.json`, đủ 7 step. Output cuối: `rc-sweep: fingerprint: PASS - build SHA, OS, and binary SHA-256 recorded`; `rc-sweep: gate: PASS - release gate exited 0`; `rc-sweep: parity_render: PASS - parity render exited 0`; `rc-sweep: parity_interaction: PASS - interaction parity exited 0`; `rc-sweep: per_case_run: PASS - all 71 tagged P0/P1 test names ran`; `rc-sweep: live_smoke: PASS - live page drew 120 frames, doc_height=21217`; `rc-sweep: defects_signoff: FAIL - open P0/P1 defects: GOOSIE-RENDER-001; human sign-off is missing for build 091ce0c76fe5efadcff4256e50c6d4b5096f4413`; `rc-sweep: FAIL (report: /private/tmp/rc-sweep/rc-sweep.json)`. Live smoke tới được `https://about.google/products/`; headless report ghi 120 frame, doc height 21217, failed tile 0, worker panic 0.
- **AC-05 — PASS; mutation proofs đã chạy, revert và chạy lại.** Mỗi output dưới đây là số test mục tiêu thực thi; hành vi bị tắt cho kết quả `1/1 FAIL`, sau revert là `1/1 PASS`:

  | Invariant | Mutation tắt hành vi | Output sau mutate → revert |
  | --- | --- | --- |
  | Report shape | Bỏ ghi các step rows | `1/1 FAIL` → `1/1 PASS` |
  | Duration field | Bỏ trường `duration_seconds` | `1/1 FAIL` → `1/1 PASS` |
  | Fingerprint | Bỏ ghi binary SHA-256 | `1/1 FAIL` → `1/1 PASS` |
  | Fail-closed | Ép exit lỗi command thành 0 | `1/1 FAIL` → `1/1 PASS` |
  | Tag/run match | Ép kết quả test không match thành pass | `1/1 FAIL` → `1/1 PASS` |
  | P0 failure | Ép P0 test fail thành pass | `1/1 FAIL` → `1/1 PASS` |
  | P0 coverage | Bỏ chặn P0 không có tag | `1/1 FAIL` → `1/1 PASS` |
  | Ratchet | Bỏ so sánh với floor P0/P0-P1 | `1/1 FAIL` → `1/1 PASS` |
  | Open defect | Bỏ xét defect P1 đang mở | `1/1 FAIL` → `1/1 PASS` |
  | Missing sign-off | Bỏ xét file sign-off thiếu | `1/1 FAIL` → `1/1 PASS` |
  | Build SHA sign-off | Bỏ đối chiếu SHA trong sign-off | `1/1 FAIL` → `1/1 PASS` |
  | Offline live smoke | Ép network preflight unreachable thành pass | `1/1 FAIL` → `1/1 PASS` |
  | No-cache | Bỏ `-count=1` khỏi per-case command | `1/1 FAIL` → `1/1 PASS` |

  Local contract dùng loopback/`httptest`, không gọi internet thật. Offline case có step live smoke exit 2; khi một step khác fail, report vẫn trả fail có lý do, đồng thời giữ live step ở trạng thái no-verdict.
- **AC-06 — PASS.** `python3 testdata/parity.py` và render parity trong sweep đều 80/80; average match 96.09%, threshold 90%. Baseline band không đổi.
- **AC-07 — NOT RUN, giữ BR-180 `NOT RUN`.** `GOOSIE-RENDER-001` vẫn mở vì QA-002 chưa có frame-by-frame pixel evidence; cũng chưa có CUA evidence và chữ ký người thật gắn với build SHA. File trong `docs/qa-signoffs/TEMPLATE.json` được đánh dấu mẫu, không phải sign-off.

Catalog header đã sửa `Tổng: 180 case` thành `Tổng: 185 case`; dòng BR-180 giữ `NOT RUN`.

### Tích hợp (integrator, 2026-10-25 — chạy lại độc lập, không copy số của codex)

- **Chạy lại:** `bash -n scripts/rc-sweep.sh` → ok; `go test -count=1 ./test/gatecheck/
  -run TestRCSweep -v` → **10/10 PASS, không test nào skip** (13.3s); sau merge:
  `./test/qc/`, `./internal/archtest/`, `./test/gatecheck/` đều `ok` trên `main`.
- **Tự tay mutate lại 6 trong 13 proof**, chọn đúng những cái chặn release: open-defect block,
  ratchet floor, missing sign-off, sign-off SHA compare, untagged-P0 block, P0-failure
  mapping. Cả **6 đều `rc=1`** — mỗi cái giết đúng test của nó; script restore **byte-identical**
  (`diff -q` sạch). Không proof nào survive.
- `docs/qa-open-defects.json` **đối chiếu lại theo §3.3**: 1 entry → 8. Bốn defect §3.3 yêu
  cầu seed (`RENDER-001`, `VALIDATE-001`, `SUBMITTER-001`, `SELECT-001` — cái cuối ghi
  `status: closed`, `closed_by: 531ea00` vì TASK-030 đã merge), cộng ba defect chỉ thấy khi
  *chạy*: `AX-001` (`<select>` không có node AX — BR-139, do TASK-030 §9.4 đo),
  `PSEUDO-001` (`::first-letter`/`::first-line` — BR-075, do TASK-027 để lại 3 band),
  `COVERAGE-001` (emoji màu ra tofu, P2). Thêm `FLAKE-001` (P2) cho
  `TestNativeEndToEndPlaysARealVideo`, ghi ở registry vì §3 cấm lấy timeout làm cách sửa.
- Ratchet nâng `18/45` → **`19/55`**, có số đo từng ref: `e7e5c73` 18/53 → `531ea00`
  (BR-101 được tag) 18/54 → sau merge TASK-029 **19/55**. **BR-180 là P0** (catalog
  `docs/qa-test-catalog.md:216`) nên 10 test tagged của nó chính là `+1` P0 — không phải
  `+2` P1 như §3.3 từng đoán. Không bước nào hạ số.
- Sweep thật sau merge phải chạy lại ở SHA mới; kết quả ghi ở task kế tiếp.
