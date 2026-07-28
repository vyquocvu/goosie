# Goosie Browser Test Cases

This document outlines the test cases for the Goosie browser, covering various testing layers.

## 1. Unit Tests

### 1.1 CSS Parser (`internal/css`)
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `TestParser` | Parse basic CSS rules | Correctly parses selectors and declarations |
| `TestParserCombinedSelector` | Parse combined selectors (tag.class) | Correctly identifies tag and class |
| `TestParserComments` | Parse CSS with comments | Ignores comments |
| `TestParserCombinators` | Parse descendant, child, adjacent, sibling combinators | Correctly builds selector sequence |
| `TestParserPseudoClasses` | Parse pseudo-classes (:hover, :nth-child) | Correctly identifies pseudo-classes |
| `TestParserMalformedCSS` | Parse invalid CSS | Should not panic, may skip invalid rules |

### 1.2 DOM Parser (`internal/dom`)
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `TestParseBodyText` | Extract text from body | Returns plain text content |
| `TestGetElementByID` | Find element by ID | Returns element with matching ID |
| `TestQuerySelector` | Find element by CSS selector | Returns first matching element |

## 2. Integration Tests (`internal/test_suite/integration`)

### 2.1 Renderer Integration
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `TestRendererIntegration` | Render HTML with CSS and verify DOM | HTML is parsed, CSS applied, elements are accessible via DOM parser, visual output is generated (non-nil) |

### 2.2 Layout Engine
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `TestFlexboxLayout` | Render Flexbox container with items | Items are positioned correctly (row/justify-content) |
| `TestGridLayout` | Render Grid container with items | Items are positioned in correct cells |

### 2.3 Network
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `TestFetcher` | Fetch URL with mock client | Content matches mock response |
| `TestFetcherError` | Fetch 404 URL | Returns error |

## 3. Web APIs (`internal/test_suite/webapi`)

### 3.1 Storage
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `TestLocalStorage` | Set, Get, Remove, Clear localStorage | Data persists and is retrievable/removable |
| `TestSessionStorage` | Set, Get, Remove sessionStorage | Data persists, isolated from localStorage |

### 3.2 Fetch API (JS)
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `fetch` (Pending) | Execute `fetch()` in JS | Returns Promise, resolves to Response |

## 4. End-to-End Tests (`internal/test_suite/e2e`)

### 4.1 User Workflow
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `TestBrowserNavigationWorkflow` | Simulate user navigation (Home -> Page1 -> Back -> Forward) | Browser state updates correctly, history is maintained, current URL matches expected |
| `TestBookmarkWorkflow` | Add and remove bookmarks | URL is added/removed from bookmark list |

## 5. Performance Tests (`internal/test_suite/performance`)

### 5.1 Page Load
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `BenchmarkRenderLargeHTML` | Render 1000 list items | Execution time < 100ms (target) |
| `BenchmarkRenderVeryLargeHTML` | Render 5000 list items | Execution time < 500ms (target) |

## 6. Security Tests (`internal/test_suite/security`)

### 6.1 XSS Protection
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `TestRendererDoSProtection` | Render malicious inputs (nested tags, long attributes, script tags) | Renderer does not panic or crash |

### 6.2 CORS & CSP (Pending)
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `TestCORSBlock` | Fetch cross-origin resource without headers | Request blocked |
| `TestCSPBlock` | Load script violated CSP | Script execution blocked |

## 7. Accessibility Tests (`internal/test_suite/a11y`)

### 7.1 ARIA Support
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `TestARIAAttributes` | Parse ARIA attributes (role, aria-label, aria-expanded) | Attributes are correctly parsed and accessible on DOM elements |

## 8. Multimedia (Pending)

### 8.1 Video/Audio
| Test Case | Description | Expected Result |
|-----------|-------------|-----------------|
| `TestVideoElement` | Parse `<video>` tag | Element created, source loaded |
| `TestAudioElement` | Parse `<audio>` tag | Element created, source loaded |

## 9. Automated Testing & Reporting

Tests are executed via GitHub Actions on every push.
Run tests locally with:
```bash
go test -v ./internal/test_suite/...
```
Coverage report:
```bash
go test -coverprofile=coverage.out ./internal/...
go tool cover -html=coverage.out
```
