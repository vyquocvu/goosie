# Stream C: Network Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce network latency and memory allocations in the HTTP fetch pipeline without changing behavior.

**Architecture:** Eliminate per-request allocations (http.Client, string conversions), deduplicate fetch methods, and add cookie jar indexing for faster lookups.

**Tech Stack:** Go net/http, internal/net package

---

## Task C1: Eliminate per-request http.Client allocation

**Files:**
- Modify: `internal/net/service.go:381-470`

- [x] **Step 1: Add redirect counter to context**

Add a context key type and helper functions to store redirect count in request context:

```go
type contextKey int

const redirectCountKey contextKey = iota

func withRedirectCount(ctx context.Context) context.Context {
    return context.WithValue(ctx, redirectCountKey, new(int))
}

func getRedirectCount(ctx context.Context) int {
    if ptr, ok := ctx.Value(redirectCountKey).(*int); ok {
        return *ptr
    }
    return 0
}

func setRedirectCount(ctx context.Context, count int) {
    if ptr, ok := ctx.Value(redirectCountKey).(*int); ok {
        *ptr = count
    }
}
```

- [x] **Step 2: Configure CheckRedirect on service client**

In `NewService`, after creating the client, set a CheckRedirect that reads/writes the counter from context:

```go
client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
    count := getRedirectCount(req.Context())
    if len(via) >= maxRedirects {
        setRedirectCount(req.Context(), maxRedirects)
        return http.ErrUseLastResponse
    }
    setRedirectCount(req.Context(), len(via))
    return nil
}
```

- [x] **Step 3: Simplify doRequest**

Remove the per-request client allocation. Use `s.client` directly. Initialize the redirect counter in the request context before calling `s.client.Do`:

```go
func (s *Service) doRequest(req *http.Request) (*http.Response, int, error) {
    req = req.WithContext(withRedirectCount(req.Context()))
    
    // ... existing httptrace code ...
    
    resp, err := s.client.Do(req)
    
    redirectCount := getRedirectCount(req.Context())
    
    // ... existing timing code ...
    
    return resp, redirectCount, err
}
```

- [x] **Step 4: Run tests**

```bash
go test ./test/internal/net/... -v
```

Expected: All tests pass

- [x] **Step 5: Commit**

```bash
git add internal/net/service.go
git commit -m "refactor(net): eliminate per-request http.Client allocation in doRequest"
```

---

## Task C2: Deduplicate FetchWithContext and FetchWithMeta

**Files:**
- Modify: `internal/net/service.go:259-355`

- [x] **Step 1: Make FetchWithContext call FetchWithMeta**

Replace the entire `FetchWithContext` implementation with a thin wrapper:

```go
func (s *Service) FetchWithContext(ctx context.Context, rawURL string, onProgress ProgressCallback) (string, error) {
    body, _, err := s.FetchWithMeta(ctx, rawURL, onProgress)
    return body, err
}
```

- [x] **Step 2: Run tests**

```bash
go test ./test/internal/net/... -v
```

Expected: All tests pass

- [x] **Step 3: Commit**

```bash
git add internal/net/service.go
git commit -m "refactor(net): deduplicate FetchWithContext by delegating to FetchWithMeta"
```

---

## Task C3: Cookie jar domain index

**Files:**
- Modify: `internal/net/cookies.go:26-142`

- [x] **Step 1: Add domain index to CookieJar**

Add a `domainIndex` field to `CookieJar`:

```go
type CookieJar struct {
    mu           sync.Mutex
    records      []CookieRecord
    domainIndex  map[string][]int // domain -> indices into records
    now          func() time.Time
}

func NewCookieJar() *CookieJar {
    return &CookieJar{
        domainIndex: make(map[string][]int),
        now:         time.Now,
    }
}
```

- [x] **Step 2: Update SetCookies to maintain index**

After adding/replacing a cookie, rebuild the domain index for that domain:

```go
func (j *CookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
    // ... existing validation ...
    
    // Rebuild index after modifications
    defer j.rebuildDomainIndex()
    
    // ... existing cookie add/replace logic ...
}

func (j *CookieJar) rebuildDomainIndex() {
    j.domainIndex = make(map[string][]int)
    for i, record := range j.records {
        j.domainIndex[record.Domain] = append(j.domainIndex[record.Domain], i)
    }
}
```

- [x] **Step 3: Update CookieRecord to use index**

In `CookieRecord(u *url.URL)`, look up by domain first:

```go
func (j *CookieJar) CookieRecord(u *url.URL) []CookieRecord {
    // ... existing validation ...
    
    hostname := canonicalCookieHost(u.Hostname())
    
    // Collect candidate indices from domain index
    var candidateIndices []int
    for domain, indices := range j.domainIndex {
        if domainMatches(hostname, domain, false) {
            candidateIndices = append(candidateIndices, indices...)
        }
    }
    
    // Filter candidates by path/expiry/secure
    var records []CookieRecord
    active := j.records[:0]
    now := j.currentTime()
    requestPath := cookieRequestPath(u)
    
    for _, idx := range candidateIndices {
        record := j.records[idx]
        if !record.Expires.IsZero() && !record.Expires.After(now) {
            continue
        }
        active = append(active, record)
        if record.Secure && u.Scheme != "https" {
            continue
        }
        if !pathMatches(requestPath, record.Path) {
            continue
        }
        records = append(records, record)
    }
    
    j.records = active
    return records
}
```

- [x] **Step 4: Run tests**

```bash
go test ./test/internal/net/... -v
```

Expected: All tests pass

- [x] **Step 5: Commit**

```bash
git add internal/net/cookies.go
git commit -m "feat(net): add domain index to CookieJar for O(1) domain lookup"
```

---

## Task C4: Cache body as []byte instead of string

**Files:**
- Modify: `internal/net/cache.go:144-255`
- Modify: `internal/net/service.go:165,232`

- [x] **Step 1: Change cache.Get to return []byte**

Update `HTTPCache.Get` signature and implementation:

```go
func (c *HTTPCache) Get(rawURL string) ([]byte, CacheEntry, bool) {
    // ... existing validation ...
    
    body, err := os.ReadFile(bodyPath)
    if err != nil {
        c.recordMiss()
        return nil, CacheEntry{}, false
    }
    
    // ... existing metrics ...
    
    return body, entry, true
}
```

- [x] **Step 2: Change cache.Put to accept []byte**

Update `HTTPCache.Put` signature:

```go
func (c *HTTPCache) Put(rawURL string, resp *http.Response, body []byte) {
    // ... existing validation ...
    
    bodyBytes := int64(len(body))
    
    // ... existing cache logic ...
    
    if err := os.WriteFile(bodyPath, body, 0o644); err != nil {
        return
    }
    
    // ... rest of implementation ...
}
```

- [x] **Step 3: Update service.go to convert at boundaries**

In `FetchWithMeta`, convert cache result to string:

```go
if bodyBytes, entry, ok := s.cache.Get(rawURL); ok {
    body := string(bodyBytes)
    // ... rest of cache hit handling ...
}
```

And when storing:

```go
if !hasCookies && responseMatchesOriginalURL(rawURL, resp) {
    s.cache.Put(rawURL, resp, []byte(body))
}
```

- [x] **Step 4: Update CachedBody**

```go
func (s *Service) CachedBody(rawURL string) (string, bool) {
    if s == nil || s.cache == nil {
        return "", false
    }
    bodyBytes, _, ok := s.cache.Get(rawURL)
    if !ok {
        return "", false
    }
    return string(bodyBytes), true
}
```

- [x] **Step 5: Run tests**

```bash
go test ./test/internal/net/... -v
```

Expected: All tests pass

- [x] **Step 6: Commit**

```bash
git add internal/net/cache.go internal/net/service.go
git commit -m "refactor(net): cache body as []byte instead of string (C4)"
```

---

## Task C5: Final verification

- [x] **Step 1: Run all net tests**

```bash
go test ./test/internal/net/... -v
```

Expected: All tests pass

- [x] **Step 2: Run full test suite**

```bash
go test ./...
```

Expected: All tests pass

- [x] **Step 3: Verify pixel hashes unchanged**

```bash
go test -v ./test/perf -run TestPixelHashManifest
```

Expected: Pass (network layer doesn't affect rendering)

- [x] **Step 4: Benchmark comparison (optional)**

If benchmarks exist, run before/after comparison:

```bash
go test -bench=BenchmarkFetch ./test/internal/net/... -benchtime=1s
```

Expected: No regression, potential improvement from reduced allocations

