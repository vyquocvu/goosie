# Stream C: Network Subsystem Optimization

## Goal
Reduce network latency and memory allocations in the HTTP fetch pipeline without changing behavior.

## Scope
- `internal/net/service.go` — fetch pipeline, doRequest
- `internal/net/cache.go` — HTTP cache
- `internal/net/cookies.go` — cookie jar

## Constraints
- Zero behavior change (same HTTP semantics, same cache eligibility)
- All existing tests pass
- No new dependencies

## Optimizations

### C1: Eliminate per-request http.Client allocation
**Problem:** `doRequest` creates a new `http.Client` on every call just to set `CheckRedirect`. This allocates a client struct per request.

**Solution:** Store redirect policy as a field on the Service's client at construction time. Use a closure over a `*int` counter that's reset per-request via context value, OR use the service's client directly with a redirect counter in the request context.

**Implementation:** Add `redirectCount` to request context. Use a single `CheckRedirect` on `s.client` that reads/writes the counter from context.

### C2: Deduplicate FetchWithContext and FetchWithMeta
**Problem:** These two methods are ~90% identical code. Maintenance burden and bug-fix surface.

**Solution:** Make `FetchWithContext` call `FetchWithMeta` and discard the metadata. Single code path.

### C3: Cookie jar domain index
**Problem:** `CookieJar.Cookies()` and `CookieRecord()` do linear scans over all cookies. For sites with hundreds of cookies, this is O(n) per request.

**Solution:** Add a `map[string][]int` index keyed by domain. On `SetCookies`, update the index. On `Cookies()`, look up by domain first, then filter by path/expiry.

**Trade-off:** Adds memory for the index, but cookie counts are typically small (<100 per domain), so the index is small.

### C4: Cache body as []byte instead of string
**Problem:** Cache pipeline converts `[]byte` → `string` → `[]byte` → `string` multiple times. For a 10MB page, that's 20-40MB of unnecessary allocations.

**Solution:** Change `cache.Get` to return `[]byte`, change `cache.Put` to accept `[]byte`. Convert to `string` only at the fetch boundary where the API requires it.

**Trade-off:** Requires changing the cache API, but it's internal to `internal/net`.

## Out of Scope
- HTTP/2 push
- Conditional GET (ETag/Last-Modified) — larger change, separate stream
- Transport sharing between Session and Service — architectural, risky

## Testing
- Existing `internal/net` tests
- Benchmark: `BenchmarkFetchWithMeta` before/after
- Pixel hash unchanged (network layer doesn't affect rendering)
