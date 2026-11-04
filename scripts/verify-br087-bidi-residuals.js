// BR-087 bidi residuals: three things the visual-order slice deliberately did
// not change, measured on Chromium 1.63 so each one can be opened as a defect
// with numbers instead of a recollection.
//
//   1. GLYPH MIRRORING - does a bracket's SHAPE flip when it resolves to an odd
//      bidi level, or only its position? Compares the ink of "(" drawn in an LTR
//      context against the same logical character in an RTL context, and against
//      that RTL ink mirrored back.
//   2. TABLE INHERITED ALIGNMENT - does an inherited `text-align: center` reach a
//      table cell's content? Uses a 300px-wide table so alignment and cell
//      padding cannot be confused, with a 300px div as the control.
//   3. `end` ACROSS A DIRECTION CHANGE - is `text-align: end` resolved from the
//      element's own direction or the direction the value was inherited with?
//
// Run: NODE_PATH=./node_modules node scripts/verify-br087-bidi-residuals.js
const { chromium } = require('playwright');

const VIEWPORT = { width: 900, height: 400 };

async function mirroring(page) {
  await page.setContent('<!doctype html><html><head><meta charset="utf-8"><style>' +
    'body{margin:0;font:64px Arial,sans-serif;color:#000;background:#fff}' +
    'div{position:absolute;height:80px;width:120px;overflow:hidden}' +
    '#a{left:0;top:0;direction:ltr}#b{left:200px;top:0;direction:rtl}' +
    '#c{left:0;top:100px;direction:ltr}#d{left:200px;top:100px;direction:rtl}' +
    '</style></head><body><div id="a">(</div><div id="b">(</div>' +
    '<div id="c">)</div><div id="d">)</div></body></html>');
  const shot = (await page.screenshot()).toString('base64');
  const rects = await page.evaluate(() =>
    ['a', 'b', 'c', 'd'].map((id) => {
      const el = document.getElementById(id);
      const rg = document.createRange();
      rg.setStart(el.firstChild, 0);
      rg.setEnd(el.firstChild, 1);
      const r = rg.getClientRects()[0];
      return { id, x: r.x, y: r.y, w: r.width, h: r.height };
    }));
  return page.evaluate(async ({ rects, shot }) => {
    const im = await new Promise((res) => {
      const img = new Image();
      img.onload = () => {
        const cv = document.createElement('canvas');
        cv.width = img.width; cv.height = img.height;
        const ctx = cv.getContext('2d');
        ctx.drawImage(img, 0, 0);
        res({ d: ctx.getImageData(0, 0, img.width, img.height).data, w: img.width });
      };
      img.src = 'data:image/png;base64,' + shot;
    });
    const rows = (b) => {
      const out = [];
      for (let y = Math.round(b.y); y < Math.round(b.y + b.h); y++) {
        let r = '';
        for (let x = Math.round(b.x); x < Math.round(b.x + b.w); x++) {
          r += im.d[(y * im.w + x) * 4] < 200 ? '#' : '.';
        }
        out.push(r);
      }
      return out;
    };
    const by = Object.fromEntries(rects.map((b) => [b.id, rows(b)]));
    const mirror = (m) => m.map((r) => r.split('').reverse().join(''));
    const diff = (p, q) => {
      let n = 0;
      for (let y = 0; y < Math.min(p.length, q.length); y++)
        for (let x = 0; x < Math.min(p[y].length, q[y].length); x++)
          if (p[y][x] !== q[y][x]) n++;
      return n;
    };
    const span = (m) => m.filter((_, y) => y % 8 === 0)
      .map((r) => [r.indexOf('#'), r.lastIndexOf('#')])
      .map(([a, b]) => (a < 0 ? '-' : `${a}..${b}`)).join(' ');
    return {
      ltr_open: { rect: rects[0], profile: span(by.a) },
      rtl_open: { rect: rects[1], profile: span(by.b) },
      ltr_close: { rect: rects[2], profile: span(by.c) },
      rtl_close: { rect: rects[3], profile: span(by.d) },
      openLTR_vs_openRTL: diff(by.a, by.b),
      openLTR_vs_mirroredOpenRTL: diff(by.a, mirror(by.b)),
      closeLTR_vs_closeRTL: diff(by.c, by.d),
      closeLTR_vs_mirroredCloseRTL: diff(by.c, mirror(by.d)),
    };
  }, { rects, shot });
}

async function tableAlign(page) {
  await page.setContent('<!doctype html><html><head><meta charset="utf-8"><style>' +
    'body{margin:0;font:16px Arial,sans-serif}body{text-align:center}' +
    'table{width:300px;border-collapse:separate}' +
    '</style></head><body>' +
    '<table id="t"><tbody><tr><td id="d">xx</td></tr></tbody></table>' +
    '<div id="r" style="width:300px">xx</div></body></html>');
  return page.evaluate(() => {
    const textRect = (el) => {
      const rg = document.createRange();
      rg.setStart(el.firstChild, 0);
      rg.setEnd(el.firstChild, el.firstChild.data.length);
      const r = rg.getClientRects()[0];
      return { x: +r.x.toFixed(3), w: +r.width.toFixed(3) };
    };
    const td = document.getElementById('d');
    const cell = td.getBoundingClientRect();
    const pad = getComputedStyle(td).paddingLeft;
    const contentLeft = cell.x + parseFloat(pad);
    const contentRight = cell.x + cell.width - parseFloat(pad);
    const t = textRect(td);
    return {
      tableComputed: getComputedStyle(document.getElementById('t')).textAlign,
      tdComputed: getComputedStyle(td).textAlign,
      tdCell: { x: +cell.x.toFixed(3), w: +cell.width.toFixed(3) },
      tdContent: { left: +contentLeft.toFixed(3), right: +contentRight.toFixed(3) },
      tdText: t,
      tdTextCentre: +(t.x + t.w / 2).toFixed(3),
      contentCentre: +((contentLeft + contentRight) / 2).toFixed(3),
      divText: textRect(document.getElementById('r')),
    };
  });
}

async function endKeyword(page) {
  await page.setContent('<!doctype html><html><head><meta charset="utf-8"><style>' +
    'body{margin:0;font:16px Arial,sans-serif}' +
    '.box{width:200px;height:20px;position:absolute;left:0}' +
    '</style></head><body>' +
    // own direction rtl + end
    '<div class="box" id="e1" style="top:0;direction:rtl;text-align:end">abc</div>' +
    // direction inherited from an rtl parent, value declared outside it
    '<div id="e2w" style="position:absolute;left:0;top:30px;width:200px;direction:rtl">' +
    '<div class="box" id="e2" style="position:static;text-align:end">abc</div></div>' +
    // ltr with end
    '<div class="box" id="e3" style="top:70px;text-align:end">abc</div>' +
    // rtl, computed value read back
    '</body></html>');
  return page.evaluate(() => {
    const read = (id) => {
      const el = document.getElementById(id);
      const rg = document.createRange();
      rg.setStart(el.firstChild, 0);
      rg.setEnd(el.firstChild, 3);
      const r = rg.getClientRects()[0];
      return {
        computed: getComputedStyle(el).textAlign,
        direction: getComputedStyle(el).direction,
        boxLeft: +el.getBoundingClientRect().x.toFixed(3),
        boxRight: +el.getBoundingClientRect().right.toFixed(3),
        textX: +r.x.toFixed(3),
        textRight: +(r.x + r.width).toFixed(3),
      };
    };
    return { e1_rtl_end: read('e1'), e2_inherited_rtl_end: read('e2'), e3_ltr_end: read('e3') };
  });
}

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT });
  console.log('---- glyph mirroring (64px "(" and ")" in ltr vs rtl context)');
  console.log(JSON.stringify(await mirroring(page), null, 1));
  console.log('---- table inherited alignment (300px table under body{text-align:center})');
  console.log(JSON.stringify(await tableAlign(page), null, 1));
  console.log('---- text-align: end across a direction change');
  console.log(JSON.stringify(await endKeyword(page), null, 1));
  await browser.close();
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
