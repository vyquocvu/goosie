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
