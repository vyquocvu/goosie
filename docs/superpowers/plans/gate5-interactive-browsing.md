# Gate 5: Interactive and Persistent Browsing Implementation Plan

**Goal:** Implement interactive browsing features (focus, forms, selection, find, zoom, Unicode/IME/accessibility) and persistent browsing (downloads, profiles, bookmarks, history, session restore, cookie/storage policy).

**Status:** Planning phase - October 8, 2026

## Roadmap Requirements

From `docs/roadmap-v2.md` section 5:

> **Gate 5**: Interactive and persistent browsing | Focus/forms/selection/find/zoom, Unicode/IME/accessibility; downloads; profiles/bookmarks/history/session restore; cookie/storage policy, clearing/private-mode tests

From section 6 (Deferred items):

| Deferred item | Current evidence | Why separate |
| --- | --- | --- |
| Links/forms/selection/accessibility/IME | `internal/raster/scheduler.go:246-317`; `internal/platform/darwin/shim_darwin.m:273-391` | Requires document event routing and native input contracts |
| Cookies, downloads, profiles and privacy UX | `internal/net/http.go:270-288,342-345`; `internal/tabs/tab.go:9-18` | New user-data contracts and lifecycle, not current rendering safeguards |

## Gate 5 Scope Analysis

### Interactive Features (UI/UX)

#### Already Implemented ✓
From test files found in `test/engine/`:
- ✅ focus_test.go - Focus state management
- ✅ selection_test.go - Text selection  
- ✅ keyboard_test.go - Keyboard input handling
- ✅ ime_test.go - IME support
- ✅ hittest_test.go - Hit testing for links
- ✅ hover_test.go - Hover cursor shaping
- ✅ control_events_test.go - Control events

#### Need Verification/Enhancement
- [ ] Link anchors navigation (`internal/raster/scheduler.go`)
- [ ] Find/search in page
- [ ] Zoom controls
- [ ] Mouse pointer actions

### Persistent Browsing (Data Management)

#### Already Implemented ✓
From `internal/` packages:
- ✅ `internal/toolbar/history.go` - History store
- ✅ `internal/bookmarks/store.go` - Bookmarks
- ✅ `internal/session/store.go` - Session management
- ✅ `internal/net/http.go:270-288,342-345` - Cookie storage
- ✅ `internal/download/` - Download handling

#### Need Integration
- [ ] Cookie persistence across sessions
- [ ] Profile switching
- [ ] Session restore on startup
- [ ] Bookmark UI integration
- [ ] History sidebar/panel

## Task Breakdown

### Task 1: Verify & Test Interactive Features

**Files:** Existing tests + new integration tests

**Steps:**
- [ ] Run existing focus/selection tests: `go test ./test/engine -run "TestFocus|TestSelection" -v`
- [ ] Verify hit testing works for links
- [ ] Test hover cursor shapes
- [ ] Check IME input flow
- [ ] Test keyboard shortcuts

**Acceptance Criteria:** All interactive tests pass without race conditions

### Task 2: Anchor Navigation & Link Clicks

**Files:** `internal/raster/scheduler.go`, `cmd/goosie/main.go`

**Steps:**
- [ ] Add URL fragment/hash parsing
- [ ] Scroll to anchor target element
- [ ] Handle link click callbacks from content area
- [ ] Update toolbar URL when clicking links
- [ ] History entry creation for link navigations

**Acceptance:** User can navigate via <a href="#section"> links

### Task 3: Find-in-Page Implementation

**Files:** `internal/toolbar/toolbar.go`, `internal/engine/engine.go`

**Steps:**
- [ ] Extend toolbar.findActive state
- [ ] Add regex/text search over document textContent
- [ ] Highlight matches (yellow background)
- [ ] Next/previous match navigation
- [ ] Match counter display ("3/12")
- [ ] Escape to close

**Acceptance:** Ctrl+F opens find bar, highlights work correctly

### Task 4: Zoom Controls

**Files:** `cmd/goosie/main.go`, `internal/tabs/tab.go`

**Steps:**
- [ ] Store zoom level per-tab (`tab.zoom float64`)
- [ ] Add zoom callback handler
- [ ] Apply zoom to viewport DPR multiplication
- [ ] Reset zoom button (+/-/100%)
- [ ] Persist zoom per-site

**Acceptance:** Ctrl++/Ctrl-- adjust zoom, viewport scales correctly

### Task 5: Cookie Policy Implementation

**Files:** `internal/net/http.go`, `internal/bookmarks/cookie_store.go`

**Steps:**
- [ ] Implement cookie jar serialization
- [ ] Load/save cookies on profile switch
- [ ] Respect Secure flag enforcement
- [ ] HttpOnly cookies inaccessible to JS (future gate)
- [ ] Partition cookies by origin
- [ ] Private mode: discard cookies on shutdown

**Acceptance:** Cookies persist across navigations, deleted in private mode

### Task 6: Downloads Manager

**Files:** `internal/download/`, `cmd/goosie/main.go`

**Steps:**
- [ ] Download queue manager
- [ ] Progress bar for active downloads
- [ ] Download completion notifications
- [ ] File collision handling (overwrite/rename)
- [ ] Download destination configuration
- [ ] Cancel in-progress downloads

**Acceptance:** User can download files, see progress, cancel operations

### Task 7: Profile System

**Files:** `internal/profile/`, `cmd/goosie/main.go`

**Steps:**
- [ ] Define profile structure (name, path, settings)
- [ ] Profile selector UI in toolbar or menu
- [ ] Switch profile clears all state
- [ ] Default profile as fallback
- [ ] Profile migration on version change

**Acceptance:** Can create multiple profiles, switch between them seamlessly

### Task 8: Session Restore

**Files:** `internal/session/store.go`, `cmd/goosie/main.go`

**Steps:**
- [ ] Save tab URLs on window close
- [ ] Restore tabs on startup
- [ ] Preserve scroll position per tab
- [ ] Reconstruct history stack
- [ ] Resume loading failed pages

**Acceptance:** Closing and reopening browser restores all tabs

### Task 9: Accessibility Support

**Files:** `internal/ax/`, `internal/raster/scheduler.go`

**Steps:**
- [ ] Build accessibility tree from DOM
- [ ] Expose role, name, value properties
- [ ] Notify screen readers on content changes
- [ ] Keyboard navigation for elements
- [ ] Alt text for images

**Acceptance:** VoiceOver/NVDA can read page content

## Priority Assessment

### High Priority (Core functionality)
1. Link clicks & anchor navigation - Essential browsing
2. Cookie persistence - Required for login states
3. Session restore - Prevents data loss
4. Selection & copy - Basic productivity

### Medium Priority (Enhanced UX)
5. Find-in-page - Essential for long documents
6. Zoom controls - Accessibility requirement
7. Downloads - Expected browser feature

### Lower Priority (Nice to have)
8. Multiple profiles - Advanced users
9. Full accessibility tree - Compliance requirement

## Dependencies

- Gate 1-4 must be complete (✅ DONE)
- Gate 6 (JS runtime) can proceed in parallel for some features
- No blocking dependencies on other gates

## Implementation Order Suggestion

Phase 1: Core Interactivity (Week 1-2)
- Task 1: Test verification
- Task 2: Links & anchors
- Task 3: Find-in-page

Phase 2: Persistence (Week 2-3)
- Task 5: Cookie policy
- Task 6: Downloads
- Task 8: Session restore

Phase 3: Advanced Features (Week 3-4)
- Task 4: Zoom controls
- Task 7: Profiles
- Task 9: Accessibility

Total Estimate: 3-4 weeks with focused development

## Risk Assessment

- **Low risk:** Most infrastructure exists, just needs wiring
- **Medium risk:** Accessibility tree building is complex
- **Known limitation:** True multi-profile requires file system access

## Acceptance Criteria Summary

1. **Links:** Can navigate via anchor links
2. **Forms:** Can enter text (basic fields work)
3. **Selection:** Can select/copy text
4. **Find:** Ctrl+F finds text on page
5. **Zoom:** Ctrl± adjusts page scale
6. **Cookies:** Persist across sessions, private-mode aware
7. **Downloads:** Files download successfully
8. **Profiles:** Can switch between data sets
9. **Session:** Tabs restored after restart
10. **Accessibility:** Screen readers can access content

## Testing Strategy

- Unit tests for each component
- Integration tests for full workflows
- Manual testing with real websites
- Automated regression suite
- Race detector under load
- Cross-platform consistency checks

## Next Steps

Ready to begin implementation starting with Task 1 (test verification) and Task 2 (link navigation).
