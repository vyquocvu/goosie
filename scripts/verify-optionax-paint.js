// Anchor for the painted spelling of a closed <select> (GOOSIE-OPTIONPAINT-001):
// does Chromium paint the label attribute's interior whitespace verbatim or
// collapsed, and does it trim the edge run? Run:
//   node scripts/verify-optionax-paint.js /tmp/optionpaint/chromium.png
//   go run ./cmd/goosie-headless -in /tmp/optionpaint/fixture.html \
//       -out /tmp/optionpaint/goosie.png -width 400 -height 400
//   node scripts/verify-optionax-paint.js --png /tmp/optionpaint/goosie.png
//
// Both PNGs are 800x800 (dpr 2). The script locates each select by the same
// absolute-position grid (left 20, top i*34, 200x26 CSS), scans dark columns
// (all channels < 110) inside the middle band of the control, and reports, per
// shape: the first ink column offset from the control's left edge, the widest
// interior gap in CSS px, and the gap converted to a space count using the
// baseline labels b0..b3 ("MyLabel", "My Label", "My  Label", "My   Label").
// A gap is only counted inside the ink span, and the dropdown-arrow band
// (right 16 CSS px) is excluded from both engines.
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const OUT = process.argv[2] && !process.argv[2].startsWith('--')
  ? process.argv[2] : '/tmp/optionpaint/chromium.png';
const pngIdx = process.argv.indexOf('--png');
const MEASURE_PNG = pngIdx >= 0 ? process.argv[pngIdx + 1] : null;
const DIR = path.dirname(OUT);
const BOXES = path.join(DIR, 'boxes.json');

const SHAPES = [
  { id: 'b0', html: '<option value="x" label="MyLabel">hidden A</option>' },
  { id: 'b1', html: '<option value="x" label="My Label">hidden A</option>' },
  { id: 'b2', html: '<option value="x" label="My  Label">hidden A</option>' },
  { id: 'b3', html: '<option value="x" label="My   Label">hidden A</option>' },
  { id: 'q1', html: '<option value="x" label="  My   Label  ">hidden A</option>' },
  { id: 'q2', html: '<option value="x" label=" My Label ">hidden A</option>' },
  { id: 'q3', html: '<option value="x">  My   Label  </option>' },
  { id: 'q4', html: '<option value="x" label="  XY">hidden A</option>' },
  { id: 'q5', html: '<option value="x" label="\fMy\f\fLabel\f">hidden A</option>' },
  { id: 'q6', html: '<option value="x" label="\vMy\vLabel\v">hidden A</option>' },
  { id: 'q7', html: '<option value="x" label="\vMy\v\vLabel">hidden A</option>' },
];

const html = `<!doctype html>
<html>
  <head><meta charset="utf-8"><style>
    body{margin:0;background:#fff;font-family:sans-serif;font-size:14px}
    select{position:absolute;left:20px;width:200px;height:26px;font-size:14px}
  </style></head>
  <body>
    ${SHAPES.map((s, i) => `<select id="${s.id}" style="top:${i * 34}px">${s.html}<option>Zeta</option></select>`).join('\n    ')}
  </body>
</html>`;

function loadPNG(file) {
  // Minimal PNG reader: only enough to get raw RGBA rows for 8-bit non-interlaced.
  const buf = fs.readFileSync(file);
  if (buf.readUInt32BE(0) !== 0x89504e47) throw new Error('not a PNG');
  let off = 8, w = 0, h = 0, bitDepth = 0, colorType = 0;
  const idat = [];
  while (off < buf.length) {
    const len = buf.readUInt32BE(off);
    const type = buf.toString('ascii', off + 4, off + 8);
    const data = buf.subarray(off + 8, off + 8 + len);
    if (type === 'IHDR') {
      w = data.readUInt32BE(0); h = data.readUInt32BE(4);
      bitDepth = data[8]; colorType = data[9];
      if (bitDepth !== 8 || (colorType !== 6 && colorType !== 2)) throw new Error('unsupported PNG ' + bitDepth + '/' + colorType);
    } else if (type === 'IDAT') idat.push(data);
    off += 12 + len;
  }
  const zlib = require('zlib');
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

function measure(png, boxes) {
  const { w, h, bpp, data } = png;
  const scale = w / 400; // PNG px per CSS px (dpr 2)
  const rows = [];
  for (const box of boxes) {
    const x0 = Math.round((box.x + 2) * scale);
    const x1 = Math.round((box.x + box.w - 16) * scale); // exclude the arrow band
    const y0 = Math.round((box.y + box.h * 0.2) * scale);
    const y1 = Math.round((box.y + box.h * 0.8) * scale);
    const dark = [];
    for (let x = x0; x < x1 && x < w; x++) {
      let d = false;
      for (let y = y0; y < y1 && y < h; y++) {
        const i = (y * w + x) * bpp;
        if (data[i] < 110 && data[i + 1] < 110 && data[i + 2] < 110) { d = true; break; }
      }
      dark.push(d);
    }
    const runs = [];
    let start = -1;
    for (let i = 0; i <= dark.length; i++) {
      const d = i < dark.length && dark[i];
      if (d && start < 0) start = i;
      if (!d && start >= 0) { runs.push([start, i]); start = -1; }
    }
    if (!runs.length) { rows.push({ id: box.id, blank: true }); continue; }
    const first = runs[0][0] / scale, last = runs[runs.length - 1][1] / scale;
    let maxGap = 0, gapCount = 0;
    for (let i = 1; i < runs.length; i++) {
      const g = (runs[i][0] - runs[i - 1][1]) / scale;
      if (g > 1.2) { gapCount++; if (g > maxGap) maxGap = g; }
    }
    rows.push({ id: box.id, first: +first.toFixed(1), span: +(last - first).toFixed(1), maxGap: +maxGap.toFixed(2), bigGaps: gapCount, runs: runs.length });
  }
  return rows;
}

(async () => {
  fs.mkdirSync(DIR, { recursive: true });
  fs.writeFileSync(path.join(DIR, 'fixture.html'), html);
  if (!MEASURE_PNG) {
    const browser = await chromium.launch();
    const page = await browser.newPage({ viewport: { width: 400, height: 400 }, deviceScaleFactor: 2 });
    await page.goto('file://' + path.join(DIR, 'fixture.html'));
    const boxes = await page.evaluate(() =>
      [...document.querySelectorAll('select')].map((s) => {
        const r = s.getBoundingClientRect();
        return { id: s.id, x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) };
      }));
    fs.writeFileSync(BOXES, JSON.stringify(boxes));
    await page.screenshot({ path: OUT });
    await browser.close();
  }
  const boxes = JSON.parse(fs.readFileSync(BOXES, 'utf8'));
  const target = MEASURE_PNG || OUT;
  const rows = measure(loadPNG(target), boxes);
  const base = {};
  for (const id of ['b0', 'b1', 'b2', 'b3']) {
    const r = rows.find((x) => x.id === id);
    base[id] = r ? r.maxGap : null;
  }
  const gap0 = base.b0 ?? 0;
  const spaceW = ((base.b1 ?? gap0) - gap0) || 1;
  for (const r of rows) {
    if (r.blank) { console.log(r.id + ' BLANK'); continue; }
    const est = Math.max(0, Math.round((r.maxGap - gap0) / spaceW));
    console.log(`${r.id} first=${r.first} span=${r.span} maxGap=${r.maxGap} spaces≈${est} (runs=${r.runs}, bigGaps=${r.bigGaps})`);
  }
  console.log(`baselines gap0=${gap0} spaceW=${spaceW.toFixed(2)} (b0..b3: ${base.b0}, ${base.b1}, ${base.b2}, ${base.b3})`);
})();
