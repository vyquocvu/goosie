# E2E Test Infra: Goosie Browser Engine Live URL Testing

## Test Philosophy
- Opaque-box, requirement-driven verification of Goosie browser engine against live target websites.
- Methodology: 4-Tier Test Architecture (Category-Partition, Boundary Value Analysis, Pairwise Combinations, Real-World Workload Testing).

## Feature Inventory & Target Websites
| # | Target Website | Category / Architecture | Tier 1 (Basic) | Tier 2 (Boundary) | Tier 3 (Cross-Feature) | Tier 4 (Workload) |
|---|----------------|-------------------------|:--------------:|:-----------------:|:----------------------:|:-----------------:|
| 1 | `https://example.com` | Viewport units (`vw`/`vh`), centering | 5 | 5 | ✓ | ✓ |
| 2 | `https://www.iana.org/help/example-domains` | External CSS, jQuery, navigation | 5 | 5 | ✓ | ✓ |
| 3 | `http://info.cern.ch/hypertext/WWW/TheProject.html` | Plain HTTP, uppercase HTML 1.0, `<dl>` | 5 | 5 | ✓ | ✓ |
| 4 | `https://text.npr.org` | Inlined CSS, gradients, `:after` pseudo | 5 | 5 | ✓ | ✓ |
| 5 | `https://motherfuckingwebsite.com` | User-agent default styles, `font:` shorthand | 5 | 5 | ✓ | ✓ |
| 6 | `https://paulgraham.com/articles.html` | Nested tables, legacy attributes, DOM iteration | 5 | 5 | ✓ | ✓ |
| 7 | `https://html5zombo.com` | Inline SVG, `<audio>`, animation timers | 5 | 5 | ✓ | ✓ |
| 8 | `https://danluu.com` | Flexbox layout, custom `<d>` tags, pre blocks | 5 | 5 | ✓ | ✓ |
| 9 | `https://lite.cnn.com` | CSS variables, JSON-LD, consent scripts | 5 | 5 | ✓ | ✓ |
| 10 | `https://go.dev/doc/` | External CSS, navigation drawer, `site.js` | 5 | 5 | ✓ | ✓ |

## Test Architecture
- **Live URL Test Runner**: `test/e2e/live_urls_test.go` (`-tags="e2e online"`) and `cmd/perf-review`.
- **Validation Channels**:
  1. HTTP 200 navigation & non-empty title extraction.
  2. DOM node tree population and non-empty snapshot generation.
  3. Screenshot / display list rasterization (non-blank RGBA image output).
  4. JavaScript execution & DOM query evaluation without unhandled exceptions or fatal panics.
- **Unit & Conformance Checks**:
  1. `go vet ./...` (0 warnings/errors)
  2. `go test ./...` across all packages
  3. `go test ./test/internal/renderer/layoutgolden/`
  4. `go run ./cmd/perf-review -urls="" -iterations=3`

## Coverage Thresholds
- Tier 1: Feature Coverage (50 test assertions covering all 10 URLs navigation & baseline render).
- Tier 2: Boundary & Corner Cases (empty responses, uppercase HTML, deep nesting, legacy tags, data scripts).
- Tier 3: Cross-Feature Combinations (CSS variables + flexbox, table layout + DOM query, audio play + timers).
- Tier 4: Real-World Application Workloads (full perf-review test sweep across all 10 URLs with 3 iterations).
