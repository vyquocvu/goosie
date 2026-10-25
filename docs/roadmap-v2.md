# Goosie — roadmap trải nghiệm trình duyệt hằng ngày

- Cập nhật: 2026-10-14
- Phạm vi: ứng dụng macOS ARM64 với engine Go hiện tại; bản headless tiếp tục dùng chung engine.
- Mục tiêu: mở, đọc, tương tác và quay lại các trang web được Goosie hỗ trợ qua giao diện thông thường.

Roadmap này ưu tiên **hành trình sử dụng** thay vì số lượng API hay tính năng engine. Mỗi chặng tạo ra một luồng hoàn chỉnh và kiểm tra được. Đây không phải cam kết tương thích mọi website hoặc ngang Safari/Chrome.

[Bản đánh giá và quyết định phát hành trước đây](roadmap-v2-history.md) được giữ để tra cứu bằng chứng và rủi ro đã được chấp nhận. Những nhận định ở đầu bản đó là ảnh chụp lịch sử; dùng trạng thái hiện tại bên dưới để lập việc mới. [PROJECT.md](../PROJECT.md) mô tả kiến trúc hiện hành.

## Hiện trạng làm điểm xuất phát

Đối chiếu mã ngày 2026-10-14; đây là kiểm kê khả năng trong mã, chưa phải kết quả thử thủ công trên website thực tế.

| Đã có nền tảng | Phần trải nghiệm còn thiếu hoặc cần hoàn thiện |
| --- | --- |
| Nhập URL, liên kết/anchor, Back/Forward/reload, tab độc lập, tab mới/đóng/chuyển/sắp xếp, tab tràn, khôi phục phiên | Tìm kiếm bằng từ khóa trong thanh địa chỉ, gợi ý, trạng thái điều hướng rõ ràng, mở lại tab vừa đóng và thao tác tab bằng menu |
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

**Cổng P0:** chạy trọn hành trình trên trang kiểm thử cục bộ và tập trang thực tế đã chọn; kiểm tra bằng native macOS và headless/Playwright ở phần có thể so sánh. Lỗi điều hướng phải hiện trong giao diện, không chỉ in ra stderr.

### P1 — Quản lý việc duyệt web trong giao diện

**Hành trình:** lưu một trang, tìm lại ngày hôm sau, tải tệp, khôi phục phiên, xóa dữ liệu mình muốn.

| Việc | Kết quả cần thấy | Tiêu chí hoàn thành |
| --- | --- | --- |
| Bookmark và history | Nút sao, danh sách có tìm kiếm, mở/xóa mục, phím tắt dễ khám phá | Thêm/xóa cập nhật ngay giao diện và tệp lưu; tiêu đề/URL sau redirect đúng; danh sách dài vẫn dùng được bằng bàn phím |
| Downloads | Bảng tải xuống với tên, tiến độ, thành công/lỗi/hủy, thư mục lưu cấu hình được và “Show in Finder” | Hủy thật sự dừng công việc; tên tệp không đè tệp cũ; người dùng thấy đường dẫn và lỗi có thể xử lý |
| Phiên và tab | Tùy chọn khởi động: tab mới hoặc phiên trước; thao tác đóng tab, đóng tab khác, sao chép tab qua menu | Khởi động lại phục hồi tab đang chọn, URL, thứ tự, lịch sử và vị trí cuộn trong giới hạn đã định; không phục hồi dữ liệu riêng tư |
| Quyền riêng tư | Mở cửa sổ riêng tư từ menu/phím tắt; giao diện xóa history, cookie, cache, phiên theo phạm vi hỗ trợ | Dữ liệu bị xóa cả trong bộ nhớ lẫn trên đĩa; cửa sổ riêng tư không ghi vào profile thường; UI nói rõ loại dữ liệu nào được xóa |
| Cài đặt cơ bản | Công cụ tìm kiếm, thư mục tải xuống và hành vi khởi động đã có cài đặt theo profile; điều khiển quyền riêng tư còn thiếu | Đổi cài đặt có hiệu lực rõ ràng, lưu qua lần mở sau và xử lý cấu hình lỗi an toàn |

**Trạng thái "Bookmark và history" (cập nhật 2026-10-22):**
- **Đã có trong mã:** trang quản lý `goosie://bookmarks` (tìm kiếm, đổi tên tiêu đề qua form GET, xóa mục, mở URL an toàn — chỉ http/https/file mới render thành link) và `goosie://history` (tìm kiếm, xóa từng entry theo URL+timestamp, xóa toàn bộ có bước xác nhận). Store có API đột biến mới (`bookmarks.Delete/Rename/Search`, `history.DeleteEntry/Clear/Search`); mọi đột biến ghi tệp atomic ngay và lỗi lưu tệp hiển thị banner trong UI. Phiên private (store nil) hiện thông báo và không có nút đột biến. Trang truy cập được qua nav header của mọi trang `goosie://` và gợi ý omnibox ("Bookmarks Manager", "Browsing History"). `normalizeURL` giữ nguyên hoa/thường query của URL `goosie://` để tham số hành động không bị hỏng.
- **Kiểm thử tự động:** test store (`test/bookmarks`, `test/history`) và test tích hợp `cmd/goosie` (xóa/rename/search, escape title/URL thù địch, phản hồi lỗi lưu, ranh giới private, routing) **PASS** ngày 2026-10-22; `go test ./...`, `go test -race ./...`, `go vet -all ./...`, archtest và gate suite đều PASS. `make parity` đạt 80/80 fixture, trung bình 95.99% so với Chromium.
- **Chưa kiểm chứng:** thao tác native macOS (CUA) trên app thật và so sánh visual/parity riêng cho giao diện quản lý mới. CUA bị BLOCKED do phiên kiểm thử không có display khả dụng; corpus parity hiện tại không chứa Chromium reference cho hai trang nội bộ.

**Trạng thái "Công cụ tìm kiếm theo profile" (cập nhật 2026-10-24):**
- **Đã có trong mã:** `goosie://settings` cho phép cấu hình URL tìm kiếm có đúng một `{query}` marker trong query string, thư mục tải xuống tuyệt đối, hành vi khởi động (`restore` hoặc `newtab`), và tùy chọn hỏi nơi lưu mỗi lượt tải. Template chỉ nhận HTTP/HTTPS không có credentials. Cấu hình lưu atomic trong `preferences.json` của profile, dùng cho nhập omnibox/gợi ý, lượt tải mới và khôi phục phiên; `-download-dir` ghi đè cấu hình khi khởi chạy; phiên private đọc được nhưng không được ghi cấu hình profile.
- **Kiểm thử tự động:** test validate/escape/load/save, test routing và lưu settings, từ chối scheme không an toàn, bảo vệ phiên private, xác nhận query Unicode, luồng tải tới thư mục cấu hình, và lựa chọn khôi phục phiên; `go test ./...`, `go vet -all ./...`, `make e2e`, `go test -race` theo phạm vi package đều PASS ngày 2026-10-24. Trang Settings headless so với Chromium đạt 94.97% pixel match trước khi thêm điều khiển khởi động; cần chạy lại parity visual cho bản trang mới.
- **Còn thiếu:** cài đặt quyền riêng tư; kiểm chứng form settings qua ứng dụng native macOS.

**Trạng thái "Downloads stream và cancel" (cập nhật 2026-10-24):**
- **Đã có trong mã:** response điều hướng HTTP(S) trả headers trước body; attachment và nội dung không render được ghi theo chunk vào tệp tạm, sau đó mới xuất hiện dưới tên collision-free. Download manager cập nhật progress theo nhịp giới hạn; Cancel hủy request đang mở và xóa phần file chưa hoàn tất. Body dùng để render vẫn giữ giới hạn 16 MiB.
- **Kiểm thử tự động:** HTTP stream trả được tệp lớn hơn giới hạn document, xác nhận headers đến trước khi body hoàn tất và context hủy đóng request; `SaveStream` kiểm tra tệp lớn, progress và dọn file khi hủy; test tích hợp `loadURLCtx` hủy attachment đang truyền và xác nhận không còn file dở dang. `go test ./...`, `make e2e`, `go test -race ./internal/download ./test/net ./cmd/goosie`, `go vet -all ./...` PASS ngày 2026-10-24.
- **Đã có trong mã:** khi bật “Ask where to save each download” trong Settings, lượt tải stream dùng native `NSSavePanel`; ghi vào tệp tạm cạnh đích đã chọn rồi rename khi hoàn tất, và đóng hộp thoại thì hủy điều hướng mà không tạo download item hay file. Tùy chọn mặc định tắt để giữ thư mục tải hiện tại.
- **Kiểm thử tự động:** fake picker xác nhận đường dẫn được chọn, nội dung file và trạng thái download manager; kiểm tra cancel dialog; kiểm tra stream hủy không đè file đích hiện có và stream hoàn tất mới thay thế file. Luồng native UI chưa được kiểm chứng trên macOS.

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

**Cổng P2:** các luồng đã công bố chạy lặp lại được trong bộ kiểm thử tự động và qua ứng dụng native. Không dùng tỷ lệ WPT chung để thay cho kết quả hành trình người dùng.

### P3 — Tiện ích và hoàn thiện sản phẩm

Ưu tiên sau khi P0–P2 ổn định: menu ngữ cảnh cho liên kết/ảnh/văn bản, bookmark bar, Reader Mode, in/xuất PDF, giao diện xem nguồn/kiểm tra phần tử, quản lý nhiều cửa sổ. Chỉ đưa từng mục vào sprint khi có hành trình người dùng, thiết kế tương tác và tiêu chí nghiệm thu cụ thể. Autofill mật khẩu và đồng bộ đám mây cần thiết kế dữ liệu/bảo mật riêng; chưa nằm trong phạm vi roadmap này.

## Quy tắc triển khai và kiểm chứng

- Giữ kiến trúc Parse → Style → Layout → Paint và giới hạn nhận đầu vào. Các binary xử lý nội dung không tin cậy phải dùng `Session.PaintChecked`; ảnh phải được kiểm tra metadata trước giải mã. Thêm giao diện không được bỏ qua các giới hạn này.
- Mỗi chặng có kiểm thử logic, hành trình qua `platform/headless`, và so sánh trực quan với Chromium/Playwright cho thay đổi UI hoặc trang. Chạy `go test ./...`, `go test -race ./...`, `go vet -all ./...`, `go test ./internal/archtest/` và gate suite trước khi đánh dấu hoàn thành.
- Ghi nhận kết quả theo hành trình: đầu vào, hành động, ảnh/trạng thái mong đợi, dữ liệu sau khởi động lại và lỗi còn mở. Đánh dấu **đã có trong mã**, **đã kiểm chứng**, **đang làm** riêng biệt.
- Chính sách origin, cookie, tải xuống và dữ liệu riêng tư đi cùng tính năng dùng chúng. Rủi ro phát hành đã được chủ sản phẩm chấp nhận trong [bản lịch sử](roadmap-v2-history.md); roadmap UX này không tự thay đổi quyết định phát hành đó.
- Color emoji: font bitmap màu (sbix/COLR như Apple Color Emoji) chưa được thư viện font hiện tại (golang.org/x/image/font/opentype) rasterize; glyph thiếu hiện được thay thế bằng tofu có advance ổn định 1em để giữ layout và không nuốt ký tự.

**Việc bắt đầu tiếp theo:** P0 thanh địa chỉ thông minh và trạng thái điều hướng, sau đó gợi ý và mở lại tab. Đây là chuỗi ngắn nhất để biến nền tảng đã có thành một trải nghiệm mở và quay lại trang hằng ngày.
