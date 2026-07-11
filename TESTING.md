# Goosie Browser Testing Strategy

This document outlines the comprehensive Test-Driven Development (TDD) approach for the Goosie browser project.

## Test Tiers

Use `go test ./... -short` for sandbox-safe checks. These tests do not require external network access, loopback listeners, GUI launch permissions, or Playwright.

Use `go test ./...` for the normal local suite. Tests that require `httptest` loopback servers skip themselves when the host environment forbids opening a listener.

Use `go test -tags=e2e ./test/e2e` for Playwright-driven browser tests. This tier requires Playwright browsers and host permissions to launch Chromium.

Use package-specific commands such as `go test ./internal/net -run TestServiceFetchRecordsRequestLog` while developing a focused subsystem.

## 1. Testing Philosophy

We follow a strict **Test-Driven Development (TDD)** process:
1.  **Red**: Write a failing test for a small piece of functionality.
2.  **Green**: Write just enough code to make the test pass.
3.  **Refactor**: Clean up the code while ensuring tests still pass.

All new features must be accompanied by tests. Bugs should be reproduced with a failing test case before being fixed (Regression Testing).

## 2. Test Layers

We organize tests into the following layers:

### 2.1 Unit Tests
*   **Scope**: Individual functions, methods, and classes.
*   **Location**: Co-located with the source code (e.g., `parser.go` -> `parser_test.go`).
*   **Focus**: Logic correctness, edge cases, error handling.
*   **Dependencies**: Mocked or stubbed.
*   **Key Components**:
    *   CSS Parser (`internal/css`)
    *   DOM Parser (`internal/dom`)
    *   JS Runtime (`internal/js`)
    *   Networking (`internal/net`)

### 2.2 Integration Tests
*   **Scope**: Interaction between two or more components.
*   **Location**: `_test.go` files that import multiple internal packages, or specific integration test suites.
*   **Focus**: Data flow between components (e.g., HTML -> DOM -> Layout -> Renderer).
*   **Key Scenarios**:
    *   Parsing HTML and applying CSS styles.
    *   Fetching resources and loading them into the cache.

### 2.3 End-to-End (E2E) Tests
*   **Scope**: Full user workflows simulation.
*   **Location**: `test/e2e`
*   **Focus**: User experience, UI interactions (clicks, input), critical paths.
*   **Tools**: Playwright for browser automation, custom helpers in `internal/testutil`.
*   **Build Tag**: Requires `-tags=e2e` to run.

#### Real Website Testing

The `test/e2e/real_websites_test.go` suite validates the browser engine against 10 live websites across three complexity tiers. Tests are milestone-gated using the `GOOSIE_MILESTONE` environment variable (default: `2`).

**Test Categories:**

- **TestRealWebsitesFetchAndParse** (M1+): HTTP fetching, HTML structure validation, content verification
- **TestRealWebsitesRendering** (M1+): Full render pipeline with screenshot capture
- **TestHTTPSchemeHandling** (M1+): HTTPS scheme handling via Goosie net layer
- **TestRealWebsitesByCategory** (M1+): Complexity-tier grouped rendering validation
- **TestRealWebsitesGoosieVsBrowser** (M3+): Visual comparison against Playwright/Chromium
- **TestRealWebsitesCSSParsing** (M3+): CSS-heavy site rendering and selector matching
- **TestFetcherResponseMetadata** (M1+): Response metadata preservation

**Website Tiers:**

| Complexity | Sites | Required Milestone |
|------------|-------|-------------------|
| Simple | example.com, iana.org, info.cern.ch, httpbin, testing.toscrape | M1 |
| Medium | w3schools, lipsum, quotes.toscrape | M2 |
| Complex | wikipedia, MDN | M3 |

**Running Real Website Tests:**

```bash
# Run all e2e tests at current milestone (M2)
go test -tags=e2e ./test/e2e/

# Unlock M3 tests
GOOSIE_MILESTONE=3 go test -tags=e2e ./test/e2e/ -run TestRealWebsites

# Run specific test
GOOSIE_MILESTONE=3 go test -tags=e2e ./test/e2e/ -run TestRealWebsitesCSSParsing
```

All tests use graceful degradation with `t.Skipf` for network failures and `context.WithTimeout` for cancellation support.

#### Roadmap Verification

The `cmd/roadmap_test/main.go` provides a standalone verification tool that tests the engine pipeline against real websites. It validates:

- Phase 1: HTTP fetching, HTML parsing, JavaScript execution
- Phase 2: Console API, error reporting
- Phase 3: CSS parsing, layout engine
- Phase 4: Screenshot capability
- Phase 5: Real website integration (M1-M3 pipeline)

```bash
# Run at current milestone
go run ./cmd/roadmap_test/

# Run at M3
GOOSIE_MILESTONE=3 go run ./cmd/roadmap_test/
```

### 2.4 Performance Tests
*   **Scope**: Speed and memory usage.
*   **Location**: `_test.go` files with `Benchmark` functions.
*   **Focus**: Rendering speed, layout calculation time, memory allocations.

### 2.5 Security Tests
*   **Scope**: Vulnerability detection.
*   **Location**: `test/security` or specific unit tests.
*   **Focus**: XSS prevention, CSRF protection, safe content handling.

### 2.6 Accessibility (A11y) Tests
*   **Scope**: Compliance with WCAG guidelines.
*   **Focus**: Color contrast, keyboard navigation, screen reader support (simulated).

## 3. Naming Conventions

*   **Test Files**: `filename_test.go`
*   **Test Functions**: `TestFunctionDescription` (e.g., `TestParseCSSColor_ValidHex`)
*   **Benchmarks**: `BenchmarkFunctionDescription`
*   **Examples**: `ExampleFunctionDescription`

## 4. Tools & Libraries

*   **Framework**: `testing` (Go standard library)
*   **Assertions**: `github.com/stretchr/testify/assert`, `github.com/stretchr/testify/require`
*   **Mocking**: `github.com/stretchr/testify/mock`
*   **UI Testing**: `fyne.io/fyne/v2/test`
*   **JS Testing**: `github.com/dop251/goja` (built-in testing)

## 5. Coverage

We aim for a minimum of **80% code coverage** for core packages.
Run coverage with:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## 6. Continuous Integration

Tests are automatically executed on every push and pull request via GitHub Actions.
