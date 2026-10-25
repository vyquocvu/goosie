# TASK-033 — `::first-letter`: drop cap phải là một box thật (BR-075)

**Priority:** P1. **Method:** PARITY + INT. **Owner:** chưa gán.
**Blocked case:** BR-075 (`docs/qa-test-catalog.md:99` — "Render pseudo-elements
::before/::after và pseudo-state được hỗ trợ", hiện `NOT RUN`).
**Defect:** `GOOSIE-PSEUDO-001` trong `docs/qa-open-defects.json`.
**Phạm vi task này ONLY `::first-letter`.** `::first-line` để riêng (xem §6).

## 1. Bằng chứng đã đo trên `main` (`261ec84` lineage, sau merge TASK-029)

| Sự thật | Chỗ trong code |
| --- | --- |
| CSS tokenizer/selector **đã nhận** tên `first-line`/`first-letter` như pseudo-element | `internal/css/selector.go:467` (`case "before", "after", "first-line", "first-letter"`) |
| Style **không bao giờ** resolve hai tên đó: chỉ lặp `{before, after}` | `internal/style/pseudo.go:37` và `:82` |
| Layout chỉ tạo box cho hai kinds đó | `internal/layout/arena.go:482-483` (`Pseudo: "before"`), `:544-545` (`"after"`), `buildPseudoElement` ở `:557` |
| BoxKind enum không có chỗ cho first-letter | `internal/layout/boxkey.go:21-22` (`BoxKindPseudoBefore`, `BoxKindPseudoAfter`) |
| Engine cũng khoá theo đúng hai tên khi vẽ background pseudo | `internal/engine/engine.go:915`, `internal/engine/style_diff.go:180` |
| Hệ quả đo được: `testdata/render/parity` fixture `selectors/03-pseudo-elements.png` còn **3 band trống** `(457,475) (539,554) (579,599)`; `::before`/`::after` trong cùng fixture **vẽ bình thường** (rows 483..498, 517..532) | `docs/tasks/TASK-027-GLYPH-COVERAGE.md` §6; baseline `testdata/render-known-gaps.json` (`selectors/03`: `blank_regions: 3`) |

Kết luận: selector vào được nhưng **chết ở style + layout**. Drop cap `font-size: 48px;
float: left` bị bỏ hoàn toàn, nên nửa dưới fixture bị đẩy lên ~20px và parity bắt được lệch
đó thành band trống. Đây **không** phải bug glyph — đừng sửa `internal/raster`.

## 2. Thiết kế tối thiểu

1. `internal/style/pseudo.go`: thêm `"first-letter"` vào **cả hai** vòng lặp (`:37`, `:82`).
   Không đổi `computePseudoStyle` ngoài việc cho phép tên mới.
2. `internal/layout/boxkey.go`: thêm `BoxKindPseudoFirstLetter` ở cuối enum (giữ các số cũ —
   `BoxKey` 64-bit đã merge ở `9f4fb4c` mã hoá kind vào key, nên chèn giữa làm đổi mọi key
   đã có fixture).
3. `internal/layout/arena.go`: trong `buildSubtree`, khi node là block có inline content và
   tồn tại `pseudoStyles[{NodeID, "first-letter"}]`:
   * tách **ký tự in được đầu tiên** của text node đầu tiên (bỏ qua leading whitespace;
     `content:` rỗng ⇒ không tạo box, giống quy tắc hiện tại cho before/after ở `pseudo.go:37`
     comment),
   * tạo một anonymous inline box chứa đúng ký tự đó với computed style của pseudo, đặt **trước**
     run còn lại, rồi để inline/float pass hiện có xử lý `float: left` như mọi box float khác,
   * text node gốc **mất** ký tự đã tách (không vẽ hai lần — đây là lỗi dễ mắc nhất).
4. `internal/engine/engine.go:915` + `style_diff.go:180`: thêm tên mới vào danh sách
   background/interactive diff, để đổi style `::first-letter` vẫn trigger reflow đúng.
5. `internal/paint`: không cần case mới nếu box là inline thường; **nếu** buộc phải chạm
   `internal/paint`, ghi rõ ở §Status trước khi làm (quy tắc kế thừa từ TASK-027 §5).

## 3. Test phải có

* `test/style/pseudo_test.go` kiểu hiện có (`internal/style/pseudo_test.go:11`): selector
  `p::first-letter { font-size: 48px }` ⇒ map có key `{id, "first-letter"}`.
* `test/layout/` — assertion **cấu trúc**, không pixel:
  1. block có text "Hello" + `::first-letter` ⇒ đúng một box chứa "H", box còn lại chứa
     "ello"; tổng số glyph source không đổi (không nhân đôi).
  2. leading whitespace `" Hello"` ⇒ chữ H vẫn là first-letter.
  3. `float: left` trên `::first-letter` ⇒ các dòng sau **quanh** box float (kiểm bằng
     `Object` line boxes, không ảnh).
  4. `content: ""` ⇒ không box.
  5. text rỗng / block chỉ có element con ⇒ không box, không panic.
  6. incremental: đổi `font-size` của `::first-letter` ⇒ layout pass chạy lại; đổi property
     không ảnh hưởng hình học ⇒ không full-rebuild (bất biến TASK-022/BR-185 phải giữ).
* Parity (nghiệm thu thật): `python3 testdata/parity.py --render --binary ./bin/goosie` rồi
  `--inspect selectors/03-pseudo-elements.png --band 32`. **Yêu cầu**: band giảm so với 3, và
  `testdata/render-known-gaps.json` phải **thu nhỏ** entry `selectors/03`. Không được xoá
  entry để "qua gate" nếu band vẫn trống.
* Gắn `// qc:BR-075` cho test 1 và 3; `go test ./test/qc/ -update` rồi kiểm golden chỉ đổi
  row BR-075. Không tự sửa status trong `docs/qa-test-catalog.md`.

## 4. Nghiệm thu `[R-7]`

Proof là **tắt hành vi**; dán `FAIL → revert → PASS`, và disclose proof nào không giết được
test nào:

* (i) bỏ `"first-letter"` khỏi `pseudo.go:37` ⇒ test style + test 1 **FAIL**.
* (ii) tạo box first-letter nhưng **không** xoá ký tự khỏi text gốc ⇒ test 1 **FAIL**
    (assert tổng glyph source) — đây là mutation nguy hiểm nhất vì ảnh vẫn "có vẻ đúng".
* (iii) khỏi box first-letter khỏi float pass (force `display: inline`, bỏ float) ⇒ test 3
    **FAIL**.
* (iv) để whitespace dẫn đầu lọt qua (`first = 0` cứng) ⇒ test 2 **FAIL**.
* (v) bỏ tên mới khỏi `engine.go:915`/`style_diff.go:180` ⇒ test 6 **FAIL**; nếu không fail,
    ghi rõ là incremental path chưa bị oracle nào chặn.

## 5. Ràng buộc vận hành

Không `git push`, không merge `main`, không tạo worktree con. Test phải xanh trên máy **không
có display**. `go test ./internal/archtest/` (import table + gofmt + globals) và
`go vet -all ./...` phải xanh. **Cấm**: đổi `PASS_AT`/`MIN_INK`/`band_gap_rows` trong
`testdata/parity.py` hoặc `testdata/render-known-gaps.json` theo hướng nới baseline;
`render-known-gaps.json` chỉ được **giảm** số band.

## 6. Ngoài phạm vi (ghi rõ để người sau không đoán)

`::first-line` (cần restyle trong lúc break line — việc riêng, task riêng), `::selection`,
`::marker`, `initial-letter`/`first-letter` trên inline element, và `::first-letter` khi chữ
đầu là emoji cluster (engine chưa có grapheme splitting; `GOOSIE-COVERAGE-001` liên quan).
Nếu fixture `selectors/03` vẫn còn band sau task này, nguyên nhân còn lại gần chắc chắn là
`::first-line` — ghi tiếp vào `GOOSIE-PSEUDO-001`, đừng mở defect mới trùng.
