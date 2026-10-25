# TASK-027: Glyph coverage — emoji và Dingbats biến mất không lại dấu vết

- **Task ID**: `TASK-027`
- **Type**: Engine bug / font fallback
- **Priority**: P1 (ảnh hưởng trực tiếp tới "browser dùng được thật")
- **Assigned to**: chưa chia (blocker: `internal/raster` đang có agent khác đọc trong TASK-022 — integrator quyết định sau khi TASK-022 merge)
- **Review**: `qoder cli` (integrator)
- **Tách ra từ**: `docs/tasks/TASK-024-RENDER-REGION-GAPS.md`

## 1. Bằng chứng

`testdata/parity.py` với region scoring (TASK-021) báo **18/80 fixture có band trống** mà Chromium có ink. Phân loại bằng mắt qua ảnh crop ở `/tmp/blankbands/`: **12 trong 18 band thuộc đúng một nguyên nhân ở đây**.

Probe tối giản (`/tmp/probe/b.html`, render bằng `./bin/goosie -backend headless -width 400 -height 200 -dpr 1 -screenshot -out /tmp/probe/b.png -url file:///tmp/probe/b.html`), kết quả nhìn trên PNG output:

| Dòng trong fixture | Goosie vẽ gì |
| --- | --- |
| `ASCII ok` | đủ |
| `check U+2714 ✔ and U+2713 ✓` | mất cả hai glyph, còn lại `check U+2714  and U+2713` — **một khoảng trắng** |
| `emoji 😀🎉` | mất toàn bộ emoji |
| `arrow → and math ∑` | **đủ cả hai** (U+2192 và U+2211 render bình thường) |

Vậy đây **không phải** bug layout/paint nói chung: ký tự BMP trong phạm vi font đang có thì vẽ được. Mất đúng các codepoint nằm ngoài coverage của mọi face trong chuỗi fallback.

Đã **loại** hai giả thuyết khác bằng probe, đừng mất thời gian thử lại:

* `::before`/`::after` **có hoạt động**: `/tmp/probe/a.html` (`p::after{content:" AFTER"}`, `p.b::before{content:"BEFORE "}`) render ra `BEFORE MIDDLE AFTER` và `PLAIN AFTER` đầy đủ. Band trống trong `testdata/render/selectors/03-pseudo-elements.png` là do `content:"✔"` — tức **cùng nguyên nhân font**, không phải cascade.
* Không phải missing-ink do màu: các glyph mất hẳn, không phải vẽ sai màu.

## 2. Nguyên nhân trong code

Hai chỗ ghép lại thành một lỗi quan sát được:

1. **Chuỗi fallback không có face nào phủ các codepoint đó.** `internal/raster/fonts.go:357 fallbackLocked` duyệt qua `f.sources` — tức đúng các slot đã đăng ký ở `systemFontFiles` (`fonts.go:110-140`: Times/Arial/Courier/Georgia/Verdana, mỗi loại 4 variant) và `systemFontCollections` (`fonts.go:154-156`: HelveticaNeue). Không file nào trong số đó chứa Dingbats (U+2700–U+27BF) hay supplementary-plane emoji (U+1F000+). `systemFontDir` trỏ `/System/Library/Fonts/Supplemental` nhưng **chỉ** để tra các slot đã liệt kê — `Arial Unicode.ttf` nằm ngay trong thư mục đó và có phủ cả hai block, mà không slot nào nhắc tới nó.
2. **Glyph thiếu bị biến thành khoảng trắng, hoàn toàn im lặng.** Chính doc comment ở `internal/raster/fonts.go:406-409` nói ra điều đó:

   > *"The zero Glyph, with Ok false, means no face has such a glyph, which the text path treats as whitespace rather than an error."*

   Đây là phần đáng giá nhất để sửa, và nó giải thích tại sao lỗi này **sống sót được qua cả một hệ thống parity**: một trang mất hết emoji cho ra pixel diff ~vài %, vẫn dưới `PASS_AT=90`, nên không gate nào bắt được. Region scoring (TASK-021) chỉ bắt được nó vì nó so *ink mass* theo band — và ngay cả thế, nó chỉ báo "band trống", không báo "mất glyph nào".

## 3. Yêu cầu

### 3.1. Fallback theo coverage, không theo danh sách slot

Chuỗi fallback phải tìm được face **có chứa codepoint đang cần**, thay vì chỉ các slot đã map. Yêu cầu tối thiểu:

* Thêm các face hệ thống có coverage rộng vào nguồn fallback (macOS: `Arial Unicode.ttf`, `Apple Symbols`; Linux: `DejaVuSans.ttf`, `NotoSansSymbols2`; đường dẫn phải được thử theo kiểu *có thì dùng, không thì bỏ qua* như `fonts.go:108-109` đã document cho host Linux).
* Không được phá thứ tự ưu tiên hiện tại: family mà CSS chỉ định phải thắng; face coverage-rộng chỉ được dùng khi **không face nào trong chuỗi hiện tại vẽ được** codepoint đó. Chứng minh bằng test: một trang `font-family: Arial` vẫn lấy Arial cho ký tự ASCII, chỉ ký tự ngoài coverage mới rơi xuống face khác.
* `→` và `∑` **đã render đúng từ trước** — nếu sau sửa mà chúng đổi hình/dổi advance width thì đó là regression, phải giữ nguyên.

### 3.2. Glyph thiếu phải nhìn thấy được, và phải khai báo được

Với codepoint mà **mọi** face đều không phủ (emoji màu là trường hợp thật, xem 3.3):

* Vẽ **tofu** (hộp chữ nhật có viền) thay vì khoảng trắng. Đây là hành xử của mọi browser thật và là điều khiến người dùng biết trang web đang hỏng thay vì nghĩ nội dung không tồn tại.
* Advance width của tofu phải **ổn định và khác 0**, để layout không dịch khi sau này có font emoji thật.
* Phải có đường đếm số glyph thiếu trong một lần render, đủ để test assert (`missing == 0` cho fixture ASCII, `missing > 0` cho fixture emoji). **Không** in từng rune ra stdout trong đường đi bình thường — stdout của binary là hợp đồng với gate scripts.

### 3.3. Emoji màu: ngoài phạm vi, nhưng phải nói rõ

`Apple Color Emoji.ttc` là bitmap `sbix`/`COLR`; `golang.org/x/image/font/opentype` không rasterize bitmap màu. **Không cố vẽ emoji màu trong task này.** Sản phẩm đúng của task với emoji là: tofu + `missing > 0` + ghi `docs/roadmap-v2.md` một dòng về color-emoji. Đừng ăn gian bằng cách map emoji về một glyph ASCII na ná.

## 4. Tiêu chí nghiệm thu

* **[AC-01]** `go test -count=1 ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test ./test/qc/` PASS; `gofmt -l` sạch.
* **[AC-02]** Repro ở §1 (`/tmp/probe/b.html`) phải cho: `✔` và `✓` **vẽ được** (không còn khoảng trắng); `😀🎉` ra **tofu** nhìn thấy được; `→` và `∑` **không đổi** so với trước khi sửa. Dán ảnh hoặc số đo ink của từng dòng vào Status.
* **[AC-03]** Region scoring không còn báo band trống cho `unicode/02-emoji-symbols.png` và `selectors/03-pseudo-elements.png`: `python3 testdata/parity.py --render --binary ./bin/goosie` rồi kiểm `testdata/render-known-gaps.json` — **xoá** hai entry đó khỏi `known_gaps` trong file (baseline chỉ được phép **thu nhỏ**, không được nới).
* **[AC-04]** Unit test cho resolver coverage: với một rune nằm ngoài mọi slot đã map, hàm chọn face phải trả về face thực sự `HasGlyph(rune) == true`; với rune mà không face nào phủ, phải trả tín hiệu "missing" chứ không phải `Ok=false` rồi bị hiểu là khoảng trắng.
* **[AC-05]** Mutation proof (bắt buộc, xem `docs/tasks/TASK-020-P1-JOURNEYS.md` §6 để hiểu vì sao): (a) gỡ `Arial Unicode.ttf` khỏi chuỗi fallback → test AC-04 **phải FAIL**; (b) cho glyph missing quay lại thành khoảng trắng → test tofu **phải FAIL**. Dán output FAIL nguyên văn và `git status` sạch sau revert. Assert nào không chứng minh được thì **xoá**, không giữ làm cảnh.
* **[AC-06]** Không đổi `PASS_AT`, `THRESHOLD`, `MIN_INK`, `BAND_GAP_ROWS`, và không sửa fixture HTML để make nó pass.

## 5. Ràng buộc

* Sở hữu: `internal/raster/fonts.go` và file test của nó, `testdata/render-known-gaps.json`, `docs/tasks/TASK-027-GLYPH-COVERAGE.md`. Nếu cần chạm `internal/paint` hoặc `internal/layout` để vẽ tofu, ghi rõ ở Status **trước khi làm** — hai package đó có test riêng đang chạy song song.
* `internal/raster` là mắt xích trong frame path; **không** đổi import graph (`internal/archtest/rules.go:40-64`).
* Không `git push`, không merge `main`, không tạo branch phụ.
* Mọi test phải chạy trên máy không có display, và **không phụ thuộc font có trên máy này**: một test mà chỉ xanh trên macOS có `Arial Unicode.ttf` thì vô nghĩa trên CI Linux — phải có nhánh assert tương ứng cho host không có face đó.
