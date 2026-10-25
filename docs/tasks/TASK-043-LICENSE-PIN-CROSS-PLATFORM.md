# TASK-043 — gate bằng sáng chế phụ thuộc platform: pin hiện tại không thoả được cả hai mục tiêu

**Priority:** P1. **Method:** INT (`test/gatecheck`), không đụng engine, không đụng render.
**Owner:** chưa gán. **Defect:** `GOOSIE-LICENSE-001`.
**Liên quan:** `test/gatecheck/licenses_test.go`, `test/gatecheck/testdata/dependency-licenses.txt`,
BR-179 (release candidate phải xanh mọi lệnh), CI job "Race detector".

Gate 8 (`TestLinkedDependenciesPinned`) là bằng chứng duy nhất cho *provenance của binary phân
phát*. Nó so hai chiều: mọi module linked phải có trong allowlist, **và** mọi dòng allowlist phải
đang được linked (`licenses_test.go:70` và `:78-82`). Vì `go list -deps` phụ thuộc build target,
hai chiều đó mâu thuẫn nhau giữa macOS và Linux — nghĩa là **không tồn tại file allowlist nào làm
xanh gate trên cả hai platform**. Đây không phải lỗi cấu hình CI; nó chặn điều kiện tiên quyết
"candidate commit exit 0 mọi lệnh".

## 1. Bằng chứng đã đo

### 1.1 CI đỏ vì chính một test này, ba run liên tiếp

`gh run view` trên ba run CI gần nhất: job "Render parity vs Chromium" fail ở cả ba
(39990406790 2026-10-23, 40069270435 + 40107385303 2026-10-24 — host-gap đó đã ghi ở
`GOOSIE-CIPARITY-001`), còn job "Race detector" thì **chỉ** fail đúng một test, không có DATA RACE:

```
--- FAIL: TestLinkedDependenciesPinned (0.32s)
    licenses_test.go:71: linked module golang.org/x/sys@v0.48.0 is not in dependency-licenses.txt (re-audit and amend)
FAIL    github.com/vyquocvu/goosie/test/gatecheck  6.259s
```

(run 40107385303, job 120911782481, head `c8cd80b`, ubuntu-latest.)

### 1.2 Đo lại được trên máy này, không cần Linux

`go list` tôn trọng `GOOS`/`GOARCH`, nên phép thử CI chạy lại được tại chỗ, tại `32e3409`:

| Lệnh | Kết quả đo |
| --- | --- |
| `go list -f '{{if not .Standard}}{{.Module.Path}}@{{.Module.Version}}{{end}}' -deps ./cmd/goosie ./cmd/goosie-headless \| sort -u` | 7 module: regexp2/v2, goja, sourcemap, pprof, x/image, x/net, x/text — **không có x/sys** |
| `GOOS=linux GOARCH=amd64` với đúng lệnh trên | 8 module, thêm **`golang.org/x/sys@v0.48.0`** |
| `GOOS=linux GOARCH=amd64 go list -deps ./cmd/goosie \| grep x/sys` | `golang.org/x/sys/cpu` (+ bản `vendor/` của stdlib) |
| `go mod why -m golang.org/x/sys` | `cmd/dump-fonts → x/image/font/opentype → x/image/vector → x/sys/cpu` |
| `grep -c x/image/vector <<<"$(go list -deps ./cmd/goosie)"` | `1` — vector **có** được linked trên darwin, nhưng `x/sys/cpu` thì không |
| `go test -count=1 -run TestLinkedDependenciesPinned ./test/gatecheck/` (darwin) | `ok` 0.815s |

Vậy `x/sys` vào graph **chỉ** ở target Linux; trên darwin chuỗi `cpu` không được link.

### 1.3 Chiều ngược lại cũng đỏ — đã chứng minh bằng mutation, không suy đoán

Thêm đúng dòng mà §1.2 yêu cầu rồi chạy trên darwin:

```
$ printf 'golang.org/x/sys@v0.48.0 BSD-3-Clause\n' >> test/gatecheck/testdata/dependency-licenses.txt
$ go test -count=1 -run TestLinkedDependenciesPinned ./test/gatecheck/
    licenses_test.go:80: allowlist entry golang.org/x/sys@v0.48.0 is linked into no release binary (stale pin?)
--- FAIL: TestLinkedDependenciesPinned (0.97s)
$ # khôi phục file, chạy lại: ok 0.979s; git status sạch
```

### 1.4 Audit license (đã đọc text, không đoán)

`go list -m -f '{{.Dir}}' golang.org/x/sys` → `/Users/vyquocvu/go/pkg/mod/golang.org/x/sys@v0.48.0`.
`LICENSE`: "Copyright 2009 The Go Authors." + ba điều kiện (giữ copyright khi redistribute source;
reproduce copyright + conditions + disclaimer khi redistribute binary; **không** dùng tên Google LLC
hoặc tên người đóng góp để quảng bá) + disclaimer ALL WARRANTIES. Đó là bản BSD-3-Clause chuẩn của
Go, thuộc tập được phép (`permittedLicenses`, `licenses_test.go:19-25`). Thư mục còn có `PATENTS`
— văn bản patente chuẩn của golang, không đổi SPDX id và không phải copyleft.

## 2. Phải đo/quyết trước khi sửa

1. **Tập target nào là hợp đồng phát hành?** `docs/roadmap-v2.md` và `PROJECT.md` khai binary nào
   được phân phát (darwin arm64/amd64, linux amd64?). Gate phải quét đúng tập đó, khai báo tường
   minh trong code — không phải "target của máy đang chạy test".
2. **Judgement nào là đúng cho staleness?** Proposed: với mỗi target trong tập hợp đồng, sinh danh
   sách module linked; (a) mọi module của mọi target phải có một dòng pin; (b) một dòng pin chỉ bị
   coi là stale khi nó **không** xuất hiện ở *bất kỳ* target nào. Không được tiện tay bỏ hẳn chiều (b).
3. `x/net@v0.60.0` và `pprof` đang pin — kiểm bằng §2.1 rằng chúng vẫn linked trên **từng** target,
   để không có bất ngờ tương tự sau khi đổi gate.
4. **Chưa đo:** liệu có module nào khác chỉ xuất hiện ở target thứ ba (windows, linux/arm64) hay
   không — đo bằng cùng một lệnh `GOOS=... GOARCH=... go list -deps` cho mọi target đã quyết ở 2.1.

## 3. Plan (chỉ làm sau khi §2 xong)

* **Bước 0 — oracle trước:** `test/gatecheck` hiện chỉ có một test đơn-fixture. Đổi thành test
  table-driven theo target list của §2.1: mỗi target một kỳ vọng "module set đã đo", và một kỳ
  vọng tổng hợp "mọi module của mọi target đều được pin". Chạy **trước khi sửa** để thấy nó red
  đúng chiều (thiếu `x/sys`) trên mọi máy, không chỉ máy Linux.
* **Bước 1 — sửa gate thành target-aware**, theo quyết định §2.2; `go list` được gọi với
  `GOOS`/`GOARCH` tường minh qua `cmd.Env`, không phụ thuộc env của người chạy.
* **Bước 2 — thêm dòng pin đã audit** `golang.org/x/sys@v0.48.0 BSD-3-Clause` (§1.4), trong **cùng
  commit** với bước 1 vì một mình nó làm darwin red (§1.3).
* **Bước 3 — không nới gì cả:** `permittedLicenses` giữ nguyên tập; không bỏ chiều staleness; copyleft
  vẫn bị cấm. Nếu một module mới xuất hiện thì câu trả lời vẫn là "re-audit, không relax test"
  (comment `licenses_test.go:32-36`).

## 4. Điều kiện nhận

1. `go test -count=1 ./test/gatecheck/` xanh trên darwin **và** danh sách module của target
   `linux/amd64` (đo bằng `GOOS=linux GOARCH=amd64 go list -deps` ngay trong test) cũng xanh.
2. Bảng §2.1/§2.4 có thật trong task doc: target list, module set từng target, không ô "chưa đo".
3. Mutation proof [R-7], tối thiểu 3 cái, mỗi cái giết đúng một check: (i) xoá dòng pin `x/sys`
   ⇒ case Linux red; (ii) thêm một dòng pin cho module không link ở target nào ⇒ staleness red;
   (iii) bỏ `cmd.Env` để gate đọc env của máy ⇒ case Linux red trên máy darwin (chứng minh gate
   cũ thực ra chỉ đo host của người chạy). Chạy từng cái, khôi phục, `git status` sạch.
4. `go test -count=1 ./...`, `go vet -all ./...`, `go test ./internal/archtest/` xanh.
5. `GOOSIE-LICENSE-001` chỉ đóng khi §5 có đủ module set của **từng** target trong hợp đồng, và khi
   một CI run thật (do người vận hành push) xanh job "Race detector".

## 5. Kết quả

*(điền sau khi chạy)*

## 6. Ngoài phạm vi

* Không đổi `permittedLicenses`, không thêm ngoại lệ, không xoá chiều kiểm tra nào.
* Không đụng `go.mod`/`go.sum` — task này không nâng cấp dependency nào.
* `GOOSIE-CIPARITY-001` (parity host-gap) là defect khác, dù cùng nguồn gốc "một máy không phải là
  mọi máy"; đừng gộp một lúc hai sửa đổi.
* Render/engine: không liên quan, không "tranh thủ" sửa.

## 7. Ràng buộc vận hành

Không `git push`, không merge `main`, không worktree con. `docs/qa-open-defects.json`,
`docs/qa-test-catalog.md`, `docs/qa-ratchet.json`, `test/qc/traceability.golden.json` thuộc
integrator; đề xuất dòng registry ghi trong báo cáo.
