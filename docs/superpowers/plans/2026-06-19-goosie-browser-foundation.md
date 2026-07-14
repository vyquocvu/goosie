# Goosie Browser Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the practical v1.0 browser foundation for Goosie: persistent profile data, shared network state, correct forms, durable storage, devtools panels, safer test tiers, and release packaging.

**Architecture:** Add `internal/profile` as the durable state boundary, extend `internal/net` into a shared browser network service, and wire these into the existing `cmd/browser`, `internal/ui`, `internal/js`, and `internal/form` packages without replacing the custom Go/Fyne renderer. Implement the foundation in commit-sized vertical slices so every slice keeps the app buildable and testable.

**Tech Stack:** Go 1.24.9, Fyne v2.7.0 (window/presentation shell only — engine never imports Fyne), Goja (JS engine), `net/http`, `net/http/cookiejar`, `encoding/json`, Go standard `testing`, `testify`, GitHub Actions.

**Render model:** Pure Go CPU raster backend (`internal/renderer/frame/raster`). No platform WebViews (WKWebView, WebView2, CEF). CoreGraphics via CGo optional (macOS only).

---

## File Structure

Create:

- `internal/profile/profile.go`: profile root, mode, atomic JSON load/save helpers.
- `internal/profile/profile_test.go`: profile path, atomic write, corrupt JSON, private no-write tests.
- `internal/profile/bookmarks.go`: durable bookmark records and store.
- `internal/profile/bookmarks_test.go`: bookmark add/remove/dedup/load/save tests.
- `internal/profile/history.go`: visit and session tab records.
- `internal/profile/history_test.go`: visit ordering and session restore tests.
- `internal/profile/settings.go`: persisted settings model.
- `internal/profile/settings_test.go`: default/load/save/private settings tests.
- `internal/profile/storage.go`: origin-scoped localStorage store.
- `internal/profile/storage_test.go`: storage origin isolation and persistence tests.
- `internal/net/service.go`: shared browser network service and request options.
- `internal/net/service_test.go`: fake transport tests for fetch logging and cache policy.
- `internal/net/cache.go`: HTTP cache metadata/body storage.
- `internal/net/cache_test.go`: cache hit/miss/expiry/private no-write tests.
- `internal/net/cookies.go`: persistent cookie jar adapter.
- `internal/net/cookies_test.go`: cookie persistence and private no-write tests.
- `internal/net/downloads.go`: download manager records, progress, cancellation.
- `internal/net/downloads_test.go`: fake transport download tests.
- `internal/net/security.go`: TLS/certificate summary types.
- `internal/net/security_test.go`: TLS summary tests from synthetic response state.
- `internal/net/log.go`: request log entries.
- `internal/ui/address.go`: URL/search normalization.
- `internal/ui/address_test.go`: address/search normalization tests.
- `internal/ui/shortcuts.go`: shortcut registration and command dispatch.
- `internal/ui/shortcuts_test.go`: shortcut dispatch tests.
- `internal/ui/network_panel.go`: devtools network list.
- `internal/ui/storage_panel.go`: devtools storage list.
- `internal/ui/security_panel.go`: devtools security details.
- `internal/ui/downloads_panel.go`: downloads panel.
- `.github/workflows/release.yml`: tag-triggered release workflow.

Modify:

- `cmd/browser/main.go`: construct profile/network services and pass them through UI/runtime.
- `internal/net/fetcher.go`: keep compatibility while delegating to `Service`.
- `internal/form/state.go`: make duplicate-submit prevention persistent until reset.
- `internal/form/submission.go`: add submit client interface, base URL resolution, typed error categories.
- `internal/form/form_submission_test.go`: replace real-network assumptions with fake submit clients.
- `internal/form/form_error_scenarios_test.go`: assert raw preservation and escaped display helper behavior.
- `internal/js/runtime.go`: add storage adapter and shared fetcher integration.
- `internal/js/runtime_test.go`: persistent localStorage and fetch integration tests.
- `internal/ui/browser.go`: dependency injection, profile sync, panels, private mode, page source state.
- `internal/ui/settings.go`: map UI settings to `internal/profile.Settings`.
- `internal/ui/console.go`: add JavaScript command entry.
- `internal/ui/inspect_panel.go`: expose source integration hooks where possible.
- `internal/ui/state.go`: keep in-memory navigation state but emit sync events to profile.
- `TESTING.md`: document short, normal, network, and e2e test tiers.
- `ROADMAP_V2.md`: mark foundation items and move standards features to the next milestone.
- `README.md`: document profile, private mode, downloads, security/devtools, and release usage.
- `TASKS.md`: close or update items addressed by this milestone.

---

### Task 1: Profile Root and Atomic JSON Store

**Files:**
- Create: `internal/profile/profile.go`
- Create: `internal/profile/profile_test.go`

- [ ] **Step 1: Write failing tests for normal and private profiles**

Add this file:

```go
package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type sampleDocument struct {
	Name string `json:"name"`
}

func TestOpenCreatesNormalProfileDirectory(t *testing.T) {
	dir := t.TempDir()

	p, err := Open(Options{Root: dir})
	require.NoError(t, err)
	require.False(t, p.Private())
	require.Equal(t, dir, p.Root())
	require.DirExists(t, dir)
}

func TestPrivateProfileDoesNotWriteFiles(t *testing.T) {
	dir := t.TempDir()

	p, err := Open(Options{Root: dir, Private: true})
	require.NoError(t, err)
	require.True(t, p.Private())

	err = p.SaveJSON("state.json", sampleDocument{Name: "secret"})
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(dir, "state.json"))
}

func TestSaveAndLoadJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p, err := Open(Options{Root: dir})
	require.NoError(t, err)

	err = p.SaveJSON("state.json", sampleDocument{Name: "goosie"})
	require.NoError(t, err)

	var loaded sampleDocument
	err = p.LoadJSON("state.json", &loaded)
	require.NoError(t, err)
	require.Equal(t, "goosie", loaded.Name)
}

func TestLoadMissingJSONLeavesTargetUnchanged(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	loaded := sampleDocument{Name: "default"}
	err = p.LoadJSON("missing.json", &loaded)
	require.NoError(t, err)
	require.Equal(t, "default", loaded.Name)
}

func TestCorruptJSONIsBackedUp(t *testing.T) {
	dir := t.TempDir()
	p, err := Open(Options{Root: dir})
	require.NoError(t, err)

	path := filepath.Join(dir, "state.json")
	err = os.WriteFile(path, []byte("{not-json"), 0o600)
	require.NoError(t, err)

	var loaded sampleDocument
	err = p.LoadJSON("state.json", &loaded)
	require.Error(t, err)
	require.FileExists(t, path+".corrupt")
	require.NoFileExists(t, path)
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run:

```bash
go test ./internal/profile -run 'Test(OpenCreatesNormalProfileDirectory|PrivateProfileDoesNotWriteFiles|SaveAndLoadJSONRoundTrip|LoadMissingJSONLeavesTargetUnchanged|CorruptJSONIsBackedUp)' -count=1
```

Expected: FAIL because package `internal/profile` or functions `Open`, `Options`, `SaveJSON`, and `LoadJSON` do not exist.

- [ ] **Step 3: Implement profile root and atomic JSON helpers**

Create `internal/profile/profile.go`:

```go
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Options struct {
	Root    string
	Private bool
}

type Profile struct {
	root    string
	private bool
}

func Open(options Options) (*Profile, error) {
	root := options.Root
	if root == "" {
		root = defaultRoot()
	}
	if !options.Private {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return nil, fmt.Errorf("create profile directory: %w", err)
		}
	}
	return &Profile{root: root, private: options.Private}, nil
}

func defaultRoot() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "goosie")
	}
	return ".goosie"
}

func (p *Profile) Root() string {
	return p.root
}

func (p *Profile) Private() bool {
	return p.private
}

func (p *Profile) path(name string) string {
	return filepath.Join(p.root, filepath.Clean(name))
}

func (p *Profile) LoadJSON(name string, target any) error {
	path := p.path(name)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		backup := path + ".corrupt"
		_ = os.Rename(path, backup)
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}

func (p *Profile) SaveJSON(name string, value any) error {
	if p.private {
		return nil
	}
	if err := os.MkdirAll(p.root, 0o700); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", name, err)
	}
	data = append(data, '\n')

	path := p.path(name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace %s: %w", name, err)
	}
	return nil
}
```

- [ ] **Step 4: Verify profile tests pass**

Run:

```bash
go test ./internal/profile -run 'Test(OpenCreatesNormalProfileDirectory|PrivateProfileDoesNotWriteFiles|SaveAndLoadJSONRoundTrip|LoadMissingJSONLeavesTargetUnchanged|CorruptJSONIsBackedUp)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/profile.go internal/profile/profile_test.go
git commit -m "feat: add browser profile root"
```

---

### Task 2: Durable Bookmarks, History, Settings, and Storage Stores

**Files:**
- Create: `internal/profile/bookmarks.go`
- Create: `internal/profile/bookmarks_test.go`
- Create: `internal/profile/history.go`
- Create: `internal/profile/history_test.go`
- Create: `internal/profile/settings.go`
- Create: `internal/profile/settings_test.go`
- Create: `internal/profile/storage.go`
- Create: `internal/profile/storage_test.go`

- [ ] **Step 1: Write failing tests for profile stores**

Create `internal/profile/bookmarks_test.go`:

```go
package profile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBookmarkStoreAddRemoveAndPersist(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	store, err := NewBookmarkStore(p)
	require.NoError(t, err)
	require.NoError(t, store.Add("https://example.com", "Example"))
	require.NoError(t, store.Add("https://example.com", "Example Updated"))

	bookmarks := store.List()
	require.Len(t, bookmarks, 1)
	require.Equal(t, "Example Updated", bookmarks[0].Title)

	reloaded, err := NewBookmarkStore(p)
	require.NoError(t, err)
	require.True(t, reloaded.Contains("https://example.com"))

	require.NoError(t, reloaded.Remove("https://example.com"))
	require.False(t, reloaded.Contains("https://example.com"))
}
```

Create `internal/profile/history_test.go`:

```go
package profile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHistoryStoreVisitsAndSessionTabs(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	store, err := NewHistoryStore(p)
	require.NoError(t, err)
	require.NoError(t, store.AddVisit("https://one.test", "One"))
	require.NoError(t, store.AddVisit("https://two.test", "Two"))
	require.NoError(t, store.SaveSession([]SessionTab{
		{URL: "https://two.test", Title: "Two", Active: true},
	}))

	reloaded, err := NewHistoryStore(p)
	require.NoError(t, err)
	require.Equal(t, []string{"https://one.test", "https://two.test"}, reloaded.VisitURLs())
	require.Equal(t, []SessionTab{{URL: "https://two.test", Title: "Two", Active: true}}, reloaded.SessionTabs())
}
```

Create `internal/profile/settings_test.go`:

```go
package profile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettingsStoreDefaultsAndPersist(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	store, err := NewSettingsStore(p)
	require.NoError(t, err)
	settings := store.Get()
	require.Equal(t, "https://example.com", settings.Homepage)
	require.True(t, settings.EnableJavaScript)
	require.True(t, settings.EnableImages)

	settings.Homepage = "https://go.dev"
	settings.DefaultSearchEngine = "https://duckduckgo.com/?q="
	require.NoError(t, store.Set(settings))

	reloaded, err := NewSettingsStore(p)
	require.NoError(t, err)
	require.Equal(t, "https://go.dev", reloaded.Get().Homepage)
	require.Equal(t, "https://duckduckgo.com/?q=", reloaded.Get().DefaultSearchEngine)
}
```

Create `internal/profile/storage_test.go`:

```go
package profile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStorageStoreIsOriginScopedAndPersistent(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	store, err := NewStorageStore(p)
	require.NoError(t, err)
	require.NoError(t, store.Set("https://one.test", "theme", "dark"))
	require.NoError(t, store.Set("https://two.test", "theme", "light"))

	reloaded, err := NewStorageStore(p)
	require.NoError(t, err)
	one, ok := reloaded.Get("https://one.test", "theme")
	require.True(t, ok)
	require.Equal(t, "dark", one)
	two, ok := reloaded.Get("https://two.test", "theme")
	require.True(t, ok)
	require.Equal(t, "light", two)
	require.Equal(t, []string{"theme"}, reloaded.Keys("https://one.test"))
}

func TestStorageStorePrivateDoesNotPersist(t *testing.T) {
	root := t.TempDir()
	p, err := Open(Options{Root: root, Private: true})
	require.NoError(t, err)

	store, err := NewStorageStore(p)
	require.NoError(t, err)
	require.NoError(t, store.Set("https://one.test", "token", "secret"))

	normal, err := Open(Options{Root: root})
	require.NoError(t, err)
	reloaded, err := NewStorageStore(normal)
	require.NoError(t, err)
	_, ok := reloaded.Get("https://one.test", "token")
	require.False(t, ok)
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run:

```bash
go test ./internal/profile -count=1
```

Expected: FAIL because store constructors and types do not exist.

- [ ] **Step 3: Implement bookmark store**

Create `internal/profile/bookmarks.go`:

```go
package profile

import (
	"sort"
	"sync"
	"time"
)

type Bookmark struct {
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BookmarkStore struct {
	mu        sync.RWMutex
	profile   *Profile
	bookmarks []Bookmark
}

func NewBookmarkStore(p *Profile) (*BookmarkStore, error) {
	store := &BookmarkStore{profile: p, bookmarks: []Bookmark{}}
	if err := p.LoadJSON("bookmarks.json", &store.bookmarks); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *BookmarkStore) Add(rawURL, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for i := range s.bookmarks {
		if s.bookmarks[i].URL == rawURL {
			s.bookmarks[i].Title = title
			s.bookmarks[i].UpdatedAt = now
			return s.persistLocked()
		}
	}
	s.bookmarks = append(s.bookmarks, Bookmark{URL: rawURL, Title: title, CreatedAt: now, UpdatedAt: now})
	sort.SliceStable(s.bookmarks, func(i, j int) bool { return s.bookmarks[i].CreatedAt.Before(s.bookmarks[j].CreatedAt) })
	return s.persistLocked()
}

func (s *BookmarkStore) Remove(rawURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.bookmarks {
		if s.bookmarks[i].URL == rawURL {
			s.bookmarks = append(s.bookmarks[:i], s.bookmarks[i+1:]...)
			return s.persistLocked()
		}
	}
	return nil
}

func (s *BookmarkStore) Contains(rawURL string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, bookmark := range s.bookmarks {
		if bookmark.URL == rawURL {
			return true
		}
	}
	return false
}

func (s *BookmarkStore) List() []Bookmark {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Bookmark, len(s.bookmarks))
	copy(out, s.bookmarks)
	return out
}

func (s *BookmarkStore) persistLocked() error {
	return s.profile.SaveJSON("bookmarks.json", s.bookmarks)
}
```

- [ ] **Step 4: Implement history store**

Create `internal/profile/history.go`:

```go
package profile

import (
	"sync"
	"time"
)

type Visit struct {
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	VisitedAt time.Time `json:"visited_at"`
}

type SessionTab struct {
	URL    string `json:"url"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
}

type historyDocument struct {
	Visits  []Visit      `json:"visits"`
	Session []SessionTab `json:"session"`
}

type HistoryStore struct {
	mu      sync.RWMutex
	profile *Profile
	doc     historyDocument
}

func NewHistoryStore(p *Profile) (*HistoryStore, error) {
	store := &HistoryStore{profile: p, doc: historyDocument{Visits: []Visit{}, Session: []SessionTab{}}}
	if err := p.LoadJSON("history.json", &store.doc); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *HistoryStore) AddVisit(rawURL, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.doc.Visits = append(s.doc.Visits, Visit{URL: rawURL, Title: title, VisitedAt: time.Now().UTC()})
	return s.profile.SaveJSON("history.json", s.doc)
}

func (s *HistoryStore) VisitURLs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	urls := make([]string, 0, len(s.doc.Visits))
	for _, visit := range s.doc.Visits {
		urls = append(urls, visit.URL)
	}
	return urls
}

func (s *HistoryStore) SaveSession(tabs []SessionTab) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.doc.Session = append([]SessionTab(nil), tabs...)
	return s.profile.SaveJSON("history.json", s.doc)
}

func (s *HistoryStore) SessionTabs() []SessionTab {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]SessionTab(nil), s.doc.Session...)
}
```

- [ ] **Step 5: Implement settings store**

Create `internal/profile/settings.go`:

```go
package profile

import "sync"

type Settings struct {
	Homepage            string `json:"homepage"`
	DefaultSearchEngine string `json:"default_search_engine"`
	EnableJavaScript    bool   `json:"enable_javascript"`
	EnableImages        bool   `json:"enable_images"`
}

func DefaultSettings() Settings {
	return Settings{
		Homepage:            "https://example.com",
		DefaultSearchEngine: "https://www.google.com/search?q=",
		EnableJavaScript:    true,
		EnableImages:        true,
	}
}

type SettingsStore struct {
	mu       sync.RWMutex
	profile  *Profile
	settings Settings
}

func NewSettingsStore(p *Profile) (*SettingsStore, error) {
	store := &SettingsStore{profile: p, settings: DefaultSettings()}
	if err := p.LoadJSON("settings.json", &store.settings); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SettingsStore) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *SettingsStore) Set(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
	return s.profile.SaveJSON("settings.json", settings)
}
```

- [ ] **Step 6: Implement origin-scoped storage store**

Create `internal/profile/storage.go`:

```go
package profile

import (
	"sort"
	"sync"
)

type StorageStore struct {
	mu      sync.RWMutex
	profile *Profile
	data    map[string]map[string]string
}

func NewStorageStore(p *Profile) (*StorageStore, error) {
	store := &StorageStore{profile: p, data: map[string]map[string]string{}}
	if err := p.LoadJSON("storage.json", &store.data); err != nil {
		return nil, err
	}
	if store.data == nil {
		store.data = map[string]map[string]string{}
	}
	return store, nil
}

func (s *StorageStore) Get(origin, key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values, ok := s.data[origin]
	if !ok {
		return "", false
	}
	value, ok := values[key]
	return value, ok
}

func (s *StorageStore) Set(origin, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data[origin] == nil {
		s.data[origin] = map[string]string{}
	}
	s.data[origin][key] = value
	return s.profile.SaveJSON("storage.json", s.data)
}

func (s *StorageStore) Remove(origin, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data[origin] != nil {
		delete(s.data[origin], key)
	}
	return s.profile.SaveJSON("storage.json", s.data)
}

func (s *StorageStore) Clear(origin string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, origin)
	return s.profile.SaveJSON("storage.json", s.data)
}

func (s *StorageStore) Keys(origin string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := s.data[origin]
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (s *StorageStore) Snapshot() map[string]map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]map[string]string, len(s.data))
	for origin, values := range s.data {
		out[origin] = make(map[string]string, len(values))
		for key, value := range values {
			out[origin][key] = value
		}
	}
	return out
}
```

- [ ] **Step 7: Verify profile store tests pass**

Run:

```bash
go test ./internal/profile -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/profile
git commit -m "feat: persist browser profile stores"
```

---

### Task 3: Shared Network Service, Cache, Cookies, Downloads, Security, and Request Log

**Files:**
- Create: `internal/net/service.go`
- Create: `internal/net/service_test.go`
- Create: `internal/net/cache.go`
- Create: `internal/net/cache_test.go`
- Create: `internal/net/cookies.go`
- Create: `internal/net/cookies_test.go`
- Create: `internal/net/downloads.go`
- Create: `internal/net/downloads_test.go`
- Create: `internal/net/security.go`
- Create: `internal/net/security_test.go`
- Create: `internal/net/log.go`
- Modify: `internal/net/fetcher.go`

- [ ] **Step 1: Write failing network service tests**

Create `internal/net/service_test.go`:

```go
package net

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestServiceFetchRecordsRequestLog(t *testing.T) {
	service := NewService(ServiceOptions{
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"text/html"}},
				Body:       io.NopCloser(strings.NewReader("<h1>ok</h1>")),
				Request:    req,
			}, nil
		})},
	})

	body, err := service.Fetch("https://example.test")
	require.NoError(t, err)
	require.Equal(t, "<h1>ok</h1>", body)

	entries := service.Log().Entries()
	require.Len(t, entries, 1)
	require.Equal(t, "GET", entries[0].Method)
	require.Equal(t, "https://example.test", entries[0].URL)
	require.Equal(t, 200, entries[0].Status)
	require.False(t, entries[0].CacheHit)
}

func TestServiceFetchUsesCacheOnSecondRequest(t *testing.T) {
	calls := 0
	service := NewService(ServiceOptions{
		Cache: NewHTTPCache(t.TempDir(), false),
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Header:     http.Header{"Cache-Control": []string{"max-age=60"}},
				Body:       io.NopCloser(strings.NewReader("cached body")),
				Request:    req,
			}, nil
		})},
	})

	first, err := service.Fetch("https://cache.test/page")
	require.NoError(t, err)
	second, err := service.Fetch("https://cache.test/page")
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Equal(t, 1, calls)
	require.True(t, service.Log().Entries()[1].CacheHit)
}
```

Create `internal/net/downloads_test.go`:

```go
package net

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDownloadManagerWritesFileAndRecordsStatus(t *testing.T) {
	target := filepath.Join(t.TempDir(), "file.txt")
	manager := NewDownloadManager(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    200,
			Status:        "200 OK",
			ContentLength: 11,
			Body:          io.NopCloser(strings.NewReader("hello world")),
			Request:       req,
		}, nil
	})})

	record, err := manager.Download("https://files.test/file.txt", target)
	require.NoError(t, err)
	require.Equal(t, DownloadComplete, record.Status)
	require.Equal(t, int64(11), record.BytesWritten)

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "hello world", string(data))
}
```

Create `internal/net/security_test.go`:

```go
package net

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSecuritySummaryFromTLSResponse(t *testing.T) {
	cert := &x509.Certificate{
		Subject:   pkixName("example.test"),
		Issuer:    pkixName("Goosie Test CA"),
		NotAfter:  time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		NotBefore: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	resp := &http.Response{
		Request: &http.Request{URL: mustURL("https://example.test")},
		TLS:     &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}},
	}

	summary := SecuritySummaryFromResponse(resp, nil)
	require.True(t, summary.Secure)
	require.Equal(t, "https", summary.Scheme)
	require.Equal(t, "example.test", summary.Subject)
	require.Equal(t, "Goosie Test CA", summary.Issuer)
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run:

```bash
go test ./internal/net -run 'TestServiceFetch|TestDownloadManager|TestSecuritySummary' -count=1
```

Expected: FAIL because service, cache, download, and security types do not exist.

- [ ] **Step 3: Implement request log**

Create `internal/net/log.go`:

```go
package net

import (
	"sync"
	"time"
)

type RequestLogEntry struct {
	Method      string
	URL         string
	Status      int
	ContentType string
	Bytes       int64
	CacheHit    bool
	Error       string
	StartedAt   time.Time
	Duration    time.Duration
}

type RequestLog struct {
	mu      sync.RWMutex
	entries []RequestLogEntry
}

func NewRequestLog() *RequestLog {
	return &RequestLog{entries: []RequestLogEntry{}}
}

func (l *RequestLog) Add(entry RequestLogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

func (l *RequestLog) Entries() []RequestLogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]RequestLogEntry, len(l.entries))
	copy(out, l.entries)
	return out
}
```

- [ ] **Step 4: Implement cache**

Create `internal/net/cache.go`:

```go
package net

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type CacheEntry struct {
	URL         string    `json:"url"`
	Status      int       `json:"status"`
	ContentType string    `json:"content_type"`
	StoredAt    time.Time `json:"stored_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	BodyFile    string    `json:"body_file"`
}

type HTTPCache struct {
	root    string
	private bool
}

func NewHTTPCache(root string, private bool) *HTTPCache {
	return &HTTPCache{root: root, private: private}
}

func (c *HTTPCache) Get(rawURL string) (string, CacheEntry, bool) {
	if c == nil || c.private {
		return "", CacheEntry{}, false
	}
	metaPath, bodyPath := c.paths(rawURL)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return "", CacheEntry{}, false
	}
	var entry CacheEntry
	if json.Unmarshal(data, &entry) != nil || time.Now().UTC().After(entry.ExpiresAt) {
		return "", CacheEntry{}, false
	}
	body, err := os.ReadFile(bodyPath)
	if err != nil {
		return "", CacheEntry{}, false
	}
	return string(body), entry, true
}

func (c *HTTPCache) Put(rawURL string, resp *http.Response, body string) {
	if c == nil || c.private || resp == nil || resp.StatusCode != http.StatusOK {
		return
	}
	maxAge := cacheMaxAge(resp.Header.Get("Cache-Control"))
	if maxAge <= 0 {
		return
	}
	_ = os.MkdirAll(c.root, 0o700)
	metaPath, bodyPath := c.paths(rawURL)
	entry := CacheEntry{
		URL:         rawURL,
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		StoredAt:    time.Now().UTC(),
		ExpiresAt:   time.Now().UTC().Add(maxAge),
		BodyFile:    filepath.Base(bodyPath),
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(bodyPath, []byte(body), 0o600)
	_ = os.WriteFile(metaPath, append(data, '\n'), 0o600)
}

func (c *HTTPCache) paths(rawURL string) (string, string) {
	sum := sha256.Sum256([]byte(rawURL))
	key := hex.EncodeToString(sum[:])
	return filepath.Join(c.root, key+".json"), filepath.Join(c.root, key+".body")
}

func cacheMaxAge(header string) time.Duration {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "max-age=") {
			seconds, err := strconv.Atoi(strings.TrimPrefix(part, "max-age="))
			if err == nil && seconds > 0 {
				return time.Duration(seconds) * time.Second
			}
		}
	}
	return 0
}
```

- [ ] **Step 5: Implement service and fetcher compatibility**

Create `internal/net/service.go`:

```go
package net

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ServiceOptions struct {
	Client    *http.Client
	Cache     *HTTPCache
	UserAgent string
}

type Service struct {
	client    *http.Client
	cache     *HTTPCache
	userAgent string
	log       *RequestLog
	security  SecuritySummary
}

func NewService(options ServiceOptions) *Service {
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	userAgent := options.UserAgent
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}
	return &Service{client: client, cache: options.Cache, userAgent: userAgent, log: NewRequestLog()}
}

func (s *Service) Fetch(rawURL string) (string, error) {
	return s.FetchWithContext(context.Background(), rawURL, nil)
}

func (s *Service) FetchWithContext(ctx context.Context, rawURL string, onProgress ProgressCallback) (string, error) {
	started := time.Now()
	if body, entry, ok := s.cache.Get(rawURL); ok {
		s.log.Add(RequestLogEntry{Method: http.MethodGet, URL: rawURL, Status: entry.Status, ContentType: entry.ContentType, Bytes: int64(len(body)), CacheHit: true, StartedAt: started, Duration: time.Since(started)})
		return body, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		s.log.Add(RequestLogEntry{Method: http.MethodGet, URL: rawURL, Error: err.Error(), StartedAt: started, Duration: time.Since(started)})
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		s.security = SecuritySummaryFromResponse(nil, err)
		s.log.Add(RequestLogEntry{Method: req.Method, URL: rawURL, Error: err.Error(), StartedAt: started, Duration: time.Since(started)})
		return "", fmt.Errorf("fetch URL: %w", err)
	}
	defer resp.Body.Close()
	s.security = SecuritySummaryFromResponse(resp, nil)

	reader := io.Reader(resp.Body)
	if onProgress != nil && resp.ContentLength > 0 {
		reader = &progressReader{Reader: resp.Body, total: resp.ContentLength, callback: onProgress}
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		s.log.Add(RequestLogEntry{Method: req.Method, URL: rawURL, Status: resp.StatusCode, Error: err.Error(), StartedAt: started, Duration: time.Since(started)})
		return "", fmt.Errorf("read response body: %w", err)
	}
	body := buf.String()
	s.cache.Put(rawURL, resp, body)
	s.log.Add(RequestLogEntry{Method: req.Method, URL: rawURL, Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Bytes: int64(len(body)), StartedAt: started, Duration: time.Since(started)})
	return body, nil
}

func (s *Service) Log() *RequestLog {
	return s.log
}

func (s *Service) Security() SecuritySummary {
	return s.security
}
```

Modify `internal/net/fetcher.go` so `Fetcher` delegates to the service while preserving the existing public API:

```go
type Fetcher struct {
	service *Service
}

func NewFetcher() *Fetcher {
	jar, _ := cookiejar.New(nil)
	return NewFetcherWithClient(&http.Client{Jar: jar})
}

func NewFetcherWithClient(client *http.Client) *Fetcher {
	return &Fetcher{service: NewService(ServiceOptions{Client: client})}
}

func NewFetcherWithService(service *Service) *Fetcher {
	return &Fetcher{service: service}
}

func (f *Fetcher) Service() *Service {
	return f.service
}

func (f *Fetcher) Fetch(url string) (string, error) {
	return f.FetchWithContext(context.Background(), url, nil)
}

func (f *Fetcher) FetchWithContext(ctx context.Context, url string, onProgress ProgressCallback) (string, error) {
	return f.service.FetchWithContext(ctx, url, onProgress)
}
```

Keep the existing `ProgressCallback` and `progressReader` definitions in `fetcher.go`. Remove old duplicate request implementation imports after the edit.

- [ ] **Step 6: Implement downloads**

Create `internal/net/downloads.go`:

```go
package net

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type DownloadStatus string

const (
	DownloadRunning  DownloadStatus = "running"
	DownloadComplete DownloadStatus = "complete"
	DownloadFailed   DownloadStatus = "failed"
)

type DownloadRecord struct {
	URL          string
	TargetPath   string
	Status       DownloadStatus
	BytesWritten int64
	Error        string
	StartedAt    time.Time
	FinishedAt   time.Time
}

type DownloadManager struct {
	client *http.Client
}

func NewDownloadManager(client *http.Client) *DownloadManager {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &DownloadManager{client: client}
}

func (m *DownloadManager) Download(rawURL, targetPath string) (DownloadRecord, error) {
	return m.DownloadWithContext(context.Background(), rawURL, targetPath)
}

func (m *DownloadManager) DownloadWithContext(ctx context.Context, rawURL, targetPath string) (DownloadRecord, error) {
	record := DownloadRecord{URL: rawURL, TargetPath: targetPath, Status: DownloadRunning, StartedAt: time.Now().UTC()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		record.Status = DownloadFailed
		record.Error = err.Error()
		record.FinishedAt = time.Now().UTC()
		return record, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		record.Status = DownloadFailed
		record.Error = err.Error()
		record.FinishedAt = time.Now().UTC()
		return record, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		err := fmt.Errorf("download HTTP status %d", resp.StatusCode)
		record.Status = DownloadFailed
		record.Error = err.Error()
		record.FinishedAt = time.Now().UTC()
		return record, err
	}
	out, err := os.Create(targetPath)
	if err != nil {
		record.Status = DownloadFailed
		record.Error = err.Error()
		record.FinishedAt = time.Now().UTC()
		return record, err
	}
	defer out.Close()
	written, err := io.Copy(out, resp.Body)
	record.BytesWritten = written
	record.FinishedAt = time.Now().UTC()
	if err != nil {
		record.Status = DownloadFailed
		record.Error = err.Error()
		return record, err
	}
	record.Status = DownloadComplete
	return record, nil
}
```

- [ ] **Step 7: Implement security summaries**

Create `internal/net/security.go`:

```go
package net

import (
	"net/http"
	"time"
)

type SecuritySummary struct {
	URL       string
	Scheme    string
	Secure    bool
	Subject   string
	Issuer    string
	NotBefore time.Time
	NotAfter  time.Time
	Error     string
}

func SecuritySummaryFromResponse(resp *http.Response, err error) SecuritySummary {
	if err != nil {
		return SecuritySummary{Secure: false, Error: err.Error()}
	}
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return SecuritySummary{}
	}
	summary := SecuritySummary{URL: resp.Request.URL.String(), Scheme: resp.Request.URL.Scheme, Secure: resp.Request.URL.Scheme == "https"}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		summary.Subject = cert.Subject.CommonName
		summary.Issuer = cert.Issuer.CommonName
		summary.NotBefore = cert.NotBefore
		summary.NotAfter = cert.NotAfter
	}
	return summary
}
```

Add test helpers at the bottom of `internal/net/security_test.go`:

```go
func pkixName(commonName string) pkix.Name {
	return pkix.Name{CommonName: commonName}
}

func mustURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return parsed
}
```

Also add imports for `crypto/x509/pkix` and `net/url` in `security_test.go`.

- [ ] **Step 8: Verify network tests pass**

Run:

```bash
go test ./internal/net -count=1
```

Expected: PASS in an environment that allows existing `httptest` tests; in a restricted sandbox, service tests with fake transports pass and existing listener-based tests may need the test-tier work in Task 8.

- [ ] **Step 9: Commit**

```bash
git add internal/net
git commit -m "feat: add shared browser network service"
```

---

### Task 4: Correct Form Submission Behavior

**Files:**
- Modify: `internal/form/state.go`
- Modify: `internal/form/submission.go`
- Modify: `internal/form/form_submission_test.go`
- Modify: `internal/form/form_error_scenarios_test.go`

- [ ] **Step 1: Replace network-dependent form tests with fake-client tests**

In `internal/form/form_submission_test.go`, replace `TestHTTPGet_Submission`, `TestHTTPPost_Submission`, and `TestHTTPResponse_Success` with:

```go
type fakeSubmitClient struct {
	requests []*http.Request
	body     string
	status   int
	err      error
}

func (c *fakeSubmitClient) Do(req *http.Request) (*http.Response, error) {
	c.requests = append(c.requests, req)
	if c.err != nil {
		return nil, c.err
	}
	status := c.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(c.body)),
		Request:    req,
	}, nil
}

func TestHTTPGet_SubmissionResolvesRelativeAction(t *testing.T) {
	htmlContent := `<html><body><form method="GET" action="/api/search"><input name="q" value="test"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)

	client := &fakeSubmitClient{}
	submitter := NewFormSubmitter()
	submitter.SetClient(client)
	submitter.SetDocumentURL("https://example.com/page")
	state := NewFormState(formNode)

	result, err := submitter.Submit(formNode, state.GetFormData())
	require.NoError(t, err)
	require.Equal(t, "GET", result.Method)
	require.Equal(t, "https://example.com/api/search?q=test", result.URL)
	require.Len(t, client.requests, 1)
}

func TestHTTPPost_SubmissionEncodesFormBody(t *testing.T) {
	htmlContent := `<html><body><form method="POST" action="/api/user"><input name="name" value="John"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)

	client := &fakeSubmitClient{}
	submitter := NewFormSubmitter()
	submitter.SetClient(client)
	submitter.SetDocumentURL("https://example.com/profile")
	state := NewFormState(formNode)

	result, err := submitter.Submit(formNode, state.GetFormData())
	require.NoError(t, err)
	require.Equal(t, "POST", result.Method)
	require.Equal(t, "John", result.Body.Get("name"))
	require.Equal(t, "application/x-www-form-urlencoded", client.requests[0].Header.Get("Content-Type"))
}

func TestHTTPResponse_Success(t *testing.T) {
	htmlContent := `<html><body><form method="POST" action="/api/submit"><input name="data" value="test"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)

	submitter := NewFormSubmitter()
	submitter.SetClient(&fakeSubmitClient{status: http.StatusOK})
	submitter.SetDocumentURL("https://example.com/form")
	state := NewFormState(formNode)
	onSuccess := false
	submitter.SetSuccessCallback(func(r *SubmissionResult) { onSuccess = true })

	_, err = submitter.Submit(formNode, state.GetFormData())
	require.NoError(t, err)
	require.True(t, onSuccess, "Success callback should be called on 200 OK")
}
```

Add imports to `form_submission_test.go`:

```go
import (
	"io"
	"net/http"
	"strings"
	"testing"
)
```

- [ ] **Step 2: Update XSS test to assert raw preservation and escaped display**

In `internal/form/form_error_scenarios_test.go`, replace `TestErrorScenario_SpecialCharacters_Input` with:

```go
func TestErrorScenario_SpecialCharacters_InputPreservedAndEscapedForDisplay(t *testing.T) {
	htmlContent := `<html><body><form><input type="text" name="comment" value="<script>alert('xss')</script>"></form></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)
	formNode := findFirstNode(doc, "form")
	require.NotNil(t, formNode)

	state := NewFormState(formNode)
	data := state.GetFormData()
	raw := data.Get("comment")
	require.Equal(t, "<script>alert('xss')</script>", raw)
	require.Equal(t, "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;", EscapeForDisplay(raw))
}
```

Remove `TestEdgeCase_FormInIframe_Submit` from `internal/form/form_error_scenarios_test.go`. The milestone does not implement iframe submission routing, and the old test asserts a hard-coded false value.

- [ ] **Step 3: Run form tests and verify the new tests fail**

Run:

```bash
go test ./internal/form -run 'TestHTTPGet_SubmissionResolvesRelativeAction|TestHTTPPost_SubmissionEncodesFormBody|TestHTTPResponse_Success|TestFormSubmission_MultiSubmit_Prevention|TestErrorScenario_SpecialCharacters_InputPreservedAndEscapedForDisplay' -count=1
```

Expected: FAIL because `SetClient`, `SetDocumentURL`, persistent duplicate-submit prevention, and `EscapeForDisplay` do not exist.

- [ ] **Step 4: Implement persistent duplicate-submit prevention**

Modify `internal/form/state.go`:

```go
type FormState struct {
	formNode      *html.Node
	submitEnabled bool
	cancelEnabled bool
	submitting    bool
	onSubmit      SubmitCallback
	onCancel      CancelCallback
}

func (s *FormState) Submit() {
	if !s.submitEnabled || s.submitting {
		return
	}
	s.submitting = true
	if s.onSubmit != nil {
		s.onSubmit(s.GetFormData())
	}
}

func (s *FormState) ResetSubmission() {
	s.submitting = false
	s.submitEnabled = true
}
```

Keep `CancelSubmission` as-is unless an existing test requires different behavior.

- [ ] **Step 5: Implement submit client, base URL resolution, and escaping**

Modify `internal/form/submission.go`:

```go
type SubmitClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type FormSubmitter struct {
	onSuccess   SuccessCallback
	onError     ErrorCallback
	timeoutMs   int
	client      SubmitClient
	documentURL string
}

func (s *FormSubmitter) SetClient(client SubmitClient) {
	s.client = client
}

func (s *FormSubmitter) SetDocumentURL(rawURL string) {
	s.documentURL = rawURL
}

func EscapeForDisplay(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&#34;",
		"'", "&#39;",
	)
	return replacer.Replace(value)
}

func (s *FormSubmitter) resolveAction(action string) (string, error) {
	parsed, err := url.Parse(action)
	if err != nil {
		return "", err
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	}
	if s.documentURL == "" {
		return action, nil
	}
	base, err := url.Parse(s.documentURL)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(parsed).String(), nil
}
```

In `NewFormSubmitter`, keep the default client as `&http.Client{Timeout: 30 * time.Second}`.

In `Submit`, resolve `action` before building query/body:

```go
resolvedAction, err := s.resolveAction(action)
if err != nil {
	if s.onError != nil {
		s.onError(err)
	}
	return nil, err
}
action = resolvedAction
```

Use `s.client.Do(req)` instead of `s.client.Do` on a concrete `*http.Client`.

- [ ] **Step 6: Verify form tests pass**

Run:

```bash
go test ./internal/form -count=1
```

Expected: PASS without opening real network sockets.

- [ ] **Step 7: Commit**

```bash
git add internal/form
git commit -m "fix: make form submission browser-correct"
```

---

### Task 5: JavaScript Runtime Persistent Storage and Shared Fetch

**Files:**
- Modify: `internal/js/runtime.go`
- Modify: `internal/js/runtime_test.go`

- [ ] **Step 1: Write failing runtime storage adapter tests**

Add to `internal/js/runtime_test.go`:

```go
type memoryStorageAdapter struct {
	values map[string]map[string]string
}

func newMemoryStorageAdapter() *memoryStorageAdapter {
	return &memoryStorageAdapter{values: map[string]map[string]string{}}
}

func (s *memoryStorageAdapter) Get(origin, key string) (string, bool) {
	if s.values[origin] == nil {
		return "", false
	}
	value, ok := s.values[origin][key]
	return value, ok
}

func (s *memoryStorageAdapter) Set(origin, key, value string) error {
	if s.values[origin] == nil {
		s.values[origin] = map[string]string{}
	}
	s.values[origin][key] = value
	return nil
}

func (s *memoryStorageAdapter) Remove(origin, key string) error {
	if s.values[origin] != nil {
		delete(s.values[origin], key)
	}
	return nil
}

func (s *memoryStorageAdapter) Clear(origin string) error {
	delete(s.values, origin)
	return nil
}

func (s *memoryStorageAdapter) Keys(origin string) []string {
	keys := make([]string, 0, len(s.values[origin]))
	for key := range s.values[origin] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestLocalStorageUsesOriginScopedAdapter(t *testing.T) {
	store := newMemoryStorageAdapter()

	first := NewRuntime()
	first.SetOrigin("https://one.test")
	first.SetLocalStorageAdapter(store)
	_, err := first.RunScript(`localStorage.setItem("theme", "dark")`)
	require.NoError(t, err)

	second := NewRuntime()
	second.SetOrigin("https://one.test")
	second.SetLocalStorageAdapter(store)
	value, err := second.RunScript(`localStorage.getItem("theme")`)
	require.NoError(t, err)
	require.Contains(t, value.String(), "dark")

	third := NewRuntime()
	third.SetOrigin("https://two.test")
	third.SetLocalStorageAdapter(store)
	missing, err := third.RunScript(`localStorage.getItem("theme")`)
	require.NoError(t, err)
	require.Equal(t, "null", missing.String())
}
```

Add imports to `runtime_test.go` if absent:

```go
import (
	"sort"

	"github.com/stretchr/testify/require"
)
```

- [ ] **Step 2: Run the runtime test and verify it fails**

Run:

```bash
go test ./internal/js -run TestLocalStorageUsesOriginScopedAdapter -count=1
```

Expected: FAIL because `SetOrigin`, `SetLocalStorageAdapter`, and the adapter interface do not exist.

- [ ] **Step 3: Add storage adapter interface and origin setter**

Modify `internal/js/runtime.go` by adding the interface near `HTTPFetcher`:

```go
type LocalStorageAdapter interface {
	Get(origin, key string) (string, bool)
	Set(origin, key, value string) error
	Remove(origin, key string) error
	Clear(origin string) error
	Keys(origin string) []string
}
```

Add these fields to the existing `Runtime` struct immediately after `sessionStorage map[string]string`:

```go
localStorageAdapter LocalStorageAdapter
origin              string
```

Add these methods below `NewRuntime`:

```go
func (r *Runtime) SetOrigin(origin string) {
	r.origin = origin
}

func (r *Runtime) SetLocalStorageAdapter(adapter LocalStorageAdapter) {
	r.localStorageAdapter = adapter
	r.setupLocalStorageAPI()
}
```

Set default origin in `NewRuntime`:

```go
origin: "about:blank",
```

- [ ] **Step 4: Route localStorage methods through the adapter when present**

In `setupLocalStorageAPI`, change `getItem`, `setItem`, `removeItem`, `clear`, `key`, and `length` to use helper methods:

```go
func (r *Runtime) localStorageGet(key string) (string, bool) {
	if r.localStorageAdapter != nil {
		return r.localStorageAdapter.Get(r.origin, key)
	}
	value, ok := r.localStorage[key]
	return value, ok
}

func (r *Runtime) localStorageSet(key, value string) {
	if r.localStorageAdapter != nil {
		_ = r.localStorageAdapter.Set(r.origin, key, value)
		return
	}
	r.localStorage[key] = value
}

func (r *Runtime) localStorageRemove(key string) {
	if r.localStorageAdapter != nil {
		_ = r.localStorageAdapter.Remove(r.origin, key)
		return
	}
	delete(r.localStorage, key)
}

func (r *Runtime) localStorageClear() {
	if r.localStorageAdapter != nil {
		_ = r.localStorageAdapter.Clear(r.origin)
		return
	}
	r.localStorage = make(map[string]string)
}

func (r *Runtime) localStorageKeys() []string {
	if r.localStorageAdapter != nil {
		return r.localStorageAdapter.Keys(r.origin)
	}
	keys := make([]string, 0, len(r.localStorage))
	for key := range r.localStorage {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
```

Preserve the existing versioned value format only for in-memory fallback if existing tests depend on it; for adapter-backed storage, store the web-visible value directly.

- [ ] **Step 5: Verify runtime tests pass**

Run:

```bash
go test ./internal/js -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/js
git commit -m "feat: persist localStorage by origin"
```

---

### Task 6: Browser UI Wiring, Address/Search Handling, Shortcuts, and Profile Sync

**Files:**
- Create: `internal/ui/address.go`
- Create: `internal/ui/address_test.go`
- Create: `internal/ui/shortcuts.go`
- Create: `internal/ui/shortcuts_test.go`
- Modify: `internal/ui/browser.go`
- Modify: `internal/ui/settings.go`
- Modify: `cmd/browser/main.go`

- [ ] **Step 1: Write failing address and shortcut tests**

Create `internal/ui/address_test.go`:

```go
package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveAddressInput(t *testing.T) {
	search := "https://duckduckgo.com/?q="
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "absolute https", input: "https://example.com/a", want: "https://example.com/a"},
		{name: "host becomes https", input: "example.com", want: "https://example.com"},
		{name: "localhost keeps http", input: "localhost:8080", want: "http://localhost:8080"},
		{name: "query uses search engine", input: "golang browser engine", want: "https://duckduckgo.com/?q=golang+browser+engine"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAddressInput(tt.input, search)
			require.Equal(t, tt.want, got)
		})
	}
}
```

Create `internal/ui/shortcuts_test.go`:

```go
package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShortcutRegistryDispatchesCommands(t *testing.T) {
	registry := NewShortcutRegistry()
	calls := []string{}
	registry.Register("focus-address", func() { calls = append(calls, "focus-address") })
	registry.Register("new-tab", func() { calls = append(calls, "new-tab") })

	require.True(t, registry.Dispatch("focus-address"))
	require.True(t, registry.Dispatch("new-tab"))
	require.False(t, registry.Dispatch("missing"))
	require.Equal(t, []string{"focus-address", "new-tab"}, calls)
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run:

```bash
go test ./internal/ui -run 'TestResolveAddressInput|TestShortcutRegistryDispatchesCommands' -count=1
```

Expected: FAIL because helper files do not exist.

- [ ] **Step 3: Implement address/search normalization**

Create `internal/ui/address.go`:

```go
package ui

import (
	"net/url"
	"strings"
)

func ResolveAddressInput(input, searchEngine string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed.String()
	}
	if looksLikeHost(trimmed) {
		scheme := "https://"
		if strings.HasPrefix(trimmed, "localhost") || strings.HasPrefix(trimmed, "127.0.0.1") {
			scheme = "http://"
		}
		return scheme + trimmed
	}
	if searchEngine == "" {
		searchEngine = "https://www.google.com/search?q="
	}
	return searchEngine + url.QueryEscape(trimmed)
}

func looksLikeHost(input string) bool {
	if strings.Contains(input, " ") {
		return false
	}
	return strings.Contains(input, ".") || strings.HasPrefix(input, "localhost") || strings.HasPrefix(input, "127.0.0.1")
}
```

- [ ] **Step 4: Implement shortcut command registry**

Create `internal/ui/shortcuts.go`:

```go
package ui

type ShortcutRegistry struct {
	commands map[string]func()
}

func NewShortcutRegistry() *ShortcutRegistry {
	return &ShortcutRegistry{commands: map[string]func(){}}
}

func (r *ShortcutRegistry) Register(name string, command func()) {
	r.commands[name] = command
}

func (r *ShortcutRegistry) Dispatch(name string) bool {
	command, ok := r.commands[name]
	if !ok {
		return false
	}
	command()
	return true
}
```

- [ ] **Step 5: Verify helper tests pass**

Run:

```bash
go test ./internal/ui -run 'TestResolveAddressInput|TestShortcutRegistryDispatchesCommands' -count=1
```

Expected: PASS.

- [ ] **Step 6: Wire dependencies into browser construction**

Modify `internal/ui/browser.go` by adding optional dependency fields:

```go
type BrowserDependencies struct {
	Profile       *profile.Profile
	Bookmarks     *profile.BookmarkStore
	History       *profile.HistoryStore
	SettingsStore *profile.SettingsStore
	Storage       *profile.StorageStore
	Network       *net.Service
}
```

Add imports:

```go
goosienet "github.com/vyquocvu/goosie/internal/net"
"github.com/vyquocvu/goosie/internal/profile"
```

Add `deps BrowserDependencies` and `shortcuts *ShortcutRegistry` to `Browser`.

Add:

```go
func NewBrowserWithDependencies(deps BrowserDependencies) *Browser {
	browser := NewBrowser()
	browser.deps = deps
	browser.shortcuts = NewShortcutRegistry()
	browser.registerDefaultShortcuts()
	if deps.SettingsStore != nil {
		browser.settings.ApplyProfileSettings(deps.SettingsStore.Get())
	}
	return browser
}
```

Keep `NewBrowser()` available for existing tests.

- [ ] **Step 7: Wire URL entry and profile sync**

In `createNavigationControls`, change submitted URL handling to:

```go
b.urlEntry.OnSubmitted = func(input string) {
	if b.onNavigate != nil && strings.TrimSpace(input) != "" {
		searchEngine := b.settings.GetDefaultSearchEngine()
		b.onNavigate(ResolveAddressInput(input, searchEngine))
	}
}
```

In `NavigateTo`, after `tab.state.AddToHistory(url)` add:

```go
if b.deps.History != nil {
	_ = b.deps.History.AddVisit(url, tab.title)
}
```

In `toggleBookmark`, mirror add/remove operations to `b.deps.Bookmarks` when present.

- [ ] **Step 8: Map settings to profile settings**

Modify `internal/ui/settings.go` to import `internal/profile` and add:

```go
func (s *Settings) ApplyProfileSettings(settings profile.Settings) {
	s.SetHomepage(settings.Homepage)
	s.SetDefaultSearchEngine(settings.DefaultSearchEngine)
	s.SetEnableJavaScript(settings.EnableJavaScript)
	s.SetEnableImages(settings.EnableImages)
}

func (s *Settings) ToProfileSettings() profile.Settings {
	return profile.Settings{
		Homepage:            s.GetHomepage(),
		DefaultSearchEngine: s.GetDefaultSearchEngine(),
		EnableJavaScript:    s.GetEnableJavaScript(),
		EnableImages:        s.GetEnableImages(),
	}
}
```

- [ ] **Step 9: Construct profile/network in main**

Modify `cmd/browser/main.go` startup:

```go
prof, err := profile.Open(profile.Options{})
if err != nil {
	log.Fatalf("failed to open profile: %v", err)
}
bookmarks, err := profile.NewBookmarkStore(prof)
if err != nil {
	log.Fatalf("failed to open bookmarks: %v", err)
}
history, err := profile.NewHistoryStore(prof)
if err != nil {
	log.Fatalf("failed to open history: %v", err)
}
settingsStore, err := profile.NewSettingsStore(prof)
if err != nil {
	log.Fatalf("failed to open settings: %v", err)
}
storage, err := profile.NewStorageStore(prof)
if err != nil {
	log.Fatalf("failed to open storage: %v", err)
}
networkService := net.NewService(net.ServiceOptions{
	Cache: net.NewHTTPCache(filepath.Join(prof.Root(), "cache"), prof.Private()),
})
fetcher := net.NewFetcherWithService(networkService)
browser := ui.NewBrowserWithDependencies(ui.BrowserDependencies{
	Profile:       prof,
	Bookmarks:     bookmarks,
	History:       history,
	SettingsStore: settingsStore,
	Storage:       storage,
	Network:       networkService,
})
```

Add imports for `path/filepath` and `internal/profile`.

- [ ] **Step 10: Verify browser packages build**

Run:

```bash
go test ./internal/ui ./cmd/browser -count=1
```

Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add internal/ui cmd/browser
git commit -m "feat: wire browser profile and address handling"
```

---

### Task 7: Developer Tools Panels and Console Execution

**Files:**
- Create: `internal/ui/network_panel.go`
- Create: `internal/ui/storage_panel.go`
- Create: `internal/ui/security_panel.go`
- Create: `internal/ui/downloads_panel.go`
- Modify: `internal/ui/console.go`
- Modify: `internal/ui/browser.go`

- [ ] **Step 1: Write failing panel smoke tests**

Create `internal/ui/network_panel_test.go`:

```go
package ui

import (
	"testing"
	"time"

	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/stretchr/testify/require"
)

func TestNetworkPanelAcceptsEntries(t *testing.T) {
	panel := NewNetworkPanel()
	panel.SetEntries([]goosienet.RequestLogEntry{
		{Method: "GET", URL: "https://example.com", Status: 200, Bytes: 42, StartedAt: time.Now()},
	})
	require.NotNil(t, panel.CanvasObject())
	require.Len(t, panel.entries, 1)
}
```

Create `internal/ui/security_panel_test.go`:

```go
package ui

import (
	"testing"

	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/stretchr/testify/require"
)

func TestSecurityPanelSummary(t *testing.T) {
	panel := NewSecurityPanel()
	panel.SetSummary(goosienet.SecuritySummary{URL: "https://example.com", Scheme: "https", Secure: true, Subject: "example.com"})
	require.NotNil(t, panel.CanvasObject())
	require.Contains(t, panel.summaryLabel.Text, "example.com")
}
```

- [ ] **Step 2: Run panel tests and verify they fail**

Run:

```bash
go test ./internal/ui -run 'TestNetworkPanelAcceptsEntries|TestSecurityPanelSummary' -count=1
```

Expected: FAIL because panels do not exist.

- [ ] **Step 3: Implement network panel**

Create `internal/ui/network_panel.go`:

```go
package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	goosienet "github.com/vyquocvu/goosie/internal/net"
)

type NetworkPanel struct {
	container *fyne.Container
	list      *widget.List
	entries   []goosienet.RequestLogEntry
}

func NewNetworkPanel() *NetworkPanel {
	panel := &NetworkPanel{entries: []goosienet.RequestLogEntry{}}
	panel.list = widget.NewList(
		func() int { return len(panel.entries) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, object fyne.CanvasObject) {
			entry := panel.entries[id]
			object.(*widget.Label).SetText(fmt.Sprintf("%s %d %s %dB", entry.Method, entry.Status, entry.URL, entry.Bytes))
		},
	)
	panel.container = container.NewBorder(widget.NewLabel("Network"), nil, nil, nil, panel.list)
	return panel
}

func (p *NetworkPanel) SetEntries(entries []goosienet.RequestLogEntry) {
	p.entries = append([]goosienet.RequestLogEntry(nil), entries...)
	p.list.Refresh()
}

func (p *NetworkPanel) CanvasObject() fyne.CanvasObject {
	return p.container
}
```

- [ ] **Step 4: Implement security, storage, and downloads panels**

Create `internal/ui/security_panel.go`:

```go
package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	goosienet "github.com/vyquocvu/goosie/internal/net"
)

type SecurityPanel struct {
	container    *fyne.Container
	summaryLabel *widget.Label
}

func NewSecurityPanel() *SecurityPanel {
	panel := &SecurityPanel{summaryLabel: widget.NewLabel("No security information")}
	panel.container = container.NewBorder(widget.NewLabel("Security"), nil, nil, nil, panel.summaryLabel)
	return panel
}

func (p *SecurityPanel) SetSummary(summary goosienet.SecuritySummary) {
	state := "Not secure"
	if summary.Secure {
		state = "Secure"
	}
	p.summaryLabel.SetText(fmt.Sprintf("%s\n%s\nSubject: %s\nIssuer: %s\nError: %s", summary.URL, state, summary.Subject, summary.Issuer, summary.Error))
}

func (p *SecurityPanel) CanvasObject() fyne.CanvasObject {
	return p.container
}
```

Create `internal/ui/storage_panel.go`:

```go
package ui

import (
	"fmt"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type StoragePanel struct {
	container *fyne.Container
	label     *widget.Label
}

func NewStoragePanel() *StoragePanel {
	panel := &StoragePanel{label: widget.NewLabel("No storage data")}
	panel.label.Wrapping = fyne.TextWrapWord
	panel.container = container.NewBorder(widget.NewLabel("Storage"), nil, nil, nil, panel.label)
	return panel
}

func (p *StoragePanel) SetSnapshot(snapshot map[string]map[string]string) {
	origins := make([]string, 0, len(snapshot))
	for origin := range snapshot {
		origins = append(origins, origin)
	}
	sort.Strings(origins)
	lines := ""
	for _, origin := range origins {
		lines += origin + "\n"
		keys := make([]string, 0, len(snapshot[origin]))
		for key := range snapshot[origin] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines += fmt.Sprintf("  %s = %s\n", key, snapshot[origin][key])
		}
	}
	if lines == "" {
		lines = "No storage data"
	}
	p.label.SetText(lines)
}

func (p *StoragePanel) CanvasObject() fyne.CanvasObject {
	return p.container
}
```

Create `internal/ui/downloads_panel.go`:

```go
package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	goosienet "github.com/vyquocvu/goosie/internal/net"
)

type DownloadsPanel struct {
	container *fyne.Container
	list      *widget.List
	records   []goosienet.DownloadRecord
}

func NewDownloadsPanel() *DownloadsPanel {
	panel := &DownloadsPanel{records: []goosienet.DownloadRecord{}}
	panel.list = widget.NewList(
		func() int { return len(panel.records) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, object fyne.CanvasObject) {
			record := panel.records[id]
			object.(*widget.Label).SetText(fmt.Sprintf("%s %s %dB", record.Status, record.TargetPath, record.BytesWritten))
		},
	)
	panel.container = container.NewBorder(widget.NewLabel("Downloads"), nil, nil, nil, panel.list)
	return panel
}

func (p *DownloadsPanel) SetRecords(records []goosienet.DownloadRecord) {
	p.records = append([]goosienet.DownloadRecord(nil), records...)
	p.list.Refresh()
}

func (p *DownloadsPanel) CanvasObject() fyne.CanvasObject {
	return p.container
}
```

- [ ] **Step 5: Add console command execution entry**

Modify `internal/ui/console.go` by adding these fields to the existing `ConsolePanel` struct after `filterLevel string`:

```go
commandEntry *widget.Entry
onExecute    func(string)
```

Add this method below `SetRefreshCallback`:

```go
func (cp *ConsolePanel) SetExecuteCallback(callback func(string)) {
	cp.onExecute = callback
}
```

In `NewConsolePanel`, create a command entry before the main container:

```go
panel.commandEntry = widget.NewEntry()
panel.commandEntry.SetPlaceHolder("Execute JavaScript")
panel.commandEntry.OnSubmitted = func(source string) {
	if panel.onExecute != nil && source != "" {
		panel.onExecute(source)
		panel.commandEntry.SetText("")
	}
}
```

Place it at the bottom:

```go
panel.container = container.NewBorder(topBar, panel.commandEntry, nil, nil, panel.messageList)
```

In `internal/ui/browser.go`, after creating `consolePanel`, set:

```go
browser.consolePanel.SetExecuteCallback(func(source string) {
	if tab := browser.ActiveTab(); tab != nil && tab.jsRuntime != nil {
		value, err := tab.jsRuntime.RunScript(source)
		if err != nil {
			browser.consolePanel.AddMessage(js.ConsoleMessage{Level: "error", Message: err.Error(), Timestamp: time.Now(), Data: err.Error()})
			return
		}
		browser.consolePanel.AddMessage(js.ConsoleMessage{Level: "log", Message: value.String(), Timestamp: time.Now(), Data: value.String()})
	}
})
```

Add `time` to `browser.go` imports if absent.

- [ ] **Step 6: Verify UI tests pass**

Run:

```bash
go test ./internal/ui -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ui
git commit -m "feat: add browser developer tools panels"
```

---

### Task 8: Sandbox-Safe Test Tiers

**Files:**
- Modify: `internal/image/loader_test.go`
- Modify: `internal/net/async_test.go`
- Modify: `internal/test_suite/e2e/page_load_test.go`
- Modify: `test/e2e/main_test.go`
- Modify: `TESTING.md`

- [ ] **Step 1: Add listener availability helper to tests that use `httptest.NewServer`**

In each test file that starts `httptest.NewServer`, add:

```go
func requireLoopbackListener(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback listener unavailable in this environment: %v", err)
	}
	require.NoError(t, ln.Close())
}
```

Add imports:

```go
"net"
"github.com/stretchr/testify/require"
```

Call `requireLoopbackListener(t)` before `httptest.NewServer(...)` in:

- `internal/image/loader_test.go`
- `internal/net/async_test.go`
- `internal/test_suite/e2e/page_load_test.go`

- [ ] **Step 2: Gate Playwright e2e tests behind the `e2e` build tag**

At the top of each file in `test/e2e`, add:

```go
//go:build e2e
```

Then a blank line before `package`.

- [ ] **Step 3: Run sandbox-safe tests**

Run:

```bash
go test ./... -short
```

Expected: PASS or only skip messages for loopback/GUI/e2e environment checks.

- [ ] **Step 4: Update TESTING.md with test tiers**

Add this section to `TESTING.md`:

```markdown
## Test Tiers

Use `go test ./... -short` for sandbox-safe checks. These tests do not require external network access, loopback listeners, GUI launch permissions, or Playwright.

Use `go test ./...` for the normal local suite. Tests that require `httptest` loopback servers skip themselves when the host environment forbids opening a listener.

Use `go test -tags=e2e ./test/e2e` for Playwright-driven browser tests. This tier requires Playwright browsers and host permissions to launch Chromium.

Use package-specific commands such as `go test ./internal/net -run TestServiceFetchRecordsRequestLog` while developing a focused subsystem.
```

- [ ] **Step 5: Verify normal build command**

Run:

```bash
go test ./cmd/browser ./internal/profile ./internal/net ./internal/form ./internal/js ./internal/ui -count=1
```

Expected: PASS, with listener-based tests skipped only when the environment forbids listeners.

- [ ] **Step 6: Commit**

```bash
git add internal/image/loader_test.go internal/net/async_test.go internal/test_suite/e2e/page_load_test.go test/e2e TESTING.md
git commit -m "test: split sandbox-safe browser test tiers"
```

---

### Task 9: Release Pipeline and Documentation

**Files:**
- Create: `.github/workflows/release.yml`
- Modify: `README.md`
- Modify: `ROADMAP_V2.md`
- Modify: `TASKS.md`

- [ ] **Step 1: Add release workflow**

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  build:
    name: Build ${{ matrix.goos }} ${{ matrix.goarch }}
    runs-on: ${{ matrix.runner }}
    strategy:
      fail-fast: false
      matrix:
        include:
          - goos: darwin
            goarch: arm64
            runner: macos-latest
          - goos: darwin
            goarch: amd64
            runner: macos-latest
          - goos: linux
            goarch: amd64
            runner: ubuntu-latest
          - goos: windows
            goarch: amd64
            runner: windows-latest

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Install Linux Fyne dependencies
        if: matrix.goos == 'linux'
        run: sudo apt-get update && sudo apt-get install -y libgl1-mesa-dev xorg-dev

      - name: Test
        run: go test ./... -short

      - name: Build
        shell: bash
        run: |
          mkdir -p dist
          name="goosie-${GITHUB_REF_NAME}-${{ matrix.goos }}-${{ matrix.goarch }}"
          ext=""
          if [ "${{ matrix.goos }}" = "windows" ]; then ext=".exe"; fi
          GOOS=${{ matrix.goos }} GOARCH=${{ matrix.goarch }} go build -o "dist/${name}${ext}" ./cmd/browser
          cd dist
          if [ "${{ matrix.goos }}" = "windows" ]; then
            powershell Compress-Archive -Path "${name}${ext}" -DestinationPath "${name}.zip"
          else
            tar -czf "${name}.tar.gz" "${name}${ext}"
          fi
          shasum -a 256 * > "${name}.sha256"

      - name: Upload release assets
        uses: softprops/action-gh-release@v2
        with:
          files: dist/*
```

- [ ] **Step 2: Update README feature and usage sections**

Add bullets under Features:

```markdown
- **Persistent Profile Foundation**: Browser data can be stored in a profile directory, including bookmarks, history, settings, origin-scoped localStorage, cookies, cache metadata, and download history.
- **Private Browsing Foundation**: Ephemeral profile mode keeps browsing state in memory and avoids writing profile data to disk.
- **Developer Tools Foundation**: Console execution, DOM inspection, network log, storage view, security summary, page source, and downloads panels.
- **Release Builds**: Tag-based GitHub Actions workflow builds cross-platform browser binaries.
```

Add usage commands:

```markdown
### Test Tiers

```bash
go test ./... -short
go test ./...
go test -tags=e2e ./test/e2e
```

Use the short tier for sandbox-safe checks and the e2e tier for Playwright-driven browser tests.
```

- [ ] **Step 3: Update ROADMAP and TASKS**

In `ROADMAP_V2.md`, add a v1.0 Browser Foundation section marking these as completed once implementation is merged:

```markdown
## Phase 5: Browser Foundation (v1.0.0)

- [x] Persistent profile stores for bookmarks, history, settings, session restore, and origin-scoped localStorage
- [x] Shared network service with cookies, HTTP cache, request log, downloads, and TLS summaries
- [x] Correct form submission behavior with relative action resolution and duplicate-submit prevention
- [x] Developer tools panels for console execution, network, storage, security, and downloads
- [x] Sandbox-safe test tier and release workflow
```

In `TASKS.md`, mark addressed items with a status line:

```markdown
**Status:** done (2026-06-19) — implemented in Goosie Browser Foundation milestone
```

Apply that status only to items genuinely completed by this plan: TLS configuration/summary, release workflow, request logging, form-related baseline failures, and test-tier documentation.

- [ ] **Step 4: Validate workflow YAML and docs**

Run:

```bash
go test ./cmd/browser ./internal/profile ./internal/net ./internal/form ./internal/js ./internal/ui -count=1
```

Expected: PASS.

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/release.yml README.md ROADMAP_V2.md TASKS.md TESTING.md
git commit -m "docs: document browser foundation release"
```

---

### Task 10: Final Verification

**Files:**
- Modify only files needed to fix verification failures from earlier tasks.

- [ ] **Step 1: Run focused package tests**

Run:

```bash
go test ./internal/profile ./internal/net ./internal/form ./internal/js ./internal/ui ./cmd/browser -count=1
```

Expected: PASS.

- [ ] **Step 2: Run sandbox-safe full suite**

Run:

```bash
go test ./... -short
```

Expected: PASS with acceptable skip messages for environment-gated tests.

- [ ] **Step 3: Build the browser binary**

Run:

```bash
go build ./cmd/browser
```

Expected: PASS and a `browser` binary appears in the repository root.

- [ ] **Step 4: Remove local build artifact**

Run:

```bash
rm -f browser
```

Expected: `browser` is removed.

- [ ] **Step 5: Check worktree and formatting**

Run:

```bash
gofmt -w internal/profile internal/net internal/form internal/js internal/ui cmd/browser
git diff --check
git status --short
```

Expected: `git diff --check` has no output. `git status --short` shows only intentional tracked changes if a verification fix was needed.

- [ ] **Step 6: Commit verification fixes if needed**

If Step 5 shows tracked changes from verification fixes:

```bash
git add internal/profile internal/net internal/form internal/js internal/ui cmd/browser README.md ROADMAP_V2.md TASKS.md TESTING.md .github/workflows/release.yml
git commit -m "fix: stabilize browser foundation verification"
```

Expected: commit succeeds. If Step 5 shows no changes, skip this commit.

---

## Self-Review Notes

- Spec coverage: profile persistence is covered by Tasks 1-2; network state and TLS/downloads are covered by Task 3; form correctness is covered by Task 4; JavaScript storage/fetch integration is covered by Task 5; UI/devtools/browser ergonomics are covered by Tasks 6-7; test tiers are covered by Task 8; release/docs are covered by Task 9; final verification is covered by Task 10.
- Type consistency: profile store names, network service names, and UI dependency names are consistent across tasks.
- Scope boundary: modern standards features such as Canvas, SVG, media, workers, service workers, modules, and WebSocket are intentionally excluded and remain future specs.
