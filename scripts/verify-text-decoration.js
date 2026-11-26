const { chromium } = require('playwright');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { execFileSync } = require('child_process');

const ROOT = path.resolve(__dirname, '..');
const OUT = fs.mkdtempSync(path.join(os.tmpdir(), 'goosie-text-decoration-'));
const DOC = path.join(OUT, 'document.html');
const REF_PNG = path.join(OUT, 'chromium.png');
const GOO_PNG = path.join(OUT, 'goosie.png');
const ROW = 48;
const CASES = [
  { id: 'ul-auto', decl: 'text-decoration: underline solid red' },
  { id: 'ul-3', decl: 'text-decoration: underline solid red; text-decoration-thickness: 3px' },
  { id: 'ul-5', decl: 'text-decoration: underline solid red; text-decoration-thickness: 5px' },
  { id: 'ul-off2', decl: 'text-decoration: underline solid red; text-underline-offset: 2px' },
  { id: 'ul-off4', decl: 'text-decoration: underline solid red; text-underline-offset: 4px' },
  { id: 'lt-solid', decl: 'text-decoration: line-through solid red' },
  { id: 'lt-dotted', decl: 'text-decoration: line-through dotted red' },
  { id: 'lt-dashed', decl: 'text-decoration: line-through dashed red' },
  { id: 'lt-double', decl: 'text-decoration: line-through double red' },
  { id: 'lt-wavy', decl: 'text-decoration: line-through wavy red' },
  { id: 'ol-solid', decl: 'text-decoration: overline solid red' },
  { id: 'ol-dotted', decl: 'text-decoration: overline dotted red' },
  { id: 'ol-dashed', decl: 'text-decoration: overline dashed red' },
  { id: 'ol-double', decl: 'text-decoration: overline double red' },
  { id: 'ol-wavy', decl: 'text-decoration: overline wavy red' },
  { id: 'ul-double', decl: 'text-decoration: underline double red' },
  { id: 'ul-dotted', decl: 'text-decoration: underline dotted red' },
  { id: 'ul-dashed', decl: 'text-decoration: underline dashed red' },
  { id: 'ul-wavy', decl: 'text-decoration: underline wavy red' },
  { id: 'all-lines', decl: 'text-decoration: underline overline line-through solid red' },
  { id: 'longhand-line', decl: 'text-decoration-line: underline; text-decoration-style: dashed; text-decoration-color: red' },
  { id: 'longhand-style', decl: 'text-decoration: underline solid red; text-decoration-style: wavy' },
  { id: 'order-style-first', decl: 'text-decoration-style: dotted; text-decoration: underline solid red' },
  { id: 'order-style-last', decl: 'text-decoration: underline solid red; text-decoration-style: dotted' },
  { id: 'shadow', decl: 'text-decoration: underline solid red; text-shadow: 2px 2px 0 blue, -2px -2px 0 green' },
  { id: 'ph-auto', placeholder: true, decl: 'text-decoration: underline solid red', pseudo: true },
  { id: 'ph-compose', placeholder: true, host: 'text-decoration-line: underline; text-decoration-style: dashed; text-decoration-color: blue', pseudo: true, decl: 'text-decoration-line: line-through; text-decoration-style: wavy; text-decoration-color: red' },
  { id: 'ph-clip', placeholder: true, pseudo: true, decl: 'text-decoration: underline wavy red; text-underline-offset: 16px' },
];
const VIEWPORT = { width: 640, height: 30 + CASES.length * ROW + 30 };
const PAGE = `<!doctype html>
<meta charset="utf-8">
<style>
  html, body { margin: 0; background: #fff; font: 16px Arial; }
  .case { position: absolute; left: 20px; width: 300px; color: transparent; }
  .marker { display: inline-block; width: 0; height: 0; vertical-align: baseline; }
  ${CASES.map((c, i) => `  #${c.id} { top: ${20 + i * ROW}px; }
  #${c.id}${c.pseudo ? '::placeholder' : ''} { ${c.decl}; }
  ${c.host ? `#${c.id} { ${c.host}; }` : ''}`).join('\n')}
</style>
${CASES.map((c) => c.placeholder
  ? `<input class="case" id="${c.id}" placeholder="HELLOWORLD">`
  : `<div class="case" id="${c.id}">HELLOWORLD<span class="marker"></span></div>`).join('\n')}
`;

function runGoosie() {
  const binary = process.env.GOOSIE_BIN || path.join(ROOT, 'bin', 'goosie');
  if (!process.env.GOOSIE_BIN) {
    execFileSync('go', ['build', '-o', binary, './cmd/goosie'], { cwd: ROOT, stdio: 'inherit' });
  }
  execFileSync(binary, ['-backend', 'headless', '-width', String(VIEWPORT.width),
    '-height', String(VIEWPORT.height), '-dpr', '1', '-screenshot', '-out', GOO_PNG,
    '-url', 'file://' + DOC], { stdio: 'inherit' });
}

async function decode(page, file) {
  const b64 = fs.readFileSync(file).toString('base64');
  return page.evaluate(async (b64) => {
    const img = new Image();
    await new Promise((resolve, reject) => {
      img.onload = resolve;
      img.onerror = reject;
      img.src = 'data:image/png;base64,' + b64;
    });
    const canvas = document.createElement('canvas');
    canvas.width = img.width;
    canvas.height = img.height;
    const ctx = canvas.getContext('2d', { willReadFrequently: true });
    ctx.drawImage(img, 0, 0);
    return { width: img.width, height: img.height, data: Array.from(ctx.getImageData(0, 0, img.width, img.height).data) };
  }, b64);
}

function scan(png, boxes) {
  const out = {};
  for (const box of boxes) {
    const colours = { R: {}, B: {} };
    const top = Math.max(0, Math.floor(box.y) - 8);
    const bottom = Math.min(png.height, Math.ceil(box.y) + 28);
    for (let y = top; y < bottom; y++) {
      const runs = { R: [], B: [] };
      let starts = { R: -1, B: -1 };
      for (let x = Math.max(0, Math.floor(box.x)); x < Math.min(png.width, Math.ceil(box.x + box.w)); x++) {
        const i = (y * png.width + x) * 4;
        const d = png.data;
        let colour = null;
        if (d[i] > 200 && d[i + 1] < 90 && d[i + 2] < 90) colour = 'R';
        else if (d[i + 2] > 200 && d[i] < 90 && d[i + 1] < 90) colour = 'B';
        for (const key of ['R', 'B']) {
          if (colour === key && starts[key] < 0) starts[key] = x;
          if (colour !== key && starts[key] >= 0) {
            runs[key].push([starts[key] - box.x, x - 1 - box.x]);
            starts[key] = -1;
          }
        }
      }
      for (const key of ['R', 'B']) {
        if (starts[key] >= 0) runs[key].push([starts[key] - box.x, Math.ceil(box.x + box.w) - 1 - box.x]);
        if (runs[key].length) colours[key][y - box.y] = runs[key];
      }
    }
    out[box.id] = colours;
  }
  return out;
}

function rows(mask, colour, baseline) {
  return Object.keys(mask[colour]).map(Number).sort((a, b) => a - b).map((y) => y - baseline);
}

function sameRows(got, want) {
  return got.length === want.length && got.every((value, index) => value === want[index]);
}

function patternOK(runs, on, off, tolerance = 0) {
  if (!runs || runs.length < 3) return false;
  for (let i = 0; i < 3; i++) {
    const x0 = runs[i][0];
    if (Math.abs(x0 - i * (on + off)) > tolerance || Math.abs(runs[i][1] - x0 + 1 - on) > tolerance) return false;
  }
  return true;
}

function sequence(from, to) {
  const out = [];
  for (let i = from; i <= to; i++) out.push(i);
  return out;
}

async function mainAsync() {
  fs.writeFileSync(DOC, PAGE);
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: VIEWPORT, deviceScaleFactor: 1 });
  await page.goto('file://' + DOC, { waitUntil: 'load' });
  const measured = await page.evaluate((ids) => ids.map((id) => {
    const el = document.getElementById(id);
    const box = el.getBoundingClientRect();
    const pseudo = id.startsWith('ph-');
    const cs = getComputedStyle(el, pseudo ? '::placeholder' : null);
    let baseline = null;
    let textTop = 0;
    let advance = null;
    if (!pseudo) {
      const range = document.createRange();
      range.selectNodeContents(el.firstChild);
      const text = range.getBoundingClientRect();
      baseline = el.querySelector('.marker').getBoundingClientRect().top - box.y;
      textTop = text.top - box.y;
      advance = text.width;
    }
    return {
      id, x: box.x, y: box.y, w: box.width, h: box.height, baseline, textTop, advance,
      line: cs.textDecorationLine, style: cs.textDecorationStyle,
      thickness: cs.textDecorationThickness, offset: cs.textUnderlineOffset,
    };
  }), CASES.map((c) => c.id));
  await page.screenshot({ path: REF_PNG, clip: { x: 0, y: 0, width: VIEWPORT.width, height: VIEWPORT.height } });
  runGoosie();
  const ref = scan(await decode(page, REF_PNG), measured);
  const got = scan(await decode(page, GOO_PNG), measured);
  await browser.close();

  const refBase = measured.find((m) => m.id === 'ul-auto').baseline;
  const gotBase = rows(got['ul-auto'], 'R', 0)[0] - 1;
  const phRefBase = rows(ref['ph-auto'], 'R', 0)[0] - 1;
  const phGotBase = rows(got['ph-auto'], 'R', 0)[0] - 1;
  const failures = [];
  const residuals = [];
  const check = (name, condition) => {
    if (!condition) failures.push(name);
  };

  console.log(`evidence=${OUT}`);
  console.log(`viewport=${VIEWPORT.width}x${VIEWPORT.height} dpr=1 chromium=${REF_PNG} goosie=${GOO_PNG}`);
  console.log(`baselineOffset chromium=${refBase} goosie=${gotBase} placeholderChromium=${phRefBase} placeholderGoosie=${phGotBase}`);
  for (const m of measured) {
    console.log(`${m.id.padEnd(18)} box=${Math.round(m.x)},${Math.round(m.y)} ${Math.round(m.w)}x${Math.round(m.h)} advance=${m.advance == null ? '-' : m.advance.toFixed(2)} line=${m.line} style=${m.style} thick=${m.thickness} off=${m.offset}`);
    if (m.id.startsWith('ph-')) {
      for (const [label, mask] of [['CHROMIUM', ref], ['GOOSIE', got]]) {
        const red = Object.keys(mask[m.id].R).sort((a, b) => Number(a) - Number(b));
        const blue = Object.keys(mask[m.id].B).sort((a, b) => Number(a) - Number(b));
        console.log(`  ${label.padEnd(9)} redRows=${JSON.stringify(red)} blueRows=${JSON.stringify(blue)}`);
      }
      continue;
    }
    for (const [label, mask, baseline] of [['CHROMIUM', ref, refBase], ['GOOSIE', got, gotBase]]) {
      const red = rows(mask[m.id], 'R', baseline);
      const first = mask[m.id].R[red[0] + baseline];
      console.log(`  ${label.padEnd(9)} redRows=${JSON.stringify(red)} firstRuns=${JSON.stringify(first || [])}`);
    }
  }

  const expected = {
    'ul-auto': [1], 'ul-3': [2, 3, 4], 'ul-5': [3, 4, 5, 6, 7],
    'ul-off2': [2], 'ul-off4': [4], 'lt-solid': [-5],
    'lt-dotted': [-6, -5], 'lt-dashed': [-6, -5], 'lt-double': [-5, -3], 'lt-wavy': [-7, -6, -5, -4],
    'ul-double': [1, 4], 'ul-dotted': [0, 1], 'ul-dashed': [0, 1], 'ul-wavy': [2, 3, 4, 5],
    'longhand-line': [0, 1], 'longhand-style': [2, 3, 4, 5],
    'order-style-first': [1], 'order-style-last': [0, 1], 'shadow': [1],
  };
  for (const [id, want] of Object.entries(expected)) {
    check(`goosie ${id} rows`, sameRows(rows(got[id], 'R', gotBase), want));
  }
  for (const id of ['ul-auto', 'ul-3', 'ul-5', 'ul-off2', 'ul-off4', 'lt-solid', 'lt-double', 'ul-double', 'longhand-line', 'longhand-style', 'order-style-first', 'order-style-last', 'shadow']) {
    check(`chromium ${id} rows`, sameRows(rows(ref[id], 'R', refBase), expected[id]));
  }
  for (const id of ['ul-dotted', 'ul-dashed', 'lt-dotted', 'lt-dashed', 'longhand-line']) {
    const dotted = id.includes('dotted');
    const on = dotted ? 2 : 6;
    const off = dotted ? 2 : 4;
    const goRows = rows(got[id], 'R', gotBase);
    const refRows = rows(ref[id], 'R', refBase);
    check(`goosie ${id} thickness`, goRows.length === 2);
    check(`chromium ${id} thickness`, refRows.length === 2);
    check(`goosie ${id} pattern`, patternOK(got[id].R[goRows[0] + gotBase], on, off));
    check(`chromium ${id} pattern`, patternOK(ref[id].R[refRows[0] + refBase], on, off, dotted ? 0 : 1));
  }
  for (const id of ['ul-wavy', 'lt-wavy', 'longhand-style']) {
    check(`goosie ${id} wave height`, rows(got[id], 'R', gotBase).length === 4);
    check(`chromium ${id} wave height`, rows(ref[id], 'R', refBase).length === 4);
  }

  const overlineExpected = {
    'ol-solid': [-1], 'ol-dotted': [-2, -1], 'ol-dashed': [-2, -1], 'ol-double': [-4, -1], 'ol-wavy': [-4, -3, -2, -1],
  };
  for (const [id, want] of Object.entries(overlineExpected)) {
    const goRows = Object.keys(got[id].R).map(Number).sort((a, b) => a - b);
    check(`goosie ${id} rows`, sameRows(goRows, want));
  }
  const refOverDouble = Object.keys(ref['ol-double'].R).map(Number).sort((a, b) => a - b);
  check('chromium ol-double rows', sameRows(refOverDouble, [-4, -1]));
  const allGo = rows(got['all-lines'], 'R', gotBase);
  check('goosie all line composition', allGo.length === 3 && allGo.includes(1) && allGo.includes(-5) && got['all-lines'].R[-1]);
  const allRef = rows(ref['all-lines'], 'R', refBase);
  check('chromium all line composition', allRef.length === 3 && allRef.includes(1) && allRef.includes(-5) && ref['all-lines'].R[-1]);

  for (const engine of ['chromium', 'goosie']) {
    const mask = engine === 'chromium' ? ref : got;
    const base = engine === 'chromium' ? phRefBase : phGotBase;
    check(`${engine} placeholder auto`, rows(mask['ph-auto'], 'R', base).length === 1);
    check(`${engine} placeholder host pseudo red wave`, rows(mask['ph-compose'], 'R', base).length === 4);
    check(`${engine} placeholder host pseudo blue dashes`, rows(mask['ph-compose'], 'B', base).length >= 1);
    check(`${engine} placeholder clip`, rows(mask['ph-clip'], 'R', base).length === 0);
  }

  const advance = measured.find((m) => m.id === 'ul-auto').advance;
  const goWidth = got['ul-auto'].R[gotBase + 1][0][1] - got['ul-auto'].R[gotBase + 1][0][0] + 1;
  if (Math.abs(advance - goWidth) > 1) residuals.push(`text advance chromium=${advance.toFixed(2)} goosie=${goWidth}`);
  const refOverWavy = Object.keys(ref['ol-wavy'].R).map(Number).sort((a, b) => a - b);
  if (!sameRows(refOverWavy, [-4, -3, -2, -1])) residuals.push(`overline wavy rows chromium=${JSON.stringify(refOverWavy)} goosie=[-4,-3,-2,-1]`);
  const refWaveRuns = JSON.stringify(ref['ul-wavy'].R[refBase + 2]);
  const goWaveRuns = JSON.stringify(got['ul-wavy'].R[gotBase + 2]);
  if (refWaveRuns !== goWaveRuns) residuals.push(`wavy peak profile chromium=${refWaveRuns} goosie=${goWaveRuns}`);
  const refDash = JSON.stringify(ref['ul-dashed'].R[refBase].slice(0, 3));
  const goDash = JSON.stringify(got['ul-dashed'].R[gotBase].slice(0, 3));
  if (refDash !== goDash) residuals.push(`dashed phase chromium=${refDash} goosie=${goDash}`);
  const refPlaceholderBlue = rows(ref['ph-compose'], 'B', phRefBase).length;
  const goPlaceholderBlue = rows(got['ph-compose'], 'B', phGotBase).length;
  if (refPlaceholderBlue !== goPlaceholderBlue) residuals.push(`placeholder dashed rows chromium=${refPlaceholderBlue} goosie=${goPlaceholderBlue}`);

  for (const residual of residuals) console.log(`RESIDUAL ${residual}`);
  if (failures.length) {
    for (const failure of failures) console.log(`FAIL ${failure}`);
    throw new Error(`${failures.length} text-decoration anchor checks failed`);
  }
  console.log(`TEXT DECORATION ANCHOR OK (${failures.length} failures, ${residuals.length} disclosed residuals)`);
}

mainAsync().catch((error) => {
  console.error(error);
  process.exit(1);
});
