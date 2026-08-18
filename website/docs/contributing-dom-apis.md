# Adding a DOM API

This guide explains how to add a new DOM API (method or property) to the Goosie engine.

## Architecture Overview

DOM APIs are implemented in two layers:

1. **Core implementation** (`internal/dom`): The DOM store and tree manipulation functions operate on `NodeID` handles.
2. **JavaScript binding** (`internal/js`): Lazy JavaScript wrappers (`NodeHandle`, `DocumentHandle`) expose core functions to the Goja JS runtime.

## Step 1: Add the Core Function

Add the implementation to `internal/dom/store.go` (or a new file in `internal/dom/`):

```go
// YourMethod performs ... on the given node.
// Returns an error if the node is stale or the operation is invalid.
func (s *Store) YourMethod(node NodeID) error {
    if s == nil {
        return ErrNilStore
    }
    if err := s.validateNode(node); err != nil {
        return err
    }
    // implementation
    return nil
}
```

**Conventions:**
- Take `NodeID` parameters (not `*html.Node`).
- Return errors for invalid states (stale node, wrong node kind, etc.).
- Use `s.validateNode(node)` for staleness checks.
- Update the generation counter (`s.gen++`) for mutating operations.
- Document the supported HTML element subset in the function comment.

## Step 2: Add Tests

Add tests in `internal/dom/store_test.go` covering:
- Normal case
- Edge case (empty store, nil receiver)
- Stale node handle
- Concurrent access (if applicable)

```go
func TestYourMethod(t *testing.T) {
    s := New()
    node := s.CreateElement("div")
    err := s.YourMethod(node)
    // assert
}
```

## Step 3: Add JavaScript Binding (if JS-accessible)

If the API should be callable from JavaScript:

1. **Add a handle method** in `internal/js/dom_handle.go`:

```go
func (h *NodeHandle) yourMethod(call goja.FunctionCall) goja.Value {
    if h.stale {
        panic(h.vm.NewTypeError("node is stale"))
    }
    err := h.store.YourMethod(h.id)
    if err != nil {
        panic(h.vm.NewTypeError(err.Error()))
    }
    return goja.Undefined()
}
```

2. **Register the method** in the `NodeHandle` constructor or prototype setup:

```go
proto.Set("yourMethod", h.vm.ToValue(h.yourMethod))
```

3. **Gate behind capability** if the API accesses network, storage, or other privileged functionality:

```go
if !h.enforcer.HasCapability(js.CapabilityYourAPI) {
    panic(h.vm.NewTypeError("permission denied"))
}
```

## Step 4: Add to Supported Platform Doc

Update `supported-web-platform.md` with the new API, its status, and any documented subset.

## Step 5: Integration Test

Add a JS integration test in `internal/js/dom_handle_test.go` or `internal/test_suite/webapi/` that exercises the new API end-to-end:

```go
func TestYourMethodJS(t *testing.T) {
    vm := goja.New()
    // set up DOM, run JS, assert results
}
```

## Step 6: Mark as Supported

Update `supported-web-platform.md` when the change affects advertised API coverage.

## Checklist

- [ ] Core implementation added to `internal/dom/store.go`
- [ ] Tests for normal, edge, and error cases
- [ ] Benchmarks for performance-sensitive paths
- [ ] JavaScript binding added (if JS-accessible)
- [ ] Capability gate added (if privileged)
- [ ] Updated `supported-web-platform.md`
- [ ] Integration test added
- [ ] Ran `go test -short ./internal/dom/... ./internal/js/...`
- [ ] Ran `go test -race -short ./internal/dom/... ./internal/js/...`
