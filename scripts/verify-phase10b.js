const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 800, height: 600 } });

  await page.setContent(`
    <!DOCTYPE html>
    <html>
    <head><style>
      body { margin: 20px; font-family: sans-serif; background: #f0f0f0; }
      h1 { font-size: 18px; margin-bottom: 16px; }
      .row { display: flex; gap: 20px; margin-bottom: 20px; flex-wrap: wrap; }
      .item { display: flex; flex-direction: column; align-items: center; }
      .label { font-size: 11px; text-align: center; margin-top: 4px; color: #333; }

      my-component {
        display: block;
        margin: 8px;
      }
    </style></head>
    <body>
      <h1>Phase 10b: Shadow DOM Verification</h1>

      <div class="row">
        <div class="item">
          <my-component id="comp1"></my-component>
          <div class="label">Shadow DOM component</div>
        </div>
        <div class="item">
          <div id="host" style="width: 150px; height: 100px; border: 2px solid #ccc; background: white;"></div>
          <div class="label">Shadow host</div>
        </div>
      </div>

      <div class="row">
        <div class="item">
          <div id="log" style="font-family: monospace; font-size: 10px; background: white; padding: 8px; border: 1px solid #ccc; min-height: 60px; min-width: 200px;"></div>
          <div class="label">Shadow DOM log</div>
        </div>
      </div>

      <script>
        const log = document.getElementById('log');
        function addLog(msg) {
          log.innerHTML += msg + '<br>';
        }

        // Test 1: attachShadow to a div
        const host = document.getElementById('host');
        const shadow = host.attachShadow({mode: 'open'});
        addLog('shadow root created, mode: ' + shadow.mode);

        const shadowDiv = document.createElement('div');
        shadowDiv.textContent = 'Shadow content';
        shadowDiv.style.cssText = 'background: #667eea; color: white; padding: 20px; border-radius: 8px;';
        shadow.appendChild(shadowDiv);
        addLog('shadow content appended');

        // Test 2: Custom element with shadow DOM
        class MyComponent extends HTMLElement {
          constructor() {
            super();
            const shadow = this.attachShadow({mode: 'open'});
            const div = document.createElement('div');
            div.textContent = 'Custom element shadow';
            div.style.cssText = 'background: #43e97b; color: white; padding: 16px; border-radius: 8px; width: 120px;';
            shadow.appendChild(div);
            addLog('my-component shadow created');
          }
        }
        customElements.define('my-component', MyComponent);

        // Test 3: Check shadowRoot property
        setTimeout(() => {
          const comp1 = document.getElementById('comp1');
          if (comp1.shadowRoot) {
            addLog('comp1.shadowRoot exists');
          } else {
            addLog('comp1.shadowRoot is null');
          }

          if (host.shadowRoot) {
            addLog('host.shadowRoot exists');
          } else {
            addLog('host.shadowRoot is null');
          }
        }, 100);
      </script>
    </body>
    </html>
  `);

  // Wait for dynamic content
  await page.waitForTimeout(500);

  await page.screenshot({ path: 'verify-phase10b-shadow-dom.png', fullPage: true });
  console.log('Screenshot saved: verify-phase10b-shadow-dom.png');

  await browser.close();
})();
