# TASK-026: Hành trình biểu mẫu thật (BR-097–BR-107) — khối P1 lớn nhất còn trống

- **Task ID**: `TASK-026`
- **Type**: Test quality / e2e journey coverage
- **Assigned to**: `agy`
- **Review**: `qoder cli` (integrator)
- **Base**: `main` sau commit `4c3cff5`

## 1. Vì sao chọn khối này

Tính từ `test/qc/traceability.golden.json` ở thời điểm viết task: chỉ còn **1 case P0** chưa có test tự động (BR-180 — sweep release candidate, việc riêng của integrator). Khối P1 chưa phủ lớn nhất là **E. JavaScript, sự kiện, biểu mẫu và thao tác trang: 30 case**. Trong đó 11 case liên tiếp **BR-097 → BR-107** đều là biểu mẫu và nhập liệu — đúng thứ mà QA manual dùng để trả lời câu hỏi "trình duyệt này dùng được chưa". Một browser mà gõ text sai, checkbox không toggle, select không đổi value, hoặc submit sai encoding thì không "dùng được", bất kể unit test xanh.

| Case | Ưu tiên | Methods | Nội dung catalog | Trạng thái automation |
| --- | --- | --- | --- | --- |
| BR-097 | P1 | CUA, INT | Focus text input, gõ, select-all, sửa → giá trị và caret đúng | chưa tag |
| BR-098 | P1 | CUA, INT | Nhập/paste textarea nhiều dòng → newline và thứ tự giữ nguyên | chưa tag |
| BR-099 | P1 | CUA, INT | Toggle checkbox bằng click và Space → `checked` và event cập nhật **một lần** | chưa tag |
| BR-100 | P1 | CUA, INT | Chọn một radio trong group → radio cũ bỏ chọn, chỉ một value được submit | chưa tag |
| BR-101 | P1 | CUA, INT | Chọn option trong select → value, focus và paint khớp lựa chọn | chưa tag |
| BR-102 | P1 | CUA, INT | Tab/Shift+Tab qua form → focus theo thứ tự DOM khả kiến | chưa tag |
| BR-103 | P1 | CUA, INT | Submit bằng Enter trong input → submit event/default action chạy **đúng một lần** | chưa tag |
| BR-104 | P1 | CUA, INT | Submit bằng nút → dữ liệu gửi đúng control name/value | chưa tag |
| BR-105 | P1 | INT | Submit form thiếu required/sai type → validation **ngăn** submit và **giữ** nội dung đã nhập | chưa tag |
| BR-106 | P1 | INT | Submit hợp lệ → URL/body/method/encoding theo thuộc tính form được hỗ trợ | chưa tag |
| BR-107 | P1 | INT | Button submit phụ có name/value riêng → **chỉ** submitter được kích hoạt mới góp vào dữ liệu | chưa tag |

**Đã có test chưa tag, phải audit trước khi viết mới** (`cmd/goosie/form_nav_test.go`): `TestFormNavigationOnSubmitClick` (:18), `TestFormNavigationOnEnterKey` (:93), `TestCheckboxClickToggle` (:231). Với mỗi test, quyết định một trong hai: (a) nó thực sự assert điều catalog đòi → thêm `// qc:BR-xxx` và **chứng minh bằng mutation** rằng nó nhạy; hoặc (b) nó không đủ → viết journey mới, ghi rõ trong Status vì sao test cũ không đạt. Đừng tag chỉ để golden đẹp lên.

## 2. Quyền sở hữu file (BẮT BUỘC TU THỦ)

Được sửa:

* `cmd/goosie/journey_p1_forms_test.go` — **file mới**, nơi mọi journey mới nằm.
* `cmd/goosie/form_nav_test.go` — chỉ để **thêm dòng tag `// qc:BR-xxx`** lên test đã có; không đổi logic test đó.
* `docs/tasks/TASK-026-FORM-JOURNEYS.md` — viết §Status.
* `test/qc/traceability.golden.json` — chỉ bằng lệnh `-update` ở AC-06.

Cấm sửa: `internal/**`, `test/**`, `testdata/**`, `docs/qa-test-catalog.md`, và **mọi file test khác trong `cmd/goosie/`** — `journey_p1_nav_test.go` và `journey_p1_tabs_test.go` vừa merge, `fuzz_input_test.go` và `test/style/shadow_test.go` đang do agent khác sở hữu trong lúc bạn chạy. Ghi đúng file của mình; ai khác cần gì thì ghi chú vào Status cho integrator.

Không tạo worktree con, không branch phụ, **không `git push`**, không merge `main`. Test phải chạy trên máy **không có display** (hợp đồng của repo).

## 3. Hai quy tắc sống còn, rút từ lỗi thật đã xảy ra trong tuần này

### 3.1. Điều kiện chờ phải bao gồm đúng thứ dòng kế tiếp đọc

`waitFor` chỉ chốt `tabMgr.Count() == 2` rồi dòng sau đọc `Active().ID()` đã làm đỏ cả `main` hai lần (xem `docs/tasks/TASK-015-P0-JOURNEY-SUITE.md` §8 và §9). Lần thứ hai tinh vi hơn: điều kiện chờ chấp nhận `axContainsLabel(tree, deadAddr)`, mà `deadAddr` là cái URL test vừa tự gõ vào thanh địa chỉ — nó **đã có mặt trong tree trước khi trang lỗi tồn tại**, nên test đọc `Title()` khi title chưa kịp đổi, fail 6/30 lượt.

Áp dụng vào form: sau khi click một control, `Focused()` đổi và `FocusedText()` đổi là **hai bước rời nhau** trong loop. Mọi assert trên `FocusedText()`/`SelectionText()`/`Value` của node AX phải nằm **trong** điều kiện chờ, không phải sau nó.

### 3.2. Không được nộp oracle chưa chứng minh

`docs/tasks/TASK-020-P1-JOURNEYS.md` §6 ghi lại: một agent khác tuyên bố 12 câu "test này sẽ FAIL nếu X hỏng" mà không chạy mutation thật. Integrator thử lại và tìm ra `TestJourneyRefreshWithPendingResponse` **vẫn PASS 6/6 sau khi xoá cả cơ chế chống stale response** — vì nó assert một điều phủ định ("resp1 không xuất hiện") vào lúc resp1 còn chưa kịp tồn tại.

Suy ra, cho chính task của bạn:

* **[R-1]** Mỗi case trong 11 case phải có một dòng trong Status: **phép mutation cụ thể → "N/N FAIL" → revert → "N/N PASS"**, kèm output nguyên văn. Con số, không phải cảm giác.
* **[R-2]** Với assert phủ định — BR-099 "event chỉ fire một lần", BR-103 "submit đúng một lần", BR-105 "validation **ngăn** submit", BR-107 "**chỉ** submitter được kích hoạt góp data" — bắt buộc chứng minh **điều bị cấm đã có cơ hội xảy ra** rồi mới kết luận nó không xảy ra. Cách làm: đếm số lần server nhận được request (server của bạn tự ghi log), hoặc đếm số lần handler chạy; assert **bằng con số** (`== 1`), không assert "không thấy".
* **[R-3]** Mutation phải **thực sự đổi hành vi**. Nếu bạn làm một phép biến đổi mà program không đổi (ví dụ xoá một dòng code đã dư), kết quả "test vẫn xanh" không có nghĩa oracle hỏng — phép mutation đó vô giá trị và bạn phải tìm phép khác, ghi rõ phép nào đã bị loại và vì sao.
* **[R-4]** Một assert không chứng minh được thì **xoá nó**, đừng giữ làm cảnh.

## 4. Yêu cầu kỹ thuật

### 4.1. Tái dùng harness, không viết lại

`cmd/goosie/e2e_test.go` đã có: `newBrowserE2E`, `close`, `inject`, `pressKey`, `click`, `clickNode`, `clickToolbarButton`, `navigateViaAddressBar`, `waitFor`, `diagnostics`, `activeURL`, `findAXNode`, `axContainsLabel`. API phía engine để đọc trạng thái: `internal/engine/focus.go:220 Focused()`, `:461 FocusedText()`, `:471 FocusedEditable()`, `engine.go:1849 SelectionBounds()`, `selection.go:252 SelectionText()`; xử lý biểu mẫu ở `internal/engine/forms.go`. **Không thêm API mới vào `internal/**`.** Nếu thiếu đường đọc để assert đúng điều catalog, ghi vào Status và dừng case đó — integrator sẽ quyết.

### 4.2. Bất biến bắt buộc cho cả 11 case

1. **Trạng thái phải đọc được từ phía người dùng**, không từ field nội bộ: `FocusedText()`/`SelectionText()`/`ax.Node.Value`/`Checked`, hoặc **yêu cầu HTTP mà test server thật sự nhận**. Ưu tiên nguồn cuối (request đến server) cho mọi case submit.
2. **Không có tác dụng phụ ngầm**: sau mỗi hành trình, gọi `sess.PaintChecked`/`browser` tương đương qua chính frame path của e2e và assert không error — đây là định nghĩa "browser còn vẽ được sau khi bị thao tác".
3. **Không phụ thuộc thời gian**: cấm `time.Sleep` để "chờ cho ổn định"; mọi chờ đi qua `waitFor` với điều kiện có thể **mạn** được.
4. **Không nới giới hạn admission**, không sửa hằng số, không tăng timeout của harness.

### 4.3. Gợi ý hình dạng test cho từng case (không bắt buộc, nhưng phải phủ đúng clause trong catalog)

* BR-097: focus input → gõ chuỗi → Cmd+A → gõ đè → assert `FocusedText()` ở **từng** bước, và caret/selection qua `SelectionText()`.
* BR-098: textarea nhiều dòng → assert newline `\n` còn nguyên và **thứ tự** ký tự, không chỉ độ dài.
* BR-099: click checkbox → `checked` đổi và `change` fire **đúng 1**; Space cùng kỳ vọng; click hai lần trở về trạng thái cũ với **2** event.
* BR-100: ba radio một group → chọn radio 2 ⇒ radio 1 bỏ chọn; submit và server nhận **đúng một** cặp `name=value`.
* BR-101: select đổi option ⇒ value + AX node phản ánh option được chọn (không chỉ value nội bộ).
* BR-102: Tab tới control kế tiếp theo **thứ tự DOM khả kiến**, Shift+Tab quay lại; assert bằng `Focused()` identity, không bằng đếm.
* BR-103: Enter trong input ⇒ submit chạy **đúng một lần** (đếm request phía server).
* BR-104: bấm nút submit ⇒ dữ liệu gửi đúng từng control name/value, so khớp **cả tập** cặp, không phải "có chứa".
* BR-105: form có `required` thiếu giá trị ⇒ server **không nhận** request nào, và nội dung đã nhập vẫn còn nguyên trong control.
* BR-106: `method=get` → query đúng encoding; `method=post` → body đúng `application/x-www-form-urlencoded` (hoặc `multipart/form-data` nếu engine hỗ trợ — nếu không hỗ trợ, ghi Status, đừng giả vờ pass).
* BR-107: hai nút submit, mỗi nút có `name`/`value` riêng ⇒ bấm nút A thì data có cặp của A **và không có** cặp của B.

## 5. Tiêu chí nghiệm thu

* **[AC-01]** `go test -count=5 -run TestJourney ./cmd/goosie/` PASS. `-count=5` là bắt buộc: một điều kiện chờ thiếu thì **trượt qua lần đầu tiên** và chỉ lộ ra ở lượt sau (bài học §3.1).
* **[AC-02]** `go test -race -count=2 -run TestJourney ./cmd/goosie/` PASS.
* **[AC-03]** `go test -count=1 ./...` PASS toàn suite trên máy **không display**.
* **[AC-04]** `go build ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test ./test/qc/` PASS; `gofmt -l cmd/goosie` sạch.
* **[AC-05]** Bảng per-case trong Status theo [R-1]: mutation → số lần FAIL → số lần PASS sau revert, output nguyên văn. Thiếu dòng nào coi như case đó chưa xong.
* **[AC-06]** `go test -count=1 -run TestTraceability ./test/qc/ -update` rồi commit golden; xác nhận 11 case BR-097…BR-107 có `tests` không rỗng.
* **[AC-07]** Nếu phát hiện bug engine cản hành trình: ghi reproducer tối giản (HTML + thao tác + kỳ vọng + actual) vào Status, **dừng ở đó**, không vá engine, không sửa test cho qua.

## 6. Nộp bài

Commit trên branch của bạn với message theo quy ước repo (`git log --oneline -8`). §Status ghi: bảng audit đầu vào (case → test cũ đã có → hành động), bảng [R-1] cho 11 case, danh sách assert đã **bỏ** vì không chứng minh được, danh sách mutation đã thử và **bị loại là vô giá trị** kèm lý do, và gap còn lại.
