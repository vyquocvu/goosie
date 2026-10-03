const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();

  await page.setContent(`
    <!DOCTYPE html>
    <html>
    <head><title>FormData Test</title></head>
    <body>
      <h1>FormData Verification</h1>
      <div id="results"></div>
      <script>
        var results = document.getElementById('results');
        var tests = [];

        try {
          var fd = new FormData();
          fd.append('name', 'John');
          fd.append('age', '30');
          fd.append('name', 'Jane');
          tests.push({ name: 'FormData.get', pass: fd.get('name') === 'John', actual: fd.get('name') });
          tests.push({ name: 'FormData.getAll', pass: fd.getAll('name').length === 2, actual: fd.getAll('name').length });
          tests.push({ name: 'FormData.has', pass: fd.has('name') === true, actual: fd.has('name') });
          tests.push({ name: 'FormData.has(missing)', pass: fd.has('missing') === false, actual: fd.has('missing') });
          fd.set('name', 'Bob');
          tests.push({ name: 'FormData.set', pass: fd.get('name') === 'Bob', actual: fd.get('name') });
          tests.push({ name: 'FormData.set replaces all', pass: fd.getAll('name').length === 1, actual: fd.getAll('name').length });
          fd.delete('age');
          tests.push({ name: 'FormData.delete', pass: fd.has('age') === false, actual: fd.has('age') });
          tests.push({ name: 'FormData.delete preserves others', pass: fd.has('name') === true, actual: fd.has('name') });
          var count = 0;
          fd.forEach(function(v, k) { count++; });
          tests.push({ name: 'FormData.forEach', pass: count === 1, actual: count });
        } catch(e) {
          tests.push({ name: 'FormData', pass: false, actual: e.message });
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
  await page.screenshot({ path: 'verify-phase7c-formdata.png', fullPage: true });

  const results = await page.evaluate(() => {
    var items = document.querySelectorAll('#results li');
    return Array.from(items).map(li => li.textContent);
  });

  console.log('FormData Verification Results:');
  results.forEach(r => console.log('  ' + r));

  const passCount = results.filter(r => r.startsWith('✓')).length;
  console.log('\nTotal: ' + passCount + '/' + results.length + ' passed');

  await browser.close();
})();
