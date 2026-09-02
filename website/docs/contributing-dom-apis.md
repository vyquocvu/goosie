# Adding a DOM API

This guide explains how to add a new DOM API (method or property) to the Goosie engine.

## Architecture Overview

DOM APIs in Goosie are implemented across two cooperating layers:

1. **Core implementation** (`internal/dom`): The compact DOM store and HTML tree manipulation functions operate on `NodeID` handles and interned string atoms.
2. **JavaScript runtime bridge** (`internal/js`): Polyfilled JavaScript DOM classes (`Node`, `Element`, `Document`, `Attr`, `NamedNodeMap`, `DOMTokenList`, `CSSStyleDeclaration`) configured via `setupDocumentAPI` in `internal/js/runtime.go` and `polyfills.go`. Tree population from Go to JS is handled by `populateJSNode`, and mutations in JS land trigger Go callbacks via `window.__onDOMChanged`.

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

Add unit tests in `test/internal/dom/store_test.go` covering:
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

1. **Add the method/property to the DOM polyfills** in `internal/js/runtime.go` (`setupDocumentAPI` or `polyfills.go`):

```javascript
// Inside setupDocumentAPI() JS polyfill definition:
Element.prototype.yourMethod = function() {
    // Perform JS-level DOM manipulation
    // Notify Go engine if DOM structure or attributes changed:
    if (typeof window.__onDOMChanged === 'function') {
        window.__onDOMChanged(nodeId(this), 'yourMethod');
    }
};
```

2. **Or register Go-backed bridge functions** in `internal/js/runtime.go`:

```go
vm.Set("__goosie_yourMethod", func(call goja.FunctionCall) goja.Value {
    // Extract arguments, validate permissions, and call dom.Store
    return goja.Undefined()
})
```

3. **Gate behind capability** if the API accesses network, storage, or other privileged functionality via `internal/js/policy.go`:

```go
if !r.policy.HasCapability(CapabilityStorage) {
    panic(r.vm.NewTypeError("permission denied"))
}
```

## Step 4: Add to Supported Platform Doc

Update `supported-web-platform.md` with the new API, its status, and any documented subset.

## Step 5: Integration Test

Add a JS integration test in `test/internal/js/` (such as `runtime_test.go` or `dom_test.go`) or `test/internal/test_suite/webapi/` that exercises the new API end-to-end:

```go
func TestYourMethodJS(t *testing.T) {
    rt := NewRuntime()
    val, err := rt.RunScript(`document.createElement("div").yourMethod()`)
    // assert results
}
```

## Step 6: Mark as Supported

Update `supported-web-platform.md` when the change affects advertised API coverage.

## Checklist

- [ ] Core implementation added to `internal/dom/store.go`
- [ ] Tests for normal, edge, and error cases in `test/internal/dom/`
- [ ] Benchmarks for performance-sensitive paths
- [ ] JavaScript binding/polyfill added in `internal/js/runtime.go` (if JS-accessible)
- [ ] Capability gate added in `internal/js/policy.go` (if privileged)
- [ ] Updated `supported-web-platform.md`
- [ ] Integration test added in `test/internal/js/` or `test/internal/test_suite/`
- [ ] Ran `go test ./test/internal/dom/... && go test -short ./test/internal/js/...`
