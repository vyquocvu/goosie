# Goosie — roadmap trải nghiệm trình duyệt hằng ngày

- Cập nhật: 2026-11-02
- Phạm vi: ứng dụng macOS ARM64 với engine Go hiện tại; bản headless tiếp tục dùng chung engine.
- Mục tiêu: mở, đọc, tương tác và quay lại các trang web được Goosie hỗ trợ qua giao diện thông thường.

Roadmap này ưu tiên **hành trình sử dụng** thay vì số lượng API hay tính năng engine. Mỗi chặng tạo ra một luồng hoàn chỉnh và kiểm tra được. Đây không phải cam kết tương thích mọi website hoặc ngang Safari/Chrome.

[Bản đánh giá và quyết định phát hành trước đây](roadmap-v2-history.md) được giữ để tra cứu bằng chứng và rủi ro đã được chấp nhận. Những nhận định ở đầu bản đó là ảnh chụp lịch sử; dùng trạng thái hiện tại bên dưới để lập việc mới. [PROJECT.md](../PROJECT.md) mô tả kiến trúc hiện hành.

## Hiện trạng làm điểm xuất phát

Đối chiếu mã ngày 2026-10-14; đây là kiểm kê khả năng trong mã, chưa phải kết quả thử thủ công trên website thực tế.

| Đã có nền tảng | Phần trải nghiệm còn thiếu hoặc cần hoàn thiện |
| --- | --- |
| Nhập URL, liên kết/anchor, Back/Forward/reload, tab độc lập, tab mới/đóng/chuyển/sắp xếp, tab tràn, khôi phục phiên; tìm kiếm bằng từ khóa, gợi ý frecency và mở lại tab vừa đóng | Trạng thái điều hướng rõ ràng (đang tải/lỗi/redirect) trong chrome, favicon và thao tác tab bằng menu |
| Tìm trong trang, chọn/sao chép văn bản, zoom bằng phím tắt, resize/reflow | Hiển thị mức zoom, thao tác bằng chuột và phím nhất quán, thông báo khi trang tải chậm/lỗi |
| Kho bookmark/history/session/cookie; `Cmd+D` lưu bookmark; `-private` và `-clear-data` | Giao diện xem/sửa/xóa bookmark và history, mở cửa sổ riêng tư, điều khiển dữ liệu duyệt web trong ứng dụng |
| Tự lưu phản hồi cần tải xuống vào thư mục cấu hình | Danh sách tiến độ/kết quả, hủy tải, chọn nơi lưu và mở tệp trong Finder |
| DOM, CSS, layout, JS, focus và điều khiển biểu mẫu cơ bản | Chứng minh các luồng biểu mẫu/đăng nhập trên một tập website chọn lọc; sửa lỗi cản hành trình thay vì mở rộng API theo danh sách chung |

Không lên lại các phần đã có như một tính năng mới. Mỗi mục dưới đây chỉ nói về khoảng trống còn lại và tích hợp giao diện.

## Thứ tự thực hiện

### P0 — Mở trang và điều hướng trơn tru

**Hành trình:** mở ứng dụng → gõ URL hoặc cụm từ → mở kết quả → đi theo liên kết → quay lại/tiến tới/reload/stop → biết trang nào đang hoạt động.

| Việc | Kết quả cần thấy | Tiêu chí hoàn thành |
| --- | --- | --- |
| Thanh địa chỉ thông minh | Phân biệt URL/hostname với cụm tìm kiếm; dùng một công cụ tìm kiếm mặc định | URL hợp lệ và hostname mở đúng; văn bản thường thành truy vấn đã mã hóa; không gửi nhầm chuỗi giống URL sang tìm kiếm; kiểm tra Unicode và ký tự đặc biệt |
| Gợi ý khi nhập | Gợi ý từ history/bookmark và lựa chọn tìm kiếm; thao tác chuột, phím mũi tên, Enter, Escape | Kết quả đúng tab/profile; không lộ dữ liệu riêng tư sang phiên thường; gõ nhanh hoặc chuyển tab không để gợi ý cũ xuất hiện |
| Trạng thái điều hướng | Nút Back/Forward vô hiệu khi không đi được; reload/stop, đang tải, URL cuối sau redirect, lỗi/cert rõ ràng | Mỗi trạng thái khớp tài liệu đang hiển thị, kể cả điều hướng lỗi, redirect, hủy tải và chuyển tab khi trang còn đang tải |
| Trạng thái tab | Hiển thị tiêu đề/favicon hoặc biểu tượng dự phòng; mở lại tab vừa đóng (`Cmd+Shift+T`) | Tab mới, tab nền, tab đóng và phiên khôi phục có nhãn đúng; mở lại giữ URL và lịch sử của tab trong phạm vi hỗ trợ |

**Trạng thái "Thanh địa chỉ thông minh, gợi ý và mở lại tab" (cập nhật 2026-11-02):**
- **Đã có trong mã:** `normalizeAddress` phân biệt URL/hostname với cụm từ, dùng template `{query}` của profile và fallback về công cụ mặc định khi template hỏng. Gợi ý omnibox gộp trang nội bộ, bookmark và history thành một danh sách xếp hạng frecency (visit × recency bucket; bookmark baseline 1.0), giữ lựa chọn tìm kiếm ở đầu, giới hạn 8 dòng và không đọc bookmark/history trong tab private hoặc phiên `-private`. Gõ phím debounce 50ms và được chạy từ `pumpChrome`; focus/click vẫn truy vấn ngay. Reopen Closed Tab (`Cmd+Shift+T`) khôi phục tab, URL, title, scroll và thứ tự stack.
- **Kiểm thử tự động:** `cmd/goosie/normalize_url_test.go`, `cmd/goosie/suggestions_test.go`, `internal/toolbar/toolbar_test.go`, `cmd/goosie/reopen_tab_test.go`, `internal/history/history_test.go` bao phủ Unicode, ký tự đặc biệt, thứ hạng frecency, giới hạn 8 dòng, debounce, chọn bằng mũi tên/Enter/Esc, rò dữ liệu private và khôi phục tab.
- **Còn thiếu:** native CUA trên app macOS (phiên không có display).

**Trạng thái "Menu thao tác tab và favicon" (cập nhật 2026-11-02):**
- **Đã có trong mã:** chuột phải trên tab mở menu ngữ cảnh nhắm đúng tab đó (New Tab, Reload, Duplicate, Pin/Unpin, Close Tab, Close Other Tabs, Close Tabs to the Right, Reopen Closed Tab); đóng hàng loạt giải phóng session và navigation của từng tab trước khi gỡ. Tab strip dành cột icon 16px: favicon khai báo bằng `<link rel="icon">`, `shortcut icon` hoặc `apple-touch-icon` được tải bất đồng bộ qua client của tab (private client cho incognito), giải mã qua giới hạn `internal/image`, và chỉ áp dụng nếu navigation serial còn hiện hành. Tab không có icon vẽ glyph dự phòng; trình duyệt không tự gọi `/favicon.ico` để tránh request ẩn tới mọi origin.
- **Kiểm thử tự động:** `cmd/goosie/favicon_test.go` (trích xuất link, decode ảnh hỏng, serial cũ, private client), `cmd/goosie/tab_menu_test.go`, `internal/tabs/favicon_test.go` (favicon và glyph dự phòng); archtest, `go vet`, race suite và gate suite PASS ngày 2026-11-02.
- **Còn thiếu:** native CUA trên macOS (BLOCKED: phiên không có display); visual parity riêng cho chrome tab.

**Cổng P0:** chạy trọn hành trình trên trang kiểm thử cục bộ và tập trang thực tế đã chọn; kiểm tra bằng native macOS và headless/Playwright ở phần có thể so sánh. Lỗi điều hướng phải hiện trong giao diện, không chỉ in ra stderr.

### P1 — Quản lý việc duyệt web trong giao diện

**Hành trình:** lưu một trang, tìm lại ngày hôm sau, tải tệp, khôi phục phiên, xóa dữ liệu mình muốn.

| Việc | Kết quả cần thấy | Tiêu chí hoàn thành |
| --- | --- | --- |
| Bookmark và history | Nút sao, danh sách có tìm kiếm, mở/xóa mục, phím tắt dễ khám phá | Thêm/xóa cập nhật ngay giao diện và tệp lưu; tiêu đề/URL sau redirect đúng; danh sách dài vẫn dùng được bằng bàn phím |
| Downloads | Bảng tải xuống với tên, tiến độ, thành công/lỗi/hủy/tạm dừng, thư mục lưu cấu hình được và “Show in Finder” | Hủy thật sự dừng công việc; tạm dừng giữ phần đã tải và tiếp tục bằng HTTP Range; tên tệp không đè tệp cũ; người dùng thấy đường dẫn và lỗi có thể xử lý |
| Phiên và tab | Tùy chọn khởi động: tab mới hoặc phiên trước; thao tác đóng tab, đóng tab khác, sao chép tab qua menu | Khởi động lại phục hồi tab đang chọn, URL, thứ tự, lịch sử và vị trí cuộn trong giới hạn đã định; không phục hồi dữ liệu riêng tư |
| Quyền riêng tư | Mở cửa sổ riêng tư từ menu/phím tắt; giao diện xóa history, cookie, cache, phiên theo phạm vi hỗ trợ | Dữ liệu bị xóa cả trong bộ nhớ lẫn trên đĩa; cửa sổ riêng tư không ghi vào profile thường; UI nói rõ loại dữ liệu nào được xóa |
| Cài đặt cơ bản | Công cụ tìm kiếm, thư mục tải xuống, hành vi khởi động và xóa dữ liệu duyệt web theo phạm vi | Đổi cài đặt có hiệu lực rõ ràng, lưu qua lần mở sau và xử lý cấu hình lỗi an toàn; dữ liệu xóa đúng phạm vi đã chọn |

**Trạng thái "Bookmark và history" (cập nhật 2026-10-22):**
- **Đã có trong mã:** trang quản lý `goosie://bookmarks` (tìm kiếm, đổi tên tiêu đề qua form GET, xóa mục, mở URL an toàn — chỉ http/https/file mới render thành link) và `goosie://history` (tìm kiếm, xóa từng entry theo URL+timestamp, xóa toàn bộ có bước xác nhận). Store có API đột biến mới (`bookmarks.Delete/Rename/Search`, `history.DeleteEntry/Clear/Search`); mọi đột biến ghi tệp atomic ngay và lỗi lưu tệp hiển thị banner trong UI. Phiên private (store nil) hiện thông báo và không có nút đột biến. Trang truy cập được qua nav header của mọi trang `goosie://` và gợi ý omnibox ("Bookmarks Manager", "Browsing History"). `normalizeURL` giữ nguyên hoa/thường query của URL `goosie://` để tham số hành động không bị hỏng.
- **Kiểm thử tự động:** test store (`test/bookmarks`, `test/history`) và test tích hợp `cmd/goosie` (xóa/rename/search, escape title/URL thù địch, phản hồi lỗi lưu, ranh giới private, routing) **PASS** ngày 2026-10-22; `go test ./...`, `go test -race ./...`, `go vet -all ./...`, archtest và gate suite đều PASS. `make parity` đạt 80/80 fixture, trung bình 95.99% so với Chromium.
- **Chưa kiểm chứng:** thao tác native macOS (CUA) trên app thật và so sánh visual/parity riêng cho giao diện quản lý mới. CUA bị BLOCKED do phiên kiểm thử không có display khả dụng; corpus parity hiện tại không chứa Chromium reference cho hai trang nội bộ.

**Trạng thái "Công cụ tìm kiếm theo profile" (cập nhật 2026-10-24):**
- **Đã có trong mã:** `goosie://settings` cho phép cấu hình URL tìm kiếm có đúng một `{query}` marker trong query string, thư mục tải xuống tuyệt đối, hành vi khởi động (`restore` hoặc `newtab`), và tùy chọn hỏi nơi lưu mỗi lượt tải. Template chỉ nhận HTTP/HTTPS không có credentials. Cấu hình lưu atomic trong `preferences.json` của profile, dùng cho nhập omnibox/gợi ý, lượt tải mới và khôi phục phiên; `-download-dir` ghi đè cấu hình khi khởi chạy; phiên private đọc được nhưng không được ghi cấu hình profile.
- **Kiểm thử tự động:** test validate/escape/load/save, test routing và lưu settings, từ chối scheme không an toàn, bảo vệ phiên private, xác nhận query Unicode, luồng tải tới thư mục cấu hình, và lựa chọn khôi phục phiên; `go test ./...`, `go vet -all ./...`, `make e2e`, `go test -race` theo phạm vi package đều PASS ngày 2026-10-24. Trang Settings headless so với Chromium đạt 94.97% pixel match trước khi thêm điều khiển khởi động; cần chạy lại parity visual cho bản trang mới.
- **Còn thiếu:** kiểm chứng form settings qua ứng dụng native macOS.

**Trạng thái "Quyền riêng tư trong Settings" (cập nhật 2026-11-02):**
- **Đã có trong mã:** thẻ Privacy & browsing data trên `goosie://settings` xóa history theo phạm vi (giờ qua, hôm nay, toàn bộ), toàn bộ cookie, cache response trong bộ nhớ, và tệp phiên/journal. Mỗi lượt xóa có bước xác nhận liệt kê đúng loại dữ liệu sẽ mất; bookmark và cấu hình được giữ. History xóa trong store rồi ghi atomic ngay; cookie xóa khỏi jar và gỡ tệp profile; network client có `ClearCache`; xóa phiên gỡ `session.json` và `session_active.json`, bỏ crash journal đang chờ và tắt ghi phiên cho tới khi thoát nên dữ liệu không quay lại lúc exit. Phiên private chỉ hiển thị giải thích, không có form xóa và không chạm profile.
- **Kiểm thử tự động:** `test/net` xác nhận `ClearCache` buộc request đi mạng lại; `cmd/goosie` kiểm tra thẻ privacy liệt kê phạm vi, yêu cầu xác nhận, xóa đúng phạm vi history (giữ mục cũ hơn một giờ), xóa cookie cả RAM lẫn tệp, xóa cache, xóa tệp phiên + chặn snapshot, và private không hiển thị form cũng không đụng tệp profile.
- **Còn thiếu:** Settings chỉ xóa cookie toàn bộ (chưa theo từng site); localStorage không nằm trong phạm vi bốn loại dữ liệu này; chưa kiểm chứng CUA trên macOS.

**Trạng thái "Downloads stream và cancel" (cập nhật 2026-10-24):**
- **Đã có trong mã:** response điều hướng HTTP(S) trả headers trước body; attachment và nội dung không render được ghi theo chunk vào tệp tạm, sau đó mới xuất hiện dưới tên collision-free. Download manager cập nhật progress theo nhịp giới hạn; Cancel hủy request đang mở và xóa phần file chưa hoàn tất. Body dùng để render vẫn giữ giới hạn 16 MiB.
- **Kiểm thử tự động:** HTTP stream trả được tệp lớn hơn giới hạn document, xác nhận headers đến trước khi body hoàn tất và context hủy đóng request; `SaveStream` kiểm tra tệp lớn, progress và dọn file khi hủy; test tích hợp `loadURLCtx` hủy attachment đang truyền và xác nhận không còn file dở dang. `go test ./...`, `make e2e`, `go test -race ./internal/download ./test/net ./cmd/goosie`, `go vet -all ./...` PASS ngày 2026-10-24.
- **Đã có trong mã:** khi bật “Ask where to save each download” trong Settings, lượt tải stream dùng native `NSSavePanel`; ghi vào tệp tạm cạnh đích đã chọn rồi rename khi hoàn tất, và đóng hộp thoại thì hủy điều hướng mà không tạo download item hay file. Tùy chọn mặc định tắt để giữ thư mục tải hiện tại.
- **Kiểm thử tự động:** fake picker xác nhận đường dẫn được chọn, nội dung file và trạng thái download manager; kiểm tra cancel dialog; kiểm tra stream hủy không đè file đích hiện có và stream hoàn tất mới thay thế file. Luồng native UI chưa được kiểm chứng trên macOS.

**Trạng thái "Download Pause/Resume bằng HTTP Range" (cập nhật 2026-11-02):**
- **Đã có trong mã:** item tải xuống có trạng thái `paused` và giữ `TempPath`; Pause hủy request đang mở bằng context cause riêng nên writer giữ nguyên phần đã ghi thay vì xóa. Resume phát lại GET với `Range: bytes=<đã nhận>-`, chỉ chấp nhận `206 Partial Content` (hoặc `416` khi đã đủ byte), ghi nối vào tệp tạm, tính lại SHA-256 trên toàn tệp rồi mới link/rename thành tên cuối. Cancel từ trạng thái paused xóa tệp tạm. Resume dùng lại đúng client của lượt tải đầu (kể cả client riêng của tab incognito). `goosie://downloads` có nút Pause/Resume, badge `paused`, và progress bar giữ nguyên tiến độ.
- **Kiểm thử tự động:** `internal/download` kiểm tra pause giữ file và byte count, resume nối đúng nội dung + digest, cancel/fail xóa file tạm, completion xóa tham chiếu temp; `internal/net` kiểm tra header Range và 206; `cmd/goosie` kiểm tra end-to-end server trả nửa đầu rồi chờ, pause dừng navigation, resume gửi đúng Range và tạo file hoàn chỉnh với SHA-256, cùng nút Pause/Resume trên trang downloads. `go test ./internal/archtest/`, `go test -race ./...` (trừ `/test/wpt`), `go test ./test/gate -run TestGate`, và `go vet -all ./...` PASS ngày 2026-11-02.
- **Còn thiếu:** chưa kiểm chứng thao tác Pause/Resume qua app native macOS.

**Trạng thái "Download robustness: 200 khi resume và thiếu Content-Length" (cập nhật 2026-11-02):**
- **Đã có trong mã:** khi resume gửi `Range` mà server trả `200 OK`, client không còn báo lỗi: tệp tạm cũ bị xóa, item được `Manager.Restart` đưa về 0 byte với tổng mới lấy từ `Content-Length`, và toàn bộ entity được ghi lại từ đầu qua đúng đường stream (Pause/Cancel vẫn hoạt động trong lượt tải lại; pause giữa chừng đăng ký lại tệp tạm và `Range` mới). `206` vẫn ghi nối như trước, `416` vẫn finalize phần đã đủ. Khi response không có `Content-Length`, item báo số byte đã nhận và tốc độ dạng xác định, còn thanh tiến độ chuyển sang trạng thái indeterminate có nhãn “size unknown” thay vì thanh đầy 100% giả; hoàn tất mới hiển thị 100%.
- **Kiểm thử tự động:** `internal/download` xác nhận `Restart` đưa item về 0 byte/tổng mới và bỏ qua item đã kết thúc; `cmd/goosie` dựng server bỏ qua `Range` và trả cả entity với `200`, rồi chốt file cuối trùng khớp từng byte + SHA-256 (không bị nối đôi), `Range` cũ vẫn được gửi, và chỉ còn một tệp kết quả; test chunked không `Content-Length` xác nhận byte đã nhận/hoàn tất và nội dung đúng; test trang downloads xác nhận bar indeterminate + “size unknown” thay cho 100%.
- **Còn thiếu:** server trả `200` cho *mọi* lần resume sẽ khiến mỗi lần pause phải tải lại từ đầu (đúng về dữ liệu nhưng tốn băng thông); chưa kiểm chứng qua app native macOS.

**Trạng thái "CSS focus-visible" (cập nhật 2026-10-24):**
- **Đã có trong mã:** `:focus-visible` khớp với focus qua Tab/Shift+Tab và với các control nhận text khi chúng được focus bằng chuột hoặc JavaScript; blur xóa trạng thái. Với control không nhập text, focus do pointer không tự bật focus-visible.
- **Kiểm thử tự động:** test selector, test pipeline Tab cập nhật computed style/work counters, và CSS reftest so với Chromium đều PASS; `make wpt-pilot` đạt 8/8 fixture, trong đó fixture mới đạt 100% pixel match.
- **Còn thiếu:** heuristic còn đơn giản so với toàn bộ thuật toán modality của trình duyệt; luồng keyboard/focus tổng thể của QA-007 và native macOS chưa có sign-off.

**Trạng thái "CSS :active" (cập nhật 2026-10-24):**
- **Đã có trong mã:** primary pointer press đặt trạng thái active lên hit target và ancestors; release hoặc đóng session xóa trạng thái. Enter/Space trên phần tử phù hợp phát `click`, chạy default action nếu keydown/click không bị hủy, và giữ trạng thái active xuyên suốt keydown/click/keyup. UI chuyển Space trên button-like control, Enter trên control activatable, điều hướng anchor Enter, và dùng Space/Shift+Space để cuộn khi focus không thuộc control nhận Space.
- **Kiểm thử tự động:** headless chrome-window test kiểm tra `:active` khi pointer nhấn/thả và kiểm tra Space đổi checkbox cùng computed `:checked` style với đúng một style/layout/display-list update. Engine test xác nhận active có trên target/ancestors trong các event bàn phím, click được phát, và preventDefault chặn activation.
- **Còn thiếu:** surface hiện nhận một key event đồng bộ nên chưa giữ trạng thái active để vẽ trong toàn bộ khoảng thời gian người dùng đang giữ phím; chưa hỗ trợ đầy đủ modifier/target behavior cho link keyboard activation; chưa có native CUA sign-off.

**Trạng thái "No-op DOM writes và render work" (cập nhật 2026-10-24):**
- **Đã có trong mã:** các setter JS cho attribute, `checked`, và `value` chỉ báo DOM mutation khi giá trị render-relevant thực sự đổi; `classList.remove()` với token không tồn tại cũng không kích hoạt invalidation. Setter `checked`/`value` giờ báo mutation khi đổi để frame mới không bị bỏ sót.
- **Đã có trong mã:** sau style resolve, interaction và PumpJS có thể giữ nguyên arena khi mọi computed layout input không đổi và arena không có anonymous boxes; node objects nhận style pointer mới để paint thấy màu/decoration mới. Thay đổi geometry hoặc box pseudo vẫn đi full layout.
- **Đã có trong mã:** `style.Resolver` giữ rule index theo stylesheet set của từng session; style và pseudo-element passes dùng chung index. Khi stylesheet đổi qua refresh, resolver được biên dịch lại. Các full pass vẫn duyệt toàn document và tính lại computed style.
- **Đã có trong mã:** với `:hover`, `:focus`, `:focus-visible`, và `:active` dùng selector cục bộ một compound, resolver nhận diện node có state đổi, resolve lại subtree cần thiết và cập nhật pseudo styles cục bộ. Snapshot cây/attribute/property và viewport bắt thay đổi ngoài phạm vi; selector có quan hệ tổ tiên/anh em, functional pseudo phụ thuộc interaction, hoặc state biểu mẫu dùng đường resolve toàn document. Khi arena không có anonymous boxes và layout input không đổi, chỉ style pointer của các node bị ảnh hưởng được cập nhật. `WorkCounts.StyleNodeVisits` đo số node thực sự được resolve.
- **Kiểm thử tự động:** work-counter tests xác nhận loạt ghi cùng giá trị không chạy style/layout/display-list; đổi class có thay đổi color chạy một style resolve, không layout build, và paint list dùng màu mới; đổi width chạy layout lại. Resolver tests xác nhận đổi hover chỉ resolve subtree được match, hover ancestor vẫn chọn đúng node, DOM mutation và selector quan hệ buộc fallback; browser callback test đo ít hơn nửa số node trong fixture nhiều sibling. Resolver test xác nhận index được dùng lại qua style/pseudo passes.
- **Còn thiếu:** DOM mutation thật vẫn đi qua full-document style resolve; selector interaction phức tạp vẫn dùng fallback; arena có anonymous boxes còn buộc full layout. `RefreshNode`/partial `Refresh` vẫn dựng lại toàn arena. Cần dependency tracking để mở rộng an toàn sang mutation và selector quan hệ, cùng cập nhật anonymous boxes cục bộ.

**Cổng P1:** kiểm tra hành trình qua hai lần khởi động ứng dụng, hai tab, một cửa sổ riêng tư và một tệp tải xuống; xác minh dữ liệu lưu/xóa thực tế.

### P2 — Tương tác với website được hỗ trợ

**Hành trình:** đọc bài viết, tìm nội dung, điền và gửi biểu mẫu, giữ phiên đăng nhập theo chính sách cookie hiện có.

1. Chọn một ma trận nhỏ các website và luồng cụ thể: trang tin/tài liệu, biểu mẫu tìm kiếm, đăng nhập cookie, trang có JS ở mức hiện hỗ trợ. Ghi rõ trình duyệt tham chiếu, kết quả mong đợi và phần chưa hỗ trợ.
2. Hoàn thiện điều khiển `input`, `textarea`, `select`, checkbox/radio, nút submit, focus, Tab, Enter, IME và thông báo lỗi biểu mẫu trên các luồng đã chọn. Nền tảng engine đã có; chỉ sửa những khoảng trống được tái hiện.
3. Làm thao tác đọc trang dễ dùng: Find có số kết quả và cuộn đến kết quả, chọn/sao chép chính xác, zoom có chỉ báo và đặt lại. Kiểm tra bằng chuột, bàn phím và accessibility tree.
4. Sửa các sai lệch HTML/CSS/JS và chính sách mạng phát hiện qua ma trận này. Mỗi lỗi có fixture hồi quy và so sánh với Chromium/Playwright khi ảnh hưởng đến hiển thị hoặc tương tác.

**Trạng thái "JS Node/DOM API để thư viện tự boot" (cập nhật 2026-10-25, đo trong TASK-039 §2.4 — nhánh C của `GOOSIE-LIVE-001`):**
- **Đã có trong mã:** `document.nodeType`, `nodeName` và `nodeType` trên mọi node được wrapper, dùng ordinal của DOM Standard (document 9, element 1, text 3, comment 8, doctype 10, fragment 11) và dịch từ ordinal nội bộ của `internal/dom`. Đây là gate đầu tiên của Sizzle/jQuery (`9 === doc.nodeType && doc.documentElement`).
- **Đã đo, chưa sửa:** sau gate đó jQuery 3.6 vẫn chết ở call kế tiếp. Verbatim trên www.gov.vn (lỗi đầu trong 21 lỗi chưa bắt được): `TypeError: Object has no member 'createDocumentFragment'`; trên bản lưu file://: `Object has no member 'insertBefore'`. Danh sách tối thiểu để một bản jQuery 3.6 boot hết: `createDocumentFragment`, `insertBefore`, `cloneNode`, `childNodes`/`lastChild`, `ownerDocument`, `defaultView`, `getComputedStyle`, `readyState`, kèm dispatch `DOMContentLoaded`/`load`.
- **Tiêu chí hoàn thành:** mỗi API một fixture riêng trong `testdata/render/js-dom/` với PNG Chromium đo thật và mutation proof giết đúng oracle của nó; gov.vn chỉ được coi là hết nhánh này khi `doc_height > 0` mà **không** cần pump event loop (tức nhánh còn lại bên dưới vẫn còn nguyên).
- **Còn thiếu:** chưa đo được bao nhiêu trang chỉ thiếu nhóm API này; mỗi API thêm một đường mutation DOM mới nên phải đi kèm gate giới hạn node/depth của `jsNodeBudget`.

**Trạng thái "Chế độ paced và sự kiện JS theo thời gian" (cập nhật 2026-10-25, đo trong TASK-039 §2.4 — nhánh D của `GOOSIE-LIVE-001`):**
- **Đúng theo thiết kế hiện tại:** `-screenshot`, `-gate` và `-bench` bỏ toolbar/tabManager, load đồng bộ và **không** cài `chromeWindow.setTick`, nên `pumpJS()`/`PumpJS` không bao giờ chạy và session đã load bị bỏ. Nhờ vậy mọi con số gate/bench/parity hiện hành là "một lượt render đồng bộ".
- **Hệ quả đo được:** mọi reveal cần thời gian (`$('#form1').fadeIn(200)` của gov.vn là ví dụ đo thật duy nhất hiện có) không có cơ hội chạy trong sweep, kể cả khi binding đã đủ API.
- **Quyết định ghi ở đây thay vì sửa ngay:** bật pump trong chế độ paced sẽ đổi **toàn bộ** đường chuẩn của các cổng đo đang tham chiếu nhau, nên theo Bước 2 của TASK-039, nhánh này được ghi vào roadmap và **defect để mở**, không hạ mức nghiêm trọng. Chưa đo blast radius; cần một task riêng với số liệu hai lần chạy trước/sau.

**Trạng thái "Unresponsive Script" (cập nhật 2026-11-02):**
- **Đã có trong mã:** `js.Runtime` phân biệt deadline của host với `Interrupt()` thủ công qua `TimedOut`/`TakeTimedOut`, nên điều hướng rời trang không bị báo nhầm là trang treo. `engine.Session` ghi `ScriptError.TimedOut`, đếm `ScriptTimeoutCount` cho script parser/defer/module, lifecycle event, và cả callback timer/RAF/event mà trước đây nuốt lỗi trong `Tick`. Khi một lượt load hoặc pump phát hiện timeout, chrome thay nội dung tab bằng prompt `goosie://unresponsive`: nêu script đã bị dừng sau 5 giây và cho hai lựa chọn — tải lại không chạy script, hoặc tải lại với giới hạn 30 giây. Hành động được route ngay trong `loadDocumentCtx`, không qua mạng và không ghi history cho lượt bị chặn.
- **Kiểm thử tự động:** `test/js/timeout_state_test.go` xác nhận timeout báo đúng một lần, host `Interrupt` không bị tính là timeout, timer callback được đánh dấu; `test/engine/script_timeout_test.go` xác nhận script parser và timer qua `PumpJS` tăng đúng bộ đếm và lần pump thứ hai không đếm lại; `cmd/goosie/unresponsive_page_test.go` xác nhận trang chậm hiển thị prompt thay vì nội dung, hai action đi đúng policy (tắt script / 30 giây), URL không phải HTTP bị từ chối, và trang bình thường không bị prompt.
- **Còn thiếu:** goja không có checkpoint nên script đã bị interrupt không thể resume; prompt xuất hiện sau khi engine đã dừng script chứ không phải hộp thoại chặn trước khi dừng như trình duyệt cũ. Với timeout sau khi trang đã hiển thị, toàn bộ trang được thay bằng prompt thay vì giữ nội dung đã render; chưa kiểm chứng CUA trên macOS.

**Trạng thái "Tìm trong trang" (cập nhật 2026-11-02):**
- **Đã có trong mã:** Cmd+F mở find ngay trên thanh địa chỉ; bộ đếm `n/total` vẽ trong ô; Enter/Shift+Enter chuyển kết quả và quay vòng; Esc đóng, nhấp ra ngoài chrome cũng đóng; highlight tô theo từng match và match đang chọn dùng màu khác. `Session.Find` giới hạn 1.000 kết quả và chỉ tìm trong word box đã layout.
- **Đã sửa trong lượt này:** mở lại Cmd+F khi find đang mở sẽ focus lại ô và chọn toàn bộ query để gõ đè; khi mở lại sau khi đóng, `OnFindOpen` chạy lại query đã lưu nên số kết quả hiện ngay thay vì `0/0`. Cuộn tới match nay clamp theo chiều cao layer (kết quả cuối không còn đẩy viewport quá cuối trang), đồng thời ghi offset vào tab, `window.scrollY` và accessibility tree giống cuộn bàn phím.
- **Kiểm thử tự động:** `cmd/goosie/find_and_zoom_test.go` xác nhận lifecycle, highlight theo từng match, focus/chọn query khi mở lại, callback query khôi phục, và clamp + đồng bộ scroll; `cmd/goosie/journey_find_test.go` chạy trọn hành trình qua `platform/headless`: Cmd+F → gõ "needle" → 2 kết quả → Enter cuộn tới kết quả thứ hai → Esc → Cmd+F mở lại vẫn hiện 2 kết quả.
- **Còn thiếu:** chưa có Cmd+G/Cmd+Shift+G; tập kết quả chỉ tính lại khi query đổi hoặc find mở lại, không theo dõi DOM mutation trong lúc find đang mở; chưa kiểm chứng CUA native macOS.

**Trạng thái "Điền và gửi biểu mẫu" (cập nhật 2026-11-02):**
- **Đã có trong mã:** `input`, `textarea`, `select`, checkbox/radio và nút submit chỉnh sửa/toggle/focus bằng chuột và bàn phím; Tab/Shift+Tab đi đúng thứ tự; Enter trong ô text submit ngầm với submitter đầu tiên, click/Enter/Space trên nút gửi đúng cặp `name`/`value` của chính nút được kích hoạt; validation `required`/`email`/`number` (cùng `novalidate`) chặn request, fire `invalid`, focus control lỗi đầu tiên và giữ nguyên nội dung; URL-encoded GET giữ Unicode và escape đúng.
- **Đã sửa trong lượt này:** Enter khi IME đang có candidate không còn chèn newline hoặc submit form — input method sở hữu phím đó, nên không điều hướng khỏi một từ đang gõ dở. CSS `:valid`/`:invalid` của tác giả nay được resolve lại sau mỗi lần sửa/toggle/kích hoạt, thay vì repaint bằng computed style cũ, nên màu báo lỗi xuất hiện và biến mất theo giá trị thật.
- **Đã sửa trong lượt này (POST):** `method="POST"` nay gửi dữ liệu form dưới dạng body `application/x-www-form-urlencoded` thay vì điều hướng rỗng. Request mang method/body xuyên qua `navigateTabRequest` → `loadDocumentCtx` → `loadURLCtx`; redirect 301/302/303 do client theo đuổi và tự chuyển sang GET (307/308 giữ POST), còn `Set-Cookie` của response POST được lưu vào jar như mọi response khác.
- **Đã sửa trong lượt này (override submitter):** nút submit được kích hoạt nay thắng `<form>` theo đúng thứ tự HTML: `formaction` đổi đích (giá trị rỗng quay về URL hiện tại), `formmethod` đổi method khi là `get`/`post`/`dialog` và rơi về method của form nếu không hợp lệ, `formnovalidate` bỏ qua constraint validation nên nút "Lưu nháp" gửi được khi ô bắt buộc còn trống. Các thuộc tính này chỉ có hiệu lực trên submit button (`input type=submit|image`, `button type=submit` hoặc không khai báo); nút thường không override.
- **Kiểm thử tự động:** `test/engine/ime_test.go` xác nhận Enter trong composition không submit, composition còn nguyên, và Enter sau commit gửi đúng giá trị; `test/engine/form_submitter_override_test.go` phủ override action/method, fallback khi `formmethod` không hỗ trợ, nút thường không override, và `formnovalidate`; `cmd/goosie/journey_p2_forms_test.go` chạy qua `platform/headless`: candidate IME hiện trong AX → Enter không rời trang → commit → submit đúng một lần với text đã commit; hai nút Save/Publish chỉ gửi cặp của nút được bấm; POST nhận đúng method, Content-Type, body `who=Ada+Lovelace&n=7&go=1`, không có query, cookie của response đi tiếp request sau; POST → 302 → GET giữ body ở request đầu và rỗng ở request redirect; Save (`formaction="/saved" formmethod="POST"`) đi tới `/saved` bằng POST trong khi Publish đi `/published` bằng GET; nút `formnovalidate` đi tới `/draft` dù ô required còn trống, còn nút thường vẫn focus ô lỗi và không gửi request. Hai `t.Skip` cũ cho BR-105/BR-107 đã bị xóa vì engine đã hỗ trợ từ TASK-031.
- **Còn thiếu:** chưa có `enctype="multipart/form-data"` và `text/plain` (`formenctype` trên nút cũng chưa xét; body vẫn URL-encoded dù tác giả khai báo khác), `:user-invalid`, bong bóng lỗi kiểu trình duyệt, hay `target`. History chỉ lưu URL, nên back/forward tới một kết quả POST phát lại GET thay vì hỏi resubmit như trình duyệt. Chưa kiểm chứng CUA native macOS.

**Cổng P2:** các luồng đã công bố chạy lặp lại được trong bộ kiểm thử tự động và qua ứng dụng native. Không dùng tỷ lệ WPT chung để thay cho kết quả hành trình người dùng.

### P3 — Tiện ích và hoàn thiện sản phẩm

Ưu tiên sau khi P0–P2 ổn định: menu ngữ cảnh cho liên kết/ảnh/văn bản, bookmark bar, Reader Mode, in/xuất PDF, giao diện xem nguồn/kiểm tra phần tử, quản lý nhiều cửa sổ. Chỉ đưa từng mục vào sprint khi có hành trình người dùng, thiết kế tương tác và tiêu chí nghiệm thu cụ thể. Autofill mật khẩu và đồng bộ đám mây cần thiết kế dữ liệu/bảo mật riêng; chưa nằm trong phạm vi roadmap này.

## Quy tắc triển khai và kiểm chứng

- Giữ kiến trúc Parse → Style → Layout → Paint và giới hạn nhận đầu vào. Các binary xử lý nội dung không tin cậy phải dùng `Session.PaintChecked`; ảnh phải được kiểm tra metadata trước giải mã. Thêm giao diện không được bỏ qua các giới hạn này.
- Mỗi chặng có kiểm thử logic, hành trình qua `platform/headless`, và so sánh trực quan với Chromium/Playwright cho thay đổi UI hoặc trang. Chạy `go test ./...`, `go test -race ./...`, `go vet -all ./...`, `go test ./internal/archtest/` và gate suite trước khi đánh dấu hoàn thành.
- Ghi nhận kết quả theo hành trình: đầu vào, hành động, ảnh/trạng thái mong đợi, dữ liệu sau khởi động lại và lỗi còn mở. Đánh dấu **đã có trong mã**, **đã kiểm chứng**, **đang làm** riêng biệt.
- Chính sách origin, cookie, tải xuống và dữ liệu riêng tư đi cùng tính năng dùng chúng. Rủi ro phát hành đã được chủ sản phẩm chấp nhận trong [bản lịch sử](roadmap-v2-history.md); roadmap UX này không tự thay đổi quyết định phát hành đó.
- Color emoji: font bitmap màu (sbix/COLR như Apple Color Emoji) chưa được thư viện font hiện tại (golang.org/x/image/font/opentype) rasterize; glyph thiếu hiện được thay thế bằng tofu có advance ổn định 1em để giữ layout và không nuốt ký tự.

**Việc bắt đầu tiếp theo:** các encoder `text/plain` và `multipart/form-data`, cùng `formenctype` trên submitter. Multipart chỉ có nghĩa khi `input type="file"` được thu thập (hiện đang bị loại), nên cần làm file input trước hoặc gộp chung; `text/plain` đơn giản hơn và có thể tách riêng. Sau đó là `:user-invalid`, bong bóng lỗi, và `target`. Các mục P1/P0 còn lại chỉ là kiểm chứng CUA native macOS, vẫn BLOCKED vì phiên kiểm thử không có display.
