# Goosie Browser Foundation Design

## Purpose

Goosie already has a custom Go browser engine with HTML parsing, CSS parsing, layout, rendering, JavaScript execution, tabs, navigation history, bookmarks, settings, screenshots, a console panel, and an inspector panel. The next milestone is to turn that engine into a practical lightweight desktop browser foundation while preserving the custom Go/Fyne architecture.

This milestone is not a full modern web platform implementation. Canvas, SVG, media playback, workers, service workers, modules, WebSocket, advanced accessibility, extension APIs, and multi-process isolation remain future milestones. The goal here is to make browsing stateful, testable, inspectable, safer, and shippable.

## Current Evidence

- `cmd/browser/main.go` wires fetching, parsing, rendering, JavaScript execution, and navigation callbacks.
- `internal/ui` contains tabs, navigation controls, browser state, settings, console, inspector, screenshots, and themes.
- `internal/net/fetcher.go` uses `http.Client` with an in-memory cookie jar and progress callbacks.
- `internal/js/runtime.go` exposes browser-like APIs including location, history, timers, fetch, localStorage, and sessionStorage, but storage is currently in memory.
- `internal/form` has validation/submission logic, but baseline tests expose gaps in duplicate-submit handling, relative action URLs, testable submissions, and sanitization policy.
- `ROADMAP.md` and `TASKS.md` still list cookies, TLS UI, caching, downloads, devtools, release pipeline, and test gaps as open or deferred.

## Design Goals

1. Persist browser profile data across launches.
2. Make network state visible and reusable: cookies, cache entries, downloads, TLS details, and request logs.
3. Fix form submission behavior so forms are browser-correct, testable without real network calls, and clear about input preservation versus output escaping.
4. Extend developer tools from console/DOM inspection toward practical page debugging: network, storage, source, and executable JavaScript.
5. Improve everyday browsing ergonomics: address/search handling, private windows/tabs, shortcuts, session restore, and page source.
6. Separate test suites by runtime needs so local sandbox-safe tests can pass without network listeners, GUI launch permissions, or Playwright.
7. Add a release pipeline that produces cross-platform browser binaries from version tags.

## Non-Goals

- Replacing the custom renderer with Chromium, WebKit, or an embedded webview.
- Implementing full browser standards compatibility in this milestone.
- Adding browser extensions, sync, password management, ad blocking, PDF viewing, or mobile apps.
- Guaranteeing perfect compatibility with arbitrary modern websites.
- Treating raw form input as sanitized data. Raw data should be preserved internally; escaping should happen at rendering, logging, or display boundaries.

## Architecture

### Profile Layer

Add `internal/profile` as the ownership boundary for durable browser state. It should manage a profile directory and JSON-backed stores with atomic writes.

Responsibilities:

- Resolve default profile path with an override for tests and portable runs.
- Persist global bookmarks, history, settings, session restore data, localStorage, cookies metadata when needed, and download history.
- Provide private/ephemeral profile instances that keep all data in memory and never write to disk.
- Expose small typed stores instead of making UI packages read/write JSON directly.

Initial files:

- `internal/profile/profile.go`: profile root, paths, load/save helpers, atomic write.
- `internal/profile/bookmarks.go`: bookmark records with title, URL, created time, updated time.
- `internal/profile/history.go`: visit records and session restore tab records.
- `internal/profile/settings.go`: persisted settings model compatible with `internal/ui.Settings`.
- `internal/profile/storage.go`: origin-scoped localStorage persistence for the JavaScript runtime.

### Network Layer

Extend `internal/net` from a fetch helper into a browser network service.

Responsibilities:

- Keep `Fetcher` as the request facade used by browser loads and JavaScript `fetch`.
- Add configurable `http.Client`, timeout, user agent, cookie jar, cache, and TLS inspection.
- Persist cookies through the profile layer for normal browsing.
- Disable persistence and cache writes in private mode.
- Record request/response metadata for the devtools network panel.
- Support downloads with progress, cancellation, target path selection, and final status.

Initial files:

- `internal/net/service.go`: high-level network service configuration and request entry points.
- `internal/net/cache.go`: HTTP cache metadata and body storage keyed by normalized request.
- `internal/net/cookies.go`: persistent cookie jar adapter.
- `internal/net/downloads.go`: download manager with progress and history records.
- `internal/net/security.go`: TLS/certificate summary for the active page.
- `internal/net/log.go`: request log entries for developer tools.

### Form Layer

Keep `internal/form` focused on form state, validation, collection, and submission policy.

Changes:

- Add a submit client interface so tests can simulate responses without opening sockets.
- Resolve relative form `action` values against the active document URL before submission.
- Make duplicate-submit prevention explicit: one in-flight submission per form until completion, failure, or reset.
- Preserve raw submitted values in `FormData`.
- Add display helpers for escaped values at UI/log boundaries when form data is rendered back into HTML, dialogs, console output, or error views.
- Remove placeholder iframe expectations unless iframe submission routing is actually implemented in the same task.

### UI Layer

Keep `internal/ui` responsible for controls, panels, and wiring, while moving durable state to `internal/profile`.

Changes:

- Browser construction accepts profile and network service dependencies.
- URL entry supports direct URL navigation and search queries through the configured search engine.
- Browser state syncs history/bookmark changes to the profile store.
- Settings dialog reads/writes persisted settings.
- Add private tab/window mode indicator and ensure it uses ephemeral profile/network state.
- Add keyboard shortcuts for navigation, reload, new tab, close tab, focus address bar, devtools panels, page source, and downloads.
- Add page source view backed by the fetched HTML for the active tab.

### Developer Tools

Extend existing console and inspector panels instead of creating an unrelated devtools surface.

Panels:

- Console: keep message filtering and add command entry that executes JavaScript in the active tab runtime.
- Inspector: keep DOM tree/details and add source view integration for selected nodes where available.
- Network: list requests, method, URL, status, type, timing, size, cache hit, and errors.
- Storage: show cookies, localStorage, sessionStorage, and cache entries for the active origin.
- Security: show current URL, scheme, TLS status, certificate subject/issuer, expiry, and errors.
- Downloads: show active and completed downloads with progress, cancel/open/retry controls where platform support allows.

### JavaScript Runtime Integration

The JavaScript runtime should remain Goja-based.

Changes:

- `localStorage` becomes origin-scoped and backed by the profile store in normal mode.
- `sessionStorage` remains tab/session-scoped and participates in session restore only if explicitly stored as part of tab state.
- JavaScript `fetch` uses the shared network service so cookies, cache, request logging, and private mode behavior are consistent.
- Runtime cleanup remains responsible for timers and tab-scoped resources.

### Test Strategy

Split tests into clear tiers:

- `go test ./... -short`: sandbox-safe unit tests that do not require listening sockets, external network, GUI launch, or Playwright.
- `go test ./...`: normal local tests, including `httptest` where the environment allows loopback listeners.
- `go test -tags=e2e ./test/e2e`: Playwright/browser-driven tests.
- `go test -tags=network ./internal/test_suite/...`: tests that intentionally require external network behavior.

Baseline form failures should be fixed as part of this milestone because forms are core browser behavior. Sandbox failures involving `httptest` listener permissions and Playwright launch permissions should be classified and skipped or tagged appropriately when the environment cannot support them.

### Release Pipeline

Add `.github/workflows/release.yml` for tag-based releases.

Behavior:

- Trigger on `v*` tags.
- Run unit tests.
- Build `cmd/browser` for supported Go/Fyne targets where CI dependencies are available.
- Upload archives for macOS, Linux, and Windows with checksums.
- Keep platform-specific dependency notes in `README.md`.

## Data Flow

Normal navigation:

1. User enters URL or search text.
2. UI resolves the input to a URL.
3. Browser state records pending navigation.
4. Network service fetches the main document with cookies, cache policy, request logging, and TLS capture.
5. Renderer receives HTML and current URL.
6. JavaScript runtime receives HTML, origin, storage adapter, and network service.
7. Profile stores history/session state unless the tab is private.
8. Devtools panels receive network, storage, console, and security updates.

Private navigation:

1. UI creates an ephemeral profile/network context.
2. Cookies, cache, localStorage, history, downloads metadata, and session records remain in memory.
3. Closing the private tab/window drops the ephemeral state.

Form submission:

1. Form state collects raw values.
2. Validator checks browser-supported constraints.
3. Submitter resolves action against the document URL.
4. Submitter marks the form in-flight.
5. Submit client sends the request through the network service or a test stub.
6. On completion, the in-flight state clears and success/error callbacks run.
7. Any UI display of submitted values escapes at the display boundary.

## Error Handling

- Profile load errors should fall back to defaults only when the file is missing or recoverable; corrupt files should be reported and backed up before replacement.
- Network errors should use typed categories: DNS, connection, timeout, TLS, HTTP status, cancellation, unsupported scheme, and sandbox/environment restrictions where detectable.
- Cache errors should never prevent navigation; they should produce devtools warnings and continue with network.
- Download failures should preserve a failed download record with the final error.
- Devtools panels should tolerate missing active tab/runtime/profile data and show empty states rather than panicking.
- Private mode should fail closed: if persistence cannot be disabled for a store, private mode should not use that store.

## Testing Requirements

- Profile stores have unit tests for load/save, atomic replacement, corrupt JSON backup, and private no-write behavior.
- Network service has unit tests with fake transports for cookies, cache hits/misses, request logging, TLS summary, downloads, and cancellation.
- Form tests cover duplicate-submit prevention, relative action resolution, fake submit clients, raw value preservation, and escaped display helpers.
- UI state tests cover address/search normalization, persistent bookmark/history sync, private mode isolation, and shortcut command dispatch.
- JavaScript runtime tests cover persistent localStorage by origin and shared network service usage for `fetch`.
- Release workflow is syntax-checked and documented.

## Documentation Requirements

- Update `README.md` with profile, private mode, downloads, security, devtools, and release usage.
- Update `ROADMAP.md` to mark completed foundation items and move modern standards features into the next milestone.
- Update `TESTING.md` with the split test tiers and commands.
- Add developer notes for profile file formats and network cache behavior.

## Milestone Boundaries

This milestone is complete when:

- Browser data persists across normal launches and is isolated in private mode.
- Cookies, HTTP cache, downloads, and TLS/security summaries are available through shared services and visible in UI/devtools.
- Forms submit correctly against relative and absolute actions without duplicate-submit bugs.
- Devtools expose console execution, network log, storage, page source, and security information.
- Local storage is origin-scoped and durable in normal mode.
- Sandbox-safe tests pass with a documented command.
- Release workflow exists and produces browser artifacts on tags.

Modern web standards work begins after this milestone and should have separate specs for Canvas/SVG/media, realtime APIs, workers/service workers, JavaScript modules, and accessibility.
