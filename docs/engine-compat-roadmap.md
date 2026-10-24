# Goosie — roadmap tương thích Web Platform

- Cập nhật: 2026-10-24
- Phạm vi: engine HTML/CSS/DOM/JavaScript của Goosie và ứng dụng browser dùng engine đó.
- Mục tiêu: tăng tương thích theo từng nhóm tính năng và tập website đo được, dùng Chromium/Playwright làm implementation tham chiếu khi phù hợp.

## Đích đến và giới hạn

“Đạt tới Chromium hoặc Servo” không phải một tiêu chí nghiệm thu đơn lẻ. Chromium là sản phẩm browser và engine có quy mô rất lớn; Servo cũng là một engine đang phát triển với kiến trúc và phạm vi riêng. Kế hoạch này chuyển mục tiêu đó thành các cổng tương thích có thể đo: Goosie phải chạy đúng một tập website và Web Platform Tests được công bố, với sai khác được phân loại và theo dõi.

Không lấy tổng số API, một tỷ lệ WPT chung, hay một lần mở được website làm bằng chứng tương thích. Mỗi đợt phải công bố tập tính năng được hỗ trợ, tập test áp dụng, kết quả Chromium tham chiếu, lỗi còn lại và giới hạn bảo mật.

## Điểm xuất phát đã có

- Pipeline Parse → Style → Layout → Paint, DOM/HTML, cascade CSS, JS qua goja, layout arena và CPU tile renderer.
- Bộ kiểm thử Go theo package, gate headless, fixture render/interaction so với Chromium và WPT runner/pilot.
- Một số layout và CSS hiện đại đã có; đây là các implementation subset, không phải tuyên bố tuân thủ toàn bộ chuẩn.
- Các tương tác cục bộ `:hover`/`:focus`/`:active` có fast path trong một số điều kiện; mutation DOM, selector phức tạp và anonymous box vẫn có đường fallback đầy đủ.

Roadmap sản phẩm và các chi tiết đã kiểm kê nằm ở [roadmap-v2.md](roadmap-v2.md), [PROJECT.md](../PROJECT.md), [WPT pilot](../test/wpt/wpt_pilot_test.go). Không mở lại tính năng đã có; task bên dưới yêu cầu đo coverage hoặc sửa một sai lệch tái hiện được.

## Tiến độ đo được

### 2026-10-24 — CSS color và selector baseline

- WPT checkout được pin tại `11ae8c1179be2dd38070846b01c2e9338ef81770`.
- `css/css-color` + `css/selectors`: 820 test được phát hiện; 352 skip theo support map; trước sửa 451/468 ca áp dụng pass (96.4%), 17 fail; sau sửa 457/468 pass (97.6%), 11 fail, 0 error.
- Sáu lỗi được xử lý: percentage reference range riêng cho Lab/LCH/Oklab/OkLCh và gamut mapping ở lightness endpoint 0/100. Sáu fixture WPT `lch-009/010`, `oklab-009`, `oklch-009/010/011` pass riêng sau rebuild.
- Fixture render cục bộ: 80/80 đạt ngưỡng hiện tại; average pixel match 95.99%. Đây là corpus parity đã chọn, không đại diện toàn bộ trang web.
- Full 10-directory WPT config đang được chạy để lập baseline rộng hơn; kết quả của subset trên chưa được dùng thay cho full-suite coverage.

## Task backlog theo thứ tự phụ thuộc

### E0 — Chốt compatibility target và baseline

**Kết quả:** một tập website/luồng đại diện có đầu vào, thao tác, browser tham chiếu và kết quả mong đợi.

- Chọn 20–30 luồng nhỏ thuộc các nhóm: bài viết/documentation, form, điều hướng SPA, trang có JS, trang responsive, ảnh/font và state lưu bằng cookie.
- Với mỗi luồng, ghi phiên bản môi trường tham chiếu, URL hoặc fixture cục bộ, hành động, DOM/ảnh/trạng thái mong đợi, và điều kiện pass.
- Gắn mỗi sai lệch vào HTML/DOM, JS/event, CSS/style, layout/text, paint, network/storage, input/accessibility hoặc browser chrome.
- Chạy lại baseline trên Goosie và Chromium; không sửa engine trong lúc thu baseline.

**Hoàn thành khi:** mọi luồng có kết quả tái chạy được; lỗi được phân loại và liên kết tới test/fixture; không còn mục “không tương thích” thiếu bước tái hiện.

### E1 — Làm WPT thành vòng phản hồi hằng ngày

WPT pilot và runner đã có; việc cần làm là biến chúng thành báo cáo coverage ổn định, không tạo một harness thứ hai.

- Xác nhận WPT checkout được pin vào commit; lưu config và danh sách test theo nhóm chuẩn.
- Bổ sung report riêng cho pass, fail, timeout, unsupported và test không áp dụng; lưu log/ảnh đầu ra cho mỗi lần chạy.
- Chọn các test đầu tiên theo lỗi E0 và khả năng của engine; bắt đầu với DOM/HTML, selectors/cascade, CSS values, block/inline, form events và navigation primitives.
- Mỗi lỗi đã sửa có fixture hồi quy tối thiểu trong package Go hoặc WPT subset; không đưa test đang fail vào danh sách “unsupported” để làm đẹp tỷ lệ.
- Tạo lệnh chạy một nhóm test ổn định trên CI/headless và ghi baseline theo commit.

**Hoàn thành khi:** cùng commit/config cho cùng kết quả phân loại; CI có thể chạy subset đã chọn; report cho phép mở trực tiếp test thất bại và so sánh ảnh khi test là reftest.

### E2 — Đóng các lỗi nền tảng HTML, DOM và event

- Dùng E0/E1 để chọn lỗi parser/tree construction, DOM API, event propagation/default action và form control có ảnh hưởng lớn nhất.
- Xác định rõ đồng bộ/asynchronous behavior cần hỗ trợ cho luồng mục tiêu; bổ sung microtask checkpoints và thứ tự dispatch nếu test chứng minh hiện sai.
- Bổ sung test cho mutation, focus, keyboard/pointer, submit/reset và lifecycle của document; kiểm tra trạng thái DOM cùng event trace.
- Chỉ sửa subset API có consumer/test cụ thể; ghi rõ API chưa hỗ trợ thay vì trả về kết quả giả thành công.

**Hoàn thành khi:** nhóm test được chọn pass trên Goosie và cùng hành vi với Chromium; lỗi không tái xuất hiện qua test hồi quy.

### E3 — CSS cascade, selector và invalidation

- Lập ma trận selector/pseudo-class/property thực dùng trong tập E0 và WPT subset; phân biệt parse, match và computed-value errors.
- Sửa theo từng nhóm selector và giá trị CSS có test thất bại; kiểm tra specificity, inheritance, source order và dynamic state.
- Thêm dependency tracking cho DOM mutation và selector quan hệ trước khi mở rộng incremental restyle; giữ full-pass fallback nếu chưa chứng minh được tính đúng.
- Đo style visits và layout passes qua counters hiện có; thêm fixture cho một mutation có thể bỏ qua vùng không bị ảnh hưởng.

**Hoàn thành khi:** test CSS mục tiêu pass, computed style và ảnh khớp tham chiếu trong ngưỡng đã đặt cho fixture; fast path có counter test và fallback test.

### E4 — Layout, text và paint parity

- Tạo bộ fixture theo lỗi thực tế cho block/inline, flex, grid, table, positioning, fragmentation/overflow và text shaping.
- Tách sai khác geometry khỏi raster/font sai khác bằng dump layout và ảnh headless; không sửa raster để che sai hình học.
- Chọn font fallback và text shaping theo target macOS; thêm kiểm tra bidi, line breaking, intrinsic sizing và zoom khi có sai lệch tái hiện.
- Chỉ mở rộng layout incremental/anonymous-box updates sau khi fixture chứng minh geometry và paint giữ đúng.

**Hoàn thành khi:** mỗi nhóm có subset test ổn định, ảnh so với Chromium được lưu, sai khác ngoài ngưỡng có report và fixture tái hiện.

### E5 — JavaScript và Web APIs theo website mục tiêu

- Lập danh sách API JS mà E0 thực sự cần; đánh dấu hỗ trợ đầy đủ, hỗ trợ một phần hoặc chưa hỗ trợ.
- Ưu tiên semantics của runtime/event loop trước khi thêm API bề mặt: exceptions, promises/microtasks, timers, URL/URLSearchParams, Fetch và observer APIs nếu test yêu cầu.
- Thêm test unit cho API, test tích hợp từ script trong document và test hành trình website; kiểm tra thứ tự tác dụng phụ.
- Không xem việc goja chạy được cú pháp JavaScript là tương đương với Web APIs của browser.

**Hoàn thành khi:** các luồng JS chọn ở E0 pass tự động; mọi API được dùng có contract/test và mọi giới hạn được báo rõ.

### E6 — Mạng, origin và lưu trữ

- Xây ma trận điều hướng HTTP(S), redirect, URL encoding, cookie, cache và lỗi tải dựa trên E0/E1.
- Thêm test cho same-origin/CORS và chính sách initiator ở các đường fetch hiện có; giữ private-address guard và các admission limit.
- Kiểm tra cookie/session persistence qua tab, reload và restart; xác nhận private session không ghi vào profile thường.
- Tách lỗi giao thức khỏi UI loading/error state để cùng một fixture kiểm tra được cả response và giao diện.

**Hoàn thành khi:** luồng có network/storage được chạy bằng server test cục bộ, trạng thái sau restart được xác nhận, và các test security/limits vẫn pass.

### E7 — Input, accessibility và browser integration

- Chạy lại luồng E0 bằng bàn phím và chuột; bổ sung test cho Tab/focus, IME, selection, form activation và accessibility tree.
- Đối chiếu semantic roles/name/state với DOM và thao tác thực tế; không chỉ kiểm tra số lượng node accessibility.
- Mỗi thay đổi giao diện có native macOS verification; giữ headless làm kiểm tra engine và hành trình có thể tự động hóa.

**Hoàn thành khi:** luồng mục tiêu có thể hoàn tất bằng chuột, bàn phím và accessibility tree với trạng thái nhất quán.

### E8 — Hiệu năng và tính đúng của pipeline

- Ghi baseline counters cho style visits, layout passes, allocations, tiles rasterized/reused trên các hành trình E0.
- Đặt ngân sách theo fixture/counters, không dùng timing assertions trong gate CI.
- Tối ưu theo profile: DOM mutation restyle, selector dependency, local layout, display-list invalidation và tile reuse; mỗi tối ưu đi cùng test correctness và fallback.
- Theo dõi memory/CPU trên tập trang mục tiêu và kiểm tra admission limits với input lớn.

**Hoàn thành khi:** mỗi tối ưu cải thiện một counter hoặc benchmark được xác định trước và không làm giảm kết quả parity/security.

## Cổng phát hành compatibility

Không gộp các giai đoạn thành một nhãn “Chrome-compatible”. Với mỗi mốc phát hành, công bố:

1. Danh sách tính năng và website/luồng được hỗ trợ.
2. WPT subset, commit/config, pass/fail/skip/timeout và xu hướng so với baseline.
3. Kết quả ảnh/interaction parity so với Chromium cho fixture liên quan.
4. Lỗi đã biết, API chưa hỗ trợ, giới hạn resource và kết quả security tests.
5. Kết quả `go test ./...`, race, vet, archtest, gate, e2e và native macOS khi thay đổi đụng tới UI.

## Việc bắt đầu tiếp theo

1. Hoàn tất E0: chốt tập website/luồng cùng người dùng sản phẩm và lưu từng luồng thành fixture tái chạy được.
2. Hoàn tất E1 phần report: lưu report WPT đầy đủ, phân loại 20–50 lỗi đầu tiên theo mức ảnh hưởng tới E0 và bổ sung allowlist cho các luồng CI ổn định.
3. Chọn một nhóm lỗi nhỏ ở E2 hoặc E3, sửa theo test đỏ → xanh, rồi chạy lại e2e, WPT subset và parity tương ứng.

Sau baseline này mới có thể ước lượng thứ tự và kích thước sprint đáng tin cậy. Không nên đặt mục tiêu tổng thể “bằng Chromium/Servo” thành một task duy nhất.
