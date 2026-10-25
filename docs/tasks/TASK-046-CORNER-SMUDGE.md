# TASK-046 — vệt ink 20 px ở góc (0,0): đo tới stage nào thì nó thuộc stage đó

**Priority:** P3 (theo registry). **Method:** PROBE (đo theo từng stage, không suy đoán từ code) +
ORACLE vĩnh viễn trong package sở hữu lỗi.
**Defect:** `GOOSIE-LIVE-006` — **để open**; hai điều kiện đóng mà registry nêu không cái nào task
này tự đóng được bằng dữ liệu hợp lệ (§6, §7.3).
**Liên quan:** `docs/tasks/TASK-039-LIVE-PAGE-NO-OUTPUT.md` §5.5 + §5.11.4 (nhánh F),
`internal/paint/builder_box.go` (nơi sửa), `internal/paint/cmd.go:151-154` (quy ước bị vi phạm),
`internal/raster/tile.go:180-189, 297-305` (nơi quy ước đó được thi hành), `testdata/parity.py`
(chỉ đọc — không sửa trong task này).

Kết luận, tóm tắt trước khi đọc số:

* Stage sở hữu là **display list** (`internal/paint`). Không phải `raster`, không phải `frame`,
  không phải `surface`.
* Cả **hai nguyên nhân registry nghi ngờ đều bị dữ liệu bác**: tile không phải là "uncovered tile",
  và "glyph-atlas bleed" không tồn tại về mặt cấu trúc. Thứ gây ra vệt là **một lệnh paint thật**,
  do một box mà CSS nói không vẽ gì sinh ra.
* Mechanism underneath rộng hơn nhiều vệt góc: mọi element có `opacity: 0` đều được paint ở **alpha
  đầy đủ**, không phải alpha 0. Fixture `bg-op0.html` đo được **1600 px đỏ thuần** ở chỗ Chromium
  paint 0 px (§2.3).
* Đã sửa bằng hai hunk nhỏ trong stage sở hữu, có oracle + mutation proof, blast radius đo được
  **0/85 fixture render thay đổi** (§6).

## 1. Nghi vấn ban đầu, và vì sao phải đo

`docs/qa-open-defects.json` `GOOSIE-LIVE-006` (P3, open) ghi vệt ink nằm "inside (0,0)-(4,3)" và
yêu cầu người đóng defect nêu "**which of the two (uncovered tile, glyph-atlas bleed) it is**".
TASK-039 §5.5 đã đo hình dạng (20 px, rows 0–4 × cols 0–3, mẫu màu (104,104,104) (163,163,163)
(101,101,101), `MIN_INK = 200`) và §5.11.4 nói rõ cái còn thiếu là "probe phía `raster`/`frame`
với fixture tối thiểu".

Đọc code không phân biệt được hai giả thuyết đó với giả thuyết thứ ba (một lệnh trong display list),
vì cả ba đều cho ra đúng một vệt ở góc trên một tile được che phủ. Nên probe phải là probe đo.

## 2. Đo theo stage

### 2.1 Ba stage, một phép tách

Probe tạm (một Go test trong package `test/t046probe`, **đã xoá trước khi commit**, theo đúng yêu cầu
task) đi tuần tự ba stage trên cùng một document, mọi thứ deterministic, không mạng, viewport
1280×1024 dpr 1 — tức đúng cấu hình mà sweep sống dùng:

| Stage | Cách đo | Câu hỏi nó trả lời |
| --- | --- | --- |
| STAGE 1 | `Session.PaintChecked(1)` → `list.Build(1)` → đếm `DisplayCmd` có `Bounds()` cắt ô (0,0)–(4,5) | list có tự sinh lệnh ở góc không? |
| STAGE 2 | `raster.RasterizeTile(dl, Rect4(0,0,256,256), out, fonts, atlas, bg)` rồi đếm ink của tile | tile raster có thêm ink nào ngoài list không? |
| STAGE 3 | cùng `RasterizeTile` nhưng với `dl == nil` | bản thân đường tile/frame có paint gì lên một vùng trống không? |

Kết luận đo được (không phải kết luận suy ra):

* **STAGE 3 cho 0 px ink.** Một tile hoàn toàn được che phủ, display list rỗng, không để lại vệt
  nào. Nên "uncovered tile" **không phải** nguyên nhân: composer (`internal/surface/composer.go`)
  chỉ fill rect damage bằng nền rồi `BlitOver` tile đã clip; damage rỗng thì không ghi gì, và nó
  không có đường nào để tự sinh ra một vệt xám.
* **STAGE 2 cho số pixel bằng từng pixel với PNG** mà `cmd/goosie-headless` xuất ra cho cùng document.
  `raster` là một consumer trung thành của list, nó không nhân đôi hay rò rỉ ink.
* **STAGE 1 đã có đủ ink ngay trong list.** Đây là stage sở hữu.

Giả thuyết "glyph-atlas bleed" bị loại bằng cấu trúc, không cần đo: `internal/raster/atlas.go` là
một LRU các glyph mask **riêng từng glyph**, không có shared sheet, không có vùng kề nhau để rò rỉ.
Một vệt 4×5 px liền khối ở góc trái trên, phủ cả cột x=0..3, cũng không có hình dạng của glyph
antialiased.

### 2.2 Lệnh nào, hình dạng nào (số tái chạy được bằng tool có sẵn trong repo)

`cmd/dump-paint` trên fixture tối thiểu `shadow0.html` (một box 0×0 **nhìn thấy được**, có
`box-shadow: 0 1px 4px rgba(0,0,0,.2)`):

```
EXTENT Rect(-4,-3 8x8)
FILL rect=(-4,-3,4,0) color=51
FILL rect=(-4,0,4,5)  color=51
FILL rect=(-2,-1,2,0) color=51
FILL rect=(-2,0,2,3)  color=51
```

Bốn fill đó là hai ring của `paintBoxShadow` (`internal/paint/builder_effects.go`, `blurLayers`,
`steps = min(blur/2, maxBlurSteps = 4)`, `expand = i*blurLayers/steps`, `frac = i/steps`): một box
không cao thì mỗi ring chỉ còn hai strip trên/dưới là khác rỗng. Hai strip trên nằm trọn ở `y < 0`;
hai strip dưới `(-4,0,4,5)` và `(-2,0,2,3)` cắt ô gốc. **Đó là vệt, nguyên shape.**

Trên bản lưu của vnexpress.net, STAGE 0 (đi `sess.Arena.Objects`) tìm thấy đúng **hai** box có
border rect 0×0 toạ độ content (0,0), mỗi box có `box-shadow` như trên, và cả hai đều
`opacity: 0; visibility: hidden` — `.sub-area` và `.nav-folder .ul-nav-folder li.has-submn
.sub-more`. Hai box × bốn fill = tám fill chồng lên nhau; phần cắt ô tile gốc là 4 cột × 5 dòng =
**20 px**, bằng con số TASK-039 §5.5 đo offline.

### 2.3 Vì sao nó đậm bằng, và vì sao bug này vô hình

Đo PNG bằng chính `parity.ink_mask` (viewport 1280×1024, dpr 1, headless):

| Fixture | ink trước fix | ô 5×5 góc, trước fix | ink sau fix |
| --- | --- | --- | --- |
| `empty.html` | **0** | — | 0 |
| `shadow0.html` (box 0×0 visible, có shadow) | **20** | (184,184,184) / (204,204,204) | **20** (không đổi — paint đúng) |
| `shadow-op0.html` (thêm `opacity:0`) | **20** | (163,163,163) / (204,204,204) | **0** |
| `bg-op0.html` (`opacity:0` + `background:#f00` 40×40) | **1600** | toàn bộ (255,0,0) | **0** |
| vnexpress.net (bản lưu) | **20** | (104,104,104)/(163,163,163) hàng 0; (101,101,101)/(158,158,158) hàng 1–2; (158,158,158) hàng 3–4 | **0** |

`shadow0` so với `shadow-op0` là phép so sánh đắt nhất trong task này: **cùng hình dạng, cùng 20 px,
khác độ đậm**. Box visible paint các ring với alpha chiết khấu (`layerOpacity = opacity * frac`:
51·1.0 = 51 và 51·0.5 ≈ 26) → 255·0.8 = 204 và 255·0.8·0.9 = 184. Box `opacity:0` thì
`layerOpacity = 0 · frac = 0`, mà `0` nghĩa là "unset → opaque" (`applyOpacity`,
`internal/raster/tile.go:180-189`; `opacityCoverage`, `:297-305`), nên mỗi ring paint ở **alpha đầy
đủ 51**. Hai box nhân đôi số lớp: hàng 0 trên nền trắng

```
255 · 0.8^4 = 104   (2 blend: 163)      247 · 0.8^4 = 101   (2 blend: 158)
```

(hàng 0 trên nền trắng của document, hàng 1 trở xuống trên nền 247 của chính trang; cột x=0..1 chịu
4 blend vì cả hai ring của cả hai box đè lên nhau, cột x=2..3 chỉ chịu 2 blend vì ring nhỏ kết thúc
ở x=2.) Tức đúng bộ ba màu mà §5.5 ghi lại: (104,104,104), (163,163,163), (101,101,101). Không còn
nghi ngờ nào nữa: vệt là shadow của box `opacity:0`, và nó paint ở **alpha đầy đủ của shadow** thay
vì độ chiết khấu của blur, đúng như quy ước "0 = opaque" dự báo.

Cũng quy ước đó giải thích vì sao bug vô hình suốt thời gian dài: nó chỉ lộ khi **toán ra 0**, tức
chỉ khi element thật sự transparent hoàn toàn — trường hợp mà `internal/paint/cmd.go:151-154` và
hai hàm trên đều giả định "đã bị drop trước khi thành lệnh". Giả định đó chưa từng được thực thi
ở đâu trong builder. `bg-op0.html` là bằng chứng nó sai tới mức nào: **1600 px opaque đỏ** ở nơi
Chromium paint không một pixel nào. Vệt góc chỉ là trường hợp 20 px của cùng lỗi đó, tình cờ rơi
vào góc và tình cờ đủ lớn để bị nhìn thấy.

## 3. Stage sở hữu, và vì sao sửa ở đó

**`internal/paint`** — cụ thể `paintBox` / `paintContextFor` trong `internal/paint/builder_box.go`.
Lý do không sửa downstream:

* `raster` và `surface` đã hành xử **đúng theo hợp đồng** của chúng với dữ liệu chúng nhận. Một bộ
  lọc "bỏ ink gần (0,0)" ở composer sẽ là sửa triệu chứng, và sẽ vẫn để list phát hành lệnh cho
  những vùng mà CSS nói không vẽ.
* Sửa ở `style` (không cho `Opacity` 0 đi qua) thì ra ngoài phạm vi task này và đụng vào file mà
  agent khác đang sửa cùng lúc; và nó không xử lý nhánh `visibility: hidden`, vốn đã có mặt trong
  defect này (`shadow-op0` và `visibility:hidden` là hai box khác nhau trên vnexpress).

## 4. Fix

Hai hunk, 17 dòng thêm / 1 dòng xoá, trong cùng một file:

1. `internal/paint/builder_box.go:27-35` — `paintBox` trả về sớm khi `obj.Style.Opacity == 0`.
   `opacity: 0` làm tròn cả subtree về alpha 0, nên không một lệnh nào dưới nó được quyền tới
   display list; và cũng không được quyền mang xuống như command opacity, vì `DisplayCmd.Opacity`
   0 nghĩa là "opaque". Đây là đúng điều `cmd.go:151-154` đã viết thành hợp đồng.
2. `internal/paint/builder_box.go:73-80` — `boxPaintContext.hidden` giờ cũng bật khi
   `obj.Style.Visibility != "visible"`, ngoài điều kiện clip suy biến đã có. `hidden` là cổng "box
   này không tự vẽ gì" trong `paintBox`: decoration + contents bị bỏ, con vẫn được đi qua — đúng
   ngữ nghĩa `visibility: hidden` (subtree có thể override lại `visible`). Cách kiểm tra này lấy
   nguyên từ đường marker, `internal/paint/builder_controls.go:466`. `ComputedStyle.Visibility`
   mặc định `"visible"` (`internal/style/style.go:679`), kế thừa (`:2389`), và chỉ đổi khi có
   khai báo (`:2560`) — nên `!= "visible"` là an toàn với phần tử không khai báo gì.

Sau fix: `shadow-op0` 20 → 0, `bg-op0` 1600 → 0, vnexpress offline 20 → 0, còn `shadow0` (box
visible, shadow thật) giữ nguyên 20 px — nghĩa là fix không dập bóng hợp lệ.

## 5. Oracle và mutation proof

Oracle vĩnh viễn: **`test/paint/invisible_box_test.go`**, `TestBoxThatCSSPaintsNothingEmitsNoCommands`
(package `paint_test`, tức package sở hữu artefact). Nó asserts trên đại lượng thật — *số lệnh có
`Bounds()` cắt ô gốc, và tổng số lệnh* — trên document tối thiểu qua `Session.PaintChecked` (đúng
hợp đồng untrusted-input), với bốn case:

| Case | lệnh ở góc | lệnh tổng |
| --- | --- | --- |
| `opacity:0` | 0 | 0 |
| `visibility:hidden` | 0 | 0 |
| `opacity:0.5` (control) | 2 | 4 |
| visible (control) | 2 | 4 |

Hai control giữ fix đúng bề rộng: một cổng dập "mọi opacity", "mọi clip" hay "mọi box 0×0" cũng đỏ
y như không dập gì. Control visible còn assert union footprint của các lệnh ở góc **bằng**
`Rect4(-4, 0, 4, 5)` — hình dạng vệt đo được, nên hai case 0 ở trên so với một tái hiện thật, không
so với một document vốn chẳng paint gì.

Mutation, mỗi lần chạy `go test ./test/paint/ -run TestBoxThatCSSPaintsNothingEmitsNoCommands`
(khôi phục file từ backup sau mỗi lần; `git diff --stat` sau đó khớp 17/1):

| Mutation | Nội dung | Test đỏ | Exit |
| --- | --- | --- | --- |
| M1 | xoá guard `Opacity == 0` | `opacity_0`: "put 2 commands in the origin tile's corner, want 0: [fill(-4,0,4,5) a=51 op=0 fill(-2,0,2,3) a=51 op=0]" + "emitted 4 commands in total, want 0" | **1** |
| M2 | bỏ điều kiện `Visibility != "visible"` khỏi `hidden` | `visibility_hidden`: "put 2 commands … want 0: [fill(-4,0,4,5) a=51 op=1 fill(-2,0,2,3) a=51 op=0.5]" + "emitted 4 … want 0" | **1** |
| M3 | revert cả file về `HEAD` | cả hai case trên đỏ đồng thời | **1** |

`op=0` trên cả hai lệnh ở M1 là chính cơ chế được ghi lại trong failure output: opacity 0 bị mang
xuống command, và bên đọc nó coi 0 là opaque.

## 6. Blast radius, và một nhận định trong registry không tái hiện được

**Không chạy `make parity`, không chạy live sweep** (ràng buộc của task; harness thuộc agent khác).
Thay vào đó đo offline bằng hai binary build từ trước/sau fix, render **toàn bộ 85 fixture** trong
`testdata/render` ở đúng cài đặt mà `parity.py render_goosie` dùng (800×600, dpr 1), so bằng
`parity.ink_mask`: tổng ink, mask equal từng pixel, và ô 8×8 góc. Chạy lại ở 1440×900 dpr 2 cho cùng
kết luận:

```
fixtures=85 changed=0
```

⇒ Không cần dịch floor nào của corpus offline cho change này, và không fixture nào trong
`testdata/render*` cần cập nhật.

Một vế trong chính mô tả defect thì **không** tái hiện được offline: `parity.content_bands` **không**
đếm vệt này như một band:

```
/tmp/t046/vne-now.png        ink=   20 bands=0
/tmp/t046/shadow-op0.png     ink=   20 bands=0
/tmp/t046/bg-op0.png         ink= 1600 bands=1 [(0, 39, 0, 40, 1600)]
MIN_INK=200  MIN_BAND_HEIGHT=6  BAND_GAP=6
```

Một vệt 4×5 px rớt cả hai ngưỡng: cao 5 dòng < `MIN_BAND_HEIGHT = 6`, và 20 px < `MIN_INK = 200`.
Nên "floor trong band metric ignore a band of that size" mà registry xin **nhiều khả năng là không
cần**, và con số `content_bands = 1` ở TASK-039 §5.5 không tái hiện được trên ảnh offline bằng code
harness hiện tại. Thứ vệt này thực sự gây ra là `ink_goosie > 0` (20 offline, 32 trong sweep) — đủ
để một trang đọc như "Goosie có vẽ gì đó" thay vì "không vẽ gì", tức đúng triệu chứng trong tiêu đề
defect; nó không tự tạo band. Nhãn của vnexpress offline là `BLANK_OUTPUT` cả trước lẫn sau fix
(20 < 200; 0 < 200), nên task này **không** kỳ vọng đổi nhãn sống.

## 7. Không gate được — nói thẳng

1. **Không có bằng chứng trên corpus sống.** Task bị ràng buộc không chạy `make parity` lẫn live
   sweep, nên mọi số ở đây là offline. INK/nhãn của 100 trang sau fix có dịch không, và dịch theo
   hướng nào, là **chưa biết**. Cần run mới; out-dir nên đặt ngoài `docs/qa-live-parity/` để không
   ghi artifact.
2. **Không có fixture render mới.** `testdata/render*` nằm trong danh sách cấm sửa, và reference
   của nó phải sinh từ Chromium — task này không chụp ảnh Chromium cho `shadow-op0.html` /
   `bg-op0.html`. Nên oracle là test trên display list, **không phải** một pixel-diff fixture. Đây
   là trade-off có ý thức: nó gate đúng nguyên nhân, nhưng không gate hình dạng pixel cuối.
3. **Không đặt được band floor.** Registry xin một con số "set from measurement". Đo offline của
   §6 nói floor đó không cần cho chính vệt này; muốn trả lời definitively cần số đo từ corpus sống
   (§7.1). Integrator quyết; task này không đụng `docs/qa-open-defects.json`,
   `docs/qa-ratchet.json`, `docs/qa-test-catalog.md` hay `testdata/`.
4. **Điều kiện đóng thứ nhất của registry không thể trả lời như nó được hỏi.** Nó đòi nêu "uncovered
   tile hay glyph-atlas bleed". Dữ liệu nói **không phải cả hai**: không phải uncovered tile
   (STAGE 3, display list rỗng → 0 px ink), và không phải glyph-atlas bleed (atlas là LRU mask riêng
   từng glyph, không shared sheet). Task này đã nêu câu trả lời ở §2.1; ghi vào registry là việc của
   người khác.
5. **Chưa chứng minh trên native macOS.** Mọi số đo là `cmd/goosie-headless`; `make gate` chạy darwin
   backend nhưng là benchmark 600 frame, không phải một trang có box `opacity:0`.
6. **Còn một chỗ va chạm quy ước khác, chưa sửa.** `internal/paint/builder_controls.go:403` đặt
   `Opacity: float32(caretColor.A) / 255.0` — alpha caret hợp lệ có thể bằng 0 (caret trong), và
   `applyOpacity`/`opacityCoverage` sẽ đọc nó thành opaque. Cùng kiểu lỗi, đường code khác, chưa có
   fixture/oracle nên để ngỏ có chủ đích thay vì vá thêm một condition trong cùng một change.
7. **Chưa verify với change group-opacity đang chạy song song.** Cả hai hunk nằm trong đúng hai hàm
   mà agent kia đang sửa (`paintBox` 22-52, `paintContextFor` 54-82 của
   `internal/paint/builder_box.go`). Nếu group-opacity walk của họ cũng drop `opacity == 0`, hunk 1
   thành dự phòng (vẫn đúng, không đổi output); nếu họ thay cách tính `box.opacity`, M1 là mutation
   cần chạy lại. Oracle này là thứ nên gate change đó.
8. **Không gate hiệu năng.** Fix thêm một phép so sánh float mỗi box trong `paintBox`; không có
   benchmark nào trong task này đo chi phí đó. `make gate` vẫn trong vùng đã thấy:
   `mean_ms=1.464 p50_ms=1.558 miss_pct=5.28` (`rasterized=2976 reused=53441 failed=0 refused=0
   panics=0 evictions=0`).

## 8. Gates

| Gate | Lệnh | Exit |
| --- | --- | --- |
| gofmt | `gofmt -l internal/paint/builder_box.go test/paint/invisible_box_test.go` | **0** (không output) |
| vet | `go vet -all ./...` | **0** |
| test | `go test ./...` | **0** (42 package `ok`) |
| race | `go test -race ./...` | **0** (42 package `ok`, không `DATA RACE`) |
| archtest | `go test ./internal/archtest/` | **0** |
| gate | `make gate` | **0** |
| whitespace | `git diff --check` | **0** |

Không sửa import mới, không thêm package-level mutable global, không thêm `// qc:` marker. Binary
path đi qua `Session.PaintChecked`.

## 9. Chạy lại

Fixture tối thiểu nằm ngoài repo (`/tmp/t046/*.html`) vì `testdata/` bị cấm; nội dung của chúng
được mô tả trong §2.3 và reproduce được bằng `invisibleBoxDoc` trong oracle.

```bash
# 1. Hình dạng lệnh sinh ra vệt (tool có sẵn trong repo)
go run ./cmd/dump-paint /tmp/t046/shadow0.html      # 4 FILL, EXTENT Rect(-4,-3 8x8)
go run ./cmd/dump-paint /tmp/t046/shadow-op0.html   # EXTENT Rect(0,0 0x0), 0 lệnh  (sau fix)

# 2. Đo ink bằng chính hàm của harness
go build -o /tmp/t046/headless-after ./cmd/goosie-headless
/tmp/t046/headless-after -in /tmp/t046/shadow-op0.html -out /tmp/after.png -width 1280 -height 1024 -dpr 1
python3 - <<'PY'
import sys; sys.path.insert(0,'testdata')
import numpy as np; from PIL import Image; import parity
m = parity.ink_mask(np.asarray(Image.open('/tmp/after.png').convert('RGB')))
print(int(m.sum()), parity.content_bands(m))   # 0 []
PY

# 3. Oracle + mutation
go test -count=1 ./test/paint/ -run TestBoxThatCSSPaintsNothingEmitsNoCommands

# 4. Blast radius offline (cần cả binary trước fix, build từ HEAD trong một bản clone tạm)
python3 - <<'PY'
import os, sys, subprocess, numpy as np
from PIL import Image
sys.path.insert(0, "testdata"); import parity
root, before, after = "testdata/render", "/tmp/t046/headless-before", "/tmp/t046/headless-after"
def shot(binary, src, tag):
    out = f"/tmp/t046/sweep/{tag}/{src.replace('/', '_')}"
    subprocess.run([binary, "-in", os.path.join(root, src), "-out", out,
                    "-width", "800", "-height", "600", "-dpr", "1"],
                   check=True, capture_output=True)
    return parity.ink_mask(np.asarray(Image.open(out).convert("RGB")))
fx = sorted(os.path.relpath(os.path.join(d, f), root)
            for d, _, fs in os.walk(root) for f in fs if f.endswith(".html"))
changed = [s for s in fx if not np.array_equal(shot(before, s, "b"), shot(after, s, "a"))]
print(f"fixtures={len(fx)} changed={len(changed)}")
PY
```
