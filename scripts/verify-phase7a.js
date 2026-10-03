const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await context.newPage();

  // Create a test page that exercises URL and URLSearchParams
  await page.setContent(`
    <!DOCTYPE html>
    <html>
    <head><title>URL API Test</title></head>
    <body>
      <h1>URL & URLSearchParams Verification</h1>
      <div id="results"></div>
      <script>
        var results = document.getElementById('results');
        var tests = [];

        // Test URL constructor
        try {
          var u = new URL('https://user:pass@example.com:8080/path?q=hello#section');
          tests.push({ name: 'URL.protocol', pass: u.protocol === 'https:', actual: u.protocol });
          tests.push({ name: 'URL.hostname', pass: u.hostname === 'example.com', actual: u.hostname });
          tests.push({ name: 'URL.port', pass: u.port === '8080', actual: u.port });
          tests.push({ name: 'URL.pathname', pass: u.pathname === '/path', actual: u.pathname });
          tests.push({ name: 'URL.search', pass: u.search === '?q=hello', actual: u.search });
          tests.push({ name: 'URL.hash', pass: u.hash === '#section', actual: u.hash });
          tests.push({ name: 'URL.username', pass: u.username === 'user', actual: u.username });
          tests.push({ name: 'URL.password', pass: u.password === 'pass', actual: u.password });
          tests.push({ name: 'URL.origin', pass: u.origin === 'https://example.com:8080', actual: u.origin });
        } catch(e) {
          tests.push({ name: 'URL constructor', pass: false, actual: e.message });
        }

        // Test URL with base
        try {
          var u2 = new URL('/api/data', 'https://api.example.com');
          tests.push({ name: 'URL with base', pass: u2.href === 'https://api.example.com/api/data', actual: u2.href });
        } catch(e) {
          tests.push({ name: 'URL with base', pass: false, actual: e.message });
        }

        // Test URLSearchParams
        try {
          var sp = new URLSearchParams('foo=1&bar=2&foo=3');
          tests.push({ name: 'URLSearchParams.get', pass: sp.get('foo') === '1', actual: sp.get('foo') });
          tests.push({ name: 'URLSearchParams.getAll', pass: sp.getAll('foo').length === 2, actual: sp.getAll('foo').length });
          tests.push({ name: 'URLSearchParams.has', pass: sp.has('foo') === true, actual: sp.has('foo') });
          tests.push({ name: 'URLSearchParams.has(missing)', pass: sp.has('baz') === false, actual: sp.has('baz') });
          tests.push({ name: 'URLSearchParams.set', pass: (sp.set('foo', '99'), sp.get('foo') === '99'), actual: sp.get('foo') });
          tests.push({ name: 'URLSearchParams.delete', pass: (sp.delete('bar'), sp.has('bar') === false), actual: sp.has('bar') });
          tests.push({ name: 'URLSearchParams.append', pass: (sp.append('new', 'val'), sp.get('new') === 'val'), actual: sp.get('new') });
        } catch(e) {
          tests.push({ name: 'URLSearchParams', pass: false, actual: e.message });
        }

        // Test URL.searchParams
        try {
          var u3 = new URL('https://example.com/path?a=1&b=2');
          tests.push({ name: 'URL.searchParams.get', pass: u3.searchParams.get('a') === '1', actual: u3.searchParams.get('a') });
          tests.push({ name: 'URL.searchParams.toString', pass: u3.searchParams.toString() === 'a=1&b=2', actual: u3.searchParams.toString() });
        } catch(e) {
          tests.push({ name: 'URL.searchParams', pass: false, actual: e.message });
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
  await page.screenshot({ path: 'verify-phase7a-url.png', fullPage: true });

  // Print results
  const results = await page.evaluate(() => {
    var items = document.querySelectorAll('#results li');
    return Array.from(items).map(li => li.textContent);
  });

  console.log('URL API Verification Results:');
  results.forEach(r => console.log('  ' + r));

  const passCount = results.filter(r => r.startsWith('✓')).length;
  console.log('\nTotal: ' + passCount + '/' + results.length + ' passed');

  await browser.close();
})();
