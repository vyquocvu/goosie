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

      .row { display: flex; gap: 20px; margin-bottom: 20px; }

      .box {
        width: 160px; height: 120px;
        border-radius: 8px;
        display: flex; align-items: center; justify-content: center;
        font-size: 12px; color: white; text-shadow: 0 1px 2px rgba(0,0,0,.5);
      }

      /* Basic radial gradient */
      .radial-basic {
        background: radial-gradient(circle, #667eea, #764ba2);
      }

      /* Ellipse shape */
      .radial-ellipse {
        background: radial-gradient(ellipse, #f093fb, #f5576c);
      }

      /* With color stops */
      .radial-stops {
        background: radial-gradient(circle, #43e97b 0%, #38f9d7 50%, #4facfe 100%);
      }

      /* Default (ellipse, farthest-corner) */
      .radial-default {
        background: radial-gradient(#ffecd2, #fcb69f);
      }

      /* Multi-stop with positioned stops */
      .radial-positioned {
        background: radial-gradient(circle at center, red 0%, yellow 25%, green 50%, blue 75%, purple 100%);
      }

      /* Combined with border-radius */
      .radial-rounded {
        background: radial-gradient(circle, #a18cd1, #fbc2eb);
        border-radius: 50%;
        width: 120px; height: 120px;
      }

      .label {
        font-size: 11px;
        text-align: center;
        margin-top: 4px;
        color: #333;
      }

      .item { display: flex; flex-direction: column; align-items: center; }
    </style></head>
    <body>
      <h1>Phase 9b: radial-gradient() Verification</h1>
      <div class="row">
        <div class="item">
          <div class="box radial-basic">circle</div>
          <div class="label">circle</div>
        </div>
        <div class="item">
          <div class="box radial-ellipse">ellipse</div>
          <div class="label">ellipse</div>
        </div>
        <div class="item">
          <div class="box radial-stops">3 stops</div>
          <div class="label">3 color stops</div>
        </div>
        <div class="item">
          <div class="box radial-default">default</div>
          <div class="label">default (ellipse)</div>
        </div>
      </div>
      <div class="row">
        <div class="item">
          <div class="box radial-positioned">rainbow</div>
          <div class="label">5 positioned stops</div>
        </div>
        <div class="item">
          <div class="radial-rounded"></div>
          <div class="label">circle + border-radius</div>
        </div>
      </div>
    </body>
    </html>
  `);

  await page.screenshot({ path: 'verify-phase9b-radial-gradient.png', fullPage: true });
  console.log('Screenshot saved: verify-phase9b-radial-gradient.png');

  await browser.close();
})();
