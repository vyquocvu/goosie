# TASK-017: Loại bỏ đồng bộ bằng `time.Sleep` trong test JS fetch (flake có nguyên nhân thật)

- **Task ID**: `TASK-017`
- **Type**: Test infrastructure / Flake removal
- **Assigned to**: `codex`
- **Review**: `qoder cli` (integrator)

## 1. Bằng chứng và nguyên nhân gốc

Ngày 2026-10-25, trong lúc hai full suite chạy song song trên cùng máy, `test/js` fail:

```
--- FAIL: TestFetchCatchOnError (0.05s)
    fetch_test.go:362: gotCatch = {Kind:4 Num:0 Str: Bool:false}, want it to contain "network down"
```

Chạy riêng lẻ `go test -count=8 -run TestFetchCatchOnError ./test/js/` trên `main`: **PASS**. Vậy đây là fail do tải máy, không phải do logic.

Nguyên nhân nằm ở helper, không phải engine:

```go
// test/js/fetch_test.go:103
func drainFetch(r *js.Runtime) {
	time.Sleep(50 * time.Millisecond)
	r.DrainFetchCallbacks()
}
```

`fetch` chạy trên goroutine riêng (`internal/js/fetch.go:316 runFetch`). `DrainFetchCallbacks` (dòng 677) chỉ invoke các callback **đã được queue**; nó không chờ in-flight request. Nên 50 ms chỉ là một phỏng đoán thời gian: dưới tải, goroutine chưa kịp đặt callback vào `fs.callbacks`, `gotCatch` vẫn rỗng và test fail.

Engine đã có bộ đếm in-flight, chỉ chưa export, và protocol đã an toàn để poll:

```go
// internal/js/fetch.go:133-138
type fetchState struct {
	client    HTTPFetcher
	mu        sync.Mutex
	active    int
	callbacks []fetchCallback
}

// internal/js/fetch.go:295  fs.active++   (bên trong doFetch, đồng bộ với lời gọi fetch())
// internal/js/fetch.go:319  fs.active--   (defer của runFetch, trên goroutine nền)
```

Vì `active++` chạy đồng bộ ngay trong `doFetch`, một vòng poll `active == 0` sau `Run()` luôn thấy đúng số request đang chạy. Fix đúng bản chất là thay phỏng đoán bằng **wait on the real counter**.

## 2. Việc cụ thể

1. Thêm một accessor read-only vào `internal/js/fetch.go`, đặt cạnh `DrainFetchCallbacks`:
   ```go
   // PendingFetches reports the number of fetch requests still running on
   // background goroutines. Hosts and tests use it to wait for in-flight work
   // instead of guessing a sleep duration.
   func (r *Runtime) PendingFetches() int {
   	fs := fetchOf(r)
   	if fs == nil {
   		return 0
   	}
   	fs.mu.Lock()
   	defer fs.mu.Unlock()
   	return fs.active
   }
   ```
   Đây là **thay đổi duy nhất** được phép trong `internal/`. Không sửa semantic của fetch/CORS/promise.
2. Viết lại `drainFetch` trong `test/js/fetch_test.go`: poll `PendingFetches()` về 0 với deadline trần (khuyến nghị 2 s), rồi gọi `DrainFetchCallbacks()`. Hết deadline thì fail kèm số pending còn lại — không im lặng bỏ qua.
3. Thêm **một test chứng minh helper đã hết fragile**: mock fetcher cố tình chậm (ví dụ `Fetch` sleep 300 ms), chạy `fetch('/x').catch(...)`, gọi `drainFetch`, assert `gotCatch` chứa lỗi. Test này **phải fail** với `drainFetch` bản 50 ms và pass với bản mới — đó là proof, không phải assertion trang trí.
4. Audit `grep -rn "time.Sleep" test/`: liệt kê vào Status những chỗ khác vẫn đồng bộ bằng sleep. **Không sửa** ngoài phạm vi fetch ở task này; ghi lại để task sau quyết.

## 3. Tiêu chí nghiệm thu

- **[AC-01]** `go test -count=20 -run TestFetch ./test/js/` PASS.
- **[AC-02]** `go test -race -count=5 ./test/js/` PASS.
- **[AC-03]** Test fetch chậm mới thêm chứng minh `drainFetch` không còn phụ thuộc 50 ms. Tự kiểm chứng: tạm khôi phục `time.Sleep(50ms)` → test mới phải fail, sau đó revert. Ghi kết quả cả hai lượt vào Status.
- **[AC-04]** `go build ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test ./test/qc/`, `go test ./test/js/` PASS.
- **[AC-05]** Status nêu danh sách `time.Sleep` còn lại trong `test/` kèm nhận định chỗ nào là flake thật sự.

## 4. Ràng buộc vận hành

- Chỉ commit trên branch của bạn (`worktree-task-017-fetch-drain`). **Không `git push`, không merge vào `main`.**
- **Cấm sửa**: `cmd/goosie/**`, `test/qc/**`, `docs/qa-*`, `internal/engine/**`, và mọi file `internal/` ngoài `internal/js/fetch.go`.
- Chạy `gofmt -l` trên file bạn đụng trước khi commit (archtest kiểm gofmt toàn repo).
- Sandbox của codex không bind được local TCP listener: nếu có test cần mạng thì ghi rõ trong Status là không chạy được tại đó, integrator sẽ tự verify trên `main`.
