# ADR 0001: Use a Compact, Index-Based DOM Store

**Status:** Accepted (implemented in Milestone 2)

**Decision Date:** 2024-Q3 (M2 milestone)

**Deciders:** Goosie engine team

---

## Context

The DOM tree is the central data structure of the browser engine. Every other subsystem (style, layout, paint, JavaScript) reads or walks it. The initial implementation used Go's standard `golang.org/x/net/html` parser, which returns a tree of `*html.Node` pointer structs. Profiling revealed two problems:

1. **Pointer density causes GC pressure.** Each `*html.Node` is a heap-allocated struct containing interface fields, string values, and child-slice headers. On documents with tens of thousands of nodes, the garbage collector spends significant time scanning the pointer graph.
2. **Per-node allocation overhead.** A single `*html.Node` occupies ~200 bytes (struct + string backing + slice headers) for what is semantically ~40 bytes of data. This doubles heap usage for large documents.
3. **Child slices fragment the heap.** Each node's `Children []*html.Node` is a separately allocated slice header + backing array, creating many small allocations.

Early measurements on fixture pages showed that DOM build accounted for 15-25% of total navigation time and 30-40% of young-gen allocations. Reducing this was prerequisite to the v2 performance targets.

### Constraints

- Must support incremental (streaming) tree construction — the parser should not need the complete document in memory before producing nodes.
- Must support standard DOM mutation operations: append, insert, remove, replace child and attribute changes.
- Must provide fast traversal (parent, first-child, next-sibling, ancestors, subtree) without allocating intermediate slices.
- The representation must be safe for concurrent read access (single writer, multiple readers).
- Must integrate with existing code that expects `*html.Node` pointers, via a temporary compatibility adapter (M2.5, removed in M5.4).

---

## Decision

Replace pointer-heavy `*html.Node` trees with a compact, index-based `Store` that holds nodes in contiguous slices and identifies them via stable integer `NodeID` handles.

### Data Model

```go
type NodeID uint32

const NodeNone NodeID = 0

type nodeRecord struct {
    parent      NodeID
    firstChild  NodeID
    nextSibling NodeID
    prevSibling NodeID
    name        uint32    // AtomID for elements, text content ID for text nodes
    attrStart   uint32    // offset into the packed attribute slice
    attrCount   uint16
    kind        NodeKind
    flags       NodeFlags
}

type NodeKind uint8    // Element, Text, Comment, Document, Doctype
type NodeFlags uint8   // Dirty, HasRareData
```

Key design choices:

- **32-bit NodeID.** A `uint32` index into the nodes slice supports up to ~4 billion nodes per document, more than sufficient for any realistic document. Using `int` (64-bit on most platforms) would double the size of every cross-reference in the struct for zero benefit.
- **Index 0 is the null handle (`NodeNone`).** This is a common C/Go convention that avoids a separate option type, simplifies zero-initialization safety, and naturally reserves the first slot. The store pre-allocates `nodes[0]` as a sentinel.
- **Linked-list child structure** (first-child + next-sibling + prev-sibling) instead of a child slice. This avoids per-node slice-header allocations for children, at the cost of O(n) child counting.
- **Packed attributes.** Attribute name/value pairs are stored in a separate flat slice (`attrRecords`), with each node record holding an offset and count. This avoids per-node `[]Attr` slice allocations.
- **Rare data offload.** Properties that are uncommon (e.g., custom data attributes, inline event handlers) are stored in a secondary `map[NodeID]rareData` structure. The `HasRareData` flag avoids a map lookup on the hot path for nodes without rare data.
- **Interned strings via AtomID.** Tag names, attribute names, and common class/ID values are interned to `AtomID` values. This eliminates string duplication across nodes and speeds up comparisons (integer equality vs. string equality). Document text content is stored in a compact text store rather than per-node heap strings.

### Traversal

Traversal is zero-allocation: iterators hold a `NodeID` cursor and advance via pointer-free field reads. No intermediate `[]NodeID` slices are allocated.

```go
type ChildIterator struct {
    store *Store
    pos   NodeID
    end   NodeID
}
```

Six iterator types are provided: `ChildIterator`, `ReverseChildIterator`, `SubtreeIterator`, `AncestorIterator`, `SiblingIterator`, `ReverseSiblingIterator`.

### Mutation Safety

A generation counter (`Store.gen`) is incremented on every structural mutation. `NodeID` values embed the generation in a separate `gen` field of the handle; stale handles are detected on access and cause a predictable panic rather than silent corruption. Removal sets a node's `kind` to 0 (the zero value), making it ineligible for traversal while preserving its children's parent references until the next compaction.

### Compatibility Adapter (temporary)

A thin adapter layer wraps the `Store` and exposes `*html.Node`-compatible interfaces for subsystems that had not yet migrated to `NodeID`-native APIs. The adapter tracked access metrics and was removed in M5.4 when all consumers had been migrated.

---

## Consequences

### Positive

- **49% fewer allocations on large HTML fixtures**, 67% on table-heavy, 45% on form-heavy — exceeding the 30% target.
- **32-byte node records** vs. ~200 bytes for `*html.Node` (5× density improvement).
- **Zero-allocation traversal.** DOM walks that formerly allocated slices now run with 0 allocs/op.
- **Faster parsing.** Streaming tree construction writes directly into the flat store without intermediate `*html.Node` allocation.
- **Improved GC behavior.** Fewer pointers on the heap means shorter scan pauses and less GC work per navigation.
- **Natural generation-based staleness detection** prevents use-after-free on removed nodes.

### Negative

- **Linked-list child structure makes child-counting O(n).** Applications that need child counts (e.g., `childElementCount`) must traverse. Mitigated by batch operations that cache counts.
- **NodeID handles require explicit store reference** for dereference. Code must carry a `*Store` pointer alongside `NodeID` values — slightly more verbose than passing `*html.Node`.
- **Mutation requires locking.** The store uses a `sync.RWMutex`; concurrent readers are allowed, but writers hold an exclusive lock. This is acceptable for the single-owner threading model (M8).
- **Node removal is lazy** (kind = 0) rather than immediate compaction. Compaction must be triggered explicitly, typically during navigation boundaries.

---

## Alternatives Considered

### 1. Keep `*html.Node` with pooled allocation
Using `sync.Pool` to recycle `*html.Node` structs was considered. This would reduce allocation rate but not pointer density or GC scan time. Rejected because GC pressure from pointer-rich trees was the primary bottleneck.

### 2. Child arrays via `[]NodeID` slice
Using a flat `[]NodeID` child array attached to each parent was considered. This makes random-access child lookup O(1) and supports `childElementCount` without traversal. Rejected because each child slice is a separate heap allocation, counteracting the compaction benefit.

### 3. Arena allocator for `*html.Node`
Allocating `*html.Node` values from a linear arena (via `arena.Arena` or a custom bump allocator) was prototyped. This reduces allocs but keeps the pointer-heavy struct layout and interface fields. Rejected because arena-go has experimental status and the struct layout improvements from index-based design are independent of allocation strategy.

### 4. 64-bit NodeID
Using `uint64` for future-proofing was discussed. Rejected because 4 billion nodes per document is far beyond any realistic fixture, and 64-bit IDs would double the cross-reference width in every node record and iterator.

### 5. Persistent functional data structure
An immutable, persistent tree (like Clojure's vector-based tree) was considered for snapshot-based concurrency. Rejected because mutation patterns in DOM APIs are overwhelmingly in-place, and copy-on-write would impose unpredictable overhead on every `appendChild`.

---

## Performance Evidence

Measured on the benchmark corpus (locked baseline from M0):

| Fixture | Alloc reduction | Time improvement |
|---|---|---|
| Large HTML (12k nodes) | 49% | 22% |
| Table-heavy | 67% | 35% |
| Form-heavy | 45% | 18% |
| Long article | 52% | 27% |

Traversal benchmarks show 0 allocs/op for all iterator types. Mutation benchmarks show O(1) append with ~24 bytes/node additional allocation (attr storage amortized).

---

## Related

- `website/docs/memory-model.md`
- `internal/dom/store.go` — canonical implementation
- `internal/dom/store_traverse.go` — iterator implementations
- `internal/dom/api_inventory.go` — migration plan from pointer-based APIs
- ADR 0002: Retained Display List Design (display list uses the same compaction principles)
