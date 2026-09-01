# internal/js — Agent Constraints

## Single Owner Goroutine

Exactly one goroutine owns the `Session`/`Runtime`. Only that goroutine may call
`Runtime` methods directly. Others MUST use `Submit()` / `SubmitAndWait()`.
Calling these from the owner goroutine WILL deadlock.

## Goja VM Is Not Thread-Safe

All script execution serialized through `scriptMu`. Never access Goja objects
from a non-owner goroutine without holding `scriptMu`.

## DOM Bridge Ownership

DOM API lives in JS land (polyfills via `setupDocumentAPI`). Mutations signal Go
via `window.__onDOMChanged`. Do NOT add direct Go-to-DOM bindings — all state
changes must flow through the mutation callback and `EventLoop` batching.

## Event Loop & Queues

HTML-spec order enforced: macrotask → drain microtasks → fire timers → flush
mutations. Never reorder. RAF callbacks registered during `Tick` defer to next frame.
Queues are bounded (task 256, microtask 512, timer 128). Async completions MUST
enqueue via `enqueueTask` — never execute inline on a worker goroutine.

## Race Detector

Mandatory: `go test -race -short ./internal/js/...`
