// Capture a page's served HTML through Playwright and print {url, html} as one
// JSON line. The parity harness uses this when urllib cannot fetch a URL: the
// snapshot corpus has to be the same bytes both engines render, and a page only
// Chromium can reach is still a page both engines can render once captured.
//
// JS stays off so the capture is the document the reader sees, not a script's
// rewrite of it - goosie has no script engine, so a JS-built DOM could not be
// compared at all.
const { chromium } = require('playwright');

async function main() {
  const url = process.argv[2];
  if (!url) {
    console.error('usage: fetch-url.js <url>');
    process.exit(2);
  }
  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 1,
    javaScriptEnabled: false,
    userAgent:
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36',
  });
  const page = await context.newPage();
  try {
    await page.goto(url, { waitUntil: 'load', timeout: 45000 });
    console.log(JSON.stringify({ url: page.url(), html: await page.content() }));
  } finally {
    await context.close();
    await browser.close();
  }
}

main().catch((err) => {
  console.error(String(err).split('\n')[0]);
  process.exit(1);
});
