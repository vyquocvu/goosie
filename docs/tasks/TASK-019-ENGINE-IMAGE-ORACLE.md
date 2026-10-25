# TASK-019: Siết oracle yếu ở tầng engine cho QC case BR-166

- **Task ID**: `TASK-019`
- **Type**: Test quality / Security assurance
- **Assigned to**: `codex`
- **Review**: `qoder cli` (integrator)

## 1. Vấn đề: một test xanh nhưng không chứng minh gì

`test/engine/adversarial_test.go:TestOversizedImageHeaderRejectedBeforeDecode` đang được tag `// qc:BR-166` ("Ảnh dimensions/pixel count vượt ngưỡng dù file nén nhỏ → từ chối trước allocation/decode"). Nguyên văn phần assert:

```go
fetcher := func(base, url string) ([]byte, error) { return header, nil }
html := `<img src="https://cdn.example.com/wide.png">`
sess, err := engine.NewSession(html, nil, 400, engine.WithImages("https://example.com/page", fetcher))
if err != nil {
    t.Fatalf("NewSession with over-limit image: %v (must degrade, not fail)", err)
}
if sess == nil {
    t.Fatal("session is nil")
}
```

Nó chỉ kết luận "session sống". Nếu engine **có** giải mã bừa một ảnh 8193x1, test này vẫn xanh. Đây chính xác là loại assurance rỗng mà toàn bộ chương trình QC traceability đang nhắm tới: tag nói "đã tự động hoá BR-166" nhưng không có post-condition nào về việc từ chối.

Nói công bằng: header giả ở dòng 66-72 có CRC placeholder nên `image.DecodeConfig` thất bại từ lâu trước giới hạn pixel — nghĩa là test thậm chí không đi tới mã cần đo.

Bằng chứng mạnh đã nằm ở `internal/image/decode_test.go` (Probe từ chối dimensions/pixel count/encoded bytes, có test biên accept-at-limit). Task này không nhân bản chúng; task này làm cho **test tầng engine nói đúng điều nó tag**, hoặc dừng tag nó.

## 2. Yêu cầu

1. Chọn một **observable mà test sẽ FAIL nếu engine thật sự giải mã** ảnh quá ngưỡng. Ưu tiên theo thứ tự:
   a. Nếu `internal/engine` đã có đường đọc được (accessor/count) cho "ảnh đã được admit để decode" thì dùng ngay.
   b. Nếu không, thêm **đúng một** accessor read-only vào `internal/engine` (tiền lệ: `js.Runtime.PendingFetches()` được thêm ở TASK-017 chỉ để test đồng bộ đúng), ví dụ đếm số ảnh qua cổng admission, không đổi behavior.
   c. Nếu cả hai hướng đều không ổn trong phạm vi cho phép, **bỏ tag `// qc:BR-166`** khỏi test engine và ghi rõ lý do vào Status — tag sai còn tệ hơn không tag.
2. Fixture phải là ảnh **hợp lệ về cấu trúc** (PNG/WEBP encode thật, dimensions vượt `MaxImageDimension` hoặc `MaxDecodedImagePixels`), để đường đi thực sự chạm tới admission gate chứ không chết ở parser.
3. Giữ hành vi "phải degrade, không fail cả trang": vẫn assert `NewSession` thành công, **và** thêm assert từ-chối-thật.
4. Thêm assert dương: một ảnh **trong ngưỡng** phải được admit, để chứng minh observable ở bước 1 nhạy theo cả hai chiều (không phải constant luôn bằng 0).
5. Nếu bạn đụng tới tag `// qc:BR-xxx`, regenerate artifact: `go test -count=1 -run TestTraceability ./test/qc/ -update` và commit `test/qc/traceability.golden.json`.

## 3. Tiêu chí nghiệm thu

- **[AC-01]** Proof bằng mutation: tạm nới `internal/image/decode.go` (hoặc bypass `Probe` ở call site engine) → test mới **phải FAIL**; revert. Sau đó chạy lại → PASS. Ghi cả hai lượt vào Status.
- **[AC-02]** `go test -count=10 ./test/engine/ ./internal/image/` PASS.
- **[AC-03]** `go test -race -count=3 ./test/engine/` PASS.
- **[AC-04]** `go build ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test ./test/qc/` PASS; `gofmt -l` sạch trên file bạn đụng.
- **[AC-05]** Status liệt kê **các test khác trong repo có dạng "chỉ assert không crash"** mà bạn tìm thấy khi đọc xung quanh (grep `if err != nil` + không có assert state). Chỉ liệt kê, không sửa.

## 4. Ràng buộc vận hành

- Chỉ commit trên branch `worktree-task-019-engine-image-oracle`. **Không `git push`, không merge vào `main`.**
- Phạm vi sửa: `test/engine/**`, `internal/image/decode_test.go` (nếu cần), **một** accessor read-only trong `internal/engine` nếu và chỉ nếu hướng 1a không khả thi, `test/qc/traceability.golden.json` khi tag đổi.
- **Cấm**: `cmd/goosie/**`, `internal/js/**`, `internal/platform/**`, `docs/qa-*`, và cấm thay đổi bất kỳ ngưỡng admission nào (`limits.go`, `decode.go` constants).
- Sandbox codex có thể không bind được TCP listener; `engine.WithImages` dùng fetcher fake nên không cần mạng. Ghi rõ vào Status nếu lệnh nào bạn không chạy được.

## Status

### Hướng chọn

Chọn **1a**: dùng observable public sẵn có là `Session.Arena` và `layout.Object.Image` của element `<img>`. Ảnh hợp lệ 1×1 phải được attach; ảnh hợp lệ 8193×1 không được attach. Không cần accessor mới vì session đã expose arena cho caller; không chọn 1b vì thêm accessor sẽ trùng lặp trạng thái đã quan sát được; không chọn 1c vì observable hiện có phân biệt được trường hợp bình thường có ảnh decode/attach và trường hợp admission từ chối. Giữ nguyên tag `// qc:BR-166`, nên không cần cập nhật traceability golden.

Fixture PNG được tạo bằng `image/png.Encode` thực sự. Cả hai trường hợp vẫn yêu cầu `NewSession` thành công. Assertion dương xác nhận ảnh 1×1 được attach; assertion âm xác nhận ảnh 8193×1 không được attach.

### Kết quả nghiệm thu

| AC | Kết quả | Lệnh đã chạy | Output rút gọn |
|---|---|---|---|
| AC-01 | PASS | `go test -count=1 ./test/engine/ -run TestOversizedImageHeaderRejectedBeforeDecode -v` trong lượt mutation; cùng lệnh sau revert | Mutation: `adversarial_test.go:96: decoded image attached = true, want false`; sau revert: `PASS`, `ok .../test/engine 0.543s`. Chi tiết hai lượt ở dưới. |
| AC-02 | PASS sau khi chạy với quyền loopback | `go test -count=10 ./test/engine/ ./internal/image/` | Ban đầu sandbox chặn listener; lần chạy được cấp quyền: `ok .../test/engine 51.515s`, `ok .../internal/image 5.479s`. |
| AC-03 | PASS | `go test -race -count=3 ./test/engine/` | `ok .../test/engine 72.278s`. |
| AC-04a | PASS | `go build ./...` | Thành công, không có output. |
| AC-04b | PASS | `go vet -all ./...` | Thành công, không có output. |
| AC-04c | PASS | `go test ./internal/archtest/` | `ok .../internal/archtest 5.349s`. |
| AC-04d | PASS | `go test ./test/qc/` | `ok .../test/qc 0.897s`. |
| AC-04e | PASS | `gofmt -l test/engine/adversarial_test.go` | Output rỗng. |
| AC-05 | Đã rà, không sửa các test liệt kê | `rg -n "Must not panic|must not panic|Should not panic|Should not crash|does not panic|no panic|not panic|not crash" test --glob '*_test.go'` và đọc các vùng xung quanh | Các test no-crash-only được liệt kê bên dưới. |

Lần chạy đầu `go test -count=10 ./test/engine/ ./internal/image/` bị sandbox chặn khi test gọi `httptest.NewServer`: `panic: httptest: failed to listen on a port: listen tcp6 [::1]:0: bind: operation not permitted`. Sau đó chạy lại cùng lệnh với quyền bind loopback; cả hai package PASS. Không còn lệnh nghiệm thu nào bị chặn.

### AC-01 — mutation proof

1. Lượt mutation thật: tạm thay `imgdec.Probe(data)` bằng `stdimage.DecodeConfig(strings.NewReader(string(data)))`, đồng thời thay `imgdec.Decode(data)` bằng `stdimage.Decode(strings.NewReader(string(data)))` tại call site engine. Cách này bỏ cả admission probe lẫn probe lặp bên trong `Decode`, khiến PNG 8193×1 thực sự được decode và attach.

   Lệnh: `go test -count=1 ./test/engine/ -run TestOversizedImageHeaderRejectedBeforeDecode -v`

   Output FAIL nguyên văn:

   ```text
   === RUN   TestOversizedImageHeaderRejectedBeforeDecode
   === RUN   TestOversizedImageHeaderRejectedBeforeDecode/within_admission_limit
   === RUN   TestOversizedImageHeaderRejectedBeforeDecode/over_dimension_limit
       adversarial_test.go:96: decoded image attached = true, want false
   --- FAIL: TestOversizedImageHeaderRejectedBeforeDecode (0.00s)
       --- PASS: TestOversizedImageHeaderRejectedBeforeDecode/within_admission_limit (0.00s)
       --- FAIL: TestOversizedImageHeaderRejectedBeforeDecode/over_dimension_limit (0.00s)
   FAIL
   FAIL github.com/vyquocvu/goosie/test/engine 0.740s
   FAIL
   ```

2. Hoàn nguyên đúng hai dòng engine. `git diff -- internal/engine/engine.go` rỗng; `git status --short` khi đó chỉ có ` M test/engine/adversarial_test.go`. Chạy lại chính lệnh trên: `PASS`, hai subtest PASS, `ok github.com/vyquocvu/goosie/test/engine 0.543s`.

Trong quá trình xác định mutation, lượt chỉ bypass `imgdec.Decode` nhưng giữ nguyên `imgdec.Probe` vẫn PASS; nó **không** được tính là mutation proof vì probe trước đó đã từ chối ảnh. Mutation proof ở mục 1 bypass cả hai gate và quan sát được FAIL thật.

### AC-05 — test no-crash-only được tìm thấy

- `test/engine/fuzz_input_test.go:51-55` — `FuzzClickAt`, gọi `ClickAt` và chỉ kiểm tra panic/deadlock.
- `test/engine/fuzz_input_test.go:64-67` — `FuzzSetHover`, không assertion kết quả.
- `test/engine/fuzz_input_test.go:75-78` — `FuzzFocusControl`, không assertion kết quả.
- `test/engine/fuzz_input_test.go:86-90` — `FuzzSelectAtTo`, không assertion kết quả.
- `test/engine/fuzz_input_test.go:104-110` — `FuzzEdit`, không assertion kết quả.
- `test/engine/fuzz_input_test.go:118-145` — `FuzzInteractionSequence`, chỉ thực thi các thao tác.
- `test/engine/js_integration_test.go:68-79` — `TestJSMutationSinkWithoutInvalidationIsNoop`, gọi sink nhưng không kiểm tra trạng thái/reasons sau đó.
- `test/style/shadow_test.go:195-215` — `TestShadowPanicsNoPanic`, chạy các khai báo qua resolver nhưng không kiểm tra style được tạo ra.

### Gap còn lại

Observable là ảnh đã attach vào arena, không phải hook trực tiếp đếm lời gọi decoder hoặc allocation. Mutation proof chứng minh đường regression thực tế (bỏ admission rồi decode/attach) bị test bắt. Một implementation lỗi mới vẫn decode ảnh nhưng chủ động loại ảnh trước khi attach có thể không bị observable này phát hiện; bổ sung phép đo trực tiếp sẽ cần hook/accessor test riêng và nằm ngoài lựa chọn 1a hiện tại.
