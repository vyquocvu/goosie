# Goosie — kế hoạch QA và bộ test case

- Cập nhật: 2026-10-22
- Đối tượng: bản Goosie cho macOS và đường render headless dùng chung engine.
- Mục đích: xác nhận các hành trình duyệt cốt lõi, điều tra lỗi render nhấp nháy trên https://about.google/products/, và tạo tiêu chí phát hành có bằng chứng.
- Bộ case chi tiết: [qa-test-catalog.md](qa-test-catalog.md), gồm 202 case theo tính năng; 15 QA case trong tài liệu này là kịch bản release tổng hợp.
- Trạng thái lần chạy: smoke CUA QA-001/QA-002 ngày 2026-10-21; kiểm thử tự động toàn repo và Chromium render parity chạy lại ngày 2026-10-22. Native CUA cho bookmark/history bị BLOCKED vì không có display window khả dụng trong phiên này.

## 1. Phạm vi

Kiểm tra các hành trình đang được roadmap ưu tiên: mở trang, điều hướng, render và scroll, tương tác bàn phím/biểu mẫu, tab, trạng thái tải, dữ liệu phiên, accessibility và các giới hạn an toàn đầu vào. Dùng CUA để thao tác bản Goosie native và lưu bằng chứng; dùng headless/Go tests cho các trường hợp cần lặp lại ổn định.

Trang Google Products là **smoke test live**, không phải fixture CI: nội dung, ảnh, script và animation của trang có thể thay đổi. Nếu tái hiện lỗi, phải cô lập thành fixture cục bộ tối thiểu trước khi chấp nhận bản sửa. Corpus có fixture nhỏ cho WebP và các lỗi CSS liên quan; fixture không đại diện toàn bộ trang live.

Không dùng kết quả của một trang live để tuyên bố Goosie tương thích toàn bộ web. Mỗi hành trình phát hành phải nêu rõ phiên bản Goosie, macOS, kích thước cửa sổ, cài đặt zoom/profile, URL và phần chưa hỗ trợ.

## 2. Mức ưu tiên và cách thực hiện

| Mức | Ý nghĩa | Điều kiện phát hành |
| --- | --- | --- |
| P0 | Không mở/đọc/điều hướng được, treo/crash, hoặc rò dữ liệu giữa phiên | Không còn lỗi mở ở luồng P0 |
| P1 | Sai render/nhấp nháy, thao tác thường dùng sai, tải/khôi phục làm mất trạng thái | Không còn lỗi P1 ảnh hưởng luồng công bố |
| P2 | Sai lệch ít gặp hoặc có cách khắc phục rõ ràng | Ghi nhận, đánh giá và có kế hoạch xử lý |

Các trạng thái phải được ghi theo lần chạy: **NOT RUN**, **PASS**, **FAIL**, **BLOCKED**, **DEFERRED**. BLOCKED chỉ dùng khi môi trường/điều kiện ngoài sản phẩm ngăn việc chạy; DEFERRED dùng cho case gắn với khoảng trống trong roadmap, chưa được tính là PASS hoặc nằm trong gate của build hiện tại. Thiếu bằng chứng không phải PASS.

### Tầng kiểm thử

1. **Unit/integration:** kiểm tra logic engine, toolbar, tabs, network, download và session bằng Go tests.
2. **Gate/frame path:** chạy headless qua scheduler, composer và Present; xét counters và lỗi worker/tile. Không thêm ngưỡng thời gian vào gate CI; timing budget thuộc macOS nightly scripts/v2-gate-check.sh.
3. **Visual/interaction parity:** render fixture hoặc event fixture so với Chromium/Playwright bằng testdata/parity.py và test/engine/parity_interaction_test.go.
4. **Native CUA:** điều khiển cửa sổ Goosie bằng chuột/bàn phím, chụp bằng chứng trước/sau thao tác. CUA kiểm tra ứng dụng thực tế, không thay thế unit/gate.
5. **Live smoke:** mở trang Google Products sau khi test local ổn định; kết quả live phải ghi giờ chạy và trạng thái mạng.

### Môi trường và dữ liệu

- macOS ARM64; chạy ứng dụng ở cửa sổ cố định 1440×900 logical pixels nếu máy kiểm thử hỗ trợ; zoom 100%, theme mặc định, profile test mới.
- Headless parity mặc định 800×600, DPR 1. Chạy từ root repo.
- Local render fixtures: testdata/render/complex/04-login-form.html, testdata/render/edge-cases/01-long-content.html, testdata/render/layout-features/04-positioning.html.
- Interaction fixtures: testdata/interaction-parity/05-focus-tab.html, 07-keyboard.html và dữ liệu sự kiện tham chiếu trong cùng thư mục.
- Live smoke: https://about.google/products/; không dùng profile chứa cookie hoặc dữ liệu cá nhân.
- Cất ảnh/chụp màn hình và log theo artifacts/qa/<YYYY-MM-DD>/<TC-ID>/; mỗi FAIL cần bước tái hiện ngắn nhất, expected/actual, ảnh, log và build SHA. Không ghi cookie, token hay dữ liệu cá nhân vào evidence.

## 3. Kịch bản và test case

### QA-001 — Khởi chạy và mở URL trực tiếp (P0, Native CUA)

**Tiền điều kiện:** build Goosie mới nhất; profile test sạch; CUA nhận diện được cửa sổ app.

**Bước:** khởi chạy Goosie → focus thanh địa chỉ → nhập https://example.com/ → Enter → chờ trạng thái tải kết thúc → chụp màn hình và ghi URL/tiêu đề.

**Kỳ vọng:** ứng dụng không crash/treo; thanh địa chỉ và tài liệu cùng trỏ tới URL cuối; nội dung có thể đọc; trạng thái loading kết thúc hoặc lỗi được thể hiện rõ. Kết quả phải có ảnh.

**Automation/bằng chứng:** CUA native; liên quan cmd/goosie/main_test.go và test/entrypoints/limits_test.go.

**Kết quả lần chạy 2026-10-21:** **PASS hành vi và khả năng đọc; sign-off còn thiếu artifact** — Goosie mở `https://example.com/`, thanh địa chỉ khớp và accessibility tree có nội dung. Ảnh native CUA trước sửa cho thấy nền tối cùng chữ tối, khó đọc; sau sửa `color-scheme: light dark` tôn trọng viewport preference và ảnh build mới có nền sáng/chữ tối dễ đọc. Fixture cục bộ đạt 99.351% match với Chromium ở light mode, nền đúng `#eeeeee`. CUA ảnh live chưa được lưu thành artifact; profile đã hiện màn khôi phục phiên và được chọn “Don't Restore”, nên chưa đủ release sign-off.

### QA-002 — Tái hiện render nhấp nháy trên Google Products (P1, Native CUA / Live)

**Tiền điều kiện:** QA-001 PASS; ghi lại macOS, kích thước cửa sổ, zoom, thời gian và tình trạng mạng; profile sạch.

**Bước:** mở https://about.google/products/ → giữ viewport và zoom cố định → đợi trang hết trạng thái tải ban đầu → chụp liên tiếp 5 ảnh cách nhau khoảng 1 giây, không cuộn hoặc di chuyển con trỏ lên nội dung → xem riêng header/nav và các card tĩnh; sau đó cuộn xuống một màn hình và lặp lại.

**Kỳ vọng:** vùng tĩnh không nháy, mất ảnh, đổi trắng/đen hoặc biến mất rồi xuất hiện lại giữa các ảnh. Bỏ qua caret, con trỏ, animation có chủ ý và nội dung live; ghi rõ vùng bị loại khỏi so sánh. Nếu ảnh/section thiếu ổn định hoặc nhấp nháy, FAIL và lưu chuỗi ảnh cùng console/log nếu có.

**Kết quả lần chạy 2026-10-21:** **PARTIAL — lỗi ảnh thiếu đã sửa; nhấp nháy chưa xác nhận.** Ban đầu trang thiếu hero và nhiều ảnh sản phẩm vì pipeline chỉ đăng ký GIF/PNG/JPEG trong khi trang dùng WebP; thêm decoder WebP có giới hạn kích thước/pixel và regression test cho ảnh hợp lệ 4×4 cùng header vượt ngưỡng 8193×1. Fixture `testdata/render/edge-cases/07-webp-image.html` so với Chromium đạt 99.62%. Retest Goosie native sau rebuild cho thấy logo sản phẩm và hero Pixel xuất hiện; Page Down chuyển tới phần nội dung dưới. Ảnh live headless lưu tại `artifacts/qa/2026-10-21/GOOSIE-RENDER-001/google-products-headless-after-webp.png`. Parser border fix vẫn được giữ: `border: 0 solid red` không còn tự nâng thành 3px; fixture `05-zero-border.html` đạt 98.85% và có 0 pixel viền đỏ.

Nhấp nháy theo chu kỳ **chưa PASS/FAIL được xác định**: các ảnh PNG CUA liên tiếp có byte hash khác nhau, nhưng hash file không chứng minh pixel vùng nội dung đổi và trước đó cả control `example.com` cũng cho kết quả như vậy. CUA lần này xác nhận cuộn bằng Page Down; pixel capture ổn định trong cùng vùng tĩnh vẫn cần thực hiện trên viewport/profile kiểm soát được. Giữ `GOOSIE-RENDER-001` OPEN tới khi QA-002 được xác nhận bằng frame comparison hoặc lỗi tái xuất hiện được cô lập. Kết quả live không đại diện mọi trang web.

**Bug đang mở:** `GOOSIE-RENDER-001` (P1) — Google Products ban đầu mất ảnh WebP và vẽ viền sai. Ba regression fix hiện có gồm explicit zero border, viewport preference cho `color-scheme: light dark`, và WebP decode an toàn. Native retest xác nhận hero cùng product icons đã hiển thị; chưa có frame-by-frame pixel evidence để đóng phần báo cáo nhấp nháy. Không đóng bug chỉ dựa trên DOM/accessibility tree hoặc byte hash PNG.

**Ghi chú:** đây là smoke test live, không phải fixture CI. Để xác nhận fix, cần rút gọn trang thành fixture local ổn định và so sánh ảnh headless/reference. Nếu chỉ không tái hiện ở một lần chạy live, giữ trạng thái chưa xác nhận.

## Kết quả smoke CUA — 2026-10-21

| Test case | Trạng thái | Kết quả quan sát |
| --- | --- | --- |
| QA-001 | PASS hành vi/visual; artifact thiếu | Điều hướng, accessibility tree và độ tương phản trên native build cuối đều đúng; fixture headless/Chromium đạt 99.351%. Ảnh live chưa lưu và profile không sạch nên chưa sign-off. |
| QA-002 | PARTIAL; render fixes retested | Border fix và WebP decode retested. Goosie native hiển thị hero Pixel cùng product icons; Page Down cuộn tới nội dung tiếp theo. Flicker theo pixel chưa xác nhận vì hash PNG thay đổi cả ở control tĩnh; cần pixel-level capture ổn định. |
| QA-003 | PARTIAL | Regression style/paint PASS (98.846%); color-scheme parity PASS (99.351%); idle repaint và resize native chưa chạy. |
| QA-004 | NOT RUN | Mới cuộn để quan sát QA-002; chưa chạy chuỗi scroll có kiểm soát trên fixture dài. |
| QA-005 | NOT RUN | Chưa kiểm tra Back/Forward, anchor và Stop. |
| QA-006 | NOT RUN | Chưa chạy điều hướng chéo nhiều tab. |
| QA-007 | NOT RUN | Chưa kiểm tra form và focus bằng bàn phím. |
| QA-008 | NOT RUN | Chưa kiểm tra resize và zoom. |
| QA-009 | NOT RUN | Chưa chạy tải ảnh chậm/lỗi/quá giới hạn. |
| QA-010 | NOT RUN | Chưa chạy redirect, lỗi mạng và Stop có kiểm soát. |
| QA-011 | NOT RUN | Chưa chạy download và khử trùng tên file. |
| QA-012 | NOT RUN | Chưa kiểm tra khôi phục session/private. |
| QA-013 | NOT RUN | Chưa kiểm tra accessibility/keyboard trên fixture riêng. |
| QA-014 | NOT RUN | Chưa chạy các case giới hạn và bảo vệ mạng cục bộ trong phiên này. |
| QA-015 | PASS render corpus | `make parity` chạy lại ngày 2026-10-22: 80/80 fixture PASS, trung bình 95.99%, threshold 90%; fixture zero-border 98.85%, color-scheme 99.35%, WebP 99.62%. Interaction parity chưa chạy. |

Môi trường ghi nhận: macOS, Goosie chạy từ `bin/Goosie.app` được build lại với các sửa cuối; CUA dùng profile đã tồn tại, không phải profile sạch. `go test ./...`, `go vet -all ./...`, `go test ./internal/archtest/` và `go test -race ./internal/image ./test/engine` đều PASS; `make parity` PASS 80/80 (95.99%). `scripts/run-parity.sh` chạy riêng fail-closed nếu Chromium references `/tmp/playwright-renders/` chưa tồn tại; `make parity` tự tạo references và chạy thành công. Ảnh native CUA được quan sát trong phiên; headless live screenshot sau WebP và fixture references lưu tại `artifacts/qa/2026-10-21/GOOSIE-RENDER-001/`.

## Kết quả automated + native QA — 2026-10-22

Trên worktree hiện tại, các kiểm tra chạy lại đều PASS: `go test ./...`, `go test -race ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test ./test/gate -run TestGate -v`, `git diff --check` và `gofmt -l` trên các file Go thay đổi không trả về lỗi. `make parity` so sánh corpus với Chromium đạt **80/80 fixture, 95.99% average match** (ngưỡng pass 90%). Các test mới cho store bookmark/history và management pages (`go test ./internal/bookmarks ./internal/history ./test/bookmarks ./test/history ./cmd/goosie -count=1`) cũng PASS. Build native `make macos-app` thành công.

Native CUA cho trang quản lý chưa có bằng chứng: Goosie process được khởi chạy nhưng CUA trả `cgWindowNotFound`; `screencapture` không tạo được ảnh vì phiên hiện tại không có display khả dụng, còn truy vấn cửa sổ qua System Events bị macOS từ chối do thiếu Assistive Access. Do đó không đánh dấu BR-050/051/052 PASS. Automated checks của BR-050/051 đã PASS; native thao tác và visual comparison riêng cho `goosie://bookmarks` / `goosie://history` vẫn BLOCKED. `make parity` chỉ chấm fixture corpus hiện hữu, chưa có Chromium reference riêng cho hai trang nội bộ. BR-052 cũng còn thiếu mục chuyên dụng trong main menu.

### QA-003 — Trang tĩnh không thay đổi sau first paint (P1, Engine + CUA)

**Tiền điều kiện:** fixture tối giản có nền, chữ, border và ảnh tĩnh; nếu chưa có fixture, tạo một fixture regression từ bước rút gọn của QA-002.

**Bước:** mở fixture bằng Goosie → chụp sau first paint → để trang idle → chụp lại cùng viewport → lặp với resize nhỏ rồi trả kích thước ban đầu.

**Kỳ vọng:** vùng nội dung tĩnh không tự nháy hoặc bị clear; sau resize layout/tiles đúng, không để lại pixel của kích thước cũ. Engine test cho RAF không thay đổi phải bảo đảm không phát sinh repaint vô ích.

**Automation hiện có:** test/engine/adversarial_regression_test.go:TestPumpJSNoopRAFDoesNotRepaint; test/engine/pump_test.go:TestPumpJSCanvasRAFRepaintsOnlyWhenDrawingChanges.

### QA-004 — Scroll dài, PageDown/PageUp và tile reuse (P1, Native CUA + Gate)

**Tiền điều kiện:** trang dài với nội dung đủ phân biệt ở đầu/giữa/cuối (edge-cases/01-long-content.html hoặc trang sản phẩm).

**Bước:** chụp đầu trang → PageDown 2 lần nhanh → quan sát/chụp viewport → PageUp → cuộn bằng trackpad/scroll wheel đến gần cuối → quay lên đầu.

**Kỳ vọng:** mỗi thao tác tiến/lùi đúng hướng; không nhảy ngược bất thường; không có dải trắng, tile cũ hoặc frame nhấp nháy; nội dung và vị trí scroll của tab vẫn đúng. Đối với trang có input đang focus, PageUp/PageDown không làm scroll document khi phím thuộc control.

**Automation hiện có:** cmd/goosie/main_test.go:TestContentKeyPageNavigationScrollsDocument, TestContentKeyPageNavigationDoesNotScrollFocusedEditable; test/raster/scheduler_test.go:TestScrollDamagesOnlyTheExposedBand, TestDefaultVerticalPrefetchSettlesWithinLandscapeCacheBudget; gate suite xét counters, không thời lượng.

### QA-005 — Link, anchor, Back/Forward và reload (P0, Native CUA + Integration)

**Tiền điều kiện:** trang local có liên kết nội bộ, anchor và liên kết tới trang thứ hai.

**Bước:** click link → ghi URL/tiêu đề → Back → Forward → click anchor → Reload → khi tải, Stop rồi mở lại trang.

**Kỳ vọng:** URL, tài liệu và nút Back/Forward nhất quán; anchor cuộn đến đúng mục; reload không tạo nhánh history giả; Stop chỉ dừng navigation đang chạy và không ghi kết quả cũ đè tài liệu hiện hành.

**Automation hiện có:** cmd/goosie/tab_lifecycle_test.go:TestStopTabCancelsOnlyActiveNavigationAndKeepsDisplayedDocument, TestStaleNavResultCannotPaintOverTheCurrentOne, TestCurrentNavResultIsApplied; test/toolbar/toolbar_test.go:TestHistoryBackForward.

### QA-006 — Hai tab và kết quả tải về đúng tab (P0, Native CUA + Integration)

**Tiền điều kiện:** hai URL local khác nhau, một URL phản hồi chậm.

**Bước:** mở URL A → tạo tab B và điều hướng B → quay lại A → bắt đầu điều hướng chậm ở A → chuyển B → đợi A hoàn tất → chuyển lại A → đóng B.

**Kỳ vọng:** nội dung, URL, title, scroll và history độc lập theo tab; kết quả tải chậm không che tab đang hoạt động; đóng/chuyển tab không crash hoặc làm đổi history tab còn lại.

**Automation hiện có:** cmd/goosie/tab_lifecycle_test.go:TestIndependentTabNavigation, TestBackgroundTabResultLeavesTheForegroundAlone, TestOutOfOrderResultsLandOnTheirOwnTabs; test/tabs/lifecycle_test.go:TestTabHistoryIsolation.

### QA-007 — Bàn phím, focus và biểu mẫu (P1, Native CUA + Interaction parity)

**Tiền điều kiện:** testdata/render/complex/04-login-form.html hoặc fixture form local có text input, checkbox, select, submit và phản hồi trạng thái.

**Bước:** dùng Tab/Shift+Tab để đổi focus → nhập text → toggle checkbox → chọn option → submit bằng Enter → lặp submit bằng click → kiểm tra focus sau lỗi validation.

**Kỳ vọng:** focus đi theo thứ tự nhìn thấy; giá trị không mất/nhân đôi; Enter kích hoạt submit hợp lệ; checkbox/select thay đổi đúng; lỗi validation giữ được nội dung nhập. Không tự gửi form khi chỉ đang focus control không liên quan.

**Automation hiện có:** test/engine/keyboard_test.go, test/engine/forms_test.go, test/engine/interaction_e2e_test.go; parity fixture testdata/interaction-parity/05-focus-tab.html, 07-keyboard.html.

### QA-008 — Resize và zoom không để lại ảnh cũ (P1, Native CUA + Integration)

**Tiền điều kiện:** trang có text wrap, flex/grid và nội dung cuộn.

**Bước:** resize cửa sổ rộng → hẹp → rộng → Cmd+Plus vài mức → Cmd+0 reset → kiểm tra đầu và cuối tài liệu.

**Kỳ vọng:** layout reflow đúng kích thước/zoom; không còn vùng bị cắt, chồng lấn hoặc pixel cũ; vị trí đọc không nhảy bất ngờ ngoài hành vi đã định; zoom indicator và reset đúng.

**Automation hiện có:** cmd/goosie/resize_reflow_test.go:TestResizeReflow; cmd/goosie/find_and_zoom_test.go:TestZoomBadgeIndicatorAndReset, TestZoomKeyboardShortcuts.

### QA-009 — Ảnh tải trễ, lỗi ảnh và ảnh vượt giới hạn (P1, Integration + CUA)

**Tiền điều kiện:** fixture local/server có ảnh chậm, URL ảnh lỗi và ảnh header khai báo kích thước/pixel vượt giới hạn.

**Bước:** mở trang khi ảnh đang tải → quan sát sau khi ảnh về → tải lại với URL ảnh lỗi → thử ảnh vượt ngưỡng.

**Kỳ vọng:** ảnh đến sau làm reflow/repaint đúng, không ghi đè navigation mới; ảnh lỗi không làm hỏng phần còn lại của trang; ảnh vượt giới hạn bị từ chối trước decode, không crash hoặc treo.

**Automation hiện có:** cmd/goosie/progressive_images_test.go; test/engine/deferred_images_test.go; test/engine/adversarial_test.go:TestOversizedImageHeaderRejectedBeforeDecode.

### QA-010 — Redirect, lỗi mạng và Stop (P1, Native CUA + Integration)

**Tiền điều kiện:** server local có redirect hợp lệ, redirect HTTPS→HTTP để từ chối, endpoint lỗi và endpoint chậm.

**Bước:** mở URL redirect → mở endpoint lỗi → bắt đầu endpoint chậm và nhấn Stop → điều hướng sang URL khác ngay khi request cũ còn chạy.

**Kỳ vọng:** URL cuối của redirect khớp nội dung/history; lỗi mạng có trạng thái quan sát được; Stop dừng đúng tab; response cũ không ghi đè nội dung mới.

**Automation hiện có:** cmd/goosie/main_test.go:TestLoadURLCtxReportsFinalURLAfterRedirect; test/net/http_test.go:TestHTTPRedirectValidation, TestHTTPSDowngradeRedirectRejected; các case stale-nav ở QA-005/006.

### QA-011 — Download và file trùng tên (P1, Native CUA + Integration)

**Tiền điều kiện:** server local phục vụ một attachment có tên file và tải lặp lại cùng attachment.

**Bước:** click link tải → mở danh sách Downloads → tải lại file cùng tên → mở vị trí file.

**Kỳ vọng:** response attachment được lưu thay vì render như trang; tên file được sanitize; file thứ hai không ghi đè file thứ nhất; đường dẫn/kết quả tải được phản ánh đúng.

**Automation hiện có:** test/download/download_test.go:TestSaveWritesFile, TestSaveDeduplicatesNames, TestFileNameFallbackAndSanitize; cmd/goosie/downloads_test.go:TestDownloadsPageRenderingAndActions.

### QA-012 — Session thường, khởi động lại và private (P1, Native CUA + Integration)

**Tiền điều kiện:** profile tạm, hai tab thường và một tab private nếu UI build cung cấp hành trình này.

**Bước:** mở nhiều URL thường → khởi động lại → xác nhận session → mở private tab và truy cập trang → đóng app → kiểm tra session/history thường và private.

**Kỳ vọng:** tab thường phục hồi theo phạm vi session đã công bố; dữ liệu private không được ghi vào session/history thường; tab thường không mất history do hoạt động private.

**Automation hiện có:** test/session/restore_test.go; cmd/goosie/incognito_test.go:TestIncognitoSessionSuppression, TestIncognitoHistorySuppression.

### QA-013 — Accessibility tree và điều khiển bằng bàn phím (P1, Integration + CUA)

**Tiền điều kiện:** trang có heading, link, ảnh có alt, button có label và trường nhập.

**Bước:** kiểm tra accessibility snapshot/internal inspector → đi qua các control chỉ bằng bàn phím → kích hoạt link/button.

**Kỳ vọng:** role/name/value/href được công bố chính xác; control tương tác được bằng bàn phím; focus hiện rõ và không bị kẹt.

**Automation hiện có:** test/engine/accessibility_test.go; cmd/goosie/main_test.go:TestPublishAccessibilityWithoutTabManagerClearsTree.

### QA-014 — Giới hạn tài liệu và bảo vệ mạng cục bộ (P0, Security/Integration)

**Tiền điều kiện:** fixture lớn vượt document cap; HTML chứa nhiều node/depth; public-origin page có subresource trỏ vào địa chỉ private/local.

**Bước:** mở từng đầu vào bằng entrypoint phù hợp → theo dõi process và thông báo lỗi → kiểm tra request local không được phát từ public initiator.

**Kỳ vọng:** nội dung quá giới hạn bị từ chối có kiểm soát trước khi gây tiêu thụ tài nguyên quá mức; PaintChecked được dùng cho binary xử lý input không tin cậy; public-origin page không truy cập private network; app vẫn dùng được sau lỗi.

**Automation hiện có:** test/entrypoints/limits_test.go; test/engine/private_network_test.go:TestPublicPageCannotFetchLocalNetworkImage; cmd/goosie/address_guard_test.go.

### QA-015 — Visual parity corpus và tương tác tham chiếu (P1, Automated)

**Tiền điều kiện:** Chromium/Playwright và các thư viện Python theo scripts/run-parity.sh đã cài; tạo render tham chiếu trước khi chấm.

**Bước:** chạy parity theo script; xem JSON/JUnit và ảnh diff cho fixture FAIL; chạy tương tác parity.

**Kỳ vọng:** script fail-closed khi thiếu render/prerequisite; fixture và interaction có kết quả xác định; không bỏ qua lỗi do thiếu ảnh tham chiếu. Chỉ chấp nhận sai khác đã được giải thích và duyệt theo fixture cụ thể.

**Lệnh:** scripts/run-parity.sh -artifacts artifacts/qa/parity; go test ./test/engine -run 'TestParity' -count=1.

## 4. Bộ lệnh xác nhận trước phát hành

Chạy trên commit sẽ phát hành, lưu stdout/stderr và exit code:

~~~sh
go test ./...
go test -race ./...
go vet -all ./...
go test ./internal/archtest/
go test ./test/gate -run TestGate -v
make gate
~~~

Nếu thay đổi render/UI, bổ sung visual parity và CUA native cho các TC liên quan. Chạy WPT pilot khi thay đổi web-platform behavior: make wpt-pilot. Chạy parity chỉ khi Chromium renders và prerequisites đã sẵn sàng; thiếu prerequisite là BLOCKED, không phải PASS. Gate CI không được thêm assertion thời lượng; nightly macOS kiểm tra timing qua scripts/v2-gate-check.sh.

## 5. Definition of Done — QA sign-off

- [ ] Các case P0/P1 trong [catalog BR-001–BR-185](qa-test-catalog.md) thuộc phạm vi release đã có kết quả và bằng chứng; case DEFERRED có roadmap item/owner và không được tính là PASS.
- [ ] Build SHA, OS, viewport, zoom, profile, URL/fixture và trạng thái mạng được ghi cho từng lượt.
- [ ] QA-001, QA-004, QA-005, QA-006 và QA-014 đạt trên môi trường đại diện; không còn P0 mở.
- [ ] Lỗi nhấp nháy QA-002 được tái hiện hoặc có bằng chứng không tái hiện được; nếu tái hiện, có fixture regression ổn định trước khi đóng lỗi.
- [ ] Mọi sửa đổi render có kiểm chứng headless/reference; không chỉ dựa vào một ảnh CUA hoặc một URL live.
- [ ] go test ./..., go test -race ./..., go vet -all ./..., archtest và gate đạt trên commit ứng viên.
- [ ] Mọi P1 còn lại có người phụ trách, mức ảnh hưởng, workaround và quyết định phát hành rõ ràng; P1 ảnh hưởng hành trình cốt lõi không được chấp nhận âm thầm.
- [ ] Không có tile raster fail/worker panic trong smoke; không vượt giới hạn đầu vào; không có rò private state.
- [ ] Bằng chứng CUA/parity, log và kết quả test được lưu cùng build SHA; test live được đánh dấu riêng, không thay thế regression CI.
- [ ] Roadmap docs/roadmap-v2.md được cập nhật riêng khi phạm vi sản phẩm thay đổi; tài liệu này chỉ ghi coverage và kết quả QA.

## 6. Phiếu ghi nhận lỗi

~~~text
ID / mức độ:
Build SHA / macOS:
Viewport / zoom / profile:
Fixture hoặc URL / thời gian chạy:
Tiền điều kiện:
Bước tái hiện:
Expected:
Actual:
Tần suất (số lần tái hiện / số lần chạy):
Ảnh/video/log:
Regression test hoặc fixture đề xuất:
~~~
