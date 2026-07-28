#!/usr/bin/env node
/**
 * Render README screenshots from synthetic demo JSON outputs.
 * Generates dark-themed HTML pages with GitHub-style chrome.
 *
 * Usage: node scripts/render_screenshots.js
 */

const fs = require("fs");
const path = require("path");

const OUT = path.resolve(__dirname, "../test/mcp-screenshots");
if (!fs.existsSync(OUT)) fs.mkdirSync(OUT, { recursive: true });

// ============================================================
// 01. Server info card
// ============================================================
const infoHtml = wrap("Server Info", `
<div class="kv">
  <div class="row"><div class="key">name</div><div class="value">goosie-mcp-server</div></div>
  <div class="row"><div class="key">version</div><div class="value">1.0.0-alpha</div></div>
  <div class="row"><div class="key">protocolVersion</div><div class="value">2025-11-25</div></div>
  <div class="row"><div class="key">goVersion</div><div class="value">go1.25</div></div>
  <div class="row"><div class="key">os</div><div class="value">darwin</div></div>
  <div class="row"><div class="key">arch</div><div class="value">arm64</div></div>
</div>`);
write("01_server_info.html", infoHtml);

// ============================================================
// 02. Initialize exchange
// ============================================================
const initHtml = wrap("POST /mcp → initialize", `
<div class="grid">
  <div class="card">
    <h2>📤 Request</h2>
<pre>{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2025-11-25",
    "clientInfo": {
      "name": "claude-desktop",
      "version": "1.0.0"
    }
  }
}</pre>
  </div>
  <div class="card">
    <h2>📥 Response</h2>
<p><span class="status ok">HTTP 200 OK</span></p>
<p><span class="hdr">Mcp-Session-Id: a4f1c2e87b1d4539...</span></p>
<pre>{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2025-11-25",
    "serverInfo": {
      "name": "goosie-mcp-server",
      "version": "1.0.0-alpha",
      "protocolVersion": "2025-11-25",
      "goVersion": "go1.25",
      "os": "darwin",
      "arch": "arm64"
    },
    "capabilities": {
      "tools": {},
      "resources": {}
    }
  }
}</pre>
  </div>
</div>`);
write("02_initialize.html", initHtml);

// ============================================================
// 03. Tool catalog
// ============================================================
const toolsHtml = wrap("MCP Tool Catalog", `
<p class="lead">10 tools exposed via the MCP protocol. Each tool maps to a
browser-control method with strict schema, ownership, and ref-binding validation.</p>
<div class="tools">
  <div class="tool"><div class="ribbon context">context</div>
    <h3>browser_context_create</h3>
    <p>Create a new private ephemeral browser context with optional viewport.</p>
    <pre>{ "type": "object", "properties": { "viewport": {...} } }</pre>
  </div>
  <div class="tool"><div class="ribbon context">context</div>
    <h3>browser_context_list</h3>
    <p>List all contexts owned by this MCP connection.</p>
  </div>
  <div class="tool"><div class="ribbon context">context</div>
    <h3>browser_context_close</h3>
    <p>Idempotently close a context and zero sensitive in-memory state.</p>
  </div>
  <div class="tool"><div class="ribbon navigate">navigate</div>
    <h3>browser_navigate</h3>
    <p>Navigate to a URL using Goosie's headless pipeline with URL policy.</p>
    <pre>{ "required": ["contextId", "url"], "url": {"format":"uri"} }</pre>
  </div>
  <div class="tool"><div class="ribbon read">read</div>
    <h3>browser_snapshot</h3>
    <p>Accessibility tree with element refs (max 5,000 nodes, 1 MiB).</p>
  </div>
  <div class="tool"><div class="ribbon read">read</div>
    <h3>browser_page_info</h3>
    <p>Page URL, title, state, viewport, revision counter.</p>
  </div>
  <div class="tool"><div class="ribbon mutate">mutate</div>
    <h3>browser_click</h3>
    <p>Click an element bound to a context + page revision.</p>
  </div>
  <div class="tool"><div class="ribbon mutate">mutate</div>
    <h3>browser_type</h3>
    <p>Type text into an editable element (typed text never logged).</p>
  </div>
  <div class="tool"><div class="ribbon eval">eval</div>
    <h3>browser_evaluate</h3>
    <p>Run JavaScript in the page runtime via the owner goroutine (256 KiB source, 5s timeout).</p>
  </div>
  <div class="tool"><div class="ribbon capture">capture</div>
    <h3>browser_screenshot</h3>
    <p>Capture viewport as PNG (rate-limited, 100/ctx).</p>
  </div>
</div>`);
write("03_tools.html", toolsHtml);

// ============================================================
// 04. Health & Metrics
// ============================================================
const healthHtml = wrap("Health & Metrics", `
<p><span class="status ok">HEALTHY — ok</span></p>
<div class="kv">
  <div class="row"><div class="key">startedAt</div><div class="value">2026-07-28T14:30:00Z</div></div>
  <div class="row"><div class="key">uptimeSeconds</div><div class="value">4281</div></div>
  <div class="row"><div class="key">totalRequests</div><div class="value">412</div></div>
  <div class="row"><div class="key">totalErrors</div><div class="value">3</div></div>
  <div class="row"><div class="key">totalTimeouts</div><div class="value">1</div></div>
  <div class="row"><div class="key">totalDenied</div><div class="value">7</div></div>
  <div class="row"><div class="key">activeContexts</div><div class="value">2</div></div>
  <div class="row"><div class="key">maxContexts</div><div class="value">100</div></div>
  <div class="row"><div class="key">memoryAllocBytes</div><div class="value">12.34 MB</div></div>
  <div class="row"><div class="key">goroutines</div><div class="value">9</div></div>
  <div class="row"><div class="key">gcRuns</div><div class="value">14</div></div>
</div>`);
write("04_health.html", healthHtml);

// ============================================================
// 05. Hardening walkthrough
// ============================================================
const hardeningHtml = wrap("Hardening Walkthrough", `
<ol class="timeline">
  <li><div class="step">step 1</div>
    <h3>Boot</h3>
    <p>Server starts with audit logger writing to stderr, rate limiter armed
    (100 tokens / 50 per sec), quotas attached (100 MB memory, 10k requests).</p>
  </li>
  <li><div class="step">step 2</div>
    <h3>Health Check</h3>
    <p><code>IsHealthy() → (true, "ok")</code> with full metrics available via <code>GET /health</code>.</p>
  </li>
  <li><div class="step">step 3</div>
    <h3>Rate Limit Exercise</h3>
    <p>Burst 10 → <strong>10 allowed</strong>, then 5 denied until tokens refill.</p>
    <pre class="audit">{ "tokensAtStart": 10, "allowedOfFifteen": 10, "deniedOfFifteen": 5 }</pre>
  </li>
  <li><div class="step">step 4</div>
    <h3>Per-context Quotas</h3>
    <pre class="audit">{
  "contextId": "ctx_a3f1c2e8",
  "usage": {
    "memoryBytes": 6291456,
    "requestCount": 42,
    "screenshotCount": 3,
    "navigationCount": 7
  }
}</pre>
  </li>
  <li><div class="step">step 5</div>
    <h3>Audit Trail (stderr, single-line JSON)</h3>
    <pre class="audit">{"ts":"2026-07-28T14:32:11Z","type":"tool_call","contextId":"ctx_a3f1c2e8","tool":"browser_navigate","outcome":"success","durationMs":412}
{"ts":"2026-07-28T14:32:15Z","type":"tool_call","contextId":"ctx_a3f1c2e8","tool":"browser_snapshot","outcome":"success","durationMs":89}
{"ts":"2026-07-28T14:32:18Z","type":"tool_call","contextId":"ctx_a3f1c2e8","tool":"browser_screenshot","outcome":"success","durationMs":230}
{"ts":"2026-07-28T14:32:25Z","type":"context_close","contextId":"ctx_a3f1c2e8","outcome":"success"}</pre>
    <p class="note">Sensitive keys (password, secret, token, cookie) are
    automatically replaced with <code>[REDACTED]</code> before logging.</p>
  </li>
  <li><div class="step">step 6</div>
    <h3>Graceful Shutdown</h3>
    <p><code>SIGTERM</code> → all in-flight requests cancelled, audit logger
    closed last so the final events flush.</p>
  </li>
</ol>`);
write("05_hardening.html", hardeningHtml);

// ============================================================
// 06. Cross-origin rejection
// ============================================================
const originHtml = wrap("Cross-Origin Rejected", `
<div class="card danger">
  <h2>🛑 HTTP 403 Forbidden</h2>
  <p>The Origin header <code>https://evil.example.com</code> is not on the
  loopback allowlist. This is the default behaviour required by the
  MCP security model to defeat DNS-rebinding and CSRF attacks.</p>
</div>
<div class="grid">
  <div class="card">
    <h3>Request</h3>
    <pre>POST /mcp HTTP/1.1
Host: 127.0.0.1:8088
Origin: https://evil.example.com
Content-Type: application/json

{"jsonrpc":"2.0","id":1,"method":"initialize"}</pre>
  </div>
  <div class="card">
    <h3>Response</h3>
    <pre>HTTP/1.1 403 Forbidden
Content-Type: text/plain; charset=utf-8

invalid Origin header</pre>
  </div>
</div>

<h3>Policy summary</h3>
<ul class="policy">
  <li><b>Default policy:</b> origin must be loopback (http(s)://localhost, 127.0.0.1, ::1)</li>
  <li><b>Strict mode:</b> set <code>--http --auth</code> with <code>--bind 127.0.0.1</code></li>
  <li><b>Explicit allowlist:</b> pass <code>AllowedOrigins</code> to <code>HTTPConfig</code></li>
  <li><b>Bind enforcement:</b> server refuses to start if <code>Bind</code> isn't loopback</li>
</ul>`);
write("06_origin.html", originHtml);

// ============================================================
// 07. Architecture diagram
// ============================================================
const archHtml = wrap("Architecture", `
<div class="layers">
  <div class="layer">
    <div class="title">MCP Client</div>
    <div class="desc">Claude Desktop, Cursor, custom JSON-RPC over stdio or HTTP</div>
  </div>
  <div class="arrow">↓ JSON-RPC 2.0</div>
  <div class="layer">
    <div class="title">mcp-server</div>
    <div class="desc">
      <code>cmd/mcp-server/main.go</code> &nbsp; — process entry, stdio or HTTP transport
    </div>
  </div>
  <div class="arrow">↓ tool call</div>
  <div class="layer">
    <div class="title">MCP SDK v1.4.0</div>
    <div class="desc">
      <code>github.com/modelcontextprotocol/go-sdk</code>
    </div>
  </div>
  <div class="arrow">↓ </div>
  <div class="layer highlight">
    <div class="title">mcpserver package</div>
    <div class="desc">
      Tool handlers, schemas, audit logger, rate limiter, quotas, health
      reporter, shutdown handler, HTTP transport
    </div>
  </div>
  <div class="arrow">↓ Service / Context interfaces</div>
  <div class="layer">
    <div class="title">browsercontrol package</div>
    <div class="desc">
      UI-independent <code>Service</code> and <code>Context</code> interfaces,
      typed errors, event-style results
    </div>
  </div>
  <div class="arrow">↓ engine Context method</div>
  <div class="layer">
    <div class="title">engine packages</div>
    <div class="desc">
      <code>engine/session</code>, <code>engine/navigation</code>,
      <code>engine/dom</code>, <code>js</code>, <code>net</code>
    </div>
  </div>
  <div class="arrow">↓ parse / fetch / render</div>
  <div class="layer">
    <div class="title">Page State</div>
    <div class="desc">DOM store, JS runtime, console history, network log, screenshots</div>
  </div>
</div>`);
write("07_architecture.html", archHtml);

// ============================================================
// helpers
// ============================================================
function wrap(title, body) {
  return `<!doctype html>
<html><head><meta charset="utf-8"><title>${escape(title)}</title>
<style>
  :root { color-scheme: dark; }
  body { margin: 0; font: 14px -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif; background: #0d1117; color: #c9d1d9; padding: 32px; }
  h1 { color: #58a6ff; font-size: 28px; margin: 0 0 16px 0; font-weight: 600; }
  h2 { font-size: 16px; color: #f0f6fc; margin: 0 0 12px 0; }
  h3 { font-size: 14px; color: #f0f6fc; margin: 16px 0 8px 0; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
  p { margin: 8px 0; line-height: 1.5; }
  pre { background: #161b22; padding: 14px; border-radius: 6px; border: 1px solid #30363d; font: 12.5px ui-monospace, SFMono-Regular, Menlo, monospace; color: #c9d1d9; overflow-x: auto; line-height: 1.5; white-space: pre-wrap; }
  pre.audit { background: #0d1117; border: 1px solid #30363d; color: #7ee787; }
  code { background: #161b22; padding: 2px 6px; border-radius: 4px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 13px; color: #79c0ff; border: 1px solid #30363d; }
  .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 18px; margin: 16px 0; }
  .card { background: #161b22; padding: 18px; border-radius: 8px; border: 1px solid #30363d; }
  .card.danger { border-color: #da3633; background: #1c0e0e; }
  .card.danger h2 { color: #f85149; }
  .status { display: inline-block; padding: 6px 14px; border-radius: 6px; font-weight: bold; font-size: 14px; }
  .ok { background: #238636; color: white; }
  .hdr { display: inline-block; padding: 4px 8px; background: #1f6feb; color: white; border-radius: 4px; font-family: ui-monospace, monospace; font-size: 12px; word-break: break-all; }
  .kv { background: #161b22; padding: 18px; border-radius: 8px; border: 1px solid #30363d; }
  .row { display: flex; padding: 6px 0; border-bottom: 1px solid #21262d; font-size: 13px; }
  .row:last-child { border-bottom: none; }
  .key { flex: 0 0 220px; color: #79c0ff; font-family: ui-monospace, monospace; }
  .value { color: #f0f6fc; }
  .lead { color: #8b949e; margin-bottom: 20px; font-size: 14px; }
  .note { color: #8b949e; font-size: 13px; font-style: italic; }
  .tools { display: grid; grid-template-columns: repeat(2, 1fr); gap: 14px; }
  .tool { background: #161b22; padding: 14px; border-radius: 8px; border: 1px solid #30363d; position: relative; }
  .tool h3 { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; color: #79c0ff; margin: 24px 0 6px 0; font-size: 13.5px; }
  .tool p { color: #8b949e; font-size: 13px; margin: 4px 0; }
  .ribbon { position: absolute; top: 8px; right: 8px; font-size: 10px; text-transform: uppercase; padding: 3px 8px; border-radius: 4px; font-weight: bold; letter-spacing: 0.5px; }
  .ribbon.context { background: #1f6feb; color: white; }
  .ribbon.navigate { background: #a371f7; color: white; }
  .ribbon.read { background: #238636; color: white; }
  .ribbon.mutate { background: #d29922; color: #0d1117; }
  .ribbon.eval { background: #db61a2; color: white; }
  .ribbon.capture { background: #f0883e; color: #0d1117; }
  .timeline { list-style: none; padding: 0; margin: 0; }
  .timeline li { padding: 12px 0 18px 80px; position: relative; border-left: 2px solid #30363d; margin-left: 8px; }
  .step { position: absolute; left: -8px; top: 8px; background: #1f6feb; color: white; padding: 4px 8px; border-radius: 4px; font-size: 11px; text-transform: uppercase; font-weight: bold; }
  .policy { list-style: none; padding: 0; }
  .policy li { padding: 8px 0; border-bottom: 1px solid #21262d; }
  .policy li:last-child { border-bottom: none; }
  .layers { display: flex; flex-direction: column; gap: 0; align-items: center; }
  .layer { background: #161b22; border: 1px solid #30363d; border-radius: 8px; padding: 14px 20px; width: 100%; max-width: 720px; text-align: center; }
  .layer.highlight { background: #1f2937; border-color: #58a6ff; }
  .layer .title { font-family: ui-monospace, monospace; color: #79c0ff; font-size: 16px; margin-bottom: 4px; }
  .layer .desc { color: #8b949e; font-size: 13px; }
  .arrow { color: #58a6ff; padding: 8px; font-family: ui-monospace, monospace; font-size: 14px; }
</style></head><body><h1>${escape(title)}</h1>${body}</body></html>`;
}

function escape(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function write(name, html) {
  const p = path.join(OUT, name);
  fs.writeFileSync(p, html, "utf8");
  console.log(`✓ ${p} (${html.length} bytes)`);
}

console.log(`\nWrote 7 demo HTML files to ${OUT}`);
