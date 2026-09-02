# internal/dom — Agent Constraints & Architecture

## Core Responsibilities

The `internal/dom` package provides the memory-compact Document Object Model (DOM) store, incremental HTML parser, tree builder, and mutation notification pipeline for Goosie.

## Compact DOM Store Architecture (`Store`)

- The compact store replaces pointer-heavy node structures with contiguous index-based storage in `Store`.
- Nodes are stored in a contiguous `[]nodeRecord` slice (32 bytes per node record), maintaining cache locality and minimizing GC scanning overhead.
- Node relationships use first-child / next-sibling / prev-sibling / parent `NodeID` links.
- Attributes are stored in a packed `[]Attr` slice indexed by `AttrStart` and `AttrCount` per node.
- Text content is stored in a dedicated `textData []byte` buffer.

## Stable `NodeID` Handles

- `NodeID` (uint32) is a lightweight index handle to a node in the store. `NodeNone = 0` denotes a nil/invalid node.
- Stale handle detection: A freed node sets its `Kind` field to 0 (the zero value, left unnamed by the kind constants, which start at `NodeKindElement = iota + 1`). Accessing a freed handle returns an error or empty record.

## Atom Interning (`internal/dom/atom`)

- Tag names and common HTML attributes are interned as `atom.Atom` (uint16) identifiers.
- Pre-computed constants (`atom.Div`, `atom.A`, `atom.Span`, `atom.Class`, `atom.Id`, `atom.Href`, `atom.Src`, `atom.Style`, etc.) enable fast integer comparisons instead of string equality checks throughout parsing and selector matching.

## Streaming Parser & Tree Builder

- `Parser` (`parser.go`) and `ParseStream` / `treeBuilder` (`treebuilder.go`) implement HTML5 compliant tokenization and tree construction.
- Supports incremental parsing of incoming network chunks without buffering entire document bodies.
- Maintains the HTML5 insertion mode state machine, open element stack, and active formatting elements list.

## Tree Traversal & Iterators

- `store_traverse.go` provides zero-allocation traversal iterators:
  - `Children(parent NodeID)`
  - `Descendants(root NodeID)`
  - `Ancestors(node NodeID)`
  - `Siblings(node NodeID)`
- Traversal loops must use iterator methods rather than manual slice copying to maintain zero allocations.

## Concurrency & Mutation Notification

- `Store` access is thread-safe, guarded internally by a `sync.RWMutex`.
- DOM mutations record `Mutation` structs (`mutation.go`) carrying `NodeID`, mutation type (`MutationInsert`, `MutationRemove`, `MutationReplace`, `MutationSetText`, `MutationSetAttribute`), and attribute keys for downstream synchronization with JavaScript runtime and renderer.

## Testing & Verification

All DOM and atom package tests reside in `test/internal/dom/...`.

Run the full DOM test suite:
```bash
go test -race -short ./test/internal/dom/...
go test ./test/internal/dom/atom/...
```
