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
   minh trong code — không phải "target của máy đang chạy test". → **đã quyết ở §5.1**: 5 cặp
   GOOS/GOARCH của matrix `.github/workflows/release.yml`, khai trong `releaseTargets`.
2. **Judgement nào là đúng cho staleness?** Proposed: với mỗi target trong tập hợp đồng, sinh danh
   sách module linked; (a) mọi module của mọi target phải có một dòng pin; (b) một dòng pin chỉ bị
   coi là stale khi nó **không** xuất hiện ở *bất kỳ* target nào. Không được tiện tay bỏ hẳn chiều (b).
   → **đã làm đúng nguyên văn, cả hai chiều: §5.2**.
3. `x/net@v0.60.0` và `pprof` đang pin — kiểm bằng §2.1 rằng chúng vẫn linked trên **từng** target,
   để không có bất ngờ tương tự sau khi đổi gate. → **đã đo ở §5.1** (`x/net` không linked bởi
   `goosie-headless`, `pprof` linked bởi cả hai, trên mọi target).
4. Liệu có module nào khác chỉ xuất hiện ở target thứ ba (windows, linux/arm64) hay
   không — đo bằng cùng một lệnh `GOOS=... GOARCH=... go list -deps` cho mọi target đã quyết ở 2.1.
   → **đã đo ở §5.1**: không, delta vẫn chỉ là `golang.org/x/sys`, và nó là amd64 chứ không phải Linux.

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

Host đo mọi thứ ở trên: `go version go1.26.10 darwin/arm64`, không display, không C toolchain
cho target lạ. Commit: `5916faf fix(gatecheck): audit licence pins per release target, not per
host` (sửa gate + thêm dòng pin trong **cùng một commit**, đúng §3 Bước 2).

### 5.1 §2.1 — tập target hợp đồng, và §2.4 module set từng target

Hợp đồng phát hành là matrix trong `.github/workflows/release.yml` (bản này build + publish
artifact cho 5 cặp), không phải "target của máy đang test":

| # | Target | Module linked (`go list -deps ./cmd/goosie ./cmd/goosie-headless`, sort -u) | Số |
| --- | --- | --- | --- |
| 1 | darwin/arm64 | regexp2/v2@v2.5.2, goja@v0.0.0-20260926152631-39ec2650adc9, sourcemap@v2.1.3+incompatible, pprof@v0.0.0-20230207041349-798e818bf904, x/image@v0.45.0, x/net@v0.60.0, x/text@v0.42.0 | 7 |
| 2 | darwin/amd64 | như dòng 1 **+ `golang.org/x/sys@v0.48.0`** | 8 |
| 3 | linux/amd64 | như dòng 1 **+ `golang.org/x/sys@v0.48.0`** | 8 |
| 4 | linux/arm64 | như dòng 1 — **không có x/sys** | 7 |
| 5 | windows/amd64 | như dòng 1 **+ `golang.org/x/sys@v0.48.0`** | 8 |

Lệnh đo (chạy thật, exit 0 cả 5; không cần display):

```
$ for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do
    goos=${t%/*}; goarch=${t#*/}
    GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go list -f \
      '{{if not .Standard}}{{.Module.Path}}@{{.Module.Version}}{{end}}' \
      -deps ./cmd/goosie ./cmd/goosie-headless | sed '/^$/d' | sort -u
  done
```

Trả lời §2.4 (ô "chưa đo" duy nhất): **không** có module thứ ba nào chỉ xuất hiện ở windows hay
linux/arm64 — delta so với darwin/arm64 vẫn đúng một phần tử, `golang.org/x/sys`. Chuỗi gây ra
delta là `golang.org/x/sys/cpu`, và nó là **amd64**, không phải Linux:

```
$ GOOS=linux GOARCH=amd64  CGO_ENABLED=0 go list -f '{{if not .Standard}}{{.ImportPath}} {{.Module.Path}}@{{.Module.Version}}{{end}}' -deps ./cmd/goosie | grep x/sys ; echo exit=$?
golang.org/x/sys/cpu golang.org/x/sys@v0.48.0
exit=0
$ GOOS=linux GOARCH=arm64  # đúng lệnh trên
exit=1                       # không có kết quả: arm64 không link x/sys/cpu
$ GOOS=darwin GOARCH=amd64 # đúng lệnh trên
golang.org/x/sys/cpu golang.org/x/sys@v0.48.0
exit=0
$ GOOS=windows GOARCH=amd64 # đúng lệnh trên
golang.org/x/sys/cpu golang.org/x/sys@v0.48.0
exit=0
```

`CGO_ENABLED` không đổi module set trên cả 5 target (đo cả `0` và `1`, kết quả từng cặp giống
hệt nhau), nên gate pin `CGO_ENABLED=0`: phép đo không cần C compiler và không cần display, mà
vẫn không bỏ sót dependency nào. Chiều §2.3: `x/net@v0.60.0` linked trên `./cmd/goosie` ở **cả 5**
target nhưng **không** linked ở `./cmd/goosie-headless`; `pprof` linked ở cả hai binary trên cả 5
target. Không có bất ngờ nào còn lại trong allowlist.

### 5.2 §2.2 — judgement được chọn

* (a) Với **từng** target trong tập hợp đồng: mọi module target đó linked phải có một dòng pin,
  và licence của dòng đó phải thuộc `permittedLicenses`.
* (b) Một dòng pin chỉ stale khi nó **không** xuất hiện ở **bất kỳ** target nào. Vì vậy
  `golang.org/x/sys@v0.48.0 BSD-3-Clause` (đã audit lại `LICENSE` + `PATENTS` trong module cache,
  BSD-3-Clause chuẩn của Go) là hợp lệ, dù darwin/arm64 không bao giờ link nó.
* (c) Oracle thứ hai, độc lập với allowlist: `linkedModulesByTarget` — bảng module set đo ở 5.1,
  nằm trong code. Một target không nằm trong bảng là một failure.
* (d) Fail closed: `go list` lỗi, output không parse được, hay resolve ra **tập rỗng** đều là
  error. `t.Skip` đã bị bỏ — xem 5.5.
* (e) `releaseTargets` phải **đúng bằng** matrix của `release.yml` (cả hai chiều), nên thêm một
  artifact mới mà không audit licence là red ngay trên laptop.

### 5.3 Bước 0 — oracle chạy trước khi sửa, red đúng chiều

Gate mới, allowlist **chưa** có dòng `x/sys` (darwin/arm64 host):

```
$ go test -count=1 -run 'TestLinkedDependenciesPinned|TestReleaseTargetsMatchTheReleaseMatrix|TestTargetResolutionFailsClosed' -v ./test/gatecheck/
=== RUN   TestLinkedDependenciesPinned/darwin-amd64
    licenses_test.go:196: linked module golang.org/x/sys@v0.48.0 is not in dependency-licenses.txt (re-audit and amend) on target darwin/amd64
=== RUN   TestLinkedDependenciesPinned/linux-amd64
    licenses_test.go:196: linked module golang.org/x/sys@v0.48.0 is not in dependency-licenses.txt (re-audit and amend) on target linux/amd64
=== RUN   TestLinkedDependenciesPinned/windows-amd64
    licenses_test.go:196: linked module golang.org/x/sys@v0.48.0 is not in dependency-licenses.txt (re-audit and amend) on target windows/amd64
--- FAIL: TestLinkedDependenciesPinned (2.13s)
    --- PASS: TestLinkedDependenciesPinned/darwin-arm64 (0.00s)
    --- FAIL: TestLinkedDependenciesPinned/darwin-amd64 (0.00s)
    --- FAIL: TestLinkedDependenciesPinned/linux-amd64 (0.00s)
    --- PASS: TestLinkedDependenciesPinned/linux-arm64 (0.00s)
    --- FAIL: TestLinkedDependenciesPinned/windows-amd64 (0.00s)
```

Đỏ ở cả ba target amd64 ngay trên máy macOS — chiều thiếu pin, không phải chiều stale. Sau Bước 2
thêm dòng pin, cũng lệnh đó: `ok github.com/vyquocvu/goosie/test/gatecheck 2.532s`, exit 0.

### 5.4 Lệnh bắt buộc, exit code

| Lệnh | Exit |
| --- | --- |
| `go test -count=1 ./...` | 0 (`FAIL` lines: 0; archtest ok 3.617s, gatecheck ok 34.406s, qc ok 2.710s) |
| `go vet -all ./...` | 0 |
| `go test ./internal/archtest/` | 0 (`ok ... 2.968s`) |
| `go test ./test/gate -run TestGate -v` | 0 (toàn bộ PASS, `--- FAIL` count = 0, 20.171s) |
| `go test -race -count=1 ./test/gatecheck/` | 0 (ok 26.997s) — CI ubuntu chạy `go test -race ./...` |
| `gofmt -l test/gatecheck/` | rỗng |

### 5.5 Mutation proof — 6 cái, mỗi cái giết đúng một check

Mỗi cái: sửa → chạy → dán output → khôi phục. Sau **từng** cái, `git status --short` rỗng
(đã commit `5916faf` trước khi mutate, khôi phục bằng `git checkout --`).

**(i) Xoá dòng pin `golang.org/x/sys@v0.48.0`** ⇒ chỉ chiều (a) chết, và chết đúng trên target link nó:

```
$ go test -count=1 -run 'TestLinkedDependenciesPinned' -v ./test/gatecheck/ ; echo exit=$?
=== RUN   TestLinkedDependenciesPinned/darwin-amd64
    licenses_test.go:196: linked module golang.org/x/sys@v0.48.0 is not in dependency-licenses.txt (re-audit and amend) on target darwin/amd64
=== RUN   TestLinkedDependenciesPinned/linux-amd64
    licenses_test.go:196: linked module golang.org/x/sys@v0.48.0 is not in dependency-licenses.txt (re-audit and amend) on target linux/amd64
=== RUN   TestLinkedDependenciesPinned/windows-amd64
    licenses_test.go:196: linked module golang.org/x/sys@v0.48.0 is not in dependency-licenses.txt (re-audit and amend) on target windows/amd64
--- FAIL: TestLinkedDependenciesPinned (1.89s)
    --- PASS: TestLinkedDependenciesPinned/darwin-arm64 (0.00s)
    --- FAIL: TestLinkedDependenciesPinned/darwin-amd64 (0.00s)
    --- FAIL: TestLinkedDependenciesPinned/linux-amd64 (0.00s)
    --- PASS: TestLinkedDependenciesPinned/linux-arm64 (0.00s)
    --- FAIL: TestLinkedDependenciesPinned/windows-amd64 (0.00s)
FAIL	github.com/vyquocvu/goosie/test/gatecheck	2.299s
exit=1
```

Cùng mutation đó, **gate cũ** (bản `6071b8b`, copy vào package với tên
`TestOldHostPinnedGateTmp`) vẫn xanh trên máy này — đó chính là defect:

```
$ go test -count=1 -run 'TestOldHostPinnedGateTmp' -v ./test/gatecheck/ ; echo old-gate-exit=$?
--- PASS: TestOldHostPinnedGateTmp (0.40s)
old-gate-exit=0
```

Và chiều ngược lại của bẫy (§1.3) cũng được đo lại sau khi pin: gate cũ read `x/sys` là stale
trên darwin/arm64, gate mới xanh.

```
$ go test -count=1 -run 'TestLinkedDependenciesPinned|TestOldHostPinnedGateTmp' ./test/gatecheck/
--- FAIL: TestOldHostPinnedGateTmp (0.43s)
    oldlicenses_tmp_test.go:60: allowlist entry golang.org/x/sys@v0.48.0 is linked into no release binary (stale pin?)
ok  	... TestLinkedDependenciesPinned (target-aware) ...
```

**(ii) Thêm một dòng pin cho module không target nào link** (`github.com/stretchr/testify@v1.9.0 MIT`)
⇒ chỉ chiều (b) chết:

```
    licenses_test.go:220: allowlist entry github.com/stretchr/testify@v1.9.0 is linked into no release target (darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64) (stale pin?)
--- FAIL: TestLinkedDependenciesPinned (1.99s)
exit=1
```

**(iii) Bỏ `cmd.Env`** (gate đọc env của máy chạy test) ⇒ oracle per-target chết ngay trên
darwin/arm64, tức chứng minh gate cũ thực chất chỉ đo host của người chạy:

```
    licenses_test.go:183: target darwin/amd64 links a different module set than audited (darwin-amd64):
        audited but not linked: golang.org/x/sys@v0.48.0
    licenses_test.go:183: target linux/amd64 links a different module set than audited (linux-amd64):
        audited but not linked: golang.org/x/sys@v0.48.0
    licenses_test.go:183: target windows/amd64 links a different module set than audited (windows-amd64):
        audited but not linked: golang.org/x/sys@v0.48.0
=== NAME  TestLinkedDependenciesPinned
    licenses_test.go:220: allowlist entry golang.org/x/sys@v0.48.0 is linked into no release target (darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64) (stale pin?)
--- FAIL: TestLinkedDependenciesPinned (2.33s)
    --- PASS: TestLinkedDependenciesPinned/darwin-arm64 (0.00s)
    --- PASS: TestLinkedDependenciesPinned/darwin-amd64 (0.00s)
    --- PASS: TestLinkedDependenciesPinned/linux-amd64 (0.00s)
    --- PASS: TestLinkedDependenciesPinned/linux-arm64 (0.00s)
    --- PASS: TestLinkedDependenciesPinned/windows-amd64 (0.00s)
FAIL	github.com/vyquocvu/goosie/test/gatecheck	2.860s
exit=1
```

**(iv) Làm target resolution trả về tập rỗng** — hai bước, vì cái này là tính chất "fail closed":

(iv-a) biến guard "tập rỗng = error" thành `return nil, nil`:

```
$ go test -count=1 -run 'TestTargetResolutionFailsClosed' -v ./test/gatecheck/
=== RUN   TestTargetResolutionFailsClosed/zero-module-package
    licenses_test.go:287: linkedModules([./internal/frame]) returned no error with 0 modules; an unverified target must fail the gate, not empty it
--- FAIL: TestTargetResolutionFailsClosed (0.09s)
exit=1
```

(iv-b) stacked lên (iv-a), trỏ `releaseBinaries` vào một package không link module ngoài nào
(`./internal/frame`), để **mọi** target resolve rỗng mà không báo lỗi: gate chính **vẫn đỏ** (exit 1),
nhờ oracle (c)+(b) — 5 target đều "audited but not linked" và mọi dòng pin bị gọi là stale:

```
--- FAIL: TestLinkedDependenciesPinned (0.30s)
    licenses_test.go:183: target darwin/arm64 links a different module set than audited (darwin-arm64):
        audited but not linked: github.com/dlclark/regexp2/v2@v2.5.2, ... golang.org/x/text@v0.42.0
    licenses_test.go:183: target linux/amd64 links a different module set than audited (linux-amd64):
        audited but not linked: ..., golang.org/x/sys@v0.48.0, golang.org/x/text@v0.42.0
    licenses_test.go:220: allowlist entry golang.org/x/net@v0.60.0 is linked into no release target (darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64) (stale pin?)
exit=1
```

Direction (a) im lặng trong kịch bản đó, đó chính xác là lý do oracle per-target tồn tại.
(Hai danh sách module trong dòng `licenses_test.go:183` bị rút gọn vì quá dài; mọi dòng khác
nguyên văn, kể cả 5 target đều report và `exit=1`.)

**(v) Re-pin `golang.org/x/image@v0.45.0` thành `GPL-3.0-only`** ⇒ chỉ nhánh `permittedLicenses`
chết, trên cả 5 target (copyleft vẫn bị cấm, không có ngoại lệ):

```
--- FAIL: TestLinkedDependenciesPinned (2.41s)
    --- FAIL: TestLinkedDependenciesPinned/darwin-arm64 (0.00s)
        licenses_test.go:200: linked module golang.org/x/image@v0.45.0 has non-permitted license "GPL-3.0-only" on target darwin-arm64
    --- FAIL: TestLinkedDependenciesPinned/darwin-amd64 (0.00s)
        licenses_test.go:200: linked module golang.org/x/image@v0.45.0 has non-permitted license "GPL-3.0-only" on target darwin/amd64
    --- FAIL: TestLinkedDependenciesPinned/linux-amd64 (0.00s)
        licenses_test.go:200: linked module golang.org/x/image@v0.45.0 has non-permitted license "GPL-3.0-only" on target linux/amd64
    --- FAIL: TestLinkedDependenciesPinned/linux-arm64 (0.00s)
        licenses_test.go:200: linked module golang.org/x/image@v0.45.0 has non-permitted license "GPL-3.0-only" on target linux/arm64
    --- FAIL: TestLinkedDependenciesPinned/windows-amd64 (0.00s)
        licenses_test.go:200: linked module golang.org/x/image@v0.45.0 has non-permitted license "GPL-3.0-only" on target windows/amd64
FAIL	github.com/vyquocvu/goosie/test/gatecheck	2.717s
exit=1
```

**(vi) Thêm một dòng matrix `{goos: linux, goarch: ppc64le, ...}` vào `release.yml`** ⇒ check (e)
chết, nên artifact mới không thể lọt khỏi audit:

```
    licenses_test.go:254: release.yml builds linux/ppc64le but releaseTargets does not audit its licences
--- FAIL: TestReleaseTargetsMatchTheReleaseMatrix (0.00s)
exit=1
```

Không có mutation nào sống.

### 5.6 Skip còn lại trong test: không có

`grep -n "t.Skip\|Skipf\|SkipNow" test/gatecheck/licenses_test.go` → một dòng duy nhất, và đó là
chính comment khai nhận không có skip: `139:// Fail closed by design: there is no t.Skip on any
path here. ...`. Ba đường lỗi có thể xảy ra của `go list` (không chạy được `go`, output parse lỗi,
tập rỗng) đều là `t.Errorf`/`t.Fatalf`; `TestTargetResolutionFailsClosed` là test giữ tính chất
đó, và mutation (iv) chứng minh nó nhạy.

### 5.7 File đã đổi, và đề xuất registry (integrator owns)

* `test/gatecheck/licenses_test.go` — gate thành target-aware theo §5.2.
* `test/gatecheck/testdata/dependency-licenses.txt` — thêm `golang.org/x/sys@v0.48.0 BSD-3-Clause`
  (audit lại text trong module cache) + header nói rõ file là **union across targets**.
* Không đổi `go.mod`/`go.sum`, không đổi `permittedLicenses`, không bỏ chiều kiểm nào (§6).

Đề xuất BR: **BR-194 | P0 | INT** — "Provenance licence của binary phân phát được audit theo từng
target phát hành, không theo máy chạy test; thiếu pin, stale pin, copyleft, target không đo được
đều fail." Oracle: `test/gatecheck/licenses_test.go` —
`TestLinkedDependenciesPinned` (subtests `darwin-arm64`, `darwin-amd64`, `linux-amd64`,
`linux-arm64`, `windows-amd64`), `TestReleaseTargetsMatchTheReleaseMatrix`,
`TestTargetResolutionFailsClosed`.
**Không** gắn `// qc:BR-194` trong code: `test/qc/traceability_test.go` sẽ báo "unknown QC ID" và
`traceability.golden.json` (file của integrator) sẽ stale. Người đăng ký BR-194 trong
`docs/qa-test-catalog.md` thêm marker rồi chạy `go test ./test/qc -update` trong commit của họ.
Đề xuất đóng `GOOSIE-LICENSE-001` ghi rõ: defect là "một allowlist không thoả được hai chiều giữa
các target"; đã sửa ở `5916faf` bằng cách pin theo union của 5 target; phần còn mở là CI xanh thật
(§5.8).

### 5.8 Còn chưa đo / chưa fix được trên máy này

* **Chưa chạy artifact nào ngoài darwin.** `docker`, `podman`, `colima`, `qemu-x86_64`, `wine` đều
  `absent` trên host này (đã `command -v` từng cái), nên **không** có bằng chứng runtime cho
  linux/amd64, linux/arm64, windows/amd64. Cái đã đo được là **graph dependency** của chúng —
  `go list` chỉ đọc source/build graph, cross-compile được, không cần display. Đã chứng minh thêm
  bằng cách build thật: `GOOS=linux|windows GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/goosie` và
  `GOOS=linux GOARCH=arm64` cả ba exit 0 (artifact để ở `/tmp`, đã xoá, không đụng `bin/` chính).
* **Chưa có CI run xanh** cho job "Race detector": cần người vận hành push (không `git push` ở
  đây), nên §4.5 chưa đóng được.
* darwin/amd64 và windows/amd64 được đo bằng cross-`go list` trên host darwin/arm64, chưa từng
  được đo trên đúng runner của chúng; `CGO_ENABLED=0` được pin vì đã đo là không đổi module set,
  nhưng bản thân giá trị đó chưa đối chiếu với env build thật của runner macos-13/windows-latest.
* GOOSIE-CIPARITY-001 (parity host-gap) **không** đụng tới, đúng §6.

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
