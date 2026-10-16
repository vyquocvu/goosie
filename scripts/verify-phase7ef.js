const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  await page.setContent(`
    <!DOCTYPE html>
    <html>
    <head><title>Crypto & Performance Test</title></head>
    <body>
      <h1>Crypto & Performance Verification</h1>
      <div id="results"></div>
      <script>
        var results = document.getElementById('results');
        var tests = [];

        try {
          var arr = new Uint8Array(10);
          crypto.getRandomValues(arr);
          var allZero = true;
          for (var i = 0; i < 10; i++) {
            if (arr[i] !== 0) allZero = false;
          }
          tests.push({ name: 'crypto.getRandomValues', pass: !allZero, actual: allZero });
          tests.push({ name: 'crypto.getRandomValues returns same array', pass: arr.length === 10, actual: arr.length });
        } catch(e) {
          tests.push({ name: 'crypto.getRandomValues', pass: false, actual: e.message });
        }

        try {
          var t1 = performance.now();
          var t2 = performance.now();
          tests.push({ name: 'performance.now returns number', pass: typeof t1 === 'number', actual: typeof t1 });
          tests.push({ name: 'performance.now increases', pass: t2 >= t1, actual: t2 - t1 });
          tests.push({ name: 'performance.timeOrigin exists', pass: typeof performance.timeOrigin === 'number', actual: typeof performance.timeOrigin });
        } catch(e) {
          tests.push({ name: 'performance', pass: false, actual: e.message });
        }

        var passCount = tests.filter(t => t.pass).length;
        var html = '<h2>Results: ' + passCount + '/' + tests.length + ' passed</h2><ul>';
        tests.forEach(function(t) {
          html += '<li style="color:' + (t.pass ? 'green' : 'red') + '">' +
            (t.pass ? '✓' : '') + ' ' + t.name +
            (t.pass ? '' : ' (got: ' + t.actual + ')') + '</li>';
        });
        html += '</ul>';
        results.innerHTML = html;
      </script>
    </body>
    </html>
  `);

  await page.waitForTimeout(500);
  await page.screenshot({ path: 'verify-phase7ef-crypto-perf.png', fullPage: true });

  const results = await page.evaluate(() => {
    var items = document.querySelectorAll('#results li');
    return Array.from(items).map(li => li.textContent);
  });

  console.log('Crypto & Performance Verification Results:');
  results.forEach(r => console.log('  ' + r));

  const passCount = results.filter(r => r.startsWith('✓')).length;
  console.log('\nTotal: ' + passCount + '/' + results.length + ' passed');

  await browser.close();
})();
