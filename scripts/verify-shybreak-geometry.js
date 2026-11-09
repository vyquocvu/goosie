// GOOSIE-SHYBREAK-001 anchor: where does Chromium actually wrap a Latin word
// that contains a soft hyphen or a zero-width space, and what ink appears at
// the break?
//
//   node scripts/verify-shybreak-geometry.js
//
// The overflowing fixtures are the same strings scripts/verify-br087-text-parity.js
// already measured (42 a's + opportunity + 48 b's, 16px, 200px box). This
// script is the dedicated re-measurement: per-character Range rects, line
// boxes, and a hyphen-width control, all from Chromium's own layout, plus
// device pixels from Chromium's own PNG so a "hyphen ink" claim cannot come
// from a stale coordinate.
const { chromium } = require('playwright');

const VIEWPORT = { width: 700, height: 420 };
const PREFIX = 'a'.repeat(42);
const SUFFIX = 'b'.repeat(48);

const PAGE = `
<!doctype html>
<meta charset="utf-8">
<style>
  body { margin: 0; font: 16px/1.2 Arial, Helvetica, sans-serif; color: #000; background: #fff; }
  #flow { position: absolute; left: 20px; top: 20px; width: 200px; }
  #flow div { margin-bottom: 16px; }
  #fit, #hy { position: absolute; top: 20px; white-space: pre; }
  #fit { left: 280px; }
  #hy { left: 400px; }
</style>
<div id="flow">
  <div id="shy">${PREFIX}&shy;${SUFFIX}</div>
  <div id="zws">${PREFIX}&#8203;${SUFFIX}</div>
  <div id="plain">${PREFIX}${SUFFIX}</div>
  <div id="short">aa&shy;bb</div>
  <div id="ka" style="word-break: keep-all">${PREFIX}&shy;${SUFFIX}</div>
  <div id="nw" style="white-space: nowrap">${PREFIX}&shy;${SUFFIX}</div>
</div>
<div id="fit"></div>
<div id="hy"></div>
`;

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.setContent(PAGE, { waitUntil: 'load' });

  const { report, probes } = await page.evaluate(() => {
    const lines = [];
    const say = (s) => lines.push(s);
    const r = (n) => Math.round(n * 1000) / 1000;

    const lineBoxes = (el) => {
      const s = el.firstChild;
      const chars = Array.from(s.nodeValue);
      const rng = document.createRange();
      const out = [];
      let start = 0;
      let prev = 1;
      const flush = (end) => {
        if (end <= start) return;
        rng.setStart(s, start);
        rng.setEnd(s, end);
        const rs = rng.getClientRects();
        if (!rs.length) return;
        const b = rs[0];
        out.push({ x: r(b.x), y: r(b.y), w: r(b.width), text: chars.slice(start, end).join('') });
      };
      for (let i = 1; i <= chars.length; i++) {
        rng.setStart(s, 0);
        rng.setEnd(s, i);
        const n = rng.getClientRects().length;
        if (n > prev) { flush(i - 1); start = i - 1; prev = n; }
      }
      flush(chars.length);
      return out;
    };

    const show = (tag, el) => {
      const ls = lineBoxes(el);
      say(`${tag} lines=${ls.length} clientWidth=${el.clientWidth} scrollWidth=${el.scrollWidth}`);
      say(`${tag} boxes: ` + ls.map((l, i) =>
        `#${i} x=${l.x} y=${l.y} w=${l.w} chars=${Array.from(l.text).length} text=${JSON.stringify(l.text)}`
      ).join(' | '));
      return ls;
    };

    const shy = show('BREAK-shy', document.getElementById('shy'));
    const zws = show('BREAK-zws', document.getElementById('zws'));
    show('BREAK-plain', document.getElementById('plain'));
    show('BREAK-short', document.getElementById('short'));
    show('BREAK-keep-all', document.getElementById('ka'));
    show('BREAK-nowrap', document.getElementById('nw'));

    const mk = (id, text) => {
      const s = document.createElement('span');
      s.textContent = text;
      document.getElementById(id).appendChild(s);
      return s.getBoundingClientRect();
    };
    const a = mk('fit', 'a');
    const aShy = mk('fit', 'a\u00AD');
    const hyphen = mk('hy', '-');
    say(`CTRL a=${r(a.width)} a+SHY=${r(aShy.width)} hyphen=${r(hyphen.width)}`);
    say(`CTRL unused-SHY-adds-nothing=${r(a.width) === r(aShy.width)}`);

    const probes = [];
    for (const [tag, ls] of [['shy', shy], ['zws', zws]]) {
      for (let i = 0; i < ls.length; i++) {
        probes.push({
          name: `${tag}-L${i}`,
          x: ls[i].x + Math.min(2, Math.max(0, ls[i].w / 2)),
          y: ls[i].y + 4,
        });
        if (ls[i].w > 0) {
          probes.push({
            name: `${tag}-L${i}-end`,
            x: ls[i].x + ls[i].w - 1,
            y: ls[i].y + 4,
          });
        }
      }
    }
    return { report: lines.join('\n'), probes };
  });

  console.log(report);

  const b64 = (await page.screenshot({ type: 'png' })).toString('base64');
  const pixels = await page.evaluate(async ({ b64, probes }) => {
    const img = new Image();
    await new Promise((res, rej) => { img.onload = res; img.onerror = rej; img.src = 'data:image/png;base64,' + b64; });
    const cv = document.createElement('canvas');
    cv.width = img.width; cv.height = img.height;
    const ctx = cv.getContext('2d', { willReadFrequently: true });
    ctx.drawImage(img, 0, 0);
    return probes.map((p) => {
      const d = ctx.getImageData(Math.round(p.x), Math.round(p.y), 1, 1).data;
      return { name: p.name, x: p.x, y: p.y, rgb: [d[0], d[1], d[2]] };
    });
  }, { b64, probes });
  console.log('PIXELS ' + pixels.map((p) => `${p.name}@(${Math.round(p.x)},${Math.round(p.y)})=rgb(${p.rgb.join(',')})`).join(' '));

  await browser.close();
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
