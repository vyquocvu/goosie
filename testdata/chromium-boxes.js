// Print Chromium's own geometry for a page so goosie's cmd/dump-url numbers can
// be compared against it line by line. JS stays disabled and the viewport stays
// 1280x800 to match the parity harness, and pointing it at the --snapshot copy
// (/tmp/goosie-url-snapshots/<slug>.html, served locally) means both engines
// are measured on identical bytes.
//
//   python3 -m http.server 8931 -d /tmp/goosie-url-snapshots &
//   NODE_PATH=$PWD/node_modules node testdata/chromium-boxes.js \
//     http://127.0.0.1:8931/gridbyexample_com_examples.html \
//     'header, .wrapper, h1, nav.main li'
const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 1,
    javaScriptEnabled: false,
    locale: 'en-US',
  });
  const page = await context.newPage();
  await page.goto(process.argv[2], { waitUntil: 'load', timeout: 45000 });
  const boxes = await page.$$eval(process.argv[3], (els) =>
    els.slice(0, 24).map((e) => {
      const r = e.getBoundingClientRect();
      const cs = getComputedStyle(e);
      return (
        `${e.tagName.toLowerCase()}.${e.className || ''} ` +
        `x=${Math.round(r.x)} y=${Math.round(r.y + scrollY)} ` +
        `w=${Math.round(r.width)} h=${Math.round(r.height)} ` +
        `lh=${cs.lineHeight} fs=${cs.fontSize} mt=${cs.marginTop} ` +
        `mb=${cs.marginBottom} disp=${cs.display}`
      );
    })
  );
  console.log(boxes.join('\n'));
  await browser.close();
})();
