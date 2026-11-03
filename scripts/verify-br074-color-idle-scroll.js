// BR-074 anchor: what colours does Chromium actually put on the pixels of a
// page that uses solid fills, one opacity group and simple gradients, and how
// stable are those pixels when nothing changes (idle), when the page scrolls,
// and when the same scroll offset is reached by a different path?
//
//   node scripts/verify-br074-color-idle-scroll.js
//
// The QA case is "màu không nhấp nháy khi idle hoặc scroll" - colour must not
// flicker on idle or on scroll. Two halves, and this script measures both:
//
// (1) the colour of a pixel, so a Goosie oracle can be checked against a real
//     reference rather than against arithmetic one of us likes;
// (2) the identity of the whole captured surface across frames that should not
//     change it: three idle screenshots, one scroll offset reached by a single
//     jump / by eight steps / by going past it and coming back.
//
// Pixels are read by feeding Chromium's own PNG back into the page through an
// Image + 2D canvas, so the numbers are the composited device pixels the user
// sees, not a style the script recomputed. deviceScaleFactor is 1, so a CSS px
// and a device px are the same thing and a document point maps to a screenshot
// row by subtracting scrollY.
//
// Measured 2026-11-04 on this host (Chromium 1.63). Colour:
//
//   PIXELS solid #e7004c        (60,60)   rgb(231,0,76)   - exact
//   PIXELS backdrop alone                 rgb(0,0,255)   - exact
//   PIXELS #f00 at opacity .5 over #00f   rgb(128,0,127) - the arithmetic 127.5
//          lands asymmetrically per channel, so an oracle of a composited group
//          needs a band, not an equality.
//   PIXELS linear-gradient(90deg,#000,#fff) 2px inside the ends rgb(3,3,3) and
//          rgb(253,253,253), at 25/50/75% rgb(64,64,64) rgb(128,128,128)
//          rgb(192,192,192) - a perfect ramp would read 0 and 255, so Chromium
//          dithers gradient ramps and a gradient oracle needs a per-channel band.
//   PIXELS linear-gradient(#111 0 50%,#eee 50% 100%) rgb(17,17,17) above the
//          50% line and rgb(238,238,238) below it - a hard stop keeps its edge.
//   PIXELS radial-gradient(circle,#f00,#00f) centre rgb(252,0,2), corner
//          rgb(33,0,221) - again the dithered end of a ramp. The box is 100x100
//          at (230,70) with centre (280,120), so two more points measure the
//          RADIUS rather than the colours: 50px above the centre reads
//          rgb(76,0,178) and midway to the corner rgb(130,0,125). Both are
//          still blending, so Chromium's circle reaches the corner
//          (sqrt(50^2+50^2) = 70.7px), not half the side - a radius of 50px
//          would have answered rgb(0,0,255) at the first of them.
//
// Stability, which is the half the case is actually about:
//
//   IDLE   three captures 300ms apart on an unattended page: byte-identical.
//   SCROLL scrollY 160 reached in one jump, in eight steps of 20, and after
//          going to 320 and coming back: three byte-identical captures, and
//          returning to the top reproduces the very first capture byte for byte.
//
// Nothing in this fixture animates, so Chromium's stability is a statement
// about the frame path, not about a paused CSS animation. A gradient whose ramp
// depends on which tile drew it, or a tile reused across a content change,
// would show up here as a byte difference.
const { chromium } = require('playwright');
const crypto = require('crypto');

const VIEWPORT = { width: 800, height: 600 };

const pageHTML = () => `<!doctype html><html><head><style>
html,body{margin:0;padding:0;background:#fff}
.box{position:absolute}
#solid{left:10px;top:10px;width:100px;height:100px;background:#e7004c}
#backdrop{left:120px;top:10px;width:100px;height:100px;background:#00f}
#over{position:absolute;left:20px;top:20px;width:60px;height:60px;background:#f00;opacity:.5}
#lin{left:240px;top:10px;width:200px;height:40px;background:linear-gradient(90deg,#000,#fff)}
#hard{left:10px;top:130px;width:200px;height:40px;background:linear-gradient(#111 0 50%,#eee 50% 100%)}
#rad{left:230px;top:70px;width:100px;height:100px;background:radial-gradient(circle,#f00,#00f)}
#tall{left:0;top:200px;width:800px;height:1600px;background:linear-gradient(180deg,#0a0 0,#a00 100%)}
</style></head><body>
<div id="solid" class="box"></div>
<div id="backdrop" class="box"><div id="over"></div></div>
<div id="lin" class="box"></div>
<div id="hard" class="box"></div>
<div id="rad" class="box"></div>
<div id="tall" class="box"></div>
</body></html>`;

// shot returns the captured surface and its digest.
async function shot(page) {
  const buf = await page.screenshot({ type: 'png' });
  return { buf, md5: crypto.createHash('md5').update(buf).digest('hex') };
}

// docPixels reads the device colour of document points by decoding Chromium's
// own PNG inside the page. scrollY is subtracted because a screenshot is a
// viewport image while the caller names document points.
async function docPixels(page, points) {
  const b64 = (await page.screenshot({ type: 'png' })).toString('base64');
  return page.evaluate(async ({ b64, points }) => {
    const img = new Image();
    await new Promise((res, rej) => { img.onload = res; img.onerror = rej; img.src = 'data:image/png;base64,' + b64; });
    const cv = document.createElement('canvas');
    cv.width = img.width; cv.height = img.height;
    const ctx = cv.getContext('2d', { willReadFrequently: true });
    ctx.drawImage(img, 0, 0);
    const sy = window.scrollY;
    return points.map(p => {
      const d = ctx.getImageData(Math.round(p.x), Math.round(p.y - sy), 1, 1).data;
      return { name: p.name, rgb: [d[0], d[1], d[2]] };
    });
  }, { b64, points });
}

const POINTS = [
  { name: 'solid #e7004c', x: 60, y: 60 },
  { name: 'opacity .5 #f00 over #00f', x: 170, y: 60 },
  { name: 'backdrop alone', x: 130, y: 20 },
  { name: 'linear 90deg at 0%', x: 242, y: 30 },
  { name: 'linear 90deg at 25%', x: 290, y: 30 },
  { name: 'linear 90deg at 50%', x: 340, y: 30 },
  { name: 'linear 90deg at 75%', x: 390, y: 30 },
  { name: 'linear 90deg at 100%', x: 438, y: 30 },
  { name: 'hard stop above 50%', x: 100, y: 145 },
  { name: 'hard stop below 50%', x: 100, y: 155 },
  { name: 'radial center', x: 280, y: 120 },
  { name: 'radial corner', x: 236, y: 76 },
  // The two points that measure the circle's RADIUS and not just its ends: #rad
  // is 100x100 at (230,70) with centre (280,120), so 50px above the centre is
  // half the SIDE and midway-to-corner is half the half-DIAGONAL. A radius of
  // 50px puts both at the end colour; a radius of 70.71px is still blending.
  { name: 'radial 50px above center', x: 280, y: 70 },
  { name: 'radial midway to corner', x: 255, y: 95 },
];

// SCROLL_POINTS must be document points inside the captured surface at EVERY
// offset the script visits, or the comparison measures the harness instead of
// the browser: a first attempt read document y=900 at scrollY 0, which lies
// below the 600px capture, and answered transparent black. #tall spans doc y
// 200..1800 with a vertical ramp, so these four rows all sit in it at scrollY
// 0 (rows 210..590) and at scrollY 160 (rows 50..430).
const SCROLL_POINTS = [
  { name: 'tall@10px', x: 400, y: 210 },
  { name: 'tall@100px', x: 400, y: 300 },
  { name: 'tall@250px', x: 400, y: 450 },
  { name: 'tall@390px', x: 400, y: 590 },
];

// SYNTAX measures which stop-position spellings Chromium accepts, because the
// parse rule below is encoded from this answer and not from anyone's memory of
// the grammar. `<length-percentage>` allows a unitless ZERO and nothing else,
// and this checks that, the double-position band, and px positions.
const SYNTAX = [
  { name: 'no-positions', css: 'linear-gradient(180deg,#0a0,#a00)' },
  { name: 'unitless-zero', css: 'linear-gradient(180deg,#0a0 0,#a00 100%)' },
  { name: 'pct-ends', css: 'linear-gradient(180deg,#0a0 0%,#a00 100%)' },
  { name: 'double-unitless-zero', css: 'linear-gradient(#111 0 50%,#eee 50% 100%)' },
  { name: 'double-pct', css: 'linear-gradient(#111 0% 50%,#eee 50% 100%)' },
  { name: 'unitless-fifty-invalid', css: 'linear-gradient(180deg,#0a0 50,#a00)' },
  { name: 'px-ends', css: 'linear-gradient(180deg,#0a0 0px,#a00 60px)' },
  { name: 'px-mid', css: 'linear-gradient(180deg,#f00,#0f0 30px,#00f 60px)' },
];

async function syntax(browser) {
  const cells = SYNTAX.map((c, i) =>
    `<div id="s${i}" style="position:absolute;left:10px;top:${10 + i * 70}px;width:120px;height:60px;background:${c.css}"></div>`).join('');
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.setContent(`<!doctype html><html><head><style>html,body{margin:0;padding:0;background:#fff}</style></head><body>${cells}</body></html>`,
    { waitUntil: 'load' });
  const read = await page.evaluate(async (names) => {
    const out = [];
    for (let i = 0; i < names.length; i++) {
      const el = document.getElementById('s' + i);
      out.push({ name: names[i], computed: getComputedStyle(el).backgroundImage });
    }
    return out;
  }, SYNTAX.map(c => c.name));
  const pts = [];
  SYNTAX.forEach((c, i) => {
    const top = 10 + i * 70;
    pts.push({ name: `${c.name} 25%`, x: 70, y: top + 15 });
    pts.push({ name: `${c.name} 50%`, x: 70, y: top + 30 });
    pts.push({ name: `${c.name} 75%`, x: 70, y: top + 45 });
  });
  const px = await docPixels(page, pts);
  for (let i = 0; i < SYNTAX.length; i++) {
    const row = px.slice(i * 3, i * 3 + 3).map(p => `rgb(${p.rgb.join(',')})`).join(' ');
    console.log(`SYNTAX ${SYNTAX[i].name.padEnd(24)} kept=${read[i].computed === 'none' ? 'DROPPED' : 'YES'} ` +
      `computed=${read[i].computed.slice(0, 72)} pixels ${row}`);
  }
  await page.close();
}

async function colours(browser) {
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.setContent(pageHTML(), { waitUntil: 'load' });
  const px = await docPixels(page, POINTS);
  for (const p of px) console.log(`PIXELS ${p.name.padEnd(28)} rgb(${p.rgb.join(',')})`);
  await page.close();
}

async function idle(browser) {
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.setContent(pageHTML(), { waitUntil: 'load' });
  const shots = [];
  for (let i = 0; i < 3; i++) {
    shots.push(await shot(page));
    if (i < 2) await page.waitForTimeout(300);
  }
  const stable = shots.every(s => s.md5 === shots[0].md5);
  console.log(`IDLE 3 captures 300ms apart md5 ${shots.map(s => s.md5.slice(0, 8)).join(' ')} ` +
    `identical=${stable} bytes ${shots.map(s => s.buf.length).join(' ')}`);
  await page.close();
}

// reachedBy drives the viewport to y with steps of size, then captures.
async function reachedBy(page, y, size) {
  for (let cur = 0; cur < y; cur += size) {
    const target = Math.min(y, cur + size);
    await page.evaluate(t => window.scrollTo(0, t), target);
    await page.waitForTimeout(16);
  }
  await page.evaluate(t => window.scrollTo(0, t), y);
  await page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))));
  return shot(page);
}

async function scrollStability(browser) {
  const Y = 160;
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.setContent(pageHTML(), { waitUntil: 'load' });
  const base = await shot(page);
  const jump = await reachedBy(page, Y, Y);
  const steps = await reachedBy(page, Y, 20);
  // past it and back: the row that was evicted on the way down has to come back
  // with the same pixels it was first drawn with.
  await reachedBy(page, 320, 320);
  await page.evaluate(t => window.scrollTo(0, t), Y);
  await page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))));
  const there = await shot(page);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))));
  const backAgain = await shot(page);
  const px0 = await docPixels(page, SCROLL_POINTS);
  console.log(`SCROLL path md5 top=${base.md5.slice(0, 8)} jump${Y}=${jump.md5.slice(0, 8)} ` +
    `steps20=${steps.md5.slice(0, 8)} via320=${there.md5.slice(0, 8)} back=${backAgain.md5.slice(0, 8)} ` +
    `all-at-${Y}-identical=${jump.md5 === steps.md5 && steps.md5 === there.md5}`);
  console.log(`SCROLL top returns identical=${base.md5 === backAgain.md5}`);
  console.log(`SCROLL-PIXELS at scrollY 0 ${px0.map(p => `${p.name}=rgb(${p.rgb.join(',')})`).join(' ')}`);
  await page.evaluate(t => window.scrollTo(0, t), Y);
  await page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))));
  const pxY = await docPixels(page, SCROLL_POINTS);
  console.log(`SCROLL-PIXELS at scrollY ${Y} ${pxY.map(p => `${p.name}=rgb(${p.rgb.join(',')})`).join(' ')}`);
  console.log(`SCROLL-PIXELS same document points across the two offsets ` +
    `identical=${px0.every((p, i) => p.rgb.join() === pxY[i].rgb.join())}`);
  await page.close();
}

(async () => {
  const browser = await chromium.launch();
  await syntax(browser);
  await colours(browser);
  await idle(browser);
  await scrollStability(browser);
  await browser.close();
  console.log('DONE');
})();
