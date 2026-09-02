# internal/net — Agent Constraints & Architecture

## Core Responsibilities

The `internal/net` package provides HTTP/HTTPS networking services for the Goosie browser engine, managing connection lifecycles, HTTP caching, cookie persistence, Content Security Policy (CSP) enforcement, MIME-type sniffing, download handling, and security guards.

## Single `http.Client` Reuse

- Never instantiate per-request `http.Client` instances.
- All requests flow through `s.client.Do(req)` on the shared service client.
- Request tracing uses `httptrace.WithClientTrace`.
- Redirect counters and cycle detection are tracked in request context via `withRedirectCount` and checked by `s.client.CheckRedirect`.

## Unified Fetch Path

- Fetch entry points are consolidated: `FetchWithContext` directly delegates to `FetchWithMeta(ctx, rawURL, onProgress)`.
- All response metadata, headers, caching decisions, and timing metrics are captured consistently through this unified pipeline.

## Cookie Jar Domain Indexing (`domainIndex`)

- `CookieJar` maintains `domainIndex map[string][]int` which is rebuilt on cookie additions or removals (`rebuildDomainIndex`).
- URL lookups (`CookieRecords(u *url.URL)`) use the domain index for O(1) candidate record retrieval instead of scanning all stored cookies across domains.
- Thread-safe access is guarded by `CookieJar.mu`.

## Byte Slice Caching (`HTTPCache`)

- `HTTPCache` operates directly on raw `[]byte` payloads (`Get` returns `([]byte, CacheEntry, bool)` and `Put` takes `[]byte`).
- Avoid converting binary image or asset payloads to/from Go strings, eliminating heap churn.
- Caching obeys `Cache-Control`, `ETag`, `Last-Modified`, and `Expires` HTTP headers with standard revalidation.

## Decompression Bomb & Size Guards

- Response streams are wrapped in `limitedContextReader` in `service.go`.
- Hard limits are strictly enforced:
  - `DefaultMaxBodySize = 100 * 1024 * 1024` (100MB max response payload)
  - `MaxDecompressionRatio = 100` (100:1 maximum decompression expansion ratio)
- Exceeding these limits terminates the download and returns an error to prevent zip/gzip bomb attacks.

## Content Security Policy (CSP) Enforcement

- `csp.go` parses `Content-Security-Policy` and `Content-Security-Policy-Report-Only` response headers.
- Evaluates directives including `default-src`, `script-src`, `style-src`, `img-src`, `connect-src`, `font-src`, `frame-src`, and `object-src`.
- Supports standard source expressions: `'self'`, `'unsafe-inline'`, `'unsafe-eval'`, `'none'`, `https:`, `http:`, and `data:`.

## Testing & Verification

All network subsystem tests reside in `test/internal/net/...`.

Run the full network test suite with the race detector:
```bash
go test -race -short ./test/internal/net/...
```
