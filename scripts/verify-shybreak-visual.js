// GOOSIE-SHYBREAK-001 visual gate: render the overflowing Latin SHY/ZWSP
// fixtures with Goosie's headless raster path and with Chromium, then compare
// ink-band structure. Both engines can draw these letters, so a missing wrap
// (one fat band) or a missing hyphen (first band shorter by a hyphen) is
// visible in the PNG.
//
// Run: node scripts/verify-shybreak-visual.js
// (needs playwright chromium, `go build -o bin/goosie-headless ./cmd/goosie-headless`)
const { chromium } = require('playwright');
const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const ROOT = path.resolve(__dirname, '..');
const FIXTURE = '/tmp/shybreak-visual.html';
const REF_PNG = '/tmp/shybreak-chromium.png';
const GOO_PNG = '/tmp/shybreak-goosie.png';
const VIEWPORT = { width: 460, height: 280 };

fs.writeFileSync(
  FIXTURE,
  '<!doctype html><html><head><meta charset="utf-8"><style>' +
    'body{margin:0;font:16px/1.2 Times,serif;color:#000;background:#fff}' +
    '#shy,#zws,#fit{position:absolute;left:20px;width:200px;margin:0}' +
    '#shy{top:20px}' +
    '#zws{top:100px}' +
    '#fit{top:180px}' +
    '</style></head><body>' +
    '<p id="shy">' + 'a'.repeat(42) + '&shy;' + 'b'.repeat(48) + '</p>' +
    '<p id="zws">' + 'a'.repeat(42) + '&#8203;' + 'b'.repeat(48) + '</p>' +
    '<p id="fit">aa&shy;bb</p>' +
    '</body></html>\n'
);

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.goto('file://' + FIXTURE);
  await page.screenshot({ path: REF_PNG, clip: { x: 0, y: 0, width: VIEWPORT.width, height: VIEWPORT.height } });

  execFileSync(path.join(ROOT, 'bin', 'goosie-headless'), [
    '-in', FIXTURE,
    '-out', GOO_PNG,
    '-width', String(VIEWPORT.width),
    '-height', String(VIEWPORT.height),
    '-dpr', '1',
  ], { cwd: ROOT, stdio: 'inherit' });

  const result = await page.evaluate(async ([refB64, gooB64]) => {
    const load = (b64) =>
      new Promise((res, rej) => {
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
        img.src = 'data:image/png;base64,' + b64;
      });
    const dark = (im, x, y) => {
      const i = (y * im.w + x) * 4;
      return im.d[i] < 128 && im.d[i + 1] < 128 && im.d[i + 2] < 128;
    };
    const bands = (im, x0, x1, y0, y1) => {
      const rows = [];
      for (let y = y0; y < y1; y++) {
        let n = 0;
        for (let x = x0; x < x1; x++) if (dark(im, x, y)) n++;
        if (n > 4) rows.push({ y, n });
      }
      const out = [];
      for (const row of rows) {
        const last = out[out.length - 1];
        if (last && row.y === last.y1) {
          last.y1 = row.y + 1;
          last.n += row.n;
        } else {
          out.push({ y0: row.y, y1: row.y + 1, n: row.n });
        }
      }
      return out;
    };
    const ref = await load(refB64);
    const goo = await load(gooB64);
    // x extends past the 200px box: the a-run overflows (~374px) and the
    // taken hyphen sits at its tail (~394px). y windows are stacked so one
    // fixture's overflow cannot paint into another's band count.
    const regions = {
      shy: [20, 450, 20, 90],
      zws: [20, 450, 100, 170],
      fit: [20, 120, 180, 220],
    };
    const report = {};
    let ok = true;
    for (const [name, box] of Object.entries(regions)) {
      const a = bands(ref, ...box);
      const b = bands(goo, ...box);
      report[name] = {
        chromium: a.map((x) => `${x.y0}-${x.y1}:${x.n}`),
        goosie: b.map((x) => `${x.y0}-${x.y1}:${x.n}`),
      };
      if (a.length !== b.length) ok = false;
    }
    // SHY and ZWSP must wrap (two ink bands). The fitting SHY must not.
    if ((report.shy.chromium.length < 2) || (report.zws.chromium.length < 2) || (report.fit.chromium.length !== 1)) {
      ok = false;
    }
    if ((report.shy.goosie.length < 2) || (report.zws.goosie.length < 2) || (report.fit.goosie.length !== 1)) {
      ok = false;
    }
    return { ok, report };
  }, [
    fs.readFileSync(REF_PNG).toString('base64'),
    fs.readFileSync(GOO_PNG).toString('base64'),
  ]);

  console.log(JSON.stringify(result.report, null, 2));
  if (!result.ok) {
    console.error('VISUAL FAIL');
    process.exit(1);
  }
  console.log('VISUAL OK');
  await browser.close();
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
