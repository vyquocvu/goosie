#!/usr/bin/env node
// Measures which colour a border edge paints when the author never declared
// one, on the reference and on this engine, in one document.
// Run: node scripts/verify-border-currentcolor.js
const { chromium } = require('playwright');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const zlib = require('node:zlib');

const OUT_DIR = '/tmp/goosie-border-color';
const DOC = path.join(OUT_DIR, 'document.html');
const REF_PNG = path.join(OUT_DIR, 'chromium.png');
const GOO_PNG = path.join(OUT_DIR, 'goosie.png');
const ROOT = path.resolve(__dirname, '..');
const VIEWPORT = { width: 460, height: 1200 };
const ROW = 46;
const BOX = { x: 20, w: 140, h: 26 };

const BLUE = 'rgb(0, 0, 255)';
const GREEN = 'rgb(0, 128, 0)';

const CASES = [
  { id: 'solidNoColor', css: `border-width:6px; border-style:solid; color:${BLUE}` },
  { id: 'shorthandNoColor', css: 'border:6px solid; color:rgb(255, 0, 0)' },
  { id: 'topNoColor', css: `border-top:6px solid; color:${GREEN}` },
  { id: 'explicitBlue', css: `border:6px solid ${BLUE}; color:rgb(255, 0, 0)` },
  { id: 'transparentColor', css: `border-width:6px; border-style:solid; border-color:transparent; color:${BLUE}` },
  { id: 'keywordCurrent', css: 'border-width:6px; border-style:solid; border-color:currentcolor; color:rgb(0, 200, 200)' },
  { id: 'twoComp', css: `border-width:6px; border-style:solid; border-color:rgb(255, 0, 0) ${BLUE}; color:${GREEN}` },
  { id: 'threeComp', css: 'border-width:6px; border-style:solid; border-color:rgb(255, 0, 0) rgb(0, 200, 0) rgb(0, 0, 255); color:rgb(9, 9, 9)' },
  { id: 'fourComp', css: 'border-width:6px; border-style:solid; border-color:rgb(255, 0, 0) rgb(0, 200, 0) rgb(0, 0, 255) rgb(200, 200, 0); color:rgb(9, 9, 9)' },
  { id: 'blockTwo', css: `border-width:6px; border-style:solid; border-block-color:rgb(255, 0, 0) ${BLUE}; color:${GREEN}` },
  { id: 'blockOne', css: `border-width:6px; border-style:solid; border-block-color:rgb(255, 0, 0); color:${GREEN}` },
  { id: 'inlineOne', css: `border-width:6px; border-style:solid; border-inline-color:${BLUE}; color:${GREEN}` },
  { id: 'inlineTwo', css: `border-width:6px; border-style:solid; border-inline-color:rgb(255, 0, 0) ${BLUE}; color:${GREEN}` },
  { id: 'topInherit', css: `border-width:6px; border-style:solid; border-top-color:inherit; color:${GREEN}`, parentCSS: `color:${BLUE}; border-color:${BLUE}` },
  { id: 'shorthandMixed', css: `border:6px solid; border-left-color:rgb(255, 0, 255); color:${BLUE}` },
  { id: 'initialKeyword', css: 'border-width:6px; border-style:solid; border-color:initial; color:rgb(10, 20, 30)' },
  { id: 'unsetKeyword', css: 'border-width:6px; border-style:solid; border-color:unset; color:rgb(10, 20, 30)' },
  { id: 'styleNone', css: `border-width:6px; border-style:none; color:${BLUE}` },
  { id: 'inheritFromParent', css: `border-width:6px; border-style:solid; border-color:inherit; color:${GREEN}`, parentCSS: `color:${BLUE}; border-color:${BLUE}` },
  { id: 'inheritUnresolvedParent', css: `border-width:6px; border-style:solid; border-color:inherit; color:${GREEN}`, parentCSS: 'color:rgb(255, 0, 0); border-width:6px; border-style:solid' },
  { id: 'invalidAfterValid', css: `border-width:6px; border-style:solid; border-color:rgb(255, 0, 0); border-color:notacolor; color:${GREEN}` },
  { id: 'invalidOnly', css: `border-width:6px; border-style:solid; border-color:notacolor; color:${BLUE}` },
  { id: 'outlineNoColor', css: `border-width:6px; border-style:solid; border-color:transparent; outline:3px solid; color:${BLUE}` },
  { id: 'placeholderNoColor', control: true, css: 'border-width:2px; border-style:solid' },
  { id: 'placeholderOwnColor', control: true, css: 'border-width:2px; border-style:solid; color:rgb(200, 0, 0)' },
];

const rules = CASES.filter((c) => !c.control && !c.parentCSS).map((c) => `  #${c.id} { top:${20 + CASES.indexOf(c) * ROW}px; ${c.css} }`);
const childRules = CASES.filter((c) => c.parentCSS).map((c) => `  #${c.id} { left:0; top:0; width:100%; height:100%; ${c.css} }`);
const parentRules = CASES.filter((c) => c.parentCSS).map((c) => `  #parentOf_${c.id} { top:${20 + CASES.indexOf(c) * ROW}px; ${c.parentCSS} }`);
const pseudoRules = CASES.filter((c) => c.control).map((c) => `  #${c.id} { top:${20 + CASES.indexOf(c) * ROW}px; }\n  #${c.id}::placeholder { ${c.css}; }`);

const PAGE = `<!doctype html>
<meta charset="utf-8">
<style>
  body { margin:0; background:#fff; font:16px sans-serif; color:#000; }
  div { position:absolute; left:${BOX.x}px; width:${BOX.w}px; height:${BOX.h}px; box-sizing:border-box; background:#fff; }
  input { position:absolute; left:${BOX.x}px; width:${BOX.w}px; height:${BOX.h}px; box-sizing:border-box; }
${rules.join('\n')}
${childRules.join('\n')}
${parentRules.join('\n')}
${pseudoRules.join('\n')}
</style>
${CASES.map((c) => (c.control
  ? `<input id="${c.id}" placeholder="HELLOWORLD">`
  : (c.parentCSS
    ? `<div id="parentOf_${c.id}"><div id="${c.id}">TEXT</div></div>`
    : `<div id="${c.id}">TEXT</div>`))).join('\n')}
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
  if (x < 0 || y < 0 || x >= png.w || y >= png.h) return [-1, -1, -1];
  const i = y * png.w * png.bpp + x * png.bpp;
  return [png.data[i], png.data[i + 1], png.data[i + 2]];
}

function classOf(c) {
  if (c[0] < 0) return 'OOB';
  if (c[0] > 245 && c[1] > 245 && c[2] > 245) return 'white';
  return `${c[0]},${c[1]},${c[2]}`;
}

async function main() {
  fs.mkdirSync(OUT_DIR, { recursive: true });
  fs.writeFileSync(DOC, PAGE);

  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.goto('file://' + DOC, { waitUntil: 'load' });
  const measured = await page.evaluate((ids) => ids.map((id) => {
    const el = document.getElementById(id);
    const pseudo = el.tagName === 'INPUT';
    const cs = getComputedStyle(el, pseudo ? '::placeholder' : null);
    const es = getComputedStyle(el);
    const b = el.getBoundingClientRect();
    return {
      id, x: Math.round(b.x), y: Math.round(b.y), w: Math.round(b.width), h: Math.round(b.height),
      color: es.color,
      top: cs.borderTopColor, right: cs.borderRightColor, bottom: cs.borderBottomColor, left: cs.borderLeftColor,
      style: `${cs.borderTopStyle}/${cs.borderRightStyle}/${cs.borderBottomStyle}/${cs.borderLeftStyle}`,
      width: `${cs.borderTopWidth}/${cs.borderRightWidth}/${cs.borderBottomWidth}/${cs.borderLeftWidth}`,
      outline: pseudo ? '' : es.outlineColor,
    };
  }), CASES.map((c) => c.id));
  await page.screenshot({ path: REF_PNG, clip: { x: 0, y: 0, width: VIEWPORT.width, height: VIEWPORT.height } });
  await browser.close();

  const goosie = path.join(ROOT, 'bin', 'goosie');
  execFileSync(goosie, ['-backend', 'headless', '-width', String(VIEWPORT.width),
    '-height', String(VIEWPORT.height), '-dpr', '1', '-screenshot', '-out', GOO_PNG,
    '-url', 'file://' + DOC], { stdio: 'inherit' });

  const ref = loadPNG(REF_PNG), got = loadPNG(GOO_PNG);
  let differ = 0;
  measured.forEach((m, i) => {
    const c = CASES[i];
    let refInk, gooInk;
    if (c.control) {
      refInk = classOf(px(ref, m.x + 4, m.y + 4));
      gooInk = classOf(px(got, m.x + 4, m.y + 4));
    } else {
      const midX = m.x + Math.round(m.w / 2), midY = m.y + Math.round(m.h / 2);
      refInk = `${classOf(px(ref, midX, m.y + 3))},${classOf(px(ref, m.x + 3, midY))}`;
      gooInk = `${classOf(px(got, midX, m.y + 3))},${classOf(px(got, m.x + 3, midY))}`;
    }
    const same = refInk === gooInk;
    if (!same) differ++;
    console.log(`${same ? 'MATCH  ' : 'DIFFER '} ${c.id.padEnd(18)} refT=${m.top.padEnd(18)} refR=${m.right.padEnd(17)} ` +
      `refB=${m.bottom.padEnd(17)} refL=${m.left.padEnd(17)} style=${m.style.padEnd(23)} ` +
      `outline=${String(m.outline).padEnd(15)} color=${m.color.padEnd(15)} refInk=${refInk.padEnd(26)} gooInk=${gooInk}`);
  });
  console.log(`\ncases=${CASES.length} ink-differ=${differ}`);
  console.log('widths=' + measured.map((m) => `${m.id}:${m.width}`).join(' '));
  console.log('doc=' + DOC + ' ref=' + REF_PNG + ' goosie=' + GOO_PNG);
}

main().catch((e) => { console.error(e); process.exit(1); });
