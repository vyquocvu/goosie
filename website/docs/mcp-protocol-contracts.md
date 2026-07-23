# MCP Tool and Resource Contracts

**Status:** Proposed contract; schemas must be golden-tested before handlers exist.

**Protocol baseline:** MCP `2025-11-25`

## 1. Contract rules

- Tool names use lowercase ASCII plus underscores and stay stable after v1.
- Every tool input sets `additionalProperties: false` unless extensibility is intentional.
- Every string/array/object has a documented maximum.
- Every result returns `contextId` where applicable and `pageRevision` when page identity matters.
- Mutation tools are explicit and narrow. There is no generic “browser command” tool.
- Tool results provide `structuredContent`; concise text content is duplicated only for broad client compatibility.
- Binary screenshots use MCP image content. Large diagnostics use resources, not huge tool text.
- Pagination uses opaque cursors; clients cannot infer internal indices.

## 2. v1 tool catalog

| Tool | Class | Phase | Purpose |
|---|---|---:|---|
| `browser_context_create` | state | 3 | Create a private ephemeral context. |
| `browser_context_list` | read | 3 | List contexts owned by this MCP connection. |
| `browser_context_close` | state | 3 | Idempotently close one context. |
| `browser_navigate` | network mutation | 3 | Navigate and optionally wait for readiness. |
| `browser_wait` | read/wait | 3 | Wait for lifecycle, URL, text, or revision condition. |
| `browser_snapshot` | read | 3 | Return bounded semantic page state and refs. |
| `browser_screenshot` | expensive read | 3 | Return bounded viewport PNG. |
| `browser_page_info` | read | 3 | URL, title, lifecycle, revision, viewport, truncation flags. |
| `browser_console_read` | sensitive read | 3 | Read bounded console entries. |
| `browser_network_read` | sensitive read | 3 | Read redacted request/response metadata. |
| `browser_security_read` | read | 3 | Read TLS/CSP/policy summary. |
| `browser_query` | read | 4 | Resolve a semantic/CSS/text locator to refs. |
| `browser_click` | page mutation | 4 | Activate one current element ref. |
| `browser_type` | sensitive mutation | 4 | Type text into one editable element. |
| `browser_press_key` | page mutation | 4 | Dispatch a supported key chord. |
| `browser_scroll` | page mutation | 4 | Scroll viewport or referenced element. |
| `browser_set_viewport` | state mutation | 4 | Set bounded viewport and scale. |
| `browser_evaluate` | high-risk mutation/read | 5 | Run bounded JavaScript in page context. |

## 3. Common fields

```json
{
  "contextId": "ctx_opaque",
  "timeoutMs": 10000
}
```

- `contextId`: required except create/list; maximum 128 bytes; opaque.
- `timeoutMs`: optional, 1–30000; server default 10000 and hard cap 30000.
- `pageRevision`: unsigned monotonic value returned by the server.
- `correlationId`: returned on errors and logged server-side; opaque.

## 4. Core contract examples

### `browser_context_create`

Input:

```json
{
  "viewport": {"width": 1280, "height": 720, "scale": 1},
  "javascriptEnabled": true
}
```

Limits: width 320–4096, height 200–4096, scale 0.5–3. Default contexts are private, in-memory/temporary, and have no host filesystem access.

Output:

```json
{
  "contextId": "ctx_opaque",
  "state": "created",
  "pageRevision": 0,
  "viewport": {"width": 1280, "height": 720, "scale": 1}
}
```

### `browser_navigate`

Input:

```json
{
  "contextId": "ctx_opaque",
  "url": "https://example.com",
  "waitUntil": "complete",
  "timeoutMs": 15000
}
```

`waitUntil` is one of `commit`, `interactive`, or `complete`. URL length maximum is 8192 bytes. Allowed schemes are policy-controlled; v1 defaults to `https` and `http`. Userinfo in URLs is rejected. Fragments are allowed but do not create a new revision unless a document navigation occurs.

Output:

```json
{
  "contextId": "ctx_opaque",
  "navigationId": "nav_42",
  "url": "https://example.com/",
  "state": "complete",
  "waitConditionMet": true,
  "pageRevision": 1,
  "httpStatus": 200
}
```

### `browser_snapshot`

Input:

```json
{
  "contextId": "ctx_opaque",
  "format": "semantic",
  "maxDepth": 20,
  "maxNodes": 2000,
  "includeHidden": false
}
```

Hard caps: depth 50, nodes 5000, output 1 MiB. Raw HTML is not a format for this tool.

Output excerpt:

```json
{
  "contextId": "ctx_opaque",
  "pageRevision": 1,
  "url": "https://example.com/",
  "title": "Example Domain",
  "nodes": [
    {"role": "heading", "name": "Example Domain", "level": 1},
    {"role": "link", "name": "More information", "ref": "e_1_opaque"}
  ],
  "truncated": false
}
```

### `browser_click`

Input:

```json
{
  "contextId": "ctx_opaque",
  "ref": "e_1_opaque",
  "button": "left",
  "timeoutMs": 10000
}
```

Output:

```json
{
  "contextId": "ctx_opaque",
  "pageRevision": 1,
  "actionApplied": true,
  "navigationStarted": false
}
```

The tool rejects refs from another context/revision and does not silently fall back to coordinates.

### `browser_type`

Input:

```json
{
  "contextId": "ctx_opaque",
  "ref": "e_1_opaque",
  "text": "hello",
  "replace": true,
  "submit": false
}
```

Text maximum is 64 KiB but the operator may lower it. Text is classified sensitive and excluded from logs. `submit` is explicit; typing never submits implicitly.

### `browser_screenshot`

Input:

```json
{
  "contextId": "ctx_opaque",
  "scope": "viewport",
  "omitBackground": false
}
```

Only `viewport` is supported initially. Hard caps: 16 megapixels raw and 8 MiB encoded. Output contains image content with MIME `image/png`, plus structured width, height, page revision, and truncation fields.

### `browser_evaluate`

Input:

```json
{
  "contextId": "ctx_opaque",
  "source": "document.title",
  "awaitPromise": true,
  "timeoutMs": 1000,
  "maxResultBytes": 65536
}
```

Hard caps: source 256 KiB, duration 5 seconds, serialized result 1 MiB, depth 20. v1 does not expose host functions, filesystem, subprocesses, or unrestricted network primitives. Results are tagged (`null`, `boolean`, `number`, `string`, `array`, `object`, `undefined`) and cycles/unsupported values return a typed serialization error.

## 5. Locator contract

`browser_query` accepts exactly one locator kind:

```json
{"role": {"name": "button", "accessibleName": "Submit", "exact": true}}
```

```json
{"css": "form#login button[type=submit]"}
```

```json
{"text": {"value": "Continue", "exact": true}}
```

Role/accessible-name is preferred. CSS support is limited to Goosie's selector engine. Query returns zero or more current refs and never performs an action. Mutation tools accept one ref, eliminating ambiguous action semantics.

## 6. Wait contract

`browser_wait` accepts one condition:

- lifecycle state reached;
- URL exact/glob match;
- page revision greater than a supplied value;
- semantic text present/absent;
- element ref state (`attached`, `visible`, `enabled`) when supported.

Polling intervals are an implementation detail; the API is event-driven where signals exist. All waits end by success, context close, page change invalidating the condition, cancellation, or deadline.

## 7. Resources

Resources complement tools for repeatable reads and client-driven context selection:

| URI | MIME | Content |
|---|---|---|
| `goosie://contexts` | `application/json` | Contexts owned by the connection. |
| `goosie://context/{id}/page` | `application/json` | Current page metadata. |
| `goosie://context/{id}/snapshot` | `application/json` | Default bounded semantic snapshot. |
| `goosie://context/{id}/console` | `application/json` | Redacted recent console entries. |
| `goosie://context/{id}/network` | `application/json` | Redacted recent network metadata. |
| `goosie://context/{id}/security` | `application/json` | Security summary. |

Use the custom `goosie` scheme because these are server-computed virtual resources, consistent with MCP resource URI guidance. Subscription support is deferred until update ordering and backpressure are proven. Screenshots stay tool results initially because generating them is expensive and parameterized.

## 8. Error result shape

```json
{
  "error": {
    "code": "page_changed",
    "message": "The element reference belongs to an earlier page revision.",
    "retryable": true,
    "correlationId": "err_opaque",
    "details": {"currentPageRevision": 2}
  }
}
```

Messages must be safe to show to users. Never include filesystem paths, headers, cookies, JS stacks containing page secrets, or raw response bodies.

## 9. Capability/version resource

The server exposes implementation name/version, MCP protocol revision, SDK version, Goosie version, enabled transports, enabled tools, hard limits, and supported readiness/locator modes. Clients must use advertised capabilities rather than infer features from a version string.
