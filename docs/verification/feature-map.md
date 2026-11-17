# Goosie feature map

The rubric grades observable behavior, not the existence of an API. Fixtures use ephemeral loopback origins and disposable profiles; never use a personal account or the normal browser profile. The reference platform is Goosie's real headless window/event loop. Chromium supplies independent rendering/event references elsewhere; it does not drive Goosie.

## Required fast journeys

| User feature | Route / target | Actions | Required result | Test |
| --- | --- | --- | --- | --- |
| Open, follow, return, reload, submit | Local `/start`, `/destination`; `#next`, `#name`, `#send`, `#result` | Address bar → Enter; click link; Back → Forward → Back → Reload; type Ada; submit | Correct URL/document at every step, expected request counts, `Hello Ada`, canceled form causes no navigation, a new frame is presented | `TestE2ECoreBrowserJourney` in `cmd/goosie/e2e_test.go` |
| Configure search and download destination | `goosie://settings`, `#search-url`, `#download-directory`; local `/search`, `/download` | Set profile search template and directory; save; search; download | Encoded query reaches local server; selected directory contains the completed download; settings read back correctly from the profile file | `TestE2ESearchEngineSettingsJourney` in `cmd/goosie/e2e_test.go` |
| Choose files and cancel | Local `/picker`, `#upload`, `#out`, `#count` | Choose two files; reopen and cancel | Script receives exact names, sizes and MIME types once; cancellation preserves selection and event count | `TestE2EFilePickerSelectionReachesScript` in `cmd/goosie/file_picker_e2e_test.go` |
| Find content, move, dismiss, miss | Local document with two `needle` matches | Cmd+F; type needle; Enter; Escape; search absent text | Two matches, next match scrolls into view, Escape closes find, zero matches do not destroy or navigate the document | `TestE2EFindJourney` in `cmd/goosie/verification_e2e_test.go` |
| Keep documents separate across tabs | Local pages A and B | Load A; Cmd+T; load B; Cmd+W | Two tabs become one; original A URL and content survive; B does not overwrite A | `TestE2ETabIsolationJourney` in `cmd/goosie/verification_e2e_test.go` |

Run with `go test -mod=readonly -count=1 -timeout=90s -v -run '^TestE2E' ./cmd/goosie`.

Each state wait names its step and emits `STEP PASS`; failure logs include the active URL, loading/error state, history, presentation counters, and accessibility snapshot. Preserve the full output, not its last line. Stability means five fresh successful executions with no retries after a failure; a failing run stops the experiment for diagnosis.

The file-selection test substitutes the operating-system picker; it proves dispatch into the page, not NSSavePanel/NSOpenPanel behavior. Headless tests cannot certify native macOS keyboard/IME, display pacing, dialogs, or accessibility integration.

## Rendering smoke

`scripts/verify.py` builds a new binary and serves a 320×200 document. `#static` must render red, `#script` must become green through JavaScript, and the background must stay white. It checks exact pixels, records the binary SHA-256, retains `document.png`, step logs and a JSON report, and shuts down all resources. An all-white screenshot or missing journey is failure, not a skip.

## Additional diff-triggered coverage

| Feature / boundary | Existing coverage to consult | What still needs separate evidence |
| --- | --- | --- |
| Private windows, storage, history and session restore | `cmd/goosie` privacy/session journeys; `test/js`, `test/session`, `test/history` | Real restart and native private-window UX; no normal-profile leakage |
| Zoom, selection, clipboard, IME | `cmd/goosie/find_and_zoom_test.go`, `cmd/goosie/journey_ime_visual_test.go`, engine interaction suites | Native composition and clipboard; DPR/zoom pixel parity |
| Streaming downloads, pause/resume/cancel | `internal/download`, `test/net`, browser download journeys | File bytes/digest, collision safety, temp-file cleanup, native picker |
| HTML/CSS/JS compatibility | `testdata/render`, `testdata/interaction-parity`, engine geometry/event oracles | Fresh Chromium references and scoped approved differences; smoke is not conformance |
| Resource limits and render lifecycle | `test/entrypoints`, `internal/archtest`, `test/gate` | Admission before allocation; no races, stale navigation results, or worker leaks |

## Grading

A journey passes only when every required state and final result is observed. Missing execution, skipped tests, process failure, timeout, stale artifacts, wrong pixels, or failed cleanup means no passing verdict. Review findings use Important / Nit / Pre-existing, with command output or a reproducer attached. Native-only or unsupported behavior is explicitly unverified, never inferred from a headless pass.
