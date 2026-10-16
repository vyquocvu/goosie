const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  await page.setContent(`
    <!DOCTYPE html>
    <html>
    <head><title>AbortController Test</title></head>
    <body>
      <h1>AbortController Verification</h1>
      <div id="results"></div>
      <script>
        var results = document.getElementById('results');
        var tests = [];

        try {
          var controller = new AbortController();
          var signal = controller.signal;
          tests.push({ name: 'AbortController.signal', pass: signal !== undefined, actual: typeof signal });
          tests.push({ name: 'signal.aborted initial', pass: signal.aborted === false, actual: signal.aborted });

          controller.abort();
          tests.push({ name: 'signal.aborted after abort', pass: signal.aborted === true, actual: signal.aborted });
          tests.push({ name: 'signal.reason default', pass: signal.reason !== undefined, actual: typeof signal.reason });
        } catch(e) {
          tests.push({ name: 'AbortController', pass: false, actual: e.message });
        }

        try {
          var controller2 = new AbortController();
          var signal2 = controller2.signal;
          var fired = false;
          signal2.onabort = function() { fired = true; };
          controller2.abort('custom');
          tests.push({ name: 'onabort fires', pass: fired === true, actual: fired });
          tests.push({ name: 'custom reason', pass: signal2.reason === 'custom', actual: signal2.reason });
        } catch(e) {
          tests.push({ name: 'AbortController events', pass: false, actual: e.message });
        }

        try {
          var controller3 = new AbortController();
          controller3.abort('first');
          controller3.abort('second');
          tests.push({ name: 'double abort keeps first', pass: controller3.signal.reason === 'first', actual: controller3.signal.reason });
        } catch(e) {
          tests.push({ name: 'double abort', pass: false, actual: e.message });
        }

        try {
          var signal4 = AbortSignal.abort('test');
          tests.push({ name: 'AbortSignal.abort', pass: signal4.aborted === true && signal4.reason === 'test', actual: signal4.reason });
        } catch(e) {
          tests.push({ name: 'AbortSignal.abort', pass: false, actual: e.message });
        }

        var passCount = tests.filter(t => t.pass).length;
        var html = '<h2>Results: ' + passCount + '/' + tests.length + ' passed</h2><ul>';
        tests.forEach(function(t) {
          html += '<li style="color:' + (t.pass ? 'green' : 'red') + '">' +
            (t.pass ? '✓' : '✗') + ' ' + t.name +
            (t.pass ? '' : ' (got: ' + t.actual + ')') + '</li>';
        });
        html += '</ul>';
        results.innerHTML = html;
      </script>
    </body>
    </html>
  `);

  await page.waitForTimeout(500);
  await page.screenshot({ path: 'verify-phase7d-abort.png', fullPage: true });

  const results = await page.evaluate(() => {
    var items = document.querySelectorAll('#results li');
    return Array.from(items).map(li => li.textContent);
  });

  console.log('AbortController Verification Results:');
  results.forEach(r => console.log('  ' + r));

  const passCount = results.filter(r => r.startsWith('✓')).length;
  console.log('\nTotal: ' + passCount + '/' + results.length + ' passed');

  await browser.close();
})();
