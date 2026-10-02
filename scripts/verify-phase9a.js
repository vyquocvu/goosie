const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();

  let passed = 0;
  let failed = 0;

  function assert(name, condition) {
    if (condition) { passed++; console.log(`  PASS: ${name}`); }
    else { failed++; console.log(`  FAIL: ${name}`); }
  }

  await page.setContent(`
    <html>
    <body style="margin:0; font-family: sans-serif;">
      <h2>Phase 9a: SVG Rendering Verification</h2>
      <div id="results"></div>

      <!-- SVG with basic shapes -->
      <svg id="test-svg" width="200" height="200" style="border: 1px solid #ccc;">
        <rect x="10" y="10" width="80" height="60" fill="#3498db" stroke="#2c3e50" stroke-width="2"/>
        <circle cx="150" cy="50" r="30" fill="#e74c3c"/>
        <ellipse cx="100" cy="150" rx="60" ry="30" fill="#2ecc71"/>
        <line x1="10" y1="190" x2="190" y2="190" stroke="#9b59b6" stroke-width="3"/>
        <polygon points="160,120 180,160 140,160" fill="#f39c12"/>
        <path d="M 10 100 L 50 100 L 50 140 L 10 140 Z" fill="#1abc9c"/>
      </svg>

      <script>
        window.__results = [];
        function log(msg) { window.__results.push(msg); }

        // Test 1: SVG element exists
        var svg = document.getElementById('test-svg');
        log('svg_exists: ' + (svg !== null));

        // Test 2: SVG namespace
        log('svg_namespace: ' + (svg.namespaceURI === 'http://www.w3.org/2000/svg'));

        // Test 3: SVG child elements exist
        var rect = svg.querySelector('rect');
        var circle = svg.querySelector('circle');
        var ellipse = svg.querySelector('ellipse');
        var line = svg.querySelector('line');
        var polygon = svg.querySelector('polygon');
        var path = svg.querySelector('path');
        log('rect_exists: ' + (rect !== null));
        log('circle_exists: ' + (circle !== null));
        log('ellipse_exists: ' + (ellipse !== null));
        log('line_exists: ' + (line !== null));
        log('polygon_exists: ' + (polygon !== null));
        log('path_exists: ' + (path !== null));

        // Test 4: SVG attributes
        log('rect_fill: ' + (rect.getAttribute('fill') === '#3498db'));
        log('circle_r: ' + (circle.getAttribute('r') === '30'));
        log('ellipse_rx: ' + (ellipse.getAttribute('rx') === '60'));

        // Test 5: SVG dimensions
        log('svg_width: ' + (svg.getAttribute('width') === '200'));
        log('svg_height: ' + (svg.getAttribute('height') === '200'));

        // Test 6: SVG viewBox
        var svg2 = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        svg2.setAttribute('viewBox', '0 0 100 100');
        log('viewBox_set: ' + (svg2.getAttribute('viewBox') === '0 0 100 100'));
      </script>
    </body>
    </html>
  `);

  const results = await page.evaluate(() => window.__results);

  console.log('Phase 9a SVG Rendering Verification:');
  for (const r of results) {
    const [name, val] = r.split(': ');
    assert(name, val === 'true');
  }

  console.log(`\nResults: ${passed} passed, ${failed} failed`);

  await page.screenshot({ path: 'verify-phase9a-svg.png', fullPage: true });
  console.log('Screenshot saved to verify-phase9a-svg.png');

  await browser.close();
  process.exit(failed > 0 ? 1 : 0);
})();
