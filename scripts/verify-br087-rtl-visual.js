// BR-087 visual gate for bidi: render RTL lines with Goosie's own headless
// raster path and with Chromium, then compare where the ink sits inside each
// box.
//
// What pixels can prove here is the ANCHOR: a right-to-left line with no
// text-align must end its ink on the content box's right edge, and
// text-align:left must pull the same line back to the left edge. That is a
// geometry claim, so both engines' renders are directly comparable.
//
// What pixels cannot prove is the glyph-level PERMUTATION: the two engines
// resolve Arabic/Hebrew to different font faces, so ink shapes differ for
// reasons unrelated to bidi. Visual order is measured against Chromium's
// per-character client rectangles instead, by scripts/verify-br087-bidi.js,
// and pinned by the BR-087 layout/paint oracles.
//
// Run: node scripts/verify-br087-rtl-visual.js  (needs playwright chromium and
// `go build -o bin/goosie-headless ./cmd/goosie-headless`).
const { chromium } = require('playwright');
const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const ROOT = path.resolve(__dirname, '..');
const FIXTURE = '/tmp/br087-rtl.html';
const REF_PNG = '/tmp/br087-rtl-chromium.png';
const GOO_PNG = '/tmp/br087-rtl-goosie.png';
const VIEWPORT = { width: 900, height: 320 };

// Each case is one 200px box whose left edge and top are fixed below, so the
// content box is known without reading any engine's idea of a layout tree.
const CASES = [
  { id: 'rtl_latin', x: 20, y: 20, dir: 'rtl', align: null, text: 'abc def' },
  { id: 'rtl_hebrew', x: 300, y: 20, dir: 'rtl', align: null, text: '&#1513;&#1500;&#1493;&#1501; &#1506;&#1493;&#1500;&#1491;' },
  { id: 'rtl_mixed', x: 580, y: 20, dir: 'rtl', align: null, text: '&#1513;&#1500;&#1493;&#1501; abc' },
  { id: 'rtl_align_left', x: 20, y: 100, dir: 'rtl', align: 'left', text: '&#1513;&#1500;&#1493;&#1501; &#1506;&#1493;&#1500;&#1491;' },
  { id: 'ltr_control', x: 300, y: 100, dir: 'ltr', align: null, text: 'abc def' },
  { id: 'rtl_numeric', x: 580, y: 100, dir: 'rtl', align: null, text: '123 456' },
];

function fixture() {
  const boxes = CASES.map((c) => {
    const align = c.align ? `text-align:${c.align};` : '';
    return `<div id="${c.id}" dir="${c.dir}" style="position:absolute;left:${c.x}px;top:${c.y}px;width:200px;${align}margin:0">${c.text}</div>`;
  }).join('');
  return '<!doctype html><html><head><meta charset="utf-8"><style>' +
    'body{margin:0;font:16px/1.2 Arial,sans-serif;color:#000;background:#fff}' +
    '</style></head><body>' + boxes + '</body></html>\n';
}

fs.writeFileSync(FIXTURE, fixture());

// inkProfile returns, for the given column range and row range, the leftmost
// and rightmost inked column, the inked-pixel count and the ink centre of mass
// normalised inside the box (0 = box left edge, 1 = box right edge).
async function profiles(page, png, boxTop) {
  const b64 = fs.readFileSync(png).toString('base64');
  return page.evaluate(async ([data, boxes]) => {
    const im = await new Promise((res, rej) => {
      const img = new Image();
      img.onload = () => {
        const c = document.createElement('canvas');
        c.width = img.width;
        c.height = img.height;
        const ctx = c.getContext('2d');
        ctx.drawImage(img, 0, 0);
        res({ d: ctx.getImageData(0, 0, img.width, img.height).data, w: img.width, h: img.height });
      };
      img.onerror = rej;
      img.src = 'data:image/png;base64,' + data;
    });
    const dark = (x, y) => {
      const i = (y * im.w + x) * 4;
      return im.d[i] < 200 && im.d[i + 1] < 200 && im.d[i + 2] < 200;
    };
    const out = {};
    for (const b of boxes) {
      let first = -1, last = -1, mass = 0, moment = 0;
      for (let x = b.x; x < b.x + b.w; x++) {
        let n = 0;
        for (let y = b.y; y < b.y + b.h; y++) if (dark(x, y)) n++;
        if (n === 0) continue;
        if (first < 0) first = x;
        last = x;
        mass += n;
        moment += n * (x - b.x);
      }
      out[b.id] = first < 0
        ? { empty: true }
        : { first, last, mass, com: moment / mass / b.w };
    }
    return out;
  }, [b64, CASES.map((c) => ({ id: c.id, x: c.x, y: c.y + boxTop, w: 200, h: 60 }))]);
}

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT });
  await page.goto('file://' + FIXTURE);
  await page.screenshot({ path: REF_PNG, clip: { x: 0, y: 0, width: VIEWPORT.width, height: VIEWPORT.height } });
  execFileSync(path.join(ROOT, 'bin', 'goosie-headless'), [
    '-in', FIXTURE, '-out', GOO_PNG,
    '-width', String(VIEWPORT.width), '-height', String(VIEWPORT.height), '-dpr', '1',
  ], { cwd: ROOT, stdio: 'inherit' });

  // Vertical text position differs between the engines by a few pixels
  // (baseline model), so probe a band tall enough to contain whichever row
  // either engine inked, and report the box-relative numbers.
  const REF = await profiles(page, REF_PNG, 0);
  const GOO = await profiles(page, GOO_PNG, 0);
  await browser.close();

  const EDGE_TOL = 6;   // px: the anchor must land on the same edge
  const COM_TOL = 0.06; // normalised centre-of-mass drift
  let ok = true;
  for (const c of CASES) {
    const r = REF[c.id], g = GOO[c.id];
    const line = (v) => (v && !v.empty
      ? `first=${v.first} last=${v.last} mass=${v.mass} com=${v.com.toFixed(3)}`
      : 'NO INK');
    const rightEdge = c.x + 200, leftEdge = c.x;
    const anchoredRight = c.dir === 'rtl' && !c.align;
    const anchoredLeft = !anchoredRight;
    let fail = false;
    if (!r || r.empty) { fail = true; }
    else if (!g || g.empty) { fail = true; }
    else {
      if (anchoredRight && Math.abs(g.last - r.last) > EDGE_TOL) fail = true;
      if (anchoredLeft && Math.abs(g.first - r.first) > EDGE_TOL) fail = true;
      if (Math.abs(g.com - r.com) > COM_TOL) fail = true;
    }
    ok = ok && !fail;
    console.log(
      `${fail ? 'FAIL' : 'ok  '} ${c.id.padEnd(14)} box=${leftEdge}..${rightEdge} ` +
      `${anchoredRight ? 'anchor=right' : 'anchor=left '} ` +
      `chromium[${line(r)}] goosie[${line(g)}]`
    );
  }
  console.log(ok
    ? 'VISUAL OK: RTL lines ink the content box right edge and LTR/left-aligned lines ink the left edge, in both engines'
    : 'VISUAL FAIL: the inked edge or the ink distribution disagrees with Chromium');
  process.exitCode = ok ? 0 : 1;
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
