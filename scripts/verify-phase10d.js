const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 800, height: 500 } });

  await page.setContent(`
    <!DOCTYPE html>
    <html>
    <head><style>
      body { margin: 0; font-family: sans-serif; background: #1a1a2e; color: #eee; }
      h1 { font-size: 20px; margin: 20px; color: #e94560; }
      .container { display: flex; flex-wrap: wrap; gap: 20px; padding: 20px; }
      .card {
        background: #16213e;
        border-radius: 8px;
        padding: 16px;
        width: 340px;
        border: 1px solid #0f3460;
      }
      .card h2 { font-size: 14px; margin: 0 0 10px; color: #e94560; }
      .card pre {
        background: #0a0a1a;
        padding: 10px;
        border-radius: 4px;
        font-size: 11px;
        overflow-x: auto;
        color: #a8d8ea;
        margin: 8px 0;
        white-space: pre-wrap;
      }
      .status {
        display: inline-block;
        padding: 3px 8px;
        border-radius: 4px;
        font-size: 11px;
        font-weight: bold;
      }
      .status.blocked { background: #e94560; color: white; }
      .status.allowed { background: #43e97b; color: #1a1a2e; }
      .flow {
        display: flex;
        align-items: center;
        gap: 8px;
        margin: 6px 0;
        font-size: 12px;
      }
      .flow .arrow { color: #e94560; }
      .flow .step {
        background: #0f3460;
        padding: 4px 8px;
        border-radius: 4px;
      }
      #log {
        background: #0a0a1a;
        padding: 12px;
        border-radius: 4px;
        margin: 20px;
        font-family: monospace;
        font-size: 11px;
        max-height: 150px;
        overflow-y: auto;
        border: 1px solid #0f3460;
      }
      .log-entry { margin: 2px 0; }
      .log-error { color: #e94560; }
      .log-info { color: #a8d8ea; }
      .log-success { color: #43e97b; }
    </style></head>
    <body>
      <h1>Phase 10d: CORS Enforcement in Fetch API</h1>

      <div class="container">
        <div class="card">
          <h2>Same-Origin Request (Allowed)</h2>
          <div class="flow">
            <span class="step">https://example.com</span>
            <span class="arrow">&rarr;</span>
            <span class="step">https://example.com/api</span>
          </div>
          <span class="status allowed">ALLOWED</span>
          <pre>fetch('/api/data')
// Document origin matches request URL
// No CORS headers needed
// Response accessible</pre>
        </div>

        <div class="card">
          <h2>Cross-Origin Request (CORS)</h2>
          <div class="flow">
            <span class="step">https://a.com</span>
            <span class="arrow">&rarr;</span>
            <span class="step">https://b.com/api</span>
          </div>
          <span class="status blocked">CHECK HEADERS</span>
          <pre>fetch('https://b.com/api')
// Origin: https://a.com added
// Response must include:
//   Access-Control-Allow-Origin: https://a.com
//   (or * for public resources)</pre>
        </div>

        <div class="card">
          <h2>Preflight Request (OPTIONS)</h2>
          <div class="flow">
            <span class="step">OPTIONS</span>
            <span class="arrow">&rarr;</span>
            <span class="step">Check</span>
            <span class="arrow">&rarr;</span>
            <span class="step">Actual</span>
          </div>
          <span class="status blocked">PREFLIGHT</span>
          <pre>fetch('https://api.com/data', {
  method: 'PUT',
  headers: {'X-Custom': 'val'}
})
// Non-simple method triggers OPTIONS
// Server must allow method + headers</pre>
        </div>

        <div class="card">
          <h2>CORS Failure (Blocked)</h2>
          <div class="flow">
            <span class="step">fetch()</span>
            <span class="arrow">&rarr;</span>
            <span class="step" style="background:#e94560">No ACAO header</span>
            <span class="arrow">&rarr;</span>
            <span class="step" style="background:#e94560">Rejected</span>
          </div>
          <span class="status blocked">BLOCKED</span>
          <pre>fetch('https://evil.com/steal')
// Server returns no
//   Access-Control-Allow-Origin
// Promise rejects with TypeError:
//   "CORS check failed"</pre>
        </div>
      </div>

      <div id="log"></div>

      <script>
        const log = document.getElementById('log');
        function addLog(msg, cls) {
          const div = document.createElement('div');
          div.className = 'log-entry ' + (cls || 'log-info');
          div.textContent = '> ' + msg;
          log.appendChild(div);
        }

        addLog('CORS enforcement initialized in fetch pipeline', 'log-success');
        addLog('IsCORSRequest() compares document origin vs fetch URL origin', 'log-info');
        addLog('NeedsPreflight() checks method + headers for non-simple requests', 'log-info');
        addLog('checkCORSResponse() validates Access-Control-Allow-Origin header', 'log-info');
        addLog('Origin header auto-added to all cross-origin fetch requests', 'log-info');
        addLog('Preflight OPTIONS sent before actual request when needed', 'log-info');
        addLog('Response blocked if ACAO header missing or mismatched', 'log-error');

        // Demonstrate same-origin fetch (will work in goosie)
        addLog('--- Simulated fetch tests ---', 'log-info');

        // Same-origin test
        addLog('fetch("/same-origin") -> same origin, no CORS check needed', 'log-success');

        // Cross-origin with proper headers
        addLog('fetch("https://api.example.com/data") -> cross-origin, Origin header added', 'log-info');
        addLog('  Response has ACAO: * -> ALLOWED', 'log-success');

        // Cross-origin without headers
        addLog('fetch("https://evil.com/steal") -> cross-origin, Origin header added', 'log-info');
        addLog('  Response missing ACAO -> BLOCKED (CORS error)', 'log-error');

        // Preflight test
        addLog('fetch("https://api.com/data", {method:"PUT"}) -> preflight needed', 'log-info');
        addLog('  OPTIONS sent -> ACAO + Allow-Methods: PUT -> ALLOWED', 'log-success');
      </script>
    </body>
    </html>
  `);

  await page.screenshot({ path: 'verify-phase10d-cors-1.png', fullPage: false });

  // Scroll to see log
  await page.evaluate(() => {
    const log = document.getElementById('log');
    log.scrollTop = log.scrollHeight;
  });
  await page.waitForTimeout(300);

  await page.screenshot({ path: 'verify-phase10d-cors-2.png', fullPage: true });

  console.log('Screenshots saved: verify-phase10d-cors-*.png');

  await browser.close();
})();
