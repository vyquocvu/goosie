# internal/profile — Agent Constraints & Architecture

## Core Responsibilities

The `internal/profile` package manages persistent and private user profile directories, settings, bookmarks, navigation history, session storage, and SQLite database connections for Goosie.

## Persistent vs Private Profiles

- **Persistent Profile**: Initialized via `Open(Options{Root: path, Private: false})`. Creates user data directories, manages SQLite database stores, and flushes settings/history to disk asynchronously.
- **Private Browsing Profile**: Initialized via `Open(Options{Private: true})`. Runs entirely in-memory using RAM snapshots. No cookies, history entries, cached files, or session state are ever written to disk.

## Asynchronous Write Queue & Snapshots

- Disk writes are queued through `writeChan chan writeTask` and processed by a background worker goroutine to avoid blocking UI or navigation flows.
- In-memory file snapshots (`snapshots map[string][]byte`, guarded by `snapshotsMu`) provide instant, non-blocking reads.
- Atomic file writing ensures configuration files are never corrupted by unexpected process termination.

## SQLite Stores & Schema Migrations

- History, cookies, and permissions use SQLite stores with explicit schema versioning (`CurrentSchemaVersion = 1`).
- Migrations run idempotently upon database opening within single transactions.
- Connection locks (`fileLocks map[string]*sync.Mutex`) serialize concurrent SQLite writes across goroutines.

## Lifecycle & Resource Cleanup

- Always call `Profile.Close()` during browser shutdown.
- `Close()` waits for all pending background writes to complete (`wg.Wait()`), flushes WAL caches, and closes database handles cleanly.

## Testing & Verification

All profile subsystem tests reside in `test/internal/profile/...`.

Run the profile test suite with the race detector:
```bash
go test -race -short ./test/internal/profile/...
```
