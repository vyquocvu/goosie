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

      /* :has() tests */
      .box {
        width: 120px; height: 80px;
        border: 2px solid #ccc;
        padding: 8px;
        background: white;
      }

      /* Parent with .child descendant */
      .parent:has(.child) {
        background: #667eea;
        color: white;
      }

      /* Parent with direct .direct-child */
      .parent:has(> .direct-child) {
        border-color: #f5576c;
      }

      /* Element with adjacent sibling */
      .item:has(+ .highlighted) .box {
        border-color: #43e97b;
      }

      .highlighted {
        background: #fee140 !important;
      }

      /* Element with general sibling */
      .item:has(~ .special) .box {
        border-style: dashed;
      }

      .special {
        background: #fa709a !important;
      }

      /* Complex: has multiple conditions */
      .complex:has(.a):has(.b) {
        background: linear-gradient(135deg, #667eea, #764ba2);
        color: white;
      }
    </style></head>
    <body>
      <h1>Phase 9d: :has() Selector Verification</h1>

      <div class="row">
        <div class="item">
          <div class="box parent">
            <div class="child">Has .child</div>
          </div>
          <div class="label">:has(.child)</div>
        </div>

        <div class="item">
          <div class="box parent">
            <div class="direct-child">Direct child</div>
          </div>
          <div class="label">:has(> .direct-child)</div>
        </div>

        <div class="item">
          <div class="box parent">
            <span>No match</span>
          </div>
          <div class="label">no :has match</div>
        </div>
      </div>

      <div class="row">
        <div class="item">
          <div class="box">Before highlighted</div>
          <div class="label">:has(+ .highlighted)</div>
        </div>
        <div class="item highlighted">
          <div class="box">Highlighted</div>
          <div class="label">.highlighted</div>
        </div>
        <div class="item">
          <div class="box">After</div>
          <div class="label">normal</div>
        </div>
      </div>

      <div class="row">
        <div class="item">
          <div class="box">Before special</div>
          <div class="label">:has(~ .special)</div>
        </div>
        <div class="item">
          <div class="box">Middle</div>
          <div class="label">middle</div>
        </div>
        <div class="item special">
          <div class="box">Special</div>
          <div class="label">.special</div>
        </div>
      </div>

      <div class="row">
        <div class="item">
          <div class="box complex">
            <div class="a">A</div>
            <div class="b">B</div>
          </div>
          <div class="label">:has(.a):has(.b)</div>
        </div>
        <div class="item">
          <div class="box complex">
            <div class="a">Only A</div>
          </div>
          <div class="label">:has(.a) only</div>
        </div>
      </div>
    </body>
    </html>
  `);

  await page.screenshot({ path: 'verify-phase9d-has-selector.png', fullPage: true });
  console.log('Screenshot saved: verify-phase9d-has-selector.png');

  await browser.close();
})();
