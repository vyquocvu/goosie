#!/usr/bin/env node
// Measures which pseudo-box insets move the placeholder ink, on the reference
// and on this engine, in one document. Run: node scripts/verify-placeholder-pseudo-insets.js
const { chromium } = require('playwright');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const zlib = require('node:zlib');

const OUT_DIR = '/tmp/goosie-placeholder-insets';
const DOC = path.join(OUT_DIR, 'document.html');
const REF_PNG = path.join(OUT_DIR, 'chromium.png');
const GOO_PNG = path.join(OUT_DIR, 'goosie.png');
const ROOT = path.resolve(__dirname, '..');

const CASES = [
  { id: 'none', css: '' },
  { id: 'padTop1', css: 'padding-top: 1px' },
  { id: 'padTop4', css: 'padding-top: 4px' },
  { id: 'padTop8', css: 'padding-top: 8px' },
  { id: 'padBottom8', css: 'padding-bottom: 8px' },
  { id: 'borderTop2', css: 'border-top: 2px solid rgb(0, 0, 255)' },
  { id: 'border2padTop8', css: 'border: 2px solid rgb(0, 0, 255); padding-top: 8px' },
  { id: 'padLeft4', css: 'padding-left: 4px' },
  { id: 'padLeft1', css: 'padding-left: 1px' },
  { id: 'padLeftHalf', css: 'padding-left: 0.5px' },
  { id: 'borderLeftHalf', css: 'border-left: 0.5px solid rgb(0, 0, 255)' },
  { id: 'deco-none', css: 'text-decoration: underline solid rgb(255, 0, 0)' },
  { id: 'deco-padTop8', css: 'text-decoration: underline solid rgb(255, 0, 0); padding-top: 8px' },
  { id: 'deco-borderTop2', css: 'text-decoration: underline solid rgb(255, 0, 0); border-top: 2px solid rgb(0, 0, 255)' },
];

const ROW = 34;
const FIELD = { w: 248, h: 21, x: 40 };
const VIEWPORT = { width: 900, height: 20 + CASES.length * ROW + 30 };

const PAGE = `<!doctype html>
<meta charset="utf-8">
<style>
  body { margin: 0; background: #fff; font: 16px sans-serif; }
  input { position: absolute; left: ${FIELD.x}px; width: ${FIELD.w}px; height: ${FIELD.h}px; box-sizing: border-box; }
${CASES.map((c, i) => `  #${c.id} { top: ${20 + i * ROW}px; }\n  #${c.id}::placeholder { ${c.css}; }`).join('\n')}
</style>
${CASES.map((c) => `<input id="${c.id}" placeholder="HELLOWORLD">`).join('\n')}
`;

function loadPNG(file) {
  const buf = fs.readFileSync(file);
  if (buf.readUInt32BE(0) !== 0x89504e47) throw new Error('not a PNG: ' + file);
  let off = 8, w = 0, h = 0, colorType = 0;
  const idat = [];
  while (off < buf.length) {
    const len = buf.readUInt32BE(off);
    const type = buf.toString('ascii', off + 4, off + 8);
    const data = buf.subarray(off + 8, off + 8 + len);
    if (type === 'IHDR') { w = data.readUInt32BE(0); h = data.readUInt32BE(4); colorType = data[9]; }
    else if (type === 'IDAT') idat.push(data);
    off += 12 + len;
  }
  const bpp = colorType === 6 ? 4 : 3;
  const raw = zlib.inflateSync(Buffer.concat(idat));
  const out = Buffer.alloc(w * h * bpp);
  let pos = 0;
  for (let y = 0; y < h; y++) {
    const filter = raw[pos++];
    const line = raw.subarray(pos, pos + w * bpp);
    pos += w * bpp;
    for (let x = 0; x < w * bpp; x++) {
      const a = x >= bpp ? out[y * w * bpp + x - bpp] : 0;
      const b = y > 0 ? out[(y - 1) * w * bpp + x] : 0;
      const c = x >= bpp && y > 0 ? out[(y - 1) * w * bpp + x - bpp] : 0;
      let v = line[x];
      if (filter === 1) v += a;
      else if (filter === 2) v += b;
      else if (filter === 3) v += (a + b) >> 1;
      else if (filter === 4) {
        const p = a + b - c, pa = Math.abs(p - a), pb = Math.abs(p - b), pc = Math.abs(p - c);
        v += pa <= pb && pa <= pc ? a : pb <= pc ? b : c;
      }
      out[y * w * bpp + x] = v & 0xff;
    }
  }
  return { data: out, w, h, bpp };
}

function px(png, x, y) {
  const i = y * png.w * png.bpp + x * png.bpp;
  return [png.data[i], png.data[i + 1], png.data[i + 2]];
}

// The control chrome is rgb(118,118,118) and the placeholder ink rgb(117,117,117),
// one step apart, so the grey class is only counted inside the field.
function isChrome(c) { return Math.abs(c[0] - 118) <= 2 && Math.abs(c[1] - 118) <= 2 && Math.abs(c[2] - 118) <= 2; }
function isInk(c) { return c[0] < 200 && c[1] < 200 && c[2] < 200 && !isChrome(c); }
function isRed(c) { return c[0] > 150 && c[1] < 90 && c[2] < 90; }

function read(png, box) {
  const rows = [], barRows = [];
  let firstCol = -1;
  for (let dy = 3; dy <= box.h - 4; dy++) {
    let n = 0, bar = 0;
    for (let x = box.x + 2; x < box.x + box.w - 2; x++) {
      const c = px(png, x, box.y + dy);
      if (isRed(c)) { bar++; if (!n) n = 0; continue; }
      if (isInk(c)) {
        n++;
        if (firstCol < 0 || x - box.x < firstCol) firstCol = x - box.x;
      }
    }
    if (n > 2) rows.push(dy);
    if (bar > 2) barRows.push(dy);
  }
  return { rows, barRows, firstCol };
}

async function main() {
  fs.mkdirSync(OUT_DIR, { recursive: true });
  fs.writeFileSync(DOC, PAGE);

  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.goto('file://' + DOC, { waitUntil: 'load' });
  const measured = await page.evaluate((ids) => ids.map((id) => {
    const el = document.getElementById(id);
    const cs = getComputedStyle(el, '::placeholder');
    const b = el.getBoundingClientRect();
    return { id, x: Math.round(b.x), y: Math.round(b.y), w: Math.round(b.width), h: Math.round(b.height),
      meta: `pad=${cs.paddingTop}/${cs.paddingRight}/${cs.paddingBottom}/${cs.paddingLeft} ` +
        `border=${cs.borderTopWidth}/${cs.borderRightWidth}/${cs.borderBottomWidth}/${cs.borderLeftWidth} ` +
        `td=${cs.textDecorationLine} tdc=${cs.textDecorationColor}` };
  }), CASES.map((c) => c.id));
  await page.screenshot({ path: REF_PNG, clip: { x: 0, y: 0, width: VIEWPORT.width, height: VIEWPORT.height } });
  await browser.close();

  const goosie = path.join(ROOT, 'bin', 'goosie');
  execFileSync(goosie, ['-backend', 'headless', '-width', String(VIEWPORT.width),
    '-height', String(VIEWPORT.height), '-dpr', '1', '-screenshot', '-out', GOO_PNG,
    '-url', 'file://' + DOC], { stdio: 'inherit' });

  const ref = loadPNG(REF_PNG), got = loadPNG(GOO_PNG);
  const reads = measured.map((m) => ({ m, ref: read(ref, m), got: read(got, m) }));
  const zero = reads.find((r) => r.m.id === 'none');
  const delta = (base, rows) => (rows.length && base.rows.length ? rows[0] - base.rows[0] : null);
  console.log(`viewport=${VIEWPORT.width}x${VIEWPORT.height} dpr=1 chromium=${REF_PNG} goosie=${GOO_PNG}`);
  console.log('deltas are relative to this engine\'s own "none" case: row=first inked row shift, col=first inked column shift');
  console.log('a case MATCHes when both engines moved the ink the same way; the engines\' absolute rows differ by the constant control-font offset, which is not this law');
  console.log('');
  for (const r of reads) {
    const refRow = delta(zero.ref, r.ref.rows), gotRow = delta(zero.got, r.got.rows);
    const refCol = r.ref.firstCol - zero.ref.firstCol, gotCol = r.got.firstCol - zero.got.firstCol;
    const refBar = delta(zero.ref, r.ref.barRows), gotBar = delta(zero.got, r.got.barRows);
    const same = refRow === gotRow && refCol === gotCol;
    console.log(`${r.m.id.padEnd(16)} ${r.m.meta}`);
    console.log(`  CHROMIUM rows=${r.ref.rows.join(',')} bar=${r.ref.barRows.join(',')} first=${r.ref.firstCol} | delta row=${refRow} col=${refCol}`);
    console.log(`  GOOSIE   rows=${r.got.rows.join(',')} bar=${r.got.barRows.join(',')} first=${r.got.firstCol} | delta row=${gotRow} col=${gotCol}` +
      `  ${same ? 'MATCH' : 'DIFFERS'} (bar delta ref=${refBar} got=${gotBar})`);
  }
}

main().catch((e) => { console.error(e); process.exit(1); });
