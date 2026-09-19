// Render testdata/urls.txt with Playwright Chromium into
// /tmp/playwright-url-renders/<slug>.png for the parity scorer.
//
// JS is disabled in the reference: goosie has no script engine, so the
// comparison scores HTML+CSS rendering, which is the parity target. Images
// stay enabled because <img> loading is a feature goosie must grow.
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const VIEWPORT = { width: 1280, height: 800 };

function slug(url) {
  return url
    .replace(/^https?:\/\//, '')
    .replace(/[^A-Za-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .slice(0, 80);
}

async function main() {
  const only = process.argv.slice(2);
  // URLS_JSON points at [{"url","out"}] pairs the parity harness froze from a
  // single fetch; without it the corpus is urls.txt and each output is named
  // after its slug.
  const fromManifest = process.env.URLS_JSON
    ? JSON.parse(fs.readFileSync(process.env.URLS_JSON, 'utf8'))
    : fs
        .readFileSync(path.join(__dirname, 'urls.txt'), 'utf8')
        .split('\n')
        .map((s) => s.trim())
        .filter((s) => s && !s.startsWith('#'))
        .map((url) => ({ url, out: path.join('/tmp/playwright-url-renders', slug(url) + '.png') }));

  fs.mkdirSync('/tmp/playwright-url-renders', { recursive: true });

  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: 1,
    javaScriptEnabled: false,
    locale: 'en-US',
  });
  for (const { url, out } of fromManifest) {
    if (only.length && !only.some((a) => url.includes(a))) continue;
    const page = await context.newPage();
    try {
      await page.goto(url, { waitUntil: 'load', timeout: 45000 });
      await page.waitForTimeout(1500);
      await page.screenshot({ path: out, fullPage: false });
      console.log(`ok   ${url} -> ${out}`);
    } catch (err) {
      console.log(`FAIL ${url}: ${String(err).split('\n')[0]}`);
    } finally {
      await page.close();
    }
  }
  await browser.close();
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
