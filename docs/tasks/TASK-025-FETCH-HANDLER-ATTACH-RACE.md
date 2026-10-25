# TASK-025: Sửa race "handler gắn sau khi fetch đã settle" — bug thật trong `internal/js/fetch.go`

- **Task ID**: `TASK-025`
- **Type**: Engine bug / Test determinism
- **Assigned to**: `codex`
- **Review**: `qoder cli` (integrator)

## 1. Đây không phải flake. TASK-017 chưa sửa xong.

TASK-017 thay `time.Sleep(50ms)` trong `test/js/fetch_test.go` bằng `drainFetch` chốt `PendingFetches()`, và kết luận "active == 0 nghĩa là mọi callback đã vào queue". Lập luận đó **đúng một nửa** và sai ở chỗ quan trọng nhất. Integrator vừa tái hiện được **không cần tải**:

```
$ go test -count=30 -run TestFetchCatchOnError ./test/js/      # 2/3 lượt chạy -> FAIL
    fetch_test.go:374: gotCatch = {Kind:4 Num:0 Str: Bool:false}, want it to contain "network down"
$ go test -race -count=20 -run TestFetchCatchOnError ./test/js/
    (fail y như nguyên)  +  KHÔNG có DATA RACE report
```

`Kind:4` là `KindString` (`internal/js/js.go:156-162`: Undefined=0, Null=1, Bool=2, Number=3, String=4). `Str:""` chính là giá trị khởi tạo `var gotCatch = ''`. Nghĩa là: **`.catch` không hề chạy** — không phải chạy với payload sai, không phải timeout. Và vì `-race` im lặng, đây là lỗi thứ tự (ordering), không phải data race.

## 2. Nguyên nhân, chỉ đúng dòng

`runFetch` chạy trên goroutine riêng. Khi kết thúc, nó **chụp snapshot handler rồi nhét snapshot đó vào queue** (`internal/js/fetch.go:404-423`):

```go
t.mu.Lock()
thenCB := t.thenCB      // <- đọc tại thời điểm request xong
catchCB := t.catchCB
t.mu.Unlock()
fs.callbacks = append(fs.callbacks, fetchCallback{fn: catchCB, isReject: true, ...})
```

Thứ tự chết người:

1. JS chạy `fetch('/x').catch(fn)`. `doFetch` (`:295`) tăng `fs.active` rồi `go r.runFetch(...)` (`:304`) — **trước khi** `.catch(fn)` kịp gắn.
2. `mockFetcher` trả lỗi ngay lập tức. Goroutine kết thúc trước cả khi hàng đợi microtask kịp chạy, đọc `t.catchCB == nil`, và queue `{fn: nil}`.
3. JS gắn `.catch(fn)` -> `t.catchCB = fn`, nhưng callback đã queue không còn liên quan gì tới `t` ngoài con trỏ `chain`.
4. `drainFetch`: `PendingFetches()==0` (đúng!), `DrainFetchCallbacks()` gọi `invokeOneCallback({fn: nil})` -> không có gì xảy ra. Test đọc `gotCatch` = `''`.

`active == 0` chưa bao giờ có nghĩa "handler sẽ được gọi". Nó chỉ có nghĩa "request đã xong".

Ba chỗ khác cùng khuôn: `:342-346`, `:357-361`, `:389-393` (preflight/CORS), và `makeRejectThenable` (`:642-664`) thì **đã** xử lý đúng — nó set `t.rejected = true; t.result = ...` *và* queue callback, nên handler gắn muộn vẫn chạy được (chú thích ở `:646-648` nói rõ ý đồ đó). Đường `runFetch` là đường duy nhất chưa làm thế.

## 3. Yêu cầu

1. **Sửa hành vi, không sửa test cho qua.** Thenable phải settle đúng semantics của promise: settle **trên thenable** (`t.rejected`/`t.resolved`/`t.result`), còn lúc *drain* thì phân phối cho handler **đang được gắn tại thời điểm gọi** — không phải snapshot. `chain` đã có sẵn trong `fetchCallback`; `invokeOneCallback` là chỗ nên quyết định gọi gì. Cơ chế "gắn muộn vẫn chạy" (`invokeResolvedCallback`) đã tồn tại cho đường resolve — dùng cùng nguyên tắc cho đường reject của `runFetch`.
2. **Test hồi quy tất định** (thứ TASK-017 thiếu): một test mà *chính ordering* là điều được đo — request settle **trước khi** handler được gắn, rồi gắn handler, rồi drain, rồi assert handler chạy. Không sleep, không đếm, không `-count` thần kỳ. Viết theo kiểu `drainFetch` hiện có nhưng thứ tự thao tác là do test áp đặt.
3. **Sửa cả 4 site** (`:342`, `:357`, `:389`, `:404-423`) — chứng minh bằng cách grep lại `t.thenCB`/`t.catchCB` trong `runFetch`: sau fix, **không còn** chỗ nào queue snapshot handler.
4. **Đây là bug web-compat thật**, không phải bug test: trên một máy chậm/nhanh khác nhau, một trang dùng `fetch(...).then(...)` với response tới sớm sẽ **im lặng không bao giờ chạy callback**. Page sẽ không bao giờ nhận dữ liệu. Viết vào Status một câu mô tả triệu chứng mà người dùng cuối gặp.

## 4. Tiêu chí nghiệm thu

- **[AC-01]** `go test -count=100 -run TestFetchCatchOnError ./test/js/` PASS (100 lượt, không lượt nào fail). Đây là con số, không phải cảm giác.
- **[AC-02]** Test hồi quy mới ở mục 3.2 PASS, và **FAIL khi revert mỗi phần sửa** — chứng minh nó bắt đúng ordering, không phải tình cờ xanh.
- **[AC-03]** `grep -n "fn:       catchCB\|fn:       thenCB\|fn: catchCB\|fn: thenCB" internal/js/fetch.go` trả về rỗng (hoặc tương đương: không còn `fetchCallback{fn:` nào nhận biến snapshot từ `runFetch`).
- **[AC-04]** `go test -count=10 ./test/js/` PASS; `go test -race -count=3 ./test/js/` PASS; `go test -count=1 ./...` PASS.
- **[AC-05]** `go build ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test ./test/qc/` PASS; `gofmt -l` sạch.
- **[AC-06]** `go test -count=30 -run 'TestFetch' ./test/js/` PASS (tất cả các fetch test, không riêng test fail trước đây).

## 5. Ràng buộc vận hành

- **Được sửa**: `internal/js/fetch.go` (và `internal/js/*.go` nếu thenable sống ở file khác — xác nhận bằng grep trước), `test/js/**`, mục Status của file task này.
- **Cấm**: `cmd/**` (agent khác đang sở hữu worktree), `testdata/**` (agent khác đang sở hữu), `internal/engine/**`, `docs/qa-*`, `.github/**`, `Makefile`, `scripts/**`.
- Không nới giới hạn admission, không đổi CORS policy, không đổi semantics `then`/`catch`/`finally` so với spec — chỉ làm cho handler **được gọi đúng lúc**.
- Không `time.Sleep` trong test mới. Không dùng `PendingFetches()` như điều kiện "handler sẽ chạy".
- Chỉ commit trên branch `worktree-task-025-fetch-attach-race`. Không `git push`, không merge `main`.
- Nếu sandbox chặn bind loopback, ghi rõ lệnh nào phải chạy lại ngoài sandbox (task trước bạn đã gặp) — kết quả cuối cùng vẫn phải là output thật, dán nguyên văn.

## 6. Bối cảnh để không làm lại việc đã xong

`drainFetch` dùng `PendingFetches()` (thêm ở TASK-017) **giữ nguyên** — nó vẫn cần để chờ request hoàn tất. Task này không đảo ngược TASK-017; nó sửa cái mà TASK-017 giả định sai. Ghi vào Status: giả định nào của TASK-017 sai, và vì sao `-count=8` một mình không phát hiện ra (nghiệm thu hồi đó chạy *dưới tải*, còn lần này fail ở `count=30` **không tải** — nghĩa là trước đó mẫu quan sát quá nhỏ).
