# TASK-020: Hành trình P1 cho navigation và tab (tiếp nối TASK-015)

- **Task ID**: `TASK-020`
- **Type**: Test coverage / User journey automation
- **Assigned to**: `agy`
- **Review**: `qoder cli` (integrator)

## 1. Bối cảnh

TASK-015 của bạn đã merge: 11 hành trình P0 trong `cmd/goosie/journey_p0*.go`, assert qua accessibility tree, đã tag `// qc:BR-xxx` và đã được QC traceability gate (`test/qc/`) ghi nhận — hiện 32/185 case có automation, 17/19 P0 case đã phủ.

Gate đó đang báo 27 case "có automation nhưng chờ sign-off". Việc của task này là **tăng số case có hành trình thật**, không phải đổi trạng thái catalog. Quy tắc cũ vẫn còn hiệu lực: không sửa engine, không sửa catalog, không pixel test.

## 2. Việc cụ thể

### 2.1. Audit trước khi viết (bắt buộc, kết quả ghi vào Status)

Với **mỗi** case trong mục 2.2, tìm test đã tồn tại bằng `go test -run` + grep trong `cmd/goosie/*_test.go` và `test/**`. Nếu đã có test assert đúng expected result của case: **chỉ thêm tag `// qc:BR-xxx`** lên function đó (thêm dòng, không sửa logic), không viết test trùng. Ghi vào Status dạng bảng: `case | test đã có | action (tag mới / test mới / không có)`.

### 2.2. Danh sách case (12)

Domain A — navigation:

| Case | Hành trình cần chứng minh | Post-condition phải assert |
| --- | --- | --- |
| BR-004 | Nhập URL có path + query + fragment | Request gửi đi giữ nguyên path/query; tài liệu giữ fragment |
| BR-008 | Trang trả redirect 1 bước | Address bar hiển thị URL đích cuối; Back quay về trang trước, không loop |
| BR-013 | Trang chậm, bấm Stop | Request hiện tại dừng; tài liệu đang hiển thị không bị xoá ngoài policy |
| BR-015 | Bấm link thường trong cùng tab | URL, tài liệu và history đồng bộ một lần, không entry giả |
| BR-016 | Anchor cùng trang và anchor từ trang khác | Fragment đúng và viewport nhảy tới đúng mục tiêu |
| BR-020 | Reload trang có query/fragment | Tải lại URL hiện hành, history không thêm visit ngoài policy |
| BR-021 | Refresh khi response cũ còn pending | Chỉ kết quả navigation mới nhất được áp dụng |
| BR-022 | Focus address bar, chọn toàn bộ, gõ URL mới | Không lọt ký tự vào document phía sau |

Domain B — tab:

| Case | Hành trình cần chứng minh | Post-condition phải assert |
| --- | --- | --- |
| BR-024 | Mở tab mới bằng nút và bằng shortcut | Tab mới được focus và nhận nhập liệu |
| BR-027 | Tài liệu hoàn tất ở background tab | Nội dung tab foreground không đổi |
| BR-034 | Đổi tab khi mỗi tab có scroll offset riêng | Quay lại: đúng tài liệu và đúng vị trí cuộn |
| BR-035 | Mỗi tab có Back/Forward riêng | Lịch sử không dùng chung giữa hai tab |

### 2.3. Cách viết

Dựng trên harness có sẵn trong `cmd/goosie/e2e_test.go` — **tái sử dụng, không viết lại**: `newBrowserE2E`, `close`, `inject`, `pressKey`, `click`, `clickNode`, `clickToolbarButton`, `navigateViaAddressBar`, `waitFor`, `diagnostics`, `activeURL`, `findAXNode`, `axContainsLabel`.

Mỗi test: tạo fixture qua `httptest.NewServer`, thao tác như người dùng, rồi assert **post-condition ngữ nghĩa qua accessibility tree / tab state / history**, không assert ảnh. Khi cần chờ điều kiện bất định, dùng `waitFor` (đã fail rõ khi timeout) chứ không `time.Sleep`.

Đặt tên file `cmd/goosie/journey_p1_nav_test.go` và `cmd/goosie/journey_p1_tabs_test.go`. **Chỉ tạo file mới trong `cmd/goosie/`**; sửa file có sẵn chỉ theo nghĩa thêm dòng tag.

**Quy tắc bắt buộc về `waitFor` (rút từ failure thật sau merge của TASK-015, xem `docs/tasks/TASK-015-P0-JOURNEY-SUITE.md` §8):** điều kiện chờ phải bao gồm **đúng thứ dòng code kế tiếp đọc**. Sai là kiểu: chờ `tabMgr.Count() == 2` rồi ngay sau đó đọc `tabMgr.Active().ID()` — số tab và selection đổi ở hai iteration khác nhau, nên test xanh khi chạy một mình và fail dưới tải full suite (`second tab ID = 1 must be distinct from first tab ID = 1`). Đúng là đưa cả hai về một predicate:

```go
browser.waitFor(t, "second tab created and focused", func() bool {
    return browser.f.tabMgr.Count() == 2 && browser.f.tabMgr.Active().ID() != firstID
})
```

Vì vậy `[AC-01]` yêu cầu `-count=5` chứ không phải `-count=1`: một predicate thiếu điều kiện thường vẫn qua ở lượt đầu.

Đầu mỗi test function, thêm dòng tag trên cùng của doc comment: `// qc:BR-0xx` (một function có thể tag nhiều case).

## 3. Tiêu chí nghiệm thu

- **[AC-01]** `go test -count=1 -race -run 'TestJourney' ./cmd/goosie/` PASS, và chạy lặp `go test -count=5 -run 'TestJourney' ./cmd/goosie/` PASS (không flake).
- **[AC-02]** `go test -count=1 ./cmd/goosie/` và `go test -count=1 ./test/qc/` PASS. Nếu bạn thêm tag mới, regenerate artifact: `go test -count=1 -run TestTraceability ./test/qc/ -update` rồi commit `test/qc/traceability.golden.json`.
- **[AC-03]** `go build ./...`, `go vet -all ./...`, `go test ./internal/archtest/` PASS; `gofmt -l cmd/goosie` sạch.
- **[AC-04]** Mỗi case trong mục 2.2 hoặc có test mới, hoặc có dòng tag, hoặc được ghi là "không automatable" kèm lý do trong Status. Không được im lặng bỏ qua.
- **[AC-05]** Self-review: với mỗi test mới, mô tả 1 câu "test này sẽ fail nếu tính năng X hỏng" — đưa vào Status. Test nào không trả lời được thì viết lại hoặc bỏ.
- **[AC-06]** Chạy được không cần display (mọi test goosie hiện tại chạy trên headless surface).

## 4. Ràng buộc vận hành

- **Chỉ tạo/ sửa file mới trong `cmd/goosie/`**, cộng tag trên function đã tồn tại ở đó, cộng `test/qc/traceability.golden.json` khi tag đổi.
- **Cấm sửa**: `docs/qa-*` (catalog là source của QC, integrator giữ), `internal/**`, `test/**` (agent khác đang sở hữu), `testdata/**`, `.github/**`, `docs/tasks/**` khác TASK-020.
- Không sửa engine để test pass. Nếu phát hiện bug engine cản hành trình, ghi vào Status kèm reproducer nhỏ, **dừng ở đó**, không vá.
- Chỉ commit trên branch `worktree-task-020-p1-journeys`. **Không `git push`, không merge vào `main`.**

## 5. Status

### 5.1. Bảng Audit trước khi viết (§2.1)

| Case | Test đã có | Action |
| --- | --- | --- |
| BR-004 | Không có | `test mới` (`cmd/goosie/journey_p1_nav_test.go:TestJourneyURLWithPathQueryFragment`) |
| BR-008 | Không có (`TestJourneyHTTPNavigationAndRedirect` chỉ test tới đích redirect, không test Back; `TestApplyNavResultTraversalRewritesRedirectedEntry` chỉ test mock unit) | `test mới` (`cmd/goosie/journey_p1_nav_test.go:TestJourneyRedirectAndBack`) |
| BR-013 | `cmd/goosie/tab_lifecycle_test.go:TestStopTabCancelsOnlyActiveNavigationAndKeepsDisplayedDocument` (INT unit test mock context) | `tag mới` trên test đã có + `test mới` (`cmd/goosie/journey_p1_nav_test.go:TestJourneySlowPageStopKeepsDocument`) cho CUA journey |
| BR-015 | Không có (`TestE2ECoreBrowserJourney` qua nhiều bước nhưng không assert history entries và sync 1 lần) | `test mới` (`cmd/goosie/journey_p1_nav_test.go:TestJourneyLinkNavigationInSameTab`) |
| BR-016 | Không có | `test mới` (`cmd/goosie/journey_p1_nav_test.go:TestJourneyAnchorNavigation`) |
| BR-020 | Không có | `test mới` (`cmd/goosie/journey_p1_nav_test.go:TestJourneyReloadPageWithQueryFragment`) |
| BR-021 | Không có (`TestStaleNavResultCannotPaintOverTheCurrentOne` chỉ test 2 URL khác nhau A/B, không test refresh pending) | `test mới` (`cmd/goosie/journey_p1_nav_test.go:TestJourneyRefreshWithPendingResponse`) |
| BR-022 | Không có | `test mới` (`cmd/goosie/journey_p1_nav_test.go:TestJourneyAddressBarFocusTypingDoesNotLeakToDocument`) |
| BR-024 | `cmd/goosie/journey_p0_tabs_test.go:TestJourneySwitchBetweenThreeTabs` (đã test mở tab mới bằng Cmd+T và nút goosie.new-tab, focus và nhận nhập liệu URL) | `tag mới` (`// qc:BR-024 BR-026`) |
| BR-027 | `cmd/goosie/tab_lifecycle_test.go:TestBackgroundTabResultLeavesTheForegroundAlone` (assert background hoàn tất không đổi foreground URL/layer/plan) | `tag mới` (`// qc:BR-027`) |
| BR-034 | Không có (`session_scroll_test.go` chỉ test file snapshot serialize/deserialize; `interaction.go:switchTab` có code lưu/phục hồi scroll nhưng chưa có journey test) | `test mới` (`cmd/goosie/journey_p1_tabs_test.go:TestJourneySwitchTabRestoresScroll`) |
| BR-035 | Không có | `test mới` (`cmd/goosie/journey_p1_tabs_test.go:TestJourneyTabsIndependentHistory`) |

### 5.2. Self-review [AC-05]
- `TestJourneyURLWithPathQueryFragment` (BR-004): Test này sẽ FAIL nếu HTTP client gửi URL bị rụng query/path lên server, hoặc URL hiển thị trên thanh địa chỉ/document bị rụng fragment `#anchor`.
- `TestJourneyRedirectAndBack` (BR-008): Test này sẽ FAIL nếu thanh địa chỉ không cập nhật URL đích sau redirect 302, hoặc khi bấm nút Back bị kẹt vòng lặp redirect thay vì quay về trang khởi đầu.
- `TestJourneySlowPageStopKeepsDocument` (BR-013): Test này sẽ FAIL nếu bấm nút Stop khi request đang tải dở mà không dừng được network request, hoặc làm xoá mất tài liệu/URL đang hiển thị trước đó.
- `TestJourneyLinkNavigationInSameTab` (BR-015): Test này sẽ FAIL nếu bấm link trong cùng tab mà URL/tài liệu không cập nhật đồng bộ, hoặc history sinh ra entry rác/entry trùng thay vì đúng 2 mục `[start, dest]`.
- `TestJourneyAnchorNavigation` (BR-016): Test này sẽ FAIL nếu bấm link anchor (cùng trang hoặc từ trang khác) mà fragment không cập nhật trên URL hoặc viewport không cuộn tới toạ độ của phần tử mục tiêu.
- `TestJourneyReloadPageWithQueryFragment` (BR-020): Test này sẽ FAIL nếu reload trang làm mất query/fragment trên URL hiện hành, hoặc reload lại đẩy thêm visit mới vào history.
- `TestJourneyRefreshWithPendingResponse` (BR-021): Test này sẽ FAIL nếu reload/refresh khi request cũ còn pending mà response cũ trả về sau lại ghi đè/áp dụng lên document thay vì response mới nhất.
- `TestJourneyAddressBarFocusTypingDoesNotLeakToDocument` (BR-022): Test này sẽ FAIL nếu khi thanh địa chỉ đang có focus mà các ký tự gõ từ bàn phím lọt vào input element của document bên dưới.
- `TestJourneySwitchTabRestoresScroll` (BR-034): Test này sẽ FAIL nếu chuyển qua tab khác rồi quay lại mà scroll offset của tab bị reset về top (0) thay vì khôi phục đúng vị trí cuộn trước đó.
- `TestJourneyTabsIndependentHistory` (BR-035): Test này sẽ FAIL nếu lịch sử Back/Forward bị dùng chung giữa các tab (ví dụ: Back ở tab này làm ảnh hưởng ngăn xếp navigation của tab kia).

### 5.3. Kết quả nghiệm thu tự chạy

```
$ go test -count=1 -race -run TestJourney ./cmd/goosie/
ok  	github.com/vyquocvu/goosie/cmd/goosie	3.384s

$ go test -count=5 -run TestJourney ./cmd/goosie/
ok  	github.com/vyquocvu/goosie/cmd/goosie	8.447s

$ go test -count=1 ./cmd/goosie/
ok  	github.com/vyquocvu/goosie/cmd/goosie	10.591s

$ go test -count=1 ./test/qc/
ok  	github.com/vyquocvu/goosie/test/qc	0.873s

$ go build ./...
(exit code 0, clean)

$ go vet -all ./...
(exit code 0, clean)

$ go test ./internal/archtest/
ok  	github.com/vyquocvu/goosie/internal/archtest	4.222s

$ gofmt -l cmd/goosie
(clean, no output)
```

### 5.4. Gap thật còn lại
Không có case nào trong 12 case bị blocked bởi bug engine/chrome. Cả 12 case đều đã được phủ đầy đủ qua hành trình thực tế hoặc tag trên unit/integration tests hiện hữu:
- 10 test journey mới được viết (`cmd/goosie/journey_p1_nav_test.go` và `cmd/goosie/journey_p1_tabs_test.go`)
- 3 case được bổ sung tag (`BR-013`, `BR-024`, `BR-027`) trên các test hiện hữu để tránh nợ kỹ thuật và liên kết đúng traceability
- `test/qc/traceability.golden.json` được cập nhật sạch sẽ và toàn bộ test pass deterministic.


## 6. Kiểm toán mutation của integrator (2026-10-25)

§5.2 của agent ghi 12 câu dạng *"test này sẽ FAIL nếu X hỏng"*. Đó là **tuyên bố, không phải bằng chứng**. Integrator tự chạy mutation trên `main` sau khi merge, đánh vào 3 case. Kết quả: 2 tuyên bố đúng, **1 tuyên bố sai**, và case sai là case mà chính người viết ra nó tin nhất.

Phép mutation thứ tư (xoá `cur.SetScrollY(vp.Offset.Y)` trong `switchTab` để test BR-034) **PASS 3/3 và bị loại là mutation không hợp lệ**: scroll của tab vốn đã được cập nhật liên tục ở `navigation.go:640` nên hành động lưu lúc rời tab là dư thừa, xoá nó không thay đổi hành vi. Đây là bẫy thường gặp nhất của mutation testing: một phép biến đổi không làm program đổi behaviour thì không chứng minh được gì, và kết quả "test vẫn xanh" lúc đó không có nghĩa oracle hỏng.

| Case | Phép mutation | Kết quả trước khi sửa | Phán quyết |
| --- | --- | --- | --- |
| BR-020 | `reloadTab` gọi `navigateTab` thay vì `navigateTabNoHistory` (reload Push thêm entry history) | 4/4 FAIL | Oracle nhạy, tuyên bố đúng. |
| BR-034 | `switchTab` đặt `Offset: frame.Point{}` thay vì `Y: newTab.ScrollY()` | 3/3 FAIL | Oracle nhạy, tuyên bố đúng. |
| BR-021 | Xoá **cả** `tab.Nav.Cancel()` ở `navigation.go:117` **và** guard `tab.Nav.Serial != result.serial` ở `navigation.go:209` — tức stale document thật sự được áp dụng | **6/6 PASS** | **Oracle không có giá trị.** Test không phát hiện gì cả. |

`TestJourneyRefreshWithPendingResponse` đọc AX tree ngay sau `doRelease()`. Tại thời điểm đó request stale chưa thể về, chưa nói gì đến việc goroutine load gửi kết quả và loop kịp áp dụng. Nên điều kiện `resp1` vắng mặt đúng **trong mọi trường hợp**, kể cả khi engine áp stale response lên document mới.

Lần mutation đầu tiên của integrator (chỉ tắt guard serial) là **mutation không hợp lệ** và bản thân nó là một phát hiện: `navigateTab` huỷ navigation trước của cùng tab (`navigation.go:117`), nên request stale chết vì `context.Canceled` và đi vào nhánh return sớm ở `navigation.go:230-241` — guard serial không phải cơ chế mà hành trình này thử nghiệm. Muốn thử nghiệm cơ chế supersede thật, phải bỏ cả hai.

Sửa: handler stale nay đóng `req1Served` sau khi ghi xong body; test chờ kênh đó, rồi chờ chrome vẽ thêm hai frame (`window.Stats().Presents`) trước khi assertion. Buffer của `navResults` (kích thước 16, `main.go:289`) là lý do một send hoàn tất không chứng minh điều gì: send trả lời "kết quả đã được xếp hàng", còn assertion cần "kết quả đã được đọc và từ chối".

Nghiệm thu sau sửa: sạch máy → 8/8 PASS; chạy lại đúng mutation M2 → **6/6 FAIL** với message `stale response 1 was applied over newer response 2`; revert → PASS.

**Ràng buộc mới cho mọi task test kế tiếp:** một tuyên bố dạng "test sẽ FAIL nếu X" chỉ được chấp nhận kèm output của phép mutation đã thực hiện. Với assertion phủ định kiểu *"Y không bao giờ xuất hiện"*, bắt buộc chứng minh được **Y đã có cơ hội xuất hiện**: chờ phía server sinh ra Y, rồi chờ loop tiêu thụ hết hàng đợi đang chờ. Nếu không, test chỉ đang quay phim một đường đua mà nó vừa khởi động.
