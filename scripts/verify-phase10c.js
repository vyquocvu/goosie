const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 800, height: 400 } });

  await page.setContent(`
    <!DOCTYPE html>
    <html>
    <head><style>
      body { margin: 0; font-family: sans-serif; background: #f0f0f0; }
      h1 { font-size: 18px; margin: 20px; }
      .spacer { height: 600px; background: linear-gradient(to bottom, #f0f0f0, #e0e0e0); display: flex; align-items: center; justify-content: center; color: #666; }
      .target {
        width: 200px;
        height: 100px;
        background: #667eea;
        color: white;
        display: flex;
        align-items: center;
        justify-content: center;
        margin: 20px auto;
        border-radius: 8px;
        transition: background 0.3s;
      }
      .target.visible {
        background: #43e97b;
      }
      #log {
        position: fixed;
        top: 10px;
        right: 10px;
        background: white;
        padding: 10px;
        border: 1px solid #ccc;
        border-radius: 4px;
        font-family: monospace;
        font-size: 11px;
        max-width: 250px;
        max-height: 200px;
        overflow-y: auto;
      }
    </style></head>
    <body>
      <h1>Phase 10c: IntersectionObserver Verification</h1>

      <div class="spacer">Scroll down to see targets</div>

      <div class="target" id="target1">Target 1</div>
      <div class="target" id="target2">Target 2</div>
      <div class="target" id="target3">Target 3</div>

      <div class="spacer">End of content</div>

      <div id="log">Intersection log:</div>

      <script>
        const log = document.getElementById('log');
        function addLog(msg) {
          log.innerHTML += '<br>' + msg;
        }

        const observer = new IntersectionObserver((entries) => {
          entries.forEach(entry => {
            const id = entry.target.id;
            const visible = entry.isIntersecting;
            addLog(id + ': ' + (visible ? 'visible' : 'hidden') + ' (' + Math.round(entry.intersectionRatio * 100) + '%)');

            if (visible) {
              entry.target.classList.add('visible');
            } else {
              entry.target.classList.remove('visible');
            }
          });
        }, {
          threshold: [0, 0.5, 1.0]
        });

        document.querySelectorAll('.target').forEach(target => {
          observer.observe(target);
        });

        addLog('Observer created with threshold [0, 0.5, 1.0]');
      </script>
    </body>
    </html>
  `);

  // Take initial screenshot
  await page.screenshot({ path: 'verify-phase10c-intersection-1.png', fullPage: false });

  // Scroll down
  await page.evaluate(() => window.scrollTo(0, 400));
  await page.waitForTimeout(500);

  // Take screenshot after scroll
  await page.screenshot({ path: 'verify-phase10c-intersection-2.png', fullPage: false });

  // Scroll more
  await page.evaluate(() => window.scrollTo(0, 800));
  await page.waitForTimeout(500);

  // Take final screenshot
  await page.screenshot({ path: 'verify-phase10c-intersection-3.png', fullPage: false });

  console.log('Screenshots saved: verify-phase10c-intersection-*.png');

  await browser.close();
})();
