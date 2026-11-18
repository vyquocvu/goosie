// Measures the ::placeholder decoration and box-model law on the reference
// engine and on this engine, over one identical document, so the oracle is
// written from what Chromium paints rather than from what the cascade stores.
//
//   node scripts/verify-placeholder-decoration.js
//
// Both engines render the same file into the same viewport at dpr 1. Per case the
// script prints the Chromium computed style of the pseudo-element and then, for
// each device row of the host rect, the ink class and x-extent in both engines.
// Colour alone
// cannot separate every class (the chrome border is rgb(118,118,118), the
// placeholder ink rgb(117,117,117)), so the grey count is confined to the rows
// strictly inside the control while the saturated classes scan the full height.

const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');
const zlib = require('zlib');
const { execFileSync } = require('child_process');

const DOC = '/tmp/goosie-placeholder/document.html';
const REF_PNG = '/tmp/goosie-placeholder/chromium.png';
const GOO_PNG = '/tmp/goosie-placeholder/goosie.png';
const ROOT = path.resolve(__dirname, '..');

const RED = 'rgb(255, 0, 0)';
const CASES = [
  { id: 'plain', css: '' },
  { id: 'ul-auto', css: `text-decoration: underline solid ${RED}` },
  { id: 'ul-gray', css: 'text-decoration: underline' },
  { id: 'ul-1px', css: `text-decoration: underline solid ${RED}; text-decoration-thickness: 1px` },
  { id: 'ul-3px', css: `text-decoration: underline solid ${RED}; text-decoration-thickness: 3px` },
  { id: 'ul-5px', css: `text-decoration: underline solid ${RED}; text-decoration-thickness: 5px` },
  { id: 'ul-off2', css: `text-decoration: underline solid ${RED}; text-underline-offset: 2px` },
  { id: 'ul-off4', css: `text-decoration: underline solid ${RED}; text-underline-offset: 4px` },
  { id: 'over-red', css: `text-decoration: overline solid ${RED}` },
  { id: 'through-red', css: `text-decoration: line-through solid ${RED}` },
  { id: 'dashed-red', css: `text-decoration: underline dashed ${RED}` },
  { id: 'dotted-red', css: `text-decoration: underline dotted ${RED}` },
  { id: 'double-red', css: `text-decoration: underline double ${RED}` },
  { id: 'wavy-red', css: `text-decoration: underline wavy ${RED}` },
  { id: 'border-pad', css: 'border: 2px solid rgb(0, 0, 255); padding: 2px' },
  { id: 'border-only', css: 'border: 2px solid rgb(0, 0, 255)' },
  { id: 'bgcolor-green', css: 'background-color: rgb(0, 255, 0)' },
  // BR-321 recorded Chromium as dropping a pseudo background. These two rows
  // separate the shown state from the declaration: a field with no placeholder
  // attribute has no placeholder box at all, so a `rgba(0,0,0,0)` reading there
  // says nothing about a field that is showing one.
  { id: 'bg-noshown', css: 'background-color: rgb(0, 255, 0)', value: 'HELLOWORLD' },
  { id: 'bg-noshown-empty', css: 'background-color: rgb(0, 255, 0)', noattr: true },
  { id: 'color-red', css: 'color: rgb(200, 0, 0)' },
  { id: 'host-deco', css: '', host: `text-decoration: underline solid ${RED}` },
  { id: 'host-deco-value', css: '', value: 'HELLOWORLD', host: `text-decoration: underline solid ${RED}` },
  { id: 'host-blue-pseudo-none', css: 'text-decoration: none', host: 'text-decoration: underline solid rgb(0, 0, 255)' },
  { id: 'host-lt-pseudo-ul', css: `text-decoration: underline solid ${RED}`, host: 'text-decoration: line-through solid rgb(0, 0, 255)' },
  { id: 'ref-div', tag: 'div', text: 'HELLOWORLD', host: `text-decoration: underline solid ${RED}` },
  // Cascade order within one declaration block: a shorthand resets the longhands
  // it covers, so which of the two wins is decided by source order, not by
  // specificity. Measured here because the engine stores both in one struct.
  { id: 'order-td-last', css: '', host: `text-decoration-color: rgb(0, 0, 255); text-decoration: underline solid ${RED}` },
  { id: 'order-td-first', css: '', host: `text-decoration: underline solid ${RED}; text-decoration-color: rgb(0, 0, 255)` },
  // The same source-order question for the OTHER longhand the shorthand covers.
  // Thickness is separable by row: the reference draws auto one device row
  // above an explicit 3px, so which of the two declarations survived shows up
  // as the row the bar lands on rather than as a colour.
  { id: 'order-thick-last', css: '', host: `text-decoration-thickness: 3px; text-decoration: underline solid ${RED}` },
  { id: 'order-thick-first', css: '', host: `text-decoration: underline solid ${RED}; text-decoration-thickness: 3px` },
  // `text-underline-offset` is NOT one of the longhands the shorthand covers, so
  // the reference must keep it across a following shorthand. The bar row shows the
  // answer: auto sits one row below the 2px offset Chromium measured earlier.
  { id: 'order-off-last', css: '', host: `text-underline-offset: 2px; text-decoration: underline solid ${RED}` },
  // The longhand family, which the engine parses no branch of.
  { id: 'longhand-line', css: '', host: 'text-decoration-color: rgb(0, 0, 255); text-decoration-line: underline' },
  { id: 'longhand-split', css: '', host: `text-decoration-line: underline; text-decoration-style: dashed; text-decoration-color: ${RED}; text-decoration-thickness: 3px` },
  // The other two control families: a button label and a textarea each reach the
  // display list through their own paint path, so the extent law is measured per
  // family instead of assumed from the text field.
  { id: 'btn-deco', tag: 'button', text: 'HELLOWORLD', host: `text-decoration: underline solid ${RED}` },
  { id: 'textarea-deco', tag: 'textarea', text: 'HELLOWORLD', host: `text-decoration: underline solid ${RED}` },
];

const ROW = 30;
const VIEWPORT = { width: 900, height: 20 + CASES.length * ROW + 30 };

const PAGE = `<!doctype html>
<meta charset="utf-8">
<style>
  body { margin: 0; background: #fff; font: 16px sans-serif; }
  .row { position: absolute; left: 20px; width: 240px; }
${CASES.map((c, i) => `  #${c.id} { top: ${20 + i * ROW}px; }
  #${c.id}::placeholder { ${c.css}; }${c.host ? `\n  #${c.id} { ${c.host}; }` : ''}`).join('\n')}
</style>
${CASES.map((c) => {
  const tag = c.tag || 'input';
  const attrs = `id="${c.id}" class="row"` + (c.value ? ` value="${c.value}"` : '') +
    (tag === 'input' && !c.value && !c.noattr ? ' placeholder="HELLOWORLD"' : '');
  return `<${tag} ${attrs}>${c.text || ''}</${tag}>`;
}).join('\n')}
`;

function loadPNG(file) {
  const buf = fs.readFileSync(file);
  if (buf.readUInt32BE(0) !== 0x89504e47) throw new Error('not a PNG: ' + file);
  let off = 8, w = 0, h = 0, bitDepth = 0, colorType = 0;
  const idat = [];
  while (off < buf.length) {
    const len = buf.readUInt32BE(off);
    const type = buf.toString('ascii', off + 4, off + 8);
    const data = buf.subarray(off + 8, off + 8 + len);
    if (type === 'IHDR') {
      w = data.readUInt32BE(0); h = data.readUInt32BE(4);
      bitDepth = data[8]; colorType = data[9];
      if (bitDepth !== 8 || (colorType !== 6 && colorType !== 2)) {
        throw new Error('unsupported PNG ' + bitDepth + '/' + colorType);
      }
    } else if (type === 'IDAT') idat.push(data);
    off += 12 + len;
  }
  const raw = zlib.inflateSync(Buffer.concat(idat));
  const bpp = colorType === 6 ? 4 : 3;
  const stride = w * bpp;
  const out = Buffer.alloc(h * stride);
  let pos = 0;
  for (let y = 0; y < h; y++) {
    const filter = raw[pos++];
    const row = raw.subarray(pos, pos + stride);
    pos += stride;
    const dst = out.subarray(y * stride, (y + 1) * stride);
    for (let x = 0; x < stride; x++) {
      const a = x >= bpp ? dst[x - bpp] : 0;
      const b = y > 0 ? out[(y - 1) * stride + x] : 0;
      const c = x >= bpp && y > 0 ? out[(y - 1) * stride + x - bpp] : 0;
      let v = row[x];
      if (filter === 1) v += a;
      else if (filter === 2) v += b;
      else if (filter === 3) v += Math.floor((a + b) / 2);
      else if (filter === 4) {
        const p = a + b - c, pa = Math.abs(p - a), pb = Math.abs(p - b), pc = Math.abs(p - c);
        v += (pa <= pb && pa <= pc) ? a : (pb <= pc ? b : c);
      }
      dst[x] = v & 0xff;
    }
  }
  return { w, h, bpp, data: out };
}

function px(png, x, y) {
  const i = y * png.w * png.bpp + x * png.bpp;
  return [png.data[i], png.data[i + 1], png.data[i + 2]];
}

// Pixel classification. The input chrome border is rgb(118,118,118) and the
// placeholder text and default decoration colour is rgb(117,117,117), so colour
// alone cannot separate them: every scan is confined to a window four device px
// inside the host rect, which excludes the chrome border on all four sides.
function classify(c) {
  if (c[0] > 150 && c[1] < 90 && c[2] < 90) return 'R';
  if (c[2] > 150 && c[0] < 90 && c[1] < 90) return 'B';
  if (c[1] > 150 && c[0] < 120 && c[2] < 120) return 'G';
  if (Math.abs(c[0] - 117) <= 22 && Math.abs(c[1] - 117) <= 22 && Math.abs(c[2] - 117) <= 22) return 'Y';
  if (c[0] < 80 && c[1] < 80 && c[2] < 80) return 'K';
  return null;
}

function profile(png, box) {
  const rows = [];
  for (let y = box.y; y < box.y + box.h; y++) {
    const dy = y - box.y;
    // The gray class is only readable inside the field: the chrome border rows
    // (the top two and the bottom two of the 21px control) are rgb(118,118,118),
    // one step from the placeholder ink, and cannot be told apart by colour.
    const grayOK = dy >= 3 && dy <= box.h - 4;
    const tally = {};
    for (let x = box.x + 4; x < box.x + box.w - 4; x++) {
      const c = classify(px(png, x, y));
      if (c === 'Y' && !grayOK) continue;
      if (!c) continue;
      let t = tally[c];
      if (!t) tally[c] = t = { n: 0, x0: x, x1: x };
      t.n++;
      if (x < t.x0) t.x0 = x;
      if (x > t.x1) t.x1 = x;
    }
    const parts = Object.keys(tally).sort().map((c) => `${c}=${tally[c].n}@${tally[c].x0 - box.x}-${tally[c].x1 - box.x}`);
    if (parts.length) rows.push(`      dy=+${String(dy).padStart(2)} ${parts.join(' ')}`);
  }
  return rows;
}

async function main() {
  fs.mkdirSync('/tmp/goosie-placeholder', { recursive: true });
  fs.writeFileSync(DOC, PAGE);

  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.goto('file://' + DOC, { waitUntil: 'load' });

  const ids = CASES.map((c) => c.id);
  const measured = await page.evaluate((list) => list.map((id) => {
    const el = document.getElementById(id);
    const cs = getComputedStyle(el, '::placeholder');
    const hs = getComputedStyle(el);
    const box = el.getBoundingClientRect();
    return {
      id,
      x: Math.round(box.x), y: Math.round(box.y), w: Math.round(box.width), h: Math.round(box.height),
      meta: `${id.padEnd(13)} host=${Math.round(box.width)}x${Math.round(box.height)} ` +
        `color=${cs.color} opacity=${cs.opacity} line=${cs.textDecorationLine} ` +
        `style=${cs.textDecorationStyle} thick=${cs.textDecorationThickness} ` +
        `tdc=${cs.textDecorationColor} off=${cs.textUnderlineOffset} ` +
        `border=${cs.borderTopWidth}/${cs.borderTopStyle}/${cs.borderTopColor} ` +
        `pad=${cs.paddingTop}/${cs.paddingLeft} display=${cs.display} ` +
        `bg=${cs.backgroundColor} fs=${cs.fontSize} ` +
        `| HOST line=${hs.textDecorationLine} style=${hs.textDecorationStyle} ` +
        `thick=${hs.textDecorationThickness} tdc=${hs.textDecorationColor} off=${hs.textUnderlineOffset}`,
    };
  }), ids);

  await page.screenshot({ path: REF_PNG, clip: { x: 0, y: 0, width: VIEWPORT.width, height: VIEWPORT.height } });
  await browser.close();

  const goosie = path.join(ROOT, 'bin', 'goosie');
  execFileSync(goosie, ['-backend', 'headless', '-width', String(VIEWPORT.width),
    '-height', String(VIEWPORT.height), '-dpr', '1', '-screenshot', '-out', GOO_PNG,
    '-url', 'file://' + DOC], { stdio: 'inherit' });

  const ref = loadPNG(REF_PNG), got = loadPNG(GOO_PNG);
  console.log(`viewport=${VIEWPORT.width}x${VIEWPORT.height} dpr=1 chromium=${REF_PNG} goosie=${GOO_PNG}`);
  console.log('legend: dy=+N rows below the host box top; R/red B/blue G/green Y/placeholder-gray K/black, count@x0-x1');
  console.log('');
  for (const m of measured) {
    console.log(m.meta);
    console.log('  CHROMIUM');
    for (const l of profile(ref, m)) console.log(l);
    console.log('  GOOSIE');
    for (const l of profile(got, m)) console.log(l);
  }
}

main().catch((e) => { console.error(e); process.exit(1); });
