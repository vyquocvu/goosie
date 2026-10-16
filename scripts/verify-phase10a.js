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

      my-card {
        display: block;
        width: 150px;
        padding: 16px;
        background: white;
        border: 2px solid #667eea;
        border-radius: 8px;
        margin: 8px;
      }

      my-card.connected {
        background: #667eea;
        color: white;
        border-color: #764ba2;
      }

      my-button {
        display: inline-block;
        padding: 8px 16px;
        background: #43e97b;
        color: white;
        border: none;
        border-radius: 4px;
        cursor: pointer;
      }

      my-button:hover {
        background: #38f9d7;
      }
    </style></head>
    <body>
      <h1>Phase 10a: Custom Elements v1 Verification</h1>

      <div class="row">
        <div class="item">
          <my-card id="card1">Card 1</my-card>
          <div class="label">my-card (pre-defined)</div>
        </div>
        <div class="item">
          <my-card id="card2">Card 2</my-card>
          <div class="label">my-card (pre-defined)</div>
        </div>
      </div>

      <div class="row">
        <div class="item">
          <my-button id="btn1">Click me</my-button>
          <div class="label">my-button</div>
        </div>
        <div class="item">
          <div id="container"></div>
          <div class="label">dynamically added</div>
        </div>
      </div>

      <div class="row">
        <div class="item">
          <div id="log" style="font-family: monospace; font-size: 10px; background: white; padding: 8px; border: 1px solid #ccc; min-height: 60px; min-width: 200px;"></div>
          <div class="label">lifecycle log</div>
        </div>
      </div>

      <script>
        const log = document.getElementById('log');
        function addLog(msg) {
          log.innerHTML += msg + '<br>';
        }

        // Define my-card custom element
        class MyCard extends HTMLElement {
          connectedCallback() {
            this.classList.add('connected');
            addLog('my-card connected: ' + this.textContent);
          }
          disconnectedCallback() {
            this.classList.remove('connected');
            addLog('my-card disconnected');
          }
        }
        customElements.define('my-card', MyCard);

        // Define my-button custom element
        class MyButton extends HTMLElement {
          connectedCallback() {
            addLog('my-button connected');
            this.addEventListener('click', () => {
              addLog('my-button clicked');
            });
          }
        }
        customElements.define('my-button', MyButton);

        // Dynamically add a custom element
        setTimeout(() => {
          const container = document.getElementById('container');
          const card = document.createElement('my-card');
          card.textContent = 'Dynamic';
          container.appendChild(card);
          addLog('dynamic my-card added');
        }, 100);

        // Test whenDefined
        customElements.whenDefined('my-card').then(() => {
          addLog('my-card is defined');
        });
      </script>
    </body>
    </html>
  `);

  // Wait for dynamic content
  await page.waitForTimeout(500);

  await page.screenshot({ path: 'verify-phase10a-custom-elements.png', fullPage: true });
  console.log('Screenshot saved: verify-phase10a-custom-elements.png');

  await browser.close();
})();
