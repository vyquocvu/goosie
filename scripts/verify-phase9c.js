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
      canvas { border: 1px solid #ccc; background: white; }
    </style></head>
    <body>
      <h1>Phase 9c: Canvas 2D Rendering Verification</h1>
      <div class="row">
        <div class="item">
          <canvas id="c1" width="150" height="100"></canvas>
          <div class="label">fillRect</div>
        </div>
        <div class="item">
          <canvas id="c2" width="150" height="100"></canvas>
          <div class="label">strokeRect</div>
        </div>
        <div class="item">
          <canvas id="c3" width="150" height="100"></canvas>
          <div class="label">fill path (triangle)</div>
        </div>
        <div class="item">
          <canvas id="c4" width="150" height="100"></canvas>
          <div class="label">stroke path (circle)</div>
        </div>
      </div>
      <div class="row">
        <div class="item">
          <canvas id="c5" width="150" height="100"></canvas>
          <div class="label">multiple rects</div>
        </div>
        <div class="item">
          <canvas id="c6" width="150" height="100"></canvas>
          <div class="label">fill + stroke</div>
        </div>
      </div>

      <script>
        // Canvas 1: fillRect
        var c1 = document.getElementById('c1');
        var ctx1 = c1.getContext('2d');
        ctx1.fillStyle = '#667eea';
        ctx1.fillRect(10, 10, 130, 80);

        // Canvas 2: strokeRect
        var c2 = document.getElementById('c2');
        var ctx2 = c2.getContext('2d');
        ctx2.strokeStyle = '#f5576c';
        ctx2.lineWidth = 3;
        ctx2.strokeRect(10, 10, 130, 80);

        // Canvas 3: fill path (triangle)
        var c3 = document.getElementById('c3');
        var ctx3 = c3.getContext('2d');
        ctx3.fillStyle = '#43e97b';
        ctx3.beginPath();
        ctx3.moveTo(75, 10);
        ctx3.lineTo(140, 90);
        ctx3.lineTo(10, 90);
        ctx3.closePath();
        ctx3.fill();

        // Canvas 4: stroke path (circle)
        var c4 = document.getElementById('c4');
        var ctx4 = c4.getContext('2d');
        ctx4.strokeStyle = '#4facfe';
        ctx4.lineWidth = 2;
        ctx4.beginPath();
        ctx4.arc(75, 50, 40, 0, Math.PI * 2);
        ctx4.stroke();

        // Canvas 5: multiple rects
        var c5 = document.getElementById('c5');
        var ctx5 = c5.getContext('2d');
        ctx5.fillStyle = '#fa709a';
        ctx5.fillRect(10, 10, 50, 50);
        ctx5.fillStyle = '#fee140';
        ctx5.fillRect(70, 10, 50, 50);
        ctx5.fillStyle = '#30cfd0';
        ctx5.fillRect(10, 70, 50, 20);
        ctx5.fillStyle = '#a8edea';
        ctx5.fillRect(70, 70, 50, 20);

        // Canvas 6: fill + stroke
        var c6 = document.getElementById('c6');
        var ctx6 = c6.getContext('2d');
        ctx6.fillStyle = '#ffecd2';
        ctx6.fillRect(0, 0, 150, 100);
        ctx6.strokeStyle = '#fcb69f';
        ctx6.lineWidth = 4;
        ctx6.strokeRect(10, 10, 130, 80);
      </script>
    </body>
    </html>
  `);

  await page.screenshot({ path: 'verify-phase9c-canvas.png', fullPage: true });
  console.log('Screenshot saved: verify-phase9c-canvas.png');

  await browser.close();
})();
