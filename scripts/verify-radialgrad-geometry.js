// GOOSIE-RADIALGRAD-001 anchor: what radius rule does Chromium actually apply
// to a radial-gradient, per shape and per size keyword, and what colour lands
// on named sample pixels because of it?
//
//   node scripts/verify-radialgrad-geometry.js
//
// Goosie today derives the radius inside internal/frame/gradient.go: a circle
// takes min(W,H)/2 and an ellipse one farthest-corner scalar, while
// style.RadialGradient.Size (closest-side / farthest-side / closest-corner /
// farthest-corner) is parsed but never reaches the rasterizer. The reference
// answer comes from this script, pixel by pixel, not from a spec recollection.
//
// Six boxes, each #f00 -> #00f, each with points chosen so the four keywords
// give four different answers:
//
//   #fc     circle (default farthest-corner), 100x100, centred - the recorded
//           BR-074 fixture: the ramp must reach the farthest corner
//           (70.71px), not half the side (50px).
//   #cs     circle closest-side at 25% 25% - radius 25px, so 25px right of the
//           centre is already the end colour and 17px right is still blending.
//   #fs     circle farthest-side at 25% 25% - radius 75px, so the same 25px
//           right sits a third of the way along the ramp.
//   #cc     circle closest-corner at 25% 25% - radius 35.36px (the diagonal to
//           the nearest corner), between the two sides.
//   #ell    ellipse (default farthest-corner) on a 200x100 box - per-axis
//           radii 141.42 x 70.71: half-width and half-height must sit at
//           DIFFERENT ramp positions, which no scalar radius can produce.
//   #ellcs  ellipse closest-side on 200x100 - radii 100 x 50: half-width and
//           half-height land on the same colour (both t=0.5), the per-axis
//           signature.
//
// Pixels are read by feeding Chromium's own PNG back into the page through an
// Image + 2D canvas, so the numbers are the composited device pixels, and
// Chromium dithers ramps, so every oracle built from these numbers carries a
// tolerance band rather than an equality.
const { chromium } = require('playwright');

const VIEWPORT = { width: 800, height: 600 };

const pageHTML = () => `<!doctype html><html><head><style>
html,body{margin:0;padding:0;background:#fff}
.box{position:absolute}
#fc{left:10px;top:10px;width:100px;height:100px;background:radial-gradient(circle,#f00,#00f)}
#cs{left:120px;top:10px;width:100px;height:100px;background:radial-gradient(circle closest-side at 25% 25%,#f00,#00f)}
#fs{left:230px;top:10px;width:100px;height:100px;background:radial-gradient(circle farthest-side at 25% 25%,#f00,#00f)}
#cc{left:340px;top:10px;width:100px;height:100px;background:radial-gradient(circle closest-corner at 25% 25%,#f00,#00f)}
#ell{left:10px;top:120px;width:200px;height:100px;background:radial-gradient(ellipse,#f00,#00f)}
#ellcs{left:220px;top:120px;width:200px;height:100px;background:radial-gradient(ellipse closest-side,#f00,#00f)}
#pxat{left:450px;top:10px;width:100px;height:100px;background:radial-gradient(circle at 30px 30px,#f00,#00f)}
#ellfs{left:10px;top:230px;width:200px;height:100px;background:radial-gradient(ellipse farthest-side at 25% 25%,#f00,#00f)}
#ellcs2{left:220px;top:230px;width:200px;height:100px;background:radial-gradient(ellipse closest-side at 25% 25%,#f00,#00f)}
</style></head><body>
<div id="fc" class="box"></div>
<div id="cs" class="box"></div>
<div id="fs" class="box"></div>
<div id="cc" class="box"></div>
<div id="ell" class="box"></div>
<div id="ellcs" class="box"></div>
<div id="pxat" class="box"></div>
<div id="ellfs" class="box"></div>
<div id="ellcs2" class="box"></div>
</body></html>`;

// docPixels reads the device colour of document points by decoding Chromium's
// own PNG inside the page.
async function docPixels(page, points) {
  const b64 = (await page.screenshot({ type: 'png' })).toString('base64');
  return page.evaluate(async ({ b64, points }) => {
    const img = new Image();
    await new Promise((res, rej) => { img.onload = res; img.onerror = rej; img.src = 'data:image/png;base64,' + b64; });
    const cv = document.createElement('canvas');
    cv.width = img.width; cv.height = img.height;
    const ctx = cv.getContext('2d', { willReadFrequently: true });
    ctx.drawImage(img, 0, 0);
    return points.map(p => {
      const d = ctx.getImageData(Math.round(p.x), Math.round(p.y), 1, 1).data;
      return { name: p.name, rgb: [d[0], d[1], d[2]] };
    });
  }, { b64, points });
}

// The size keywords and shapes Chromium must keep, read off computed style -
// the parse side of the slice is encoded from this answer.
const SYNTAX = [
  { name: 'circle-default', css: 'radial-gradient(circle,#f00,#00f)' },
  { name: 'circle-closest-side', css: 'radial-gradient(circle closest-side at 25% 25%,#f00,#00f)' },
  { name: 'circle-farthest-side', css: 'radial-gradient(circle farthest-side at 25% 25%,#f00,#00f)' },
  { name: 'circle-closest-corner', css: 'radial-gradient(circle closest-corner at 25% 25%,#f00,#00f)' },
  { name: 'ellipse-default', css: 'radial-gradient(ellipse,#f00,#00f)' },
  { name: 'ellipse-closest-side', css: 'radial-gradient(ellipse closest-side,#f00,#00f)' },
  { name: 'px-position', css: 'radial-gradient(circle at 30px 30px,#f00,#00f)' },
  // The header Chromium drops outright - the Goosie parser must refuse it too,
  // not guess a ramp out of it.
  { name: 'header-invalid', css: 'radial-gradient(circle nonsense,#f00,#00f)' },
  // The explicit-radii size form: valid for Chromium, recorded here as a
  // disclosed Goosie residual (not implemented in this slice).
  { name: 'explicit-radii', css: 'radial-gradient(100px 50px,#f00,#00f)' },
];

const POINTS = [
  // #fc: centred circle 100x100, centre (60,60), farthest corner 70.71px.
  { name: 'fc centre', x: 60, y: 60 },
  { name: 'fc 50px above (half side)', x: 60, y: 10 },
  { name: 'fc midway to corner', x: 35, y: 35 },
  { name: 'fc near corner', x: 14, y: 14 },
  // #cs: circle closest-side at 25% 25%, centre (145,35), radius 25px.
  { name: 'cs centre', x: 145, y: 35 },
  { name: 'cs 25px right (at the side)', x: 170, y: 35 },
  { name: 'cs 17px right', x: 162, y: 35 },
  { name: 'cs corner (35px away)', x: 125, y: 15 },
  // #fs: circle farthest-side at 25% 25%, centre (255,35), radius 75px.
  { name: 'fs centre', x: 255, y: 35 },
  { name: 'fs 25px right (third of ramp)', x: 280, y: 35 },
  { name: 'fs corner', x: 235, y: 15 },
  { name: 'fs 70px below (t=.93)', x: 255, y: 105 },
  // #cc: circle closest-corner at 25% 25%, centre (365,35), radius 35.36px.
  { name: 'cc centre', x: 365, y: 35 },
  { name: 'cc 17px right', x: 382, y: 35 },
  { name: 'cc corner', x: 345, y: 15 },
  // #ell: ellipse farthest-corner 200x100, centre (110,170), radii 141.42x70.71.
  { name: 'ell centre', x: 110, y: 170 },
  { name: 'ell 50px right (t=.35)', x: 160, y: 170 },
  { name: 'ell 35px above (t=.50)', x: 110, y: 135 },
  { name: 'ell diagonal (t=.71)', x: 181, y: 205 },
  // #ellcs: ellipse closest-side 200x100, centre (320,170), radii 100x50.
  { name: 'ellcs centre', x: 320, y: 170 },
  { name: 'ellcs 50px right (t=.5)', x: 370, y: 170 },
  { name: 'ellcs 25px above (t=.5)', x: 320, y: 145 },
  { name: 'ellcs 75px right (t=.75)', x: 395, y: 170 },
  { name: 'ellcs 45px below (t=.9)', x: 320, y: 215 },
  // #ellfs: ellipse farthest-side at 25% 25% on 200x100, centre (60,255),
  // radii (150,75): the farthest side on each axis is a different side here.
  { name: 'ellfs centre', x: 60, y: 255 },
  { name: 'ellfs 75px right (t=.5)', x: 135, y: 255 },
  { name: 'ellfs 37px below (t=.49)', x: 60, y: 292 },
  // #ellcs2: ellipse closest-side at 25% 25% on 200x100, centre (270,255),
  // radii (50,25): the nearest side differs per axis, so 50px right and 25px
  // below are both already the end colour.
  { name: 'ellcs2 centre', x: 270, y: 255 },
  { name: 'ellcs2 50px right (at the side)', x: 320, y: 255 },
  { name: 'ellcs2 25px below (at the side)', x: 270, y: 280 },
  // #pxat: circle at 30px 30px on 100x100, centre (480,40), farthest corner
  // sqrt(70^2+70^2) = 98.99px. The centre pixel alone separates this centre
  // from the default one, which sits 28.28px away and would read a blend.
  { name: 'pxat centre (30,30)', x: 480, y: 40 },
  { name: 'pxat near far corner (t=.94)', x: 545, y: 105 },
];

(async () => {
  const browser = await chromium.launch();

  // Computed style: which headers Chromium keeps, and how it serializes them.
  // These cells are generated separately from the pixel boxes because two of
  // the rows are invalid headers with no painted box.
  const spage = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  const cells = SYNTAX.map((c, i) =>
    `<div id="s${i}" style="position:absolute;left:10px;top:${10 + i * 70}px;width:120px;height:60px;background:${c.css}"></div>`).join('');
  await spage.setContent(`<!doctype html><html><head><style>html,body{margin:0;padding:0;background:#fff}</style></head><body>${cells}</body></html>`,
    { waitUntil: 'load' });
  const syntax = await spage.evaluate((n) => {
    return n.map(id => ({ id, computed: getComputedStyle(document.getElementById(id)).backgroundImage }));
  }, SYNTAX.map((c, i) => 's' + i));
  for (let i = 0; i < SYNTAX.length; i++) {
    console.log(`SYNTAX ${SYNTAX[i].name.padEnd(22)} kept=${syntax[i].computed === 'none' ? 'DROPPED' : 'YES'} computed=${syntax[i].computed}`);
  }
  await spage.close();

  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.setContent(pageHTML(), { waitUntil: 'load' });

  const px = await docPixels(page, POINTS);
  for (const p of px) console.log(`PIXELS ${p.name.padEnd(28)} rgb(${p.rgb.join(',')})`);

  await page.close();
  await browser.close();
  console.log('DONE');
})();
