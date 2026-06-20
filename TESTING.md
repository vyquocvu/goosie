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
*   **Location**: `test/e2e` (to be created).
*   **Focus**: User experience, UI interactions (clicks, input), critical paths.
*   **Tools**: Fyne's test driver, custom helpers in `internal/testutil`.

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
