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
| Downloads | Bảng tải xuống với tên, tiến độ, thành công/lỗi/hủy, vị trí lưu và “Show in Finder” | Hủy thật sự dừng công việc; tên tệp không đè tệp cũ; người dùng thấy đường dẫn và lỗi có thể xử lý |
| Phiên và tab | Tùy chọn khởi động: tab mới hoặc phiên trước; thao tác đóng tab, đóng tab khác, sao chép tab qua menu | Khởi động lại phục hồi tab đang chọn, URL, thứ tự, lịch sử và vị trí cuộn trong giới hạn đã định; không phục hồi dữ liệu riêng tư |
| Quyền riêng tư | Mở cửa sổ riêng tư từ menu/phím tắt; giao diện xóa history, cookie, cache, phiên theo phạm vi hỗ trợ | Dữ liệu bị xóa cả trong bộ nhớ lẫn trên đĩa; cửa sổ riêng tư không ghi vào profile thường; UI nói rõ loại dữ liệu nào được xóa |
| Cài đặt cơ bản | Công cụ tìm kiếm, thư mục tải xuống, hành vi khởi động và quyền riêng tư | Đổi cài đặt có hiệu lực rõ ràng, lưu qua lần mở sau và xử lý cấu hình lỗi an toàn |

**Trạng thái "Bookmark và history" (cập nhật 2026-10-22):**
- **Đã có trong mã:** trang quản lý `goosie://bookmarks` (tìm kiếm, đổi tên tiêu đề qua form GET, xóa mục, mở URL an toàn — chỉ http/https/file mới render thành link) và `goosie://history` (tìm kiếm, xóa từng entry theo URL+timestamp, xóa toàn bộ có bước xác nhận). Store có API đột biến mới (`bookmarks.Delete/Rename/Search`, `history.DeleteEntry/Clear/Search`); mọi đột biến ghi tệp atomic ngay và lỗi lưu tệp hiển thị banner trong UI. Phiên private (store nil) hiện thông báo và không có nút đột biến. Trang truy cập được qua nav header của mọi trang `goosie://` và gợi ý omnibox ("Bookmarks Manager", "Browsing History"). `normalizeURL` giữ nguyên hoa/thường query của URL `goosie://` để tham số hành động không bị hỏng.
- **Kiểm thử tự động:** test store (`test/bookmarks`, `test/history`) và test tích hợp `cmd/goosie` (xóa/rename/search, escape title/URL thù địch, phản hồi lỗi lưu, ranh giới private, routing) **PASS** ngày 2026-10-22; `go test ./...`, `go test -race ./...`, `go vet -all ./...`, archtest và gate suite đều PASS. `make parity` đạt 80/80 fixture, trung bình 95.99% so với Chromium.
- **Chưa kiểm chứng:** thao tác native macOS (CUA) trên app thật và so sánh visual/parity riêng cho giao diện quản lý mới. CUA bị BLOCKED do phiên kiểm thử không có display khả dụng; corpus parity hiện tại không chứa Chromium reference cho hai trang nội bộ.

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

**Việc bắt đầu tiếp theo:** P0 thanh địa chỉ thông minh và trạng thái điều hướng, sau đó gợi ý và mở lại tab. Đây là chuỗi ngắn nhất để biến nền tảng đã có thành một trải nghiệm mở và quay lại trang hằng ngày.
