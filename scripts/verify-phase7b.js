const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  await page.setContent(`
    <!DOCTYPE html>
    <html>
    <head><title>TextEncoding Test</title></head>
    <body>
      <h1>TextEncoder & TextDecoder Verification</h1>
      <div id="results"></div>
      <script>
        var results = document.getElementById('results');
        var tests = [];

        // Test TextEncoder
        try {
          var enc = new TextEncoder();
          var encoded = enc.encode('Hello');
          tests.push({ name: 'TextEncoder.encode length', pass: encoded.length === 5, actual: encoded.length });
          tests.push({ name: 'TextEncoder.encode first byte', pass: encoded[0] === 72, actual: encoded[0] });
          tests.push({ name: 'TextEncoder.encoding', pass: enc.encoding === 'utf-8', actual: enc.encoding });
        } catch(e) {
          tests.push({ name: 'TextEncoder', pass: false, actual: e.message });
        }

        // Test TextEncoder with Unicode
        try {
          var enc2 = new TextEncoder();
          var encoded2 = enc2.encode('世界');
          tests.push({ name: 'TextEncoder Unicode length', pass: encoded2.length === 6, actual: encoded2.length });
        } catch(e) {
          tests.push({ name: 'TextEncoder Unicode', pass: false, actual: e.message });
        }

        // Test TextDecoder
        try {
          var dec = new TextDecoder();
          var decoded = dec.decode(new Uint8Array([72, 101, 108, 108, 111]));
          tests.push({ name: 'TextDecoder.decode', pass: decoded === 'Hello', actual: decoded });
          tests.push({ name: 'TextDecoder.encoding', pass: dec.encoding === 'utf-8', actual: dec.encoding });
        } catch(e) {
          tests.push({ name: 'TextDecoder', pass: false, actual: e.message });
        }

        // Test TextDecoder with Unicode
        try {
          var dec2 = new TextDecoder();
          var decoded2 = dec2.decode(new Uint8Array([228, 184, 150, 231, 149, 140]));
          tests.push({ name: 'TextDecoder Unicode', pass: decoded2 === '世界', actual: decoded2 });
        } catch(e) {
          tests.push({ name: 'TextDecoder Unicode', pass: false, actual: e.message });
        }

        // Test round-trip
        try {
          var enc3 = new TextEncoder();
          var dec3 = new TextDecoder();
          var original = 'Hello, 世界! 🌍';
          var encoded3 = enc3.encode(original);
          var decoded3 = dec3.decode(encoded3);
          tests.push({ name: 'Round-trip', pass: original === decoded3, actual: decoded3 });
        } catch(e) {
          tests.push({ name: 'Round-trip', pass: false, actual: e.message });
        }

        // Render results
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
  await page.screenshot({ path: 'verify-phase7b-textencoding.png', fullPage: true });

  const results = await page.evaluate(() => {
    var items = document.querySelectorAll('#results li');
    return Array.from(items).map(li => li.textContent);
  });

  console.log('TextEncoding Verification Results:');
  results.forEach(r => console.log('  ' + r));

  const passCount = results.filter(r => r.startsWith('✓')).length;
  console.log('\nTotal: ' + passCount + '/' + results.length + ' passed');

  await browser.close();
})();
