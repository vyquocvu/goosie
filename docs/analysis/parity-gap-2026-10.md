# Render parity 差距根因分析（2026-10）

- 分析日期：2026-10-24
- 代码版本：worktree HEAD `c8cd80b`（仅 docs 提交）；最新 CI run `40107385303`（head `c8cd80b`），失败 run `40069270435`（head `63d226b`，即 `docs/engine-compatibility-review.md:318-328` 引用的 run）
- 本机环境：macOS 25.6.0 arm64，Go 1.26.9，Python 3.9.6，Playwright 1.49.1，Chromium 131.0.6778.33（playwright build v1148）
- CI 环境：ubuntu-24.04（runner image `20261012.334.1`），Python 3.12.15，Playwright 1.49.1，Chromium 131.0.6778.33（playwright build v1148），`fonts-liberation 1:2.1.5-3`、`fonts-unifont 1:15.1.01-1build1`

## 结论

1. **差距不是引擎在 Linux 上“行为不同”，而是字体输入/oracle 未受控**：Goosie 的系统字体加载只认 macOS 路径（`internal/raster/fonts.go:105-157`），Linux 上所有字体槽都退回内置 Go 字体；Chromium 在 Linux 上则用 Liberation/DejaVu。同一 commit、同一 Playwright 1.49.1 / Chromium 131.0.6778.33 下，本机 macOS 复现 **80/80、96.085%**，把 Goosie 的字体输入切到“仅内置字体”（等价 Linux CI 状态）后降到 **68/80、94.060%**，几乎重合 CI 的 **66/80、93.638%**。
2. **“Chromium 版本未固定、每次取最新”不成立**：CI 日志实际下载的是 `Chromium 131.0.6778.33 (playwright build v1148)`，与本机一致；`playwright==1.49.*` 已经把浏览器 revision 绑定到 Playwright 版本，不是浮动最新版。文档里的 93.54% 来自旧 run，最新 run 是 93.638%。
3. **30/channel 容差不是跨平台差距的原因**（两边用同一阈值），但它是 90% 门槛敏感度的来源之一；阈值扫描显示即使放宽到 80/channel，内置字体仍会有 4 个 fixture 低于 90%，不能用“调阈值”静默关掉差距。

## 证据表（假设 → 结论 → 证据 → 可信度 → 最便宜的证伪实验）

| # | 假设 | 结论 | 证据 | 可信度 | 最便宜的证伪实验 |
| --- | --- | --- | --- | --- | --- |
| (a) | CI(Linux) 与 macOS 的字体/抗锯齿不同 | **成立（主因）** | CI 安装 `fonts-liberation`、`fonts-unifont`（CI run `40069270435` 日志）；Goosie 只读 macOS 字体路径（`internal/raster/fonts.go:105-157`）；本机 macOS 96.085%/80 通过 vs 仅内置字体 94.060%/68 通过（附录 A2/A3） | 高 | 已做：`GOOSIE_SYSTEM_FONTS=off` 重渲染后分数从 96.085 掉到 94.060 |
| (b) | Chromium 版本未固定，每次 `install` 取最新 | **排除** | CI 日志：`Downloading Chromium 131.0.6778.33 (playwright build v1148)`；本机 `Chromium --version` 同为 131.0.6778.33；CI 用 `playwright==1.49.*`（`.github/workflows/ci.yml:92-95`） | 高 | 已做：对比 CI 日志与本机 `--version` |
| (c) | 评分容差 30/channel 过松或过严 | **不是跨平台差距的原因；是门槛敏感度来源** | 阈值扫描（附录 A5）：thr=30 时 macOS 0 失败、内置字体 12 失败；thr=80 时内置字体仍 4 失败；thr=25 时 macOS 反而 1 失败 | 高 | 已做：同一批渲染按 0–80 重新评分 |
| (d) | 引擎自身在 Linux 上的字体回退/度量差异 | **成立（机制）** | Linux 上 `NewFonts` 读不到任何 `systemFontFiles`/`systemFontCollections`，`resolveLocked` 退回内置 Go regular/bold（`internal/raster/fonts.go:162-197, 296-327`）；默认字体是 Times（`internal/style/style.go:675`）；语料中 61 条 `font-family: sans-serif` 声明 | 高 | 已做：等价环境重渲染；也可在 Linux 上跑 `go run ./cmd/dump-fonts` 验证无系统字体 |
| (e1) | 滚动条占位差异 | **排除** | Playwright 默认参数包含 `--hide-scrollbars`（本机 `.../playwright/driver/package/lib/server/chromium/chromium.js:275`） | 高 | 在 Linux 上截图对比 `window.innerWidth`/`clientWidth` |
| (e2) | DPR/视口/尺寸不一致 | **排除** | `parity.py` 显式 800×600、DPR 1（`testdata/parity.py:51-70, 534-535`）；CI 与本地均 0 个 dimension error | 高 | 已做：CI JSON `errors=0` |
| (e3) | Web 字体加载竞态 | **排除** | `testdata/render` 中 `@font-face` 数量为 0，且无外部 `src`/`href` 引用（附录 A8） | 高 | 已做：grep 语料 |
| (e4) | 颜色管理/Chromium 平台抗锯齿 | **次要（残余约 0.42pp）** | 低文字量（<100 字符）的 5 个 fixture 在 CI 仅掉 0.21–0.56pp（附录 A7）；但剩余 0.42pp 未直接测量 | 中 | 在 Linux 上单独跑低文字量 fixture；或对同一 fixture 做 0/30 阈值差分 |
| (e5) | GPU/软件光栅化差异 | **未验证** | CI headless Chromium 的 GPU 路径未记录 | 低 | 固定 `--disable-gpu`/`--use-gl=swiftshader` 后重跑 |

### 关键量化分解

| 环境 | 平均匹配 | 通过数 | 来源 |
| --- | --- | --- | --- |
| 本机 macOS，Goosie 用系统字体（默认） | **96.085%** | **80/80** | `/tmp/parity-local.json`（附录 A2） |
| 本机 macOS，Goosie 仅内置字体（等价 Linux CI 字体输入） | **94.060%** | **68/80** | `/tmp/parity-nofonts.json`（附录 A3） |
| CI 最新 run `40107385303`（`c8cd80b`） | **93.638%** | **66/80** | `/tmp/parity-ci-latest/parity.json` |
| CI 文档引用 run `40069270435`（`63d226b`） | **93.539%** | **66/80** | `/tmp/parity-ci-report/parity.json` |

- 总差距（本机 macOS − 最新 CI）：`96.085 − 93.638 = 2.447pp`。
- 其中 Goosie 侧字体输入可解释：`96.085 − 94.060 = 2.025pp`，约 **83%**。
- 残余约 `94.060 − 93.638 = 0.422pp`（≈17%），归因于 Chromium 在 Linux 侧的字形替换/抗锯齿；这是估算，因为“仅内置字体”是对 macOS Chromium 参考评分，而不是对 Linux Chromium 评分。

### 机制：为什么字体差异会放大到 10+ pp

字体替换不只改字形，还改 advance/line-height，导致换行和整块内容位移：

- `internal/raster/fonts.go:105-157`：`systemFontDir = "/System/Library/Fonts/Supplemental"`，只列 macOS 的 Times/Arial/Courier/Georgia/Verdana 与 `HelveticaNeue.ttc`。
- `internal/raster/fonts.go:170-195`：文件读不到就 `continue`；Linux 上全部跳过。
- `internal/raster/fonts.go:296-327`：找不到槽位时先降级同族，再落到内置 face；内置只有 Go regular/bold（`internal/raster/fonts.go:199-218`）。
- 同一份 Goosie 渲染，仅切换字体输入（系统字体 vs 内置字体），14 个 CI 失败 fixture 的平均像素差异为 **10.62%**（中位数 10.57%，最大 17.72%），而 66 个通过 fixture 平均仅 **3.79%**（中位数 4.09%）（附录 A6）。
- `colors/01-text-background` 的分带差异从顶部约 5–7% 增长到下半部 20–31%（附录 A9），是典型的文本度量变化导致的纵向重排，而不是局部字形抗锯齿。
- `colors/02-gradients` 实际内容是纯色背景 + 白字（`testdata/render/colors/02-gradients.html:13-32`，没有任何 gradient），CI 仍掉 9.31pp；它不可能是渐变实现问题，只能是文字/字体差异。

## 14 个低分 fixture（最新 CI run `40107385303`）

| Fixture | CI 分数 | macOS 分数 | 掉幅 | 可见字符数 | 类别 |
| --- | ---: | ---: | ---: | ---: | --- |
| colors/01-text-background | 80.33% | 93.64% | 13.31 | 501 | 颜色 |
| selectors/01-basic-selectors | 81.64% | 94.15% | 12.51 | 878 | 选择器 |
| positioning/03-fixed | 83.95% | 90.64% | 6.69 | 2003 | 定位 |
| tables/02-colspan-rowspan | 86.39% | 96.79% | 10.40 | 345 | 表格 |
| tables/03-table-styling | 87.08% | 97.43% | 10.34 | 313 | 表格 |
| edge-cases/01-long-content | 87.08% | 90.47% | 3.39 | 3156 | 边界用例 |
| colors/02-gradients | 87.19% | 96.50% | 9.31 | 220 | 颜色 |
| edge-cases/02-deep-nesting | 88.37% | 98.69% | 10.31 | 705 | 边界用例 |
| typography/04-text-align | 88.65% | 91.08% | 2.43 | 788 | 文字 |
| layout-features/01-flexbox-complete | 88.76% | 90.12% | 1.36 | 553 | 布局 |
| typography/06-line-height | 88.78% | 92.03% | 3.24 | 1129 | 文字 |
| overflow/01-overflow | 89.28% | 90.40% | 1.12 | 705 | 溢出 |
| typography/09-white-space | 89.62% | 91.66% | 2.04 | 659 | 文字 |
| html-features/02-semantic-html | 89.90% | 95.32% | 5.42 | 1302 | HTML |

分类汇总：文字类 3、表格类 2、颜色类 2、边界用例 2，定位/选择器/布局/溢出/语义各 1。

- **14 个失败 fixture 全部含文字**（最少 220 字符，中位数 705，最多 3156）；而可见字符 <100 的 5 个 fixture 全部通过，CI 掉幅仅 0.21–0.56pp（附录 A7）。
- 字符数与掉幅的 Pearson 相关系数只有 **0.156**（附录 A7）：文字量本身不是线性预测因子，因为字体替换主要通过“换行/行高导致整块位移”放大差异，而不是按文字面积成比例。强证据是受控实验（仅改字体输入，失败集从 0 变 12），不是字符数相关。

## 建议

### P0 — 让门槛可靠且可复现（最小改动）

1. **把 oracle 元数据写进报告并在启动时断言（不需要改引擎）**
   - 将 CI 的 `pip install "playwright==1.49.*"` 固定为 `playwright==1.49.1`（`.github/workflows/ci.yml:94`）；记录 Chromium build（`playwright install` 输出的 v1148 / `131.0.6778.33`）、runner image、`dpkg -l 'fonts-*'` 版本。
   - `parity.py` 目前在 `render_chromium` 中只 `pw.chromium.launch()`，不记录浏览器 build（`testdata/parity.py:110-112`）；应把 Playwright 版本、Chromium 版本、OS、字体清单写入 JSON 并在评分前断言。
   - CI 目前只上传 JSON/JUnit（`.github/workflows/ci.yml:98-105`）；应同时上传 `/tmp/goosie-renders`、`/tmp/playwright-renders` 和 `--diffs` 产物，否则失败无法从 CI artifact 直接定位。
2. **确定“权威平台”（需要产品负责人决策）**
   - **推荐 A：把 gating parity 放到 macOS**，让 Goosie 和 Chromium 使用同一套 Apple 系统字体；本机已复现 **96.085%、80/80**。Ubuntu parity 保留为 report-only。代价是 macOS runner 成本；这是产品负责人决策。
   - **备选 B：继续用 Ubuntu gating，但先做 P1-1 字体对齐**；在对齐完成前，用“Linux 基线 + 回归 delta”而不是绝对的 90% pass_at 作为门槛（当前 Linux 基线 93.638%、66/80）。**这属于放宽/改变测试语义，必须由产品负责人明确批准，不能静默修改。**
   - 不建议直接把 90% 或 30/channel 调低来“修绿”；阈值扫描证明放宽到 80 仍有 4 个结构性失败。
3. **在字体对齐前加平台化 expected-failure 清单**
   - 以 `平台 + oracle fingerprint` 为 key，记录当前 14 个已知平台性失败，带 owner 和到期日；清单之外的新失败仍然 fail。这是保住回归信号的最小折中，也需要产品负责人批准。

### P1 — 后续改进

1. **给 `internal/raster/fonts.go` 增加 Linux 字体源**：使用 fontconfig 或固定目录加载 Liberation Sans/Serif/Mono（Playwright `--with-deps` 已安装），按 Chromium 的 fontconfig 替换关系映射槽位；特别要验证 generic `sans-serif`（Goosie 现在映射到 `FontArial`，见 `internal/style/style.go:4237`）与 Linux Chromium 的实际 generic 解析是否一致。需要新增测试覆盖“字体存在/缺失”两种分辨率。
2. **提交或在固定环境生成 Chromium 参考 PNG**：带 SHA-256、浏览器 build、OS、字体集元数据；不要用 macOS 现场生成的参考去评 Linux 现场生成的 Goosie。
3. **保留 30/channel、90% 作为回归围栏，而不是兼容性结论**（与 `docs/engine-compatibility-review.md:365` 一致）；报告中按平台、按 fixture 发布分数和分母。
4. **报告分层**：把“文字/字形差异”和“布局/颜色差异”分开统计，避免字体噪声掩盖真正的引擎缺陷。
5. **增加 macOS ARM64 CI job**（`docs/engine-compatibility-review.md:371`），同时覆盖 cgo 平台代码与 macOS parity。

### 需要产品负责人决策的事项

- 权威 gating 平台选 macOS 还是 Linux（以及随之而来的 CI 成本）。
- 任何对 `pass_at` / `threshold` / expected-failure 清单的修改——这些都属于放宽测试，必须显式批准。
- 是否 bundle 开源字体（Liberation/DejaVu 可再分发；Apple 系统字体不可再分发）。
- 是否接受“字体对齐前 Linux parity 只做 report-only”。

## 未能运行 / 未能验证

1. **没有 Linux 运行环境，也没有 Docker**（`docker unavailable`），因此没有在本机直接跑 Linux 版 Goosie/Chromium。替代实验是 macOS 上 `GOOSIE_SYSTEM_FONTS=off`：因为渲染管线没有任何 `runtime.GOOS` 分支（对 `internal/engine|raster|paint|layout|style|dom|frame` 的 grep 无结果），而 Linux 上 `NewFonts` 的行为恰好等于 `NewEmbeddedFonts` + 读取 macOS 路径全部失败。
2. **CI artifact 只有 JSON/JUnit，没有 PNG**，所以无法对 CI 的实际渲染做逐像素/分带分析；分带分析用的是本机渲染。
3. 没有运行 `go test ./...` / `go test ./internal/archtest/`：本次是只读调查且未改任何源码；只运行了 `make build` 与 parity。
4. 没有直接验证 Ubuntu runner 上 `sans-serif` 的 fontconfig 最终解析族（Liberation vs DejaVu）；`fonts-liberation`、`fonts-unifont` 的安装有 CI 日志证据，DejaVu 是 ubuntu-24.04 镜像常规内容但未在本任务中直接取证。
5. 文档中的 95.99% 未能按其原始环境精确复现（未记录 oracle 版本）；但用当前本机 Playwright 1.49.1 / Chromium 131.0.6778.33 复现到 96.085%，结论不受影响。

## 残余风险

- “仅内置字体”实验是在 macOS 上对 macOS Chromium 评分；残余 0.42pp 的 Linux 侧分摊是估算，不是直接测量。
- **runner image 字体集未固定**：即使 Playwright 固定，ubuntu-24.04 镜像和 `fonts-*` 包仍可能随 runner 更新而漂移；这是当前门槛不可复现的另一来源。
- `playwright==1.49.*` 只固定到 patch 系列；虽然浏览器 build 通常随 Playwright 版本固定，但应改为精确版本并断言。
- 字体对齐后，当前被字体噪声掩盖的真实引擎差异可能暴露出来（例如阴影、渐变、文本抗锯齿），届时仍会有 fixture 低于 90%；这是预期内的“真实信号”，不是新回归。
- 本次未改任何代码或测试；上述 P0/P1 的落地需要单独的实现任务与产品决策。

---

## 附录：命令与真实输出

### A1. 构建

```text
$ make build
Building goosie...
go build -trimpath -ldflags "-s -w" -o bin/goosie ./cmd/goosie
Build complete: bin/goosie
```

### A2. 本机 macOS 全量 parity（系统字体，Playwright 1.49.1 / Chromium v1148）

```text
$ python3 testdata/parity.py --render --render-chromium --binary ./bin/goosie \
    --json /tmp/parity-local.json --junit /tmp/parity-local.xml
...
  80/80 passing  (100.0%), 0 failing, 0 errors
  average match: 96.09%  (threshold=30, pass_at=90.0%)
```

`/tmp/parity-local.json` summary: `avg_match_pct=96.085, passing=80, failing=0, errors=0`。

### A3. 仅内置字体（等价 Linux CI 字体输入）

```text
$ GOOSIE_SYSTEM_FONTS=off python3 testdata/parity.py --render --binary ./bin/goosie \
    --json /tmp/parity-nofonts.json
...
  68/80 passing  (85.0%), 12 failing, 0 errors
  average match: 94.06%  (threshold=30, pass_at=90.0%)

  Failing fixtures:
    edge-cases/02-deep-nesting.png  (89.95%)
    typography/09-white-space.png  (89.88%)
    typography/04-text-align.png  (89.63%)
    colors/02-gradients.png  (89.37%)
    typography/06-line-height.png  (89.18%)
    tables/03-table-styling.png  (89.01%)
    layout-features/01-flexbox-complete.png  (88.97%)
    tables/02-colspan-rowspan.png  (88.65%)
    edge-cases/01-long-content.png  (87.73%)
    positioning/03-fixed.png  (84.92%)
    colors/01-text-background.png  (82.90%)
    selectors/01-basic-selectors.png  (82.26%)
```

### A4. CI 日志（run `40069270435`，parity job `120798981594`）

```text
2026-10-24T06:13:13.1953037Z fonts-liberation is already the newest version (1:2.1.5-3).
2026-10-24T06:13:18.3078218Z Setting up fonts-unifont (1:15.1.01-1build1) ...
2026-10-24T06:13:20.3965614Z Downloading Chromium 131.0.6778.33 (playwright build v1148) from https://playwright.azureedge.net/builds/chromium/1148/chromium-linux.zip
2026-10-24T06:13:24.8254444Z Chromium 131.0.6778.33 (playwright build v1148) downloaded to /home/runner/.cache/ms-playwright/chromium-1148
...
2026-10-24T06:13:49.9305928Z   66/80 passing  (82.5%), 14 failing, 0 errors
2026-10-24T06:13:49.9306502Z   average match: 93.54%  (threshold=30, pass_at=90.0%)
```

### A5. 阈值扫描（同一批渲染，多次评分）

```text
thr  mac avg  mac<90 nofont avg  nofont<90
  0    92.90      12      90.68         24
  5    94.74       7      92.27         21
 10    94.97       6      92.45         20
 15    95.17       6      92.66         20
 20    95.76       4      93.73         15
 25    95.94       1      93.91         14
 30    96.09       0      94.06         12
 35    96.22       0      94.23          9
 40    96.37       0      94.38          8
 50    96.60       0      94.64          8
 60    96.85       0      94.87          6
 80    97.27       0      95.32          4
```

### A6. 仅改字体输入对 Goosie 自身输出的影响

```text
Goosie macfonts vs goosie embedded-only, mismatch%: all=4.98  failing-set=10.62  passing-set=3.79
  failing fixtures n=14 min/median/max=2.28/10.57/17.72
  passing fixtures n=66 min/median/max=0.46/4.09/7.60
```

### A7. 文字量与 CI 掉幅

```text
Pearson r(chars, mac-ci drop) = 0.156
FAIL n=14 chars min/median/max=220/705/3156
PASS n=66 chars min/median/max=55/418/2759
Under 100 chars: [
  ('borders/03-border-colors.png', 65, 'PASS', 0.26),
  ('colors/03-opacity.png', 93, 'PASS', 0.56),
  ('edge-cases/05-zero-border.png', 91, 'PASS', 0.47),
  ('edge-cases/06-color-scheme-list.png', 67, 'PASS', 0.21),
  ('edge-cases/07-webp-image.png', 55, 'PASS', 0.25)
]
```

### A8. 语料中没有 Web 字体/外部资源

```text
$ grep -rl "@font-face" testdata/render --include='*.html' | wc -l
       0
$ grep -rhoE "(src|href)=\"[^\"]+\"" testdata/render --include='*.html' | grep -vE '^\w+="(#|data:)' | sort -u
(no output)
```

### A9. 分带差异（50px 行带，颜色/文本类最差 fixture）

```text
== colors/01-text-background.png 800x600
  macfonts vs chromium   overall= 6.36%  rows(50px)= 1.5  5.5  5.0  3.7  6.9  3.8  4.4 22.1  3.8  7.0  8.7  4.0
  nofonts vs chromium    overall=17.10%  rows(50px)= 3.6  6.4  6.7  6.7 16.6 17.5 19.9 30.7 25.8 24.9 21.6 24.9
  nofonts vs macfonts    overall=17.72%  rows(50px)= 3.7  2.5 10.4  6.3 20.1 19.4 19.2 30.4 27.7 26.4 21.6 24.9

== selectors/01-basic-selectors.png 800x600
  macfonts vs chromium   overall= 5.85%  rows(50px)= 4.9  6.6  6.8  4.8  6.4  5.3  5.6  7.0  7.5  5.6  4.0  5.6
  nofonts vs chromium    overall=17.74%  rows(50px)= 4.9  8.5 14.2 11.5 17.2 16.7 17.6 15.9 16.9 13.4 29.3 46.8
  nofonts vs macfonts    overall=17.27%  rows(50px)= 5.1  8.3 13.9 11.2 16.4 16.4 16.7 15.3 15.7 12.9 29.1 46.2

== tables/02-colspan-rowspan.png 800x600
  macfonts vs chromium   overall= 3.21%  rows(50px)= 2.9  1.6  3.0  4.8  2.7  2.9  3.4  3.4  3.9  4.2  4.2  1.4
  nofonts vs chromium    overall=11.35%  rows(50px)= 6.9  2.9 13.0  8.0  7.1 20.8  7.1  9.0 10.0 13.2 13.4 24.8
  nofonts vs macfonts    overall=10.67%  rows(50px)= 6.9  2.8 12.5  5.7  6.8 20.6  6.4  8.4  9.2 11.6 12.6 24.5
```

### A10. 最近 CI run 列表

```text
40107385303 c8cd80b failure 2026-10-24T15:31:45Z docs(engine): add comprehensive engine c
40069270435 63d226b failure 2026-10-24T06:12:20Z chore(ci): consolidate CI workflows and
39990407251 31445ab failure 2026-10-23T16:52:39Z Implement SVG rendering and text paintin
39990406790 31445ab failure 2026-10-23T16:52:39Z Implement SVG rendering and text paintin
39990406742 31445ab failure 2026-10-23T16:52:39Z Implement SVG rendering and text paintin
39935672868 730cb0d failure 2026-10-23T09:57:55Z Add headless browser E2E and refactor or
```

最新 run `40107385303` 的 parity artifact summary：

```json
{
  "total": 80,
  "passing": 66,
  "failing": 14,
  "errors": 0,
  "avg_match_pct": 93.638,
  "threshold": 30,
  "pass_at": 90.0,
  "all_pass": false
}
```
