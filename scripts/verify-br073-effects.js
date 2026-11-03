// BR-073 reference probe: what does Chromium report for a page whose boxes
// carry border-radius, outline (+offset) and a spread box-shadow?
//
// Prints two things for the fixture testdata/layout-parity/effects-001.html:
//   BOX  <id> x y w h        - getBoundingClientRect per id-bearing element
//   HIT  x,y -> <id>         - document.elementFromPoint per probe
// The Go oracles read the same fixture, so this script is the anchor for both
// halves of the acceptance criterion: effects must not move the border box and
// must not change what a pointer hits.
const { chromium } = require('playwright');
const path = require('path');

const FIXTURE = path.join(__dirname, '..', 'testdata', 'layout-parity', 'effects-001.html');

// Every probe is chosen against a specific rule:
//   inside a rounded corner's curve vs outside it
//   in the outline ring (outside the border box)
//   in the shadow band (outside the border box)
//   inside each border box
const POINTS = [
  [260, 50, 'radius centre'],
  [202, 22, 'radius corner outside the curve'],
  [230, 22, 'radius corner inside the curve'],
  [192, 170, 'outline ring'],
  [196, 170, 'gap between box and outline'],
  [330, 300, 'shadow band right'],
  [250, 330, 'shadow band below'],
  [260, 290, 'shadow box interior'],
  [10, 10, 'plain underlay'],
];

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 800, height: 600 }, deviceScaleFactor: 1 });
  await page.goto('file://' + FIXTURE, { waitUntil: 'load' });
  const boxes = await page.evaluate(() => {
    const out = [];
    for (const el of document.querySelectorAll('[id]')) {
      const r = el.getBoundingClientRect();
      out.push([el.id, r.x, r.y, r.width, r.height]);
    }
    return out;
  });
  for (const [id, x, y, w, h] of boxes) {
    console.log(`BOX ${id} ${Math.round(x)} ${Math.round(y)} ${Math.round(w)} ${Math.round(h)}`);
  }
  for (const [x, y, label] of POINTS) {
    const hit = await page.evaluate(
      ([x, y]) => {
        const el = document.elementFromPoint(x, y);
        return el ? (el.id || el.tagName.toLowerCase()) : 'null';
      },
      [x, y],
    );
    console.log(`HIT ${x},${y} -> ${hit}   # ${label}`);
  }
  await browser.close();
})();
