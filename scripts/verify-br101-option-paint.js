// Visual anchor for the painted option label: what a closed select shows when a
// label attribute competes with the text content. Run:
//   node scripts/verify-br101-option-paint.js /tmp/optvis/chromium.png
//   go run ./cmd/goosie-headless -in /tmp/optvis/fixture.html \
//       -out /tmp/optvis/goosie.png -width 400 -height 140
// Both PNGs are 800x280 (dpr 2), so crop CSS coords multiply by 2. Measured
// 2026-11-04 on this host (dark pixels, all channels < 110, inside the control
// border, x 11..148, rows y+5..y+17; span in CSS px):
//
//   row  markup                                  goosie        chromium
//   a    label="  La  Bel  ", text "hidden text A"  span 35.0   span 39.0   ("La Bel")
//   b    label="   ",         text "hidden text B"  BLANK       BLANK
//   c    text "   Wa  sy   "                        span 35.0   span 36.0   ("Wa sy")
//   d    label="Label Wins",  text "Text D"         span 65.5   span 64.0   ("Label Wins")
//
// Text starts at x=11 in Goosie and x=13-14 in Chromium, the engine-wide control
// padding difference, and the spans differ by the engine-wide font-metric
// difference; nothing here shows a 13-character "hidden text" string, which is
// what the pre-slice paint did.
//
// Nothing in test/ quotes a rule from memory that this script has not printed.
const { chromium } = require('playwright');
const path = require('path');
const fs = require('fs');

const OUT = process.argv[2] || '/tmp/optvis/chromium.png';
const html = `<!doctype html>
<html>
  <head><meta charset="utf-8"><style>body{margin:0;font-family:sans-serif;font-size:14px;background:#fff}select{width:160px;height:22px;display:block;margin:6px 0 0 8px}</style></head>
  <body>
    <select id="a"><option value="x" label="  La  Bel  ">hidden text A</option><option value="z">Zeta</option></select>
    <select id="b"><option value="y" label="   ">hidden text B</option><option value="z">Zeta</option></select>
    <select id="c"><option>   Wa  sy   </option><option value="z">Zeta</option></select>
    <select id="d"><option value="k" label="Label Wins">Text D</option><option value="z">Zeta</option></select>
  </body>
</html>`;

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 400, height: 140 }, deviceScaleFactor: 2 });
  fs.mkdirSync(path.dirname(OUT), { recursive: true });
  const file = '/tmp/optvis/fixture.html';
  fs.writeFileSync(file, html);
  await page.goto('file://' + file);
  const shown = await page.evaluate(() =>
    [...document.querySelectorAll('select')].map(s => ({
      id: s.id,
      box: (r => ({ x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) }))(s.getBoundingClientRect()),
    })));
  console.log('BOXES ' + JSON.stringify(shown));
  await page.screenshot({ path: OUT });
  await browser.close();
})();
