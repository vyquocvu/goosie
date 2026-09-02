# MCP Threat Model and Security Requirements

**Status:** Implemented

**Scope:** Goosie MCP server, browser contexts, transports, tools, and returned data.

## 1. Security posture

The v1 server is a local, stdio-launched, private-context browser automation process. It is not a general remote automation daemon. Browser content, MCP clients, and navigated origins are untrusted. The host filesystem, local network, credentials, profile data, and other MCP connections are protected assets.

MCP's official [security best practices](https://modelcontextprotocol.io/docs/tutorials/security/security_best_practices) and [transport security requirements](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports) are minimums, not substitutes for browser policy.

## 2. Trust boundaries

```text
untrusted MCP request
  → protocol/schema validation
  → connection ownership + operator policy
  → browser-control limits
  → untrusted remote origin / page JavaScript
  → bounded DOM/render/result serialization
  → MCP response

protected side boundaries:
  host filesystem | local/private network | other contexts | logs | process env
```

## 3. Principal threats and controls

| Threat | Example | Required controls |
|---|---|---|
| Arbitrary host access | `file:///etc/passwd`, download path, JS host binding | Deny host filesystem by default; no path-taking v1 tools; retain engine file-origin checks. |
| SSRF/local network reach | Navigate to cloud metadata, router, localhost service | URL policy before every hop, DNS/IP classification, redirect revalidation, optional allow/deny lists; local network denied by default for remote/server deployments. |
| DNS rebinding | HTTP server reached from hostile web origin | HTTP phase: loopback bind, Origin/Host validation, SDK protection explicitly configured and tested. |
| Cross-context data leak | Guess another context ID/resource URI | Cryptographic opaque IDs plus per-connection ownership checks on every operation. |
| Stale-reference action | Old ref clicks new page control | Bind refs to context and page revision; fail closed on mismatch. |
| Prompt/tool injection | Page text instructs model to exfiltrate data | Tools expose page data, not authority escalation; clear tool descriptions; never treat page text as server policy. |
| Secret leakage | Cookies/auth headers/password fields in snapshot/logs | Field-level redaction, header allowlist, password-value omission, bounded diagnostic output, no sensitive audit payloads. |
| Resource exhaustion | Huge DOM/screenshot, infinite JS, navigation storms | Global/context quotas, timeouts, cancellation, body/pixel/node/output caps, bounded queues/rings, rate limits. |
| Protocol corruption | Logs printed to stdout | stdout reserved for MCP frames; logs only on stderr; subprocess conformance test. |
| Session hijacking | Reuse HTTP MCP session ID | Cryptographic IDs, authenticated principal binding, secure comparison, expiry, DELETE cleanup. |
| Confused deputy | Server uses broad credential for client-directed request | No ambient third-party credentials in v1; least-privilege scopes if remote auth is later added. |
| Dependency drift | SDK update changes Origin behavior | Exact version pin, release-note/security review, regression suite before bump. |

## 4. Mandatory v1 stdio requirements

### Process and transport

- stdout MUST contain only valid newline-delimited MCP JSON-RPC frames.
- stderr MUST be the only default human log sink.
- The process MUST exit when stdin closes after cancelling and closing contexts.
- Environment variables and command-line flags containing secrets MUST never be echoed.
- Server configuration MUST reject unknown unsafe flags and invalid limits.

### Context isolation

- Contexts MUST be private/ephemeral by default and MUST NOT load the GUI user's persistent Goosie profile.
- Context IDs MUST be generated using cryptographic randomness and MUST be scoped to their MCP connection.
- Each operation MUST perform ownership lookup before resolving a page/ref.
- Close MUST zero/clear sensitive in-memory rings where practical and remove registry ownership.

### Navigation and network

- URL parsing MUST occur before policy decisions; reject userinfo and malformed/ambiguous encodings.
- Policy MUST apply to initial URL and every redirect.
- `file`, `data`, `javascript`, custom schemes, loopback, link-local, private ranges, and non-default ports MUST have explicit policy, never accidental fallthrough.
- Response-body, redirect-count, header, decompressed-size, and duration limits MUST be enforced.
- TLS verification MUST remain enabled; no “ignore certificate errors” v1 tool.
- Downloads MUST NOT write to arbitrary paths. Attachment responses return metadata/policy denial unless a later sandboxed design is approved.

### Page actions and JavaScript

- Mutation actions MUST require a current opaque ref except viewport/key/scroll operations where the target is explicit.
- Password values MUST never appear in snapshots. Typed text MUST never appear in default logs.
- Evaluation MUST execute only inside the page runtime via its owner queue, with deadline/interruption and serialization limits.
- No Go host object may expose filesystem, subprocess, environment, raw sockets, or server internals to evaluated JavaScript.
- An action MUST NOT silently broaden from semantic targeting to coordinates.

### Output

- Structured outputs MUST be size-checked after serialization as well as before.
- Network output MUST allowlist safe headers; always remove `Authorization`, `Proxy-Authorization`, `Cookie`, `Set-Cookie`, and configured sensitive headers.
- Error messages returned to clients MUST omit stacks, absolute host paths, raw response bodies, and credentials.
- Screenshot dimensions and encoded size MUST be bounded; encoder cancellation and memory pressure MUST be handled.

## 5. Streamable HTTP security requirements

Streamable HTTP mode is activated via `--http` and enforces strict security controls:

- Binds to `127.0.0.1` by default (via `--bind`); never `0.0.0.0` without explicit operator override.
- Configurable port via `--port` (default ephemeral port `0` or explicit integer).
- Bearer token authentication required when `--auth` is set (specified via `--auth-token` or `env:VAR_NAME`).
- Validate `Origin` on every POST/GET; reject invalid present origins with 403.
- Validate Host/local-address relationships to prevent DNS rebinding.
- Configure cross-origin/localhost protection explicitly.
- Bind each MCP session to the authenticated user/client, expire idle sessions, use cryptographic session IDs, and implement cleanup.
- Validate `MCP-Protocol-Version`, content type, Accept, body size, method, and endpoint path (`--path /mcp`).
- Apply request/concurrency rate limits before allocating a browser context.
- Respect reverse-proxy headers only from configured trusted proxies.
- Never forward client bearer tokens to navigated sites.

## 6. Redaction policy

| Data | Default handling |
|---|---|
| Authorization/cookie headers | Remove entirely. |
| Password/file input value | Replace with `[REDACTED]`; never retain original in snapshot. |
| Typed text | Use only for action execution; log byte count, not content. |
| JS source/result | Do not audit payload; return bounded result to requesting client only. |
| URL query | Return to owning client; audit normalized origin/path with configured sensitive keys redacted. |
| Console messages | Return bounded to owner; redact configured secret patterns; do not mirror into server logs by default. |
| Screenshot | Return to owner; never persist implicitly. |
| Host paths/stacks | stderr debug only under explicit secure debug mode; never client errors. |

## 7. Security test matrix

| Requirement | Test type |
|---|---|
| Context ownership | Two-client integration attempts cross-access for every tool/resource. |
| Ref revision binding | Navigate between snapshot/action; action must return `page_changed`. |
| URL/redirect policy | Table/unit tests plus fixture redirects to loopback/private/link-local/file. |
| Body/decompression caps | Fixture sends oversized and compressed expansion bodies. |
| JS timeout/host isolation | Infinite loops, queue saturation, forbidden global inspection. |
| Redaction | Golden outputs containing every sensitive header/field class. |
| stdout purity | Launch subprocess, split every stdout line, parse as JSON-RPC. |
| Shutdown/leaks | Close/disconnect under active fetch/JS/screenshot; race and goroutine checks. |
| HTTP Origin/Host/auth | Raw HTTP integration matrix before HTTP can be enabled. |
| Fuzz resilience | URL policy, MCP input decoding, locator parsing, result serialization. |

## 8. Security release gate

- No unresolved critical/high finding.
- All MUST requirements have tests with stable names referenced from the implementation PR.
- `go test -race ./test/internal/browsercontrol/... ./test/internal/mcpserver/...` passes for browser-control/MCP packages.
- Dependency vulnerability and license review is recorded.
- A reviewer validates the pinned SDK's actual security defaults at the selected tag.
- Remote/HTTP features remain build/runtime disabled unless their separate gate is complete.
