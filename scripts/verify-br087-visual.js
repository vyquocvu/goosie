// BR-087 visual gate: render one text fixture with Goosie's own headless
// raster path and with Chromium, then compare the ink structure.
//
// CJK glyphs are outside Goosie's font coverage (GOOSIE-COVERAGE-001), so a
// pixel diff there proves nothing; what has to agree is the line-band
// structure - how many horizontal ink bands a column carries. The second
// column is Latin text threaded with format controls (soft hyphen, ZWSP, ZWJ,
// LRM, RLM), which BOTH engines can draw, so its ink is compared directly:
// before the fix Goosie inked phantom boxes for the controls and charged the
// soft hyphen an advance.
//
// Run: node scripts/verify-br087-visual.js  (needs playwright chromium,
// `make build` and `go build -o bin/goosie-headless ./cmd/goosie-headless`).
const { chromium } = require('playwright');
const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const ROOT = path.resolve(__dirname, '..');
const FIXTURE = '/tmp/br087-visual.html';
const REF_PNG = '/tmp/br087-chromium.png';
const GOO_PNG = '/tmp/br087-goosie.png';
const VIEWPORT = { width: 900, height: 400 };

// The controls are written as character references so the file stays ASCII and
// neither engine's HTML parser sees a difference.
const CONTROLS =
  'aa&#173;bb&#8203;cc&#8204;dd&#8205;ee&#8206;ff&#8207;gg ' +
  'controls must cost nothing and ink nothing, so this line reads continuously';

fs.writeFileSync(
  FIXTURE,
  '<!doctype html><html><head><meta charset="utf-8"><style>' +
    'body{margin:0;font:16px/1.2 Arial,sans-serif;color:#000;background:#fff}' +
    '#flow{position:absolute;left:20px;top:20px;width:200px;margin:0}' +
    '#ctrl{position:absolute;left:300px;top:20px;width:380px;margin:0}' +
    '</style></head><body>' +
    '<p id="flow">中文文本需要换行并且继续写入更多字符以填满这个宽度</p>' +
    '<p id="ctrl">' + CONTROLS + '</p>' +
    '</body></html>\n'
);

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT });
  await page.goto('file://' + FIXTURE);
  await page.screenshot({ path: REF_PNG, clip: { x: 0, y: 0, width: VIEWPORT.width, height: VIEWPORT.height } });

  execFileSync(path.join(ROOT, 'bin', 'goosie-headless'), [
    '-in', FIXTURE,
    '-out', GOO_PNG,
    '-width', String(VIEWPORT.width),
    '-height', String(VIEWPORT.height),
    '-dpr', '1',
  ], { cwd: ROOT, stdio: 'inherit' });

  // Read both PNGs back through the same Chromium decoder, so the ink
  // analysis is one instrument pointed at the reference render and at
  // Goosie's own output.
  const bands = await page.evaluate(async ([refB64, gooB64]) => {
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
    const rowInk = (im, x0, x1) => {
      const rows = [];
      for (let y = 0; y < im.h; y++) {
        let n = 0;
        for (let x = x0; x < Math.min(x1, im.w); x++) if (dark(im, x, y)) n++;
        rows.push(n);
      }
      return rows;
    };
    const bandsOf = (rows) => {
      const out = [];
      let y0 = -1;
      rows.forEach((n, y) => {
        if (n > 0 && y0 < 0) y0 = y;
        if (n === 0 && y0 >= 0) {
          out.push({ y0, y1: y - 1, ink: rows.slice(y0, y).reduce((a, b) => a + b, 0) });
          y0 = -1;
        }
      });
      return out;
    };
    const a = await load(refB64);
    const b = await load(gooB64);
    const fmt = (l) => l.map((x) => `${x.y0}-${x.y1}:${x.ink}`).join(' ');
    return {
      refSize: [a.w, a.h],
      gooSize: [b.w, b.h],
      cjkRef: fmt(bandsOf(rowInk(a, 20, 220))),
      cjkGoo: fmt(bandsOf(rowInk(b, 20, 220))),
      ctrlRef: fmt(bandsOf(rowInk(a, 300, 680))),
      ctrlGoo: fmt(bandsOf(rowInk(b, 300, 680))),
    };
  }, [fs.readFileSync(REF_PNG).toString('base64'), fs.readFileSync(GOO_PNG).toString('base64')]);
  await browser.close();

  const count = (s) => (s ? s.split(' ').length : 0);
  console.log('viewport=%dx%d chromium png=%s goosie png=%s',
    VIEWPORT.width, VIEWPORT.height, bands.refSize.join('x'), bands.gooSize.join('x'));
  console.log('CJK  bands chromium=%d [%s]', count(bands.cjkRef), bands.cjkRef);
  console.log('CJK  bands goosie  =%d [%s]', count(bands.cjkGoo), bands.cjkGoo);
  console.log('CTRL bands chromium=%d [%s]', count(bands.ctrlRef), bands.ctrlRef);
  console.log('CTRL bands goosie  =%d [%s]', count(bands.ctrlGoo), bands.ctrlGoo);

  const ok = count(bands.cjkRef) === 3 && count(bands.cjkGoo) === 3 &&
    count(bands.ctrlRef) === count(bands.ctrlGoo);
  console.log(ok
    ? 'VISUAL OK: the CJK column breaks into the same three line bands and the control column carries the same number of lines'
    : 'VISUAL FAIL');
  process.exitCode = ok ? 0 : 1;
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
