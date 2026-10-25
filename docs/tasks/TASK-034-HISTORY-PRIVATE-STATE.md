# TASK-034 — History, session restore và private mode phải có oracle (BR-043/044/045/049/054/055/056)

**Priority:** P1. **Method:** INT. **Owner:** chưa gán.
**Blocked cases:** 7 case P1 trong `docs/qa-test-catalog.md`, tất cả đều `NOT RUN` và
**không có test nào được tag** (đo từ `test/qc/traceability.golden.json` tại `960b722`:
BR-043, BR-044, BR-045, BR-049, BR-054, BR-055, BR-056 — `tests: 0` cho cả 7).
**Defect liên quan:** `GOOSIE-TRACE-001` (104/161 case P0-P1 chưa có tag).

## 0. Vì sao chọn 7 case này trước

Đây là nhóm "trình duyệt có dùng được thật không": ghi history theo policy nào, Back rồi đi
URL mới thì nhánh forward ra sao, reload có nhân bản visit không, khởi động lại có dựng lại
tab không, và private mode có thực sự không để lại dấu vết. Tất cả **hành vi đã tồn tại trong
code** (bảng §1), chỉ thiếu oracle — nên chi phí là test, không phải product work. Nếu trong
lúc viết test phát hiện hành vi sai, **dừng lại và báo cáo**, không sửa product trong task này.

## 1. Hành vi đã đo trên `main` (`960b722`) — đây là thứ phải bị khoá bằng test

| Hành vi | Chỗ trong code | Rule thực tế (đọc từ source, không đoán) |
| --- | --- | --- |
| Nhánh forward bị cắt khi Push giữa trail | `internal/toolbar/history.go:18-26` | `if h.index >= 0 && h.index < len(h.entries)-1 { h.entries = h.entries[:h.index+1] }` rồi append; `index = len-1` |
| Commit traversal chỉ khi load thành công | `internal/toolbar/history.go:61-76` (`Step`), `cmd/goosie/navigation.go:435-456` | `Step` từ chối nếu `h.index != base` (cursor đã bị ai đó dịch); `traverseTab` **peek** trước, chỉ commit trong `applyNavResult` |
| Redirect của một traversal ghi lại URL cuối | `internal/toolbar/history.go:82-89` (`ReplaceCurrent`) | chỉ thay phần tử dưới cursor, "The forward trail is untouched" |
| Visit log chống trùng liên tiếp và có trần | `internal/history/history.go:57-68` | `Record` return sớm nếu phần tử cuối **cùng URL**; cắt về `MaxEntries` |
| Private: cookie không ghi file, history/bookmark không mở store | `cmd/goosie/main.go:202-220` | `private` ⇒ `net.DefaultPrivateClient()`; `bookmarkStore`/`historyStore` chỉ Load khi `!c.private` |
| Private: không restore session | `cmd/goosie/main.go:495-497` | `shouldRestoreSession(private, mode) = !private && mode != StartupNewTab` |
| Private: profile management read-only | `cmd/goosie/internal_pages.go:775,809` | "Private sessions cannot change profile settings." |
| Tab trail được lưu/restore | `internal/session/session.go:16` (`TabState` "where it was, and the back/forward trail"), `:34` (ghi atomic) | `RestoreHistory(entries, index)` ở `internal/toolbar/history.go:153` |

## 2. Test phải có — mỗi case một oracle, không gộp

Đặt ở `test/tabs/`, `test/history/`, `test/session/` (đã có `test/history/history_test.go`,
`test/session/{restore,session,scroll}_test.go`) và `cmd/goosie/` cho phần cần app thật. Tái
dụng helper đã có trong các package đó; **không** sinh helper song song.

1. **BR-043** — điều hướng A→B→C: visit log có đúng 3 mục theo đúng thứ tự, URL và title khớp
   những gì đã hiển thị; không có bản ghi cho URL chưa load thành công (kiểm bằng server trả
   500/ERR: `Record` không được chạy cho failure đã commit ở `navigation.go:312`).
2. **BR-044** — A→B→C, Back về B, rồi đi D: `Entries()` phải là `[A,B,D]` với index=2, và
   `CanForward()` **false**. Assert cấu trúc, không pixel.
3. **BR-045** — reload trang (và reload sau khi Back): số visit không đổi; Push/Record lặp
   cùng URL liên tiếp phải là no-op theo `history.go:60-62`.
4. **BR-049** — mở 2 tab với trail khác nhau → save → `RestoreHistory` → mỗi tab có đúng
   `entries` + `index` của nó, con trỏ trỏ đúng trang đang xem; session file ghi atomic
   (`session.go:34`) nên test ghi rồi đọc lại phải thấy dữ liệu nguyên vẹn.
5. **BR-054** — chạy với `private=true`: cookie jar là `DefaultPrivateClient`, **không** có
   file cookie nào được ghi, `historyStore`/`bookmarkStore` không được chạm tới (assert bằng
   file trên disk không đổi/mất tồn tại, không assert bằng log).
6. **BR-055** — sequence: thường (set cookie X) → private (set cookie Y) → thường: profile
   vẫn chỉ thấy X; private không thấy X; sau khi thoát private, X vẫn còn và Y không bao giờ
   xuất hiện trong file cookie của profile.
7. **BR-056** — private rồi khởi động lại: `shouldRestoreSession` trả false, và danh sách tab
   restore **không** chứa URL đã mở trong phiên private.

## 3. Nghiệm thu `[R-7]`

Mỗi proof là **tắt hành vi**, không phải trả giá trị sai; dán nguyên văn `FAIL → revert → PASS`
và bảng "mutation → test mục tiêu → rc". Ít nhất:

* (i) bỏ khối cắt nhánh trong `history.go:21-23` ⇒ BR-044 **FAIL** (forward branch còn dư).
* (ii) cho `Step` bỏ kiểm tra `h.index != base` ⇒ test BR-044/BR-049 **FAIL** (commit đè lên
  cursor đã dịch) — nếu không fail thì nói rõ, đừng im.
* (iii) bỏ điều kiện dedupe trong `history.go:57-62` (`return` sớm) ⇒ BR-045 **FAIL**.
* (iv) gọi `Record` cho cả navigation thất bại (bỏ cổng thành công ở `navigation.go:312`) ⇒
  BR-043 **FAIL**.
* (v) `shouldRestoreSession = !private || mode != NewTab` (đổi `&&` thành `||`) ⇒ BR-056
  **FAIL**.
* (vi) cho private dùng `DefaultClientWithCookies(cookieFile)` (bỏ nhánh `private` ở
  `main.go:202-204`) ⇒ BR-054 **FAIL**.

Proof nào không giết được test nào thì **disclose** trong §Status kèm lý do, theo đúng tinh thần
`docs/superpowers` R-7 mà TASK-031 đã áp dụng.

## 4. Ràng buộc vận hành

* Không `git push`, không merge `main`, không tạo worktree con. Branch từ **HEAD của `main`**
  tại thời điểm nhận prompt.
* Không sửa `docs/qa-test-catalog.md` (status do người chạy QC ghi). `docs/qa-open-defects.json`
  do integrator sửa: chỉ **đề xuất** dòng mới trong báo cáo cuối theo schema
  `id, priority, status, owner, summary, evidence, closed_by`.
* `go test -count=1 ./test/qc/ -update` chỉ được làm đổi **đúng 7 row** BR-043/044/045/049/054/055/056;
  nếu row khác đổi, giải thích được hoặc dừng.
* Verification dán output nguyên văn: `go test -count=1 ./...` full,
  `go test -count=1 -race ./test/tabs/ ./test/history/ ./test/session/ ./cmd/goosie/`,
  `go test -count=1 ./internal/archtest/`, `go test -count=1 ./test/qc/`, `go vet -all ./...`.
* **Trước khi chạy gate footprint hoặc suite full, kiểm `uptime`**: nếu load 1-minute > 30 thì
  chờ. Gate at-rest rất nhạy với tải (xem `GOOSIE-FLAKE-001`: retained=257 MiB chỉ là đỏ giả).
  Không tạo tải giả bằng `while :`; nếu cần stress, `trap 'kill $(jobs -p)' EXIT`.
* Test phải xanh trên máy **không có display** (`platform/headless`). Không assert thời gian
  thật; không thêm duration assertion vào gate suite.

## 5. Ngoài phạm vi

Bookmark CRUD (BR-046/BR-048) để batch sau — cùng họ state nhưng cần store fixture riêng.
`-clear-data` (BR-053) cần quyết định policy về phạm vi xóa trước khi viết oracle. Cookie
isolation ở tầng `internal/net` (jar scope) đã có test riêng; task này khoá ở tầng app.
Không đụng `testdata/render-known-gaps.json`, `PASS_AT`, `MIN_INK`, `gateFootprintSlack`.
