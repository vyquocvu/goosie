// TASK-044 integrator probe: does Chromium apply group opacity once per
// stacking context, including for text that sits in an anonymous block?
// Prints the darkest glyph pixel for each case; alpha = 1 - (grey/255) for
// black text over white.
const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

const CASES = {
  span: '<div id="t" style="opacity:0.5"><span>anonymous text</span></div>',
  anon: '<div id="t" style="opacity:0.5">anonymous text</div>',
  anondiv: '<div id="t" style="opacity:0.5">anonymous text<div style="background:rgb(10,10,120)">bg</div></div>',
  quarter: '<div id="t" style="opacity:0.25">anonymous text</div>',
  nested: '<div style="opacity:0.5"><div id="t" style="opacity:1">anonymous text</div></div>',
};

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 400, height: 200 }, deviceScaleFactor: 1 });
  for (const [name, body] of Object.entries(CASES)) {
    const html = `<!doctype html><html><head><style>html,body{margin:0;background:#fff}` +
      `#t{font:16px 'Times New Roman';color:#000}</style></head><body>${body}</body></html>`;
    const f = path.join('/tmp', `t044-${name}.png`);
    await page.setContent(html);
    await page.screenshot({ path: f });
    console.log(`SHOT ${name} ${f}`);
  }
  await browser.close();
})();
