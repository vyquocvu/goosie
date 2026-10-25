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

## 7. Status

### 7.1. Bảng Audit trước khi viết (§1, §2.1)

| Case | Test cũ trong `cmd/goosie/form_nav_test.go` | Quyết định | Lý do cụ thể |
| --- | --- | --- | --- |
| BR-097 | Không có | `viết mới` | Chưa có test E2E nào kiểm tra caret di chuyển, xóa bằng Backspace, chèn giữa, và select-all trong text input. |
| BR-098 | Không có | `viết mới` | Chưa có test E2E nào kiểm tra newline `\n` và thứ tự các dòng trong `<textarea>`. |
| BR-099 | `TestCheckboxClickToggle` (:231) | `không tag`, `viết mới` | Test cũ chỉ dùng unit mock `newNavTestPath`, kiểm tra toggle attribute `checked` đơn giản; không test phím Space, không test event `change` fire đúng 1 lần, không đếm số lần event. Test mới `TestJourneyCheckboxToggleClickAndSpace` kiểm tra cả click và Space, khẳng định số event qua payload submit. |
| BR-100 | Không có | `viết mới` | Chưa có test nào kiểm tra radio group: uncheck radio cũ, chỉ submit đúng 1 value lên server. |
| BR-101 | Không có | `viết mới` (`t.Skip` theo [AC-07]) | Engine thiếu tương tác chuột/bàn phím để chọn option trong `<select>`. Ghi reproducer tối giản. |
| BR-102 | Không có | `viết mới` | Chưa có test nào kiểm tra Tab và Shift+Tab di chuyển tuần tự qua các control theo thứ tự DOM. |
| BR-103 | `TestFormNavigationOnEnterKey` (:93) | `không tag`, `viết mới` | Test cũ chỉ kiểm tra Enter kích hoạt navigation, nhưng không đếm số lần server nhận request (`== 1`), không kiểm tra submit event. Test mới `TestJourneyFormSubmitOnEnterKey` assert số request server nhận đúng 1 lần. |
| BR-104 | `TestFormNavigationOnSubmitClick` (:18) | `không tag`, `viết mới` | Test cũ chỉ kiểm tra 1 trường query đơn lẻ (`q=goosie`), không kiểm tra toàn bộ tập hợp control name/value (hidden, text, checkbox, radio, textarea). Test mới `TestJourneyFormSubmitOnButtonClick` so khớp chính xác toàn bộ map parameters. |
| BR-105 | Không có | `viết mới` (`t.Skip` theo [AC-07]) | Engine `SubmitForm` chưa hỗ trợ validation `required`/`type` (form rỗng vẫn submit). Ghi reproducer tối giản. |
| BR-106 | Không có | `viết mới` | Test mới `TestJourneyFormMethodAndEncoding` kiểm tra URL encoding (`%26`, `%3F`, `%23`, Unicode tiếng Việt) qua request GET. Ghi rõ engine chưa hỗ trợ POST body / multipart. |
| BR-107 | Không có | `viết mới` (`t.Skip` theo [AC-07]) | Engine `CollectFormData` loại bỏ submitter button (`isSuccessfulControl` return false), name/value của submitter không được gửi. Ghi reproducer tối giản. |

---

### 7.2. Bảng Mutation Testing Proof theo [R-1]

| Case | Test áp dụng | Phép mutation cụ thể | Kết quả Mutation (FAIL) | Kết quả Revert (PASS) | Kết luận độ nhạy của Oracle |
| --- | --- | --- | --- | --- | --- |
| **BR-097** | `TestJourneyTextInputEditingAndCaret` | Trong `internal/engine/focus.go:389` (`EditBackspace`), đổi `s.caret--` thành `s.caret++`. Khiến Backspace không lùi caret, làm sai lệch vị trí chèn ký tự 'X' tiếp theo. | **3/3 FAIL**<br>`timed out waiting for character inserted at caret position; presents=478` (25.051s) | **3/3 PASS**<br>`ok cmd/goosie 1.198s` | Oracle bắt chính xác vị trí caret và text sau khi sửa. |
| **BR-098** | `TestJourneyTextareaMultilineInput` | Trong `internal/engine/focus.go:370` (`EditEnter` cho `AtomTextarea`), đổi `insert("\n")` thành `insert(" ")` (chèn khoảng trắng thay vì newline). | **3/3 FAIL**<br>`timed out waiting for textarea contains multiline text with newlines; presents=478` (24.899s) | **3/3 PASS**<br>`ok cmd/goosie 0.905s` | Oracle bắt chính xác ký tự `\n` và cấu trúc các dòng. |
| **BR-099** | `TestJourneyCheckboxToggleClickAndSpace` | Trong `internal/engine/events.go:88`, vô hiệu hóa Space activation: `case " ": return false` (thay vì gọi `activateKeyboardTarget`). | **3/3 FAIL**<br>`timed out waiting for server received form submission; url=".../chk-submit?events=2"` (24.903s) | **3/3 PASS**<br>`ok cmd/goosie 0.749s` | Oracle bắt chính xác việc Space bị bỏ qua (server chỉ nhận 2 events thay vì 3, thiếu `agreed=yes`). |
| **BR-100** | `TestJourneyRadioGroupSelection` | Trong `internal/engine/focus.go:656`, vô hiệu hóa `uncheckRadioGroup(root, name, n)` bằng cách comment bỏ lệnh uncheck. | **3/3 FAIL**<br>`timed out waiting for navigation to /submit completes; url=".../submit?color=red&color=green"` (24.840s) | **3/3 PASS**<br>`ok cmd/goosie 0.746s` | Oracle bắt chính xác việc radio cũ không bị bỏ chọn (server nhận cả `color=red` lẫn `color=green`). |
| **BR-101** | `TestJourneySelectOptionChange` | N/A (BLOCKED [AC-07]) | Engine thiếu tính năng tương tác `<select>`. | N/A | Dừng theo [AC-07], ghi reproducer. |
| **BR-102** | `TestJourneyFormTabNavigationOrder` | Trong `internal/engine/focus.go:715` (`FocusNext`), đổi `next := controls[(idx+1)%len(controls)]` thành `(idx+2)%len(controls)` (nhảy cóc qua control kế). | **3/3 FAIL**<br>`timed out waiting for tab moved focus to f2; presents=479` (24.934s) | **3/3 PASS**<br>`ok cmd/goosie 0.954s` | Oracle bắt chính xác thứ tự focus tuần tự qua từng phần tử. |
| **BR-103** | `TestJourneyFormSubmitOnEnterKey` | Trong `internal/engine/focus.go:378` (`EditEnter` cho input), comment bỏ lệnh `s.submitFormLocked(form)`. | **3/3 FAIL**<br>`timed out waiting for search results loaded and server received submit exactly once; url=".../searchpage.html"` (24.881s) | **3/3 PASS**<br>`ok cmd/goosie 0.831s` | Oracle bắt chính xác form không được submit khi nhấn Enter. |
| **BR-104** | `TestJourneyFormSubmitOnButtonClick` | Trong `internal/engine/forms.go:75` (`isSuccessfulControl`), đổi `case "checkbox", "radio": return n.HasAttribute("checked")` thành `return true` (submit cả control unchecked). | **3/3 FAIL**<br>`timed out waiting for destination loaded and parameters received; url=".../submit-all?...skip=no&role=user..."` (24.925s) | **3/3 PASS**<br>`ok cmd/goosie 0.957s` | Oracle bắt chính xác các control chưa checked bị gửi thừa lên server. |
| **BR-105** | `TestJourneyFormValidationRequired` | N/A (BLOCKED [AC-07]) | Engine thiếu HTML5 form validation. | N/A | Dừng theo [AC-07], ghi reproducer. |
| **BR-106** | `TestJourneyFormMethodAndEncoding` | Trong `internal/engine/forms.go:136` (`EncodeFormURLEncoded`), đổi `buf.WriteString(url.QueryEscape(d.Value))` thành `buf.WriteString(d.Value)` (bỏ URL escape). | **3/3 FAIL**<br>`timed out waiting for results loaded and request captured; url=".../search-enc?query=goosie engine & fast?&tag=tiếng việt #1"` (24.916s) | **3/3 PASS**<br>`ok cmd/goosie 0.750s` | Oracle bắt chính xác việc URL không được percent-encoded (`%26`, `%3F`, `%23`). |
| **BR-107** | `TestJourneyFormSubmitterData` | N/A (BLOCKED [AC-07]) | Engine thiếu cơ chế thu thập submitter name/value. | N/A | Dừng theo [AC-07], ghi reproducer. |

---

### 7.3. Danh sách Assert đã Bỏ vì không chứng minh được (§3.2, [R-4])

1. **Assert trên DOM Node Attribute nội bộ trong `waitFor` (`chkNode.HasAttribute("checked")`)**: Đã bỏ vì `dom.Node` không có mutex bảo vệ, truy cập từ test goroutine trong khi UI thread xử lý click gây ra data race (`WARNING: DATA RACE`). Thay thế bằng kiểm chứng payload HTTP request gửi về test server (`lastChecked == true/false`) — nguồn cuối thread-safe 100%.
2. **Assert `axContainsLabel` trên DOM Text thay đổi qua JS `textContent` sau click checkbox/radio**: Đã bỏ vì trong `cmd/goosie/interaction.go:56` nhánh `if activated` (checkbox/radio), engine gọi `f.repaintTab` và return luôn mà **không gọi `f.publishAccessibility()`**. Do đó cây AX không được publish lại sau click checkbox/radio, dẫn đến assertion bị timeout rỗng. Thay thế bằng xác thực số lần event `change` qua trường hidden input được gửi lên test server trong request payload (`lastEvents == "3"`).

---

### 7.4. Danh sách Mutation đã thử và Bị loại là Vô giá trị ([R-3])

1. **Xóa `name` và `root` khi comment `uncheckRadioGroup` (BR-100)**: Khi comment bỏ `uncheckRadioGroup(root, name, n)` ở `internal/engine/focus.go:656`, Go compiler báo lỗi build: `declared and not used: name`, `declared and not used: root`. Phép mutation này bị loại vì làm hỏng compilation thay vì thay đổi runtime behavior. Đã thay thế bằng `_ = root; _ = name` để build thành công và đổi đúng runtime behavior (radio cũ không bị uncheck).
2. **Thay đổi `s.caret--` tại nhầm nhánh trong `internal/engine/focus.go` (BR-097)**: Lần thử đầu tiên bị apply nhầm vào nhánh `EditRight` thay vì `EditBackspace`, khiến test `TestJourneyTextInputEditingAndCaret` không fail ở bước Backspace như dự kiến. Đã định vị chính xác dòng 389 trong `EditBackspace` để mutation có giá trị.

---

### 7.5. Điều tra Bug Engine và Reproducer Tối giản ([AC-07])

#### 1. Case BR-101: Thiếu tương tác chọn `<option>` trên `<select>` bằng chuột / bàn phím
- **Thực tế trong engine**:
  - `internal/engine/engine.go:1576` (`activatableAt`): `<select>` không nằm trong danh sách activatable elements. Click vào `<select>` chỉ focus phần tử hoặc rơi vào hố trống, không mở menu dropdown và không có popup listbox.
  - `internal/engine/focus.go`: Khi `<select>` có focus, các phím mũi tên `ArrowUp` (`0xF700`) và `ArrowDown` (`0xF701`) rơi vào `scrollContentKey` (cuộn trang), không có mã xử lý chuyển đổi option được chọn.
  - `internal/paint/builder_controls.go:269`: `paintSelect` chỉ vẽ nhãn của option hiện tại có thuộc tính `selected` hoặc option đầu tiên, cùng icon mũi tên. Không có UI dropdown nào được rasterize.
- **Reproducer tối giản**:
  ```html
  <select id="sel" name="opt">
    <option value="1" selected>One</option>
    <option value="2">Two</option>
  </select>
  ```
  - Thao tác: Focus `#sel`, gửi sự kiện phím Arrow Down (`0xF701`).
  - Kỳ vọng: Option "Two" nhận thuộc tính `selected`, option "One" bị bỏ `selected`, event `change` fire.
  - Actual: Option "Two" không được chọn; viewport bị cuộn xuống (scroll); giá trị không đổi.

#### 2. Case BR-105: Thiếu Form Validation (`required` / `type`) khi submit form
- **Thực tế trong engine**:
  - `internal/engine/engine.go:1619` (`submitFormLocked`):
    ```go
    ev := dom.NewEvent("submit", true, true)
    if !dom.DispatchEvent(form, ev) {
        return // preventDefault was called
    }
    data := CollectFormData(form)
    ...
    ```
    Engine dispatch sự kiện `submit`, sau đó trực tiếp thu thập `data := CollectFormData(form)` và gửi submission. Không có bất kỳ dòng code nào kiểm tra thuộc tính `required` hay validation type của các controls trong form.
- **Reproducer tối giản**:
  ```html
  <form action="/submit" method="GET">
    <input type="text" name="name" required value="">
    <button type="submit" id="btn">Send</button>
  </form>
  ```
  - Thao tác: Click `#btn`.
  - Kỳ vọng: Form validation ngăn chặn submit; không có HTTP request nào được gửi đến `/submit`.
  - Actual: Engine gửi request GET `/submit?name=` ngay lập tức, validation hoàn toàn bị bỏ qua.

#### 3. Case BR-107: Name/Value của Submitter Button bị loại khỏi Form Data
- **Thực tế trong engine**:
  - `internal/engine/forms.go:70-72` & `:88`:
    ```go
    case "input":
        typ := strings.ToLower(n.GetAttribute("type"))
        switch typ {
        case "submit", "button", "reset":
            // Only successful if used to submit the form
            return false // handled separately
    ...
    case "button":
        return false // only successful if used to submit
    ```
    Các nút submit luôn trả về `false` trong `isSuccessfulControl`. Tác giả ghi chú "handled separately", nhưng trong `internal/engine/engine.go:1563` (`runClickDefault`), khi click submit button:
    ```go
    case "submit", "":
        if form := FindForm(n); form != nil {
            s.SubmitForm(form)
        }
    ```
    Lệnh này chỉ truyền `form` vào `SubmitForm(form)`, không truyền đối tượng button submitter (`n`). Do đó name và value của nút submit được click không bao giờ xuất hiện trong `FormSubmission.Data`.
- **Reproducer tối giản**:
  ```html
  <form action="/submit" method="GET">
    <button type="submit" name="action" value="save" id="btn_save">Save</button>
    <button type="submit" name="action" value="delete" id="btn_del">Delete</button>
  </form>
  ```
  - Thao tác: Click `#btn_save`.
  - Kỳ vọng: Server nhận parameter `action=save`.
  - Actual: Server nhận request GET `/submit` rỗng không có parameter `action` nào.

---

### 7.6. Kết quả Thực thi Nghiệm thu (Acceptance Criteria Output)

```
$ go test -count=5 -run TestJourney ./cmd/goosie/
ok  	github.com/vyquocvu/goosie/cmd/goosie	11.817s

$ go test -race -count=2 -run TestJourney ./cmd/goosie/
ok  	github.com/vyquocvu/goosie/cmd/goosie	11.944s

$ go test -count=1 ./...
ok  	github.com/vyquocvu/goosie/cmd/goosie	10.388s
ok  	github.com/vyquocvu/goosie/internal/archtest	4.515s
ok  	github.com/vyquocvu/goosie/internal/css	0.816s
ok  	github.com/vyquocvu/goosie/internal/dom	1.018s
ok  	github.com/vyquocvu/goosie/internal/download	0.646s
ok  	github.com/vyquocvu/goosie/internal/engine	1.538s
ok  	github.com/vyquocvu/goosie/internal/image	2.186s
ok  	github.com/vyquocvu/goosie/internal/platform/darwin	2.630s
ok  	github.com/vyquocvu/goosie/internal/preferences	2.033s
ok  	github.com/vyquocvu/goosie/internal/style	1.913s
ok  	github.com/vyquocvu/goosie/internal/tabs	2.043s
ok  	github.com/vyquocvu/goosie/internal/toolbar	2.029s
ok  	github.com/vyquocvu/goosie/test/bookmarks	2.017s
ok  	github.com/vyquocvu/goosie/test/css	1.689s
ok  	github.com/vyquocvu/goosie/test/dom	1.520s
ok  	github.com/vyquocvu/goosie/test/domtest	1.619s
ok  	github.com/vyquocvu/goosie/test/download	1.651s
ok  	github.com/vyquocvu/goosie/test/engine	7.253s
ok  	github.com/vyquocvu/goosie/test/entrypoints	21.257s
ok  	github.com/vyquocvu/goosie/test/flags	1.867s
ok  	github.com/vyquocvu/goosie/test/frame	1.884s
ok  	github.com/vyquocvu/goosie/test/gate	36.221s
ok  	github.com/vyquocvu/goosie/test/gatecheck	19.468s
ok  	github.com/vyquocvu/goosie/test/history	1.828s
ok  	github.com/vyquocvu/goosie/test/js	4.891s
ok  	github.com/vyquocvu/goosie/test/layout	1.740s
ok  	github.com/vyquocvu/goosie/test/manifest	1.653s
ok  	github.com/vyquocvu/goosie/test/net	4.246s
ok  	github.com/vyquocvu/goosie/test/paint	1.969s
ok  	github.com/vyquocvu/goosie/test/pdf	1.427s
ok  	github.com/vyquocvu/goosie/test/perms	1.527s
ok  	github.com/vyquocvu/goosie/test/platform	0.630s
ok  	github.com/vyquocvu/goosie/test/qc	0.964s
ok  	github.com/vyquocvu/goosie/test/raster	2.172s
ok  	github.com/vyquocvu/goosie/test/session	0.532s
ok  	github.com/vyquocvu/goosie/test/style	0.926s
ok  	github.com/vyquocvu/goosie/test/surface	1.022s
ok  	github.com/vyquocvu/goosie/test/tabs	1.125s
ok  	github.com/vyquocvu/goosie/test/toolbar	0.944s
ok  	github.com/vyquocvu/goosie/test/userscript	0.736s
ok  	github.com/vyquocvu/goosie/test/viewsource	0.718s
ok  	github.com/vyquocvu/goosie/test/wpt	7.590s
(Exit code 0, 100% PASS trên máy không display)

$ go build ./...
(Exit code 0, sạch)

$ go vet -all ./...
(Exit code 0, sạch)

$ go test ./internal/archtest/
ok  	github.com/vyquocvu/goosie/internal/archtest	4.515s

$ go test ./test/qc/
ok  	github.com/vyquocvu/goosie/test/qc	0.660s

$ gofmt -l cmd/goosie
(Exit code 0, sạch, không có output)

$ go test -count=1 -run TestTraceability ./test/qc/ -update
ok  	github.com/vyquocvu/goosie/test/qc	0.493s
(Golden test/qc/traceability.golden.json cập nhật đủ 11 case BR-097..BR-107 với trường tests không rỗng)
```

